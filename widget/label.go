package widget

import (
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/text"

	"github.com/stubbedev/lys-ui/internal/style"
)

// EllipsizeMode names where an overflowing label truncates. It is
// render.EllipsizeMode; the alias lets label call sites read against
// the widget package.
type EllipsizeMode = render.EllipsizeMode

const (
	// EllipsizeNone never truncates: long text overflows and clips.
	EllipsizeNone = render.EllipsizeNone
	// EllipsizeStart cuts from the front: "…cated text".
	EllipsizeStart = render.EllipsizeStart
	// EllipsizeMiddle keeps the head and the tail: "trunc…text" - the
	// mode for paths and filenames.
	EllipsizeMiddle = render.EllipsizeMiddle
	// EllipsizeEnd cuts from the back: "truncated te…".
	EllipsizeEnd = render.EllipsizeEnd
)

// Label paints text. It is passive: it hits as itself but has no
// interaction behavior.
//
// The default is one line whose text overflows a narrow arrangement and
// clips. SetWrap turns on word wrapping at the width it is given -
// Measure then reports one line height per wrapped row - and SetEllipsize
// truncates with an ellipsis instead, at the start, middle, or end.
// With both on, wrapping fills rows and the ellipsis applies to the
// final row only (GTK's rule): a lone unbreakable token left
// overflowing there truncates rather than spilling. A label too small
// for its text is the classic tooltip case: keep the full text in
// SetTooltip and let the mode shorten what paints.
type Label struct {
	node
	face render.Font
	// faces memoizes the cascade-styled face (styledFace).
	faces  faceCache
	text   string
	sizePx float64
	color  render.Color
	align  render.Alignment
	dir    Direction
	wrap   bool
	ell    EllipsizeMode
	// maxChars caps the natural width at that many approximate
	// character widths (GTK max-width-chars); 0 is uncapped.
	maxChars int
	// widthChars floors the natural width at that many approximate
	// character widths (GTK width-chars); 0 is unset.
	widthChars int
	// maxLines caps a wrapping, ellipsizing label at that many rows
	// (GTK lines); 0 is uncapped.
	maxLines int
	shaped   *render.ShapedText
}

// NewLabel returns a label that paints text with face at sizePx
// pixels in color. Face may be a render.Chain for mixed-script
// fallback. Constructor argument order is the toolkit-wide one: face,
// then sizePx, then the content, then colors. A nil face panics here
// (see requireFace) instead of failing later, in shaping.
func NewLabel(face render.Font, sizePx float64, text string, color render.Color) *Label {
	l := &Label{face: requireFace("widget.NewLabel", face), sizePx: sizePx, text: text, color: color}
	l.retext()
	return l
}

// SetText replaces the label text and invalidates the label's bounds.
func (l *Label) SetText(text string) {
	if l.text == text {
		return
	}
	l.text = text
	l.retext()
	l.InvalidateLayout()
}

// Text returns the current label text.
func (l *Label) Text() string {
	return l.text
}

// SetFace swaps the shaping face (a fallback chain works here too);
// the shaped caches drop and the bounds invalidate.
func (l *Label) SetFace(face render.Font) {
	if l.face == face {
		return
	}
	l.face = requireFace("(*Label).SetFace", face)
	l.retext()
	l.InvalidateLayout()
}

// SizePx returns the shaping size in logical pixels.
func (l *Label) SizePx() float64 { return l.sizePx }

// SetSizePx changes the shaping size; the shaped caches drop and the
// bounds invalidate.
func (l *Label) SetSizePx(px float64) {
	if l.sizePx == px {
		return
	}
	l.sizePx = px
	l.retext()
	l.InvalidateLayout()
}

// SetAlignment selects horizontal placement when the arranged rect is
// wider than the text; wrapped rows align individually.
func (l *Label) SetAlignment(a render.Alignment) {
	if l.align == a {
		return
	}
	l.align = a
	l.Invalidate()
}

// Alignment returns the horizontal placement.
func (l *Label) Alignment() render.Alignment { return l.align }

