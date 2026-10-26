package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// flooredStub is a stub with a MinSizer floor.
type flooredStub struct {
	stub
	floor Size
}

func (s *flooredStub) MinSize() Size { return s.floor }

// TestGridSqueezeHonorsFloors pins the deficit negotiation (#67): a
// squeezed grid shrinks its tracks proportionally, stops at per-track
// floors derived from MinSizer children, and overflows only once every
// floor is reached - spanning children ride the shrunken run, and
// aligned children keep their natural sizes like squeezed Box children.
func TestGridSqueezeHonorsFloors(t *testing.T) {
	t.Run("floors stop the shrink and absorb the spill", func(t *testing.T) {
		g := NewGrid(0, 0)
		wide := &flooredStub{floor: Size{W: 30, H: 10}}
		wide.nat = Size{W: 90, H: 10}
		narrow := &flooredStub{floor: Size{W: 10, H: 10}}
		narrow.nat = Size{W: 10, H: 10}
		g.Attach(wide, 0, 0, 1, 1)
		g.Attach(narrow, 1, 0, 1, 1)
		g.Measure(Constraints{Max: Size{W: 400, H: 100}})

		// Natural 100 in a 60 rect: the narrow track's floor equals its
		// natural (no room), so the whole 40px deficit falls on the wide
		// track, which has room (90-30): 50 + 10 = 60 exactly.
		g.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 10})
		if wide.rect.W != 50 {
			t.Errorf("wide track = %d, want 50 (the whole deficit)", wide.rect.W)
		}
		if narrow.rect.X != 50 || narrow.rect.W != 10 {
			t.Errorf("narrow child = %+v, want x=50 w=10 (at its floor)", narrow.rect)
		}

		// Below every floor the grid overflows rather than collapsing:
		// a 30 rect needs a 70px cut, the wide track only has 60 of room,
		// so both rest at their floors (30 + 10) and overflow.
		g.Arrange(render.Rect{X: 0, Y: 0, W: 30, H: 10})
		if wide.rect.W != 30 || narrow.rect.W != 10 {
			t.Errorf("below the floors: %+v %+v, want both resting at their floors", wide.rect, narrow.rect)
		}
	})

	t.Run("a spanning child rides the shrunken run", func(t *testing.T) {
		g := NewGrid(0, 0)
		a := newStub(10, 5)
		c := newStub(10, 5)
		span := newStub(20, 5)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(c, 1, 0, 1, 1)
		g.Attach(span, 0, 1, 2, 1)
		g.Measure(Constraints{Max: Size{W: 100, H: 100}})

		// Natural columns 10+10 in a 12 rect: each column takes 6, so
		// the span (both columns, no spacing) gets 12.
		g.Arrange(render.Rect{X: 0, Y: 0, W: 12, H: 10})
		if a.rect.W != 6 || c.rect.X != 6 || c.rect.W != 6 {
			t.Errorf("squeezed columns = %+v %+v, want 6 and 6", a.rect, c.rect)
		}
		if span.rect != (render.Rect{X: 0, Y: 5, W: 12, H: 5}) {
			t.Errorf("spanning child = %+v, want the full squeezed 12", span.rect)
		}
	})

	t.Run("an aligned child keeps its natural size in a shrunken cell", func(t *testing.T) {
		g := NewGrid(0, 0)
		a := newStub(10, 5)
		c := newStub(10, 5)
		g.Attach(a, 0, 0, 1, 1)
		g.Attach(c, 1, 0, 1, 1)
		g.SetAlign(c, AlignStart, AlignFill)
		g.Measure(Constraints{Max: Size{W: 100, H: 100}})
		g.Arrange(render.Rect{X: 0, Y: 0, W: 10, H: 5})

		// The fill child takes the squeezed 5-wide cell; the start
		// child keeps its natural 10 and overflows, exactly like a
		// squeezed Box child.
		if a.rect != (render.Rect{X: 0, Y: 0, W: 5, H: 5}) {
			t.Errorf("fill child = %+v, want the shrunken 5-wide cell", a.rect)
		}
		if c.rect != (render.Rect{X: 5, Y: 0, W: 10, H: 5}) {
			t.Errorf("aligned child = %+v, want the natural 10 kept", c.rect)
		}
	})

	t.Run("the grid reports its floors for nested squeezing", func(t *testing.T) {
		inner := NewGrid(0, 0)
		wide := &flooredStub{floor: Size{W: 30, H: 10}}
		wide.nat = Size{W: 90, H: 10}
		inner.Attach(wide, 0, 0, 1, 1)
		inner.Measure(Constraints{Max: Size{W: 400, H: 100}})

		outer := NewGrid(0, 0)
		outer.Attach(inner, 0, 0, 1, 1)
		outer.Measure(Constraints{Max: Size{W: 400, H: 100}})
		if got := outer.colMin[0]; got != 30 {
			t.Errorf("outer column floor = %d, want the inner grid's 30", got)
		}
	})
}

// TestGridMinSizerLabelFloor pins the real MinSizer consumer: a
// wrapping label's widest-token floor stops its column, where a plain
// stub with the same natural width squeezes to nothing.
func TestGridMinSizerLabelFloor(t *testing.T) {
	g := NewGrid(0, 0)
	label := NewLabel(entryFace(t), 12, "alpha beta", render.RGB(0, 0, 0))
	label.SetWrap(true)
	plain := newStub(100, 16)
	g.Attach(label, 0, 0, 1, 1)
	g.Attach(plain, 1, 0, 1, 1)
	g.Measure(Constraints{Max: Size{W: 400, H: 100}})

	floor := label.MinSize().W
	if floor <= 0 {
		t.Fatalf("wrapping label floor = %d, want the widest token", floor)
	}
	// A very narrow rect: the label column rests at its token floor and
	// the plain column takes what little is left.
	g.Arrange(render.Rect{X: 0, Y: 0, W: floor + 4, H: 20})
	if got := label.Bounds().W; got != floor {
		t.Errorf("label column = %d, want its %d token floor", got, floor)
	}
	if got := plain.rect.W; got != 4 {
		t.Errorf("plain column = %d, want the remaining 4", got)
	}
}
