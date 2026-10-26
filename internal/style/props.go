package style

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// propDef describes one property name: the longhands it sets and the
// parser that writes them into a scratch Values.
type propDef struct {
	covers PropSet
	parse  func(ts []token, cx *ctx, v *Values) bool
}

// ignoredProps are real GTK/CSS properties the engine accepts and
// drops without a warning: they parse in every GTK stylesheet but have
// no painter here (animations, icon transforms, text decoration, ...).
var ignoredProps = map[string]bool{
	"-gtk-icon-shadow": true, "-gtk-icon-style": true,
	"-gtk-icon-filter": true, "-gtk-dpi": true, "-gtk-secondary-caret-color": true,
	"font-stretch": true, "font-kerning": true,
	"background-blend-mode": true,
	"outline-radius":        true, "-gtk-outline-radius": true,
}

// propTable maps every supported property name to its definition.
var propTable map[string]propDef

func init() {
	propTable = map[string]propDef{
		"color":                 {setOf(PropColor), colorInto(func(v *Values) *render.Color { return &v.Color })},
		"background-color":      {setOf(PropBackgroundColor), colorInto(func(v *Values) *render.Color { return &v.Background })},
		"background-image":      {setOf(PropBackgroundImage), parseBackgroundImage},
		"background":            {backgroundProps, parseBackground},
		"background-size":       {setOf(PropBackgroundSize), parseBackgroundSize},
		"background-position":   {setOf(PropBackgroundPosition), parseBackgroundPosition},
		"background-repeat":     {setOf(PropBackgroundRepeat), parseBackgroundRepeat},
		"background-clip":       {setOf(PropBackgroundClip), parseBackgroundClip},
		"background-origin":     {setOf(PropBackgroundOrigin), parseBackgroundOrigin},
		"background-attachment": {setOf(PropBackgroundAttachment), parseBackgroundAttachment},
		"opacity":               {setOf(PropOpacity), parseOpacity},
		"filter":                {setOf(PropFilter), parseFilter},
		"backdrop-filter":       {setOf(PropBackdropFilter), parseBackdropFilter},
		"mix-blend-mode":        {setOf(PropMixBlendMode), parseMixBlendMode},
		"isolation":             {setOf(PropIsolation), parseIsolation},
		"clip-path":             {setOf(PropClipPath), parseClipPath},
		"mask":                  {setOf(PropMaskImage, PropMaskPosition, PropMaskSize, PropMaskRepeat, PropMaskMode), parseMask},
		"mask-image":            {setOf(PropMaskImage), parseMaskImage},
		"mask-position":         {setOf(PropMaskPosition), parseMaskPosition},
		"mask-size":             {setOf(PropMaskSize), parseMaskSize},
		"mask-repeat":           {setOf(PropMaskRepeat), parseMaskRepeat},
		"mask-mode":             {setOf(PropMaskMode), parseMaskMode},
		"perspective":           {setOf(PropPerspective), parsePerspective},
		"transform-style":       {setOf(PropTransformStyle), parseTransformStyle},
		"backface-visibility":   {setOf(PropBackfaceVisibility), parseBackfaceVisibility},
		"-lys-layout-motion":    {setOf(PropLayoutMotion), parseLayoutMotion},
		"container-type":        {setOf(PropContainerType), parseContainerType},
		"container-name":        {setOf(PropContainerName), parseContainerName},
		"container":             {setOf(PropContainerName, PropContainerType), parseContainer},

		"padding":        {setOf(PropPaddingTop, PropPaddingRight, PropPaddingBottom, PropPaddingLeft), sidesInto(func(v *Values) *Sides { return &v.Padding }, false)},
		"padding-top":    {setOf(PropPaddingTop), sideInto(func(v *Values) *int { return &v.Padding.Top }, false)},
		"padding-right":  {setOf(PropPaddingRight), sideInto(func(v *Values) *int { return &v.Padding.Right }, false)},
		"padding-bottom": {setOf(PropPaddingBottom), sideInto(func(v *Values) *int { return &v.Padding.Bottom }, false)},
		"padding-left":   {setOf(PropPaddingLeft), sideInto(func(v *Values) *int { return &v.Padding.Left }, false)},
		"margin":         {setOf(PropMarginTop, PropMarginRight, PropMarginBottom, PropMarginLeft), parseMargin},
		"margin-top":     {setOf(PropMarginTop), marginInto(0)},
		"margin-right":   {setOf(PropMarginRight), marginInto(1)},
		"margin-bottom":  {setOf(PropMarginBottom), marginInto(2)},
		"margin-left":    {setOf(PropMarginLeft), marginInto(3)},

		"border-width":        {borderWidths, sidesInto(func(v *Values) *Sides { return &v.BorderWidth }, false)},
		"border-top-width":    {setOf(PropBorderTopWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Top }, false)},
		"border-right-width":  {setOf(PropBorderRightWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Right }, false)},
		"border-bottom-width": {setOf(PropBorderBottomWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Bottom }, false)},
		"border-left-width":   {setOf(PropBorderLeftWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Left }, false)},
		"border-style":        {borderStyles, parseBorderStyles},
		"border-top-style":    {setOf(PropBorderTopStyle), borderStyleInto(0)},
		"border-right-style":  {setOf(PropBorderRightStyle), borderStyleInto(1)},
		"border-bottom-style": {setOf(PropBorderBottomStyle), borderStyleInto(2)},
		"border-left-style":   {setOf(PropBorderLeftStyle), borderStyleInto(3)},
		"border-color":        {borderColors, parseBorderColors},
		"border-top-color":    {setOf(PropBorderTopColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[0] })},
		"border-right-color":  {setOf(PropBorderRightColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[1] })},
		"border-bottom-color": {setOf(PropBorderBottomColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[2] })},
		"border-left-color":   {setOf(PropBorderLeftColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[3] })},
		"border":              {borderWidths.Or(borderStyles).Or(borderColors).Or(borderImageProps), borderSideInto(0b1111)},
		"border-top":          {setOf(PropBorderTopWidth, PropBorderTopStyle, PropBorderTopColor), borderSideInto(0b0001)},
		"border-right":        {setOf(PropBorderRightWidth, PropBorderRightStyle, PropBorderRightColor), borderSideInto(0b0010)},
		"border-bottom":       {setOf(PropBorderBottomWidth, PropBorderBottomStyle, PropBorderBottomColor), borderSideInto(0b0100)},
		"border-left":         {setOf(PropBorderLeftWidth, PropBorderLeftStyle, PropBorderLeftColor), borderSideInto(0b1000)},

		"border-radius":              {borderRadii, parseBorderRadius},
		"border-top-left-radius":     {setOf(PropBorderTopLeftRadius), cornerInto(func(v *Values) *int { return &v.Radius.TopLeft })},
		"border-top-right-radius":    {setOf(PropBorderTopRightRadius), cornerInto(func(v *Values) *int { return &v.Radius.TopRight })},
		"border-bottom-right-radius": {setOf(PropBorderBottomRightRadius), cornerInto(func(v *Values) *int { return &v.Radius.BottomRight })},
		"border-bottom-left-radius":  {setOf(PropBorderBottomLeftRadius), cornerInto(func(v *Values) *int { return &v.Radius.BottomLeft })},

		"box-shadow": {setOf(PropBoxShadow), parseBoxShadow},

		"outline":        {setOf(PropOutlineWidth, PropOutlineStyle, PropOutlineColor), parseOutline},
		"outline-width":  {setOf(PropOutlineWidth), lengthInto(func(v *Values) *int { return &v.OutlineWidth }, false)},
		"outline-style":  {setOf(PropOutlineStyle), outlineStyleInto},
		"outline-color":  {setOf(PropOutlineColor), parseOutlineColor},
		"outline-offset": {setOf(PropOutlineOffset), lengthInto(func(v *Values) *int { return &v.OutlineOffset }, true)},

		"border-spacing":      {setOf(PropBorderSpacing), parseBorderSpacing},
		"border-image":        {borderImageProps, parseBorderImage},
		"border-image-source": {setOf(PropBorderImageSource), parseBorderImageSource},
		"border-image-slice":  {setOf(PropBorderImageSlice), parseBorderImageSlice},
		"border-image-width":  {setOf(PropBorderImageWidth), parseBorderImageWidth},
		"border-image-outset": {setOf(PropBorderImageOutset), parseBorderImageOutset},
		"border-image-repeat": {setOf(PropBorderImageRepeat), parseBorderImageRepeat},

		"font-family":             {setOf(PropFontFamily), parseFontFamily},
		"font-size":               {setOf(PropFontSize), parseFontSize},
		"font-weight":             {setOf(PropFontWeight), parseFontWeight},
		"font-style":              {setOf(PropFontStyle), parseFontStyle},
		"font":                    {setOf(PropFontFamily, PropFontSize, PropFontWeight, PropFontStyle, PropLineHeight), parseFont},
		"letter-spacing":          {setOf(PropLetterSpacing), parseLetterSpacing},
		"text-transform":          {setOf(PropTextTransform), parseTextTransform},
		"line-height":             {setOf(PropLineHeight), parseLineHeight},
		"transform":               {setOf(PropTransform), parseTransform},
		"transform-origin":        {setOf(PropTransformOrigin), parseTransformOrigin},
		"font-feature-settings":   {setOf(PropFontFeatures), parseFontFeatures},
		"font-variation-settings": {setOf(PropFontVariations), parseFontVariations},
		"-gtk-icon-size":          {setOf(PropIconSize), lengthInto(func(v *Values) *int { return &v.IconSize }, false)},

		"transition":                 {transitionProps, parseTransition},
		"transition-property":        {setOf(PropTransitionProperty), parseTransitionProperty},
		"transition-duration":        {setOf(PropTransitionDuration), timeListInto(func(v *Values) *[]float64 { return &v.Transition.Durations }, false)},
		"transition-delay":           {setOf(PropTransitionDelay), timeListInto(func(v *Values) *[]float64 { return &v.Transition.Delays }, true)},
		"transition-timing-function": {setOf(PropTransitionTiming), timingListInto(func(v *Values) *[]Timing { return &v.Transition.Timings })},
		"transition-behavior":        {setOf(PropTransitionBehavior), parseTransitionBehavior},

		"animation":                 {animationProps, parseAnimation},
		"animation-name":            {setOf(PropAnimation), parseAnimationName},
		"animation-duration":        {setOf(PropAnimDuration), timeListInto(func(v *Values) *[]float64 { return &v.Anim.Durations }, false)},
		"animation-delay":           {setOf(PropAnimDelay), timeListInto(func(v *Values) *[]float64 { return &v.Anim.Delays }, true)},
		"animation-timing-function": {setOf(PropAnimTiming), timingListInto(func(v *Values) *[]Timing { return &v.Anim.Timings })},
		"animation-iteration-count": {setOf(PropAnimIteration), parseAnimationIterations},
		"animation-direction":       {setOf(PropAnimDirection), keywordListInto(animDirections, func(v *Values) *[]uint8 { return &v.Anim.Directions })},
		"animation-fill-mode":       {setOf(PropAnimFill), keywordListInto(animFills, func(v *Values) *[]uint8 { return &v.Anim.Fills })},
		"animation-play-state":      {setOf(PropAnimationPlayState), parseAnimationPlayState},
		"animation-composition":     {setOf(PropAnimComposition), keywordListInto(animCompositions, func(v *Values) *[]uint8 { return &v.Anim.Compositions })},

		"scroll-behavior":       {setOf(PropScrollBehavior), parseScrollBehavior},
		"scroll-margin":         {scrollMarginProps, parseScrollMargin},
		"scroll-margin-top":     {setOf(PropScrollMarginTop), sideInto(func(v *Values) *int { return &v.ScrollMargin.Top }, true)},
		"scroll-margin-right":   {setOf(PropScrollMarginRight), sideInto(func(v *Values) *int { return &v.ScrollMargin.Right }, true)},
		"scroll-margin-bottom":  {setOf(PropScrollMarginBottom), sideInto(func(v *Values) *int { return &v.ScrollMargin.Bottom }, true)},
		"scroll-margin-left":    {setOf(PropScrollMarginLeft), sideInto(func(v *Values) *int { return &v.ScrollMargin.Left }, true)},
		"scroll-padding":        {scrollPaddingProps, parseScrollPadding},
		"scroll-padding-top":    {setOf(PropScrollPaddingTop), sideInto(func(v *Values) *int { return &v.ScrollPad.Top }, false)},
		"scroll-padding-right":  {setOf(PropScrollPaddingRight), sideInto(func(v *Values) *int { return &v.ScrollPad.Right }, false)},
		"scroll-padding-bottom": {setOf(PropScrollPaddingBottom), sideInto(func(v *Values) *int { return &v.ScrollPad.Bottom }, false)},
		"scroll-padding-left":   {setOf(PropScrollPaddingLeft), sideInto(func(v *Values) *int { return &v.ScrollPad.Left }, false)},
		"scroll-snap-type":      {setOf(PropScrollSnapType), parseScrollSnapType},
		"scroll-snap-align":     {setOf(PropScrollSnapAlign), parseScrollSnapAlign},
		"scroll-snap-stop":      {setOf(PropScrollSnapStop), parseScrollSnapStop},
		"overscroll-behavior":   {setOf(PropOverscrollX, PropOverscrollY), func(ts []token, cx *ctx, v *Values) bool { return parseOverscroll(ts, cx, v, 0) }},
		"overscroll-behavior-x": {setOf(PropOverscrollX), func(ts []token, cx *ctx, v *Values) bool { return parseOverscroll(ts, cx, v, 0) }},
		"overscroll-behavior-y": {setOf(PropOverscrollY), func(ts []token, cx *ctx, v *Values) bool { return parseOverscroll(ts, cx, v, 1) }},
		"scrollbar-color":       {setOf(PropScrollbarColor), parseScrollbarColor},
		"scrollbar-width":       {setOf(PropScrollbarWidth), parseScrollbarWidth},
		"scrollbar-gutter":      {setOf(PropScrollbarGutter), parseScrollbarGutter},
		"-gtk-icon-transform":   {setOf(PropIconTransform), parseIconTransform},
		"-gtk-icon-source":      {setOf(PropIconSource), parseIconSource},
		"-gtk-icon-palette":     {setOf(PropIconPalette), parseIconPalette},
		"caret-color":           {setOf(PropCaretColor), parseCaretColor},
		"text-decoration":       {textDecorationProps, parseTextDecoration},
		"text-decoration-line":  {setOf(PropTextDecorationLine), parseTextDecorationLine},
		"text-decoration-style": {setOf(PropTextDecorationStyle), parseTextDecorationStyle},
		"text-decoration-color": {setOf(PropTextDecorationColor), colorInto(func(v *Values) *render.Color { return &v.Decoration.Color })},
		"text-shadow":           {setOf(PropTextShadow), parseTextShadow},
		"cursor":                {setOf(PropCursor), parseCursor},
		"pointer-events":        {setOf(PropPointerEvents), parsePointerEvents},
		"user-select":           {setOf(PropUserSelect), parseUserSelect},
		"caret-shape":           {setOf(PropCaretShape), parseCaretShape},
		"accent-color":          {setOf(PropAccentColor), parseAccentColor},
		"appearance":            {setOf(PropAppearance), parseAppearance},
		"touch-action":          {setOf(PropTouchAction), parseTouchAction},

		"text-align":              {setOf(PropTextAlign), parseTextAlign},
		"text-indent":             {setOf(PropTextIndent), parseTextIndent},
		"white-space":             {setOf(PropWhiteSpace), parseWhiteSpace},
		"word-break":              {setOf(PropWordBreak), parseWordBreak},
		"line-break":              {setOf(PropLineBreak), parseLineBreak},
		"hyphens":                 {setOf(PropHyphens), parseHyphens},
		"word-spacing":            {setOf(PropWordSpacing), parseWordSpacing},
		"vertical-align":          {setOf(PropVerticalAlign), parseVerticalAlign},
		"line-clamp":              {setOf(PropLineClamp), parseLineClamp},
		"-webkit-line-clamp":      {setOf(PropLineClamp), parseLineClamp},
		"text-overflow":           {setOf(PropTextOverflow), parseTextOverflow},
		"font-variant-caps":       {setOf(PropFontVariantCaps), parseFontVariantCaps},
		"font-variant-numeric":    {setOf(PropFontVariantNumeric), parseFontVariantNumeric},
		"font-variant-ligatures":  {setOf(PropFontVariantLigatures), parseFontVariantLigatures},
		"font-variant-east-asian": {setOf(PropFontVariantEastAsian), parseFontVariantEastAsian},
		"font-variant-position":   {setOf(PropFontVariantPosition), parseFontVariantPosition},
		"font-variant-alternates": {setOf(PropFontVariantAlternates), parseFontVariantAlternates},
		"font-variant":            {fontVariantProps, parseFontVariant},
		"font-synthesis":          {setOf(PropFontSynthesis), parseFontSynthesis},
		"font-optical-sizing":     {setOf(PropFontOpticalSizing), parseFontOpticalSizing},
		"font-palette":            {setOf(PropFontPalette), parseFontPalette},
		"overflow-wrap":           {setOf(PropOverflowWrap), parseOverflowWrap},
		"word-wrap":               {setOf(PropOverflowWrap), parseOverflowWrap},
		"direction":               {setOf(PropDirection), parseDirection},
		"unicode-bidi":            {setOf(PropUnicodeBidi), parseUnicodeBidi},
		"writing-mode":            {setOf(PropWritingMode), parseWritingMode},
	}
	maps.Copy(propTable, layoutPropDefs())
	maps.Copy(propTable, gridPropDefs())
}

