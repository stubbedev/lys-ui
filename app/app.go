// Package app owns the shared event loop: buffer pooling, input routing
// into the widget tree, frame-callback pacing, and idle dispatch. A Host
// abstracts over layer surfaces and toplevel windows so either can carry
// a widget tree.
//
// Threading: all widget and callback work runs on the loop goroutine;
// other goroutines reach the loop only through Application.Invoke and
// Application.Every. The contract and the relm4 mapping are in
// docs/threading.md.
package app

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/neurlang/wayland/wl"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/clipboard"
	"github.com/stubbedev/lys-ui/internal/compose"
	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/internal/dragdrop"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/touchinput"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// ErrClosed reports that the surface ended and the loop exited. Callers
// that treat a plain close as success can match it with errors.Is.
var ErrClosed = errors.New("app: surface closed")

// Host is the piece of the shell a widget tree is hosted on: a layer
// surface or a toplevel window. Both types implement it.
type Host interface {
	// EnsureUsable gates drawing until the first configure completed.
	EnsureUsable() error
	// Closed reports whether the compositor or user ended the surface.
	Closed() bool
	// Size returns the current size in surface (logical) pixels.
	Size() (int, int)
	// HostSurface returns the underlying wl_surface.
	HostSurface() *wl.Surface
}

// Config describes an app run.
type Config struct {
	// Session is the wayland connection.
	Session *wlsession.Session
	// Host carries the widget tree.
	Host Host
	// Scale is the initial integer output scale the surface renders at;
	// zero means 1. With the fractional-scale protocol the compositor's
	// preferred scale (1.25 and friends) overrides this live.
	Scale int
	// Root is the widget tree.
	Root widget.Widget
	// Background fills the frame before the tree paints. An alpha of
	// 255 marks the surface fully opaque and the frame pipeline sets
	// wl_surface.set_opaque_region automatically; an alpha below 255
	// leaves the surface translucent so the compositor blends what is
	// behind it (compositor blur: panels keep alpha in the 200-235
	// range - see docs/architecture.md, rule 13).
	Background render.Color
	// Opaque forces the opaque region even though Background is
	// translucent - a promise the tree must keep: every pixel painted
	// fully opaque, or the compositor shows stale content behind the
	// surface. Leave false and let the background alpha decide.
	Opaque bool
	// OnPress, when set, fires after the router recorded a press with
	// the button and the widget under the pointer.
	OnPress func(button uint32, over widget.Widget)
	// OnPointerMove, when set, receives raw pointer positions in
	// surface (logical) coordinates, for anchoring context menus.
	OnPointerMove func(x, y float64)
	// TooltipFace renders hover tooltips; nil is the system UI face.
	TooltipFace *render.Typeface
	// Clipboard, when set, enables ctrl+c, ctrl+x, and ctrl+v on the
	// focused widget's selection, middle-click paste of the primary
	// selection, and (with CopyOnSelect) copy-on-select.
	Clipboard *clipboard.Clipboard
	// CopyOnSelect mirrors selection changes into the primary
	// selection (X11 style: select text, then middle-click elsewhere
	// to paste it). Default off; needs a configured Clipboard.
	CopyOnSelect bool
	// OnKey, when set, receives every key press (repeats included)
	// together with the router, for apps that map keycodes to typing
	// or actions.
	OnKey func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	// Inspect arms the debug inspector: the widget-tree overlay plus
	// the ctrl+shift+i (toggle) and ctrl+shift+d (dump) chords.
	// LYS_UI_INSPECT=1 arms it for every app; this field is the
	// in-code opt-in. See docs/inspector.md.
	Inspect bool
	// OnDisconnect observes the compositor connection dying (restart,
	// crash, reload): it fires exactly once on the loop goroutine
	// before Run releases the windows and closes the session, with the
	// classified reason. Run then returns an error matching
	// ErrDisconnected; exit with DisconnectExitCode so a supervisor
	// respawns on the new session. See docs/application-model.md.
	OnDisconnect func(DisconnectedEvent)
}

