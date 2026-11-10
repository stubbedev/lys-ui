// The paint-count harness: drives the real hostWindow.draw pipeline
// over fake buffers and a recording surface, pinning how many pixels a
// damage-restricted frame paints and which rects go out as
// wl_surface.damage_buffer. No wayland connection involved.
package app

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// fakeHost is a wire-free Host of a fixed size.
type fakeHost struct {
	w, h      int
	surf      wl.Surface
	closed    bool
	destroyed int
}

func (f *fakeHost) EnsureUsable() error      { return nil }
func (f *fakeHost) Closed() bool             { return f.closed }
func (f *fakeHost) Size() (int, int)         { return f.w, f.h }
func (f *fakeHost) HostSurface() *wl.Surface { return &f.surf }
func (f *fakeHost) DestroySurface()          { f.destroyed++ }

// resizeTo plays a compositor configure: the next frame sees the new
// size.
func (f *fakeHost) resizeTo(w, h int) { f.w, f.h = w, h }

// fakeSurface records the wire traffic draw produces.
type fakeSurface struct {
	withholdFrames            bool
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
	blurOffered  bool
	blur         [][]render.Rect
	presentation bool
	feedback     []func(shown bool)
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

func (f *fakeSurface) Frame(done *wlsession.FrameDone) error {
	f.frames++
	f.calls = append(f.calls, "frame")
	done.Ready = !f.withholdFrames
	return nil
}

func (f *fakeSurface) Feedback(done func(shown bool)) error {
	if !f.presentation {
		return errNoPresentation
	}
	f.calls = append(f.calls, "feedback")
	f.feedback = append(f.feedback, done)
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
		Stale: buffer.RegionOf(render.Rect{W: w, H: h}),
	}
}

// paintHarness drives a hostWindow the way the compositor would: a
// buffer is busy until the frame after the one that used it, mirroring
// the release latency that rotates the pool.
type paintHarness struct {
	wnd  *hostWindow
	surf *fakeSurface
	bufs []*buffer.Buffer
	// stale is frame's scratch: the buffers busy before the frame.
	stale []*buffer.Buffer
}

