package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// rectsOf reads the arranged rects of ws.
func rectsOf(ws ...Widget) []render.Rect {
	out := make([]render.Rect, len(ws))
	for i, w := range ws {
		out[i] = boundsOf(w)
	}
	return out
}

// items makes n w x h stubs carrying classes.
func items(n, w, h int, classes ...string) []*stub {
	out := make([]*stub, n)
	for i := range out {
		out[i] = item(w, h, classes...)
	}
	return out
}

// widgets widens stubs to widgets.
func widgets[T Widget](ws []T) []Widget {
	out := make([]Widget, len(ws))
	for i, w := range ws {
		out[i] = w
	}
	return out
}

// gridOf lays kids out in a box of class "g" under css at w x h.
func gridOf(t *testing.T, css string, w, h int, kids ...Widget) *Box {
	t.Helper()
	root := cssBox(Row, "g", kids...)
	cssTree(t, css, root, w, h)
	return root
}

// TestCSSGridFixedFrAutoTracks pins the basic track sizing: a fixed track
// keeps its length, an auto track its content, and the fr tracks share
// what is left by their factors; the lone auto row stretches over the
// definite height (align-content normal) and the items stretch in it.
func TestCSSGridFixedFrAutoTracks(t *testing.T) {
	its := items(4, 30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: 100px 1fr 2fr auto; }`, 400, 200, widgets(its)...)
	if got, want := xs(its...), []int{0, 100, 190, 370}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v: 400 - 100 - 30 leaves 270 for 1fr + 2fr", got, want)
	}
	if got, want := widths(its...), []int{100, 90, 180, 30}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v", got, want)
	}
	if h := its[0].Bounds().H; h != 200 {
		t.Errorf("height %d, want the stretched auto row's 200", h)
	}
}

// TestCSSGridGaps pins row-gap and column-gap between the tracks, a
// percentage gap against the content box, and the interaction rule: an
// undeclared gap keeps the box spacing.
func TestCSSGridGaps(t *testing.T) {
	its := items(6, 30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px 20px; align-content: start; }`,
		340, 200, widgets(its)...)
	if got, want := xs(its[:3]...), []int{0, 120, 240}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v: three 100px columns 20px apart", got, want)
	}
	if got, want := ys(its[0], its[3]), []int{0, 30}; !slices.Equal(got, want) {
		t.Errorf("y %v, want %v: rows 10px apart", got, want)
	}

	pct := items(3, 30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: repeat(3, 1fr); column-gap: 10%; }`, 300, 100, widgets(pct)...)
	if got, want := xs(pct...), []int{0, 110, 220}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v: a 30px gap leaves 80px columns", got, want)
	}

	sp := items(2, 30, 20)
	box := NewBox(Row, 7, 0)
	box.AddClass("g")
	box.Append(sp[0], false).Append(sp[1], false)
	cssTree(t, `.g { display: grid; grid-template-columns: 50px 50px; }`, box, 300, 100)
	if got, want := xs(sp...), []int{0, 57}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v: the box spacing as the column gap", got, want)
	}
}

// TestCSSGridMinmax pins minmax(): the maximized track grows to its
// maximum, and a flexible maximum takes the rest above its minimum.
func TestCSSGridMinmax(t *testing.T) {
	its := items(2, 30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: minmax(100px, 200px) minmax(50px, 1fr); }`, 400, 100, widgets(its)...)
	if got, want := xs(its...), []int{0, 200}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
	if got, want := widths(its...), []int{200, 200}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v", got, want)
	}
}

// TestCSSGridFrContentMinimum pins 1fr as minmax(auto, 1fr): an item wider
// than its share keeps its column at its minimum and the other fr
// columns share the rest; minmax(0, 1fr) splits evenly regardless.
func TestCSSGridFrContentMinimum(t *testing.T) {
	a, b := item(150, 20), item(20, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: 1fr 1fr; }`, 200, 100, a, b)
	if got, want := widths(a, b), []int{150, 50}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v: the 150px item's automatic minimum holds its column", got, want)
	}
	c, d := item(150, 20), item(20, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }`, 200, 100, c, d)
	if got, want := widths(c, d), []int{100, 100}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v", got, want)
	}
}

// TestCSSGridFitAndContentTracks pins fit-content() (the max-content
// contribution clamped to the limit, the min-content one below) and the
// min-content and max-content keywords.
func TestCSSGridFitAndContentTracks(t *testing.T) {
	give := func() *widthGiver { return &widthGiver{nat: Size{W: 100, H: 20}, floor: 20} }
	a, b := give(), give()
	gridOf(t, `.g { display: grid; grid-template-columns: fit-content(50px) fit-content(200px); }`, 400, 100, a, b)
	if got, want := []int{a.Bounds().X, b.Bounds().X, a.Bounds().W, b.Bounds().W}, []int{0, 50, 50, 100}; !slices.Equal(got, want) {
		t.Errorf("x, x, w, w = %v, want %v", got, want)
	}
	c, d := give(), give()
	gridOf(t, `.g { display: grid; grid-template-columns: min-content max-content; }`, 400, 100, c, d)
	if got, want := []int{c.Bounds().W, d.Bounds().X, d.Bounds().W}, []int{20, 20, 100}; !slices.Equal(got, want) {
		t.Errorf("w, x, w = %v, want %v", got, want)
	}
}

