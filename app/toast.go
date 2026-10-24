package app

import (
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// ToastPosition picks the window edge a toast stack hugs.
type ToastPosition uint8

const (
	// ToastBottom stacks toasts above the bottom edge; zero, the
	// default.
	ToastBottom ToastPosition = iota
	// ToastTop stacks toasts below the top edge.
	ToastTop
)

const (
	// toastMargin keeps the stack off the window edge by default, and
	// toastSpacing separates stacked cards.
	toastMargin  = 16
	toastSpacing = 8
	// toastMaxStack caps the stack: a burst of toasts drops its oldest
	// members instead of burying the window.
	toastMaxStack = 3
	// toastMaxWidth clamps a card's width so a long line cannot span
	// the window.
	toastMaxWidth = 420
)

// ToastConfig declares a toast's placement: which window hosts it, the
// edge the stack hugs, the stack cap, and the text face.
type ToastConfig struct {
	// Host picks the window; nil shows the toast on the focused
	// window, or the first one.
	Host Host
	// Position picks the edge; zero is the bottom.
	Position ToastPosition
	// Margin keeps the stack off the edge; zero uses the default.
	Margin int
	// MaxStack caps simultaneous toasts, oldest dropping first; zero
	// uses the default.
	MaxStack int
	// Face renders the text; nil falls back to the tooltip face, then
	// the default sans face.
	Face *render.Typeface
}

// Toast errors: ShowToast had nowhere to show the toast.
var (
	ErrNoToastWindow = errors.New("app: toast dropped: no window to show it on")
	ErrNoToastFace   = errors.New("app: toast dropped: no text face available")
)

// ShowToast shows a transient feedback card over a window: bottom-
// centered by default (ToastConfig moves it), auto-dismissed after
// timeout, hover-pausable, optionally actionable through the returned
// toast (widget.Toast.SetAction). Toasts stack on the host window up
// to the cap, the oldest dropping first; when the last one goes, the
// window's tree is back to exactly what it was. A host window that
// closes hands its live toasts to the window toasts go to next, so a
// toast outlives the dialog it was raised from. A nil config uses the
// defaults. With no window or no font to show it, the toast is
// dropped and the error says why.
func (a *Application) ShowToast(text string, timeout time.Duration, cfg *ToastConfig) (*widget.Toast, error) {
	if cfg == nil {
		cfg = &ToastConfig{}
	}
	hw := a.windowForToast(cfg.Host)
	if hw == nil {
		return nil, ErrNoToastWindow
	}
	face := a.resolveFace(cfg.Face)
	if face == nil {
		return nil, ErrNoToastFace
	}
	t := widget.NewToast(face, text, timeout)
	a.hostToast(hw, t, toastPlacement{
		pos:    cfg.Position,
		margin: orDefault(max(cfg.Margin, 0), toastMargin),
		max:    orDefault(max(cfg.MaxStack, 0), toastMaxStack),
	})
	return t, nil
}

// SetToastHost names the window toasts go to when their config names
// none (nil: the focused window, then the first) - a main window that
// collects every dialog's feedback.
func (a *Application) SetToastHost(h Host) { a.toastHost = h }

// toastPlacement is a stack's geometry.
type toastPlacement struct {
	pos         ToastPosition
	margin, max int
}

// hostToast stacks t on hw's toast layer, wrapping the window's tree
// with the layer on the first toast.
func (a *Application) hostToast(hw *hostWindow, t *widget.Toast, pl toastPlacement) {
	layer := a.toasts.layerFor(hw.host, func() *toastLayer {
		l := &toastLayer{hw: hw, root: hw.router.Root}
		// The first toast wraps the window's tree; the last removal
		// unwraps it.
		hw.router.Root = l
		hw.dirty = true
		return l
	})
	layer.pos, layer.margin, layer.max = pl.pos, pl.margin, pl.max
	t.OnDismissed = func() { layer.remove(t) }
	layer.add(t)
}

// rehostToasts moves toasts a closed window carried onto the window
// toasts go to now; with none left they stop.
func (a *Application) rehostToasts(moved []movedToasts) {
	for _, m := range moved {
		hw := a.windowForToast(nil)
		for _, t := range m.toasts {
			if hw == nil {
				t.Close()
				continue
			}
			a.hostToast(hw, t, m.placement)
		}
	}
}

// movedToasts are one closed window's live toasts and their stack's
// geometry.
type movedToasts struct {
	toasts    []*widget.Toast
	placement toastPlacement
}

// resolveFace resolves the text face for app-provided widget text:
// the asked-for face, else the configured tooltip face, else the
// default sans face. It can return nil when the system has no usable
// fonts; callers then skip the text surface (log and drop) rather
// than hand a nil face into a widget constructor, which would panic.
func (a *Application) resolveFace(asked *render.Typeface) *render.Typeface {
	if asked != nil {
		return asked
	}
	if a.tooltipFace != nil {
		return a.tooltipFace
	}
	f, err := sysfont.UI()
	if err != nil {
		debug.Log("input", "ui face: %v", err)
		return nil
	}
	return f
}

// windowForToast resolves the hosting window: the one asked for, else
// the application's toast host while it is open, else the focused
// window, else the first.
func (a *Application) windowForToast(h Host) *hostWindow {
	if h != nil {
		return a.hostWindowOf(h)
	}
	if a.toastHost != nil {
		if hw := a.hostWindowOf(a.toastHost); hw != nil && !a.toastHost.Closed() {
			return hw
		}
	}
	if len(a.windows) == 0 {
		return nil
	}
	return a.focused()
}

// toastRegistry tracks each host's toast layer.
type toastRegistry struct {
	mu   sync.Mutex
	open map[Host]*toastLayer
}

// layerFor returns the host's layer, building it with mk on first use.
func (r *toastRegistry) layerFor(h Host, mk func() *toastLayer) *toastLayer {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.open == nil {
		r.open = make(map[Host]*toastLayer)
	}
	if l := r.open[h]; l != nil {
		return l
	}
	l := mk()
	r.open[h] = l
	return l
}

// closeHost tears down a closing window's toast layer: its toasts
// stop holding timers, so the animation clock parks after the surface
// they painted on is gone.
func (r *toastRegistry) closeHost(h Host) {
	r.mu.Lock()
	l := r.open[h]
	delete(r.open, h)
	r.mu.Unlock()
	if l != nil {
		l.close()
	}
}

// takeHost detaches a closing window's toast layer without stopping
// its toasts, for rehosting: the window gets its bare tree back.
func (r *toastRegistry) takeHost(h Host) (movedToasts, bool) {
	r.mu.Lock()
	l := r.open[h]
	delete(r.open, h)
	r.mu.Unlock()
	if l == nil || len(l.slots) == 0 {
		return movedToasts{}, false
	}
	m := movedToasts{toasts: l.slots, placement: toastPlacement{pos: l.pos, margin: l.margin, max: l.max}}
	l.slots = nil
	l.close()
	return m, true
}

// toastLayer hosts one window's toast stack: the wrapped root keeps
// the whole window rect and the toasts lay out at the configured
// edge, newest toward the interior. It is a hand-rolled
// widget.Widget — app-side trees cannot embed widget.node — so its
// structural changes repaint through the window's dirty flag, while
// each toast invalidates its own pixels through the damage collector
// (which descends through Children).
type toastLayer struct {
	hw   *hostWindow
	root widget.Widget
	pos  ToastPosition
	// margin, max are the stack geometry; add refreshes them from the
	// latest toast's config.
	margin, max int
	slots       []*widget.Toast
	// children is the cached Children() view over root and slots,
	// rebuilt on add/remove so the damage walk borrows it for free.
	children []widget.Widget
}

// Children exposes the wrapped root and the live toasts in paint
// order, so damage collection and focus traversal descend. The view
// is cached and rebuilt only on structural change; a never-built one
// is built here, so layers constructed bare stay correct.
func (l *toastLayer) Children() []widget.Widget {
	if l.children == nil && l.root != nil {
		l.syncChildren()
	}
	return l.children
}

// syncChildren rebuilds the cached children view after a structural
// change.
func (l *toastLayer) syncChildren() {
	l.children = append(l.children[:0], l.root)
	for _, t := range l.slots {
		l.children = append(l.children, t)
	}
}

// Measure passes through to the wrapped root: the toasts float, the
// window's natural size is unchanged.
func (l *toastLayer) Measure(con widget.Constraints) widget.Size {
	return l.root.Measure(con)
}

// Arrange lays the root over the whole rect and stacks the toasts at
// the configured edge, centered along it: the newest card hugs the
// edge, older ones grow toward the interior.
func (l *toastLayer) Arrange(r render.Rect) {
	l.root.Arrange(r)
	if len(l.slots) == 0 {
		return
	}
	maxW := min(max(0, r.W-2*l.margin), toastMaxWidth)
	con := widget.Constraints{Max: widget.Size{W: maxW, H: max(0, r.H/2)}}
	if l.pos == ToastTop {
		y := r.Y + l.margin
		for _, t := range slices.Backward(l.slots) {
			rect := l.slotRect(t, con, r, y)
			t.Arrange(rect)
			y += rect.H + toastSpacing
		}
		return
	}
	y := r.Y + r.H - l.margin
	for _, t := range slices.Backward(l.slots) {
		rect := l.slotRect(t, con, r, y)
		rect.Y -= rect.H
		t.Arrange(rect)
		y -= rect.H + toastSpacing
	}
}

// slotRect measures one toast against the stack constraints and
// centers it horizontally at baseline y (the card's top edge for a
// top stack; the caller flips it for a bottom stack).
func (l *toastLayer) slotRect(t *widget.Toast, con widget.Constraints, r render.Rect, y int) render.Rect {
	nat := t.Measure(con)
	return render.Rect{
		X: r.X + max(0, (r.W-nat.W)/2),
		Y: y,
		W: min(nat.W, max(0, r.W-2*l.margin)),
		H: nat.H,
	}
}

// Paint draws the wrapped root, then the toasts on top.
func (l *toastLayer) Paint(cv *render.Canvas) {
	l.root.Paint(cv)
	for _, t := range l.slots {
		t.Paint(cv)
	}
}

// HitTest resolves toasts first (they float above the root), then the
// wrapped root.
func (l *toastLayer) HitTest(p widget.Point) widget.Widget {
	for _, t := range slices.Backward(l.slots) {
		if hit := t.HitTest(p); hit != nil {
			return hit
		}
	}
	return l.root.HitTest(p)
}

// add appends a toast, dropping — and thereby closing — the oldest
// past the cap. Structural changes repaint through the window flag.
func (l *toastLayer) add(t *widget.Toast) {
	for len(l.slots) >= l.max && len(l.slots) > 0 {
		oldest := l.slots[0]
		l.slots = l.slots[1:]
		oldest.Close()
		l.markDirty()
	}
	l.slots = append(l.slots, t)
	l.syncChildren()
	l.markDirty()
}

// remove detaches a toast whose exit fade landed (or that the stack
// dropped); the last removal unwraps the layer, handing the window
// its bare tree back.
func (l *toastLayer) remove(t *widget.Toast) {
	for i, s := range l.slots {
		if s == t {
			l.slots = append(l.slots[:i], l.slots[i+1:]...)
			break
		}
	}
	t.Close()
	l.syncChildren()
	l.markDirty()
	if len(l.slots) == 0 {
		l.close()
	}
}

// close tears the layer down — toasts stop, the wrapped root is
// restored — and detaches it from its window.
func (l *toastLayer) close() {
	for _, s := range l.slots {
		s.Close()
	}
	l.slots = nil
	l.syncChildren()
	if l.hw != nil && l.hw.router.Root == widget.Widget(l) {
		l.hw.router.Root = l.root
		l.markDirty()
	}
	l.hw = nil
}

// markDirty schedules a full repaint: structural toast changes are
// rare, and the window flag is the one invalidation an app-side
// wrapper owns.
func (l *toastLayer) markDirty() {
	if l.hw != nil {
		l.hw.dirty = true
	}
}
