package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// testClock hands the undo stack a controllable clock so coalescing's
// idle break is deterministic.
type testClock struct {
	at time.Time
}

func newTestClock() *testClock { return &testClock{at: time.Unix(0, 0)} }

func (c *testClock) Now() time.Time { return c.at }

func (c *testClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func TestEntryUndo(t *testing.T) {
	newUndoEntry := func(t *testing.T, c *testClock) *Entry {
		t.Helper()
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.hist.now = c.Now
		return e
	}

	t.Run("a typing run is one undo entry", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		for _, r := range "hello" {
			e.InsertRune(r)
			c.advance(50 * time.Millisecond)
		}
		if !e.Undo() {
			t.Fatal("undo reported nothing to do")
		}
		if got := e.Text(); got != "" {
			t.Errorf("text after one undo = %q, want empty (run coalesced)", got)
		}
		if e.Undo() {
			t.Error("second undo must find nothing")
		}
		if !e.Redo() || e.Text() != "hello" {
			t.Errorf("redo gave %q, want hello", e.Text())
		}
	})

	t.Run("a word boundary breaks the run", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		for _, r := range "ab cd" {
			e.InsertRune(r)
			c.advance(10 * time.Millisecond)
		}
		// Runs: "ab" | " " | "cd".
		if !e.Undo() || e.Text() != "ab " {
			t.Errorf("first undo gave %q, want 'ab '", e.Text())
		}
		if !e.Undo() || e.Text() != "ab" {
			t.Errorf("second undo gave %q, want ab", e.Text())
		}
		if !e.Undo() || e.Text() != "" {
			t.Errorf("third undo gave %q, want empty", e.Text())
		}
		if e.Undo() {
			t.Error("history must be exhausted")
		}
	})

	t.Run("an idle gap breaks the run", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.InsertRune('a')
		c.advance(2 * time.Second)
		e.InsertRune('b')
		if !e.Undo() || e.Text() != "a" {
			t.Errorf("first undo gave %q, want a (idle closed the run)", e.Text())
		}
		if !e.Undo() || e.Text() != "" {
			t.Errorf("second undo gave %q, want empty", e.Text())
		}
	})

	t.Run("a paste is one entry", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.Insert("hello world")
		if !e.Undo() || e.Text() != "" {
			t.Errorf("undo gave %q, want empty (paste never splits)", e.Text())
		}
	})

	t.Run("undo across a selection replace restores exactly", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.SetText("hello world")
		e.MoveHome()
		e.MoveCursorExtending(5)
		e.InsertRune('X')
		if got := e.Text(); got != "X world" {
			t.Fatalf("text = %q, want 'X world'", got)
		}
		if !e.Undo() {
			t.Fatal("undo reported nothing to do")
		}
		if got := e.Text(); got != "hello world" {
			t.Errorf("text = %q, want 'hello world'", got)
		}
		start, end, active := e.Selection()
		if !active || start != 0 || end != 5 {
			t.Errorf("selection = %d..%d active=%v, want the replaced 0..5 back", start, end, active)
		}
		// Redo then re-undo must be stable.
		if !e.Redo() || e.Text() != "X world" {
			t.Errorf("redo gave %q, want 'X world'", e.Text())
		}
		if !e.Undo() || e.Text() != "hello world" {
			t.Errorf("re-undo gave %q, want 'hello world'", e.Text())
		}
		if start, end, active := e.Selection(); !active || start != 0 || end != 5 {
			t.Errorf("selection = %d..%d active=%v, want 0..5 stable", start, end, active)
		}
	})

	t.Run("each delete is its own entry", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		for _, r := range "abc" {
			e.InsertRune(r)
			c.advance(10 * time.Millisecond)
		}
		e.Backspace()
		e.Backspace()
		if !e.Undo() || e.Text() != "ab" {
			t.Errorf("first undo gave %q, want ab", e.Text())
		}
		if !e.Undo() || e.Text() != "abc" {
			t.Errorf("second undo gave %q, want abc", e.Text())
		}
	})

	t.Run("the stack caps at 100 and drops the oldest", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		for i := range undoLimit + 1 {
			e.InsertRune(rune('a' + i%26))
			c.advance(2 * time.Second) // every rune its own entry
		}
		for range undoLimit {
			if !e.Undo() {
				t.Fatalf("undo ran dry after %d entries", undoLimit)
			}
		}
		if got := e.Text(); len(got) != 1 {
			t.Errorf("text after %d undos = %d runes, want 1 (the oldest entry was dropped)", len(got), undoLimit)
		}
		if e.Undo() {
			t.Error("the oldest entry must have been dropped, not kept")
		}
	})

	t.Run("SetText from the app clears the stack", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.InsertRune('a')
		e.SetText("reset")
		if e.Undo() {
			t.Error("undo must not resurrect pre-SetText contents")
		}
		if e.Redo() {
			t.Error("redo must be gone too")
		}
	})

	t.Run("a fresh edit clears redo", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.InsertRune('a')
		e.Undo()
		e.InsertRune('b')
		if e.Redo() {
			t.Error("redo must not resurrect the forked branch")
		}
		if got := e.Text(); got != "b" {
			t.Errorf("text = %q, want b", got)
		}
	})

	t.Run("undo fires OnChanged and schedules damage", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.InsertRune('a')
		seen := ""
		e.OnChanged = func(s string) { seen = s }
		e.invalid = false
		if !e.Undo() {
			t.Fatal("undo reported nothing to do")
		}
		if seen != "" {
			t.Errorf("OnChanged saw %q, want empty", seen)
		}
		if !e.invalid {
			t.Error("undo must invalidate like the edit it reverts")
		}
	})

	t.Run("IME preedit records nothing; the commit lands one entry", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.MoveEnd()
		e.IMEPreedit("か", 3, 3)
		c.advance(10 * time.Millisecond)
		e.IMEPreedit("かな", 6, 6)
		c.advance(10 * time.Millisecond)
		if e.Undo() {
			t.Fatal("preedit ticks must not enter the history")
		}
		e.IMECommit("かな")
		if !e.Undo() || e.Text() != "" {
			t.Errorf("undo gave %q, want empty (the commit is one entry)", e.Text())
		}
		if e.Undo() {
			t.Error("one commit must be one entry")
		}
	})

	t.Run("an IME delete is one entry", func(t *testing.T) {
		c := newTestClock()
		e := newUndoEntry(t, c)
		e.SetText("héllo")
		e.MoveHome()
		e.MoveCursor(2)
		e.IMEDelete(3, 2) // removes "ll"
		if !e.Undo() || e.Text() != "héllo" {
			t.Errorf("undo gave %q, want héllo", e.Text())
		}
	})
}

