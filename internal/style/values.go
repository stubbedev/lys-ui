package style

import (
	"math"
	"math/bits"
	"slices"

	"github.com/stubbedev/lys-lib/render"
)

// Prop names one longhand property of the docs/css.md table. Shorthands
// (padding, margin, border, border-radius, background, outline,
// transition, all) expand onto these at parse time; the cascade runs
// per longhand, exactly like CSS.
type Prop uint8

// The longhand properties, as bit indexes into PropSet.
const (
	PropColor Prop = iota
	PropBackgroundColor
	PropBackgroundImage
	PropOpacity
	PropFilter
	PropPaddingTop
	PropPaddingRight
	PropPaddingBottom
	PropPaddingLeft
	PropMarginTop
	PropMarginRight
	PropMarginBottom
	PropMarginLeft
	PropBorderTopWidth
	PropBorderRightWidth
	PropBorderBottomWidth
	PropBorderLeftWidth
	PropBorderTopStyle
	PropBorderRightStyle
	PropBorderBottomStyle
	PropBorderLeftStyle
	PropBorderTopColor
	PropBorderRightColor
	PropBorderBottomColor
	PropBorderLeftColor
	PropBorderTopLeftRadius
	PropBorderTopRightRadius
	PropBorderBottomRightRadius
	PropBorderBottomLeftRadius
	PropBoxShadow
	PropOutlineWidth
	PropOutlineStyle
	PropOutlineColor
	PropOutlineOffset
	PropMinWidth
	PropMinHeight
	PropBorderSpacing
	PropFontFamily
	PropFontSize
	PropFontWeight
	PropFontStyle
	PropLetterSpacing
	PropTextTransform
	PropLineHeight
	PropFontFeatures
	PropFontVariations
	PropTransform
	PropTransformOrigin
	PropIconSize
	PropTransitionProperty
	PropTransitionDuration
	PropTransitionTiming
	PropTransitionDelay
	PropAnimation
	PropAnimationPlayState
	PropAnimDuration
	PropAnimDelay
	PropAnimDirection
	PropAnimFill
	PropIconTransform
	PropIconSource
	PropIconPalette
	PropCaretColor
	PropTextDecorationLine
	PropCursor
	PropPointerEvents
	PropUserSelect
	PropCaretShape
	PropAccentColor
	PropAppearance
	PropTouchAction
	PropTextAlign
	PropTextIndent
	PropWhiteSpace
	PropWordBreak
	PropLineBreak
	PropHyphens
	PropWordSpacing
	PropVerticalAlign
	PropLineClamp
	PropTextOverflow
	PropFontVariantCaps
	PropFontVariantNumeric
	PropFontVariantLigatures
	PropFontVariantEastAsian
	PropFontVariantPosition
	PropFontVariantAlternates
	PropFontSynthesis
	PropFontOpticalSizing
	PropDirection
	PropUnicodeBidi
	PropWritingMode
	PropBackdropFilter
	PropMixBlendMode
	PropIsolation
	PropClipPath
	PropMaskImage
	PropMaskPosition
	PropMaskSize
	PropMaskRepeat
	PropMaskMode
	PropPerspective
	PropTransformStyle
	PropBackfaceVisibility
	PropLayoutMotion
	PropScrollBehavior
	PropScrollMarginTop
	PropScrollMarginRight
	PropScrollMarginBottom
	PropScrollMarginLeft
	PropScrollPaddingTop
	PropScrollPaddingRight
	PropScrollPaddingBottom
	PropScrollPaddingLeft
	PropScrollSnapType
	PropScrollSnapAlign
	PropScrollSnapStop
	PropOverscrollX
	PropOverscrollY
	PropScrollbarColor
	PropScrollbarWidth
	PropScrollbarGutter
	PropTransitionBehavior
	PropAnimComposition
	PropContainerType
	PropContainerName
	PropDisplay
	PropPosition
	PropTop
	PropRight
	PropBottom
	PropLeft
	PropZIndex
	PropWidth
	PropHeight
	PropMaxWidth
	PropMaxHeight
	PropBoxSizing
	PropAspectRatio
	PropFlexDirection
	PropFlexWrap
	PropJustifyContent
	PropAlignItems
	PropAlignSelf
	PropAlignContent
	PropFlexGrow
	PropFlexShrink
	PropFlexBasis
	PropOrder
	PropRowGap
	PropColumnGap
	PropOverflowX
	PropOverflowY
	PropVisibility
	PropFloat
	PropClear
	PropGridTemplateColumns
	PropGridTemplateRows
	PropGridTemplateAreas
	PropGridAutoColumns
	PropGridAutoRows
	PropGridAutoFlow
	PropGridColumnStart
	PropGridColumnEnd
	PropGridRowStart
	PropGridRowEnd
	PropJustifyItems
	PropJustifySelf
	PropObjectFit
	PropObjectPosition
	PropTextDecorationStyle
	PropTextDecorationColor
	PropTextShadow
	PropFontPalette
	PropOverflowWrap
	PropAnimTiming
	PropAnimIteration
	PropBackgroundSize
	PropBackgroundPosition
	PropBackgroundRepeat
	PropBackgroundClip
	PropBackgroundOrigin
	PropBackgroundAttachment
	PropBorderImageSource
	PropBorderImageSlice
	PropBorderImageWidth
	PropBorderImageOutset
	PropBorderImageRepeat
	numProps
)

