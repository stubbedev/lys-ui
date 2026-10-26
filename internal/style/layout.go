package style

// The CSS layout properties (docs/css.md): display, the box sizes and
// box-sizing, aspect-ratio, positioning and z-index, the flexbox group
// with gap, overflow, visibility, and float/clear, and the grid group
// (grid.go). They compute here; the widget layer's layout engines read
// the Layout group. Lengths that may carry a percentage keep it
// unresolved (Length), since only the layout knows the containing
// block.

import (
	"math"
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// Length is a computed <length-percentage>: Px logical pixels plus Pct
// percent of the basis the layout resolves it against. A calc() sum
// mixes the two (calc(100% - 2em)); a plain length leaves Pct zero and
// a plain percentage Px.
type Length struct {
	Px  int
	Pct float64
}

// Resolve returns the length against basis, rounded to whole pixels.
func (l Length) Resolve(basis int) int { return l.Px + px(l.Pct*float64(basis)/100) }

// Relative reports whether the length depends on its basis.
func (l Length) Relative() bool { return l.Pct != 0 }

// Negative reports whether the length is negative whatever its basis:
// what a property taking no negative sizes rejects. A mix negative only
// for some bases (calc(100% - 20px)) is valid, and the layout clamps
// its used value at zero, as CSS does.
func (l Length) Negative() bool { return l.Px <= 0 && l.Pct <= 0 && l != Length{} }

// SizingKind is the form of a sizing value.
type SizingKind uint8

// Sizing kinds. Auto is the zero value: width/height/min-*/flex-basis
// auto, and max-* none. FitContent carries its limit in Len; the bare
// keyword is fit-content(100%), the available space.
const (
	SizingAuto SizingKind = iota
	SizingLength
	SizingMinContent
	SizingMaxContent
	SizingFitContent
	SizingContent
)

// Sizing is one computed sizing value: width, height, their min-* and
// max-* bounds, flex-basis, and the top/right/bottom/left insets (auto
// or a length there).
type Sizing struct {
	Kind SizingKind
	Len  Length
}

// Px returns the value as plain pixels: the length of a basis-free
// SizingLength, zero for everything else. The GTK-style consumers that
// read min-width as a pixel floor (a switch knob, a slider) take it.
func (s Sizing) Px() int {
	if s.Kind != SizingLength || s.Len.Relative() {
		return 0
	}
	return s.Len.Px
}

// PxSizing is the sizing value of a plain pixel length.
func PxSizing(px int) Sizing { return Sizing{Kind: SizingLength, Len: Length{Px: px}} }

// Display is the display keyword. The two-keyword forms (block flow,
// inline flex, inline flow-root, ...) compute to these.
type Display uint8

// display keywords. Block is the zero value; whether the stylesheet
// declared it is the Set bit's business.
const (
	DisplayBlock Display = iota
	DisplayInline
	DisplayInlineBlock
	DisplayFlowRoot
	DisplayFlex
	DisplayInlineFlex
	DisplayGrid
	DisplayInlineGrid
	DisplayContents
	DisplayNone
)

// Inline reports whether the display is inline-level: the box sits on
// a line box beside its inline siblings.
func (d Display) Inline() bool {
	return d == DisplayInline || d == DisplayInlineBlock || d == DisplayInlineFlex || d == DisplayInlineGrid
}

// Position is the position keyword.
type Position uint8

// position keywords.
const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

// OutOfFlow reports whether the box leaves its container's flow.
func (p Position) OutOfFlow() bool { return p == PositionAbsolute || p == PositionFixed }

// BoxSizing is the box-sizing keyword.
type BoxSizing uint8

// box-sizing keywords.
const (
	BoxSizingContent BoxSizing = iota
	BoxSizingBorder
)

// FlexDirection is the flex-direction keyword.
type FlexDirection uint8

// flex-direction keywords.
const (
	FlexRow FlexDirection = iota
	FlexRowReverse
	FlexColumn
	FlexColumnReverse
)

// FlexWrap is the flex-wrap keyword.
type FlexWrap uint8

// flex-wrap keywords.
const (
	FlexNowrap FlexWrap = iota
	FlexWrapOn
	FlexWrapReverse
)

// BoxAlignKind is one CSS Box Alignment keyword, shared by
// justify-content, align-items, align-self and align-content.
type BoxAlignKind uint8

// Alignment keywords. Normal is the initial value of every one of the
// properties but align-self, whose initial Auto defers to the
// container's align-items.
const (
	BoxNormal BoxAlignKind = iota
	BoxAuto
	BoxStretch
	BoxStart
	BoxEnd
	BoxFlexStart
	BoxFlexEnd
	BoxSelfStart
	BoxSelfEnd
	BoxCenter
	BoxLeft
	BoxRight
	BoxBaseline
	BoxLastBaseline
	BoxSpaceBetween
	BoxSpaceAround
	BoxSpaceEvenly
)

// BoxAlign is one computed alignment: the keyword and its overflow
// mode, Safe falling back to start when the subject overflows the
// alignment container.
type BoxAlign struct {
	Kind BoxAlignKind
	Safe bool
}

// Overflow is an overflow-x/overflow-y keyword.
type Overflow uint8

// overflow keywords.
const (
	OverflowVisible Overflow = iota
	OverflowHidden
	OverflowClip
	OverflowScroll
	OverflowAuto
)

// Clips reports whether the box clips its content on that axis.
func (o Overflow) Clips() bool { return o != OverflowVisible }

// Visibility is the visibility keyword.
type Visibility uint8

// visibility keywords.
const (
	VisibilityVisible Visibility = iota
	VisibilityHidden
	VisibilityCollapse
)

// Float is the float keyword; the logical sides resolve against the
// direction at layout time.
type Float uint8

// float keywords.
const (
	FloatNone Float = iota
	FloatLeft
	FloatRight
	FloatInlineStart
	FloatInlineEnd
)

// Clear is the clear keyword.
type Clear uint8

// clear keywords.
const (
	ClearNone Clear = iota
	ClearLeft
	ClearRight
	ClearBoth
	ClearInlineStart
	ClearInlineEnd
)

// Layout is the computed CSS layout group, comparable so a restyle
// diffs it with == (the grid's interned lists compare by value that
// way). Each field's longhand Set bit says whether the
// stylesheet declared it: an undeclared display, flex-direction,
// flex-grow, align-self or gap leaves the widget's programmatic layout
// in charge (docs/css.md's interaction rule).
type Layout struct {
	Display  Display
	Position Position
	// Inset is top, right, bottom, left: auto or a length.
	Inset [4]Sizing
	// ZIndex is z-index; auto computes to 0, the stacking order a
	// container's children share.
	ZIndex int

	Width, Height       Sizing
	MinWidth, MinHeight Sizing
	MaxWidth, MaxHeight Sizing
	BoxSizing           BoxSizing
	// AspectRatio is width over height; zero is auto.
	AspectRatio float64

	FlexDirection  FlexDirection
	FlexWrap       FlexWrap
	JustifyContent BoxAlign
	AlignItems     BoxAlign
	AlignSelf      BoxAlign
	AlignContent   BoxAlign
	FlexGrow       float64
	FlexShrink     float64
	FlexBasis      Sizing
	Order          int
	// RowGap and ColumnGap are the gaps between a flex container's
	// lines and its items; normal computes to zero.
	RowGap, ColumnGap Length

	OverflowX, OverflowY Overflow
	Visibility           Visibility
	Float                Float
	Clear                Clear

	// The grid container's explicit grid (grid.go): its track lists and
	// area map, nil for none, the implicit tracks' sizes, nil for auto,
	// and the auto-placement flow.
	GridTemplateColumns, GridTemplateRows *TrackList
	GridTemplateAreas                     *GridAreas
	GridAutoColumns, GridAutoRows         *AutoTracks
	GridAutoFlow                          GridAutoFlow
	// A grid item's placement lines and its inline-axis alignment;
	// JustifyItems is the container's default for its items'
	// justify-self: auto.
	GridColumnStart, GridColumnEnd GridLine
	GridRowStart, GridRowEnd       GridLine
	JustifyItems, JustifySelf      BoxAlign

	// ObjectFit is object-fit as the render scale policy implementing
	// it; ObjectPosition is object-position, horizontal then vertical
	// (objectfit.go).
	ObjectFit      render.ImageScale
	ObjectPosition [2]AxisPos
}

// LayoutProps are the longhands of the Layout group.
var LayoutProps = setOf(PropDisplay, PropPosition, PropTop, PropRight, PropBottom, PropLeft,
	PropZIndex, PropWidth, PropHeight, PropMinWidth, PropMinHeight, PropMaxWidth, PropMaxHeight,
	PropBoxSizing, PropAspectRatio, PropFlexDirection, PropFlexWrap, PropJustifyContent,
	PropAlignItems, PropAlignSelf, PropAlignContent, PropFlexGrow, PropFlexShrink, PropFlexBasis,
	PropOrder, PropRowGap, PropColumnGap, PropOverflowX, PropOverflowY, PropVisibility, PropFloat,
	PropClear).Or(gridContainerProps).Or(setOf(PropGridColumnStart, PropGridColumnEnd,
	PropGridRowStart, PropGridRowEnd, PropJustifyItems, PropJustifySelf, PropObjectFit, PropObjectPosition))

// layoutWideProps are the layout longhands a CSS-wide keyword returns
// to undeclared rather than to a CSS initial value: initial, unset and
// `all` give the widget back its programmatic layout, which is what a
// GTK-style `* { all: unset; }` reset expects of a toolkit whose
// containers lay out without CSS. visibility inherits, and min-width
// and min-height keep their GTK reading of `all: unset` as a zero
// floor.
var layoutWideProps = LayoutProps.AndNot(setOf(PropVisibility, PropMinWidth, PropMinHeight))

// textWideProps are the text longhands whose initial value is likewise
// a widget's own setting (Label.SetEllipsize, SetMaxLines; GTK keeps
// both off CSS): a CSS-wide keyword returns them to undeclared, so a
// `* { all: unset; }` reset keeps a label's ellipsizing.
var textWideProps = setOf(PropTextOverflow, PropLineClamp)

// copyLayoutProp copies the layout longhand p from src to dst; any
// other longhand is copyProp's.
func copyLayoutProp(dst, src *Layout, p Prop) {
	switch p {
	case PropDisplay:
		dst.Display = src.Display
	case PropPosition:
		dst.Position = src.Position
	case PropTop, PropRight, PropBottom, PropLeft:
		i := p - PropTop
		dst.Inset[i] = src.Inset[i]
	case PropZIndex:
		dst.ZIndex = src.ZIndex
	case PropWidth:
		dst.Width = src.Width
	case PropHeight:
		dst.Height = src.Height
	case PropMinWidth:
		dst.MinWidth = src.MinWidth
	case PropMinHeight:
		dst.MinHeight = src.MinHeight
	case PropMaxWidth:
		dst.MaxWidth = src.MaxWidth
	case PropMaxHeight:
		dst.MaxHeight = src.MaxHeight
	case PropBoxSizing:
		dst.BoxSizing = src.BoxSizing
	case PropAspectRatio:
		dst.AspectRatio = src.AspectRatio
	case PropFlexDirection:
		dst.FlexDirection = src.FlexDirection
	case PropFlexWrap:
		dst.FlexWrap = src.FlexWrap
	case PropJustifyContent:
		dst.JustifyContent = src.JustifyContent
	case PropAlignItems:
		dst.AlignItems = src.AlignItems
	case PropAlignSelf:
		dst.AlignSelf = src.AlignSelf
	case PropAlignContent:
		dst.AlignContent = src.AlignContent
	case PropFlexGrow:
		dst.FlexGrow = src.FlexGrow
	case PropFlexShrink:
		dst.FlexShrink = src.FlexShrink
	case PropFlexBasis:
		dst.FlexBasis = src.FlexBasis
	case PropOrder:
		dst.Order = src.Order
	case PropRowGap:
		dst.RowGap = src.RowGap
	case PropColumnGap:
		dst.ColumnGap = src.ColumnGap
	case PropOverflowX:
		dst.OverflowX = src.OverflowX
	case PropOverflowY:
		dst.OverflowY = src.OverflowY
	case PropVisibility:
		dst.Visibility = src.Visibility
	case PropFloat:
		dst.Float = src.Float
	case PropClear:
		dst.Clear = src.Clear
	case PropGridTemplateColumns:
		dst.GridTemplateColumns = src.GridTemplateColumns
	case PropGridTemplateRows:
		dst.GridTemplateRows = src.GridTemplateRows
	case PropGridTemplateAreas:
		dst.GridTemplateAreas = src.GridTemplateAreas
	case PropGridAutoColumns:
		dst.GridAutoColumns = src.GridAutoColumns
	case PropGridAutoRows:
		dst.GridAutoRows = src.GridAutoRows
	case PropGridAutoFlow:
		dst.GridAutoFlow = src.GridAutoFlow
	case PropGridColumnStart:
		dst.GridColumnStart = src.GridColumnStart
	case PropGridColumnEnd:
		dst.GridColumnEnd = src.GridColumnEnd
	case PropGridRowStart:
		dst.GridRowStart = src.GridRowStart
	case PropGridRowEnd:
		dst.GridRowEnd = src.GridRowEnd
	case PropJustifyItems:
		dst.JustifyItems = src.JustifyItems
	case PropJustifySelf:
		dst.JustifySelf = src.JustifySelf
	case PropObjectFit:
		dst.ObjectFit = src.ObjectFit
	case PropObjectPosition:
		dst.ObjectPosition = src.ObjectPosition
	}
}

// layoutPropDefs are the layout properties' definitions, merged into
// propTable.
func layoutPropDefs() map[string]propDef {
	lay := func(f func(l *Layout) *Sizing) func(v *Values) *Sizing {
		return func(v *Values) *Sizing { return f(&v.Layout) }
	}
	return map[string]propDef{
		"display":  {setOf(PropDisplay), parseDisplay},
		"position": {setOf(PropPosition), enumInto(func(v *Values) *Position { return &v.Layout.Position }, "static", "relative", "absolute", "fixed", "sticky")},
		"top":      {setOf(PropTop), sizingInto(lay(func(l *Layout) *Sizing { return &l.Inset[0] }), insetForms)},
		"right":    {setOf(PropRight), sizingInto(lay(func(l *Layout) *Sizing { return &l.Inset[1] }), insetForms)},
		"bottom":   {setOf(PropBottom), sizingInto(lay(func(l *Layout) *Sizing { return &l.Inset[2] }), insetForms)},
		"left":     {setOf(PropLeft), sizingInto(lay(func(l *Layout) *Sizing { return &l.Inset[3] }), insetForms)},
		"inset":    {setOf(PropTop, PropRight, PropBottom, PropLeft), parseInset},
		"z-index":  {setOf(PropZIndex), parseZIndex},

		"width":        {setOf(PropWidth), sizingInto(lay(func(l *Layout) *Sizing { return &l.Width }), sizeForms)},
		"height":       {setOf(PropHeight), sizingInto(lay(func(l *Layout) *Sizing { return &l.Height }), sizeForms)},
		"min-width":    {setOf(PropMinWidth), sizingInto(lay(func(l *Layout) *Sizing { return &l.MinWidth }), sizeForms)},
		"min-height":   {setOf(PropMinHeight), sizingInto(lay(func(l *Layout) *Sizing { return &l.MinHeight }), sizeForms)},
		"max-width":    {setOf(PropMaxWidth), sizingInto(lay(func(l *Layout) *Sizing { return &l.MaxWidth }), maxForms)},
		"max-height":   {setOf(PropMaxHeight), sizingInto(lay(func(l *Layout) *Sizing { return &l.MaxHeight }), maxForms)},
		"box-sizing":   {setOf(PropBoxSizing), enumInto(func(v *Values) *BoxSizing { return &v.Layout.BoxSizing }, "content-box", "border-box")},
		"aspect-ratio": {setOf(PropAspectRatio), parseAspectRatio},

		"flex":            {setOf(PropFlexGrow, PropFlexShrink, PropFlexBasis), parseFlex},
		"flex-grow":       {setOf(PropFlexGrow), factorInto(func(v *Values) *float64 { return &v.Layout.FlexGrow })},
		"flex-shrink":     {setOf(PropFlexShrink), factorInto(func(v *Values) *float64 { return &v.Layout.FlexShrink })},
		"flex-basis":      {setOf(PropFlexBasis), sizingInto(lay(func(l *Layout) *Sizing { return &l.FlexBasis }), basisForms)},
		"flex-direction":  {setOf(PropFlexDirection), enumInto(func(v *Values) *FlexDirection { return &v.Layout.FlexDirection }, flexDirectionNames...)},
		"flex-wrap":       {setOf(PropFlexWrap), enumInto(func(v *Values) *FlexWrap { return &v.Layout.FlexWrap }, flexWrapNames...)},
		"flex-flow":       {setOf(PropFlexDirection, PropFlexWrap), parseFlexFlow},
		"order":           {setOf(PropOrder), intInto(func(v *Values) *int { return &v.Layout.Order })},
		"justify-content": {setOf(PropJustifyContent), alignInto(func(v *Values) *BoxAlign { return &v.Layout.JustifyContent }, justifyContentKinds)},
		"align-items":     {setOf(PropAlignItems), alignInto(func(v *Values) *BoxAlign { return &v.Layout.AlignItems }, alignItemsKinds)},
		"align-self":      {setOf(PropAlignSelf), alignInto(func(v *Values) *BoxAlign { return &v.Layout.AlignSelf }, alignSelfKinds)},
		"align-content":   {setOf(PropAlignContent), alignInto(func(v *Values) *BoxAlign { return &v.Layout.AlignContent }, alignContentKinds)},
		"place-content":   {setOf(PropAlignContent, PropJustifyContent), parsePlaceContent},
		"gap":             {setOf(PropRowGap, PropColumnGap), parseGap},
		"row-gap":         {setOf(PropRowGap), gapInto(func(v *Values) *Length { return &v.Layout.RowGap })},
		"column-gap":      {setOf(PropColumnGap), gapInto(func(v *Values) *Length { return &v.Layout.ColumnGap })},

		"overflow":   {setOf(PropOverflowX, PropOverflowY), parseOverflow},
		"overflow-x": {setOf(PropOverflowX), enumInto(func(v *Values) *Overflow { return &v.Layout.OverflowX }, overflowNames...)},
		"overflow-y": {setOf(PropOverflowY), enumInto(func(v *Values) *Overflow { return &v.Layout.OverflowY }, overflowNames...)},
		"visibility": {setOf(PropVisibility), enumInto(func(v *Values) *Visibility { return &v.Layout.Visibility }, "visible", "hidden", "collapse")},
		"float":      {setOf(PropFloat), enumInto(func(v *Values) *Float { return &v.Layout.Float }, "none", "left", "right", "inline-start", "inline-end")},
		"clear":      {setOf(PropClear), enumInto(func(v *Values) *Clear { return &v.Layout.Clear }, "none", "left", "right", "both", "inline-start", "inline-end")},

		"object-fit":      {setOf(PropObjectFit), parseObjectFit},
		"object-position": {setOf(PropObjectPosition), parseObjectPosition},
	}
}

// The keyword tables whose order is the enum's.
var (
	flexDirectionNames = []string{"row", "row-reverse", "column", "column-reverse"}
	flexWrapNames      = []string{"nowrap", "wrap", "wrap-reverse"}
	overflowNames      = []string{"visible", "hidden", "clip", "scroll", "auto"}
)

// enumInto parses one keyword out of names, the enum's values in
// order: names[i] computes to T(i).
func enumInto[T ~uint8](get func(v *Values) *T, names ...string) func([]token, *ctx, *Values) bool {
	return func(ts []token, _ *ctx, v *Values) bool {
		k, ok := keywordOf(components(ts), names)
		if ok {
			*get(v) = T(k)
		}
		return ok
	}
}

// keywordOf finds the single identifier comps holds in names.
func keywordOf(comps [][]token, names []string) (int, bool) {
	s, ok := identOf(comps)
	if !ok {
		return 0, false
	}
	for i, n := range names {
		if s == n {
			return i, true
		}
	}
	return 0, false
}

// lengthPctOf evaluates one <length-percentage> component: a length
// (unitless numbers read as pixels, as lengthOf allows), a percentage,
// or a calc() mixing the two.
func lengthPctOf(ts []token, cx *ctx) (Length, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok {
		return Length{}, false
	}
	switch n.kind {
	case numLength, numNumber:
		return Length{Px: px(n.v)}, true
	case numPercent:
		return Length{Pct: n.v}, true
	case numLengthPct:
		return Length{Px: px(n.v), Pct: n.pct}, true
	}
	return Length{}, false
}

// sizingForms says which forms a sizing property takes: its auto
// keyword's spelling (none for max-*), whether the content keywords
// and content apply, and whether lengths may be negative.
type sizingForms struct {
	auto     string
	content  bool
	basis    bool
	negative bool
}

var (
	sizeForms  = sizingForms{auto: "auto", content: true}
	maxForms   = sizingForms{auto: "none", content: true}
	basisForms = sizingForms{auto: "auto", content: true, basis: true}
	insetForms = sizingForms{auto: "auto", negative: true}
)

// sizingOf parses one sizing value in the forms f allows.
func sizingOf(comps [][]token, cx *ctx, f sizingForms) (Sizing, bool) {
	if len(comps) != 1 {
		return Sizing{}, false
	}
	c := comps[0]
	if s, ok := identOf(comps); ok {
		switch {
		case s == f.auto:
			return Sizing{}, true
		case f.content && s == "min-content":
			return Sizing{Kind: SizingMinContent}, true
		case f.content && s == "max-content":
			return Sizing{Kind: SizingMaxContent}, true
		case f.content && s == "fit-content":
			return Sizing{Kind: SizingFitContent, Len: Length{Pct: 100}}, true
		case f.basis && s == "content":
			return Sizing{Kind: SizingContent}, true
		}
		return Sizing{}, false
	}
	if f.content && c[0].kind == tkFunc && strings.EqualFold(c[0].s, "fit-content") {
		l, ok := lengthPctOf(funcArgs(c), cx)
		if !ok || l.Negative() {
			return Sizing{}, false
		}
		return Sizing{Kind: SizingFitContent, Len: l}, true
	}
	l, ok := lengthPctOf(c, cx)
	if !ok || !f.negative && l.Negative() {
		return Sizing{}, false
	}
	return Sizing{Kind: SizingLength, Len: l}, true
}

// sizingInto parses one sizing value into the slot the getter returns.
func sizingInto(get func(v *Values) *Sizing, f sizingForms) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		s, ok := sizingOf(components(ts), cx, f)
		if ok {
			*get(v) = s
		}
		return ok
	}
}

