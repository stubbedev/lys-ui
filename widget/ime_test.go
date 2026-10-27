package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// newIMEEntry returns an entry holding "hello" with the caret between
// the e and the first l.
func newIMEEntry(t *testing.T) *Entry {
	t.Helper()
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	e.SetText("hello")
	e.MoveCursor(-3)
	if e.Cursor() != 2 {
		t.Fatalf("caret = %d, want 2", e.Cursor())
	}
	return e
}

func TestEntryIMEPreedit(t *testing.T) {
	t.Run("composing text shows without entering the contents", func(t *testing.T) {
		e := newIMEEntry(t)
		e.IMEPreedit("せかい", 6, 6) // caret after the second rune (3+3 bytes)
		if got := e.Text(); got != "hello" {
			t.Errorf("text = %q, want hello (composing text is display-only)", got)
		}
		if !e.Composing() {
			t.Error("entry must report composing while preedit is up")
		}
		if e.Cursor() != 2 {
			t.Errorf("content caret = %d, want unchanged 2", e.Cursor())
		}
		if got := string(e.displayRunes()); got != "heせかいllo" {
			t.Errorf("display = %q, want heせかいllo", got)
		}
		if caret := e.caretRune(); caret != 4 {
			t.Errorf("display caret = %d, want 4 (inside the composing text)", caret)
		}
	})

	t.Run("commit replaces the display and inserts at the caret", func(t *testing.T) {
		e := newIMEEntry(t)
		e.IMEPreedit("せかい", 6, 6)
		e.IMECommit("世界")
		if got := e.Text(); got != "he世界llo" {
			t.Errorf("text = %q, want he世界llo", got)
		}
		if e.Composing() {
			t.Error("entry must stop composing after a commit")
		}
		if e.Cursor() != 4 {
			t.Errorf("caret = %d, want 4 (after the committed text)", e.Cursor())
		}
	})

	t.Run("empty preedit ends composing", func(t *testing.T) {
		e := newIMEEntry(t)
		e.IMEPreedit("せ", 3, 3)
		e.IMEPreedit("", 0, 0)
		if e.Composing() {
			t.Error("empty preedit must end composing")
		}
		if e.Cursor() != 2 || e.Text() != "hello" {
			t.Errorf("contents changed by preedit lifecycle: %q caret %d", e.Text(), e.Cursor())
		}
	})

	t.Run("hidden composing caret hides the caret", func(t *testing.T) {
		e := newIMEEntry(t)
		e.IMEPreedit("x", -1, -1)
		if caret := e.caretRune(); caret != -1 {
			t.Errorf("caret = %d, want -1 (hidden)", caret)
		}
	})

	t.Run("backspace trims the composing text before touching contents", func(t *testing.T) {
		e := newIMEEntry(t)
		e.IMEPreedit("ab", 1, 1)
		e.KeyAction(KeyBackspace, 0)
		if got := string(e.displayRunes()); got != "heallo" {
			t.Errorf("display = %q, want heallo", got)
		}
		e.KeyAction(KeyBackspace, 0)
		if e.Composing() {
			t.Error("composing must end once the composing text is gone")
		}
		if got := e.Text(); got != "hello" {
			t.Errorf("text = %q, want hello (backspace never touched contents)", got)
		}
	})

	t.Run("caret motion drops the composing display", func(t *testing.T) {
		e := newIMEEntry(t)
		e.IMEPreedit("x", 1, 1)
		e.MoveCursor(-1)
		if e.Composing() {
			t.Error("caret motion must drop the composing display")
		}
	})

	t.Run("composing replaces a selection; commit inserts at its start", func(t *testing.T) {
		e := newIMEEntry(t)
		e.SelectAll()
		e.IMEPreedit("x", 1, 1)
		if start, end, active := e.Selection(); active {
			t.Errorf("selection %d..%d must be dropped while composing", start, end)
		}
		if e.Text() != "" {
			t.Errorf("text = %q, want empty (composing removed the selection)", e.Text())
		}
		e.IMECommit("y")
		if got := e.Text(); got != "y" {
			t.Errorf("text = %q, want y (commit replaced the selection)", got)
		}
	})

	t.Run("IME delete removes bytes around the caret", func(t *testing.T) {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("héllo")
		e.MoveHome()
		e.MoveCursor(2) // after hé, 3 bytes in
		e.IMEDelete(3, 2)
		if got := e.Text(); got != "o" {
			t.Errorf("text = %q, want o", got)
		}
		if e.Cursor() != 0 {
			t.Errorf("caret = %d, want 0", e.Cursor())
		}
	})

	t.Run("surrounding text reports byte offsets and excludes composing", func(t *testing.T) {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("héllo")
		e.MoveHome()
		e.MoveCursor(2) // after hé
		e.IMEPreedit("x", 1, 1)
		text, cursor, anchor := e.IMESurrounding()
		if text != "héllo" {
			t.Errorf("surrounding = %q, want héllo", text)
		}
		if cursor != 3 || anchor != 3 {
			t.Errorf("caret offsets = %d..%d, want 3..3 (bytes of hé)", cursor, anchor)
		}
	})

	t.Run("cursor rect sits inside the field", func(t *testing.T) {
		e := newIMEEntry(t)
		SizeWithin(e, Size{W: 200, H: 100})
		e.Arrange(render.Rect{X: 10, Y: 20, W: 200, H: 30})
		e.IMEPreedit("せ", 3, 3)
		r := e.IMECursorRect()
		if r.X < e.bounds.X || r.X > e.bounds.X+e.bounds.W {
			t.Errorf("caret x = %d outside field %v", r.X, e.bounds)
		}
		if r.Y < e.bounds.Y || r.Y+r.H > e.bounds.Y+e.bounds.H {
			t.Errorf("caret rect %v outside field %v", r, e.bounds)
		}
	})

	t.Run("paint draws the composing display and underline", func(t *testing.T) {
		e := newIMEEntry(t)
		SizeWithin(e, Size{W: 200, H: 100})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
		e.IMEPreedit("せ", 3, 3)
		cv := render.New(make([]byte, render.Stride(200)*30), render.Stride(200), 200, 30)
		e.Paint(cv)
	})
}

