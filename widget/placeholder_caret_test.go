package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

var placeholderCaretColor = render.RGB(0xff, 0x00, 0x00)

// placeholderCaretField is a text field with a placeholder and no text,
// and its caret rect.
type placeholderCaretField struct {
	w     Widget
	caret func() (render.Rect, bool)
}

// assertPlaceholderCaret paints f blurred and focused and checks the
// caret: none while blurred, the caret color filling its rect while
// focused, and the placeholder drawn under both.
func assertPlaceholderCaret(t *testing.T, f placeholderCaretField) {
	t.Helper()
	pinAnimClock(t)
	loadCSS(t, `entry, textview { caret-color: #ff0000; } text > placeholder { color: #00ff00; }`)
	const w, h = 200, 80
	other := newFocusTarget()
	root := NewBox(Column, 0, 0)
	root.Append(f.w, false).Append(other, false)
	arrangeTree(t, root, w, h)
	r := &Router{Root: root}
	shot := func() []byte {
		data := make([]byte, render.Stride(w)*h)
		f.w.Paint(render.New(data, render.Stride(w), w, h))
		return data
	}
	countOf := func(data []byte, c render.Color) int {
		n := 0
		for y := range h {
			for x := range w {
				if pxAt(data, w, x, y) == c {
					n++
				}
			}
		}
		return n
	}
	ink := func(data []byte) int {
		n := 0
		for y := range h {
			for x := range w {
				if c := pxAt(data, w, x, y); c.G() > 128 && c.R() < 64 && c.B() < 64 {
					n++
				}
			}
		}
		return n
	}

	if err := r.SetFocus(other); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.caret(); ok {
		t.Fatal("a blurred field has a caret")
	}
	blurred := shot()
	if n := countOf(blurred, placeholderCaretColor); n != 0 {
		t.Errorf("a blurred field painted %d caret pixels", n)
	}
	if ink(blurred) == 0 {
		t.Error("a blurred field painted no placeholder")
	}

	if err := r.SetFocus(f.w); err != nil {
		t.Fatal(err)
	}
	caret, ok := f.caret()
	if !ok {
		t.Fatal("a focused empty field has no caret")
	}
	focused := shot()
	if got := pxAt(focused, w, caret.X, caret.Y+caret.H/2); got != placeholderCaretColor {
		t.Errorf("caret %v painted %v, want the caret color %v", caret, got, placeholderCaretColor)
	}
	if ink(focused) == 0 {
		t.Error("the caret hid the placeholder")
	}
}

func TestEntryPaintsCaretOverPlaceholder(t *testing.T) {
	for _, tc := range []struct {
		name string
		echo Echo
	}{{"normal", EchoNormal}, {"password", EchoPassword}} {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEntry(testFace(t), 14, render.RGB(0xff, 0xff, 0xff))
			e.SetEcho(tc.echo)
			e.SetPlaceholder("Password")
			assertPlaceholderCaret(t, placeholderCaretField{e, func() (render.Rect, bool) {
				return e.caretRect(e.shape(e.displayText()))
			}})
			if c, _ := e.caretRect(e.shape(e.displayText())); c.X != e.contentRect().X {
				t.Errorf("caret x %d, want the start edge %d", c.X, e.contentRect().X)
			}
		})
	}
}

func TestTextAreaPaintsCaretOverPlaceholder(t *testing.T) {
	ta := NewTextArea(testFace(t), 14, render.RGB(0xff, 0xff, 0xff))
	ta.SetPlaceholder("Message")
	assertPlaceholderCaret(t, placeholderCaretField{ta, ta.caretRect})
	if c, _ := ta.caretRect(); c.X != ta.textRect().X {
		t.Errorf("caret x %d, want the start edge %d", c.X, ta.textRect().X)
	}
}
