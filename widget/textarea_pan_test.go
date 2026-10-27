package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// newUnwrappedArea builds a wrap-off area with a narrow 80px viewport:
// the horizontal-overflow case, where each logical line may pan.
func newUnwrappedArea(t *testing.T, s string) *TextArea {
	t.Helper()
	ta := newTextArea(t, s)
	ta.SetWrap(false)
	SizeWithin(ta, Size{W: 5000, H: 1000})
	ta.Arrange(render.Rect{X: 3, Y: 0, W: 80, H: 80})
	return ta
}

// areaCaretX reports the caret bar's root-space x: the same math Paint
// uses, the shaped line's caret table at the line origin linePan
// positions.
func areaCaretX(ta *TextArea) int {
	caret := ta.caretPos()
	return ta.caretX(caret.line, caret.col)
}

// assertAreaCaretInside fails when the caret sits outside the padded
// inner rect of the caret's line.
func assertAreaCaretInside(t *testing.T, ta *TextArea) {
	t.Helper()
	left, right := ta.bounds.X+textPad.Left, ta.bounds.X+textPad.Left+ta.wrapWidth()
	if cx := areaCaretX(ta); cx < left || cx > right-1 {
		t.Errorf("caret x = %d outside inner rect [%d,%d) (pan = %d)", cx, left, right, ta.linePan(ta.caretPos().line))
	}
}

// areaMaxPan is the largest pan line 0 may hold: line width plus the
// caret margin past the right edge.
func areaMaxPan(ta *TextArea) int {
	return max(0, int(ta.face.Shape(string(ta.displayLine(0)), ta.sizePx).Advance()+0.5)+caretPad-ta.wrapWidth())
}

func TestTextAreaPanPerLogicalLine(t *testing.T) {
	ta := newUnwrappedArea(t, strings.Repeat("M", 40)+"\nshort")

	t.Run("typing on an unwrapped line pans that line", func(t *testing.T) {
		ta := newUnwrappedArea(t, "")
		for i := 0; i < 100 && ta.linePan(0) == 0; i++ {
			ta.InsertRune('M')
		}
		if ta.linePan(0) <= 0 {
			t.Fatalf("pan = %d after typing 100 runes, want panned", ta.linePan(0))
		}
		assertAreaCaretInside(t, ta)
	})

	t.Run("the pan is per line: the short line stays put", func(t *testing.T) {
		ta.SetCursor(0, 40)
		if ta.linePan(0) <= 0 {
			t.Fatalf("long line pan = %d, want panned", ta.linePan(0))
		}
		assertAreaCaretInside(t, ta)

		ta.SetCursor(1, 5)
		if ta.linePan(1) != 0 {
			t.Errorf("short line pan = %d, want 0", ta.linePan(1))
		}
		// A click on the short line maps unshifted — the long line's
		// pan must not drag other lines sideways.
		sh := ta.face.Shape("short", ta.sizePx)
		ta.ClickAt(Point{X: ta.bounds.X + textPad.Left + int(sh.CaretX(3)), Y: textPad.Top + ta.lineHeight() + ta.lineHeight()/2})
		if line, col := ta.CursorPos(); line != 1 || col != 3 {
			t.Errorf("click on the unpanned line = %d:%d, want 1:3", line, col)
		}
	})

	t.Run("wrap-on rows fit the width and never pan", func(t *testing.T) {
		ta := newTextArea(t, strings.Repeat("M", 100))
		ta.Arrange(render.Rect{X: 3, Y: 0, W: 80, H: 200})
		ta.SetCursor(0, 100)
		if pan := ta.linePan(0); pan != 0 {
			t.Errorf("wrapped line pan = %d, want 0", pan)
		}
		ta.ensureRows(ta.wrapWidth())
		r := ta.rows[ta.rowOf(ta.clamp(ta.cursor))]
		if w := int(ta.face.Shape(string(ta.lines[r.line][r.startCol:r.endCol]), ta.sizePx).Advance() + 0.5); w > ta.wrapWidth() {
			t.Errorf("wrapped row is %dpx wide in a %dpx viewport", w, ta.wrapWidth())
		}
	})
}

func TestTextAreaClickMapsThroughPan(t *testing.T) {
	ta := newUnwrappedArea(t, strings.Repeat("M", 40))
	// Park the caret mid-line: the pan then cuts a rune at the right
	// edge. (At the end, the caret margin keeps every rune on show.)
	ta.SetCursor(0, 30)
	if ta.linePan(0) <= 0 {
		t.Fatalf("pan = %d, want the line panned", ta.linePan(0))
	}
	sh := ta.face.Shape(string(ta.lines[0]), ta.sizePx)
	avail := ta.wrapWidth()
	k := -1
	for i := range 40 {
		rel := sh.CaretX(i) - float64(ta.linePan(0))
		if rel > 0 && rel < float64(avail) && sh.CaretX(i+1)-float64(ta.linePan(0)) > float64(avail) {
			k = i
		}
	}
	if k <= 0 {
		t.Fatalf("no half-visible rune found (k = %d, pan = %d)", k, ta.linePan(0))
	}
	ta.ClickAt(Point{X: ta.bounds.X + textPad.Left + int(sh.CaretX(k)+0.5) - ta.linePan(0), Y: textPad.Top + ta.lineHeight()/2})
	if line, col := ta.CursorPos(); line != 0 || col != k {
		t.Errorf("click = %d:%d, want 0:%d (must map through x + pan)", line, col, k)
	}
}

