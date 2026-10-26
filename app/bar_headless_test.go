package app_test

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

// A bar shows along its edge, as thick as its content, and moves to
// another edge whole.
func TestHeadlessBarMovesEdges(t *testing.T) {
	wltest.Require(t)
	ink := render.RGB(0xd0, 0x30, 0x20)
	type shown struct {
		a *app.Application
		w *app.BarWindow
	}
	moved := make(chan shown, 1)
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		bar := widget.NewBar(widget.BarTop)
		bar.SetInlineStyle("background-color: #d03020;")
		bar.Center().Append(widget.NewSpacer(24, 24), false)
		w, err := a.NewBar(app.BarConfig{Output: out, Bar: bar, Layer: app.LayerTop, Exclusive: true, Namespace: "lys-ui-bar-test"})
		moved <- shown{a, w}
		return err
	})
	s := <-moved
	c := inproc.Capture(t)
	at := func(what string, x, y int, want render.Color) {
		t.Helper()
		wltest.Eventually(t, what, func() error {
			f, err := c.CaptureOutput(c.Outputs()[0], capture.Options{})
			if err != nil {
				return err
			}
			img, err := f.Image()
			if err != nil {
				return err
			}
			return inproc.WantColor(img, x, y, want)
		})
	}
	at("the bar along the top", 600, 4, ink)
	done := make(chan error, 1)
	s.a.Invoke(func() { done <- s.w.SetEdge(widget.BarLeft) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	at("the bar along the left", 4, 600, ink)
	wltest.Eventually(t, "the top clear of the bar", func() error {
		f, err := c.CaptureOutput(c.Outputs()[0], capture.Options{})
		if err != nil {
			return err
		}
		img, err := f.Image()
		if err != nil {
			return err
		}
		if inproc.WantColor(img, 600, 4, ink) == nil {
			return errors.New("the top still shows the bar")
		}
		return nil
	})
}
