package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestGridViewFitsColumnsWithinItsBounds(t *testing.T) {
	g := NewGridView[Widget](newCountingModel(100), 50, 40)
	for _, c := range []struct{ width, min, max, want int }{
		{500, 1, 7, 7},
		{260, 1, 7, 5},
		{30, 1, 7, 1},
		{30, 3, 7, 3},
		{1000, 1, 0, 20},
	} {
		g.SetMinColumns(c.min)
		g.SetMaxColumns(c.max)
		g.Measure(Constraints{Max: Size{W: c.width, H: 400}})
		g.Arrange(render.Rect{W: c.width, H: 400})
		if got := g.Columns(); got != c.want {
			t.Errorf("width %d, columns [%d,%d]: %d columns, want %d", c.width, c.min, c.max, got, c.want)
		}
	}
}

func TestGridViewStaysVirtualizedAndNavigatesInTwoDimensions(t *testing.T) {
	model := newCountingModel(10000)
	g := NewGridView[Widget](model, 50, 40)
	g.Measure(Constraints{Max: Size{W: 250, H: 200}})
	g.Arrange(render.Rect{W: 250, H: 200})
	g.Paint(render.New(make([]uint8, 250*200*4), 250*4, 250, 200))
	if g.Columns() != 5 {
		t.Fatalf("columns = %d, want 5", g.Columns())
	}
	if n := model.distinct(); n == 0 || n > 5*7 {
		t.Errorf("built %d cells for a 5x5 viewport over 10000 items", n)
	}
	g.Select(7)
	g.KeyAction(KeyDown, 0)
	if g.Selected() != 12 {
		t.Errorf("down from 7 selected %d, want 12", g.Selected())
	}
	g.KeyAction(KeyLeft, 0)
	if g.Selected() != 11 {
		t.Errorf("left from 12 selected %d, want 11", g.Selected())
	}
}

func TestGridViewStylesAsGridviewWithChildCells(t *testing.T) {
	g := NewGridView[Widget](newCountingModel(3), 50, 40)
	g.Measure(Constraints{Max: Size{W: 200, H: 100}})
	g.Arrange(render.Rect{W: 200, H: 100})
	g.Paint(render.New(make([]uint8, 200*100*4), 200*4, 200, 100))
	if len(g.rows) == 0 {
		t.Fatal("no cells were built")
	}
	if g.Element() != "gridview" {
		t.Errorf("element %q, want gridview", g.Element())
	}
	for _, r := range g.rows {
		if r.Element() != "child" {
			t.Errorf("cell element %q, want child", r.Element())
		}
	}
}

// Spacing opens gaps between columns and lines that count toward the
// fit and the content height, and hit no cell (#372).
func TestGridViewSpacing(t *testing.T) {
	g := NewGridView[Widget](newCountingModel(10), 50, 20)
	g.SetColumnSpacing(5)
	g.SetRowSpacing(4)
	if got := g.Measure(Constraints{Max: Size{W: 160, H: 1 << 20}}); got.H != 4*20+3*4 {
		t.Errorf("content height %d, want 4 lines of 20 and 3 gaps of 4", got.H)
	}
	frame(t, g, 160, 100)
	if g.Columns() != 3 {
		t.Fatalf("columns = %d, want 3: three 50px cells and two 5px gaps fit 160", g.Columns())
	}
	for i, want := range []render.Rect{
		{X: 0, Y: 0, W: 50, H: 20},
		{X: 55, Y: 0, W: 50, H: 20},
		{X: 110, Y: 0, W: 50, H: 20},
		{X: 0, Y: 24, W: 50, H: 20},
	} {
		if got := g.cellRect(i); got != want {
			t.Errorf("cell %d = %+v, want %+v", i, got, want)
		}
	}
	for _, p := range []Point{{X: 52, Y: 10}, {X: 10, Y: 22}} {
		if got := g.HitTest(p); got != Widget(&g.List) {
			t.Errorf("gap point %v hit %T, want the grid", p, got)
		}
	}
	if got, ok := g.HitTest(Point{X: 60, Y: 30}).(*listRow); !ok || got.idx != 4 {
		t.Errorf("a point in cell 4 hit %v, want its row", got)
	}
}

// The stylesheet's border-spacing adds to the programmatic spacing.
func TestGridViewBorderSpacing(t *testing.T) {
	loadCSS(t, `gridview { border-spacing: 6px 2px; }`)
	g := NewGridView[Widget](newCountingModel(10), 50, 20)
	g.SetRowSpacing(1)
	frame(t, g, 200, 100)
	// Three columns of (200-2*6)/3 = 62px fit; cell 4 opens line 1's
	// second column.
	if got := g.cellRect(4); got.X != 62+6 || got.Y != 20+3 {
		t.Errorf("cell 4 at (%d, %d), want (68, 23) with 6px columns and 3px lines", got.X, got.Y)
	}
}

// A selection made before the first layout reveals once the grid has a
// viewport: an item already in view leaves the grid at the top, and one
// further down scrolls just far enough (#373).
func TestGridViewRevealsAnEarlySelectionAfterLayout(t *testing.T) {
	g := NewGridView[Widget](newCountingModel(100), 50, 20)
	g.Select(1)
	frame(t, g, 160, 60)
	if g.offY != 0 {
		t.Errorf("selecting an item of the first line scrolled to %d, want 0", g.offY)
	}

	g = NewGridView[Widget](newCountingModel(100), 50, 20)
	g.Select(31)
	frame(t, g, 160, 60)
	// Item 31 is line 10 of three columns: its bottom, 220, meets the
	// viewport's.
	if g.offY != 220-60 {
		t.Errorf("selecting item 31 scrolled to %d, want 160", g.offY)
	}
}
