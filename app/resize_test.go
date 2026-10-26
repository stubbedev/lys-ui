// Interactive resize and configure-driven relayout coverage: the edge
// hit test and its cursor shapes, the frame-grabbing edge press,
// configure relayout at several sizes (the first frame at a new size is
// already correct), and the min/max clamps on layout. The wire requests
// themselves are pinned over a loopback socket in internal/window.
package app

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/widget"
)

func TestResizeEdgeDetection(t *testing.T) {
	const w, h, border = 300, 200, 6

	t.Run("interior points match no edge", func(t *testing.T) {
		for _, p := range []struct{ x, y float64 }{
			{150, 100},
			{border + 1, border + 1},
			{float64(w) - border - 1, float64(h) - border - 1},
		} {
			if e := resizeEdge(w, h, p.x, p.y); e != 0 {
				t.Errorf("resizeEdge(%.0f,%.0f) = %d, want 0", p.x, p.y, e)
			}
		}
	})

	t.Run("edges and corners map to the protocol bits", func(t *testing.T) {
		cases := []struct {
			x, y float64
			want uint32
		}{
			{2, 100, xdg.ToplevelResizeEdgeLeft},
			{298, 100, xdg.ToplevelResizeEdgeRight},
			{150, 2, xdg.ToplevelResizeEdgeTop},
			{150, 198, xdg.ToplevelResizeEdgeBottom},
			{2, 2, xdg.ToplevelResizeEdgeTopLeft},
			{298, 2, xdg.ToplevelResizeEdgeTopRight},
			{2, 198, xdg.ToplevelResizeEdgeBottomLeft},
			{298, 198, xdg.ToplevelResizeEdgeBottomRight},
		}
		for _, c := range cases {
			if e := resizeEdge(w, h, c.x, c.y); e != c.want {
				t.Errorf("resizeEdge(%.0f,%.0f) = %d, want %d", c.x, c.y, e, c.want)
			}
		}
	})

	t.Run("an axis too small for two handles matches no edge on it", func(t *testing.T) {
		// 10px wide: left and right handles would overlap; only the
		// vertical edges stay grabbable.
		if e := resizeEdge(10, 200, 1, 100); e != 0 {
			t.Errorf("narrow-width edge = %d, want 0", e)
		}
		if e := resizeEdge(10, 200, 1, 1); e != xdg.ToplevelResizeEdgeTop {
			t.Errorf("narrow-width corner = %d, want the top edge only", e)
		}
	})
}

func TestResizeCursorNames(t *testing.T) {
	cases := []struct {
		edges uint32
		want  string
	}{
		{xdg.ToplevelResizeEdgeTop, "top_side"},
		{xdg.ToplevelResizeEdgeBottom, "bottom_side"},
		{xdg.ToplevelResizeEdgeLeft, "left_side"},
		{xdg.ToplevelResizeEdgeRight, "right_side"},
		{xdg.ToplevelResizeEdgeTopLeft, "top_left_corner"},
		{xdg.ToplevelResizeEdgeTopRight, "top_right_corner"},
		{xdg.ToplevelResizeEdgeBottomLeft, "bottom_left_corner"},
		{xdg.ToplevelResizeEdgeBottomRight, "bottom_right_corner"},
		{0, ""},
		{xdg.ToplevelResizeEdgeLeft | xdg.ToplevelResizeEdgeRight, ""},
	}
	for _, c := range cases {
		if got := resizeCursor(c.edges); got != c.want {
			t.Errorf("resizeCursor(%d) = %q, want %q", c.edges, got, c.want)
		}
	}
}

// resizeGrab is one recorded xdg_toplevel.resize grab.
type resizeGrab struct {
	edges  uint32
	serial uint32
}

