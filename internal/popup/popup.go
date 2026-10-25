// Package popup manages transient surfaces with the xdg_popup role:
// menus and tooltips. New positions a popup relative to its parent via
// an xdg_positioner, grabs it for dismissal, and Run drives a nested
// render/dispatch loop until the popup is destroyed. Dismissal is a
// state (see internal/surfx): Dismiss seals input, fires the close
// callback once, and the exit tween keeps the surface mapped and
// repainting until it lands — the wire teardown follows, whatever the
// dismissal path.
package popup

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/touchinput"
	"github.com/stubbedev/lys-ui/internal/wlnull"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/wlr"
)

// Gravity picks which side of the anchor rect the popup opens on.
type Gravity uint8

// Popup gravities.
const (
	GravityBottom Gravity = iota
	GravityTop
	GravityRight
	GravityLeft
)

// ErrClosed reports that the popup was dismissed; Run returns it when
// the loop exits.
var ErrClosed = errors.New("popup: dismissed")

// Popup is one open transient surface.
type Popup struct {
	WLSurface  *wl.Surface
	XdgSurface *xdg.Surface
	XdgPopup   *xdg.Popup

	// sc carries the surface's scale state (viewport destination or
	// integer set_buffer_scale); Run applies the parent window's scale.
	sc *scale.Controller

	w          int32
	h          int32
	x, y       int32 // the placed position, relative to the parent
	configured bool

	// fx is the dismissal state machine: Dismarking the popup dismissed
	// seals input and fires OnClosed exactly once, the exit tween keeps
	// the surface mapped until it lands, and only then does the real
	// wire teardown (Destroy) run. Close is the teardown path through
	// the same machine.
	fx *surfx.Coordinator
	// anim wraps the content root for tween painting; built by Run,
	// which owns the content.
	anim *animView
	// mu serializes tween frames (which may fire on the application's
	// loop goroutine) against Run's measure/arrange/paint passes.
	mu sync.Mutex
	// dirty is set by MarkFrame (tween callbacks) and the input
	// handlers, and consumed by Run's loop; atomic because tween ticks
	// run on whichever goroutine called animclock.Tick.
	dirty atomic.Bool
	sess  *wlsession.Session
	// gravity orients the enter/exit slide toward the anchor.
	gravity Gravity
	// gutter is the shadow margin (Config.Gutter) the painter insets
	// the content by.
	gutter int
	// grabbing is whether the popup holds the seat grab, which only
	// its destruction or the compositor's popup_done releases.
	grabbing bool
	// painter is the frame pipeline whoever paints this surface built
	// (Run internally, or the application loop for tooltips).
	painter *Painter
	// cfg is the placement the popup opened with, for a reposition at
	// a new size; repositions counts the requests (their tokens).
	cfg         Config
	repositions uint32
}

// Config describes where the popup goes and how big it is.
type Config struct {
	// Parent is the parent's xdg_surface; a popup always nests under a
	// shell surface. Layer-parented popups leave this nil and set
	// LayerParent instead.
	Parent *xdg.Surface
	// LayerParent parents the popup to a layer surface (bars, panels).
	LayerParent *wlr.ZwlrLayerSurfaceV1
	// X, Y is the anchor point in parent surface coordinates.
	X, Y int
	// AnchorRect, when non-empty, anchors the popup to a widget's rect
	// (parent surface coordinates) instead of the X, Y point: it opens on
	// the Gravity side, centered on the rect across it, and the
	// compositor flips it to the opposite side or slides it along when
	// it would leave the output.
	// Popovers and tooltips anchor this way; menus use the point.
	AnchorRect render.Rect
	// Width, Height is the popup's size in surface pixels.
	Width, Height int
	// Gravity picks which side of the anchor the popup opens on; zero
	// is below.
	Gravity Gravity
	// Gap is the distance between an AnchorRect and the content on the
	// opening side; it follows the popup when the compositor flips it.
	Gap int
	// NoPlate skips the theme's plate (fill and shadow) behind the
	// content: the content paints its own look, its stylesheet
	// declaring the background, and the rest of the surface stays
	// transparent.
	NoPlate bool
	// NoGrab skips the seat grab: the popup tracks hover without
	// taking input, which tooltips need. A grabbing popup anchors its
	// grab to the session's latest input event (Session.InputSerial).
	NoGrab bool
	// Gutter reserves that many logical pixels on every side for the
	// theme's box shadow: Width and Height cover content plus two
	// gutters, the content occupies the inner rect (arranged and
	// hit-tested there), the gutter stays transparent so the falloff
	// blends over whatever the popup floats above, and the input
	// region excludes it — a shadow never extends a hit area. Zero
	// paints the popup exactly as before the shadow work.
	Gutter int
	// Kind picks the animation profile; zero is the menu/popover one
	// (pass surfx.KindTooltip for tooltips).
	Kind surfx.Kind
	// NoEnter opens the popup at rest, without its enter tween: a
	// shared-element morph into its content carries the opening.
	NoEnter bool
}

