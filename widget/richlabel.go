package widget

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/text"

	"github.com/stubbedev/lys-ui/internal/style"
)

// VariantFunc returns the face a bold or italic run shapes with. It
// must never return nil; returning the base face for a variant the
// platform lacks is the expected downgrade and renders fine.
type VariantFunc func(bold, italic bool) *render.Typeface

// BaseVariants serves base for every style. Use it when only one face
// is loaded: markup bold and italic then render with the base face.
// lys-ui never synthesizes faux bold or italic outlines. Apps that load
// real variants wire them here, e.g. via internal/sysfont's Variant.
func BaseVariants(base *render.Typeface) VariantFunc {
	return func(bool, bool) *render.Typeface { return base }
}

// RichLabel paints a single line of styled text driven by the minimal
// markup subset ParseMarkup accepts; see that function for the grammar
// and the escaping rules. Input ParseMarkup rejects renders literally,
// tags and all, so malformed markup can never panic or break a layout.
//
// Text is shaped once per run and drawn on one shared baseline; the
// line height is the maximum over the runs, so a taller face grows the
// box instead of misaligning its neighbors. Links (<a href=...>) are
// opt-in: they underline in the run's color, and installing
// OnLinkClick makes them clickable with a hand cursor.
type RichLabel struct {
	node
	base     render.Font
	faces    faceCache
	variants VariantFunc
	markup   string
	sizePx   float64
	// shapedPx is the font size the runs were shaped at: the cascade's,
	// else sizePx. Measure re-shapes when the cascade moved it.
	shapedPx float64
	color    render.Color
	align    render.Alignment
	dir      Direction

	// runs is the parsed (or literal fallback) run list; shaped
	// mirrors it with per-run shaping and metrics.
	runs   []MarkupRun
	shaped []*richRun
	// lineAsc and lineDesc are the line's max ascent and descent,
	// advance the total line width, runes the rune count of text.
	lineAsc, lineDesc, advance float64
	runes                      int
	text                       string
	natural                    Size
	// content is the arranged content box the text lays out in: the
	// border box less the CSS border and padding.
	content render.Rect

	// selStart/selEnd bound the highlighted rune range while selOn.
	selStart, selEnd int
	selOn            bool
	// selectable lets a press drag a selection across the text (a
	// readout the user copies); selAnchor is the pressed rune, selDrag
	// whether a drag extends it.
	selectable bool
	selAnchor  int
	selDrag    bool

	// hoverP is the last pointer position while hovered; hoverValid
	// gates link cursor lookups.
	hoverP     Point
	hoverValid bool

	// OnLinkClick fires when a press-release lands on a link run.
	OnLinkClick func(href string)

	// ell is the truncation mode; fitW the width the shaped runs were
	// last fitted to, -1 while they hold the whole line.
	ell  EllipsizeMode
	fitW int
	// maxChars caps the natural width at n approximate characters
	// (GTK's max-width-chars), 0 unset.
	maxChars int

	// wrap breaks the line at words to the arranged width; maxLines
	// caps a wrapping, ellipsizing label at that many rows. lines holds
	// the laid-out rows in wrap mode (nil: single-line, shaped above).
	wrap     bool
	maxLines int
	lines    []*richLine
}

// richLine is one laid-out row: the shaped runs in visual order and
// the row's advance.
type richLine struct {
	runs    []*richRun
	advance float64
}

// richRun is one shaped markup run.
type richRun struct {
	sh    *render.ShapedText
	face  render.Font
	style TextStyle
	start int // rune offset of the run's first rune within Text()
}

// NewRichLabel returns a rich label that paints markup with face at
// sizePx pixels in color. Face may be a render.Chain for mixed-script
// fallback; it shapes every run until SetVariants provides real
// variants. The argument order is the toolkit-wide constructor order:
// face, then sizePx, then the content, then colors. A nil face panics
// here (see requireFace) instead of failing later, in shaping.
func NewRichLabel(face render.Font, sizePx float64, markup string, color render.Color) *RichLabel {
	l := &RichLabel{base: requireFace("widget.NewRichLabel", face), markup: markup, sizePx: sizePx, color: color, fitW: -1}
	l.retext()
	return l
}

// SetVariants installs the face variants bold and italic runs shape
// with, and re-shapes the label. Plain runs always shape with the base
// face.
func (l *RichLabel) SetVariants(v VariantFunc) {
	l.variants = v
	l.retext()
	l.InvalidateLayout()
}

// SetMarkup replaces the label markup and invalidates its bounds.
// Markup ParseMarkup rejects renders literally.
func (l *RichLabel) SetMarkup(markup string) {
	if l.markup == markup {
		return
	}
	l.markup = markup
	l.retext()
	l.InvalidateLayout()
}

