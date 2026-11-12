package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/widget"
)

func TestAnIncompleteGPUFrameGoesOutAndStaysOwed(t *testing.T) {
	label := widget.NewSpacer(40, 20)
	h := newPaintHarness(label, 120, 60)
	w := h.wnd
	incomplete := false
	w.renderer = &gpuRenderer{
		name: "test", rec: render.NewRecorder(1, 1, 1, 1), wake: func(time.Duration) {},
		render: func(*render.List, *gpu.Target, []gpu.Carry, bool) (gpuFrame, error) {
			return gpuFrame{incomplete: incomplete}, nil
		},
	}
	w.newBuffer = func() (*buffer.Buffer, error) {
		b := &buffer.Buffer{Width: 120, Height: 60, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 120, H: 60}), Target: &gpu.Target{}}
		h.bufs = append(h.bufs, b)
		return b, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	h.frame()
	if !w.damage.Empty() {
		t.Fatal("a complete frame left damage owed")
	}

	incomplete = true
	label.InvalidateRect(render.Rect{X: 4, Y: 4, W: 10, H: 10})
	commits := h.surf.commits
	h.frame()
	if h.surf.commits != commits+1 {
		t.Errorf("an incomplete frame made %d commits, want 1: what it drew goes out", h.surf.commits-commits)
	}
	if w.damage.Empty() || !w.dirty {
		t.Fatal("an incomplete frame's damage was not kept for the next frame")
	}

	incomplete = false
	commits = h.surf.commits
	h.frame()
	if h.surf.commits != commits+1 || w.paintedPixels == 0 {
		t.Errorf("the frame after an incomplete one made %d commits painting %d px, want it to repaint what was owed", h.surf.commits-commits, w.paintedPixels)
	}
	if !w.damage.Empty() {
		t.Error("damage still owed after a complete frame")
	}
}

type countingLeaf struct {
	widget.Base
	paints int
}

func (c *countingLeaf) Measure(widget.Axis, int) widget.Sizes { return widget.FixedSizes(40) }

func (c *countingLeaf) Paint(cv *render.Canvas) {
	c.paints++
	cv.FillRect(c.Bounds(), render.RGB(200, 40, 40))
}

func (c *countingLeaf) HitTest(p widget.Point) widget.Widget { return c.HitLeaf(c, p) }

func TestARevealerLayerRecordedInAnIncompleteGPUFrameIsRecordedAgain(t *testing.T) {
	base := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return base }))
	leaf := &countingLeaf{}
	rev := widget.NewRevealer(leaf)
	rev.SetTransition(widget.RevealZoom)
	h := newPaintHarness(rev, 120, 60)
	w := h.wnd
	incomplete := false
	w.renderer = &gpuRenderer{
		name: "test", rec: render.NewRecorder(1, 1, 1, 1), wake: func(time.Duration) {},
		render: func(l *render.List, _ *gpu.Target, _ []gpu.Carry, _ bool) (gpuFrame, error) {
			l.Paint(render.NewScaled(make([]byte, render.Stride(120)*60), render.Stride(120), 120, 60, scale.Denom, scale.Denom))
			return gpuFrame{incomplete: incomplete}, nil
		},
	}
	w.newBuffer = func() (*buffer.Buffer, error) {
		b := &buffer.Buffer{Width: 120, Height: 60, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 120, H: 60}), Target: &gpu.Target{}}
		h.bufs = append(h.bufs, b)
		return b, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	h.frame()
	rev.SetRevealed(true)
	w.anim.Tick(base.Add(100 * time.Millisecond))
	h.frame()
	rev.Invalidate()
	h.frame()
	if leaf.paints != 1 {
		t.Fatalf("a zoom held still painted its child %d times over two frames, want once", leaf.paints)
	}

	incomplete = true
	leaf.Invalidate()
	h.frame()
	incomplete = false
	rev.Invalidate()
	h.frame()
	if leaf.paints != 3 {
		t.Errorf("the child painted %d times in all, want its layer recorded again after the incomplete frame", leaf.paints)
	}
}