// New positions and maps a popup, then grabs the seat so clicks outside
// dismiss it. The configure handshake completes inside New: callers can
// draw immediately.
func New(sess *wlsession.Session, cfg Config) (*Popup, error) {
	serial := sess.InputSerial()
	if !cfg.NoGrab && serial == 0 {
		return nil, fmt.Errorf("popup: grab: %w", wlsession.ErrNoInputEvent)
	}
	wmBase := sess.WmBase()
	if wmBase == nil {
		return nil, errors.New("popup: compositor has no xdg_wm_base")
	}
	positioner, err := wmBase.CreatePositioner()
	if err != nil {
		return nil, fmt.Errorf("popup: create positioner: %w", err)
	}
	defer func() { _ = positioner.Destroy() }()
	if err := positioner.SetSize(int32(cfg.Width), int32(cfg.Height)); err != nil {
		return nil, err
	}
	if err := place(positioner, cfg); err != nil {
		return nil, err
	}

	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("popup: create surface: %w", err)
	}
	debug.Log("input", "popup surface %d created", surf.Id())
	p := &Popup{WLSurface: surf, w: int32(cfg.Width), h: int32(cfg.Height), sess: sess, gravity: cfg.Gravity, gutter: cfg.Gutter, cfg: cfg}
	p.sc = scale.New(sess, surf, nil)
	p.fx = surfx.NewCoordinator(cfg.Kind, p, func() bool { return widget.Current().Animations })
	// The gutter is display-only: clicks in it route to whatever is
	// beneath instead of landing on a shadow pixel. Sealed at open (and
	// at each resize); the dismissal state machine re-seals it empty,
	// and later commits keep the region (double-buffered state persists
	// until changed).
	p.sealGutter(cfg.Width, cfg.Height)

	xdgSurf, err := wmBase.GetSurface(surf)
	if err != nil {
		return nil, fmt.Errorf("popup: get xdg surface: %w", err)
	}
	pop, err := getPopup(xdgSurf, cfg.Parent, positioner)
	if err != nil {
		return nil, fmt.Errorf("popup: get popup: %w", err)
	}
	// A layer-shell parent has no xdg_surface: the popup is created
	// parentless and the layer surface adopts it (get_popup), before
	// the grab and the mapping commit.
	if cfg.LayerParent != nil {
		if err := cfg.LayerParent.GetPopup(pop); err != nil {
			return nil, fmt.Errorf("popup: layer get_popup: %w", err)
		}
	}
	p.XdgSurface, p.XdgPopup = xdgSurf, pop
	debug.Log("input", "popup xdg_surface %d xdg_popup %d", xdgSurf.Id(), pop.Id())
	xdgSurf.AddConfigureHandler(p)
	pop.AddConfigureHandler(p)
	pop.AddPopupDoneHandler(p)

	// The grab goes out before the initial commit: compositors treat a
	// grab on an already-committed popup as xdg_popup.error
	// invalid_grab (sway never engages it; wlroots-based ones kill the
	// connection over it), so the commit - which maps the popup - must
	// come second. The enter tween changes nothing about this ordering:
	// it only mutates widget state that Run paints on later commits, so
	// the initial commit stays the one and only mapping commit and
	// keeps its place after the grab.
	if !cfg.NoGrab {
		debug.Log("input", "popup grab serial=%d", serial)
		if err := pop.Grab(sess.Seat(), serial); err != nil {
			return nil, fmt.Errorf("popup: grab: %w", err)
		}
		p.grabbing = true
	}
	if err := surf.Commit(); err != nil {
		return nil, fmt.Errorf("popup: commit: %w", err)
	}
	for range 20 {
		if p.EnsureUsable() == nil {
			break
		}
		if err := sess.Roundtrip(); err != nil {
			return nil, fmt.Errorf("popup: configure roundtrip: %w", err)
		}
	}
	if err := p.EnsureUsable(); err != nil {
		return nil, fmt.Errorf("popup: %w", err)
	}
	// The enter tween starts mapped-at-progress-0: ApplyVisual(0) lands
	// synchronously here, so Run's first paint is the hidden or offset
	// first frame, never a flash of the finished surface. Reduced
	// motion collapses the tween and lands at rest inside this call.
	if !cfg.NoEnter {
		p.fx.Enter()
	}
	return p, nil
}

