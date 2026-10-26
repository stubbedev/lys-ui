package widget

import (
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// areaShot lays ta out at w x h and paints it over black.
func areaShot(ta *TextArea, w, h int) ([]byte, int) {
	ta.Measure(Constraints{Max: Size{W: w, H: h}})
	ta.Arrange(render.Rect{W: w, H: h})
	stride := render.Stride(w)
	data := make([]byte, stride*h)
	cv := render.New(data, stride, w, h)
	cv.Clear(cv.Rect(), render.RGB(0, 0, 0))
	ta.Paint(cv)
	return data, stride
}

// inkIn counts pixels near col inside r.
func inkIn(data []byte, stride int, r render.Rect, col render.Color) int {
	n := 0
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			o := y*stride + x*4
			if near(data[o+2], col.R()) && near(data[o+1], col.G()) && near(data[o], col.B()) && (data[o] > 60 || data[o+1] > 60 || data[o+2] > 60) {
				n++
			}
		}
	}
	return n
}

func TestTextAreaLineNumbers(t *testing.T) {
	loadCSS(t, `textview { background-color: #000000; }`)
	ta := NewTextArea(entryFace(t), 14, render.RGB(0xff, 0xff, 0xff))
	ta.SetText("one\ntwo\n" + string(make([]rune, 0)) + "three")
	plain := ta.textRect()
	ta.SetLineNumbers(true)
	if !ta.LineNumbers() {
		t.Error("LineNumbers does not report SetLineNumbers")
	}
	numCol := render.RGB(0, 0xff, 0)
	ta.SetHighlighter(nil, TextScheme{"line-numbers": {Color: numCol}})
	data, stride := areaShot(ta, 300, 100)
	g := ta.gutterWidth()
	if g < 2*gutterPad+10 || ta.textRect().X != ta.bounds.X+textPad.Left+g || plain.X == ta.textRect().X {
		t.Fatalf("gutter %d, text at %d (plain %d)", g, ta.textRect().X, plain.X)
	}
	gutter := render.Rect{X: textPad.Left, Y: 0, W: g, H: 100}
	lineH := ta.lineHeight()
	for l := range 3 {
		row := render.Rect{X: gutter.X, Y: textPad.Top + l*lineH, W: g, H: lineH}
		if inkIn(data, stride, row, numCol) == 0 {
			t.Errorf("line %d has no number in the gutter", l+1)
		}
	}
	if inkIn(data, stride, render.Rect{X: gutter.X, Y: textPad.Top + 3*lineH, W: g, H: lineH}, numCol) != 0 {
		t.Error("a number past the last line")
	}
	// A wrapped line numbers its first row only.
	ta.SetText("word word word word word word word word word word word")
	data, stride = areaShot(ta, 120, 200)
	if len(ta.rows) < 2 {
		t.Fatalf("premise: %d rows", len(ta.rows))
	}
	if inkIn(data, stride, render.Rect{X: gutter.X, Y: textPad.Top + lineH, W: g, H: lineH}, numCol) != 0 {
		t.Error("a wrapped row repeated the line number")
	}
	// The gutter widens for a three-digit line count.
	two := ta.gutterWidth()
	ta.SetText(string(make([]rune, 0)) + "x" + strings.Repeat("\nx", 120))
	if ta.gutterWidth() <= two {
		t.Errorf("121 lines keep the %dpx two-digit gutter", two)
	}
	ta.SetLineNumbers(false)
	if ta.gutterWidth() != 0 {
		t.Error("the gutter outlived SetLineNumbers(false)")
	}
}

// spanHL styles runes [from, to) of every line with class, counting
// its calls.
type spanHL struct {
	from, to int
	class    string
	calls    int
	states   []int
}

func (h *spanHL) Highlight(line []rune, state int) ([]TextSpan, int) {
	h.calls++
	h.states = append(h.states, state)
	end := state
	if slices.Contains(line, '"') {
		end = 1 - state
	}
	if len(line) < h.to {
		return nil, end
	}
	return []TextSpan{{Start: h.from, End: h.to, Class: h.class}}, end
}

