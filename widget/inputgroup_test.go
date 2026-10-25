package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget/css"
)

// The focus ring of a focused entry goes around its whole group.
func TestInputGroupOwnsTheFocusRing(t *testing.T) {
	face := goldenFace(t)
	e := NewEntry(face, 14, 0)
	g := NewInputGroup(e)
	g.Prepend(NewSymbol(SymbolCrosshair, 16))
	arrangeTree(t, g, 300, 36)
	if FocusRingOwner(e) != Widget(g) {
		t.Fatal("the entry owns its ring inside a group")
	}
	if FocusRingRect(e) != FocusRingRect(g) {
		t.Error("the entry's ring rect is not the group's")
	}
}

// TestGoldenInputGroup pins groups with a leading icon, trailing
// text, a trailing button, a focused entry and an invalid group, in
// both palettes.
func TestGoldenInputGroup(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			col := NewBox(Column, 16, 0)
			search := NewEntry(face, 14, 0)
			search.SetPlaceholder("Search...")
			sg := NewInputGroup(search)
			sg.Prepend(NewSymbol(SymbolCrosshair, 16))
			sg.Append(NewLabel(face, 14, "12 results", 0))
			col.Append(sg, false)
			url := NewEntry(face, 14, 0)
			url.SetText("example.com")
			ug := NewInputGroup(url)
			ug.Prepend(NewLabel(face, 14, "https://", 0))
			copyBtn := NewButton(NewLabel(face, 12, "Copy", 0), 0, 0)
			copyBtn.AddClass(css.Ghost, css.Small)
			ug.Append(copyBtn)
			col.Append(ug, false)
			bad := NewInputGroup(NewEntry(face, 14, 0))
			bad.Prepend(NewLabel(face, 14, "@", 0))
			bad.AddClass(css.Error)
			col.Append(bad, false)
			NewGolden(t, col, "inputgroup-"+tc.name, goldenTheme(tc.th), goldenFrame(360, 140), goldenFocus(search))
		})
	}
}