// Markup returns the current markup string.
func (l *RichLabel) Markup() string { return l.markup }

// Text returns the label's decoded plain text: the concatenated run
// text, or the raw input when the markup was rejected.
func (l *RichLabel) Text() string { return l.text }

// SetAlignment selects horizontal placement when the arranged rect is
// wider than the line.
func (l *RichLabel) SetAlignment(a render.Alignment) {
	if l.align == a {
		return
	}
	l.align = a
	l.Invalidate()
}

// Alignment returns the horizontal placement.
func (l *RichLabel) Alignment() render.Alignment { return l.align }

// SetDirection selects the base paragraph direction the line resolves
// and lays out with. DirectionAuto (the default) reads it off the
// line's first strong character; the resolution spans the whole line,
// so styled spans reorder together. Start/end alignment mirrors with
// the resolved direction. Changing the direction re-resolves the line
// and drops the measure cache.
func (l *RichLabel) SetDirection(d Direction) {
	if l.dir == d {
		return
	}
	l.dir = d
	l.retext()
	l.InvalidateLayout()
}

// Direction returns the base paragraph direction the line resolves
// with.
func (l *RichLabel) Direction() Direction { return l.dir }

// px is the effective font size: the cascade's font-size, else the
// constructor's.
func (l *RichLabel) px() float64 { return fontPx(l.style(l), l.sizePx) }

// runPx is the size a run shapes at: its absolute size, else the
// label's, times its scale.
func (l *RichLabel) runPx(st TextStyle) float64 {
	px := l.shapedPx
	if st.SizePx > 0 {
		px = st.SizePx
	}
	if st.Scale > 0 {
		px *= st.Scale
	}
	return px
}

// faceFor resolves the face a run shapes with: a run in another family
// resolves that family at its weight; bold and italic runs use the
// variants when one is installed, and every other run - and any
// variant the provider cannot supply - uses the base face. The run's
// weight, letter spacing and features then apply.
func (l *RichLabel) faceFor(st TextStyle) render.Font {
	f := l.base
	family := false
	if st.Family != "" {
		if rf, ok := resolveFace(st.Family, st.Weight, f); ok {
			f, family = rf, true
		}
	}
	if !family && l.variants != nil && (st.Bold || st.Italic) {
		if v := l.variants(st.Bold, st.Italic); v != nil {
			f = v
		}
	}
	return runFace(l.faces.get(f, l.style(l), st.Weight, int(l.runPx(st))), st)
}

// runFace is f with a run's own letter spacing and OpenType features.
func runFace(f render.Font, st TextStyle) render.Font {
	if st.LetterSpacing == 0 && st.Features == "" {
		return f
	}
	switch t := f.(type) {
	case *render.Typeface:
		return t.WithFeatureSpec(st.Features).Spaced(st.LetterSpacing)
	case *render.Chain:
		return t.WithFeatureSpec(st.Features).Spaced(st.LetterSpacing)
	}
	return f
}

// styleRestyled implements styleRestyler: a changed letter spacing or
// face setting reshapes the runs.
func (l *RichLabel) styleRestyled(old, new style.Values) {
	if old.LetterSpacing != new.LetterSpacing || old.Variations != new.Variations || old.Features != new.Features {
		l.retext()
		l.InvalidateLayout()
	}
}

// retext re-parses the markup, resolves the line's directional runs
// over the whole text, and reshapes every styled span the runs touch.
// The line resolves once — a bold Hebrew word and its plain Latin
// neighbor reorder together — and each visual piece keeps its span's
// face and style; inside a right-to-left run the spans draw in reverse,
// so the pieces' slice is already the visual order. Runs rejected by
// the parser render literally.
func (l *RichLabel) retext() {
	spans, ok := ParseMarkup(l.markup)
	if !ok {
		spans = []MarkupRun{{Text: l.markup}}
	}
	l.runs = spans
	l.fitW = -1
	l.shapedPx = l.px()
	line, runes := spansText(spans)
	l.text = line
	l.runes = runes
	l.shapeSpans(spans)
	l.natural = Size{W: int(math.Ceil(l.advance)), H: int(math.Ceil(l.lineAsc + l.lineDesc))}
}

// spansText is the spans' joined text and its rune count.
func spansText(spans []MarkupRun) (string, int) {
	var sb strings.Builder
	for _, r := range spans {
		sb.WriteString(r.Text)
	}
	return sb.String(), utf8.RuneCountInString(sb.String())
}

// shapeSpans resolves the line's directional runs over the spans'
// whole text and shapes every styled span the runs touch into l.shaped,
// with the line's ascent, descent, and advance.
func (l *RichLabel) shapeSpans(spans []MarkupRun) {
	line, asc, desc := l.shapeOne(spans)
	l.shaped = line.runs
	l.lineAsc, l.lineDesc, l.advance = asc, desc, line.advance
}

