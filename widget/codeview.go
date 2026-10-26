package widget

import (
	"slices"
	"strconv"

	"github.com/stubbedev/lys-lib/render"
)

// The TextArea's code-view features, GtkSourceView's: a line-number
// gutter, syntax highlighting through a Highlighter and a TextScheme,
// Enter carrying the line's indentation, and the caret kept in view.

// Highlighter splits lines into styled spans (a GtkSourceView
// language). Highlight styles line given the state the line before it
// ended in (0 before the first line) and returns the state this one
// ends in, so a construct spanning lines — a multi-line string —
// carries on. Spans are rune ranges of the line, in order, not
// overlapping.
type Highlighter interface {
	Highlight(line []rune, state int) (spans []TextSpan, end int)
}

// TextSpan styles runes [Start, End) of a line with the scheme's style
// for Class (a GtkSourceView style id, such as def:string).
type TextSpan struct {
	Start, End int
	Class      string
}

// SchemeStyle is one style of a TextScheme. Zero colors keep what
// the text would paint with; Bold and Italic shape with the faces
// SetVariants gives.
type SchemeStyle struct {
	Color, Background render.Color
	Bold, Italic      bool
}

// TextScheme maps style ids to styles: the highlighter's span classes,
// and "text", "cursor", "selection" and "line-numbers" for the view
// itself (a GtkSourceView style scheme).
type TextScheme map[string]SchemeStyle

// schemeFallback is GtkSourceView's def.lang style map: a style the
// scheme lacks takes the one it maps to (def:decimal to def:number to
// def:constant).
var schemeFallback = map[string]string{
	"def:shebang":          "def:comment",
	"def:doc-comment":      "def:comment",
	"def:character":        "def:constant",
	"def:string":           "def:constant",
	"def:number":           "def:constant",
	"def:floating-point":   "def:number",
	"def:decimal":          "def:number",
	"def:base-n-integer":   "def:number",
	"def:complex":          "def:number",
	"def:special-constant": "def:constant",
	"def:boolean":          "def:special-constant",
	"def:function":         "def:identifier",
	"def:builtin":          "def:identifier",
	"def:operator":         "def:statement",
	"def:keyword":          "def:statement",
	"def:reserved":         "def:error",
	"def:underlined":       "def:net-address",
	// The markup ids (Markdown): a scheme without them still sets
	// headings, code, links and markers apart.
	"def:heading":              "def:keyword",
	"def:inline-code":          "def:preformatted-section",
	"def:preformatted-section": "def:string",
	"def:link-text":            "def:underlined",
	"def:link-destination":     "def:net-address",
	"def:list-marker":          "def:statement",
	"def:blockquote-marker":    "def:statement",
	"def:thematic-break":       "def:statement",
}

// Style is class's style, following the def.lang map to the nearest
// style the scheme has; false when none is styled.
func (s TextScheme) Style(class string) (SchemeStyle, bool) {
	for range len(schemeFallback) + 1 {
		if st, ok := s[class]; ok {
			return st, true
		}
		next, ok := schemeFallback[class]
		if !ok {
			break
		}
		class = next
	}
	return SchemeStyle{}, false
}

// hlLine caches one line's spans and the states around them.
type hlLine struct {
	text    []rune
	in, out int
	spans   []TextSpan
}

// SetLineNumbers shows the line-number gutter at the text's left.
func (t *TextArea) SetLineNumbers(on bool) {
	if t.lineNumbers != on {
		t.lineNumbers = on
		t.rowsValid = false
		t.InvalidateLayout()
	}
}

// LineNumbers reports SetLineNumbers.
func (t *TextArea) LineNumbers() bool { return t.lineNumbers }

// SetAutoIndent makes Enter start the new line with the current line's
// leading whitespace (GtkSourceView's auto-indent).
func (t *TextArea) SetAutoIndent(on bool) { t.autoIndent = on }

// AutoIndent reports SetAutoIndent.
func (t *TextArea) AutoIndent() bool { return t.autoIndent }

// SetHighlighter styles the text with h through scheme; a nil h paints
// it plain.
func (t *TextArea) SetHighlighter(h Highlighter, scheme TextScheme) {
	t.highlighter, t.scheme, t.hl = h, scheme, nil
	t.Invalidate()
}

// SetScheme replaces the scheme, keeping the highlighter (a palette
// change).
func (t *TextArea) SetScheme(scheme TextScheme) {
	t.scheme = scheme
	t.Invalidate()
}

// SetVariants installs the faces bold and italic styles shape with
// (RichLabel's VariantFunc); without, they draw with the regular face.
func (t *TextArea) SetVariants(v VariantFunc) {
	t.variants = v
	t.Invalidate()
}

// gutterPad is the space each side of the line numbers.
const gutterPad = 4

// gutterWidth is the gutter's width: the widest line number and its
// padding, nothing without line numbers.
func (t *TextArea) gutterWidth() int {
	if !t.lineNumbers {
		return 0
	}
	digits := max(len(strconv.Itoa(len(t.lines))), 2)
	return int(float64(digits)*t.font().ShapeRune('0', t.px()).Advance()+0.5) + 2*gutterPad
}

