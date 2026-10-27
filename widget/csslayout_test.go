package widget

import (
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// cssTree loads css, then measures and arranges root in a w x h rect
// until the layout settles, as the frame loop does.
func cssTree(t *testing.T, css string, root Widget, w, h int) {
	t.Helper()
	loadCSS(t, css)
	settle(t, root, w, h)
}

// settle measures and arranges root until no layout is pending.
func settle(t *testing.T, root Widget, w, h int) {
	t.Helper()
	for range 6 {
		root.Measure(Constraints{Max: Size{W: w, H: h}})
		root.Arrange(render.Rect{W: w, H: h})
		if !LayoutPending(root) {
			return
		}
	}
	t.Fatal("layout did not settle")
}

// item is a w x h stub carrying classes for the stylesheet.
func item(w, h int, classes ...string) *stub {
	s := newStub(w, h)
	s.AddClass(classes...)
	return s
}

// cssBox is a box carrying classes, holding kids.
func cssBox(axis Axis, classes string, kids ...Widget) *Box {
	b := NewBox(axis, 0, 0)
	b.AddClass(strings.Fields(classes)...)
	for _, k := range kids {
		b.Append(k, false)
	}
	return b
}

// xs, ys and ws read the arranged rects of ws.
func xs(ws ...*stub) []int {
	out := make([]int, len(ws))
	for i, w := range ws {
		out[i] = w.Bounds().X
	}
	return out
}

func ys(ws ...*stub) []int {
	out := make([]int, len(ws))
	for i, w := range ws {
		out[i] = w.Bounds().Y
	}
	return out
}

func widths(ws ...*stub) []int {
	out := make([]int, len(ws))
	for i, w := range ws {
		out[i] = w.Bounds().W
	}
	return out
}

// baseStub is a stub reporting a text baseline.
type baseStub struct {
	stub
	base int
}

func (b *baseStub) Baseline() (int, bool) { return b.base, true }

// paintLog is a stub recording its paints into a shared log.
type paintLog struct {
	stub
	name string
	log  *[]string
}

func (p *paintLog) Paint(*render.Canvas) { *p.log = append(*p.log, p.name) }

func (p *paintLog) HitTest(pt Point) Widget { return p.HitLeaf(p, pt) }

// paintOnce paints root once onto a scratch canvas.
func paintOnce(root Widget, w, h int) {
	cv := render.New(make([]byte, render.Stride(w)*h), render.Stride(w), w, h)
	root.Paint(cv)
}

// TestFlexGrowDistributesFreeSpace pins §9.7's growth: free space goes
// to the items in proportion to flex-grow, and a single-line
// container's definite cross size stretches them.
func TestFlexGrowDistributesFreeSpace(t *testing.T) {
	a, b, z := item(50, 20, "a"), item(50, 20, "b"), item(50, 20)
	cssTree(t, `.c { display: flex; } .a { flex-grow: 1; } .b { flex-grow: 2; }`, cssBox(Row, "c", a, b, z), 300, 100)
	if got, want := widths(a, b, z), []int{100, 150, 50}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v: 150px free split 1:2", got, want)
	}
	if got, want := xs(a, b, z), []int{0, 100, 250}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
	if a.Bounds().H != 100 {
		t.Errorf("height %d, want the stretched 100", a.Bounds().H)
	}
}

// TestFlexShrinkScalesByBase pins the scaled flex shrink factor: an
// overflow is taken in proportion to flex-shrink times the base size.
func TestFlexShrinkScalesByBase(t *testing.T) {
	a, b := item(50, 20, "a"), item(50, 20, "b")
	cssTree(t, `.c { display: flex; } .a { flex: 0 1 200px; } .b { flex: 0 3 200px; }`, cssBox(Row, "c", a, b), 300, 100)
	if got, want := widths(a, b), []int{175, 125}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v: the 100px overflow split 1x200 : 3x200", got, want)
	}
}

// TestFlexFreezeLoop pins the min/max violation rounds: a clamped item
// freezes and the rest share what it left.
func TestFlexFreezeLoop(t *testing.T) {
	t.Run("max violation", func(t *testing.T) {
		m, i1, i2 := item(10, 20, "i", "m"), item(10, 20, "i"), item(10, 20, "i")
		cssTree(t, `.c { display: flex; } .i { flex: 1 1 0px; } .m { max-width: 50px; }`, cssBox(Row, "c", m, i1, i2), 300, 100)
		if got, want := widths(m, i1, i2), []int{50, 125, 125}; !slices.Equal(got, want) {
			t.Errorf("widths %v, want %v", got, want)
		}
	})
	t.Run("min violation", func(t *testing.T) {
		n, i := item(10, 20, "i", "n"), item(10, 20, "i")
		cssTree(t, `.c { display: flex; } .i { flex: 0 1 100px; min-width: 0; } .n { min-width: 80px; }`, cssBox(Row, "c", n, i), 100, 100)
		if got, want := widths(n, i), []int{80, 20}; !slices.Equal(got, want) {
			t.Errorf("widths %v, want %v", got, want)
		}
	})
}

// TestFlexBasis pins the base sizes: a percentage of the container, and
// auto deferring to the width property.
func TestFlexBasis(t *testing.T) {
	p, w := item(10, 20, "p"), item(10, 20, "w")
	cssTree(t, `.c { display: flex; } .p { flex: 0 0 25%; } .w { flex: none; width: 120px; }`, cssBox(Row, "c", p, w), 400, 100)
	if got, want := widths(p, w), []int{100, 120}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v", got, want)
	}
}