func TestTextAreaIMEPreedit(t *testing.T) {
	newArea := func(t *testing.T) *TextArea {
		t.Helper()
		a := NewTextArea(entryFace(t), 13, render.RGB(255, 255, 255))
		a.SetText("hello\nworld")
		a.SetCursor(0, 5)
		return a
	}

	t.Run("composing text shows without entering the contents", func(t *testing.T) {
		a := newArea(t)
		a.IMEPreedit("せ", 3, 3)
		if got := a.Text(); got != "hello\nworld" {
			t.Errorf("text = %q, want hello\\nworld", got)
		}
		if !a.Composing() {
			t.Error("area must report composing while preedit is up")
		}
		if got := string(a.displayLine(0)); got != "helloせ" {
			t.Errorf("display line = %q, want helloせ", got)
		}
		if line, _ := a.CursorPos(); line != 0 {
			t.Errorf("cursor line = %d, want unchanged 0", line)
		}
	})

	t.Run("commit inserts at the caret", func(t *testing.T) {
		a := newArea(t)
		a.IMEPreedit("せ", 3, 3)
		a.IMECommit("世界")
		if got := a.Text(); got != "hello世界\nworld" {
			t.Errorf("text = %q, want hello世界\\nworld", got)
		}
		if a.Composing() {
			t.Error("area must stop composing after a commit")
		}
	})

	t.Run("the visual row cache covers the composing display", func(t *testing.T) {
		a := newArea(t)
		SizeWithin(a, Size{W: 60, H: 400})
		a.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 400})
		a.IMEPreedit("せせせせ", 12, 12) // wraps "helloせせせせ" onto several rows
		a.ensureRows(a.wrapWidth())
		covered := 0
		for _, r := range a.rows {
			if r.line != 0 {
				continue
			}
			if r.startCol >= r.endCol {
				t.Fatalf("empty row %v in the cache", r)
			}
			covered += r.endCol - r.startCol
		}
		if want := len(a.displayLine(0)); covered != want {
			t.Errorf("rows cover %d runes of the composing line, want %d", covered, want)
		}
		if caret := a.caretPos(); caret.col != 5+4 {
			t.Errorf("display caret = %d, want 9", caret.col)
		}
		cv := render.New(make([]byte, render.Stride(60)*200), render.Stride(60), 60, 200)
		a.Paint(cv) // the underline and caret must land inside the rebuilt rows
	})

	t.Run("IME delete removes bytes across a line break", func(t *testing.T) {
		a := NewTextArea(entryFace(t), 13, render.RGB(255, 255, 255))
		a.SetText("ab\ncd")
		a.SetCursor(1, 1) // end of the document, rune 4 in "ab\ncd"
		a.IMEDelete(2, 0) // c and the line break
		if got := a.Text(); got != "abd" {
			t.Errorf("text = %q, want abd", got)
		}
		if line, col := a.CursorPos(); line != 0 || col != 2 {
			t.Errorf("caret = %d:%d, want 0:2", line, col)
		}
	})

	t.Run("IME delete removes bytes forward across a line break", func(t *testing.T) {
		a := NewTextArea(entryFace(t), 13, render.RGB(255, 255, 255))
		a.SetText("ab\ncd")
		a.SetCursor(0, 1)
		a.IMEDelete(0, 2) // b and the line break
		if got := a.Text(); got != "acd" {
			t.Errorf("text = %q, want acd", got)
		}
	})

	t.Run("surrounding text counts newlines in byte offsets", func(t *testing.T) {
		a := newArea(t)
		a.SetCursor(1, 0) // byte 6 in "hello\nworld"
		text, cursor, anchor := a.IMESurrounding()
		if text != "hello\nworld" || cursor != 6 || anchor != 6 {
			t.Errorf("surrounding = %q %d..%d, want hello\\nworld 6..6", text, cursor, anchor)
		}
	})

	t.Run("repositioning drops the composing display", func(t *testing.T) {
		a := newArea(t)
		a.IMEPreedit("せ", 3, 3)
		a.SetCursor(1, 0)
		if a.Composing() {
			t.Error("SetCursor must drop the composing display")
		}
		a.IMEPreedit("せ", 3, 3)
		a.ClickAt(Point{X: 8, Y: 6})
		if a.Composing() {
			t.Error("ClickAt must drop the composing display")
		}
	})

	t.Run("backspace trims the composing text", func(t *testing.T) {
		a := newArea(t)
		a.IMEPreedit("ab", 1, 1)
		a.KeyAction(KeyBackspace, 0)
		if got := string(a.displayLine(0)); got != "helloa" {
			t.Errorf("display = %q, want helloa", got)
		}
		if a.Text() != "hello\nworld" {
			t.Errorf("text = %q, contents must be untouched", a.Text())
		}
	})

	t.Run("cursor rect and multiline flag", func(t *testing.T) {
		a := newArea(t)
		SizeWithin(a, Size{W: 200, H: 100})
		a.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60})
		a.IMEPreedit("せ", 3, 3)
		if !a.IMEMultiline() {
			t.Error("text area must hint multiline")
		}
		r := a.IMECursorRect()
		if r.Y < a.bounds.Y || r.Y+r.H > a.bounds.Y+a.bounds.H {
			t.Errorf("caret rect %v outside the field %v", r, a.bounds)
		}
		if r.X < a.bounds.X || r.X > a.bounds.X+a.bounds.W {
			t.Errorf("caret x %d outside the field %v", r.X, a.bounds)
		}
	})
}
