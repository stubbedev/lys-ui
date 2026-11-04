package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func stepDamage(t *testing.T, c *animClock, root Widget, r render.Rect) []render.Rect {
	t.Helper()
	root.Arrange(r)
	CollectDamage(root)
	cv := render.New(make([]byte, render.Stride(r.W)*r.H), render.Stride(r.W), r.W, r.H)
	root.Paint(cv)
	CollectDamage(root)
	if !c.step() {
		t.Fatal("the indicator scheduled no frame")
	}
	root.Arrange(r)
	rects, _ := CollectDamage(root)
	return rects
}

func TestSpinnerDamagesItsSquareInStretchedBounds(t *testing.T) {
	c := pinAnimClock(t)
	s := NewSpinner(24)
	s.SetSpinning(true)
	root := NewBox(Column, 0, 0).Append(s, true)
	t.Cleanup(Host(root))
	r := render.Rect{W: 800, H: 600}
	SizeWithin(root, Size{W: 800, H: 600})
	rects := stepDamage(t, c, root, r)
	want := alignRect(s.Bounds(), Size{W: 600, H: 600}, AlignCenter, AlignCenter)
	if s.Bounds() != r {
		t.Fatalf("spinner bounds %+v, want it stretched to %+v", s.Bounds(), r)
	}
	if len(rects) != 1 || rects[0] != want {
		t.Errorf("spinner step damaged %+v, want only the square %+v", rects, want)
	}
}

func TestSpinnerDamageIsItsSizeWhenCentered(t *testing.T) {
	c := pinAnimClock(t)
	s := NewSpinner(24)
	s.SetSpinning(true)
	s.SetHAlign(AlignCenter)
	root := NewBox(Row, 0, 0).Append(s, true)
	t.Cleanup(Host(root))
	r := render.Rect{W: 800, H: 24}
	SizeWithin(root, Size{W: 800, H: 24})
	rects := stepDamage(t, c, root, r)
	if got := DamageArea(rects); got != 24*24 {
		t.Errorf("spinner step damaged %d px (%+v), want 24x24", got, rects)
	}
}

func TestProgressRingEasesInsideItsSquare(t *testing.T) {
	c := pinAnimClock(t)
	ring := NewProgressRing(0)
	root := NewBox(Column, 0, 0).Append(ring, true)
	t.Cleanup(Host(root))
	r := render.Rect{W: 400, H: 100}
	SizeWithin(root, Size{W: 400, H: 100})
	root.Arrange(r)
	CollectDamage(root)
	ring.SetValue(0.5)
	if !c.step() {
		t.Fatal("the ring scheduled no frame")
	}
	rects, _ := CollectDamage(root)
	if got := DamageArea(rects); got != 100*100 {
		t.Errorf("ring step damaged %d px (%+v), want its 100x100 square", got, rects)
	}
}

func TestProgressPulseDamagesItsTroughAlone(t *testing.T) {
	c := pinAnimClock(t)
	bar := NewProgressBar(0)
	bar.SetPulsing(true)
	root := NewBox(Column, 0, 0).Append(bar, true)
	t.Cleanup(Host(root))
	r := render.Rect{W: 400, H: 100}
	SizeWithin(root, Size{W: 400, H: 100})
	rects := stepDamage(t, c, root, r)
	if got := render.UnionAll(rects); got != bar.trough.bounds || bar.trough.bounds.H >= r.H {
		t.Errorf("pulse step damaged %+v, want only the trough %+v of the %+v bar", got, bar.trough.bounds, bar.Bounds())
	}
}
