package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

const scrollOverscan = 64

// ScrollMove is a viewport whose pixels moved by DX, DY as they stand:
// a frame can copy them there instead of painting them, and owes only
// what the move exposed, which the damage holds. All in logical pixels.
type ScrollMove struct {
	View   render.Rect
	DX, DY int
}

var (
	arrangeShifted bool
	arrangeCulling bool
	arrangeCull    render.Rect
	shiftBroken    bool
	culledNodes    int
	walkTakesMoves bool
	walkMoves      []ScrollMove
	walkClip       render.Rect
	walkClipped    bool
	walkShift      viewShift
	walkShiftNext  viewShift
	// walkPlateCorners are the rounded plate corners under the view the
	// last blittable check passed over: flat everywhere else, so a move
	// still copies, and owes them where they are and where it carried
	// their pixels.
	walkPlateCorners []render.Rect
)

type viewShift struct{ dx, dy int }

func (s viewShift) moved() bool { return s.dx != 0 || s.dy != 0 }

type shiftState struct {
	shifted, culling bool
	cull             render.Rect
}

func beginShift(view render.Rect, culling bool) shiftState {
	st := shiftState{arrangeShifted, arrangeCulling, arrangeCull}
	arrangeShifted, shiftBroken = true, false
	if !culling {
		return st
	}
	cull := render.UniformInsets(scrollOverscan).Grow(view)
	if arrangeCulling {
		cull = cull.Intersect(arrangeCull)
	}
	arrangeCulling, arrangeCull = true, cull
	return st
}

func endShift(st shiftState) (broken bool) {
	broken = shiftBroken
	arrangeShifted, arrangeCulling, arrangeCull = st.shifted, st.culling, st.cull
	shiftBroken = st.shifted && broken
	return broken
}

func shifted(r render.Rect, dx, dy int) render.Rect {
	r.X += dx
	r.Y += dy
	return r
}

func (n *node) cullBy(dx, dy int) {
	n.bounds = shifted(n.bounds, dx, dy)
	n.inkBox = shifted(n.inkBox, dx, dy)
	n.solidBox = shifted(n.solidBox, dx, dy)
	if !n.culled {
		n.culled = true
		culledNodes++
	}
}

func (n *node) settleCulled() {
	var top *node
	for a := n; a != nil; a = parentNodeOf(a) {
		if a.culled {
			top = a
		}
	}
	if top == nil || top.self == nil {
		return
	}
	st := beginShift(render.Rect{}, false)
	wasCulling := arrangeCulling
	arrangeCulling = false
	top.self.Arrange(top.slot)
	arrangeCulling = wasCulling
	endShift(st)
}

func (b *Box) settleShiftable(inner render.Rect) {
	b.shiftable = !inner.Empty() && len(b.departing) == 0
	for _, c := range b.child {
		n := nodeOf(c.w)
		switch {
		case n == nil:
			b.shiftable = false
		case !ownVisible(c.w):
			n.slotSet = false
		case flowOf(c.w) == flowOut || !n.slotSet:
			b.shiftable = false
		}
	}
}

func (b *Box) canShift() bool {
	s := b.arranged
	return !b.arrangeStale && b.shiftable && !s.volatile && s.style == styleGen && s.theme == themeGen
}

func (b *Box) shiftTo(r render.Rect) {
	dx, dy := r.X-b.arranged.rect.X, r.Y-b.arranged.rect.Y
	b.arranged.rect = r
	border, _ := boxRects(b.box(b.style(b)), r)
	b.ArrangeRoot(border)
	for _, c := range b.child {
		n := nodeOf(c.w)
		if !n.slotSet {
			continue
		}
		n.slot = shifted(n.slot, dx, dy)
		if arrangeCulling && n.inkClosed && shifted(n.inkBox, dx, dy).Intersect(arrangeCull).Empty() {
			n.cullBy(dx, dy)
			n.bubbleInk()
			continue
		}
		c.w.Arrange(n.slot)
	}
}

func (n *node) drainShift(sh *viewShift, view, area render.Rect, blit bool) {
	moved := *sh
	*sh = viewShift{}
	if !moved.moved() || n.invalid {
		return
	}
	if !blit || !walkTakesMoves || max(moved.dx, -moved.dx) >= area.W || max(moved.dy, -moved.dy) >= area.H {
		n.extras = append(n.extras, view)
		return
	}
	walkMoves = append(walkMoves, ScrollMove{View: area, DX: moved.dx, DY: moved.dy})
	walkShiftNext = moved
	switch {
	case moved.dy < 0:
		n.extras = append(n.extras, render.Rect{X: area.X, Y: area.Y + area.H + moved.dy, W: area.W, H: -moved.dy})
	case moved.dy > 0:
		n.extras = append(n.extras, render.Rect{X: area.X, Y: area.Y, W: area.W, H: moved.dy})
	}
	switch {
	case moved.dx < 0:
		n.extras = append(n.extras, render.Rect{X: area.X + area.W + moved.dx, Y: area.Y, W: -moved.dx, H: area.H})
	case moved.dx > 0:
		n.extras = append(n.extras, render.Rect{X: area.X, Y: area.Y, W: moved.dx, H: area.H})
	}
	if side := view.W - area.W; side > 0 {
		n.extras = append(n.extras, render.Rect{X: area.X + area.W, Y: view.Y, W: side, H: view.H})
	}
	if below := view.H - area.H; below > 0 {
		n.extras = append(n.extras, render.Rect{X: view.X, Y: area.Y + area.H, W: view.W, H: below})
	}
}

