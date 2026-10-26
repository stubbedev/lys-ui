package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// anchorFixture is a window-like tree (.win > .bar > .item) and a
// separate, parentless popover tree whose label tests read.
type anchorFixture struct {
	win, bar, item *Box
	pop            *Box
	label          *Label
}

func newAnchorFixture(t *testing.T) anchorFixture {
	t.Helper()
	face := goldenFace(t)
	f := anchorFixture{
		win:   NewBox(Column, 0, 0),
		bar:   NewBox(Row, 0, 0),
		item:  NewBox(Row, 0, 0),
		pop:   NewBox(Column, 0, 0),
		label: NewLabel(face, 14, "x", 0),
	}
	f.win.AddClass("win")
	f.bar.AddClass("bar")
	f.item.AddClass("item")
	f.win.Append(f.bar, false)
	f.bar.Append(f.item, false)
	f.item.Append(NewLabel(face, 14, "anchor", 0), false)
	f.pop.Append(f.label, false)
	arrangeTree(t, f.win, 200, 40)
	arrangeTree(t, f.pop, 200, 40)
	return f
}

const anchorCSS = `
	:root { --ink: #00ff00; }
	.win { color: #ff0000; }
	.win.night { color: #0000ff; }
	.bar label { background-color: var(--ink); }
`

func TestAnAnchoredTreeStylesAsAChildOfItsAnchor(t *testing.T) {
	loadCSS(t, anchorCSS)
	f := newAnchorFixture(t)
	if got := CascadeColor(f.label); got == render.RGB(0xff, 0, 0) {
		t.Fatalf("unanchored label inherits the window's color %v", got)
	}

	SetStyleAnchor(f.pop, f.item)
	if got := StyleAnchor(f.pop); got != f.item {
		t.Fatalf("StyleAnchor = %v, want the item", got)
	}
	if got := CascadeColor(f.label); got != render.RGB(0xff, 0, 0) {
		t.Errorf("anchored label color = %v, want the window's inherited red", got)
	}
	// .bar label reaches through the anchor's ancestry, and var(--ink)
	// resolves from the window's :root, not the popover's own root.
	if got := StyleBackground(f.label); got != render.RGB(0, 0xff, 0) {
		t.Errorf("anchored label background = %v, want .bar label's var(--ink) green", got)
	}

	// A change up the anchor's chain restyles the anchored tree.
	f.win.AddClass("night")
	CollectDamage(f.win)
	if got := CascadeColor(f.label); got != render.RGB(0, 0, 0xff) {
		t.Errorf("after the window's change the label is %v, want blue", got)
	}

	SetStyleAnchor(f.pop, nil)
	if StyleAnchor(f.pop) != nil {
		t.Error("unanchoring kept the anchor")
	}
	if got := CascadeColor(f.label); got == render.RGB(0, 0, 0xff) {
		t.Errorf("unanchored label still inherits %v", got)
	}
	if got := StyleBackground(f.label); got == render.RGB(0, 0xff, 0) {
		t.Error("unanchored label still matches .bar label")
	}
}

// Re-anchoring moves the tree: the old anchor no longer drives it.
func TestReanchoringLeavesTheOldAnchor(t *testing.T) {
	loadCSS(t, anchorCSS)
	f := newAnchorFixture(t)
	other := NewBox(Row, 0, 0)
	arrangeTree(t, other, 10, 10)
	SetStyleAnchor(f.pop, f.item)
	SetStyleAnchor(f.pop, other)
	if n := nodeOf(f.item); len(n.anchored) != 0 {
		t.Errorf("the old anchor still lists %d anchored trees", len(n.anchored))
	}
	if got := CascadeColor(f.label); got == render.RGB(0xff, 0, 0) {
		t.Error("the label still inherits from the old anchor's window")
	}
}
