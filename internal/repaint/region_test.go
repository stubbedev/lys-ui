package repaint

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/testbuild"
	"github.com/stubbedev/lys-ui/widget"
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
	stale := render.Rect{X: 100, Y: 0, W: 50, H: 50}
	b := &buffer.Buffer{Width: 200, Height: 100, Stale: buffer.RegionOf(stale)}
	rects, changed, regions := r.Plan(b, 2*scale.Denom, false)
	if want := (render.Rect{X: 20, Y: 20, W: 40, H: 20}); len(changed.Rects()) != 1 || changed.Rects()[0] != want {
		t.Errorf("changed %v, want the owed rect at device scale %v", changed.Rects(), want)
	}
	if len(rects) != 2 || rects[1] != stale {
		t.Errorf("rects %v, want the owed rect and the buffer's stale one", rects)
	}
	if want := (render.Rect{X: 20, Y: 0, W: 130, H: 50}); len(regions) != 1 || regions[0] != want {
		t.Errorf("regions %v, want the one box %v: it is hardly bigger than the rects", regions, want)
	}
	if !r.Empty() {
		t.Error("the plan left the owed rects pending")
	}
}

// Draining a damaged tree reuses the region's buffers: a frame that
// collects and plans its damage allocates nothing. Regression: every
// drain built a fresh slice of the tree's rects.
func TestCollectAllocatesNothing(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	root := widget.NewBox(widget.Column, 0, 0)
	bar := widget.NewProgressBar(0)
	root.Append(bar, false)
	root.Arrange(render.Rect{W: 100, H: 50})
	bounds := render.Rect{W: 100, H: 50}
	b := &buffer.Buffer{Width: 100, Height: 50}
	var r Region
	frame := func() {
		bar.Invalidate()
		if !r.Collect(root, bounds) {
			t.Fatal("an invalidated bar left nothing to collect")
		}
		r.Plan(b, scale.Denom, false)
	}
	frame()
	if allocs := testing.AllocsPerRun(50, frame); allocs != 0 {
		t.Errorf("a damaged frame allocates %v, want nothing", allocs)
	}
}

// A clock and a gauge at opposite ends of a bar repaint themselves, not the
// bar between them.
func TestDisjointDamagePaintsApart(t *testing.T) {
	var r Region
	bounds := render.Rect{W: 1000, H: 30}
	left, right := render.Rect{X: 0, W: 40, H: 30}, render.Rect{X: 960, W: 40, H: 30}
	r.Add(bounds, left, right)
	b := &buffer.Buffer{Width: 1000, Height: 30}
	_, _, regions := r.Plan(b, scale.Denom, false)
	if len(regions) != 2 || regions[0] != left || regions[1] != right {
		t.Errorf("regions %v, want the two rects", regions)
	}
	if got := Area(regions); got != 2400 {
		t.Errorf("painted %d pixels, want 2400", got)
	}
}

func TestDropCovered(t *testing.T) {
	var r Region
	bounds := render.Rect{W: 100, H: 100}
	r.Add(bounds, render.Rect{X: 10, Y: 10, W: 20, H: 20})
	past := render.Rect{X: 10, Y: 10, W: 10, H: 10}
	b := &buffer.Buffer{Width: 200, Height: 200, Stale: buffer.RegionOf(past)}
	b.Stale.Add(render.Rect{X: 40, Y: 40, W: 10, H: 10})
	b.Stale.Add(render.Rect{X: 120, Y: 120, W: 10, H: 10})
	r.DropCovered(b, 2*scale.Denom)
	got := b.Stale.Rects()
	if len(got) == 0 || slices.Contains(got, render.Rect{X: 40, Y: 40, W: 10, H: 10}) {
		t.Errorf("stale %v, want the rect inside the owed one at device scale gone", got)
	}
	if !slices.Contains(got, render.Rect{X: 120, Y: 120, W: 10, H: 10}) {
		t.Errorf("stale %v lost a rect past the owed one", got)
	}
}

func TestAPlanCarryingPaintsNoStaleness(t *testing.T) {
	var r Region
	bounds := render.Rect{W: 100, H: 100}
	owed := render.Rect{X: 10, Y: 10, W: 5, H: 5}
	r.Add(bounds, owed)
	b := &buffer.Buffer{Width: 100, Height: 100, Stale: buffer.RegionOf(render.Rect{X: 80, Y: 80, W: 5, H: 5})}
	rects, _, regions := r.Plan(b, scale.Denom, true)
	if len(rects) != 1 || rects[0] != owed || len(regions) != 1 || regions[0] != owed {
		t.Errorf("rects %v, regions %v: want the owed rect alone, the carry brings the rest", rects, regions)
	}
	if b.Stale.Empty() {
		t.Error("planning a carry cleared the staleness; only the presented frame may")
	}
}

func TestRefuseOwesThePlanAgain(t *testing.T) {
	var r Region
	bounds := render.Rect{W: 100, H: 100}
	owed := render.Rect{X: 10, Y: 10, W: 5, H: 5}
	r.Add(bounds, owed)
	b := &buffer.Buffer{Width: 100, Height: 100}
	r.Plan(b, scale.Denom, false)
	if !r.Empty() {
		t.Fatal("a plan left its rects owed")
	}
	r.Refuse()
	if r.Empty() {
		t.Fatal("a refused plan owes nothing")
	}
	if rects, _, _ := r.Plan(b, scale.Denom, false); len(rects) != 1 || rects[0] != owed {
		t.Errorf("rects %v after the refusal, want %v", rects, owed)
	}
	r.Refuse()
	r.Refuse()
	r.Plan(b, scale.Denom, false)
	if rects, _, _ := r.Plan(b, scale.Denom, false); len(rects) != 0 {
		t.Errorf("a second refusal owed the plan twice: %v", rects)
	}
}