// parseInset parses the inset shorthand: one to four top/right/bottom/
// left values, expanded like margin.
func parseInset(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	vals := make([]Sizing, 0, len(comps))
	for _, c := range comps {
		s, ok := sizingOf([][]token{c}, cx, insetForms)
		if !ok {
			return false
		}
		vals = append(vals, s)
	}
	sides, ok := expand4(vals)
	if ok {
		v.Layout.Inset = sides
	}
	return ok
}

// marginSideOf parses one margin side: a length, negative allowed, or
// auto.
func marginSideOf(c []token, cx *ctx) (n int, auto, ok bool) {
	if len(c) == 1 && c[0].ident("auto") {
		return 0, true, true
	}
	l, ok := lengthOf(c, cx)
	return px(l), false, ok
}

// marginInto parses one margin longhand, side by the Sides index.
func marginInto(side int) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) != 1 {
			return false
		}
		n, auto, ok := marginSideOf(comps[0], cx)
		if ok {
			*v.Margin.ptr(side) = n
			v.MarginAuto[side] = auto
		}
		return ok
	}
}

// parseMargin parses the margin shorthand: one to four sides, each a
// length or auto.
func parseMargin(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	lens := make([]int, 0, len(comps))
	autos := make([]bool, 0, len(comps))
	for _, c := range comps {
		n, auto, ok := marginSideOf(c, cx)
		if !ok {
			return false
		}
		lens, autos = append(lens, n), append(autos, auto)
	}
	m, ok := expandSides(lens)
	if !ok {
		return false
	}
	v.Margin = m
	v.MarginAuto, _ = expand4(autos)
	return true
}

