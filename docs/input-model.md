# Input model

Input semantics follow GTK's event controllers and seat grabs.

## Event flow

    compositor
      |  wl_pointer / wl_keyboard / wl_data_device events
      v
    wlsession.Session                 Step and Roundtrip are the only
      |  routed per surface           readers of the connection
      v
    wlsession.SurfacePointerHandler   one per surface
    wlsession.SurfaceDropHandler      one per surface
      v
    app (window or popup input)
      v
    widget.Router                     hover, grab, click, drag, focus, scroll
      v
    widget callbacks                  OnClick, OnChanged, InsertRune, ...

Every pointer event goes to the handler registered for its surface
(`Session.SetSurfaceInput`). Keyboard input is seat-wide: it goes to
the window holding keyboard focus.

## Pointer

- `wl_pointer.enter` sets pointer focus; enter, motion, button and axis
  events go to that surface in its logical coordinates.
- Tooltips have an empty input region and never receive input.
- A button press opens an implicit grab on the focus surface: motion,
  further buttons and axis events stay there until a release, so a drag
  keeps feeding the widget it started on. The compositor's popup grab
  handles click-away dismissal.
- A click is a press and release on the same widget (`Clicker`). A
  second click within 400 ms on a `DoubleClicker` is a double-click.
- A drag is motion while pressed (`DragMover`), with no threshold.
- A primary press moves keyboard focus to the widget unless it set
  `SetFocusOnClick(false)`.
- `SetOnPointer` (any button, click count, modifiers) and `SetOnScroll`
  (steps, modifiers) work on every widget.

## Hit testing

- `HitTest` returns the deepest widget containing the point.
- `Overlay` returns its last (topmost) child containing the point.
- A container returns itself for points in its padding.

## Keyboard

- Text goes to the focused widget (`RuneHandler`). Editing keys map to
  `KeyAction`s through `widget.KeyActionForSym`. ctrl+c/x/v/a act on the
  focused widget's selection.
- Escape and other app keys go through accelerators or
  `WindowConfig.OnKey`.
- Key repeat is synthesized from the compositor's rate and delay.
- Tab and shift+Tab move focus (`FocusNext`, `FocusPrev`). A widget
  implementing `widget.TabTrapper` (`TextArea`) takes a plain Tab as
  indentation; ctrl+Tab and shift+Tab still move focus.
- shift+click and shift+arrows extend a selection in `Entry` and
  `TextArea`.

## Disabled and read-only

- `SetEnabled(false)` makes a widget inert: no hover, press, drag,
  wheel or keys, and traversal skips it. A press on it is swallowed and
  does not move focus.
- Disabling a container disables its subtree by query
  (`widget.IsEnabled`); each widget keeps its own flag.
- If the focused widget becomes disabled, the next key moves focus to
  the next focusable widget.
- Read-only text (`SetReadOnly`) blocks typing, paste, drop,
  composition, cut, undo/redo and Tab indentation. Selection, copy,
  caret motion and panning work. The undo history is kept.

## Tree mutation

`Box.Remove`, `RemoveAt`, `Clear`, `InsertAt`, `Stack.Remove`,
`Scroll.SetChild`, `Grid.Remove` and `Notebook.CloseTab` mutate the
tree. Each one invalidates layout, unlinks the removed widget (it can be
added elsewhere), and updates every window router: hover clears, an
active press cancels without a click, the drop target clears, and focus
inside the removed subtree moves to the next focusable widget.
`Children()` returns a snapshot, so walks survive mutation from
callbacks.

## Drag and drop

Drag and drop runs beside the clipboard on the same data device; drag
offers and selection offers are tracked separately.

- **Source.** `widget.DragSource` declares MIME types, best first, and a
  provider that writes one on demand. A press plus 8 logical px of
  motion starts the drag with a snapshot of the widget as the icon. The
  press does not produce a click.
- **Destination.** The router bubbles from the deepest widget to the
  nearest `DragEnterer`, which returns the MIME type it accepts or "" to
  reject. `DragOverSetter` shows the highlight for accepted drags.
  Moving between targets inside one surface retargets.
- **Payload.** A drop inside the same process skips the wire. Other
  drops read through the offer pipe and send `finish` on data-device
  version 3 or later.
