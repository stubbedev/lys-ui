package app

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/gpu"
	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/gputest"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// LYS_UI_RENDERER picks how windows draw: cpu never tries the GPU,
// auto falls back to the CPU where the GPU cannot draw for the
// compositor, gpu makes that an error, and anything else is one.
func TestRendererModes(t *testing.T) {
	for _, tc := range []struct {
		mode, err string
	}{
		{mode: "cpu"},
		{mode: ""},
		{mode: "auto"},
		{mode: "gpu", err: "LYS_UI_RENDERER=gpu: the compositor has no linux-dmabuf"},
		{mode: "vulkan", err: `LYS_UI_RENDERER="vulkan": want cpu, gpu or auto`},
	} {
		t.Setenv(rendererEnv, tc.mode)
		a := &Application{sess: &wlsession.Session{}}
		r, err := a.renderer()
		switch {
		case r != nil:
			t.Errorf("%q: a GPU renderer on a compositor without linux-dmabuf", tc.mode)
		case tc.err == "" && err != nil:
			t.Errorf("%q: %v, want the CPU", tc.mode, err)
		case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
			t.Errorf("%q: error %v, want %q", tc.mode, err, tc.err)
		}
		if r2, err2 := a.renderer(); r2 != r || !errors.Is(err2, err) {
			t.Errorf("%q: a second call gave %v, %v", tc.mode, r2, err2)
		}
	}
}

// A GPU frame that fails sends the window to the CPU: the frame's
// buffer goes back, the pool makes shared-memory buffers, and the next
// pass repaints the window into one; under LYS_UI_RENDERER=gpu the
// failure is the loop's error instead.
func TestGPUFrameFailureFallsBackToCPU(t *testing.T) {
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	dev, err := gpu.Open()
	if err != nil {
		if gputest.Enabled() {
			t.Fatal(err)
		}
		t.Skipf("no GPU: %v", err)
	}
	t.Cleanup(dev.Close)
	for _, required := range []bool{false, true} {
		h := newPaintHarness(widget.NewSpacer(40, 30), 40, 30)
		w := h.wnd
		lost := errors.New("device lost")
		w.renderer = &gpuRenderer{
			name: dev.Name(), rec: render.NewRecorder(1, 1, 1, 1), required: required,
			render: func(*render.List, *gpu.Target) error { return lost },
		}
		fakes := w.newBuffer
		var gpuBufs []*buffer.Buffer
		w.newBuffer = func() (*buffer.Buffer, error) {
			if w.renderer == nil {
				return fakes()
			}
			tg, err := dev.NewOffscreen(40, 30)
			if err != nil {
				return nil, err
			}
			b := &buffer.Buffer{Width: 40, Height: 30, Scale: 1, Stale: render.Rect{W: 40, H: 30}, Target: tg}
			gpuBufs = append(gpuBufs, b)
			return b, nil
		}
		w.pool = buffer.New(w.allocator(), 3)
		w.dirty = true
		ok := w.draw()
		if required {
			if ok || !errors.Is(w.drawErr, lost) {
				t.Errorf("required GPU: draw = %v, %v; want the frame's error", ok, w.drawErr)
			}
			continue
		}
		if !ok || w.renderer != nil {
			t.Fatalf("draw = %v with renderer %v; want the window on the CPU", ok, w.renderer)
		}
		if len(gpuBufs) != 1 || gpuBufs[0].Busy() {
			t.Fatalf("GPU buffers %d, busy %v: the failed frame's buffer was not given back", len(gpuBufs), gpuBufs[0].Busy())
		}
		if n, _ := h.frame(); n == 0 {
			t.Error("the next pass painted nothing on the CPU")
		}
		if len(h.bufs) == 0 {
			t.Error("the next pass drew into no shared-memory buffer")
		}
	}
}

// After a reconnect the renderer agrees its layouts with the new
// compositor: one without linux-dmabuf sends the app to the CPU, or,
// under LYS_UI_RENDERER=gpu, fails the reconnect.
func TestReconnectReagreesRenderer(t *testing.T) {
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	for _, required := range []bool{false, true} {
		dev, err := gpu.Open()
		if err != nil {
			if gputest.Enabled() {
				t.Fatal(err)
			}
			t.Skipf("no GPU: %v", err)
		}
		a := &Application{sess: &wlsession.Session{}, gpu: &gpuRenderer{dev: dev, name: dev.Name(), required: required}, gpuTried: true}
		err = a.reagreeRenderer()
		switch {
		case required && (err == nil || a.gpu == nil):
			t.Errorf("required: reagree = %v, renderer %v; want an error and the renderer kept", err, a.gpu)
		case !required && (err != nil || a.gpu != nil):
			t.Errorf("auto: reagree = %v, renderer %v; want the CPU", err, a.gpu)
		}
		if r, rerr := a.renderer(); !required && (r != nil || rerr != nil) {
			t.Errorf("auto: renderer after reagree = %v, %v; want the CPU", r, rerr)
		}
		if a.gpu != nil {
			a.gpu.dev.Close()
		}
	}
}

// Falling back to the CPU says so once, at Info, with the reason: an
// application installed where no Vulkan driver loads (Nix on another
// distribution) otherwise renders slowly without a word.
func TestRendererFallbackIsLogged(t *testing.T) {
	t.Setenv(rendererEnv, "auto")
	var log bytes.Buffer
	logutil.Set(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { logutil.Set(nil) })
	a := &Application{sess: &wlsession.Session{}}
	for range 2 {
		if r, err := a.renderer(); r != nil || err != nil {
			t.Fatalf("renderer = %v, %v; want the CPU", r, err)
		}
	}
	if n := strings.Count(log.String(), "draws on the CPU"); n != 1 || !strings.Contains(log.String(), "no linux-dmabuf") {
		t.Errorf("log %q: want one fallback line naming the reason", log.String())
	}
}
