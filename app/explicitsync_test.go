package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/drm"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/widget"
)

type fakeTimelines struct {
	acquired map[drm.Syncobj][]uint64
	watched  map[drm.Syncobj]func()
	points   map[drm.Syncobj]uint64
}

func newFakeTimelines() *fakeTimelines {
	return &fakeTimelines{acquired: map[drm.Syncobj][]uint64{}, watched: map[drm.Syncobj]func(){}, points: map[drm.Syncobj]uint64{}}
}

func (f *fakeTimelines) create() (drm.Syncobj, int, error) {
	return drm.Syncobj(len(f.points) + 1), -1, nil
}

func (f *fakeTimelines) acquire(s drm.Syncobj, point uint64, _ int) error {
	f.acquired[s] = append(f.acquired[s], point)
	return nil
}

func (f *fakeTimelines) watch(s drm.Syncobj, point uint64, released func()) error {
	f.points[s] = point
	f.watched[s] = released
	return nil
}

func (f *fakeTimelines) destroy(drm.Syncobj) {}
func (f *fakeTimelines) close()              {}

// signal is the compositor signalling every release point watched.
func (f *fakeTimelines) signal() {
	for s, released := range f.watched {
		delete(f.watched, s)
		released()
	}
}

type pointsSurface struct {
	*fakeSurface
	points [][2]uint64
	plain  int
}

func (p *pointsSurface) Attach(b *buffer.Buffer) error {
	if t := b.Timeline; t != nil {
		p.points = append(p.points, [2]uint64{t.Acquire, t.Release})
	} else {
		p.plain++
	}
	return p.fakeSurface.Attach(b)
}

