package app_test

import (
	"errors"
	"image"
	"testing"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

var fitInk = render.Color(0xff30a050)

// A layer anchored on one edge with automatic axes takes its content's
// natural size, centered along the free axis, and follows the content
// when it grows (gtk4-layer-shell's sizing). Before, the surface was
// refused for its half-anchored automatic axes.
func TestHeadlessLayerSizesToItsContent(t *testing.T) {
	wltest.Require(t)
	content := widget.NewDrawingArea(200, 60)
	content.OnDraw = func(cv *render.Canvas, r render.Rect) { cv.FillRect(r, fitInk) }
	var win *app.LayerWindow
	var application *app.Application
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		application = a
		w, err := a.NewLayer(app.LayerConfig{
			Output:    out,
			Layer:     app.LayerOverlay,
			Anchor:    app.AnchorBottom,
			Namespace: "lys-ui-layerfit-test",
			Root:      content,
		})
		win = w
		return err
	})
	c := inproc.Capture(t)
	screen := func(check func(img *image.RGBA) error) func() error {
		return func() error {
			f, err := c.CaptureOutput(c.Outputs()[0], capture.Options{})
			if err != nil {
				return err
			}
			img, err := f.Image()
			if err != nil {
				return err
			}
			return check(img)
		}
	}
	wltest.Eventually(t, "the 200x60 content at the bottom center", screen(func(img *image.RGBA) error {
		b := img.Bounds()
		cx := b.Dx() / 2
		return errors.Join(
			inproc.WantColor(img, cx, b.Dy()-30, fitInk),
			inproc.WantColor(img, cx+90, b.Dy()-5, fitInk),
			wantNot(img, cx+110, b.Dy()-30, fitInk),
			wantNot(img, cx, b.Dy()-70, fitInk),
		)
	}))
	application.Invoke(func() { content.SetContentSize(300, 120) })
	wltest.Eventually(t, "the surface grown to the 300x120 content", screen(func(img *image.RGBA) error {
		b := img.Bounds()
		cx := b.Dx() / 2
		return errors.Join(
			inproc.WantColor(img, cx+140, b.Dy()-110, fitInk),
			wantNot(img, cx+160, b.Dy()-30, fitInk),
		)
	}))
	if w, h := win.Size(); w != 300 || h != 120 {
		t.Errorf("surface %dx%d, want 300x120", w, h)
	}
}

func wantNot(img *image.RGBA, x, y int, c render.Color) error {
	if inproc.WantColor(img, x, y, c) == nil {
		return errors.New("content painted outside its natural size")
	}
	return nil
}

// A toplevel opened without a size maps on sway, whose first configure
// is 0x0: the window answers with its content's natural size instead of
// waiting forever with nothing to draw.
func TestHeadlessUnsizedWindowMaps(t *testing.T) {
	wltest.Require(t)
	ink := render.Color(0xff2060d0)
	content := widget.NewDrawingArea(240, 160)
	content.OnDraw = func(cv *render.Canvas, r render.Rect) { cv.FillRect(r, ink) }
	inproc.Show(t, func(a *app.Application, _ *app.Output) error {
		_, err := a.NewWindow(app.WindowConfig{Title: "lys-ui-unsized", Root: content})
		return err
	})
	c := inproc.Capture(t)
	wltest.Eventually(t, "the unsized window on screen", func() error {
		f, err := c.CaptureOutput(c.Outputs()[0], capture.Options{})
		if err != nil {
			return err
		}
		img, err := f.Image()
		if err != nil {
			return err
		}
		b := img.Bounds()
		return inproc.WantColor(img, b.Dx()/2, b.Dy()/2, ink)
	})
}