func newPaintHarness(root widget.Widget, w, hgt int) *paintHarness {
	animclock.Reset()
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
	h.stale = h.stale[:0]
	for _, b := range h.bufs {
		if b.Busy() {
			h.stale = append(h.stale, b)
		}
	}
	h.wnd.dirty = true
	if !h.wnd.draw() {
		panic(h.wnd.drawErr)
	}
	for _, b := range h.stale {
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

// A buffer coming into rotation owes everything that changed since it
// was last shown, and it copies that from the buffer presented last
// rather than painting it: the fresh second buffer and then the first,
// which missed the second's frame, each paint and damage only their own
// frame's change (TestReusedBufferCopiesForward pins the pixels).
func TestRotatedBufferRepaintsMissedDamage(t *testing.T) {
	root := widget.NewBox(widget.Column, 8, 8)
	progress := widget.NewProgressBar(0)
	root.Append(progress, false)
	h := newPaintHarness(root, 320, 200)
	full, _ := h.frame() // buf A: full (fresh, nothing to copy from)
	bar := progress.Bounds()

	progress.SetValue(0.25)
	painted2, damage2 := h.frame() // buf B: fresh, copies the rest from A
	if region := render.UnionAll(damage2); region != bar {
		t.Errorf("fresh buffer damaged %+v, want just the bar %+v", region, bar)
	}
	if painted2*4 >= full {
		t.Errorf("fresh buffer painted %d px of the first frame's %d, want only the bar", painted2, full)
	}

	progress.SetValue(0.6)
	painted3, damage3 := h.frame() // buf A again: copies frame two's bar from B
	if region := render.UnionAll(damage3); region != bar {
		t.Errorf("rotated buffer damaged %+v, want just the bar %+v", region, bar)
	}
	if painted3*4 >= full {
		t.Errorf("rotated buffer painted %d px of the first frame's %d, want only the bar", painted3, full)
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

// A buffer that sat out frames is brought up to the screen by copying
// what it owes from the buffer presented last, and only the frame's own
// damage is painted: a fresh buffer coming into play while the first is
// still held repaints the changed label, not the window (#395).
func TestReusedBufferCopiesForward(t *testing.T) {
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
	h.frame()
	front := h.bufs[len(h.bufs)-1]
	labels[10].SetText("row ten, changed")
	h.wnd.dirty = true
	if !h.wnd.draw() {
		t.Fatal(h.wnd.drawErr)
	}
	b := h.bufs[len(h.bufs)-1]
	if b == front {
		t.Fatal("the frame reused the buffer the compositor still holds")
	}
	if all := 400 * 600; h.wnd.paintedPixels*10 > all {
		t.Errorf("painted %d of %d pixels, want only the changed label", h.wnd.paintedPixels, all)
	}
	changed := labels[10].Bounds()
	differs := false
	for y := range 600 {
		for x := range 400 {
			at := y*b.Stride + x*4
			same := [4]byte(b.Data[at:at+4]) == [4]byte(front.Data[at:at+4])
			if changed.Contains(x, y) {
				differs = differs || !same
			} else if !same {
				t.Fatalf("(%d,%d) differs from the screen outside the changed label", x, y)
			}
		}
	}
	if !differs {
		t.Error("the changed label did not repaint")
	}
}

// A timer that changes no widget wakes no window: a hot-reload poll
// finding nothing leaves the window parked, without a layout pass, and
// one that invalidates a widget owes it a draw (#402).
func TestIdleTimerWakesNoWindow(t *testing.T) {
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(widget.NewProgressBar(0.5), false)
	h := newPaintHarness(root, 200, 100)
	h.frame()
	a := &Application{windows: []*hostWindow{h.wnd}}
	var touch func()
	a.Every(time.Millisecond, func() {
		if touch != nil {
			touch()
		}
	})
	clock := time.Now()
	run := func() {
		clock = clock.Add(time.Second)
		gen := widget.DamageGeneration()
		if a.pump(clock) {
			a.markDirtyIfDamaged(gen)
		}
	}
	h.wnd.dirty = false
	run()
	if h.wnd.dirty {
		t.Error("a poll that changed nothing marked the window for a draw")
	}
	touch = func() { root.Invalidate() }
	run()
	if !h.wnd.dirty {
		t.Error("a timer that invalidated a widget left the window parked")
	}
}

// A window that stopped drawing gives its spare buffers back after the
// trim interval and keeps the front; the trim timer then stops, so
// the idle application parks without waking for it.
func TestIdleWindowFreesItsSpareBuffers(t *testing.T) {
	root := widget.NewBox(widget.Column, 0, 0)
	bar := widget.NewProgressBar(0)
	root.Append(bar, false)
	h := newPaintHarness(root, 200, 100)
	v := 0.0
	h.warm(func() { v += 0.1; bar.SetValue(v) }, 4)
	for _, b := range h.bufs {
		b.Release()
	}
	a := &Application{windows: []*hostWindow{h.wnd}}
	a.armTrim()
	if a.stopTrim == nil {
		t.Fatal("a window holding spare buffers armed no trim")
	}
	clock := time.Now()
	for range 2 {
		clock = clock.Add(bufferTrimInterval)
		a.pump(clock)
	}
	if n, _ := h.wnd.pool.Footprint(); n != 1 {
		t.Errorf("the idle window holds %d buffers, want only the front", n)
	}
	if a.stopTrim != nil || a.queues.timerCount() != 0 {
		t.Error("the trim timer kept running with no spare buffer left")
	}
	bar.SetValue(0.9)
	if px, _ := h.frame(); px == 0 {
		t.Error("the trimmed window painted nothing on its next frame")
	}
}

func TestFarApartBarsCopyForwardOnlyTheirStrips(t *testing.T) {
	root := widget.NewBox(widget.Column, 0, 0)
	top, bottom := widget.NewProgressBar(0), widget.NewProgressBar(0)
	root.Append(top, false)
	root.Append(widget.NewBox(widget.Column, 0, 0), true)
	root.Append(bottom, false)
	h := newPaintHarness(root, 1000, 800)
	v := 0.0
	turn := false
	nudge := func() {
		v += 0.01
		turn = !turn
		if turn {
			top.SetValue(v)
			return
		}
		bottom.SetValue(v)
	}
	h.warm(nudge, 4)
	strips := top.Bounds().W*top.Bounds().H + bottom.Bounds().W*bottom.Bounds().H
	before := h.wnd.pool.CopiedPixels()
	const frames = 20
	for range frames {
		nudge()
		h.frame()
	}
	got := (h.wnd.pool.CopiedPixels() - before) / frames
	if got == 0 || got > strips {
		t.Errorf("copied %d px per frame forward, want the bars' %d px at most, and some", got, strips)
	}
}

func spinnerHarness(t *testing.T) (*paintHarness, *widget.Spinner, *int, func(time.Duration)) {
	t.Helper()
	clock := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return clock }))
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	sp := widget.NewSpinner(24)
	root := widget.NewBox(widget.Column, 0, 0).Append(sp, false)
	t.Cleanup(widget.Host(root))
	h := newPaintHarness(root, 200, 100)
	made := 0
	inner := h.wnd.newBuffer
	h.wnd.newBuffer = func() (*buffer.Buffer, error) {
		made++
		return inner()
	}
	advance := func(d time.Duration) {
		clock = clock.Add(d)
		animclock.Tick(clock)
	}
	return h, sp, &made, advance
}

func spin(h *paintHarness, advance func(time.Duration), frames int) {
	for range frames {
		advance(16 * time.Millisecond)
		h.frame()
	}
}

func TestAnimationRestartFindsItsSpares(t *testing.T) {
	h, sp, made, advance := spinnerHarness(t)
	h.frame()
	sp.SetSpinning(true)
	spin(h, advance, 8)
	sp.SetSpinning(false)
	h.frame()
	for _, b := range h.bufs {
		if b.Busy() {
			b.Release()
		}
	}
	for range 3 {
		advance(bufferTrimInterval)
		h.wnd.trimPool()
	}
	before, deferred := *made, h.wnd.pool.Deferrals()
	sp.SetSpinning(true)
	spin(h, advance, 8)
	if got := *made - before; got != 0 {
		t.Errorf("a warm restart allocated %d buffers on the loop, want 0", got)
	}
	if got := h.wnd.pool.Deferrals() - deferred; got != 0 {
		t.Errorf("a warm restart deferred %d frames with the pool busy, want 0", got)
	}
}

func TestSparesGoAfterTheAnimationIsLongOver(t *testing.T) {
	h, sp, _, advance := spinnerHarness(t)
	h.frame()
	sp.SetSpinning(true)
	spin(h, advance, 8)
	sp.SetSpinning(false)
	h.frame()
	for _, b := range h.bufs {
		if b.Busy() {
			b.Release()
		}
	}
	advance(spareKeep)
	for range 2 {
		h.wnd.trimPool()
	}
	if n, _ := h.wnd.pool.Footprint(); n != 1 {
		t.Errorf("the window holds %d buffers long after its animation, want the front alone", n)
	}
}

func TestAStaticWindowPreparesNoSpare(t *testing.T) {
	h, _, made, _ := spinnerHarness(t)
	h.frame()
	if *made != 1 {
		t.Errorf("a window that drew once with nothing animating made %d buffers, want 1", *made)
	}
}

func TestAColdAnimationPreparesItsSpareAheadOfTheNextFrame(t *testing.T) {
	h, sp, made, advance := spinnerHarness(t)
	h.frame()
	sp.SetSpinning(true)
	advance(16 * time.Millisecond)
	h.wnd.dirty = true
	if !h.wnd.draw() {
		t.Fatal(h.wnd.drawErr)
	}
	if *made != 2 {
		t.Errorf("after the first animated frame the pool made %d buffers, want the spare ready before the next", *made)
	}
}

func scrollWindow(t *testing.T) (*paintHarness, *widget.Scroll, []*widget.Label) {
	t.Helper()
	face, err := Font("sans", 14)
	if err != nil {
		t.Skip(err)
	}
	col := widget.NewBox(widget.Column, 2, 4)
	var labels []*widget.Label
	for i := range 800 {
		l := widget.NewLabel(face, 14, fmt.Sprintf("row %d of the long list", i), 0)
		labels = append(labels, l)
		col.Append(l, false)
	}
	sc := widget.NewScroll(col)
	root := widget.NewBox(widget.Column, 0, 8).Append(sc, true)
	return newPaintHarness(root, 400, 300), sc, labels
}

func wholePaint(h *paintHarness) []byte {
	w, ht := h.wnd.lastW, h.wnd.lastH
	buf := make([]byte, render.Stride(w)*ht)
	cv := render.New(buf, render.Stride(w), w, ht)
	h.wnd.paintTree(cv, render.Rect{W: w, H: ht})
	return buf
}

func assertShowsWholePaint(t *testing.T, h *paintHarness, step int) {
	t.Helper()
	got, want := h.wnd.pool.Front(), wholePaint(h)
	for y := range got.Height {
		for x := range got.Width {
			at := y*got.Stride + x*4
			if [4]byte(got.Data[at:at+4]) != [4]byte(want[at:at+4]) {
				t.Fatalf("step %d: (%d,%d) shows %v, a whole paint %v", step, x, y, got.Data[at:at+4], want[at:at+4])
			}
		}
	}
}

func TestScrollBlitMatchesAWholePaint(t *testing.T) {
	h, sc, labels := scrollWindow(t)
	h.frame()
	y := 0
	for step, d := range []int{13, 30, 1, 299, 7, -40, -3, 500, -501, 2, 2, 2, 60, -60} {
		y += d
		sc.SetOffset(0, y)
		switch step {
		case 4:
			labels[20].SetText("a row that changed as it moved")
		case 9:
			labels[3].SetText("row 3")
		}
		h.frame()
		assertShowsWholePaint(t, h, step)
	}
}

func TestScrollBlitWithAReleasedFrontMatchesAWholePaint(t *testing.T) {
	h, sc, _ := scrollWindow(t)
	h.frame()
	y := 0
	for step := range 12 {
		y += 9
		sc.SetOffset(0, y)
		h.wnd.dirty = true
		if !h.wnd.draw() {
			t.Fatal(h.wnd.drawErr)
		}
		for _, b := range h.bufs {
			if b.Busy() {
				b.Release()
			}
		}
		assertShowsWholePaint(t, h, step)
	}
}

func TestScrollStepPaintsOnlyItsStrip(t *testing.T) {
	h, sc, _ := scrollWindow(t)
	h.frame()
	y := 0
	nudge := func() { y += 10; sc.SetOffset(0, y) }
	h.warm(nudge, 4)
	nudge()
	painted, damage := h.frame()
	if limit := 400 * 10 * 2; painted > limit {
		t.Errorf("a 10 px scroll step painted %d px, want its strip, at most %d", painted, limit)
	}
	if got := render.UnionAll(damage); got.Intersect(sc.Bounds()) != got || got.W*got.H < 300*300 {
		t.Errorf("wire damage %+v, want the viewport %+v, which moved", got, sc.Bounds())
	}
}

func TestDeferredScrollStepsPaintTheirViewport(t *testing.T) {
	h, sc, _ := scrollWindow(t)
	h.frame()
	sc.SetOffset(0, 40)
	h.wnd.syncBlur()
	h.wnd.router.Root.Arrange(render.Rect{W: 400, H: 300})
	h.wnd.damage.Collect(h.wnd.router.Root, render.Rect{W: 400, H: 300})
	sc.SetOffset(0, 90)
	h.frame()
	assertShowsWholePaint(t, h, 0)
}

func TestScrollStepAtAFractionalScaleMatchesAWholePaint(t *testing.T) {
	h, sc, _ := scrollWindow(t)
	h.wnd.frac120 = 180
	h.frame()
	y := 0
	for _, d := range []int{2, 4, 3, 7, 2} {
		y += d
		sc.SetOffset(0, y)
		h.frame()
	}
	front := h.wnd.pool.Front()
	w, ht := front.Width, front.Height
	buf := make([]byte, front.Stride*ht)
	cv := render.NewScaled(buf, front.Stride, w, ht, 180, scale.Denom)
	h.wnd.paintTree(cv, render.Rect{W: w, H: ht})
	for at := range buf {
		if buf[at] != front.Data[at] {
			t.Fatalf("byte %d at 1.5x differs from a whole paint", at)
		}
	}
}

func TestStyledScrollBlitMatchesAWholePaint(t *testing.T) {
	face, err := Font("sans", 14)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { widget.LoadStylesheet("") })
	widget.LoadStylesheet(`
		.row { background-color: #334455; border-radius: 4px; box-shadow: 0 1px 3px #000000; margin: 3px; padding: 2px; }
		.faded { opacity: 0.6; }
		.moved { transform: translate(4px, 2px); }
		.panel { background-color: #101820; padding: 6px; }
	`)
	for name, decorate := range map[string]func(i int, row *widget.Box){
		"rounded shadowed rows": func(_ int, row *widget.Box) { row.AddClass("row") },
		"group opacity":         func(i int, row *widget.Box) { row.AddClass("row"); row.SetClass("faded", i%3 == 0) },
		"a transform":           func(i int, row *widget.Box) { row.SetClass("moved", i%5 == 0) },
	} {
		t.Run(name, func(t *testing.T) {
			col := widget.NewBox(widget.Column, 2, 0)
			for i := range 200 {
				row := widget.NewBox(widget.Row, 4, 0).Append(widget.NewLabel(face, 14, fmt.Sprintf("row %d", i), 0), false)
				decorate(i, row)
				col.Append(row, false)
			}
			sc := widget.NewScroll(col)
			sc.ShowBars = true
			panel := widget.NewBox(widget.Column, 4, 0)
			panel.AddClass("panel")
			panel.Append(widget.NewLabel(face, 14, "a header over the list", 0), false)
			panel.Append(sc, true)
			panel.Append(widget.NewProgressBar(0.4), false)
			h := newPaintHarness(panel, 360, 280)
			h.frame()
			y := 0
			for step, d := range []int{11, 40, 3, -20, 120, -1} {
				y += d
				sc.SetOffset(0, y)
				h.frame()
				assertShowsWholePaint(t, h, step)
			}
		})
	}
}

