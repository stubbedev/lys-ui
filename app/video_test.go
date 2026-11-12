package app_test

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/testbuild"
	"github.com/stubbedev/lys-lib/wlr"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/waittest"
	"github.com/stubbedev/lys-ui/media"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	surfaceDamage                   = 2
	surfaceFrame                    = 3
	surfaceSetOpaqueRegion          = 4
	surfaceDamageBuffer             = 9
	subcompositorGetSubsurface      = 1
	viewporterGetViewport           = 1
	viewportSetDestination          = 2
	dmabufCreateParams              = 1
	dmabufEventModifier             = 1
	paramsCreate                    = 2
	paramsEventCreated              = 0
	bufferEventRelease              = 0
	colorRepGetSurface              = 1
	colorRepEventCoefficients       = 1
	colorRepEventDone               = 2
	colorRepSetCoefficientsAndRange = 2
	contentTypeGetSurface           = 1
	contentTypeSetType              = 1
	firstServerID                   = 0xff000000
	videoW, videoH                  = 32, 16
)

// videoCompositor plays a compositor that imports the YUV formats it
// is given as linear dmabufs: it creates their wl_buffers, releases a
// surface's buffer once the next one replaces it, answers frame
// callbacks on commit, and records what the client asked of every
// surface.
type videoCompositor struct {
	f       *fakeCompositor
	formats []fourcc.Format
	argb    []fourcc.Modifier

	mu            sync.Mutex
	nextServer    uint32
	video         uint32
	pending       map[uint32]uint32
	shown         map[uint32]uint32
	callbacks     map[uint32][]uint32
	surfaceOf     map[uint32]uint32
	buffers       int
	videoCommits  int
	windowDamage  int
	windowCommits int
	colorRep      map[uint32][2]uint32
	contentType   map[uint32]uint32
	destination   map[uint32][2]uint32
	opaque        map[uint32]int
}

var videoGlobals = slices.Concat(kioskGlobals, []fakeGlobal{
	{"wl_subcompositor", 1},
	{"wp_viewporter", 1},
	{"zwp_linux_dmabuf_v1", 3},
	{"wp_color_representation_manager_v1", 1},
	{"wp_content_type_manager_v1", 1},
})

func newVideoCompositor(formats ...fourcc.Format) *videoCompositor {
	return &videoCompositor{
		formats: formats, nextServer: firstServerID,
		pending: map[uint32]uint32{}, shown: map[uint32]uint32{}, callbacks: map[uint32][]uint32{},
		surfaceOf: map[uint32]uint32{}, colorRep: map[uint32][2]uint32{}, contentType: map[uint32]uint32{},
		destination: map[uint32][2]uint32{}, opaque: map[uint32]int{},
	}
}

func (c *videoCompositor) setup(f *fakeCompositor) {
	c.f = f
	f.onBind = c.bound
	f.onRequest = c.request
}

func (c *videoCompositor) bound(iface string, id uint32) {
	switch iface {
	case "zwp_linux_dmabuf_v1":
		for _, format := range c.formats {
			c.f.send(id, dmabufEventModifier, uint32(format), 0, 0)
		}
		for _, m := range c.argb {
			c.f.send(id, dmabufEventModifier, uint32(fourcc.ARGB8888), uint32(m>>32), uint32(m))
		}
	case "wp_color_representation_manager_v1":
		for _, r := range []uint32{wlr.WpColorRepresentationSurfaceV1RangeLimited, wlr.WpColorRepresentationSurfaceV1RangeFull} {
			c.f.send(id, colorRepEventCoefficients, wlr.WpColorRepresentationSurfaceV1CoefficientsBt601, r)
		}
		c.f.send(id, colorRepEventDone)
	}
}

