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

	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/internal/wlsession"
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
			render: func(*render.List, *gpu.Target, []gpu.Carry, bool) (gpuFrame, error) { return gpuFrame{}, lost },
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
			b := &buffer.Buffer{Width: 40, Height: 30, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 40, H: 30}), Target: tg}
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

// A dmabuf import the compositor refuses sends every window to the
// CPU without touching the connection, or, under LYS_UI_RENDERER=gpu,
// fails the next buffer with the refusal (#365).
func TestRefusedImportFallsBackToCPU(t *testing.T) {
	refusal := errors.New("refused")
	r := &gpuRenderer{name: "test"}
	r.refuse(refusal)
	if !r.Failed() {
		t.Error("a refused import left the windows on the GPU")
	}
	r = &gpuRenderer{name: "test", required: true}
	r.refuse(refusal)
	if r.Failed() {
		t.Error("required GPU: a refused import sent the windows to the CPU")
	}
	if _, err := r.NewBuffer(nil, 4, 4, 1); !errors.Is(err, refusal) {
		t.Errorf("required GPU: the next buffer gave %v, want the refusal", err)
	}
}

// The renderer opens the device the compositor imports from: a named
// main device no Vulkan device is fails, rather than opening another
// GPU whose buffers the compositor cannot import (#365).
func TestRendererOpensTheCompositorsDevice(t *testing.T) {
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	nodev := unix.Mkdev(1, 3) // /dev/null, a character device no GPU is
	d, err := openDevice(nodev, true)
	if err == nil {
		d.Close()
		t.Fatalf("main device 1:3 opened %s", d.Name())
	}
	if !strings.Contains(err.Error(), "main device 1:3") {
		t.Errorf("error %v does not name the main device", err)
	}
}

// A refusal logs as one, not as a failed frame: a warning when the
// device was a guess (the compositor named none), an error when the
// compositor refused buffers of its own main device (#376).
func TestRefusedImportIsLoggedAsARefusal(t *testing.T) {
	for _, named := range []bool{false, true} {
		var log bytes.Buffer
		logutil.Set(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelInfo})))
		r := &gpuRenderer{name: "llvmpipe", plan: rendererPlan{hasNode: named}}
		r.refuse(errors.New("refused"))
		r.refuse(errors.New("refused again"))
		logutil.Set(nil)
		want := "level=WARN"
		if named {
			want = "level=ERROR"
		}
		if got := log.String(); strings.Count(got, "refused the GPU's buffers") != 1 || !strings.Contains(got, want) || strings.Contains(got, "frame failed") {
			t.Errorf("named %v: log %q; want one refusal line at %s", named, got, want)
		}
	}
}

// On the GPU a reused buffer copies what it owes from the target
// presented last, and the frame records only its own damage (#395).
func TestGPUReusedBufferCopiesForward(t *testing.T) {
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
	face, err := Font("sans", 14)
	if err != nil {
		t.Skip(err)
	}
	col := widget.NewBox(widget.Column, 6, 12)
	var labels []*widget.Label
	for i := range 20 {
		l := widget.NewLabel(face, 14, fmt.Sprintf("row %d", i), 0)
		labels = append(labels, l)
		col.Append(l, false)
	}
	h := newPaintHarness(col, 400, 600)
	w := h.wnd
	w.renderer = deviceRenderer(dev)
	var bufs []*buffer.Buffer
	w.newBuffer = func() (*buffer.Buffer, error) {
		tg, err := dev.NewOffscreen(400, 600)
		if err != nil {
			return nil, err
		}
		b := &buffer.Buffer{Width: 400, Height: 600, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 400, H: 600}), Target: tg}
		bufs = append(bufs, b)
		return b, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	w.dirty = true
	if !w.draw() {
		t.Fatal(w.drawErr)
	}
	labels[10].SetText("row ten, changed")
	w.dirty = true
	if !w.draw() {
		t.Fatal(w.drawErr)
	}
	if len(bufs) != 2 {
		t.Fatalf("%d buffers, want a second while the first is held", len(bufs))
	}
	if all := 400 * 600; w.paintedPixels*10 > all {
		t.Errorf("recorded %d of %d pixels, want only the changed label", w.paintedPixels, all)
	}
	got, err := bufs[1].Target.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	want, err := bufs[0].Target.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	changed := labels[10].Bounds()
	for y := range 600 {
		for x := range 400 {
			if !changed.Contains(x, y) && got.RGBAAt(x, y) != want.RGBAAt(x, y) {
				t.Fatalf("(%d,%d) = %v, want the screen's %v outside the changed label", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
			}
		}
	}
}

