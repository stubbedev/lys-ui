package app

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"
	"github.com/stubbedev/lys-lib/wlr"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/popup"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// Gravity is the side of its anchor a popover opens on.
type Gravity = popup.Gravity

// Popover gravities; the compositor flips to the opposite side when
// there is no room on the asked-for one.
const (
	GravityBottom = popup.GravityBottom
	GravityTop    = popup.GravityTop
	GravityRight  = popup.GravityRight
	GravityLeft   = popup.GravityLeft
)

// PopoverConfig declares a popover anchored to a widget.
type PopoverConfig struct {
	// Anchor is the widget to open beside; its Bounds must be current
	// (any arranged widget qualifies). AnchorAt and AnchorRect anchor
	// to a point or a rect instead - a context menu at the pointer.
	Anchor widget.Boundser
	// Gravity picks the side; zero opens below the anchor.
	Gravity Gravity
	// Gap is the distance between the anchor and the popover, kept
	// on whichever side the compositor places it.
	Gap int
	// Content is the popover's widget tree (a Menu is the common
	// case). Content that sets no layout motion of its own gets a
	// smooth one, so when it grows while open the popover springs to
	// the new size: the surface grows at once, the visible box follows.
	Content widget.Widget
	// OnClosed runs exactly once when the popover goes away, whatever
	// the cause.
	OnClosed func()
	// Focus, when set, is the widget inside Content that takes keyboard
	// focus as the popover opens (a form's first Entry, as GTK focuses
	// a popover's first input): typing then reaches it without a click.
	Focus widget.Widget
	// Parent, when set, nests the popover under that open popover (an
	// xdg_popup child of its surface, GTK's nested PopoverMenu): Anchor
	// is a widget inside the parent, the host argument is ignored, and
	// the popover closes with its parent. A parent holds one child at a
	// time: opening another closes the one before.
	Parent *Popover
	// NoAutohide is GTK's autohide=false: the popover takes no seat
	// grab, so a click elsewhere does not close it - only Dismiss (a
	// re-click on its button, a click in its own empty area) does - and
	// a bar-style host holds the keyboard on demand rather than
	// exclusively, leaving other windows their keys. A nested popover
	// always grabs.
	NoAutohide bool
	// MaxWidth and MaxHeight cap the content when measured, in place
	// of the default ceiling, the largest output's size - a list held
	// shorter than the output; a wide one wrapping its text. Zero keeps
	// the default.
	MaxWidth, MaxHeight int
}

// popoverKeyboard is the keyboard mode a bar-style host holds while a
// root popover is open: exclusive under a grabbing popover (its Esc,
// menus and entries need the keys whatever was clicked), on demand
// under a NoAutohide one, which must not take other windows' keys.
func popoverKeyboard(cfg PopoverConfig) KeyboardMode {
	if cfg.NoAutohide {
		return KeyboardOnDemand
	}
	return KeyboardExclusive
}

// AnchorAt is an anchor at a point in the host's coordinates - a
// context menu at the pointer, a popover over a canvas position.
func AnchorAt(x, y int) widget.Boundser { return AnchorRect(render.Rect{X: x, Y: y, W: 1, H: 1}) }

// AnchorRect is an anchor on a rect in the host's coordinates - a
// text range, a cell of a custom-drawn grid.
func AnchorRect(r render.Rect) widget.Boundser { return rectAnchor(r) }

// rectAnchor is a fixed rect standing in for an anchor widget.
type rectAnchor render.Rect

func (r rectAnchor) Bounds() render.Rect { return render.Rect(r) }

// Popover is a widget-anchored transient popup. It closes on
// click-away (the popup grab), Esc, or Dismiss; OnClosed fires exactly
// once.
type Popover struct {
	mu      sync.Mutex
	closed  bool
	closeFn func()
	// focus moves the open popover's keyboard focus (loop only).
	focus func(w widget.Widget) error

	// host is the window the popover (or its root) opened over, pop its
	// surface, teardown its no-tween close; parent and children link a
	// nested chain (loop only).
	host     Host
	pop      *popup.Popup
	teardown func()
	parent   *Popover
	children []*Popover
	// autohide closes the popover on a press anywhere else, the parent
	// window included (PopoverConfig.NoAutohide off).
	autohide bool
}

