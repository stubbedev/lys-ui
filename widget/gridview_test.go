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