// ForgetImage drops a raster's GPU copy where there is a GPU and does
// nothing on the CPU; a raster without an identity is a caller bug.
func TestForgetImage(t *testing.T) {
	a := &Application{}
	a.ForgetImage(render.NewImageID())
	defer func() {
		if recover() == nil {
			t.Error("ForgetImage(0) did not panic")
		}
	}()
	a.ForgetImage(0)
}

// The buffer trim gives an idle GPU device's scratch back once it has
// drawn nothing for a whole interval, keeps the timer running while it
// draws, and trims it once, not on every tick.
func TestGPUTrimWaitsForAnIdleDevice(t *testing.T) {
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
	tg, err := dev.NewOffscreen(16, 16)
	if err != nil {
		t.Fatal(err)
	}
	r := deviceRenderer(dev)
	b := &buffer.Buffer{Width: 16, Height: 16, Scale: 1, Target: tg}
	if ok, err := r.Frame(b, 120, nil, func(cv *render.Canvas) { cv.Clear(cv.Rect(), render.RGB(1, 2, 3)) }); ok != buffer.FrameDrawn || err != nil {
		t.Fatalf("frame: %v, %v", ok, err)
	}
	a := &Application{gpu: r}
	a.armTrim()
	if a.stopTrim == nil {
		t.Fatal("a GPU frame armed no trim")
	}
	if !r.trimIdle() || r.trimmed {
		t.Error("a device that just drew was trimmed")
	}
	if r.trimIdle() || !r.trimmed {
		t.Error("a device idle for an interval was not trimmed")
	}
	a.trimBuffers()
	if a.stopTrim != nil {
		t.Error("the trim timer kept running for a trimmed, idle device")
	}
}

func TestAFrameTheGPURefusedStaysOwedAndWakesTheLoop(t *testing.T) {
	first, second := widget.NewProgressBar(0), widget.NewProgressBar(0)
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(first, false)
	root.Append(widget.NewBox(widget.Column, 0, 0), true)
	root.Append(second, false)
	h := newPaintHarness(root, 200, 200)
	w := h.wnd
	busy := false
	var carried []gpu.Carry
	var wakes []time.Duration
	r := &gpuRenderer{
		name: "test", rec: render.NewRecorder(1, 1, 1, 1), required: true,
		wake: func(d time.Duration) { wakes = append(wakes, d) },
		render: func(_ *render.List, _ *gpu.Target, carry []gpu.Carry, _ bool) (gpuFrame, error) {
			carried = append(carried[:0], carry...)
			if busy {
				return gpuFrame{}, gpu.ErrBusy
			}
			return gpuFrame{}, nil
		},
	}
	w.renderer = r
	var bufs []*buffer.Buffer
	w.newBuffer = func() (*buffer.Buffer, error) {
		b := &buffer.Buffer{Width: 200, Height: 200, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 200, H: 200}), Target: &gpu.Target{}}
		bufs = append(bufs, b)
		return b, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	frame := func() {
		t.Helper()
		w.dirty = true
		if !w.draw() {
			t.Fatal(w.drawErr)
		}
	}
	frame()
	first.SetValue(0.5)
	frame()
	if len(bufs) != 2 {
		t.Fatalf("%d buffers, want a second while the first is held", len(bufs))
	}
	bufs[0].Release()

	busy = true
	second.SetValue(0.5)
	commits := h.surf.commits
	frame()
	if r.Failed() || w.renderer == nil || w.drawErr != nil {
		t.Fatalf("a busy GPU failed the renderer: %v, renderer %v, %v", r.failed, w.renderer, w.drawErr)
	}
	if h.surf.commits != commits {
		t.Error("a refused frame committed")
	}
	if bufs[0].Busy() {
		t.Error("the refused frame's buffer was not given back")
	}
	if !w.dirty || w.damage.Empty() {
		t.Errorf("after the refusal dirty %v, damage empty %v: the frame must stay owed", w.dirty, w.damage.Empty())
	}
	if bufs[0].Stale.Empty() {
		t.Error("the refused frame cleared the buffer's staleness")
	}
	if len(wakes) != 1 || wakes[0] != gpuBusyRetry {
		t.Errorf("wakes %v, want one retry at %v: no frame callback will come", wakes, gpuBusyRetry)
	}

	busy = false
	frame()
	if h.surf.commits != commits+1 {
		t.Errorf("the retry made %d commits, want 1", h.surf.commits-commits)
	}
	want := image.Rect(first.Bounds().X, first.Bounds().Y, first.Bounds().X+first.Bounds().W, first.Bounds().Y+first.Bounds().H)
	if len(carried) != 1 || carried[0].From != bufs[1].Target || !slices.Contains(carried[0].Rects, want) {
		t.Errorf("the retry carried %+v, want the first bar %v from the front", carried, want)
	}
	if !bufs[0].Stale.Empty() {
		t.Errorf("stale %v after the frame went out", bufs[0].Stale.Rects())
	}
	if len(wakes) != 1 {
		t.Errorf("wakes %v, want no further retry", wakes)
	}
	for _, b := range bufs {
		b.Target = nil
	}
}

