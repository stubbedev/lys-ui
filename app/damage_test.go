// The paint-count harness: drives the real hostWindow.draw pipeline
// over fake buffers and a recording surface, pinning how many pixels a
// damage-restricted frame paints and which rects go out as
// wl_surface.damage_buffer. No wayland connection involved.
package app

import (
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/widget"
)

// fakeHost is a wire-free Host of a fixed size.
type fakeHost struct{ w, h int }

func (f *fakeHost) EnsureUsable() error      { return nil }
func (f *fakeHost) Closed() bool             { return false }
func (f *fakeHost) Size() (int, int)         { return f.w, f.h }
func (f *fakeHost) HostSurface() *wl.Surface { return nil }

// resizeTo plays a compositor configure: the next frame sees the new
// size.
func (f *fakeHost) resizeTo(w, h int) { f.w, f.h = w, h }

// fakeSurface records the wire traffic draw produces.
type fakeSurface struct {
	attaches, commits, frames int
	// calls is the request order ("frame", "commit").
	calls  []string
	damage []render.Rect
	// opaque records the SetOpaqueRegion surface rects in call order;
	// an opaque window sets one per size change, translucent ones
	// never.
	opaque []render.Rect
	// holes records each SetOpaqueRegion's cut-outs, call by call.
	holes [][]render.Rect
	// blurOffered is what BlurAvailable reports; blur records each
	// SetBlurRegion call's rects (nil: the effect removed).
	blurOffered bool
	blur        [][]render.Rect
}

func (f *fakeSurface) BlurAvailable() bool { return f.blurOffered }

func (f *fakeSurface) SetBlurRegion(rects []render.Rect) error {
	f.blur = append(f.blur, rects)
	return nil
}

func (f *fakeSurface) Attach(*buffer.Buffer) error { f.attaches++; return nil }

func (f *fakeSurface) Damage(rects []render.Rect) error {
	f.damage = append(f.damage, rects...)
	return nil
}

func (f *fakeSurface) Commit() error {
	f.commits++
	f.calls = append(f.calls, "commit")
	return nil
}

func (f *fakeSurface) Frame(ready *bool) error {
	f.frames++
	f.calls = append(f.calls, "frame")
	*ready = true
	return nil
}

func (f *fakeSurface) SetOpaqueRegion(w, h int, holes []render.Rect) error {
	f.opaque = append(f.opaque, render.Rect{W: w, H: h})
	f.holes = append(f.holes, holes)
	return nil
}

// fakeBuffer is a pooled buffer without a wire object; Stale starts
// full exactly like a fresh wl_shm mapping.
func fakeBuffer(w, h int) *buffer.Buffer {
	stride := render.Stride(w)
	return &buffer.Buffer{
		Data:  make([]byte, stride*h),
		Width: w, Height: h, Stride: stride, Scale: 1,
		Stale: render.Rect{W: w, H: h},
	}
}

// paintHarness drives a hostWindow the way the compositor would: a
// buffer is busy until the frame after the one that used it, mirroring
// the release latency that rotates the pool.
type paintHarness struct {
	wnd  *hostWindow
	surf *fakeSurface
	bufs []*buffer.Buffer
}

func newPaintHarness(root widget.Widget, w, hgt int) *paintHarness {
	ph := &paintHarness{
		surf: &fakeSurface{},
		wnd: &hostWindow{
			host:   &fakeHost{w: w, h: hgt},
			cfg:    windowHooks{background: widget.Current().Bg},
			scale:  1,
			router: &widget.Router{Root: root},
			tip:    &tooltipCtl{since: time.Now()},
			lastW:  w, lastH: hgt,
			dirty: true,
		},
	}
	ph.wnd.surf = ph.surf
	ph.wnd.input = &surfaceInput{router: ph.wnd.router}
	// The fake allocator hangs off newBuffer, not the pool, so a live
	// rescale's pool rebuild keeps allocating fakes at the new size -
	// and a configure-driven resize too: the size is read at
	// allocation time, exactly like the production create.
	ph.wnd.newBuffer = func() (*buffer.Buffer, error) {
		bw, bh := ph.wnd.layoutSize()
		b := fakeBuffer(scale.DeviceSize(bw, ph.wnd.frac120), scale.DeviceSize(bh, ph.wnd.frac120))
		ph.bufs = append(ph.bufs, b)
		return b, nil
	}
	ph.wnd.pool = buffer.New(ph.wnd.allocator(), 3)
	return ph
}