// Parent is the popover this one is nested under, nil for a root.
func (p *Popover) Parent() *Popover { return p.parent }

// Child is the open popover nested under this one, nil without one.
func (p *Popover) Child() *Popover {
	if p == nil || len(p.children) == 0 {
		return nil
	}
	return p.children[len(p.children)-1]
}

// Root is the outermost popover of the chain.
func (p *Popover) Root() *Popover {
	for p != nil && p.parent != nil {
		p = p.parent
	}
	return p
}

// closeChildren tears every nested popover down on the spot, deepest
// first through each one's own close: a child surface must be gone
// before its parent's (xdg_popup's topmost rule), and an exit tween
// would outlive the parent's.
func (p *Popover) closeChildren() {
	for len(p.children) > 0 {
		child := p.children[len(p.children)-1]
		p.children = p.children[:len(p.children)-1]
		if child.teardown != nil {
			child.teardown()
		}
	}
}

// settle is a popover's close bookkeeping, whatever closed it: the
// first call flips it closed, closes what nests under it, and leaves
// its parent's list, reporting true; later calls report false.
func (p *Popover) settle() bool {
	if !p.markClosed() {
		return false
	}
	p.closeChildren()
	if p.parent != nil {
		p.parent.dropChild(p)
	}
	return true
}

// dropChild forgets a child that closed on its own.
func (p *Popover) dropChild(child *Popover) {
	p.children = slices.DeleteFunc(p.children, func(c *Popover) bool { return c == child })
}

// SetFocus moves keyboard focus to w inside the open popover - a form
// that appears in it takes typing at once (GTK's grab_focus). It fails
// as widget.Router.SetFocus does, and with ErrClosed once the popover
// is gone; nil clears focus. Loop goroutine only.
func (p *Popover) SetFocus(w widget.Widget) error {
	if p == nil || p.focus == nil || p.Closed() {
		return fmt.Errorf("app: focus %T: %w", w, ErrClosed)
	}
	return p.focus(w)
}

// Dismiss closes the popover programmatically; OnClosed still fires
// exactly once.
func (p *Popover) Dismiss() {
	if p != nil && p.closeFn != nil {
		p.closeFn()
	}
}

// Closed reports whether the popover has gone away.
func (p *Popover) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// markClosed flips the closed flag and reports whether this call is
// the first, so OnClosed hooks fire exactly once.
func (p *Popover) markClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.closed = true
	return true
}

// popoverRegistry tracks the one open popover per host; opening a new
// one on the same host re-anchors by closing the previous. Hosts are
// keyed by their surface, so an *app.Window and the raw host it wraps
// name the same entry.
type popoverRegistry struct {
	open map[*wl.Surface]*Popover
}

// openOrReplace registers p for host and returns the popover it
// replaced, if any; the caller closes it.
func (r *popoverRegistry) openOrReplace(h Host, p *Popover) *Popover {
	if r.open == nil {
		r.open = make(map[*wl.Surface]*Popover)
	}
	prev := r.open[h.HostSurface()]
	r.open[h.HostSurface()] = p
	return prev
}

// get returns the popover registered for host, nil without one.
func (r *popoverRegistry) get(h Host) *Popover { return r.open[h.HostSurface()] }

// dismissAutohide closes the autohide popover open over host, if any,
// and reports whether it did. Compositors deliver a press on the
// popover's own client through the popup grab without a popup_done
// (sway does), so the parent window's press path closes it here: the
// press is the click away, and it goes nowhere else.
func (a *Application) dismissAutohide(h Host) bool {
	p := a.popovers.get(h)
	if p == nil || !p.autohide || p.Closed() {
		return false
	}
	p.Dismiss()
	return true
}

// take removes and returns the popover registered for host.
func (r *popoverRegistry) take(h Host) *Popover {
	p := r.open[h.HostSurface()]
	delete(r.open, h.HostSurface())
	return p
}

