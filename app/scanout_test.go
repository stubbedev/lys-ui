package app

import (
	"errors"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	feedbackDevice = uint64(226<<8 | 128)
	feedbackTiled  = fourcc.Modifier(0x0100000000000001)
	feedbackOther  = fourcc.Modifier(0x0100000000000002)
)

var (
	renderTranche = wltest.FeedbackTranche{Device: feedbackDevice, Formats: []wltest.FormatModifier{
		{Format: fourcc.ARGB8888, Modifier: feedbackTiled}, {Format: fourcc.ARGB8888, Modifier: fourcc.Linear},
	}}
	scanoutTranche = wltest.FeedbackTranche{Device: feedbackDevice, Flags: wlr.ZwpDmabufFeedbackV1TrancheFlagsScanout, Formats: []wltest.FormatModifier{
		{Format: fourcc.ARGB8888, Modifier: feedbackOther}, {Format: fourcc.ARGB8888, Modifier: fourcc.Linear},
	}}
)

type feedbackClient struct {
	display *wl.Display
	reg     *wl.Registry
	comp    *wl.Compositor
	dmabuf  *wlr.ZwpDmabufV1
}

func (c *feedbackClient) HandleRegistryGlobal(ev wl.RegistryGlobalEvent) {
	switch ev.Interface {
	case "wl_compositor":
		c.comp = wl.NewCompositor(c.display.Context())
		_ = c.reg.Bind(ev.Name, ev.Interface, 4, c.comp)
	case "zwp_linux_dmabuf_v1":
		c.dmabuf = wlr.NewZwpDmabufV1(c.display.Context())
		_ = c.reg.Bind(ev.Name, ev.Interface, 4, c.dmabuf)
	}
}

func (c *feedbackClient) HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent) {}

func (c *feedbackClient) roundtrip(t *testing.T) {
	t.Helper()
	cb, err := c.display.Sync()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.display.Context().RunTill(cb); err != nil {
		t.Fatal(err)
	}
}

func connectFeedbackClient(t *testing.T) *feedbackClient {
	t.Helper()
	display, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = display.Context().Close() })
	c := &feedbackClient{display: display}
	if c.reg, err = display.GetRegistry(); err != nil {
		t.Fatal(err)
	}
	c.reg.AddGlobalHandler(c)
	c.roundtrip(t)
	if c.comp == nil || c.dmabuf == nil {
		t.Fatal("the fake offers no wl_compositor or zwp_linux_dmabuf_v1")
	}
	return c
}

type scanoutHarness struct {
	*paintHarness
	allocs [][]fourcc.Modifier
}

func newScanoutHarness(t *testing.T, c *feedbackClient) *scanoutHarness {
	t.Helper()
	h := &scanoutHarness{paintHarness: newPaintHarness(widget.NewSpacer(40, 30), 40, 30)}
	w := h.wnd
	w.renderer = &gpuRenderer{
		name: "test", rec: render.NewRecorder(1, 1, 1, 1),
		node: feedbackDevice, hasNode: true, modifiers: []fourcc.Modifier{fourcc.Linear, feedbackTiled},
		render: func(*render.List, *gpu.Target, []gpu.Carry, bool) (gpuFrame, error) { return gpuFrame{}, nil },
	}
	w.newBuffer = func() (*buffer.Buffer, error) {
		h.allocs = append(h.allocs, w.bufferModifiers())
		b := fakeBuffer(40, 30)
		h.bufs = append(h.bufs, b)
		return b, nil
	}
	w.pool = buffer.New(w.allocator(), 3)
	surface, err := c.comp.CreateSurface()
	if err != nil {
		t.Fatal(err)
	}
	watch, err := dmabufwl.WatchSurfaceFeedback(c.dmabuf, 4, surface, w.dmabufFeedback)
	if err != nil {
		t.Fatal(err)
	}
	w.feedbackWatch = watch
	return h
}

func (h *scanoutHarness) warmed(t *testing.T) {
	t.Helper()
	h.frame()
	h.frame()
	h.allocs = nil
	h.frame()
	if len(h.allocs) != 0 {
		t.Fatalf("a warm pool allocated %v", h.allocs)
	}
}

func TestAWindowAllocatesFromItsSurfacesScanoutTranche(t *testing.T) {
	f := wltest.NewDmabufFeedback(t, feedbackDevice, renderTranche)
	c := connectFeedbackClient(t)
	h := newScanoutHarness(t, c)
	c.roundtrip(t)
	if f.Sent() != 1 {
		t.Fatalf("%d feedbacks sent, want the first", f.Sent())
	}
	h.warmed(t)

	f.SetFeedback(feedbackDevice, scanoutTranche, renderTranche)
	c.roundtrip(t)
	h.frame()
	if want := [][]fourcc.Modifier{{fourcc.Linear}}; !slices.EqualFunc(h.allocs, want, slices.Equal) {
		t.Fatalf("after the scanout tranche the window allocated %v, want %v", h.allocs, want)
	}

	h.allocs = nil
	h.wnd.refuseScanout(errors.New("refused"))
	h.wnd.refuseScanout(errors.New("refused again"))
	h.frame()
	if want := [][]fourcc.Modifier{nil}; !slices.EqualFunc(h.allocs, want, slices.Equal) {
		t.Errorf("after the scanout buffer failed the window allocated %v, want %v", h.allocs, want)
	}
	if err := h.wnd.feedbackWatch.Destroy(); err != nil {
		t.Fatal(err)
	}
}

func TestAFeedbackResentUnchangedReallocatesNothing(t *testing.T) {
	f := wltest.NewDmabufFeedback(t, feedbackDevice, renderTranche)
	c := connectFeedbackClient(t)
	h := newScanoutHarness(t, c)
	c.roundtrip(t)
	h.warmed(t)

	f.SetFeedback(feedbackDevice, scanoutTranche, renderTranche)
	c.roundtrip(t)
	h.frame()
	if want := [][]fourcc.Modifier{{fourcc.Linear}}; !slices.EqualFunc(h.allocs, want, slices.Equal) {
		t.Fatalf("a change allocated %v, want one buffer in %v", h.allocs, want)
	}
	h.frame()
	h.frame()

	h.allocs = nil
	f.SetFeedback(feedbackDevice, scanoutTranche, renderTranche)
	c.roundtrip(t)
	h.frame()
	if f.Sent() != 3 || len(h.allocs) != 0 {
		t.Errorf("the feedback resent unchanged (%d sent) allocated %v", f.Sent(), h.allocs)
	}
}

func TestATargetIsPreparedInTheModifiersAsked(t *testing.T) {
	r := &gpuRenderer{modifiers: []fourcc.Modifier{feedbackTiled, fourcc.Linear}, invoke: func(fn func()) { fn() }}
	var asked [][]fourcc.Modifier
	r.prepare = func(_, _ int, mods []fourcc.Modifier) (*gpu.Target, error) {
		asked = append(asked, mods)
		return nil, errors.New("no device")
	}
	r.allocateIn([]fourcc.Modifier{fourcc.Linear})(8, 8, func(*gpu.Target, error) {})
	r.workers.Wait()
	r.allocateIn(nil)(8, 8, func(*gpu.Target, error) {})
	r.workers.Wait()
	if want := [][]fourcc.Modifier{{fourcc.Linear}, {feedbackTiled, fourcc.Linear}}; !slices.EqualFunc(asked, want, slices.Equal) {
		t.Errorf("targets prepared in %v, want %v", asked, want)
	}
}