// TestFlexOneEqualizes pins flex: 1: a zero basis, so unequal contents
// share the line equally.
func TestFlexOneEqualizes(t *testing.T) {
	a, b := item(50, 20, "f"), item(100, 20, "f")
	cssTree(t, `.c { display: flex; } .f { flex: 1; }`, cssBox(Row, "c", a, b), 300, 100)
	if got, want := widths(a, b), []int{150, 150}; !slices.Equal(got, want) {
		t.Errorf("widths %v, want %v", got, want)
	}
}

// TestFlexAutoMinimumSize pins min-width: auto on flex items: an item
// shrinks no smaller than its content, unless min-width or a clipping
// overflow lets it.
func TestFlexAutoMinimumSize(t *testing.T) {
	for _, c := range []struct {
		name, css string
		want      []int
	}{
		{"content floor", ``, []int{80, 80}},
		{"min-width zero", `.i { min-width: 0; }`, []int{50, 50}},
		{"overflow hidden", `.i { overflow: hidden; }`, []int{50, 50}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, b := item(80, 20, "i"), item(80, 20, "i")
			cssTree(t, `.c { display: flex; } `+c.css, cssBox(Row, "c", a, b), 100, 100)
			if got := widths(a, b); !slices.Equal(got, c.want) {
				t.Errorf("widths %v, want %v", got, c.want)
			}
		})
	}
}

// TestFlexWrap pins line breaking with gaps, and wrap-reverse stacking
// the lines from the cross end.
func TestFlexWrap(t *testing.T) {
	five := func() []*stub {
		return []*stub{item(80, 20), item(80, 20), item(80, 20), item(80, 20), item(80, 20)}
	}
	for _, c := range []struct {
		name, css string
		x, y      []int
	}{
		{"wrap", `flex-wrap: wrap;`, []int{0, 90, 0, 90, 0}, []int{0, 0, 30, 30, 60}},
		{"wrap-reverse", `flex-wrap: wrap-reverse;`, []int{0, 90, 0, 90, 0}, []int{80, 80, 50, 50, 20}},
	} {
		t.Run(c.name, func(t *testing.T) {
			its := five()
			ws := make([]Widget, len(its))
			for i, it := range its {
				ws[i] = it
			}
			cssTree(t, `.c { display: flex; gap: 10px; align-content: flex-start; `+c.css+` }`, cssBox(Row, "c", ws...), 200, 100)
			if got := xs(its...); !slices.Equal(got, c.x) {
				t.Errorf("x %v, want %v", got, c.x)
			}
			if got := ys(its...); !slices.Equal(got, c.y) {
				t.Errorf("y %v, want %v", got, c.y)
			}
		})
	}
}

// TestFlexJustifyContent pins every justify-content value in every
// main-axis direction: 150px of free space over three 50px items.
func TestFlexJustifyContent(t *testing.T) {
	for _, c := range []struct {
		css  string
		axis Axis
		want []int
	}{
		{`justify-content: flex-start;`, Row, []int{0, 50, 100}},
		{`justify-content: flex-end;`, Row, []int{150, 200, 250}},
		{`justify-content: center;`, Row, []int{75, 125, 175}},
		{`justify-content: space-between;`, Row, []int{0, 125, 250}},
		{`justify-content: space-around;`, Row, []int{25, 125, 225}},
		{`justify-content: space-evenly;`, Row, []int{38, 125, 213}},
		{`justify-content: start;`, Row, []int{0, 50, 100}},
		{`justify-content: end;`, Row, []int{150, 200, 250}},
		{`justify-content: left;`, Row, []int{0, 50, 100}},
		{`justify-content: right;`, Row, []int{150, 200, 250}},
		{`justify-content: normal;`, Row, []int{0, 50, 100}},
		{`justify-content: stretch;`, Row, []int{0, 50, 100}},
		{`flex-direction: row-reverse;`, Row, []int{250, 200, 150}},
		{`flex-direction: row-reverse; justify-content: start;`, Row, []int{100, 50, 0}},
		{`flex-direction: row-reverse; justify-content: flex-end;`, Row, []int{100, 50, 0}},
		{`flex-direction: row-reverse; justify-content: left;`, Row, []int{100, 50, 0}},
		{`direction: rtl;`, Row, []int{250, 200, 150}},
		{`direction: rtl; justify-content: left;`, Row, []int{100, 50, 0}},
		{`direction: rtl; justify-content: end;`, Row, []int{100, 50, 0}},
		{`direction: rtl; flex-direction: row-reverse;`, Row, []int{0, 50, 100}},
		{`flex-direction: column; justify-content: flex-end;`, Column, []int{150, 200, 250}},
		{`flex-direction: column-reverse;`, Column, []int{250, 200, 150}},
		{`flex-direction: column-reverse; justify-content: start;`, Column, []int{100, 50, 0}},
		{`flex-direction: column; justify-content: space-between;`, Column, []int{0, 125, 250}},
	} {
		its := []*stub{item(50, 50), item(50, 50), item(50, 50)}
		cssTree(t, `.c { display: flex; `+c.css+` }`, cssBox(Row, "c", its[0], its[1], its[2]), 300, 300)
		got := xs(its...)
		if c.axis == Column {
			got = ys(its...)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: main positions %v, want %v", c.css, got, c.want)
		}
	}
}

