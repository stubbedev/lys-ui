package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// gridList is a 10-item grid of 50px cells in a 160px viewport: three
// columns (160/50), four lines, the last holding one item.
func gridList(t *testing.T) (*List, *countingModel) {
	t.Helper()
	model := newCountingModel(10)
	l := NewList(model, 20)
	l.SetCellWidth(50)
	l.Measure(Constraints{Max: Size{W: 160, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: 40})
	return l, model
}

func TestListGridLaysCellsOutInLines(t *testing.T) {
	l, _ := gridList(t)
	if got := l.Measure(Constraints{Max: Size{W: 160, H: 1 << 20}}); got.H != 80 {
		t.Errorf("grid content height = %d, want 4 lines of 20 = 80", got.H)
	}
	// The width's remainder goes to the last column: 53, 53, 54.
	for i, want := range []render.Rect{
		{X: 0, Y: 0, W: 53, H: 20},
		{X: 53, Y: 0, W: 53, H: 20},
		{X: 106, Y: 0, W: 54, H: 20},
		{X: 0, Y: 20, W: 53, H: 20},
	} {
		if got := l.cellRect(i); got != want {
			t.Errorf("cell %d = %+v, want %+v", i, got, want)
		}
	}
	if got := l.rowAt(Point{X: 120, Y: 25}); got != 5 {
		t.Errorf("point in line 1, column 2 hit %d, want 5", got)
	}
	// The trailing cells of the short last line hit nothing.
	l.ScrollBy(0, 1)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: 40})
	if i, ok := l.rowAtExact(Point{X: 10, Y: 39}); !ok || i != 9 {
		t.Errorf("last line's first cell = %d, %v; want 9, true", i, ok)
	}
	if i, ok := l.rowAtExact(Point{X: 100, Y: 39}); ok {
		t.Errorf("empty cell after the last item hit %d", i)
	}
}

func TestListGridVirtualizesByLine(t *testing.T) {
	model := newCountingModel(3000)
	l := NewList(model, 20)
	l.SetCellWidth(50)
	l.Measure(Constraints{Max: Size{W: 160, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: 40})
	l.Paint(render.New(make([]uint8, 160*40*4), 160*4, 160, 40))
	// Two visible lines plus one partial, three cells each.
	if got := model.distinct(); got != 9 {
		t.Errorf("painting 2 lines requested %d items, want 9", got)
	}
}

func TestListGridKeysMoveByLineAndItem(t *testing.T) {
	l, _ := gridList(t)
	l.Select(4)
	l.KeyAction(KeyUp, 0)
	if got := l.Selected(); got != 1 {
		t.Errorf("up from 4 = %d, want 1 (one line of 3)", got)
	}
	l.KeyAction(KeyRight, 0)
	if got := l.Selected(); got != 2 {
		t.Errorf("right from 1 = %d, want 2", got)
	}
	l.KeyAction(KeyDown, 0)
	l.KeyAction(KeyDown, 0)
	l.KeyAction(KeyDown, 0)
	if got := l.Selected(); got != 9 {
		t.Errorf("down past the short last line = %d, want clamped 9", got)
	}
	l.KeyAction(KeyLeft, 0)
	if got := l.Selected(); got != 8 {
		t.Errorf("left from 9 = %d, want 8", got)
	}
}

// A plain list ignores left and right; a grid back to width zero is a
// list again.
func TestListLeftRightOnlyMoveInAGrid(t *testing.T) {
	model := newCountingModel(5)
	l := NewList(model, 20)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: 100})
	l.Select(2)
	l.KeyAction(KeyRight, 0)
	l.KeyAction(KeyLeft, 0)
	if got := l.Selected(); got != 2 {
		t.Errorf("left/right moved a list's selection to %d", got)
	}
	l.SetCellWidth(50)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: 100})
	l.SetCellWidth(0)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: 100})
	if got := l.cellRect(1); got.X != 0 || got.W != 160 || got.Y != 20 {
		t.Errorf("row 1 after leaving grid mode = %+v, want a full-width row", got)
	}

	ml := NewList(newCountingModel(5), 20)
	ml.SetSelectionMode(SelectionMultiple)
	ml.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: 100})
	ml.Select(2)
	ml.KeyAction(KeyRight, 0)
	if got := ml.Selection(); len(got) != 1 || got[0] != 2 {
		t.Errorf("right moved a multiple list's selection to %v", got)
	}
}