func TestAWindowAtRestBehindAMorphMovesItsScroll(t *testing.T) {
	h, sc, _ := scrollWindow(t)
	h.wnd.router.Root = widget.NewMorph(h.wnd.router.Root)
	h.frame()
	y := 0
	nudge := func() { y += 10; sc.SetOffset(0, y) }
	h.warm(nudge, 4)
	nudge()
	painted, _ := h.frame()
	if limit := 400 * 10 * 2; painted > limit {
		t.Errorf("a step behind a resting morph painted %d px, want its strip, at most %d", painted, limit)
	}
	assertShowsWholePaint(t, h, 0)
}

func TestAGPUScrollStepCarriesItsPixels(t *testing.T) {
	h, sc, _ := scrollWindow(t)
	w := h.wnd
	var carried []gpu.Carry
	busy := false
	w.renderer = &gpuRenderer{
		name: "test", rec: render.NewRecorder(1, 1, 1, 1), wake: func(time.Duration) {},
		render: func(_ *render.List, _ *gpu.Target, carry []gpu.Carry, _ bool) (gpuFrame, error) {
			if busy {
				return gpuFrame{}, gpu.ErrBusy
			}
			carried = append(carried[:0], carry...)
			return gpuFrame{}, nil
		},
	}
	w.newBuffer = func() (*buffer.Buffer, error) {
		return &buffer.Buffer{Width: 400, Height: 300, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 400, H: 300}), Target: &gpu.Target{}}, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	h.frame()
	sc.SetOffset(0, 30)
	w.dirty = true
	if !w.draw() {
		t.Fatal(w.drawErr)
	}
	view := sc.Bounds()
	if got, limit := w.paintedPixels, view.W*30; got > limit {
		t.Errorf("a GPU step painted %d px, want the strip it exposed, at most %d", got, limit)
	}
	var moved *gpu.Carry
	for i := range carried {
		if carried[i].Offset != (image.Point{}) {
			moved = &carried[i]
		}
	}
	if moved == nil {
		t.Fatalf("the step carried %v, want the view moved", carried)
	}
	if moved.Offset != image.Pt(0, 30) {
		t.Errorf("the view moved by %v, want 30 px up", moved.Offset)
	}
	for i, a := range carried {
		for _, ra := range a.Rects {
			for _, b := range carried[i+1:] {
				for _, rb := range b.Rects {
					if !ra.Intersect(rb).Empty() {
						t.Errorf("carries overlap at %v: the stale copy wrote what the move overwrites", ra.Intersect(rb))
					}
				}
			}
		}
	}

	busy = true
	sc.SetOffset(0, 60)
	w.dirty = true
	if !w.draw() {
		t.Fatal(w.drawErr)
	}
	busy = false
	h.frame()
	if got, want := w.paintedPixels, view.W*view.H; got < want {
		t.Errorf("after a refused step the next frame painted %d px, want the moved view whole, %d", got, want)
	}
}

