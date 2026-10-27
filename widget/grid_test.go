package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestGridMeasure(t *testing.T) {
	t.Run("columns take their widest child plus spacing", func(t *testing.T) {
		g := NewGrid(2, 1)
		g.Attach(newStub(10, 5), 0, 0, 1, 1)
		g.Attach(newStub(8, 4), 1, 0, 1, 1)
		got := SizeWithin(g, Size{W: 100, H: 100})
		if got.W != 10+2+8 {
			t.Errorf("width = %d, want 20", got.W)
		}
		if got.H != 5 {
			t.Errorf("height = %d, want the tallest child 5", got.H)
		}
	})

	t.Run("rows take their tallest child plus spacing", func(t *testing.T) {
		g := NewGrid(0, 3)
		g.Attach(newStub(10, 5), 0, 0, 1, 1)
		g.Attach(newStub(8, 7), 0, 1, 1, 1)
		got := SizeWithin(g, Size{W: 100, H: 100})
		if got.H != 5+3+7 {
			t.Errorf("height = %d, want 15", got.H)
		}
		if got.W != 10 {
			t.Errorf("width = %d, want the widest child 10", got.W)
		}
	})

	t.Run("a spanning child's deficit spreads over the spanned tracks", func(t *testing.T) {
		g := NewGrid(2, 0)
		g.Attach(newStub(10, 5), 0, 0, 1, 1)
		g.Attach(newStub(8, 5), 1, 0, 1, 1)
		// The singles make the run 10+2+8 = 20 wide; a 25-wide spanner
		// is 5 short, split as 2+1 (remainder to the earlier track).
		g.Attach(newStub(25, 4), 0, 1, 2, 1)
		got := SizeWithin(g, Size{W: 100, H: 100})
		if got.W != 13+2+10 {
			t.Errorf("width = %d, want 25 (columns 13 and 10)", got.W)
		}
		if got.H != 5+4 {
			t.Errorf("height = %v, want 9 (rows 5 and 4)", got.H)
		}
	})

	t.Run("homogeneous evens the tracks out", func(t *testing.T) {
		g := NewGrid(2, 0)
		g.Attach(newStub(10, 5), 0, 0, 1, 1)
		g.Attach(newStub(8, 7), 1, 0, 1, 1)
		nat := SizeWithin(g, Size{W: 100, H: 100})
		if nat != (Size{W: 20, H: 7}) {
			t.Fatalf("natural = %v, want 20x7", nat)
		}
		g.SetColumnHomogeneous(true).SetRowHomogeneous(true)
		got := SizeWithin(g, Size{W: 100, H: 100})
		if got != (Size{W: 10 + 2 + 10, H: 7}) {
			t.Errorf("homogeneous = %v, want 22x7", got)
		}
	})

	t.Run("an empty grid measures zero", func(t *testing.T) {
		got := SizeWithin(NewGrid(4, 4), Size{W: 100, H: 100})
		if got != (Size{}) {
			t.Errorf("empty grid = %v, want 0x0", got)
		}
	})

	t.Run("overflows a smaller host rather than squeeze", func(t *testing.T) {
		g := NewGrid(2, 0)
		g.Attach(newStub(50, 5), 0, 0, 1, 1)
		if got := SizeWithin(g, Size{W: 30, H: 3}); got != (Size{W: 50, H: 5}) {
			t.Errorf("measured %v, want its 50x5 minimum", got)
		}
	})
}

