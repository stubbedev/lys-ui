package repaint

import (
	"testing"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
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
	b := &buffer.Buffer{Width: 200, Height: 100, Stale: render.Rect{X: 100, Y: 0, W: 50, H: 50}}
	rects, changed, regions := r.Plan(b, 2*scale.Denom)
	if want := (render.Rect{X: 20, Y: 20, W: 40, H: 20}); changed != want {
		t.Errorf("changed %v, want the owed rect at device scale %v", changed, want)
	}
	if len(rects) != 2 || rects[1] != b.Stale {
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
	if debug.Enabled {
		t.Skip("lysdebug names every drained rect")
	}
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
		r.Plan(b, scale.Denom)
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
	_, _, regions := r.Plan(b, scale.Denom)
	if len(regions) != 2 || regions[0] != left || regions[1] != right {
		t.Errorf("regions %v, want the two rects", regions)
	}
	if got := Area(regions); got != 2400 {
		t.Errorf("painted %d pixels, want 2400", got)
	}
}

// Rects that touch, or whose box is hardly bigger, paint as one walk.
func TestNearDamagePaintsAsOne(t *testing.T) {
	for name, rects := range map[string][]render.Rect{
		"overlapping": {{X: 0, W: 40, H: 30}, {X: 20, W: 40, H: 30}},
		"cross":       {{X: 0, Y: 45, W: 100, H: 10}, {X: 45, Y: 0, W: 10, H: 100}},
		"neighbours":  {{X: 0, W: 40, H: 30}, {X: 50, W: 40, H: 30}},
	} {
		got := coalesce(nil, rects)
		if len(got) != 1 {
			t.Errorf("%s: %v, want one region", name, got)
		}
	}
}

// However scattered the damage, a frame walks the tree a bounded number of
// times, and every rect stays covered.
func TestRegionsAreBoundedAndCoverEverything(t *testing.T) {
	var rects []render.Rect
	for i := range 40 {
		rects = append(rects, render.Rect{X: i * 100, Y: i * 100, W: 5, H: 5})
	}
	got := coalesce(nil, rects)
	if len(got) > maxRegions {
		t.Errorf("%d regions, want at most %d", len(got), maxRegions)
	}
	for _, want := range rects {
		covered := false
		for _, g := range got {
			covered = covered || g.Intersect(want) == want
		}
		if !covered {
			t.Errorf("%v is in no region of %v", want, got)
		}
	}
}

func TestCovers(t *testing.T) {
	var r Region
	bounds := render.Rect{W: 100, H: 100}
	r.Add(bounds, render.Rect{X: 10, Y: 10, W: 20, H: 20})
	if !r.Covers(render.Rect{X: 40, Y: 40, W: 10, H: 10}, 2*scale.Denom) {
		t.Error("a stale rect inside the owed one at device scale must be covered")
	}
	if r.Covers(render.Rect{X: 10, Y: 10, W: 10, H: 10}, 2*scale.Denom) {
		t.Error("a stale rect past the owed one must still be copied")
	}
	if r.Covers(render.Rect{}, scale.Denom) {
		t.Error("nothing stale has nothing to cover")
	}
}