func TestAllocBudgetScrollFrame(t *testing.T) {
	quietAllocBudget(t)
	h, sc, _ := scrollWindow(t)
	h.frame()
	y := 0
	step := func() {
		y = (y + 7) % 900
		sc.SetOffset(0, y)
		h.frame()
	}
	for range 100 {
		step()
	}
	const budget = 0
	if allocs := testing.AllocsPerRun(100, step); allocs > budget {
		t.Errorf("a scroll frame through draw allocates %v, budget %d", allocs, budget)
	}
}

type stripedRow struct {
	widget.Base
	shade uint8
}

func (r *stripedRow) Measure(axis widget.Axis, _ int) widget.Sizes {
	if axis == widget.Row {
		return widget.FixedSizes(360)
	}
	return widget.FixedSizes(18)
}

func (r *stripedRow) Paint(cv *render.Canvas) {
	b := r.Bounds()
	cv.FillRect(b, render.RGB(r.shade, 80, 120))
	cv.FillRect(render.Rect{X: b.X + 4, Y: b.Y + 4, W: int(r.shade) % 200, H: 6}, render.RGB(240, 240, 240))
}

func (r *stripedRow) HitTest(p widget.Point) widget.Widget { return r.HitLeaf(r, p) }

func TestAScrollOfBaseRowsBlitsAndMatchesAWholePaint(t *testing.T) {
	col := widget.NewBox(widget.Column, 0, 0)
	for i := range 2000 {
		row := &stripedRow{shade: uint8(i * 37)}
		row.SetInkBounds(render.Insets{})
		col.Append(row, false)
	}
	sc := widget.NewScroll(col)
	h := newPaintHarness(widget.NewBox(widget.Column, 0, 8).Append(sc, true), 400, 300)
	h.frame()
	y := 0
	nudge := func() { y += 10; sc.SetOffset(0, y) }
	h.warm(nudge, 4)
	for step := range 6 {
		nudge()
		painted, _ := h.frame()
		if limit := 400 * 10 * 3; painted > limit {
			t.Errorf("step %d painted %d px, want its strip and the scrollbar, at most %d", step, painted, limit)
		}
		assertShowsWholePaint(t, h, step)
	}
}

