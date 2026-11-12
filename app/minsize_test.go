package app

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/widget"
)

type minSizeHarness struct {
	*paintHarness
	content *widget.DrawingArea
	sent    []widget.Size
}

func newMinSizeHarness(t *testing.T, root widget.Widget, content *widget.DrawingArea, limits *window.Window) *minSizeHarness {
	t.Helper()
	h := &minSizeHarness{paintHarness: newPaintHarness(root, 300, 200), content: content}
	h.wnd.styleRoot = root
	h.wnd.limits = limits.SizeLimits
	minW, minH, _, _ := limits.SizeLimits()
	h.wnd.minSent = widget.Size{W: minW, H: minH}
	h.wnd.publishMin = func(s widget.Size) error {
		h.sent = append(h.sent, s)
		return nil
	}
	return h
}

func (h *minSizeHarness) wantSent(t *testing.T, step string, want ...widget.Size) {
	t.Helper()
	if !slices.Equal(h.sent, want) {
		t.Fatalf("%s: set_min_size went out as %v, want %v", step, h.sent, want)
	}
}

func (h *minSizeHarness) wantBuffer(t *testing.T, step string, w, hgt int) {
	t.Helper()
	last := h.bufs[len(h.bufs)-1]
	if last.Width != w || last.Height != hgt {
		t.Fatalf("%s: the window drew %dx%d, want %dx%d", step, last.Width, last.Height, w, hgt)
	}
}

func TestAToplevelsMinimumFollowsItsContent(t *testing.T) {
	content := widget.NewDrawingArea(200, 60)
	h := newMinSizeHarness(t, content, content, &window.Window{})
	h.frame()
	h.wantSent(t, "the first frame", widget.Size{W: 200, H: 60})
	h.frame()
	h.wantSent(t, "a frame with the content unchanged", widget.Size{W: 200, H: 60})
	content.SetContentSize(240, 320)
	h.frame()
	h.wantSent(t, "the content grown past the window", widget.Size{W: 200, H: 60}, widget.Size{W: 240, H: 320})
	h.wantBuffer(t, "the content grown past the window", 300, 200)
	content.SetContentSize(100, 40)
	h.frame()
	h.wantSent(t, "the content shrunk", widget.Size{W: 200, H: 60}, widget.Size{W: 240, H: 320}, widget.Size{W: 100, H: 40})
	h.wantBuffer(t, "the content shrunk", 300, 200)
}

func TestAToplevelsMinimumStaysWithinItsConfiguredLimits(t *testing.T) {
	content := widget.NewDrawingArea(200, 60)
	limits := &window.Window{}
	if err := limits.SetMinSize(250, 100); err != nil {
		t.Fatal(err)
	}
	if err := limits.SetMaxSize(0, 220); err != nil {
		t.Fatal(err)
	}
	h := newMinSizeHarness(t, content, content, limits)
	h.frame()
	h.wantSent(t, "content inside the configured minimum")
	content.SetContentSize(280, 400)
	h.frame()
	h.wantSent(t, "content past the configured maximum", widget.Size{W: 280, H: 220})
	content.SetContentSize(10, 10)
	h.frame()
	h.wantSent(t, "content shrunk below the configured minimum", widget.Size{W: 280, H: 220}, widget.Size{W: 250, H: 100})
}

func TestScrollingContentKeepsAToplevelShrinkable(t *testing.T) {
	content := widget.NewDrawingArea(800, 2000)
	h := newMinSizeHarness(t, widget.NewScroll(content), content, &window.Window{})
	h.frame()
	if len(h.sent) > 1 || len(h.sent) == 1 && (h.sent[0].W >= 300 || h.sent[0].H >= 200) {
		t.Fatalf("set_min_size went out as %v, want the scroller's least size, below the 300x200 window", h.sent)
	}
}

func TestAConfiguredMinimumGivesWayToTheContentsAgain(t *testing.T) {
	content := widget.NewDrawingArea(200, 260)
	limits := &window.Window{}
	h := newMinSizeHarness(t, content, content, limits)
	a := accelApp()
	w := &Window{app: a, win: limits}
	h.wnd.win = w
	h.wnd.anim = &a.anim
	a.windows = []*hostWindow{h.wnd}
	h.frame()
	h.wantSent(t, "the first frame", widget.Size{W: 200, H: 260})
	w.SetMinSize(100, 100)
	h.frame()
	h.wantSent(t, "a configured minimum below the content", widget.Size{W: 200, H: 260}, widget.Size{W: 200, H: 260})
	w.SetMinSize(400, 300)
	h.frame()
	h.wantSent(t, "a configured minimum above the content", widget.Size{W: 200, H: 260}, widget.Size{W: 200, H: 260})
	w.SetMaxSize(350, 0)
	h.frame()
	h.wantSent(t, "a configured maximum narrower than the minimum", widget.Size{W: 200, H: 260}, widget.Size{W: 200, H: 260}, widget.Size{W: 350, H: 300})
}