// surfaceInput routes one host surface's pointer events into the
// widget tree: hover tracking, press/release with click detection,
// drags, scroll, cursor shapes, and tooltip state. It implements
// wlsession.SurfacePointerHandler, so events arrive only when the
// compositor's pointer focus (or an active grab) belongs to this
// surface. Pointer coordinates are logical pixels and route into the
// widget tree unchanged - the device scale lives entirely in the paint
// pipeline.
type surfaceInput struct {
	// Input routes touch and touchpad gestures (internal/touchinput);
	// the surface itself is its TouchPointer.
	touchinput.Input
	sess    *wlsession.Session
	surf    *wl.Surface
	router  *widget.Router
	tip     *tooltipCtl
	onPress func(button uint32, over widget.Widget)
	// dismissPopover closes the autohide popover open over this
	// surface's window and reports whether it did.
	dismissPopover func() bool
	onMove         func(x, y float64)
	// onRelative hears the pointer's relative motion (OnRelativeMotion).
	onRelative func(dx, dy, rawDX, rawDY float64)
	// dnd, when set, starts drags from press+motion gestures and
	// receives this surface's data-device events (dragdrop.Target).
	dnd dragController
	// transferError hears a failed primary paste or drop
	// (Application.OnTransferError).
	transferError func(error)
	// frac reports the host window's current 120-based device scale,
	// for one-shot surfaces created from this input (drag icons).
	frac func() uint32
	// resizeAt maps a pointer position to the xdg_toplevel.resize_edge
	// bits under it (0 when interior); set only for toplevel windows.
	resizeAt func(x, y float64) uint32
	// startResize engages the compositor's resize grab; while both are
	// set, an edge press belongs to the window frame, not the widgets.
	startResize func(edges uint32, serial uint32)
	// startMove engages the compositor's xdg_toplevel.move grab for a
	// press on a WindowMover widget (a CSD header bar); set only for
	// toplevel windows. lastPress* feeds the double-click detection
	// that rides the same grab.
	startMove              func(serial uint32)
	lastPressAt            time.Time
	lastPressX, lastPressY float64
	// primary carries the primary-selection behavior (middle-click
	// paste, copy-on-select) shared by the application's windows.
	primary *primarySelection

	x, y       float64
	lastCursor string
	// cursorPin, when set, overrides the hover-derived shape for the
	// life of a gesture: edge resize and chrome drags pin a shape by
	// name ("resize_e", "grabbing") and clear the pin with "".
	cursorPin      string
	pressX, pressY float64
	pressSerial    uint32
	dragStarted    bool
	// request schedules a repaint; installed by Run once the redraw
	// channel exists.
	request func()
	// blocked suppresses all pointer input while a modal dialog is
	// open over this window.
	blocked func() bool
}

// dropInput reports whether a modal dialog over this window currently
// owns the pointer.
func (in *surfaceInput) dropInput() bool { return in.blocked != nil && in.blocked() }

// HandlePointerEnter implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerEnter(x, y float64) {
	if in.dropInput() {
		// The position still tracks while blocked: the block owns
		// routing, not bookkeeping, so a press that arrives after the
		// block lifts routes where the pointer actually is — never
		// where it sat when the block began.
		in.x, in.y = x, y
		return
	}
	in.move(x, y)
}

// HandlePointerMotion implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerMotion(x, y float64) {
	if in.dropInput() {
		in.x, in.y = x, y
		return
	}
	in.move(x, y)
}

// HandleRelativeMotion implements wlsession.SurfaceRelativeHandler.
func (in *surfaceInput) HandleRelativeMotion(dx, dy, rawDX, rawDY float64) {
	if in.dropInput() || in.onRelative == nil {
		return
	}
	in.onRelative(dx, dy, rawDX, rawDY)
	in.request()
}

// TouchMove implements widget.TouchPointer: an emulating contact
// moves the pointer through the same path as a real one.
func (in *surfaceInput) TouchMove(p widget.Point) { in.move(float64(p.X), float64(p.Y)) }

// TouchPress implements widget.TouchPointer: the contact's down is a
// left press - window-frame grabs and drag sources included - with the
// touch-down serial.
func (in *surfaceInput) TouchPress(p widget.Point) {
	in.move(float64(p.X), float64(p.Y))
	in.HandlePointerButton(widget.BTNLeft, 1, in.Serial)
}

