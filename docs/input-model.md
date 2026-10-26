# lys-ui input model

lys-ui's input semantics, modeled on GTK's event controllers and seat
grabs: the contract behind `wlsession` -> `app` -> `widget.Router`.
Change the code and this document together.

## Event flow

    compositor
      |  wl_pointer / wl_keyboard / wl_data_device events
      v
    wlsession.Session            ONE dispatch path: Session.Step and
      |  routed per surface      Session.Roundtrip are the only readers
      v                          of the connection; nothing else reads it.
    wlsession.SurfacePointerHandler   (one per surface)
    wlsession.SurfaceDropHandler      (one per surface, via internal/dragdrop)
      v
    app.surfaceInput / popup.popupInput
      v
    widget.Router                hover, grab, click, drag, focus, scroll
      v
    widget callbacks (OnClick, OnChanged, InsertRune, ...)

There is a single event source: the session owns the connection and
every pointer event is routed to the handler registered for the surface
it belongs to (`Session.SetSurfaceInput`). Surfaces never swap global
hooks; nested loops (popup.Run) stay safe because routing, not hook
replacement, decides who receives an event.

## The parked loop

The app loop parks in `Session.Step` (one blocking dispatch) instead of
polling. Wakeups come from exactly three sources:

- compositor events: pointer, keyboard, frame callbacks, configure,
  buffer releases;
- timer deadlines — key repeat, animation frame deadlines (a running
  tween wakes one frame period past the last tick; a finished one
  schedules nothing), tooltip dwell — each
  armed through `Session.WakeAfter`, which kicks the parked read with a
  `wl_display.sync` from the timer goroutine;
- nothing else. With no dirty state and no pending timers the loop
  schedules no kick and holds no CPU.

