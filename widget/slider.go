package widget

import (
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Slider is a slider over [min, max] with an optional step, horizontal
// by default; SetAxis(Column) stands it up (GTK's orientation), the
// minimum at the top unless inverted (GTK's inverted, the volume-style
// minimum at the bottom). Pressing warps the value to the pointer and a
// drag follows it.
//
// It is GtkScale's node tree: `scale` (its box, opacity, outline, and
// :hover while the pointer is over it) holding `trough` (background,
// radius, min-height, and min-width as the natural width) holding
// `highlight` (the filled part) and `slider` (the knob: background,
// radius, min size, opacity, box-shadow). Unstyled it keeps the theme's
// 4px trough and 14px knob. The scale carries .horizontal or .vertical;
// across a vertical scale the trough's min-width is its thickness and
// min-height its length.
type Slider struct {
	node
	min, max, step, value float64
	axis                  Axis
	inverted              bool

	// Pressed reports whether a drag is in progress.
	Pressed bool
	hovered bool
	trough  sliderTrough

	// OnChanged fires after every value change, including programmatic
	// ones.
	OnChanged func(v float64)
}

// NewSlider returns a horizontal slider over [min, max] at value, with
// changes snapped to step (0 disables snapping). The pointer turns to
// the hand over it, the way wayle's slider primitives ask.
func NewSlider(min, max, step, value float64) *Slider {
	s := &Slider{min: min, max: max, step: step}
	s.cursorName = "pointer"
	s.value = s.clamp(value)
	s.AddClass("horizontal")
	s.trough.SetElement("trough")
	s.trough.highlight.SetElement("highlight")
	s.trough.knob.SetElement("slider")
	setParents(s, &s.trough)
	setParents(&s.trough, &s.trough.highlight, &s.trough.knob)
	return s
}

// SetAxis orients the slider: Row horizontal, Column vertical.
func (s *Slider) SetAxis(axis Axis) {
	if s.axis == axis {
		return
	}
	s.axis = axis
	if axis == Column {
		s.RemoveClass("horizontal")
		s.AddClass("vertical")
	} else {
		s.RemoveClass("vertical")
		s.AddClass("horizontal")
	}
	s.InvalidateLayout()
}

// Axis reports the orientation.
func (s *Slider) Axis() Axis { return s.axis }

// SetInverted puts the minimum at the far end: the right of a
// horizontal slider, the bottom of a vertical one.
func (s *Slider) SetInverted(on bool) {
	s.inverted = on
	s.Invalidate()
}

// Inverted reports whether the minimum is at the far end.
func (s *Slider) Inverted() bool { return s.inverted }

// sliderTrough is the scale's trough node and its highlight and knob.
type sliderTrough struct {
	stylePart
	highlight, knob stylePart
}

func (t *sliderTrough) appendStyleKids(buf []Widget) []Widget {
	return append(buf, &t.highlight, &t.knob)
}

// appendStyleKids is the trough (styleKids).
func (s *Slider) appendStyleKids(buf []Widget) []Widget { return append(buf, &s.trough) }

// SetHovered implements HoverSetter: `scale:hover` restyles the
// trough and knob (a knob shown only on hover).
func (s *Slider) SetHovered(on bool) {
	if s.hovered != on {
		s.hovered = on
		s.invalidateState(style.Hover)
	}
}

// sliderPad is an unstyled trough's inset each side, room for the
// knob at the extremes.
const sliderPad = 4

// troughProps are the trough's length and thickness properties along
// the slider's axis.
func (s *Slider) troughProps() (length, thickness style.Prop) {
	if s.axis == Column {
		return style.PropMinHeight, style.PropMinWidth
	}
	return style.PropMinWidth, style.PropMinHeight
}

// troughRect is the trough inside the content box: its thickness (6px
// unstyled) centered across the axis, the content's length along it
// (less the unstyled inset).
func (s *Slider) troughRect() render.Rect {
	_, c := boxRects(boxOf(s.style(s), render.Insets{}), s.bounds)
	tv := s.trough.style(&s.trough)
	_, thickProp := s.troughProps()
	th := picki(tv, thickProp, 6)
	pad := sliderPad
	if tv.Has(thickProp) || tv.Has(style.PropBackgroundColor) {
		pad = 0
	}
	if s.axis == Column {
		return render.Rect{X: c.X + (c.W-th)/2, Y: c.Y + pad, W: th, H: max(c.H-2*pad, 0)}
	}
	return render.Rect{X: c.X + pad, Y: c.Y + (c.H-th)/2, W: max(c.W-2*pad, 0), H: th}
}

// along maps the trough onto the value axis: the pixel the minimum
// sits at and the signed length to the maximum.
func (s *Slider) along(tr render.Rect) (origin, span int) {
	origin, span = tr.X, tr.W
	if s.axis == Column {
		origin, span = tr.Y, tr.H
	}
	// Vertical runs top-down by default; inverted flips either axis.
	if s.inverted {
		return origin + span, -span
	}
	return origin, span
}

// knobSize is the knob's min size (14px unstyled).
func (s *Slider) knobSize() (w, h int) {
	kv := s.trough.knob.style(&s.trough.knob)
	return picki(kv, style.PropMinWidth, 16), picki(kv, style.PropMinHeight, 16)
}

// Value returns the current value.
func (s *Slider) Value() float64 {
	return s.value
}

// SetValue clamps v to [min, max], snaps it to step, and fires OnChanged
// when it moved.
func (s *Slider) SetValue(v float64) {
	v = s.clamp(v)
	if v == s.value {
		return
	}
	s.value = v
	s.Invalidate()
	if s.OnChanged != nil {
		s.OnChanged(v)
	}
}

func (s *Slider) clamp(v float64) float64 {
	if s.step > 0 {
		v = math.Round(v/s.step) * s.step
	}
	return math.Min(s.max, math.Max(s.min, v))
}

// ValueAt maps a point across the trough to a value (an unstyled
// trough is inset 4px each end so the extremes are reachable).
func (s *Slider) ValueAt(p Point) float64 {
	origin, span := s.along(s.troughRect())
	if span == 0 {
		return s.min
	}
	pos := p.X
	if s.axis == Column {
		pos = p.Y
	}
	t := float64(pos-origin) / float64(span)
	t = math.Min(1, math.Max(0, t))
	return s.min + t*(s.max-s.min)
}

// Measure wants the trough's length (200px unstyled) by the thicker
// of the trough and the knob, at least 18px, inside the scale's box.
func (s *Slider) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	v := s.style(s)
	tv := s.trough.style(&s.trough)
	kw, kh := s.knobSize()
	lenProp, thickProp := s.troughProps()
	length := picki(tv, lenProp, 200)
	sz := Size{W: length, H: max(18, picki(tv, thickProp, 4), kh)}
	if s.axis == Column {
		sz = Size{W: max(18, picki(tv, thickProp, 4), kw), H: length}
	}
	return s.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(sz, inner)
	}))
}