func (c *videoCompositor) request(iface string, obj, opcode uint32, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case iface == "wl_subcompositor" && opcode == subcompositorGetSubsurface:
		c.video = word(body, 1)
	case iface == "wp_viewporter" && opcode == viewporterGetViewport:
		c.f.objects[word(body, 0)] = "wp_viewport"
		c.surfaceOf[word(body, 0)] = word(body, 1)
	case iface == "wp_viewport" && opcode == viewportSetDestination:
		c.destination[c.surfaceOf[obj]] = [2]uint32{word(body, 0), word(body, 1)}
	case iface == "zwp_linux_dmabuf_v1" && opcode == dmabufCreateParams:
		c.f.objects[word(body, 0)] = "zwp_linux_buffer_params_v1"
	case iface == "zwp_linux_buffer_params_v1" && opcode == paramsCreate:
		id := c.nextServer
		c.nextServer++
		c.buffers++
		c.f.objects[id] = "wl_buffer"
		c.f.send(obj, paramsEventCreated, id)
	case iface == "wp_color_representation_manager_v1" && opcode == colorRepGetSurface:
		c.f.objects[word(body, 0)] = "wp_color_representation_surface_v1"
		c.surfaceOf[word(body, 0)] = word(body, 1)
	case iface == "wp_color_representation_surface_v1" && opcode == colorRepSetCoefficientsAndRange:
		c.colorRep[c.surfaceOf[obj]] = [2]uint32{word(body, 0), word(body, 1)}
	case iface == "wp_content_type_manager_v1" && opcode == contentTypeGetSurface:
		c.f.objects[word(body, 0)] = "wp_content_type_v1"
		c.surfaceOf[word(body, 0)] = word(body, 1)
	case iface == "wp_content_type_v1" && opcode == contentTypeSetType:
		c.contentType[c.surfaceOf[obj]] = word(body, 0)
	case iface == "wl_surface":
		c.surfaceRequest(obj, opcode, body)
	}
}

func (c *videoCompositor) surfaceRequest(surface, opcode uint32, body []byte) {
	window := surface != c.video
	switch opcode {
	case surfaceAttach:
		c.pending[surface] = word(body, 0)
		if window {
			c.windowDamage++
		}
	case surfaceDamage, surfaceDamageBuffer:
		if window {
			c.windowDamage++
		}
	case surfaceFrame:
		c.callbacks[surface] = append(c.callbacks[surface], word(body, 0))
	case surfaceSetOpaqueRegion:
		c.opaque[surface]++
	case surfaceCommit:
		for _, cb := range c.callbacks[surface] {
			c.f.send(cb, callbackEventDone, 0)
			c.f.send(wlDisplayID, displayEventDeleteID, cb)
		}
		c.callbacks[surface] = c.callbacks[surface][:0]
		if window {
			c.windowCommits++
		}
		b, ok := c.pending[surface]
		if !ok || b == 0 {
			return
		}
		delete(c.pending, surface)
		if old := c.shown[surface]; old != 0 && old != b {
			c.f.send(old, bufferEventRelease)
		}
		c.shown[surface] = b
		if !window {
			c.videoCommits++
		}
	}
}

type videoCounts struct{ buffers, videoCommits, windowDamage, windowCommits int }

func (c *videoCompositor) counts() videoCounts {
	c.mu.Lock()
	defer c.mu.Unlock()
	return videoCounts{c.buffers, c.videoCommits, c.windowDamage, c.windowCommits}
}

func (c *videoCompositor) awaitVideoCommits(t *testing.T, n int) videoCounts {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		got := c.counts()
		if got.videoCommits >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d video frames committed, want %d: %+v", got.videoCommits, n, got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func videoClip(full bool, frames int) []byte {
	var b bytes.Buffer
	rng := "LIMITED"
	if full {
		rng = "FULL"
	}
	fmt.Fprintf(&b, "YUV4MPEG2 W%d H%d F100:1 C420jpeg XCOLORRANGE=%s\n", videoW, videoH, rng)
	n := media.YUVFrameBytes(videoW, videoH)
	for f := range frames {
		b.WriteString("FRAME\n")
		for i := range n {
			b.WriteByte(byte(16 + (i+3*f)%200))
		}
	}
	return b.Bytes()
}