// integerOf evaluates one integer component.
func integerOf(comps [][]token, cx *ctx) (int, bool) {
	if len(comps) != 1 {
		return 0, false
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numNumber || n.v != math.Trunc(n.v) {
		return 0, false
	}
	return int(n.v), true
}

// intInto parses one integer (order).
func intInto(get func(v *Values) *int) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		n, ok := integerOf(components(ts), cx)
		if ok {
			*get(v) = n
		}
		return ok
	}
}

// parseZIndex parses z-index: auto (which stacks as 0) or an integer.
func parseZIndex(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if s, ok := identOf(comps); ok {
		if s != "auto" {
			return false
		}
		v.Layout.ZIndex = 0
		return true
	}
	n, ok := integerOf(comps, cx)
	if ok {
		v.Layout.ZIndex = n
	}
	return ok
}

// factorOf evaluates one non-negative number: a flex factor.
func factorOf(ts []token, cx *ctx) (float64, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok || n.kind != numNumber || n.v < 0 {
		return 0, false
	}
	return n.v, true
}

// factorInto parses flex-grow or flex-shrink.
func factorInto(get func(v *Values) *float64) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) != 1 {
			return false
		}
		f, ok := factorOf(comps[0], cx)
		if ok {
			*get(v) = f
		}
		return ok
	}
}