// TestFlexJustifySafe pins the overflow alignment: unsafe center
// overflows both edges, safe center falls back to start.
func TestFlexJustifySafe(t *testing.T) {
	for _, c := range []struct {
		css  string
		want []int
	}{
		{`justify-content: center;`, []int{-25, 25, 75}},
		{`justify-content: safe center;`, []int{0, 50, 100}},
		{`justify-content: space-around;`, []int{-25, 25, 75}},
		{`justify-content: space-between;`, []int{0, 50, 100}},
	} {
		its := []*stub{item(50, 20), item(50, 20), item(50, 20)}
		cssTree(t, `.c { display: flex; `+c.css+` }`, cssBox(Row, "c", its[0], its[1], its[2]), 100, 100)
		if got := xs(its...); !slices.Equal(got, c.want) {
			t.Errorf("%s: x %v, want %v", c.css, got, c.want)
		}
	}
}

// TestFlexAlignItems pins every align-items value across a 100px line,
// align-self overriding it, and stretch honoring max-height.
func TestFlexAlignItems(t *testing.T) {
	for _, c := range []struct {
		css  string
		y, h int
	}{
		{`.c { align-items: flex-start; }`, 0, 20},
		{`.c { align-items: start; }`, 0, 20},
		{`.c { align-items: self-start; }`, 0, 20},
		{`.c { align-items: flex-end; }`, 80, 20},
		{`.c { align-items: end; }`, 80, 20},
		{`.c { align-items: center; }`, 40, 20},
		{`.c { align-items: stretch; }`, 0, 100},
		{`.c { align-items: normal; }`, 0, 100},
		{`.c { align-items: flex-start; } .i { align-self: flex-end; }`, 80, 20},
		{`.i { max-height: 30px; }`, 0, 30},
		{`.i { margin-top: auto; }`, 80, 20},
		{`.i { margin-top: auto; margin-bottom: auto; }`, 40, 20},
		{`.c { flex-wrap: wrap-reverse; align-items: flex-start; }`, 80, 20},
		{`.c { flex-wrap: wrap-reverse; align-items: start; }`, 0, 20},
	} {
		i := item(50, 20, "i")
		cssTree(t, `.c { display: flex; } `+c.css, cssBox(Row, "c", i), 300, 100)
		if r := i.Bounds(); r.Y != c.y || r.H != c.h {
			t.Errorf("%s: y %d h %d, want y %d h %d", c.css, r.Y, r.H, c.y, c.h)
		}
	}
}

// TestFlexAlignBaseline pins baseline alignment: the items share the
// line's baseline, the one reaching highest above it at the top.
func TestFlexAlignBaseline(t *testing.T) {
	a := &baseStub{nat: Size{W: 50, H: 20}, base: 15}
	b := &baseStub{nat: Size{W: 50, H: 40}, base: 10}
	c := cssBox(Row, "c")
	c.Append(a, false)
	c.Append(b, false)
	cssTree(t, `.c { display: flex; align-items: baseline; }`, c, 300, 100)
	if ya, yb := a.Bounds().Y, b.Bounds().Y; ya != 0 || yb != 5 {
		t.Errorf("y %d and %d, want 0 and 5: baselines at 15", ya, yb)
	}
}

// TestFlexAlignContent pins every align-content value: two 20px lines
// in a 100px container.
func TestFlexAlignContent(t *testing.T) {
	for _, c := range []struct {
		value  string
		y0, y1 int
		h      int
	}{
		{"flex-start", 0, 20, 20},
		{"start", 0, 20, 20},
		{"flex-end", 60, 80, 20},
		{"end", 60, 80, 20},
		{"center", 30, 50, 20},
		{"space-between", 0, 80, 20},
		{"space-around", 15, 65, 20},
		{"space-evenly", 20, 60, 20},
		{"stretch", 0, 50, 50},
		{"normal", 0, 50, 50},
	} {
		its := []*stub{item(80, 20), item(80, 20), item(80, 20), item(80, 20)}
		cssTree(t, `.c { display: flex; flex-wrap: wrap; align-content: `+c.value+`; }`,
			cssBox(Row, "c", its[0], its[1], its[2], its[3]), 200, 100)
		if y0, y1, h := its[0].Bounds().Y, its[2].Bounds().Y, its[0].Bounds().H; y0 != c.y0 || y1 != c.y1 || h != c.h {
			t.Errorf("align-content %s: lines at %d and %d, %d high; want %d, %d, %d", c.value, y0, y1, h, c.y0, c.y1, c.h)
		}
	}
}

// TestFlexOrder pins order: the layout and the paint follow the
// order-modified document order.
func TestFlexOrder(t *testing.T) {
	var log []string
	mk := func(name, class string) *paintLog {
		p := &paintLog{nat: Size{W: 50, H: 20}, name: name, log: &log}
		p.AddClass(class)
		return p
	}
	a, b, c := mk("a", "o2"), mk("b", "o0"), mk("c", "o1")
	root := cssBox(Row, "c", a, b, c)
	cssTree(t, `.c { display: flex; } .o2 { order: 2; } .o1 { order: 1; }`, root, 300, 100)
	if got, want := []int{a.Bounds().X, b.Bounds().X, c.Bounds().X}, []int{100, 0, 50}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
	paintOnce(root, 300, 100)
	if want := []string{"b", "c", "a"}; !slices.Equal(log, want) {
		t.Errorf("paint order %v, want %v", log, want)
	}
}

