package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/waittest"
	"github.com/stubbedev/lys-ui/widget"
)

func TestTheGPUDeviceOpensOffTheLoopAndTheWindowsMoveToIt(t *testing.T) {
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	h := newPaintHarness(widget.NewSpacer(40, 30), 40, 30)
	woke := make(chan struct{}, 8)
	a := &Application{windows: []*hostWindow{h.wnd}}
	a.wake = func(time.Duration) { woke <- struct{}{} }
	release := make(chan struct{})
	opened := 0
	a.startRenderer(rendererPlan{}, func(rendererPlan) (*gpuRenderer, error) {
		<-release
		dev, err := gpu.Open()
		if err != nil {
			return nil, err
		}
		opened++
		r := deviceRenderer(dev)
		return r, nil
	})
	if opened != 0 || a.gpu != nil {
		t.Fatal("the device opened where it was asked")
	}
	h.frame()
	if h.wnd.renderer != nil || h.bufs[0].Data == nil {
		t.Fatal("the window's first frame waited for the GPU instead of drawing on the CPU")
	}
	close(release)
	waittest.ForSlow(t, woke, "the worker's answer")
	for _, fn := range a.queues.drain() {
		fn()
	}
	if a.gpu == nil {
		if gputest.Enabled() {
			t.Fatal("no renderer landed")
		}
		t.Skip("no GPU")
	}
	t.Cleanup(a.closeRenderer)
	if h.wnd.renderer != a.gpu {
		t.Error("the window stayed on the CPU after the renderer landed")
	}
	gpuBufs := 0
	h.wnd.newBuffer = func() (*buffer.Buffer, error) {
		gpuBufs++
		tg, err := a.gpu.dev.NewOffscreen(40, 30)
		if err != nil {
			return nil, err
		}
		return &buffer.Buffer{Width: 40, Height: 30, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 40, H: 30}), Target: tg}, nil
	}
	h.wnd.pool = buffer.New(h.wnd.allocator(), 3)
	h.frame()
	if gpuBufs != 1 || h.wnd.paintedPixels != 40*30 {
		t.Errorf("the frame after the move made %d GPU buffers and painted %d px, want one and the whole window", gpuBufs, h.wnd.paintedPixels)
	}
}

func TestARendererLandingAfterTheLoopEndedIsClosed(t *testing.T) {
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	a := &Application{}
	a.wake = func(time.Duration) {}
	var dev *gpu.Device
	a.startRenderer(rendererPlan{}, func(rendererPlan) (*gpuRenderer, error) {
		d, err := gpu.Open()
		if err != nil {
			return nil, err
		}
		dev = d
		return deviceRenderer(d), nil
	})
	a.closeRenderer()
	if dev == nil {
		if gputest.Enabled() {
			t.Fatal("no device opened")
		}
		t.Skip("no GPU")
	}
	if a.gpuLanding.Load() != nil {
		t.Error("the renderer that landed after the loop is still waiting for it")
	}
	for _, fn := range a.queues.drain() {
		fn()
	}
	if a.gpu != nil {
		t.Error("a renderer landed after the loop was installed")
	}
}