// parseFlex parses the flex shorthand: none (0 0 auto), auto (1 1
// auto), or `<grow> <shrink>? || <basis>`. An omitted grow is 1, an
// omitted shrink 1, and an omitted basis zero — the spec's 0%, which
// lys-ui's layout reads as the zero length it resolves to.
func parseFlex(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if s, ok := identOf(comps); ok {
		switch s {
		case "none":
			v.Layout.FlexGrow, v.Layout.FlexShrink, v.Layout.FlexBasis = 0, 0, Sizing{}
			return true
		case "auto":
			v.Layout.FlexGrow, v.Layout.FlexShrink, v.Layout.FlexBasis = 1, 1, Sizing{}
			return true
		}
	}
	n := len(comps)
	if n < 1 || n > 3 {
		return false
	}
	grow, shrink, basis := 1.0, 1.0, PxSizing(0)
	i := 0
	// factors reads `<grow> <shrink>?` at i; the shrink must follow the
	// grow directly.
	factors := func() bool {
		f, ok := factorOf(comps[i], cx)
		if !ok {
			return false
		}
		grow = f
		i++
		if i < n {
			if f, ok := factorOf(comps[i], cx); ok {
				shrink = f
				i++
			}
		}
		return true
	}
	basisAt := func() bool {
		s, ok := sizingOf(comps[i:i+1], cx, basisForms)
		basis = s
		i++
		return ok
	}
	if factors() {
		if i < n && !basisAt() {
			return false
		}
	} else if !basisAt() || i < n && !factors() {
		return false
	}
	if i != n {
		return false
	}
	v.Layout.FlexGrow, v.Layout.FlexShrink, v.Layout.FlexBasis = grow, shrink, basis
	return true
}

