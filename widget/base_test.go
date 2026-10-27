package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// probe is a consumer-defined widget on Base: it records the passes it
// receives so the test can pin the contract.
type probe struct {
	Base
	arranged render.Rect
	painted  int
}

func (p *probe) Measure(axis Axis, forSize int) Sizes {
	return FixedSizes(Size{W: 10, H: 5}.Along(axis))
}

func (p *probe) Arrange(r render.Rect) {
	p.node.Arrange(r)
	p.arranged = r
}

func (p *probe) Paint(cv *render.Canvas) { p.painted++ }

func (p *probe) HitTest(point Point) Widget { return p.HitLeaf(p, point) }

func TestBaseCarriesBoundsAndHits(t *testing.T) {
	w := &probe{}
	rect := render.Rect{X: 3, Y: 4, W: 10, H: 5}
	w.Arrange(rect)
	if w.Bounds() != rect {
		t.Fatalf("Bounds = %+v, want %+v", w.Bounds(), rect)
	}
	if got := w.HitTest(Point{X: 5, Y: 6}); got == nil {
		t.Fatal("HitTest inside the bounds: want the widget, got nil")
	}
	if got := w.HitTest(Point{X: 500, Y: 500}); got != nil {
		t.Fatalf("HitTest outside the bounds = %v, want nil", got)
	}
}

func TestBaseInvalidationDrivesDamage(t *testing.T) {
	w := &probe{}
	w.Arrange(render.Rect{X: 0, Y: 0, W: 10, H: 5})
	rects, dirty := CollectDamage(w)
	if !dirty || len(rects) == 0 {
		t.Fatalf("fresh widget: CollectDamage = %+v dirty=%v, want the arranged rect", rects, dirty)
	}
	rects, dirty = CollectDamage(w)
	if dirty {
		t.Fatalf("clean widget: CollectDamage = %+v dirty=%v, want nothing to paint", rects, dirty)
	}
	w.Invalidate()
	if _, dirty := CollectDamage(w); !dirty {
		t.Fatal("Invalidate: want the widget dirty again")
	}
}

func TestBaseJoinstheCascade(t *testing.T) {
	parent := NewBox(Row, 0, 0)
	child := &probe{}
	parent.Append(child, false)
	parent.Arrange(render.Rect{X: 0, Y: 0, W: 10, H: 5})
	if child.Parent() == nil {
		t.Fatal("arranged child: Parent = nil, want the arranging box")
	}
	child.AddClass("marked")
	if !HasClass(child, "marked") {
		t.Fatal("AddClass on a Base widget: class not carried")
	}
}
