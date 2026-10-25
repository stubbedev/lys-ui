package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

func TestEntryClickToCaret(t *testing.T) {
	face := entryFace(t)
	e := NewEntry(face, 13, render.RGB(255, 255, 255))
	e.SetText("hello")
	e.Measure(Constraints{Max: Size{W: 200, H: 100}})
	e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})

	t.Run("clicking past the end puts the caret at the end", func(t *testing.T) {
		textW := int(face.Shape("hello", 13).Advance())
		e.ClickAt(Point{X: e.Bounds().X + entryPad.Left + textW + 50, Y: 15})
		if e.Cursor() != 5 {
			t.Errorf("cursor = %d, want 5", e.Cursor())
		}
	})

	t.Run("clicking mid-text splits the caret", func(t *testing.T) {
		sh := face.Shape("hello", 13)
		e.ClickAt(Point{X: e.Bounds().X + entryPad.Left + int(sh.CaretX(3)), Y: 15})
		if e.Cursor() != 3 {
			t.Errorf("cursor = %d, want 3", e.Cursor())
		}
	})

	t.Run("clicking before the text puts the caret at the start", func(t *testing.T) {
		e.ClickAt(Point{X: e.Bounds().X + 1, Y: 15})
		if e.Cursor() != 0 {
			t.Errorf("cursor = %d, want 0", e.Cursor())
		}
	})

	t.Run("typing after a click inserts at the caret", func(t *testing.T) {
		sh := face.Shape("hello", 13)
		e.ClickAt(Point{X: e.Bounds().X + entryPad.Left + int(sh.CaretX(2)), Y: 15})
		e.InsertRune('X')
		if got := e.Text(); got != "heXllo" {
			t.Errorf("text = %q, want heXllo", got)
		}
	})
}

func TestEntryOnChanged(t *testing.T) {
	seen := ""
	fired := 0
	e := NewEntry(testFace(t), 14, render.RGB(255, 255, 255))
	e.OnChanged = func(s string) { fired++; seen = s }

	t.Run("typing fires with the new contents", func(t *testing.T) {
		e.InsertRune('a')
		if fired != 1 || seen != "a" {
			t.Errorf("after insert fired=%d seen=%q, want 1 %q", fired, seen, "a")
		}
	})

	t.Run("backspace fires", func(t *testing.T) {
		e.Backspace()
		if fired != 2 || seen != "" {
			t.Errorf("after backspace fired=%d seen=%q, want 2 empty", fired, seen)
		}
	})

	t.Run("delete on an empty entry does not fire", func(t *testing.T) {
		e.Delete()
		if fired != 2 {
			t.Errorf("fired = %d, want unchanged", fired)
		}
	})

	t.Run("SetText fires only on real changes", func(t *testing.T) {
		e.SetText("hello")
		if fired != 3 || seen != "hello" {
			t.Errorf("after SetText fired=%d seen=%q", fired, seen)
		}
		e.SetText("hello")
		if fired != 3 {
			t.Errorf("identical SetText fired again: %d", fired)
		}
	})
}