func TestTextAreaUndo(t *testing.T) {
	newUndoArea := func(t *testing.T, c *testClock, s string) *TextArea {
		t.Helper()
		ta := NewTextArea(entryFace(t), 14, render.RGB(255, 255, 255))
		ta.hist.now = c.Now
		ta.SetText(s)
		return ta
	}

	t.Run("typing coalesces and Enter breaks it", func(t *testing.T) {
		c := newTestClock()
		ta := newUndoArea(t, c, "")
		ta.InsertRune('a')
		ta.InsertRune('b')
		c.advance(10 * time.Millisecond)
		ta.KeyAction(KeyEnter, 0)
		ta.InsertRune('c')
		if !ta.Undo() || ta.Text() != "ab\n" {
			t.Errorf("first undo gave %q, want ab\\n", ta.Text())
		}
		if !ta.Undo() || ta.Text() != "ab" {
			t.Errorf("second undo gave %q, want ab", ta.Text())
		}
		if !ta.Undo() || ta.Text() != "" {
			t.Errorf("third undo gave %q, want empty", ta.Text())
		}
	})

	t.Run("undo across a selection replace restores exactly", func(t *testing.T) {
		c := newTestClock()
		ta := newUndoArea(t, c, "one\ntwo\nthree")
		ta.SelectAll()
		ta.InsertRune('X')
		if got := ta.Text(); got != "X" {
			t.Fatalf("text = %q, want X", got)
		}
		if !ta.Undo() {
			t.Fatal("undo reported nothing to do")
		}
		if got := ta.Text(); got != "one\ntwo\nthree" {
			t.Errorf("text = %q, want the full document back", got)
		}
		start, end, active := ta.Selection()
		if !active || start != (pos{0, 0}) || end != (pos{2, 5}) {
			t.Errorf("selection = %v..%v active=%v, want the replaced 0:0..2:5 back", start, end, active)
		}
		if !ta.Redo() || ta.Text() != "X" {
			t.Errorf("redo gave %q, want X", ta.Text())
		}
		if !ta.Undo() || ta.Text() != "one\ntwo\nthree" {
			t.Errorf("re-undo gave %q, want the full document", ta.Text())
		}
	})

	t.Run("backspace across a line break is one entry", func(t *testing.T) {
		c := newTestClock()
		ta := newUndoArea(t, c, "one\ntwo")
		ta.SetCursor(1, 0)
		ta.Backspace()
		if got := ta.Text(); got != "onetwo" {
			t.Fatalf("text = %q, want onetwo", got)
		}
		if !ta.Undo() {
			t.Fatal("undo reported nothing to do")
		}
		if got := ta.Text(); got != "one\ntwo" {
			t.Errorf("text = %q, want one\\ntwo", got)
		}
		if line, col := ta.CursorPos(); line != 1 || col != 0 {
			t.Errorf("cursor = %d:%d, want 1:0 (back where the break was)", line, col)
		}
	})

	t.Run("a paste is one entry", func(t *testing.T) {
		c := newTestClock()
		ta := newUndoArea(t, c, "keep ")
		ta.KeyAction(KeyEnd, 0)
		ta.Insert("a b c")
		if !ta.Undo() || ta.Text() != "keep " {
			t.Errorf("undo gave %q, want 'keep ' (paste never splits)", ta.Text())
		}
	})

	t.Run("the stack caps at 100 and drops the oldest", func(t *testing.T) {
		c := newTestClock()
		ta := newUndoArea(t, c, "")
		for i := range undoLimit + 1 {
			ta.InsertRune(rune('a' + i%26))
			c.advance(2 * time.Second)
		}
		for range undoLimit {
			if !ta.Undo() {
				t.Fatalf("undo ran dry after %d entries", undoLimit)
			}
		}
		if got := ta.Text(); len(got) != 1 {
			t.Errorf("text after %d undos = %d runes, want 1", len(got), undoLimit)
		}
		if ta.Undo() {
			t.Error("the oldest entry must have been dropped")
		}
	})

	t.Run("SetText from the app clears the stack", func(t *testing.T) {
		c := newTestClock()
		ta := newUndoArea(t, c, "draft")
		ta.InsertRune('!')
		ta.SetText("reset")
		if ta.Undo() || ta.Redo() {
			t.Error("SetText must clear undo and redo")
		}
	})

	t.Run("undo schedules damage and rebuilds the row cache", func(t *testing.T) {
		c := newTestClock()
		ta := newUndoArea(t, c, "one")
		ta.Measure(Constraints{Max: Size{W: 200, H: 100}})
		ta.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60})
		ta.KeyAction(KeyEnd, 0)
		ta.InsertRune('X')
		ta.Backspace()
		ta.Undo()
		ta.invalid = false
		if !ta.Undo() {
			t.Fatal("undo reported nothing to do")
		}
		if !ta.invalid {
			t.Error("undo must invalidate like the edit it reverts")
		}
		if ta.rowsValid {
			t.Error("undo must drop the visual row cache")
		}
	})
}