// Paint draws the trough, its highlight, and the knob, each from its
// node's style over the theme's colors. Disabled, the theme colors fade
// through the derived disabled colors (a stylesheet fades with its own
// :disabled rules).
func (s *Slider) Paint(cv *render.Canvas) {
	t := Current()
	troughCol, fillCol, knobCol := t.MutedColor(), t.Accent, t.Text
	v := s.style(s)
	if !IsEnabled(s) {
		troughCol, fillCol, knobCol = t.DisabledText(), t.DisabledAccent(), t.DisabledText()
	}
	fillCol = accentOf(v, fillCol)
	fx := pushEffects(cv, v, s.bounds, render.Corners{})
	cv = fx.cv
	defer fx.pop()
	tr := s.troughRect()
	tv := s.trough.style(&s.trough)
	trRadii := radiusOr(tv, min(tr.W, tr.H)/2)
	paintBoxBehind(cv, tv, tr, trRadii, borderOf(tv), pickc(0, tv, style.PropBackgroundColor, troughCol))

	// The highlight runs from the minimum's end to the knob.
	origin, span := s.along(tr)
	at := origin + int(float64(span)*s.fraction())
	lo, hi := min(origin, at), max(origin, at)
	filled := render.Rect{X: lo, Y: tr.Y, W: hi - lo, H: tr.H}
	if s.axis == Column {
		filled = render.Rect{X: tr.X, Y: lo, W: tr.W, H: hi - lo}
	}
	hv := s.trough.highlight.style(&s.trough.highlight)
	if hi > lo {
		paintBoxBehind(cv, hv, filled, radiusOr(hv, trRadii.TopLeft), borderOf(hv), pickc(0, hv, style.PropBackgroundColor, fillCol))
	}

	kv := s.trough.knob.style(&s.trough.knob)
	kw, kh := s.knobSize()
	knob := render.Rect{X: at - kw/2, Y: tr.Y + tr.H/2 - kh/2, W: kw, H: kh}
	if s.axis == Column {
		knob = render.Rect{X: tr.X + tr.W/2 - kw/2, Y: at - kh/2, W: kw, H: kh}
	}
	kfx := pushEffects(cv, kv, knob, radiusOr(kv, min(kw, kh)/2))
	paintBoxBehind(kfx.cv, kv, knob, radiusOr(kv, min(kw, kh)/2), borderOf(kv), pickc(0, kv, style.PropBackgroundColor, knobCol))
	kfx.pop()
	paintOutline(cv, v, s.bounds, radiusOr(v, 0))
}

