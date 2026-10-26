package widget

// The rendering-effect paint pipeline (#159): filter,
// backdrop-filter, mix-blend-mode, isolation, clip-path and mask
// applied to a widget's painted result. The no-effect frame keeps the
// old path — opacity through PushAlpha, brightness through
// PushBrightness, no layer — so a stylesheet without effects paints
// exactly as before, allocation-free. Anything heavier paints the
// widget offscreen (a Canvas Layer reused across frames) and
// composites the layer back through the effect chain: filters, then
// the clip-path mask, then the mask-image mask, then the blend mode.

import (
	"math"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// effects is the pushed effect state: the canvas to paint into (the
// layer's when any effect forced one, the caller's otherwise) and
// what pop owes to composite back.
type effects struct {
	cv     *render.Canvas
	dst    *render.Canvas
	layer  *render.Layer
	region render.Rect
	v      *style.Values
	bounds render.Rect
	radii  render.Corners
	pa, pb float64
	op, br bool
	blend  render.BlendMode
}

// effectsHeavy reports whether v's effect declarations need the
// offscreen layer: an opacity under one (CSS group opacity: the
// subtree composites as one image, so its overlapping paints do not
// show through each other), a filter beyond brightness, a clip-path, a
// mask, a blend mode other than normal, or isolation (whose group the
// descendants' blends then land on).
func effectsHeavy(v *style.Values) bool {
	if v.Has(style.PropOpacity) && v.Opacity != 1 {
		return true
	}
	if v.Has(style.PropFilter) && !v.Filter.Empty() {
		if _, only := v.Filter.BrightnessOnly(); !only {
			return true
		}
	}
	if v.Has(style.PropClipPath) && !v.Clip.Empty() {
		return true
	}
	if v.Has(style.PropMaskImage) && !v.Mask.Image.None() {
		return true
	}
	if v.Has(style.PropMixBlendMode) && v.Blend != render.BlendNormal {
		return true
	}
	return v.Has(style.PropIsolation) && v.Isolate
}

// pushEffects applies the subtree effects — opacity, brightness, and
// the rendering effects of #159 — and returns the state pop
// composites with. bounds and radii are the border box the effects
// apply to.
// The returned canvas (fx.cv) is what the caller paints into.
func pushEffects(cv *render.Canvas, v *style.Values, bounds render.Rect, radii render.Corners) effects {
	if !backfaceHidden(v) {
		paintBackdropOf(cv, v, bounds, radii)
	}
	return pushLayerEffects(cv, v, bounds, radii)
}

// backfaceHidden reports backface-visibility on the flat pipeline: a
// transform whose composed map flips the plane hides the widget.
func backfaceHidden(v *style.Values) bool {
	return v.Has(style.PropBackfaceVisibility) && v.BackfaceHidden &&
		v.Has(style.PropTransform) && v.Transform.Backface
}

// paintBackdropOf applies v's backdrop filter, when it has one, to
// what has been painted behind the widget so far this frame — first,
// whether or not the widget itself needs a layer.
func paintBackdropOf(cv *render.Canvas, v *style.Values, bounds render.Rect, radii render.Corners) {
	if v.Has(style.PropBackdropFilter) && !v.Backdrop.Empty() {
		paintBackdrop(cv, v, bounds, radii)
	}
}

// pushLayerEffects is pushEffects without the backdrop filter: the
// effects on the widget's own painted result, for a caller that
// sampled the backdrop itself (the transform layer, whose canvas holds
// no backdrop). A hidden backface paints at zero opacity, so the
// hidden case allocates nothing.
func pushLayerEffects(cv *render.Canvas, v *style.Values, bounds render.Rect, radii render.Corners) effects {
	e := effects{cv: cv, dst: cv, v: v, bounds: bounds, radii: radii}
	if backfaceHidden(v) {
		e.pa = cv.PushAlpha(0)
		e.op = true
		return e
	}
	e.op = v.Has(style.PropOpacity) && v.Opacity != 1
	if v.Has(style.PropFilter) && v.Brightness != 1 {
		if _, only := v.Filter.BrightnessOnly(); only || v.Filter.Empty() {
			e.br = true
		}
	}
	if !effectsHeavy(v) {
		e.pushFast(cv, v)
		return e
	}
	e.region = cv.MapRect(v.Filter.Extent().Grow(bounds)).Intersect(cv.Rect())
	e.layer = cv.ScratchLayer(e.region)
	// The layer paints opaque; finishEffects composites it at the
	// opacity.
	e.op = false
	e.pushFast(e.layer.Canvas(), v)
	if v.Has(style.PropMixBlendMode) {
		e.blend = v.Blend
	}
	return e
}

// pushFast pushes only the cheap effects — opacity and brightness —
// onto cv; popFast restores them.
func (e *effects) pushFast(cv *render.Canvas, v *style.Values) {
	e.cv = cv
	if e.op {
		e.pa = cv.PushAlpha(v.Opacity)
	}
	if e.br {
		e.pb = cv.PushBrightness(v.Brightness)
	}
}

// popFast restores pushFast's pushes.
func (e *effects) popFast() {
	if e.br {
		e.cv.PopBrightness(e.pb)
	}
	if e.op {
		e.cv.PopAlpha(e.pa)
	}
}

// pop restores what pushEffects pushed: the fast path pops in place,
// the heavy path filters the layer, applies the clip and mask, and
// blends it back onto the canvas.
func (e *effects) pop() {
	e.popFast()
	if e.layer != nil {
		finishEffects(e.dst, e.v, e.bounds, e.radii, e.layer, e.region, e.blend)
		e.layer.Release()
	}
}

// finishEffects composites a painted layer through the heavy effect
// chain: the filters, the clip-path mask, the mask-image mask, then
// the blend-mode draw at the opacity.
func finishEffects(cv *render.Canvas, v *style.Values, bounds render.Rect, radii render.Corners,
	layer *render.Layer, region render.Rect, mode render.BlendMode,
) {
	if !v.Filter.Empty() {
		render.ApplyFilters(layer, region, &v.Filter, render.FilterShape{Box: bounds, Radii: radii})
	}
	if v.Has(style.PropClipPath) && !v.Clip.Empty() {
		if p, ok := clipShapePath(&v.Clip, bounds, radii); ok {
			layer.ApplyPathMask(p, region)
		}
	}
	if v.Has(style.PropMaskImage) && !v.Mask.Image.None() {
		applyMaskLayer(cv, v, layer, region, bounds)
	}
	if v.Has(style.PropOpacity) && v.Opacity != 1 {
		prev := cv.PushAlpha(v.Opacity)
		defer cv.PopAlpha(prev)
	}
	cv.DrawLayer(layer, region, mode)
}

// paintBackdrop applies backdrop-filter: the canvas region under the
// widget's box (grown by the filters' extent) is snapshotted,
// filtered, clipped to the rounded border box, and composited back
// over the unfiltered original — the filtered half replacing the
// backdrop inside the shape, the original showing outside it.
func paintBackdrop(cv *render.Canvas, v *style.Values, bounds render.Rect, radii render.Corners) {
	region := cv.MapRect(v.Backdrop.Extent().Grow(bounds)).Intersect(cv.Rect())
	if region.Empty() {
		return
	}
	snap := cv.ScratchSnapshot(region)
	defer snap.Release()
	filt := cv.ScratchLayer(region)
	defer filt.Release()
	filt.Canvas().DrawCanvasDevice(snap.Canvas(), 0, 0)
	render.ApplyFilters(filt, region, &v.Backdrop, render.FilterShape{Box: bounds, Radii: radii})
	p := &render.Path{}
	render.RoundedRectPath(p, bounds, radii)
	filt.ApplyPathMask(p, region)
	cv.DrawLayer(filt, region, render.BlendNormal)
}

// clipShapePath builds the clip shape's geometry against the border
// box (logical pixels): the inset rect with its rounded corners, the
// circle and ellipse, the polygon, or the parsed path() data offset
// onto the box's origin.
func clipShapePath(c *style.ClipShape, bounds render.Rect, radii render.Corners) (*render.Path, bool) {
	p := &render.Path{}
	w, h := float64(bounds.W), float64(bounds.H)
	circleR := func(nv style.NumVal) float64 {
		if nv.Pct {
			return nv.V / 100 * mathSqrtDiag(w, h)
		}
		return nv.V
	}
	switch c.Kind {
	case style.ClipInset:
		t := c.Inset[0].Resolve(h)
		r := c.Inset[1].Resolve(w)
		b := c.Inset[2].Resolve(h)
		l := c.Inset[3].Resolve(w)
		in := render.Rect{
			X: bounds.X + int(math.Round(l)), Y: bounds.Y + int(math.Round(t)),
			W: max(0, bounds.W-int(math.Round(l+r))), H: max(0, bounds.H-int(math.Round(t+b))),
		}
		cr := render.Corners{
			TopLeft: c.InsetRad[0], TopRight: c.InsetRad[1],
			BottomRight: c.InsetRad[2], BottomLeft: c.InsetRad[3],
		}
		if cr == (render.Corners{}) {
			cr = radii
		}
		render.RoundedRectPath(p, in, cr)
	case style.ClipCircle:
		cx := float64(bounds.X) + c.CenterX.Resolve(w)
		cy := float64(bounds.Y) + c.CenterY.Resolve(h)
		render.EllipsePath(p, cx, cy, circleR(c.Rad), circleR(c.Rad))
	case style.ClipEllipse:
		rx, ry := circleR(c.RadX), circleR(c.RadY)
		cx := float64(bounds.X) + c.CenterX.Resolve(w)
		cy := float64(bounds.Y) + c.CenterY.Resolve(h)
		render.EllipsePath(p, cx, cy, rx, ry)
	case style.ClipPolygon:
		for i := 0; i+1 < len(c.Poly); i += 2 {
			x := float64(bounds.X) + c.Poly[i].Resolve(w)
			y := float64(bounds.Y) + c.Poly[i+1].Resolve(h)
			if i == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		p.Close()
	case style.ClipPathData:
		d, ok := render.ParsePathData(c.Path)
		if !ok {
			return nil, false
		}
		p = d.Offset(float64(bounds.X), float64(bounds.Y))
	default:
		return nil, false
	}
	if p.Empty() {
		return nil, false
	}
	return p, true
}

// mathSqrtDiag is sqrt(w²+h²)/sqrt(2): CSS's reference for a
// percentage circle radius.
func mathSqrtDiag(w, h float64) float64 {
	return math.Sqrt(w*w+h*h) / math.Sqrt2
}

// applyMaskLayer paints the mask layer into its own layer over the
// effect region — sized, positioned and repeated in the border box
// like a background layer — and multiplies the widget layer's pixels
// by it: the alpha channel for mask-mode alpha, the WCAG luminance for
// luminance.
func applyMaskLayer(cv *render.Canvas, v *style.Values, layer *render.Layer, region, bounds render.Rect) {
	m := &v.Mask
	mask := cv.ScratchLayer(region)
	defer mask.Release()
	mc := mask.Canvas()
	mc.ClearDevice(region, 0)
	if img, ok := resolveLayerImage(m.Image); ok {
		paintLayerImage(mc, img, layerTile(img, m.Size, m.Pos, m.Repeat, bounds), bounds, render.Corners{})
	}
	layer.ApplyMaskLayer(mask, region, m.Luminance)
}
