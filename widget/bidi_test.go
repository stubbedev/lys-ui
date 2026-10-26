package widget

// The bidirectional editing and layout tests (#68): visual arrow and
// word motion over mixed Hebrew/Latin content, the Direction API on
// the text widgets and Box, and RTL mirroring of alignment and pan.
// Editing positions stay logical rune indexes throughout; only the
// arrows' mapping is visual. Hebrew shapes through the fixture chain —
// Cantarell covers no Hebrew.

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/text"
)

// bidiFace shapes Latin and digits with Cantarell, Hebrew with the
// Hebrew fixture face — the mixed-script fallback production chains
// take. One per caller: faces are stateful.
func bidiFace(tb testing.TB) render.Font {
	tb.Helper()
	c, err := render.FixtureChain()
	if err != nil {
		tb.Fatalf("load fixture chain: %v", err)
	}
	return c
}

// placeCursor parks the entry caret (and anchor) at a logical column.
func placeCursor(e *Entry, col int) {
	e.clearPreedit()
	e.cursor, e.anchor = col, col
}

func TestEntryVisualArrows(t *testing.T) {
	const mixed = "abc אבג 123"
	newMixed := func() *Entry {
		e := NewEntry(bidiFace(t), 14, render.RGB(255, 255, 255))
		e.SetText(mixed)
		e.MoveHome()
		return e
	}

	t.Run("right from the junction walks around the Hebrew block", func(t *testing.T) {
		e := newMixed()
		placeCursor(e, 4)
		e.KeyAction(KeyRight, 0)
		if got := e.Cursor(); got != 8 {
			t.Fatalf("right from 4 = %d, want 8", got)
		}
		e.KeyAction(KeyRight, 0)
		if got := e.Cursor(); got != 9 {
			t.Fatalf("right from 8 = %d, want 9", got)
		}
	})
	t.Run("left from inside the Hebrew block moves visually", func(t *testing.T) {
		e := newMixed()
		placeCursor(e, 5) // between א and ב, visually right of ב
		e.KeyAction(KeyLeft, 0)
		if got := e.Cursor(); got != 6 {
			t.Fatalf("left from 5 = %d, want 6", got)
		}
	})
	t.Run("shift extends the selection along the visual walk", func(t *testing.T) {
		e := newMixed()
		placeCursor(e, 4)
		e.KeyAction(KeyRight, ModShift)
		start, end, active := e.Selection()
		if !active || start != 4 || end != 8 {
			t.Fatalf("selection = %d..%d active=%v, want 4..8 true", start, end, active)
		}
	})
	t.Run("an LTR-only line steps logically", func(t *testing.T) {
		e := NewEntry(bidiFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("abc def")
		e.MoveHome()
		e.KeyAction(KeyRight, 0)
		if got := e.Cursor(); got != 1 {
			t.Errorf("right from 0 = %d, want 1", got)
		}
	})
	t.Run("MoveCursor keeps its logical contract", func(t *testing.T) {
		e := newMixed()
		placeCursor(e, 4)
		e.MoveCursor(1)
		if got := e.Cursor(); got != 5 {
			t.Errorf("logical right from 4 = %d, want 5", got)
		}
	})
	t.Run("selection collapse follows the visual edge", func(t *testing.T) {
		e := newMixed()
		e.SelectAll()
		e.KeyAction(KeyLeft, 0)
		// The visually leftmost boundary of the whole line is logical 0.
		if got := e.Cursor(); got != 0 {
			t.Errorf("collapse left = %d, want 0", got)
		}
	})
}

func TestEntryWordMotionBidi(t *testing.T) {
	e := NewEntry(bidiFace(t), 14, render.RGB(255, 255, 255))
	e.SetText("alpha אבג beta")
	e.MoveHome()
	e.KeyAction(KeyRight, ModCtrl)
	if got := e.Cursor(); got != 5 {
		t.Fatalf("word right from 0 = %d, want 5", got)
	}
	// Word right steps across the Hebrew block to its far edge.
	e.KeyAction(KeyRight, ModCtrl)
	if got := e.Cursor(); got != 9 {
		t.Fatalf("word right from 5 = %d, want 9", got)
	}
	e.KeyAction(KeyRight, ModCtrl)
	if got := e.Cursor(); got != 14 {
		t.Fatalf("word right from 9 = %d, want 14", got)
	}
	// Word left crosses back into the Hebrew block.
	e.KeyAction(KeyLeft, ModCtrl)
	if got := e.Cursor(); got != 10 {
		t.Fatalf("word left from 14 = %d, want 10", got)
	}
}

func TestEntryRTLDirection(t *testing.T) {
	newRTL := func(text string) *Entry {
		e := NewEntry(bidiFace(t), 14, render.RGB(255, 255, 255))
		e.SetText(text)
		e.SetDirection(DirectionRTL)
		e.MoveHome()
		return e
	}

	t.Run("short text hugs the right edge of the field", func(t *testing.T) {
		e := newRTL("hello")
		e.Measure(Constraints{Max: Size{W: 300, H: 40}})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 40})
		sh := e.shape(e.displayText())
		lx := e.lineX(sh)
		inner := e.innerRect()
		want := inner.X + inner.W - int(sh.Advance()+0.5)
		if lx <= e.bounds.X+8 {
			t.Fatalf("line origin = %d, want the right edge %d", lx, want)
		}
	})
	t.Run("auto keeps the same text left-hugging", func(t *testing.T) {
		e := NewEntry(bidiFace(t), 14, render.RGB(255, 255, 255))
		e.SetText("hello")
		e.Measure(Constraints{Max: Size{W: 300, H: 40}})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 40})
		if got := e.lineX(e.shape(e.displayText())); got != e.bounds.X+entryPad.Left {
			t.Errorf("auto LTR line origin = %d, want %d", got, e.bounds.X+entryPad.Left)
		}
	})
	t.Run("the caret starts at the reading edge and arrows move visually", func(t *testing.T) {
		e := newRTL("hello אבג")
		if got := e.Cursor(); got != 0 {
			t.Fatalf("cursor = %d, want 0 (the reading start)", got)
		}
		e.KeyAction(KeyRight, 0) // visually right of the reading start enters the island
		if got := e.Cursor(); got != 1 {
			t.Fatalf("right from the reading start = %d, want 1", got)
		}
		e.KeyAction(KeyLeft, 0)
		e.KeyAction(KeyLeft, 0) // visually left: through the island into the Hebrew block
		if got := e.Cursor(); got != 5 {
			t.Fatalf("left from the reading start = %d, want 5", got)
		}
	})
	t.Run("typing at the caret inserts logically", func(t *testing.T) {
		e := newRTL("")
		e.InsertRune('א')
		e.InsertRune('b')
		if got, want := e.Text(), "אb"; got != want {
			t.Fatalf("text = %q, want %q", got, want)
		}
		if got := e.Cursor(); got != 2 {
			t.Errorf("cursor = %d, want 2", got)
		}
	})
}