// resizeProbe builds a surface input whose edge probe answers for a
// fixed window size, recording every resize grab.
func resizeProbe(t *testing.T, root widget.Widget, w, h int) (*surfaceInput, *[]resizeGrab) {
	t.Helper()
	grabs := &[]resizeGrab{}
	in := &surfaceInput{
		router:  &widget.Router{Root: root},
		request: func() {},
		resizeAt: func(x, y float64) uint32 {
			return resizeEdge(w, h, x, y)
		},
		startResize: func(edges uint32, serial uint32) {
			*grabs = append(*grabs, resizeGrab{edges, serial})
		},
	}
	return in, grabs
}

func TestEdgePressGrabsResizeInsteadOfWidgetPress(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	clicks := 0
	btn := widget.NewButton(widget.NewLabel(face, 10, "fill", render.RGB(255, 255, 255)), 0, 0)
	btn.OnClick = func() { clicks++ }
	root := widget.NewBox(widget.Row, 0, 0).Append(btn, true)
	root.Measure(widget.Constraints{Max: widget.Size{W: 300, H: 200}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})

	t.Run("a corner press starts the resize with the press serial", func(t *testing.T) {
		in, grabs := resizeProbe(t, root, 300, 200)
		in.HandlePointerMotion(298, 198) // bottom-right handle, over the button
		in.HandlePointerButton(widget.BTNLeft, 1, 4242)
		if len(*grabs) != 1 {
			t.Fatalf("resize grab sent %d times, want once", len(*grabs))
		}
		g := (*grabs)[0]
		if g.edges != xdg.ToplevelResizeEdgeBottomRight {
			t.Errorf("grab edges = %d, want bottom|right (%d)",
				g.edges, xdg.ToplevelResizeEdgeBottomRight)
		}
		if g.serial != 4242 {
			t.Errorf("grab serial = %d, want the press serial 4242", g.serial)
		}
		// The grab replaces the widget press entirely: no click on
		// release.
		in.HandlePointerButton(widget.BTNLeft, 0, 4243)
		if clicks != 0 {
			t.Errorf("edge press clicked the button %d times", clicks)
		}
	})

	t.Run("an interior press stays a widget press", func(t *testing.T) {
		in, grabs := resizeProbe(t, root, 300, 200)
		bb := btn.Bounds()
		in.HandlePointerMotion(float64(bb.X+bb.W/2), float64(bb.Y+bb.H/2))
		in.HandlePointerButton(widget.BTNLeft, 1, 1)
		in.HandlePointerButton(widget.BTNLeft, 0, 2)
		if len(*grabs) != 0 {
			t.Errorf("interior press started %d resizes", len(*grabs))
		}
		if clicks != 1 {
			t.Errorf("interior click count = %d, want 1", clicks)
		}
	})
}

func TestEdgeHoverShowsResizeCursor(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	root.Measure(widget.Constraints{Max: widget.Size{W: 300, H: 200}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})
	in, _ := resizeProbe(t, root, 300, 200)

	in.HandlePointerMotion(298, 198)
	if in.lastCursor != "bottom_right_corner" {
		t.Errorf("corner hover cursor = %q, want bottom_right_corner", in.lastCursor)
	}
	in.HandlePointerMotion(150, 100)
	if in.lastCursor != "" {
		t.Errorf("interior hover cursor = %q, want the widget default", in.lastCursor)
	}
}

