# lys-ui application model

How lys-ui composes many windows in one process, and the relm4 and
gtk4-layer-shell concepts it borrows.

## Pieces

- `app.Application` — one per process. Owns the parked event loop, the
  key repeater, and the window list. `Quit` ends the loop; `Run`
  returns `ErrClosed` when quitting or when the last window closed.
- `app.Window` — a toplevel (`xdg_toplevel`) declared through
  `WindowConfig` and driven by the application. `SetCloseRequest`
  vetoes the compositor's close request (unsaved-changes prompts);
  client-side `Close` always wins.
- `app.LayerWindow` — a layer surface (`zwlr_layer_surface_v1`)
  declared through `LayerConfig`: layer, anchors, margins, exclusive
  zone, keyboard interactivity, namespace. No close request exists in
  the protocol; the surface dies when the compositor closes it or its
  output disappears.

## Compositor disconnects

Wayland has no reconnection: when the compositor restarts (crash,
reload, upgrade), the kernel kills every client's socket and every
proxy object on the connection dies with it. lys-ui turns that from
"whatever a dead socket read does" into a defined story.

**Detection.** Every dispatch surface — the loop's park
(`Session.Step`), `Session.Roundtrip`, and the frame path's
attach/damage/commit — maps wire failures onto one typed error,
`*wlsession.DisconnectError` (matchable as `app.ErrDisconnected` via
`errors.Is`), carrying a classified reason:

| wire shape | reason |
| --- | --- |
| EOF on the socket read (`ErrContextRunConnectionClosed`) | `DisconnectConnectionLost` — the compositor went away |
| dead-socket errno on read or write (EPIPE, ECONNRESET, ENOTCONN, EBADF, …) | `DisconnectConnectionLost` |
| fatal `wl_display.error` recorded, or a protocol-error marker in the dispatch failure | `DisconnectProtocol` |
| anything else (timeout, decode failure with no verdict) | not classified — returned as-is; not every wire hiccup is a disconnect |

The destroyed-proxy retry inside dispatch (`ErrContextRunProxyNil`, a
normal mid-queue abort) is bounded, so no error shape — a dead fd chief
among them — can turn a roundtrip or the park into a spin.

**Policy.** `Application.OnDisconnect` fires exactly once, on the loop goroutine, before
teardown, with `DisconnectedEvent{Reason, Err}`. After it returns, Run
releases every window (buffer pools included), closes the session — the
shared arena's memfd, mapping, and fd, and the display — and shuts the
invoke queues, so the exiting process leaves nothing mapped and no
timer or queued fn reaches for a dead connection. Run then returns the
disconnect error.

The sanctioned behavior is a **clean exit**: `main` matches
`errors.Is(err, app.ErrDisconnected)` and exits with
`app.DisconnectExitCode` (75, EX_TEMPFAIL), which systemd
`Restart=on-failure` — or a wayle supervisor — treats as respawn-us on
the new session. The hook itself is optional observation (flush state
before the teardown, never block indefinitely); without it the same
clean exit runs.

**Reconnect with rebuild.** `Application.SetReconnect` turns a
lost connection into a rebuild instead of the exit, for the case it can
honestly cover: the connection was lost without a protocol verdict (a
compositor that killed the client would do it again) and every window
came from `NewWindow`, `NewLayer`, or a dialog (a host built outside
the application cannot be rebuilt, so its loop exits with the
disconnect error).
`DisconnectedEvent.Reconnecting` tells the hook which way it goes.

1. The policy hook fires, the windows are set aside with their toasts,
   open popovers close (their `OnClosed` fires), and the usual teardown
   runs - every proxy died with the socket.
2. Run dials again (`ReconnectOptions.Connect`, default
   `wlsession.Connect`), backing off from 50ms to a second until
   `Timeout` (ten seconds). The new session is adopted: dispatch and
   wake seams, drag and drop, the input method, key repeat, the
   clipboard handle (`Clipboard.Reset`, empty on the new session), and
   the session hooks.
3. Every window reopens through the same path that opened it, on the
   same handle - an app's `*Window` and `*LayerWindow` stay valid -
   from its config updated with what the old objects carried: title,
   size, size limits, maximized and fullscreen, layer anchor, margin,
   zone, layer, and declared keyboard mode, content type, opacity, and
   the output a layer was pinned to, re-resolved by its xdg-output name.
   Then transient parents and dialog modality re-link across the
   rebuilt set, window icons are posted again from their source images,
   toasts move onto their rebuilt windows, and keyboard focus inside
   each tree is restored. Widget trees, queues, timers, and
   accelerators never left.

Lost, because it was the compositor's: clipboard contents, drags,
popovers and tooltips, session locks, idle inhibitors and idle watches,
pointer constraints, and pending configure serials. Code holding the
`*wlsession.Session` reads the current one from `Application.Session`;
key handlers use `Application.KeySym` and never hold one.

