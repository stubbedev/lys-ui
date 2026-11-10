package app

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/logutil"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/buffer"
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

type videoStub struct {
	rig         *lossRig
	s           *textureSurface
	lostOpen    []bool
	wokenOn     []*gpuRenderer
	wokePending []bool
}

func (rig *lossRig) video() *videoStub {
	v := &videoStub{rig: rig, s: &textureSurface{video: &widget.Video{}}}
	v.s.app, v.s.wake, v.s.lost = rig.a, v.wake, v.lost
	v.attach(rig.a.gpu)
	return v
}

func (v *videoStub) attach(r *gpuRenderer) {
	v.s.r = r
	r.videos = append(r.videos, v.s)
}

func (v *videoStub) lost() {
	r := v.s.r
	v.lostOpen = append(v.lostOpen, !r.closed)
	r.videos = slices.DeleteFunc(r.videos, func(s *textureSurface) bool { return s == v.s })
}

func (v *videoStub) wake() {
	a := v.rig.a
	v.wokenOn = append(v.wokenOn, a.gpu)
	v.wokePending = append(v.wokePending, a.gpuPending)
	if a.gpu != nil && !a.gpu.Failed() {
		v.attach(a.gpu)
	}
}

var errVideoLoss = fmt.Errorf("app test: a video's decoding: %w", gpu.ErrDeviceLost)

func TestAGPUVideoIsToldOfALostDeviceBeforeItClosesAndWokenOnTheNext(t *testing.T) {
	rig := newLossRig(t)
	v := rig.video()
	lost := rig.a.gpu

	rig.loseFrame(0.2)
	rig.drain()
	if len(v.lostOpen) != 1 || !v.lostOpen[0] || !lost.closed {
		t.Fatalf("the video heard of the loss %v (true: with the device still open), device closed %v; want once, before the close", v.lostOpen, lost.closed)
	}
	if len(v.wokenOn) != 0 {
		t.Fatalf("the video was woken %d times before the next device opened", len(v.wokenOn))
	}
	if v.s.err != nil {
		t.Errorf("the surface of a lost device was failed: %v", v.s.err)
	}
	rig.land()
	if len(v.wokenOn) != 1 || v.wokenOn[0] != rig.a.gpu || v.wokenOn[0] == lost {
		t.Fatalf("the video was woken on %v, want once on the new device", v.wokenOn)
	}
	if !slices.Contains(rig.a.gpu.videos, v.s) {
		t.Error("the video is not on the new device")
	}
}

func TestAGPUDeviceLossAVideoFindsIsAnsweredLikeAFramesOwn(t *testing.T) {
	for _, c := range []struct {
		name   string
		report func(v *videoStub) error
	}{
		{"found decoding", func(v *videoStub) error {
			v.s.DeviceLost(errVideoLoss)
			return nil
		}},
		{"found drawing a frame", func(v *videoStub) error {
			v.s.bounds = render.Rect{W: 4, H: 4}
			v.s.ring[0].wl = &wl.Buffer{}
			v.s.r.video = func(*gpu.Target, []image.Rectangle, *gpu.VideoFrame) error { return errVideoLoss }
			shown, err := v.s.Present(nil)
			if shown {
				return errors.New("a frame the lost device never drew was shown")
			}
			return err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			rig := newLossRig(t)
			v := rig.video()
			lost := rig.a.gpu
			if err := c.report(v); err != nil {
				t.Fatalf("the loss failed the video: %v", err)
			}
			if len(v.lostOpen) != 0 {
				t.Fatal("the video was told of the loss from inside its own report")
			}
			rig.drain()
			if len(v.lostOpen) != 1 || !lost.closed || rig.h.wnd.renderer != nil {
				t.Fatalf("after the report: video told %v, device closed %v, window renderer %v; want the device given up", v.lostOpen, lost.closed, rig.h.wnd.renderer)
			}
			rig.land()
			if rig.opens != 2 || rig.h.wnd.renderer != rig.a.gpu || len(v.wokenOn) != 1 || v.wokenOn[0] != rig.a.gpu {
				t.Errorf("%d devices opened, window on %v, video woken on %v; want both on a second device", rig.opens, rig.h.wnd.renderer, v.wokenOn)
			}
			if v.s.err != nil {
				t.Errorf("the surface was failed: %v", v.s.err)
			}
		})
	}
}

func TestAGPUVideoLearnsThatNoDeviceFollowsALostOne(t *testing.T) {
	t.Run("lost too often", func(t *testing.T) {
		logutil.Set(slog.New(slog.DiscardHandler))
		t.Cleanup(func() { logutil.Set(nil) })
		rig := newLossRig(t)
		v := rig.video()
		for i := range gpu.LossLimit {
			rig.loseFrame(float64(i) / 10)
			rig.drain()
			rig.land()
		}
		woken := len(v.wokenOn)
		rig.loseFrame(0.9)
		rig.drain()
		if len(v.wokenOn) != woken+1 || v.wokenOn[woken] != nil || v.wokePending[woken] {
			t.Fatalf("after the last loss the video was woken on %v, pending %v; want once, with no device there or coming", v.wokenOn[woken:], v.wokePending[woken:])
		}
	})
	t.Run("the GPU is required", func(t *testing.T) {
		logutil.Set(slog.New(slog.DiscardHandler))
		t.Cleanup(func() { logutil.Set(nil) })
		rig := newLossRig(t)
		v := rig.video()
		r := rig.a.gpu
		r.required = true
		v.s.DeviceLost(errVideoLoss)
		rig.drain()
		if len(v.lostOpen) != 1 || len(v.wokenOn) != 1 || v.wokenOn[0] != r || !r.Failed() || rig.opens != 1 {
			t.Fatalf("video told %v, woken on %v, renderer failed %v, %d devices opened; want the video sent back to a failed renderer", v.lostOpen, v.wokenOn, r.Failed(), rig.opens)
		}
		if !errors.Is(r.failed, gpu.ErrDeviceLost) {
			t.Errorf("the renderer failed with %v, want the loss", r.failed)
		}
	})
}
