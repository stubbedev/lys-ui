package widget

import (
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

const (
	// progressRingSize is the ring's diameter when min-width and
	// min-height leave it unset.
	progressRingSize = 32
	// progressRingStroke is the stroke when the trough's min-height
	// leaves it unset.
	progressRingStroke = 4
)

// ProgressRing is a circular determinate progress: the fraction sweeps
// clockwise from twelve o'clock over a full-circle track, with an
// optional child centered inside, a percentage or an icon. Its node
// tree is ProgressBar's, `progressring > trough > progress`: the ring's
// min-width and min-height set the diameter, the trough's min-height
// the stroke, and the trough's and progress's background colors the
// track and the fill, the fill the accent (or the accent-color) and the
// track the fill at a fifth of its opacity unless set. The success,
// warning and error classes recolor it. A new value eases in over the
// theme's medium motion.
type ProgressRing struct {
	node
	value, shown float64
	cancel       anim.Cancel
	trough       meterTrough
	child        Widget
}

// NewProgressRing returns a ring at value clamped to [0, 1].
func NewProgressRing(value float64) *ProgressRing {
	r := &ProgressRing{value: math01(value), shown: math01(value)}
	r.SetElement("progressring")
	r.trough.SetElement("trough")
	r.trough.fill.SetElement("progress")
	setParents(r, &r.trough)
	setParents(&r.trough, &r.trough.fill)
	return r
}

func (r *ProgressRing) appendStyleKids(buf []Widget) []Widget { return append(buf, &r.trough) }

// Value returns the fraction in [0, 1].
func (r *ProgressRing) Value() float64 { return r.value }

// SetValue clamps v to [0, 1] and eases the sweep to it.
func (r *ProgressRing) SetValue(v float64) {
	v = math01(v)
	if v == r.value {
		return
	}
	r.value = v
	if r.cancel != nil {
		r.cancel()
	}
	from := r.shown
	_, medium, _ := Current().MotionDurations()
	r.cancel = anim.Start(medium, func(t float64) {
		r.shown = from + (v-from)*t
		r.Invalidate()
	})
}

// SetChild centers w inside the ring; nil removes it.
func (r *ProgressRing) SetChild(w Widget) {
	if r.child == w {
		return
	}
	if r.child != nil {
		notifyRemoved(r.child)
		clearParents(r.child)
	}
	r.child = w
	r.InvalidateLayout()
}

// Child returns the centered widget, nil when none.
func (r *ProgressRing) Child() Widget { return r.child }

// Children returns the centered widget.
func (r *ProgressRing) Children() []Widget { return nonNil(r.child) }

// diameter is the ring's side: the larger of min-width and min-height,
// progressRingSize when neither is set.
func (r *ProgressRing) diameter() int {
	v := r.style(r)
	if !v.Has(style.PropMinWidth) && !v.Has(style.PropMinHeight) {
		return progressRingSize
	}
	return max(picki(v, style.PropMinWidth, 0), picki(v, style.PropMinHeight, 0))
}

// Measure wants the diameter square inside the ring's CSS box.
func (r *ProgressRing) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := r.measureHit(axis, forSize); ok {
		return sz
	}
	d := r.diameter()
	v := r.style(r)
	return r.measureStore(axis, forSize, measureBox(v, boxOf(v, render.Insets{}), axis, forSize, func(axis Axis, forSize int) Sizes {
		return FixedSizes(d)
	}))
}

// Arrange centers the ring's square in the content box and the child
// at its natural size in the square.
func (r *ProgressRing) Arrange(rect render.Rect) {
	border, inner := boxRects(boxOf(r.style(r), render.Insets{}), rect)
	r.node.Arrange(border)
	side := min(inner.W, inner.H)
	sq := alignRect(inner, Size{W: side, H: side}, AlignCenter, AlignCenter)
	r.trough.Arrange(sq)
	r.trough.fill.Arrange(sq)
	if r.child == nil {
		return
	}
	arrangeChild(r.child, alignRect(sq, childWithin(r, r.child, Size{W: sq.W, H: sq.H}), AlignCenter, AlignCenter))
	setParents(r, r.child)
}

// Paint strokes the track and the sweep, then the child.
func (r *ProgressRing) Paint(cv *render.Canvas) {
	sq := r.trough.bounds
	tv := r.trough.style(&r.trough)
	fv := r.trough.fill.style(&r.trough.fill)
	stroke := float64(picki(tv, style.PropMinHeight, progressRingStroke))
	radius := float64(sq.W)/2 - stroke/2
	if radius > 0 {
		fill := pickc(0, fv, style.PropBackgroundColor, accentOf(r.style(r), Current().Accent))
		track := pickc(0, tv, style.PropBackgroundColor, progressTrack(fill))
		cx, cy := float64(sq.X)+float64(sq.W)/2, float64(sq.Y)+float64(sq.H)/2
		cv.Arc(cx, cy, radius, stroke, 0, 2*math.Pi, track)
		cv.Arc(cx, cy, radius, stroke, -math.Pi/2, 2*math.Pi*r.shown, fill)
	}
	if r.child != nil && IsVisible(r.child) {
		PaintChild(cv, r.child)
	}
}

// Role implements Roleer.
func (r *ProgressRing) Role() Role { return RoleProgressBar }

// HitTest returns the child under p, else the ring inside its bounds.
func (r *ProgressRing) HitTest(p Point) Widget {
	if r.child != nil && IsVisible(r.child) {
		if hit := HitChild(r.child, p); hit != nil {
			return hit
		}
	}
	return r.HitLeaf(r, p)
}
