package app

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/widget"
)

type lossRig struct {
	t       *testing.T
	a       *Application
	h       *paintHarness
	bar     *widget.ProgressBar
	woke    chan struct{}
	opens   int
	openErr error
	lose    bool
	gpuBufs int
	cpuBufs int
}

func newLossRig(t *testing.T) *lossRig {
	t.Helper()
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	rig := &lossRig{t: t, bar: widget.NewProgressBar(0), woke: make(chan struct{}, 8)}
	rig.h = newPaintHarness(rig.bar, 40, 30)
	rig.a = &Application{windows: []*hostWindow{rig.h.wnd}}
	rig.a.wake = func(time.Duration) { rig.woke <- struct{}{} }
	t.Cleanup(rig.a.closeRenderer)
	w := rig.h.wnd
	fakes := w.newBuffer
	w.newBuffer = func() (*buffer.Buffer, error) {
		if w.renderer == nil {
			rig.cpuBufs++
			return fakes()
		}
		tg, err := w.renderer.dev.NewOffscreen(40, 30)
		if err != nil {
			return nil, err
		}
		rig.gpuBufs++
		return &buffer.Buffer{Width: 40, Height: 30, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 40, H: 30}), Target: tg}, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	rig.a.startRenderer(rendererPlan{}, rig.open)
	rig.land()
	return rig
}

func (rig *lossRig) open(rendererPlan) (*gpuRenderer, error) {
	dev, err := gpu.Open()
	if err != nil {
		rig.openErr = err
		return nil, err
	}
	rig.opens++
	r := deviceRenderer(dev)
	r.render = func(l *render.List, tg *gpu.Target, carry []gpu.Carry, explicit bool) (gpuFrame, error) {
		if rig.lose {
			rig.lose = false
			return gpuFrame{}, fmt.Errorf("app test: injected: %w", gpu.ErrDeviceLost)
		}
		return r.deviceFrame(l, tg, carry, explicit)
	}
	return r, nil
}

func (rig *lossRig) land() {
	rig.t.Helper()
	deadline := time.After(time.Minute)
	for rig.a.gpu == nil {
		select {
		case <-rig.woke:
		case <-deadline:
			rig.t.Fatal("the device worker never answered")
		}
		rig.a.gpuOpening.Wait()
		rig.drain()
		if rig.openErr != nil {
			if gputest.Enabled() {
				rig.t.Fatal(rig.openErr)
			}
			rig.t.Skipf("no GPU: %v", rig.openErr)
		}
	}
}

func (rig *lossRig) drain() {
	for _, fn := range rig.a.queues.drain() {
		fn()
	}
}

func (rig *lossRig) frame(v float64) []render.Rect {
	rig.t.Helper()
	rig.bar.SetValue(v)
	_, damage := rig.h.frame()
	return damage
}

func wholeWindow(damage []render.Rect) bool {
	return slices.Contains(damage, render.Rect{W: 40, H: 30})
}

func (rig *lossRig) loseFrame(v float64) {
	rig.t.Helper()
	rig.lose = true
	rig.frame(v)
	if rig.lose {
		rig.t.Fatal("the frame never reached the GPU")
	}
	if rig.h.wnd.renderer != nil {
		rig.t.Fatal("the window stayed on the lost device")
	}
}

func TestALostDeviceIsReopenedAndTheWindowsReturnToTheGPU(t *testing.T) {
	rig := newLossRig(t)
	w := rig.h.wnd
	if w.renderer != rig.a.gpu {
		t.Fatal("the window did not move to the renderer")
	}
	rig.frame(0.1)
	if rig.gpuBufs != 1 {
		t.Fatalf("%d GPU buffers before the loss, want 1", rig.gpuBufs)
	}
	lost := rig.a.gpu

	rig.loseFrame(0.2)
	cpu := rig.cpuBufs
	if damage := rig.frame(0.3); !wholeWindow(damage) || rig.cpuBufs != cpu+1 {
		t.Fatalf("the frame after the loss damaged %v in %d new CPU buffers, want the whole window in one", damage, rig.cpuBufs-cpu)
	}
	rig.drain()
	if rig.a.gpu != nil || !lost.closed {
		t.Fatal("the lost device was kept")
	}
	rig.land()
	if rig.opens != 2 {
		t.Fatalf("%d devices opened, want the first and one after the loss", rig.opens)
	}
	if w.renderer != rig.a.gpu || w.renderer == lost {
		t.Fatal("the window did not move to the new device")
	}
	gpu := rig.gpuBufs
	if n, damage := rig.h.frame(); n != 40*30 || !wholeWindow(damage) || rig.gpuBufs != gpu+1 {
		t.Fatalf("the first frame on the new device painted %d px, damaged %v, in %d new GPU buffers, want the whole window in one", n, damage, rig.gpuBufs-gpu)
	}
	if n, _ := rig.h.frame(); n != 0 {
		t.Errorf("the frame after the full repaint painted %d px, want nothing", n)
	}
	if rig.opens != 2 {
		t.Errorf("%d devices opened, want 2", rig.opens)
	}
}

func TestADeviceLostTooOftenLeavesTheWindowsOnTheCPU(t *testing.T) {
	var log bytes.Buffer
	logutil.Set(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { logutil.Set(nil) })
	rig := newLossRig(t)
	for i := range gpu.LossLimit {
		rig.loseFrame(float64(i) / 10)
		rig.drain()
		rig.land()
	}
	rig.loseFrame(0.9)
	rig.drain()
	if rig.a.gpu != nil || rig.h.wnd.renderer != nil {
		t.Fatal("a device lost past the limit was reopened")
	}
	rig.a.gpuOpening.Wait()
	rig.drain()
	if rig.a.gpu != nil || rig.opens != gpu.LossLimit+1 {
		t.Errorf("%d devices opened, want %d", rig.opens, gpu.LossLimit+1)
	}
	cpu := rig.cpuBufs
	if damage := rig.frame(0.95); !wholeWindow(damage) || rig.cpuBufs != cpu+1 || rig.h.wnd.renderer != nil {
		t.Errorf("after the last loss the window damaged %v in %d new CPU buffers with renderer %v, want a whole CPU frame", damage, rig.cpuBufs-cpu, rig.h.wnd.renderer)
	}
	out := log.String()
	if n := strings.Count(out, "the GPU device was lost;"); n != gpu.LossLimit {
		t.Errorf("%d recovered losses logged, want %d:\n%s", n, gpu.LossLimit, out)
	}
	if !strings.Contains(out, "lost too often") || !strings.Contains(out, fmt.Sprintf("losses=%d", gpu.LossLimit+1)) {
		t.Errorf("no final loss naming all %d:\n%s", gpu.LossLimit+1, out)
	}
	last := out[strings.Index(out, "lost too often"):]
	if n := strings.Count(last, "injected"); n != gpu.LossLimit+1 {
		t.Errorf("the final loss reported %d losses, want all %d:\n%s", n, gpu.LossLimit+1, last)
	}
	if !errors.Is(rig.a.gpuLosses.Err(), gpu.ErrDeviceLost) {
		t.Error("the recorded losses lost their cause")
	}
}