// parseFlexFlow parses flex-flow: a flex-direction and a flex-wrap, in
// either order, either one omitted.
func parseFlexFlow(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || len(comps) > 2 {
		return false
	}
	dir, wrap := FlexRow, FlexNowrap
	haveDir, haveWrap := false, false
	for _, c := range comps {
		one := [][]token{c}
		if k, ok := keywordOf(one, flexDirectionNames); ok && !haveDir {
			dir, haveDir = FlexDirection(k), true
			continue
		}
		if k, ok := keywordOf(one, flexWrapNames); ok && !haveWrap {
			wrap, haveWrap = FlexWrap(k), true
			continue
		}
		return false
	}
	v.Layout.FlexDirection, v.Layout.FlexWrap = dir, wrap
	return true
}

// gapOf parses one gap value: normal (zero) or a non-negative
// <length-percentage>.
func gapOf(c []token, cx *ctx) (Length, bool) {
	if len(c) == 1 && c[0].ident("normal") {
		return Length{}, true
	}
	l, ok := lengthPctOf(c, cx)
	if !ok || l.Negative() {
		return Length{}, false
	}
	return l, true
}

// gapInto parses row-gap or column-gap.
func gapInto(get func(v *Values) *Length) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) != 1 {
			return false
		}
		l, ok := gapOf(comps[0], cx)
		if ok {
			*get(v) = l
		}
		return ok
	}
}