func TestTextAreaHighlights(t *testing.T) {
	loadCSS(t, `textview { background-color: #000000; }`)
	ta := NewTextArea(entryFace(t), 14, render.RGB(0xff, 0xff, 0xff))
	ta.SetText("MMMMMMMM")
	red := render.RGB(0xff, 0, 0)
	h := &spanHL{from: 0, to: 4, class: "def:keyword"}
	// def:keyword falls back to def:statement.
	ta.SetHighlighter(h, TextScheme{"def:statement": {Color: red}})
	data, stride := areaShot(ta, 300, 60)
	c := ta.textRect()
	sh := ta.face.Shape("MMMM", ta.px())
	mid := c.X + int(sh.Advance())
	if inkIn(data, stride, render.Rect{X: c.X, Y: c.Y, W: mid - c.X, H: ta.lineHeight()}, red) == 0 {
		t.Error("the span is not in the scheme's color")
	}
	if inkIn(data, stride, render.Rect{X: mid + 1, Y: c.Y, W: 100, H: ta.lineHeight()}, red) != 0 {
		t.Error("color leaked past the span")
	}
	if inkIn(data, stride, render.Rect{X: mid + 1, Y: c.Y, W: 100, H: ta.lineHeight()}, render.RGB(0xff, 0xff, 0xff)) == 0 {
		t.Error("the unstyled rest lost the text color")
	}
	// The runes before a span keep the text color.
	ta.SetHighlighter(&spanHL{from: 4, to: 8, class: "def:keyword"}, TextScheme{"def:statement": {Color: red}})
	data, stride = areaShot(ta, 300, 60)
	if inkIn(data, stride, render.Rect{X: c.X, Y: c.Y, W: mid - c.X - 1, H: ta.lineHeight()}, render.RGB(0xff, 0xff, 0xff)) == 0 ||
		inkIn(data, stride, render.Rect{X: mid + 1, Y: c.Y, W: 100, H: ta.lineHeight()}, red) == 0 {
		t.Error("a span after plain text lost the plain run or its own color")
	}
	// An unstyled class paints the text color.
	ta.SetHighlighter(&spanHL{from: 0, to: 4, class: "def:nothing"}, TextScheme{})
	data, stride = areaShot(ta, 300, 60)
	if inkIn(data, stride, render.Rect{X: c.X, Y: c.Y, W: mid - c.X, H: ta.lineHeight()}, red) != 0 {
		t.Error("an unstyled class painted a color")
	}
}

func TestTextAreaHighlightCache(t *testing.T) {
	ta := newTextArea(t, "a\n\"b\nc\nd")
	h := &spanHL{to: 1, class: "x"}
	ta.SetHighlighter(h, TextScheme{})
	areaShot(ta, 200, 120)
	if h.calls != 4 || !slices.Equal(h.states, []int{0, 0, 1, 1}) {
		t.Fatalf("first paint: %d calls, states %v; want each line once, the quote's state carried", h.calls, h.states)
	}
	areaShot(ta, 200, 120)
	if h.calls != 4 {
		t.Errorf("an unchanged repaint highlighted %d more lines", h.calls-4)
	}
	// Editing the last line restyles it alone.
	ta.SetCursor(3, 1)
	ta.InsertRune('x')
	areaShot(ta, 200, 120)
	if h.calls != 5 {
		t.Errorf("editing one line restyled %d", h.calls-4)
	}
	// Removing the quote changes the state the lines after start in.
	ta.SetCursor(1, 1)
	ta.Backspace()
	areaShot(ta, 200, 120)
	if h.calls != 8 || !slices.Equal(h.states[5:], []int{0, 0, 0}) {
		t.Errorf("after the quote went: %d calls, states %v", h.calls, h.states)
	}
}

func TestTextAreaBoldUsesTheVariant(t *testing.T) {
	ta := newTextArea(t, "bold rest")
	var asked [][2]bool
	ta.SetVariants(func(bold, italic bool) *render.Typeface {
		asked = append(asked, [2]bool{bold, italic})
		return entryFace(t)
	})
	ta.SetHighlighter(&spanHL{from: 0, to: 4, class: "k"}, TextScheme{"k": {Bold: true}})
	areaShot(ta, 200, 60)
	if len(asked) == 0 || asked[0] != [2]bool{true, false} {
		t.Errorf("variants asked %v, want bold", asked)
	}
}

func TestTextAreaAutoIndent(t *testing.T) {
	ta := newTextArea(t, "  key = [")
	ta.SetCursor(0, 9)
	ta.KeyAction(KeyEnter, 0)
	if ta.Text() != "  key = [\n" {
		t.Errorf("without auto-indent: %q", ta.Text())
	}
	ta.SetAutoIndent(true)
	if !ta.AutoIndent() {
		t.Error("AutoIndent does not report SetAutoIndent")
	}
	ta.SetText("\t  key")
	ta.SetCursor(0, 6)
	ta.KeyAction(KeyEnter, 0)
	if line, col := ta.CursorPos(); ta.Text() != "\t  key\n\t  " || line != 1 || col != 3 {
		t.Errorf("auto-indent: %q caret %d:%d", ta.Text(), line, col)
	}
	// Inside the indentation it copies only what is before the caret.
	ta.SetText("    x")
	ta.SetCursor(0, 2)
	ta.KeyAction(KeyEnter, 0)
	if ta.Text() != "  \n    x" {
		t.Errorf("mid-indent: %q", ta.Text())
	}
}