func TestTextAreaVisualArrows(t *testing.T) {
	ta := NewTextArea(bidiFace(t), 14, render.RGB(255, 255, 255))
	ta.SetText("abc אבג 123")
	ta.SetCursor(0, 4)
	ta.KeyAction(KeyRight, 0)
	if _, col := ta.CursorPos(); col != 8 {
		t.Fatalf("right from 4 = %d, want 8", col)
	}
	ta.KeyAction(KeyLeft, 0)
	if _, col := ta.CursorPos(); col != 4 {
		t.Fatalf("left from 8 = %d, want 4", col)
	}
	ta.KeyAction(KeyLeft, 0)
	if _, col := ta.CursorPos(); col != 3 {
		t.Fatalf("left from 4 = %d, want 3", col)
	}
}

func TestLabelDirection(t *testing.T) {
	paintInkLeft := func(l *Label, w, h int) int {
		t.Helper()
		stride := render.Stride(w)
		buf := make([]byte, stride*h)
		cv := render.New(buf, stride, w, h)
		l.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
		l.Paint(cv)
		left := w
		for x := range w {
			for y := range h {
				if px := render.ColorFromBytes(buf[y*stride+x*4:]); px.A() > 8 {
					left = min(left, x)
				}
			}
		}
		return left
	}

	const hebrew = "אבג אבג"
	t.Run("auto resolves right to left and hugs the right edge", func(t *testing.T) {
		l := NewLabel(bidiFace(t), 14, hebrew, render.RGB(255, 255, 255))
		if !text.RTL(l.Text(), l.Direction()) {
			t.Fatalf("auto base = LTR, want RTL for %q", hebrew)
		}
		if got := paintInkLeft(l, 300, 24); got < 200 {
			t.Errorf("ink starts at %d, want the right half", got)
		}
	})
	t.Run("forced LTR keeps the same text left-hugging", func(t *testing.T) {
		l := NewLabel(bidiFace(t), 14, hebrew, render.RGB(255, 255, 255))
		l.SetDirection(DirectionLTR)
		if got := paintInkLeft(l, 300, 24); got > 4 {
			t.Errorf("ink starts at %d, want the left edge", got)
		}
	})
	t.Run("auto leaves Latin text left-hugging", func(t *testing.T) {
		l := NewLabel(bidiFace(t), 14, "hello", render.RGB(255, 255, 255))
		if got := paintInkLeft(l, 300, 24); got > 4 {
			t.Errorf("ink starts at %d, want the left edge", got)
		}
	})
	t.Run("explicit end alignment mirrors for RTL", func(t *testing.T) {
		l := NewLabel(bidiFace(t), 14, "hello", render.RGB(255, 255, 255))
		l.SetDirection(DirectionRTL)
		l.SetAlignment(render.AlignEnd)
		if got := paintInkLeft(l, 300, 24); got > 8 {
			t.Errorf("end in RTL ink starts at %d, want the mirrored left edge", got)
		}
	})
}

