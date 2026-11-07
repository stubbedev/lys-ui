package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A wrapping rich label breaks the text at words to the offered width:
// measure reports one line height per row and the widest row, arrange
// lays the rows out, and paint draws every row (smoke-checked through
// the laid-out rows).
func TestRichLabelWrap(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "the quick brown fox jumps over the lazy dog", 0)
	one := SizeWithin(l, Size{W: 600, H: 400})
	l.SetWrap(true)
	if !l.Wrap() {
		t.Fatal("wrap did not stick")
	}
	tall := SizeWithin(l, Size{W: 120, H: 400})
	if tall.H <= one.H {
		t.Fatalf("wrapped height %d, want more than the single line's %d", tall.H, one.H)
	}
	if tall.W >= one.W {
		t.Errorf("wrapped natural width %d, want under the single line's %d", tall.W, one.W)
	}
	arrangeTree(t, l, 120, tall.H)
	if len(l.lines) < 3 {
		t.Errorf("rows = %d, want the text broken over several", len(l.lines))
	}
	rowText := func(i int) string {
		var sb strings.Builder
		for _, r := range l.lines[i].runs {
			sb.WriteString(r.sh.Text())
		}
		return sb.String()
	}
	for i := range l.lines {
		if strings.TrimSpace(rowText(i)) == "" {
			t.Errorf("row %d is blank", i)
		}
		if first := rowText(i); strings.HasPrefix(first, " ") {
			t.Errorf("row %d starts with a dropped break space: %q", i, first)
		}
	}
	joined := rowText(0) + rowText(1)
	if !strings.Contains(strings.Join([]string{"the", "quick"}, " "), " ") {
		t.Fatal("test text changed")
	}
	if !strings.Contains(joined, "the quick") || !strings.Contains(joined, "quick") {
		t.Errorf("rows lost words: %q", joined)
	}
}

// maxLines caps the rows and ellipsizes the merged last row.
func TestRichLabelWrapMaxLines(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "one two three four five six seven eight nine ten", 0)
	l.SetWrap(true)
	l.SetEllipsize(EllipsizeEnd)
	l.SetMaxLines(2)
	sz := SizeWithin(l, Size{W: 100, H: 400})
	arrangeTree(t, l, 100, sz.H)
	if len(l.lines) != 2 {
		t.Fatalf("rows = %d, want the 2-line cap", len(l.lines))
	}
	var last strings.Builder
	for _, r := range l.lines[1].runs {
		last.WriteString(r.sh.Text())
	}
	if !strings.Contains(last.String(), "…") {
		t.Errorf("last row %q, want the ellipsis", last.String())
	}
	if l.MaxLines() != 2 {
		t.Error("MaxLines did not stick")
	}
}

// Styling survives the wrap: a bold word carries its variant face onto
// whichever row it lands.
func TestRichLabelWrapKeepsStyles(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "plain <b>boldword</b> plain plain plain", 0)
	l.SetWrap(true)
	sz := SizeWithin(l, Size{W: 90, H: 400})
	arrangeTree(t, l, 90, sz.H)
	bold := 0
	for _, line := range l.lines {
		for _, r := range line.runs {
			if r.style.Bold {
				bold++
			}
		}
	}
	if bold == 0 {
		t.Error("the bold span lost its style across the wrap")
	}
}

// max-width-chars caps the natural width as GTK's does: a long
// wrapping text asks for its widest row wrapped at the cap, over more
// rows; a short text keeps its own width; 0 removes the cap.
func TestRichLabelMaxWidthChars(t *testing.T) {
	long := NewRichLabel(entryFace(t), 14, "the quick brown fox jumps over the lazy dog again and again", 0)
	long.SetWrap(true)
	wide := SizeWithin(long, Size{W: 2000, H: 400})
	long.SetMaxWidthChars(12)
	capped := SizeWithin(long, Size{W: 2000, H: 400})
	if capped.W >= wide.W || capped.H <= wide.H {
		t.Errorf("capped %v, want narrower and taller than the free %v", capped, wide)
	}
	if long.MaxWidthChars() != 12 {
		t.Error("MaxWidthChars did not stick")
	}
	long.SetMaxWidthChars(0)
	if got := SizeWithin(long, Size{W: 2000, H: 400}); got != wide {
		t.Errorf("after clearing %v, want the free %v", got, wide)
	}

	short := NewRichLabel(entryFace(t), 14, "hi", 0)
	bare := SizeWithin(short, Size{W: 400, H: 40}).W
	short.SetMaxWidthChars(24)
	if got := SizeWithin(short, Size{W: 400, H: 40}).W; got != bare {
		t.Errorf("a short text under the cap measured %d, want its own %d", got, bare)
	}

	// GTK's max-width-chars sizes a label that can narrow: a line that
	// neither wraps nor ellipsizes keeps its width.
	line := NewRichLabel(entryFace(t), 14, "a single line that runs past the cap", 0)
	free := SizeWithin(line, Size{W: 2000, H: 40}).W
	line.SetMaxWidthChars(6)
	if got := SizeWithin(line, Size{W: 2000, H: 40}).W; got != free {
		t.Errorf("a single line under the cap measured %d, want its own %d", got, free)
	}
	line.SetEllipsize(EllipsizeEnd)
	if got := SizeWithin(line, Size{W: 2000, H: 40}).W; got >= free {
		t.Errorf("an ellipsizing line under the cap measured %d, want less than %d", got, free)
	}
}