**Testing.** Unit: dead-socket classification, the bounded retry, the
exactly-once hook, the pool/queue/fd teardown, and the goroutine budget
(app and wlsession packages). Live: `internal/headlesstest`'s kill test
boots the headless session, runs the showcase, SIGKILLs sway mid-run,
and asserts one policy trace and exit code 75; the reconnect test
kills sway under a client with `SetReconnect`, boots it again on the
same socket, and asserts the policy trace, the rebuilt window taking
keyboard input on the new session, and no file descriptor left over
from the old one.

## Loop sharing

All windows live on one connection and one parked loop (see
docs/input-model.md). Each window carries its own dirty flag, buffer
pool, frame callback, and tooltip state machine; input routes by
surface, keyboard routes to the window holding the seat's keyboard
focus. A window that closes is reaped and its `OnClosed` hook runs.

## Multi-output

The session tracks `wl_output` globals in arrival order
(`Session.Outputs`) and exposes hotplug hooks:

- `Session.OnOutputAdded` — fires for outputs that appear after the
  hooks are installed; enumerate `Session.Outputs()` at startup for the
  initial set.
- `Session.OnOutputRemoved` — fires when an output's global goes away.
  A layer surface pinned to that output receives the layer-shell closed
  event, so its `OnClosed` is the spawn-cleanup path.

Per-output scale comes from the output's integer scale; `LayerConfig`
derives it when `Scale` is zero. With the fractional-scale protocols
(`wp_viewporter` plus `wp_fractional_scale_manager_v1`) the compositor's
`preferred_scale` — 1.25 and friends included — overrides the integer
scale live: buffers resize in place, no window recreation. Compositors
without the protocols keep the integer behavior.

## Single instance and remote activation

`app.ClaimInstance` is the GApplication single-instance guard:
the first process to claim an AppID becomes the primary and listens on
a unix socket under `XDG_RUNTIME_DIR`; every later process forwards
its invocation - argv, `--open` paths, working directory, activation
token - and exits with status 0 without ever connecting the session.

```go
ins, primary, err := app.ClaimInstance(app.InstanceConfig{
    AppID: "dev.stubbe.lys.ui.messages",
    OnCommandLine: func(args []string, cwd string) { ... },
    OnOpen:       func(paths []string, cwd string) { ... },
}, app.OSInvocation(os.Args[1:]))
if err != nil { return err }
if !primary { return nil } // forwarded to the primary; exit 0

defer ins.Close()
application := app.NewApplication(sess)
ins.Bind(application) // hooks run on the loop; buffered forwards flush
```

The hooks fire on the loop goroutine with event-callback guarantees;
invocations that arrive between the claim and `Bind` buffer and flush
through the first pump. An invocation carrying open paths fires
`OnOpen` alone (GApplication's open replacing command-line); a token
the launcher exported as `XDG_ACTIVATION_TOKEN` is spent after the
hooks to focus the application. A crashed primary leaves a stale
socket; the next claim dials it, gets no answer, and reclaims it.
`AllowMultipleInstances` opts out entirely (relm4's
`allow_multiple_instances`). The claim races safely between two
simultaneous launches: the socket bind is atomic, the loser forwards.

The worked example is `cmd/lys-ui-messages`: run it twice and the second
run toasts in the first's windows and exits. The compositor-in-the-loop
test is `TestSingleInstanceForwardsToPrimary`
(internal/headlesstest): two real processes, one wins, the loser's
`--open` lands in the winner's shell trace.

## File dialogs

`Application.OpenFileDialog`, `OpenFilesDialog`, `OpenFolderDialog`,
and `SaveFileDialog` wrap `widget.FileChooser` - the pure-Go picker
- in the standard Dialog: a places row (Up, Home, Filesystem, and
Recent when recents are on), the directory listing, pattern filters,
a name row in save mode, and an ok button that refuses to close the
dialog until the choice is valid (the `DialogConfig.ValidateResponse`
veto). With `ConfirmOverwrite` set, save asks before replacing an existing file;
choices land in `OnOpen`/`OnSave` on the loop goroutine, exactly once.

**The picker is portal-free by decision.** lys-ui ships every widget the
picker needs, a portal FileChooser adds a second UI to keep honest, and portal-less sessions (lab machines,
early boot, tests) get the same dialog. The seam is
`FileChooser.SetSource` - the listing comes from an injected
`DirSource`, so a different listing backend plugs in under the same
dialog API without touching the picker's UI. Recents are the
XDG `recently-used.xbel` list (`internal/recentfiles`), shared with
the desktop's other citizens, bounded and written atomically, with
their uris through the same `transfer.FileURI`/`URIPath` the
clipboard and drag and drop use.

## Clipboard and drag and drop

Every data transfer shares one shape, `transfer.Content`: the mimes a
payload is offered as, best first, and a writer per mime. The
builders encode up front - `Text`, `Image` (PNG plus a JPEG flattened
onto white, so image editors that only read JPEG paste too), `Files`
and `URIs` (`text/uri-list` with the uris as plain text), `HTML`
(formatted text with its plain fallback) - and `Merge` combines them.
`Clipboard.Write` claims the selection with any content;
`Clipboard.Read(prefs...)` reads the best mime offered, with
`ReadText`, `ReadImageBytes`, `ReadURIs` and `ReadHTML` on top.

A drag is a `transfer.Drag` (`widget.DragContent`): content plus the
actions it offers (copy unless set; a row reorder offers move),
`OnFeedback` for the destination's acceptance and the negotiated
action, and `OnDone` with the action the drop finished as
(`ActionMove`: delete the source data; `ActionNone`: cancelled). A
drop target picks among the offered actions through
`widget.DropActionChooser`; an ask drop settles on that pick, since
lys-ui shows no drop menu. Escape cancels the application's own drag
when the compositor delivers the key (most cancel on Escape
themselves), and `Application.CancelDrag` does it from code.

A non-text paste target implements `widget.ContentPaster` (its mimes,
best first); `widget.Image` reads images that way. Failed transfers -
a paste or drop whose peer exceeded `xfer.MaxPayload`, stalled past
the deadline, or broke the pipe - reach
`Application.SetTransferErrorHandler` for a toast; an empty clipboard
is not an error. A middle-click paste over a read-only text widget is
refused there, ringing the error bell, rather than redirected to the
keyboard focus.

## Window chrome

`widget.HeaderBar` is the CSD title bar - title/subtitle, start/end
packs, `WindowControls` (close/minimize/maximize buttons, shown when
asked and hooked by the app), `widget.MenuBar` the in-window primary
navigation opening the existing menu popovers, `widget.ActionBar` the
bottom bar. A press on the bar's background starts the
compositor's `xdg_toplevel.move` grab (the `WindowMover` contract any
widget can implement), a double press maximizes, and
`Application.AttachHeader(win, bar)` wires controls and double-press to the
window in one call - `Window.SetTitle`, `Minimize`, and
`ToggleMaximize` are the runtime setters the controls fire into.

