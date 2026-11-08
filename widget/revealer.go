package widget

import (
	"time"

	"github.com/stubbedev/lys-lib/icons"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// RevealTransition is how a Revealer brings its child in and out.
type RevealTransition uint8

const (
	// RevealNone shows and hides at once; a hidden child takes no space.
	RevealNone RevealTransition = iota
	// RevealFade fades the child's opacity.
	RevealFade
	// RevealSlideUp slides the child in from below, clipped to its slot.
	RevealSlideUp
	// RevealSlideDown slides the child in from above.
	RevealSlideDown
	// RevealSlideLeft slides the child in from the right.
	RevealSlideLeft
	// RevealSlideRight slides the child in from the left.
	RevealSlideRight
	// RevealBounce scales the child up from its center, overshooting,
	// while it fades in.
	RevealBounce
	// RevealZoom scales the child up from its center while it fades in.
	RevealZoom
	// RevealRotate spins the child half a turn while it grows from 30%
	// and fades in.
	RevealRotate
	// RevealFlip widens the child from a vertical line at its center.
	RevealFlip
	// RevealSwingUp swings the child up about its bottom edge.
	RevealSwingUp
	// RevealSwingDown swings the child down about its top edge.
	RevealSwingDown
	// RevealSwingLeft swings the child about its right edge.
	RevealSwingLeft
	// RevealSwingRight swings the child about its left edge.
	RevealSwingRight
	// RevealGenie sucks the child toward the revealer's genie edge.
	RevealGenie
)

// Edge is one side of a rect.
type Edge uint8

// The four edges.
const (
	EdgeBottom Edge = iota
	EdgeTop
	EdgeLeft
	EdgeRight
)

// defaultRevealDuration is the transition length before SetDuration.
const defaultRevealDuration = 200 * time.Millisecond

// Revealer shows one child through an enter/exit transition: a fade, a
// slide clipped to the child's slot, or a transformed composite (zoom,
// bounce, rotate, flip, swing, genie) drawn through an offscreen layer
// with its opacity. Progress runs linearly in time toward the target;
// each transition shapes it with its own curve, the bounce overshooting.
// A transformed composite records the child into its layer once and
// composites that layer again on later frames, the one it lands on
// included, while nothing below the revealer is damaged.
//
// Every transition but RevealNone reserves the child's full size for
// the whole run, hidden included, and moves the child within it, so a
// content-sized surface keeps its size while a card slides in over its
// own slot. RevealNone takes no space while hidden.
//
// The child hit-tests at its slot whenever any of it shows; a transform
// moves pixels, not input.
type Revealer struct {
	node
	child      Widget
	transition RevealTransition
	duration   time.Duration
	edge       Edge
	reveal     bool
	progress   float64
	cancel     anim.Cancel
	onDone     func(revealed bool)
	layer      *render.Layer
	// painted is the logical rect the last frame drew into, so the
	// next one repaints what a transform left behind.
	painted render.Rect
	// collapse sizes a slide's slot by its progress (SetCollapse);
	// full is the child's natural size along the slide at the last
	// Arrange.
	collapse     bool
	full         Size
	surfaceAlpha bool
	drawnOpaque  bool
	fadeFrom     float64
	stamp        layerStamp
}

type layerStamp struct {
	dev                       render.Rect
	list                      *render.List
	style, theme, icons, kept uint64
}

// NewRevealer wraps child hidden, fading over 200ms.
func NewRevealer(child Widget) *Revealer {
	return &Revealer{child: child, transition: RevealFade, duration: defaultRevealDuration}
}

// Child returns the wrapped widget.
func (r *Revealer) Child() Widget { return r.child }

// SetTransition picks the transition the next reveal or hide plays.
func (r *Revealer) SetTransition(t RevealTransition) {
	if r.transition == t {
		return
	}
	r.transition = t
	r.InvalidateLayout()
}

// Transition returns the transition style.
func (r *Revealer) Transition() RevealTransition { return r.transition }

// shownTransition is the transition drawn: under the desktop's
// reduced-motion preference everything that moves becomes a fade.
func (r *Revealer) shownTransition() RevealTransition {
	if r.transition != RevealNone && anim.Reduced() {
		return RevealFade
	}
	return r.transition
}

// SetDuration sets the full transition length; zero snaps.
func (r *Revealer) SetDuration(d time.Duration) { r.duration = max(0, d) }

// SetGenieEdge picks the edge RevealGenie collapses toward.
func (r *Revealer) SetGenieEdge(e Edge) { r.edge = e }

// SetFadeFrom sets the opacity a fade in from hidden starts at, 0 (the
// default) to 1: the child shows at alpha from the reveal's first frame
// and fades to full over the whole duration. A fade out still runs to
// hidden, and the other transitions start from hidden whatever this
// says.
func (r *Revealer) SetFadeFrom(alpha float64) { r.fadeFrom = min(max(alpha, 0), 1) }

// FadeFrom reports SetFadeFrom.
func (r *Revealer) FadeFrom() float64 { return r.fadeFrom }

// SetOnTransitionDone runs fn whenever a transition lands, with the
// state it landed in: the hook for unmapping a surface or dropping a
// card once its exit has played.
func (r *Revealer) SetOnTransitionDone(fn func(revealed bool)) { r.onDone = fn }

// Revealed reports the target state: true once SetRevealed(true) asked
// for the child, whether or not the enter has finished.
func (r *Revealer) Revealed() bool { return r.reveal }

// Progress is how far the child is shown, 0 hidden to 1 shown, before
// the transition's curve.
func (r *Revealer) Progress() float64 { return r.progress }

// SetRevealed starts the transition toward shown or hidden. A reversal
// mid-flight starts from where the child is and runs the full
// duration. Asking for the current target changes nothing.
func (r *Revealer) SetRevealed(reveal bool) {
	if r.reveal == reveal {
		return
	}
	r.reveal = reveal
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	target := 0.0
	if reveal {
		target = 1
	}
	if r.shownTransition() == RevealNone {
		r.InvalidateLayout()
	}
	if r.duration == 0 {
		r.land(target)
		return
	}
	from := r.progress
	if reveal && from == 0 && r.shownTransition() == RevealFade {
		from = r.fadeFrom
		r.setProgress(from)
	}
	r.cancel = anim.Play(anim.Animate(r.duration, func(t float64) {
		if t >= 1 {
			r.cancel = nil
			r.land(target)
			return
		}
		r.setProgress(from + float64((target-from)*t))
	}).Easing(anim.Linear))
}

// SetCollapse makes a slide take space as it shows, the way a GTK
// revealer does: along the slide's axis the revealer measures the
// child's natural size scaled by the eased progress, nothing while
// hidden, and the child keeps its full size, its leading edge riding
// the slot's growing edge, clipped to the slot. Content below a
// collapsing revealer moves with it. Only the four slides collapse;
// the other transitions reserve the child's full size whatever this
// says.
func (r *Revealer) SetCollapse(collapse bool) {
	if r.collapse != collapse {
		r.collapse = collapse
		r.InvalidateLayout()
	}
}

// Collapse reports SetCollapse.
func (r *Revealer) Collapse() bool { return r.collapse }

// collapses reports whether the slot follows the progress.
func (r *Revealer) collapses() bool {
	switch r.shownTransition() {
	case RevealSlideUp, RevealSlideDown, RevealSlideLeft, RevealSlideRight:
		return r.collapse
	}
	return false
}

// vertical reports a slide along y.
func (r *Revealer) vertical() bool {
	return r.shownTransition() == RevealSlideUp || r.shownTransition() == RevealSlideDown
}

// Finish lands a running transition at once, reporting it as its end
// would; with none running it does nothing. It is the escape for a
// surface that must go now (a shutdown, a test without a clock).
func (r *Revealer) Finish() {
	if r.cancel == nil {
		return
	}
	r.cancel()
	r.cancel = nil
	target := 0.0
	if r.reveal {
		target = 1
	}
	r.land(target)
}

// land settles the progress at a transition's end and reports it.
func (r *Revealer) land(target float64) {
	r.setProgress(target)
	if r.onDone != nil {
		r.onDone(target == 1)
	}
}

func (r *Revealer) setProgress(p float64) {
	if r.progress == p {
		return
	}
	r.progress = p
	if r.fadesSurface(p) && r.drawnOpaque {
		return
	}
	if r.collapses() {
		r.InvalidateLayout()
	}
	r.Invalidate()
	if !r.painted.Empty() {
		r.InvalidateRect(r.painted)
	}
	if ink := r.ink(); !ink.Empty() {
		r.InvalidateRect(ink)
	}
}

func (r *Revealer) fadesSurface(alpha float64) bool {
	if r.shownTransition() != RevealFade {
		return false
	}
	r.surfaceAlpha = fadeSurface(r, alpha)
	return r.surfaceAlpha
}

// Measure is the child's size, nothing for a hidden RevealNone, and
// for a collapsing slide the child's natural extent along the slide
// scaled by the eased progress.
func (r *Revealer) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := r.measureHit(axis, forSize); ok {
		return sz
	}
	if r.child == nil || (r.shownTransition() == RevealNone && !r.reveal) {
		return r.measureStore(axis, forSize, Sizes{})
	}
	if !r.collapses() {
		return r.measureStore(axis, forSize, measureChild(r, r.child, axis, forSize))
	}
	if axis != r.slideAxis() {
		// The child lays out at its full extent along the slide, not
		// at the slot's.
		return r.measureStore(axis, forSize, measureChild(r, r.child, axis, Unknown))
	}
	nat := measureChild(r, r.child, axis, forSize).Nat
	t := anim.EaseInOutCubic(r.progress)
	return r.measureStore(axis, forSize, FixedSizes(int(roundHalfUp(float64(float64(nat)*t)))))
}