func platedScrollWindow(t *testing.T, opaque bool) (*paintHarness, *widget.Scroll) {
	t.Helper()
	h, sc, _ := scrollWindow(t)
	m := widget.NewMorph(h.wnd.router.Root)
	m.SetRadius(8)
	m.SetPlate(6, func(cv *render.Canvas, content render.Rect) {
		cv.RoundedRect(content, 8, render.RGB(30, 34, 40))
	})
	m.SetPlateOpaque(opaque)
	h.wnd.router.Root = m
	return h, sc
}

func TestAScrollOnAnOpaquePlateMovesAndMatchesAWholePaint(t *testing.T) {
	h, sc := platedScrollWindow(t, true)
	h.frame()
	y := 0
	nudge := func() { y += 10; sc.SetOffset(0, y) }
	h.warm(nudge, 4)
	for step := range 4 {
		nudge()
		painted, _ := h.frame()
		if limit := 400 * 10 * 3; painted > limit {
			t.Errorf("step %d on an opaque plate painted %d px, want its strip and the scrollbar, at most %d", step, painted, limit)
		}
		assertShowsWholePaint(t, h, step)
	}
}

func TestAScrollOnAPlateNotDeclaredOpaqueRepaintsItsView(t *testing.T) {
	h, sc := platedScrollWindow(t, false)
	h.frame()
	y := 0
	nudge := func() { y += 10; sc.SetOffset(0, y) }
	h.warm(nudge, 4)
	nudge()
	if painted, _ := h.frame(); painted < sc.Bounds().W*sc.Bounds().H {
		t.Errorf("a step on a plate that may not be flat painted %d px, want the whole view", painted)
	}
	assertShowsWholePaint(t, h, 0)
}

