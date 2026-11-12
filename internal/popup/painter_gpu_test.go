package popup

import (
	"errors"
	"image/color"
	"testing"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// gpuFrames is a Renderer over a real device, or one whose frames fail.
type gpuFrames struct {
	dev        *gpu.Device
	lost       bool
	refused    bool
	incomplete bool
	failed     bool
}

func (g *gpuFrames) NewBuffer(*wlsession.Session, int, int, int) (*buffer.Buffer, error) {
	return nil, errors.New("not used")
}

func (g *gpuFrames) Frame(b *buffer.Buffer, num int, _ []gpu.Carry, paint func(cv *render.Canvas)) (buffer.FrameOutcome, error) {
	if g.refused {
		return buffer.FrameRefused, nil
	}
	if g.incomplete {
		return buffer.FrameIncomplete, nil
	}
	if g.lost {
		g.failed = true
		return buffer.FrameLost, nil
	}
	rec := render.NewRecorder(b.Width, b.Height, num, scale.Denom)
	paint(rec)
	return buffer.FrameDrawn, g.dev.Render(rec.Recording(), b.Target)
}

func (g *gpuFrames) Failed() bool { return g.failed }

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
		if outcome, err := pc.paint(&buffer.Buffer{Width: w, Height: h, Target: tg}, nil, []render.Rect{{W: w, H: h}}); outcome != buffer.FrameDrawn || err != nil {
			t.Fatalf("reveal %v: GPU frame outcome %v, %v", reveal, outcome, err)
		}
		cpu := &buffer.Buffer{Width: w, Height: h, Stride: render.Stride(w), Data: make([]byte, render.Stride(w)*h)}
		if outcome, err := (&Painter{frac120: scale.Denom, visual: v.morph}).paint(cpu, nil, []render.Rect{{W: w, H: h}}); outcome != buffer.FrameDrawn || err != nil {
			t.Fatalf("reveal %v: CPU frame outcome %v, %v", reveal, outcome, err)
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
	if outcome, err := pc.paint(b, nil, []render.Rect{{W: b.Width, H: b.Height}}); outcome != buffer.FrameLost || err != nil {
		t.Fatalf("failed frame reported %v, %v; want lost and no error", outcome, err)
	}
	if pc.gpu != nil {
		t.Error("the popup still draws on the GPU after a failed frame")
	}
	if b.Busy() {
		t.Error("the failed frame's buffer was not given back")
	}
}

func TestPopupFrameTheGPURefusedStaysOwedOnTheGPU(t *testing.T) {
	v, _ := newTweenView(t, 1, GravityBottom)
	frames := &gpuFrames{refused: true}
	pc := &Painter{gpu: frames, frac120: scale.Denom, visual: v.morph}
	bounds := render.Rect{W: 64, H: 24}
	pc.damage.Full(bounds)
	pool := buffer.New(func() (*buffer.Buffer, error) {
		return &buffer.Buffer{Width: 64, Height: 24, Target: &gpu.Target{}, Stale: buffer.RegionOf(bounds)}, nil
	}, 2)
	b, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	_, _, regions := pc.damage.Plan(b, scale.Denom, false)
	outcome, err := pc.paint(b, nil, regions)
	if outcome != buffer.FrameRefused || err != nil {
		t.Fatalf("refused frame reported %v, %v", outcome, err)
	}
	if pc.gpu == nil {
		t.Error("a busy GPU turned the popup to the CPU")
	}
	if b.Busy() {
		t.Error("the refused frame's buffer was not given back")
	}
	if pc.damage.Empty() {
		t.Error("the refused frame's damage was lost")
	}
	if b.Stale.Empty() {
		t.Error("the refused frame's stale rects were lost")
	}
}

func TestAnIncompletePopupFrameStaysOwedAfterItsCommit(t *testing.T) {
	v, _ := newTweenView(t, 1, GravityBottom)
	pc := &Painter{clock: new(anim.Clock), gpu: &gpuFrames{incomplete: true}, frac120: scale.Denom, visual: v.morph}
	bounds := render.Rect{W: 64, H: 24}
	pc.damage.Full(bounds)
	pc.pool = buffer.New(func() (*buffer.Buffer, error) {
		return &buffer.Buffer{Width: 64, Height: 24, Target: &gpu.Target{}, Stale: buffer.RegionOf(bounds)}, nil
	}, 2)
	b, err := pc.pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	_, changed, regions := pc.damage.Plan(b, scale.Denom, false)
	outcome, err := pc.paint(b, nil, regions)
	if outcome != buffer.FrameIncomplete || err != nil {
		t.Fatalf("incomplete frame reported %v, %v", outcome, err)
	}
	if !b.Busy() {
		t.Error("the incomplete frame's buffer went back before its commit")
	}
	if err := pc.presented(b, changed, outcome); err != nil {
		t.Fatal(err)
	}
	if pc.damage.Empty() || !pc.Owes() {
		t.Error("the incomplete frame's damage was not kept for the next frame")
	}
}