// SetDirection selects the base paragraph direction the text resolves
// and lays out with. DirectionAuto (the default) reads it off the
// text's first strong character, so a Hebrew- or Arabic-first label
// mirrors on its own; DirectionLTR and DirectionRTL force the base,
// and start/end alignment mirrors with it. Changing the direction
// re-resolves the text and drops the measure cache.
func (l *Label) SetDirection(d Direction) {
	if l.dir == d {
		return
	}
	l.dir = d
	l.retext()
	l.InvalidateLayout()
}

// Direction returns the base paragraph direction the label resolves
// with.
func (l *Label) Direction() Direction { return l.dir }

// SetWrap toggles word wrapping at the width the label is given.
// Measure then needs only the widest word and reports one line height
// per row at a width, breaking at
// Unicode UAX #14 opportunities (spaces, hyphens, CJK); a run with no
// opportunity before the width - an unbreakable token - keeps its own
// row and overflows, clipped, never disappearing. Changing the mode
// drops the measure cache.
func (l *Label) SetWrap(on bool) {
	if l.wrap == on {
		return
	}
	l.wrap = on
	l.InvalidateLayout()
}

// Wrap reports whether the label wraps at the width it is given.
func (l *Label) Wrap() bool { return l.wrap }

// SetEllipsize selects where an overflowing line truncates with an
// ellipsis: at the end, at the start, in the middle (paths and
// filenames), or not at all. With wrap on, the ellipsis applies to the
// final row only; otherwise it applies to the single line. The label's
// text is never rewritten - pair the mode with SetTooltip to keep the
// whole string reachable. Changing the mode drops the measure cache.
func (l *Label) SetEllipsize(mode EllipsizeMode) {
	if l.ell == mode {
		return
	}
	l.ell = mode
	l.InvalidateLayout()
}

// Ellipsize returns the label's truncation mode.
func (l *Label) Ellipsize() EllipsizeMode { return l.ell }

// SetMaxWidthChars caps the label's natural width at n approximate
// character widths (GTK's max-width-chars): longer text overflows the
// cap and truncates by the ellipsize mode. n <= 0 removes the cap.
func (l *Label) SetMaxWidthChars(n int) {
	n = max(n, 0)
	if l.maxChars == n {
		return
	}
	l.maxChars = n
	l.InvalidateLayout()
}

// MaxWidthChars returns the width cap in characters, 0 when uncapped.
func (l *Label) MaxWidthChars() int { return l.maxChars }

// SetMaxLines limits a wrapping, ellipsizing label to n rows (GTK's
// set_lines): the text past row n folds into the final row, which the
// ellipsize mode truncates. Like GTK it has no effect unless the label
// both wraps and ellipsizes. n <= 0 removes the limit.
func (l *Label) SetMaxLines(n int) {
	n = max(n, 0)
	if l.maxLines == n {
		return
	}
	l.maxLines = n
	l.InvalidateLayout()
}

// MaxLines returns the row limit, 0 when unlimited.
func (l *Label) MaxLines() int { return l.maxLines }

// SetWidthChars floors the label's natural width at n approximate
// character widths (GTK's width-chars): a shorter text still measures
// that wide, so labels of one row line up. n <= 0 restores the text's
// own width.
func (l *Label) SetWidthChars(n int) {
	n = max(n, 0)
	if l.widthChars == n {
		return
	}
	l.widthChars = n
	l.InvalidateLayout()
}

// WidthChars returns the natural-width floor in characters, 0 when
// unset.
func (l *Label) WidthChars() int { return l.widthChars }

// approxCharSample stands in for pango's per-language sample text: the
// approximate character width is its mean advance.
const approxCharSample = "abcdefghijklmnopqrstuvwxyz0123456789"

// charsWidth is n approximate character widths in pixels: the
// width-chars and max-width-chars unit.
func (l *Label) charsWidth(n int) int {
	face, px := l.effStyle()
	return int(math.Ceil(approxCharPx(face, px) * float64(n)))
}