// PropSet is a set of longhands, one bit per Prop over three words:
// separate fields rather than an array, so a set stays in registers
// through the cascade's per-longhand loops.
type PropSet struct{ lo, mid, hi uint64 }

// propWordBits is the props per set word.
const propWordBits = 64

// The longhand table must fit the three words; a table that outgrows
// them fails to compile here (a negative constant has no uint).
const _ = uint(3*propWordBits - int(numProps))

// bit returns p's bit within its word.
func (p Prop) bit() uint64 { return 1 << (p % propWordBits) }

// Has reports whether p is in the set.
func (s PropSet) Has(p Prop) bool {
	switch p / propWordBits {
	case 0:
		return s.lo&p.bit() != 0
	case 1:
		return s.mid&p.bit() != 0
	}
	return s.hi&p.bit() != 0
}

// Empty reports whether the set has no member.
func (s PropSet) Empty() bool { return s == PropSet{} }

// With returns the set plus p.
func (s PropSet) With(p Prop) PropSet {
	switch p / propWordBits {
	case 0:
		s.lo |= p.bit()
	case 1:
		s.mid |= p.bit()
	default:
		s.hi |= p.bit()
	}
	return s
}

// Without returns the set minus p.
func (s PropSet) Without(p Prop) PropSet {
	switch p / propWordBits {
	case 0:
		s.lo &^= p.bit()
	case 1:
		s.mid &^= p.bit()
	default:
		s.hi &^= p.bit()
	}
	return s
}

// Or returns the union of the sets.
func (s PropSet) Or(o PropSet) PropSet { return PropSet{s.lo | o.lo, s.mid | o.mid, s.hi | o.hi} }

// And returns the intersection of the sets.
func (s PropSet) And(o PropSet) PropSet { return PropSet{s.lo & o.lo, s.mid & o.mid, s.hi & o.hi} }

// AndNot is s minus o.
func (s PropSet) AndNot(o PropSet) PropSet {
	return PropSet{s.lo &^ o.lo, s.mid &^ o.mid, s.hi &^ o.hi}
}

// words is the set's bit words, low first.
func (s PropSet) words() [3]uint64 { return [3]uint64{s.lo, s.mid, s.hi} }

// Each calls fn for every member, in Prop order.
func (s PropSet) Each(fn func(Prop)) {
	for i, word := range s.words() {
		for word != 0 {
			fn(Prop(i*propWordBits + bits.TrailingZeros64(word)))
			word &= word - 1
		}
	}
}

// setOf builds a PropSet from its members.
func setOf(ps ...Prop) PropSet {
	var s PropSet
	for _, p := range ps {
		s = s.With(p)
	}
	return s
}

// SetOf builds a PropSet from its members; the exported form for the
// consumers outside the engine.
func SetOf(ps ...Prop) PropSet { return setOf(ps...) }

// allProps is every longhand: the reach of the `all` shorthand.
var allProps = func() (s PropSet) {
	for p := range numProps {
		s = s.With(p)
	}
	return s
}()

// inheritedProps is the CSS (and GTK) inherited set: color, the font
// group, letter-spacing, text-transform, line-height, -gtk-icon-size,
// the CSS Text group with text-shadow, direction, the interaction
// properties the spec inherits (cursor, pointer-events, caret-color,
// caret-shape, accent-color), scrollbar-color, and visibility. Custom
// properties inherit too; they live outside the longhand table.
var inheritedProps = setOf(PropColor, PropFontFamily, PropFontSize, PropFontWeight,
	PropFontStyle, PropLetterSpacing, PropTextTransform, PropLineHeight,
	PropFontFeatures, PropFontVariations, PropIconSize, PropCursor,
	PropTextAlign, PropTextIndent,
	PropWhiteSpace, PropWordBreak, PropLineBreak, PropHyphens, PropWordSpacing,
	PropOverflowWrap, PropTextShadow,
	PropFontVariantCaps, PropFontVariantNumeric, PropFontVariantLigatures,
	PropFontVariantEastAsian, PropFontVariantPosition, PropFontVariantAlternates,
	PropFontSynthesis, PropFontOpticalSizing, PropFontPalette, PropDirection, PropWritingMode,
	PropPointerEvents, PropCaretColor, PropCaretShape, PropAccentColor,
	PropScrollbarColor, PropVisibility)