var (
	borderWidths = setOf(PropBorderTopWidth, PropBorderRightWidth, PropBorderBottomWidth, PropBorderLeftWidth)
	borderStyles = setOf(PropBorderTopStyle, PropBorderRightStyle, PropBorderBottomStyle, PropBorderLeftStyle)
	borderColors = setOf(PropBorderTopColor, PropBorderRightColor, PropBorderBottomColor, PropBorderLeftColor)
	borderRadii  = setOf(PropBorderTopLeftRadius, PropBorderTopRightRadius, PropBorderBottomRightRadius,
		PropBorderBottomLeftRadius)
	scrollMarginProps  = setOf(PropScrollMarginTop, PropScrollMarginRight, PropScrollMarginBottom, PropScrollMarginLeft)
	scrollPaddingProps = setOf(PropScrollPaddingTop, PropScrollPaddingRight, PropScrollPaddingBottom,
		PropScrollPaddingLeft)
	transitionProps = setOf(PropTransitionProperty, PropTransitionDuration, PropTransitionTiming, PropTransitionDelay,
		PropTransitionBehavior)
	animationProps = setOf(PropAnimation, PropAnimationPlayState, PropAnimDuration, PropAnimDelay,
		PropAnimDirection, PropAnimFill, PropAnimTiming, PropAnimIteration)
	textDecorationProps = setOf(PropTextDecorationLine, PropTextDecorationStyle, PropTextDecorationColor)
	borderImageProps    = setOf(PropBorderImageSource, PropBorderImageSlice, PropBorderImageWidth,
		PropBorderImageOutset, PropBorderImageRepeat)
	backgroundProps = setOf(PropBackgroundColor, PropBackgroundImage, PropBackgroundSize, PropBackgroundPosition,
		PropBackgroundRepeat, PropBackgroundClip, PropBackgroundOrigin, PropBackgroundAttachment)
	fontVariantProps = setOf(PropFontVariantCaps, PropFontVariantNumeric, PropFontVariantLigatures,
		PropFontVariantEastAsian, PropFontVariantPosition, PropFontVariantAlternates)
)

