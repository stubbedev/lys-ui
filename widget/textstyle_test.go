package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestCSSLetterSpacing pins letter-spacing through the cascade: a
// label measures wider by the spacing per cluster, and an entry's
// caret lands on the spaced text.
func TestCSSLetterSpacing(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `.wide { letter-spacing: 2px; }`)
	root := NewBox(Column, 0, 0)
	plain := NewLabel(face, 14, "spacing", DarkTheme().Text)
	wide := NewLabel(face, 14, "spacing", DarkTheme().Text)
	wide.AddClass("wide")
	e := NewEntry(face, 14, DarkTheme().Text)
	e.SetText("abc")
	e.AddClass("wide")
	for _, w := range []Widget{plain, wide, e} {
		root.Append(w, false)
	}
	arrangeTree(t, root, 300, 200)
	con := Constraints{Max: Size{W: 300, H: 100}}
	if d := wide.Measure(con).W - plain.Measure(con).W; d < 13 || d > 15 {
		t.Errorf("seven clusters at 2px widened the label by %d, want 14", d)
	}
	sh := e.shape("abc")
	if base := face.Shape("abc", 14); sh.Advance()-base.Advance() < 5.9 {
		t.Errorf("the entry shapes unspaced: %v vs %v", sh.Advance(), base.Advance())
	}
}

// TestGoldenTextStyle pins letter-spacing and decorations through CSS
// on a label, an entry, and a right-to-left rich label with a link.
func TestGoldenTextStyle(t *testing.T) {
	face := goldenFace(t)
	hb, err := render.NewFixtureHebrewTypeface()
	if err != nil {
		t.Fatal(err)
	}
	th := DarkTheme()
	root := NewBox(Column, 6, 0)
	root.AttachStylesheet(NewStylesheet(`
		.wide  { letter-spacing: 3px; text-decoration: underline; }
		.wavy  { text-decoration: wavy underline #f38ba8; }
		.strike { text-decoration: line-through; }
	`, StylePriorityUser))
	wide := NewLabel(face, 15, "Spaced title", th.Text)
	wide.AddClass("wide")
	wavy := NewLabel(face, 15, "Mispeled word", th.Text)
	wavy.AddClass("wavy")
	e := NewEntry(face, 14, th.Text)
	e.SetText("Struck entry")
	e.AddClass("strike")
	rtl := NewRichLabel(render.NewChain(face, hb), 15, `שלום <a href="x">עולם</a>`, th.Text)
	rtl.SetDirection(DirectionRTL)
	for _, w := range []Widget{wide, wavy, e, rtl} {
		root.Append(w, false)
	}
	NewGolden(t, root, "text-style", goldenTheme(th), goldenFrame(220, 150))
}
