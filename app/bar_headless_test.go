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

// A bar says when it first reaches the screen, and starts and stops
// reserving its edge in place: a top layer that respects exclusive
// zones sits below the bar while it reserves, and at the very top once
// it stops (#390).
func TestHeadlessBarPresentsAndTogglesExclusive(t *testing.T) {
	wltest.Require(t)
	blue := render.RGB(0x20, 0x40, 0xd0)
	type shown struct {
		a *app.Application
		w *app.BarWindow
	}
	ready := make(chan shown, 1)
	presented := make(chan struct{}, 4)
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		bar := widget.NewBar(widget.BarTop)
		bar.SetInlineStyle("background-color: #d03020;")
		bar.Center().Append(widget.NewSpacer(24, 24), false)
		w, err := a.NewBar(app.BarConfig{
			Output: out, Bar: bar, Layer: app.LayerTop, Exclusive: true, Namespace: "lys-ui-bar-test",
			OnPresented: func() { presented <- struct{}{} },
		})
		if err != nil {
			return err
		}
		_, err = a.NewLayer(app.LayerConfig{
			Output: out, Layer: app.LayerTop, Anchor: app.AnchorTop | app.AnchorLeft | app.AnchorRight,
			Height: 10, Background: blue, Root: widget.NewBox(widget.Column, 0, 0), Namespace: "lys-ui-bar-neighbour",
		})
		ready <- shown{a, w}
		return err
	})
	s := <-ready
	<-presented
	c := inproc.Capture(t)
	at := func(what string, y int, want render.Color, is bool) {
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
			err = inproc.WantColor(img, 600, y, want)
			if !is {
				if err == nil {
					return errors.New("still there")
				}
				return nil
			}
			return err
		})
	}
	thick := make(chan int, 1)
	s.a.Invoke(func() { _, h := s.w.Size(); thick <- h })
	below := <-thick + 5
	at("the neighbour below the reserved bar", below, blue, true)
	set := func(on bool) {
		t.Helper()
		done := make(chan error, 1)
		s.a.Invoke(func() { done <- s.w.SetExclusive(on) })
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	set(false)
	at("the neighbour moved up once the bar stopped reserving", below, blue, false)
	set(true)
	at("the neighbour below the bar again", below, blue, true)
	select {
	case <-presented:
		t.Error("OnPresented ran again for the same surface")
	default:
	}
}
