package widget

import "github.com/stubbedev/lys-lib/render"

// RevealRect scrolls every Scroll above w just far enough that r, in
// root coordinates, shows in its viewport: a rect below the viewport
// lands at its bottom edge, one above at its top, one taller than the
// viewport with its top showing. w's scroll-margin grows the target
// and each scroll's scroll-padding insets its viewport, and a
// scroll-behavior: smooth scroll animates to the reveal. The
// innermost scroll moves first and the outer ones reveal where r then
// sits, the way GTK's viewports scroll a focused widget into view.
// The scrolls re-lay out within the frame.
func RevealRect(w Widget, r render.Rect) { reveal(w, expandScrollMargin(w, r), true) }

// reveal scrolls the Scrolls above w toward r: every one, or the
// nearest alone.
func reveal(w Widget, r render.Rect, all bool) {
	for p := parentOf(w); p != nil; p = parentOf(p) {
		s, ok := p.(*Scroll)
		if !ok {
			continue
		}
		dx, dy := s.revealDelta(r)
		ox, oy := s.offX, s.offY
		tx, ty := s.scrollTo(ox+dx, oy+dy)
		if tx != ox || ty != oy {
			s.InvalidateLayout()
			r.X -= tx - ox
			r.Y -= ty - oy
		}
		if !all {
			return
		}
	}
}

// revealDelta is the offset change that brings r into the viewport,
// inset by the computed scroll-padding.
func (s *Scroll) revealDelta(r render.Rect) (dx, dy int) {
	pad := s.scrollPadding()
	view := render.Rect{X: s.view.X + pad.Left, Y: s.view.Y + pad.Top, W: s.viewW - pad.Horizontal(), H: s.viewH - pad.Vertical()}
	return revealAxis(r.X, r.W, view.X, view.W), revealAxis(r.Y, r.H, view.Y, view.H)
}

// revealAxis moves [pos, pos+size) into [view, view+viewSize) along
// one axis: the near edge when it starts before the view or is too
// long for it, the far edge when it ends past the view.
func revealAxis(pos, size, view, viewSize int) int {
	switch {
	case pos < view || size > viewSize:
		return pos - view
	case pos+size > view+viewSize:
		return pos + size - (view + viewSize)
	}
	return 0
}

// boundsOf is w's arranged rect, empty for a widget that keeps none.
func boundsOf(w Widget) render.Rect {
	if b, ok := w.(Boundser); ok {
		return b.Bounds()
	}
	return render.Rect{}
}
