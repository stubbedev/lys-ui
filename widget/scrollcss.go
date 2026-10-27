package widget

// The CSS scroll properties' widget half: scroll-behavior's animated
// programmatic scrolls, scroll-margin and scroll-padding in the reveal
// math, the scroll-snap group's settling, overscroll-behavior's
// chaining, and the scrollbar-color/width/gutter trio. The wheel stays
// instant whatever scroll-behavior says — the spec scopes smooth
// scrolling to programmatic scrolls — and snap runs only when the
// container declares a snap type, so the no-snap paths allocate
// nothing.

import (
	"math"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

// smoothScrollDur is the scroll-behavior: smooth tween's duration.
const smoothScrollDur = 300 * time.Millisecond

// snapProximity is how far a resting offset may sit from a snap
// position — as a fraction of the viewport — before proximity
// strictness lets it stay. Mandatory always snaps; the fraction is
// the UA freedom the spec leaves, and half a viewport is the reading
// browsers settle on.
const snapProximity = 0.5

// snapPoint is one snap position along an axis: the offset that aligns
// a snap area in the viewport, and whether scroll-snap-stop pins it.
type snapPoint struct {
	pos    int
	always bool
}

// smoothStyle reports whether the computed scroll-behavior is smooth.
func (s *Scroll) smoothStyle() bool {
	v := s.scrollStyle()
	return v.Has(style.PropScrollBehavior) && v.ScrollSmooth
}

// scrollStyle returns the scroll's computed style, where the container
// properties live.
func (s *Scroll) scrollStyle() *style.Values { return s.style(s) }

// scrollPadding returns the computed scroll-padding, zero when the
// cascade declares none.
func (s *Scroll) scrollPadding() style.Sides {
	v := s.scrollStyle()
	if !v.HasAny(style.PropScrollPaddingTop, style.PropScrollPaddingRight,
		style.PropScrollPaddingBottom, style.PropScrollPaddingLeft) {
		return style.Sides{}
	}
	return v.ScrollPad
}

// stopSmooth cancels a running smooth-scroll tween.
func (s *Scroll) stopSmooth() {
	if s.cancelScroll != nil {
		s.cancelScroll()
		s.cancelScroll = nil
	}
}

// scrollTo scrolls to the clamped x, y — at once, or eased over
// smoothScrollDur when the computed scroll-behavior is smooth and
// motion is on — and returns where it lands. The target snaps first
// when the container declares a snap type.
func (s *Scroll) scrollTo(x, y int) (int, int) {
	x, y = s.snapTarget(x, y)
	maxX, maxY := s.scrollMax()
	x, y = min(max(0, x), maxX), min(max(0, y), maxY)
	s.stopSmooth()
	if x == s.offX && y == s.offY {
		return x, y
	}
	if !s.smoothStyle() || anim.Instant() {
		s.SetOffset(x, y)
		return x, y
	}
	fx, fy := float64(s.offX), float64(s.offY)
	tx, ty := float64(x), float64(y)
	s.cancelScroll = anim.Play(anim.Animate(smoothScrollDur, func(t float64) {
		s.SetOffset(int(math.Round(fx+(tx-fx)*t)), int(math.Round(fy+(ty-fy)*t)))
	}).Easing(anim.EaseOutCubic))
	return x, y
}

// expandScrollMargin grows r by w's computed scroll-margin on every
// side: the reveal target keeps its margin clear of the viewport edge.
func expandScrollMargin(w Widget, r render.Rect) render.Rect {
	n := nodeOf(w)
	if n == nil {
		return r
	}
	v := n.style(w)
	if !v.HasAny(style.PropScrollMarginTop, style.PropScrollMarginRight,
		style.PropScrollMarginBottom, style.PropScrollMarginLeft) {
		return r
	}
	m := v.ScrollMargin
	return render.Rect{X: r.X - m.Left, Y: r.Y - m.Top, W: r.W + m.Horizontal(), H: r.H + m.Vertical()}
}

// snapStyle returns the computed scroll-snap-type, snapping nothing
// when unset.
func (s *Scroll) snapStyle() style.SnapType {
	v := s.scrollStyle()
	if !v.Has(style.PropScrollSnapType) {
		return style.SnapType{}
	}
	return v.Snap
}

// snapTarget adjusts a scroll target per the container's snap type:
// mandatory lands on the nearest snap position, proximity only when
// one lies within snapProximity of a viewport of it.
func (s *Scroll) snapTarget(x, y int) (int, int) {
	st := s.snapStyle()
	if st.Axis == style.SnapAxisNone {
		return x, y
	}
	if st.Horizontal() {
		x = s.snapAxisTo(Row, x, st.Mandatory)
	}
	if st.Vertical() {
		y = s.snapAxisTo(Column, y, st.Mandatory)
	}
	return x, y
}

// snapCurrent settles both axes on their snap positions, the release
// path after a drag or glide.
func (s *Scroll) snapCurrent() {
	if s.snapStyle().Axis == style.SnapAxisNone {
		return
	}
	if x, y := s.snapTarget(s.offX, s.offY); x != s.offX || y != s.offY {
		s.SetOffset(x, y)
	}
}

// snapAxisTo picks the snap position for one axis's target. A
// scroll-snap-stop: always area between the current offset and the
// target ends the travel there, so a long fling cannot skip a panel.
func (s *Scroll) snapAxisTo(axis Axis, target int, mandatory bool) int {
	pts := s.snapPoints(axis)
	if len(pts) == 0 {
		return target
	}
	cur, view := s.offY, s.viewH
	if axis == Row {
		cur, view = s.offX, s.viewW
	}
	if target != cur {
		for _, p := range pts {
			between := (target > cur && p.pos > cur && p.pos <= target) ||
				(target < cur && p.pos < cur && p.pos >= target)
			if p.always && between {
				target = p.pos
				break
			}
		}
	}
	nearest := pts[0].pos
	for _, p := range pts[1:] {
		if iabs(p.pos-target) < iabs(nearest-target) {
			nearest = p.pos
		}
	}
	if !mandatory && iabs(nearest-target) > int(math.Round(snapProximity*float64(view))) {
		return target
	}
	return nearest
}

// snapPoints walks the scrolled content for snap areas — descendants
// whose scroll-snap-align names an edge on axis — and returns the
// offsets that align each in the viewport, inset by the container's
// scroll-padding, ascending, held inside the scrollable range
// (css-scroll-snap-1 §6.1). The positions come from the children's
// last arranged bounds against the current offset, so they stay right
// through a scroll and re-snap on a relayout.
func (s *Scroll) snapPoints(axis Axis) []snapPoint {
	if s.child == nil {
		return nil
	}
	pad := s.scrollPadding()
	maxX, maxY := s.scrollMax()
	limit := maxY
	if axis == Row {
		limit = maxX
	}
	var pts []snapPoint
	eachDescendant(s.child, func(k Widget) {
		n := nodeOf(k)
		if n == nil {
			return
		}
		v := n.style(k)
		if !v.Has(style.PropScrollSnapAlign) {
			return
		}
		b := boundsOf(k)
		if b.Empty() {
			return
		}
		b = expandScrollMargin(k, b)
		align := v.SnapAlign.Block
		if axis == Row {
			align = v.SnapAlign.Inline
		}
		pos, ok := snapPosFor(axis, s, b, align, pad)
		if !ok {
			return
		}
		pts = append(pts, snapPoint{pos: min(max(pos, 0), limit), always: v.SnapStopAlways})
	})
	slices.SortFunc(pts, func(a, b snapPoint) int { return a.pos - b.pos })
	return pts
}

// snapPosFor maps one child's snap area (its border box grown by its
// scroll-margin) to the offset that aligns it: the aligning edge or
// center meets the scroll-padding-inset viewport edge or center. The
// area's content position is its bounds against the offset the child
// was last arranged at.
func snapPosFor(axis Axis, s *Scroll, b render.Rect, align style.SnapAlignK, pad style.Sides) (int, bool) {
	if align == style.SnapNone {
		return 0, false
	}
	if axis == Row {
		at := s.arrOffX + b.X - s.view.X
		switch align {
		case style.SnapStart:
			return at - pad.Left, true
		case style.SnapEnd:
			return at + b.W - (s.viewW - pad.Right), true
		case style.SnapCenter:
			return at + b.W/2 - (pad.Left + (s.viewW-pad.Horizontal())/2), true
		}
		return 0, false
	}
	at := s.arrOffY + b.Y - s.view.Y
	switch align {
	case style.SnapStart:
		return at - pad.Top, true
	case style.SnapEnd:
		return at + b.H - (s.viewH - pad.Bottom), true
	case style.SnapCenter:
		return at + b.H/2 - (pad.Top + (s.viewH-pad.Vertical())/2), true
	}
	return 0, false
}

// eachDescendant visits w and every descendant below it, the damage
// walk's child order.
func eachDescendant(w Widget, fn func(Widget)) {
	var kids []Widget
	if cb, ok := w.(childBuf); ok {
		kids = cb.appendChildren(nil)
	} else if cs, ok := w.(childser); ok {
		kids = cs.Children()
	}
	for _, k := range kids {
		if k == nil {
			continue
		}
		fn(k)
		eachDescendant(k, fn)
	}
}

// iabs is the integer absolute value.
func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// overscrollMode reads one axis's computed overscroll-behavior, auto
// when the cascade says nothing.
func (s *Scroll) overscrollMode(axis int) style.Overscroll {
	if v := s.scrollStyle(); v.HasAny(style.PropOverscrollX, style.PropOverscrollY) {
		return v.Overscroll[axis]
	}
	return style.OverscrollAuto
}

// overscroll disposes of the motion a clamp swallowed on axis (0 x,
// 1 y): auto chains it to the nearest ancestor Scroll — the root
// scroll, with none above, is last and takes the edge glow — contain
// keeps it here as the glow, none shows nothing at all.
func (s *Scroll) overscroll(axis int, lost int) {
	mode := s.overscrollMode(axis)
	if mode == style.OverscrollNone {
		return
	}
	if mode == style.OverscrollAuto {
		if p := scrollAbove(s); p != nil {
			dx, dy := 0.0, 0.0
			if axis == 0 {
				dx = float64(lost)
			} else {
				dy = float64(lost)
			}
			if p.movePixels(dx, dy) {
				return
			}
		}
	}
	s.pushGlow(axis, float64(lost))
}

// scrollAbove returns the nearest Scroll above s, nil at the root.
func scrollAbove(s *Scroll) *Scroll {
	for p := parentOf(s); p != nil; p = parentOf(p) {
		if sc, ok := p.(*Scroll); ok {
			return sc
		}
	}
	return nil
}

// gutterStable reports the computed scrollbar-gutter: whether the
// bar's space is reserved even without overflow, and whether a
// matching space is reserved on the opposite edge too.
func (s *Scroll) gutterStable() (stable, both bool) {
	v := s.scrollStyle()
	return v.BarGutter&style.GutterStable != 0, v.BarGutter&style.GutterBothEdges != 0
}