// slideAxis is the axis a slide runs along.
func (r *Revealer) slideAxis() Axis {
	if r.vertical() {
		return Column
	}
	return Row
}

// Arrange gives the child the whole rect; a collapsing slide's child
// keeps its natural extent along the slide, its leading edge on the
// slot's growing edge.
func (r *Revealer) Arrange(rect render.Rect) {
	r.node.Arrange(rect)
	if r.child == nil {
		return
	}
	setParents(r, r.child)
	if r.collapses() {
		r.full = naturalChild(r, r.child)
		if r.vertical() {
			r.full.H = measureChild(r, r.child, Column, rect.W).Nat
		}
		rect = r.childRect(rect)
	}
	arrangeChild(r.child, rect)
}

// childRect places a collapsing slide's child in slot: entering from
// above (slide down) its bottom shows first, from below its top, and
// likewise across.
func (r *Revealer) childRect(slot render.Rect) render.Rect {
	c := slot
	switch r.shownTransition() {
	case RevealSlideDown:
		c.H = max(r.full.H, slot.H)
		c.Y = slot.Y + slot.H - c.H
	case RevealSlideUp:
		c.H = max(r.full.H, slot.H)
	case RevealSlideRight:
		c.W = max(r.full.W, slot.W)
		c.X = slot.X + slot.W - c.W
	case RevealSlideLeft:
		c.W = max(r.full.W, slot.W)
	}
	return c
}