// sealGutter limits input to the content inside the shadow gutter.
func (p *Popup) sealGutter(w, h int) {
	if p.gutter <= 0 {
		return
	}
	if region, err := p.sess.Compositor().CreateRegion(); err == nil {
		g := int32(p.gutter)
		_ = region.Add(g, g, max(0, int32(w)-2*g), max(0, int32(h)-2*g))
		_ = p.WLSurface.SetInputRegion(region)
		_ = region.Destroy()
	}
}

// Resize re-places the popup at a new size against its original
// anchor (xdg_popup.reposition, xdg_wm_base 3 and later): the
// compositor answers with a configure carrying the new geometry, which
// the next frame paints at. It reports false when the compositor
// cannot reposition popups; the popup then keeps its size.
func (p *Popup) Resize(w, h int) bool {
	if p.Dismissed() || p.sess.WmBaseVersion() < 3 {
		return false
	}
	positioner, err := p.sess.WmBase().CreatePositioner()
	if err != nil {
		return false
	}
	defer func() { _ = positioner.Destroy() }()
	p.mu.Lock()
	defer p.mu.Unlock()
	cfg := p.cfg
	cfg.Width, cfg.Height = w, h
	if positioner.SetSize(int32(w), int32(h)) != nil || place(positioner, cfg) != nil {
		return false
	}
	p.repositions++
	if p.XdgPopup.Reposition(positioner, p.repositions) != nil {
		return false
	}
	p.cfg = cfg
	p.sealGutter(w, h)
	return true
}

// RequestedSize is the size last asked for: at open, or by Resize.
func (p *Popup) RequestedSize() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cfg.Width, p.cfg.Height
}

// Position is where the compositor placed the popup's surface,
// relative to its parent's (neither sets a window geometry, so it is
// surface to surface); false before the first configure.
func (p *Popup) Position() (x, y int, ok bool) {
	return int(p.x), int(p.y), p.configured
}

// Gutter is the shadow margin the content sits inside.
func (p *Popup) Gutter() int { return p.gutter }

// HandleSurfaceConfigure completes the configure round.
func (p *Popup) HandleSurfaceConfigure(ev xdg.SurfaceConfigureEvent) {
	p.configured = true
	if p.XdgSurface != nil {
		_ = p.XdgSurface.AckConfigure(ev.Serial)
	}
}

// HandlePopupConfigure records the placed geometry.
func (p *Popup) HandlePopupConfigure(ev xdg.PopupConfigureEvent) {
	p.x, p.y = ev.X, ev.Y
	if ev.Width != 0 {
		p.w = ev.Width
	}
	if ev.Height != 0 {
		p.h = ev.Height
	}
	// A reposition lands here with the new size: repaint at it.
	p.dirty.Store(true)
}

// HandlePopupPopupDone implements xdg.PopupPopupDoneEvent: outside-
// click dismissal. It routes through Dismiss like every other path —
// no fast lane: the surface stays mapped for the exit tween and the
// compositor has already ended the grab by the time this fires (a
// grab dropped while the popup is mapped is exactly the wire state
// the exit needs).
func (p *Popup) HandlePopupPopupDone(xdg.PopupPopupDoneEvent) {
	p.grabbing = false
	p.Dismiss()
}