// CharsWithin is how many approximate character widths fit in px, the
// max-width-chars that holds the label's natural width to px.
func (l *Label) CharsWithin(px int) int {
	face, size := l.effStyle()
	return int(float64(px) / approxCharPx(face, size))
}

// approxCharPx is face's approximate character width at px: the mean
// advance of approxCharSample.
func approxCharPx(face render.Font, px float64) float64 {
	return face.Shape(approxCharSample, px).Advance() / float64(len(approxCharSample))
}

// effStyleIn resolves the paint parameters one cascade value implies:
// the face (a font-family/font-weight declaration shaped through the
// installed face resolver, else the constructor face - a variable one
// taking the weight on its wght axis), its font-variation-settings and
// tnum twin (styledFace), and the size (font-size, else the
// constructor size).
func (l *Label) effStyleIn(v *style.Values) (render.Font, float64) {
	px := fontPx(v, l.sizePx)
	face, weight := l.face, 0
	if v.Has(style.PropFontWeight) {
		weight = v.FontWeight
	}
	if v.Has(style.PropFontFamily) || weight != 0 {
		family := ""
		if v.Has(style.PropFontFamily) {
			family = v.FontFamily
		}
		if f, ok := resolveFace(family, weight, l.face); ok {
			face, weight = f, 0 // the resolver served the weight
		}
	}
	return l.faces.get(face, v, weight, int(px)), px
}

// effStyle is effStyleIn over the widget's current cascade.
func (l *Label) effStyle() (render.Font, float64) { return l.effStyleIn(l.style(l)) }

// styleRestyled implements styleRestyler: a changed effective face,
// size, text transform or white-space collapsing reshapes the cached
// run and drops the measure cache.
func (l *Label) styleRestyled(old, new style.Values) {
	of, opx := l.effStyleIn(&old)
	nf, npx := l.effStyleIn(&new)
	if of != nf || opx != npx || old.TextTransform != new.TextTransform ||
		old.Has(style.PropWhiteSpace) != new.Has(style.PropWhiteSpace) || old.WhiteSpace != new.WhiteSpace ||
		l.effDirection(&old) != l.effDirection(&new) {
		l.retext()
		l.InvalidateLayout()
	}
}

// textColor is the ink: the programmatic color when set, else the
// stylesheet's (direct or inherited), else the constructor value —
// zero paints nothing, the no-stylesheet behavior.
func (l *Label) textColor(v *style.Values) render.Color {
	return pickc(l.color, v, style.PropColor, inkOr(l.color))
}

// shown is the text as painted: the stylesheet's text-transform
// applied (uppercase, lowercase, capitalize), then a declared
// white-space's collapsing.
func (l *Label) shown() string {
	v := l.style(l)
	return collapseSpaces(transformText(l.text, v.TextTransform), v)
}

// collapseSpaces applies a declared white-space's collapsing (CSS
// Text 3 §4.1): normal and nowrap collapse spaces and newlines,
// pre-line spaces only; the preserving values, and an undeclared
// white-space (the toolkit's text as written), keep s.
func collapseSpaces(s string, v *style.Values) string {
	if !v.Has(style.PropWhiteSpace) {
		return s
	}
	switch v.WhiteSpace {
	case style.WhiteSpaceNormal, style.WhiteSpaceNowrap:
		return render.CollapseWhiteSpace(s, true)
	case style.WhiteSpacePreLine:
		return render.CollapseWhiteSpace(s, false)
	}
	return s
}

// effDirection is the base paragraph direction the label resolves
// with: the CSS direction (and unicode-bidi's overrides, which force
// the base the same way) over the programmatic one.
func (l *Label) effDirection(v *style.Values) Direction {
	if v.Has(style.PropDirection) {
		if v.Direction == 2 {
			return DirectionRTL
		}
		return DirectionLTR
	}
	if v.Has(style.PropUnicodeBidi) && (v.UnicodeBidi == style.BidiOverride || v.UnicodeBidi == style.BidiIsolateOverride) {
		if l.shaped != nil && l.shaped.RTL() {
			return DirectionRTL
		}
		return DirectionLTR
	}
	return l.dir
}