// shows reports whether any of the child is drawn.
func (r *Revealer) shows() bool {
	if r.shownTransition() == RevealNone {
		return r.reveal
	}
	return r.progress > 0
}

// Paint draws the child through the transition at its progress.
func (r *Revealer) Paint(cv *render.Canvas) {
	r.painted = render.Rect{}
	r.drawnOpaque = false
	if r.surfaceAlpha && r.shownTransition() != RevealFade {
		fadeSurface(r, 1)
		r.surfaceAlpha = false
	}
	// Only a transformed frame keeps the layer, a full-canvas buffer:
	// at rest, hidden or fading, the child paints directly, but for the
	// frame a transform lands on, which composites the layer once more.
	layer := r.layer
	r.layer = nil
	defer func() {
		if layer != nil {
			keepOrFree(layer, &r.layer)
		}
	}()
	if r.child == nil {
		return
	}
	if !r.shows() {
		if r.surfaceAlpha || r.fadesSurface(0) {
			PaintChild(cv, r.child)
			r.painted = r.bounds
			r.drawnOpaque = true
		}
		return
	}
	p := r.progress
	switch {
	case r.shownTransition() == RevealNone || p >= 1 || r.surfaceAlpha:
		if p >= 1 && r.landLayer(cv, layer) {
			layer = nil
		} else {
			PaintChild(cv, r.child)
		}
		r.painted = r.bounds
		r.drawnOpaque = true
		return
	case r.shownTransition() == RevealFade:
		prev := cv.PushAlpha(p)
		PaintChild(cv, r.child)
		cv.PopAlpha(prev)
		r.painted = r.bounds
		return
	case r.collapses():
		// The arrangement already slid the child: clip it to the slot.
		prev := cv.PushClip(r.bounds)
		PaintChild(cv, r.child)
		cv.PopClip(prev)
		r.painted = r.bounds
		return
	}
	dev := cv.MapRect(r.bounds)
	m, alpha, clip := r.transform(dev, p)
	if r.reuses(cv, layer, dev) {
		r.layer = layer
	} else {
		r.subDamaged, r.stamp = false, r.stampOf(cv, dev)
		r.layer = cv.Layer(layer, dev)
		r.child.Paint(r.layer.Canvas())
	}
	if clip {
		prev := cv.PushClipDevice(dev)
		cv.Composite(r.layer, dev, m, alpha)
		cv.PopClip(prev)
		r.painted = r.bounds
		return
	}
	cv.Composite(r.layer, dev, m, alpha)
	r.painted = r.logical(cv, m.MapBounds(dev))
}