// popoverKeyRoot wraps popover content with Esc dismissal and the
// raw-key routes a popup owes: Alt-letter presses activate the
// content's mnemonics first (an open menu owns its Alt-letters), then
// application-wide accelerators fire from the very table the main
// routeKey path uses — so a combo fires exactly once whichever path
// sees it, and the two can never disagree.
type popoverKeyRoot struct {
	onDismiss func()
	content   widget.Widget
	// maxW and maxH cap the content at (re)fit, the open's ceilings
	// (popoverCeiling; zero is the default).
	maxW, maxH int
	// fireAccel fires the application's accelerators for one raw key,
	// reporting whether one fired.
	fireAccel func(sym xkb.Keysym, mods wlsession.Mods) bool
	bounds    render.Rect
}

// Measure measures the content as its container would, so the
// content's own size request (GtkPopover's width and height requests)
// floors the popover.
func (r *popoverKeyRoot) Measure(con widget.Constraints) widget.Size {
	return widget.MeasureAsChild(r.content, con)
}

// Arrange lays the content out at its measured size inside the
// surface: the same rect while they agree, and while a shrinking
// popover holds its larger surface (fitContent), the smaller one the
// content springs down to.
func (r *popoverKeyRoot) Arrange(rect render.Rect) {
	r.bounds = rect
	nat := widget.MeasureAsChild(r.content, widget.Constraints{Max: widget.Size{W: rect.W, H: rect.H}})
	rect.W, rect.H = min(rect.W, nat.W), min(rect.H, nat.H)
	widget.ArrangeAsChild(r.content, rect)
}

func (r *popoverKeyRoot) Paint(cv *render.Canvas) { widget.PaintChild(cv, r.content) }

// ShownRect reports the content's shown rect, so the popup's plate and
// shadow follow the content through a layout motion.
func (r *popoverKeyRoot) ShownRect() render.Rect { return widget.ShownRect(r.content) }

// Children exposes the content to the tree walks (damage, focus).
func (r *popoverKeyRoot) Children() []widget.Widget { return []widget.Widget{r.content} }

// HitTest is the content's own hit, so the widget under the pointer
// (a button, an entry) takes the press, and the root itself off the
// content.
func (r *popoverKeyRoot) HitTest(p widget.Point) widget.Widget {
	if hit := r.content.HitTest(p); hit != nil {
		return hit
	}
	return r
}

func (r *popoverKeyRoot) KeyAction(a widget.KeyAction, mods widget.Mods) {
	if a == widget.KeyDismiss {
		r.onDismiss()
		return
	}
	if h, ok := r.content.(widget.KeyActionHandler); ok {
		h.KeyAction(a, mods)
	}
}

// RawKey implements widget.RawKeyHandler: the popover's mnemonics and
// accelerators, in that order.
func (r *popoverKeyRoot) RawKey(code uint32, mods widget.Mods, sym xkb.Keysym) bool {
	if mods&widget.ModAlt != 0 && mods&widget.ModCtrl == 0 {
		if widget.ActivateMnemonic(r.content, sym) {
			return true
		}
	}
	return r.fireAccel != nil && r.fireAccel(sym, wlsession.Mods(mods))
}

// LayerSurfacer is implemented by layer-shell hosts so popovers can
// parent to the layer surface, which has no xdg_surface of its own.
type LayerSurfacer interface {
	// LayerPopupSurface returns the layer surface popups attach to.
	LayerPopupSurface() *wlr.ZwlrLayerSurfaceV1
}

