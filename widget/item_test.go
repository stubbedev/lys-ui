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

// An item's text updates in place, an empty description hiding its
// line, and an activatable item fires on a click outside its actions
// and on Enter, while a plain one takes no focus (#270).
func TestItemLive(t *testing.T) {
	face := goldenFace(t)
	it := NewItem(face, "Headphones", "")
	if IsVisible(it.desc) {
		t.Error("an empty description shows its line")
	}
	it.SetTitle("Speakers")
	it.SetDescription("Connecting…")
	if it.title.Text() != "Speakers" || it.desc.Text() != "Connecting…" || !IsVisible(it.desc) {
		t.Errorf("after the setters: %q / %q visible %v", it.title.Text(), it.desc.Text(), IsVisible(it.desc))
	}
	it.SetDescription("")
	if IsVisible(it.desc) {
		t.Error("clearing the description kept its line")
	}

	if isFocusable(it) {
		t.Error("a plain item takes the focus")
	}
	fired := 0
	it.SetOnActivate(func() { fired++ })
	action := NewButton(NewLabel(face, 14, "Forget", 0), 0, 0)
	clicked := 0
	action.OnClick = func() { clicked++ }
	it.AddAction(action)
	arrangeTree(t, it, 400, 80)
	if !isFocusable(it) {
		t.Error("an activatable item takes no focus")
	}
	b := it.Bounds()
	if hit := it.HitTest(Point{X: b.X + 20, Y: b.Y + b.H/2}); hit != Widget(it) {
		t.Errorf("a click on the text hits %T, want the item", hit)
	}
	ab := action.Bounds()
	if hit := it.HitTest(Point{X: ab.X + ab.W/2, Y: ab.Y + ab.H/2}); hit != Widget(action) {
		t.Errorf("a click on the action hits %T, want the action", hit)
	}
	it.ClickAt(Point{})
	it.KeyAction(KeyEnter, 0)
	if fired != 2 {
		t.Errorf("activations %d, want the click and Enter", fired)
	}
}

// Removing an item from a group drops the separator beside it, wherever
// the item sits, and Clear empties it.
func TestItemGroupRemove(t *testing.T) {
	face := goldenFace(t)
	items := []*Item{NewItem(face, "a", ""), NewItem(face, "b", ""), NewItem(face, "c", ""), NewItem(face, "d", "")}
	g := NewItemGroup()
	for _, it := range items {
		g.Append(it)
	}
	check := func(want ...*Item) {
		t.Helper()
		kids := g.box.Children()
		if len(kids) != 2*len(want)-1 {
			t.Fatalf("%d children for %d items", len(kids), len(want))
		}
		for i, k := range kids {
			if i%2 == 1 {
				if _, ok := k.(*Separator); !ok {
					t.Fatalf("child %d is %T, want a separator", i, k)
				}
				continue
			}
			if k != Widget(want[i/2]) {
				t.Fatalf("child %d is the wrong item", i)
			}
		}
	}
	if !g.Remove(items[1]) {
		t.Fatal("Remove of a member reported false")
	}
	check(items[0], items[2], items[3])
	g.Remove(items[0])
	check(items[2], items[3])
	g.Remove(items[3])
	check(items[2])
	if g.Remove(items[1]) {
		t.Error("removing a non-member reported true")
	}
	g.Clear()
	g.Append(items[0])
	check(items[0])
}

// The composites' text follows the root font size, their parts sized
// in rem, and a stylesheet part rule still wins (#270, #271).
func TestCompositeTextFollowsTheRootSize(t *testing.T) {
	face := goldenFace(t)
	t.Cleanup(func() { SetRootFontSize(16) })
	px := func(l *Label) float64 { _, p := l.effStyle(); return p }
	it := NewItem(face, "Speakers", "Connected")
	sp := NewStatusPage(face, "", "No devices", "Turn Bluetooth on")
	arrangeTree(t, it, 400, 100)
	arrangeTree(t, sp, 400, 300)
	if px(it.title) != 14 || px(sp.title) != 18 {
		t.Fatalf("defaults: item title %v, status title %v; want 14 and 18", px(it.title), px(sp.title))
	}
	SetRootFontSize(20)
	arrangeTree(t, it, 400, 100)
	arrangeTree(t, sp, 400, 300)
	if px(it.title) != 17.5 || px(it.desc) != 17.5 || px(sp.title) != 22.5 || px(sp.desc) != 17.5 {
		t.Errorf("at a 20px root: item %v/%v, status %v/%v; want 17.5/17.5 and 22.5/17.5", px(it.title), px(it.desc), px(sp.title), px(sp.desc))
	}
	loadCSS(t, `item .title { font-size: 30px; }`)
	arrangeTree(t, it, 400, 100)
	if px(it.title) != 30 {
		t.Errorf("a part rule gave the title %v, want 30", px(it.title))
	}
}

// An ellipsizing item gives up its width down to the ellipsis instead
// of its longest token, so a narrow row keeps its actions inside.
func TestItemEllipsize(t *testing.T) {
	face := goldenFace(t)
	it := NewItem(face, "WH-1000XM4-WIRELESS-NOISE-CANCELLING", "00:1B:66:A2:5C:3F:00:1B:66:A2")
	it.AddAction(NewButton(NewLabel(face, 14, "Connect", 0), 0, 0))
	wrapped := MinWidth(it)
	it.SetEllipsize(true)
	if got := MinWidth(it); got >= wrapped {
		t.Errorf("ellipsizing item's minimum %d, not below the wrapping one's %d", got, wrapped)
	}
}
