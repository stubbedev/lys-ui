package widget

import (
	"math"
	"slices"

	"github.com/stubbedev/lys-lib/render"
)

// fixedChild is one Fixed child: the widget and its position in
// logical pixels.
type fixedChild struct {
	w    Widget
	x, y int
}

// Fixed places its children at explicit coordinates, GTK's Fixed:
// there are no tracks and no negotiation - a child sits at the (x, y)
// it was put at, negative coordinates included, at its natural size,
// and children may overlap freely, later puts painting and hitting on
// top of earlier ones.
//
// The container measures to the union of its children's extents: from
// the leftmost child's left edge to the rightmost child's right edge,
// likewise vertically, so children at negative coordinates grow the
// request in that direction while still arranging at their negative
// offset - the painter's clip decides visibility, exactly like a
// squeezed Box child overflowing its cell. An empty Fixed measures as
// nothing.
type Fixed struct {
	node
	child []*fixedChild
}

// NewFixed returns an empty fixed container.
func NewFixed() *Fixed { return &Fixed{} }

// Put adds w at (x, y) and returns the fixed for chaining. Putting a
// widget that is already in the container moves it to the new
// coordinates (a widget sits in the fixed once). The measure cache
// drops so the next frame reflows.
func (f *Fixed) Put(w Widget, x, y int) *Fixed {
	for _, c := range f.child {
		if c.w == w {
			f.move(c, x, y)
			return f
		}
	}
	f.child = append(f.child, &fixedChild{w: w, x: x, y: y})
	f.InvalidateLayout()
	restyleChildren(f)
	return f
}

// Move moves an attached child to (x, y) in place, without detaching
// it, so its focus and state survive; the old and new pixels both
// repaint. Unknown widgets are ignored.
func (f *Fixed) Move(w Widget, x, y int) *Fixed {
	for _, c := range f.child {
		if c.w == w {
			f.move(c, x, y)
			return f
		}
	}
	return f
}

// move applies new coordinates to c: the child's current pixels are
// owed a repaint for where it is leaving, and the layout drop reflows
// the union and re-arranges the child, whose moved rect damages the
// rest.
func (f *Fixed) move(c *fixedChild, x, y int) {
	if c.x == x && c.y == y {
		return
	}
	if v, ok := c.w.(interface{ Invalidate() }); ok {
		v.Invalidate()
	}
	c.x, c.y = x, y
	f.InvalidateLayout()
}

// Remove detaches w and reports whether it was attached. The removed
// widget's parent link clears and the removal hook fires, like every
// tree mutation; the measure cache drops so the next frame reflows
// without it.
func (f *Fixed) Remove(w Widget) bool {
	for i, c := range f.child {
		if c.w == w {
			notifyRemoved(w)
			f.child = slices.Delete(f.child, i, i+1)
			clearParents(w)
			f.InvalidateLayout()
			restyleChildren(f)
			return true
		}
	}
	return false
}

// Children exposes the children in put order, the order focus
// traversal and painting follow.
func (f *Fixed) Children() []Widget {
	return f.appendChildren(make([]Widget, 0, len(f.child)))
}

// appendChildren appends the children in put order, matching Children
// - the snapshot the per-frame damage walk buffers instead of a fresh
// slice per container per frame.
func (f *Fixed) appendChildren(buf []Widget) []Widget {
	for _, c := range f.child {
		buf = append(buf, c.w)
	}
	return buf
}

// SetEnabled turns the fixed's subtree on or off through the per-query
// enable walk, like Box.
func (f *Fixed) SetEnabled(enabled bool) {
	f.node.SetEnabled(enabled)
	invalidateTree(f)
}

// Measure measures every child at its natural size and reports the
// union of their extents from their coordinates. The result is cached
// until an InvalidateLayout anywhere in the subtree.
func (f *Fixed) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := f.measureHit(axis, forSize); ok {
		return sz
	}
	left, top, right, bottom := math.MaxInt, math.MaxInt, math.MinInt, math.MinInt
	for _, c := range f.child {
		if !IsVisible(c.w) {
			setParents(f, c.w)
			continue
		}
		nat := naturalChild(f, c.w)
		left, top = min(left, c.x), min(top, c.y)
		right, bottom = max(right, c.x+nat.W), max(bottom, c.y+nat.H)
	}
	if left > right || top > bottom {
		return f.measureStore(axis, forSize, Sizes{})
	}
	return f.measureStore(axis, forSize, FixedSizes(Size{W: right - left, H: bottom - top}.Along(axis)))
}

// Arrange places every child at its coordinates at its natural size,
// relative to the container's origin.
func (f *Fixed) Arrange(r render.Rect) {
	f.ArrangeRoot(r)
	for _, c := range f.child {
		nat := naturalChild(f, c.w)
		arrangeChild(c.w, render.Rect{
			X: r.X + c.x, Y: r.Y + c.y, W: nat.W, H: nat.H,
		})
	}
	setParents(f, f.Children()...)
}

// ArrangeRoot records the container's own rect.
func (f *Fixed) ArrangeRoot(r render.Rect) {
	f.node.Arrange(r)
}

// Paint draws the children in put order, so overlapping children
// composite bottom to top.
func (f *Fixed) Paint(cv *render.Canvas) {
	for _, c := range f.child {
		PaintChild(cv, c.w)
	}
}

// HitTest returns the topmost child under p (later puts sit on top),
// or the fixed itself when p is inside its bounds but over no child.
func (f *Fixed) HitTest(p Point) Widget {
	for i := range slices.Backward(f.child) {
		if hit := HitChild(f.child[i].w, p); hit != nil {
			return hit
		}
	}
	return f.HitLeaf(f, p)
}