// OpenPopover opens content beside anchor on the given host, on the
// side gravity asks for, flipping inside the host when the anchor sits
// near an edge. Opening a second popover on the same host re-anchors:
// the previous popover closes first. The popover's own key router
// dismisses on Esc and forwards everything else to the content. A
// grabbing popover anchors its grab to the latest input event, the
// click or key that opened it; opened before any input event it fails.
func (a *Application) OpenPopover(host Host, cfg PopoverConfig) (*Popover, error) {
	if cfg.Anchor == nil || cfg.Content == nil {
		return nil, errors.New("app: popover needs an anchor and content")
	}
	if m, ok := cfg.Content.(interface {
		SetLayoutMotion(widget.LayoutMotion)
		LayoutMotion() widget.LayoutMotion
	}); ok && m.LayoutMotion() == widget.LayoutMotionNone {
		m.SetLayoutMotion(widget.LayoutMotionSmooth)
	}
	parent := cfg.Parent
	if parent != nil {
		if parent.Closed() || parent.pop == nil {
			return nil, errors.New("app: the parent popover is closed")
		}
		host = parent.host
		parent.closeChildren()
	} else if prev := a.popovers.take(host); prev != nil {
		prev.Dismiss()
	}

	anchor := cfg.Anchor.Bounds()
	// The popover's content styles as the `popover` element (css.md)
	// with GtkPopover's .background class: whatever widget tree the app
	// hands over IS the card, and as a child of the widget it opens
	// from, like GTK's popover CSS node: it inherits that window's fonts
	// and colors and its selectors see the anchor's ancestry. Anchored
	// before the measure, which reads the inherited font.
	nameSurfaceElement(cfg.Content, elemPopover)
	if c, ok := cfg.Content.(interface{ AddClass(...string) }); ok {
		c.AddClass(popoverClass)
	}
	styleAnchor := widget.AnchorWidget(cfg.Anchor)
	if styleAnchor != nil {
		widget.SetStyleAnchor(cfg.Content, styleAnchor)
	}
	unanchor := func() {
		if styleAnchor != nil && widget.StyleAnchor(cfg.Content) == styleAnchor {
			widget.SetStyleAnchor(cfg.Content, nil)
		}
	}
	size := widget.MeasureAsChild(cfg.Content, widget.Constraints{Max: popoverCeiling(a.outputs(), cfg.MaxWidth, cfg.MaxHeight)})
	// The positioner anchors to the anchor widget's rect on the gravity
	// side and the compositor flips or slides it against the output (a
	// bar is never tall enough to judge room by). The shadow gutter rides
	// on the surface: it grows the popup on every side and the
	// positioner offset keeps the CONTENT (not the gutter) aligned. The theme can
	// turn it off (ShadowGutter 0), collapsing to the exact pre-shadow
	// geometry.
	gutter := widget.Current().ShadowGutter()
	debug.Log("input", "popover anchor %+v size %dx%d gravity %d gutter %d", anchor, size.W, size.H, cfg.Gravity, gutter)

	p := &Popover{host: host, parent: parent, autohide: !cfg.NoAutohide}
	keyRoot := &popoverKeyRoot{
		onDismiss: p.Dismiss,
		content:   cfg.Content,
		maxW:      cfg.MaxWidth,
		maxH:      cfg.MaxHeight,
		fireAccel: func(sym xkb.Keysym, mods wlsession.Mods) bool {
			// App-wide bindings only: the popup holds the keyboard, so
			// there is no focused widget whose bindings could own the
			// combo — exactly the GTK rule that an open menu fires the
			// accelerator group and nothing else.
			return a.accels.fire(nil, sym, mods)
		},
	}

	pcfg := popup.Config{
		AnchorRect: anchor,
		Width:      size.W + 2*gutter, Height: size.H + 2*gutter,
		Gutter:  gutter,
		Gravity: cfg.Gravity,
		Gap:     cfg.Gap,
		NoGrab:  cfg.NoAutohide && parent == nil,
		// The theme's plate yields to a stylesheet that styles the
		// popover's background, as GTK leaves a popover's look to CSS.
		NoPlate: widget.StylesBackground(cfg.Content),
	}
	switch {
	case parent != nil:
		pcfg.Parent = parent.pop.XdgSurface
	default:
		var ok bool
		if pcfg.Parent, pcfg.LayerParent, ok = popupParentOf(host); !ok {
			return nil, errors.New("app: host cannot carry popups")
		}
	}

	// A bar-style layer declines the keyboard; the popover's grab needs
	// it (Esc, menus, a form's Entry), so the layer holds the keyboard
	// exclusively while the popover is open. Exclusive, not on-demand:
	// on a v4 layer shell on-demand focuses only on a click into the
	// layer, which a popover opened from a key or a hover never gets
	// (before the shell was bound at v4, the on-demand value was read
	// as the v1 boolean - exclusive in effect). A nested popover rides
	// on its root's.
	restoreKeys := func() {}
	if km, ok := host.(keyboardModer); ok && parent == nil && km.KeyboardMode() == KeyboardNone {
		if err := km.holdKeyboard(popoverKeyboard(cfg)); err == nil {
			restoreKeys = func() { _ = km.holdKeyboard(KeyboardNone) }
		}
	}
	// A tagged widget in the content that last showed on a placed
	// surface morphs out of it; that morph carries the opening, so the
	// popup skips its own enter tween (#200's anchor reveal). It needs
	// this popup placed too, so a nested popover under an unplaced
	// parent keeps the reveal.
	base, basePlaced := widget.Point{}, true
	if parent != nil {
		base, basePlaced = a.originOf(parent)
	}
	pcfg.NoEnter = basePlaced && widget.SharedMorphPending(cfg.Content)
	pop, err := popup.New(a.sess, pcfg)
	if err != nil {
		restoreKeys()
		unanchor()
		return nil, err
	}
	router := &widget.Router{Root: keyRoot}
	if cfg.Focus != nil {
		if err := router.SetFocus(cfg.Focus); err != nil {
			pop.Close()
			restoreKeys()
			unanchor()
			return nil, fmt.Errorf("app: popover focus: %w", err)
		}
	}
	// Dismiss, not Close: the programmatic path takes the same two-
	// phase route as outside-clicks and Esc — input seals, OnClosed
	// fires, and the surface runs its exit tween before the wire
	// teardown.
	p.closeFn = pop.Dismiss
	p.teardown = pop.Close
	p.pop = pop
	fireClosed := func() {
		// Whatever closed this popover (Esc, a click away, the
		// compositor's popup_done, Dismiss) closes what nests under it.
		if !p.settle() {
			return
		}
		restoreKeys()
		if cfg.OnClosed != nil {
			cfg.OnClosed()
		}
	}
	pop.SetOnClosed(fireClosed)

	if parent != nil {
		parent.children = append(parent.children, p)
	} else {
		a.popovers.openOrReplace(host, p)
	}
	// The application loop drives the popover like any of its windows:
	// one painter pass per loop pass, pointer input through a Router on
	// the loop goroutine, keys through the same Router once a widget
	// holds focus. Content the app mutates from Invoke repaints on the
	// next pass, and nothing paints from a second goroutine.
	widget.SetSurfaceHost(cfg.Content, surfaceHost{a: a, host: host, parent: p, router: router})
	op := &openPopover{
		host:       host,
		pop:        pop,
		painter:    pop.NewPainter(a.sess, a.popupRenderer(), a.fracFor(host), keyRoot, widget.Current().Surface),
		router:     router,
		keyRoot:    keyRoot,
		popover:    p,
		fireClosed: fireClosed,
		unanchor:   unanchor,
	}
	if x, y, ok := pop.Position(); ok && basePlaced {
		op.origin, op.placed = widget.Point{X: base.X + x, Y: base.Y + y}, true
	}
	p.focus = func(w widget.Widget) error {
		if err := router.SetFocus(w); err != nil {
			return err
		}
		op.pop.MarkFrame()
		return nil
	}
	op.detach = pop.AttachInput(a.sess, router)
	a.openPopovers = append(a.openPopovers, op)
	return p, nil
}

