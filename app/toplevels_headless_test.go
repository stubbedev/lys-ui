package app_test

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

// A window another client maps is listed on the application's own
// connection with its title, app id and output, and as fullscreen once
// it asks to be, a rectangle on one of the application's surfaces is
// accepted for it, and its close takes it off the list, each change heard
// through OnToplevelsChanged.
func TestHeadlessToplevelsListAnotherClientsWindow(t *testing.T) {
	wltest.Require(t)
	var (
		a      *app.Application
		output string
		panel  *app.LayerWindow
	)
	changes := make(chan struct{}, 64)
	inproc.Show(t, func(application *app.Application, out *app.Output) error {
		a, output = application, out.Name
		if !a.ToplevelsAvailable() {
			return errors.New("the compositor offers no foreign-toplevel manager")
		}
		a.OnToplevelsChanged(func() {
			select {
			case changes <- struct{}{}:
			default:
			}
		})
		var err error
		panel, err = a.NewLayer(app.LayerConfig{Output: out, Layer: app.LayerTop, Anchor: app.AnchorBottom, Namespace: "lys-ui-toplevels-panel", Root: widget.NewDrawingArea(40, 40)})
		return err
	})
	var b *app.Application
	other := make(chan *app.Window, 1)
	inproc.Show(t, func(client *app.Application, _ *app.Output) error {
		b = client
		w, err := b.NewWindow(app.WindowConfig{Title: "listed", AppID: "org.example.Listed", Root: widget.NewDrawingArea(64, 64)})
		other <- w
		return err
	})
	window := <-other
	find := func() (app.Toplevel, bool) {
		found := make(chan []app.Toplevel, 1)
		a.Invoke(func() { found <- a.Toplevels() })
		for _, tl := range <-found {
			if tl.AppID() == "org.example.Listed" {
				return tl, true
			}
		}
		return app.Toplevel{}, false
	}
	wltest.Eventually(t, "the other client's window listed on its output", func() error {
		tl, ok := find()
		if !ok {
			return errors.New("not listed")
		}
		onOutput := make(chan bool, 1)
		a.Invoke(func() {
			onOutput <- tl.Title() == "listed" && tl.OnOutput(output) && !tl.OnOutput("nowhere") && !tl.Fullscreen()
		})
		if !<-onOutput {
			return errors.New("listed without its title or output")
		}
		return nil
	})
	if len(changes) == 0 {
		t.Fatal("OnToplevelsChanged heard nothing while the window appeared")
	}
	b.Invoke(window.Fullscreen)
	wltest.Eventually(t, "the other client's window listed fullscreen", func() error {
		tl, ok := find()
		if !ok {
			return errors.New("not listed")
		}
		full := make(chan bool, 1)
		a.Invoke(func() { full <- tl.Fullscreen() })
		if !<-full {
			return errors.New("listed, not fullscreen")
		}
		return nil
	})
	tl, _ := find()
	a.Invoke(func() { tl.SetRectangle(panel, render.Rect{X: 4, Y: 4, W: 32, H: 32}) })
	b.Invoke(window.Close)
	wltest.Eventually(t, "the closed window off the list", func() error {
		if _, ok := find(); ok {
			return errors.New("still listed")
		}
		return nil
	})
}