// takeShift drains a view's shift as drainShift does, and owes the plate
// corners a copied view carried over.
func (n *node) takeShift(sh *viewShift, view, area render.Rect, blit bool) {
	moved := *sh
	n.drainShift(sh, view, area, blit)
	if !blit || !moved.moved() {
		return
	}
	for _, c := range walkPlateCorners {
		n.extras = append(n.extras, c)
		if to := shifted(c, moved.dx, moved.dy).Intersect(area); !to.Empty() {
			n.extras = append(n.extras, to)
		}
	}
	walkPlateCorners = walkPlateCorners[:0]
}

func flatBehind(n *node, w Widget, area render.Rect) bool {
	v := n.style(w)
	if n.sharedTag != "" || n.motion != nil || n.anims != nil || n.trCancel != nil || n.pseudo != nil {
		return false
	}
	if effectsHeavy(v) || v.Has(style.PropTransform) || v.Has(style.PropBackdropFilter) || v.Bg.Painted() || v.Has(style.PropBorderImageSource) {
		return false
	}
	for _, sh := range v.Shadow.List() {
		if sh.Inset {
			return false
		}
	}
	if pickc(0, v, style.PropBackgroundColor, 0) == 0 && borderOf(v).Zero() {
		return true
	}
	c := radiusOr(v, 0)
	inner := render.UniformInsets(max(c.TopLeft, c.TopRight, c.BottomRight, c.BottomLeft)).Shrink(borderOf(v).Shrink(n.bounds))
	return area.Intersect(inner) == area
}

func clearOfSiblings(p Widget, self Widget, area render.Rect) bool {
	var b *Box
	switch v := p.(type) {
	case *Box:
		b = v
	case *Scroll:
		return true
	case *Morph:
		if v.reveal < 1 {
			return false
		}
		var ok bool
		walkPlateCorners, ok = v.plateCorners(area, walkPlateCorners)
		return ok
	default:
		return false
	}
	for _, c := range b.child {
		if c.w == self || !ownVisible(c.w) {
			continue
		}
		n := nodeOf(c.w)
		if n == nil || !n.inkClosed {
			return false
		}
		box := n.solidBox
		if n.focusWithin > 0 {
			box = n.inkBox
		}
		if !box.Intersect(area).Empty() {
			return false
		}
	}
	return true
}

func (n *node) blittable(self Widget, area render.Rect) bool {
	if !n.inkClosed || !flatBehind(n, self, area) {
		return false
	}
	at := self
	for p := parentOf(at); p != nil; at, p = p, parentOf(p) {
		pn := nodeOf(p)
		if pn == nil || !clearOfSiblings(p, at, area) || !flatBehind(pn, p, area) {
			return false
		}
	}
	return true
}

func (s *Scroll) contentArea() render.Rect {
	return render.Rect{X: s.view.X, Y: s.view.Y, W: s.viewW, H: s.viewH}
}

func (s *Scroll) arrangeContent(to render.Rect) {
	dx, dy := to.X-s.childAt.X, to.Y-s.childAt.Y
	offsetMoved := s.offX != s.arrOffX || s.offY != s.arrOffY
	same := s.viewAt == s.view && to.W == s.childAt.W && to.H == s.childAt.H && (dx != 0 || dy != 0)
	s.viewAt, s.childAt = s.view, to
	if b, ok := s.child.(*Box); ok && same && offsetMoved && b.canShift() {
		st := beginShift(s.view, true)
		arrangeChild(s.child, to)
		if endShift(st) {
			s.Invalidate()
			return
		}
		s.shift.dx += dx
		s.shift.dy += dy
		return
	}
	arrangeChild(s.child, to)
	if offsetMoved {
		s.Invalidate()
	}
}

func (s *Scroll) takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool) {
	if s.child != nil && (s.offX != s.arrOffX || s.offY != s.arrOffY) {
		s.invalid = true
	}
	_, both := s.gutterStable()
	area := s.contentArea()
	walkPlateCorners = walkPlateCorners[:0]
	blit := s.shift.moved() && !both && s.overshoot == [2]float64{} && s.blittable(s, area)
	s.takeShift(&s.shift, s.view, area, blit)
	return s.node.takeDamage()
}

func (s *Scroll) damageClip() (render.Rect, bool) { return s.view, true }

func (l *List) damageClip() (render.Rect, bool) { return l.view, true }

func (b *Box) damageClip() (render.Rect, bool) {
	if !b.inkClips {
		return render.Rect{}, false
	}
	return b.overflowClip(b.style(b))
}

func (l *List) takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool) {
	walkPlateCorners = walkPlateCorners[:0]
	blit := l.shift.moved() && l.blittable(l, l.view)
	l.takeShift(&l.shift, l.view, l.view, blit)
	return l.node.takeDamage()
}

func (n *node) ownInkBounded(w Widget) bool {
	b, ok := w.(inkBounder)
	if !ok || n.opensInk(w) {
		return false
	}
	owner, _, closed := b.ownInk()
	return closed && owner == w
}
