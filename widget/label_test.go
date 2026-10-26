package widget

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"
)

func testFace(t *testing.T) *render.Typeface {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func TestLabelMeasure(t *testing.T) {
	face := testFace(t)

	t.Run("nonempty text wants positive space", func(t *testing.T) {
		got := NewLabel(face, 14, "lys-ui", render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		if got.W <= 0 || got.H <= 0 {
			t.Errorf("measure = %v, want positive", got)
		}
	})

	t.Run("empty text keeps zero width and the font line height", func(t *testing.T) {
		got := NewLabel(face, 14, "", render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		if got.W != 0 {
			t.Errorf("empty label width = %d, want 0", got.W)
		}
		if got.H <= 0 {
			t.Errorf("empty label height = %d, want the font line height", got.H)
		}
	})

	t.Run("wider text wants more width", func(t *testing.T) {
		short := NewLabel(face, 14, "g", render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		long := NewLabel(face, 14, "lysland", render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		if long.W <= short.W {
			t.Errorf("long %d <= short %d", long.W, short.W)
		}
	})

	t.Run("clamps to the constraint ceiling", func(t *testing.T) {
		got := NewLabel(face, 14, "lys-ui is great", render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 20, H: 8}})
		if got.W != 20 || got.H != 8 {
			t.Errorf("measure = %v, want clamped to 20x8", got)
		}
	})

	t.Run("SetText retakes the measurement", func(t *testing.T) {
		l := NewLabel(face, 14, "a", render.RGB(255, 255, 255))
		before := l.Measure(Constraints{Max: Size{W: 500, H: 100}})
		l.SetText("a much longer label")
		after := l.Measure(Constraints{Max: Size{W: 500, H: 100}})
		if after.W <= before.W {
			t.Errorf("after SetText width %d <= before %d", after.W, before.W)
		}
		if l.Text() != "a much longer label" {
			t.Errorf("Text() = %q", l.Text())
		}
	})

	t.Run("SetText to the same value is a no-op", func(t *testing.T) {
		l := NewLabel(face, 14, "same", render.RGB(255, 255, 255))
		l.SetText("same")
		if l.Text() != "same" {
			t.Errorf("Text() = %q", l.Text())
		}
	})
}

func TestLabelPaint(t *testing.T) {
	face := testFace(t)

	t.Run("text leaves ink in the arranged rect", func(t *testing.T) {
		data := make([]byte, render.Stride(200)*32)
		cv := render.New(data, render.Stride(200), 200, 32)
		l := NewLabel(face, 16, "ink", render.RGB(255, 255, 255))
		l.Measure(Constraints{Max: Size{W: 500, H: 100}})
		l.Arrange(render.Rect{X: 4, Y: 0, W: 100, H: 32})
		l.Paint(cv)
		ink := 0
		for y := range 32 {
			for x := range 200 {
				if render.ColorFromBytes(data[y*render.Stride(200)+x*4:]).A() > 0 {
					ink++
				}
			}
		}
		if ink == 0 {
			t.Error("painted label produced no ink")
		}
	})
}

func TestLabelNaturalHeightPaints(t *testing.T) {
	face := entryFace(t)

	t.Run("a label at its measured size leaves ink", func(t *testing.T) {
		// Regression: Measure rounded the line height down while the
		// paint guard compared the exact float, so some sizes (13px
		// DejaVu) measured a box their own painter rejected.
		const px = 13.0
		l := NewLabel(face, px, "server-01.example", render.RGB(255, 255, 255))
		got := l.Measure(Constraints{Max: Size{W: 500, H: 100}})
		stride := render.Stride(200)
		data := make([]byte, stride*40)
		cv := render.New(data, stride, 200, 40)
		l.Arrange(render.Rect{X: 4, Y: 10, W: got.W, H: got.H})
		l.Paint(cv)
		ink := 0
		for y := 10; y < 10+got.H; y++ {
			for x := range 200 {
				if render.ColorFromBytes(data[y*stride+x*4:]).A() > 0 {
					ink++
				}
			}
		}
		if ink == 0 {
			t.Errorf("label measured %v but its painter drew nothing: natural height must satisfy the paint guard", got)
		}
	})
}

// TestLabelFitsItsNaturalWidth pins that a label laid out at exactly
// its natural width shows its whole text: Paint ellipsizes and wraps
// against the raw advance, so the natural width must round up. Rounded
// to nearest, every advance with a fraction under a half ellipsized at
// its own natural size ("Images" painted as "Imag…").
func TestLabelFitsItsNaturalWidth(t *testing.T) {
	face := testFace(t)
	checked := 0
	for _, text := range []string{"Images", "Cancel", "All Files", "Downloads", "wayle", "ij", "Mm", "Open With", "Text Editor", "x"} {
		for _, px := range []float64{11, 12, 13, 14, 15, 17} {
			adv := face.Shape(text, px).Advance()
			if frac := adv - float64(int(adv)); frac == 0 || frac >= 0.5 {
				continue
			}
			checked++
			l := NewLabel(face, px, text, render.RGB(255, 255, 255))
			l.SetEllipsize(EllipsizeEnd)
			nat := l.Measure(Constraints{Max: Size{W: 1000, H: 100}})
			if float64(nat.W) < adv {
				t.Errorf("%q at %vpx: natural width %d is short of its %.2f advance", text, px, nat.W, adv)
			}
			w := NewLabel(face, px, text+" "+text, render.RGB(255, 255, 255))
			w.SetWrap(true)
			wn := w.Measure(Constraints{Max: Size{W: 1000, H: 100}})
			if rows := w.wrapped(float64(wn.W)); len(rows) != 1 {
				t.Errorf("%q at %vpx: wrapping at its natural width %d broke into %d rows", text, px, wn.W, len(rows))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no sample had an advance with a fraction under a half")
	}
}

// TestRowGivesWayToAnEllipsizingLabel pins the row half of the shrink
// protocol: a Row Box short of room narrows its expanding ellipsizing
// label, so the fixed cells after it stay inside the row instead of
// overflowing; never below the ellipsis; and a label that clips or
// wraps keeps its width.
func TestRowGivesWayToAnEllipsizingLabel(t *testing.T) {
	face := testFace(t)
	row := func(mode EllipsizeMode, wrap bool) (*Box, *Label, *stub) {
		l := NewLabel(face, 13, "a rather long file name that will not fit.png", render.RGB(255, 255, 255))
		l.SetEllipsize(mode)
		l.SetWrap(wrap)
		cell := newStub(50, 10)
		b := NewBox(Row, 0, 0)
		b.Append(l, true)
		b.Append(cell, false)
		b.Measure(Constraints{Max: Size{W: 1000, H: 100}})
		b.Arrange(render.Rect{W: 150, H: 20})
		return b, l, cell
	}
	_, l, cell := row(EllipsizeEnd, false)
	if got := cell.rect; got.X != 100 || got.W != 50 {
		t.Errorf("the fixed cell = %+v, want it at the row's end, 100..150", got)
	}
	if l.Bounds().W != 100 {
		t.Errorf("the label = %d wide, want the 100 left over", l.Bounds().W)
	}
	// Not below the ellipsis.
	b, l, _ := row(EllipsizeEnd, false)
	b.Arrange(render.Rect{W: 52, H: 20})
	floor := int(math.Ceil(face.Shape(render.Ellipsis, 13).Advance()))
	if l.Bounds().W != floor {
		t.Errorf("a starved label = %d wide, want the ellipsis's %d", l.Bounds().W, floor)
	}
	for _, c := range []struct {
		name string
		mode EllipsizeMode
		wrap bool
	}{{"clipping", EllipsizeNone, false}} {
		_, l, cell := row(c.mode, c.wrap)
		if cell.rect.X <= 100 || l.Bounds().W <= 100 {
			t.Errorf("a %s label gave way: label %d, cell at %d", c.name, l.Bounds().W, cell.rect.X)
		}
	}
	wrapped := NewLabel(face, 13, "wraps instead", render.RGB(255, 255, 255))
	wrapped.SetWrap(true)
	wrapped.SetEllipsize(EllipsizeEnd)
	wrapped.Measure(Constraints{Max: Size{W: 1000, H: 100}})
	if got := wrapped.ShrinkableWidth(); got != 0 {
		t.Errorf("a wrapping label offers %dpx of width", got)
	}
}

// width-chars floors the natural width: a short text measures the
// floored width; 0 restores the text's own.
func TestLabelWidthChars(t *testing.T) {
	face := testFace(t)
	nat := func(text string, chars int) int {
		l := NewLabel(face, 13, text, 0)
		l.SetWidthChars(chars)
		return l.Measure(Constraints{Max: Size{W: 300, H: 40}}).W
	}
	wide, plain, free := nat("100%", 6), nat("1%", 6), nat("1%", 0)
	if wide != plain {
		t.Errorf("widths %d vs %d: the floor must even them out", wide, plain)
	}
	if plain <= free {
		t.Errorf("floored width %d, want above the free %d", plain, free)
	}
}

// A stylesheet line-height lays the label out at the given line box:
// unitless numbers scale the font size, the row pitch of a wrapped
// label follows it, and `normal` leaves the font's own box.
func TestLabelLineHeight(t *testing.T) {
	face := goldenFace(t)
	const px = 14
	natural := face.Shape("Hamburgefonstiv", px).LineHeight()
	con := Constraints{Max: Size{W: 400, H: 400}}

	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(`label { line-height: 2; }`, StylePriorityUser))
	l := NewLabel(face, px, "Hamburgefonstiv", render.RGBA(0, 0, 0, 255))
	root.Append(l, false)
	got := root.Measure(con)
	// The unitless number scales the size the label paints at: its
	// constructor 14px, which no font-size rule overrides here, not the
	// 16px root default the cascade would otherwise read.
	if got.H != 28 {
		t.Errorf("line-height: 2 on a 14px label measured %v, want 28", got.H)
	}

	// The wrapped row pitch: two rows stand at one line box apart.
	wrapped := NewLabel(face, px, "one two three four five six", render.RGBA(0, 0, 0, 255))
	wrapped.SetWrap(true)
	wrapped.AttachStylesheet(NewStylesheet(`label { line-height: 40px; }`, StylePriorityUser))
	sz := wrapped.Measure(Constraints{Max: Size{W: 60, H: 400}})
	rows := len(wrapped.wrapped(60))
	if rows < 2 {
		t.Fatalf("the sample did not wrap into rows (%d)", rows)
	}
	if want := rows * 40; sz.H != want {
		t.Errorf("wrapped at 40px/row measured %v, want %v", sz.H, want)
	}

	// Unstyled labels keep the font's own box.
	plain := NewLabel(face, px, "Hamburgefonstiv", render.RGBA(0, 0, 0, 255))
	if got := plain.Measure(con); got.H != natural {
		t.Errorf("unstyled measured %v, want the natural %v", got.H, natural)
	}
}

// A font-feature-settings declaration with tnum shapes through the
// face's tabular twin; without one the constructor face shapes plain.
func TestLabelTabularFeature(t *testing.T) {
	face := goldenFace(t)
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(`label { font-feature-settings: "tnum"; }`, StylePriorityUser))
	l := NewLabel(face, 14, "0123456789", render.RGBA(0, 0, 0, 255))
	root.Append(l, false)
	root.Measure(Constraints{Max: Size{W: 400, H: 400}})
	if got, _ := l.effStyle(); got == render.Font(face) {
		t.Error("the tnum declaration did not switch the label to the tabular face")
	}
	if face.Tabular() == face {
		t.Error("the tabular twin is not distinct")
	}

	plain := NewLabel(face, 14, "0123456789", render.RGBA(0, 0, 0, 255))
	if got, _ := plain.effStyle(); got != render.Font(face) {
		t.Error("an unstyled label does not shape with its own face")
	}
}
