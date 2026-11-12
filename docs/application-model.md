# Application model

Many windows in one process, on one connection and one loop.

## Pieces

| Type | Is |
| --- | --- |
| `app.Application` | One per process. Owns the loop, key repeat and the window list. `Quit` ends the loop; `Run` returns `ErrClosed` after `Quit` or when the last window closes, and closes the application. `Close` does the same for an application that never ran: it stops the font warm-up, the watchers and `OnStop` hooks, the animations still running, the desktop settings monitor and its session bus connection, the notifier, the portal sessions and the GPU renderer, and takes out the hooks `NewApplication` installed for the widgets (image loads, stylesheet hot reload, the error bell, the video and GPU surfaces), leaving alone those a later application installed in their place. |
| `app.Window` | An `xdg_toplevel` from a `WindowConfig`. `SetCloseRequest` can veto the compositor's close request; `Close` always closes. |
| `app.LayerWindow` | A `zwlr_layer_surface_v1` from a `LayerConfig`: layer, anchors, margins, exclusive zone, keyboard interactivity, namespace. It closes when the compositor closes it or its output goes away. Without layer shell, `NewLayer` fails with `ErrLayerShellUnavailable`. |

Every window has its own dirty flag, buffer pool, frame callback and
tooltip state. Pointer input routes by surface; keyboard input goes to
the window with keyboard focus. A closed window is reaped and its
`OnClosed` hook runs. Bars are ordinary layer windows, one per output.

| relm4 / GTK | lys-ui |
| --- | --- |
| `RelmApp`, `gtk::Application` | `app.Application`: construct, hook, `Run`; no `startup`/`activate` split |
| `ApplicationWindow` | `app.Window` from a `WindowConfig` |
| gtk4-layer-shell | `app.LayerWindow` from a `LayerConfig` |
| `close-request` | `Window.SetCloseRequest`; return false to veto |
| `gtk::Application::quit` | `Application.Quit`; ignores vetoes |
| GApplication single instance | `app.ClaimInstance` |
| `FileDialog` | `Application.OpenFileDialog` and friends |
| `UriLauncher`, `FileLauncher` | `Application.OpenURL`, `OpenPath`: the OpenURI portal with an activation token, else `xdg-open` |
| `scale-factor` | per-window scale, following `preferred_scale` live |

The component layer is in [Components](components.md); the full
capability map is [Completeness](completeness.md).

## Outputs and scale

`Session.Outputs()` lists the current outputs. `Session.OnOutputAdded`
fires for outputs that appear later; `Session.OnOutputRemoved` fires
when one goes away, and a layer surface pinned to it gets its closed
event (`OnClosed`).

`LayerConfig` with zero `Scale` uses the output's integer scale. With
`wp_viewporter` and `wp_fractional_scale_manager_v1`, the compositor's
`preferred_scale` overrides it live and buffers resize in place.

## Compositor disconnects

A lost connection surfaces as `*wlsession.DisconnectError`, matched by
`errors.Is(err, app.ErrDisconnected)`, with a reason:

| Cause | Reason |
| --- | --- |
| EOF or a dead-socket errno (EPIPE, ECONNRESET, ENOTCONN, EBADF, ...) on read or write | `DisconnectConnectionLost` |
| a fatal `wl_display.error` | `DisconnectProtocol` |
| anything else | not a disconnect; returned as is |

`Application.OnDisconnect` fires on the loop, at most once per
connection, before teardown, with `DisconnectedEvent{Reason, Err,
Reconnecting}`. Then `Run` releases every window, closes the session
(arena, fd, display), stops the invoke queues and returns the error.

The expected handling is a clean exit with `app.DisconnectExitCode`
(75, EX_TEMPFAIL), which systemd `Restart=on-failure` restarts:

```go
if errors.Is(err, app.ErrDisconnected) {
	os.Exit(app.DisconnectExitCode)
}
```

### Reconnect

`Application.SetReconnect(&app.ReconnectOptions{...})` rebuilds instead
of exiting when the connection was lost without a protocol error and
every window came from `NewWindow`, `NewLayer` or a dialog. Otherwise
`Run` returns the error.

1. `OnDisconnect` fires with `Reconnecting` true. Windows are set
   aside, popovers close (their `OnClosed` fires), and the old session
   is torn down.
2. `Run` dials again (`ReconnectOptions.Connect`, default
   `wlsession.Connect`), backing off from 50 ms to 1 s, for
   `ReconnectOptions.Timeout` (default 10 s).
3. Every window reopens on the same `*Window` or `*LayerWindow` handle
   with its title, size, limits, maximized and fullscreen state, layer
   anchor, margin, zone, layer, keyboard mode, content type, opacity and
   output (matched by xdg-output name). Transient parents, dialog
   modality, window icons, toasts and keyboard focus are restored.
   Widget trees, queues, timers and accelerators are untouched.
   `ReconnectOptions.OnReconnected` then runs on the loop.

Lost on reconnect: clipboard contents, drags, popovers, tooltips,
session locks, idle inhibitors and watches, pointer constraints. Read
the current session from `Application.Session`; key handlers use
`Application.KeySym`.

## Single instance