// colorInto parses a single color into the slot the getter returns.
func colorInto(get func(v *Values) *render.Color) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		cv, ok := parseColor(ts, cx)
		if !ok {
			return false
		}
		*get(v) = cx.resolve(cv)
		return true
	}
}

// lengthInto parses one length into an int field.
func lengthInto(get func(v *Values) *int, negative bool) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) != 1 {
			return false
		}
		l, ok := lengthOf(comps[0], cx)
		if !ok || l < 0 && !negative {
			return false
		}
		*get(v) = px(l)
		return true
	}
}

// sideInto parses one side length (padding-top, margin-left, ...).
func sideInto(get func(v *Values) *int, negative bool) func([]token, *ctx, *Values) bool {
	return lengthInto(get, negative)
}

// cornerInto parses one corner radius (a second, elliptical radius is
// accepted and dropped: corners are circular here).
func cornerInto(get func(v *Values) *int) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) < 1 || len(comps) > 2 {
			return false
		}
		r, ok := radiusOf(comps[0], cx)
		if !ok {
			return false
		}
		*get(v) = r
		return true
	}
}

// radiusOf reads a corner radius; a percentage has no box to resolve
// against at compute time, so 50% (the pill idiom) maps to a very large
// radius the painters clamp to half the short side, and other
// percentages are rejected.
func radiusOf(ts []token, cx *ctx) (int, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok {
		return 0, false
	}
	switch n.kind {
	case numLength, numNumber:
		if n.v < 0 {
			return 0, false
		}
		return px(n.v), true
	case numPercent:
		if n.v >= 50 {
			return 1 << 14, true
		}
	}
	return 0, false
}

// expand4 applies the CSS 1-to-4 value expansion, top, right, bottom,
// left.
func expand4[T any](vals []T) ([4]T, bool) {
	switch len(vals) {
	case 1:
		return [4]T{vals[0], vals[0], vals[0], vals[0]}, true
	case 2:
		return [4]T{vals[0], vals[1], vals[0], vals[1]}, true
	case 3:
		return [4]T{vals[0], vals[1], vals[2], vals[1]}, true
	case 4:
		return [4]T{vals[0], vals[1], vals[2], vals[3]}, true
	}
	return [4]T{}, false
}

// expandSides applies the CSS 1-to-4 value expansion to lengths.
func expandSides(vals []int) (Sides, bool) {
	s, ok := expand4(vals)
	return Sides{s[0], s[1], s[2], s[3]}, ok
}

// sidesInto parses a 1-4 length shorthand (padding, margin,
// border-width).
func sidesInto(get func(v *Values) *Sides, negative bool) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		var vals []int
		for _, c := range components(ts) {
			l, ok := lengthOf(c, cx)
			if !ok || l < 0 && !negative {
				return false
			}
			vals = append(vals, px(l))
		}
		s, ok := expandSides(vals)
		if !ok {
			return false
		}
		*get(v) = s
		return true
	}
}

// parseBorderRadius parses 1-4 radii (an elliptical `/` part is
// accepted and ignored).
func parseBorderRadius(ts []token, cx *ctx, v *Values) bool {
	var vals []int
	for _, c := range components(ts) {
		if len(c) == 1 && c[0].is('/') {
			break
		}
		r, ok := radiusOf(c, cx)
		if !ok {
			return false
		}
		vals = append(vals, r)
	}
	s, ok := expandSides(vals)
	if !ok {
		return false
	}
	v.Radius = Corners{TopLeft: s.Top, TopRight: s.Right, BottomRight: s.Bottom, BottomLeft: s.Left}
	return true
}

// borderStyleOf parses a line style keyword.
func borderStyleOf(ts []token) (BorderStyle, bool) {
	if len(ts) != 1 || ts[0].kind != tkIdent {
		return 0, false
	}
	switch strings.ToLower(ts[0].s) {
	case "none":
		return BorderNone, true
	case "hidden":
		return BorderHidden, true
	case "solid":
		return BorderSolid, true
	case "dashed":
		return BorderDashed, true
	case "dotted":
		return BorderDotted, true
	case "double":
		return BorderDouble, true
	case "groove":
		return BorderGroove, true
	case "ridge":
		return BorderRidge, true
	case "inset":
		return BorderInset, true
	case "outset":
		return BorderOutset, true
	}
	return 0, false
}

func borderStyleInto(side int) func([]token, *ctx, *Values) bool {
	return func(ts []token, _ *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) != 1 {
			return false
		}
		st, ok := borderStyleOf(comps[0])
		if !ok {
			return false
		}
		v.BorderStyle[side] = st
		return true
	}
}

func parseBorderStyles(ts []token, _ *ctx, v *Values) bool {
	var styles []BorderStyle
	for _, c := range components(ts) {
		st, ok := borderStyleOf(c)
		if !ok {
			return false
		}
		styles = append(styles, st)
	}
	s, ok := expand4(styles)
	if !ok {
		return false
	}
	v.BorderStyle = s
	return true
}

func parseBorderColors(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || len(comps) > 4 {
		return false
	}
	var cs [4]render.Color
	for i, c := range comps {
		cv, ok := parseColor(c, cx)
		if !ok {
			return false
		}
		cs[i] = cx.resolve(cv)
	}
	switch len(comps) {
	case 1:
		cs[1], cs[2], cs[3] = cs[0], cs[0], cs[0]
	case 2:
		cs[2], cs[3] = cs[0], cs[1]
	case 3:
		cs[3] = cs[1]
	}
	v.BorderColor = cs
	return true
}

// lineParts parses the `<width> || <style> || <color>` grammar of the
// border and outline shorthands. Omitted parts take their initial
// values: width 0 (GTK's initial border width), style none, color
// currentColor.
func lineParts(ts []token, cx *ctx, styleOf func([]token) (BorderStyle, bool)) (width int, style BorderStyle, color render.Color, haveC, ok bool) {
	var haveW, haveS bool
	for c := range eachComponent(ts) {
		if !haveS {
			if st, ok := styleOf(c); ok {
				style, haveS = st, true
				continue
			}
		}
		if !haveW {
			if l, ok := lengthOf(c, cx); ok && l >= 0 {
				width, haveW = px(l), true
				continue
			}
			if len(c) == 1 && c[0].kind == tkIdent {
				switch strings.ToLower(c[0].s) {
				case "thin":
					width, haveW = 1, true
					continue
				case "medium":
					width, haveW = 3, true
					continue
				case "thick":
					width, haveW = 5, true
					continue
				}
			}
		}
		if !haveC {
			if cv, ok := parseColor(c, cx); ok {
				color, haveC = cx.resolve(cv), true
				continue
			}
		}
		return 0, 0, 0, false, false
	}
	if !haveC {
		// Read only when the color is omitted: currentColor makes the
		// declaration node-dependent, which costs it the parse cache.
		color = cx.cur()
	}
	return width, style, color, haveC, haveW || haveS || haveC
}

// borderSideInto parses the border / border-<side> shorthands; mask
// selects the sides (bit 0 top, clockwise).
func borderSideInto(mask int) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		w, st, col, _, ok := lineParts(ts, cx, borderStyleOf)
		if !ok {
			return false
		}
		widths := [4]*int{&v.BorderWidth.Top, &v.BorderWidth.Right, &v.BorderWidth.Bottom, &v.BorderWidth.Left}
		for i := range 4 {
			if mask&(1<<i) == 0 {
				continue
			}
			*widths[i] = w
			v.BorderStyle[i] = st
			v.BorderColor[i] = col
		}
		return true
	}
}