// TouchRelease implements widget.TouchPointer: the lift releases, and
// the finger leaves - it does not hover.
func (in *surfaceInput) TouchRelease(p widget.Point) {
	in.HandlePointerButton(widget.BTNLeft, 0, in.Serial)
	in.router.Leave()
	in.request()
}

// TouchCancel implements widget.TouchPointer: a gesture took the
// contact; the press ends without a click.
func (in *surfaceInput) TouchCancel() {
	in.router.CancelPress()
	in.router.Leave()
	in.request()
}

// rehover re-picks the hovered widget after a frame's layout moved
// the tree under a resting pointer, and the cursor shape with it.
func (in *surfaceInput) rehover() {
	if in.dropInput() || !in.router.Rehover() {
		return
	}
	debug.Log("input", "rehover (%.1f,%.1f) hit %T", in.x, in.y, in.router.Hovered())
	in.applyCursorShape()
}

// move feeds one pointer position to the router and updates the
// cursor shape, hover bookkeeping, and the drag gesture. The position
// is logical surface coordinates; the tree is laid out in the same
// space, so it routes 1:1 at every device scale.
func (in *surfaceInput) move(x, y float64) {
	in.x, in.y = x, y
	in.router.Move(widget.Point{X: int(x), Y: int(y)})
	in.startDrag()
	debug.Log("input", "route move (%.1f,%.1f) hit %T", x, y, in.router.Hovered())
	if debug.Enabled {
		if bs, ok := in.router.Hovered().(widget.Boundser); ok {
			fb := bs.Bounds()
			debug.Log("input", "hit bounds (%d,%d)+%dx%d", fb.X, fb.Y, fb.W, fb.H)
		}
	}
	in.applyCursorShape()
	if in.onMove != nil {
		in.onMove(x, y)
	}
	in.request()
}

// cursorAt resolves the pointer shape at a position: a resize handle's
// resize_* shape wins over the hovered widget's own request - the frame
// owns the edges.
func (in *surfaceInput) cursorAt(x, y float64) string {
	if in.resizeAt != nil {
		if shape := resizeCursor(in.resizeAt(x, y)); shape != "" {
			return shape
		}
	}
	return cursorFor(in.router.Hovered())
}

// HandlePointerButton implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerButton(button, state, serial uint32) {
	if in.dropInput() {
		return
	}
	p := widget.Point{X: int(in.x), Y: int(in.y)}
	debug.Log("input", "route button %d %s at (%.1f,%.1f) over %T",
		button, buttonStateName(state), in.x, in.y, in.router.Hovered())
	if state == 1 {
		if in.tip != nil {
			in.tip.press()
		}
		if in.dismissPopover != nil && in.dismissPopover() {
			debug.Log("input", "press dismissed the open popover")
			return
		}
		// A press on a resize handle belongs to the window frame: the
		// xdg_toplevel.resize grab replaces the widget press entirely.
		if in.startResize != nil && in.resizeAt != nil {
			if edges := in.resizeAt(in.x, in.y); edges != 0 {
				debug.Log("input", "edge %d grabbed for resize (serial %d)", edges, serial)
				in.startResize(edges, serial)
				in.request()
				return
			}
		}
		// A press on a CSD drag handle belongs to the window frame like
		// an edge press: the xdg_toplevel.move grab replaces the widget
		// press, and a double press maximizes - the GTK header contract.
		if in.startMove != nil && button == widget.BTNLeft {
			if mover, ok := in.router.Hovered().(widget.WindowMover); ok && mover.WindowMoveGrab() {
				sameSpot := in.x-in.lastPressX < 6 && in.lastPressX-in.x < 6 &&
					in.y-in.lastPressY < 6 && in.lastPressY-in.y < 6
				double := time.Since(in.lastPressAt) < 400*time.Millisecond && sameSpot
				in.lastPressAt, in.lastPressX, in.lastPressY = time.Now(), in.x, in.y
				if double {
					debug.Log("input", "header double press (serial %d)", serial)
					if clickable, ok := mover.(widget.WindowDoubleClickable); ok {
						clickable.WindowDoubleClick()
					}
				} else {
					debug.Log("input", "header grabbed for move (serial %d)", serial)
					in.startMove(serial)
				}
				in.request()
				return
			}
		}
		// Middle-click pastes the primary selection at the caret (X11
		// refugees): the router ignores non-left presses, so this is
		// the whole handling a middle press gets.
		if button == widget.BTNMiddle {
			_, err := in.primary.pasteAt(in.router)
			in.reportTransfer(err)
		}
		in.router.Press(button, p, in.mods())
		// Remember the press for the drag gesture: the threshold is
		// measured from here and start_drag wants this serial.
		in.pressX, in.pressY = in.x, in.y
		in.pressSerial = serial
		in.dragStarted = false
		if in.onPress != nil {
			in.onPress(button, in.router.Hovered())
		}
	} else {
		in.router.Release(button, p)
		// A finished left-button selection is what copy-on-select
		// mirrors into the primary selection.
		if button == widget.BTNLeft {
			in.primary.copyAfterRelease(in.router, serial)
		}
	}
	in.request()
}

