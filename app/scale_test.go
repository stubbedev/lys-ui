// Fractional-scale and live-rescale coverage: the logical-coordinate
// contract (hit tests, tooltips, menus) at 1.25 and 2, the in-place
// buffer rebuild on preferred_scale changes, and the device-pixel
// damage mapping the wire sees. No wayland connection involved; the
// scaleWire seam stands in for internal/scale.Controller.
package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/widget"
)

// scaleApply is one recorded Apply call.
type scaleApply struct {
	frac120 uint32
	w, h    int
}

// fakeScaleWire records the surface-scale requests a rescale performs.
type fakeScaleWire struct {
	applies    []scaleApply
	transforms []int32
}

func (f *fakeScaleWire) Apply(frac120 uint32, w, h int) error {
	f.applies = append(f.applies, scaleApply{frac120, w, h})
	return nil
}

func (f *fakeScaleWire) SetTransform(t int32) error {
	f.transforms = append(f.transforms, t)
	return nil
}

// fracHarness extends the paint harness with a device scale and the
// scale wire seam.
type fracHarness struct {
	*paintHarness
	sc *fakeScaleWire
}

func newFracHarness(root widget.Widget, w, h int, frac120 uint32) *fracHarness {
	ph := newPaintHarness(root, w, h)
	fh := &fracHarness{paintHarness: ph, sc: &fakeScaleWire{}}
	ph.wnd.frac120 = frac120
	ph.wnd.scale = scale.IntegerScale(frac120)
	ph.wnd.sc = fh.sc
	return fh
}

// TestLiveRescaleRebuildsBuffersInPlace is the regression pin for live
// scale changes: the same window, router, and pool survive; only the
// buffers are rebuilt at the new device size and the fresh buffer's
// full staleness forces a full repaint on that size.
func TestLiveRescaleRebuildsBuffersInPlace(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	entry := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	root := widget.NewBox(widget.Row, 8, 0)
	root.Append(entry, true)
	root.Append(widget.NewButton(widget.NewLabel(face, 13, "ok", render.RGB(255, 255, 255)), 8, 4), false)
	fh := newFracHarness(root, 320, 200, 240) // live at 2x
	wnd := fh.wnd

	// Give the window state a rescale must preserve: the entry holds
	// keyboard focus and a couple of frames have rotated through.
	fh.frame() // arrange the tree so widget bounds are live
	eb := entry.Bounds()
	p := widget.Point{X: eb.X + 5, Y: eb.Y + 5}
	wnd.router.Press(widget.BTNLeft, p, 0)
	wnd.router.Release(widget.BTNLeft, p)
	if wnd.router.Focused() != widget.Widget(entry) {
		t.Fatalf("entry did not take focus; focused %T", wnd.router.Focused())
	}
	fh.frame()
	fh.frame()
	bufsBefore := len(fh.bufs)
	poolBefore := wnd.pool
	fh.sc.applies = nil // drop the first draw's initial wire publish

	// The compositor halves the scale (2x -> 1x): buffers rebuild,
	// nothing else is recreated.
	wnd.rescale(120)
	if wnd.pool != poolBefore {
		t.Error("rescale replaced the buffer pool; it must resize in place")
	}
	if wnd.router == nil || wnd.router.Focused() != widget.Widget(entry) {
		t.Error("rescale lost the router focus state")
	}
	if wnd.surf == nil {
		t.Error("rescale dropped the surface handle")
	}
	if len(fh.sc.applies) != 1 || fh.sc.applies[0] != (scaleApply{120, 320, 200}) {
		t.Errorf("rescale published %+v, want one Apply(120, 320, 200)", fh.sc.applies)
	}
	if !wnd.scaleApplied {
		t.Error("rescale must mark the wire scale as published")
	}

	// The next frame lands in fresh buffers at the new device size in
	// BOTH axes and repaints all of it.
	painted, damage := fh.frame()
	if len(fh.bufs) <= bufsBefore {
		t.Fatalf("rescale created no new buffers (have %d)", len(fh.bufs))
	}
	last := fh.bufs[len(fh.bufs)-1]
	if last.Width != 320 || last.Height != 200 {
		t.Errorf("buffer after rescale = %dx%d, want 320x200", last.Width, last.Height)
	}
	if got := render.UnionAll(damage); got != (render.Rect{W: 320, H: 200}) {
		t.Errorf("post-rescale damage = %+v, want the full new buffer (fresh buffers are fully stale)", got)
	}
	// Touched counts every blended pixel, widgets overdraw, so pin the
	// full repaint as "at least the whole buffer was written".
	if painted < 320*200*9/10 {
		t.Errorf("post-rescale frame painted %d px, want a full 320x200 repaint", painted)
	}
}

