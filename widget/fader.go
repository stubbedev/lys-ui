package widget

import "github.com/stubbedev/lys-lib/render"

// Fader modulates one child's whole subtree: every color the child and
// its descendants paint is scaled by the fader's opacity — fills,
// borders, text, and image blits alike — without threading an alpha
// through their colors. It is the one-off shape of Opacity: wrap a
// child, tween SetOpacity from the animation clock
// (anim.Animate(dur, f.SetOpacity)), and discard it with the subtree.
//
// Layout is a pass-through — the fader claims and paints exactly the
// child's rect — so its invalidation covers everything the fade
// changes. The subtree still hit-tests at zero opacity: the factor is
// visual, and a mid-fade menu must keep taking clicks.
type Fader struct {
	node
	Opacity
	child Widget
}

// NewFader wraps child in a fully opaque fader.
func NewFader(child Widget) *Fader {
	f := &Fader{child: child}
	f.alpha = 1
	f.bindOpacity(f)
	return f
}

// Child returns the wrapped widget.
func (f *Fader) Child() Widget { return f.child }

// Measure reports the child's request.
func (f *Fader) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := f.measureHit(axis, forSize); ok {
		return sz
	}
	return f.measureStore(axis, forSize, measureChild(f, f.child, axis, forSize))
}

// Arrange passes the rect straight through to the child.
func (f *Fader) Arrange(r render.Rect) {
	f.ArrangeRoot(r)
	arrangeChild(f.child, r)
	setParents(f, f.child)
}

// ArrangeRoot records the fader's own rect.
func (f *Fader) ArrangeRoot(r render.Rect) { f.node.Arrange(r) }

// Paint paints the child under the fader's opacity; see
// Opacity.paintChild for the zero/one fast paths.
func (f *Fader) Paint(cv *render.Canvas) {
	f.paintChild(cv, f.child)
}

// HitTest returns the deepest widget under p: the child's pick, or the
// fader itself over its own padding-less bounds — opacity never
// empties the hit region.
func (f *Fader) HitTest(p Point) Widget {
	if hit := HitChild(f.child, p); hit != nil {
		return hit
	}
	return f.HitLeaf(f, p)
}

// Children exposes the child for the damage collector, focus
// traversal, and the accessibility walk.
func (f *Fader) Children() []Widget {
	if f.child == nil {
		return nil
	}
	return []Widget{f.child}
}

// appendChildren appends the wrapped child, matching Children.
func (f *Fader) appendChildren(buf []Widget) []Widget {
	if f.child != nil {
		return append(buf, f.child)
	}
	return buf
}

// SetEnabled turns the fader's child on or off through the per-query
// enable walk, like Box.
func (f *Fader) SetEnabled(enabled bool) {
	f.node.SetEnabled(enabled)
	invalidateTree(f)
}
