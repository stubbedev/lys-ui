package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// stub is a fixed-size leaf that records its arranged rect and paints
// nothing.
type stub struct {
	node
	nat  Size
	rect render.Rect
}

func newStub(w, h int) *stub {
	return &stub{nat: Size{W: w, H: h}}
}

func (s *stub) Measure(con Constraints) Size {
	return clampSize(s.nat, con)
}

func (s *stub) Arrange(r render.Rect) {
	s.node.Arrange(r)
	s.rect = r
}

func (s *stub) Paint(*render.Canvas) {}

func (s *stub) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

func TestBoxMeasureRow(t *testing.T) {
	t.Run("sums children, spacing, and padding", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		b.Append(newStub(10, 5), false)
		b.Append(newStub(8, 4), false)
		got := b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got.W != 1+10+2+8+1 {
			t.Errorf("main extent = %d, want 22", got.W)
		}
		if got.H != 5+2 {
			t.Errorf("cross extent = %d, want 7", got.H)
		}
	})

	t.Run("transposes for a column", func(t *testing.T) {
		b := NewBox(Column, 2, 1)
		b.Append(newStub(10, 5), false)
		b.Append(newStub(8, 4), false)
		got := b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got.H != 1+5+2+4+1 {
			t.Errorf("main extent = %d, want 13", got.H)
		}
		if got.W != 10+2 {
			t.Errorf("cross extent = %d, want 12", got.W)
		}
	})

	t.Run("empty box is just padding", func(t *testing.T) {
		got := NewBox(Row, 4, 3).Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got != (Size{W: 6, H: 6}) {
			t.Errorf("empty box = %v, want 6x6", got)
		}
	})

	t.Run("clamps to constraints", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		b.Append(newStub(50, 5), false)
		got := b.Measure(Constraints{Max: Size{W: 30, H: 3}})
		if got.W != 30 || got.H != 3 {
			t.Errorf("measured %v, want clamped to 30x3", got)
		}
	})
}

func TestBoxArrangeRow(t *testing.T) {
	t.Run("children line up with spacing and stretch the cross axis", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		a := newStub(10, 5)
		c := newStub(8, 4)
		b.Append(a, false)
		b.Append(c, false)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

		if a.rect != (render.Rect{X: 1, Y: 1, W: 10, H: 18}) {
			t.Errorf("first child rect = %v, want x=1 h stretched to 18", a.rect)
		}
		if c.rect != (render.Rect{X: 13, Y: 1, W: 8, H: 18}) {
			t.Errorf("second child rect = %v, want x=13", c.rect)
		}
	})

	t.Run("expanders split the leftover main space", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		a := newStub(10, 5)
		c := newStub(10, 5)
		d := newStub(10, 5)
		b.Append(a, false)
		b.Append(c, true)
		b.Append(d, true)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 70, H: 20})

		// 70 - 30 natural = 40 free, split between two expanders.
		if c.rect.W != 30 || d.rect.W != 30 {
			t.Errorf("expander widths = %d, %d, want 30, 30", c.rect.W, d.rect.W)
		}
		if a.rect.W != 10 {
			t.Errorf("non-expander width = %d, want 10", a.rect.W)
		}
		if d.rect.X != 40 {
			t.Errorf("last child x = %d, want 40", d.rect.X)
		}
	})

	t.Run("no expander leaves leftover space unused", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		a := newStub(10, 5)
		b.Append(a, false)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 20})

		if a.rect.W != 10 {
			t.Errorf("child width = %d, want natural 10", a.rect.W)
		}
	})

	t.Run("overflowing children keep their natural size", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		a := newStub(30, 5)
		c := newStub(30, 5)
		b.Append(a, false)
		b.Append(c, false)

		b.Measure(Constraints{Max: Size{W: 40, H: 20}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

		if a.rect.W != 30 || c.rect.W != 30 {
			t.Errorf("overflowing children shrank: %d, %d", a.rect.W, c.rect.W)
		}
		if c.rect.X != 30 {
			t.Errorf("second child x = %d, want 30 (overflow, not clamped)", c.rect.X)
		}
	})

	t.Run("column layout runs down", func(t *testing.T) {
		b := NewBox(Column, 3, 0)
		a := newStub(10, 5)
		c := newStub(10, 7)
		b.Append(a, false)
		b.Append(c, false)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 50, H: 40})

		if a.rect != (render.Rect{X: 0, Y: 0, W: 50, H: 5}) {
			t.Errorf("first child = %v, want w stretched to 50", a.rect)
		}
		if c.rect != (render.Rect{X: 0, Y: 8, W: 50, H: 7}) {
			t.Errorf("second child = %v, want y=8", c.rect)
		}
	})
}

