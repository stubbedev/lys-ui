package app_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/vinput"
	"github.com/stubbedev/lys-ui/widget"
)

// A floating panel on a full-screen overlay takes input only over
// itself: a click beside it reaches the surface beneath, one on it
// does not.
func TestHeadlessFloatingPanelPassesClicksThrough(t *testing.T) {
	inproc.Require(t)
	var below, panel atomic.Int32
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		all := app.AnchorTop | app.AnchorBottom | app.AnchorLeft | app.AnchorRight
		under := widget.NewDrawingArea(10, 10)
		under.SetOnPointer(func(widget.PointerEvent) bool { below.Add(1); return true })
		if _, err := a.NewLayer(app.LayerConfig{Output: out, Layer: app.LayerBottom, Anchor: all, Namespace: "lys-ui-region-below", Root: under, Background: render.RGB(0x20, 0x40, 0xd0)}); err != nil {
			return err
		}
		body := widget.NewDrawingArea(300, 200)
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
	click := func(x, y uint32) {
		t.Helper()
		for _, step := range []error{d.MotionAbsolute(x, y, uint32(w), uint32(h)), d.Button(0x110, true), d.Button(0x110, false), d.Roundtrip()} {
			if step != nil {
				t.Fatal(step)
			}
		}
	}
	inproc.Eventually(t, "a click beside the panel reaching the layer beneath", func() error {
		click(10, 10)
		if below.Load() == 0 {
			return fmt.Errorf("the layer beneath saw no press (the panel saw %d)", panel.Load())
		}
		return nil
	})
	// The retried clicks land on the app's goroutine after the check
	// that ended the retries: wait for the count to settle.
	var seen int32
	inproc.Eventually(t, "the clicks beneath settling", func() error {
		seen = below.Load()
		time.Sleep(100 * time.Millisecond)
		if n := below.Load(); n != seen {
			return fmt.Errorf("still counting (%d→%d)", seen, n)
		}
		return nil
	})
	inproc.Eventually(t, "a click on the panel staying on it", func() error {
		click(uint32(w/2), uint32(h/2))
		if panel.Load() == 0 {
			return errors.New("the panel saw no press")
		}
		return nil
	})
	if below.Load() != seen {
		t.Errorf("a click on the panel also reached the layer beneath (%d→%d)", seen, below.Load())
	}
}
