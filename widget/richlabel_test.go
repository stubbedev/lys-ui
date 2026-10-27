package widget

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// boldFace loads the Go bold face for variant tests.
func boldFace(t *testing.T) *render.Typeface {
	t.Helper()
	face, err := render.LoadFont(gobold.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// tableOffset returns the file offset of a TrueType table, -1 when
// absent.
func tableOffset(data []byte, tag string) int {
	n := int(binary.BigEndian.Uint16(data[4:6]))
	for i := range n {
		rec := data[12+i*16 : 12+i*16+16]
		if string(rec[:4]) == tag {
			return int(binary.BigEndian.Uint32(rec[8:12]))
		}
	}
	return -1
}

// tallFace loads goregular with its hhea ascender doubled: the same
// outlines with taller line metrics, so mixed-run line boxes can be
// pinned hermetically.
func tallFace(t *testing.T) *render.Typeface {
	t.Helper()
	data := slices.Clone(goregular.TTF)
	off := tableOffset(data, "hhea")
	if off < 0 {
		t.Fatal("goregular has no hhea table")
	}
	asc := int(int16(binary.BigEndian.Uint16(data[off+4 : off+6])))
	binary.BigEndian.PutUint16(data[off+4:off+6], uint16(int16(asc*2)))
	face, err := render.LoadFont(data)
	if err != nil {
		t.Fatalf("patched font rejected: %v", err)
	}
	return face
}

// caretTable assembles the label's global caret table: each run's
// cluster-snapped positions offset by the advances before it. A run's
// final boundary is the next run's first, so it is dropped.
func caretTable(l *RichLabel) []float64 {
	xs := make([]float64, 0, l.runes+1)
	var x float64
	for i, r := range l.shaped {
		cs := r.sh.CaretPositions()
		if i < len(l.shaped)-1 {
			cs = cs[:len(cs)-1]
		}
		for _, cx := range cs {
			xs = append(xs, x+cx)
		}
		x += r.sh.Advance()
	}
	return xs
}

const richPx = 13.0

func TestRichLabelMeasure(t *testing.T) {
	face := testFace(t)
	con := Size{W: 500, H: 100}

	t.Run("plain markup measures like a Label", func(t *testing.T) {
		plain := SizeWithin(NewLabel(face, richPx, "hello world", render.RGB(255, 255, 255)), con)
		rich := SizeWithin(NewRichLabel(face, richPx, "hello world", render.RGB(255, 255, 255)), con)
		if rich.W != plain.W || rich.H != plain.H {
			t.Errorf("rich %v != plain label %v", rich, plain)
		}
	})

	t.Run("downgraded bold measures like plain", func(t *testing.T) {
		// The documented downgrade: without variants, <b> renders with
		// the base face and cannot change the metrics.
		plain := SizeWithin(NewLabel(face, richPx, "hello world", render.RGB(255, 255, 255)), con)
		rich := SizeWithin(NewRichLabel(face, richPx, "<b>hello</b> world", render.RGB(255, 255, 255)), con)
		if rich.W != plain.W || rich.H != plain.H {
			t.Errorf("downgraded rich %v != plain %v", rich, plain)
		}
	})

	t.Run("styled runs shape with their variant face", func(t *testing.T) {
		bold := boldFace(t)
		l := NewRichLabel(face, richPx, "ab<b>cd</b>", render.RGB(255, 255, 255))
		l.SetVariants(func(b, i bool) *render.Typeface {
			if b {
				return bold
			}
			return face
		})
		want := face.Shape("ab", richPx).Advance() + bold.Shape("cd", richPx).Advance()
		if got := SizeWithin(l, con).W; math.Abs(float64(got)-want) > 1 {
			t.Errorf("width = %d, want %.2f (regular ab + bold cd)", got, want)
		}
	})

	t.Run("empty markup keeps the font line height", func(t *testing.T) {
		plain := SizeWithin(NewLabel(face, richPx, "", render.RGB(255, 255, 255)), con)
		rich := SizeWithin(NewRichLabel(face, richPx, "", render.RGB(255, 255, 255)), con)
		if rich.W != 0 || rich.H != plain.H {
			t.Errorf("empty rich = %v, want width 0 and the label height %v", rich, plain)
		}
	})
}

// TestRichLabelRunSplitting pins run splitting: the per-run caret
// tables concatenate into one monotone line whose run boundaries land
// exactly where the pieces shape on their own.
func TestRichLabelRunSplitting(t *testing.T) {
	face := testFace(t)
	l := NewRichLabel(face, richPx, "ab<b>cd</b>ef", render.RGB(255, 255, 255))
	con := Size{W: 500, H: 100}
	SizeWithin(l, con)
	l.Arrange(render.Rect{X: 4, Y: 2, W: l.natural.W, H: l.natural.H})

	if len(l.shaped) != 3 {
		t.Fatalf("run count = %d, want 3", len(l.shaped))
	}
	for i, r := range l.shaped {
		if want := []int{0, 2, 4}[i]; r.start != want {
			t.Errorf("run %d starts at rune %d, want %d", i, r.start, want)
		}
	}
	xs := caretTable(l)
	if len(xs) != l.runes+1 {
		t.Fatalf("caret boundaries = %d, want %d", len(xs), l.runes+1)
	}
	prev := math.Inf(-1)
	for i, x := range xs {
		if x < prev {
			t.Fatalf("caret %d at %.2f went backwards from %.2f", i, x, prev)
		}
		prev = x
	}
	// Run boundaries coincide with the pieces shaped independently.
	pieces := []string{"ab", "cd", "ef"}
	for i, piece := range pieces {
		bound := face.Shape(piece, richPx).Advance()
		if math.Abs(xs[2*(i+1)]-(bound+xs[2*i])) > 1e-6 {
			t.Errorf("boundary %d at %.3f, want piece advance %.3f on top of %.3f", 2*(i+1), xs[2*(i+1)], bound, xs[2*i])
		}
	}
	// The line's total advance is the sum of the runs'.
	if total := xs[l.runes]; math.Abs(total-l.advance) > 1e-6 {
		t.Errorf("total caret x %.3f != line advance %.3f", total, l.advance)
	}
}

// TestRichLabelClustersAcrossRuns pins that splitting into styled runs
// never splits a shaping cluster: a base rune plus combining mark that
// share a run keep their snapped caret, and the global table stays
// monotone across the run boundary.
func TestRichLabelClustersAcrossRuns(t *testing.T) {
	face := testFace(t)
	l := NewRichLabel(face, richPx, "a\u0301<b>b</b>", render.RGB(255, 255, 255))
	con := Size{W: 500, H: 100}
	SizeWithin(l, con)
	l.Arrange(render.Rect{X: 0, Y: 0, W: l.natural.W, H: l.natural.H})

	if got := l.Text(); got != "a\u0301b" {
		t.Fatalf("Text() = %q, want the three decoded runes", got)
	}
	xs := caretTable(l)
	if len(xs) != 4 {
		t.Fatalf("caret boundaries = %d, want 4 (a, ◌́, b)", len(xs))
	}
	if xs[1] != xs[0] {
		t.Errorf("caret inside the combining cluster at %.3f, snapped is %.3f", xs[1], xs[0])
	}
	for i := 1; i < len(xs); i++ {
		if xs[i] < xs[i-1] {
			t.Fatalf("caret %d at %.3f went backwards", i, xs[i])
		}
	}
	if xs[3] <= xs[2] {
		t.Errorf("boundary after the bold run at %.3f did not advance past %.3f", xs[3], xs[2])
	}
}

// TestRichLabelMixedMetrics pins the mixed-run line box: one Shape per
// run, one shared baseline, and a line height that is the maximum over
// the runs - never their sum, never a single run's metrics.
func TestRichLabelMixedMetrics(t *testing.T) {
	// The glyphs' own metrics, without the theme's line box.
	loadCSS(t, `label { line-height: normal; }`)
	face := testFace(t)
	tall := tallFace(t)
	con := Size{W: 500, H: 100}

	reg, tallSh := face.Shape("yg", richPx), tall.Shape("yg", richPx)
	if tallSh.LineHeight() <= reg.LineHeight() {
		t.Fatalf("tall face line height %d did not exceed %d", tallSh.LineHeight(), reg.LineHeight())
	}

	l := NewRichLabel(face, richPx, "<b>yg</b>yg", render.RGB(255, 255, 255))
	l.SetVariants(func(b, i bool) *render.Typeface {
		if b {
			return tall
		}
		return face
	})
	got := SizeWithin(l, con)
	wantH := int(math.Ceil(math.Max(reg.Ascent(), tallSh.Ascent()) + math.Max(reg.Descent(), tallSh.Descent())))
	if got.H != wantH {
		t.Errorf("mixed line height = %d, want max over runs %d", got.H, wantH)
	}
	if single := SizeWithin(NewLabel(face, richPx, "yg yg", render.RGB(255, 255, 255)), con); got.H <= single.H {
		t.Errorf("mixed height %d did not exceed the single-face height %d", got.H, single.H)
	}
	if sum := reg.LineHeight() + tallSh.LineHeight(); got.H >= sum {
		t.Errorf("mixed height %d must stay below the sum of run heights %d", got.H, sum)
	}

	// Shared baseline: both runs' descenders end on the same rows.
	stride := render.Stride(300)
	data := make([]byte, stride*40)
	cv := render.New(data, stride, 300, 40)
	l.Arrange(render.Rect{X: 2, Y: 4, W: 290, H: got.H})
	l.Paint(cv)
	boldAdv := int(l.shaped[0].sh.Advance() + 0.5)
	bottomOf := func(lo, hi int) int {
		for y := 39; y >= 0; y-- {
			for x := lo; x < hi; x++ {
				if render.ColorFromBytes(data[y*stride+x*4:]).A() > 0 {
					return y
				}
			}
		}
		return -1
	}
	tallBottom := bottomOf(3, 2+boldAdv-1)
	regBottom := bottomOf(2+boldAdv+1, 290)
	if tallBottom < 0 || regBottom < 0 {
		t.Fatalf("no ink under the runs (tall %d, regular %d)", tallBottom, regBottom)
	}
	if math.Abs(float64(tallBottom-regBottom)) > 1 {
		t.Errorf("descender bottoms diverge: tall run %d vs regular run %d - baselines are not shared", tallBottom, regBottom)
	}
}

// TestRichLabelMalformedRendersLiterally pins passthrough: input the
// parser rejects paints as raw text, unmolested.
func TestRichLabelMalformedRendersLiterally(t *testing.T) {
	face := testFace(t)
	con := Size{W: 500, H: 100}
	for _, in := range []string{"<b>hi", "a < b & c", "&nbsp;", "<b><i>deep</b>"} {
		l := NewRichLabel(face, richPx, in, render.RGB(255, 255, 255))
		if got := l.Text(); got != in {
			t.Errorf("Text() = %q, want the literal input %q", got, in)
		}
		if len(l.runs) != 1 || l.runs[0].Style != (TextStyle{}) {
			t.Errorf("%q: runs = %+v, want one plain run", in, l.runs)
		}
		want := face.Shape(in, richPx).Advance()
		if got := SizeWithin(l, con).W; math.Abs(float64(got)-want) > 1 {
			t.Errorf("%q: width %d, want the raw text advance %.2f", in, got, want)
		}
	}
}

// TestRichLabelSelectionBands pins band geometry: bands partition a
// rune range across run boundaries at the caret positions, span the
// full line box, and stay inside the aligned line.
func TestRichLabelSelectionBands(t *testing.T) {
	face := testFace(t)
	l := NewRichLabel(face, richPx, "ab<b>cd</b>ef", render.RGB(255, 255, 255))
	con := Size{W: 500, H: 100}
	size := SizeWithin(l, con)
	box := render.Rect{X: 40, Y: 7, W: l.natural.W + 12, H: size.H}
	l.Arrange(box)

	t.Run("empty and inverted ranges have no bands", func(t *testing.T) {
		for _, r := range [][2]int{{3, 3}, {5, 2}, {-4, 0}, {99, 99}} {
			if bands := l.SelectionBands(r[0], r[1]); len(bands) != 0 {
				t.Errorf("range %v: bands = %v, want none", r, bands)
			}
		}
	})

	t.Run("a range crossing runs splits at the boundaries", func(t *testing.T) {
		bands := l.SelectionBands(1, 5)
		if len(bands) != 3 {
			t.Fatalf("bands = %v, want one per touched run", bands)
		}
		xs := caretTable(l)
		wantW := int(math.Round(xs[5]-xs[1])) - 2 // two boundary roundings
		gotW := 0
		for i, b := range bands {
			if b.Y != box.Y || b.H != box.H {
				t.Errorf("band %d %v does not span the line box", i, b)
			}
			gotW += b.W
			if i > 0 && bands[i-1].X+bands[i-1].W != b.X {
				t.Errorf("band %d leaves a gap before %v", i, b)
			}
		}
		if math.Abs(float64(gotW-wantW)) > 2 {
			t.Errorf("bands cover %d px, want the caret span %.2f ±2", gotW, xs[5]-xs[1])
		}
		if bands[0].X < box.X || bands[2].X+bands[2].W > box.X+box.W {
			t.Errorf("bands %v escape the arranged box", bands)
		}
	})

	t.Run("full range spans the line", func(t *testing.T) {
		bands := l.SelectionBands(0, l.runes)
		gotW := 0
		for _, b := range bands {
			gotW += b.W
		}
		if math.Abs(float64(gotW)-l.advance) > 3 {
			t.Errorf("full-range bands cover %d px, want the line advance %.2f ±3", gotW, l.advance)
		}
	})

	t.Run("alignment moves the bands with the line", func(t *testing.T) {
		l.SetAlignment(render.AlignEnd)
		bands := l.SelectionBands(0, l.runes)
		last := bands[len(bands)-1]
		if last.X+last.W != box.X+box.W {
			t.Errorf("end-aligned last band ends at %d, want the box edge %d", last.X+last.W, box.X+box.W)
		}
		l.SetAlignment(render.AlignStart)
	})
}

// TestRichLabelSelectionPaint pins that the highlight repaints on
// mutation and lands inside the bands.
func TestRichLabelSelectionPaint(t *testing.T) {
	face := testFace(t)
	stride := render.Stride(200)
	data := make([]byte, stride*32)
	cv := render.New(data, stride, 200, 32)

	l := NewRichLabel(face, richPx, "selected", render.RGB(255, 255, 255))
	con := Size{W: 500, H: 100}
	size := SizeWithin(l, con)
	l.Arrange(render.Rect{X: 3, Y: 2, W: size.W, H: size.H})
	l.Paint(cv)
	if ink := countInk(data, stride, 200, 32); ink == 0 {
		t.Fatal("label painted no text")
	}

	l.SetSelection(0, l.runes)
	bands := l.SelectionBands(0, l.runes)
	if len(bands) == 0 {
		t.Fatal("full selection produced no bands")
	}
	cv.ResetTouched()
	l.Paint(cv)
	touched := cv.Touched()
	if touched == 0 {
		t.Fatal("SetSelection did not invalidate: nothing repainted")
	}
	bandArea := 0
	for _, b := range bands {
		bandArea += b.W * b.H
	}
	if touched < bandArea/2 {
		t.Errorf("repaint touched %d px, want at least the band area %d", touched, bandArea)
	}

	l.ClearSelection()
	cv.ResetTouched()
	l.Paint(cv)
	if cv.Touched() == 0 {
		t.Error("ClearSelection did not invalidate")
	}
}

func countInk(data []byte, stride, w, h int) int {
	ink := 0
	for y := range h {
		for x := range w {
			if render.ColorFromBytes(data[y*stride+x*4:]).A() > 0 {
				ink++
			}
		}
	}
	return ink
}

// TestRichLabelDamage pins the #15 contract: mutations invalidate, and
// identical mutations do not.
func TestRichLabelDamage(t *testing.T) {
	face := testFace(t)
	con := Size{W: 500, H: 100}
	l := NewRichLabel(face, richPx, "before", Current().Text)
	root := NewBox(Column, 4, 0).Append(l, false)
	SizeWithin(root, con)
	root.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60})
	warmDamage(root)

	l.SetMarkup("after <b>bold</b>")
	rects, any := CollectDamage(root)
	if !any || len(rects) != 1 || rects[0] != l.Bounds() {
		t.Errorf("SetMarkup damage = %v, want exactly the label bounds", rects)
	}
	if _, warm := CollectDamage(root); warm {
		t.Error("SetMarkup left pending damage behind")
	}

	l.SetMarkup("after <b>bold</b>")
	if _, any := CollectDamage(root); any {
		t.Error("setting the same markup invalidated")
	}

	before := SizeWithin(l, con).W
	l.SetVariants(BaseVariants(face))
	if after := SizeWithin(l, con).W; after != before {
		t.Errorf("downgraded variants changed the width: %d -> %d", before, after)
	}
}