// HandlePointerAxis implements wlsession.SurfacePointerHandler: dx
// from the horizontal axis, dy from the vertical, both positive
// downward/rightward.
func (in *surfaceInput) HandlePointerAxis(dx, dy float64) {
	if in.dropInput() {
		return
	}
	dxSteps, dySteps := wlsession.AxisSteps(dx), wlsession.AxisSteps(dy)
	debug.Log("input", "route axis dx=%.1f dy=%.1f steps=%d,%d hover %T", dx, dy, dxSteps, dySteps, in.router.Hovered())
	in.router.Axis(float64(dxSteps), float64(dySteps), in.mods())
	in.request()
}

// HandlePointerLeave implements wlsession.SurfacePointerHandler. The
// session reports a leave only when the pointer truly left or the
// grabbing input device went away, so this ends every pointer-derived
// gesture: hover clears and an in-flight press/drag is cancelled
// without a click, since its release will never arrive. The shape
// cache reset above makes the next enter re-apply the cursor.
func (in *surfaceInput) HandlePointerLeave() {
	// Forget the shape even while a modal blocks this window: the
	// session restores the default arrow on leave, and a stale cache
	// would skip the next re-apply on re-entry.
	in.lastCursor = ""
	if in.dropInput() {
		return
	}
	debug.Log("input", "route leave")
	in.router.PointerLost()
	in.request()
}

// cursorShape is the shape this surface asks for right now: the
// pinned gesture shape while one lasts, else the position's shape -
// a resize edge wins, then the hovered widget's request.
func (in *surfaceInput) cursorShape() string {
	if in.cursorPin != "" {
		return in.cursorPin
	}
	return in.cursorAt(in.x, in.y)
}

// mods are the keyboard modifiers held; none without a session.
func (in *surfaceInput) mods() widget.Mods {
	if in.sess == nil {
		return 0
	}
	return widget.Mods(in.sess.Mods())
}

// applyCursorShape pushes the shape to the session when it changed.
// A failed apply forgets the cache so the next motion retries.
func (in *surfaceInput) applyCursorShape() {
	shape := in.cursorShape()
	if shape == in.lastCursor {
		return
	}
	in.lastCursor = shape
	if in.sess == nil {
		return
	}
	if err := in.sess.SetCursor(shape); err != nil {
		in.lastCursor = ""
	}
}

// pinCursor overrides the hover-derived cursor shape for a gesture:
// a resize interaction pins "resize_e" while the edge drag runs, a
// chrome drag can pin "grabbing". While pinned, hover changes do not
// clobber the shape, and the pin survives pointer leave so an active
// grab keeps its cursor. Clear the pin with "" when the gesture
// ends; the hovered widget's shape - or the default arrow - returns.
func (in *surfaceInput) pinCursor(shape string) {
	in.cursorPin = shape
	in.applyCursorShape()
}