// alignOf is the horizontal placement: the CSS text-align over the
// programmatic alignment, left and right absolute against the
// resolved direction (start and end mirror with it).
func (l *Label) alignOf(v *style.Values) render.Alignment {
	if !v.Has(style.PropTextAlign) {
		return l.align
	}
	rtl := l.shaped != nil && l.shaped.RTL()
	switch v.TextAlign {
	case style.AlignEnd:
		return render.AlignEnd
	case style.AlignLeft:
		if rtl {
			return render.AlignEnd
		}
		return render.AlignStart
	case style.AlignRight:
		if rtl {
			return render.AlignStart
		}
		return render.AlignEnd
	case style.AlignCenter:
		return render.AlignCenter
	default: // start, and justify on a single line
		return render.AlignStart
	}
}

// justifies reports whether wrapped rows distribute their leftover
// width across the spaces (text-align justify: everything but the
// final row).
func (l *Label) justifies(v *style.Values) bool {
	return v.Has(style.PropTextAlign) && v.TextAlign == style.AlignJustify
}

// wrapOf is whether rows wrap at the width given: white-space
// nowrap and pre turn wrapping off, pre-wrap, pre-line and
// break-spaces on; normal keeps the programmatic mode.
func (l *Label) wrapOf(v *style.Values) bool {
	if v.Has(style.PropLineClamp) && v.LineClamp > 0 {
		return true
	}
	if !v.Has(style.PropWhiteSpace) {
		return l.wrap
	}
	switch v.WhiteSpace {
	case style.WhiteSpaceNowrap, style.WhiteSpacePre:
		return false
	}
	return true
}

// multiline reports whether the label lays out rows rather than one
// line: it wraps, or a white-space that preserves newlines keeps one.
func (l *Label) multiline(v *style.Values) bool {
	if l.wrapOf(v) {
		return true
	}
	if !v.Has(style.PropWhiteSpace) || v.WhiteSpace == style.WhiteSpaceNormal || v.WhiteSpace == style.WhiteSpaceNowrap {
		return false
	}
	return strings.Contains(l.shown(), "\n")
}

// ellOf is the truncation mode: line-clamp implies an end ellipsis,
// text-overflow ellipsis takes end when nothing stronger is set, and
// clip clears a stylesheet ellipsis.
func (l *Label) ellOf(v *style.Values) EllipsizeMode {
	if v.Has(style.PropLineClamp) && v.LineClamp > 0 {
		return EllipsizeEnd
	}
	if v.Has(style.PropTextOverflow) {
		if v.Overflow == style.TextOverflowEllipsis {
			if l.ell == EllipsizeNone {
				return EllipsizeEnd
			}
			return l.ell
		}
		return EllipsizeNone
	}
	return l.ell
}

// maxLinesOf is the row cap: CSS line-clamp over GTK's lines.
func (l *Label) maxLinesOf(v *style.Values) int {
	if v.Has(style.PropLineClamp) && v.LineClamp > 0 {
		return v.LineClamp
	}
	return l.maxLines
}

// wrapOpts is the CSS wrap configuration the breaker takes: word-break,
// hyphens (manual, the initial value, when undeclared), line-break
// anywhere, overflow-wrap, the white-space family's space
// preservation, and the first row's text-indent.
func (l *Label) wrapOpts(v *style.Values) render.WrapOptions {
	var o render.WrapOptions
	switch v.WordBreak {
	case style.WordBreakBreakAll:
		o.Break = render.WrapWordBreakAll
	case style.WordBreakKeepAll:
		o.Break = render.WrapWordKeepAll
	}
	switch {
	case !v.Has(style.PropHyphens), v.Hyphens == style.HyphensManual:
		o.Hyphens = render.HyphensManual
	case v.Hyphens == style.HyphensAuto:
		o.Hyphens = render.HyphensAuto
	}
	o.Anywhere = v.LineBreak == style.LineBreakAnywhere
	o.BreakWords = v.OverflowWrap != style.OverflowWrapNormal
	switch v.WhiteSpace {
	case style.WhiteSpacePre, style.WhiteSpacePreWrap:
		o.PreserveSpaces = true
	case style.WhiteSpaceBreakSpaces:
		o.PreserveSpaces, o.BreakSpaces = true, true
	}
	o.Indent = l.indentOf(v)
	return o
}