Redraws happen only when dirty, paced by the frame callback. A frame
that never completes (fully occluded surface) leaves the loop waiting
for events rather than spinning: dirty input still forces the next
redraw, a running animation takes over the pacing itself once its
frame callback stays unanswered past a frame period and a half (the
animation clock's timer keeps the tween moving), and configure
size changes mark the frame dirty themselves.

A timeout-based wake without a timer (a single-shot wait) goes
through `WakeAfter`; a raw `Roundtrip` is for handshakes that must
complete before proceeding (popup configure).

## Pointer focus and routing

- The compositor's `wl_pointer.enter` names the surface under the
  pointer; the session records it as *pointer focus*. Enter, motion,
  button, and axis events route to the focus surface's handler with
  coordinates in that surface's logical space.
- `wl_pointer.leave` clears focus and notifies that surface. Popups and
  tooltips that must not take input defend themselves with an empty
  input region (tooltips), so the compositor never routes to them.

## Implicit grab

- A button **press** opens a client-side implicit grab on the focus
  surface. While grabbed, motion, further buttons, and axis route to
  the grabbing surface even if the compositor would move focus — drags
  keep feeding the widget the drag started on.
- A **release** of the grabbing button ends the grab. Press/release
  pairs of different buttons may nest conceptually; lys-ui tracks a
  single grab because its widgets act on the primary button.
- This mirrors GTK: `gdk_seat_grab` semantics at the toolkit level,
  with the compositor's own popup grab (xdg_popup.grab) handling
  click-away dismissal above the client.

## Click, double-click, drag

- A click is a press and release on the same widget (`Clicker`).
- A second click within 400ms on a `DoubleClicker` is a double-click;
  widgets without double-click behavior treat each release as its own
  click.
- Dragging is motion while pressed (`DragMover`); the grab guarantees
  delivery. There is no slop threshold for widget drags (sliders must
  track from the first pixel), but the demo's chrome drag-move uses
  `xdg_toplevel.move`, whose threshold is the compositor's.
- A scroll-drag distinction therefore does not arise inside a widget:
  axis events route to the grab during a drag (scroll-follows-drag),
  and to the hover chain otherwise.

## Drag and drop (wl_data_device)

Data-device drag and drop runs beside the pointer path on the same
session, and beside the clipboard, which keeps the same device's
*selection*: offers are tracked per object, so a drag's offer never
touches the selection and vice versa.

- **Source.** A widget declares content with `widget.DragSource`:
  mimes best first plus a provider that writes the bytes for one mime
  on demand. A press on the widget plus motion past the app's
  threshold (8 logical px) starts `data_device.start_drag` with the
  press serial (the implicit grab) and a drag icon surface — a
  standalone snapshot of the source widget; failures degrade to the
  compositor's fallback icon. A drag that started cancels the click:
  the router press is dropped, so the release never fires `Clicker`.
- **Destination.** The session routes `data_device.enter/motion/
  leave/drop` to the handler registered for the named surface
  (`Session.SetSurfaceDrop`); enter names the surface, motion and
  drop broadcast to every registered handler, and only the surface
  holding the drag acts. The router bubbles from the deepest widget to
  the nearest `DragEnterer`, the same way scroll finds its handler;
  the target decides by mime — return the mime to accept, "" to
  reject — and `DragOverSetter` carries the highlight, lit only for
  accepted drags. Crossing between targets inside one surface
  retargets on hover (the compositor only sends enter on surface
  changes).
- **Serials.** `start_drag` carries the press serial;
  `data_offer.accept` carries the enter serial on every enter and
  whenever the accepted mime changes. Every other request the
  protocols anchor to a user action - popup grabs, interactive move
  and resize, `set_selection`, activation tokens - carries the
  session's latest input serial (`Session.InputSerial`): the last
  pointer button press, key press, touch down, or tablet tool press
  on any of the client's surfaces. Apps never pass serials.
- **Payload.** On drop the app fetches the bytes for the accepted
  mime. A drag that originated in the same process short-circuits the
  wire: the provider hands over the bytes directly and the source
  concludes locally — no `receive`/`finish` round-trip, and no
  deadlock, since both ends share one connection. Cross-process drops
  read through the offer pipe (request, flush, read to EOF) and, on
  data-device version 3+, acknowledge with `finish`; below version 3
  there is no `dnd_finished`, so the source concludes on
  `dnd_drop_performed`.
- **Version.** The session binds `wl_data_device_manager` capped at
  version 3 and reports the negotiated version
  (`Session.DataDeviceVersion`); `set_actions`/`finish` and the dnd
  action events exist only from version 3, and the controller gates
  them accordingly (both sides declare the copy action there).

## Click-to-focus

Pressing any widget moves keyboard focus to it (`Router.Press` sets
focus from the hit test), matching GTK click-to-focus. Tab and
shift+Tab traverse focus (`FocusNext` / `FocusPrev`); shift+click
extends a text selection (`Entry` / `TextArea` handle shift+left /
shift+right and shift+click anchor).

## Tree mutation and router state

The container APIs mutate the retained tree: `Box.Remove` (by
identity), `RemoveAt`, `Clear`, `InsertAt`, `Stack.Remove`,
`Scroll.SetChild`, plus `Grid.Remove` and `Notebook.CloseTab`. Every
mutation drops the measure cache and invalidates the subtree
(`InvalidateLayout`), clears the removed widget's parent link (it can
be re-appended elsewhere, never double-parented), and fires the
removal hook (`widget.SetRemovedHook`) while the widget is still
linked, so a router can find the focus-traversal neighbor.

The application points the hook at every window router
(`Router.Forget`): hover clears, the active press is cancelled without
a click, the drop target clears, and focus — even on a descendant of
the removed widget — moves to the next focusable widget like a Tab
would, or nowhere. No key lands in a dead widget, and since tooltips
follow the router's hover, no tooltip dwells on a ghost.

`Children()` returns snapshots (`Box`, `Overlay`, `Grid`), so tree
walks — damage, focus traversal, a11y — survive a callback that
mutates the tree mid-traversal.

## Disabled and read-only

- `SetEnabled(false)` on any widget makes it inert: no hover shade, no
  press, no drag, no wheel, no keys, and traversal skips it. A press
  on a disabled widget is swallowed whole — it does not even move
  focus. A disabled control still counts as interactive
  (`widget.IsInteractive`): it consumes the press, it is not chrome.
- Propagation is a **per-query ancestor walk** (`widget.IsEnabled`),
  never a recursive flag rewrite: disabling a container (`Box`,
  `Scroll`, `Grid`, ...) disables the subtree by query while each
  widget keeps its own flag. Re-enabling the container therefore never
  resurrects a child the app disabled on purpose.
- Focus never rests on a disabled widget: if the focused widget is
  disabled (directly or through a container), the next key delivery
  moves focus to the next focusable widget instead of feeding keys
  into a dead control.