// TestFlexGapDefaultsToSpacing pins the interaction rule for gap: an
// undeclared gap keeps the box's own spacing.
func TestFlexGapDefaultsToSpacing(t *testing.T) {
	a, b, c := item(50, 20), item(50, 20), item(50, 20)
	box := NewBox(Row, 7, 0)
	box.AddClass("c")
	box.Append(a, false).Append(b, false).Append(c, false)
	cssTree(t, `.c { display: flex; }`, box, 300, 100)
	if got, want := xs(a, b, c), []int{0, 57, 114}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
}

// TestFlexProgrammaticDefaults pins the interaction rule for direction
// and growth: a column box stays a column under display: flex, and an
// expanding child grows.
func TestFlexProgrammaticDefaults(t *testing.T) {
	a, b := item(50, 20), item(50, 20)
	box := NewBox(Column, 0, 0)
	box.AddClass("c")
	box.Append(a, true).Append(b, false)
	cssTree(t, `.c { display: flex; }`, box, 300, 100)
	if ra, rb := a.Bounds(), b.Bounds(); ra.H != 80 || rb.Y != 80 || ra.W != 300 {
		t.Errorf("a %v b %v, want a 300x80 (grown, stretched) and b at y 80", ra, rb)
	}
}

// TestFlexAutoMargins pins auto margins on the main axis: they take the
// free space before justify-content.
func TestFlexAutoMargins(t *testing.T) {
	a, b, p := item(50, 20), item(50, 20), item(50, 20, "push")
	cssTree(t, `.c { display: flex; justify-content: center; } .push { margin-left: auto; }`, cssBox(Row, "c", a, b, p), 300, 100)
	if got, want := xs(a, b, p), []int{0, 50, 250}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
	m := item(50, 20, "mid")
	cssTree(t, `.c { display: flex; } .mid { margin: 0 auto; }`, cssBox(Row, "c", m), 300, 100)
	if x := m.Bounds().X; x != 125 {
		t.Errorf("x %d, want 125: centered by both auto margins", x)
	}
}

// TestFlexVisibilityCollapse pins collapsed flex items: no main space,
// but their cross size still props the line open.
func TestFlexVisibilityCollapse(t *testing.T) {
	a, b, c := item(50, 20), item(50, 60, "gone"), item(50, 20)
	flex := cssBox(Row, "c", a, b, c)
	root := cssBox(Column, "", flex)
	cssTree(t, `.c { display: flex; } .gone { visibility: collapse; }`, root, 300, 200)
	if ra, rc := a.Bounds(), c.Bounds(); ra.H != 60 || rc.X != 50 {
		t.Errorf("a %v c %v, want a stretched to the 60px strut and c right after it", ra, rc)
	}
	if w := b.Bounds().W; w != 0 {
		t.Errorf("collapsed width %d, want 0", w)
	}
}

// TestDisplayNone pins display: none in CSS flow and in a programmatic
// box: the widget leaves the layout, the siblings close up, and it
// takes no hits.
func TestDisplayNone(t *testing.T) {
	a, n, b := item(50, 20), item(50, 20, "n"), item(50, 20)
	root := cssBox(Column, "c", a, n, b)
	cssTree(t, `.c { display: block; } .n { display: none; }`, root, 300, 100)
	if got, want := ys(a, b), []int{0, 20}; !slices.Equal(got, want) {
		t.Errorf("y %v, want %v", got, want)
	}
	if IsVisible(n) || !n.Bounds().Empty() {
		t.Errorf("display:none child visible %v at %v, want invisible and unarranged", IsVisible(n), n.Bounds())
	}
	a2, n2, b2 := item(50, 20), item(50, 20, "n"), item(50, 20)
	cssTree(t, `.n { display: none; }`, cssBox(Row, "", a2, n2, b2), 300, 100)
	if got, want := xs(a2, b2), []int{0, 50}; !slices.Equal(got, want) {
		t.Errorf("programmatic row x %v, want %v", got, want)
	}
}

// TestDisplayContents pins display: contents: the box's children lay
// out as its parent's flex items while staying its children, and the
// box itself covers them without an area of its own.
func TestDisplayContents(t *testing.T) {
	a, b, c, d := item(50, 20), item(50, 20), item(50, 20), item(50, 20)
	inner := cssBox(Row, "dc", b, c)
	root := cssBox(Row, "c", a, inner, d)
	cssTree(t, `.c { display: flex; align-items: flex-start; } .dc { display: contents; }`, root, 300, 100)
	if got, want := xs(a, b, c, d), []int{0, 50, 100, 150}; !slices.Equal(got, want) {
		t.Errorf("x %v, want %v", got, want)
	}
	if got := inner.Bounds(); got != (render.Rect{X: 50, W: 100, H: 20}) {
		t.Errorf("contents box bounds %v, want the union of its children", got)
	}
	if b.Parent() != Widget(inner) {
		t.Error("a dissolved box's child must stay its child")
	}
	if hit := root.HitTest(Point{X: 60, Y: 5}); hit != Widget(b) {
		t.Errorf("hit %T, want the dissolved box's child", hit)
	}
}