func TestRichLabelDirection(t *testing.T) {
	t.Run("styled spans reorder with the line", func(t *testing.T) {
		l := NewRichLabel(bidiFace(t), 14, "hello <b>אבג</b> 123", render.RGB(255, 255, 255))
		if l.Text() != "hello אבג 123" {
			t.Fatalf("text = %q", l.Text())
		}
		// Visual order: hello, digits, Hebrew bold — the bold piece is
		// last in the draw list and keeps its bold face.
		last := l.shaped[len(l.shaped)-1]
		if !last.style.Bold {
			t.Errorf("last visual piece style = %+v, want the bold span", last.style)
		}
		for _, r := range l.shaped {
			if r.start < 0 || r.start >= len([]rune(l.Text())) {
				t.Errorf("piece start %d outside the line", r.start)
			}
		}
	})
}

func TestBoxDirection(t *testing.T) {
	t.Run("an RTL row flows from the right edge", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		b.SetDirection(DirectionRTL)
		a, c := newStub(10, 5), newStub(8, 4)
		b.Append(a, false)
		b.Append(c, false)
		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})
		if a.rect.X != 40-1-10 {
			t.Errorf("first child x = %d, want rightmost %d", a.rect.X, 40-1-10)
		}
		if c.rect.X != 40-1-10-2-8 {
			t.Errorf("second child x = %d, want left of it", c.rect.X)
		}
	})
	t.Run("LTR is unchanged", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		a, c := newStub(10, 5), newStub(8, 4)
		b.Append(a, false)
		b.Append(c, false)
		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})
		if a.rect != (render.Rect{X: 1, Y: 1, W: 10, H: 18}) {
			t.Errorf("first child rect = %v", a.rect)
		}
	})
}

func TestGoldenBidi(t *testing.T) {
	chain := bidiFace(t)
	th := DarkTheme()

	t.Run("label", func(t *testing.T) {
		l := NewLabel(chain, 14, "hello אבג 123!", th.Text)
		NewGolden(t, l, "label-bidi", goldenTheme(th), goldenFrame(220, 24))
	})
	t.Run("label rtl", func(t *testing.T) {
		l := NewLabel(chain, 14, "שלום, world — זהו RTL label", th.Text)
		l.SetDirection(DirectionRTL)
		NewGolden(t, l, "label-bidi-rtl", goldenTheme(th), goldenFrame(260, 24))
	})
	t.Run("entry", func(t *testing.T) {
		e := NewEntry(chain, 14, th.Text)
		e.SetText("abc אבג 123")
		NewGolden(t, e, "entry-bidi", goldenTheme(th), goldenFrame(220, 36),
			goldenFocus(e), goldenAfterArrange(func() {
				e.MoveHome()
				e.MoveCursorExtending(3) // select "abc" across the junction
			}))
	})
}