- **Serials.** Apps never pass serials. Requests that need one use the
  latest input serial (`Session.InputSerial`).
- **Version.** `wl_data_device_manager` binds up to version 3
  (`Session.DataDeviceVersion`). Actions and `finish` need version 3.

See [Application model](application-model.md#clipboard-and-drag-and-drop)
for content, actions and errors.

## Text wrapping

`TextArea` soft-wraps by default. Painting, vertical motion, click
mapping and Home/End use visual rows; Left/Right and edits use logical
positions, which `Text()` reports. An unbreakable token wider than the
view wraps per rune.

## Input methods

With `zwp_text_input_manager_v3`, the focused `Entry` or `TextArea`
drives the input method:

- Focus sends enable with up to 4000 bytes of surrounding text
  (excluding composing text) and the caret rectangle; other focus sends
  disable.
- Local edits send `set_text_change_cause: other`.
- On `done`, deletes, commits and preedit apply in protocol order. A
  stale serial still applies the commit.
- Composing text is display-only: `Text()` and `OnChanged` change only
  on commit. It shows at the caret with an accent underline. Backspace
  trims it; any other edit or caret move drops it. Composing over a
  selection removes the selection.

Without the protocol, text input calls do nothing.

## Cursors

`Session.SetCursor` takes CSS names (`text`, `pointer`, `ns-resize`),
`resize_*` names and classic xcursor names. With cursor-shape-v1 the
compositor draws the cursor; otherwise, for the hidden cursor, and for
names the protocol lacks, the xcursor theme is used.
`LYS_UI_NO_CURSOR_SHAPE=1` forces the theme.

## Pointer lock and confinement

| Call | Does |
| --- | --- |
| `Application.LockPointer(host, persistent)` | locks the pointer over the host |
| `Application.ConfinePointer(host, region, persistent)` | keeps it inside logical rects of the host |
| `Application.OnRelativeMotion(host, fn)` | delivers accelerated and raw deltas while over the host |

Both return a `*PointerConstraint`. The compositor activates it while
the pointer is over the host (`OnActive`). A one-shot constraint ends at
its first deactivation; a persistent one reactivates. A host holds one
constraint; closing the host releases it. `SetCursorHint` sets where
the cursor reappears.

The compositor offers no escape key. An app that locks the pointer must
release it itself (`PointerConstraint.Release`, usually on Escape) and
should release on focus loss.

## Touch, gestures and tablets

- Each touch contact routes to the surface it went down on, with its
  own implicit grab. `wl_touch.cancel` ends all contacts.
- One contact emulates the pointer (`TouchPointer`): down presses,
  motion drags, lift releases (a tap clicks), then the pointer leaves.
- A contact held within 8 px for 500 ms offers `GestureLongPress` up
  the tree; a claimer cancels the press.
- A second contact cancels the press. `GesturePinch` (scale and
  rotation) goes to a widget that claims it; otherwise two fingers
  scroll, ending in a kinetic glide.
- Touchpad swipe, pinch and hold (pointer-gestures v1–v3) arrive as
  `Gesture` events. `Router.Gesture` offers a Begin to the widget under
  the point and its ancestors; the first `GestureHandler` to claim keeps
  the gesture. `Carousel` claims swipes.
- Tablet tools (zwp_tablet_v2; pads are not bound) act as a pointer:
  in range hovers, the tip presses. Each frame also delivers a `Stylus`
  sample (pressure and distance 0..1, tilt and rotation in degrees, tip
  state, changed button) to the first `StylusHandler` up the tree: the
  pressed widget during a stroke, else the hovered one.
  `widget.DrawingArea` takes them.

Losing a seat capability drops its object and routing state; regaining
it creates a fresh one.

## Debug traces and logging

lys-lib's `debug` package (build tag `lysdebug`, categories in
`LYS_DEBUG`) traces input, routing and frame pacing; `app.Trace` adds
app events to the same timeline. Without the tag, traces compile out.

The library logs nothing unless the app installs a logger with
`app.SetLogger` (nil discards):

| Level | For |
| --- | --- |
| Error | terminal conditions, such as a fatal protocol error |
| Warn | degraded but running: a failed optional bind, a theme below WCAG AA |
| Info | no GPU renderer opened |
| Debug | protocol details: missing optional globals, capability changes |

Nothing logs per frame or per key.