**Decoration policy.** xdg-decoration stays as it is: a compositor
offering server-side decoration gets asked for it. A HeaderBar in the
tree is the app's explicit opt into client-side chrome - lys-ui windows
ship undecorated otherwise, the GTK4 default inverted. The two can
coexist (a compositor frame around an app-drawn header); the app is
always right about what it draws itself, the compositor about what it
draws around it. Minimize and maximize buttons follow the
compositor's `wm_capabilities`: shown until it declares what it
cannot do, hidden for that.

**Window requests from code.** `Window.BeginMove` and
`BeginResize(edge)` start the compositor's grabs from the press under
way (custom title areas, drag handles - the session supplies the
serial of the latest input event the protocol wants); `FullscreenOn(output)` names the monitor;
`SetTransientFor(parent)` (or `WindowConfig.Parent`) keeps a toolbox
above its owner, the relation dialogs get by construction; and
`RequestAttention` is the urgency hint: Wayland has none, so it is an
xdg-activation request with no interaction behind it, which
compositors answer by marking the window urgent rather than raising
it. Activation tokens are per request, so an attention request and a
URI launch never trade tokens.

## Concept mapping

| relm4 / GTK | lys-ui | note |
| --- | --- | --- |
| `relm4::RelmApp`, `gtk::Application` | `app.Application` | construct, hook, `Run`; there is no `startup`/`activate` split |
| `ApplicationWindow` | `app.Window` from a `WindowConfig` | a declarative config instead of builder chains |
| gtk4-layer-shell | `app.LayerWindow` from a `LayerConfig` | the same wire concepts: layer, anchors, margins, exclusive zone, keyboard mode |
| `gtk::Window::close-request` | `Window.SetCloseRequest` | return false to veto |
| `gtk::Application::quit` | `Application.Quit` | authoritative, bypasses vetoes |
| GApplication single instance, `command-line`, `open` | `app.ClaimInstance` | see Single instance above |
| `FileDialog`, relm4 `open_dialog`/`save_dialog` | the `Application.OpenFileDialog` family | see File dialogs above |
| `UriLauncher`, `FileLauncher` | `Application.OpenURL`, `OpenPath` | portal OpenURI with an activation token, xdg-open fallback |
| per-window `scale-factor` | per-window scale with live `preferred_scale` | see Multi-output above |

The component side of relm4 (components, workers, commands, factories)
is described in [components.md](components.md) and mapped in
[threading.md](threading.md#the-relm4-mapping). The widget-by-widget
map is [completeness.md](completeness.md).

## What is deliberately absent

Bars are windows like any other, one per output: there is no implicit
single-window shortcut for layer surfaces. The process-wide non-goals
(no per-window goroutines, no window manager) are in
[architecture.md](architecture.md#non-goals).