// TestConfigureRelayoutAtSeveralSizes pins the reactive relayout: after
// a configure changes the size, the very first frame measures, arranges,
// paints, and damages at the NEW size - no stale frame at the old size,
// and nothing else dirty is needed to get there.
func TestConfigureRelayoutAtSeveralSizes(t *testing.T) {
	progress := widget.NewProgressBar(0)
	root := widget.NewBox(widget.Column, 8, 8)
	root.Append(progress, true)
	h := newPaintHarness(root, 320, 200)
	host := h.wnd.host.(*fakeHost)

	_, damage := h.frame()
	if got := render.UnionAll(damage); got != (render.Rect{W: 320, H: 200}) {
		t.Fatalf("first frame damage %+v, want the full 320x200", got)
	}
	if got := progress.Bounds(); got != (render.Rect{X: 8, Y: 8, W: 320 - 16, H: 200 - 16}) {
		t.Fatalf("initial bar bounds %+v, want the 320x200 arrangement", got)
	}

	for _, size := range [][2]int{{640, 400}, {240, 160}, {1024, 300}} {
		w, hgt := size[0], size[1]
		host.resizeTo(w, hgt)
		if !h.wnd.syncSize() {
			t.Fatalf("%dx%d: configure not picked up", w, hgt)
		}
		if !h.wnd.dirty {
			t.Fatalf("%dx%d: configure did not schedule a repaint", w, hgt)
		}
		commits := h.surf.commits
		painted, damage := h.frame()

		if got := render.UnionAll(damage); got != (render.Rect{W: w, H: hgt}) {
			t.Errorf("%dx%d: first frame damage %+v, want the full new size", w, hgt, got)
		}
		last := h.bufs[len(h.bufs)-1]
		if last.Width != w || last.Height != hgt {
			t.Errorf("%dx%d: buffer = %dx%d, want the new size", w, hgt, last.Width, last.Height)
		}
		if painted < w*hgt*9/10 {
			t.Errorf("%dx%d: first frame painted %d px, want a full repaint", w, hgt, painted)
		}
		want := render.Rect{X: 8, Y: 8, W: w - 16, H: hgt - 16}
		if got := progress.Bounds(); got != want {
			t.Errorf("%dx%d: bar bounds %+v, want the re-arranged %+v", w, hgt, got, want)
		}
		if h.surf.commits != commits+1 {
			t.Errorf("%dx%d: %d commits for one configure, want exactly one frame",
				w, hgt, h.surf.commits-commits)
		}
		// The change was consumed: a no-op sync must not fire again.
		if h.wnd.syncSize() {
			t.Errorf("%dx%d: syncSize fired twice for one configure", w, hgt)
		}
	}
}

// TestSyncSizeReactsWithoutWidgetDamage is the loop-level pin: a
// configure arriving while nothing else is dirty still resizes the pool
// and schedules the repaint - before the fix the stale frame persisted
// until some widget happened to invalidate.
func TestSyncSizeReactsWithoutWidgetDamage(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 320, 200)
	h.frame()
	h.wnd.dirty = false // exactly what the loop looks like after an idle frame

	h.wnd.host.(*fakeHost).resizeTo(500, 300)
	if h.wnd.dirty {
		t.Fatal("the host size changed but nothing noticed; the loop polls syncSize")
	}
	if !h.wnd.syncSize() {
		t.Fatal("syncSize missed the configure")
	}
	if !h.wnd.dirty {
		t.Error("syncSize must schedule the repaint")
	}
	painted, damage := h.frame()
	if got := render.UnionAll(damage); got != (render.Rect{W: 500, H: 300}) {
		t.Errorf("damage %+v, want the full 500x300 repaint", got)
	}
	if painted < 500*300*9/10 {
		t.Errorf("painted %d px, want a full 500x300 repaint", painted)
	}
	if h.wnd.syncSize() {
		t.Error("syncSize fired again with no new configure")
	}
}

