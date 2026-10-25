package widget

import (
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// ScrollRange is one axis of a scroll position (GTK Adjustment's
// numbers): Offset into a Total extent, of which Page is visible. The
// bar geometry and the drag and page arithmetic live here once, for
// Scroll's own bars and the standalone Scrollbar alike.
type ScrollRange struct{ Offset, Page, Total int }

// Max is the largest offset.
func (r ScrollRange) Max() int { return max(0, r.Total-r.Page) }

// Overflows reports whether there is anything to scroll.
func (r ScrollRange) Overflows() bool { return r.Max() > 0 && r.Page > 0 && r.Total > 0 }

// thumb is the handle's start and length along a track of n pixels:
// proportional to the visible page, never shorter than 24 px.
func (r ScrollRange) thumb(n int) (pos, length int) {
	length = min(n, max(24, n*r.Page/max(1, r.Total)))
	return (n - length) * r.Offset / max(1, r.Max()), length
}

// dragged is the offset after the thumb moved delta pixels from where
// it sat at offset start: pointer motion maps 1:1 onto the thumb.
func (r ScrollRange) dragged(start, delta, n int) int {
	_, length := r.thumb(n)
	return start + delta*r.Max()/max(1, n-length)
}

// paged is the offset after a track click at pixel at: one page toward
// the click, unchanged on the thumb itself.
func (r ScrollRange) paged(at, n int) int {
	pos, length := r.thumb(n)
	switch {
	case at < pos:
		return r.Offset - r.Page
	case at >= pos+length:
		return r.Offset + r.Page
	}
	return r.Offset
}

// thumbRect places the thumb inside track along axis.
func (r ScrollRange) thumbRect(track render.Rect, axis Axis) render.Rect {
	if axis == Row {
		pos, length := r.thumb(track.W)
		return render.Rect{X: track.X + pos, Y: track.Y, W: length, H: track.H}
	}
	pos, length := r.thumb(track.H)
	return render.Rect{X: track.X, Y: track.Y + pos, W: track.W, H: length}
}

// along is p's coordinate and track's length along axis.
func along(p Point, track render.Rect, axis Axis) (at, n int) {
	if axis == Row {
		return p.X - track.X, track.W
	}
	return p.Y - track.Y, track.H
}

// barSpec is the computed look of one bar: the scrollbar-color pair
// and the scrollbar-width thickness. The zero value is the built-in
// look at full thickness.
type barSpec struct {
	thumb, track render.Color
	width        uint8
}

// barSpecOf reads the scrollbar-color/width pair off one computed
// style — the Scroll's own for its bars, the standalone Scrollbar's
// (inherited) for itself. auto keeps the built-in look: a
// border-colored thumb on a transparent track.
func barSpecOf(v *style.Values) barSpec {
	spec := barSpec{thumb: Current().Border}
	if v.Has(style.PropScrollbarColor) && !v.BarColorAuto {
		spec.thumb, spec.track = v.BarThumb, v.BarTrack
	}
	if v.Has(style.PropScrollbarWidth) {
		spec.width = v.BarWidth
	}
	return spec
}

// thickness is the painted thickness of the bar's parts.
func (b barSpec) thickness() int {
	if b.width == style.BarWidthThin {
		return 4
	}
	return gutter
}

// paintScrollbar draws a track and its thumb at alpha (0..1): the one
// look Scroll's fading bars and the standalone Scrollbar share, under
// the scrollbar-color pair and scrollbar-width thickness. none draws
// nothing.
func paintScrollbar(cv *render.Canvas, track, thumb render.Rect, alpha float64, spec barSpec) {
	if spec.width == style.BarWidthNone || alpha <= 0 {
		return
	}
	if t := spec.thickness(); t < min(track.W, track.H) {
		// thin: centered in the gutter across the bar's axis.
		if track.W <= track.H {
			thumb.X += (track.W - t) / 2
			thumb.W = t
		} else {
			thumb.Y += (track.H - t) / 2
			thumb.H = t
		}
	}
	if spec.track.A() > 0 {
		cv.RoundedRect(track, min(track.W, track.H)/2, scaleAlpha(spec.track, alpha/3))
	}
	cv.RoundedRect(thumb, min(thumb.W, thumb.H)/2, scaleAlpha(spec.thumb, alpha))
}

// Adjustable is a scroll position a Scrollbar shows and drives - GTK's
// Adjustment role, implemented by Scroll.
type Adjustable interface {
	// AxisRange reports the position along axis (Row: horizontal).
	AxisRange(axis Axis) ScrollRange
	// SetAxisOffset scrolls along axis, clamped.
	SetAxisOffset(axis Axis, offset int)
	// WatchRange registers fn to run whenever a range changed - the
	// offset, the page, or the total.
	WatchRange(fn func())
}

// Scrollbar is a standalone, always-visible bar bound to an
// Adjustable (GTK Scrollbar): the thumb tracks the target's range,
// dragging it scrolls 1:1, and a track click pages. It draws nothing
// while the target does not overflow. Place it beside the scrolled
// widget; the target's own auto-hiding bars are independent of it.
type Scrollbar struct {
	node
	target   Adjustable
	axis     Axis
	drag     dragGesture
	dragFrom int
	moved    bool
}

// NewScrollbar returns a bar driving target along axis (Row for a
// horizontal bar, Column for a vertical one).
func NewScrollbar(target Adjustable, axis Axis) *Scrollbar {
	b := &Scrollbar{target: target, axis: axis}
	b.SetElement("scrollbar")
	target.WatchRange(b.Invalidate)
	return b
}

// Axis reports the bar's orientation.
func (b *Scrollbar) Axis() Axis { return b.axis }

// Measure is the bar's thickness across its axis; along it the bar
// takes what its parent stretches it to. scrollbar-width: none
// collapses it, thin keeps the slot and paints slimmer.
func (b *Scrollbar) Measure(con Constraints) Size {
	sz := Size{W: gutter}
	if b.axis == Row {
		sz = Size{H: gutter}
	}
	if barSpecOf(b.style(b)).width == style.BarWidthNone {
		sz = Size{}
	}
	return clampSize(sz, con)
}

// Paint draws the track and thumb while the target overflows.
func (b *Scrollbar) Paint(cv *render.Canvas) {
	if r := b.target.AxisRange(b.axis); r.Overflows() {
		paintScrollbar(cv, b.bounds, r.thumbRect(b.bounds, b.axis), 1, barSpecOf(b.style(b)))
	}
}

// PressAt grabs the thumb under p.
func (b *Scrollbar) PressAt(p Point) {
	b.moved = false
	r := b.target.AxisRange(b.axis)
	if r.Overflows() && r.thumbRect(b.bounds, b.axis).Contains(p.X, p.Y) {
		b.drag.begin(p)
		b.dragFrom = r.Offset
	}
}

// DragMove scrolls the grabbed thumb with the pointer.
func (b *Scrollbar) DragMove(p Point) {
	dx, dy, ok := b.drag.travel(p)
	if !ok {
		return
	}
	b.moved = true
	delta := dy
	if b.axis == Row {
		delta = dx
	}
	_, n := along(p, b.bounds, b.axis)
	b.target.SetAxisOffset(b.axis, b.target.AxisRange(b.axis).dragged(b.dragFrom, delta, n))
}

// PressEnd releases the thumb.
func (b *Scrollbar) PressEnd() { b.drag.end() }

// ClickAt pages toward a track click; the release ending a thumb drag
// is not a click.
func (b *Scrollbar) ClickAt(p Point) {
	if b.moved {
		return
	}
	r := b.target.AxisRange(b.axis)
	if !r.Overflows() {
		return
	}
	at, n := along(p, b.bounds, b.axis)
	b.target.SetAxisOffset(b.axis, r.paged(at, n))
}

// Role implements Roleer.
func (b *Scrollbar) Role() Role { return RoleScrollBar }

// HitTest implements Widget: the whole strip is the bar.
func (b *Scrollbar) HitTest(p Point) Widget { return b.HitLeaf(b, p) }
