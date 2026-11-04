package app

import (
	"slices"

	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/widget"
)

// Scale returns the device scale the window presents at, num device
// pixels per denom logical ones; denom is 120. The compositor reports it
// before the window's first frame; until then it is the scale the window
// was created at.
func (w *Window) Scale() (num, denom int) { return w.app.scaleOf(w) }

// OnScaleChanged runs fn on the loop each time the window's device scale
// changes, with the new one as Scale reports it, until cancel.
func (w *Window) OnScaleChanged(fn func(num, denom int)) (cancel func()) {
	return w.app.onScaleChanged(w, fn)
}

// Scale returns the device scale the layer surface presents at, as
// Window.Scale does.
func (l *LayerWindow) Scale() (num, denom int) { return l.app.scaleOf(l) }

// OnScaleChanged runs fn on the loop each time the layer surface's device
// scale changes, as Window.OnScaleChanged does.
func (l *LayerWindow) OnScaleChanged(fn func(num, denom int)) (cancel func()) {
	return l.app.onScaleChanged(l, fn)
}

// Scale returns the device scale the popover presents at: its host's,
// when it opened.
func (p *Popover) Scale() (num, denom int) { return int(p.frac), scale.Denom }

func (a *Application) scaleOf(h Host) (num, denom int) {
	return int(a.fracFor(h)), scale.Denom
}

func (a *Application) onScaleChanged(h Host, fn func(num, denom int)) (cancel func()) {
	w := a.hostWindowOf(h)
	if w == nil || fn == nil {
		return func() {}
	}
	p := &fn
	w.scaleListeners = append(w.scaleListeners, p)
	return func() {
		w.scaleListeners = slices.DeleteFunc(w.scaleListeners, func(q *func(num, denom int)) bool { return q == p })
	}
}

func (w *hostWindow) noteScale() {
	if !w.scaleKnown || w.router == nil {
		return
	}
	root := w.styleRoot
	if root == nil {
		root = w.router.Root
	}
	if root == nil || (root == w.notedRoot && w.frac120 == w.notedFrac) {
		return
	}
	w.notedRoot, w.notedFrac = root, w.frac120
	widget.SetDeviceScale(root, int(w.frac120), scale.Denom)
}

func (w *hostWindow) scaleChanged() {
	for _, fn := range slices.Clone(w.scaleListeners) {
		(*fn)(int(w.frac120), scale.Denom)
	}
}