// parseGap parses the gap shorthand: a row gap and an optional column
// gap, which defaults to the row gap.
func parseGap(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || len(comps) > 2 {
		return false
	}
	row, ok := gapOf(comps[0], cx)
	if !ok {
		return false
	}
	col := row
	if len(comps) == 2 {
		if col, ok = gapOf(comps[1], cx); !ok {
			return false
		}
	}
	v.Layout.RowGap, v.Layout.ColumnGap = row, col
	return true
}

// parseOverflow parses the overflow shorthand: overflow-x, then an
// optional overflow-y defaulting to it.
func parseOverflow(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || len(comps) > 2 {
		return false
	}
	x, ok := keywordOf(comps[:1], overflowNames)
	if !ok {
		return false
	}
	y := x
	if len(comps) == 2 {
		if y, ok = keywordOf(comps[1:], overflowNames); !ok {
			return false
		}
	}
	v.Layout.OverflowX, v.Layout.OverflowY = Overflow(x), Overflow(y)
	return true
}

// parseAspectRatio parses aspect-ratio: auto, a ratio (`16 / 9`, or
// one number meaning n / 1), or auto with a ratio. A widget has no
// natural ratio of its own, so `auto <ratio>` computes to the ratio.
func parseAspectRatio(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) > 0 && comps[0][0].ident("auto") {
		comps = comps[1:]
		if len(comps) == 0 {
			v.Layout.AspectRatio = 0
			return true
		}
	} else if n := len(comps); n > 0 && comps[n-1][0].ident("auto") {
		comps = comps[:n-1]
	}
	var w, h float64
	var ok bool
	switch len(comps) {
	case 1:
		w, ok = factorOf(comps[0], cx)
		h = 1
	case 3:
		if !comps[1][0].is('/') {
			return false
		}
		var okH bool
		w, ok = factorOf(comps[0], cx)
		h, okH = factorOf(comps[2], cx)
		ok = ok && okH
	}
	if !ok || w <= 0 || h <= 0 {
		return false
	}
	v.Layout.AspectRatio = w / h
	return true
}

