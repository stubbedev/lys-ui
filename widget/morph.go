package widget

import (
	"math"

	"github.com/stubbedev/lys-lib/render"
)

// Morph is a transient surface's enter and exit visual: it grows its
// child out of an origin side - scaling up from 92% while a rounded
// clip unrolls from that side and the subtree fades in - and shrinks
// it back on the way out. A reveal past 1 (a spring's overshoot)
// swells the child a touch past its size. Without morphing it only
// fades. Layout and hit-testing pass straight through: the motion is
// visual, and a surface mid-enter takes clicks like one at rest. The
// paint never leaves the wrapper's bounds.
type Morph struct {
	node
	child Widget
	// reveal is 0 hidden through 1 at rest; morph picks growing over a
	// plain fade.
	reveal float64
	morph  bool
	// originX and originY are the fractions of the bounds the motion
	// grows from: (0.5, 0) the top edge's middle, (0.5, 0.5) the center.
	originX, originY float64
	// radius rounds the unrolling clip's corners.
	radius int
	// inset and plate describe a surface whose content sits inside a
	// margin (a popup's shadow gutter): the content is the bounds less
	// inset, and plate paints the surface's own backing (shadow and
	// plate) for a content rect - at rest the whole content, mid-motion
	// the unrolled part, so the elevation follows the visible shape.
	inset int
	plate func(cv *render.Canvas, content render.Rect)
	layer *render.Layer
	inner *render.Layer
}

// NewMorph wraps child at rest, growing from its center.
func NewMorph(child Widget) *Morph {
	return &Morph{child: child, reveal: 1, originX: 0.5, originY: 0.5}
}

// Child returns the wrapped widget.
func (m *Morph) Child() Widget { return m.child }

// SetReveal applies one motion frame: the reveal fraction and whether
// the child grows (true) or only fades.
func (m *Morph) SetReveal(reveal float64, morph bool) {
	if m.reveal == reveal && m.morph == morph {
		return
	}
	m.reveal, m.morph = reveal, morph
	m.Invalidate()
}

// Reveal is the current reveal fraction.
func (m *Morph) Reveal() float64 { return m.reveal }

// SetOrigin sets the point the motion grows from, as fractions of the
// bounds: a popover below its anchor grows from (0.5, 0).
func (m *Morph) SetOrigin(fx, fy float64) {
	m.originX, m.originY = fx, fy
	m.Invalidate()
}

// SetRadius rounds the unrolling clip's corners, the surface's own.
func (m *Morph) SetRadius(r int) {
	m.radius = r
	m.Invalidate()
}

// SetPlate gives the surface a margin of inset around its content and
// the painter of its backing for a content rect; see the field.
func (m *Morph) SetPlate(inset int, plate func(cv *render.Canvas, content render.Rect)) {
	m.inset, m.plate = inset, plate
	m.Invalidate()
}

// content is the content rect: the bounds less the plate inset.
func (m *Morph) content() render.Rect {
	return render.UniformInsets(m.inset).Shrink(m.bounds)
}

// shownContent is the content rect the backing is drawn for: the
// child's own report when it has one (a surface root whose content is
// mid layout motion), else the content rect.
func (m *Morph) shownContent() render.Rect {
	if s, ok := m.child.(interface{ ShownRect() render.Rect }); ok {
		return s.ShownRect()
	}
	return m.content()
}

// paintAtRest paints the backing and the child, unanimated.
func (m *Morph) paintAtRest(cv *render.Canvas) {
	if m.plate != nil {
		m.plate(cv, m.shownContent())
	}
	PaintChild(cv, m.child)
}

// Measure reports the child's natural size plus the plate inset on
// every side, clamped to con.
func (m *Morph) Measure(con Constraints) Size {
	if sz, ok := m.measureHit(con); ok {
		return sz
	}
	in := 2 * m.inset
	inner := Constraints{Max: Size{W: max(0, con.Max.W-in), H: max(0, con.Max.H-in)}}
	sz := measureChild(m, m.child, inner)
	return m.measureStore(con, clampSize(Size{W: sz.W + in, H: sz.H + in}, con))
}

// Arrange gives the child the content rect: r less the plate inset.
func (m *Morph) Arrange(r render.Rect) {
	m.ArrangeRoot(r)
	arrangeChild(m.child, render.UniformInsets(m.inset).Shrink(r))
	setParents(m, m.child)
}