`app.ClaimInstance` makes the first process for an AppID the primary,
listening on a unix socket under `XDG_RUNTIME_DIR`. Later processes
forward argv, `--open` paths, working directory and activation token,
then exit without connecting.

```go
ins, primary, err := app.ClaimInstance(app.InstanceConfig{
	AppID:         "dev.example.app",
	OnCommandLine: func(args []string, cwd string) { ... },
	OnOpen:        func(paths []string, cwd string) { ... },
}, app.OSInvocation(os.Args[1:]))
if err != nil {
	return err
}
if !primary {
	return nil
}
defer ins.Close()
application := app.NewApplication(sess)
ins.Bind(application)
```

- Hooks run on the loop. Invocations that arrive before `Bind` are
  buffered.
- An invocation with paths fires `OnOpen` only, else `OnCommandLine`.
- An `XDG_ACTIVATION_TOKEN` from the launcher is used after the hooks to
  focus the application.
- A stale socket from a crashed primary is reclaimed. Two simultaneous
  launches resolve to one primary.
- `AllowMultipleInstances` disables the guard.

`cmd/lys-ui-messages` is the example: a second run toasts in the
first's windows and exits.

## File dialogs

`Application.OpenFileDialog`, `OpenFilesDialog`, `OpenFolderDialog` and
`SaveFileDialog` show `widget.FileChooser`, a pure-Go picker (no
portal), in a standard dialog:

- a places row (Up, Home, Filesystem, and Recent when recents are on),
  the listing, pattern filters, and a name row when saving;
- OK stays refused until the choice is valid
  (`DialogConfig.ValidateResponse`);
- saving over an existing file asks first, unless
  `SkipOverwriteConfirm`;
- the result arrives once, on the loop, in `OnOpen` or `OnSave`.

`FileChooser.SetSource` swaps the listing backend (`DirSource`).
Recents are the XDG `recently-used.xbel`, shared with other desktop
apps and written atomically. An environment where neither
`XDG_DATA_HOME` nor `HOME` is an absolute path keeps no list.

## Clipboard and drag and drop

All transfers use `transfer.Content`: the MIME types offered, best
first, each with a writer. Builders:

| Builder | Offers |
| --- | --- |
| `Text` | plain text |
| `Image` | PNG, plus a JPEG flattened onto white |
| `Files`, `URIs` | `text/uri-list` and the URIs as plain text |
| `HTML` | HTML with a plain-text fallback |
| `Merge` | several of the above |

`Application.Clipboard()`:

- writes with `Write`, `WriteText`, `WriteImage`, `WritePrimary`, and
  `WriteExpiring` (clears the selection after a TTL if still owned);
- reads with `Read(prefs, done)`, `ReadText`, `ReadImageBytes`,
  `ReadURIs`, `ReadHTML` and `ReadPrimary`; `Offers` tests for a MIME
  type.

Reads take a callback: the transfer runs off the loop and `done` runs
on the loop. A ctrl+v result lands in the widget focused at the press;
a drop lands in the widget under it. Writers run off the loop and must
not touch widgets. At most 8 transfers run at once; payloads are capped
at 16 MiB. Failed transfers (oversize, stalled, broken pipe) reach
`Application.SetTransferErrorHandler`; an empty clipboard is not an
error.

A drag is a `transfer.Drag` (`widget.DragContent`):

- content plus the offered actions (copy by default; a row reorder
  offers move);
- `OnFeedback` reports the destination's acceptance and the negotiated
  action;
- `OnDone` reports the final action: `ActionMove` means delete the
  source data, `ActionNone` means cancelled.

A drop target chooses among offered actions with
`widget.DropActionChooser`; an "ask" drop uses that choice (no drop
menu). Escape cancels the app's own drag when the compositor delivers
the key; `Application.CancelDrag` cancels from code.

A non-text paste target implements `widget.ContentPaster`;
`widget.Image` pastes images that way. A middle-click paste over a
read-only text widget is refused with the error bell.

## Window chrome

| Widget | Role |
| --- | --- |
| `widget.HeaderBar` | client-side title bar: title, subtitle, start and end packs, `WindowControls` |
| `widget.MenuBar` | in-window menu bar opening menu popovers |
| `widget.ActionBar` | bottom bar |

A press on the header's background starts `xdg_toplevel.move` (any
widget can implement `WindowMover`); a double press toggles maximize.
`Application.AttachHeader(win, bar)` wires the controls and double press
to the window.

Decorations: lys-ui asks for server-side decoration when the compositor
offers xdg-decoration. A `HeaderBar` in the tree is the opt-in to
client-side chrome; without one, windows are undecorated. Minimize and
maximize buttons hide when the compositor's `wm_capabilities` excludes
them.

`Window` requests:

| Method | Does |
| --- | --- |
| `SetTitle`, `Minimize`, `ToggleMaximize` | the runtime setters the controls call |
| `BeginMove`, `BeginResize(edge)` | start the compositor's grab from the current press |
| `FullscreenOn(output)` | fullscreen on one output |
| `SetTransientFor(parent)` (or `WindowConfig.Parent`) | keep the window above its parent |
| `RequestAttention` | an xdg-activation request without user interaction; compositors mark the window urgent |

Each activation request uses its own token.
