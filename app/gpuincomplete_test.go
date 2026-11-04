package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/widget"
)

func TestAnIncompleteGPUFrameGoesOutAndStaysOwed(t *testing.T) {
	label := widget.NewSpacer(40, 20)
	h := newPaintHarness(label, 120, 60)
	w := h.wnd
	incomplete := false
	w.renderer = &gpuRenderer{
		name: "test", rec: render.NewRecorder(1, 1, 1, 1), wake: func(time.Duration) {},
		render: func(*render.List, *gpu.Target, []gpu.Carry) (gpuFrame, error) {
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
