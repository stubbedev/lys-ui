package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// newPanEntry builds a 80px-wide entry holding s: 64px of text viewport,
// so forty wide runes overflow it several times over.
func newPanEntry(t *testing.T, s string) *Entry {
	t.Helper()
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	e.SetText(s)
	SizeWithin(e, Size{W: 5000, H: 100})
	e.Arrange(render.Rect{X: 3, Y: 0, W: 80, H: 30})
	return e
}

// entryCaretX reports the caret bar's root-space x: the same math Paint
// uses, display shape at -scrollX inside the padded field.
func entryCaretX(e *Entry) int {
	sh := e.face.Shape(e.displayText(), e.sizePx)
	return e.bounds.X + entryPad.Left + int(sh.CaretX(e.panCaret())+0.5) - e.scrollX
}

// assertCaretInside fails when the entry's caret sits outside the
// padded inner rect — the "caret walks off the field" regression.
func assertCaretInside(t *testing.T, e *Entry) {
	t.Helper()
	inner := e.innerRect()
	if cx := entryCaretX(e); cx < inner.X || cx > inner.X+inner.W-1 {
		t.Errorf("caret x = %d outside inner rect %v (scrollX = %d)", cx, inner, e.scrollX)
	}
}

// maxPan is the largest scrollX e may hold: display width plus the
// caret margin past the right edge.
func maxPan(e *Entry) int {
	return max(0, int(e.face.Shape(e.displayText(), e.sizePx).Advance()+0.5)+caretPad-e.innerRect().W)
}

func TestEntryPanTypePastWidth(t *testing.T) {
	e := newPanEntry(t, "")

	t.Run("typing past the width advances the pan", func(t *testing.T) {
		for range 100 {
			if e.ScrollX() > 0 {
				break
			}
			e.InsertRune('M')
		}
		if e.ScrollX() <= 0 {
			t.Fatalf("scrollX = %d after typing 100 runes into a narrow field, want panned", e.ScrollX())
		}
		assertCaretInside(t, e)
	})

	t.Run("undo is not a pan entry and still keeps the caret in view", func(t *testing.T) {
		e.SetText(strings.Repeat("M", 40))
		e.Insert("tail") // one undo entry; the caret and pan follow it
		assertCaretInside(t, e)
		if !e.Undo() {
			t.Fatal("undo had nothing to restore")
		}
		if e.Cursor() != 40 {
			t.Errorf("restored cursor = %d, want 40", e.Cursor())
		}
		assertCaretInside(t, e)
		if e.ScrollX() < 0 || e.ScrollX() > maxPan(e) {
			t.Errorf("scrollX = %d outside range [0, %d]", e.ScrollX(), maxPan(e))
		}
	})

	t.Run("a long paste pans to the caret at the end", func(t *testing.T) {
		e.SetText(strings.Repeat("M", 40))
		if e.ScrollX() <= 0 {
			t.Fatalf("scrollX = %d after a 40-rune paste, want panned", e.ScrollX())
		}
		if e.Cursor() != 40 {
			t.Errorf("cursor = %d, want 40", e.Cursor())
		}
		assertCaretInside(t, e)
	})
}

func TestEntryClickMapsThroughPan(t *testing.T) {
	e := newPanEntry(t, strings.Repeat("M", 40))
	// Park the caret mid-text: the pan then cuts a rune at the right
	// edge. (At the end, the caret margin keeps every rune on show.)
	e.MoveCursor(-10)
	sh := e.face.Shape(e.displayText(), e.sizePx)
	avail := e.innerRect().W
	// The last caret boundary fully inside the viewport: the rune right
	// of it is the half-visible one a click must land on.
	k := -1
	for i := range 40 {
		rel := sh.CaretX(i) - float64(e.ScrollX())
		if rel > 0 && rel < float64(avail) && sh.CaretX(i+1)-float64(e.ScrollX()) > float64(avail) {
			k = i
		}
	}
	if k <= 0 {
		t.Fatalf("no half-visible rune found (k = %d, scrollX = %d)", k, e.ScrollX())
	}
	p := Point{X: e.bounds.X + entryPad.Left + int(sh.CaretX(k)+0.5) - e.ScrollX(), Y: 15}
	e.ClickAt(p)
	if e.Cursor() != k {
		t.Errorf("cursor = %d, want %d (click must map through x + scrollX)", e.Cursor(), k)
	}
}