// parseOutline parses the outline shorthand: a width, a style (auto
// too) and a color in any order, the color resetting to auto when
// omitted.
func parseOutline(ts []token, cx *ctx, v *Values) bool {
	w, st, col, haveC, ok := lineParts(ts, cx, outlineStyleOf)
	if !ok {
		return false
	}
	v.OutlineWidth, v.OutlineStyle, v.OutlineColor, v.OutlineAuto = w, st, col, !haveC
	return true
}

// outlineStyleOf reads an outline style: the border styles but hidden,
// and auto (css-ui-4 §3.3).
func outlineStyleOf(ts []token) (BorderStyle, bool) {
	if len(ts) == 1 && ts[0].ident("auto") {
		return BorderAuto, true
	}
	st, ok := borderStyleOf(ts)
	return st, ok && st != BorderHidden
}

// parseOutlineColor parses outline-color: auto or a color.
func parseOutlineColor(ts []token, cx *ctx, v *Values) bool {
	if comps := components(ts); len(comps) == 1 && comps[0][0].ident("auto") {
		v.OutlineColor, v.OutlineAuto = cx.cur(), true
		return true
	}
	cv, ok := parseColor(ts, cx)
	if ok {
		v.OutlineColor, v.OutlineAuto = cx.resolve(cv), false
	}
	return ok
}

func outlineStyleInto(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	st, ok := outlineStyleOf(comps[0])
	if !ok {
		return false
	}
	v.OutlineStyle = st
	return true
}

// parseBorderSpacing parses one or two lengths: horizontal, vertical.
func parseBorderSpacing(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || len(comps) > 2 {
		return false
	}
	h, ok := lengthOf(comps[0], cx)
	if !ok || h < 0 {
		return false
	}
	vv := h
	if len(comps) == 2 {
		if vv, ok = lengthOf(comps[1], cx); !ok || vv < 0 {
			return false
		}
	}
	v.BorderSpacingH, v.BorderSpacingV = px(h), px(vv)
	return true
}

// parseOpacity parses a number or percentage, clamped to [0,1].
func parseOpacity(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok {
		return false
	}
	switch n.kind {
	case numNumber:
	case numPercent:
		n.v /= 100
	default:
		return false
	}
	v.Opacity = math.Min(1, math.Max(0, n.v))
	return true
}

// backgroundImageURL parses url(...) — a quoted string — to a local
// file path: a file:// URL strips the scheme, anything else is the
// path itself. lys-ui loads no network images; a bare (unquoted) path
// tokenizes piecemeal and is not accepted, the way producers write
// quoted urls anyway.
func backgroundImageURL(ts []token) (string, bool) {
	if len(ts) == 0 || ts[0].kind != tkFunc || ts[0].s != "url" {
		return "", false
	}
	args := funcArgs(ts)
	if len(args) != 1 || args[0].kind != tkString {
		return "", false
	}
	path := strings.Trim(strings.TrimSpace(args[0].s), "\"'")
	path = strings.TrimPrefix(path, "file://")
	if path == "" {
		return "", false
	}
	return path, true
}

// gradientFuncs maps the gradient functions onto their kind and
// whether they repeat.
var gradientFuncs = map[string]struct {
	kind   render.GradientKind
	repeat bool
}{
	"linear-gradient":           {render.GradientLinear, false},
	"repeating-linear-gradient": {render.GradientLinear, true},
	"radial-gradient":           {render.GradientRadial, false},
	"repeating-radial-gradient": {render.GradientRadial, true},
	"conic-gradient":            {render.GradientConic, false},
	"repeating-conic-gradient":  {render.GradientConic, true},
}

// gradientOf parses the CSS gradient functions: linear-gradient([<angle>
// | to <side-or-corner>,] stops), radial-gradient([<shape> || <size>]
// [at <position>], stops), conic-gradient([from <angle>] [at
// <position>], stops), and their repeating- forms. Corner directions
// use the square-box angle (45° multiples); stop positions are
// percentages (and angles in a conic gradient), auto ones spread
// evenly between their neighbors; positions are keywords or
// percentages.
func gradientOf(ts []token, cx *ctx) (Gradient, bool) {
	if len(ts) == 0 || ts[0].kind != tkFunc {
		return Gradient{}, false
	}
	fn, ok := gradientFuncs[ts[0].s]
	if !ok {
		return Gradient{}, false
	}
	args := splitTop(funcArgs(ts), tkComma)
	g := Gradient{Kind: fn.kind, Repeat: fn.repeat, Angle: 180, CenterX: 0.5, CenterY: 0.5}
	if fn.kind == render.GradientConic {
		g.Angle = 0
	}
	if len(args) > 0 {
		prelude, ok := g.prelude(components(args[0]), cx)
		if !ok {
			return Gradient{}, false
		}
		if prelude {
			args = args[1:]
		}
	}
	if !g.stops(args, cx) {
		return Gradient{}, false
	}
	return g, true
}

// prelude parses the gradient's leading geometry argument into g;
// false (with ok) when the first argument is already a stop.
func (g *Gradient) prelude(first [][]token, cx *ctx) (prelude, ok bool) {
	if len(first) == 0 {
		return false, false
	}
	if _, isColor := parseColor(first[0], cx); isColor {
		return false, true
	}
	switch g.Kind {
	case render.GradientLinear:
		return true, g.linearPrelude(first, cx)
	case render.GradientRadial:
		return true, g.radialPrelude(first, cx)
	}
	return true, g.conicPrelude(first, cx)
}

// linearPrelude parses `<angle>` or `to <side-or-corner>`.
func (g *Gradient) linearPrelude(first [][]token, cx *ctx) bool {
	if len(first) >= 2 && first[0][0].ident("to") {
		var dx, dy int
		for _, c := range first[1:] {
			switch {
			case c[0].ident("top"):
				dy = -1
			case c[0].ident("bottom"):
				dy = 1
			case c[0].ident("left"):
				dx = -1
			case c[0].ident("right"):
				dx = 1
			default:
				return false
			}
		}
		g.Angle = math.Mod(math.Atan2(float64(dx), float64(-dy))*180/math.Pi+360, 360)
		return true
	}
	if len(first) != 1 {
		return false
	}
	n, ok := evalNumeric(first[0], cx)
	switch {
	case ok && n.kind == numAngle:
		g.Angle = n.v
	case ok && n.kind == numNumber && n.v == 0:
		g.Angle = 0
	default:
		return false
	}
	return true
}

// radialPrelude parses `[<shape> || <size>] [at <position>]`.
func (g *Gradient) radialPrelude(first [][]token, cx *ctx) bool {
	var lengths []float64
	for i, c := range first {
		switch {
		case c[0].ident("at"):
			return g.position(first[i+1:], cx) && g.radialLengths(lengths)
		case c[0].ident("circle"):
			g.Circle = true
		case c[0].ident("ellipse"):
		case c[0].ident("closest-side"):
			g.Size = render.ClosestSide
		case c[0].ident("closest-corner"):
			g.Size = render.ClosestCorner
		case c[0].ident("farthest-side"):
			g.Size = render.FarthestSide
		case c[0].ident("farthest-corner"):
			g.Size = render.FarthestCorner
		default:
			n, ok := evalNumeric(c, cx)
			if !ok || n.kind != numLength {
				return false
			}
			lengths = append(lengths, n.v)
		}
	}
	return g.radialLengths(lengths)
}

// radialLengths applies explicit radii: one for a circle, two for an
// ellipse.
func (g *Gradient) radialLengths(ls []float64) bool {
	switch len(ls) {
	case 0:
		return true
	case 1:
		g.Circle, g.Size, g.RadiusX, g.RadiusY = true, render.RadiusExplicit, ls[0], ls[0]
		return true
	case 2:
		if g.Circle {
			return false
		}
		g.Size, g.RadiusX, g.RadiusY = render.RadiusExplicit, ls[0], ls[1]
		return true
	}
	return false
}

// conicPrelude parses `[from <angle>] [at <position>]`.
func (g *Gradient) conicPrelude(first [][]token, cx *ctx) bool {
	for i := 0; i < len(first); i++ {
		switch {
		case first[i][0].ident("from") && i+1 < len(first):
			n, ok := evalNumeric(first[i+1], cx)
			if !ok || (n.kind != numAngle && (n.kind != numNumber || n.v != 0)) {
				return false
			}
			g.Angle = n.v
			i++
		case first[i][0].ident("at"):
			return g.position(first[i+1:], cx)
		default:
			return false
		}
	}
	return true
}

// position parses a center: one or two keywords or percentages (x
// before y, keywords in either order).
func (g *Gradient) position(cs [][]token, cx *ctx) bool {
	if len(cs) == 0 || len(cs) > 2 {
		return false
	}
	var xs, ys []float64
	var free []float64 // percentages and center, by order
	for _, c := range cs {
		switch {
		case c[0].ident("left"):
			xs = append(xs, 0)
		case c[0].ident("right"):
			xs = append(xs, 1)
		case c[0].ident("top"):
			ys = append(ys, 0)
		case c[0].ident("bottom"):
			ys = append(ys, 1)
		case c[0].ident("center"):
			free = append(free, 0.5)
		default:
			n, ok := evalNumeric(c, cx)
			if !ok || n.kind != numPercent {
				return false
			}
			free = append(free, n.v/100)
		}
	}
	for _, f := range free {
		if len(xs) == 0 {
			xs = append(xs, f)
		} else {
			ys = append(ys, f)
		}
	}
	if len(xs) > 1 || len(ys) > 1 {
		return false
	}
	if len(xs) == 1 {
		g.CenterX = xs[0]
	}
	if len(ys) == 1 {
		g.CenterY = ys[0]
	}
	return true
}