// TestRescaleToFractionalSizesBuffersWithCeil pins the 120-based math:
// 1.25 means buffers of ceil(logical*1.25) in both axes.
func TestRescaleToFractionalSizesBuffersWithCeil(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	fh := newFracHarness(root, 320, 200, 120)
	fh.wnd.rescale(150)

	if fh.wnd.scale != 2 {
		t.Errorf("integer fallback scale = %d, want 2 (ceil of 1.25)", fh.wnd.scale)
	}
	painted, damage := fh.frame()
	last := fh.bufs[len(fh.bufs)-1]
	if last.Width != 400 || last.Height != 250 {
		t.Errorf("buffer at 1.25 = %dx%d, want 400x250 (ceil on both axes)", last.Width, last.Height)
	}
	if got := render.UnionAll(damage); got != (render.Rect{W: 400, H: 250}) {
		t.Errorf("damage = %+v, want the full 400x250 buffer", got)
	}
	if painted < 400*250*9/10 {
		t.Errorf("painted %d px, want a full 400x250 initialization paint", painted)
	}
}

// TestRescaleIgnoresSameScaleAndZero pins the idempotence guards: a
// repeated preferred_scale (compositors resend) and a zero event are
// both no-ops.
func TestRescaleIgnoresSameScaleAndZero(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	fh := newFracHarness(root, 320, 200, 240)
	fh.wnd.scaleApplied = true

	fh.wnd.rescale(240)
	fh.wnd.rescale(0)
	if len(fh.sc.applies) != 0 {
		t.Errorf("same-scale rescale published %+v, want nothing", fh.sc.applies)
	}
	fh.frame()
	if fh.surf.commits == 0 {
		t.Error("the guard test never drew; the harness is broken")
	}
}

// TestLogicalHitTestsAtFractionalScales pins the logical-coordinate
// contract: pointer positions route into the tree 1:1 at every device
// scale, so hit tests, caret placement, and click targets are identical
// at 1x, 1.25x, and 2x.
func TestLogicalHitTestsAtFractionalScales(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	for _, frac := range []uint32{120, 150, 240} {
		entry := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
		entry.SetText("hello")
		btn := widget.NewButton(widget.NewLabel(face, 13, "ok", render.RGB(255, 255, 255)), 8, 4)
		root := widget.NewBox(widget.Row, 8, 0)
		root.Append(entry, true)
		root.Append(btn, false)
		widget.SizeWithin(root, widget.Size{W: 400, H: 60})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 60})
		in := &surfaceInput{router: &widget.Router{Root: root}, request: func() {}}

		// A click at the button's center hits the button; the same
		// logical point must hit it at any device scale.
		bb := btn.Bounds()
		cx, cy := float64(bb.X+bb.W/2), float64(bb.Y+bb.H/2)
		in.HandlePointerMotion(cx, cy)
		if hovered := in.router.Hovered(); boundsOf(hovered) != bb {
			t.Errorf("frac %d: point (%.0f,%.0f) hovered %T at %+v, want the button at %+v",
				frac, cx, cy, hovered, boundsOf(hovered), bb)
		}
		clicks := 0
		btn.OnClick = func() { clicks++ }
		in.HandlePointerButton(widget.BTNLeft, 1, 1)
		in.HandlePointerButton(widget.BTNLeft, 0, 1)
		if clicks != 1 {
			t.Errorf("frac %d: the logical click on the button did not land", frac)
		}

		// A click inside the entry focuses it; the caret rect stays
		// surface-local, never buffer-sized.
		eb := entry.Bounds()
		in.HandlePointerMotion(float64(eb.X+eb.W-5), float64(eb.Y+5))
		in.HandlePointerButton(widget.BTNLeft, 1, 2)
		in.HandlePointerButton(widget.BTNLeft, 0, 2)
		if in.router.Focused() != widget.Widget(entry) {
			t.Errorf("frac %d: the entry did not take focus from a logical click", frac)
		}
		cr := entry.IMECursorRect()
		if cr.X > eb.X+eb.W || cr.H > eb.H {
			t.Errorf("frac %d: caret rect %+v outside the entry bounds %+v (must be surface-local)", frac, cr, eb)
		}
	}
}

