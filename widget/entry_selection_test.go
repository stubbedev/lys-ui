package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestEntrySelection(t *testing.T) {
	newEntry := func() *Entry {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("hello world")
		e.MoveHome()
		return e
	}

	t.Run("shift+arrows grow the selection from the anchor", func(t *testing.T) {
		e := newEntry()
		e.KeyAction(KeyRight, ModShift)
		e.KeyAction(KeyRight, ModShift)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 2 {
			t.Errorf("selection = %d..%d active=%v, want 0..2 true", start, end, active)
		}
	})

	t.Run("reversing shift+arrows shrinks back to the anchor", func(t *testing.T) {
		e := newEntry()
		e.KeyAction(KeyRight, ModShift)
		e.KeyAction(KeyRight, ModShift)
		e.KeyAction(KeyLeft, ModShift)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 1 {
			t.Errorf("selection = %d..%d active=%v, want 0..1 true", start, end, active)
		}
	})

	t.Run("plain arrows collapse to the direction edge", func(t *testing.T) {
		e := newEntry()
		e.MoveCursorExtending(3)
		e.KeyAction(KeyLeft, 0)
		if e.Cursor() != 0 {
			t.Errorf("cursor = %d, want 0 (left edge)", e.Cursor())
		}
		_, _, active := e.Selection()
		if active {
			t.Error("plain arrow must clear the selection")
		}
		e.MoveCursorExtending(4)
		e.KeyAction(KeyRight, 0)
		if e.Cursor() != 4 {
			t.Errorf("cursor = %d, want 4 (right edge)", e.Cursor())
		}
	})

	t.Run("first arrow press after selection only collapses", func(t *testing.T) {
		e := newEntry()
		e.MoveCursorExtending(3)
		e.KeyAction(KeyRight, 0)
		if e.Cursor() != 3 {
			t.Errorf("cursor = %d, want 3 (collapse without moving)", e.Cursor())
		}
		e.KeyAction(KeyRight, 0)
		if e.Cursor() != 4 {
			t.Errorf("cursor = %d, want 4 (second press moves)", e.Cursor())
		}
	})

	t.Run("typing replaces the selection", func(t *testing.T) {
		e := newEntry()
		e.MoveCursorExtending(5)
		e.InsertRune('H')
		if got := e.Text(); got != "H world" {
			t.Errorf("text = %q, want 'H world'", got)
		}
		if e.Cursor() != 1 {
			t.Errorf("cursor = %d, want 1", e.Cursor())
		}
	})

	t.Run("backspace deletes the selection", func(t *testing.T) {
		e := newEntry()
		e.MoveCursorExtending(5)
		e.Backspace()
		if got := e.Text(); got != " world" {
			t.Errorf("text = %q, want ' world'", got)
		}
	})

	t.Run("drag from the click point selects", func(t *testing.T) {
		e := newEntry()
		sh := e.face.Shape("hello world", 14)
		e.ClickAt(Point{X: entryPad.Left + int(sh.CaretX(1)), Y: 15})
		e.DragMove(Point{X: entryPad.Left + int(sh.CaretX(4)), Y: 15})
		start, end, active := e.Selection()
		if !active || start != 1 || end != 4 {
			t.Errorf("selection = %d..%d active=%v, want 1..4 true", start, end, active)
		}
	})

	t.Run("select all covers the text and typing replaces it", func(t *testing.T) {
		e := newEntry()
		e.SelectAll()
		start, end, active := e.Selection()
		if !active || start != 0 || end != 11 {
			t.Errorf("selection = %d..%d active=%v, want 0..11 true", start, end, active)
		}
		e.InsertRune('x')
		if got := e.Text(); got != "x" {
			t.Errorf("text = %q, want x", got)
		}
	})

	t.Run("double-click selects the word under the pointer", func(t *testing.T) {
		e := newEntry()
		sh := e.face.Shape("hello world", 14)
		e.DoubleClickAt(Point{X: entryPad.Left + int(sh.CaretX(7)), Y: 15})
		start, end, active := e.Selection()
		if !active || start != 6 || end != 11 {
			t.Errorf("selection = %d..%d active=%v, want 6..11 true", start, end, active)
		}
	})

	t.Run("double-click between words selects the whitespace run", func(t *testing.T) {
		e := newEntry()
		sh := e.face.Shape("hello world", 14)
		mid := (sh.CaretX(5) + sh.CaretX(6)) / 2
		e.DoubleClickAt(Point{X: entryPad.Left + int(mid), Y: 15})
		start, end, active := e.Selection()
		if !active || start != 5 || end != 6 {
			t.Errorf("selection = %d..%d active=%v, want 5..6 true", start, end, active)
		}
	})

	t.Run("a plain click collapses the selection", func(t *testing.T) {
		e := newEntry()
		e.MoveCursorExtending(3)
		e.ClickAt(Point{X: entryPad.Left + int(e.face.Shape("hello world", 14).CaretX(8)), Y: 15})
		_, _, active := e.Selection()
		if active || e.Cursor() != 8 {
			t.Errorf("click must collapse: cursor %d active %v", e.Cursor(), active)
		}
	})

	t.Run("selection paints a highlight", func(t *testing.T) {
		e := newEntry()
		SizeWithin(e, Size{W: 200, H: 100})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
		e.MoveCursorExtending(4)
		data := make([]byte, render.Stride(200)*30)
		cv := render.New(data, render.Stride(200), 200, 30)
		e.Paint(cv)
		hits := 0
		for y := range 30 {
			if render.ColorFromBytes(data[y*render.Stride(200)+12*4:]).A() > 0 {
				hits++
			}
		}
		if hits < 5 {
			t.Errorf("selection highlight rows = %d, want a visible band", hits)
		}
	})
}