// TestRichLabelLinks pins link geometry, the click callback, and the
// hand cursor.
func TestRichLabelLinks(t *testing.T) {
	face := testFace(t)
	con := Size{W: 500, H: 100}
	l := NewRichLabel(face, richPx, `see <a href="https://example.com">here</a>!`, render.RGB(255, 255, 255))
	size := SizeWithin(l, con)
	l.Arrange(render.Rect{X: 10, Y: 5, W: size.W + 20, H: size.H})
	_, baseline, ok := l.lineGeom()
	if !ok {
		t.Fatal("line does not fit its own natural box")
	}

	var got string
	l.OnLinkClick = func(href string) { got = href }

	linkRun := l.shaped[1]
	if linkRun.style.Href != "https://example.com" {
		t.Fatalf("link run = %+v", linkRun.style)
	}
	midX := 10 + int(l.shaped[0].sh.Advance()+linkRun.sh.Advance()/2+0.5)
	l.ClickAt(Point{X: midX, Y: baseline - 2})
	if got != "https://example.com" {
		t.Errorf("click fired with %q", got)
	}

	got = ""
	l.ClickAt(Point{X: 10, Y: baseline - 2}) // over "see "
	if got != "" {
		t.Errorf("plain text click fired %q", got)
	}

	// Hand cursor over the link, nothing over plain text, nothing
	// after leave.
	l.HoverMove(Point{X: midX, Y: baseline - 2})
	if name := l.CursorName(); name != "hand1" {
		t.Errorf("cursor over link = %q, want hand1", name)
	}
	l.HoverMove(Point{X: 10, Y: baseline - 2})
	if name := l.CursorName(); name != "" {
		t.Errorf("cursor over plain text = %q, want none", name)
	}
	l.SetHovered(false)
	if name := l.CursorName(); name != "" {
		t.Errorf("cursor after leave = %q, want none", name)
	}

	// No callback installed: a click is a no-op, not a panic.
	l.OnLinkClick = nil
	l.ClickAt(Point{X: midX, Y: baseline - 2})
}

