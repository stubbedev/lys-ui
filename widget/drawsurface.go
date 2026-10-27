package widget

import (
	"github.com/stubbedev/lys-lib/render"
)

// DrawSurface is DrawingArea's retained counterpart (relm4's
// DrawHandler): a backing canvas whose pixels persist between paints,
// so an incremental application - a paint program, a plot that appends
// samples - draws only what changed and never repaints the rest. The
// application draws through Draw any time on the loop; Paint shows the
// backing canvas as it stands. OnStylus receives tablet samples over
// the area, like DrawingArea's.
//
// A resize at the same device scale keeps the content and leaves the
// newly exposed area transparent; a rescale (or a first paint before
// any Draw, which allocates at 1x) reallocates the buffer, drops the
// pixels, and hands the fresh canvas to OnResize - the one place an
// application redraws everything it owes.
type DrawSurface struct {
	node
	w, h int

	buf []byte
	cv  *render.Canvas
	// OnResize redraws the whole surface into the fresh backing
	// canvas; it runs on the loop, from the paint that reallocated.
	OnResize func(cv *render.Canvas)
	// OnStylus receives every tablet sample over the surface.
	OnStylus func(s Stylus)
}

// NewDrawSurface returns a surface requesting w x h logical pixels.
func NewDrawSurface(w, h int) *DrawSurface {
	d := &DrawSurface{w: w, h: h}
	d.SetElement("drawsurface")
	return d
}

// SetContentSize changes the requested size.
func (d *DrawSurface) SetContentSize(w, h int) {
	d.w, d.h = w, h
	d.InvalidateLayout()
}

// Canvas returns the backing canvas, or nil before the surface first
// drew or painted. It carries the device scale the pixels were laid
// down at; draw through Draw, which also schedules the repaint.
func (d *DrawSurface) Canvas() *render.Canvas { return d.cv }

// Draw runs fn over the backing canvas with drawing clipped to r, a
// rect in the surface's own logical pixels, and schedules a repaint of
// exactly that rect. fn must draw only inside r: the clip enforces it,
// so a stray primitive cannot land outside the damage. It runs on the
// loop, like every widget mutation.
func (d *DrawSurface) Draw(r render.Rect, fn func(cv *render.Canvas)) {
	checkLoop("DrawSurface.Draw")
	if fn == nil || d.bounds.Empty() {
		return
	}
	num, denom := d.scale()
	dev := render.MapRect(render.Rect{W: d.bounds.W, H: d.bounds.H}, num, denom)
	d.ensure(dev.W, dev.H, num, denom)
	prev := d.cv.PushClip(r)
	fn(d.cv)
	d.cv.PopClip(prev)
	d.InvalidateRect(render.Rect{
		X: d.bounds.X + r.X, Y: d.bounds.Y + r.Y, W: r.W, H: r.H,
	}.Intersect(d.bounds))
}

// scale is the device scale the backing pixels are laid down at: the
// one the last paint saw, or 1x before the first paint.
func (d *DrawSurface) scale() (num, denom int) {
	if d.cv == nil {
		return 1, 1
	}
	return d.cv.DeviceScale()
}

// ensure brings the backing canvas to w x h device pixels at the given
// scale, reusing the buffer when it already fits. A size change at the
// same scale keeps the overlapping pixels and clears the rest; any
// other change reallocates from transparent and hands the canvas to
// OnResize.
func (d *DrawSurface) ensure(w, h, num, denom int) {
	if d.cv != nil {
		r := d.cv.Rect()
		on, od := d.cv.DeviceScale()
		if on == num && od == denom {
			if r.W == w && r.H == h {
				return
			}
			stride, old := render.Stride(w), render.Stride(r.W)
			cw, ch := min(r.W, w), min(r.H, h)
			buf := make([]byte, stride*h)
			for y := range ch {
				copy(buf[y*stride:y*stride+cw*4], d.buf[y*old:y*old+cw*4])
			}
			d.buf = buf
			d.cv = render.NewScaled(buf, stride, w, h, num, denom)
			return
		}
	}
	d.buf = make([]byte, render.Stride(w)*h)
	d.cv = render.NewScaled(d.buf, render.Stride(w), w, h, num, denom)
	if d.OnResize != nil {
		d.OnResize(d.cv)
	}
}

// QueueDraw repaints the surface.
func (d *DrawSurface) QueueDraw() { d.Invalidate() }

// Measure requests the content size.
func (d *DrawSurface) Measure(axis Axis, forSize int) Sizes {
	return FixedSizes(Size{W: d.w, H: d.h}.Along(axis))
}

// Paint shows the backing canvas, allocating it first if this paint is
// the surface's first look at its size or scale.
func (d *DrawSurface) Paint(cv *render.Canvas) {
	if d.bounds.Empty() {
		return
	}
	num, denom := cv.DeviceScale()
	r := render.MapRect(render.Rect{W: d.bounds.W, H: d.bounds.H}, num, denom)
	d.ensure(r.W, r.H, num, denom)
	at := cv.MapRect(d.bounds)
	cv.DrawCanvasDevice(d.cv, at.X, at.Y)
}

// HitTest makes the whole surface the target.
func (d *DrawSurface) HitTest(p Point) Widget { return d.HitLeaf(d, p) }

// Stylus implements StylusHandler.
func (d *DrawSurface) Stylus(s Stylus) {
	if d.OnStylus != nil {
		d.OnStylus(s)
	}
}