// shapeOne shapes one row of spans: the directional runs intersect the
// styled spans, each visual piece keeping its span's face and style.
// The row's metrics ride along; an empty row still reserves the font's
// line height, like Label.
func (l *RichLabel) shapeOne(spans []MarkupRun) (*richLine, float64, float64) {
	line, _ := spansText(spans)
	out := &richLine{runs: make([]*richRun, 0, len(spans))}
	// Span rune spans, to intersect the styled spans with the
	// directional runs.
	spanStart := make([]int, len(spans)+1)
	for i, r := range spans {
		spanStart[i+1] = spanStart[i] + utf8.RuneCountInString(r.Text)
	}
	rs := []rune(line)
	asc, desc, adv := 0.0, 0.0, 0.0
	for _, br := range text.BidiRuns(line, l.dir) {
		dir := DirectionLTR
		if br.RTL {
			dir = DirectionRTL
		}
		for i := range spans {
			k := i
			if br.RTL {
				k = len(spans) - 1 - i // visual order inside the run
			}
			lo, hi := max(br.Start, spanStart[k]), min(br.End, spanStart[k+1])
			if lo >= hi {
				continue
			}
			face := l.faceFor(spans[k].Style)
			px := l.runPx(spans[k].Style)
			sh := face.ShapeDir(string(rs[lo:hi]), px, dir)
			rise := spans[k].Style.Shift * px
			asc = math.Max(asc, sh.Ascent()+rise)
			desc = math.Max(desc, sh.Descent()-rise)
			adv += sh.Advance()
			out.runs = append(out.runs, &richRun{sh: sh, face: face, style: spans[k].Style, start: lo})
		}
	}
	if len(out.runs) == 0 {
		// Empty text still reserves the font's line height, like
		// Label.
		sh := l.faceFor(TextStyle{}).Shape("", l.shapedPx)
		asc, desc = sh.Ascent(), sh.Descent()
	}
	out.advance = adv
	return out, asc, desc
}

// SetEllipsize sets how a line wider than its box truncates: Start,
// Middle, or End trade runes for an ellipsis, in the style of the run
// it stands beside, until the line fits; None (the default) lets it
// overflow. The natural width stays the whole line's.
func (l *RichLabel) SetEllipsize(mode EllipsizeMode) {
	if l.ell == mode {
		return
	}
	l.ell = mode
	l.retext()
	l.InvalidateLayout()
}

// Ellipsize returns the truncation mode.
func (l *RichLabel) Ellipsize() EllipsizeMode { return l.ell }

// SetWrap toggles word wrapping at the arranged width: Measure reports
// the widest wrapped row and one line height per row, and Paint lays
// the rows out with the alignment. Changing it relayouts.
func (l *RichLabel) SetWrap(on bool) {
	if l.wrap == on {
		return
	}
	l.wrap = on
	l.fitW = -1
	l.lines = nil
	l.InvalidateLayout()
}

// Wrap reports whether the label wraps at the arranged width.
func (l *RichLabel) Wrap() bool { return l.wrap }

// SetMaxLines caps a wrapping, ellipsizing label at n rows: past that
// the rows merge into the last and the truncation mode ellipsizes it.
// 0, the default, is no cap.
func (l *RichLabel) SetMaxLines(n int) {
	n = max(n, 0)
	if l.maxLines == n {
		return
	}
	l.maxLines = n
	l.fitW = -1
	l.lines = nil
	l.InvalidateLayout()
}

// MaxLines returns the row cap, 0 for none.
func (l *RichLabel) MaxLines() int { return l.maxLines }

// SetMaxWidthChars caps the label's natural width at n approximate
// character widths, GTK's max-width-chars: a wrapping label lays its
// text out wrapped at the cap and asks for its widest row, a single
// line truncates by the ellipsize mode; a shorter text keeps its own
// width. n <= 0 removes the cap.
func (l *RichLabel) SetMaxWidthChars(n int) {
	n = max(n, 0)
	if l.maxChars == n {
		return
	}
	l.maxChars = n
	l.InvalidateLayout()
}

// MaxWidthChars returns the natural-width cap in characters, 0 when
// unset.
func (l *RichLabel) MaxWidthChars() int { return l.maxChars }

// Arrange records the border box inside r, the margin box, and fits
// the line to the content box's width.
func (l *RichLabel) Arrange(r render.Rect) {
	border, content := boxRects(boxOf(l.style(l), render.Insets{}), r)
	l.node.Arrange(border)
	l.content = content
	if l.wrap {
		l.reflow(content.W)
		return
	}
	l.fit(content.W)
}