func TestTextAreaKeepsTheCaretInItsScroll(t *testing.T) {
	ta := newTextArea(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12")
	scroll := NewScroll(ta)
	scroll.VerticalOnly = true
	scroll.PropagateNaturalWidth = true // as wide as its text in the outer scroll
	outer := NewScroll(scroll)
	scroll.SetMaxContentHeight(60)
	layoutRoot(outer, 200, 40)
	ta.focused = true
	ta.SetCursor(11, 0)
	layoutRoot(outer, 200, 40)
	if _, y := outer.Offset(); y != 0 {
		t.Errorf("the caret scrolled the outer view to %d: it follows in its own scroll only", y)
	}
	_, y := scroll.Offset()
	caret := ta.IMECursorRect()
	if y == 0 || caret.Y < scroll.view.Y || caret.Y+caret.H > scroll.view.Y+scroll.view.H {
		t.Fatalf("offset %d, caret %v, view %v: the caret is not in view", y, caret, scroll.view)
	}
	// The user scrolls away; the unmoved caret stays where it is.
	scroll.SetOffset(0, 0)
	layoutRoot(outer, 200, 40)
	if _, y := scroll.Offset(); y != 0 {
		t.Errorf("an unmoved caret scrolled back to %d", y)
	}
	// Unfocused, a moved caret scrolls nothing.
	ta.focused = false
	ta.SetCursor(10, 0)
	layoutRoot(outer, 200, 40)
	if _, y := scroll.Offset(); y != 0 {
		t.Errorf("an unfocused area scrolled to %d", y)
	}
}

func TestTextAreaCSSBoxAndTextNode(t *testing.T) {
	loadCSS(t, `
		textview.ed { padding: 10px; border: 2px solid #202020; background-color: #000000; font-size: 20px; }
		textview.ed > text { padding-left: 5px; color: #ff0000; }
		textview.ed > text > selection { background-color: #0000ff; color: #ffff00; }
	`)
	ta := NewTextArea(entryFace(t), 14, 0)
	ta.AddClass("ed")
	ta.SetText("MMMM")
	if in := ta.textInsets(); in != (render.Insets{Top: 12, Right: 12, Bottom: 12, Left: 17}) {
		t.Errorf("insets %+v, want border+padding and the text node's 5px", in)
	}
	if ta.px() != 20 || ta.lineHeight() != ta.face.Shape("lg", 20).LineHeight() {
		t.Errorf("px %v: the stylesheet's font-size is not used", ta.px())
	}
	data, stride := areaShot(ta, 300, 80)
	c := ta.textRect()
	if inkIn(data, stride, render.Rect{X: c.X, Y: c.Y, W: 100, H: ta.lineHeight()}, render.RGB(0xff, 0, 0)) == 0 {
		t.Error("the text is not in the text node's color")
	}
	ta.SelectAll()
	data, stride = areaShot(ta, 300, 80)
	if inkIn(data, stride, render.Rect{X: c.X, Y: c.Y, W: 100, H: ta.lineHeight()}, render.RGB(0, 0, 0xff)) == 0 ||
		inkIn(data, stride, render.Rect{X: c.X, Y: c.Y, W: 100, H: ta.lineHeight()}, render.RGB(0xff, 0xff, 0)) == 0 {
		t.Error("the selection lost its node's colors")
	}
	// A scheme's selection wins over the node's.
	ta.SetHighlighter(nil, TextScheme{"selection": {Background: render.RGB(0, 0xff, 0)}})
	data, stride = areaShot(ta, 300, 80)
	if inkIn(data, stride, render.Rect{X: c.X, Y: c.Y, W: 100, H: ta.lineHeight()}, render.RGB(0, 0xff, 0)) == 0 {
		t.Error("the scheme's selection did not paint")
	}
	if !slices.Contains(styleKids(ta), Widget(&ta.text)) || parentOf(&ta.text) != Widget(ta) {
		t.Error("the text node is not parented and walked")
	}
}

// The wrapped rows follow the stylesheet's font size.
func TestTextAreaRowsFollowTheFontSize(t *testing.T) {
	ta := newTextArea(t, "word word word word word word word")
	areaShot(ta, 200, 300)
	small := len(ta.rows)
	loadCSS(t, `textview { font-size: 28px; }`)
	areaShot(ta, 200, 300)
	if len(ta.rows) <= small {
		t.Errorf("%d rows at 28px, %d at 14px: the row cache kept the old size", len(ta.rows), small)
	}
}

func TestTextAreaOnChanged(t *testing.T) {
	ta := newTextArea(t, "ab")
	n := 0
	ta.OnChanged = func() { n++ }
	steps := []struct {
		name string
		do   func()
	}{
		{"typing", func() { ta.InsertRune('c') }},
		{"insert", func() { ta.Insert("de") }},
		{"enter", func() { ta.KeyAction(KeyEnter, 0) }},
		{"backspace", func() { ta.Backspace() }},
		{"delete word", func() { ta.SetCursor(0, 0); ta.DeleteWordForward() }},
		{"undo", func() { ta.Undo() }},
		{"redo", func() { ta.Redo() }},
		{"set text", func() { ta.SetText("new") }},
		{"input method delete", func() { ta.SetCursor(0, 2); ta.IMEDelete(1, 0) }},
		{"input method commit", func() { ta.IMECommit("ok") }},
	}
	for _, st := range steps {
		before := n
		st.do()
		if n != before+1 {
			t.Errorf("%s fired OnChanged %d times, want once", st.name, n-before)
		}
	}
	before := n
	ta.SetCursor(0, 1)
	ta.KeyAction(KeyRight, 0)
	ta.SelectAll()
	ta.SetReadOnly(true)
	ta.InsertRune('x')
	if n != before {
		t.Errorf("motion, selection and a read-only edit fired OnChanged %d times", n-before)
	}
}
