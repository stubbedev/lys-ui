package widget

import (
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// entryShot paints e arranged at its natural size on a black canvas.
func entryShot(t *testing.T, e *Entry) ([]byte, int, Size) {
	t.Helper()
	sz := e.Measure(Constraints{Max: Size{W: 400, H: 100}})
	e.Arrange(render.Rect{W: sz.W, H: sz.H})
	stride := render.Stride(sz.W)
	data := make([]byte, stride*sz.H)
	cv := render.New(data, stride, sz.W, sz.H)
	cv.Clear(cv.Rect(), render.RGB(0, 0, 0))
	e.Paint(cv)
	return data, stride, sz
}

// inked reports the leftmost x holding a pixel near col, or -1, and
// how many there are.
func inked(data []byte, stride int, sz Size, col render.Color) (first, n int) {
	first = -1
	for x := range sz.W {
		for y := range sz.H {
			o := y*stride + x*4
			if near(data[o+2], col.R()) && near(data[o+1], col.G()) && near(data[o], col.B()) {
				if first < 0 {
					first = x
				}
				n++
			}
		}
	}
	return first, n
}

func near(a, b uint8) bool { return int(a)-int(b) < 40 && int(b)-int(a) < 40 }

const entryNodesCSS = `
	entry { padding: 0px 10px; border: 1px solid #202020; background-color: #000000; min-height: 0px; }
	entry > text { padding: 4px 3px; margin: 0px 2px; color: #ff0000; }
	entry > text > placeholder { color: #00ff00; }
	entry > text > selection { background-color: #0000ff; color: #ffff00; }
`

func TestEntryTextNodeInsetsAndColorsTheText(t *testing.T) {
	loadCSS(t, entryNodesCSS)
	face := entryFace(t)
	e := NewEntry(face, 14, 0)
	e.SetText("abc")
	if in := e.textInsets(); in != (render.Insets{Top: 5, Right: 16, Bottom: 5, Left: 16}) {
		t.Errorf("text insets %+v, want border+padding+text box: 5 / 16", in)
	}
	lg := face.Shape("lg", 14)
	data, stride, sz := entryShot(t, e)
	if want := int(lg.Ascent()+lg.Descent()+0.5) + 10; sz.H != want {
		t.Errorf("height %d, want the line plus 10 of insets (%d)", sz.H, want)
	}
	first, n := inked(data, stride, sz, render.RGB(0xff, 0, 0))
	if n == 0 || first < 16 || first > 20 {
		t.Errorf("text ink in the text node's red from x=%d (%d px), want from the 16px inset", first, n)
	}
	if !slices.Contains(styleKids(e), Widget(&e.text)) || parentOf(&e.text) != Widget(e) ||
		parentOf(&e.text.placeholder) != Widget(&e.text) || !slices.Contains(styleKids(&e.text), Widget(&e.text.selection)) {
		t.Error("the text node tree is not parented and walked")
	}

	e.SetText("")
	e.SetPlaceholder("hint")
	e.SetTextWidth(100)
	data, stride, sz = entryShot(t, e)
	phFirst, phN := inked(data, stride, sz, render.RGB(0, 0xff, 0))
	if phN == 0 || phFirst < 16 || phFirst > 20 {
		t.Errorf("placeholder in its green from x=%d (%d px), want from the 16px inset", phFirst, phN)
	}
	// It inks exactly where the same text typed would.
	typed := NewEntry(face, 14, 0)
	typed.SetText("hint")
	typed.SetTextWidth(100)
	tdata, tstride, tsz := entryShot(t, typed)
	if first, n := inked(tdata, tstride, tsz, render.RGB(0xff, 0, 0)); first != phFirst || n != phN {
		t.Errorf("placeholder inks %d px from x=%d, the typed text %d px from x=%d", phN, phFirst, n, first)
	}
	if _, n := inked(data, stride, sz, render.RGB(0xff, 0, 0)); n != 0 {
		t.Errorf("the placeholder painted %d px in the text's color", n)
	}

	e.SetText("abc")
	e.SelectAll()
	data, stride, sz = entryShot(t, e)
	if _, n := inked(data, stride, sz, render.RGB(0, 0, 0xff)); n == 0 {
		t.Error("no selection background")
	}
	if _, n := inked(data, stride, sz, render.RGB(0xff, 0xff, 0)); n == 0 {
		t.Error("the selected text is not in the selection's color")
	}
	if _, n := inked(data, stride, sz, render.RGB(0xff, 0, 0)); n != 0 {
		t.Errorf("%d px of selected text kept the text color", n)
	}
}

func TestEntryPartsStyleOnlyWhatTheirRulesName(t *testing.T) {
	// A field color reaches the text but not the placeholder, and a
	// selection without rules keeps the translucent accent and the
	// text's color.
	loadCSS(t, `entry { color: #ff0000; background-color: #000000; }`)
	face := entryFace(t)
	e := NewEntry(face, 14, 0)
	e.SetPlaceholder("hint")
	data, stride, sz := entryShot(t, e)
	if _, n := inked(data, stride, sz, render.RGB(0xff, 0, 0)); n != 0 {
		t.Errorf("the placeholder took the field's color (%d px)", n)
	}
	if first, _ := inked(data, stride, sz, Current().DisabledText()); first < 8 {
		t.Errorf("unstyled placeholder at x=%d, want inside the 8px padding", first)
	}
	if in := e.textInsets(); in != entryPad {
		t.Errorf("unstyled text insets %+v, want %+v", in, entryPad)
	}
	e.SetText("abc")
	e.SelectAll()
	data, stride, sz = entryShot(t, e)
	if _, n := inked(data, stride, sz, render.RGB(0xff, 0, 0)); n == 0 {
		t.Error("selected text lost the field's color without a selection rule")
	}
}

func TestSpinButtonTextNode(t *testing.T) {
	loadCSS(t, `spinbutton > text { padding-left: 20px; } entry > text { padding-left: 1px; }`)
	face := entryFace(t)
	s := NewSpinButton(face, 14, 0, 0, 10, 1, 0)
	if in := s.textInsets(); in.Left != entryPad.Left+20 {
		t.Errorf("spin text inset %d, want the spinbutton rule's 20 over the padding", in.Left)
	}
	e := NewEntry(face, 14, 0)
	if in := e.textInsets(); in.Left != entryPad.Left+1 {
		t.Errorf("entry text inset %d, want its own rule's 1", in.Left)
	}
}

func TestEntryTextMinHeightFloorsTheLine(t *testing.T) {
	loadCSS(t, `entry { padding: 0px; } entry > text { min-height: 40px; }`)
	e := NewEntry(entryFace(t), 14, 0)
	if h := e.Measure(Constraints{Max: Size{W: 400, H: 100}}).H; h != 40 {
		t.Errorf("height %d, want the text node's 40px floor", h)
	}
	loadCSS(t, `entry { padding: 0px; min-height: 0px; } entry > text { min-height: 2px; }`)
	e = NewEntry(entryFace(t), 14, 0)
	lg := entryFace(t).Shape("lg", 14)
	if h := e.Measure(Constraints{Max: Size{W: 400, H: 100}}).H; h != int(lg.Ascent()+lg.Descent()+0.5) {
		t.Errorf("height %d, want the line over a smaller floor", h)
	}
}

// A row too narrow for its fields takes the shortfall from their text
// widths (GtkText's minimum is no text), never below the insets and
// min-width; a row with room keeps them whole.
func TestEntriesShrinkInATightRow(t *testing.T) {
	loadCSS(t, `entry.floor { min-width: 120px; }`)
	face := entryFace(t)
	a, b := NewEntry(face, 14, 0), NewEntry(face, 14, 0)
	a.SetTextWidth(GTKTextWidth)
	b.SetTextWidth(GTKTextWidth)
	row := NewBox(Row, 0, 0)
	row.Append(a, true)
	row.Append(b, true)
	nat := a.Measure(Constraints{Max: Size{W: 1000, H: 100}}).W
	row.Measure(Constraints{Max: Size{W: 1000, H: 100}})
	if a.ShrinkableWidth() != nat-(entryPad.Left+entryPad.Right) {
		t.Errorf("shrinkable %d, want the text width %d", a.ShrinkableWidth(), nat-(entryPad.Left+entryPad.Right))
	}
	row.Arrange(render.Rect{W: 2 * nat, H: 40})
	if a.bounds.W != nat || b.bounds.W != nat {
		t.Errorf("with room: widths %d, %d, want %d each", a.bounds.W, b.bounds.W, nat)
	}
	row.Arrange(render.Rect{W: 200, H: 40})
	if a.bounds.W != 100 || b.bounds.W != 100 || b.bounds.X != 100 {
		t.Errorf("tight row: %v %v, want two 100px fields side by side", a.bounds, b.bounds)
	}
	b.AddClass("floor")
	row.Measure(Constraints{Max: Size{W: 1000, H: 100}})
	if got := b.ShrinkableWidth(); got != nat-120 {
		t.Errorf("min-width floor: shrinkable %d, want %d", got, nat-120)
	}
}

// Width in characters (GtkEntry width-chars, max-width-chars): the
// natural text width is max characters, the floor a tight row stops
// at is min characters, and the text typed changes neither.
func TestEntryWidthChars(t *testing.T) {
	face := entryFace(t)
	con := Constraints{Max: Size{W: 1000, H: 100}}
	e := NewEntry(face, 14, 0)
	in := e.textInsets()
	pad := in.Left + in.Right
	cp := e.charPx()
	e.SetWidthChars(6, 10)
	if mn, mx := e.WidthChars(); mn != 6 || mx != 10 {
		t.Errorf("WidthChars = %d, %d", mn, mx)
	}
	if w := e.Measure(con).W; w != pad+10*cp {
		t.Errorf("natural %d, want ten characters (%d)", w, pad+10*cp)
	}
	e.SetText(strings.Repeat("W", 40))
	if w := e.Measure(con).W; w != pad+10*cp {
		t.Errorf("a long text resized the field to %d", w)
	}
	if got := e.ShrinkableWidth(); got != 4*cp {
		t.Errorf("shrinkable %d, want down to six characters (%d)", got, 4*cp)
	}
	// Min alone: the GTK natural width, at least min.
	e.SetWidthChars(30, 0)
	if w := e.Measure(con).W; w != pad+max(GTKTextWidth, 30*cp) {
		t.Errorf("min only: %d", w)
	}
	e.SetWidthChars(0, 0)
	if w := e.Measure(con).W; w <= pad+10*cp {
		t.Errorf("unset: %d, want the text's width back", w)
	}
}

// SetColor's ink outranks the stylesheet; zero hands the text back to
// the text node's rule - a SearchEntry, born with the theme's ink,
// takes the application's stylesheet that way.
func TestEntrySetColorYieldsToTheStylesheetAtZero(t *testing.T) {
	loadCSS(t, entryNodesCSS)
	blue := render.RGB(0, 0, 0xff)
	s := NewSearchEntry(entryFace(t), 14, "")
	s.SetText("abc")
	s.SetColor(blue)
	if s.Color() != blue {
		t.Fatalf("color = %#08x, want blue", s.Color())
	}
	data, stride, sz := entryShot(t, s.Entry)
	if _, n := inked(data, stride, sz, blue); n == 0 {
		t.Error("the programmatic blue does not paint the text")
	}
	if _, n := inked(data, stride, sz, render.RGB(0xff, 0, 0)); n != 0 {
		t.Error("the stylesheet's red painted over the programmatic ink")
	}
	s.SetColor(0)
	data, stride, sz = entryShot(t, s.Entry)
	if _, n := inked(data, stride, sz, render.RGB(0xff, 0, 0)); n == 0 {
		t.Error("with no programmatic ink the text node's red does not paint")
	}
}