// Sides is a per-side length set, in logical pixels, in CSS's
// top-right-bottom-left order.
type Sides struct {
	Top, Right, Bottom, Left int
}

// Uniform reports whether every side is v.
func (s Sides) Uniform(v int) bool { return s == Sides{v, v, v, v} }

// Horizontal is left + right.
func (s Sides) Horizontal() int { return s.Left + s.Right }

// Vertical is top + bottom.
func (s Sides) Vertical() int { return s.Top + s.Bottom }

// Add sums two side sets.
func (s Sides) Add(o Sides) Sides {
	return Sides{s.Top + o.Top, s.Right + o.Right, s.Bottom + o.Bottom, s.Left + o.Left}
}

// At reads side i by the transition engine's index: 0 top, 1 right,
// 2 bottom, 3 left.
func (s Sides) At(i int) int {
	return [4]int{s.Top, s.Right, s.Bottom, s.Left}[i]
}

// ptr addresses side i.
func (s *Sides) ptr(i int) *int { return [4]*int{&s.Top, &s.Right, &s.Bottom, &s.Left}[i] }

// Corners is a per-corner radius set, clockwise from top-left.
type Corners struct {
	TopLeft, TopRight, BottomRight, BottomLeft int
}

// At reads corner i, the same clockwise order as Sides.
func (c Corners) At(i int) int {
	return [4]int{c.TopLeft, c.TopRight, c.BottomRight, c.BottomLeft}[i]
}

// ptr addresses corner i.
func (c *Corners) ptr(i int) *int {
	return [4]*int{&c.TopLeft, &c.TopRight, &c.BottomRight, &c.BottomLeft}[i]
}

// BorderStyle is a border or outline line style. The subset draws solid
// lines; every other visible style strokes solid, and none/hidden
// zero the width, per the CSS computed-value rule.
type BorderStyle uint8

// Border styles.
const (
	BorderNone BorderStyle = iota
	BorderHidden
	BorderSolid
	BorderDashed
	BorderDotted
	BorderDouble
	BorderGroove
	BorderRidge
	BorderInset
	BorderOutset
	// BorderAuto is outline-style: auto, the platform focus ring; no
	// border takes it.
	BorderAuto
)

// Visible reports whether the style draws a line.
func (b BorderStyle) Visible() bool { return b != BorderNone && b != BorderHidden }

// Shadow is one box-shadow layer.
type Shadow struct {
	X, Y, Blur, Spread int
	Color              render.Color
	Inset              bool
}

// MaxShadows bounds the box-shadow list; later layers past it are
// dropped with a warning. Four covers every stacked-elevation idiom.
const MaxShadows = 4

// Shadows is a fixed-capacity box-shadow list, comparable so a restyle
// can diff it.
type Shadows struct {
	N      int
	Layers [MaxShadows]Shadow
}

// List returns the active layers, front (first declared) first.
func (s *Shadows) List() []Shadow { return s.Layers[:s.N] }

// MaxStops bounds a gradient's color stops and color hints; a
// double-position stop counts twice.
const MaxStops = 16

// hintSteps is how many linear segments draw a color hint's curve:
// close to a count of the exact power curve per channel.
const hintSteps = 8

// MaxResolvedStops bounds Gradient.Resolve's output: a hint expands
// into hintSteps-1 stops.
const MaxResolvedStops = MaxStops * hintSteps

// GradientStop is one color stop or color hint. At a stop's position
// is Pos of the gradient line plus Px pixels (a <length-percentage>,
// resolved against the line by Gradient.Resolve); without At the
// position is auto. A Hint carries no color: it moves the midpoint of
// the interpolation between its neighbor stops.
type GradientStop struct {
	Pos, Px  float64
	At, Hint bool
	Color    render.Color
}

// Gradient is a gradient background image - linear, radial, or conic,
// optionally repeating - in render's geometry terms. N zero means none.
// Angle is in degrees, CSS convention: 0 points up, 90 right (a conic
// gradient's start). CenterX/Y place a radial or conic center as box
// fractions; RadiusX/Y are explicit radial radii in px.
type Gradient struct {
	Kind             render.GradientKind
	Repeat           bool
	Angle            float64
	CenterX, CenterY float64
	Circle           bool
	Size             render.RadialSize
	RadiusX, RadiusY float64
	N                int
	Stops            [MaxStops]GradientStop
}