func TestAScrollReachingAnOpaquePlatesCornersMatchesAWholePaint(t *testing.T) {
	face, err := Font("sans", 14)
	if err != nil {
		t.Skip(err)
	}
	col := widget.NewBox(widget.Column, 0, 0)
	for i := range 800 {
		col.Append(widget.NewLabel(face, 14, fmt.Sprintf("row %d reaching the corner", i), 0), false)
	}
	sc := widget.NewScroll(col)
	m := widget.NewMorph(sc)
	m.SetRadius(16)
	m.SetPlate(6, func(cv *render.Canvas, content render.Rect) {
		cv.RoundedRect(content, 16, render.RGB(30, 34, 40))
	})
	m.SetPlateOpaque(true)
	h := newPaintHarness(m, 400, 300)
	h.frame()
	y := 0
	for step, d := range []int{10, 3, 25, 10, -7, 40, -12, 10} {
		y += d
		sc.SetOffset(0, y)
		painted, _ := h.frame()
		if limit := 400 * 120; painted > limit {
			t.Errorf("step %d painted %d px, want the strip and the corners, not the view", step, painted)
		}
		assertShowsWholePaint(t, h, step)
	}
}

func TestAScrollHoldingStickyContentCopiesTheRestAndRepaintsTheStickyPart(t *testing.T) {
	face, err := Font("sans", 14)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { widget.LoadStylesheet("") })
	widget.LoadStylesheet(`.pinned { position: sticky; top: 0px; background-color: #203040; }`)
	col := widget.NewBox(widget.Column, 2, 4)
	header := widget.NewLabel(face, 14, "a header that sticks", 0)
	header.AddClass("pinned")
	col.Append(header, false)
	for i := range 800 {
		col.Append(widget.NewLabel(face, 14, fmt.Sprintf("row %d under the header", i), 0), false)
	}
	sc := widget.NewScroll(col)
	h := newPaintHarness(widget.NewBox(widget.Column, 0, 8).Append(sc, true), 400, 300)
	h.frame()
	y := 0
	nudge := func() { y += 10; sc.SetOffset(0, y) }
	h.warm(nudge, 4)
	for step := range 6 {
		nudge()
		painted, _ := h.frame()
		pinned := header.Bounds()
		if limit := 400*10*3 + 2*pinned.W*pinned.H; painted > limit {
			t.Errorf("step %d painted %d px, want the strip, the scrollbar and the sticky header, at most %d", step, painted, limit)
		}
		if view := sc.Bounds(); pinned.Y != view.Y+4 && pinned.Y != view.Y {
			t.Errorf("step %d: the header sits at %d, want it held at the top of the view %d", step, pinned.Y, view.Y)
		}
		assertShowsWholePaint(t, h, step)
	}
}

