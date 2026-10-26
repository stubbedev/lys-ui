package widget

import (
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// loadVF loads the Quicksand variable test face (wght 300-700).
func loadVF(t *testing.T) *render.Typeface {
	t.Helper()
	data, err := os.ReadFile("testdata/Quicksand-VF.ttf")
	if err != nil {
		t.Fatal(err)
	}
	tf, err := render.LoadFont(data)
	if err != nil {
		t.Fatal(err)
	}
	return tf
}

// TestCSSVariableWeight pins the cascade on a variable constructor
// face: font-weight instantiates wght, font-variation-settings wins
// over it, and the derived face is stable across style passes.
func TestCSSVariableWeight(t *testing.T) {
	vf := loadVF(t)
	loadCSS(t, `.bold { font-weight: 700; } .axis { font-weight: 700; font-variation-settings: "wght" 450; }`)
	root := NewBox(Column, 0, 0)
	bold := NewLabel(vf, 16, "Bold", DarkTheme().Text)
	bold.AddClass("bold")
	axis := NewLabel(vf, 16, "Axis", DarkTheme().Text)
	axis.AddClass("axis")
	root.Append(bold, false)
	root.Append(axis, false)
	arrangeTree(t, root, 200, 100)
	bf, _ := bold.effStyle()
	af, _ := axis.effStyle()
	if bf != render.Font(vf.Weighted(700)) {
		t.Errorf("font-weight 700 resolved %v", bf.(*render.Typeface).Variations())
	}
	if got := af.(*render.Typeface).Variations(); len(got) != 1 || got[0].Value != 450 {
		t.Errorf("font-variation-settings did not win: %v", got)
	}
	if again, _ := bold.effStyle(); again != bf {
		t.Error("the derived face changed identity between passes")
	}
}

// TestGoldenVariableWeights pins wght 300, 400 and 700 set through CSS.
func TestGoldenVariableWeights(t *testing.T) {
	vf := loadVF(t)
	root := NewBox(Column, 4, 0)
	root.AttachStylesheet(NewStylesheet(`.w3 { font-weight: 300; } .w4 { font-weight: 400; } .w7 { font-variation-settings: "wght" 700; }`, StylePriorityUser))
	for _, c := range []string{"w3", "w4", "w7"} {
		l := NewLabel(vf, 20, "Variable "+c, DarkTheme().Text)
		l.AddClass(c)
		root.Append(l, false)
	}
	NewGolden(t, root, "variable-weights", goldenTheme(DarkTheme()))
}