func TestRichLabelA11yAndRole(t *testing.T) {
	face := testFace(t)
	l := NewRichLabel(face, richPx, "<b>hi</b>", render.RGB(255, 255, 255))
	if RoleOf(l) != RoleLabel {
		t.Errorf("role = %v, want label", RoleOf(l))
	}
	st := Describe(l)
	if st.Text != "hi" || st.Name != "hi" {
		t.Errorf("a11y state text/name = %q/%q, want the decoded text", st.Text, st.Name)
	}
}

// A <u> run paints a line under its glyphs, an <s> run one through
// them; a plain run neither.
func TestRichLabelDecorationRuns(t *testing.T) {
	face := testFace(t)
	white := render.RGB(255, 255, 255)
	inkRows := func(markup string) map[int]bool {
		l := NewRichLabel(face, richPx, markup, white)
		size := SizeWithin(l, Size{W: 500, H: 100})
		l.Arrange(render.Rect{W: size.W, H: size.H})
		stride := render.Stride(size.W)
		data := make([]byte, stride*size.H)
		cv := render.New(data, stride, size.W, size.H)
		l.Paint(cv)
		rows := map[int]bool{}
		for y := range size.H {
			n := 0
			for x := range size.W {
				if data[y*stride+x*4+3] > 0 {
					n++
				}
			}
			// A decoration spans the run; glyph rows do not.
			if n >= size.W*9/10 {
				rows[y] = true
			}
		}
		return rows
	}
	if rows := inkRows("l l l l l l"); len(rows) != 0 {
		t.Errorf("plain text has full-width rows %v", rows)
	}
	under, through := inkRows("<u>l l l l l l</u>"), inkRows("<s>l l l l l l</s>")
	if len(under) == 0 || len(through) == 0 {
		t.Fatalf("underline rows %v, strike rows %v; want a full-width line each", under, through)
	}
	for y := range under {
		if through[y] {
			t.Errorf("underline and strikethrough share row %d", y)
		}
	}
}

