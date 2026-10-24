package widget

import (
	"math"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// LayoutMotion is how a widget animates a change of its laid-out rect:
// not at all, or on one of the named springs. The stylesheet's
// -lys-layout-motion outranks it, as it outranks every constructor
// default; containers opt their children in with a selector
// (`.list > * { -lys-layout-motion: smooth; }`).
type LayoutMotion = style.LayoutMotion

// The layout motions.
const (
	LayoutMotionNone   = style.LayoutMotionNone
	LayoutMotionSmooth = style.LayoutMotionSmooth
	LayoutMotionSnappy = style.LayoutMotionSnappy
	LayoutMotionBouncy = style.LayoutMotionBouncy
)

// layoutMotionPrecision is the layout spring's settle precision, in
// logical pixels: a tenth is invisible and ends the motion promptly.
const layoutMotionPrecision = 0.1

// SetLayoutMotion animates the widget's moves and resizes on m: when
// layout puts it somewhere new, it springs there from where it was
// shown. Layout itself runs once, at the destination; the frames in
// between paint the laid-out widget translated to the spring's
// position and clipped to its size, so text never stretches. A widget
// inserted into an already shown parent grows in from no height.
func (n *node) SetLayoutMotion(m LayoutMotion) {
	n.layoutMotion = m
	n.Invalidate()
}

// LayoutMotion returns the programmatic layout motion.
func (n *node) LayoutMotion() LayoutMotion { return n.layoutMotion }

// layoutMotionState is a widget's running layout animation.
type layoutMotionState struct {
	spring *anim.Spring
	// target is the laid-out rect it heads for, cur the rect it is
	// shown at now.
	target, cur render.Rect
	layer       *render.Layer
}

// motionSpring maps a motion to its spring.
func motionSpring(m LayoutMotion) anim.SpringParams {
	switch m {
	case LayoutMotionSnappy:
		return anim.SpringSnappy
	case LayoutMotionBouncy:
		return anim.SpringBouncy
	}
	return anim.SpringSmooth
}

// effLayoutMotion is the widget's motion: the stylesheet's when it
// declares one, else the programmatic one.
func (n *node) effLayoutMotion(w Widget) LayoutMotion {
	if v := n.style(w); v.Has(style.PropLayoutMotion) {
		return v.LayoutMotion
	}
	return n.layoutMotion
}

// moving updates the widget's layout motion for this paint and reports
// whether it is mid-flight: a new laid-out rect retargets the spring
// from where the widget is shown, a first paint inside an already
// shown parent starts from no height, and a widget without a motion
// drops any it had.
func (n *node) moving(w Widget) bool {
	walk := damageWalks.Load()
	inserted := n.shownWalk == 0 && n.parentShownBefore(walk)
	n.shownWalk = walk
	m := n.effLayoutMotion(w)
	if m == LayoutMotionNone {
		if n.motion != nil {
			n.motion.spring.Stop()
			n.motion = nil
		}
		return false
	}
	b := n.bounds
	if n.motion == nil {
		start := b
		if inserted {
			start.H = 0
		}
		n.motion = &layoutMotionState{target: start, cur: start}
		mo := n.motion
		mo.spring = anim.NewSpring(motionSpring(m), rectComps(start), func(pos []float64) {
			next := compsRect(pos)
			n.InvalidateRect(mo.cur.Union(next))
			mo.cur = next
		})
		mo.spring.SetPrecision(layoutMotionPrecision)
	}
	mo := n.motion
	if b != mo.target {
		mo.target = b
		mo.spring.SetParams(motionSpring(m))
		mo.spring.SetTarget(rectComps(b)...)
	}
	return mo.cur != b
}

// parentShownBefore reports whether the widget's parent showed in a
// walk before this one - a widget appearing inside it then is an
// insertion, not part of the first frame - and stamps the parent as
// shown in this one (a root is painted directly, never as a child).
func (n *node) parentShownBefore(walk uint64) bool {
	if n.parent == nil {
		return false
	}
	pn := nodeOf(n.parent)
	if pn == nil {
		return false
	}
	before := pn.shownWalk != 0 && pn.shownWalk < walk
	pn.shownWalk = walk
	return before
}

// paintMoving paints w mid-motion: laid out at its destination, into
// a layer that composites translated to the shown rect and clipped to
// its size.
func (n *node) paintMoving(cv *render.Canvas, w Widget) {
	mo := n.motion
	t, c := n.bounds, mo.cur
	dev := cv.MapRect(t)
	if dev.Empty() || c.Empty() {
		return
	}
	l := cv.Layer(mo.layer, dev)
	mo.layer = l
	w.Paint(l.Canvas())
	num, denom := cv.DeviceScale()
	f := float64(num) / float64(denom)
	prev := cv.PushClip(c)
	cv.Composite(l, dev, render.Translate(float64(c.X-t.X)*f, float64(c.Y-t.Y)*f), 1)
	cv.PopClip(prev)
}

// ShownRect is the rect w is shown at: its laid-out bounds, or
// mid-motion the rect its layout motion paints it at. A surface's
// backing (a popup's plate and shadow) follows it, so the visible box
// springs with the content.
func ShownRect(w Widget) render.Rect {
	if n := nodeOf(w); n != nil && n.motion != nil {
		return n.motion.cur
	}
	if b, ok := w.(Boundser); ok {
		return b.Bounds()
	}
	return render.Rect{}
}

// LayoutShownSize is the size w is shown at and whether it animates
// its layout: mid-motion the size its spring paints it at, else its
// laid-out size. A surface sized from its content (a popover
// shrinking) keeps its size until that is the content's new size, so
// the content springs down inside it before the surface follows.
func LayoutShownSize(w Widget) (Size, bool) {
	n := nodeOf(w)
	if n == nil || n.effLayoutMotion(w) == LayoutMotionNone {
		return Size{}, false
	}
	r := ShownRect(w)
	return Size{W: r.W, H: r.H}, true
}

// rectComps and compsRect convert a rect to and from spring components.
func rectComps(r render.Rect) []float64 {
	return []float64{float64(r.X), float64(r.Y), float64(r.W), float64(r.H)}
}

func compsRect(c []float64) render.Rect {
	round := func(v float64) int { return int(math.Round(v)) }
	return render.Rect{X: round(c[0]), Y: round(c[1]), W: max(0, round(c[2])), H: max(0, round(c[3]))}
}
