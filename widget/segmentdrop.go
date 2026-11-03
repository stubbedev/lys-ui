package widget

import (
	"math"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

var (
	dropLead  = anim.SpringParams{Response: 200 * time.Millisecond, Damping: 0.72}
	dropTrail = anim.SpringParams{Response: 340 * time.Millisecond, Damping: 1}
)

const (
	dropLeft = iota
	dropTop
	dropRight
	dropBottom
)

// SegmentDrop is a SegmentIndicator's one travelling indicator, painted
// behind the segments as the indicator element under segments. It rests
// on the active segment's box. On a switch its leading edges spring
// ahead and its trailing edges follow, so it stretches towards the
// target, travels there and settles as one shape, along whichever axis
// the segments stack. It shows nothing while no segment is active, and
// it appears and goes at once, without travelling. Under reduced motion
// or a theme without animations it lands on the target at once.
type SegmentDrop struct {
	stylePart
	owner  *SegmentIndicator
	edges  [4]*anim.Spring
	rest   render.Rect
	travel bool
}

func newSegmentDrop(owner *SegmentIndicator) *SegmentDrop {
	d := &SegmentDrop{owner: owner}
	d.SetElement("indicator")
	for i := range d.edges {
		d.edges[i] = anim.NewSpring(dropTrail, []float64{0}, func([]float64) { d.place() })
		d.edges[i].SetPrecision(layoutMotionPrecision)
	}
	setParents(owner, d)
	return d
}

// Moving reports whether the indicator is in flight. A settled or
// hidden indicator is not, and schedules no frame.
func (d *SegmentDrop) Moving() bool {
	for _, e := range d.edges {
		if e.Running() {
			return true
		}
	}
	return false
}

func (d *SegmentDrop) follow(seg *Button) {
	travel := d.travel
	d.travel = false
	if seg == nil || seg.bounds.Empty() {
		d.hide()
		return
	}
	at := d.owner.bounds
	r := seg.bounds
	r.X -= at.X
	r.Y -= at.Y
	switch {
	case r == d.rest && !travel:
	case !d.rest.Empty() && (travel || d.Moving()) && Current().Moves():
		d.head(r)
	default:
		d.snap(r)
	}
	d.rest = r
	d.place()
}

func (d *SegmentDrop) hide() {
	if d.rest.Empty() {
		return
	}
	for _, e := range d.edges {
		e.Stop()
	}
	d.rest = render.Rect{}
	d.bounds = render.Rect{}
	d.owner.Invalidate()
}

func (d *SegmentDrop) snap(r render.Rect) {
	for i, v := range dropEdges(r) {
		d.edges[i].Snap(v)
	}
}

func (d *SegmentDrop) head(r render.Rect) {
	at := d.owner.bounds
	d.pullInside(dropLeft, dropRight, float64(at.W))
	d.pullInside(dropTop, dropBottom, float64(at.H))
	to := dropEdges(r)
	d.shape(dropLeft, dropRight, to)
	d.shape(dropTop, dropBottom, to)
	for i, v := range to {
		d.edges[i].SetTarget(v)
	}
}

func (d *SegmentDrop) pullInside(near, far int, size float64) {
	lo, hi := d.edges[near].Position()[0], d.edges[far].Position()[0]
	length := hi - lo
	switch {
	case lo >= size:
		lo, hi = max(size-length, 0), size
	case hi <= 0:
		lo, hi = 0, min(length, size)
	default:
		return
	}
	d.edges[near].Snap(lo)
	d.edges[far].Snap(hi)
}

func (d *SegmentDrop) shape(near, far int, to [4]float64) {
	lead, trail := near, far
	if to[near] >= d.edges[near].Position()[0] {
		lead, trail = far, near
	}
	d.edges[lead].SetParams(dropLead)
	d.edges[trail].SetParams(dropTrail)
}

func (d *SegmentDrop) place() {
	at := d.owner.bounds
	x0, x1 := d.span(dropLeft, dropRight)
	y0, y1 := d.span(dropTop, dropBottom)
	r := render.Rect{X: at.X + x0, Y: at.Y + y0, W: x1 - x0, H: y1 - y0}.Intersect(at)
	if r == d.bounds {
		return
	}
	d.bounds = r
	d.owner.Invalidate()
}

func (d *SegmentDrop) span(near, far int) (lo, hi int) {
	a := int(math.Round(d.edges[near].Position()[0]))
	b := int(math.Round(d.edges[far].Position()[0]))
	return min(a, b), max(a, b)
}

func (d *SegmentDrop) paint(cv *render.Canvas) {
	if d.bounds.Empty() {
		return
	}
	v := d.style(d)
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, d.bounds, radii)
	paintBoxBehind(fx.cv, v, d.bounds, radii, borderOf(v), pickc(0, v, style.PropBackgroundColor, 0))
	fx.pop()
}

func dropEdges(r render.Rect) [4]float64 {
	return [4]float64{
		dropLeft:   float64(r.X),
		dropTop:    float64(r.Y),
		dropRight:  float64(r.X + r.W),
		dropBottom: float64(r.Y + r.H),
	}
}