// EnsureUsable gates drawing until the first configure completed and
// the popup is not dismissed.
func (p *Popup) EnsureUsable() error {
	if p.Dismissed() {
		return ErrClosed
	}
	if !p.configured {
		return errors.New("popup: not configured yet")
	}
	return nil
}

// Closed reports whether the popup was dismissed (logically gone; the
// surface may still be running its exit tween).
func (p *Popup) Closed() bool { return p.Dismissed() }

// Dismissed reports whether the logical close happened.
func (p *Popup) Dismissed() bool { return p != nil && p.fx.Dismissed() }

// Destroyed reports whether the wire teardown happened.
func (p *Popup) Destroyed() bool { return p != nil && p.fx.Destroyed() }

// Size returns the placed popup size.
func (p *Popup) Size() (int, int) { return int(p.w), int(p.h) }

// Scale returns the popup's scale controller. A surface can hold only
// one wp_viewport (a second get_viewport is a fatal protocol error),
// so every caller that rescales the popup must go through this one -
// building another controller on HostSurface kills the client.
func (p *Popup) Scale() *scale.Controller { return p.sc }

// HostSurface returns the underlying wl_surface.
func (p *Popup) HostSurface() *wl.Surface { return p.WLSurface }

// SetOnClosed runs f when the popup is dismissed — exactly once,
// inside the first Dismiss (or a teardown that beats any dismissal).
func (p *Popup) SetOnClosed(f func()) { p.fx.SetOnDismissed(f) }

// Dismiss starts the two-phase close: the logical state flips (input
// region empties, OnClosed fires, the popup stops taking pointer
// events), the exit tween keeps the surface mapped and repainting
// until it lands, and only then does the wire teardown run. A second
// Dismiss mid-exit — another outside click, another Esc — is a no-op.
// A popup still holding the seat grab tears down at once instead: the
// grab outlives any exit tween, and a toplevel the dismissing pick or
// click maps meanwhile never gets the keyboard focus the grab held.
func (p *Popup) Dismiss() {
	if p == nil || p.Destroyed() {
		return
	}
	if p.WLSurface != nil && !p.Dismissed() {
		debug.Log("input", "popup %d dismissed", p.WLSurface.Id())
	}
	if p.grabbing {
		p.Close()
		return
	}
	p.fx.Dismiss()
}

// Close tears the popup down on the spot: the teardown path through
// the same state machine, for error paths and callers that want no
// exit (lys-ui-hello's replace-on-open, buffer allocation failures).
// OnClosed still fires exactly once, and the wire objects are
// destroyed exactly once even if a tween was mid-flight.
func (p *Popup) Close() {
	if p == nil {
		return
	}
	if p.WLSurface != nil {
		debug.Log("input", "popup %d closed", p.WLSurface.Id())
	}
	p.fx.Teardown()
}

// Destroy implements surfx.Driver: the real wayland teardown -
// xdg_popup, xdg_surface, wl_surface - so the compositor can drop the
// popup's buffers and release them back to the session pool. Runs
// after the exit tween lands, or immediately from Teardown. Under
// p.mu, so a frame mid-paint on the popup's loop goroutine finishes
// its commit before the proxies go away. After it, HostSurface is no
// longer valid.
func (p *Popup) Destroy() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.XdgPopup != nil {
		_ = p.XdgPopup.Destroy()
		p.XdgPopup = nil
	}
	if p.XdgSurface != nil {
		_ = p.XdgSurface.Destroy()
		p.XdgSurface = nil
	}
	if p.WLSurface != nil {
		debug.Log("input", "popup %d destroyed", p.WLSurface.Id())
		_ = p.WLSurface.Destroy()
		p.WLSurface = nil
	}
}

// SealInput implements surfx.Driver: an empty input region, committed,
// so clicks during the exit fall through to whatever is beneath — the
// tooltip input-region trick, applied at dismissal. The commit only
// refreshes the region (the current buffer stays attached), and later
// animation commits keep it: the input region is double-buffered state
// that persists until changed. Under p.mu, serialized against frames
// like Destroy.
func (p *Popup) SealInput() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.WLSurface == nil || p.sess == nil {
		return
	}
	region, err := p.sess.Compositor().CreateRegion()
	if err != nil {
		return
	}
	_ = region.Add(0, 0, 0, 0)
	_ = p.WLSurface.SetInputRegion(region)
	_ = region.Destroy()
	_ = p.WLSurface.Commit()
}