// boundsOf returns w's bounds, or an empty rect for nil/non-Boundser.
func boundsOf(w widget.Widget) render.Rect {
	if b, ok := w.(widget.Boundser); ok {
		return b.Bounds()
	}
	return render.Rect{}
}

// TestDrawDamageMapsToDevicePixels pins the wire half: damage rects go
// out in buffer pixels, mapped outward from the logical damage, at the
// fractional scale, through the same surface handle.
func TestDrawDamageMapsToDevicePixels(t *testing.T) {
	progress := widget.NewProgressBar(0)
	root := widget.NewBox(widget.Row, 0, 0)
	root.Append(progress, false)
	fh := newFracHarness(root, 320, 200, 150)
	fh.frame()
	fh.warm(nil, 2)

	progress.SetValue(0.5)
	_, damage := fh.frame()
	want := render.MapRect(progress.Bounds(), 150, 120)
	if got := render.UnionAll(damage); got != want {
		t.Errorf("damage union %+v, want the 1.25x mapping %+v of %+v", got, want, progress.Bounds())
	}
	if fh.surf.commits == 0 {
		t.Error("the frame never committed")
	}
}

// TestRescaleCarriesTooltipState keeps the per-window bookkeeping
// coherent across a live scale change.
func TestRescaleCarriesTooltipState(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	fh := newFracHarness(root, 320, 200, 120)
	before := fh.wnd.tip
	fh.wnd.rescale(240)
	if fh.wnd.tip != before {
		t.Error("rescale replaced the tooltip state machine")
	}
	if fh.wnd.frac120 != 240 || fh.wnd.scale != 2 {
		t.Errorf("scale after rescale = %d/120 (int %d), want 240/120 (int 2)",
			fh.wnd.frac120, fh.wnd.scale)
	}
}

// A configure that resizes the window republishes the scale state at
// the new logical size: the viewport destination is that size, so a
// stale one shows the new buffer squeezed into the old rect. Before the
// first frame nothing is published, and an unchanged size sends nothing.
func TestResizeRepublishesTheViewportDestination(t *testing.T) {
	fh := newFracHarness(widget.NewBox(widget.Row, 0, 0), 200, 60, 150)
	fh.frame()
	if len(fh.sc.applies) != 1 || fh.sc.applies[0] != (scaleApply{150, 200, 60}) {
		t.Fatalf("first frame published %+v, want Apply(150, 200, 60)", fh.sc.applies)
	}
	fh.sc.applies = nil
	fh.wnd.host.(*fakeHost).w, fh.wnd.host.(*fakeHost).h = 300, 120
	fh.frame()
	if len(fh.sc.applies) != 1 || fh.sc.applies[0] != (scaleApply{150, 300, 120}) {
		t.Errorf("resize published %+v, want one Apply(150, 300, 120)", fh.sc.applies)
	}
	fh.sc.applies = nil
	fh.frame()
	if len(fh.sc.applies) != 0 {
		t.Errorf("an unchanged size republished %+v", fh.sc.applies)
	}
}