// TestLayoutClampsToSizeLimits pins the client-side min/max fallback:
// sizes a compositor configures outside the limits are clamped before
// layout, buffers, and damage see them.
func TestLayoutClampsToSizeLimits(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 300, 200)
	wnd := h.wnd
	wnd.limits = func() (minW, minH, maxW, maxH int) { return 200, 150, 800, 600 }

	t.Run("below the minimum lays out at the minimum", func(t *testing.T) {
		wnd.host.(*fakeHost).resizeTo(100, 80)
		wnd.syncSize()
		painted, damage := h.frame()
		last := h.bufs[len(h.bufs)-1]
		if last.Width != 200 || last.Height != 150 {
			t.Errorf("buffer = %dx%d, want the 200x150 minimum", last.Width, last.Height)
		}
		if got := render.UnionAll(damage); got != (render.Rect{W: 200, H: 150}) {
			t.Errorf("damage %+v, want the clamped 200x150", got)
		}
		if painted < 200*150*9/10 {
			t.Errorf("painted %d px, want a full 200x150 repaint", painted)
		}
	})

	t.Run("above the maximum lays out at the maximum", func(t *testing.T) {
		wnd.host.(*fakeHost).resizeTo(1200, 900)
		wnd.syncSize()
		_, damage := h.frame()
		last := h.bufs[len(h.bufs)-1]
		if last.Width != 800 || last.Height != 600 {
			t.Errorf("buffer = %dx%d, want the 800x600 maximum", last.Width, last.Height)
		}
		if got := render.UnionAll(damage); got != (render.Rect{W: 800, H: 600}) {
			t.Errorf("damage %+v, want the clamped 800x600", got)
		}
	})

	t.Run("inside the limits the configure wins", func(t *testing.T) {
		wnd.host.(*fakeHost).resizeTo(500, 400)
		wnd.syncSize()
		_, damage := h.frame()
		if got := render.UnionAll(damage); got != (render.Rect{W: 500, H: 400}) {
			t.Errorf("damage %+v, want the configured 500x400", got)
		}
	})
}

// TestResizeEdgeAtGatesServerDecorations pins the delegation rule: a
// server-decorated window reports no client edges - the compositor owns
// the handles then.
func TestResizeEdgeAtGatesServerDecorations(t *testing.T) {
	h := newPaintHarness(widget.NewBox(widget.Row, 0, 0), 300, 200)
	decorated := false
	h.wnd.decorated = func() bool { return decorated }

	if e := h.wnd.resizeEdgeAt(2, 2); e == 0 {
		t.Error("client-decorated window lost its edge handles")
	}
	decorated = true
	if e := h.wnd.resizeEdgeAt(2, 2); e != 0 {
		t.Errorf("server-decorated window still reports edges (%d)", e)
	}
}

// TestCloseRequestVetoRoutesProtocolClose pins the app-level routing:
// the veto installed on app.Window guards the protocol close event, so
// a vetoing app keeps its window (and the loop keeps it alive).
func TestCloseRequestVetoRoutesProtocolClose(t *testing.T) {
	t.Run("a vetoing callback survives the compositor close", func(t *testing.T) {
		w := &Window{win: &window.Window{}}
		w.win.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{}) // mapped
		calls := 0
		w.SetCloseRequest(func() bool {
			calls++
			return false
		})
		w.win.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if w.Closed() {
			t.Fatal("vetoed protocol close destroyed the window")
		}
		if calls != 1 {
			t.Errorf("veto called %d times, want once", calls)
		}
		if err := w.win.EnsureUsable(); err != nil {
			t.Errorf("vetoed window is no longer usable: %v", err)
		}
	})

	t.Run("an accepting callback closes through the same path", func(t *testing.T) {
		w := &Window{win: &window.Window{}}
		w.SetCloseRequest(func() bool { return true })
		w.win.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if !w.Closed() {
			t.Error("accepted protocol close did not close the window")
		}
	})

	t.Run("a window without a veto closes", func(t *testing.T) {
		w := &Window{win: &window.Window{}}
		w.win.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if !w.Closed() {
			t.Error("protocol close without a veto did not close the window")
		}
	})
}

func TestOnResizeHearsEachConfiguredSizeOnce(t *testing.T) {
	h := newPaintHarness(widget.NewBox(widget.Row, 0, 0), 320, 200)
	var sizes [][2]int
	h.wnd.cfg.onResize = func(w, hgt int) { sizes = append(sizes, [2]int{w, hgt}) }
	host := h.wnd.host.(*fakeHost)
	h.frame()
	h.wnd.dirty = true
	h.frame()
	host.resizeTo(640, 470)
	h.frame()
	if !slices.Equal(sizes, [][2]int{{320, 200}, {640, 470}}) {
		t.Errorf("OnResize heard %v, want the first size and each change once", sizes)
	}
}