// Resolve lays g's stops on a gradient line line pixels long: CSS
// Images 3's color-stop fixup (an auto first stop at 0%, an auto last
// at 100%, every position at least its predecessors', runs of auto
// stops spread evenly between their neighbors), then each color hint
// drawn as a power curve of hintSteps segments between its stops. The
// result appends to dst, which should hold MaxResolvedStops.
func (g *Gradient) Resolve(line float64, dst []render.GradientStop) []render.GradientStop {
	var buf [MaxStops]float64
	n := g.N
	pos := buf[:n]
	for i := range n {
		s := &g.Stops[i]
		pos[i] = math.NaN()
		if s.At {
			pos[i] = s.Pos
			if line > 0 {
				pos[i] += s.Px / line
			}
		}
	}
	if math.IsNaN(pos[0]) {
		pos[0] = 0
	}
	if last := &pos[len(pos)-1]; math.IsNaN(*last) {
		*last = 1
	}
	for i := 1; i < n; i++ {
		if !math.IsNaN(pos[i]) {
			pos[i] = math.Max(pos[i], pos[i-1])
			continue
		}
		j := i
		for math.IsNaN(pos[j]) {
			j++
		}
		step := (pos[j] - pos[i-1]) / float64(j-i+1)
		for k := i; k < j; k++ {
			pos[k] = pos[k-1] + step
		}
	}
	for i, at := range pos {
		s := &g.Stops[i]
		if !s.Hint || i == 0 || i == len(pos)-1 {
			dst = append(dst, render.GradientStop{Pos: at, Color: s.Color})
			continue
		}
		c0, c1 := g.Stops[i-1].Color, g.Stops[i+1].Color
		before, after := pos[:i], pos[i+1:]
		p0, p1 := before[len(before)-1], after[0]
		h := 0.5
		if p1 > p0 {
			h = (pos[i] - p0) / (p1 - p0)
		}
		for k := 1; k < hintSteps; k++ {
			p := float64(k) / hintSteps
			dst = append(dst, render.GradientStop{Pos: p0 + p*(p1-p0), Color: mixPremul(c0, c1, 1-hintCurve(p, h), 1)})
		}
	}
	return dst
}

// hintCurve is how far toward the next stop's color the interpolation
// has moved at fraction p of the way when the hint sits at fraction h
// (CSS Images 3 §3.5.3): p^(ln 0.5 / ln h), with the degenerate hints
// snapping to either color.
func hintCurve(p, h float64) float64 {
	switch {
	case h <= 0:
		return 1
	case h >= 1:
		return 0
	}
	return math.Pow(p, math.Log(0.5)/math.Log(h))
}

// TextTransform is the text-transform keyword.
type TextTransform uint8

// Text transforms.
const (
	TransformNone TextTransform = iota
	TransformUppercase
	TransformLowercase
	TransformCapitalize
)

// TextAlign is the text-align keyword.
type TextAlign uint8

// Text alignments. Start and End resolve against the resolved base
// direction; Left and Right are absolute; Justify distributes a
// wrapped row's leftover width across its spaces.
const (
	AlignStart TextAlign = iota
	AlignEnd
	AlignLeft
	AlignRight
	AlignCenter
	AlignJustify
)

// WhiteSpace is the white-space keyword.
type WhiteSpace uint8

// White-space keywords.
const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpacePre
	WhiteSpaceNowrap
	WhiteSpacePreWrap
	WhiteSpacePreLine
	WhiteSpaceBreakSpaces
)

// WordBreak is the word-break keyword.
type WordBreak uint8

// Word-break keywords.
const (
	WordBreakNormal WordBreak = iota
	WordBreakBreakAll
	WordBreakKeepAll
)

// LineBreakK is the line-break keyword (K for the line-break clash
// with the render-side breaker).
type LineBreakK uint8

// Line-break keywords.
const (
	LineBreakAuto LineBreakK = iota
	LineBreakLoose
	LineBreakNormal
	LineBreakStrict
	LineBreakAnywhere
)

// HyphensK is the hyphens keyword.
type HyphensK uint8

// Hyphens keywords.
const (
	HyphensNone HyphensK = iota
	HyphensManual
	HyphensAuto
)

// OverflowWrapK is the overflow-wrap keyword (and its word-wrap
// alias).
type OverflowWrapK uint8

// Overflow-wrap keywords: break-word and anywhere both break an
// otherwise unbreakable word that overflows its line; only anywhere's
// opportunities count toward the min-content size.
const (
	OverflowWrapNormal OverflowWrapK = iota
	OverflowWrapBreakWord
	OverflowWrapAnywhere
)