// frame draws once and releases the buffers the compositor would have
// handed back by now (every buffer busy before this frame). It returns
// the pixels written and the damage rects sent on the wire.
func (h *paintHarness) frame() (int, []render.Rect) {
	var stale []*buffer.Buffer
	for _, b := range h.bufs {
		if b.Busy() {
			stale = append(stale, b)
		}
	}
	h.wnd.dirty = true
	if !h.wnd.draw() {
		panic(h.wnd.drawErr)
	}
	for _, b := range stale {
		b.Release()
	}
	return h.wnd.paintedPixels, h.wnd.lastDamage
}

// warm rotates every pooled buffer through a present, so the frames
// under test land in recycled buffers instead of fresh, fully stale
// ones. nudge forces real damage each frame; nil does nothing and the
// frame skips.
func (h *paintHarness) warm(nudge func(), n int) {
	for range n {
		if nudge != nil {
			nudge()
		}
		h.frame()
	}
}

func TestFirstFrameRepaintsFully(t *testing.T) {
	root := widget.NewBox(widget.Column, 8, 8)
	root.Append(widget.NewProgressBar(0), false)
	root.Append(widget.NewSlider(0, 1, 0, 0.5), false)
	h := newPaintHarness(root, 320, 200)

	painted, damage := h.frame()
	if painted == 0 {
		t.Fatalf("first frame painted %d px, want > 0", painted)
	}
	if got := render.UnionAll(damage); got != (render.Rect{W: 320, H: 200}) {
		t.Errorf("first frame damage union %+v, want the full buffer", got)
	}
	if h.surf.attaches != 1 || h.surf.commits != 1 || h.surf.frames != 1 {
		t.Errorf("wire traffic = %d attach, %d commit, %d frame, want 1 of each",
			h.surf.attaches, h.surf.commits, h.surf.frames)
	}
}

func TestProgressOnlyFramePaintsUnderFivePercent(t *testing.T) {
	root := widget.NewBox(widget.Column, 8, 8)
	progress := widget.NewProgressBar(0)
	// A Row keeps the bar at its natural 160px width; a bare child of a
	// Column would stretch across the window and dominate the pixel
	// budget.
	root.Append(widget.NewBox(widget.Row, 0, 0).Append(progress, false), false)
	root.Append(widget.NewSlider(0, 1, 0, 0.5), false)
	root.Append(widget.NewSwitch(true), false)
	h := newPaintHarness(root, 640, 470)
	h.warm(func() { progress.SetValue(progress.Value() + 0.05) }, 3)

	total := 640 * 470
	for i, v := range []float64{0.2, 0.4, 0.6} {
		progress.SetValue(v)
		painted, damage := h.frame()
		if got := render.UnionAll(damage); got != progress.Bounds() {
			t.Errorf("frame %d damaged %+v, want only %+v", i+2, got, progress.Bounds())
		}
		if ratio := float64(painted) / float64(total); ratio >= 0.05 {
			t.Errorf("frame %d painted %d px = %.1f%%, want < 5%%", i+2, painted, ratio*100)
		}
	}
	if h.surf.commits != 6 {
		t.Errorf("commits = %d, want 6 (three warm-up + three progress frames)", h.surf.commits)
	}
}

func TestIdleFrameSkipsBuffers(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	root.Append(widget.NewProgressBar(0.5), false)
	h := newPaintHarness(root, 320, 200)
	h.frame()

	painted, damage := h.frame() // dirty without any invalidation
	if painted != 0 || len(damage) != 0 {
		t.Errorf("idle frame painted %d px with damage %v, want a skip", painted, damage)
	}
	if h.surf.commits != 1 {
		t.Errorf("idle frame committed; commits = %d, want 1", h.surf.commits)
	}
}

