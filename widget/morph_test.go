package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

type backdropProbe struct {
	node
	shape []render.Rect
}

func (b *backdropProbe) BackdropRegion() []render.Rect { return b.shape }
func (b *backdropProbe) HitTest(Point) Widget          { return nil }
func (b *backdropProbe) Paint(*render.Canvas)          {}
func (b *backdropProbe) Measure(Axis, int) Sizes       { return Sizes{} }
func (b *backdropProbe) Arrange(r render.Rect)         { b.node.Arrange(r) }

func extent(rects []render.Rect) render.Rect {
	var u render.Rect
	for _, r := range rects {
		u = u.Union(r)
	}
	return u
}

// The blur region never leads the shape it backs: hidden there is none,
// it grows with the unrolling clip and the scale, and at rest it is the
// content's exactly.
func TestMorphBackdropRegionFollowsTheReveal(t *testing.T) {
	probe := &backdropProbe{shape: []render.Rect{{X: 10, Y: 10, W: 100, H: 200}}}
	m := NewMorph(probe)
	m.SetOrigin(0.5, 0)
	m.SetPlate(10, nil)
	m.Arrange(render.Rect{W: 120, H: 220})

	m.SetReveal(0, true)
	if got := m.BackdropRegion(); len(got) != 0 {
		t.Fatalf("hidden: region %v, want none", got)
	}
	m.SetReveal(0.05, true)
	early := extent(m.BackdropRegion())
	if early.H >= 100 || early.W >= 100 || early.Y != 10 {
		t.Fatalf("early in the motion: region %+v, want a short, scaled one from the top edge", early)
	}
	m.SetReveal(0.5, true)
	mid := extent(m.BackdropRegion())
	if mid.H <= early.H || mid.W <= early.W || mid.H >= 200 {
		t.Fatalf("mid motion: region %+v after %+v, want it grown but not whole", mid, early)
	}
	m.SetReveal(1, true)
	if got := m.BackdropRegion(); !slices.Equal(got, probe.shape) {
		t.Fatalf("at rest: region %v, want the content's %v", got, probe.shape)
	}
	m.SetReveal(0.5, false)
	if got := m.BackdropRegion(); !slices.Equal(got, probe.shape) {
		t.Fatalf("a plain fade: region %v, want the content's %v", got, probe.shape)
	}
}

// The motion cuts the content's own shape rather than replacing it: a
// transparent surface holding one small glass card frosts the card alone.
func TestMorphBackdropRegionKeepsTheContentsShape(t *testing.T) {
	probe := &backdropProbe{shape: []render.Rect{{X: 40, Y: 20, W: 20, H: 20}}}
	m := NewMorph(probe)
	m.SetOrigin(0.5, 0)
	m.Arrange(render.Rect{W: 100, H: 100})
	m.SetReveal(0.8, true)
	got := extent(m.BackdropRegion())
	if got.W > 21 || got.H > 21 || got.X < 38 {
		t.Fatalf("region %+v, want the card, scaled", got)
	}
}

func TestMorphFadeLeadsTheMotion(t *testing.T) {
	m := NewMorph(&backdropProbe{})
	for _, c := range []struct {
		reveal float64
		morph  bool
		want   float64
	}{{0, true, 0}, {0.5, true, 0.8}, {1, true, 1}, {1.05, true, 1}, {-0.1, true, 0}, {0.5, false, 0.5}} {
		m.SetReveal(c.reveal, c.morph)
		if got := m.Fade(); got != c.want {
			t.Errorf("reveal %v morph %v: fade %v, want %v", c.reveal, c.morph, got, c.want)
		}
	}
}