- Read-only (`Entry.SetReadOnly` / `TextArea.SetReadOnly`) is a text
  editing gate, not an input gate: typing, paste, drop, composition,
  cut, undo/redo, and Tab indentation no-op, while selection, copy,
  caret motion, and pan keep working. The undo history built before
  the flip is untouched, not cleared — read-only back off and the same
  edits are undoable again. `SetText` stays programmatic and
  applies. Visually: a disabled text widget fades its text; a
  read-only one keeps the text and fades only the caret.

## Hit testing and z-order

- `HitTest` returns the deepest widget whose area contains the point.
- Overlay paints all children; its hit test must return the **last
  (topmost)** child containing the point, never the first. Pinned by
  the "topmost child wins the hit test" case in TestOverlay
  (widget/widgets_test.go).
- Containers return themselves when the point is inside their padding,
  so presses on chrome within a control still belong to the control.

## Keyboard

- Keymap-driven text goes to the focused widget (`RuneHandler`);
  editing keys map to `KeyAction`s; ctrl+c/x/v/a act on the focused
  widget's selection when a clipboard is configured; Escape is an app
  keybinding (`Config.OnKey`).
- Key repeat is synthesized on the compositor's reported rate/delay.
- Tab trap: a plain Tab is indentation inside a widget that implements
  `widget.TabTrapper` (multi-line text areas); ctrl+Tab and shift+Tab
  always move focus, so a focused TextArea can contain tab characters
  while Tab-based traversal keeps working everywhere else.
- shift+click extends a text selection (`Entry` / `TextArea` handle
  shift+left / shift+right and shift+click anchor).
- Keyboard input is seat-wide (one focused window), unlike pointer
  input which routes per surface.

## Text wrapping

`TextArea` soft-wraps by default. All painting, vertical motion, click
mapping, and Home/End resolve through a visual-row cache
(`line, startCol, endCol` per painted row) built from shaped advance
widths at the offered width; Left/Right and every edit operation stay
in logical coordinates, which remain canonical for `Text()`. A row
always holds at least one rune, so an unbreakable token wider than the
viewport wraps rune-by-rune instead of vanishing, and a zero wrap
width degenerates to one rune per row.

## Input methods (zwp_text_input_v3)

When the compositor advertises `zwp_text_input_manager_v3` the session
binds one text input per seat; the global is optional, so compositors
without it keep the plain keyboard path — every text-input call is a
no-op.

`app.Application` drives the input method from the focused widget once
per loop iteration: focusing an editable widget (Entry, TextArea) sends
enable with the widget's surrounding text (caret and anchor as byte
offsets, composing text excluded) and caret rectangle (surface
coordinates, for the candidate window); focusing anything else sends
disable. Pushes are deduped against the last state; local changes (all
routing, not just keys) carry `set_text_change_cause: other` to ask the
input method to drop its composing state.

done events assemble the double-buffered batch and the controller
applies it to the focused widget in protocol order: IMEDelete removes
the requested bytes around the caret, IMECommit inserts the commit
string through the widget's normal Insert path, and IMEPreedit shows
the next composing text. A stale done serial still applies — committed
text is never dropped — but skips the state re-push.

While composing, the composing text is display-only: `Text()` and
`OnChanged` stay untouched until a commit lands, and surrounding text
keeps excluding it. It renders at the caret with an accent underline;
backspace trims its last rune; any other edit or reposition drops the
local display (the cause=other resync tells the input method).
Composing over a selection removes the selection, as the protocol's
done ordering prescribes. TextArea builds its visual-row cache from
the composed display, so soft wrap stays consistent while composing.

## Cursors

`Session.SetCursor` takes one vocabulary - CSS names (`text`,
`pointer`, `ns-resize`), the toolkit's `resize_*` spellings, and the
classic xcursor names. When the compositor offers cursor-shape-v1 the
shape goes to it as one `set_shape` per pointer enter or change: the
compositor draws its own cursors, animated shapes included, with no
theme lookup and no buffer. The client xcursor theme is the fallback -
compositors without the protocol, the hidden cursor, and names the
protocol does not carry - and `LYS_UI_NO_CURSOR_SHAPE=1` forces it (the
headless suite runs both paths).

## Pointer lock, confinement, relative motion

`Application.LockPointer(host, persistent)` locks the pointer in place
over a host (an FPS-style view, a whiteboard pan) and
`ConfinePointer(host, region, persistent)` keeps it inside logical rects
of it (zwp_pointer_constraints_v1). The compositor activates a
constraint while the pointer is over the host and reports each
activation (`PointerConstraint.OnActive`); a oneshot constraint ends
with its first deactivation, a persistent one re-activates when the
pointer returns. A host holds one constraint at a time, and closing the
host releases it. `OnRelativeMotion(host, fn)` delivers the device's
motion while the pointer is over the host - accelerated and raw deltas
(zwp_relative_pointer_v1) - which is the only motion a locked pointer
produces, and what a remote desktop client forwards.

