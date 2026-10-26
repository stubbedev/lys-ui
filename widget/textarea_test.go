package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func newTextArea(t *testing.T, s string) *TextArea {
	t.Helper()
	ta := NewTextArea(entryFace(t), 14, render.RGB(255, 255, 255))
	ta.SetText(s)
	return ta
}

func TestTextAreaEditing(t *testing.T) {
	t.Run("typing splits lines on enter", func(t *testing.T) {
		ta := newTextArea(t, "")
		ta.InsertRune('a')
		ta.InsertRune('b')
		ta.KeyAction(KeyEnter, 0)
		ta.InsertRune('c')
		if got := ta.Text(); got != "ab\nc" {
			t.Errorf("text = %q, want ab\\nc", got)
		}
		line, col := ta.CursorPos()
		if line != 1 || col != 1 {
			t.Errorf("cursor = %d:%d, want 1:1", line, col)
		}
	})

	t.Run("backspace joins the previous line", func(t *testing.T) {
		ta := newTextArea(t, "ab\ncd")
		ta.SetCursor(1, 0)
		ta.Backspace()
		if got := ta.Text(); got != "abcd" {
			t.Errorf("text = %q, want abcd", got)
		}
		line, col := ta.CursorPos()
		if line != 0 || col != 2 {
			t.Errorf("cursor = %d:%d, want 0:2", line, col)
		}
	})

	t.Run("delete joins the next line", func(t *testing.T) {
		ta := newTextArea(t, "ab\ncd")
		ta.SetCursor(0, 2)
		ta.Delete()
		if got := ta.Text(); got != "abcd" {
			t.Errorf("text = %q, want abcd", got)
		}
	})

	t.Run("arrows walk across line boundaries", func(t *testing.T) {
		ta := newTextArea(t, "ab\ncd")
		ta.SetCursor(1, 0)
		ta.KeyAction(KeyLeft, 0)
		line, col := ta.CursorPos()
		if line != 0 || col != 2 {
			t.Errorf("cursor = %d:%d, want 0:2", line, col)
		}
		ta.KeyAction(KeyRight, 0)
		line, col = ta.CursorPos()
		if line != 1 || col != 0 {
			t.Errorf("cursor = %d:%d, want 1:0", line, col)
		}
	})

	t.Run("vertical motion keeps the preferred column", func(t *testing.T) {
		ta := newTextArea(t, "abcdef\nx\nlonger line")
		ta.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 100})
		ta.SetCursor(0, 4)
		ta.KeyAction(KeyDown, 0)
		line, col := ta.CursorPos()
		if line != 1 || col != 1 {
			t.Errorf("cursor = %d:%d, want 1:1", line, col)
		}
		ta.KeyAction(KeyDown, 0)
		line, col = ta.CursorPos()
		if line != 2 || col < 3 {
			t.Errorf("cursor = %d:%d, want line 2 near col 4", line, col)
		}
		ta.KeyAction(KeyUp, 0)
		line, _ = ta.CursorPos()
		if line != 1 {
			t.Errorf("cursor line = %d, want 1", line)
		}
	})

	t.Run("typing replaces a multi-line selection", func(t *testing.T) {
		ta := newTextArea(t, "one\ntwo\nthree")
		ta.SelectAll()
		ta.InsertRune('X')
		if got := ta.Text(); got != "X" {
			t.Errorf("text = %q, want X", got)
		}
	})

	t.Run("selected text spans lines with newlines", func(t *testing.T) {
		ta := newTextArea(t, "one\ntwo\nthree")
		ta.SetCursor(2, 3)
		ta.anchor = pos{0, 1}
		got, ok := ta.SelectedText()
		if !ok {
			t.Fatal("no selection reported")
		}
		if got != "ne\ntwo\nthr" {
			t.Errorf("selected = %q, want ne\\ntwo\\nthr", got)
		}
	})

	t.Run("cut via delete removes across lines", func(t *testing.T) {
		ta := newTextArea(t, "one\ntwo")
		ta.SelectAll()
		ta.Delete()
		if got := ta.Text(); got != "" {
			t.Errorf("text = %q, want empty", got)
		}
		if strings.Count(ta.Text(), "\n") != 0 {
			t.Error("document must collapse to a single empty line")
		}
	})
}