// logical maps a device rect back to logical pixels, rounded outward.
func (r *Revealer) logical(cv *render.Canvas, d render.Rect) render.Rect {
	num, denom := cv.DeviceScale()
	return render.MapRect(d, denom, num)
}

// ink is the logical rect the current progress draws into, for damage.
func (r *Revealer) ink() render.Rect {
	if r.bounds.Empty() || !r.shows() {
		return render.Rect{}
	}
	switch r.shownTransition() {
	case RevealNone, RevealFade, RevealSlideUp, RevealSlideDown, RevealSlideLeft, RevealSlideRight:
		return r.bounds
	}
	m, _, _ := r.transform(r.bounds, r.progress)
	return m.MapBounds(r.bounds)
}

// transform is the map, opacity and slot clip that draw the child of
// rect b at progress p (the WayleRevealer snapshot: slides translate
// within a clip, the rest transform about a pivot while fading).
func (r *Revealer) transform(b render.Rect, p float64) (m render.Affine, alpha float64, clip bool) {
	x, y, w, h := float64(b.X), float64(b.Y), float64(b.W), float64(b.H)
	cx, cy := x+float64(w/2), y+float64(h/2)
	t := anim.EaseInOutCubic(p)
	switch r.shownTransition() {
	case RevealSlideUp, RevealSlideDown, RevealSlideLeft, RevealSlideRight:
		d := 1 - t
		var dx, dy float64
		switch r.shownTransition() {
		case RevealSlideUp:
			dy = d * h
		case RevealSlideDown:
			dy = -d * h
		case RevealSlideLeft:
			dx = d * w
		default:
			dx = -d * w
		}
		// Whole pixels keep the sliding content crisp.
		return render.Translate(roundHalfUp(dx), roundHalfUp(dy)), 1, true
	case RevealBounce:
		s := easeOutBackUnclamped(p)
		return render.Scale(s, s).About(cx, cy), p, false
	case RevealZoom:
		return render.Scale(t, t).About(cx, cy), p, false
	case RevealRotate:
		s := 0.3 + float64(0.7*t)
		return render.Rotate((1-t)*180).Mul(render.Scale(s, s)).About(cx, cy), p, false
	case RevealFlip:
		return render.Scale(max(t, 0.001), 1).About(cx, cy), p, false
	case RevealSwingUp, RevealSwingDown, RevealSwingLeft, RevealSwingRight:
		angle := (1 - t) * 90
		switch r.shownTransition() {
		case RevealSwingUp:
			return render.Rotate(-angle).About(cx, y+h), p, false
		case RevealSwingDown:
			return render.Rotate(angle).About(cx, y), p, false
		case RevealSwingLeft:
			return render.Rotate(angle).About(x+w, cy), p, false
		default:
			return render.Rotate(-angle).About(x, cy), p, false
		}
	case RevealGenie:
		narrow := 0.15 + float64(0.85*t)
		switch r.edge {
		case EdgeTop:
			return render.Scale(max(narrow, 0.001), max(t, 0.001)).About(cx, y), p, false
		case EdgeLeft:
			return render.Scale(max(t, 0.001), max(narrow, 0.001)).About(x, cy), p, false
		case EdgeRight:
			return render.Scale(max(t, 0.001), max(narrow, 0.001)).About(x+w, cy), p, false
		default:
			return render.Scale(max(narrow, 0.001), max(t, 0.001)).About(cx, y+h), p, false
		}
	}
	return render.Identity, p, false
}