// minContentOpts is the breaking that sets the min-content width: every
// soft wrap opportunity, with overflow-wrap's emergency breaks counting
// only under anywhere (CSS Text 3 §5.5) and no indent.
func (l *Label) minContentOpts(v *style.Values) render.WrapOptions {
	o := l.wrapOpts(v)
	o.BreakWords = v.OverflowWrap == style.OverflowWrapAnywhere
	o.Indent = 0
	return o
}

// indentOf is the first row's indent (text-indent) at the start edge.
func (l *Label) indentOf(v *style.Values) float64 {
	if !v.Has(style.PropTextIndent) {
		return 0
	}
	return v.TextIndent
}

// retext re-resolves and reshapes the run under the label's base
// direction and refreshes the cached natural size.
func (l *Label) retext() {
	face, px := l.effStyle()
	l.shaped = face.ShapeDir(l.shown(), px, l.effDirection(l.style(l)))
}

// lineBox is the line height the label lays text out at: the CSS
// line-height when the style sets one - a unitless number resolved
// against the size the label paints at, which a cascade without a
// font-size does not know - else the shaped line's natural ascent plus
// descent.
func (l *Label) lineBox(v *style.Values) int {
	if v.Has(style.PropLineHeight) && v.LineHeightFactor > 0 {
		_, px := l.effStyleIn(v)
		return int(math.Ceil(v.LineHeightFactor * px))
	}
	if lh := pickf(v, style.PropLineHeight, 0); lh > 0 {
		return int(math.Ceil(lh))
	}
	return l.shaped.LineHeight()
}

// Measure reports the text's request inside the CSS box (padding,
// border, margin, min sizes), GTK's label measure. Across: its natural
// width is the text unwrapped; its minimum is the ellipsis when it
// ellipsizes, else the widest word when it wraps, else the whole text,
// so it overflows rather than clip; max-width-chars holds
// the natural width of one that can narrow, and width-chars floors
// both. Down: the rows it lays out
// at the width it is given, or at its natural width before any. Widths
// round up: Paint ellipsizes (and wraps) against the raw advance, so a
// width rounded to nearest would truncate a text laid out at exactly
// its natural size.
func (l *Label) Measure(axis Axis, forSize int) Sizes {
	if s, ok := l.measureHit(axis, forSize); ok {
		return s
	}
	v := l.style(l)
	return l.measureStore(axis, forSize, measureBox(v, boxOf(v, render.Insets{}), axis, forSize, l.textRequest))
}

// textRequest is the content box's request along axis at forSize.
func (l *Label) textRequest(axis Axis, forSize int) Sizes {
	v := l.style(l)
	if axis == Column {
		if !l.multiline(v) {
			return FixedSizes(l.lineBox(v))
		}
		width := forSize
		if width == Unknown {
			width = l.textRequest(Row, Unknown).Nat
		}
		return FixedSizes(len(l.wrapped(float64(width))) * l.lineBox(v))
	}
	face, px := l.effStyle()
	indent := l.indentOf(v)
	natural := l.shaped.Advance() + indent
	if l.multiline(v) {
		natural = 0
		for i, ln := range l.wrapped(math.Inf(1)) {
			adv := face.Shape(ln, px).Advance()
			if i == 0 {
				adv += indent
			}
			natural = math.Max(natural, adv)
		}
	}
	req := FixedSizes(int(math.Ceil(natural)))
	switch {
	case l.ellOf(v) != EllipsizeNone:
		req.Min = min(int(math.Ceil(face.Shape(render.Ellipsis, px).Advance()+indent)), req.Nat)
	case l.wrapOf(v) && l.text != "":
		widest := 0.0
		for _, tok := range render.WrapTextOpts(face, l.shown(), 1, px, l.minContentOpts(v)) {
			widest = math.Max(widest, face.Shape(tok, px).Advance())
		}
		req.Min = min(int(math.Ceil(widest)), req.Nat)
	}
	if l.maxChars > 0 && req.Min < req.Nat {
		req.Nat = max(min(req.Nat, l.charsWidth(l.maxChars)), req.Min)
	}
	if l.widthChars > 0 {
		req = req.atLeast(l.charsWidth(l.widthChars))
	}
	return req
}