// testMemory hands out memfd-backed buffers and keeps their mappings,
// for the test to read what the client wrote.
type testMemory struct {
	mu   sync.Mutex
	mems [][]byte
}

func (m *testMemory) alloc(size int) (int, []byte, func() error, error) {
	fd, err := unix.MemfdCreate("lys-ui-video-test", unix.MFD_CLOEXEC)
	if err != nil {
		return -1, nil, nil, err
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		_ = unix.Close(fd)
		return -1, nil, nil, err
	}
	mem, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		_ = unix.Close(fd)
		return -1, nil, nil, err
	}
	m.mu.Lock()
	m.mems = append(m.mems, mem)
	m.mu.Unlock()
	return fd, mem, func() error { return errors.Join(unix.Munmap(mem), unix.Close(fd)) }, nil
}

func (m *testMemory) written() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []int
	for _, mem := range m.mems {
		n := 0
		for _, b := range mem {
			if b != 0 {
				n++
			}
		}
		out = append(out, n)
	}
	return out
}

// wantFrames checks that each ring buffer the client wrote holds exactly
// one frame's bytes, its padding untouched, and that frames went to
// more than one buffer: a compositor releasing each buffer as the next
// replaces it keeps two of the three busy.
func wantFrames(t *testing.T, mem *testMemory, frame int) {
	t.Helper()
	used := 0
	for i, n := range mem.written() {
		switch n {
		case 0:
		case frame:
			used++
		default:
			t.Errorf("ring buffer %d holds %d written bytes, want one frame's %d", i, n, frame)
		}
	}
	if used < 2 {
		t.Errorf("frames went to %d ring buffers, want them cycled", used)
	}
}

func showVideo(t *testing.T, c *videoCompositor, full bool) (*app.Application, *widget.Video, *testMemory) {
	t.Helper()
	_, a := connectFakeWith(t, videoGlobals, c.setup)
	mem := &testMemory{}
	a.SetVideoMemory(mem.alloc)
	src, err := media.NewY4M(bytes.NewReader(videoClip(full, 24)))
	if err != nil {
		t.Fatal(err)
	}
	stream := media.NewStream(src, "test")
	t.Cleanup(stream.Close)
	stream.SetLoop(true)
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	v := widget.NewVideo(face, 13, stream)
	v.SetAutoplay(true)
	if _, err := a.NewWindow(app.WindowConfig{Title: "video", Width: fakeSurfaceW, Height: fakeSurfaceH, Root: v}); err != nil {
		t.Fatal(err)
	}
	runUntilCleanup(t, a)
	return a, v, mem
}

func offloadErr(a *app.Application, v *widget.Video) error {
	got := make(chan error, 1)
	a.Invoke(func() { got <- v.OffloadErr() })
	return <-got
}