// TestVisibilityHidden pins visibility: hidden: the widget keeps its
// space but paints nothing and takes no hits, and a visible child
// inside a hidden box still shows.
func TestVisibilityHidden(t *testing.T) {
	var log []string
	mk := func(name string, classes ...string) *paintLog {
		p := &paintLog{nat: Size{W: 50, H: 20}, name: name, log: &log}
		p.AddClass(classes...)
		return p
	}
	a, h, b := mk("a"), mk("h", "hide"), mk("b")
	shownKid := mk("k", "show")
	hiddenBox := cssBox(Column, "hide", shownKid)
	root := cssBox(Column, "c", a, h, b, hiddenBox)
	cssTree(t, `.c { display: block; } .hide { visibility: hidden; } .show { visibility: visible; }`, root, 300, 200)
	if got, want := []int{a.Bounds().Y, h.Bounds().Y, b.Bounds().Y}, []int{0, 20, 40}; !slices.Equal(got, want) {
		t.Errorf("y %v, want %v: the hidden widget keeps its space", got, want)
	}
	if hit := root.HitTest(Point{X: 10, Y: 25}); hit != Widget(root) {
		t.Errorf("hit %T over the hidden widget, want the container", hit)
	}
	if hit := root.HitTest(Point{X: 10, Y: 65}); hit != Widget(shownKid) {
		t.Errorf("hit %T, want the visible child of the hidden box", hit)
	}
	paintOnce(root, 300, 200)
	if want := []string{"a", "b", "k"}; !slices.Equal(log, want) {
		t.Errorf("painted %v, want %v", log, want)
	}
}

// TestVisibilityFlipReachesDescendants pins visibility's inheritance on
// a change: hiding a box restyles its subtree, so a child declaring
// nothing hides with it.
func TestVisibilityFlipReachesDescendants(t *testing.T) {
	var log []string
	kid := &paintLog{nat: Size{W: 50, H: 20}, name: "k", log: &log}
	inner := cssBox(Column, "", kid)
	root := cssBox(Column, "", inner)
	cssTree(t, `.hide { visibility: hidden; }`, root, 100, 100)
	inner.AddClass("hide")
	settle(t, root, 100, 100)
	paintOnce(root, 100, 100)
	if len(log) != 0 || root.HitTest(Point{X: 5, Y: 5}) != Widget(root) {
		t.Errorf("painted %v, hit %T: the child of a box turned hidden must hide too", log, root.HitTest(Point{X: 5, Y: 5}))
	}
}

// TestBlockFlow pins block layout: blocks stretch to the container,
// definite widths stay, auto margins center or push, and adjacent
// vertical margins collapse.
func TestBlockFlow(t *testing.T) {
	s, w, cen, push := item(50, 20), item(50, 20, "w"), item(50, 20, "w", "cen"), item(50, 20, "w", "push")
	cssTree(t, `.c { display: block; } .w { width: 100px; } .cen { margin: 0 auto; } .push { margin-left: auto; }`,
		cssBox(Column, "c", s, w, cen, push), 300, 200)
	got := []render.Rect{s.Bounds(), w.Bounds(), cen.Bounds(), push.Bounds()}
	want := []render.Rect{{W: 300, H: 20}, {Y: 20, W: 100, H: 20}, {X: 100, Y: 40, W: 100, H: 20}, {X: 200, Y: 60, W: 100, H: 20}}
	if !slices.Equal(got, want) {
		t.Errorf("rects %v, want %v", got, want)
	}

	m1, m2, m3 := cssBox(Row, "m"), cssBox(Row, "m"), cssBox(Row, "m neg")
	cssTree(t, `.c { display: block; } .m { margin: 20px 0; min-height: 10px; } .neg { margin-top: -5px; }`,
		cssBox(Column, "c", m1, m2, m3), 300, 200)
	if got, want := []int{m1.Bounds().Y, m2.Bounds().Y, m3.Bounds().Y}, []int{20, 50, 75}; !slices.Equal(got, want) {
		t.Errorf("border-box tops %v, want %v: 20px margins collapse to 20, 20 and -5 to 15", got, want)
	}

	r := item(50, 20, "w")
	cssTree(t, `.c { display: block; direction: rtl; } .w { width: 100px; }`, cssBox(Column, "c", r), 300, 100)
	if x := r.Bounds().X; x != 200 {
		t.Errorf("rtl block x %d, want 200", x)
	}
}

// TestInlineFlow pins inline-level layout: line boxes fill and break,
// text-align places them, and vertical-align lines the items up.
func TestInlineFlow(t *testing.T) {
	a, b, c := item(100, 20, "ib"), item(100, 20, "ib"), item(100, 20, "ib")
	cssTree(t, `.c { display: block; text-align: center; } .ib { display: inline-block; }`, cssBox(Column, "c", a, b, c), 250, 100)
	if gx, gy := xs(a, b, c), ys(a, b, c); !slices.Equal(gx, []int{25, 125, 75}) || !slices.Equal(gy, []int{0, 0, 20}) {
		t.Errorf("x %v y %v, want x [25 125 75] y [0 0 20]", gx, gy)
	}

	lo := &baseStub{nat: Size{W: 50, H: 20}, base: 15}
	hi := &baseStub{nat: Size{W: 50, H: 40}, base: 30}
	top := item(50, 10, "top")
	lo.AddClass("ib")
	hi.AddClass("ib")
	top.AddClass("ib")
	after := item(50, 10)
	box := cssBox(Column, "c")
	box.Append(lo, false).Append(hi, false).Append(top, false).Append(after, false)
	cssTree(t, `.c { display: block; } .ib { display: inline; } .top { vertical-align: top; }`, box, 300, 100)
	if got, want := []int{lo.Bounds().Y, hi.Bounds().Y, top.Bounds().Y, after.Bounds().Y}, []int{15, 0, 0, 40}; !slices.Equal(got, want) {
		t.Errorf("y %v, want %v: baselines at 30, the line 40 tall", got, want)
	}

	r1, r2 := item(50, 20, "ib"), item(50, 20, "ib")
	cssTree(t, `.c { display: block; direction: rtl; } .ib { display: inline-block; }`, cssBox(Column, "c", r1, r2), 300, 100)
	if got := xs(r1, r2); !slices.Equal(got, []int{250, 200}) {
		t.Errorf("rtl inline x %v, want [250 200]", got)
	}
}