// wrapped lays the text out in rows at width - soft-wrapped when the
// label wraps, split only at preserved newlines when it does not -
// ellipsizing the final row when a mode is set: the shared body of
// Measure and Paint, so the label reports and draws the same rows.
func (l *Label) wrapped(width float64) []string {
	face, px := l.effStyle()
	v := l.style(l)
	ell, maxLines := l.ellOf(v), l.maxLinesOf(v)
	breakAt := width
	if !l.wrapOf(v) {
		breakAt = math.Inf(1)
	}
	lines := render.WrapTextOpts(face, l.shown(), breakAt, px, l.wrapOpts(v))
	if maxLines > 0 && ell != EllipsizeNone && len(lines) > maxLines {
		lines = append(lines[:maxLines-1], strings.Join(lines[maxLines-1:], " "))
	}
	if last := len(lines) - 1; ell != EllipsizeNone && lines[last] != "" {
		w := width
		if last == 0 {
			w -= l.indentOf(v)
		}
		lines[last] = render.EllipsizeText(face, lines[last], ell, w, px)
	}
	return lines
}

// Arrange records the border box inside r, the margin box.
func (l *Label) Arrange(r render.Rect) {
	border, _ := boxRects(boxOf(l.style(l), render.Insets{}), r)
	l.node.Arrange(border)
}

// Paint draws the label's CSS layers (when the stylesheet gives it
// any), then the text inside the content box: the rows when the label
// is multiline, the single line ellipsized as configured otherwise.
func (l *Label) Paint(cv *render.Canvas) {
	v := l.style(l)
	b := boxOf(v, render.Insets{})
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, l.bounds, radii)
	cv = fx.cv
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, l.bounds, radii, b.border, bg)
	}
	paintPseudos(cv, l, nodeOf(l), v)
	content := b.padding.Shrink(b.border.Shrink(l.bounds))
	if clip, ok := overflowClipOf(v, l.bounds); ok {
		prev := cv.PushClip(clip)
		l.paintText(cv, v, content)
		cv.PopClip(prev)
	} else {
		l.paintText(cv, v, content)
	}
	paintOutline(cv, v, l.bounds, radii)
	fx.pop()
}

// paintText draws the text inside the content box: the rows when the
// label is multiline, the single line ellipsized as configured
// otherwise.
func (l *Label) paintText(cv *render.Canvas, v *style.Values, content render.Rect) {
	face, px := l.effStyleIn(v)
	tp := l.textPaint(v, face, px)
	if l.multiline(v) {
		l.paintWrapped(cv, tp, content, v)
		return
	}
	text := l.shown()
	line := l.indented(content, l.indentOf(v))
	if ell := l.ellOf(v); ell != EllipsizeNone && l.shaped.Advance() > float64(line.W) {
		text = render.EllipsizeText(face, text, ell, float64(line.W), px)
	}
	// The CSS line box, centered in the content box (or overflowing a
	// shorter one), holds the glyphs centered in it: half-leading for a
	// line-height taller than the glyphs, overflow for a smaller one,
	// never a skipped draw. Unstyled labels keep the plain centering
	// draw.
	if v.Has(style.PropLineHeight) {
		if h := l.lineBox(v); h > line.H {
			line.H = h
		} else if h < line.H {
			line.Y += (line.H - h) / 2
			line.H = h
		}
	}
	if fl := PseudoStyle(l, "first-letter"); fl != nil && text != "" &&
		fl.HasAny(style.PropColor, style.PropFontSize, style.PropFontWeight, style.PropFontFamily) {
		l.drawFirstLetter(cv, fl, tp, text, line)
		return
	}
	l.drawLine(cv, tp, text, line, 0)
}