func explicitHarness(t *testing.T, explicit bool) (*paintHarness, *pointsSurface, *fakeTimelines, *[2]int) {
	t.Helper()
	label := widget.NewSpacer(40, 20)
	h := newPaintHarness(label, 120, 60)
	w := h.wnd
	surf := &pointsSurface{fakeSurface: h.surf}
	w.surf = surf
	var calls [2]int
	tl := newFakeTimelines()
	syncFile := -1
	w.renderer = &gpuRenderer{
		name: "test", rec: render.NewRecorder(1, 1, 1, 1), wake: func(time.Duration) {},
		render: func(_ *render.List, _ *gpu.Target, _ []gpu.Carry, explicit bool) (gpuFrame, error) {
			if explicit {
				calls[1]++
				fd, err := unix.Eventfd(0, unix.EFD_CLOEXEC)
				if err != nil {
					t.Fatal(err)
				}
				syncFile = fd
				return gpuFrame{syncFile: fd}, nil
			}
			calls[0]++
			return gpuFrame{syncFile: -1}, nil
		},
	}
	if explicit {
		w.renderer.timelines = tl
	}
	w.newBuffer = func() (*buffer.Buffer, error) {
		b := &buffer.Buffer{Width: 120, Height: 60, Scale: 1, Stale: buffer.RegionOf(render.Rect{W: 120, H: 60}), Target: &gpu.Target{}}
		if explicit {
			s, _, _ := tl.create()
			tl.points[s] = 0
			b.Timeline = buffer.NewTimeline(nil, s, func() {})
			b.Timeline.Released = b.Release
		}
		h.bufs = append(h.bufs, b)
		return b, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	t.Cleanup(func() {
		if syncFile >= 0 {
			if _, err := unix.FcntlInt(uintptr(syncFile), unix.F_GETFD, 0); err == nil {
				t.Error("a frame's sync_file was left open")
			}
		}
	})
	return h, surf, tl, &calls
}

func explicitFrame(t *testing.T, h *paintHarness) {
	t.Helper()
	h.wnd.router.Root.(interface{ InvalidateRect(render.Rect) }).InvalidateRect(render.Rect{W: 4, H: 4})
	h.wnd.dirty = true
	if !h.wnd.draw() {
		t.Fatal(h.wnd.drawErr)
	}
}

func TestAnExplicitWindowSetsBothPointsOnEveryCommitAndNeverWaits(t *testing.T) {
	h, surf, tl, calls := explicitHarness(t, true)
	for range 6 {
		explicitFrame(t, h)
		tl.signal()
	}
	if calls[0] != 0 || calls[1] != 6 {
		t.Errorf("%d frames took the waiting path and %d the explicit one, want 0 and 6", calls[0], calls[1])
	}
	if len(surf.points) != h.surf.commits || surf.plain != 0 {
		t.Fatalf("%d commits, %d with points and %d without, want points on every one", h.surf.commits, len(surf.points), surf.plain)
	}
	for i, p := range surf.points {
		if p[0] == 0 || p[0] >= p[1] {
			t.Errorf("commit %d: acquire %d, release %d, want 0 < acquire < release on one timeline", i, p[0], p[1])
		}
	}
	for s, points := range tl.acquired {
		for i := 1; i < len(points); i++ {
			if points[i] <= points[i-1] {
				t.Errorf("timeline %d acquired %v, want its points rising", s, points)
			}
		}
	}
}

func TestAnExplicitBufferIsReusedOnlyOnceItsReleasePointSignals(t *testing.T) {
	h, _, tl, _ := explicitHarness(t, true)
	h.wnd.pool = buffer.New(h.wnd.allocator(), 1)
	explicitFrame(t, h)
	b := h.bufs[0]
	buffer.ReleaseHandler{B: b}.HandleBufferRelease(wl.BufferReleaseEvent{})
	if !b.Busy() {
		t.Fatal("wl_buffer.release freed a buffer whose release point has not signalled")
	}
	explicitFrame(t, h)
	if h.surf.commits != 1 {
		t.Fatalf("%d commits with the only buffer unreleased, want the frame deferred", h.surf.commits)
	}
	tl.signal()
	if b.Busy() {
		t.Fatal("the release point signalled and the buffer stayed busy")
	}
	explicitFrame(t, h)
	if h.surf.commits != 2 {
		t.Errorf("%d commits after the release point, want the deferred frame drawn", h.surf.commits)
	}
}

func TestACompositorWithoutExplicitSyncKeepsTodaysPath(t *testing.T) {
	h, surf, _, calls := explicitHarness(t, false)
	for range 3 {
		explicitFrame(t, h)
		for _, b := range h.bufs {
			b.Release()
		}
	}
	if calls[1] != 0 || calls[0] != 3 || len(surf.points) != 0 || surf.plain != 3 {
		t.Errorf("explicit %d, implicit %d frames, %d commits with points; want 3 implicit frames and no points", calls[1], calls[0], len(surf.points))
	}
}

func TestExplicitSyncTakesTheCompositorsNodeOrNone(t *testing.T) {
	if _, ok := syncNode(rendererPlan{}); ok {
		t.Error("a compositor that named no device got a node to guess at")
	}
	if _, ok := syncNode(rendererPlan{node: unix.Mkdev(1, 3), hasNode: true}); ok {
		t.Error("a device with no render node got one")
	}
	const major, minor = 226, 128
	if _, err := os.Stat("/sys/dev/char/226:128/device/drm/renderD128"); err != nil {
		t.Skip("no DRM render node in /sys")
	}
	if node, ok := syncNode(rendererPlan{node: unix.Mkdev(major, minor), hasNode: true}); !ok || node != "/dev/dri/renderD128" {
		t.Errorf("the compositor's renderD128 gave %q, %v", node, ok)
	}
}

func TestDRMTimelinesHandAReleaseToTheLoop(t *testing.T) {
	nodes, _ := filepath.Glob("/dev/dri/renderD*")
	if len(nodes) == 0 {
		t.Skip("no DRM render node")
	}
	tl, err := openTimelines(nodes[0])
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(tl.close)
	queued := make(chan func(), 1)
	tl.setInvoke(func(fn func()) { queued <- fn })
	s, fd, err := tl.create()
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if err := tl.acquire(s, 1, -1); err != nil {
		t.Fatal(err)
	}
	released := false
	if err := tl.watch(s, 2, func() { released = true }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-queued:
		t.Fatal("the release came before its point signalled")
	case <-time.After(50 * time.Millisecond):
	}
	if err := tl.dev.Signal(s, 2); err != nil {
		t.Fatal(err)
	}
	select {
	case fn := <-queued:
		fn()
	case <-time.After(time.Minute):
		t.Fatal("the signalled release point never reached the loop")
	}
	if !released {
		t.Error("the release ran something else")
	}
	tl.destroy(s)
}
