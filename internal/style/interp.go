package style

import (
	"math"

	"github.com/stubbedev/lys-lib/render"
)

// Interpolation and composition of computed values (css-values-4 §3,
// web-animations-1 §5.3): the one routine the keyframe animations and
// the transitions share. A longhand with a numeric, color, length or
// transform value interpolates smoothly; every other longhand animates
// discretely, taking the start value before the midpoint and the end
// value from it on.

// interp is one longhand's interpolation: lerp writes the value at t
// and reports false when the pair has no smooth midpoint (a keyword
// against a length, a transform list of other functions); add is its
// additive composition; eq compares two values. declared marks a
// longhand that interpolates only between two declared values: an
// undeclared side is the widget's programmatic geometry, not a CSS
// value to start from, so the change is discrete.
type interp struct {
	lerp     func(dst, a, b *Values, t float64) bool
	add      func(dst, under *Values)
	eq       func(a, b *Values) bool
	declared bool
}

// interps holds the smoothly interpolating longhands.
var interps [numProps]interp

// discreteEq compares the discrete longhands a transition may flip
// under transition-behavior: allow-discrete.
var discreteEq = map[Prop]func(a, b *Values) bool{
	PropIsolation:          func(a, b *Values) bool { return a.Isolate == b.Isolate },
	PropBackfaceVisibility: func(a, b *Values) bool { return a.BackfaceHidden == b.BackfaceHidden },
	PropLayoutMotion:       func(a, b *Values) bool { return a.LayoutMotion == b.LayoutMotion },
	PropMixBlendMode:       func(a, b *Values) bool { return a.Blend == b.Blend },
	PropAppearance:         func(a, b *Values) bool { return a.Appearance == b.Appearance },
	PropCaretShape:         func(a, b *Values) bool { return a.CaretShape == b.CaretShape },
	PropVisibility:         func(a, b *Values) bool { return a.Layout.Visibility == b.Layout.Visibility },
}

// initial holds the CSS initial values the interpolation reads for an
// endpoint that does not declare the longhand.
var initial = func() Values {
	v := initialValues()
	v.Layout.FlexShrink = 1
	return v
}()

