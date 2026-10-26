package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestTextAreaWordEditing(t *testing.T) {
	t.Run("ctrl+right lands on word ends and stops at the line end", func(t *testing.T) {
		ta := newTextArea(t, "alpha beta")
		ta.SetCursor(0, 0)
		for _, want := range []int{5, 10, 10} {
			ta.KeyAction(KeyRight, ModCtrl)
			if _, col := ta.CursorPos(); col != want {
				t.Fatalf("ctrl+right col = %d, want %d", col, want)
			}
		}
	})

	t.Run("ctrl+left lands on word starts and no-ops at the line start", func(t *testing.T) {
		ta := newTextArea(t, "alpha beta\ngamma delta")
		ta.SetCursor(1, 3) // "gam|ma"
		for _, want := range []int{0, 0} {
			ta.KeyAction(KeyLeft, ModCtrl)
			line, col := ta.CursorPos()
			if line != 1 || col != want {
				t.Fatalf("ctrl+left cursor = %d:%d, want 1:%d", line, col, want)
			}
		}
	})

	t.Run("ctrl+shift+arrow extends the selection word-wise", func(t *testing.T) {
		ta := newTextArea(t, "alpha beta")
		ta.SetCursor(0, 0)
		ta.KeyAction(KeyRight, ModCtrl|ModShift)
		start, end, active := ta.Selection()
		if !active || start != (pos{0, 0}) || end != (pos{0, 5}) {
			t.Fatalf("selection = %v..%v active=%v, want 0:0..0:5 true", start, end, active)
		}
		if got, _ := ta.SelectedText(); got != "alpha" {
			t.Fatalf("selected = %q, want alpha", got)
		}
		ta.KeyAction(KeyLeft, ModCtrl|ModShift)
		if _, _, active := ta.Selection(); active {
			t.Error("extending back to the anchor must collapse the selection")
		}
	})

	t.Run("ctrl+backspace deletes the word before the caret as one undo entry", func(t *testing.T) {
		ta := newTextArea(t, "alpha beta\ngamma")
		ta.SetCursor(0, 10)
		ta.KeyAction(KeyBackspace, ModCtrl)
		if got := ta.Text(); got != "alpha \ngamma" {
			t.Fatalf("text = %q, want 'alpha \\ngamma'", got)
		}
		if line, col := ta.CursorPos(); line != 0 || col != 6 {
			t.Fatalf("cursor = %d:%d, want 0:6", line, col)
		}
		if !ta.Undo() {
			t.Fatal("first undo must restore the word")
		}
		if got := ta.Text(); got != "alpha beta\ngamma" {
			t.Fatalf("after undo text = %q, want the original", got)
		}
		if ta.Undo() {
			t.Fatal("a word delete is one undo entry, got two")
		}
	})

	t.Run("ctrl+backspace at the line start is a no-op", func(t *testing.T) {
		ta := newTextArea(t, "ab\ncd")
		ta.SetCursor(1, 0)
		ta.KeyAction(KeyBackspace, ModCtrl)
		if got := ta.Text(); got != "ab\ncd" {
			t.Fatalf("text = %q, want unchanged (no line join)", got)
		}
		if ta.Undo() {
			t.Fatal("a no-op word delete must not record undo history")
		}
	})

	t.Run("ctrl+delete deletes the word after the caret as one undo entry", func(t *testing.T) {
		ta := newTextArea(t, "alpha beta")
		ta.SetCursor(0, 0)
		ta.KeyAction(KeyDelete, ModCtrl)
		if got := ta.Text(); got != " beta" {
			t.Fatalf("text = %q, want ' beta'", got)
		}
		if !ta.Undo() || ta.Undo() {
			t.Fatal("a word delete is exactly one undo entry")
		}
	})

	t.Run("ctrl+delete at the line end is a no-op", func(t *testing.T) {
		ta := newTextArea(t, "ab\ncd")
		ta.SetCursor(0, 2)
		ta.KeyAction(KeyDelete, ModCtrl)
		if got := ta.Text(); got != "ab\ncd" {
			t.Fatalf("text = %q, want unchanged (no line join)", got)
		}
		if ta.Undo() {
			t.Fatal("a no-op word delete must not record undo history")
		}
	})

	t.Run("alt+backspace aliases ctrl+backspace", func(t *testing.T) {
		ta := newTextArea(t, "alpha beta")
		ta.SetCursor(0, 10)
		ta.KeyAction(KeyBackspace, ModAlt)
		if got := ta.Text(); got != "alpha " {
			t.Fatalf("text = %q, want 'alpha '", got)
		}
	})

	t.Run("readonly blocks word deletion but keeps word motion", func(t *testing.T) {
		ta := newTextArea(t, "alpha beta")
		ta.SetReadOnly(true)
		ta.SetCursor(0, 10)
		ta.KeyAction(KeyBackspace, ModCtrl)
		if got := ta.Text(); got != "alpha beta" {
			t.Fatalf("text = %q, want unchanged", got)
		}
		ta.SetCursor(0, 0)
		ta.KeyAction(KeyDelete, ModCtrl)
		if got := ta.Text(); got != "alpha beta" {
			t.Fatalf("text = %q, want unchanged", got)
		}
		ta.KeyAction(KeyRight, ModCtrl)
		if _, col := ta.CursorPos(); col != 5 {
			t.Fatalf("readonly word motion col = %d, want 5", col)
		}
	})

	t.Run("double-click selects the span the word arrows step", func(t *testing.T) {
		ta := newTextArea(t, "foo.bar baz")
		ta.Arrange(render.Rect{X: 3, Y: 0, W: 600, H: 80})
		sh := ta.face.Shape("foo.bar baz", ta.sizePx)
		ta.DoubleClickAt(Point{X: ta.bounds.X + 8 + int(sh.CaretX(5)), Y: 6 + ta.lineHeight()/2})
		start, end, active := ta.Selection()
		if !active || start != (pos{0, 4}) || end != (pos{0, 7}) {
			t.Fatalf("double-click selection = %v..%v active=%v, want 0:4..0:7 true", start, end, active)
		}
		ta.SetCursor(0, 7)
		ta.KeyAction(KeyLeft, ModCtrl)
		if line, col := ta.CursorPos(); line != start.line || col != start.col {
			t.Errorf("ctrl+left from the span end = %d:%d, want the double-click start %v", line, col, start)
		}
		ta.SetCursor(0, 4)
		ta.KeyAction(KeyRight, ModCtrl)
		if line, col := ta.CursorPos(); line != end.line || col != end.col {
			t.Errorf("ctrl+right from the span start = %d:%d, want the double-click end %v", line, col, end)
		}
	})
}
