package repaint

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
)

// A plan repaints what the surface owes and what the buffer is stale
// in, but only the owed rects count as changed on screen; the owed
// rects clear, and rects outside the surface never keep a frame alive.
func TestPlan(t *testing.T) {
	var r Region
	bounds := render.Rect{W: 100, H: 50}
	r.Add(bounds, render.Rect{X: 200, W: 10, H: 10})
	if !r.Empty() {
		t.Fatal("a rect outside the surface was owed")
	}
	r.Add(bounds, render.Rect{X: 10, Y: 10, W: 20, H: 10})
	b := &buffer.Buffer{Width: 200, Height: 100, Stale: render.Rect{X: 100, Y: 0, W: 50, H: 50}}
	rects, changed, region := r.Plan(b, 2*scale.Denom)
	if want := (render.Rect{X: 20, Y: 20, W: 40, H: 20}); changed != want {
		t.Errorf("changed %v, want the owed rect at device scale %v", changed, want)
	}
	if len(rects) != 2 || rects[1] != b.Stale {
		t.Errorf("rects %v, want the owed rect and the buffer's stale one", rects)
	}
	if want := (render.Rect{X: 20, Y: 0, W: 130, H: 50}); region != want {
		t.Errorf("region %v, want %v", region, want)
	}
	if !r.Empty() {
		t.Error("the plan left the owed rects pending")
	}
}
