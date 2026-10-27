package popup

import (
	"errors"
	"image/color"
	"testing"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// gpuFrames is a Renderer over a real device, or one whose frames fail.
type gpuFrames struct {
	dev    *gpu.Device
	lost   bool
	failed bool
}

func (g *gpuFrames) NewBuffer(*wlsession.Session, int, int, int) (*buffer.Buffer, error) {
	return nil, errors.New("not used")
}

func (g *gpuFrames) Frame(b *buffer.Buffer, num int, paint func(cv *render.Canvas)) (bool, error) {
	if g.lost {
		g.failed = true
		return false, nil
	}
	rec := render.NewRecorder(b.Width, b.Height, num, scale.Denom)
	paint(rec)
	return true, g.dev.Render(rec.Recording(), b.Target)
}

func (g *gpuFrames) Failed() bool { return g.failed }

func (g *gpuFrames) Copy(*buffer.Buffer, *buffer.Buffer, render.Rect) (bool, error) {
	return false, nil
}

func openGPU(t *testing.T) *gpu.Device {
	t.Helper()
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	dev, err := gpu.Open()
	if err != nil {
		if gputest.Enabled() {
			t.Fatal(err)
		}
		t.Skipf("no GPU: %v", err)
	}
	t.Cleanup(dev.Close)
	return dev
}

// A popup on the GPU renderer draws its frames there (#319), mid-reveal ones
// (the morph composite) included, with the pixels the CPU paints.
func TestPopupFramesDrawOnTheGPU(t *testing.T) {
	dev := openGPU(t)
	const w, h = 64, 24
	for _, reveal := range []float64{0.5, 1} {
		v, _ := newTweenView(t, reveal, GravityBottom)
		tg, err := dev.NewOffscreen(w, h)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tg.Close)
		pc := &Painter{gpu: &gpuFrames{dev: dev}, frac120: scale.Denom, visual: v.morph}
		if ok, err := pc.paint(&buffer.Buffer{Width: w, Height: h, Target: tg}, render.Rect{W: w, H: h}); !ok || err != nil {
			t.Fatalf("reveal %v: GPU frame drawn %v, %v", reveal, ok, err)
		}
		cpu := &buffer.Buffer{Width: w, Height: h, Stride: render.Stride(w), Data: make([]byte, render.Stride(w)*h)}
		if ok, err := (&Painter{frac120: scale.Denom, visual: v.morph}).paint(cpu, render.Rect{W: w, H: h}); !ok || err != nil {
			t.Fatalf("reveal %v: CPU frame drawn %v, %v", reveal, ok, err)
		}
		got, err := tg.ReadPixels()
		if err != nil {
			t.Fatal(err)
		}
		for y := range h {
			for x := range w {
				c := render.ColorFromBytes(cpu.Data[y*cpu.Stride+x*4:])
				if want := (color.RGBA{R: c.R(), G: c.G(), B: c.B(), A: c.A()}); got.RGBAAt(x, y) != want {
					t.Fatalf("reveal %v at (%d, %d): GPU %v, CPU %v", reveal, x, y, got.RGBAAt(x, y), want)
				}
			}
		}
	}
}

// A GPU frame that fails gives its buffer back and turns the popup to
// the CPU for the next pass.
func TestPopupGPUFrameFailureFallsBackToCPU(t *testing.T) {
	dev := openGPU(t)
	v, _ := newTweenView(t, 1, GravityBottom)
	tg, err := dev.NewOffscreen(64, 24)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tg.Close)
	pc := &Painter{gpu: &gpuFrames{dev: dev, lost: true}, frac120: scale.Denom, visual: v.morph}
	pc.pool = buffer.New(pc.create, 2)
	b := &buffer.Buffer{Width: 64, Height: 24, Target: tg}
	if ok, err := pc.paint(b, render.Rect{W: b.Width, H: b.Height}); ok || err != nil {
		t.Fatalf("failed frame reported %v, %v; want not drawn and no error", ok, err)
	}
	if pc.gpu != nil {
		t.Error("the popup still draws on the GPU after a failed frame")
	}
	if b.Busy() {
		t.Error("the failed frame's buffer was not given back")
	}
}