// TextOverflowK is the text-overflow keyword.
type TextOverflowK uint8

// Text-overflow keywords.
const (
	TextOverflowClip TextOverflowK = iota
	TextOverflowEllipsis
)

// VerticalAlignK is the vertical-align alignment kind.
type VerticalAlignK uint8

// Vertical-align kinds. Length is VerticalAlignLength with the shift
// in Values.VerticalAlignPx.
const (
	VerticalAlignBaseline VerticalAlignK = iota
	VerticalAlignSub
	VerticalAlignSuper
	VerticalAlignTextTop
	VerticalAlignTextBottom
	VerticalAlignTop
	VerticalAlignMiddle
	VerticalAlignBottom
	VerticalAlignLength
)

// FontVariantCaps is the font-variant-caps keyword.
type FontVariantCaps uint8

// font-variant-caps keywords.
const (
	CapsNormal FontVariantCaps = iota
	CapsSmallCaps
	CapsAllSmallCaps
	CapsPetiteCaps
	CapsAllPetiteCaps
	CapsUnicase
	CapsTitlingCaps
)

// FontVariantNumeric is font-variant-numeric's flag set.
type FontVariantNumeric uint16

// font-variant-numeric flags.
const (
	NumLining FontVariantNumeric = 1 << iota
	NumOldStyle
	NumProportional
	NumTabular
	NumDiagonalFractions
	NumStackedFractions
	NumOrdinal
	NumSlashedZero
)

// FontVariantLigatures is font-variant-ligatures' flag set: each
// group's on and off keyword, neither meaning the font's default.
type FontVariantLigatures uint16

// font-variant-ligatures flags.
const (
	LigCommon FontVariantLigatures = 1 << iota
	LigNoCommon
	LigDiscretionary
	LigNoDiscretionary
	LigHistorical
	LigNoHistorical
	LigContextual
	LigNoContextual
)

// FontVariantEastAsian is font-variant-east-asian's flag set.
type FontVariantEastAsian uint16

// font-variant-east-asian flags.
const (
	EAJis78 FontVariantEastAsian = 1 << iota
	EAJis83
	EAJis90
	EAJis04
	EASimplified
	EATraditional
	EAFullWidth
	EAProportional
	EARuby
)

// FontVariantPosition is the font-variant-position keyword.
type FontVariantPosition uint8

// font-variant-position keywords.
const (
	PositionNormal FontVariantPosition = iota
	PositionSub
	PositionSuper
)

// FontVariantAlternates is the font-variant-alternates keyword.
type FontVariantAlternates uint8

// font-variant-alternates keywords.
const (
	AlternatesNormal FontVariantAlternates = iota
	AlternatesHistorical
)

// FontSynthesis is font-synthesis' flag set: which face qualities the
// engine may synthesize when the chosen face lacks them.
type FontSynthesis uint8

// font-synthesis flags.
const (
	SynthWeight FontSynthesis = 1 << iota
	SynthStyle
	SynthSmallCaps
)

// FontPalette is the font-palette keyword.
type FontPalette uint8

// font-palette keywords.
const (
	PaletteNormal FontPalette = iota
	PaletteLight
	PaletteDark
)

// UnicodeBidi is the unicode-bidi keyword.
type UnicodeBidi uint8

// unicode-bidi keywords.
const (
	BidiNormal UnicodeBidi = iota
	BidiEmbed
	BidiIsolate
	BidiOverride
	BidiIsolateOverride
	BidiPlaintext
)

// WritingMode is the writing-mode keyword.
type WritingMode uint8

// writing-mode keywords.
const (
	WritingHorizontalTB WritingMode = iota
	WritingVerticalRL
	WritingVerticalLR
)

// Values is the cascade result for one node: the winning declaration
// per longhand, computed (var() substituted, calc() evaluated, units
// resolved to pixels), with Set recording which longhands the
// stylesheet provided, directly or through inheritance. Consumers layer
// programmatic widget values above it and the theme under it. The
// comparable values sit in the embedded Scalars, so a restyle diffs
// most of a node's style with one ==; the list- and pointer-valued
// groups beside it compare through Equal.
type Values struct {
	Scalars
	// Bg is the background-* longhands' layer lists.
	Bg Background
	// Clip is the computed clip-path; a zero Kind clips nothing.
	Clip ClipShape
	// Mask is the computed mask-image group (position, size, repeat,
	// mode).
	Mask Mask
	// BorderImage is the border-image group; UsedBorderImage reads it
	// with the undeclared longhands at their initial values.
	BorderImage BorderImage
	// Transform carries the transform property: the composed affine
	// the widget paints its subtree through, with the primitive list
	// the tweens interpolate. The identity means unset; the parser
	// always writes the full Xform, so a declared `none` reads as it.
	Transform Xform
	// Transition is the transition-* longhands' lists.
	Transition Transition
	// Starting is the node's computed @starting-style block, nil when
	// no matched rule carries one: the values a first style's
	// transitions start from (the entry transition). Only the
	// properties its rules declare are meaningful.
	Starting *Values
	// Anim is the animation-* longhands' lists as declared, and
	// Animation the animations Compute zips from them (css-animations-1
	// §3): one per name that names a @keyframes rule. Neither inherits.
	Anim      AnimLists
	Animation []Animation
	// IconXform is -gtk-icon-transform's composed transform; icons
	// draw through it.
	IconXform Xform
	// Counters is the node's counter environment (counter-reset,
	// counter-increment): nil when no sheet counts anything.
	Counters *CounterScope
}