func (s *Slider) fraction() float64 {
	if s.max == s.min {
		return 0
	}
	return (s.value - s.min) / (s.max - s.min)
}

// Role implements Roleer.
func (s *Slider) Role() Role { return RoleSlider }

// HitTest returns the slider when p is inside its bounds.
func (s *Slider) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

// SetPressed implements PressSetter; the trough brightens while dragging.
func (s *Slider) SetPressed(on bool) {
	if s.Pressed == on {
		return
	}
	s.Pressed = on
	s.invalidateState(style.Active)
}

// PressAt implements PressAter: the value warps to the press, and a
// drag carries on from there (GTK's primary-button warp). Disabled
// sliders ignore it.
func (s *Slider) PressAt(p Point) {
	if IsEnabled(s) {
		s.SetValue(s.ValueAt(p))
	}
}

// DragMove sets the value from the pointer position while pressed.
// Disabled sliders ignore the drag.
func (s *Slider) DragMove(p Point) {
	if !IsEnabled(s) {
		return
	}
	s.SetValue(s.ValueAt(p))
}

// keyDirection is the value's direction for an arrow: the arrows
// along the axis move the knob the way they point (so the value's
// sign follows the orientation and inversion), the cross arrows keep
// up/right as more.
func (s *Slider) keyDirection(a KeyAction) float64 {
	d := 1.0
	if a == KeyLeft || a == KeyDown {
		d = -1
	}
	alongAxis := (s.axis == Row) == (a == KeyLeft || a == KeyRight)
	if !alongAxis {
		return d
	}
	if s.axis == Column {
		d = -d // down the screen is toward a vertical slider's far end
	}
	if s.inverted {
		d = -d
	}
	return d
}

// KeyAction implements KeyActionHandler: arrows nudge the value by one
// step (or a tenth of the range without one), Home and End jump.
// Disabled sliders ignore keys.
func (s *Slider) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(s) {
		return
	}
	step := s.step
	if step <= 0 {
		step = (s.max - s.min) / 10
	}
	switch a {
	case KeyLeft, KeyRight, KeyUp, KeyDown:
		s.SetValue(s.value + step*s.keyDirection(a))
	case KeyHome:
		s.SetValue(s.min)
	case KeyEnd:
		s.SetValue(s.max)
	}
}