// TestFloats pins floats: a float takes its side, blocks and lines flow
// beside it, a float after inline content joins the line it fits on,
// clear moves below every float, and the container holds them.
func TestFloats(t *testing.T) {
	fl, blk, ib, fr, cl := item(50, 50, "fl"), item(50, 20), item(40, 20, "ib"), item(30, 30, "fr"), item(50, 20, "cl")
	root := cssBox(Column, "c", fl, blk, ib, fr, cl)
	cssTree(t, `.c { display: block; } .fl { float: left; } .fr { float: right; } .ib { display: inline-block; } .cl { clear: both; }`, root, 300, 200)
	want := []render.Rect{
		{W: 50, H: 50},
		{X: 50, W: 250, H: 20},
		{X: 50, Y: 20, W: 40, H: 20},
		{X: 270, Y: 20, W: 30, H: 30},
		{Y: 50, W: 300, H: 20},
	}
	if got := []render.Rect{fl.Bounds(), blk.Bounds(), ib.Bounds(), fr.Bounds(), cl.Bounds()}; !slices.Equal(got, want) {
		t.Errorf("rects %v, want %v", got, want)
	}
	only := cssBox(Column, "c", item(50, 50, "fl"))
	loadCSS(t, `.c { display: block; } .fl { float: left; }`)
	if sz := only.Measure(Constraints{Max: Size{W: 300, H: 300}}); sz.H != 50 {
		t.Errorf("container height %d, want the float's 50", sz.H)
	}
}

// TestCSSSizingOnEveryWidget pins the sizing properties in a
// programmatic container: definite and percentage widths, max/min
// bounds, the content keywords, box-sizing and aspect-ratio.
func TestCSSSizingOnEveryWidget(t *testing.T) {
	for _, c := range []struct {
		css  string
		axis Axis
		want render.Rect
	}{
		{`.i { width: 100px; }`, Column, render.Rect{W: 100, H: 20}},
		{`.i { width: 50%; }`, Column, render.Rect{W: 200, H: 20}},
		{`.i { max-width: 120px; }`, Column, render.Rect{W: 120, H: 20}},
		{`.i { width: 300px; max-width: 120px; }`, Column, render.Rect{W: 120, H: 20}},
		{`.i { min-width: 50%; }`, Row, render.Rect{W: 200, H: 300}},
		{`.i { width: max-content; }`, Column, render.Rect{W: 80, H: 20}},
		{`.i { width: fit-content; }`, Column, render.Rect{W: 80, H: 20}},
		{`.i { width: 100px; aspect-ratio: 2; }`, Row, render.Rect{W: 100, H: 50}},
		{`.i { height: 50px; aspect-ratio: 2; }`, Column, render.Rect{W: 100, H: 50}},
		{`.i { height: 50%; }`, Row, render.Rect{W: 80, H: 150}},
		{`.i { width: calc(50% - 20px); }`, Column, render.Rect{W: 180, H: 20}},
	} {
		i := item(80, 20, "i")
		cssTree(t, c.css, cssBox(c.axis, "", i), 400, 300)
		if got := i.Bounds(); got != c.want {
			t.Errorf("%s: %v, want %v", c.css, got, c.want)
		}
	}
	g := &widthGiver{nat: Size{W: 80, H: 20}, floor: 10}
	g.AddClass("i")
	cssTree(t, `.i { width: min-content; }`, cssBox(Column, "", g), 400, 300)
	if w := g.Bounds().W; w != 10 {
		t.Errorf("min-content width %d, want the 10px floor", w)
	}
}

// TestBoxSizing pins box-sizing on a box with padding: content-box
// sizes add the padding, border-box sizes include it.
func TestBoxSizing(t *testing.T) {
	for _, c := range []struct {
		css  string
		want int
	}{
		{`.b { width: 100px; padding: 10px; }`, 120},
		{`.b { width: 100px; padding: 10px; box-sizing: border-box; }`, 100},
		{`.b { min-width: 100px; padding: 10px; box-sizing: border-box; }`, 100},
	} {
		b := cssBox(Row, "b")
		cssTree(t, c.css, cssBox(Row, "", b), 400, 100)
		if w := b.Bounds().W; w != c.want {
			t.Errorf("%s: width %d, want %d", c.css, w, c.want)
		}
	}
}

// TestPositionRelative pins relative offsets: the widget shifts, the
// siblings stay where the flow put them.
func TestPositionRelative(t *testing.T) {
	for _, c := range []struct {
		css  string
		x, y int
	}{
		{`.r { left: 10px; top: 5px; }`, 60, 5},
		{`.r { right: 10px; bottom: 5px; }`, 40, -5},
		{`.r { left: 10%; }`, 80, 0},
		{`.r { left: 10px; right: 99px; }`, 60, 0},
	} {
		a, r, b := item(50, 20), item(50, 20, "r"), item(50, 20)
		cssTree(t, `.r { position: relative; } `+c.css, cssBox(Row, "", a, r, b), 300, 100)
		if got := r.Bounds(); got.X != c.x || got.Y != c.y {
			t.Errorf("%s: at %d,%d, want %d,%d", c.css, got.X, got.Y, c.x, c.y)
		}
		if got := xs(a, b); !slices.Equal(got, []int{0, 100}) {
			t.Errorf("%s: siblings at %v, want [0 100]", c.css, got)
		}
	}
}