func TestGridArrange(t *testing.T) {
	t.Run("cells line up with spacing at natural size", func(t *testing.T) {
		g := NewGrid(2, 0)
		a := newStub(10, 5)
		c := newStub(8, 4)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(c, 1, 0, 1, 1)
		SizeWithin(g, Size{W: 100, H: 100})
		g.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 5})

		if a.rect != (render.Rect{X: 0, Y: 0, W: 10, H: 5}) {
			t.Errorf("first child = %v, want natural cell", a.rect)
		}
		if c.rect != (render.Rect{X: 12, Y: 0, W: 8, H: 5}) {
			t.Errorf("second child = %v, want x=12 with the cross axis filled", c.rect)
		}
	})

	t.Run("surplus splits equally, remainder to the earlier tracks", func(t *testing.T) {
		g := NewGrid(2, 0)
		a := newStub(10, 5)
		c := newStub(8, 5)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(c, 1, 0, 1, 1)
		SizeWithin(g, Size{W: 100, H: 100})
		g.Arrange(render.Rect{X: 0, Y: 0, W: 41, H: 5})

		// Natural 20 in a 41 rect leaves 21: +11 and +10.
		if a.rect.W != 21 {
			t.Errorf("first column = %d, want 21", a.rect.W)
		}
		if c.rect != (render.Rect{X: 23, Y: 0, W: 18, H: 5}) {
			t.Errorf("second child = %v, want x=23 w=18", c.rect)
		}
	})

	t.Run("a spanning child gets its share of the surplus", func(t *testing.T) {
		g := NewGrid(0, 2)
		a := newStub(10, 5)
		c := newStub(10, 5)
		d := newStub(10, 5)
		span := newStub(20, 5)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(c, 1, 0, 1, 1)
		g.Attach(d, 2, 0, 1, 1)
		g.Attach(span, 0, 1, 2, 1)
		SizeWithin(g, Size{W: 100, H: 100})
		g.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 12})

		// Natural 30 in a 60 rect doubles every column, so the two
		// spanned cells hand the child 40 plus the zero internal
		// spacing.
		if span.rect != (render.Rect{X: 0, Y: 7, W: 40, H: 5}) {
			t.Errorf("spanning child = %v, want x=0 w=40 at row 1", span.rect)
		}
		if d.rect != (render.Rect{X: 40, Y: 0, W: 20, H: 5}) {
			t.Errorf("third column = %v, want x=40 w=20", d.rect)
		}
	})

	t.Run("a smaller rect squeezes the tracks proportionally", func(t *testing.T) {
		g := NewGrid(2, 0)
		a := squeezable(10, 5)
		c := squeezable(8, 5)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(c, 1, 0, 1, 1)
		SizeWithin(g, Size{W: 100, H: 100})
		g.Arrange(render.Rect{X: 0, Y: 0, W: 15, H: 5})

		// Natural 20 (10 + 2 spacing + 8) in a 15 rect: the 5px deficit
		// splits by weight - 10:8 - with the rounding pixel to the
		// earlier track, so 7 + 2 + 6 = 15 exactly. Overflow only at the
		// floors, and these stubs have none.
		if a.rect != (render.Rect{X: 0, Y: 0, W: 7, H: 5}) {
			t.Errorf("first child = %v, want the squeezed 7-wide rect", a.rect)
		}
		if c.rect != (render.Rect{X: 9, Y: 0, W: 6, H: 5}) {
			t.Errorf("second child = %v, want the squeezed 6-wide rect", c.rect)
		}
	})

	t.Run("alignment pins a natural-size child inside its cell", func(t *testing.T) {
		newGrid := func(h, v Align) *Grid {
			g := NewGrid(0, 0)
			g.Attach(newStub(10, 5), 0, 0, 1, 1)
			g.SetAlign(g.Children()[0], h, v)
			SizeWithin(g, Size{W: 100, H: 100})
			return g
		}

		g := newGrid(AlignStart, AlignStart)
		g.Arrange(render.Rect{X: 0, Y: 0, W: 30, H: 15})
		if got := g.Children()[0].(interface{ Bounds() render.Rect }).Bounds(); got != (render.Rect{X: 0, Y: 0, W: 10, H: 5}) {
			t.Errorf("start alignment = %v, want the natural rect at the origin", got)
		}

		g = newGrid(AlignEnd, AlignCenter)
		g.Arrange(render.Rect{X: 0, Y: 0, W: 30, H: 15})
		if got := g.Children()[0].(interface{ Bounds() render.Rect }).Bounds(); got != (render.Rect{X: 20, Y: 5, W: 10, H: 5}) {
			t.Errorf("end/center alignment = %v, want pinned right, centered vertically", got)
		}
	})

	t.Run("an empty grid arranges nothing and survives", func(t *testing.T) {
		g := NewGrid(4, 4)
		g.Arrange(render.Rect{X: 0, Y: 0, W: 50, H: 50})
		if len(g.Children()) != 0 {
			t.Errorf("children = %d, want none", len(g.Children()))
		}
	})
}