Escape hatch: the compositor does not force-release a constraint on a
key of its own, so an application that locks the pointer must offer
the way out - by convention an accelerator (Escape) that calls
`PointerConstraint.Release` - and should release when it loses focus.
`SetCursorHint` says where the cursor reappears when a lock ends.

## Touch and gestures

lys-ui binds wl_touch. Every contact routes to the surface it went
down on and holds its own implicit grab there, independent of the
pointer's; a wl_touch.cancel ends every surface's contacts.

`widget.TouchTracker` turns contacts into input the toolkit already
speaks:

- One contact emulates the pointer through `TouchPointer`: down is a
  left press, motion a drag, the lift a release (a tap clicks), then the
  emulated pointer leaves (a finger does not hover). App windows emulate
  through their real pointer path, so a finger drags a CSD header or
  starts a drag source; popups emulate straight onto their router.
- A contact held still (within 8 px) for 500 ms offers
  `GestureLongPress` up the tree; a claimer cancels the press, so no
  click follows.
- A second contact cancels the emulated press and makes the pair a
  gesture: `GesturePinch` (scale and rotation from the start) for a
  widget that claims it, otherwise a two-finger scroll down the
  precise-axis path (content follows the fingers), ending in the
  kinetic glide.

Touchpads speak wp_pointer_gestures v1-v3: swipe, pinch and hold arrive
at the pointer's surface (staying with the surface they began on) and
route as `Gesture` events. `Router.Gesture` delivers a Begin to the
widget under the point and up its ancestors until one claims it, then
keeps the rest of that gesture on the claimer; `GestureHandler` is the
widget side. The carousel claims swipes; BottomSheet and every
drag-driven widget work under a finger through the pointer emulation.

Testing: there is no standard virtual-touch protocol for the headless
compositor to accept, so touch is tested where it enters - synthetic
wl_touch and gesture events driven into the session handlers
(internal/wlsession) and contact sequences driven into the tracker
(widget) - rather than injected from the harness side.

## Tablets

zwp_tablet_v2 tools (pens, erasers, brushes, ...) accumulate their
events over a frame - proximity, motion, pressure, distance, tilt,
rotation, tip down/up, stylus buttons - and each frame routes to the
surface the tool is near (pads are not bound). The bridge every input
surface embeds (internal/touchinput) makes the tool a pointer - in
range it hovers, the tip presses and releases (through the same
`TouchPointer` path as touch, so window-frame grabs work), leaving
range leaves - and delivers a `Stylus` sample (pressure and distance
0..1, tilt and rotation in degrees, the tip state, a changed stylus
button) to the first `StylusHandler` up the tree: the pressed widget's
while a stroke is held, so the stroke and its end stay with the widget
it began on, else the hovered one's. `widget.DrawingArea` is the
painting surface that takes them. Like touch, tablets are tested by
synthetic tool events at the session handlers and tablet frames into
the bridge - the protocol has no virtual tablet to inject from the
harness.

## Seat capability churn

Capabilities are state, not a one-shot: when the compositor reports a
capability lost, the session drops the matching object and any routing
state (focus, grab) that depended on it; a later gain creates a fresh
object. This keeps input alive across virtual-device cycles, tablet
mode switches, and unplug-replug.

## Debug traces

lys-lib's `debug` (build tag `lysdebug`, categories via LYS_DEBUG)
traces the path at its seams, and `app.Trace` lets an application
trace its own events into the same timeline: `wire`-level pointer events with surface
ids in `wlsession`, routing decisions and hit bounds in `app`, frame
pacing under `frame`. Without the tag every call compiles out.

Traces are not logging. The library itself is silent by default: an
embedding application installs its `*slog.Logger` once
(`app.SetLogger`; nil, the default, discards everything) and only three things flow through it: Warn for
degraded-but-running (an advertised optional protocol whose bind
failed, a theme below WCAG AA), Error for terminal conditions (the
compositor raising a fatal protocol error), and Debug for protocol
chatter (optional globals the compositor does not advertise,
capability changes). Nothing logs per-frame or per-keypress, and
nothing logs at Info. The demo binaries keep their own
`log.Printf` — they are apps, not the library.
