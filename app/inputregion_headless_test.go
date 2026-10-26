package app_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/vinput"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

var (
	regionBelow = render.RGB(0x20, 0x40, 0xd0)
	regionPanel = render.RGB(0xd0, 0x30, 0x20)
)

// A floating panel on a full-screen overlay takes input only over
// itself: a click beside it reaches the surface beneath, one on it
// does not. Each click goes out once both surfaces are on screen, so a
// retried click never lands late.
func TestHeadlessFloatingPanelPassesClicksThrough(t *testing.T) {
	wltest.Require(t)
	var below, panel atomic.Int32
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		all := app.AnchorTop | app.AnchorBottom | app.AnchorLeft | app.AnchorRight
		under := widget.NewDrawingArea(10, 10)
		under.SetOnPointer(func(widget.PointerEvent) bool { below.Add(1); return true })
		if _, err := a.NewLayer(app.LayerConfig{Output: out, Layer: app.LayerBottom, Anchor: all, Namespace: "lys-ui-region-below", Root: under, Background: regionBelow}); err != nil {
			return err
		}
		body := widget.NewDrawingArea(300, 200)
		body.OnDraw = func(cv *render.Canvas, r render.Rect) { cv.FillRect(r, regionPanel) }
		body.SetOnPointer(func(widget.PointerEvent) bool { panel.Add(1); return true })
		f := widget.NewFloatingPanel(nil, body)
		_, err := a.NewLayer(app.LayerConfig{Output: out, Layer: app.LayerOverlay, Anchor: all, Namespace: "lys-ui-region-panel", Root: f})
		return err
	})
	d, err := vinput.Dial("", vinput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	w, h := d.OutputSize()
	c := inproc.Capture(t)
	wltest.Eventually(t, "the panel over the layer beneath on screen", func() error {
		f, err := c.CaptureOutput(c.Outputs()[0], capture.Options{})
		if err != nil {
			return err
		}
		img, err := f.Image()
		if err != nil {
			return err
		}
		return errors.Join(inproc.WantColor(img, 10, 10, regionBelow), inproc.WantColor(img, int(w/2), int(h/2), regionPanel))
	})
	click := func(x, y uint32) {
		t.Helper()
		for _, step := range []error{d.MotionAbsolute(x, y, uint32(w), uint32(h)), d.Button(0x110, true), d.Button(0x110, false), d.Roundtrip()} {
			if step != nil {
				t.Fatal(step)
			}
		}
	}
	reach := func(what string, n *atomic.Int32, want int32) {
		t.Helper()
		wltest.Eventually(t, what, func() error {
			if got := n.Load(); got != want {
				return fmt.Errorf("%d presses, want %d", got, want)
			}
			return nil
		})
	}
	click(10, 10)
	reach("a click beside the panel reaching the layer beneath", &below, 1)
	click(uint32(w/2), uint32(h/2))
	reach("a click on the panel staying on it", &panel, 1)
	time.Sleep(100 * time.Millisecond)
	if n := below.Load(); n != 1 {
		t.Errorf("a click on the panel also reached the layer beneath (%d presses there)", n)
	}
}