func TestAStickySectionHeaderThatCatchesTheViewMatchesAWholePaint(t *testing.T) {
	face, err := Font("sans", 14)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { widget.LoadStylesheet("") })
	widget.LoadStylesheet(`.pinned { position: sticky; top: 0px; background-color: #203040; }`)
	col := widget.NewBox(widget.Column, 2, 4)
	for section := range 4 {
		box := widget.NewBox(widget.Column, 2, 0)
		header := widget.NewLabel(face, 14, fmt.Sprintf("section %d", section), 0)
		header.AddClass("pinned")
		box.Append(header, false)
		for i := range 30 {
			box.Append(widget.NewLabel(face, 14, fmt.Sprintf("section %d row %d", section, i), 0), false)
		}
		col.Append(box, false)
	}
	sc := widget.NewScroll(col)
	h := newPaintHarness(widget.NewBox(widget.Column, 0, 8).Append(sc, true), 400, 300)
	h.frame()
	y := 0
	for step, d := range []int{10, 37, 120, 3, 250, 18, -40, 60, 300, -7, 11} {
		y += d
		sc.SetOffset(0, y)
		h.frame()
		assertShowsWholePaint(t, h, step)
	}
}

func TestANonASCIILabelBesideAScrollKeepsItsBlit(t *testing.T) {
	face, err := Font("sans", 14)
	if err != nil {
		t.Skip(err)
	}
	col := widget.NewBox(widget.Column, 2, 4)
	for i := range 800 {
		col.Append(widget.NewLabel(face, 14, fmt.Sprintf("row %d of the long list", i), 0), false)
	}
	sc := widget.NewScroll(col)
	title := widget.NewLabel(face, 14, "Åström över ÿ fjärden", 0)
	h := newPaintHarness(widget.NewBox(widget.Column, 6, 8).Append(title, false).Append(sc, true), 400, 300)
	h.frame()
	y := 0
	nudge := func() { y += 10; sc.SetOffset(0, y) }
	h.warm(nudge, 4)
	for step := range 4 {
		nudge()
		painted, _ := h.frame()
		if limit := 400 * 10 * 3; painted > limit {
			t.Errorf("step %d beside a non-ASCII label painted %d px, want the strip and the scrollbar, at most %d", step, painted, limit)
		}
		assertShowsWholePaint(t, h, step)
	}
}