// reflow lays the wrapped rows out to width w: the whole text on one
// row when it fits, else word-wrapped rows, capped at maxLines with
// the overflow merged into an ellipsized last row.
func (l *RichLabel) reflow(w int) {
	if l.fitW == w && l.lines != nil {
		return
	}
	rows, starts := l.wrapSpans(w)
	if n := l.maxLines; n > 0 && l.ell != EllipsizeNone && len(rows) > n {
		merged := sliceSpans(l.runs, starts[n-1], l.runes)
		rows = append(rows[:n-1], merged)
		rows[n-1] = l.fitLine(rows[n-1], w)
	}
	l.lines = make([]*richLine, 0, len(rows))
	for _, row := range rows {
		line, _, _ := l.shapeOne(row)
		l.lines = append(l.lines, line)
	}
	l.fitW = w
}

// wrapSpans breaks the spans into rows of at most w pixels: greedy
// over the text's line-break opportunities (UAX #14, as a plain label
// breaks), the spaces after a word riding with it and dropped at a
// break. A word wider than a row breaks between grapheme clusters, as
// Pango's word wrap falls back to, so nothing runs past the label.
// starts are the rows' first rune offsets in the text.
func (l *RichLabel) wrapSpans(w int) (rows [][]MarkupRun, starts []int) {
	lineW := 0.0
	pos := 0
	var row []MarkupRun
	flush := func() {
		rows = append(rows, trimRowEnd(row))
		row, lineW = nil, 0
	}
	add := func(s string, st TextStyle, adv float64) {
		if len(row) == 0 {
			starts = append(starts, pos)
		}
		pos += utf8.RuneCountInString(s)
		if n := len(row); n > 0 && row[n-1].Style == st {
			row[n-1].Text += s
		} else {
			row = append(row, MarkupRun{Text: s, Style: st})
		}
		lineW += adv
	}
	for _, span := range l.runs {
		face, px := l.faceFor(span.Style), l.runPx(span.Style)
		advance := func(s string) float64 { return face.Shape(s, px).Advance() }
		for _, seg := range text.LineSegments(span.Text) {
			word := strings.TrimRight(seg, " \t")
			wordW := advance(word)
			if lineW+wordW > float64(w) && len(row) > 0 {
				flush()
			}
			if wordW <= float64(w) {
				add(seg, span.Style, advance(seg))
				continue
			}
			rs := []rune(seg)
			for i := 0; i < len(rs); {
				j := text.NextCluster(rs, i)
				g := string(rs[i:j])
				gw := advance(g)
				if lineW+gw > float64(w) && len(row) > 0 {
					flush()
				}
				add(g, span.Style, gw)
				i = j
			}
		}
	}
	if len(row) > 0 {
		rows = append(rows, trimRowEnd(row))
	}
	if rows == nil {
		rows, starts = [][]MarkupRun{l.runs}, []int{0}
	}
	return rows, starts
}

// trimRowEnd drops the spaces a row ends with, which a break swallows.
func trimRowEnd(row []MarkupRun) []MarkupRun {
	for len(row) > 0 {
		last := &row[len(row)-1]
		last.Text = strings.TrimRight(last.Text, " \t")
		if last.Text != "" {
			break
		}
		row = row[:len(row)-1]
	}
	return row
}

// fitLine is one row ellipsized to w, the truncation mode placing the
// marks; a row already inside w comes back untouched.
func (l *RichLabel) fitLine(spans []MarkupRun, w int) []MarkupRun {
	line, _, _ := l.shapeOne(spans)
	if line.advance <= float64(w) || l.ell == EllipsizeNone {
		return spans
	}
	n := 0
	for _, run := range spans {
		n += utf8.RuneCountInString(run.Text)
	}
	width := func(keep int) float64 {
		cut, _, _ := l.shapeOne(cutWithin(spans, keep))
		return cut.advance
	}
	lo, hi := 0, n-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if width(mid) <= float64(w) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return cutWithin(spans, lo)
}

// cutWithin is the spans with keep runes kept and the ellipsis after
// them, in the last kept span's style (a wrapped last row always cuts
// toward its end).
func cutWithin(spans []MarkupRun, keep int) []MarkupRun {
	n := 0
	for _, run := range spans {
		n += utf8.RuneCountInString(run.Text)
	}
	if keep >= n {
		return spans
	}
	st := TextStyle{}
	if len(spans) > 0 {
		st = spans[len(spans)-1].Style
	}
	out := sliceSpans(spans, 0, keep)
	return append(out, MarkupRun{Text: render.Ellipsis, Style: st})
}