// Run drives one host until it closes: acquire a buffer, measure and
// arrange the tree, paint, commit full-frame damage, pace on the frame
// callback, and dispatch input into a router meanwhile. It is the
// single-window convenience over Application; the loop parks in
// Session.Step between events, so with nothing pending the process
// holds no CPU — even when the compositor stops sending frame callbacks
// for an occluded surface. It returns ErrClosed when the surface ended.
func Run(cfg Config) error {
	app := NewApplication(cfg.Session)
	if cfg.Inspect {
		app.SetInspect(true)
	}
	app.SetClipboard(cfg.Clipboard)
	app.SetCopyOnSelect(cfg.CopyOnSelect)
	app.SetTooltipFace(cfg.TooltipFace)
	if cfg.OnDisconnect != nil {
		app.OnDisconnect(cfg.OnDisconnect)
	}
	// Layer hosts animate (launcher/panel overlays); other hosts get
	// the plain toplevel behavior — instant open, instant close.
	animKind := surfx.KindMenu
	if _, isLayer := cfg.Host.(*layerHost); isLayer {
		animKind = surfx.KindOverlay
	}
	app.newWindow(cfg.Host, cfg.Scale, cfg.Root, windowHooks{
		background: cfg.Background,
		opaque:     opaqueFor(cfg.Background, cfg.Opaque),
		onPress:    cfg.OnPress,
		onMove:     cfg.OnPointerMove,
		onKey:      cfg.OnKey,
	}, nil, animKind)
	return app.Run()
}

// loopKicker coalesces timer wakeups: one outstanding WakeAfter per
// deadline, skipped while an earlier kick still covers the requested
// time.
type loopKicker struct {
	until time.Time
}

// schedule arms a wakeup for at unless one is already pending that
// covers it. While a tween runs the loop passes here once per frame:
// re-arming a covered deadline would queue another sync per pass and
// the parked loop would stop parking at all. wake is the loop's wake
// seam (Application.wake), so tests drive ticks without a session.
func (k *loopKicker) schedule(wake func(time.Duration), at time.Time) {
	now := time.Now()
	if k.covers(at, now) {
		return
	}
	k.until = at
	wake(at.Sub(now))
}

// covers reports whether the pending kick still satisfies at: it has
// not fired yet, and fires no more than 2ms before at. A pending kick
// that already fired (or lands earlier than that) does not cover.
func (k *loopKicker) covers(at, now time.Time) bool {
	return k.until.After(now) && !at.Add(-2*time.Millisecond).After(k.until)
}

// nextWake computes the earliest timer deadline the parked loop must
// wake for; false means nothing is pending and the loop may sleep until
// the next compositor event. animFrame is the animation clock's next
// tick deadline while tweens run; timers is the earliest Every poller
// deadline. Deadlines already due return false: the next loop
// iteration handles them, and parking for zero duration would only
// burn a cycle.
func nextWake(repeat, animFrame, tipNext, timers, now time.Time) (time.Time, bool) {
	wake := time.Time{}
	found := false
	consider := func(t time.Time) {
		if t.IsZero() || !t.After(now) {
			return
		}
		if !found || t.Before(wake) {
			wake, found = t, true
		}
	}
	consider(repeat)
	consider(animFrame)
	consider(tipNext)
	consider(timers)
	return wake, found
}

// shouldDraw reports whether a dirty window may paint this iteration.
// A configure-driven resize always repaints: compositors hold the
// surface's frame callback until it commits at the configured size, so
// a resize repaint gated on pacing would deadlock the resize - a
// liveness requirement, not politeness. Otherwise frameOwed pacing
// decides: the previous frame's callback returned, or an animation's
// timer deadline passed with the callback still unheard (an occluded
// surface stops getting callbacks and the animation clock keeps it
// moving at the frame period).
func shouldDraw(w *hostWindow, animating bool, now time.Time, resized bool) bool {
	return w.dirty && (resized || w.frameOwed(animating, now))
}

// frameDone flips ready when the compositor reports the frame as taken
// and unregisters the callback: done is a destructor event, so the
// object is dead on both sides and its id must rejoin the client's
// pool. A frame loop that skips this leaks a proxy per frame.
type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(ev wl.CallbackDoneEvent) {
	debug.Log("frame", "frame callback done")
	ev.C.Unregister()
	*f.ready = true
}

// buttonStateName renders a wl_pointer.button state for traces.
func buttonStateName(state uint32) string {
	if state == 1 {
		return "press"
	}
	return "release"
}

// ctrl reports whether ctrl is held.
func ctrl(m wlsession.Mods) bool { return m&wlsession.ModCtrl != 0 }

// keyTranslator is the keymap-facing slice of the session.
type keyTranslator interface {
	KeyUTF8(code uint32) string
	KeySym(code uint32) xkb.Keysym
}

