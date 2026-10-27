package widget

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// testCommandGroups are two headed groups, one item disabled.
func testCommandGroups(picked *[]string) []CommandGroup {
	item := func(label, shortcut string, disabled bool, keywords ...string) CommandItem {
		return CommandItem{
			Label: label, Shortcut: shortcut, Disabled: disabled, Keywords: keywords,
			OnSelect: func() { *picked = append(*picked, label) },
		}
	}
	return []CommandGroup{
		{Heading: "Suggestions", Items: []CommandItem{
			item("Calendar", "", false), item("Search Emoji", "", false, "smiley"), item("Calculator", "", true),
		}},
		{Heading: "Settings", Items: []CommandItem{
			item("Profile", "Ctrl P", false), item("Billing", "Ctrl B", false), item("Settings", "Ctrl S", false),
		}},
	}
}

func shownLabels(c *Command) []string {
	var out []string
	for _, r := range c.shown {
		out = append(out, r.item.Label)
	}
	return out
}

// The query filters fuzzily by label and keyword, ranks the best match
// and its group first, and shows the empty state when nothing matches.
func TestCommandFilters(t *testing.T) {
	var picked []string
	c := NewCommand(goldenFace(t), "Type a command...", testCommandGroups(&picked)...)
	c.SetQuery("se")
	if got, want := shownLabels(c), []string{"Settings", "Search Emoji"}; !slices.Equal(got, want) {
		t.Errorf("se shows %v, want %v", got, want)
	}
	c.SetQuery("smiley")
	if got := shownLabels(c); !slices.Equal(got, []string{"Search Emoji"}) {
		t.Errorf("a keyword shows %v", got)
	}
	c.SetQuery("zzz")
	if len(c.shown) != 0 || !slices.Contains(c.list.Children(), Widget(c.empty)) {
		t.Error("no match did not show the empty state")
	}
	if _, ok := c.Highlighted(); ok {
		t.Error("an empty list has a highlight")
	}
}

// The navigation keys move the highlight past disabled items while the
// entry keeps the focus, Enter picks, and a click picks without taking
// the focus from the entry.
func TestCommandKeyboardAndClick(t *testing.T) {
	var picked []string
	c := NewCommand(goldenFace(t), "", testCommandGroups(&picked)...)
	arrangeTree(t, c, 400, 400)
	r := &Router{Root: c}
	r.SetFocus(c.FocusTarget())
	r.KeyAction(KeyDown, 0)
	r.KeyAction(KeyDown, 0)
	if it, _ := c.Highlighted(); it.Label != "Profile" {
		t.Fatalf("highlight on %q, want Profile past the disabled Calculator", it.Label)
	}
	r.KeyAction(KeyEnd, 0)
	r.KeyAction(KeyEnter, 0)
	billing := c.shown[4]
	b := billing.Bounds()
	at := Point{X: b.X + 4, Y: b.Y + b.H/2}
	r.Press(BTNLeft, at, 0)
	r.Release(BTNLeft, at)
	if !slices.Equal(picked, []string{"Settings", "Billing"}) {
		t.Errorf("picked %v, want Settings then Billing", picked)
	}
	if r.Focused() != c.FocusTarget() {
		t.Errorf("focus on %T after the click, want the search entry", r.Focused())
	}
}

// TestGoldenCommand pins the command with its groups, a
// shortcut, a disabled item and the highlight, in both palettes.
func TestGoldenCommand(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			var picked []string
			c := NewCommand(face, "Type a command or search...", testCommandGroups(&picked)...)
			c.AddClass(css.Card)
			NewGolden(t, c, "command-"+tc.name, goldenTheme(tc.th), goldenFrame(360, 330))
		})
	}
}

// The rows sit as far from the command's left edge as from its right,
// and the search entry draws no field of its own: the list's scroll
// reserved a bar gutter on the right though nothing overflowed, and
// the dark theme's input fill boxed the search entry (#281).
func TestCommandRowsAreCentered(t *testing.T) {
	prev := Current()
	t.Cleanup(func() { SetTheme(prev) })
	SetTheme(DarkTheme())
	var picked []string
	c := NewCommand(testFace(t), "Search...", testCommandGroups(&picked)...)
	arrangeTree(t, c, 320, 400)
	cb, row := c.Bounds(), c.rows[0].Bounds()
	if left, right := row.X-cb.X, cb.X+cb.W-(row.X+row.W); left != right {
		t.Errorf("row inset %d on the left, %d on the right", left, right)
	}
	if bg := c.input.style(c.input).Background; bg.A() != 0 {
		t.Errorf("search entry background %#x, want none", uint32(bg))
	}
}

// A list long enough to scroll keeps its rows centered too: its scroll
// shows no bars, so it reserves no bar gutter on the right (#317).
func TestCommandListOverflowingRowsAreCentered(t *testing.T) {
	items := make([]CommandItem, 40)
	for i := range items {
		items[i] = CommandItem{Label: fmt.Sprintf("Item %d", i)}
	}
	c := NewCommandList(testFace(t), CommandGroup{Heading: "Many", Items: items})
	arrangeTree(t, c, 320, 400)
	if r := c.scroll.AxisRange(Column); !r.Overflows() {
		t.Fatal("the list does not overflow")
	}
	cb, row := c.Bounds(), c.rows[0].Bounds()
	if left, right := row.X-cb.X, cb.X+cb.W-(row.X+row.W); left != right {
		t.Errorf("row inset %d on the left, %d on the right", left, right)
	}
}

