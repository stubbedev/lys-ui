package widget

import (
	"slices"
	"testing"

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
