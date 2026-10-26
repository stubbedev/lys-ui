package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/widget"
)

// fakeMenuOpener records each opened level and keeps the real chain
// bookkeeping (settle) without a compositor.
type fakeMenuOpener struct {
	opened []fakeLevel
}

type fakeLevel struct {
	parent *Popover
	anchor render.Rect
	pop    *Popover
}

func (f *fakeMenuOpener) open(parent *Popover, anchor widget.Boundser, content widget.Widget, onClosed func()) (*Popover, error) {
	p := &Popover{parent: parent}
	done := func() {
		if p.settle() {
			onClosed()
		}
	}
	p.closeFn, p.teardown = done, done
	if parent != nil {
		parent.children = append(parent.children, p)
	}
	content.Measure(widget.Constraints{Max: widget.Size{W: 200, H: 400}})
	content.Arrange(render.Rect{W: 200, H: 400})
	f.opened = append(f.opened, fakeLevel{parent: parent, anchor: anchor.Bounds(), pop: p})
	return p, nil
}

// rowPoint is the middle of a menu row.
func rowPoint(m *widget.Menu, row int) widget.Point {
	r := m.RowBounds(row)
	return widget.Point{X: r.X + r.W/2, Y: r.Y + r.H/2}
}

func TestMenuPopoverNestsSubmenus(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	closed := 0
	items := []widget.MenuItem{
		{Label: "Open", OnClick: func() { log = append(log, "open") }},
		{Label: "Recent", OnOpen: func() { log = append(log, "recent opened") }, Items: []widget.MenuItem{
			{Label: "a.txt", OnClick: func() { log = append(log, "a") }},
			{Label: "More", Items: []widget.MenuItem{{Label: "deep", OnClick: func() { log = append(log, "deep") }}}},
		}},
		{Label: "Locked", Disabled: true, Items: []widget.MenuItem{{Label: "x"}}},
	}
	f := &fakeMenuOpener{}
	m, err := openMenuPopover(MenuPopoverConfig{
		Anchor: rowAnchor{render.Rect{W: 10, H: 10}}, Face: face, SizePx: 13, Items: items,
		OnClosed: func() { closed++ },
	}, f.open)
	if err != nil || m.Depth() != 1 || len(f.opened) != 1 || f.opened[0].parent != nil {
		t.Fatalf("root: %v depth %d", err, m.Depth())
	}
	root := m.Level(0)

	// Hovering the submenu row opens it beside the row, nested.
	root.HoverMove(rowPoint(root, 1))
	if m.Depth() != 2 || len(f.opened) != 2 || f.opened[1].parent != f.opened[0].pop || f.opened[1].anchor != root.RowBounds(1) {
		t.Fatalf("hover: depth %d, opened %d", m.Depth(), len(f.opened))
	}
	// Hovering it again reopens nothing; a sibling closes it.
	root.HoverMove(rowPoint(root, 1))
	if len(f.opened) != 2 || len(log) != 1 || log[0] != "recent opened" {
		t.Errorf("re-hovering the open row opened it again, or OnOpen ran %v", log)
	}
	log = nil
	root.HoverMove(rowPoint(root, 0))
	if m.Depth() != 1 || f.opened[1].pop.Closed() != true {
		t.Errorf("sibling hover: depth %d", m.Depth())
	}
	// A disabled submenu row stays shut.
	root.HoverMove(rowPoint(root, 2))
	if m.Depth() != 1 {
		t.Error("a disabled row opened its submenu")
	}

	// A click opens it too, and the next level nests under it.
	root.ClickAt(rowPoint(root, 1))
	sub := m.Level(1)
	sub.HoverMove(rowPoint(sub, 1))
	if m.Depth() != 3 || f.opened[len(f.opened)-1].parent != f.opened[len(f.opened)-2].pop {
		t.Fatalf("third level: depth %d", m.Depth())
	}
	// Hovering a leaf at level 1 closes level 2 only.
	sub.HoverMove(rowPoint(sub, 0))
	if m.Depth() != 2 {
		t.Errorf("leaf hover at level 1: depth %d, want 2", m.Depth())
	}
	// Esc (the level's own dismiss) closes the innermost level only.
	m.levels[1].pop.Dismiss()
	if m.Depth() != 1 || m.Closed() {
		t.Errorf("Esc in the submenu: depth %d closed %v", m.Depth(), m.Closed())
	}

	// A leaf deep down closes the whole chain, then acts, once.
	log = nil
	root.ClickAt(rowPoint(root, 1))
	sub = m.Level(1)
	sub.ClickAt(rowPoint(sub, 1))
	deep := m.Level(2)
	deep.ClickAt(rowPoint(deep, 0))
	if !m.Closed() || closed != 1 || len(log) != 2 || log[0] != "recent opened" || log[1] != "deep" {
		t.Errorf("leaf: closed %v (%d), log %v", m.Closed(), closed, log)
	}
	for _, lvl := range f.opened {
		if !lvl.pop.Closed() {
			t.Error("a level outlived the chain")
		}
	}
}