func TestEntryDragAutoPans(t *testing.T) {
	e := newPanEntry(t, strings.Repeat("M", 40))
	e.MoveHome() // unpan; the anchor click lands at the field start
	e.ClickAt(Point{X: e.bounds.X + 10, Y: 15})
	if anchor := e.Cursor(); anchor != 0 {
		t.Fatalf("anchor = %d, want 0", anchor)
	}
	// Drag past the right edge until the pan and the caret bottom out.
	for range 100 {
		e.DragMove(Point{X: e.bounds.X + e.bounds.W + 30, Y: 15})
	}
	if e.Cursor() != 40 {
		t.Errorf("cursor = %d, want 40 after dragging past the edge", e.Cursor())
	}
	if e.ScrollX() != maxPan(e) {
		t.Errorf("scrollX = %d, want clamped to %d", e.ScrollX(), maxPan(e))
	}
	assertCaretInside(t, e)
	start, end, active := e.Selection()
	if !active || start != 0 || end != 40 {
		t.Errorf("selection = %d..%d active=%v, want 0..40 true", start, end, active)
	}
}

func TestEntryHomeEndPans(t *testing.T) {
	e := newPanEntry(t, strings.Repeat("M", 40))

	e.MoveEnd()
	if e.ScrollX() != maxPan(e) {
		t.Errorf("after End scrollX = %d, want %d (panned to the end)", e.ScrollX(), maxPan(e))
	}
	assertCaretInside(t, e)

	e.MoveHome()
	if e.ScrollX() != 0 {
		t.Errorf("after Home scrollX = %d, want 0", e.ScrollX())
	}
	if e.Cursor() != 0 {
		t.Errorf("cursor = %d, want 0", e.Cursor())
	}
}

func TestEntryResizeReclampsPan(t *testing.T) {
	e := newPanEntry(t, strings.Repeat("M", 40))
	if e.ScrollX() <= 0 {
		t.Fatal("expected a panned field before resize")
	}

	// Wider than the text: the pan clamps away and the caret is plain.
	e.Arrange(render.Rect{X: 3, Y: 0, W: 600, H: 30})
	if e.ScrollX() != 0 {
		t.Errorf("scrollX = %d after widening past the text, want 0", e.ScrollX())
	}
	assertCaretInside(t, e)

	// Narrow again: the caret is pulled back into view.
	e.Arrange(render.Rect{X: 3, Y: 0, W: 80, H: 30})
	if e.ScrollX() != maxPan(e) {
		t.Errorf("scrollX = %d after shrinking, want %d", e.ScrollX(), maxPan(e))
	}
	assertCaretInside(t, e)
}

func TestEntryPlaceholderNeverPans(t *testing.T) {
	face := entryFace(t)
	mk := func(ph string) *Entry {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		if ph != "" {
			e.SetPlaceholder(ph)
		}
		SizeWithin(e, Size{W: 5000, H: 100})
		e.Arrange(render.Rect{X: 0, Y: 0, W: 80, H: 30})
		return e
	}
	with, without := mk("hint"), mk("")
	if with.ScrollX() != 0 {
		t.Fatalf("placeholder entry scrollX = %d, want 0", with.ScrollX())
	}
	stride := render.Stride(80)
	bufA := make([]byte, stride*30)
	with.Paint(render.New(bufA, stride, 80, 30))
	bufB := make([]byte, stride*30)
	without.Paint(render.New(bufB, stride, 80, 30))

	first := -1
	for x := range 80 {
		if first >= 0 {
			break
		}
		for y := range 30 {
			o := y*stride + x*4
			if bufA[o] != bufB[o] {
				first = x
				break
			}
		}
	}
	if first < 0 {
		t.Fatal("placeholder painted nothing")
	}
	// The placeholder sits where the text starts (GTK draws it inside
	// the text node): unpanned, at the text area's left edge.
	if left := with.textInsets().Left; first < left || first > left+4 {
		t.Errorf("placeholder ink starts at x = %d, want the text area's origin (~%d..%d)", first, left, left+4)
	}
}

func TestEntryEchoPansByDisplayWidth(t *testing.T) {
	face := entryFace(t)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	e.SetEcho(EchoPassword)
	e.SetText(strings.Repeat("M", 40))
	SizeWithin(e, Size{W: 5000, H: 100})
	e.Arrange(render.Rect{X: 0, Y: 0, W: 80, H: 30})

	// The pan follows the dots on show, not the runes behind them.
	dots := strings.Repeat(passwordDot, 40)
	want := max(0, int(face.Shape(dots, 14).Advance()+0.5)+caretPad-e.innerRect().W)
	if e.ScrollX() != want {
		t.Errorf("scrollX = %d, want %d (display width)", e.ScrollX(), want)
	}
	assertCaretInside(t, e)

	// Park the caret mid-text so a dot cuts at the right edge, then a
	// click lands where the dots are: the logical index of the last
	// fully visible boundary.
	e.MoveCursor(-20)
	sh := face.Shape(e.displayText(), e.sizePx)
	avail := e.innerRect().W
	k := -1
	for i := range 40 {
		rel := sh.CaretX(i) - float64(e.ScrollX())
		if rel > 0 && rel < float64(avail) && sh.CaretX(i+1)-float64(e.ScrollX()) > float64(avail) {
			k = i
		}
	}
	if k <= 0 {
		t.Fatalf("no half-visible dot found (k = %d)", k)
	}
	e.ClickAt(Point{X: e.bounds.X + entryPad.Left + int(sh.CaretX(k)+0.5) - e.ScrollX(), Y: 15})
	if e.Cursor() != k {
		t.Errorf("cursor = %d, want %d (masked click maps through the pan)", e.Cursor(), k)
	}
}