// A command list has no search row and is its own focus target: it
// navigates past disabled items on its own keys, picks on Enter, Space
// or a click, and jumps by type-ahead.
func TestCommandList(t *testing.T) {
	pinAnimClock(t)
	var picked []string
	c := NewCommandList(goldenFace(t), testCommandGroups(&picked)...)
	arrangeTree(t, c, 400, 400)
	if c.input != nil || len(c.column.Children()) != 1 {
		t.Fatal("the list has a search row")
	}
	r := &Router{Root: c}
	r.SetFocus(c.FocusTarget())
	if r.Focused() != Widget(c) {
		t.Fatalf("focus on %T, want the list itself", r.Focused())
	}
	r.KeyAction(KeyDown, 0)
	r.KeyAction(KeyDown, 0)
	if it, _ := c.Highlighted(); it.Label != "Profile" {
		t.Fatalf("highlight on %q, want Profile past the disabled Calculator", it.Label)
	}
	r.KeyAction(KeyHome, 0)
	r.KeyAction(KeyEnter, 0)
	r.KeyAction(KeyEnd, 0)
	r.Type(' ')
	r.Type('b')
	r.KeyAction(KeyEnter, 0)
	clickAt(r, c.shown[0])
	if want := []string{"Calendar", "Settings", "Billing", "Calendar"}; !slices.Equal(picked, want) {
		t.Errorf("picked %v, want %v", picked, want)
	}
	if r.Focused() != Widget(c) {
		t.Errorf("focus on %T after the click, want the list", r.Focused())
	}
	if c.Query() != "" {
		t.Errorf("the list has query %q", c.Query())
	}
	defer func() {
		if recover() == nil {
			t.Error("SetQuery on a list without a search row did not panic")
		}
	}()
	c.SetQuery("x")
}

// A searching command is no focus stop of its own: traversal lands in
// its entry.
func TestCommandSearchKeepsTheFocusInTheEntry(t *testing.T) {
	var picked []string
	c := NewCommand(goldenFace(t), "", testCommandGroups(&picked)...)
	arrangeTree(t, c, 400, 400)
	if isFocusable(c) {
		t.Error("a searching command takes the focus itself")
	}
	r := &Router{Root: c}
	r.FocusNext()
	if r.Focused() != c.FocusTarget() {
		t.Errorf("traversal focused %T, want the search entry", r.Focused())
	}
}

// manyGroups is n headed groups of m items each.
func manyGroups(n, m int) []CommandGroup {
	groups := make([]CommandGroup, n)
	for g := range groups {
		groups[g].Heading = fmt.Sprintf("Group %d", g)
		for i := range m {
			groups[g].Items = append(groups[g].Items, CommandItem{Label: fmt.Sprintf("Item %d.%d", g, i)})
		}
	}
	return groups
}

// A pick list is as tall as its rows wherever its ceiling allows, and
// scrolls only when held shorter; a search palette keeps cmdk's 300px
// list (#322).
func TestCommandListGrowsToItsRows(t *testing.T) {
	face := testFace(t)
	list := NewCommandList(face, manyGroups(6, 6)...)
	tall := SizeWithin(list, Size{W: 300, H: 4000})
	if tall.H <= commandListHeight {
		t.Fatalf("pick list measured %dpx tall under a 4000px ceiling, want its rows' height past %d", tall.H, commandListHeight)
	}
	arrangeTree(t, list, 300, tall.H)
	if list.scroll.AxisRange(Column).Overflows() {
		t.Error("a pick list at its natural height scrolls")
	}
	short := NewCommandList(face, manyGroups(6, 6)...)
	if h := SizeWithin(short, Size{W: 300, H: 200}).H; h != 200 {
		t.Errorf("pick list under a 200px ceiling measured %d, want 200", h)
	}
	arrangeTree(t, short, 300, 200)
	if !short.scroll.AxisRange(Column).Overflows() {
		t.Error("a pick list held to 200px does not scroll")
	}
	var picked []string
	palette := NewCommand(face, "Search...", append(testCommandGroups(&picked), manyGroups(6, 6)...)...)
	if h := SizeWithin(palette.scroll, Size{W: 300, H: 4000}).H; h != commandListHeight {
		t.Errorf("search palette list %dpx tall, want cmdk's %d", h, commandListHeight)
	}
}

// A highlight set before the list is laid out shows once it is, its
// group's heading whole above it when both fit, and no heading opens
// cut at the viewport's top edge; a viewport too short for both still
// shows the row (#322).
func TestCommandListRevealsTheHighlightWithItsHeading(t *testing.T) {
	face := testFace(t)
	for _, tc := range []struct {
		h, row  int
		heading bool
	}{{200, 4 * 6, true}, {200, 4*6 + 3, true}, {200, 4*6 + 4, false}, {40, 4 * 6, false}} {
		list := NewCommandList(face, manyGroups(6, 6)...)
		list.highlightRow(tc.row)
		for range 3 {
			arrangeTree(t, list, 300, tc.h)
			if !LayoutPending(list) {
				break
			}
		}
		view := list.scroll.view
		view.H = list.scroll.viewH
		inside := func(r render.Rect) bool { return r.Y >= view.Y && r.Y+r.H <= view.Y+view.H }
		row := list.shown[list.highlight].Bounds()
		if !inside(row) {
			t.Errorf("%dpx: highlighted row %v outside the viewport %v", tc.h, row, view)
		}
		if heading := list.headings[4].Bounds(); tc.heading && !inside(heading) {
			t.Errorf("%dpx, row %d: heading %v cut by the viewport %v", tc.h, tc.row, heading, view)
		}
		for _, hd := range list.headings {
			if b := hd.Bounds(); b.Y < view.Y && b.Y+b.H > view.Y {
				t.Errorf("%dpx, row %d: heading %v cut at the viewport's top %v", tc.h, tc.row, b, view)
			}
		}
	}
}