// A RichLabel shapes at the cascade's font-size like a Label, and
// without a rule keeps its constructor size (#256).
func TestRichLabelFollowsTheCSSFontSize(t *testing.T) {
	face := testFace(t)
	measure := func(w Widget) Size {
		host := NewBox(Row, 0, 0)
		host.Append(w, false)
		arrangeTree(t, host, 600, 200)
		return SizeWithin(w, Size{W: 600, H: 200})
	}
	plain := measure(NewRichLabel(face, 30, "Body text", 0))
	if want := measure(NewLabel(face, 30, "Body text", 0)); plain != want {
		t.Errorf("unstyled rich label %v, want the 30px label's %v", plain, want)
	}
	loadCSS(t, `.body { font-size: 12px; }`)
	rich := NewRichLabel(face, 30, "Body text", 0)
	rich.AddClass("body")
	label := NewLabel(face, 12, "Body text", 0)
	if got, want := measure(rich), measure(label); got != want {
		t.Errorf("rich label under font-size 12px %v, want the 12px label's %v", got, want)
	}
}

// A RichLabel measures and paints inside its CSS box, as a Label does:
// padding insets the text, and wraps at the content width (#274).
func TestRichLabelCSSBox(t *testing.T) {
	face := testFace(t)
	loadCSS(t, `.x { padding: 0px 40px; }`)
	firstInk := func(w Widget) int {
		box := NewBox(Column, 0, 0)
		box.Append(w, false)
		buf := paintPixels(t, box, 300, 60)
		for x := range 300 {
			for y := range 60 {
				if pxAt(buf, 300, x, y).A() > 0x40 {
					return x
				}
			}
		}
		return -1
	}
	rl := NewRichLabel(face, 14, "Hello <b>x</b>", render.RGB(0xff, 0xff, 0xff))
	rl.AddClass("x")
	l := NewLabel(face, 14, "Hello x", render.RGB(0xff, 0xff, 0xff))
	l.AddClass("x")
	if r, p := firstInk(rl), firstInk(l); r != p || r < 40 {
		t.Errorf("rich label inks from x=%d, label from x=%d; want both inset by the 40px padding", r, p)
	}
	plain := NewRichLabel(face, 14, "Hello <b>x</b>", 0)
	if got, want := naturalSize(rl).W, naturalSize(plain).W+80; got != want {
		t.Errorf("padded rich label measures %d wide, want %d", got, want)
	}
}
