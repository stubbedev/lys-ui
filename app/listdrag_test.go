package app

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// fileRows is a list model of fixed-height plain rows.
type fileRows []*widget.Box

func (m fileRows) Len() int              { return len(m) }
func (m fileRows) Row(i int) *widget.Box { return m[i] }
func newFileRows(n int) fileRows {
	rows := make(fileRows, n)
	for i := range rows {
		rows[i] = widget.NewBox(widget.Row, 0, 0)
	}
	return rows
}

// newDragList is a four-row multiple-selection list, 20px rows in a
// 100px-wide viewport, offering a uri-list of the dragged rows.
func newDragList(t *testing.T) (*widget.List, *[][]int) {
	t.Helper()
	l := widget.NewList(newFileRows(4), 20)
	l.SetSelectionMode(widget.SelectionMultiple)
	var dragged [][]int
	l.DragContent = func(rows []int) *widget.DragContent {
		dragged = append(dragged, rows)
		return &widget.DragContent{
			Mimes: []string{"text/uri-list"},
			Write: func(string, io.Writer) error { return nil },
		}
	}
	widget.SizeWithin(l, widget.Size{W: 100, H: 80})
	l.Arrange(render.Rect{W: 100, H: 80})
	l.Paint(render.New(make([]byte, render.Stride(100)*80), render.Stride(100), 100, 80))
	return l, &dragged
}

// A press on a selected row that moves past the threshold drags the
// whole selection out, leaves the selection alone, and does not click.
func TestListDragsTheSelectionOut(t *testing.T) {
	l, dragged := newDragList(t)
	l.SelectAll()
	dnd := &fakeDnD{}
	in := newDragTestInput(l, dnd)
	in.HandlePointerMotion(10, 30)
	in.HandlePointerButton(widget.BTNLeft, 1, 7)
	in.HandlePointerMotion(10, 70)
	if len(dnd.starts) != 1 || len(*dragged) != 1 || !slices.Equal((*dragged)[0], []int{0, 1, 2, 3}) {
		t.Fatalf("%d drags carrying %v, want one of the whole selection", len(dnd.starts), *dragged)
	}
	if got := strings.Join(dnd.starts[0].Content.Mimes, ","); got != "text/uri-list" {
		t.Errorf("offered %s", got)
	}
	in.HandlePointerButton(widget.BTNLeft, 0, 8)
	if !slices.Equal(l.Selection(), []int{0, 1, 2, 3}) {
		t.Errorf("selection %v after the drag, want it untouched", l.Selection())
	}
}

// A press on an unselected row rubber-bands instead, and a list
// without DragContent never drags.
func TestListUnselectedPressRubberBands(t *testing.T) {
	l, dragged := newDragList(t)
	l.Select(0)
	dnd := &fakeDnD{}
	in := newDragTestInput(l, dnd)
	in.HandlePointerMotion(10, 30)
	in.HandlePointerButton(widget.BTNLeft, 1, 7)
	in.HandlePointerMotion(10, 70)
	in.HandlePointerButton(widget.BTNLeft, 0, 8)
	if len(dnd.starts) != 0 || len(*dragged) != 0 {
		t.Fatalf("a press on an unselected row dragged %v", *dragged)
	}
	if !slices.Equal(l.Selection(), []int{1, 2, 3}) {
		t.Errorf("rubber band selected %v, want rows 1 to 3", l.Selection())
	}

	plain, _ := newDragList(t)
	plain.DragContent = nil
	plain.SelectAll()
	dnd = &fakeDnD{}
	in = newDragTestInput(plain, dnd)
	in.HandlePointerMotion(10, 30)
	in.HandlePointerButton(widget.BTNLeft, 1, 7)
	in.HandlePointerMotion(10, 70)
	if len(dnd.starts) != 0 {
		t.Error("a list without DragContent started a drag")
	}
}
