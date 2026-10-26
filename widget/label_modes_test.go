package widget

// The label layout modes: word wrap, ellipsize, their interaction, and
// the constraint-driven re-measure that panel resizes rely on. The
// golden half of this coverage lives in TestGoldenLabelModes.

import (
	"math"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

const modePx = 14

// modeLineH is one row of a modePx label: the theme's line box.
func modeLineH(t *testing.T, face render.Font) int {
	t.Helper()
	l := NewLabel(face, modePx, "x", 0)
	return l.lineBox(l.style(l))
}

// widthForLines returns a constraint width that wraps text into exactly
// n rows with the mode fixtures' face.
func widthForLines(t *testing.T, face render.Font, text string, n int) int {
	t.Helper()
	for w := 16; w <= 800; w++ {
		if lines := render.WrapText(face, text, float64(w), modePx); len(lines) == n {
			return w
		}
	}
	t.Fatalf("no width in 16..800 wraps %q into %d rows", text, n)
	return 0
}

func TestLabelWrap(t *testing.T) {
	face := testFace(t)
	lineH := modeLineH(t, face)
	mixed := "alpha beta gam-ma delta epsilon zeta eta theta iota kappa lambda"

	t.Run("three rows with mixed word lengths", func(t *testing.T) {
		w := widthForLines(t, face, mixed, 3)
		l := NewLabel(face, modePx, mixed, render.RGB(255, 255, 255))
		l.SetWrap(true)
		got := l.Measure(Constraints{Max: Size{W: w, H: 400}})
		if got.W != w {
			t.Errorf("wrapped width = %d, want the offered %d", got.W, w)
		}
		if want := 3 * lineH; got.H != want {
			t.Errorf("wrapped height = %d, want %d (3 rows)", got.H, want)
		}
	})

	t.Run("a wide constraint keeps one row", func(t *testing.T) {
		l := NewLabel(face, modePx, mixed, render.RGB(255, 255, 255))
		l.SetWrap(true)
		got := l.Measure(Constraints{Max: Size{W: 4096, H: 400}})
		if got.H != lineH {
			t.Errorf("height = %d, want one line %d", got.H, lineH)
		}
		if got.W != l.natural.W {
			t.Errorf("width = %d, want the widest row %d", got.W, l.natural.W)
		}
	})

	t.Run("an unbreakable token keeps its own overflowing row", func(t *testing.T) {
		token := strings.Repeat("x", 40)
		l := NewLabel(face, modePx, token, render.RGB(255, 255, 255))
		l.SetWrap(true)
		got := l.Measure(Constraints{Max: Size{W: 50, H: 400}})
		if got.H != lineH {
			t.Errorf("height = %d, want one (overflowing) row %d", got.H, lineH)
		}
		if a := face.Shape(token, modePx).Advance(); a <= 50 {
			t.Errorf("token advance %v unexpectedly fits 50; fixture lost its overflow", a)
		}
	})

	t.Run("hard newlines split rows", func(t *testing.T) {
		l := NewLabel(face, modePx, "aa\nbb\nc", render.RGB(255, 255, 255))
		l.SetWrap(true)
		got := l.Measure(Constraints{Max: Size{W: 4096, H: 400}})
		if want := 3 * lineH; got.H != want {
			t.Errorf("height = %d, want %d (three hard rows)", got.H, want)
		}
	})

	t.Run("paint puts ink on first and last rows", func(t *testing.T) {
		w := widthForLines(t, face, mixed, 3)
		l := NewLabel(face, modePx, mixed, render.RGB(255, 255, 255))
		l.SetWrap(true)
		l.Measure(Constraints{Max: Size{W: w, H: 3 * lineH}})
		const pad = 4
		stride := render.Stride(w + 2*pad)
		buf := make([]byte, stride*(3*lineH+2*pad))
		cv := render.New(buf, stride, w+2*pad, 3*lineH+2*pad)
		l.Arrange(render.Rect{X: pad, Y: pad, W: w, H: 3 * lineH})
		l.Paint(cv)
		inkAt := func(y0, y1 int) int {
			n := 0
			for y := y0; y < y1; y++ {
				for x := range w + 2*pad {
					if render.ColorFromBytes(buf[y*stride+x*4:]).A() > 0 {
						n++
					}
				}
			}
			return n
		}
		if inkAt(0, pad+lineH) == 0 {
			t.Error("no ink on the first row")
		}
		if inkAt(pad+2*lineH, pad+3*lineH) == 0 {
			t.Error("no ink on the last row")
		}
		if mid := inkAt(pad+lineH, pad+2*lineH); mid == 0 {
			t.Error("no ink on the middle row")
		}
	})
}

func TestLabelEllipsize(t *testing.T) {
	face := testFace(t)
	text := "alphabetagammadeltaepsilon"

	t.Run("fitting text is untouched", func(t *testing.T) {
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		l.SetEllipsize(EllipsizeEnd)
		got := l.Measure(Constraints{Max: Size{W: 4096, H: 100}})
		if got.W != l.natural.W {
			t.Errorf("width = %d, want the untruncated natural %d", got.W, l.natural.W)
		}
	})

	t.Run("truncation starts one pixel short of fitting", func(t *testing.T) {
		adv := face.Shape(text, modePx).Advance()
		fits := int(math.Ceil(adv)) // >= adv, so the whole text fits
		short := fits - 1           // strictly below adv, so it cannot

		full := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		full.SetEllipsize(EllipsizeEnd)
		if got := full.Measure(Constraints{Max: Size{W: fits, H: 100}}); got.W != full.natural.W {
			t.Errorf("at the fitting width %d: width = %d, want natural %d", fits, got.W, full.natural.W)
		}
		cut := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		cut.SetEllipsize(EllipsizeEnd)
		got := cut.Measure(Constraints{Max: Size{W: short, H: 100}})
		if got.W > short {
			t.Errorf("one pixel short: width = %d, want <= %d", got.W, short)
		}
		if got.W >= full.natural.W {
			t.Errorf("one pixel short: width = %d, want a truncation below %d", got.W, full.natural.W)
		}
	})

	t.Run("middle keeps head and tail where end keeps only the head", func(t *testing.T) {
		adv := face.Shape(text, modePx).Advance()
		w := adv * 3 / 5 // wide enough to keep runes on both sides
		end := render.EllipsizeText(face, text, EllipsizeEnd, w, modePx)
		if !strings.HasSuffix(end, "…") || !strings.HasPrefix(text, strings.TrimSuffix(end, "…")) {
			t.Errorf("end = %q, want a prefix of %q plus an ellipsis", end, text)
		}
		mid := render.EllipsizeText(face, text, EllipsizeMiddle, w, modePx)
		head, tail, ok := strings.Cut(mid, "…")
		if !ok {
			t.Fatalf("middle = %q, want an embedded ellipsis", mid)
		}
		if head == "" || tail == "" {
			t.Errorf("middle = %q, want both a head and a tail around the ellipsis", mid)
		}
		if !strings.HasPrefix(text, head) || !strings.HasSuffix(text, tail) {
			t.Errorf("middle = %q, want head from %q's front and tail from its back", mid, text)
		}
		start := render.EllipsizeText(face, text, EllipsizeStart, w, modePx)
		if !strings.HasPrefix(start, "…") || !strings.HasSuffix(text, strings.TrimPrefix(start, "…")) {
			t.Errorf("start = %q, want an ellipsis plus a suffix of %q", start, text)
		}
	})

	t.Run("SetEllipsize reports the truncated advance", func(t *testing.T) {
		adv := face.Shape(text, modePx).Advance()
		w := int(adv) - 5
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		l.SetEllipsize(EllipsizeMiddle)
		got := l.Measure(Constraints{Max: Size{W: w, H: 100}})
		want := render.EllipsizeText(face, text, EllipsizeMiddle, float64(w), modePx)
		if got.W != int(face.Shape(want, modePx).Advance()+0.5) {
			t.Errorf("width = %d, want the middle-truncated advance of %q", got.W, want)
		}
		if l.Ellipsize() != EllipsizeMiddle {
			t.Errorf("Ellipsize() = %v, want Middle", l.Ellipsize())
		}
	})
}

func TestLabelWrapEllipsizeInteraction(t *testing.T) {
	face := testFace(t)
	lineH := modeLineH(t, face)
	text := "fits fine here " + strings.Repeat("z", 30)
	const w = 100

	plain := render.WrapText(face, text, w, modePx)
	if len(plain) < 2 {
		t.Fatalf("fixture wraps into %d rows, want the token on its own final row", len(plain))
	}

	t.Run("the final row truncates, earlier rows wrap untouched", func(t *testing.T) {
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		l.SetWrap(true)
		l.SetEllipsize(EllipsizeEnd)
		rows := l.wrapped(w)
		if len(rows) != len(plain) {
			t.Fatalf("rows = %d, want the unwrapped count %d", len(rows), len(plain))
		}
		for i, row := range rows[:len(rows)-1] {
			if row != plain[i] {
				t.Errorf("row %d = %q, want the plain wrap %q", i, row, plain[i])
			}
			if strings.Contains(row, "…") {
				t.Errorf("row %d = %q grew an ellipsis; only the final row truncates", i, row)
			}
		}
		last := rows[len(rows)-1]
		if !strings.HasSuffix(last, "…") {
			t.Errorf("final row = %q, want an ellipsis on the overflowing token", last)
		}
		if a := face.Shape(last, modePx).Advance(); a > w {
			t.Errorf("final row advances %v > %d; the ellipsis must fit", a, w)
		}
		if got := l.Measure(Constraints{Max: Size{W: w, H: 400}}); got.H != len(rows)*lineH {
			t.Errorf("height = %d, want %d rows worth %d", got.H, len(rows), len(rows)*lineH)
		}
	})

	t.Run("wrap without a mode keeps the overflowing token", func(t *testing.T) {
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		l.SetWrap(true)
		rows := l.wrapped(w)
		last := rows[len(rows)-1]
		if last != plain[len(plain)-1] || strings.Contains(last, "…") {
			t.Errorf("final row = %q, want the untouched overflow %q", last, plain[len(plain)-1])
		}
	})

	t.Run("both modes off overflows a single clipped line", func(t *testing.T) {
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		got := l.Measure(Constraints{Max: Size{W: w, H: 400}})
		if got.H != lineH {
			t.Errorf("height = %d, want one line %d", got.H, lineH)
		}
		if got.W != w {
			t.Errorf("width = %d, want the clamped offered %d", got.W, w)
		}
	})
}

func TestLabelConstraintRemeasure(t *testing.T) {
	face := testFace(t)
	lineH := modeLineH(t, face)
	mixed := "alpha beta gam-ma delta epsilon zeta eta theta iota kappa lambda"
	w := widthForLines(t, face, mixed, 3)

	t.Run("a narrower constraint re-wraps, a wider one reverts", func(t *testing.T) {
		l := NewLabel(face, modePx, mixed, render.RGB(255, 255, 255))
		l.SetWrap(true)
		if got := l.Measure(Constraints{Max: Size{W: 4096, H: 400}}); got.H != lineH {
			t.Fatalf("wide height = %d, want one row", got.H)
		}
		if got := l.Measure(Constraints{Max: Size{W: w, H: 400}}); got.H != 3*lineH {
			t.Fatalf("narrow height = %d, want three rows", got.H)
		}
		if got := l.Measure(Constraints{Max: Size{W: 4096, H: 400}}); got.H != lineH {
			t.Fatalf("wide again height = %d, want one row from the per-constraint cache", got.H)
		}
	})

	t.Run("mode changes drop the measure cache", func(t *testing.T) {
		l := NewLabel(face, modePx, mixed, render.RGB(255, 255, 255))
		l.Measure(Constraints{Max: Size{W: w, H: 400}})
		l.SetWrap(true)
		if !l.measureDirty {
			t.Error("SetWrap left the cached measure valid")
		}
		if got := l.Measure(Constraints{Max: Size{W: w, H: 400}}); got.H != 3*lineH {
			t.Fatalf("after SetWrap height = %d, want three rows", got.H)
		}
		l.SetEllipsize(EllipsizeEnd)
		if !l.measureDirty {
			t.Error("SetEllipsize left the cached measure valid")
		}
		l.Measure(Constraints{Max: Size{W: w, H: 400}})
		l.SetEllipsize(EllipsizeEnd)
		if l.measureDirty {
			t.Error("a redundant SetEllipsize dropped the cache")
		}
	})
}

func TestLabelEllipsizeTooltipCombo(t *testing.T) {
	// The classic pairing: an ellipsized status line whose full text
	// lives in the tooltip. The mode must shorten only what paints.
	face := testFace(t)
	full := "server-02.example.com: syncing 4096 blocks"
	l := NewLabel(face, modePx, full, render.RGB(255, 255, 255))
	l.SetEllipsize(EllipsizeEnd)
	l.SetTooltip(full)
	got := l.Measure(Constraints{Max: Size{W: 90, H: 100}})
	if got.W > 90 {
		t.Errorf("measure = %d wide, want the ellipsized line within 90", got.W)
	}
	if l.Text() != full || l.TooltipText() != full {
		t.Errorf("text %q / tooltip %q, want the full string kept for both", l.Text(), l.TooltipText())
	}
	if _, ok := Widget(l).(TooltipTexter); !ok {
		t.Error("Label does not satisfy TooltipTexter; the tooltip plumbing cannot read it")
	}
}

// SetMaxWidthChars caps the natural width at n approximate character
// widths: longer text measures at the cap (and ellipsizes there),
// shorter text keeps its own width, and 0 lifts the cap.
func TestLabelMaxWidthChars(t *testing.T) {
	face := testFace(t)
	white := render.RGB(255, 255, 255)
	avg := face.Shape(approxCharSample, modePx).Advance() / float64(len(approxCharSample))
	limit := int(math.Ceil(avg * 5))
	wide := Constraints{Max: Size{W: 4096, H: 100}}

	long := NewLabel(face, modePx, "alphabetagammadeltaepsilon", white)
	long.SetEllipsize(EllipsizeEnd)
	long.SetMaxWidthChars(5)
	if got := long.Measure(wide).W; got > limit || got == 0 {
		t.Errorf("capped width = %d, want within the 5-char cap %d", got, limit)
	}
	if long.MaxWidthChars() != 5 {
		t.Errorf("MaxWidthChars = %d", long.MaxWidthChars())
	}

	short := NewLabel(face, modePx, "ab", white)
	short.SetMaxWidthChars(5)
	if got := short.Measure(wide).W; got != short.natural.W {
		t.Errorf("short text = %d, want its natural %d", got, short.natural.W)
	}

	long.SetMaxWidthChars(0)
	if got := long.Measure(wide).W; got != long.natural.W {
		t.Errorf("uncapped width = %d, want the natural %d", got, long.natural.W)
	}
	// A negative count is no cap, not a zero-width label.
	long.SetMaxWidthChars(-3)
	if long.MaxWidthChars() != 0 || long.Measure(wide).W != long.natural.W {
		t.Error("a negative cap was applied")
	}
}

func TestLabelMaxLines(t *testing.T) {
	face := testFace(t)
	lineH := modeLineH(t, face)
	text := "alpha beta gam-ma delta epsilon zeta eta theta iota kappa lambda"
	w := widthForLines(t, face, text, 4)

	t.Run("wrap and ellipsize fold the overflow into the last row", func(t *testing.T) {
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		l.SetWrap(true)
		l.SetEllipsize(EllipsizeEnd)
		l.SetMaxLines(2)
		rows := l.wrapped(float64(w))
		if len(rows) != 2 {
			t.Fatalf("rows = %d, want 2", len(rows))
		}
		plain := render.WrapText(face, text, float64(w), modePx)
		if rows[0] != plain[0] {
			t.Errorf("row 0 = %q, want the plain wrap %q", rows[0], plain[0])
		}
		if !strings.HasSuffix(rows[1], "…") {
			t.Errorf("last row = %q, want an ellipsis for the dropped rows", rows[1])
		}
		if got := l.Measure(Constraints{Max: Size{W: w, H: 400}}); got.H != 2*lineH {
			t.Errorf("height = %d, want two rows %d", got.H, 2*lineH)
		}
	})

	t.Run("text within the limit is untouched", func(t *testing.T) {
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		l.SetWrap(true)
		l.SetEllipsize(EllipsizeEnd)
		l.SetMaxLines(9)
		if rows := l.wrapped(float64(w)); len(rows) != 4 || strings.Contains(strings.Join(rows, ""), "…") {
			t.Errorf("rows = %q, want the four plain rows", rows)
		}
	})

	t.Run("without ellipsize the limit has no effect, as in GTK", func(t *testing.T) {
		l := NewLabel(face, modePx, text, render.RGB(255, 255, 255))
		l.SetWrap(true)
		l.SetMaxLines(2)
		if rows := l.wrapped(float64(w)); len(rows) != 4 {
			t.Errorf("rows = %d, want all 4", len(rows))
		}
		if l.MaxLines() != 2 {
			t.Errorf("MaxLines = %d", l.MaxLines())
		}
		l.SetMaxLines(-3)
		if l.MaxLines() != 0 {
			t.Errorf("negative limit = %d, want 0", l.MaxLines())
		}
	})
}