// parseDisplay parses display: one keyword, or the two-keyword
// outer/inner form (block flow, inline flow-root, inline flex, ...).
func parseDisplay(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	var words []string
	for _, c := range comps {
		s, ok := identOf([][]token{c})
		if !ok {
			return false
		}
		words = append(words, s)
	}
	d, ok := displayOf(words)
	if ok {
		v.Layout.Display = d
	}
	return ok
}

// displayOf maps the display keywords to the computed value.
func displayOf(words []string) (Display, bool) {
	switch len(words) {
	case 1:
		switch words[0] {
		case "block":
			return DisplayBlock, true
		case "inline":
			return DisplayInline, true
		case "inline-block":
			return DisplayInlineBlock, true
		case "flow-root":
			return DisplayFlowRoot, true
		case "flex":
			return DisplayFlex, true
		case "inline-flex":
			return DisplayInlineFlex, true
		case "grid":
			return DisplayGrid, true
		case "inline-grid":
			return DisplayInlineGrid, true
		case "contents":
			return DisplayContents, true
		case "none":
			return DisplayNone, true
		}
	case 2:
		outer, inner := words[0], words[1]
		if outer == "flow" || outer == "flow-root" || outer == "flex" || outer == "grid" {
			outer, inner = inner, outer
		}
		inline := outer == "inline"
		if !inline && outer != "block" {
			return 0, false
		}
		switch inner {
		case "flow":
			if inline {
				return DisplayInline, true
			}
			return DisplayBlock, true
		case "flow-root":
			if inline {
				return DisplayInlineBlock, true
			}
			return DisplayFlowRoot, true
		case "flex":
			if inline {
				return DisplayInlineFlex, true
			}
			return DisplayFlex, true
		case "grid":
			if inline {
				return DisplayInlineGrid, true
			}
			return DisplayGrid, true
		}
	}
	return 0, false
}