// ArrangeRoot records the wrapper's own rect.
func (m *Morph) ArrangeRoot(r render.Rect) { m.node.Arrange(r) }

// morphScale and morphClip are where the growth starts: the scale and
// the clip's unrolled fraction at reveal 0. The fade runs ahead of
// them (morphFade), so the surface is opaque well before it is whole.
const (
	morphScale = 0.92
	morphClip  = 0.3
	morphFade  = 1.6
)

// Paint draws the child at the current reveal.
func (m *Morph) Paint(cv *render.Canvas) {
	r := m.reveal
	// Only a morphing frame keeps the layers, full-canvas buffers: at
	// rest, hidden or fading, the content paints directly.
	layer, inner := m.layer, m.inner
	m.layer, m.inner = nil, nil
	defer keepOrFree(layer, &m.layer)
	defer keepOrFree(inner, &m.inner)
	switch {
	case r <= 0 || m.child == nil:
		return
	case !m.morph && r >= 1, m.morph && r == 1:
		m.paintAtRest(cv)
		return
	case !m.morph:
		prev := cv.PushAlpha(r)
		m.paintAtRest(cv)
		cv.PopAlpha(prev)
		return
	}
	b := m.bounds
	dev := cv.MapRect(b)
	if dev.Empty() {
		return
	}
	l := cv.Layer(layer, dev)
	m.layer = l
	content := m.shownContent()
	// The clip unrolls from the origin side; a centered origin has no
	// side, so it only scales.
	clip := m.clipAt(content, r)
	if clip == content {
		m.paintAtRest(l.Canvas())
	} else {
		if m.plate != nil {
			m.plate(l.Canvas(), clip)
		}
		in := cv.Layer(inner, dev)
		m.inner = in
		PaintChild(in.Canvas(), m.child)
		p := &render.Path{}
		render.RoundedRectPath(p, clip, render.UniformCorners(m.radius))
		in.ApplyPathMask(p, dev)
		l.Canvas().DrawLayer(in, dev, render.BlendNormal)
	}
	s := morphScale + (1-morphScale)*r
	devContent := cv.MapRect(content)
	ox := float64(devContent.X) + m.originX*float64(devContent.W)
	oy := float64(devContent.Y) + m.originY*float64(devContent.H)
	prev := cv.PushClip(b)
	cv.Composite(l, dev, render.Scale(s, s).About(ox, oy), math.Min(1, r*morphFade))
	cv.PopClip(prev)
}

// keepOrFree frees taken, a layer a transition took for this frame,
// unless the frame kept a layer in *kept (nil for none): canvas.Layer
// reuses a layer it is handed or frees it for a bigger one, so only a
// frame at rest, which takes no layer, leaves taken to free.
func keepOrFree(taken *render.Layer, kept **render.Layer) {
	if taken == nil || (kept != nil && *kept != nil) {
		return
	}
	taken.Free()
}

// clipAt is the unrolled clip of the content rect b at reveal r: cut
// from the side opposite the origin, whole from morphClip of the size
// at 0 to all of it at rest.
func (m *Morph) clipAt(b render.Rect, r float64) render.Rect {
	f := math.Min(1, math.Max(0, morphClip+(1-morphClip)*r))
	switch {
	case m.originY == 0:
		b.H = int(math.Ceil(float64(b.H) * f))
	case m.originY == 1:
		h := int(math.Ceil(float64(b.H) * f))
		b.Y, b.H = b.Y+b.H-h, h
	case m.originX == 0:
		b.W = int(math.Ceil(float64(b.W) * f))
	case m.originX == 1:
		w := int(math.Ceil(float64(b.W) * f))
		b.X, b.W = b.X+b.W-w, w
	}
	return b
}

// HitTest returns the child's pick, or the wrapper over its bounds.
func (m *Morph) HitTest(p Point) Widget {
	if hit := HitChild(m.child, p); hit != nil {
		return hit
	}
	return m.HitLeaf(m, p)
}

// Children exposes the child for the damage collector, focus
// traversal, and the accessibility walk.
func (m *Morph) Children() []Widget {
	if m.child == nil {
		return nil
	}
	return []Widget{m.child}
}

// appendChildren appends the wrapped child, matching Children.
func (m *Morph) appendChildren(buf []Widget) []Widget {
	if m.child != nil {
		return append(buf, m.child)
	}
	return buf
}
