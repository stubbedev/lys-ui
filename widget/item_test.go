package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget/css"
)

// TestGoldenShadcnItemButtonGroup pins the item variants and sizes,
// and joined button groups - horizontal, nested with a separator, and
// vertical - in both palettes.
func TestGoldenShadcnItemButtonGroup(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			button := func(text string) *Button {
				b := NewButton(NewLabel(face, 14, text, 0), 0, 0)
				b.AddClass(css.Outline)
				return b
			}
			col := NewBox(Column, 12, 0)
			for _, v := range []ItemVariant{ItemDefault, ItemOutline, ItemMuted} {
				it := NewItem(face, "Basic Item", "A simple item with title and description.")
				it.SetMedia(NewSymbol(SymbolSearch, 16), false)
				it.AddAction(button("Action"))
				it.SetVariant(v)
				col.Append(it, false)
			}
			small := NewItem(face, "Your profile has been verified.", "")
			small.SetVariant(ItemOutline)
			small.SetSmall(true)
			col.Append(small, false)
			row := NewBox(Row, 16, 0)
			row.AppendAligned(NewButtonGroup(button("Archive"), button("Report"), button("Snooze")), false, AlignStart)
			nested := NewButtonGroup(button("Copy"), NewSeparator(Vertical), button("Paste"))
			row.AppendAligned(NewButtonGroup(nested, NewButtonGroup(button("More"))), false, AlignStart)
			vertical := NewButtonGroup(button("Top"), button("Middle"), button("Bottom"))
			vertical.SetVertical(true)
			row.AppendAligned(vertical, false, AlignStart)
			col.Append(row, false)
			NewGolden(t, col, "shadcn-item-"+tc.name, goldenTheme(tc.th), goldenFrame(640, 470))
		})
	}
}

// An ItemGroup puts one separator between consecutive items and none
// around them (#265).
func TestItemGroupSeparators(t *testing.T) {
	face := goldenFace(t)
	g := NewItemGroup(NewItem(face, "shadcn", "shadcn@vercel.com"), NewItem(face, "maxleiter", "maxleiter@vercel.com"))
	g.Append(NewItem(face, "evilrabbit", "evilrabbit@vercel.com"))
	kids := g.box.Children()
	seps := 0
	for i, k := range kids {
		_, isSep := k.(*Separator)
		if isSep != (i%2 == 1) {
			t.Fatalf("child %d is %T, want items and separators alternating", i, k)
		}
		if isSep {
			seps++
		}
	}
	if len(kids) != 5 || seps != 2 {
		t.Errorf("%d children with %d separators, want 3 items and 2 separators", len(kids), seps)
	}
}

func TestGoldenItemGroup(t *testing.T) {
	face := goldenFace(t)
	for _, shot := range bothPalettes("itemgroup") {
		g := NewItemGroup(
			NewItem(face, "shadcn", "shadcn@vercel.com"),
			NewItem(face, "maxleiter", "maxleiter@vercel.com"),
			NewItem(face, "evilrabbit", "evilrabbit@vercel.com"),
		)
		NewGolden(t, g, shot.name, goldenTheme(shot.th), goldenFrame(320, 250))
	}
}