// ApplyVisual implements surfx.Driver: one tween frame.
func (p *Popup) ApplyVisual(reveal float64, morph bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.anim != nil {
		p.anim.setProgress(reveal, morph)
	}
}

// MarkFrame implements surfx.Driver: schedule a repaint of this
// surface. The owning loop's pacing does the waking — Run arms the
// animation clock's next frame deadline itself, and the application
// loop is already woken by the same deadlines through its kicker.
// (Deliberately no WakeAfter(0) here: pacing every tween callback as
// an immediate wake turns a tween into a busy spin.)
func (p *Popup) MarkFrame() {
	p.dirty.Store(true)
	if pc := p.painter; pc != nil {
		pc.dirty.Store(true)
	}
}

// Run drives the popup's own render loop until it is destroyed: paint
// through the Painter each frame, dispatch input through the session,
// and advance the enter/exit tweens. This nested loop is for grabbed
// menus: it dispatches, so the caller must already own the loop
// goroutine. The popup surface registers its own pointer handler, so
// events land here only while the compositor routes focus (or the
// popup grab) to it - the host surface keeps receiving nothing and no
// hooks are swapped. A non-nil keys handler receives seat keyboard
// events translated to KeyActions, which menus need for arrow and
// Enter navigation. It is the handler itself, not a Router: a fresh
// menu has no focused widget, so a Router would swallow every action
// before the menu saw it. frac120 is the parent window's 120-based
// device scale; the popup's buffers are built at that device scale
// while its widget tree stays logical. Surfaces that must not run a
// second dispatcher (tooltips) use NewPainter from the application
// loop instead.
func Run(sess *wlsession.Session, p *Popup, frac120 uint32, root widget.Widget, bg render.Color, keys widget.KeyActionHandler) error {
	frame := p.NewPainter(sess, frac120, root, bg)
	defer frame.Close()

	router := &widget.Router{Root: root}
	defer p.AttachInput(sess, router)()

	if keys != nil {
		prevKey := sess.OnKey
		sess.OnKey = func(code uint32, mods wlsession.Mods) {
			sym := sess.KeySym(code)
			if a, ok := widget.KeyActionForSym(sym); ok {
				keys.KeyAction(a, widget.Mods(mods))
				return
			}
			// Keys the action translation does not cover (letters, and
			// anything riding modifiers) reach the root raw: mnemonics and
			// accelerators live there. A decline swallows the press, as
			// before — text never leaks through an open popup into the
			// window behind it.
			if rk, ok := keys.(widget.RawKeyHandler); ok {
				_ = rk.RawKey(code, widget.Mods(mods), sym)
			}
		}
		defer func() { sess.OnKey = prevKey }()
	}

	for !p.Destroyed() {
		// Advance the animation clock for this surface's tweens. The app
		// loop ticks it too when both run; Tick is absolute-time based
		// and fires each callback exactly once, so the duplication only
		// decides which loop notices first. A tween callback flips dirty
		// and wakes the park, so an exit keeps this loop event-driven:
		// paint, park, wake on the next frame.
		if animclock.Tick(time.Now()) {
			p.dirty.Store(true)
		}
		if _, err := frame.Pass(); err != nil {
			return fmt.Errorf("popup: paint: %w", err)
		}
		// Park: dismissal, tween frames, pointer events, and buffer
		// releases are all events; nothing here polls. While a tween
		// runs, arm the animation clock's next frame deadline as the
		// wake — frame-callback pacing is the Pass gate, the timer is
		// the fallback, and both park when nothing runs.
		if wake, ok := animclock.Next(); ok {
			sess.WakeAfter(time.Until(wake))
		}
		if err := sess.Step(); err != nil {
			return fmt.Errorf("popup: dispatch: %w", err)
		}
	}
	return ErrClosed
}