func init() {
	colors := map[Prop]func(*Values) *render.Color{
		PropColor:               func(v *Values) *render.Color { return &v.Color },
		PropBackgroundColor:     func(v *Values) *render.Color { return &v.Background },
		PropBorderTopColor:      func(v *Values) *render.Color { return &v.BorderColor[0] },
		PropBorderRightColor:    func(v *Values) *render.Color { return &v.BorderColor[1] },
		PropBorderBottomColor:   func(v *Values) *render.Color { return &v.BorderColor[2] },
		PropBorderLeftColor:     func(v *Values) *render.Color { return &v.BorderColor[3] },
		PropOutlineColor:        func(v *Values) *render.Color { return &v.OutlineColor },
		PropCaretColor:          func(v *Values) *render.Color { return &v.CaretColor },
		PropTextDecorationColor: func(v *Values) *render.Color { return &v.Decoration.Color },
	}
	for p, get := range colors {
		interps[p] = colorInterp(get)
	}
	floats := map[Prop]func(*Values) *float64{
		PropLetterSpacing: func(v *Values) *float64 { return &v.LetterSpacing },
		PropWordSpacing:   func(v *Values) *float64 { return &v.WordSpacing },
		PropLineHeight:    func(v *Values) *float64 { return &v.LineHeight },
		PropFontSize:      func(v *Values) *float64 { return &v.FontSize },
		PropTextIndent:    func(v *Values) *float64 { return &v.TextIndent },
		PropPerspective:   func(v *Values) *float64 { return &v.Perspective },
		PropFlexGrow:      func(v *Values) *float64 { return &v.Layout.FlexGrow },
		PropFlexShrink:    func(v *Values) *float64 { return &v.Layout.FlexShrink },
	}
	for p, get := range floats {
		interps[p] = floatInterp(get, math.Inf(-1), math.Inf(1), true)
	}
	interps[PropOpacity] = floatInterp(func(v *Values) *float64 { return &v.Opacity }, 0, 1, false)
	ints := map[Prop]func(*Values) *int{
		PropFontWeight:    func(v *Values) *int { return &v.FontWeight },
		PropOutlineWidth:  func(v *Values) *int { return &v.OutlineWidth },
		PropOutlineOffset: func(v *Values) *int { return &v.OutlineOffset },
		PropZIndex:        func(v *Values) *int { return &v.Layout.ZIndex },
		PropOrder:         func(v *Values) *int { return &v.Layout.Order },
	}
	for p, get := range ints {
		interps[p] = intInterp(get)
	}
	for i := range 4 {
		interps[PropPaddingTop+Prop(i)] = intInterp(func(v *Values) *int { return v.Padding.ptr(i) })
		interps[PropMarginTop+Prop(i)] = intInterp(func(v *Values) *int { return v.Margin.ptr(i) })
		interps[PropBorderTopWidth+Prop(i)] = intInterp(func(v *Values) *int { return v.BorderWidth.ptr(i) })
		interps[PropBorderTopLeftRadius+Prop(i)] = intInterp(func(v *Values) *int { return v.Radius.ptr(i) })
		interps[PropTop+Prop(i)] = sizingInterp(func(v *Values) *Sizing { return &v.Layout.Inset[i] })
	}
	sizings := map[Prop]func(*Values) *Sizing{
		PropWidth:     func(v *Values) *Sizing { return &v.Layout.Width },
		PropHeight:    func(v *Values) *Sizing { return &v.Layout.Height },
		PropMinWidth:  func(v *Values) *Sizing { return &v.Layout.MinWidth },
		PropMinHeight: func(v *Values) *Sizing { return &v.Layout.MinHeight },
		PropMaxWidth:  func(v *Values) *Sizing { return &v.Layout.MaxWidth },
		PropMaxHeight: func(v *Values) *Sizing { return &v.Layout.MaxHeight },
		PropFlexBasis: func(v *Values) *Sizing { return &v.Layout.FlexBasis },
	}
	for p, get := range sizings {
		interps[p] = sizingInterp(get)
	}
	interps[PropRowGap] = lengthInterp(func(v *Values) *Length { return &v.Layout.RowGap })
	interps[PropColumnGap] = lengthInterp(func(v *Values) *Length { return &v.Layout.ColumnGap })
	interps[PropTransform] = xformInterp(func(v *Values) *Xform { return &v.Transform })
	interps[PropIconTransform] = xformInterp(func(v *Values) *Xform { return &v.IconXform })
	interps[PropTransformOrigin] = interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			for i := range 2 {
				dst.OriginFrac[i] = lerpF(a.OriginFrac[i], b.OriginFrac[i], t)
				dst.OriginPx[i] = lerpF(a.OriginPx[i], b.OriginPx[i], t)
			}
			return true
		},
		eq: func(a, b *Values) bool { return a.OriginFrac == b.OriginFrac && a.OriginPx == b.OriginPx },
	}
	interps[PropFilter] = interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			_, ao := a.Filter.BrightnessOnly()
			_, bo := b.Filter.BrightnessOnly()
			if !ao && !a.Filter.Empty() || !bo && !b.Filter.Empty() {
				return false
			}
			dst.Filter = b.Filter
			if b.Filter.Empty() {
				dst.Filter = a.Filter
			}
			dst.Brightness = lerpF(a.Brightness, b.Brightness, t)
			return true
		},
		add: func(dst, under *Values) { dst.Brightness *= under.Brightness },
		eq:  func(a, b *Values) bool { return a.Filter == b.Filter && a.Brightness == b.Brightness },
	}
	interps[PropAccentColor] = interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			if a.AccentAuto || b.AccentAuto {
				return false
			}
			dst.AccentColor, dst.AccentAuto = lerpColor(a.AccentColor, b.AccentColor, t), false
			return true
		},
		eq: func(a, b *Values) bool { return a.AccentColor == b.AccentColor && a.AccentAuto == b.AccentAuto },
	}
	interps[PropScrollbarColor] = interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			if a.BarColorAuto || b.BarColorAuto {
				return false
			}
			dst.BarThumb, dst.BarTrack = lerpColor(a.BarThumb, b.BarThumb, t), lerpColor(a.BarTrack, b.BarTrack, t)
			dst.BarColorAuto = false
			return true
		},
		eq: func(a, b *Values) bool {
			return a.BarThumb == b.BarThumb && a.BarTrack == b.BarTrack && a.BarColorAuto == b.BarColorAuto
		},
	}
	interps[PropBoxShadow] = interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			s, ok := lerpShadows(&a.Shadow, &b.Shadow, t)
			dst.Shadow = s
			return ok
		},
		eq: func(a, b *Values) bool { return a.Shadow == b.Shadow },
	}
}