// TestVideoPlaysOnAYUVSubsurface pins #460: a Video of a YUV stream on
// a compositor importing linear YUV dmabufs shows its frames on a
// subsurface of their own. Each frame writes exactly width x height x
// 1.5 bytes into one of a fixed ring of buffers and costs no repaint of
// the window and no heap allocation once warm; the subsurface carries
// the stream's colour representation and the video content type, and
// the ring stays at its three buffers over a thousand frames.
func TestVideoPlaysOnAYUVSubsurface(t *testing.T) {
	c := newVideoCompositor(fourcc.YUV420, fourcc.NV12)
	a, v, mem := showVideo(t, c, false)
	c.awaitVideoCommits(t, 60)
	if err := offloadErr(a, v); err != nil {
		t.Fatalf("the frames are painted into the window: %v", err)
	}

	const steady = 120
	var (
		before videoCounts
		allocs uint64
	)
	func() {
		defer debug.SetGCPercent(debug.SetGCPercent(-1))
		c.awaitVideoCommits(t, c.counts().videoCommits+10)
		before = c.counts()
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		c.awaitVideoCommits(t, before.videoCommits+steady)
		runtime.ReadMemStats(&m1)
		allocs = m1.Mallocs - m0.Mallocs
	}()
	after := c.counts()
	frames := after.videoCommits - before.videoCommits
	t.Logf("%d steady video frames: %d heap allocations, %d window damage requests, %d window commits", frames, allocs, after.windowDamage-before.windowDamage, after.windowCommits-before.windowCommits)
	if d := after.windowDamage - before.windowDamage; d != 0 {
		t.Errorf("the window took %d damage or attach requests over %d video frames, want none", d, frames)
	}
	if d := after.windowCommits - before.windowCommits; d != 0 {
		t.Errorf("the window committed %d times over %d video frames, want none", d, frames)
	}
	if testbuild.AllocBudgetsHold && allocs/uint64(frames) > 0 {
		t.Errorf("%d heap allocations over %d steady video frames, want none per frame", allocs, frames)
	}

	want := media.YUVFrameBytes(videoW, videoH)
	if want != videoW*videoH*3/2 {
		t.Fatalf("a %dx%d frame is %d bytes", videoW, videoH, want)
	}
	wantFrames(t, mem, want)

	c.mu.Lock()
	video := c.video
	rep, ct, dst, opaque := c.colorRep[video], c.contentType[video], c.destination[video], c.opaque[video]
	c.mu.Unlock()
	if rep != [2]uint32{wlr.WpColorRepresentationSurfaceV1CoefficientsBt601, wlr.WpColorRepresentationSurfaceV1RangeLimited} {
		t.Errorf("the video surface's colour representation is %v, want BT.601 limited", rep)
	}
	if ct != wlr.WpContentTypeV1TypeVideo {
		t.Errorf("the video surface's content type is %d, want video", ct)
	}
	if dst != [2]uint32{fakeSurfaceW, fakeSurfaceW * videoH / videoW} {
		t.Errorf("the viewport scales the frames to %v, want the 2:1 frame fitted into %dx%d", dst, fakeSurfaceW, fakeSurfaceH)
	}
	if opaque == 0 {
		t.Error("the video surface is not marked opaque")
	}

	end := c.awaitVideoCommits(t, 1000)
	if end.buffers != 3 || len(mem.written()) != 3 {
		t.Errorf("%d wl_buffers and %d allocations after %d frames, want the ring's 3", end.buffers, len(mem.written()), end.videoCommits)
	}
}

// Full-range video says so, and a compositor offering only NV12 gets
// the chroma interleaved: the same bytes, in NV12's layout.
func TestVideoTagsItsRangeAndWritesNV12(t *testing.T) {
	c := newVideoCompositor(fourcc.NV12)
	a, v, mem := showVideo(t, c, true)
	c.awaitVideoCommits(t, 10)
	if err := offloadErr(a, v); err != nil {
		t.Fatalf("the frames are painted into the window: %v", err)
	}
	c.mu.Lock()
	rep := c.colorRep[c.video]
	c.mu.Unlock()
	if rep != [2]uint32{wlr.WpColorRepresentationSurfaceV1CoefficientsBt601, wlr.WpColorRepresentationSurfaceV1RangeFull} {
		t.Errorf("colour representation %v, want BT.601 full", rep)
	}
	wantFrames(t, mem, videoW*videoH*3/2)
}

// A compositor importing no YUV dmabufs leaves the frames painted into
// the window, as before #460: the Video says why, and every frame
// repaints the window.
func TestVideoWithoutYUVDmabufsPaintsIntoTheWindow(t *testing.T) {
	c := newVideoCompositor(fourcc.ARGB8888)
	a, v, mem := showVideo(t, c, false)
	waittest.Until(t, func() bool { return c.counts().windowCommits >= 10 }, "the window to commit 10 frames of the playing video")
	if err := offloadErr(a, v); !errors.Is(err, app.ErrNoYUVDmabuf) {
		t.Errorf("OffloadErr = %v, want ErrNoYUVDmabuf", err)
	}
	if got := c.counts(); got.videoCommits != 0 || got.buffers != 0 || len(mem.written()) != 0 {
		t.Errorf("a video surface without YUV formats: %+v, %d allocations", got, len(mem.written()))
	}
}