// AttachInput routes the popup surface's pointer events into router
// (press, release, motion, leave), each one marking the popup for a
// repaint, until the returned detach. Run attaches its own; the
// application loop attaches one for the popovers it drives.
func (p *Popup) AttachInput(sess *wlsession.Session, router *widget.Router) (detach func()) {
	var pointer struct{ x, y float64 }
	input := &popupInput{
		dismissed: p.Dismissed,
		router:    router,
		pointer:   &pointer,
		mods:      func() widget.Mods { return widget.Mods(sess.Mods()) },
		markDirty: func() { p.dirty.Store(true) },
	}
	input.Input = touchinput.Input{
		Tracker: widget.TouchTracker{Router: router},
		Pointer: func() widget.Point { return widget.Point{X: int(pointer.x), Y: int(pointer.y)} },
		Blocked: p.Dismissed,
		Changed: input.markDirty,
	}
	sess.SetSurfaceInput(p.WLSurface, input)
	return func() { sess.SetSurfaceInput(p.WLSurface, nil) }
}

// popupInput routes one popup surface's pointer events into its widget
// tree. It implements wlsession.SurfacePointerHandler. Once the popup
// is dismissed the handlers go inert: the dying surface takes no
// input, belt-and-braces beside the empty input region (which is what
// actually stops the compositor from routing events here).
type popupInput struct {
	// Input routes touch and touchpad gestures (internal/touchinput).
	touchinput.Input
	dismissed func() bool
	router    *widget.Router
	pointer   *struct{ x, y float64 }
	// mods reads the keyboard modifiers held.
	mods      func() widget.Mods
	markDirty func()
}

// HandlePointerEnter implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerEnter(x, y float64) { in.move(x, y) }

// HandlePointerMotion implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerMotion(x, y float64) { in.move(x, y) }

func (in *popupInput) move(x, y float64) {
	if in.dismissed() {
		return
	}
	in.pointer.x, in.pointer.y = x, y
	in.router.Move(widget.Point{X: int(x), Y: int(y)})
	in.markDirty()
}

// HandlePointerButton implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerButton(button, state, _ uint32) {
	if in.dismissed() {
		return
	}
	pt := widget.Point{X: int(in.pointer.x), Y: int(in.pointer.y)}
	debug.Log("input", "popup route button %d state=%d at (%d,%d)", button, state, pt.X, pt.Y)
	if state == 1 {
		in.router.Press(button, pt, in.mods())
	} else {
		in.router.Release(button, pt)
	}
	in.markDirty()
}

// HandlePointerAxis implements wlsession.SurfacePointerHandler: the
// wheel scrolls whatever scrolls under the pointer, as in a window.
func (in *popupInput) HandlePointerAxis(dx, dy float64) {
	if in.dismissed() {
		return
	}
	in.router.Axis(float64(wlsession.AxisSteps(dx)), float64(wlsession.AxisSteps(dy)), in.mods())
	in.markDirty()
}

// HandlePointerLeave implements wlsession.SurfacePointerHandler. The
// session leaves a surface only when the pointer truly left or the
// device went away, so the whole gesture ends: an in-flight press is
// cancelled without a click, its release will never arrive.
func (in *popupInput) HandlePointerLeave() { in.router.PointerLost() }

// getPopup is xdg_surface.get_popup. A layer-parented popup has no
// xdg parent, which goes on the wire as wlnull.Null.
func getPopup(s *xdg.Surface, parent *xdg.Surface, positioner *xdg.Positioner) (*xdg.Popup, error) {
	if parent != nil {
		return s.GetPopup(parent, positioner)
	}
	pop := xdg.NewPopup(s.Context())
	return pop, s.Context().SendRequest(s, 2, pop, wlnull.Null, positioner)
}

// HandlePointerScrollPixels implements wlsession.SurfacePreciseScroller.
func (in *popupInput) HandlePointerScrollPixels(dx, dy float64) {
	if in.dismissed() {
		return
	}
	in.router.AxisPixels(dx, dy, in.mods())
	in.markDirty()
}

// HandlePointerScrollEnd implements wlsession.SurfaceScrollEnder.
func (in *popupInput) HandlePointerScrollEnd() {
	if in.dismissed() {
		return
	}
	in.router.AxisEnd()
	in.markDirty()
}