// fit shapes the line for width w: the whole line when it fits or
// truncation is off, else the longest cut that fits with the ellipsis.
func (l *RichLabel) fit(w int) {
	if l.ell == EllipsizeNone || l.natural.W <= w {
		if l.fitW != -1 {
			l.shapeSpans(l.runs)
			l.fitW = -1
		}
		return
	}
	if l.fitW == w {
		return
	}
	width := func(keep int) float64 {
		l.shapeSpans(l.cutSpans(keep))
		return l.advance
	}
	// The largest kept rune count that fits; 0 is the bare ellipsis.
	lo, hi := 0, l.runes-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if width(mid) <= float64(w) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	l.shapeSpans(l.cutSpans(lo))
	l.fitW = w
}

// cutSpans is the line with keep runes kept and the ellipsis where the
// mode puts it.
func (l *RichLabel) cutSpans(keep int) []MarkupRun {
	n := l.runes
	switch l.ell {
	case EllipsizeStart:
		tail := sliceSpans(l.runs, n-keep, n)
		return append([]MarkupRun{{Text: render.Ellipsis, Style: styleAt(l.runs, n-keep)}}, tail...)
	case EllipsizeMiddle:
		head, tail := (keep+1)/2, keep/2
		out := sliceSpans(l.runs, 0, head)
		out = append(out, MarkupRun{Text: render.Ellipsis, Style: styleAt(l.runs, max(head-1, 0))})
		return append(out, sliceSpans(l.runs, n-tail, n)...)
	}
	out := sliceSpans(l.runs, 0, keep)
	return append(out, MarkupRun{Text: render.Ellipsis, Style: styleAt(l.runs, max(keep-1, 0))})
}

// sliceSpans is the spans' runes [lo, hi), each piece keeping its
// span's style.
func sliceSpans(spans []MarkupRun, lo, hi int) []MarkupRun {
	var out []MarkupRun
	at := 0
	for _, s := range spans {
		rs := []rune(s.Text)
		a, b := max(lo-at, 0), min(hi-at, len(rs))
		if a < b {
			out = append(out, MarkupRun{Text: string(rs[a:b]), Style: s.Style})
		}
		at += len(rs)
	}
	return out
}

// styleAt is the style of the span holding rune i.
func styleAt(spans []MarkupRun, i int) TextStyle {
	at := 0
	for _, s := range spans {
		at += utf8.RuneCountInString(s.Text)
		if i < at {
			return s.Style
		}
	}
	if len(spans) > 0 {
		return spans[len(spans)-1].Style
	}
	return TextStyle{}
}

// Measure returns the line's total advance and its line height - the
// maximum over the runs - inside the CSS box, clamped to con. Wrap mode
// reports the widest wrapped row and one line height per row at the
// offered content width.
func (l *RichLabel) Measure(con Constraints) Size {
	if s, ok := l.measureHit(con); ok {
		return s
	}
	if l.px() != l.shapedPx {
		l.retext()
	}
	v := l.style(l)
	return l.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, l.measureText))
}

// measureText is Measure inside the CSS box.
func (l *RichLabel) measureText(con Constraints) Size {
	limit := con.Max.W
	if c := l.widthCap(); c > 0 {
		limit = min(limit, c)
	}
	if l.wrap && l.markup != "" && limit < l.natural.W {
		l.reflow(limit)
		w := 0
		for _, line := range l.lines {
			w = max(w, int(math.Ceil(line.advance)))
		}
		lineH, _ := l.lineBox()
		return clampSize(Size{W: w, H: len(l.lines) * lineH}, con)
	}
	lineH, _ := l.lineBox()
	nat := Size{W: min(l.natural.W, limit), H: lineH}
	return clampSize(nat, con)
}

// lineBox is a row's height and its baseline's depth below the row's
// top, as a plain label's: under a CSS line-height a unitless number
// gives each run a box that many times its own size, centered on its
// glyphs, and the row spans them all on the shared baseline; a length
// is the row's height, the glyphs centered in it; with neither the
// row is the glyphs' ascent plus descent.
func (l *RichLabel) lineBox() (int, float64) {
	glyphs := l.lineAsc + l.lineDesc
	h := float64(l.natural.H)
	if v := l.style(l); v.Has(style.PropLineHeight) {
		if f := v.LineHeightFactor; f > 0 {
			above, below := 0.0, 0.0
			for i, r := range l.shaped {
				asc, desc := r.sh.Ascent(), r.sh.Descent()
				lead := (f*l.runPx(r.style) - (asc + desc)) / 2
				if i == 0 || asc+lead > above {
					above = asc + lead
				}
				if i == 0 || desc+lead > below {
					below = desc + lead
				}
			}
			if len(l.shaped) == 0 {
				lead := (f*l.shapedPx - glyphs) / 2
				above, below = l.lineAsc+lead, l.lineDesc+lead
			}
			return int(math.Ceil(above + below)), above
		}
		if v.LineHeight > 0 {
			h = math.Ceil(v.LineHeight)
		}
	}
	return int(h), (h-glyphs)/2 + l.lineAsc
}