// composeFeeder is the dead-key compose half of the key translator:
// the seat's compose state machine (internal/compose, wired by
// wlsession). Translators without one — test stubs, or a machine with
// no compose file — simply do not implement it, and routeKey's text
// path runs unchanged.
type composeFeeder interface {
	ComposePending() bool
	FeedCompose(sym xkb.Keysym) (compose.Result, string)
	ComposeBackspace() bool
}

// feedCompose feeds one keysym to the translator's compose machine,
// reporting none for translators without one.
func feedCompose(sess keyTranslator, sym xkb.Keysym) (compose.Result, string) {
	cf, ok := sess.(composeFeeder)
	if !ok {
		return compose.None, ""
	}
	return cf.FeedCompose(sym)
}

// primarySelectionSource is the clipboard-facing slice the
// primary-selection behavior needs: read the current primary
// selection and claim it with the triggering event's serial.
// *clipboard.Clipboard implements it; tests stub it, mirroring
// keyTranslator.
type primarySelectionSource interface {
	ReadPrimary() (string, error)
	WritePrimary(text string, serial uint32) error
}

// routeKey turns one key press into text or a widget action through the
// compositor's keymap, then gives the app the raw event for any custom
// bindings. The precedence is fixed:
//
//  1. built-in widget handling: clipboard (ctrl+c/x/v), select-all
//     (ctrl+a), undo/redo (ctrl+z, ctrl+shift+z or ctrl+y), and Tab
//     focus movement — with shift held these stand down, so ctrl+shift
//     combos stay free for accelerators (ctrl+shift+z is the one
//     exception, and only while an undoable widget holds focus);
//  2. accelerators, per-widget for the focused widget first, then
//     app-wide; a fired accelerator consumes the event;
//  3. text routing: typed characters and remaining editing keysyms
//     into the focused widget. Alt is never text; ctrl combos skip
//     text and still act on editing keys. Dead-key compose sits at
//     this edge: keysyms feeding a sequence reach nothing else, the
//     committed string lands when the sequence completes (X11 style,
//     no preedit), and backspace unwinds a level instead of deleting;
//  4. extra (OnKey), which observes every press either way.
func routeKey(sess keyTranslator, router *widget.Router, keycode uint32, mods wlsession.Mods, clip *clipboard.Clipboard, accels *accelTable, extra func(*widget.Router, uint32, wlsession.Mods)) (pasteErr error) {
	sym := sess.KeySym(keycode)
	isCtrl := ctrl(mods)
	// shift re-keys letters and keeps shift+combo free for accels, so
	// the built-ins below only claim plain ctrl combos.
	noShift := mods&wlsession.ModShift == 0
	handled := false
	switch {
	case isCtrl && noShift && clip != nil && (sym == xkb.Keysym('c') || sym == xkb.Keysym('C')):
		copySelection(router, clip)
		handled = true
	case isCtrl && noShift && clip != nil && (sym == xkb.Keysym('x') || sym == xkb.Keysym('X')):
		if copySelection(router, clip) {
			router.KeyAction(widget.KeyDelete, widget.Mods(mods))
		}
		handled = true
	case isCtrl && noShift && clip != nil && (sym == xkb.Keysym('v') || sym == xkb.Keysym('V')):
		pasteErr = pasteSelection(router, clip)
		handled = true
	case isCtrl && noShift && (sym == xkb.Keysym('a') || sym == xkb.Keysym('A')):
		router.SelectAll()
		handled = true
	case isCtrl && (sym == xkb.Keysym('z') || sym == xkb.Keysym('Z')):
		// Undo history is built-in widget handling; shift turns it into
		// redo. Either press is consumed only when an undoable widget
		// holds focus, leaving the combos free everywhere else.
		if noShift {
			handled = undoFocused(router)
		} else {
			handled = redoFocused(router)
		}
	case isCtrl && noShift && (sym == xkb.Keysym('y') || sym == xkb.Keysym('Y')):
		handled = redoFocused(router)
	case !isCtrl && !noShift && sym == xkb.KeyF10:
		// Shift+F10 opens the focused widget's context menu, as the
		// Menu key does (GTK's binding).
		router.KeyAction(widget.KeyContextMenu, widget.Mods(mods))
		handled = true
	case !isCtrl && mods&(wlsession.ModAlt|wlsession.ModSuper) == 0 && sym == xkb.KeyTab:
		// Tab trap: inside a widget that absorbs tabs (a multi-line
		// text area) a plain Tab indents; ctrl+Tab and shift+Tab move
		// focus. Alt and Super chords are accelerators, never traversal.
		if mods&wlsession.ModShift != 0 {
			router.FocusPrev()
			break
		}
		if f := router.Focused(); f != nil {
			if tt, ok := f.(widget.TabTrapper); ok && tt.TrapTab(false) {
				break
			}
		}
		router.FocusNext()
		handled = true
	}
	// Accelerators beat text routing: a ctrl+P binding must fire and
	// insert nothing, while an unbound plain p still types.
	if !handled && accels.fire(router, sym, mods) {
		if extra != nil {
			extra(router, keycode, mods)
		}
		return nil
	}
	if !handled {
		// Ctrl, Alt and Super chords are shortcuts, never text.
		if !isCtrl && mods&(wlsession.ModAlt|wlsession.ModSuper) == 0 {
			// Compose consumes backspace ONLY while a sequence is in
			// flight (one level per press); every other time backspace
			// falls through to the widget's delete action below.
			if cf, ok := sess.(composeFeeder); ok && sym == xkb.KeyBackSpace && cf.ComposePending() {
				cf.ComposeBackspace()
			} else if res, text := feedCompose(sess, sym); res == compose.Pending {
				// Mid-sequence: nothing is emitted yet. Compose commits
				// on completion only, like most X11 apps.
			} else if res == compose.Done {
				insertText(router, text)
			} else if txt := sess.KeyUTF8(keycode); txt != "" {
				if r, _ := utf8.DecodeRuneInString(txt); r != utf8.RuneError && r != 0 {
					router.Type(r)
				}
			} else if a, ok := actionForSym(sym); ok {
				router.KeyAction(a, widget.Mods(mods))
			}
		} else if a, ok := actionForSym(sym); ok {
			router.KeyAction(a, widget.Mods(mods))
		}
	}
	if extra != nil {
		extra(router, keycode, mods)
	}
	return pasteErr
}