// alignKinds is a set of alignment keywords, one bit per BoxAlignKind.
type alignKinds uint32

func kindsOf(ks ...BoxAlignKind) alignKinds {
	var s alignKinds
	for _, k := range ks {
		s |= 1 << k
	}
	return s
}

// The keyword sets each alignment property takes, and the positional
// ones safe/unsafe may prefix.
var (
	positionalKinds = kindsOf(BoxStart, BoxEnd, BoxFlexStart, BoxFlexEnd, BoxSelfStart, BoxSelfEnd,
		BoxCenter, BoxLeft, BoxRight)
	distributedKinds    = kindsOf(BoxSpaceBetween, BoxSpaceAround, BoxSpaceEvenly)
	justifyContentKinds = kindsOf(BoxNormal, BoxStretch, BoxStart, BoxEnd, BoxFlexStart, BoxFlexEnd,
		BoxCenter, BoxLeft, BoxRight) | distributedKinds
	alignItemsKinds = kindsOf(BoxNormal, BoxStretch, BoxBaseline, BoxLastBaseline, BoxStart, BoxEnd,
		BoxFlexStart, BoxFlexEnd, BoxSelfStart, BoxSelfEnd, BoxCenter)
	alignSelfKinds    = alignItemsKinds | kindsOf(BoxAuto)
	alignContentKinds = kindsOf(BoxNormal, BoxStretch, BoxBaseline, BoxLastBaseline, BoxStart, BoxEnd,
		BoxFlexStart, BoxFlexEnd, BoxCenter) | distributedKinds
)

// alignNames spells each BoxAlignKind.
var alignNames = map[string]BoxAlignKind{
	"normal": BoxNormal, "auto": BoxAuto, "stretch": BoxStretch, "start": BoxStart, "end": BoxEnd,
	"flex-start": BoxFlexStart, "flex-end": BoxFlexEnd, "self-start": BoxSelfStart,
	"self-end": BoxSelfEnd, "center": BoxCenter, "left": BoxLeft, "right": BoxRight,
	"baseline": BoxBaseline, "space-between": BoxSpaceBetween, "space-around": BoxSpaceAround,
	"space-evenly": BoxSpaceEvenly,
}

// alignOf parses one alignment value: a keyword from allowed, the
// baseline forms (first baseline, last baseline), or a positional
// keyword behind safe/unsafe.
func alignOf(comps [][]token, allowed alignKinds) (BoxAlign, bool) {
	var words []string
	for _, c := range comps {
		s, ok := identOf([][]token{c})
		if !ok {
			return BoxAlign{}, false
		}
		words = append(words, s)
	}
	var out BoxAlign
	switch {
	case len(words) == 2 && (words[0] == "first" || words[0] == "last") && words[1] == "baseline":
		out.Kind = BoxBaseline
		if words[0] == "last" {
			out.Kind = BoxLastBaseline
		}
	case len(words) == 2 && (words[0] == "safe" || words[0] == "unsafe"):
		k, ok := alignNames[words[1]]
		if !ok || positionalKinds&(1<<k) == 0 {
			return BoxAlign{}, false
		}
		out = BoxAlign{Kind: k, Safe: words[0] == "safe"}
	case len(words) == 1:
		k, ok := alignNames[words[0]]
		if !ok {
			return BoxAlign{}, false
		}
		out.Kind = k
	default:
		return BoxAlign{}, false
	}
	return out, allowed&(1<<out.Kind) != 0
}

// alignInto parses one alignment property.
func alignInto(get func(v *Values) *BoxAlign, allowed alignKinds) func([]token, *ctx, *Values) bool {
	return func(ts []token, _ *ctx, v *Values) bool {
		a, ok := alignOf(components(ts), allowed)
		if ok {
			*get(v) = a
		}
		return ok
	}
}

// parsePlaceContent parses place-content: align-content, then an
// optional justify-content defaulting to it. Every split point is
// tried, since either half may be one or two keywords.
func parsePlaceContent(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 || len(comps) > 4 {
		return false
	}
	for split := 1; split <= len(comps); split++ {
		a, ok := alignOf(comps[:split], alignContentKinds)
		if !ok {
			continue
		}
		j := a
		if split < len(comps) {
			if j, ok = alignOf(comps[split:], justifyContentKinds); !ok {
				continue
			}
		} else if justifyContentKinds&(1<<j.Kind) == 0 {
			// A lone baseline keyword justifies as start.
			j = BoxAlign{Kind: BoxStart}
		}
		v.Layout.AlignContent, v.Layout.JustifyContent = a, j
		return true
	}
	return false
}