// textPaint is what one label draw needs beyond the text and its box:
// the face and size, the ink, the decoration and shadows, the
// alignment and direction, and the CSS line box a row needs to draw.
type textPaint struct {
	face    render.Font
	px      float64
	col     render.Color
	deco    render.Decoration
	shadows *style.Shadows
	align   render.Alignment
	dir     Direction
	lineH   int
}

// textPaint gathers the label's draw parameters under v.
func (l *Label) textPaint(v *style.Values, face render.Font, px float64) textPaint {
	return textPaint{
		face: face, px: px, col: l.textColor(v), deco: cssDecoration(v), shadows: &v.TextShadow,
		align: l.alignOf(v), dir: l.effDirection(v), lineH: l.lineBox(v),
	}
}

// indented narrows a row by the text indent at its start edge: the
// left in a left-to-right label, the right in a right-to-left one.
func (l *Label) indented(row render.Rect, indent float64) render.Rect {
	if indent == 0 {
		return row
	}
	in := int(math.Round(indent))
	if l.shaped == nil || !l.shaped.RTL() {
		row.X += in
	}
	row.W -= in
	return row
}

// drawFirstLetter paints the plain single-line draw with the
// ::first-letter cascade applied: the first grapheme cluster in the
// pseudo-element's own face, size, and color at the line start, the
// remainder in the label's style in the box the cluster vacates.
func (l *Label) drawFirstLetter(cv *render.Canvas, fl *style.Values, tp textPaint, line string, box render.Rect) {
	rs := []rune(line)
	_, end := text.ClusterRun(rs, 0)
	first, rest := string(rs[:end]), string(rs[end:])
	fFace, fPx := tp.face, tp.px
	if fl.Has(style.PropFontSize) {
		fPx = fl.FontSize
	}
	if fl.Has(style.PropFontWeight) || fl.Has(style.PropFontFamily) {
		family := ""
		if fl.Has(style.PropFontFamily) {
			family = fl.FontFamily
		}
		weight := 0
		if fl.Has(style.PropFontWeight) {
			weight = fl.FontWeight
		}
		if f, ok := resolveFace(family, weight, tp.face); ok {
			fFace = f
		}
	}
	fCol := tp.col
	if fl.Has(style.PropColor) {
		fCol = fl.Color
	}
	shF := fFace.Shape(first, fPx)
	if rest == "" {
		shF.Draw(cv, box.X, box.Y+(box.H-shF.LineHeight())/2+int(shF.Ascent()), fCol)
		return
	}
	restBox := box
	if tp.dir == DirectionRTL {
		restBox.W = max(restBox.W-int(shF.Advance()), 0)
	} else {
		restBox.X += int(shF.Advance())
		restBox.W = max(restBox.W-int(shF.Advance()), 0)
	}
	l.drawLine(cv, tp, rest, restBox, 0)
	shF.Draw(cv, restBox.X-int(shF.Advance()), box.Y+(box.H-shF.LineHeight())/2+int(shF.Ascent()), fCol)
}

// drawLine draws one aligned line of text, its shadows under it and
// its decoration over it. A box shorter than the CSS line box (a label
// squeezed below its line) draws nothing; one shorter only than the
// glyphs (a line-height below them) draws them overflowing, centered.
func (l *Label) drawLine(cv *render.Canvas, tp textPaint, text string, box render.Rect, justify float64) {
	if box.H < tp.lineH {
		return
	}
	sh := tp.face.ShapeDir(text, tp.px, tp.dir)
	x, baseline := render.AlignedPen(sh, box, tp.align, tp.dir)
	l.drawShadows(cv, sh, x, baseline, tp, justify)
	drawShaped(cv, sh, x, baseline, tp.col, justify)
	if tp.deco.Lines != 0 {
		sh.DrawDecoration(cv, x, baseline, tp.deco, tp.col)
	}
}