// stops parses the color-stop list (CSS Images 4): each a color with
// up to two positions, a double position standing for two stops of
// the color, and color hints — a lone position — between two stops.
// Positions are <length-percentage>s (angles and percentages in a
// conic gradient); the fixup that settles auto and backwards positions
// runs against the gradient line at paint (Gradient.Resolve).
func (g *Gradient) stops(args [][]token, cx *ctx) bool {
	n, colors := 0, 0
	add := func(s GradientStop) bool {
		if n == MaxStops {
			return false
		}
		g.Stops[n] = s
		n++
		return true
	}
	for i, a := range args {
		comps := components(a)
		if len(comps) == 1 {
			if hint, ok := g.stopPos(comps[0], cx); ok {
				if i == 0 || i == len(args)-1 || g.Stops[n-1].Hint {
					return false
				}
				hint.Hint = true
				if !add(hint) {
					return false
				}
				continue
			}
		}
		if len(comps) < 1 || len(comps) > 3 {
			return false
		}
		cv, ok := parseColor(comps[0], cx)
		if !ok {
			return false
		}
		col := cx.resolve(cv)
		colors++
		if len(comps) == 1 {
			if !add(GradientStop{Color: col}) {
				return false
			}
			continue
		}
		for _, c := range comps[1:] {
			s, ok := g.stopPos(c, cx)
			s.Color = col
			if !ok || !add(s) {
				return false
			}
		}
	}
	if colors < 2 || g.Stops[n-1].Hint {
		return false
	}
	g.N = n
	return true
}

// stopPos parses one stop or hint position: a percentage or length (a
// unitless zero too, and calc() mixing them) along a linear or radial
// gradient's line, a percentage or angle around a conic one.
func (g *Gradient) stopPos(ts []token, cx *ctx) (GradientStop, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok {
		return GradientStop{}, false
	}
	conic := g.Kind == render.GradientConic
	switch {
	case n.kind == numPercent:
		return GradientStop{Pos: n.v / 100, At: true}, true
	case n.kind == numNumber && n.v == 0:
		return GradientStop{At: true}, true
	case conic && n.kind == numAngle:
		return GradientStop{Pos: n.v / 360, At: true}, true
	case !conic && n.kind == numLength:
		return GradientStop{Px: n.v, At: true}, true
	case !conic && n.kind == numLengthPct:
		return GradientStop{Pos: n.pct / 100, Px: n.v, At: true}, true
	}
	return GradientStop{}, false
}

// parseBoxShadow parses `none` or a comma list of shadows: `inset? <x>
// <y> [<blur> [<spread>]] <color>?`, in any order of the inset keyword
// and the color relative to the lengths. An omitted color is
// currentColor.
func parseBoxShadow(ts []token, cx *ctx, v *Values) bool {
	return parseShadows(ts, cx, &v.Shadow, true)
}

// parseTextShadow parses the text-shadow list: box-shadow's layers
// without inset or spread, each two offsets, an optional blur and an
// optional color (currentColor by default).
func parseTextShadow(ts []token, cx *ctx, v *Values) bool {
	return parseShadows(ts, cx, &v.TextShadow, false)
}

// parseShadows parses a comma list of shadow layers, or none, into
// out. box admits box-shadow's inset keyword, spread length and lys-ui's
// legacy one-length `COLOR BLUR` glow; text-shadow takes neither.
func parseShadows(ts []token, cx *ctx, out *Shadows, box bool) bool {
	*out = Shadows{}
	nLayers := 0
	for range eachTop(ts, tkComma) {
		nLayers++
	}
	if t := trimWS(ts); nLayers == 1 && len(t) == 1 && t[0].ident("none") {
		return true
	}
	for layer := range eachTop(ts, tkComma) {
		var sh Shadow
		var lenBuf [4]int
		lens := lenBuf[:0]
		haveC := false
		for c := range eachComponent(layer) {
			if box && c[0].ident("inset") && !sh.Inset {
				sh.Inset = true
				continue
			}
			if l, ok := lengthOf(c, cx); ok {
				if len(lens) == cap(lens) {
					return false
				}
				lens = append(lens, px(l))
				continue
			}
			if !haveC {
				if cv, ok := parseColor(c, cx); ok {
					sh.Color, haveC = cx.resolve(cv), true
					continue
				}
			}
			return false
		}
		if !box && len(lens) > 3 {
			return false
		}
		if !haveC {
			// currentColor, read only when the color is omitted: it makes
			// the declaration node-dependent, which costs it the parse
			// cache.
			sh.Color = cx.cur()
		}
		switch len(lens) {
		case 1:
			// lys-ui's pre-CSS-3 one-shadow form, `COLOR BLUR`: a centered
			// glow. Kept for existing stylesheets; standard CSS always
			// carries two offsets.
			if !box || !haveC || lens[0] < 0 || nLayers != 1 {
				return false
			}
			sh.Blur = lens[0]
		case 4:
			sh.Spread = lens[3]
			fallthrough
		case 3:
			if lens[2] < 0 {
				return false
			}
			sh.Blur = lens[2]
			fallthrough
		case 2:
			sh.X, sh.Y = lens[0], lens[1]
		default:
			return false
		}
		if out.N == MaxShadows {
			warnf("shadow layers past %d dropped", MaxShadows)
			break
		}
		out.Layers[out.N] = sh
		out.N++
	}
	return true
}

// parseFontFamily keeps the head of the family list: the face the
// consumer shapes with (fallback runs are the font store's job).
func parseFontFamily(ts []token, _ *ctx, v *Values) bool {
	fams := splitTop(ts, tkComma)
	var head string
	for i, f := range fams {
		f = trimWS(f)
		if len(f) == 0 {
			return false
		}
		var name string
		if len(f) == 1 && f[0].kind == tkString {
			name = f[0].s
		} else {
			parts := make([]string, 0, len(f))
			for _, t := range f {
				switch t.kind {
				case tkIdent:
					parts = append(parts, t.s)
				case tkWS:
				default:
					return false
				}
			}
			name = strings.Join(parts, " ")
		}
		if i == 0 {
			head = name
		}
	}
	if head == "" {
		return false
	}
	v.FontFamily = head
	return true
}

// parseFontSize parses a length (em against the parent's size) or a
// keyword; percentages scale the parent's size.
func parseFontSize(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].kind == tkIdent {
		scale, ok := map[string]float64{
			"xx-small": 3.0 / 5, "x-small": 3.0 / 4, "small": 8.0 / 9, "medium": 1,
			"large": 6.0 / 5, "x-large": 3.0 / 2, "xx-large": 2, "xxx-large": 3, "smaller": 1 / 1.2, "larger": 1.2,
		}[strings.ToLower(comps[0][0].s)]
		if !ok {
			return false
		}
		cx.usedRem = true
		base := cx.rem
		if s := strings.ToLower(comps[0][0].s); s == "smaller" || s == "larger" {
			base = cx.emPx()
		}
		v.FontSize = base * scale
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok {
		return false
	}
	switch n.kind {
	case numLength, numNumber:
	case numPercent:
		n.v = cx.emPx() * n.v / 100
	default:
		return false
	}
	if n.v <= 0 || n.v > 1<<10 {
		return false
	}
	v.FontSize = n.v
	return true
}

// parseLineHeight parses `normal` (0, the font's own line box) or a
// length, percentage, or unitless number resolved against the computed
// font-size into pixels.
func parseLineHeight(ts []token, cx *ctx, v *Values) bool {
	// One component, iterated rather than collected: a number
	// resolves against each element's font-size, so this parses per
	// node.
	var comp []token
	n := 0
	for c := range eachComponent(ts) {
		comp = c
		n++
	}
	if n != 1 {
		return false
	}
	if comp[0].ident("normal") {
		v.LineHeight, v.LineHeightFactor = 0, 0
		return true
	}
	num, ok := evalNumeric(comp, cx)
	if !ok {
		return false
	}
	factor := 0.0
	switch num.kind {
	case numNumber:
		factor = num.v
		num.v = cx.emPx() * num.v
	case numPercent:
		num.v = cx.emPx() * num.v / 100
	case numLength:
	default:
		return false
	}
	if num.v < 0 || num.v > 1<<14 {
		return false
	}
	v.LineHeight, v.LineHeightFactor = num.v, factor
	return true
}

// parseTransform parses the full 2-D transform list: none, matrix,
// translate / translateX / translateY, scale / scaleX / scaleY,
// rotate, and skew / skewX / skewY, lengths resolving vars against the
// computed font size and angles taking deg, grad, rad, and turn. The
// functions compose left to right into the affine the widget paints
// through - exactly GTK's own set, lengths and all, since GTK CSS
// accepts no percentages in transforms.
func parseTransform(ts []token, cx *ctx, v *Values) bool {
	x, ok := composeTransform(ts, cx)
	if !ok {
		return false
	}
	v.Transform = x
	return true
}

