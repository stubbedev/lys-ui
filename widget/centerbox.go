package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// CenterBox lays three children like gtk::CenterBox along its
// orientation (Row by default): the center one at the middle of the
// allocation, the start and end children at the ends. The center child
// stays centered however long the sides run, until one would overlap
// it: then it moves just clear of that side (GtkCenterLayout).
type CenterBox struct {
	node
	start, center, end Widget
	axis               Axis
}

// NewCenterBox returns a three-slot row; any slot may be nil.
func NewCenterBox(start, center, end Widget) *CenterBox {
	b := &CenterBox{start: start, center: center, end: end}
	setParents(b, nonNil(start, center, end)...)
	return b
}

// SetOrientation lays the slots along a
// (gtk_orientable_set_orientation): a Column runs top to bottom.
func (b *CenterBox) SetOrientation(a Axis) {
	if b.axis != a {
		b.axis = a
		b.InvalidateLayout()
	}
}

// Orientation is the axis the slots run along.
func (b *CenterBox) Orientation() Axis { return b.axis }

func nonNil(ws ...Widget) []Widget {
	out := make([]Widget, 0, len(ws))
	for _, w := range ws {
		if w != nil {
			out = append(out, w)
		}
	}
	return out
}

// Children exposes the slots in start, center, end order.
func (b *CenterBox) Children() []Widget { return nonNil(b.start, b.center, b.end) }

// along is s's extent on the box's axis, across the extent on the
// other.
func (b *CenterBox) along(s Size) (main, cross int) {
	if b.axis == Column {
		return s.H, s.W
	}
	return s.W, s.H
}

// Measure sizes to the three naturals end to end along the axis, as
// thick as the thickest.
func (b *CenterBox) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := b.measureHit(axis, forSize); ok {
		return sz
	}
	v := b.style(b)
	return b.measureStore(axis, forSize, measureBox(v, boxOf(v, render.Insets{}), axis, forSize, func(axis Axis, forSize int) Sizes {
		var req Sizes
		for _, c := range b.Children() {
			if axis == b.axis {
				req = req.plus(measureChild(b, c, axis, forSize))
				continue
			}
			req = req.widest(measureChild(b, c, axis, measureChild(b, c, b.axis, Unknown).Nat))
		}
		return req
	}))
}

// ArrangeRoot records the box's own rect.
func (b *CenterBox) ArrangeRoot(r render.Rect) { b.node.Arrange(r) }

// Arrange places the start slot at the start, the end slot at the end,
// and the center slot at the middle unless a side would overlap it;
// every slot spans the box's thickness.
func (b *CenterBox) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(b.style(b), render.Insets{}), r)
	b.ArrangeRoot(border)
	size, cross := b.along(Size{W: inner.W, H: inner.H})
	extent := func(w Widget) int {
		if w == nil {
			return 0
		}
		return min(measureChild(b, w, b.axis, cross).Nat, size)
	}
	ss, cs, es := extent(b.start), extent(b.center), extent(b.end)
	cpos := centerPos(size, ss, cs, es)
	b.place(b.start, inner, 0, ss)
	b.place(b.center, inner, cpos, cs)
	b.place(b.end, inner, size-es, es)
}

// centerPos is the center slot's offset: the middle of size, moved
// clear of the start slot, then of the end slot.
func centerPos(size, start, center, end int) int {
	pos := (size - center) / 2
	if pos < start {
		pos = start
	} else if pos+center > size-end {
		pos = max(size-end-center, start)
	}
	return pos
}

// place arranges w over [off, off+n) along the axis, across all of r.
func (b *CenterBox) place(w Widget, r render.Rect, off, n int) {
	if w == nil {
		return
	}
	if b.axis == Column {
		arrangeChild(w, render.Rect{X: r.X, Y: r.Y + off, W: r.W, H: n})
		return
	}
	arrangeChild(w, render.Rect{X: r.X + off, Y: r.Y, W: n, H: r.H})
}

// HitTest returns the slot under p, else the box.
func (b *CenterBox) HitTest(p Point) Widget {
	for _, c := range b.Children() {
		if hit := HitChild(c, p); hit != nil {
			return hit
		}
	}
	return b.HitLeaf(b, p)
}

// Paint draws the box's CSS layers, then the slots.
func (b *CenterBox) Paint(cv *render.Canvas) {
	v := b.style(b)
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, b.bounds, radii)
	cv = fx.cv
	bg := pickc(0, v, style.PropBackgroundColor, 0)
	if bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, b.bounds, radii, borderOf(v), bg)
	}
	for _, c := range b.Children() {
		if IsVisible(c) {
			PaintChild(cv, c)
		}
	}
	paintOutline(cv, v, b.bounds, radii)
	fx.pop()
}