// easeOutBackUnclamped is the back-ease overshoot without anim's
// endpoint clamp, which only matters at the exact ends.
func easeOutBackUnclamped(t float64) float64 {
	const c1 = 1.70158
	const c3 = c1 + 1
	u := t - 1
	return 1 + float64(c3*u*u*u) + float64(c1*u*u)
}

func roundHalfUp(v float64) float64 {
	if v < 0 {
		return -roundHalfUp(-v)
	}
	return float64(int(v + 0.5))
}

// HitTest picks inside the child while any of it shows.
func (r *Revealer) HitTest(p Point) Widget {
	if r.child == nil || !r.shows() {
		return nil
	}
	if r.collapses() && !r.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := HitChild(r.child, p); hit != nil {
		return hit
	}
	return r.HitLeaf(r, p)
}

// Children exposes the child for damage, focus and accessibility
// while any of it shows or is being revealed, so keyboard traversal
// and the accessibility tree skip a hidden revealer's content.
func (r *Revealer) Children() []Widget {
	if !r.showing() {
		return nil
	}
	return []Widget{r.child}
}

// appendChildren appends the shown child, matching Children.
func (r *Revealer) appendChildren(buf []Widget) []Widget {
	if r.showing() {
		return append(buf, r.child)
	}
	return buf
}

// appendStyleKids is the child, shown or not (styleKids): a hidden
// child keeps its cascade, so a reveal starts from its styled look.
func (r *Revealer) appendStyleKids(buf []Widget) []Widget {
	if r.child != nil {
		return append(buf, r.child)
	}
	return buf
}

// showing reports a child that is shown, or on its way in or out.
func (r *Revealer) showing() bool { return r.child != nil && (r.reveal || r.progress > 0) }

// stampOf is what a layer recorded for dev on cv depends on.
func (r *Revealer) stampOf(cv *render.Canvas, dev render.Rect) layerStamp {
	return layerStamp{dev: dev, list: cv.Recording(), style: styleGen, theme: themeGen, icons: icons.Default().Generation(), kept: keptLayerGen.Load()}
}

// reuses reports whether layer still holds the child as cv would paint
// it at dev, and readies it to composite again.
func (r *Revealer) reuses(cv *render.Canvas, layer *render.Layer, dev render.Rect) bool {
	return layer != nil && !r.subDamaged && r.stamp == r.stampOf(cv, dev) && cv.ReuseLayer(layer, dev)
}

// landLayer draws the frame a transform lands on from the layer its last
// frame kept, untransformed, rather than painting the child again, and
// retires the layer once the List compositing it has ended. It reports
// false, drawing nothing, when the layer no longer holds the child.
func (r *Revealer) landLayer(cv *render.Canvas, layer *render.Layer) bool {
	dev := cv.MapRect(r.bounds)
	if !r.reuses(cv, layer, dev) {
		return false
	}
	cv.Composite(layer, dev, render.Identity, 1)
	render.Retire(layer.Free)
	return true
}
