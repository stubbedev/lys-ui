package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/widget/css"
)

// newTestSheet builds a right sheet with a header, a field and a
// footer over a content button.
func newTestSheet(t *testing.T, side SheetSide) (*Sheet, *Button, *Entry, *Button) {
	t.Helper()
	t.Cleanup(anim.SetInstant(true))
	face := goldenFace(t)
	behind := NewButton(NewLabel(face, 14, "Open", 0), 8, 8)
	field := NewEntry(face, 14, 0)
	save := NewButton(NewLabel(face, 14, "Save changes", 0), 8, 8)
	save.AddClass(css.SuggestedAction)
	s := NewSheet(NewBox(Column, 0, 24).Append(behind, false), NewField(face, "Name", field), side)
	s.SetHeader(face, "Edit profile", "Make changes to your profile here.")
	s.SetFooter(save)
	arrangeTree(t, s, 600, 400)
	return s, behind, field, save
}

// A side sheet slides in over three quarters at most of the width, up
// to sheetWidth, on its own edge.
func TestSheetSides(t *testing.T) {
	for _, tc := range []struct {
		side SheetSide
		x    int
	}{{SheetRight, 600 - sheetWidth}, {SheetLeft, 0}} {
		s, _, _, _ := newTestSheet(t, tc.side)
		s.SetOpen(true)
		if r := s.sheetRect(); r.W != sheetWidth || r.X != tc.x || r.H != 400 {
			t.Errorf("side %d: sheet at %+v", tc.side, r)
		}
	}
	s, _, _, _ := newTestSheet(t, SheetTop)
	s.SetOpen(true)
	if r := s.sheetRect(); r.Y != 0 || r.W != 600 || r.H <= 0 {
		t.Errorf("top sheet at %+v", r)
	}
}

// While open, Tab stays in the panel and Escape anywhere in it closes
// the sheet; the close button closes it too.
func TestSheetTrapsFocusAndCloses(t *testing.T) {
	s, behind, field, save := newTestSheet(t, SheetRight)
	r := &Router{Root: s}
	s.SetOpen(true)
	arrangeTree(t, s, 600, 400)
	r.SetFocus(field)
	for range 6 {
		r.FocusNext()
		if r.Focused() == Widget(behind) {
			t.Fatal("Tab left the open sheet")
		}
	}
	r.SetFocus(field)
	r.KeyAction(KeyDismiss, 0)
	if s.Open() {
		t.Fatal("Escape in the field left the sheet open")
	}
	s.SetOpen(true)
	s.close.OnClick()
	if s.Open() {
		t.Error("the close button left the sheet open")
	}
	_ = save
}

// TestGoldenSheet pins an open right sheet with its header,
// field and footer, in both palettes.
func TestGoldenSheet(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			s, _, _, _ := newTestSheet(t, SheetRight)
			s.SetOpen(true)
			SizeWithin(s, Size{W: 600, H: 400})
			s.Arrange(render.Rect{W: 600, H: 400})
			NewGolden(t, s, "sheet-"+tc.name, goldenTheme(tc.th), goldenFrame(600, 400))
		})
	}
}