// TestPositionAbsolute pins out-of-flow placement: against the
// positioned ancestor's padding box by its insets, stretched between
// opposite insets, centered by auto margins, at its static position
// with no insets, and never displacing the flow.
func TestPositionAbsolute(t *testing.T) {
	for _, c := range []struct {
		css  string
		want render.Rect
	}{
		{`.a { right: 10px; bottom: 20px; width: 50px; height: 30px; }`, render.Rect{X: 235, Y: 145, W: 50, H: 30}},
		{`.a { inset: 0; }`, render.Rect{X: 5, Y: 5, W: 290, H: 190}},
		{`.a { inset: 0; width: 100px; height: 50px; margin: auto; }`, render.Rect{X: 100, Y: 75, W: 100, H: 50}},
		{`.a { left: 10%; top: 0; }`, render.Rect{X: 34, Y: 5, W: 40, H: 10}},
		{``, render.Rect{X: 5, Y: 25, W: 40, H: 10}},
	} {
		before, abs, after := item(50, 20), item(40, 10, "a"), item(50, 20)
		root := cssBox(Column, "r", before, abs, after)
		cssTree(t, `.r { position: relative; border: 5px solid red; } .a { position: absolute; } `+c.css, root, 300, 200)
		if got := abs.Bounds(); got != c.want {
			t.Errorf("%q: %v, want %v", c.css, got, c.want)
		}
		if y := after.Bounds().Y; y != 25 {
			t.Errorf("%q: the next sibling at y %d, want 25: an absolute takes no space", c.css, y)
		}
	}
	fixed := item(10, 10, "f")
	inner := cssBox(Column, "pos", fixed)
	root := cssBox(Column, "", item(50, 50), inner)
	cssTree(t, `.pos { position: relative; } .f { position: fixed; right: 0; bottom: 0; }`, root, 300, 200)
	if got := fixed.Bounds(); got != (render.Rect{X: 290, Y: 190, W: 10, H: 10}) {
		t.Errorf("fixed %v, want the window's bottom-right corner", got)
	}
	flexAbs, sib := item(40, 10, "a"), item(50, 20)
	cssTree(t, `.c { display: flex; position: relative; } .a { position: absolute; right: 0; top: 0; }`, cssBox(Row, "c", flexAbs, sib), 300, 100)
	if got, x := flexAbs.Bounds(), sib.Bounds().X; got != (render.Rect{X: 260, W: 40, H: 10}) || x != 0 {
		t.Errorf("flex absolute %v and sibling x %d, want 260,0 40x10 and 0", got, x)
	}
}

// TestPositionSticky pins sticky: inside a scrolled viewport the header
// keeps to the top inset instead of scrolling away.
func TestPositionSticky(t *testing.T) {
	head, body := item(80, 20, "s"), item(80, 300)
	col := cssBox(Column, "", head, body)
	sc := NewScroll(col)
	cssTree(t, `.s { position: sticky; top: 0; }`, sc, 100, 100)
	if y := head.Bounds().Y; y != 0 {
		t.Fatalf("unscrolled header at %d, want 0", y)
	}
	sc.SetOffset(0, 50)
	settle(t, sc, 100, 100)
	if y, by := head.Bounds().Y, body.Bounds().Y; y != 0 || by != -30 {
		t.Errorf("scrolled header at %d (body at %d), want 0 (and -30)", y, by)
	}
}

// TestZIndexOrdersPaintAndHits pins z-index: the stacking order decides
// both what paints on top and what the pointer hits.
func TestZIndexOrdersPaintAndHits(t *testing.T) {
	for _, c := range []struct {
		css   string
		paint []string
		hit   string
	}{
		{``, []string{"a", "b"}, "b"},
		{`.a { position: relative; z-index: 2; }`, []string{"b", "a"}, "a"},
		{`.b { z-index: -1; }`, []string{"b", "a"}, "a"},
		{`.a { position: relative; } .b { z-index: 0; }`, []string{"a", "b"}, "b"},
	} {
		var log []string
		a := &paintLog{nat: Size{W: 50, H: 50}, name: "a", log: &log}
		b := &paintLog{nat: Size{W: 50, H: 50}, name: "b", log: &log}
		a.AddClass("a")
		b.AddClass("b")
		root := cssBox(Row, "")
		root.Append(a, false).Append(b, false)
		cssTree(t, `.b { position: relative; left: -25px; } `+c.css, root, 200, 50)
		paintOnce(root, 200, 50)
		if !slices.Equal(log, c.paint) {
			t.Errorf("%q: paint order %v, want %v", c.css, log, c.paint)
		}
		hit, _ := root.HitTest(Point{X: 30, Y: 10}).(*paintLog)
		if hit == nil || hit.name != c.hit {
			t.Errorf("%q: hit %v, want %s on top", c.css, hit, c.hit)
		}
	}
}