func TestTextAreaDragAutoPans(t *testing.T) {
	ta := newUnwrappedArea(t, strings.Repeat("M", 40))
	ta.ClickAt(Point{X: ta.bounds.X + 10, Y: textPad.Top + ta.lineHeight()/2})
	if line, col := ta.CursorPos(); line != 0 || col != 0 {
		t.Fatalf("anchor = %d:%d, want 0:0", line, col)
	}
	for range 100 {
		ta.DragMove(Point{X: ta.bounds.X + ta.bounds.W + 30, Y: textPad.Top + ta.lineHeight()/2})
	}
	if line, col := ta.CursorPos(); line != 0 || col != 40 {
		t.Errorf("cursor = %d:%d, want 0:40 after dragging past the edge", line, col)
	}
	if got := ta.linePan(0); got != areaMaxPan(ta) {
		t.Errorf("pan = %d, want clamped to %d", got, areaMaxPan(ta))
	}
	assertAreaCaretInside(t, ta)
}

func TestTextAreaHomeEndPan(t *testing.T) {
	ta := newUnwrappedArea(t, strings.Repeat("M", 40))

	ta.KeyAction(KeyEnd, 0)
	if got := ta.linePan(0); got != areaMaxPan(ta) {
		t.Errorf("after End pan = %d, want %d", got, areaMaxPan(ta))
	}
	assertAreaCaretInside(t, ta)

	ta.KeyAction(KeyHome, 0)
	if got := ta.linePan(0); got != 0 {
		t.Errorf("after Home pan = %d, want 0", got)
	}
}

func TestTextAreaPanUndoAndIME(t *testing.T) {
	ta := newUnwrappedArea(t, strings.Repeat("M", 40))

	t.Run("undo keeps the restored caret in view", func(t *testing.T) {
		ta.KeyAction(KeyEnd, 0) // pan to the end first
		ta.Insert("tail")
		if _, col := ta.CursorPos(); col != 44 {
			t.Fatalf("cursor col = %d, want 44", col)
		}
		if !ta.Undo() {
			t.Fatal("undo had nothing to restore")
		}
		if _, col := ta.CursorPos(); col != 40 {
			t.Errorf("restored col = %d, want 40", col)
		}
		assertAreaCaretInside(t, ta)
	})

	t.Run("the IME caret rectangle is pan-aware", func(t *testing.T) {
		ta.KeyAction(KeyEnd, 0)
		r := ta.IMECursorRect()
		if r.X < ta.bounds.X || r.X+2 > ta.bounds.X+ta.bounds.W {
			t.Errorf("IME caret rect x = %d outside field %v (pan = %d)", r.X, ta.bounds, ta.linePan(0))
		}
	})
}

func TestTextAreaResizeReclampsPan(t *testing.T) {
	ta := newUnwrappedArea(t, strings.Repeat("M", 40))
	ta.KeyAction(KeyEnd, 0) // pan to the end first
	if ta.linePan(0) <= 0 {
		t.Fatal("expected a panned line before resize")
	}

	// Wider than the line: the pan clamps away.
	ta.Arrange(render.Rect{X: 3, Y: 0, W: 600, H: 80})
	if ta.linePan(0) != 0 {
		t.Errorf("pan = %d after widening past the line, want 0", ta.linePan(0))
	}
	assertAreaCaretInside(t, ta)

	// Narrow again: the caret is pulled back into view.
	ta.Arrange(render.Rect{X: 3, Y: 0, W: 80, H: 80})
	if got := ta.linePan(0); got != areaMaxPan(ta) {
		t.Errorf("pan = %d after shrinking, want %d", got, areaMaxPan(ta))
	}
	assertAreaCaretInside(t, ta)
}

func TestTextAreaMeasureCapsNaturalWidth(t *testing.T) {
	paste := strings.Repeat("M", 500)

	t.Run("uncapped, the long line widens the natural width", func(t *testing.T) {
		ta := newTextArea(t, paste)
		ta.SetWrap(false)
		if got := SizeWithin(ta, Size{W: 5000, H: 1000}); got.W < 1000 {
			t.Errorf("natural width = %d, want the content-hugging width (large)", got.W)
		}
	})

	t.Run("MaxWidth caps what Measure reports", func(t *testing.T) {
		ta := newTextArea(t, paste)
		ta.SetWrap(false)
		ta.MaxWidth = 200
		if got := SizeWithin(ta, Size{W: 5000, H: 1000}); got.W != 200 {
			t.Errorf("capped width = %d, want 200 after a 500-rune paste", got.W)
		}
	})
}