// Interpolate writes longhand p's value at progress t between a (t=0)
// and b (t=1) into dst and marks it set: smoothly where p interpolates,
// else discretely, flipping at t = 0.5 (a side that does not declare p
// then leaves it unset). An endpoint that does not declare a smooth
// longhand reads its initial value.
func Interpolate(dst, a, b *Values, p Prop, t float64) {
	if Smooth(a, b, p) {
		ea, eb := a, b
		if !a.Has(p) {
			ea = &initial
		}
		if !b.Has(p) {
			eb = &initial
		}
		if interps[p].lerp(dst, ea, eb, t) {
			dst.Set = dst.Set.With(p)
			return
		}
	}
	src := a
	if t >= 0.5 {
		src = b
	}
	CopyLonghand(dst, src, p)
}

// Smooth reports whether p interpolates smoothly between a and b
// rather than discretely: p has a smooth interpolation, and both sides
// declare it where the longhand needs that.
func Smooth(a, b *Values, p Prop) bool {
	ip := &interps[p]
	return ip.lerp != nil && (!ip.declared || a.Has(p) && b.Has(p))
}

// Changed reports whether p's computed value differs between a and b,
// for the longhands a transition can carry — the smooth ones and the
// discrete ones allow-discrete flips; known is false for the rest.
func Changed(a, b *Values, p Prop) (changed, known bool) {
	eq := interps[p].eq
	if eq == nil {
		eq = discreteEq[p]
	}
	if eq == nil {
		return false, false
	}
	return a.Has(p) != b.Has(p) || !eq(a, b), true
}

// Compose joins the effect value of p in dst onto the underlying value
// under by additive composition (css-animations-2 §4.1: add and
// accumulate agree on these value types): numbers and lengths sum,
// colors add per channel, transforms append the effect's functions.
// A longhand that does not add keeps the effect value, the replace
// composition.
func Compose(dst, under *Values, p Prop) {
	if f := interps[p].add; f != nil && under.Has(p) {
		f(dst, under)
	}
}

func lerpF(a, b, t float64) float64 { return a + (b-a)*t }

func lerpI(a, b int, t float64) int {
	return int(math.Round(float64(a) + float64(b-a)*t))
}

// lerpColor interpolates two premultiplied sRGB colors per channel,
// the space CSS Color 4 §12 interpolates legacy colors in.
func lerpColor(a, b render.Color, t float64) render.Color {
	mix := func(x, y uint8) uint32 {
		return uint32(min(max(math.Round(float64(x)+(float64(y)-float64(x))*t), 0), 255))
	}
	return render.Color(mix(a.A(), b.A())<<24 | mix(a.R(), b.R())<<16 | mix(a.G(), b.G())<<8 | mix(a.B(), b.B()))
}

// addColor sums two premultiplied colors per channel, saturating.
func addColor(a, b render.Color) render.Color {
	sum := func(x, y uint8) uint32 { return uint32(min(uint16(x)+uint16(y), 255)) }
	return render.Color(sum(a.A(), b.A())<<24 | sum(a.R(), b.R())<<16 | sum(a.G(), b.G())<<8 | sum(a.B(), b.B()))
}

func colorInterp(get func(*Values) *render.Color) interp {
	return interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			*get(dst) = lerpColor(*get(a), *get(b), t)
			return true
		},
		add: func(dst, under *Values) { *get(dst) = addColor(*get(under), *get(dst)) },
		eq:  func(a, b *Values) bool { return *get(a) == *get(b) },
	}
}