// widthCap is the max-width-chars cap in pixels: n approximate
// character widths, 0 when unset.
func (l *RichLabel) widthCap() int {
	if l.maxChars <= 0 {
		return 0
	}
	sh := l.faceFor(TextStyle{}).Shape(approxCharSample, l.shapedPx)
	avg := sh.Advance() / float64(len(approxCharSample))
	return int(math.Ceil(avg * float64(l.maxChars)))
}

// ShrinkableWidth implements WidthShrinker: a wrapping label gives up
// its measured width down to its widest token (MinSize), wrapping onto
// more lines, as a GTK wrapping label's minimum is; a single-line label
// keeps its width.
func (l *RichLabel) ShrinkableWidth() int {
	if !l.wrap || l.text == "" || !l.measureSeen {
		return 0
	}
	return max(0, l.measureOut.W-l.MinSize().W)
}

// MinSize is a wrapping label's floor: the widest unbreakable token —
// wrapping narrower than that clips tokens, the one thing wrapping
// promised not to do — and one line height, inside the CSS box.
func (l *RichLabel) MinSize() Size {
	o := boxOf(l.style(l), render.Insets{}).outer()
	floor := Size{H: l.natural.H}
	if l.wrap && l.text != "" {
		w := 0.0
		rows, _ := l.wrapSpans(1)
		for _, tok := range rows {
			line, _, _ := l.shapeOne(tok)
			w = math.Max(w, line.advance)
		}
		floor.W = int(math.Ceil(w))
	}
	return Size{W: floor.W + o.Left + o.Right, H: floor.H + o.Top + o.Bottom}
}

// lineGeom resolves the aligned line origin and the runs' shared
// baseline, mirroring start and end for a right-to-left line. ok is
// false when the arranged box cannot hold one line, the same guard a
// plain label's DrawAligned applies.
func (l *RichLabel) lineGeom() (lineX float64, baseline int, ok bool) {
	lineH, depth := l.lineBox()
	if l.content.H < lineH {
		return 0, 0, false
	}
	align := l.align
	if text.RTL(l.text, l.dir) {
		switch align {
		case render.AlignStart:
			align = render.AlignEnd
		case render.AlignEnd:
			align = render.AlignStart
		}
	}
	switch align {
	case render.AlignCenter:
		lineX = float64(l.content.X) + (float64(l.content.W)-l.advance)/2
	case render.AlignEnd:
		lineX = float64(l.content.X) + float64(l.content.W) - l.advance
	default:
		lineX = float64(l.content.X)
	}
	baseline = l.content.Y + int(math.Round(float64(l.content.H-lineH)/2+depth))
	return lineX, baseline, true
}

// rowGeom is row i's aligned origin and baseline in wrap mode: rows
// align individually, the block centers in the bounds.
func (l *RichLabel) rowGeom(i int) (float64, int) {
	line := l.lines[i]
	align := l.align
	if text.RTL(l.text, l.dir) {
		switch align {
		case render.AlignStart:
			align = render.AlignEnd
		case render.AlignEnd:
			align = render.AlignStart
		}
	}
	var x float64
	switch align {
	case render.AlignCenter:
		x = float64(l.content.X) + (float64(l.content.W)-line.advance)/2
	case render.AlignEnd:
		x = float64(l.content.X) + float64(l.content.W) - line.advance
	default:
		x = float64(l.content.X)
	}
	lineH, baseline := l.lineBox()
	top := l.content.Y + max(0, (l.content.H-len(l.lines)*lineH)/2)
	return x, top + i*lineH + int(math.Round(baseline))
}

// Paint draws the selection highlight, then each run: its background,
// its variant face off its baseline shift, its color (a span color wins
// over the label color, faded by its alpha), and an underline for link
// runs. Wrap mode lays one row per baseline.
func (l *RichLabel) Paint(cv *render.Canvas) {
	if v := l.style(l); pickc(0, v, style.PropBackgroundColor, 0) != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, l.bounds, radiusOr(v, 0), borderOf(v), pickc(0, v, style.PropBackgroundColor, 0))
	}
	if start, end, on := l.selection(); on {
		a := Current().Accent
		hl := render.RGBA(a.R(), a.G(), a.B(), 90)
		for _, b := range l.SelectionBands(start, end) {
			cv.FillRect(b, hl)
		}
	}
	if l.wrap && l.lines != nil {
		for i, line := range l.lines {
			x, baseline := l.rowGeom(i)
			l.paintLine(cv, line.runs, x, baseline)
		}
		return
	}
	lineX, baseline, ok := l.lineGeom()
	if !ok {
		return
	}
	l.paintLine(cv, l.shaped, lineX, baseline)
}