// paintGutter draws each line's number, right-aligned in the gutter
// left of the text rect c, on the line's first row.
func (t *TextArea) paintGutter(cv *render.Canvas, c render.Rect, lineH int, fade func(render.Color) render.Color) {
	w := t.gutterWidth()
	if w == 0 {
		return
	}
	g := render.Rect{X: c.X - w - t.textNodeLeft(), Y: t.bounds.Y, W: w, H: t.bounds.H}
	col := Current().Border
	if st, ok := t.scheme["line-numbers"]; ok {
		if st.Background != 0 {
			cv.FillRect(g, st.Background)
		}
		if st.Color != 0 {
			col = st.Color
		}
	}
	prev := cv.PushClip(g)
	defer cv.PopClip(prev)
	last := -1
	for i, r := range t.rows {
		if r.line == last {
			continue
		}
		last = r.line
		sh := t.font().Shape(strconv.Itoa(r.line+1), t.px())
		y := c.Y + i*lineH
		baseline := y + int((float64(lineH)-float64(sh.LineHeight()))/2+sh.Ascent()+0.5)
		sh.Draw(cv, g.X+w-gutterPad-int(sh.Advance()+0.5), baseline, fade(col))
	}
}

// textNodeLeft is the text node's left margin, border and padding,
// between the gutter and the text.
func (t *TextArea) textNodeLeft() int {
	return boxOf(t.text.style(&t.text), render.Insets{}).outer().Left
}

// updateHighlight brings the span cache up to the lines: a line is
// styled again when its text or the state it starts in changed.
func (t *TextArea) updateHighlight() {
	if t.highlighter == nil {
		t.hl = t.hl[:0]
		return
	}
	if len(t.hl) > len(t.lines) {
		t.hl = t.hl[:len(t.lines)]
	}
	state := 0
	for i, line := range t.lines {
		if i < len(t.hl) && t.hl[i].in == state && slices.Equal(t.hl[i].text, line) {
			state = t.hl[i].out
			continue
		}
		spans, out := t.highlighter.Highlight(line, state)
		entry := hlLine{text: slices.Clone(line), in: state, out: out, spans: spans}
		if i < len(t.hl) {
			t.hl[i] = entry
		} else {
			t.hl = append(t.hl, entry)
		}
		state = out
	}
}

// drawRow draws row r of line: plain, or span by span in the scheme's
// styles. A right-to-left line, and the line holding composing text
// (its columns are not the logical ones), draw plain.
func (t *TextArea) drawRow(cv *render.Canvas, r visualRow, line []rune, sh *render.ShapedText, rowX, baseline int, textCol render.Color, rtl bool, fade func(render.Color) render.Color) {
	if t.highlighter == nil || rtl || r.line >= len(t.hl) || (t.composing() && r.line == t.peAt.line) {
		sh.Draw(cv, rowX, baseline, textCol)
		return
	}
	at := r.startCol
	segment := func(to int, st SchemeStyle) {
		if to <= at {
			return
		}
		col := textCol
		if st.Color != 0 {
			col = fade(st.Color)
		}
		face := t.font()
		if (st.Bold || st.Italic) && t.variants != nil {
			if f := t.variants(st.Bold, st.Italic); f != nil {
				face = t.faces.get(f, t.style(t), 0, 0)
			}
		}
		x := rowX + int(sh.CaretX(at-r.startCol)+0.5)
		face.ShapeDir(string(line[at:to]), t.px(), t.dir).Draw(cv, x, baseline, col)
		at = to
	}
	for _, sp := range t.hl[r.line].spans {
		if sp.End <= r.startCol || sp.Start >= r.endCol {
			continue
		}
		segment(max(sp.Start, r.startCol), SchemeStyle{})
		st, _ := t.scheme.Style(sp.Class)
		segment(min(sp.End, r.endCol), st)
	}
	segment(r.endCol, SchemeStyle{})
}

// enterIndent is what Enter adds after the newline: the current line's
// leading whitespace, up to the caret, with auto-indent on.
func (t *TextArea) enterIndent() string {
	if !t.autoIndent {
		return ""
	}
	c := t.clamp(t.cursor)
	line := t.lines[c.line]
	n := 0
	for n < len(line) && n < c.col && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return string(line[:n])
}

// revealCaret keeps the caret showing in the nearest Scroll while the
// area is focused: after the caret moved, the next arrangement scrolls
// it into view (GtkTextView following its cursor). An unmoved caret
// leaves the scroll where the user put it.
func (t *TextArea) revealCaret() {
	if !t.focused || t.face == nil {
		return
	}
	c := t.caretPos()
	if t.revealedOK && c == t.revealed {
		return
	}
	t.revealed, t.revealedOK = c, true
	reveal(t, t.IMECursorRect(), false)
}

// appendStyleKids is the text node (styleKids).
func (t *TextArea) appendStyleKids(buf []Widget) []Widget { return append(buf, &t.text) }