// Scalars are the comparable computed values of a node.
type Scalars struct {
	Set PropSet
	// Own are the properties the node's own rules declared, before
	// inheritance filled in the rest of Set: a part that styles only
	// what a stylesheet names for it (an entry's placeholder color)
	// reads these.
	Own PropSet

	Color      render.Color
	Background render.Color
	Opacity    float64
	Brightness float64

	// Filter is the computed filter list; Brightness stays the
	// brightness-only fast path's product, kept in sync by the
	// parser so the tween engines keep their channel.
	Filter render.Filters
	// Backdrop is the computed backdrop-filter list, applied to what
	// paints behind the widget rather than the widget itself.
	Backdrop render.Filters
	// Blend is the computed mix-blend-mode; BlendNormal composites as
	// plain source-over.
	Blend render.BlendMode
	// Isolate is isolation: isolate — the subtree becomes its own
	// compositing group.
	Isolate bool
	// Perspective is the perspective distance in px; zero is none.
	Perspective float64
	// Preserve3D is transform-style: preserve-3d; flat otherwise. The
	// painter flattens either way today — see docs/css.md.
	Preserve3D bool
	// BackfaceHidden is backface-visibility: hidden.
	BackfaceHidden bool
	// LayoutMotion is lys-ui's -lys-layout-motion: the spring a change
	// of the box's laid-out rect animates on, none by default.
	LayoutMotion LayoutMotion

	Padding     Sides
	Margin      Sides
	BorderWidth Sides
	BorderStyle [4]BorderStyle // top, right, bottom, left
	BorderColor [4]render.Color
	Radius      Corners
	Shadow      Shadows

	OutlineWidth  int
	OutlineStyle  BorderStyle
	OutlineColor  render.Color
	OutlineOffset int
	// OutlineAuto is outline-color: auto (the initial value): the
	// focus ring's accent under outline-style auto, currentColor
	// otherwise.
	OutlineAuto bool

	// MarginAuto marks the margin sides declared auto (top, right,
	// bottom, left): Margin holds zero there, and the CSS layout
	// engines hand those sides the free space.
	MarginAuto [4]bool

	// Layout is the CSS layout group: display, the box sizes,
	// positioning, flexbox, grid, overflow, visibility and float.
	Layout Layout
	// BorderSpacing is GTK's border-spacing: the gap a box leaves between
	// its children, horizontal and vertical.
	BorderSpacingH, BorderSpacingV int

	FontFamily    string
	FontSize      float64
	FontWeight    int
	Italic        bool
	LetterSpacing float64
	TextTransform TextTransform
	// LineHeight is the computed line-height in pixels; 0 is `normal`,
	// the font's own line box.
	LineHeight float64
	// LineHeightFactor is the unitless number line-height was given,
	// 0 for normal, a length or a percentage: a number inherits as
	// itself, so each element resolves it against its own font-size.
	LineHeightFactor float64
	// Features is the font-feature-settings tag list in canonical form
	// (`on=1;ss01=0;tnum=1`, sorted); empty is `normal`. The text
	// shaper honors the tags it knows (tnum today).
	Features string
	// Variations is the font-variation-settings axis list in canonical
	// form (`wdth=80.5;wght=650`, sorted); empty is `normal`.
	Variations string
	// TransformOrigin is the transform's pivot: fraction-of-box plus
	// pixel offset per axis; the default is the box's center.
	OriginFrac, OriginPx [2]float64
	IconSize             int

	// IconSource is -gtk-icon-source's -gtk-icontheme() name: a themed
	// symbolic icon a widget draws instead of its painted mark (the
	// checkbutton's tick).
	IconSource string
	// PaletteTint is -gtk-icon-palette's recolor: the color the source
	// icon takes (the stylesheet writes `success <color>`).
	PaletteTint render.Color
	// CaretColor is caret-color: an entry's caret takes it over the
	// text color.
	CaretColor render.Color
	// Decoration is the text-decoration group: text-decoration-line,
	// -style and -color, each its own longhand.
	Decoration render.Decoration
	// TextShadow is the text-shadow list (spread and inset are always
	// zero); it inherits.
	TextShadow Shadows
	// Cursor is cursor: the hover shape a stylesheet asks for, a
	// keyword or a url() file cursor with its hotspot. The zero value
	// is auto; it inherits.
	Cursor CursorSpec
	// PointerEvents is pointer-events: none takes the widget out of
	// hit testing.
	PointerEvents PointerEvents
	// UserSelect is user-select on the text surfaces that select.
	UserSelect UserSelect
	// CaretShape is caret-shape: the caret's painted form.
	CaretShape CaretShape
	// AccentColor is accent-color over the theme accent; AccentAuto
	// marks the auto keyword falling back to the theme.
	AccentColor render.Color
	AccentAuto  bool
	// Appearance is appearance: parsed only, none's chrome stripping
	// is deferred (docs/css.md).
	Appearance Appearance
	// TouchAction is touch-action: the pan axes a touch scroll may
	// move.
	TouchAction TouchAction

	// The scroll-container group: scroll-behavior gates animated
	// programmatic scrolls, scroll-padding insets the viewport for
	// reveal and snap math, scroll-snap-type turns snapping on,
	// overscroll-behavior gates chaining to the parent scroll, and
	// scrollbar-gutter reserves the bar's space in measure. The
	// scrollbar-color/width pair is inherited and reaches the bars.
	ScrollSmooth bool
	ScrollPad    Sides
	Snap         SnapType
	Overscroll   [2]Overscroll
	BarThumb     render.Color
	BarTrack     render.Color
	// BarColorAuto marks scrollbar-color: auto (the default look).
	BarColorAuto bool
	BarWidth     uint8
	BarGutter    uint8

	// The scrolled-child group: scroll-margin insets the revealed rect,
	// scroll-snap-align names the child a snap position, and
	// scroll-snap-stop makes one the gesture cannot skip past.
	ScrollMargin   Sides
	SnapAlign      SnapAlign
	SnapStopAlways bool

	// The CSS Text 3 properties. TextAlign, TextIndent, WhiteSpace,
	// WordBreak, LineBreak, Hyphens, WordSpacing and the font-variant
	// family inherit; VerticalAlign, LineClamp, TextOverflow,
	// UnicodeBidi and FontSynthesis do not.
	TextAlign       TextAlign
	TextIndent      float64
	WhiteSpace      WhiteSpace
	WordBreak       WordBreak
	LineBreak       LineBreakK
	Hyphens         HyphensK
	WordSpacing     float64
	VerticalAlign   VerticalAlignK
	VerticalAlignPx float64
	// VerticalAlignPct marks VerticalAlignPx a fraction of the line
	// box instead of pixels.
	VerticalAlignPct bool
	LineClamp        int
	Overflow         TextOverflowK
	VariantCaps      FontVariantCaps
	VariantNumeric   FontVariantNumeric
	VariantLig       FontVariantLigatures
	VariantEA        FontVariantEastAsian
	VariantPos       FontVariantPosition
	VariantAlt       FontVariantAlternates
	Synthesis        FontSynthesis
	OpticalSizing    bool
	// Palette is font-palette; it inherits.
	Palette FontPalette
	// OverflowWrap is overflow-wrap; it inherits.
	OverflowWrap OverflowWrapK
	// Direction is the CSS direction: 1 ltr, 2 rtl (0 cannot be set -
	// direction takes no auto keyword).
	Direction   uint8
	UnicodeBidi UnicodeBidi
	WritingMode WritingMode

	// ContainerType is container-type (container queries): normal
	// (the default) names no query container, inline-size one whose
	// inline axis its descendants query, size one whose both axes do.
	ContainerType ContainerType
	// ContainerNames is container-name: the space-separated custom-ident
	// list a @container name filter matches against.
	ContainerNames string

	// Content is a pseudo-element's resolved content (::before,
	// ::after): the strings, attr() values, and counter readings joined;
	// HasContent reports a content declaration won at all.
	Content    string
	HasContent bool
	// ContentURL is content: url(...): an image painted as the
	// pseudo-element's generated content, beside Content's text.
	ContentURL string
	// QuoteDepth is the open-quote nesting depth after this node's
	// content resolved: descendants' open-quote/close-quote start from
	// it, CSS's inherited quote counter.
	QuoteDepth int

	// Vars is the node's custom-property environment: its own
	// declarations over the inherited chain. Nodes that declare none
	// share their parent's pointer, so equality stays cheap.
	Vars *Vars
}