// composeTransform walks one transform property's function list into
// an Xform: the composed affine plus the primitive list the tweens
// interpolate function by function. Each function maps points through
// the ones before it, the CSS order, so the composition is left
// multiplication.
func composeTransform(ts []token, cx *ctx) (Xform, bool) {
	x := XformIdentity
	comps := components(ts)
	if len(comps) == 1 && comps[0][0].ident("none") {
		return x, true
	}
	for _, comp := range comps {
		if len(comp) == 0 || comp[0].kind != tkFunc {
			return Xform{}, false
		}
		_, part, ok := transformPrim(comp[0].s, splitTop(funcArgs(comp), tkComma), cx)
		if !ok {
			return Xform{}, false
		}
		x.compose(part)
	}
	return x, true
}

// transformPrim builds one transform function's affine: the name (the
// tokenizer lowercases function names) over its comma-separated args.
// Length and number args resolve per slot; angle args are the
// functions' own.
func transformPrim(name string, args [][]token, cx *ctx) (render.Affine, XformPart, bool) {
	num := func(i int) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		comps := components(args[i])
		if len(comps) != 1 {
			return 0, false
		}
		n, ok := evalNumeric(comps[0], cx)
		if !ok {
			return 0, false
		}
		switch n.kind {
		case numLength, numNumber:
		default:
			return 0, false
		}
		return n.v, true
	}
	angle := func(i int) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		comps := components(args[i])
		if len(comps) != 1 {
			return 0, false
		}
		n, ok := evalNumeric(comps[0], cx)
		if !ok || (n.kind != numAngle && (n.kind != numNumber || n.v != 0)) {
			return 0, false
		}
		return n.v, true
	}
	switch name {
	case "scale":
		if len(args) == 0 || len(args) > 2 {
			return render.Affine{}, XformPart{}, false
		}
		sx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		sy := sx
		if len(args) == 2 {
			if sy, ok = num(1); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return render.Scale(sx, sy), XformPart{Op: XformScale, N: [6]float64{sx, sy}}, true
	case "scalex":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		sx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Scale(sx, 1), XformPart{Op: XformScale, N: [6]float64{sx, 1}}, true
	case "scaley":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		sy, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Scale(1, sy), XformPart{Op: XformScale, N: [6]float64{1, sy}}, true
	case "translate":
		if len(args) == 0 || len(args) > 2 {
			return render.Affine{}, XformPart{}, false
		}
		dx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		dy := 0.0
		if len(args) == 2 {
			if dy, ok = num(1); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return render.Translate(dx, dy), XformPart{Op: XformTranslate, N: [6]float64{dx, dy}}, true
	case "translatex":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		dx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Translate(dx, 0), XformPart{Op: XformTranslate, N: [6]float64{dx, 0}}, true
	case "translatey":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		dy, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Translate(0, dy), XformPart{Op: XformTranslate, N: [6]float64{0, dy}}, true
	case "rotate":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		deg, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Rotate(deg), XformPart{Op: XformRotate, N: [6]float64{deg}}, true
	case "skew":
		if len(args) == 0 || len(args) > 2 {
			return render.Affine{}, XformPart{}, false
		}
		x, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		y := 0.0
		if len(args) == 2 {
			if y, ok = angle(1); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return skewAffine(x, y), XformPart{Op: XformSkew, N: [6]float64{x, y}}, true
	case "skewx":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		x, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return skewAffine(x, 0), XformPart{Op: XformSkew, N: [6]float64{x, 0}}, true
	case "skewy":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		y, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return skewAffine(0, y), XformPart{Op: XformSkew, N: [6]float64{0, y}}, true
	case "matrix":
		if len(args) != 6 {
			return render.Affine{}, XformPart{}, false
		}
		var nums [6]float64
		for i := range nums {
			var ok bool
			if nums[i], ok = num(i); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return render.Affine{A: nums[0], B: nums[1], C: nums[2], D: nums[3], E: nums[4], F: nums[5]}, XformPart{Op: XformMatrix, N: nums}, true
	case "perspective":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		d, ok := num(0)
		if !ok || d <= 0 {
			return render.Affine{}, XformPart{}, false
		}
		return render.Identity, XformPart{Op: XformPerspective, N: [6]float64{d}}, true
	case "rotatex":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		deg, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Identity, XformPart{Op: XformRotateX, N: [6]float64{deg}}, true
	case "rotatey":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		deg, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Identity, XformPart{Op: XformRotateY, N: [6]float64{deg}}, true
	case "translatez":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		dz, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Identity, XformPart{Op: XformTranslateZ, N: [6]float64{dz}}, true
	}
	return render.Affine{}, XformPart{}, false
}

// skewAffine is the shear by the two angles: x leans by tan(ax), y by
// tan(ay).
func skewAffine(ax, ay float64) render.Affine {
	return render.Affine{
		A: 1,
		B: math.Tan(ay * math.Pi / 180),
		C: math.Tan(ax * math.Pi / 180),
		D: 1,
	}
}

// parseTransformOrigin parses transform-origin: one or two of the
// edge keywords, lengths, and percentages, lengths resolving against
// the computed font size. One value centers the other axis; keywords
// land in either order (left top == top left).
func parseTransformOrigin(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 || len(comps) > 2 {
		return false
	}
	var fx, fy, px, py float64
	seenX, seenY := false, false
	for _, comp := range comps {
		t := comp[0]
		if len(comp) != 1 {
			return false
		}
		if t.kind == tkIdent {
			switch strings.ToLower(t.s) {
			case "left", "right":
				if seenX {
					return false
				}
				seenX = true
				if strings.EqualFold(t.s, "left") {
					fx = 0
				} else {
					fx = 1
				}
				continue
			case "top", "bottom":
				if seenY {
					return false
				}
				seenY = true
				if strings.EqualFold(t.s, "top") {
					fy = 0
				} else {
					fy = 1
				}
				continue
			case "center":
				if !seenX {
					seenX, fx = true, 0.5
				} else if !seenY {
					seenY, fy = true, 0.5
				} else {
					return false
				}
				continue
			}
			return false
		}
		n, ok := evalNumeric(comp, cx)
		if !ok {
			return false
		}
		if n.kind == numNumber && n.v == 0 {
			n.kind = numLength
		}
		switch n.kind {
		case numLength:
			if !seenX {
				seenX, px = true, n.v
			} else if !seenY {
				seenY, py = true, n.v
			} else {
				return false
			}
		case numPercent:
			f := n.v / 100
			if !seenX {
				seenX, fx = true, f
			} else if !seenY {
				seenY, fy = true, f
			} else {
				return false
			}
		default:
			return false
		}
	}
	if !seenY {
		fy = 0.5
	}
	v.OriginFrac, v.OriginPx = [2]float64{fx, fy}, [2]float64{px, py}
	return true
}

// parseFontVariations parses `normal` or the axis list: each "tag" (a
// quoted four-character axis tag) and its number. The list lands in
// canonical sorted `tag=value` form so the comparable Values can hold
// it.
func parseFontVariations(ts []token, _ *ctx, v *Values) bool {
	if first := components(ts)[0]; first[0].ident("normal") {
		v.Variations = ""
		return true
	}
	var parts []string
	for _, comp := range splitTop(ts, tkComma) {
		comps := components(comp)
		if len(comps) != 2 {
			return false
		}
		tag := strings.Trim(comps[0][0].s, "\"'")
		if len(tag) != 4 {
			return false
		}
		n, ok := evalNumeric(comps[1], &ctx{})
		if !ok || n.kind != numNumber {
			return false
		}
		parts = append(parts, fmt.Sprintf("%s=%g", tag, n.v))
	}
	slices.Sort(parts)
	v.Variations = strings.Join(parts, ";")
	return true
}

// parseFontFeatures parses `normal` or the feature-tag list: each
// "tag" (a quoted or bare four-character tag), with an optional value
// (a number, or on/off) defaulting to 1. The list lands in canonical
// sorted `tag=value` form so the comparable Values can hold it.
func parseFontFeatures(ts []token, _ *ctx, v *Values) bool {
	if first := components(ts)[0]; first[0].ident("normal") {
		v.Features = ""
		return true
	}
	type fv struct {
		tag string
		val int
	}
	var feats []fv
	for _, comp := range splitTop(ts, tkComma) {
		comps := components(comp)
		if len(comps) == 0 {
			return false
		}
		tag := strings.ToLower(strings.Trim(comps[0][0].s, "\"'"))
		if len(comps[0][0].s) < 3 || len(tag) == 0 || len(tag) > 4 {
			return false
		}
		val := 1
		if len(comps) > 1 {
			switch {
			case comps[1][0].ident("on"):
			case comps[1][0].ident("off"):
				val = 0
			default:
				n, ok := evalNumeric(comps[1], &ctx{})
				if !ok || n.kind != numNumber {
					return false
				}
				val = int(math.Round(n.v))
			}
		}
		feats = append(feats, fv{fmt.Sprintf("%-4s", tag), val})
	}
	slices.SortFunc(feats, func(a, b fv) int { return strings.Compare(a.tag, b.tag) })
	parts := make([]string, len(feats))
	for i, f := range feats {
		parts[i] = fmt.Sprintf("%s=%d", f.tag, f.val)
	}
	v.Features = strings.Join(parts, ";")
	return true
}

// parseFontWeight parses normal/bold/bolder/lighter or 1..1000.
func parseFontWeight(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].kind == tkIdent {
		switch strings.ToLower(comps[0][0].s) {
		case "normal":
			v.FontWeight = 400
		case "bold":
			v.FontWeight = 700
		case "bolder":
			v.FontWeight = bolderWeight(cx.wt())
		case "lighter":
			v.FontWeight = lighterWeight(cx.wt())
		default:
			return false
		}
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numNumber || n.v < 1 || n.v > 1000 {
		return false
	}
	v.FontWeight = int(math.Round(n.v))
	return true
}

func parseFontStyle(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "normal":
		v.Italic = false
	case "italic", "oblique":
		v.Italic = true
	default:
		return false
	}
	return true
}

// bolderWeight and lighterWeight are the relative weights CSS Fonts 4
// §2.2 tabulates against the inherited weight.
func bolderWeight(w int) int {
	switch {
	case w < 350:
		return 400
	case w < 550:
		return 700
	}
	return 900
}

func lighterWeight(w int) int {
	switch {
	case w < 100:
		return w
	case w < 550:
		return 100
	case w < 750:
		return 400
	}
	return 700
}

// parseFont parses the font shorthand: [style] [weight] size
// [/ line-height] family. The omitted longhands reset to their
// initial values, line-height to normal.
func parseFont(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	v.Italic, v.FontWeight, v.LineHeight, v.LineHeightFactor = false, 400, 0, 0
	i := 0
	for ; i < len(comps); i++ {
		if parseFontStyle(comps[i], cx, v) && !comps[i][0].ident("normal") {
			continue
		}
		if parseFontWeight(comps[i], cx, v) {
			continue
		}
		break
	}
	if i >= len(comps) || !parseFontSize(comps[i], cx, v) {
		return false
	}
	if i+1 < len(comps) && len(comps[i+1]) == 1 && comps[i+1][0].is('/') {
		if i+2 >= len(comps) || !parseLineHeight(comps[i+2], cx, v) {
			return false
		}
		i += 2
	}
	// The family is everything after the size (and line-height).
	var fam []token
	for j, t := range ts {
		if t.pos >= comps[i][len(comps[i])-1].end {
			fam = ts[j:]
			break
		}
	}
	return parseFontFamily(trimWS(fam), cx, v)
}

func parseLetterSpacing(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].ident("normal") {
		v.LetterSpacing = 0
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numLength && n.kind != numNumber {
		return false
	}
	v.LetterSpacing = n.v
	return true
}

func parseTextTransform(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "none":
		v.TextTransform = TransformNone
	case "uppercase":
		v.TextTransform = TransformUppercase
	case "lowercase":
		v.TextTransform = TransformLowercase
	case "capitalize":
		v.TextTransform = TransformCapitalize
	default:
		return false
	}
	return true
}

// identOf returns comps' single identifier, lowercased, if the value
// is exactly one identifier.
func identOf(comps [][]token) (string, bool) {
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return "", false
	}
	return strings.ToLower(comps[0][0].s), true
}