// fracFor returns host's current 120-based device scale, so popups and
// other one-shot surfaces match the window they open over; 120 (1x)
// before the first rescale or for unknown hosts.
func (a *Application) fracFor(host Host) uint32 {
	if w := a.hostWindowOf(host); w != nil {
		return w.frac120
	}
	return scale.Denom
}

// EnsureUsable implements Host for the toplevel handle.
func (w *Window) EnsureUsable() error { return w.win.EnsureUsable() }

// Size implements Host for the toplevel handle.
func (w *Window) Size() (int, int) { return w.win.Size() }

// HostSurface implements Host for the toplevel handle.
func (w *Window) HostSurface() *wl.Surface { return w.win.HostSurface() }

// EnsureUsable implements Host for the layer handle.
func (l *LayerWindow) EnsureUsable() error { return l.ls.EnsureUsable() }

// Size implements Host for the layer handle.
func (l *LayerWindow) Size() (int, int) { return l.ls.Size() }

// HostSurface implements Host for the layer handle.
func (l *LayerWindow) HostSurface() *wl.Surface { return l.ls.HostSurface() }

// LayerPopupSurface implements LayerSurfacer for the layer handle.
func (l *LayerWindow) LayerPopupSurface() *wlr.ZwlrLayerSurfaceV1 {
	return l.ls.LayerPopupSurface()
}

