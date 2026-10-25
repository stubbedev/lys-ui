package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

func TestEntryEcho(t *testing.T) {
	newEntry := func() *Entry {
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("secret")
		e.MoveHome()
		return e
	}

	t.Run("password dots replace every rune in the display only", func(t *testing.T) {
		e := newEntry()
		e.SetEcho(EchoPassword)
		if got := e.Text(); got != "secret" {
			t.Errorf("text = %q, want secret (masking is display-only)", got)
		}
		if got := e.displayText(); got != strings.Repeat(passwordDot, 6) {
			t.Errorf("display = %q, want six dots", got)
		}
	})

	t.Run("the caret stays on logical runes under masking", func(t *testing.T) {
		e := newEntry()
		e.Measure(Constraints{Max: Size{W: 200, H: 100}})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
		e.SetEcho(EchoPassword)
		sh := e.face.Shape(e.displayText(), 14)
		e.ClickAt(Point{X: entryPad.Left + int(sh.CaretX(4)), Y: 15})
		if e.Cursor() != 4 {
			t.Errorf("cursor = %d, want 4 (logical rune index)", e.Cursor())
		}
		if caret := e.caretRune(); caret != 4 {
			t.Errorf("caret = %d, want 4", caret)
		}
		e.InsertRune('X')
		if got := e.Text(); got != "secrXet" {
			t.Errorf("text = %q, want secrXet (typing inserts at the logical caret)", got)
		}
	})

	t.Run("a selection maps to runes and reveals nothing", func(t *testing.T) {
		e := newEntry()
		e.SetEcho(EchoPassword)
		e.MoveCursorExtending(6)
		start, end, active := e.Selection()
		if !active || start != 0 || end != 6 {
			t.Fatalf("selection = %d..%d active=%v, want 0..6 true", start, end, active)
		}
		if got := e.displayText(); got != strings.Repeat(passwordDot, 6) {
			t.Errorf("display = %q, want only dots (the selection must not reveal runes)", got)
		}
	})

	t.Run("EchoNone shows nothing while the runes stay logical", func(t *testing.T) {
		e := newEntry()
		e.SetEcho(EchoNone)
		if got := e.displayText(); got != "" {
			t.Errorf("display = %q, want empty", got)
		}
		if got := e.Text(); got != "secret" {
			t.Errorf("text = %q, want secret", got)
		}
		e.MoveCursorExtending(3)
		if e.Cursor() != 3 {
			t.Errorf("cursor = %d, want 3 (logical)", e.Cursor())
		}
	})

	t.Run("EchoReveal shows the runes and toggles off again", func(t *testing.T) {
		e := newEntry()
		e.SetEcho(EchoPassword)
		e.EchoReveal(true)
		if !e.Revealing() {
			t.Error("Revealing = false, want true")
		}
		if got := e.displayText(); got != "secret" {
			t.Errorf("display = %q, want secret while revealed", got)
		}
		e.EchoReveal(false)
		if got := e.displayText(); got != strings.Repeat(passwordDot, 6) {
			t.Errorf("display = %q, want dots again after the reveal ends", got)
		}
	})

	t.Run("SetEcho keeps the editing state and fires nothing", func(t *testing.T) {
		e := newEntry()
		e.MoveCursorExtending(4)
		fired := 0
		e.OnChanged = func(string) { fired++ }
		e.SetEcho(EchoPassword)
		if e.Cursor() != 4 {
			t.Errorf("cursor = %d, want 4 (echo must not move the caret)", e.Cursor())
		}
		if _, _, active := e.Selection(); !active {
			t.Error("echo must not drop the selection")
		}
		if fired != 0 {
			t.Errorf("OnChanged fired %d times, want 0", fired)
		}
		if e.Echo() != EchoPassword {
			t.Errorf("Echo = %v, want EchoPassword", e.Echo())
		}
	})

	t.Run("masking composes with preedit display", func(t *testing.T) {
		e := newEntry()
		e.SetEcho(EchoPassword)
		e.IMEPreedit("ab", 2, 2)
		// The caret sits at rune 0, so the composing text splices in
		// front of the six masked runes; masking stays one-to-one, so
		// display indices keep matching displayRunes.
		if got, want := e.displayText(), strings.Repeat(passwordDot, 8); got != want {
			t.Errorf("display = %d runes, want %d masked runes", len([]rune(got)), len([]rune(want)))
		}
		if caret := e.caretRune(); caret != 2 {
			t.Errorf("caret = %d, want 2 (inside the composing range)", caret)
		}
		e.IMECommit("x")
		if got := e.Text(); got != "xsecret" {
			t.Errorf("text = %q, want xsecret", got)
		}
	})

	t.Run("every mode paints", func(t *testing.T) {
		for _, mode := range []Echo{EchoNormal, EchoPassword, EchoNone} {
			e := newEntry()
			e.Measure(Constraints{Max: Size{W: 200, H: 100}})
			e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
			e.SetEcho(mode)
			e.MoveCursorExtending(3)
			cv := render.New(make([]byte, render.Stride(200)*30), render.Stride(200), 200, 30)
			e.Paint(cv)
		}
	})
}

// TestEntryDisplayTracksContents pins the setRunes discipline: the
// cached display string must equal the runes after every mutation path
// (typing, splice, delete, word deletes, undo/redo, restore, IME
// replace). A missed site would leave the paint reading stale text.
func TestEntryDisplayTracksContents(t *testing.T) {
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	check := func(when string) {
		t.Helper()
		if string(e.runes) != e.disp {
			t.Errorf("%s: display %q drifted from contents %q", when, e.disp, string(e.runes))
		}
		if got := e.displayText(); got != string(e.runes) {
			t.Errorf("%s: displayText %q != contents %q", when, got, string(e.runes))
		}
	}
	e.SetText("hello")
	for _, r := range " world" {
		e.InsertRune(r)
		check("InsertRune " + string(r))
	}
	e.SelectAll()
	e.InsertRune('x')
	check("replace selection")
	e.Undo()
	check("undo")
	e.Redo()
	check("redo")
	e.SetText("again")
	check("SetText")
	e.SelectAll()
	e.Backspace()
	check("Backspace over selection")
	e.Undo()
	check("undo restore")
	e.IMEDelete(1, 1)
	check("IME delete")
}