// A rich label without a programmatic color paints the cascade's ink,
// as a plain label does; a programmatic color still wins.
func TestRichLabelPaintsTheCascadeColor(t *testing.T) {
	ink := render.RGB(0x10, 0xc0, 0x20)
	paint := func(color render.Color) bool {
		root := NewBox(Column, 0, 0)
		root.AttachStylesheet(NewStylesheet(".body { color: #10c020; }", StylePriorityUser))
		l := NewRichLabel(entryFace(t), 14, "inked text", color)
		l.AddClass("body")
		root.Append(l, false)
		SetStyleRoot(root)
		for range 2 {
			SizeWithin(root, Size{W: 200, H: 40})
			root.Arrange(render.Rect{W: 200, H: 40})
		}
		data := make([]byte, render.Stride(200)*40)
		cv := render.NewScaled(data, render.Stride(200), 200, 40, 1, 1)
		cv.Clear(cv.Rect(), 0)
		l.Paint(cv)
		for i := 0; i+3 < len(data); i += 4 {
			if data[i+3] == 0xff && render.RGB(data[i+2], data[i+1], data[i]) == ink {
				return true
			}
		}
		return false
	}
	if !paint(0) {
		t.Error("an uncolored rich label did not paint the stylesheet's ink")
	}
	if paint(render.RGB(0xff, 0, 0)) {
		t.Error("the stylesheet's ink overrode a programmatic color")
	}
}

// A word wider than a row breaks between clusters into a piece per row,
// shaped once: an unbroken run of thousands of characters (a hash, a
// minified line) wraps in time and allocations linear in its rows, and
// no row runs past the width.
func TestRichLabelWrapsALongWordPerRow(t *testing.T) {
	word := strings.Repeat("x", 2000)
	l := NewRichLabel(entryFace(t), 14, "ab "+word+" cd", 0)
	l.SetWrap(true)
	const w = 200
	sz := SizeWithin(l, Size{W: w, H: 4000})
	arrangeTree(t, l, w, sz.H)
	var got strings.Builder
	for i, line := range l.lines {
		var row strings.Builder
		for _, r := range line.runs {
			row.WriteString(r.sh.Text())
		}
		if line.advance > w {
			t.Errorf("row %d is %.0fpx, past the %dpx width", i, line.advance, w)
		}
		got.WriteString(row.String())
	}
	if want := "ab" + word + "cd"; strings.ReplaceAll(got.String(), " ", "") != want {
		t.Errorf("the rows lost text: %d runes of %d", len(got.String()), len(want))
	}
	if len(l.lines) < 10 {
		t.Fatalf("%d rows for a 2000-rune word in %dpx", len(l.lines), w)
	}
	rows := float64(len(l.lines))
	allocs := testing.AllocsPerRun(5, func() {
		l.fitW = -2
		l.reflow(w)
	})
	if allocs > 12*rows {
		t.Errorf("wrapping allocates %.0f times for %.0f rows, want at most %.0f", allocs, rows, 12*rows)
	}
}

// A rich label shapes in the cascade's font-family, as a plain label
// does: under a rule naming a family the resolver serves, it measures as
// a label built with that family's face.
func TestRichLabelTakesTheCascadeFontFamily(t *testing.T) {
	other, err := render.NewFixtureTypeface()
	if err != nil {
		t.Fatal(err)
	}
	SetFaceResolver(func(family string, _ int) (render.Font, bool) { return other, family == "Other" })
	t.Cleanup(func() { SetFaceResolver(nil) })
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(".other { font-family: Other; }", StylePriorityUser))
	styled := NewRichLabel(entryFace(t), 14, "Wide words <b>here</b>", 0)
	styled.AddClass("other")
	plain := NewRichLabel(entryFace(t), 14, "Wide words <b>here</b>", 0)
	root.Append(styled, false)
	root.Append(plain, false)
	SetStyleRoot(root)
	SizeWithin(root, Size{W: 600, H: 100})
	want := NewRichLabel(other, 14, "Wide words <b>here</b>", 0)
	if got, w := styled.Measure(Row, -1).Nat, want.Measure(Row, -1).Nat; got != w {
		t.Errorf("the styled label is %dpx wide, the family's own %dpx", got, w)
	}
	if plain.Measure(Row, -1).Nat == want.Measure(Row, -1).Nat {
		t.Fatal("the two faces measure alike: the test proves nothing")
	}
}