// drawShaped draws a shaped line, justified by gap px per word
// separator when gap is positive.
func drawShaped(cv *render.Canvas, sh *render.ShapedText, x, baseline int, col render.Color, gap float64) {
	if gap > 0 && sh.SeparatorCount() > 0 {
		sh.DrawJustified(cv, x, baseline, col, gap)
		return
	}
	sh.Draw(cv, x, baseline, col)
}

// drawShadows paints the text-shadow layers under a line, the first
// listed on top (CSS Text Decoration 3 §4): a sharp layer is the line
// in the shadow's color at its offset, a blurred one paints through
// the label's offscreen layer and a Gaussian of half the blur radius.
func (l *Label) drawShadows(cv *render.Canvas, sh *render.ShapedText, x, baseline int, tp textPaint, justify float64) {
	layers := tp.shadows.List()
	for _, s := range slices.Backward(layers) {
		if s.Blur <= 0 {
			drawShaped(cv, sh, x+s.X, baseline+s.Y, s.Color, justify)
			continue
		}
		var blur render.Filters
		blur.Append(render.FilterOp{Kind: render.FilterBlur, Amount: float64(s.Blur) / 2})
		ink := render.Rect{
			X: x + s.X, Y: baseline + s.Y - int(math.Ceil(sh.Ascent())),
			W: int(math.Ceil(sh.Advance()+justify*float64(sh.SeparatorCount()))) + 1, H: sh.LineHeight(),
		}
		region := cv.MapRect(blur.Extent().Grow(ink)).Intersect(cv.Rect())
		if region.Empty() {
			continue
		}
		shadow := cv.ScratchLayer(region)
		drawShaped(shadow.Canvas(), sh, x+s.X, baseline+s.Y, s.Color, justify)
		render.ApplyFilters(shadow, region, &blur, render.FilterShape{})
		cv.DrawLayer(shadow, region, render.BlendNormal)
		shadow.Release()
	}
}

// paintWrapped draws the rows, the stack vertically centered in a
// taller rect, each row aligned per the cascade - justify spreads
// everything but the final row across the width, and the first row
// carries the text indent at its start edge.
func (l *Label) paintWrapped(cv *render.Canvas, tp textPaint, box render.Rect, v *style.Values) {
	lines := l.wrapped(float64(box.W))
	y := box.Y
	if extra := box.H - len(lines)*tp.lineH; extra > 0 {
		y += extra / 2
	}
	for i, ln := range lines {
		row, just := render.Rect{X: box.X, Y: y, W: box.W, H: tp.lineH}, 0.0
		if i == 0 {
			row = l.indented(row, l.indentOf(v))
		}
		if l.justifies(v) && i < len(lines)-1 {
			if sh := tp.face.ShapeDir(ln, tp.px, tp.dir); sh.SeparatorCount() > 0 {
				if gap := (float64(row.W) - sh.Advance()) / float64(sh.SeparatorCount()); gap > 0 {
					just = gap
				}
			}
		}
		l.drawLine(cv, tp, ln, row, just)
		y += tp.lineH
	}
}

// transformText applies a CSS text-transform.
func transformText(s string, t style.TextTransform) string {
	switch t {
	case style.TransformUppercase:
		return strings.ToUpper(s)
	case style.TransformLowercase:
		return strings.ToLower(s)
	case style.TransformCapitalize:
		rs := []rune(s)
		start := true
		for i, r := range rs {
			if unicode.IsSpace(r) {
				start = true
				continue
			}
			if start {
				rs[i] = unicode.ToTitle(r)
			}
			start = false
		}
		return string(rs)
	}
	return s
}

// Role implements Roleer.
func (l *Label) Role() Role { return RoleLabel }

func (l *Label) pseudoFace() (render.Font, float64) {
	face, px := l.effStyleIn(l.style(l))
	return face, px
}

// HitTest returns the label when p is inside its bounds.
func (l *Label) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