func floatInterp(get func(*Values) *float64, lo, hi float64, declared bool) interp {
	return interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			*get(dst) = min(max(lerpF(*get(a), *get(b), t), lo), hi)
			return true
		},
		add:      func(dst, under *Values) { *get(dst) = min(max(*get(dst)+*get(under), lo), hi) },
		eq:       func(a, b *Values) bool { return *get(a) == *get(b) },
		declared: declared,
	}
}

func intInterp(get func(*Values) *int) interp {
	return interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			*get(dst) = lerpI(*get(a), *get(b), t)
			return true
		},
		add:      func(dst, under *Values) { *get(dst) += *get(under) },
		eq:       func(a, b *Values) bool { return *get(a) == *get(b) },
		declared: true,
	}
}

func lengthInterp(get func(*Values) *Length) interp {
	return interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			x, y := *get(a), *get(b)
			*get(dst) = Length{Px: lerpI(x.Px, y.Px, t), Pct: lerpF(x.Pct, y.Pct, t)}
			return true
		},
		add: func(dst, under *Values) {
			d, u := get(dst), *get(under)
			d.Px, d.Pct = d.Px+u.Px, d.Pct+u.Pct
		},
		eq:       func(a, b *Values) bool { return *get(a) == *get(b) },
		declared: true,
	}
}

// sizingInterp interpolates two lengths pixels and percentages apart,
// CSS's calc() interpolation; a keyword at either end has no midpoint.
func sizingInterp(get func(*Values) *Sizing) interp {
	return interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			x, y := *get(a), *get(b)
			if x.Kind != SizingLength || y.Kind != SizingLength {
				return false
			}
			*get(dst) = Sizing{Kind: SizingLength, Len: Length{Px: lerpI(x.Len.Px, y.Len.Px, t), Pct: lerpF(x.Len.Pct, y.Len.Pct, t)}}
			return true
		},
		add: func(dst, under *Values) {
			d, u := get(dst), *get(under)
			if d.Kind == SizingLength && u.Kind == SizingLength {
				d.Len.Px, d.Len.Pct = d.Len.Px+u.Len.Px, d.Len.Pct+u.Len.Pct
			}
		},
		eq:       func(a, b *Values) bool { return *get(a) == *get(b) },
		declared: true,
	}
}

// xformInterp interpolates transforms by their function lists
// (css-transforms-2 §11): LerpXform walks matching primitives and
// falls back to the decomposed matrices.
func xformInterp(get func(*Values) *Xform) interp {
	return interp{
		lerp: func(dst, a, b *Values, t float64) bool {
			*get(dst) = LerpXform(*get(a), *get(b), t)
			return true
		},
		add: func(dst, under *Values) { *get(dst) = get(under).After(*get(dst)) },
		eq:  func(a, b *Values) bool { return get(a).M == get(b).M && get(a).P == get(b).P },
	}
}

// lerpShadows interpolates two box-shadow lists (css-backgrounds-3
// §7.2): layer by layer, the shorter list padded with transparent
// zero shadows of the other's inset; layers whose inset differs have
// no midpoint.
func lerpShadows(a, b *Shadows, t float64) (Shadows, bool) {
	var out Shadows
	out.N = max(a.N, b.N)
	for i := range out.N {
		x, y := a.Layers[i], b.Layers[i]
		switch {
		case i >= a.N:
			x = Shadow{Inset: y.Inset}
		case i >= b.N:
			y = Shadow{Inset: x.Inset}
		}
		if x.Inset != y.Inset {
			return Shadows{}, false
		}
		out.Layers[i] = Shadow{
			X: lerpI(x.X, y.X, t), Y: lerpI(x.Y, y.Y, t),
			Blur: max(0, lerpI(x.Blur, y.Blur, t)), Spread: lerpI(x.Spread, y.Spread, t),
			Color: lerpColor(x.Color, y.Color, t), Inset: x.Inset,
		}
	}
	return out, true
}