func TestMenuPopoverSetItems(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeMenuOpener{}
	m, _ := openMenuPopover(MenuPopoverConfig{
		Anchor: rowAnchor{}, Face: face, SizePx: 13,
		Items: []widget.MenuItem{{Label: "Sub", Items: []widget.MenuItem{{Label: "x"}}}},
	}, f.open)
	m.Level(0).ClickAt(rowPoint(m.Level(0), 0))
	if m.Depth() != 2 {
		t.Fatal("submenu did not open")
	}
	// The open submenu's row still leads to one: it stays open and
	// takes the new rows, the same menu updated in place.
	sub := m.Level(1)
	m.SetItems(widget.MenuItem{Label: "Sub", Items: []widget.MenuItem{{Label: "x"}, {Label: "y"}}})
	if m.Depth() != 2 || m.Level(1) != sub || len(sub.Items()) != 2 {
		t.Fatalf("live update: depth %d, same level %v, %d rows", m.Depth(), m.Level(1) == sub, len(sub.Items()))
	}
	// The path broke: the submenu closes, the root updates.
	m.SetItems(widget.MenuItem{Label: "One"}, widget.MenuItem{Label: "Two"})
	if m.Depth() != 1 || len(m.Level(0).Items()) != 2 || m.Closed() {
		t.Errorf("after SetItems: depth %d, %d rows", m.Depth(), len(m.Level(0).Items()))
	}
	if _, err := openMenuPopover(MenuPopoverConfig{Face: face}, f.open); err == nil {
		t.Error("no anchor: want an error")
	}
}

// TestMenuPopoverRootTakesKeys pins the key path into the root menu:
// the popover hands key actions to its content, the frame, which
// passes them to the menu, so Down then Right opens a submenu.
func TestMenuPopoverRootTakesKeys(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeMenuOpener{}
	m, _ := openMenuPopover(MenuPopoverConfig{
		Anchor: rowAnchor{}, Face: face, SizePx: 13,
		Items: []widget.MenuItem{{Label: "Sub", Items: []widget.MenuItem{{Label: "x"}}}},
	}, f.open)
	keys, ok := widget.Widget(m.frame).(widget.KeyActionHandler)
	if !ok {
		t.Fatal("the root content takes no key actions")
	}
	keys.KeyAction(widget.KeyDown, 0)
	keys.KeyAction(widget.KeyRight, 0)
	if m.Depth() != 2 {
		t.Errorf("Down, Right: depth %d, want the submenu open", m.Depth())
	}
}

// Submenus open after their row in its reading direction unless asked
// otherwise; accelerators display through AccelLabel.
func TestSubmenuSideAndAccelLabel(t *testing.T) {
	cases := []struct {
		side SubmenuSide
		dir  widget.Direction
		want Gravity
	}{
		{SubmenuAfter, widget.DirectionAuto, GravityRight},
		{SubmenuAfter, widget.DirectionRTL, GravityLeft},
		{SubmenuBefore, widget.DirectionAuto, GravityLeft},
		{SubmenuBefore, widget.DirectionRTL, GravityRight},
	}
	for _, c := range cases {
		if got := c.side.gravity(c.dir); got != c.want {
			t.Errorf("side %d dir %d = %d, want %d", c.side, c.dir, got, c.want)
		}
	}
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeMenuOpener{}
	m, _ := openMenuPopover(MenuPopoverConfig{
		Anchor: rowAnchor{}, Face: face, SizePx: 13,
		Items:      []widget.MenuItem{{Label: "Save", Accel: "Ctrl+S", OnClick: func() {}}},
		AccelLabel: func(a string) string { return "<" + a + ">" },
	}, f.open)
	if lbl := m.Level(0).AccelLabel; lbl == nil || lbl("Ctrl+S") != "<Ctrl+S>" {
		t.Error("AccelLabel did not reach the menu")
	}
}