// TestOverflowClipsChildren pins overflow on a box: a clipping axis
// keeps the children's hits inside the padding box; a visible one lets
// them out.
func TestOverflowClipsChildren(t *testing.T) {
	for _, c := range []struct {
		css     string
		outside bool
	}{
		{``, true},
		{`.o { overflow: hidden; }`, false},
		{`.o { overflow-x: clip; }`, false},
		{`.o { overflow-y: hidden; }`, true},
	} {
		k := item(50, 20, "k")
		o := cssBox(Row, "o", k)
		root := cssBox(Column, "", o)
		cssTree(t, `.o { width: 100px; } .k { position: relative; left: 80px; } `+c.css, root, 300, 100)
		if hit := root.HitTest(Point{X: 120, Y: 5}) == Widget(k); hit != c.outside {
			t.Errorf("%q: hit past the box's edge %v, want %v", c.css, hit, c.outside)
		}
		if hit := root.HitTest(Point{X: 90, Y: 5}); hit != Widget(k) {
			t.Errorf("%q: hit inside %T, want the child", c.css, hit)
		}
	}
}

// TestLayoutPropertyChangeRelayouts pins the invalidation: a class flip
// that changes a layout property relayouts on the next frame.
func TestLayoutPropertyChangeRelayouts(t *testing.T) {
	a, b := item(50, 20, "g"), item(50, 20)
	root := cssBox(Row, "c", a, b)
	cssTree(t, `.c { display: flex; } .c.col { flex-direction: column; } .g.big { flex-grow: 1; }`, root, 300, 100)
	if y := b.Bounds().Y; y != 0 {
		t.Fatalf("row: b at y %d, want 0", y)
	}
	root.AddClass("col")
	settle(t, root, 300, 100)
	if y := b.Bounds().Y; y != 20 {
		t.Errorf("after the flip to column: b at y %d, want 20", y)
	}
	root.RemoveClass("col")
	a.AddClass("big")
	settle(t, root, 300, 100)
	if w := a.Bounds().W; w != 250 {
		t.Errorf("after flex-grow on a: width %d, want 250", w)
	}
}

// TestCSSLayoutSteadyFrameAllocatesNothing pins the layout buffers: a
// static flex and flow tree measures and arranges without allocating
// once the per-depth buffers have grown.
func TestCSSLayoutSteadyFrameAllocatesNothing(t *testing.T) {
	skipAllocBudget(t)
	inner := cssBox(Row, "f", item(40, 20, "g"), item(40, 20), item(40, 20, "fl"))
	root := cssBox(Column, "b", item(50, 20), inner, item(50, 20, "ib"), item(50, 20, "ib"))
	cssTree(t, `.b { display: block; } .f { display: flex; flex-wrap: wrap; gap: 4px; } .g { flex: 1; z-index: 1; }
		.fl { order: -1; } .ib { display: inline-block; }`, root, 300, 200)
	frame := func() {
		styleGen++
		root.Measure(Constraints{Max: Size{W: 300, H: 200}})
		root.Arrange(render.Rect{W: 300, H: 200})
	}
	frame()
	if allocs := testing.AllocsPerRun(20, frame); allocs != 0 {
		t.Errorf("CSS layout frame allocates %v allocs/op, want 0", allocs)
	}
}

// TestHiddenWidgetsTakeNoFocus pins that a widget off the screen —
// display: none or visibility: hidden — is skipped by Tab, refused by
// SetFocus, loses a focus it held, and leaves the a11y snapshot.
func TestHiddenWidgetsTakeNoFocus(t *testing.T) {
	a, none, hid := NewSwitch(false), NewSwitch(false), NewSwitch(false)
	none.AddClass("none")
	hid.AddClass("hid")
	root := cssBox(Column, "", a, none, hid)
	cssTree(t, `.none { display: none; } .hid { visibility: hidden; }`, root, 200, 200)
	r := &Router{Root: root}
	r.FocusNext()
	r.FocusNext()
	if r.Focused() != Widget(a) {
		t.Errorf("Tab landed on %T, want the only shown switch", r.Focused())
	}
	r.SetFocus(hid)
	if r.Focused() != Widget(a) {
		t.Error("SetFocus moved focus onto a hidden switch")
	}
	if n := len(DescribeTree(root)); n != 2 {
		t.Errorf("a11y snapshot holds %d widgets, want 2: the box and the shown switch", n)
	}
	a.AddClass("none")
	settle(t, root, 200, 200)
	r.KeyAction(KeyEnter, 0)
	if r.Focused() != nil {
		t.Errorf("focus %T stayed on a switch that went display: none", r.Focused())
	}
}

// With @container rules loaded, a query container's resize restyles
// its descendants; any other widget's resize, a tooltip arranged or a
// row moving, leaves every style as it was, so it damages only its own
// pixels instead of every widget in every window (#328).
func TestOnlyAQueryContainerResizeRestyles(t *testing.T) {
	loadCSS(t, `.q { container-type: inline-size; } @container (min-width: 100px) { .x { color: #ff0000; } }`)
	plain, query := newStub(10, 10), newStub(10, 10)
	query.AddClass("q")
	row := NewBox(Row, 0, 0).Append(plain, false).Append(query, false)
	settle(t, row, 50, 10)
	before := styleGen
	plain.Arrange(render.Rect{W: 80, H: 10})
	if styleGen != before {
		t.Errorf("a plain widget's resize restyled everything (styleGen %d -> %d)", before, styleGen)
	}
	query.Arrange(render.Rect{W: 120, H: 10})
	if styleGen == before {
		t.Error("a query container's resize restyled nothing")
	}
}