// Equal reports whether two computed styles are the same: the
// comparable values with one ==, the groups beside them through their
// own comparisons. A restyle that leaves a node Equal changes nothing.
func (v *Values) Equal(o *Values) bool {
	return v.Scalars == o.Scalars && v.Bg.Equal(&o.Bg) && v.Clip.Equal(&o.Clip) &&
		v.Mask.Equal(&o.Mask) && v.BorderImage.Equal(&o.BorderImage) &&
		v.Transform.Equal(&o.Transform) && v.IconXform.Equal(&o.IconXform) &&
		v.Transition.Equal(&o.Transition) && v.Anim.Equal(&o.Anim) &&
		slices.Equal(v.Animation, o.Animation) && v.Counters.Equal(o.Counters)
}

// Has reports whether the property was set for this node.
func (v *Values) Has(p Prop) bool { return v.Set.Has(p) }

// Origin is the transform-origin pivot: fractions of the box plus
// pixel offsets per axis, the box's center when nothing declares it.
func (v *Values) Origin() (frac, px [2]float64) {
	if !v.Has(PropTransformOrigin) {
		return [2]float64{0.5, 0.5}, [2]float64{}
	}
	return v.OriginFrac, v.OriginPx
}

// Declares reports whether the node's own rules set p, not its
// inheritance.
func (v *Values) Declares(p Prop) bool { return v.Own.Has(p) }