func TestGridChildrenRowMajor(t *testing.T) {
	g := NewGrid(0, 0)
	t1 := newFocusTarget()
	t2 := newFocusTarget()
	t3 := newFocusTarget()
	// Attach out of visual order: last column first, the two-column
	// span second, and row 1 last.
	g.Attach(t1, 2, 0, 1, 1)
	g.Attach(t2, 0, 0, 2, 1)
	g.Attach(t3, 0, 1, 1, 1)

	t.Run("children read left-to-right down the rows", func(t *testing.T) {
		want := []Widget{t2, t1, t3}
		got := g.Children()
		if len(got) != len(want) {
			t.Fatalf("children = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("child %d = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("tab walks the same order through spans", func(t *testing.T) {
		r := &Router{Root: g}
		for _, want := range []Widget{t2, t1, t3, t2} {
			r.FocusNext()
			if r.Focused() != want {
				t.Fatalf("focused = %v, want %v", r.Focused(), want)
			}
		}
	})
}

func TestGridAttachConflict(t *testing.T) {
	t.Run("attaching over an occupied cell keeps the last child", func(t *testing.T) {
		g := NewGrid(0, 0)
		a := newStub(10, 5)
		b := newStub(8, 4)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(b, 0, 0, 1, 1)

		got := g.Children()
		if len(got) != 1 || got[0] != Widget(b) {
			t.Fatalf("children = %v, want only the later attach", got)
		}
		if sz := SizeWithin(g, Size{W: 100, H: 100}); sz != (Size{W: 8, H: 4}) {
			t.Errorf("measure = %v, want the last child's 8x4", sz)
		}
		g.Arrange(render.Rect{X: 0, Y: 0, W: 8, H: 4})
		if b.rect.W != 8 {
			t.Errorf("winner width = %d, want 8", b.rect.W)
		}
		if a.rect != (render.Rect{}) {
			t.Errorf("loser arranged at %v, want untouched", a.rect)
		}
	})

	t.Run("reattaching a widget moves it instead of doubling it", func(t *testing.T) {
		g := NewGrid(2, 0)
		a := newStub(10, 5)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(a, 1, 0, 1, 1)

		if len(g.Children()) != 1 {
			t.Fatalf("children = %d, want one entry", len(g.Children()))
		}
		// Column 0 is empty, so it is zero wide; the widget sits in
		// column 1 after the spacing.
		if sz := SizeWithin(g, Size{W: 100, H: 100}); sz.W != 0+2+10 {
			t.Errorf("width = %d, want 12", sz.W)
		}
	})

	t.Run("reattaching onto an occupied cell evicts its occupant", func(t *testing.T) {
		g := NewGrid(0, 0)
		a := newStub(10, 5)
		b := newStub(8, 4)
		g.Attach(b, 1, 0, 1, 1)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(a, 1, 0, 1, 1) // moves a onto b's cell

		got := g.Children()
		if len(got) != 1 || got[0] != Widget(a) {
			t.Fatalf("children = %v, want only a after the move", got)
		}
		if sz := SizeWithin(g, Size{W: 100, H: 100}); sz != (Size{W: 10, H: 5}) {
			t.Errorf("measure = %v, want a's 10x5", sz)
		}
	})

	t.Run("overlapping a span without its origin cell is not a conflict", func(t *testing.T) {
		g := NewGrid(0, 0)
		span := newStub(20, 5)
		top := newStub(8, 4)
		g.Attach(span, 0, 0, 2, 1)
		g.Attach(top, 1, 0, 1, 1)

		if len(g.Children()) != 2 {
			t.Fatalf("children = %d, want both", len(g.Children()))
		}
		SizeWithin(g, Size{W: 100, H: 100})
		g.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 5})
		if got := g.HitTest(Point{X: 15, Y: 2}); got != Widget(top) {
			t.Errorf("hit = %v, want the later attach on top", got)
		}
	})

	t.Run("arranging before any measure lays the children out", func(t *testing.T) {
		g := NewGrid(0, 0)
		a := newStub(10, 5)
		g.Attach(a, 0, 0, 1, 1)
		g.Arrange(render.Rect{X: 0, Y: 0, W: 50, H: 50})
		if a.rect != (render.Rect{W: 50, H: 50}) {
			t.Errorf("unmeasured child arranged at %v, want the whole cell", a.rect)
		}
	})
}

func TestGridRemove(t *testing.T) {
	g := NewGrid(2, 0)
	a := newStub(10, 5)
	c := newStub(8, 4)
	g.Attach(a, 0, 0, 1, 1)
	g.Attach(c, 1, 0, 1, 1)
	if sz := SizeWithin(g, Size{W: 100, H: 100}); sz.W != 20 {
		t.Fatalf("width before removal = %d, want 20", sz.W)
	}

	if !g.Remove(a) {
		t.Fatal("Remove reported false for an attached child")
	}
	if g.Remove(a) {
		t.Error("second Remove reported true")
	}
	if sz := SizeWithin(g, Size{W: 100, H: 100}); sz.W != 0+2+8 {
		// The survivor keeps its explicit cell (column 1), so the grid
		// stays two tracks wide with an empty first column - and the
		// stale cached 20 is gone.
		t.Errorf("width after removal = %d, want 10 (cache dropped)", sz.W)
	}
	if got := g.Children(); len(got) != 1 || got[0] != Widget(c) {
		t.Errorf("children = %v, want only the survivor", got)
	}
}

func TestGridHitTest(t *testing.T) {
	g := NewGrid(2, 1)
	a := newStub(10, 5)
	c := newStub(8, 4)
	g.Attach(a, 0, 0, 1, 1)
	g.Attach(c, 1, 0, 1, 1)
	SizeWithin(g, Size{W: 100, H: 100})
	g.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 5})

	t.Run("over a child returns the child", func(t *testing.T) {
		if got := g.HitTest(Point{X: 5, Y: 2}); got != Widget(a) {
			t.Errorf("hit = %v, want the first stub", got)
		}
		if got := g.HitTest(Point{X: 15, Y: 2}); got != Widget(c) {
			t.Errorf("hit = %v, want the second stub", got)
		}
	})

	t.Run("over spacing returns the grid", func(t *testing.T) {
		if got := g.HitTest(Point{X: 11, Y: 2}); got != Widget(g) {
			t.Errorf("hit = %v, want the grid", got)
		}
	})

	t.Run("outside returns nil", func(t *testing.T) {
		if got := g.HitTest(Point{X: 200, Y: 200}); got != nil {
			t.Errorf("hit = %v, want nil", got)
		}
	})
}