// copySelection puts the focused widget's selection on the clipboard
// and reports whether there was one.
func copySelection(router *widget.Router, clip *clipboard.Clipboard) bool {
	text, ok := focusedSelection(router)
	if !ok {
		return false
	}
	return clip.WriteText(text) == nil
}

// copySelectionPrimary claims the primary selection with the focused
// widget's selection, X11 style. serial is the triggering event's
// serial, which the protocol wants with the claim.
func copySelectionPrimary(router *widget.Router, src primarySelectionSource, serial uint32) bool {
	text, ok := focusedSelection(router)
	if !ok {
		return false
	}
	return src.WritePrimary(text, serial) == nil
}

// focusedSelection returns the focused widget's selected text, or ok
// false when nothing is focused or selected.
func focusedSelection(router *widget.Router) (string, bool) {
	f := router.Focused()
	if f == nil {
		return "", false
	}
	sel, ok := f.(widget.SelectedTexter)
	if !ok {
		return "", false
	}
	return sel.SelectedText()
}

// pasteSelection puts the clipboard payload where the focused widget
// wants it: a ContentPaster takes the best of its own mimes (an image
// view decodes image bytes off the loop goroutine); everything else
// takes text — Entry and TextArea are text-only by design.
//
// Every error rejects the paste. A failure worth telling the user - a
// hostile peer's payload past xfer.MaxPayload, one that stalls past
// the transfer deadline, a broken pipe - is returned for
// Application.OnTransferError; an empty selection pastes nothing
// quietly.
func pasteSelection(router *widget.Router, clip *clipboard.Clipboard) error {
	if paster, ok := router.Focused().(widget.ContentPaster); ok {
		data, mime, err := clip.Read(paster.PasteMimes()...)
		if err != nil {
			return transferFailure(err)
		}
		paster.PasteContent(mime, data)
		return nil
	}
	text, err := clip.ReadText()
	if err != nil {
		return transferFailure(err)
	}
	insertText(router, text)
	return nil
}

// transferFailure keeps the transfer errors worth reporting: nothing
// to paste (an empty selection, none in a usable type) is not one.
func transferFailure(err error) error {
	if errors.Is(err, clipboard.ErrUnavailable) || errors.Is(err, dragdrop.ErrNoPayload) {
		return nil
	}
	return err
}