// paintLine draws the runs on one baseline: each its variant face, its
// color, and its decorations (the stylesheet's, an underline for link
// and <u> runs, a line through <s> runs).
func (l *RichLabel) paintLine(cv *render.Canvas, runs []*richRun, lineX float64, baseline int) {
	v := l.style(l)
	css := cssDecoration(v)
	ink := pickc(l.color, v, style.PropColor, inkOr(l.color))
	x := lineX
	for _, r := range runs {
		dx := int(math.Round(x))
		col := ink
		if r.style.Color != 0 {
			col = r.style.Color
		}
		if r.style.Alpha > 0 {
			col = scaleAlpha(col, r.style.Alpha)
		}
		y := baseline - int(math.Round(r.style.Shift*l.runPx(r.style)))
		if r.style.Background != 0 {
			top := y - int(math.Ceil(r.sh.Ascent()))
			cv.FillRect(render.Rect{X: dx, Y: top, W: int(math.Ceil(r.sh.Advance())), H: y + int(math.Ceil(r.sh.Descent())) - top}, r.style.Background)
		}
		r.face.Draw(cv, r.sh, dx, y, col)
		deco := css
		if r.style.Href != "" || r.style.Underline {
			deco.Lines |= render.Underline
		}
		if r.style.Underline {
			deco.Style = r.style.UnderlineStyle
		}
		if r.style.Strikethrough {
			deco.Lines |= render.LineThrough
		}
		r.sh.DrawDecoration(cv, dx, y, deco, col)
		x += r.sh.Advance()
	}
}

// selection returns the ordered, clamped selected range and whether
// one is active.
func (l *RichLabel) selection() (start, end int, on bool) {
	if !l.selOn {
		return 0, 0, false
	}
	start, end = l.selStart, l.selEnd
	if start > end {
		start, end = end, start
	}
	start = max(0, min(start, l.runes))
	end = max(0, min(end, l.runes))
	return start, end, start < end
}

// SetSelection highlights the rune range [start, end) of Text(),
// clamped; an empty range paints nothing.
func (l *RichLabel) SetSelection(start, end int) {
	l.selStart, l.selEnd, l.selOn = start, end, true
	l.Invalidate()
}

// ClearSelection drops the highlight.
func (l *RichLabel) ClearSelection() {
	if !l.selOn {
		return
	}
	l.selOn = false
	l.Invalidate()
}

// SelectionBands returns the selection rectangles covering runes
// [start, end) of Text(), in root coordinates: one band per run the
// range touches, spanning the widget's full height. Rich labels have
// no editing caret; bands are the whole of their selection geometry.
func (l *RichLabel) SelectionBands(start, end int) []render.Rect {
	start = max(0, min(start, l.runes))
	end = max(0, min(end, l.runes))
	if start >= end {
		return nil
	}
	lineX, _, ok := l.lineGeom()
	if !ok {
		return nil
	}
	var bands []render.Rect
	x := lineX
	for _, r := range l.shaped {
		n := utf8.RuneCountInString(r.sh.Text())
		runX := x
		x += r.sh.Advance()
		// Clamp the range into the run's rune span.
		lo := min(max(start, r.start), r.start+n) - r.start
		hi := min(max(end, r.start), r.start+n) - r.start
		if lo >= hi {
			continue
		}
		cs := r.sh.CaretPositions()
		x0 := int(math.Round(runX + cs[lo]))
		x1 := int(math.Round(runX + cs[hi]))
		if x1 < x0 {
			x0, x1 = x1, x0 // an RTL piece's caret table runs backwards
		}
		if x1 <= x0 {
			continue
		}
		bands = append(bands, render.Rect{X: x0, Y: l.content.Y, W: x1 - x0, H: l.content.H})
	}
	return bands
}

// linkAt returns the href of the link run under p, in root
// coordinates.
func (l *RichLabel) linkAt(p Point) (string, bool) {
	if !l.bounds.Contains(p.X, p.Y) {
		return "", false
	}
	if l.wrap && l.lines != nil {
		lineH, _ := l.lineBox()
		top := l.content.Y + max(0, (l.content.H-len(l.lines)*lineH)/2)
		row := (p.Y - top) / lineH
		if row < 0 || row >= len(l.lines) {
			return "", false
		}
		x, _ := l.rowGeom(row)
		for _, r := range l.lines[row].runs {
			w := r.sh.Advance()
			if r.style.Href != "" && float64(p.X) >= x && float64(p.X) < x+w {
				return r.style.Href, true
			}
			x += w
		}
		return "", false
	}
	lineX, _, ok := l.lineGeom()
	if !ok {
		return "", false
	}
	x := lineX
	for _, r := range l.shaped {
		w := r.sh.Advance()
		if r.style.Href != "" && float64(p.X) >= x && float64(p.X) < x+w {
			return r.style.Href, true
		}
		x += w
	}
	return "", false
}

