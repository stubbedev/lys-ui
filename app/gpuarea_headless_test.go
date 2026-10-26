package app_test

import (
	"errors"
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/capture"
	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/udmabuf"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/headlesstest/inproc"
	"github.com/stubbedev/lys-ui/widget"
)

var (
	gpuBackground = render.Color(0xff2040d0)
	gpuFrame      = render.Color(0xff20c040)
	gpuCover      = render.Color(0xffd02030)
	gpuDmabuf     = render.Color(0xffe0a010)
)

func solidImage(c render.Color, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rgba := color.RGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 255}
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, rgba)
		}
	}
	return img
}

func showGPUArea(t *testing.T, onFrame func(area *widget.GPUArea, f widget.GPUFrame)) {
	t.Helper()
	inproc.Show(t, func(a *app.Application, out *app.Output) error {
		area := widget.NewGPUArea()
		area.OnFrame = func(f widget.GPUFrame) { onFrame(area, f) }
		cover := widget.NewDrawingArea(100, 100)
		cover.OnDraw = func(cv *render.Canvas, r render.Rect) { cv.FillRect(r, gpuCover) }
		root := widget.NewOverlay()
		root.Append(area)
		root.AppendAligned(cover, widget.AlignStart, widget.AlignStart)
		_, err := a.NewLayer(app.LayerConfig{
			Output:     out,
			Layer:      app.LayerOverlay,
			Anchor:     app.AnchorTop | app.AnchorBottom | app.AnchorLeft | app.AnchorRight,
			Namespace:  "lys-ui-gpuarea-test",
			Root:       root,
			Background: gpuBackground,
		})
		return err
	})
}

func wantScreen(t *testing.T, inside render.Color) {
	t.Helper()
	c := inproc.Capture(t)
	wltest.Eventually(t, "the GPUArea on screen", func() error {
		f, err := c.CaptureOutput(c.Outputs()[0], capture.Options{})
		if err != nil {
			return err
		}
		img, err := f.Image()
		if err != nil {
			return err
		}
		return errors.Join(inproc.WantColor(img, 300, 300, inside), inproc.WantColor(img, 50, 50, gpuCover))
	})
}

func TestHeadlessGPUAreaShowsImagesBelowTheWindow(t *testing.T) {
	wltest.Require(t)
	var frames atomic.Int32
	showGPUArea(t, func(area *widget.GPUArea, f widget.GPUFrame) {
		frames.Add(1)
		if f.Size.W <= 0 || f.Size.H <= 0 {
			t.Errorf("frame request for %v", f.Size)
		}
		if err := area.PresentImage(solidImage(gpuFrame, 64, 64)); err != nil {
			t.Error(err)
		}
	})
	wantScreen(t, gpuFrame)
	if n := frames.Load(); n < 2 {
		t.Errorf("%d frame requests, want the first and one per presented frame", n)
	}
}

func TestHeadlessGPUAreaPresentsDmabufs(t *testing.T) {
	wltest.Require(t)
	const w, h = 64, 64
	mem, err := udmabuf.New(w * h * 4)
	if err != nil {
		t.Skipf("no udmabuf: %v", err)
	}
	t.Cleanup(func() { _ = mem.Close() })
	for i := 0; i < w*h*4; i += 4 {
		mem.Mem[i], mem.Mem[i+1], mem.Mem[i+2], mem.Mem[i+3] = uint8(gpuDmabuf), uint8(gpuDmabuf>>8), uint8(gpuDmabuf>>16), 0xff
	}
	var buf widget.GPUBuffer
	imported := make(chan error, 1)
	showGPUArea(t, func(area *widget.GPUArea, _ widget.GPUFrame) {
		if buf == nil {
			b, err := area.Import(dmabuf.Buffer{
				Width: w, Height: h, Format: dmabuf.FormatXRGB8888, Modifier: dmabuf.ModifierLinear,
				Planes: []dmabuf.Plane{{FD: mem.FD, Stride: w * 4}},
			})
			imported <- err
			if err != nil {
				return
			}
			buf = b
		}
		if err := area.Present(buf); err != nil {
			t.Error(err)
		}
	})
	select {
	case err := <-imported:
		if errors.Is(err, app.ErrNoDmabuf) || errors.Is(err, app.ErrDmabufLayout) {
			t.Skipf("the compositor imports no linear XRGB8888 dmabufs: %v", err)
		}
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("the area never asked for a frame")
	}
	wantScreen(t, gpuDmabuf)
}