func TestEntryPreeditPansIntoView(t *testing.T) {
	e := newPanEntry(t, strings.Repeat("M", 40))
	before := e.ScrollX()
	e.IMEPreedit("AB", 1, 1)
	if !e.Composing() {
		t.Fatal("preedit did not start composing")
	}
	// The composing display extends past the old end; the pan must
	// follow so the composing caret (after the A) stays on show.
	if e.ScrollX() <= before {
		t.Errorf("scrollX = %d during composing, want > %d", e.ScrollX(), before)
	}
	assertCaretInside(t, e)
	r := e.IMECursorRect()
	if r.X < e.bounds.X || r.X+2 > e.bounds.X+e.bounds.W {
		t.Errorf("IME caret rect x = %d outside field %v", r.X, e.bounds)
	}
}

func TestEntryMeasureCapsNaturalWidth(t *testing.T) {
	face := entryFace(t)
	paste := strings.Repeat("M", 500)

	t.Run("uncapped, the paste widens the natural width", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.SetText(paste)
		if got := SizeWithin(e, Size{W: 5000, H: 100}); got.W < 1000 {
			t.Errorf("natural width = %d, want the content-hugging width (large)", got.W)
		}
	})

	t.Run("MaxWidth caps what Measure reports", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.SetText(paste)
		e.MaxWidth = 150
		if got := SizeWithin(e, Size{W: 5000, H: 100}); got.W != 150 {
			t.Errorf("capped width = %d, want 150 after a 500-rune paste", got.W)
		}
	})
}

// A field at its natural width shows its whole text with the caret at
// the end: the natural width left no room for the pan's caret margin,
// so the pan pushed the first runes out (an input group's "https://"
// read "xample.com" after it, #281).
func TestEntryNaturalWidthNeedsNoPan(t *testing.T) {
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	e.SetText("example.com")
	size := SizeWithin(e, Size{W: 5000, H: 100})
	e.Arrange(render.Rect{W: size.W, H: size.H})
	e.MoveCursor(-1)
	e.MoveCursor(1)
	if e.Cursor() != len([]rune(e.Text())) || e.scrollX != 0 {
		t.Errorf("caret %d, pan %d at the natural width %d, want the caret at the end and no pan", e.Cursor(), e.scrollX, size.W)
	}
}

func TestEntryTextWidthIsFixed(t *testing.T) {
	face := entryFace(t)
	con := Size{W: 5000, H: 100}
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	in := e.textInsets()
	pad := in.Left + in.Right
	hug := SizeWithin(e, con).W
	if hug != pad {
		t.Fatalf("empty content-hugging width = %d, want the padding %d", hug, pad)
	}
	e.SetTextWidth(GTKTextWidth)
	if e.TextWidth() != GTKTextWidth {
		t.Errorf("TextWidth = %d", e.TextWidth())
	}
	if got := SizeWithin(e, con).W; got != GTKTextWidth+pad {
		t.Errorf("empty fixed width = %d, want %d", got, GTKTextWidth+pad)
	}
	e.SetText(strings.Repeat("M", 200))
	if got := SizeWithin(e, con).W; got != GTKTextWidth+pad {
		t.Errorf("a long text resized the fixed field to %d", got)
	}
	e.MaxWidth = 100
	e.InvalidateLayout()
	if got := SizeWithin(e, con).W; got != 100 {
		t.Errorf("MaxWidth no longer caps a fixed field: %d", got)
	}
	e.MaxWidth = 0
	e.SetTextWidth(0)
	if got := SizeWithin(e, con).W; got < 1000 {
		t.Errorf("SetTextWidth(0) kept the field fixed at %d, want it hugging the text again", got)
	}
	e.SetTextWidth(-5)
	if e.TextWidth() != 0 {
		t.Errorf("a negative width stored %d, want 0", e.TextWidth())
	}
}