// popSurface is the popup half the loop drives (a *popup.Popup).
type popSurface interface {
	Destroyed() bool
	Dismissed() bool
	MarkFrame()
	// RequestedSize, Gutter and Resize fit the surface to its content.
	RequestedSize() (int, int)
	// Position is where the compositor placed the surface relative to
	// its parent, false before it did.
	Position() (x, y int, ok bool)
	Gutter() int
	Resize(w, h int) bool
}

// popPainter is the frame pipeline half (a *popup.Painter).
type popPainter interface {
	Pass() (bool, error)
	Close()
}

// openPopover is one popover the application loop drives.
type openPopover struct {
	host       Host
	pop        popSurface
	painter    popPainter
	router     *widget.Router
	keyRoot    *popoverKeyRoot
	popover    *Popover
	detach     func()
	fireClosed func()
	// unanchor ends the content's style anchoring, unless a newer open
	// re-anchored it.
	unanchor func()
	// origin is where the popup's surface sits in its toplevel's
	// coordinates, placed once the compositor placed it and its parent
	// is placed too (shared-element morphs cross surfaces through it).
	origin widget.Point
	placed bool
}

// originOf is where the open popover p's surface sits in its
// toplevel's coordinates, false while it is not placed.
func (a *Application) originOf(p *Popover) (widget.Point, bool) {
	for _, op := range a.openPopovers {
		if op.popover == p {
			return op.origin, op.placed
		}
	}
	return widget.Point{}, false
}

// surfaceOrigin is the widget package's surface placement
// (widget.SetSurfaceOriginResolver): a window's or layer's tree sits at
// its own origin, a popover's where the compositor placed it.
func (a *Application) surfaceOrigin(root widget.Widget) (widget.Point, bool) {
	for _, op := range a.openPopovers {
		if widget.RootOf(op.keyRoot.content) == root {
			return op.origin, op.placed
		}
	}
	for _, hw := range a.windows {
		if widget.RootOf(hw.router.Root) == root {
			return widget.Point{}, true
		}
	}
	return widget.Point{}, false
}

// release frees what the open popover holds once its surface is gone:
// the painter, the input, and the content's style anchor.
func (op *openPopover) release() {
	op.painter.Close()
	op.detach()
	op.unanchor()
}

// drivePopovers runs one loop pass over the open popovers: a destroyed
// one (its exit tween landed) releases its painter and input and fires
// its close; a live one refits and repaints when its content drained
// damage, as a window draws only what changed, and paints at most one
// frame.
func (a *Application) drivePopovers() error {
	// Newest first: a nested popover destroyed in the same pass as its
	// parent must leave the wire before it (xdg_popup's topmost rule).
	kept := make([]*openPopover, 0, len(a.openPopovers))
	for _, op := range slices.Backward(a.openPopovers) {
		if op.pop.Destroyed() {
			op.release()
			if a.popovers.get(op.host) == op.popover {
				a.popovers.take(op.host)
			}
			op.fireClosed()
			continue
		}
		if _, damaged := widget.CollectDamage(op.keyRoot); damaged {
			op.fitContent(a.outputs())
			op.pop.MarkFrame()
		}
		if _, err := op.painter.Pass(); err != nil {
			return fmt.Errorf("app: popover paint: %w", err)
		}
		kept = append(kept, op)
	}
	slices.Reverse(kept)
	a.openPopovers = kept
	return nil
}