// reportTransfer hands a failed primary paste or drop to the
// application's hook.
func (in *surfaceInput) reportTransfer(err error) {
	if err = transferFailure(err); err != nil && in.transferError != nil {
		in.transferError(err)
	}
}

// insertText puts text at the focused widget's cursor — one Insert, so
// a paste or a multi-rune compose result lands as one undo entry.
// Widgets that cannot take a bulk insert fall back to typed runes.
func insertText(router *widget.Router, text string) {
	if ins, ok := router.Focused().(widget.TextInserter); ok {
		ins.Insert(text)
		return
	}
	for _, r := range text {
		router.Type(r)
	}
}

// undoFocused undoes the focused widget's last edit, reporting whether
// anything changed; false when nothing undoable holds focus.
func undoFocused(router *widget.Router) bool {
	u, ok := router.Focused().(widget.Undoer)
	return ok && u.Undo()
}

// redoFocused reapplies the focused widget's most recently undone
// edit, reporting whether anything changed.
func redoFocused(router *widget.Router) bool {
	u, ok := router.Focused().(widget.Undoer)
	return ok && u.Redo()
}

// actionForSym maps editing keysyms to widget actions.
func actionForSym(sym xkb.Keysym) (widget.KeyAction, bool) {
	return widget.KeyActionForSym(sym)
}

// keyRepeater synthesizes repeat presses for a held key, on the
// compositor's schedule: one repeat after the initial delay, then one
// every 1/rate.
type keyRepeater struct {
	rate, delay time.Duration
	held        *heldKey
	next        time.Time
}

type heldKey struct {
	code uint32
	mods wlsession.Mods
}

// newKeyRepeater builds a repeater from the compositor's repeat info;
// zeros fall back to 400ms delay and 20 keys per second.
func newKeyRepeater(rate, delayMs uint32) *keyRepeater {
	if rate == 0 {
		rate = 20
	}
	if delayMs == 0 {
		delayMs = 400
	}
	return &keyRepeater{
		rate:  time.Second / time.Duration(rate),
		delay: time.Duration(delayMs) * time.Millisecond,
	}
}

// press records the key now held; a new press replaces and re-arms the
// previous one, matching physical repeat behavior.
func (r *keyRepeater) press(code uint32, mods wlsession.Mods) {
	r.held = &heldKey{code: code, mods: mods}
	r.next = time.Now().Add(r.delay)
}

// release stops repeating when the held key comes up.
func (r *keyRepeater) release(code uint32) {
	if r.held != nil && r.held.code == code {
		r.held = nil
	}
}

// stop ends the repeat of whatever key is held: keyboard focus moved,
// and the key's release goes wherever it went.
func (r *keyRepeater) stop() { r.held = nil }

// tick fires the next repeat when one is due and reports whether a key
// press should be delivered.
func (r *keyRepeater) tick() (code uint32, mods wlsession.Mods, ok bool) {
	if r.held == nil || time.Now().Before(r.next) {
		return 0, 0, false
	}
	r.next = time.Now().Add(r.rate)
	return r.held.code, r.held.mods, true
}

// nextDeadline reports when the next repeat would fire; false when no
// key is held. The parked loop wakes for it instead of polling.
func (r *keyRepeater) nextDeadline() (time.Time, bool) {
	if r.held == nil {
		return time.Time{}, false
	}
	return r.next, true
}

// HandlePointerScrollPixels implements wlsession.SurfacePreciseScroller:
// touchpad scrolling by the pixels the fingers moved.
func (in *surfaceInput) HandlePointerScrollPixels(dx, dy float64) {
	if in.dropInput() {
		return
	}
	debug.Log("input", "route precise scroll (%.1f,%.1f)", dx, dy)
	in.router.AxisPixels(dx, dy, in.mods())
	in.request()
}

// HandlePointerScrollEnd implements wlsession.SurfaceScrollEnder: the
// fingers lifted; a kinetic scroller glides on.
func (in *surfaceInput) HandlePointerScrollEnd() {
	if in.dropInput() {
		return
	}
	debug.Log("input", "route scroll end")
	in.router.AxisEnd()
	in.request()
}
