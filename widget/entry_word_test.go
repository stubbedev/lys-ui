package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestEntryWordEditing(t *testing.T) {
	newWordEntry := func(t *testing.T, s string) *Entry {
		t.Helper()
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText(s)
		return e
	}

	t.Run("ctrl+right lands on word ends across punctuation", func(t *testing.T) {
		e := newWordEntry(t, "server.hostname.example")
		e.MoveHome()
		for _, want := range []int{6, 15, 23, 23} {
			e.KeyAction(KeyRight, ModCtrl)
			if got := e.Cursor(); got != want {
				t.Fatalf("ctrl+right cursor = %d, want %d", got, want)
			}
		}
	})

	t.Run("ctrl+left lands on word starts across punctuation", func(t *testing.T) {
		e := newWordEntry(t, "server.hostname.example")
		for _, want := range []int{16, 7, 0, 0} {
			e.KeyAction(KeyLeft, ModCtrl)
			if got := e.Cursor(); got != want {
				t.Fatalf("ctrl+left cursor = %d, want %d", got, want)
			}
		}
	})

	t.Run("a punctuation run is skipped as one gap", func(t *testing.T) {
		e := newWordEntry(t, "foo...bar")
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != "foo..." {
			t.Fatalf("text = %q, want foo...", got)
		}
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != "" {
			t.Fatalf("text = %q, want empty", got)
		}
	})

	t.Run("unicode words move whole", func(t *testing.T) {
		e := newWordEntry(t, "æøå blåbær")
		e.MoveHome()
		e.KeyAction(KeyRight, ModCtrl)
		if got := e.Cursor(); got != 3 {
			t.Fatalf("cursor = %d, want 3 (end of æøå)", got)
		}
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != " blåbær" {
			t.Fatalf("text = %q, want ' blåbær'", got)
		}
	})

	t.Run("ctrl+shift+arrow extends the selection word-wise", func(t *testing.T) {
		e := newWordEntry(t, "server.hostname.example")
		e.MoveHome()
		e.KeyAction(KeyRight, ModCtrl|ModShift)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 6 {
			t.Fatalf("selection = %d..%d active=%v, want 0..6 true", start, end, active)
		}
		e.KeyAction(KeyRight, ModCtrl|ModShift)
		if got, _ := e.SelectedText(); got != "server.hostname" {
			t.Fatalf("selected = %q, want server.hostname", got)
		}
		e.KeyAction(KeyLeft, ModCtrl|ModShift)
		if got, _ := e.SelectedText(); got != "server." {
			t.Fatalf("selected = %q, want server.", got)
		}
	})

	t.Run("ctrl+arrow with a selection collapses to the direction edge first", func(t *testing.T) {
		e := newWordEntry(t, "hello world")
		e.MoveHome()
		e.MoveCursorExtending(5)
		e.KeyAction(KeyRight, ModCtrl)
		if got := e.Cursor(); got != 5 {
			t.Fatalf("cursor = %d, want 5 (collapse without moving)", got)
		}
		e.KeyAction(KeyRight, ModCtrl)
		if got := e.Cursor(); got != 11 {
			t.Fatalf("cursor = %d, want 11 (second press moves a word)", got)
		}
	})

	t.Run("ctrl+backspace deletes the word before the caret as one undo entry", func(t *testing.T) {
		e := newWordEntry(t, "hello world")
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != "hello " {
			t.Fatalf("text = %q, want 'hello '", got)
		}
		if got := e.Cursor(); got != 6 {
			t.Fatalf("cursor = %d, want 6", got)
		}
		if !e.Undo() {
			t.Fatal("first undo must restore the word")
		}
		if got := e.Text(); got != "hello world" {
			t.Fatalf("after undo text = %q, want 'hello world'", got)
		}
		if e.Undo() {
			t.Fatal("a word delete is one undo entry, got two")
		}
	})

	t.Run("ctrl+delete deletes the word after the caret as one undo entry", func(t *testing.T) {
		e := newWordEntry(t, "hello world")
		e.MoveHome()
		e.KeyAction(KeyDelete, ModCtrl)
		if got := e.Text(); got != " world" {
			t.Fatalf("text = %q, want ' world'", got)
		}
		if got := e.Cursor(); got != 0 {
			t.Fatalf("cursor = %d, want 0", got)
		}
		if !e.Undo() || e.Undo() {
			t.Fatal("a word delete is exactly one undo entry")
		}
	})

	t.Run("ctrl+delete with a selection deletes just the selection", func(t *testing.T) {
		e := newWordEntry(t, "hello world")
		e.MoveHome()
		e.MoveCursorExtending(5)
		e.KeyAction(KeyDelete, ModCtrl)
		if got := e.Text(); got != " world" {
			t.Fatalf("text = %q, want ' world'", got)
		}
	})

	t.Run("alt+backspace aliases ctrl+backspace", func(t *testing.T) {
		e := newWordEntry(t, "hello world")
		e.KeyAction(KeyBackspace, ModAlt)
		if got := e.Text(); got != "hello " {
			t.Fatalf("text = %q, want 'hello '", got)
		}
	})

	t.Run("ctrl+backspace at the field start is a no-op", func(t *testing.T) {
		e := newWordEntry(t, "word")
		e.MoveHome()
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != "word" {
			t.Fatalf("text = %q, want unchanged", got)
		}
		if e.Undo() {
			t.Fatal("a no-op word delete must not record undo history")
		}
	})

	t.Run("readonly blocks word deletion but keeps word motion", func(t *testing.T) {
		e := newWordEntry(t, "alpha beta")
		e.SetReadOnly(true)
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != "alpha beta" {
			t.Fatalf("text = %q, want unchanged", got)
		}
		e.MoveHome()
		e.KeyAction(KeyDelete, ModCtrl)
		if got := e.Text(); got != "alpha beta" {
			t.Fatalf("text = %q, want unchanged", got)
		}
		e.KeyAction(KeyRight, ModCtrl)
		if got := e.Cursor(); got != 5 {
			t.Fatalf("readonly word motion cursor = %d, want 5", got)
		}
	})

	t.Run("echo masking keeps word math logical", func(t *testing.T) {
		e := newWordEntry(t, "ab cd")
		e.SetEcho(EchoPassword)
		e.MoveHome()
		e.KeyAction(KeyRight, ModCtrl)
		if got := e.Cursor(); got != 2 {
			t.Fatalf("cursor = %d, want 2 (logical word end)", got)
		}
		if got, want := e.displayText(), strings.Repeat(passwordDot, 5); got != want {
			t.Fatalf("display = %q, want %q", got, want)
		}
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != " cd" {
			t.Fatalf("text = %q, want ' cd'", got)
		}
		if got, want := e.displayText(), strings.Repeat(passwordDot, 3); got != want {
			t.Fatalf("display = %q, want %q", got, want)
		}
	})

	t.Run("double-click selects the span the word arrows step", func(t *testing.T) {
		e := newWordEntry(t, "foo.bar baz")
		sh := e.face.Shape("foo.bar baz", 14)
		e.DoubleClickAt(Point{X: 8 + int(sh.CaretX(5)), Y: 15})
		start, end, active := e.Selection()
		if !active || start != 4 || end != 7 {
			t.Fatalf("double-click selection = %d..%d active=%v, want 4..7 true", start, end, active)
		}
		e.ClickAt(Point{X: 8 + int(sh.CaretX(7)), Y: 15})
		e.KeyAction(KeyLeft, ModCtrl)
		if got := e.Cursor(); got != start {
			t.Errorf("ctrl+left from the span end = %d, want the double-click start %d", got, start)
		}
		e.MoveHome()
		for range start {
			e.MoveCursor(1)
		}
		e.KeyAction(KeyRight, ModCtrl)
		if got := e.Cursor(); got != end {
			t.Errorf("ctrl+right from the span start = %d, want the double-click end %d", got, end)
		}
	})
}