func TestBoxHitTest(t *testing.T) {
	b := NewBox(Row, 2, 1)
	a := newStub(10, 5)
	b.Append(a, false)
	b.Measure(Constraints{Max: Size{W: 100, H: 100}})
	b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

	t.Run("over a child returns the child", func(t *testing.T) {
		if got := b.HitTest(Point{X: 5, Y: 5}); got != Widget(a) {
			t.Errorf("hit = %v, want the stub", got)
		}
	})

	t.Run("over padding returns the box", func(t *testing.T) {
		if got := b.HitTest(Point{X: 0, Y: 0}); got != Widget(b) {
			t.Errorf("hit = %v, want the box", got)
		}
	})

	t.Run("outside returns nil", func(t *testing.T) {
		if got := b.HitTest(Point{X: 200, Y: 200}); got != nil {
			t.Errorf("hit = %v, want nil", got)
		}
	})
}

// An aligned child keeps its natural cross size, pinned where asked;
// a plain one still stretches.
func TestBoxAppendAligned(t *testing.T) {
	row := NewBox(Row, 0, 0)
	fill, mid, end := newStub(10, 6), newStub(10, 6), newStub(10, 6)
	row.Append(fill, false).AppendAligned(mid, false, AlignCenter).AppendAligned(end, false, AlignEnd)
	row.Measure(Constraints{Max: Size{W: 100, H: 100}})
	row.Arrange(render.Rect{W: 30, H: 20})
	for name, c := range map[string]struct {
		w    Widget
		want render.Rect
	}{
		"fill":   {fill, render.Rect{X: 0, Y: 0, W: 10, H: 20}},
		"center": {mid, render.Rect{X: 10, Y: 7, W: 10, H: 6}},
		"end":    {end, render.Rect{X: 20, Y: 14, W: 10, H: 6}},
	} {
		if got := c.w.(interface{ Bounds() render.Rect }).Bounds(); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	col := NewBox(Column, 0, 0)
	start := newStub(8, 5)
	col.AppendAligned(start, true, AlignStart)
	col.Measure(Constraints{Max: Size{W: 100, H: 100}})
	col.Arrange(render.Rect{W: 40, H: 20})
	if got := start.Bounds(); got != (render.Rect{X: 0, Y: 0, W: 8, H: 20}) {
		t.Errorf("column start: %v, want natural width, expanded height", got)
	}
}

// A transform scales the box's paint about its center: a 10x10 swatch
// at scale(2) inks a 20x20 area, and the tween eases between states.
func TestBoxTransform(t *testing.T) {
	c := pinAnimClock(t)
	face := goldenFace(t)
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(`
		.swatch { background-color: #ff0000; padding: 5px; }
		.swatch:hover { transform: scale(2); transition: transform 100ms; }
	`, StylePriorityUser))
	root.SetPadding(render.Insets{Left: 20, Top: 15})
	sw := NewBox(Row, 0, 0)
	sw.AddClass("swatch")
	root.Append(NewLabel(face, 10, "pad", render.RGBA(0, 0, 0, 255)), false)
	root.Append(sw, false)
	sz := root.Measure(Constraints{Max: Size{W: 200, H: 200}})
	root.Arrange(render.Rect{W: sz.W, H: sz.H})

	data := make([]byte, render.Stride(200)*100)
	cv := render.NewScaled(data, render.Stride(200), 200, 100, 1, 1)
	cv.Clear(cv.Rect(), 0)
	PaintChild(cv, root)
	span := func() (minX, maxX int) {
		minX, maxX = 1<<30, -1
		for y := range 100 {
			for x := range 200 {
				if render.ColorFromBytes(data[(y*200+x)*4:]).A() == 255 && render.ColorFromBytes(data[(y*200+x)*4:]).R() == 255 {
					minX = min(minX, x)
					maxX = max(maxX, x)
				}
			}
		}
		return minX, maxX
	}
	a, b := span()
	base := b - a
	if base <= 0 {
		t.Fatal("the unhovered swatch painted nothing")
	}

	// Hover: the tween runs; settled, the paint spans twice the box.
	setHoverChain(nil, sw)
	_ = sw.style(sw)
	c.drive()
	data = make([]byte, render.Stride(200)*100)
	cv = render.NewScaled(data, render.Stride(200), 200, 100, 1, 1)
	cv.Clear(cv.Rect(), 0)
	PaintChild(cv, root)
	a, b = span()
	if width := b - a; width < base*9/5 || width > base*11/5 {
		t.Errorf("hovered swatch inks %dpx, want twice the unhovered %d (bilinear edges within tolerance)", width, base)
	}
}

// A box arranged at a size its last measure did not produce lays out
// for the arranged size: a header measured under a zero-height probe
// still gets its height when the box is arranged tall.
func TestBoxArrangesForItsAllocationNotAStaleMeasure(t *testing.T) {
	header := NewSpacer(50, 20)
	col := NewBox(Column, 0, 0)
	col.Append(header, false)
	col.Append(NewSpacer(50, 100), true)
	col.Measure(Constraints{Max: Size{W: 200, H: 0}})
	col.Arrange(render.Rect{W: 200, H: 300})
	if b := header.Bounds(); b.H != 20 || b.W != 200 {
		t.Errorf("header %v, want 200x20 after the zero-height probe", b)
	}
}