// HasAny reports whether any of the properties was set.
func (v *Values) HasAny(ps ...Prop) bool {
	return slices.ContainsFunc(ps, v.Set.Has)
}

// EffBorder returns the used border widths: a side whose style draws no
// line has zero width, the CSS computed-value rule.
func (v *Values) EffBorder() Sides {
	b := v.BorderWidth
	if !v.BorderStyle[0].Visible() {
		b.Top = 0
	}
	if !v.BorderStyle[1].Visible() {
		b.Right = 0
	}
	if !v.BorderStyle[2].Visible() {
		b.Bottom = 0
	}
	if !v.BorderStyle[3].Visible() {
		b.Left = 0
	}
	return b
}

// EffOutline returns the used outline width: zero unless the style
// draws a line.
func (v *Values) EffOutline() int {
	if !v.OutlineStyle.Visible() {
		return 0
	}
	return v.OutlineWidth
}

// Var returns the computed value of the custom property name (with its
// leading dashes) as source text, and whether it is defined.
func (v *Values) Var(name string) (string, bool) {
	ts, ok := v.Vars.lookup(name)
	if !ok {
		return "", false
	}
	return tokensText(ts), true
}

// initialValues returns each longhand's initial value: what `initial`,
// `unset` on a non-inherited property, and `all: unset` compute to.
// Zero would be a visible value for opacity and brightness, and a zero
// scale paints nothing, so those carry their identities; the transform
// origin defaults to the box's center.
func initialValues() Values {
	return Values{
		Opacity:    1,
		Brightness: 1,
		FontWeight: 400,
		Transform:  XformIdentity,
		IconXform:  XformIdentity,
		OriginFrac: [2]float64{0.5, 0.5},
	}
}

// Vars is one node's custom-property environment: its own computed
// declarations, falling back to the parent's.
type Vars struct {
	parent *Vars
	own    map[string][]token
}

// lookup finds name, own declarations first, then up the chain.
func (vs *Vars) lookup(name string) ([]token, bool) {
	for ; vs != nil; vs = vs.parent {
		if ts, ok := vs.own[name]; ok {
			return ts, ts != nil
		}
	}
	return nil, false
}

// equalOwn reports whether two environments hold the same own
// declarations over the same parent, so a recompute can keep the old
// pointer.
func (vs *Vars) equalOwn(o *Vars) bool {
	if vs == nil || o == nil || vs.parent != o.parent || len(vs.own) != len(o.own) {
		return false
	}
	for k, a := range vs.own {
		b, ok := o.own[k]
		if !ok || !tokensEqual(a, b) {
			return false
		}
	}
	return true
}

// tokensEqual compares two token lists by content, ignoring positions.
func tokensEqual(a, b []token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].kind != b[i].kind || a[i].s != b[i].s || a[i].num != b[i].num {
			return false
		}
	}
	return true
}

// LayoutMotion is a -lys-layout-motion value: how a box animates a
// change of its laid-out rect - not at all, or on one of the named
// springs.
type LayoutMotion uint8

// The layout motions.
const (
	LayoutMotionNone LayoutMotion = iota
	LayoutMotionSmooth
	LayoutMotionSnappy
	LayoutMotionBouncy
)