// fitContent grows (or shrinks) an open popover to its content's
// natural size when that changed - a tray menu re-publishing more rows
// - measured as at open, against the same ceiling (popoverCeiling).
// Growing, the surface grows at once and the content springs into it;
// shrinking, the surface keeps its size while the content's layout
// motion carries it down, and takes the final size, one reposition,
// when the motion lands.
func (op *openPopover) fitContent(outputs []*wlsession.Output) {
	if op.pop.Dismissed() {
		return
	}
	size := op.keyRoot.Measure(widget.Constraints{Max: popoverCeiling(outputs, op.keyRoot.maxW, op.keyRoot.maxH)})
	g := op.pop.Gutter()
	w, h := size.W+2*g, size.H+2*g
	rw, rh := op.pop.RequestedSize()
	if shown, ok := widget.LayoutShownSize(op.keyRoot.content); ok && shown != size {
		w, h = max(w, rw), max(h, rh)
	}
	if rw != w || rh != h {
		op.pop.Resize(w, h)
	}
}

// keyPopover is the popover holding the keyboard: the newest one still
// taking input. An open popover grabs the seat, so keys never reach
// the windows beneath it.
func (a *Application) keyPopover() *openPopover {
	for _, op := range slices.Backward(a.openPopovers) {
		if !op.pop.Dismissed() {
			return op
		}
	}
	return nil
}

// deliverPopoverKey routes one press inside a popover. Esc dismisses
// whatever has focus (GTK's popover rule). With a focused widget — an
// Entry the user clicked — the press takes the windows' full key path
// (editing, clipboard, focus traversal, text); without one, the
// actions and raw keys go to the popover root, which forwards them to
// the content (a menu's arrows, mnemonics, accelerators).
func (a *Application) deliverPopoverKey(tr keyTranslator, op *openPopover, keycode uint32, mods wlsession.Mods) {
	defer op.pop.MarkFrame()
	sym := tr.KeySym(keycode)
	act, isAction := widget.KeyActionForSym(sym)
	debug.Log("input", "popover key code=%d sym=%v action=%v focused=%T", keycode, sym, isAction, op.router.Focused())
	if isAction && act == widget.KeyDismiss {
		op.keyRoot.onDismiss()
		return
	}
	if op.router.Focused() != nil {
		a.reportTransfer(routeKey(tr, op.router, keycode, mods, a.clip, a.accels, nil))
		return
	}
	if isAction {
		op.keyRoot.KeyAction(act, widget.Mods(mods))
		return
	}
	_ = op.keyRoot.RawKey(keycode, widget.Mods(mods), sym)
}

// keyboardModer is a host whose keyboard interactivity a popover can
// raise while open (*LayerWindow).
type keyboardModer interface {
	KeyboardMode() KeyboardMode
	// holdKeyboard changes the mode for the popover's lifetime only:
	// the surface's declared mode (what a rebuild restores) stays.
	holdKeyboard(KeyboardMode) error
}

// outputs is the session's outputs; a wireless application (tests)
// has none.
func (a *Application) outputs() []*wlsession.Output {
	if a.sess == nil {
		return nil
	}
	return a.sess.Outputs()
}

// popoverCeiling is the content's measure ceiling: maxW and maxH, or
// where unset the largest output's size (fitBounds) - the one rule the
// open and every refit share. A popup may outgrow the window it hangs
// from; the positioner keeps it on its output, resizing it where the
// room left is shorter, and its content scrolls inside.
func popoverCeiling(outputs []*wlsession.Output, maxW, maxH int) widget.Size {
	bound := fitBounds(nil, outputs)
	if maxW <= 0 {
		maxW = bound.W
	}
	if maxH <= 0 {
		maxH = bound.H
	}
	return widget.Size{W: maxW, H: maxH}
}

// popupParentOf is the surface a popup on host hangs from: a layer's
// layer surface, a toplevel's (Window or internal host) xdg surface.
func popupParentOf(host Host) (xdgParent *xdg.Surface, layerParent *wlr.ZwlrLayerSurfaceV1, ok bool) {
	switch h := host.(type) {
	case LayerSurfacer:
		return nil, h.LayerPopupSurface(), true
	case *Window:
		if h.win != nil && h.win.XdgSurface != nil {
			return h.win.XdgSurface, nil, true
		}
	case tooltipSurfacer:
		return h.TooltipSurface(), nil, true
	}
	return nil, nil, false
}