// SetSelectable lets a press drag a text selection across the label,
// the readout a user copies; ctrl+c copies it through SelectedText.
func (l *RichLabel) SetSelectable(on bool) {
	if l.selectable == on {
		return
	}
	l.selectable = on
	if !on {
		l.ClearSelection()
	}
}

// Selectable reports whether the label takes drag selections.
func (l *RichLabel) Selectable() bool { return l.selectable }

// PressAt starts a drag selection at the pressed rune. user-select
// none never starts one; all selects the whole text.
func (l *RichLabel) PressAt(p Point) {
	if !l.selectable {
		return
	}
	switch userSelectOf(l) {
	case style.SelectNone:
		return
	case style.SelectAll:
		l.selDrag = true
		l.SetSelection(0, len([]rune(l.Text())))
		return
	}
	l.selAnchor = l.runeAt(p)
	l.selDrag = true
	l.SetSelection(l.selAnchor, l.selAnchor)
}

// DragMove extends the drag selection to the pointer.
func (l *RichLabel) DragMove(p Point) {
	if !l.selDrag || userSelectOf(l) == style.SelectNone {
		return
	}
	l.SetSelection(l.selAnchor, l.runeAt(p))
}

// SelectedText is the selected runes and whether any are selected, the
// copy path's source.
func (l *RichLabel) SelectedText() (string, bool) {
	start, end, on := l.selection()
	if !on {
		return "", false
	}
	return string([]rune(l.text)[start:end]), true
}

// runeAt maps a point to its rune index: the row under it, then the
// caret table of the run under its x.
func (l *RichLabel) runeAt(p Point) int {
	if l.wrap && l.lines != nil {
		lineH, _ := l.lineBox()
		top := l.content.Y + max(0, (l.content.H-len(l.lines)*lineH)/2)
		row := max(0, (p.Y-top)/lineH)
		if row >= len(l.lines) {
			return l.runes
		}
		x, _ := l.rowGeom(row)
		off := 0
		for i := range l.lines {
			if i == row {
				break
			}
			for _, r := range l.lines[i].runs {
				off += utf8.RuneCountInString(r.sh.Text())
			}
		}
		idx := caretIndex(l.lines[row].runs, x, p.X)
		return min(off+idx, l.runes)
	}
	lineX, _, ok := l.lineGeom()
	if !ok {
		return 0
	}
	return caretIndex(l.shaped, lineX, p.X)
}

// caretIndex is the rune offset inside the run list a pointer sits at:
// the nearest caret position of the run under x.
func caretIndex(runs []*richRun, lineX float64, px int) int {
	x := lineX
	for _, r := range runs {
		w := r.sh.Advance()
		n := utf8.RuneCountInString(r.sh.Text())
		if float64(px) < x+w || r == runs[len(runs)-1] {
			cs := r.sh.CaretPositions()
			best := 0
			for i := 1; i <= n; i++ {
				if math.Abs(cs[i]+x-float64(px)) < math.Abs(cs[best]+x-float64(px)) {
					best = i
				}
			}
			return r.start + best
		}
		x += w
	}
	return 0
}

// ClickAt fires OnLinkClick when the click lands on a link run; a
// selectable label's plain click drops the selection.
func (l *RichLabel) ClickAt(p Point) {
	if href, ok := l.linkAt(p); ok && l.OnLinkClick != nil {
		l.OnLinkClick(href)
		return
	}
	if l.selectable {
		l.selDrag = false
		l.ClearSelection()
	}
}

// HoverMove records the pointer position for link lookups.
func (l *RichLabel) HoverMove(p Point) {
	l.hoverP, l.hoverValid = p, true
}

// SetHovered forgets the pointer position on leave.
func (l *RichLabel) SetHovered(on bool) {
	if !on && l.hoverValid {
		l.hoverValid = false
		l.invalidateState(style.Hover)
	}
}

// CursorName asks for the pointing hand while the pointer sits on a
// link run.
func (l *RichLabel) CursorName() string {
	if l.hoverValid {
		if _, ok := l.linkAt(l.hoverP); ok {
			return "hand1"
		}
	}
	return ""
}

// Role implements Roleer.
func (l *RichLabel) Role() Role { return RoleLabel }

// HitTest returns the label when p is inside its bounds.
func (l *RichLabel) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