// TestCSSGridAutoRepeat pins auto-fill and auto-fit: as many repetitions
// as fit the width with the gaps, and auto-fit collapsing the empty
// ones, which justify-content then centers around.
func TestCSSGridAutoRepeat(t *testing.T) {
	fill := items(2, 30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: repeat(auto-fill, 100px); justify-content: center; }`, 350, 100, widgets(fill)...)
	if got, want := xs(fill...), []int{25, 125}; !slices.Equal(got, want) {
		t.Errorf("auto-fill x %v, want %v: three 100px tracks centered in 350", got, want)
	}
	fit := items(2, 30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: repeat(auto-fit, 100px); justify-content: center; }`, 350, 100, widgets(fit)...)
	if got, want := xs(fit...), []int{75, 175}; !slices.Equal(got, want) {
		t.Errorf("auto-fit x %v, want %v: the empty third track collapses", got, want)
	}
	gapped := items(4, 30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: repeat(auto-fill, 100px); gap: 10px; align-content: start; }`, 330, 100, widgets(gapped)...)
	if got, want := xs(gapped...), []int{0, 110, 220, 0}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v: 3 x 100 + 2 x 10 fits 330", got, want)
	}
	if y := gapped[3].Bounds().Y; y != 30 {
		t.Errorf("fourth item y %d, want 30 on the second row", y)
	}
}

// TestCSSGridTemplateAreas pins named areas: grid-area places each item on
// its area's rectangle whatever the document order.
func TestCSSGridTemplateAreas(t *testing.T) {
	h, n, m, f := item(30, 20, "h"), item(30, 20, "n"), item(30, 20, "m"), item(30, 20, "f")
	gridOf(t, `.g { display: grid; grid-template-columns: 100px 1fr; grid-template-rows: 30px 1fr 20px;
			grid-template-areas: "head head" "nav main" "foot foot"; }
		.h { grid-area: head; } .n { grid-area: nav; } .m { grid-area: main; } .f { grid-area: foot; }`,
		300, 200, f, m, n, h)
	want := []render.Rect{{W: 300, H: 30}, {Y: 30, W: 100, H: 150}, {X: 100, Y: 30, W: 200, H: 150}, {Y: 180, W: 300, H: 20}}
	if got := rectsOf(h, n, m, f); !slices.Equal(got, want) {
		t.Errorf("areas %v, want %v", got, want)
	}
}

// TestCSSGridNamedLines pins placement by line names: a lone name, a name
// with a count from either end, and a span to a named line.
func TestCSSGridNamedLines(t *testing.T) {
	p, q, r := item(10, 10, "p"), item(10, 10, "q"), item(10, 10, "r")
	gridOf(t, `.g { display: grid; grid-template-columns: [a] 50px [b c] 60px [d] 70px [e]; justify-content: start; }
		.p { grid-column: b / e; } .q { grid-column: a / d; } .r { grid-column: span d / e; }`, 300, 100, p, q, r)
	if got, want := [][2]int{{p.Bounds().X, p.Bounds().W}, {q.Bounds().X, q.Bounds().W}, {r.Bounds().X, r.Bounds().W}},
		[][2]int{{50, 130}, {0, 110}, {110, 70}}; !slices.Equal(got, want) {
		t.Errorf("x, w %v, want %v", got, want)
	}
	s := item(10, 10, "s")
	gridOf(t, `.g { display: grid; grid-template-columns: [x] 50px [x] 60px [x] 70px [x]; }
		.s { grid-column: 2 x / -2 x; }`, 300, 100, s)
	if got := [2]int{s.Bounds().X, s.Bounds().W}; got != [2]int{50, 60} {
		t.Errorf("x, w %v, want [50 60]: the second x line to the second from the end", got)
	}
}

// TestCSSGridNamedSpanPastTheGrid pins a named span searching back from
// a line past the explicit grid: the implicit lines behind the search
// carry no name, so it reaches the explicit line so called. Regression:
// the search stopped at once and took an implicit line before the grid.
func TestCSSGridNamedSpanPastTheGrid(t *testing.T) {
	s, k := item(30, 20, "s"), item(30, 20, "k")
	gridOf(t, `.g { display: grid; grid-template-columns: [a] 50px [b] 50px; grid-auto-columns: 25px; }
		.s { grid-column: span a / 5; } .k { grid-column: 1; }`, 400, 100, s, k)
	if got, want := []int{s.Bounds().X, s.Bounds().W, k.Bounds().X}, []int{0, 150, 0}; !slices.Equal(got, want) {
		t.Errorf("span x, w, first column x = %v, want %v", got, want)
	}
}

// TestCSSGridNegativeLines pins negative line numbers from the end of the
// explicit grid, and an item before it adding an implicit track there.
func TestCSSGridNegativeLines(t *testing.T) {
	a, b := item(30, 20, "a"), item(30, 20, "b")
	gridOf(t, `.g { display: grid; grid-template-columns: 100px 100px 100px; justify-content: start; }
		.a { grid-column: -2 / -1; } .b { grid-column: 1 / -1; }`, 400, 100, a, b)
	if got, want := [][2]int{{a.Bounds().X, a.Bounds().W}, {b.Bounds().X, b.Bounds().W}}, [][2]int{{200, 100}, {0, 300}}; !slices.Equal(got, want) {
		t.Errorf("x, w %v, want %v", got, want)
	}
	c, d := item(30, 20, "c"), item(30, 20, "d")
	gridOf(t, `.g { display: grid; grid-template-columns: 100px 100px 100px; justify-content: start; }
		.c { grid-column: -5 / -4; } .d { grid-column: 1; }`, 400, 100, c, d)
	if got, want := [][2]int{{c.Bounds().X, c.Bounds().W}, {d.Bounds().X, d.Bounds().W}}, [][2]int{{0, 30}, {30, 100}}; !slices.Equal(got, want) {
		t.Errorf("x, w %v, want %v: an auto track before the explicit grid", got, want)
	}
}

// TestCSSGridImplicitTracks pins grid-auto-columns and grid-auto-rows: the
// first implicit track after the explicit grid takes the first size,
// the last one before it the last size, the pattern repeating; and
// grid-auto-flow: column growing columns.
func TestCSSGridImplicitTracks(t *testing.T) {
	a, b, c := item(5, 5, "a"), item(5, 5, "b"), item(5, 5, "c")
	gridOf(t, `.g { display: grid; grid-template-columns: 100px; grid-auto-columns: 10px 20px 30px; }
		.a { grid-column: -3; } .b { grid-column: -4 / -3; } .c { grid-column: 3; }`, 400, 100, a, b, c)
	if got, want := [][2]int{{b.Bounds().X, b.Bounds().W}, {a.Bounds().X, a.Bounds().W}, {c.Bounds().X, c.Bounds().W}},
		[][2]int{{0, 20}, {20, 30}, {160, 20}}; !slices.Equal(got, want) {
		t.Errorf("x, w %v, want %v: tracks 20 30 | 100 | 10 20", got, want)
	}
	rows := items(6, 30, 5)
	gridOf(t, `.g { display: grid; grid-template-columns: 50px 50px; grid-auto-rows: 10px 20px; align-content: start; }`, 300, 200, widgets(rows)...)
	if got, want := ys(rows...), []int{0, 0, 10, 10, 30, 30}; !slices.Equal(got, want) {
		t.Errorf("y %v, want %v: rows 10 20 10", got, want)
	}
	cols := items(5, 30, 5)
	gridOf(t, `.g { display: grid; grid-template-rows: 20px 20px; grid-auto-flow: column; grid-auto-columns: 40px;
		justify-content: start; align-content: start; }`, 300, 200, widgets(cols)...)
	if got, want := xs(cols...), []int{0, 0, 40, 40, 80}; !slices.Equal(got, want) {
		t.Errorf("column flow x %v, want %v", got, want)
	}
	if got, want := ys(cols...), []int{0, 20, 0, 20, 0}; !slices.Equal(got, want) {
		t.Errorf("column flow y %v, want %v", got, want)
	}
}

// TestCSSGridAutoPlacement pins §8.5: sparse placement never backtracks
// past a placed item, dense fills the first hole; items locked to a row
// fill it in order, and a column-fixed item searches down from the
// cursor.
func TestCSSGridAutoPlacement(t *testing.T) {
	const base = `.g { display: grid; grid-template-columns: repeat(3, 100px); align-content: start; }
		.s2 { grid-column: span 2; } `
	for _, c := range []struct {
		flow string
		want render.Rect
	}{
		{"row", render.Rect{X: 200, Y: 20, W: 100, H: 20}},
		{"row dense", render.Rect{X: 200, W: 100, H: 20}},
	} {
		a, b, s := item(30, 20, "s2"), item(30, 20, "s2"), item(30, 20)
		gridOf(t, base+`.g { grid-auto-flow: `+c.flow+`; }`, 300, 200, a, b, s)
		if b.Bounds().Y != 20 || a.Bounds().W != 200 {
			t.Errorf("%s: the second span-2 item at %v, want the second row", c.flow, b.Bounds())
		}
		if got := s.Bounds(); got != c.want {
			t.Errorf("%s: the single item at %v, want %v", c.flow, got, c.want)
		}
	}
	d, e, f, g := item(30, 20, "r2"), item(30, 20, "r2"), item(30, 20), item(30, 20, "c3")
	gridOf(t, base+`.r2 { grid-row: 2; } .c3 { grid-column: 3; }`, 300, 200, d, e, f, g)
	want := []render.Rect{{Y: 20, W: 100, H: 20}, {X: 100, Y: 20, W: 100, H: 20}, {W: 100, H: 20}, {X: 200, W: 100, H: 20}}
	if got := rectsOf(d, e, f, g); !slices.Equal(got, want) {
		t.Errorf("locked and column-fixed items %v, want %v", got, want)
	}
}

// TestCSSGridSpanningRows pins a spanning item's contribution (§11.5): what
// it needs beyond the rows its single-row neighbors sized is shared
// equally between the rows it spans.
func TestCSSGridSpanningRows(t *testing.T) {
	a, b, c := item(30, 50, "a"), item(30, 20), item(30, 20)
	gridOf(t, `.g { display: grid; grid-template-columns: 100px 100px; align-content: start; } .a { grid-row: span 2; }`, 300, 200, a, b, c)
	if got, want := []int{b.Bounds().H, c.Bounds().Y, a.Bounds().H}, []int{25, 25, 50}; !slices.Equal(got, want) {
		t.Errorf("row 1 height, row 2 y, span height = %v, want %v", got, want)
	}
}

// TestCSSGridSpanningColumnsIntrinsic pins the spanning distribution past
// the growth limits: an item spanning two auto columns grows both
// equally beyond their single items' sizes, and the grid's max-content
// width is the spanning item's.
func TestCSSGridSpanningColumnsIntrinsic(t *testing.T) {
	a, b, c := item(200, 20, "a"), item(50, 20), item(30, 20)
	root := gridOf(t, `.g { display: grid; grid-template-columns: auto auto; justify-content: start; } .a { grid-column: span 2; }`,
		400, 100, a, b, c)
	if got, want := []int{b.Bounds().W, c.Bounds().X, c.Bounds().W, a.Bounds().W}, []int{110, 110, 90, 200}; !slices.Equal(got, want) {
		t.Errorf("b w, c x, c w, a w = %v, want %v: 120px shared over 50 + 30", got, want)
	}
	if w := root.Measure(Constraints{Max: Size{W: indefinite, H: indefinite}}).W; w != 200 {
		t.Errorf("max-content width %d, want 200", w)
	}
}

// TestCSSGridMeasure pins the intrinsic size: fr tracks under a
// max-content constraint take the largest fr their items ask for, and
// an auto-fill without a definite width repeats once.
func TestCSSGridMeasure(t *testing.T) {
	loadCSS(t, `.g { display: grid; grid-template-columns: 1fr 2fr; } .f { display: grid; grid-template-columns: repeat(auto-fill, 40px); }`)
	g := cssBox(Row, "g", item(30, 20), item(30, 20))
	if got := g.Measure(Constraints{Max: Size{W: indefinite, H: indefinite}}); got != (Size{W: 90, H: 20}) {
		t.Errorf("fr grid measures %v, want 90x20: 1fr is 30px", got)
	}
	f := cssBox(Row, "f", item(30, 20), item(30, 20))
	if got := f.Measure(Constraints{Max: Size{W: indefinite, H: indefinite}}); got != (Size{W: 40, H: 40}) {
		t.Errorf("auto-fill grid measures %v, want one 40px column, two rows", got)
	}
}

// TestCSSGridSelfAlignment pins every justify-self and align-self keyword
// in a 100x60 area, auto margins, the container's justify-items and
// place-items, place-self, an aspect-ratio item's normal, and the RTL
// readings (the column itself on the right, at 200) and an item's own
// direction.
func TestCSSGridSelfAlignment(t *testing.T) {
	const base = `.g { display: grid; grid-template-columns: 100px; grid-template-rows: 60px; } `
	for _, c := range []struct {
		css  string
		w    int
		want render.Rect
	}{
		{".i { justify-self: start; align-self: start; }", 30, render.Rect{W: 30, H: 20}},
		{".i { justify-self: end; align-self: end; }", 30, render.Rect{X: 70, Y: 40, W: 30, H: 20}},
		{".i { justify-self: center; align-self: center; }", 30, render.Rect{X: 35, Y: 20, W: 30, H: 20}},
		{".i { justify-self: left; align-self: self-end; }", 30, render.Rect{Y: 40, W: 30, H: 20}},
		{".i { justify-self: right; align-self: self-start; }", 30, render.Rect{X: 70, W: 30, H: 20}},
		{".i { justify-self: self-start; align-self: flex-end; }", 30, render.Rect{Y: 40, W: 30, H: 20}},
		{".i { justify-self: self-end; align-self: flex-start; }", 30, render.Rect{X: 70, W: 30, H: 20}},
		{".i { justify-self: flex-start; align-self: stretch; }", 30, render.Rect{W: 30, H: 60}},
		{".i { justify-self: flex-end; align-self: normal; }", 30, render.Rect{X: 70, W: 30, H: 60}},
		{".i { justify-self: stretch; align-self: start; }", 30, render.Rect{W: 100, H: 20}},
		{".i { justify-self: normal; align-self: start; }", 30, render.Rect{W: 100, H: 20}},
		{".i { justify-self: baseline; align-self: start; }", 30, render.Rect{W: 30, H: 20}},
		{".i { justify-self: last baseline; align-self: start; }", 30, render.Rect{X: 70, W: 30, H: 20}},
		{".i { justify-self: safe center; align-self: start; }", 130, render.Rect{W: 130, H: 20}},
		{".i { justify-self: unsafe center; align-self: start; }", 130, render.Rect{X: -15, W: 130, H: 20}},
		{".i { justify-self: center; align-self: start; }", 130, render.Rect{X: -15, W: 130, H: 20}},
		{".i { margin-left: auto; align-self: start; }", 30, render.Rect{X: 70, W: 30, H: 20}},
		{".i { margin: auto; }", 30, render.Rect{X: 35, Y: 20, W: 30, H: 20}},
		{".g { justify-items: center; } .i { align-self: start; }", 30, render.Rect{X: 35, W: 30, H: 20}},
		{".g { justify-items: center; } .i { justify-self: end; align-self: start; }", 30, render.Rect{X: 70, W: 30, H: 20}},
		{".g { place-items: center end; }", 30, render.Rect{X: 70, Y: 20, W: 30, H: 20}},
		{".i { place-self: end center; }", 30, render.Rect{X: 35, Y: 40, W: 30, H: 20}},
		{".i { aspect-ratio: 1; }", 30, render.Rect{W: 30, H: 30}},
		{".g { direction: rtl; } .i { justify-self: start; align-self: start; }", 30, render.Rect{X: 270, W: 30, H: 20}},
		{".g { direction: rtl; } .i { justify-self: end; align-self: start; }", 30, render.Rect{X: 200, W: 30, H: 20}},
		{".g { direction: rtl; } .i { justify-self: left; align-self: start; }", 30, render.Rect{X: 200, W: 30, H: 20}},
		{".g { direction: rtl; } .i { justify-self: right; align-self: start; }", 30, render.Rect{X: 270, W: 30, H: 20}},
		{".i { direction: rtl; justify-self: self-start; align-self: start; }", 30, render.Rect{X: 70, W: 30, H: 20}},
	} {
		it := item(c.w, 20, "i")
		gridOf(t, base+c.css, 300, 100, it)
		if got := it.Bounds(); got != c.want {
			t.Errorf("%s: %v, want %v", c.css, got, c.want)
		}
	}
}

// TestCSSGridBaselineAlignment pins baseline-sharing groups: a row's
// baseline-aligned items line their first (or last) baselines up, a
// widget without one on its bottom edge, the shims growing the row;
// the grid reports the shared first baseline as its own.
func TestCSSGridBaselineAlignment(t *testing.T) {
	mk := func(h, base int) *baseStub { return &baseStub{nat: Size{W: 30, H: h}, base: base} }
	a, b, c := mk(20, 15), mk(30, 5), item(30, 10)
	root := gridOf(t, `.g { display: grid; grid-template-columns: 50px 50px 50px; align-items: baseline; align-content: start; }`,
		300, 100, a, b, c, item(30, 10))
	if got, want := []int{a.Bounds().Y, b.Bounds().Y, c.Bounds().Y}, []int{0, 10, 5}; !slices.Equal(got, want) {
		t.Errorf("y %v, want %v: baselines on 15", got, want)
	}
	if base, ok := root.Baseline(); !ok || base != 15 {
		t.Errorf("grid baseline %d, %v, want 15", base, ok)
	}
	if y := boundsOf(root.child[3].w).Y; y != 40 {
		t.Errorf("second row at %d, want 40: the shimmed item is 40 tall", y)
	}
	d, e := mk(20, 15), mk(30, 5)
	gridOf(t, `.g { display: grid; grid-template-columns: 50px 50px; align-items: last baseline; align-content: start; }`, 300, 100, d, e)
	if got, want := []int{d.Bounds().Y, e.Bounds().Y}, []int{0, 10}; !slices.Equal(got, want) {
		t.Errorf("last baseline y %v, want %v", got, want)
	}
}

// TestCSSGridContentAlignment pins justify-content and align-content over
// the tracks: every keyword, safe and unsafe overflow, stretch of auto
// tracks, and RTL.
func TestCSSGridContentAlignment(t *testing.T) {
	for _, c := range []struct {
		css  string
		want []int
	}{
		{"justify-content: start", []int{0, 50}},
		{"justify-content: end", []int{200, 250}},
		{"justify-content: center", []int{100, 150}},
		{"justify-content: space-between", []int{0, 250}},
		{"justify-content: space-around", []int{50, 200}},
		{"justify-content: space-evenly", []int{67, 183}},
		{"justify-content: stretch", []int{0, 50}},
		{"justify-content: left", []int{0, 50}},
		{"justify-content: right", []int{200, 250}},
		{"justify-content: flex-end", []int{200, 250}},
		{"direction: rtl; justify-content: start", []int{250, 200}},
		{"direction: rtl; justify-content: left", []int{50, 0}},
		{"grid-template-columns: 200px 200px; justify-content: safe center", []int{0, 200}},
		{"grid-template-columns: 200px 200px; justify-content: center", []int{-50, 150}},
		{"grid-template-columns: auto auto; justify-content: stretch", []int{0, 150}},
		{"grid-template-columns: auto auto; justify-content: start", []int{0, 50}},
	} {
		a, b := item(50, 20), item(50, 20)
		gridOf(t, `.g { display: grid; grid-template-columns: 50px 50px; `+c.css+`; }`, 300, 100, a, b)
		if got := xs(a, b); !slices.Equal(got, c.want) {
			t.Errorf("%s: x %v, want %v", c.css, got, c.want)
		}
	}
	for _, c := range []struct {
		css  string
		want []int
	}{
		{"align-content: center", []int{30, 50}},
		{"align-content: space-between", []int{0, 80}},
		{"align-content: end", []int{60, 80}},
	} {
		a, b := item(50, 20), item(50, 20)
		gridOf(t, `.g { display: grid; grid-template-rows: 20px 20px; `+c.css+`; }`, 300, 100, a, b)
		if got := ys(a, b); !slices.Equal(got, c.want) {
			t.Errorf("%s: y %v, want %v", c.css, got, c.want)
		}
	}
}

// TestCSSGridOrder pins order: auto-placement and paint follow the
// order-modified document order, and z-index orders static grid items.
func TestCSSGridOrder(t *testing.T) {
	var log []string
	mk := func(name, class string) *paintLog {
		p := &paintLog{nat: Size{W: 50, H: 20}, name: name, log: &log}
		p.AddClass(class)
		return p
	}
	a, b, c, z := mk("a", "o2"), mk("b", "o0"), mk("c", "o1"), mk("z", "under")
	root := gridOf(t, `.g { display: grid; grid-template-columns: repeat(4, 100px); }
		.o2 { order: 2; } .o1 { order: 1; } .under { order: 3; z-index: -1; }`, 400, 100, a, b, c, z)
	if got, want := []int{a.Bounds().X, b.Bounds().X, c.Bounds().X, z.Bounds().X}, []int{200, 0, 100, 300}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
	paintOnce(root, 400, 100)
	if want := []string{"z", "b", "c", "a"}; !slices.Equal(log, want) {
		t.Errorf("paint order %v, want %v", log, want)
	}
}

// TestCSSGridAbsoluteItems pins absolutely positioned children of a
// positioned grid: the grid area their lines name is the containing
// block, an auto line standing for the padding edge, and a line outside
// the grid reading as auto.
func TestCSSGridAbsoluteItems(t *testing.T) {
	in1, in2 := item(30, 20), item(30, 20)
	a1, a2, a3 := item(30, 20, "a1"), item(30, 20, "a2"), item(30, 20, "a3")
	gridOf(t, `.g { display: grid; position: relative; padding: 10px; grid-template-columns: 100px 100px;
			grid-template-rows: 50px 50px; }
		.a1 { position: absolute; grid-column: 2 / 3; grid-row: 1 / 3; inset: 0; }
		.a2 { position: absolute; grid-column: auto / 2; grid-row: 2; inset: 0; }
		.a3 { position: absolute; grid-column: 5; top: 0; left: 0; }`, 300, 200, in1, a1, in2, a2, a3)
	want := []render.Rect{{X: 110, Y: 10, W: 100, H: 100}, {Y: 60, W: 110, H: 140}, {W: 30, H: 20}}
	if got := rectsOf(a1, a2, a3); !slices.Equal(got, want) {
		t.Errorf("absolute items %v, want %v", got, want)
	}
	if got, want := rectsOf(in1, in2), []render.Rect{{X: 10, Y: 10, W: 100, H: 50}, {X: 110, Y: 10, W: 100, H: 50}}; !slices.Equal(got, want) {
		t.Errorf("in-flow items %v, want %v: the absolute ones take no cell", got, want)
	}
	r1, r2 := item(30, 20, "r1"), item(30, 20, "r2")
	gridOf(t, `.g { display: grid; direction: rtl; position: relative; padding: 10px; grid-template-columns: 100px 100px; }
		.r1 { position: absolute; grid-column: 1 / 2; inset: 0; } .r2 { position: absolute; grid-column: 2; inset: 0; }`,
		300, 200, r1, r2)
	if got, want := [][2]int{{r1.Bounds().X, r1.Bounds().W}, {r2.Bounds().X, r2.Bounds().W}}, [][2]int{{190, 100}, {0, 190}}; !slices.Equal(got, want) {
		t.Errorf("RTL absolute x, w %v, want %v: column 1 on the right, an auto end at the left padding edge", got, want)
	}
}

// TestCSSGridRTL pins an RTL grid: the first column on the right.
func TestCSSGridRTL(t *testing.T) {
	its := items(2, 30, 20)
	gridOf(t, `.g { display: grid; direction: rtl; grid-template-columns: 100px 100px; }`, 300, 100, widgets(its)...)
	if got, want := xs(its...), []int{200, 100}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
}

// TestCSSGridItemRestyleRelayouts pins the measure cache: a class change
// moving an item to another line relayouts the grid.
func TestCSSGridItemRestyleRelayouts(t *testing.T) {
	a := item(30, 20, "c1")
	root := gridOf(t, `.g { display: grid; grid-template-columns: repeat(3, 100px); } .c1 { grid-column: 1; } .c3 { grid-column: 3; }`, 300, 100, a)
	a.RemoveClass("c1")
	a.AddClass("c3")
	settle(t, root, 300, 100)
	if x := a.Bounds().X; x != 200 {
		t.Errorf("x %d after the restyle, want 200", x)
	}
}

// TestCSSGridContainerRestyleRelayouts pins the layout key on the
// container: a class switching its track list relayouts, and switching
// back to an equal list (interned to the same value) lays out as before.
func TestCSSGridContainerRestyleRelayouts(t *testing.T) {
	a, b := item(30, 20), item(30, 20)
	root := gridOf(t, `.g { display: grid; grid-template-columns: 100px 100px; } .g.wide { grid-template-columns: 200px 100px; }`, 400, 100, a, b)
	root.AddClass("wide")
	settle(t, root, 400, 100)
	if x := b.Bounds().X; x != 200 {
		t.Errorf("x %d after widening, want 200", x)
	}
	root.RemoveClass("wide")
	settle(t, root, 400, 100)
	if x := b.Bounds().X; x != 100 {
		t.Errorf("x %d after narrowing back, want 100", x)
	}
}

// TestCSSGridShorthands pins grid-template's area form and grid's
// auto-flow forms driving the layout.
func TestCSSGridShorthands(t *testing.T) {
	h, m := item(30, 20, "h"), item(30, 20, "m")
	gridOf(t, `.g { display: grid; grid-template: "h h" 40px "s m" 60px / 70px 1fr; }
		.h { grid-area: h; } .m { grid-area: m; }`, 300, 100, m, h)
	if got, want := rectsOf(h, m), []render.Rect{{W: 300, H: 40}, {X: 70, Y: 40, W: 230, H: 60}}; !slices.Equal(got, want) {
		t.Errorf("grid-template areas %v, want %v", got, want)
	}
	its := items(3, 30, 20)
	gridOf(t, `.g { display: grid; grid: 25px 25px / auto-flow 60px; justify-content: start; }`, 300, 100, widgets(its)...)
	if got, want := [][2]int{{its[0].Bounds().X, its[0].Bounds().Y}, {its[1].Bounds().X, its[1].Bounds().Y}, {its[2].Bounds().X, its[2].Bounds().Y}},
		[][2]int{{0, 0}, {0, 25}, {60, 0}}; !slices.Equal(got, want) {
		t.Errorf("grid auto-flow columns %v, want %v", got, want)
	}
}

// TestCSSGridAutoFillFromMaxSize pins the repetition count against a
// container whose extent is indefinite but whose max size is definite:
// as many repetitions as fit max-height.
func TestCSSGridAutoFillFromMaxSize(t *testing.T) {
	its := items(4, 30, 10)
	g := cssBox(Row, "g", widgets(its)...)
	cssTree(t, `.g { display: grid; max-height: 70px; grid-template-rows: repeat(auto-fill, 20px); grid-auto-flow: column;
		grid-auto-columns: 50px; }`, cssBox(Column, "", g), 300, 300)
	if got, want := []int{g.Bounds().H, its[2].Bounds().Y, its[3].Bounds().X}, []int{60, 40, 50}; !slices.Equal(got, want) {
		t.Errorf("grid h, third y, fourth x = %v, want %v: three 20px rows fit 70px", got, want)
	}
}

// TestCSSGridShrinkableHeight pins the grid's min-content height: a
// Scroll 132 tall that can give 100 sets its auto row's floor at 32,
// above its 20px neighbor, so the grid can give 100 too.
func TestCSSGridShrinkableHeight(t *testing.T) {
	sc := propagating(NewScroll(newStub(50, 132)))
	g := cssBox(Row, "g", sc, item(30, 20))
	cssTree(t, `.g { display: grid; grid-template-columns: 100px 100px; }`, cssBox(Column, "", g), 300, 300)
	g.Measure(Constraints{Max: Size{W: 300, H: indefinite}})
	if got := []int{shrinkableOf(sc), g.Shrinkable()}; !slices.Equal(got, []int{100, 100}) {
		t.Errorf("scroll and grid shrinkable %v, want [100 100]", got)
	}
}

// TestCSSGridHiddenAndContentsChildren pins the items a grid places: a
// display:none child takes no cell, and a display:contents box's
// children are grid items of their own.
func TestCSSGridHiddenAndContentsChildren(t *testing.T) {
	gone, c1, c2, last := item(30, 20, "none"), item(30, 20), item(30, 20), item(30, 20)
	contents := cssBox(Row, "contents", c1, c2)
	gridOf(t, `.g { display: grid; grid-template-columns: 100px 100px; align-content: start; } .none { display: none; }
		.contents { display: contents; }`, 300, 100, gone, contents, last)
	want := []render.Rect{{W: 100, H: 20}, {X: 100, W: 100, H: 20}, {Y: 20, W: 100, H: 20}}
	if got := rectsOf(c1, c2, last); !slices.Equal(got, want) {
		t.Errorf("items %v, want %v", got, want)
	}
}

// TestCSSGridInlineAndMinContent pins the grid container's own sizing: an
// inline-grid in flow shrinks to its max-content width where a block
// grid fills the line, and width: min-content reads the grid's
// min-content width from its columns.
func TestCSSGridInlineAndMinContent(t *testing.T) {
	inline := cssBox(Row, "ig", item(30, 20), item(30, 20))
	block := cssBox(Row, "bg", item(30, 20), item(30, 20))
	cssTree(t, `.p { display: block; } .ig { display: inline-grid; grid-template-columns: 1fr 1fr; }
		.bg { display: grid; grid-template-columns: 1fr 1fr; }`, cssBox(Column, "p", inline, block), 300, 200)
	if got, want := []int{inline.Bounds().W, block.Bounds().W}, []int{60, 300}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v", got, want)
	}

	give := &widthGiver{nat: Size{W: 100, H: 20}, floor: 40}
	other := item(30, 20)
	g := cssBox(Row, "g", give, other)
	cssTree(t, `.g { display: grid; width: min-content; grid-template-columns: auto auto; }`, cssBox(Column, "", g), 300, 200)
	if got, want := []int{g.Bounds().W, give.Bounds().W, other.Bounds().X}, []int{70, 40, 40}; !slices.Equal(got, want) {
		t.Errorf("grid w, first w, second x = %v, want %v", got, want)
	}
	g.Measure(Constraints{Max: Size{W: indefinite, H: indefinite}})
	if s := g.ShrinkableWidth(); s != 60 {
		t.Errorf("ShrinkableWidth %d, want 130 - 70", s)
	}
}

// TestCSSGridSteadyFrameAllocatesNothing pins the grid buffers: a static
// grid tree — areas, named lines, auto-fill, spans, dense packing,
// baselines, an absolute item and a nested flexbox — measures and
// arranges without allocating once the per-depth buffers have grown.
func TestCSSGridSteadyFrameAllocatesNothing(t *testing.T) {
	nested := cssBox(Row, "f", item(20, 10), item(20, 10))
	root := cssBox(Column, "g", item(30, 20, "h"), item(30, 20, "s2"), &baseStub{nat: Size{W: 30, H: 20}, base: 12},
		nested, item(30, 20, "abs"), item(30, 20), item(30, 20, "n"))
	inner := cssBox(Row, "fill", item(30, 20), item(30, 20), item(30, 20))
	root.Append(inner, false)
	cssTree(t, `.g { display: grid; position: relative; gap: 4px; grid-auto-flow: dense; align-items: baseline;
			grid-template-columns: [a] 80px [b] 1fr [c] minmax(40px, 2fr) [d];
			grid-template-areas: "head head head"; grid-auto-rows: minmax(20px, auto); }
		.h { grid-area: head; } .s2 { grid-column: span 2; } .n { grid-column: b / d; }
		.abs { position: absolute; grid-area: 1 / 2 / 3 / 3; inset: 0; } .f { display: flex; }
		.fill { display: grid; grid-template-columns: repeat(auto-fit, minmax(50px, 1fr)); grid-column: 1 / -1; }`, root, 400, 300)
	frame := func() {
		styleGen++
		root.Measure(Constraints{Max: Size{W: 400, H: 300}})
		root.Arrange(render.Rect{W: 400, H: 300})
	}
	frame()
	if allocs := testing.AllocsPerRun(20, frame); allocs != 0 {
		t.Errorf("grid layout frame allocates %v allocs/op, want 0", allocs)
	}
}
