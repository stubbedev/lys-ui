package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// stripNotebook is a 250px notebook over n tabs named a, b, c, ...
func stripNotebook(t *testing.T, n int) *Notebook {
	nb := NewNotebook(chromeFace(t))
	for i := range n {
		nb.AppendTab(string(rune('a'+i)), NewSpacer(10, 10))
	}
	nb.Measure(Constraints{Max: Size{W: 250, H: 100}})
	nb.Arrange(render.Rect{W: 250, H: 100})
	return nb
}

// TestNotebookReorder pins the drag: a reorderable tab moves into the
// slot under the pointer as it crosses, the selection following its
// page; a fixed strip ignores the drag.
func TestNotebookReorder(t *testing.T) {
	nb := stripNotebook(t, 2)
	nb.Reorderable = true
	var moves []string
	nb.OnReorder = func(name string, i int) { moves = append(moves, name+string(rune('0'+i))) }
	a := nb.tabRect(0)
	b := nb.tabRect(1)
	nb.PressAt(Point{X: a.X + 10, Y: a.Y + 5})
	nb.DragMove(Point{X: b.X + 10, Y: b.Y + 5})
	nb.PressEnd()
	if nb.tabs[0].name != "b" || nb.tabs[1].name != "a" || nb.SelectedTab() != "a" || nb.selected != 1 {
		t.Errorf("after the drag: %s %s, selected %q at %d", nb.tabs[0].name, nb.tabs[1].name, nb.SelectedTab(), nb.selected)
	}
	if len(moves) != 1 || moves[0] != "a1" {
		t.Errorf("OnReorder = %v", moves)
	}
	nb.Reorderable = false
	nb.PressAt(Point{X: a.X + 10, Y: a.Y + 5})
	nb.DragMove(Point{X: b.X + 10, Y: b.Y + 5})
	if nb.tabs[0].name != "b" {
		t.Error("a fixed strip reordered")
	}
}

// TestNotebookOverflow pins the scrolling strip: arrows appear only on
// overflow, step a tab, selecting reveals, the wheel scrolls over the
// strip and passes on elsewhere, and ctrl+PageDown cycles.
func TestNotebookOverflow(t *testing.T) {
	bareTheme(t)
	if r := stripNotebook(t, 2).arrowRects(); !r[0].Empty() {
		t.Error("arrows on a strip that fits")
	}
	nb := stripNotebook(t, 5) // 480px of tabs in 250
	arrows := nb.arrowRects()
	if arrows[0].Empty() || nb.stripRange().Max() != 480-210 {
		t.Fatalf("overflow: arrows=%v max=%d", arrows, nb.stripRange().Max())
	}
	nb.ClickAt(Point{X: arrows[1].X + 5, Y: 5})
	if nb.scroll != tabWidth {
		t.Errorf("right arrow scrolled to %d", nb.scroll)
	}
	nb.SelectTab("a")
	if nb.scroll != 0 {
		t.Errorf("selecting the first tab left the strip at %d", nb.scroll)
	}
	nb.KeyAction(KeyPriorPage, ModCtrl)
	if nb.SelectedTab() != "e" || nb.tabRect(4).X+tabWidth > nb.tabViewport().X+nb.tabViewport().W {
		t.Errorf("ctrl+PageUp wrapped to %q at x=%d, want e revealed", nb.SelectedTab(), nb.tabRect(4).X)
	}
	nb.scrollStrip(0)
	nb.HoverMove(Point{X: 100, Y: 5})
	if !nb.claimScroll(ScrollEvent{DY: 1}) || nb.scroll != tabWidth/3 {
		t.Errorf("wheel over the strip: consumed? scroll=%d", nb.scroll)
	}
	nb.SetHovered(false)
	if nb.claimScroll(ScrollEvent{DY: 1}) {
		t.Error("the wheel off the strip was consumed")
	}
}

// TestGoldenNotebookStrip pins an overflowing, closable strip.
func TestGoldenNotebookStrip(t *testing.T) {
	th := DarkTheme()
	nb := NewNotebook(goldenFace(t))
	nb.Closable = true
	for _, s := range []string{"One", "Two", "Three", "Four"} {
		nb.AppendTab(s, NewSpacer(10, 10))
	}
	nb.SelectTab("Two")
	NewGolden(t, nb, "notebook-strip", goldenTheme(th), goldenFrame(260, 60))
}

// Tabs whose labels expand, or that are set to expand, share the
// strip's leftover width; the others keep their natural width (#261).
func TestNotebookTabExpand(t *testing.T) {
	face := chromeFace(t)
	build := func(expand ...bool) *Notebook {
		nb := NewNotebook(face)
		for i, e := range expand {
			name := string(rune('a' + i))
			nb.AppendTab(name, NewSpacer(10, 10))
			l := NewLabel(face, 13, name, 0)
			l.SetHExpand(e)
			nb.SetTabLabel(name, l)
		}
		frame(t, nb, 600, 100)
		return nb
	}
	nb := build(true, true, true)
	row := nb.strip.row
	w := nb.tabRect(0).W
	if last := nb.tabRect(2); last.X+last.W != row.X+row.W || nb.tabRect(1).W != w || last.W != w {
		t.Errorf("expanding tabs %v %v %v in row %v, want equal widths ending at the row's end", nb.tabRect(0), nb.tabRect(1), last, row)
	}
	plain := build(false, false, false)
	if last := plain.tabRect(2); last.X+last.W >= plain.strip.header.X+plain.strip.header.W {
		t.Errorf("non-expanding tabs reach %d, want natural widths leaving the header's rest", last.X+last.W)
	}
	mixed := build(false, true, false)
	if mixed.tabRect(0).W != plain.tabRect(0).W || mixed.tabRect(2).W != plain.tabRect(2).W || mixed.tabRect(1).W <= plain.tabRect(1).W {
		t.Errorf("mixed tabs %v %v %v, want the leftover in the middle one only", mixed.tabRect(0), mixed.tabRect(1), mixed.tabRect(2))
	}
	plain.SetTabExpand("a", true)
	frame(t, plain, 600, 100)
	if last := plain.tabRect(2); last.X+last.W != plain.strip.row.X+plain.strip.row.W {
		t.Errorf("SetTabExpand left the row ending at %d", last.X+last.W)
	}
}

// A notebook is at least as wide as its tab strip: its natural width
// was the widest page's, so a notebook of short pages, aligned to its
// natural size, measured narrower than its own tabs (#281).
func TestNotebookNaturalWidthHoldsItsTabs(t *testing.T) {
	face := goldenFace(t)
	n := NewNotebook(face)
	n.AppendTab("Account", NewLabel(face, 14, "x", 0))
	n.AppendTab("Password", NewLabel(face, 14, "y", 0))
	got := n.Measure(Constraints{Max: Size{W: 1000, H: 400}})
	if strip := n.layoutStrip(render.Rect{W: 1000}); got.W < strip.total {
		t.Errorf("natural width %d, narrower than its tabs' %d", got.W, strip.total)
	}
}
