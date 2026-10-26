package widget

import "github.com/stubbedev/lys-lib/render"

// DrawingArea is a widget the application paints itself (GTK
// DrawingArea): OnDraw receives the canvas, clipped to the area, on
// every paint; QueueDraw asks for one. OnStylus receives tablet samples
// over the area - pressure and tilt for freehand drawing - while the
// pen also drives the pointer like any tool.
type DrawingArea struct {
	node
	w, h int

	// OnDraw paints the area into its bounds.
	OnDraw func(cv *render.Canvas, area render.Rect)
	// OnStylus receives every tablet sample over the area.
	OnStylus func(s Stylus)
}

// NewDrawingArea returns an area requesting w x h logical pixels.
func NewDrawingArea(w, h int) *DrawingArea {
	d := &DrawingArea{w: w, h: h}
	d.SetElement("drawingarea")
	return d
}

// SetContentSize changes the requested size.
func (d *DrawingArea) SetContentSize(w, h int) {
	d.w, d.h = w, h
	d.InvalidateLayout()
}

// QueueDraw repaints the area.
func (d *DrawingArea) QueueDraw() { d.Invalidate() }

// Measure requests the content size.
func (d *DrawingArea) Measure(con Constraints) Size {
	return clampSize(Size{W: d.w, H: d.h}, con)
}

// Paint runs OnDraw clipped to the area.
func (d *DrawingArea) Paint(cv *render.Canvas) {
	if d.OnDraw == nil {
		return
	}
	prev := cv.PushClip(d.bounds)
	d.OnDraw(cv, d.bounds)
	cv.PopClip(prev)
}

// HitTest makes the whole area the target.
func (d *DrawingArea) HitTest(p Point) Widget { return d.HitLeaf(d, p) }

// Stylus implements StylusHandler.
func (d *DrawingArea) Stylus(s Stylus) {
	if d.OnStylus != nil {
		d.OnStylus(s)
	}
}
