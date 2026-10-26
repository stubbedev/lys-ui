package widget

import (
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// departurePrecision settles a departure's 0-to-1 progress.
const departurePrecision = 1e-3

// departure is a removed child painting its way out: collapsing along
// its box's axis and fading, at the rect it was last shown at, on its
// layout motion's spring. It takes no input and no layout space; the
// box keeps it parented, so it paints with its own cascade, until the
// spring lands or the widget is linked somewhere else.
type departure struct {
	w     Widget
	from  render.Rect
	left  float64 // 1 at removal, 0 when gone
	layer *render.Layer
	ended bool
}

// depart starts w's way out when it has a layout motion and was
// shown, reporting false (remove it at once) otherwise. The siblings
// spring into the space through their own motions.
func (b *Box) depart(w Widget) bool {
	n := nodeOf(w)
	if n == nil || n.shownWalk == 0 || parentOf(w) != Widget(b) {
		return false
	}
	m := n.effLayoutMotion(w)
	from := ShownRect(w)
	if m == LayoutMotionNone || from.Empty() {
		return false
	}
	if n.motion != nil {
		n.motion.spring.Stop()
		n.motion = nil
	}
	d := &departure{w: w, from: from, left: 1}
	b.departing = append(b.departing, d)
	n.departingFrom = b
	var spring *anim.Spring
	spring = anim.NewSpring(motionSpring(m), []float64{1}, func(pos []float64) {
		b.InvalidateRect(d.from)
		d.left = min(1, max(0, pos[0]))
		if pos[0] == 0 && spring.Velocity()[0] == 0 {
			b.endDeparture(d)
		}
	})
	spring.SetPrecision(departurePrecision)
	spring.SetTarget(0)
	return true
}

// endDeparture drops a departure and releases the widget.
func (b *Box) endDeparture(d *departure) {
	if d.ended {
		return
	}
	d.ended = true
	b.departing = slices.DeleteFunc(b.departing, func(o *departure) bool { return o == d })
	if n := nodeOf(d.w); n != nil {
		n.departingFrom = nil
	}
	clearParents(d.w)
}

// endDepartureOf ends the departure of the widget behind n: it is
// being linked somewhere else, so it stops painting here and links
// there fresh.
func (b *Box) endDepartureOf(n *node) {
	for _, d := range b.departing {
		if nodeOf(d.w) == n {
			b.endDeparture(d)
			return
		}
	}
}

// endDepartures drops every departure at once (a wholesale Clear).
func (b *Box) endDepartures() {
	for len(b.departing) > 0 {
		b.endDeparture(b.departing[0])
	}
}

// paintDepartures paints the departing children over the others:
// each laid out where it was, composited clipped to its shrinking
// rect at its fading opacity.
func (b *Box) paintDepartures(cv *render.Canvas) {
	for _, d := range b.departing {
		if d.left <= 0 {
			continue
		}
		bw, ok := d.w.(Boundser)
		if !ok {
			continue
		}
		t := bw.Bounds()
		dev := cv.MapRect(t)
		if dev.Empty() {
			continue
		}
		shown := d.from
		if b.axis == Row {
			shown.W = int(float64(shown.W)*d.left + 0.5)
		} else {
			shown.H = int(float64(shown.H)*d.left + 0.5)
		}
		d.layer = cv.Layer(d.layer, dev)
		d.w.Paint(d.layer.Canvas())
		num, denom := cv.DeviceScale()
		f := float64(num) / float64(denom)
		prev := cv.PushClip(shown)
		cv.Composite(d.layer, dev, render.Translate(float64(d.from.X-t.X)*f, float64(d.from.Y-t.Y)*f), d.left)
		cv.PopClip(prev)
	}
}