// parseFontPalette parses font-palette: normal, light or dark.
func parseFontPalette(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.Palette = PaletteNormal
	case "light":
		v.Palette = PaletteLight
	case "dark":
		v.Palette = PaletteDark
	default:
		return false
	}
	return true
}

// parseOverflowWrap parses overflow-wrap (and word-wrap): normal,
// break-word or anywhere.
func parseOverflowWrap(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.OverflowWrap = OverflowWrapNormal
	case "break-word":
		v.OverflowWrap = OverflowWrapBreakWord
	case "anywhere":
		v.OverflowWrap = OverflowWrapAnywhere
	default:
		return false
	}
	return true
}

// parseTextAlign parses text-align: start, end, left, right, center,
// justify (and GTK's fill, which reads as justify).
func parseTextAlign(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "start":
		v.TextAlign = AlignStart
	case "end":
		v.TextAlign = AlignEnd
	case "left":
		v.TextAlign = AlignLeft
	case "right":
		v.TextAlign = AlignRight
	case "center":
		v.TextAlign = AlignCenter
	case "justify", "fill":
		v.TextAlign = AlignJustify
	default:
		return false
	}
	return true
}

// parseTextIndent parses text-indent: a length (percentages need the
// containing block, which the style pass never sees, so they are
// invalid here).
func parseTextIndent(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numLength && n.kind != numNumber {
		return false
	}
	v.TextIndent = n.v
	return true
}

// parseWhiteSpace parses white-space: normal, pre, nowrap,
// pre-wrap, pre-line, break-spaces.
func parseWhiteSpace(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.WhiteSpace = WhiteSpaceNormal
	case "pre":
		v.WhiteSpace = WhiteSpacePre
	case "nowrap":
		v.WhiteSpace = WhiteSpaceNowrap
	case "pre-wrap":
		v.WhiteSpace = WhiteSpacePreWrap
	case "pre-line":
		v.WhiteSpace = WhiteSpacePreLine
	case "break-spaces":
		v.WhiteSpace = WhiteSpaceBreakSpaces
	default:
		return false
	}
	return true
}

// parseWordBreak parses word-break: normal, break-all, keep-all.
func parseWordBreak(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.WordBreak = WordBreakNormal
	case "break-all":
		v.WordBreak = WordBreakBreakAll
	case "keep-all":
		v.WordBreak = WordBreakKeepAll
	default:
		return false
	}
	return true
}

// parseLineBreak parses line-break: auto, loose, normal, strict,
// anywhere.
func parseLineBreak(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "auto":
		v.LineBreak = LineBreakAuto
	case "loose":
		v.LineBreak = LineBreakLoose
	case "normal":
		v.LineBreak = LineBreakNormal
	case "strict":
		v.LineBreak = LineBreakStrict
	case "anywhere":
		v.LineBreak = LineBreakAnywhere
	default:
		return false
	}
	return true
}

// parseHyphens parses hyphens: none, manual, auto.
func parseHyphens(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "none":
		v.Hyphens = HyphensNone
	case "manual":
		v.Hyphens = HyphensManual
	case "auto":
		v.Hyphens = HyphensAuto
	default:
		return false
	}
	return true
}

// parseWordSpacing parses word-spacing: normal or a length.
// Percentages resolve against the computed line-height.
func parseWordSpacing(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].ident("normal") {
		v.WordSpacing = 0
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numLength && n.kind != numNumber {
		return false
	}
	v.WordSpacing = n.v
	return true
}

// parseVerticalAlign parses vertical-align: baseline, sub, super,
// text-top, text-bottom, top, middle, bottom, or a length/percentage
// (against the computed line-height).
func parseVerticalAlign(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].kind == tkIdent {
		switch strings.ToLower(comps[0][0].s) {
		case "baseline":
			v.VerticalAlign = VerticalAlignBaseline
		case "sub":
			v.VerticalAlign = VerticalAlignSub
		case "super":
			v.VerticalAlign = VerticalAlignSuper
		case "text-top":
			v.VerticalAlign = VerticalAlignTextTop
		case "text-bottom":
			v.VerticalAlign = VerticalAlignTextBottom
		case "top":
			v.VerticalAlign = VerticalAlignTop
		case "middle":
			v.VerticalAlign = VerticalAlignMiddle
		case "bottom":
			v.VerticalAlign = VerticalAlignBottom
		default:
			return false
		}
		v.VerticalAlignPx = 0
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok {
		return false
	}
	switch n.kind {
	case numNumber, numLength:
		v.VerticalAlign, v.VerticalAlignPx, v.VerticalAlignPct = VerticalAlignLength, n.v, false
	case numPercent:
		// A fraction of the line box, resolved by the consumer: the
		// parse cache is per declaration, so it cannot see this node's
		// line-height.
		v.VerticalAlign, v.VerticalAlignPx, v.VerticalAlignPct = VerticalAlignLength, n.v/100, true
	default:
		return false
	}
	return true
}

// parseLineClamp parses line-clamp (and -webkit-line-clamp): none or a
// positive integer.
func parseLineClamp(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].ident("none") {
		v.LineClamp = 0
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numNumber || n.v < 1 || n.v != math.Trunc(n.v) {
		return false
	}
	v.LineClamp = int(n.v)
	return true
}

// parseTextOverflow parses text-overflow: clip or ellipsis.
func parseTextOverflow(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "clip":
		v.Overflow = TextOverflowClip
	case "ellipsis":
		v.Overflow = TextOverflowEllipsis
	default:
		return false
	}
	return true
}

// parseFontVariantCaps parses font-variant-caps.
func parseFontVariantCaps(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.VariantCaps = CapsNormal
	case "small-caps":
		v.VariantCaps = CapsSmallCaps
	case "all-small-caps":
		v.VariantCaps = CapsAllSmallCaps
	case "petite-caps":
		v.VariantCaps = CapsPetiteCaps
	case "all-petite-caps":
		v.VariantCaps = CapsAllPetiteCaps
	case "unicase":
		v.VariantCaps = CapsUnicase
	case "titling-caps":
		v.VariantCaps = CapsTitlingCaps
	default:
		return false
	}
	return true
}