func TestRotatedBufferRepaintsMissedDamage(t *testing.T) {
	root := widget.NewBox(widget.Column, 8, 8)
	progress := widget.NewProgressBar(0)
	root.Append(progress, false)
	h := newPaintHarness(root, 320, 200)
	h.frame() // buf A: full (fresh)

	// Frame two lands in the fresh buffer B: a full initialization
	// paint, the documented fallback. The wire damage covers the whole
	// repaint (repaint region plus stale, EGL-style); the part that
	// matters for other buffers' staleness is only what changed.
	progress.SetValue(0.25)
	painted2, damage2 := h.frame()
	if painted2 == 0 {
		t.Errorf("fresh buffer painted nothing, want a full initialization paint")
	}
	if got := render.UnionAll(damage2); got != (render.Rect{W: 320, H: 200}) {
		t.Errorf("fresh buffer repaint region %+v, want the full buffer", got)
	}

	// Frame three reuses buffer A, which missed frame two's bar change:
	// it repaints that missed rect plus its own - the same rect here,
	// since a bar's bounds do not move - and a fraction of the window.
	progress.SetValue(0.6)
	painted3, damage3 := h.frame()
	bar := progress.Bounds()
	region := render.UnionAll(damage3)
	if region != bar {
		t.Errorf("rotated buffer repainted %+v, want just the bar rect %+v", region, bar)
	}
	if len(damage3) < 2 {
		t.Errorf("rotated buffer sent %d damage rect(s), want the frame's own plus the stale rect", len(damage3))
	}
	if painted3 >= painted2 {
		t.Errorf("rotated buffer painted %d px, want well under the fresh buffer's %d", painted3, painted2)
	}
}

func TestFocusChangeDamagesBothRings(t *testing.T) {
	// Sliders, not buttons: sliders implement the two-argument
	// KeyAction the router focuses on.
	focus1 := widget.NewSlider(0, 1, 0, 0)
	focus2 := widget.NewSlider(0, 1, 0, 0)
	root := widget.NewBox(widget.Column, 10, 10)
	root.Append(focus1, false)
	root.Append(focus2, false)
	h := newPaintHarness(root, 320, 200)
	h.warm(func() { focus1.SetValue(focus1.Value() + 0.25) }, 3) // cycle the fresh buffers out

	h.wnd.router.FocusNext() // nil focus lands on the first slider
	painted, damage := h.frame()
	// A ring-sized repaint, not the window; the exact fraction depends
	// on the ring's width in this small test window.
	if float64(painted)/float64(320*200) >= 0.25 {
		t.Errorf("focus change repainted the window: %d px", painted)
	}
	ring := render.UniformInsets(widget.FocusRingWidth).Grow(focus1.Bounds())
	if got := render.UnionAll(damage); got != ring {
		t.Errorf("damage %v, want exactly the new focus ring %v", damage, ring)
	}

	// Moving focus again must erase the old ring and draw the new one:
	// both rects join the damage union.
	h.wnd.router.FocusNext()
	_, damage = h.frame()
	oldRing := ring
	newRing := render.UniformInsets(widget.FocusRingWidth).Grow(focus2.Bounds())
	got := render.UnionAll(damage)
	if got != oldRing.Union(newRing) {
		t.Errorf("focus move damage %v, want old ring %v ∪ new ring %v", got, oldRing, newRing)
	}
}

func BenchmarkProgressOnlyFrames(b *testing.B) {
	root := widget.NewBox(widget.Column, 8, 8)
	progress := widget.NewProgressBar(0)
	root.Append(widget.NewBox(widget.Row, 0, 0).Append(progress, false), false)
	root.Append(widget.NewSlider(0, 1, 0, 0.5), false)
	h := newPaintHarness(root, 640, 470)
	h.warm(func() { progress.SetValue(progress.Value() + 0.05) }, 3)
	painted := 0
	b.ResetTimer()
	for i := range b.N {
		progress.SetValue(float64(i%100) / 100)
		painted, _ = h.frame()
	}
	b.ReportMetric(float64(painted), "px/frame")
	b.ReportMetric(100*float64(painted)/(640*470), "%-painted")
}

// TestFrameCallbackRidesItsOwnCommit pins the request order: the frame
// callback is double-buffered state, so it must be requested before the
// commit it is meant for. Regression: requested after, it waited on a
// next commit the pending callback itself held back, and a window with
// nothing animating painted once and never again (the launcher froze
// on its first frame).
func TestFrameCallbackRidesItsOwnCommit(t *testing.T) {
	ph := newPaintHarness(widget.NewBox(widget.Column, 0, 0), 40, 20)
	ph.frame()
	calls := ph.surf.calls
	if len(calls) < 2 || calls[len(calls)-2] != "frame" || calls[len(calls)-1] != "commit" {
		t.Errorf("calls = %v, want the frame request right before its commit", calls)
	}
}