func deviceRenderer(dev *gpu.Device) *gpuRenderer {
	r := &gpuRenderer{dev: dev, name: dev.Name(), rec: render.NewRecorder(1, 1, 1, 1), wake: func(time.Duration) {}}
	r.render, r.prepare = r.deviceFrame, r.deviceTarget
	return r
}

func TestAGPUScrollMatchesAWholeRepaint(t *testing.T) {
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
	onDevice := func(h *paintHarness) {
		w := h.wnd
		w.renderer = deviceRenderer(dev)
		w.newBuffer = func() (*buffer.Buffer, error) {
			tg, err := dev.NewOffscreen(400, 300)
			if err != nil {
				return nil, err
			}
			b := &buffer.Buffer{Width: 400, Height: 300, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 400, H: 300}), Target: tg}
			h.bufs = append(h.bufs, b)
			return b, nil
		}
		w.pool = buffer.New(w.allocator(), 3)
	}
	scrolled, sc, _ := scrollWindow(t)
	onDevice(scrolled)
	scrolled.frame()
	for _, y := range []int{30, 64, 50, 51} {
		sc.SetOffset(0, y)
		scrolled.frame()
	}
	if limit := 400 * 40; scrolled.wnd.paintedPixels > limit {
		t.Errorf("the last step painted %d px, want its exposed strip, at most %d", scrolled.wnd.paintedPixels, limit)
	}
	whole, wsc, _ := scrollWindow(t)
	onDevice(whole)
	whole.frame()
	wsc.SetOffset(0, 51)
	whole.wnd.pool = buffer.New(whole.wnd.allocator(), 3)
	whole.frame()
	if painted := whole.wnd.paintedPixels; painted != 400*300 {
		t.Fatalf("the reference painted %d px, want the whole window", painted)
	}
	got, err := scrolled.wnd.pool.Front().Target.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	want, err := whole.wnd.pool.Front().Target.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	for y := range 300 {
		for x := range 400 {
			if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
				t.Fatalf("(%d,%d) after scrolling = %v, a whole repaint %v", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
			}
		}
	}
}

func TestATargetIsAllocatedOffTheLoopAndLandsThroughIt(t *testing.T) {
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
	queued := make(chan func(), 4)
	r := deviceRenderer(dev)
	r.invoke = func(fn func()) { queued <- fn }
	if dev.Exports() {
		if r.modifiers, err = dev.Modifiers(fourcc.ARGB8888); err != nil {
			t.Fatal(err)
		}
	} else {
		r.prepare = func(w, h int, _ []fourcc.Modifier) (*gpu.Target, error) { return dev.PrepareOffscreen(w, h) }
	}
	a := &Application{gpu: r}
	var got *gpu.Target
	answers := 0
	r.allocateIn(nil)(64, 32, func(tg *gpu.Target, err error) {
		answers++
		if err != nil {
			t.Error(err)
		}
		got = tg
	})
	if answers != 0 {
		t.Fatal("the target came back where it was asked, not through the loop")
	}
	select {
	case fn := <-queued:
		fn()
	case <-time.After(time.Minute):
		t.Fatal("the worker never answered")
	}
	if got == nil || got.Size() != image.Pt(64, 32) {
		t.Fatalf("the worker made %v", got)
	}

	r.allocateIn(nil)(64, 32, func(tg *gpu.Target, err error) {
		answers++
		if !errors.Is(err, gpu.ErrDeviceClosed) || tg != nil {
			t.Errorf("a target landing after the close: %v, %v; want none and the close", tg, err)
		}
	})
	a.closeRenderer()
	select {
	case fn := <-queued:
		fn()
	default:
		t.Fatal("the renderer closed with its worker still running")
	}
	if answers != 2 {
		t.Errorf("%d answers, want 2", answers)
	}
}