// keywordFlags parses a space-separated keyword list into a flag set:
// every keyword at most once, and at most one keyword from each
// exclusive group (a mask of the flags that exclude each other).
func keywordFlags[F ~uint8 | ~uint16](comps [][]token, words map[string]F, groups ...F) (F, bool) {
	var flags F
	if len(comps) == 0 {
		return 0, false
	}
	for _, c := range comps {
		s, ok := identOf([][]token{c})
		if !ok {
			return 0, false
		}
		f, ok := words[s]
		if !ok || flags&f != 0 {
			return 0, false
		}
		for _, g := range groups {
			if g&f != 0 && flags&g != 0 {
				return 0, false
			}
		}
		flags |= f
	}
	return flags, true
}

// keywordFlagsOrNormal is keywordFlags with the lone `normal` keyword
// meaning the empty set.
func keywordFlagsOrNormal[F ~uint8 | ~uint16](comps [][]token, words map[string]F, groups ...F) (F, bool) {
	if s, ok := identOf(comps); ok && s == "normal" {
		return 0, true
	}
	return keywordFlags(comps, words, groups...)
}

// The font-variant list longhands' keywords.
var (
	numericWords = map[string]FontVariantNumeric{
		"lining-nums": NumLining, "oldstyle-nums": NumOldStyle,
		"proportional-nums": NumProportional, "tabular-nums": NumTabular,
		"diagonal-fractions": NumDiagonalFractions, "stacked-fractions": NumStackedFractions,
		"ordinal": NumOrdinal, "slashed-zero": NumSlashedZero,
	}
	ligatureWords = map[string]FontVariantLigatures{
		"common-ligatures": LigCommon, "no-common-ligatures": LigNoCommon,
		"discretionary-ligatures": LigDiscretionary, "no-discretionary-ligatures": LigNoDiscretionary,
		"historical-ligatures": LigHistorical, "no-historical-ligatures": LigNoHistorical,
		"contextual": LigContextual, "no-contextual": LigNoContextual,
	}
	eastAsianWords = map[string]FontVariantEastAsian{
		"jis78": EAJis78, "jis83": EAJis83, "jis90": EAJis90, "jis04": EAJis04,
		"simplified": EASimplified, "traditional": EATraditional,
		"full-width": EAFullWidth, "proportional-width": EAProportional, "ruby": EARuby,
	}
)

// parseFontVariantNumeric parses font-variant-numeric: normal, or at
// most one of each figure, spacing and fraction pair plus ordinal and
// slashed-zero.
func parseFontVariantNumeric(ts []token, _ *ctx, v *Values) bool {
	f, ok := keywordFlagsOrNormal(components(ts), numericWords,
		NumLining|NumOldStyle, NumProportional|NumTabular, NumDiagonalFractions|NumStackedFractions)
	if ok {
		v.VariantNumeric = f
	}
	return ok
}

// parseFontVariantLigatures parses font-variant-ligatures: normal,
// none (every ligature group off), or at most one of each group's on
// and off keywords.
func parseFontVariantLigatures(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if s, ok := identOf(comps); ok && s == "none" {
		v.VariantLig = LigNoCommon | LigNoDiscretionary | LigNoHistorical | LigNoContextual
		return true
	}
	f, ok := keywordFlagsOrNormal(comps, ligatureWords, LigCommon|LigNoCommon,
		LigDiscretionary|LigNoDiscretionary, LigHistorical|LigNoHistorical, LigContextual|LigNoContextual)
	if ok {
		v.VariantLig = f
	}
	return ok
}

// parseFontVariantEastAsian parses font-variant-east-asian: normal,
// or at most one variant form, at most one width, and ruby.
func parseFontVariantEastAsian(ts []token, _ *ctx, v *Values) bool {
	f, ok := keywordFlagsOrNormal(components(ts), eastAsianWords,
		EAJis78|EAJis83|EAJis90|EAJis04|EASimplified|EATraditional, EAFullWidth|EAProportional)
	if ok {
		v.VariantEA = f
	}
	return ok
}

// parseFontVariantPosition parses font-variant-position.
func parseFontVariantPosition(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.VariantPos = PositionNormal
	case "sub":
		v.VariantPos = PositionSub
	case "super":
		v.VariantPos = PositionSuper
	default:
		return false
	}
	return true
}

// parseFontVariantAlternates parses font-variant-alternates: normal
// or historical-forms (the functional forms need per-glyph knowledge
// the font stack does not expose).
func parseFontVariantAlternates(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.VariantAlt = AlternatesNormal
	case "historical-forms":
		v.VariantAlt = AlternatesHistorical
	default:
		return false
	}
	return true
}

// fontVariantParsers are the font-variant shorthand's longhand parsers,
// in the order the shorthand offers a keyword to them.
var fontVariantParsers = [...]func([]token, *ctx, *Values) bool{
	parseFontVariantCaps, parseFontVariantNumeric, parseFontVariantLigatures,
	parseFontVariantEastAsian, parseFontVariantPosition, parseFontVariantAlternates,
}

// parseFontVariant parses the font-variant shorthand: normal, none
// (ligatures off, the rest normal), or a mix of the longhands'
// keywords — each keyword lands on the longhand that takes it, the
// list longhands gathering theirs, and every longhand left out
// resets to normal.
func parseFontVariant(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 {
		return false
	}
	var out Values
	if s, ok := identOf(comps); ok && (s == "normal" || s == "none") {
		if s == "none" {
			out.VariantLig = LigNoCommon | LigNoDiscretionary | LigNoHistorical | LigNoContextual
		}
		v.VariantCaps, v.VariantNumeric, v.VariantLig = out.VariantCaps, out.VariantNumeric, out.VariantLig
		v.VariantEA, v.VariantPos, v.VariantAlt = out.VariantEA, out.VariantPos, out.VariantAlt
		return true
	}
	groups := make([][]token, len(fontVariantParsers))
	for _, c := range comps {
		if s, ok := identOf([][]token{c}); !ok || s == "normal" || s == "none" {
			return false
		}
		owned := false
		for i, parse := range fontVariantParsers {
			var probe Values
			if parse(c, cx, &probe) {
				groups[i], owned = append(groups[i], c...), true
				break
			}
		}
		if !owned {
			return false
		}
	}
	for i, parse := range fontVariantParsers {
		if len(groups[i]) > 0 && !parse(groups[i], cx, &out) {
			return false
		}
	}
	v.VariantCaps, v.VariantNumeric, v.VariantLig = out.VariantCaps, out.VariantNumeric, out.VariantLig
	v.VariantEA, v.VariantPos, v.VariantAlt = out.VariantEA, out.VariantPos, out.VariantAlt
	return true
}

// parseFontSynthesis parses font-synthesis: none or weight, style,
// small-caps.
func parseFontSynthesis(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 {
		return false
	}
	var flags FontSynthesis
	for _, c := range comps {
		s, ok := identOf([][]token{c})
		if !ok {
			return false
		}
		switch s {
		case "none":
			flags = 0
		case "weight":
			flags |= SynthWeight
		case "style":
			flags |= SynthStyle
		case "small-caps":
			flags |= SynthSmallCaps
		default:
			return false
		}
	}
	v.Synthesis = flags
	return true
}

// parseFontOpticalSizing parses font-optical-sizing: auto or none.
func parseFontOpticalSizing(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "auto":
		v.OpticalSizing = true
	case "none":
		v.OpticalSizing = false
	default:
		return false
	}
	return true
}

// parseDirection parses direction: ltr or rtl.
func parseDirection(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "ltr":
		v.Direction = 1
	case "rtl":
		v.Direction = 2
	default:
		return false
	}
	return true
}

// parseUnicodeBidi parses unicode-bidi.
func parseUnicodeBidi(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "normal":
		v.UnicodeBidi = BidiNormal
	case "embed":
		v.UnicodeBidi = BidiEmbed
	case "isolate":
		v.UnicodeBidi = BidiIsolate
	case "bidi-override":
		v.UnicodeBidi = BidiOverride
	case "isolate-override":
		v.UnicodeBidi = BidiIsolateOverride
	case "plaintext":
		v.UnicodeBidi = BidiPlaintext
	default:
		return false
	}
	return true
}

// parseWritingMode parses writing-mode: horizontal-tb, vertical-rl,
// vertical-lr.
func parseWritingMode(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	if !ok {
		return false
	}
	switch s {
	case "horizontal-tb":
		v.WritingMode = WritingHorizontalTB
	case "vertical-rl":
		v.WritingMode = WritingVerticalRL
	case "vertical-lr":
		v.WritingMode = WritingVerticalLR
	default:
		return false
	}
	return true
}

// timeOf reads a time value in seconds.
func timeOf(ts []token, cx *ctx) (float64, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok || n.kind != numTime && !(n.kind == numNumber && n.v == 0) {
		return 0, false
	}
	return n.v, true
}
