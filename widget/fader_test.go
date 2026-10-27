// Fader and the embeddable Opacity: subtree pixel modulation goldens,
// nested composition, the zero-alpha paint skip, invalidation damage,
// and the animation-clock tween contract the enter/exit work consumes.
package widget

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// pxColorAt reads one ARGB8888 pixel from a canvas buffer.
func pxColorAt(data []byte, stride, x, y int) render.Color {
	o := y*stride + x*4
	return render.Color(binary.LittleEndian.Uint32(data[o : o+4]))
}

// faderCanvas returns a 10x10 canvas over a fresh black buffer.
func faderCanvas() (*render.Canvas, []byte) {
	data := make([]byte, render.Stride(10)*10)
	return render.New(data, render.Stride(10), 10, 10), data
}

// paintProbe fills its bounds and counts its paints.
type paintProbe struct {
	node
	col    render.Color
	paints int
}

func newPaintProbe(col render.Color) *paintProbe {
	return &paintProbe{col: col}
}

func (p *paintProbe) Measure(axis Axis, forSize int) Sizes {
	return FixedSizes(Size{W: 10, H: 10}.Along(axis))
}

func (p *paintProbe) Arrange(r render.Rect) { p.node.Arrange(r) }

func (p *paintProbe) Paint(cv *render.Canvas) {
	p.paints++
	cv.FillRect(p.bounds, p.col)
}

func (p *paintProbe) HitTest(at Point) Widget { return p.HitLeaf(p, at) }

// arrangeWidget measures and arranges w at rect, the way a frame does.
func arrangeWidget(w Widget, rect render.Rect) {
	SizeWithin(w, Size{W: 100, H: 100})
	w.Arrange(rect)
}

func TestFaderModulatesSubtree(t *testing.T) {
	t.Run("half opacity lands on the hand-derived pixel", func(t *testing.T) {
		cv, data := faderCanvas()
		probe := newPaintProbe(render.RGB(255, 0, 0))
		f := NewFader(probe)
		f.SetOpacity(0.5)
		arrangeWidget(f, render.Rect{X: 0, Y: 0, W: 10, H: 10})
		f.Paint(cv)

		// scale 128: red (255,0,0,255) premultiplies to a 128, r 128.
		if got := pxColorAt(data, 10, 4, 4); got != render.Color(0x80800000) {
			t.Errorf("pixel = %#08x, want %#08x", got, render.Color(0x80800000))
		}
		if probe.paints != 1 {
			t.Errorf("child painted %d times, want 1", probe.paints)
		}
	})

	t.Run("nested faders multiply to the single fader", func(t *testing.T) {
		paint := func() (render.Color, *paintProbe) {
			cv, data := faderCanvas()
			probe := newPaintProbe(render.RGB(255, 0, 0))
			f := NewFader(NewFader(probe))
			f.SetOpacity(0.5)
			f.Child().(*Fader).SetOpacity(0.5)
			arrangeWidget(f, render.Rect{X: 0, Y: 0, W: 10, H: 10})
			f.Paint(cv)
			return pxColorAt(data, 10, 4, 4), probe
		}
		got, probe := paint()
		if want := render.Color(0x40400000); got != want {
			t.Errorf("nested pixel = %#08x, want the 0.25 golden %#08x", got, want)
		}
		if probe.paints != 1 {
			t.Errorf("inner child painted %d times, want 1", probe.paints)
		}

		cv, data := faderCanvas()
		one := NewFader(newPaintProbe(render.RGB(255, 0, 0)))
		one.SetOpacity(0.25)
		arrangeWidget(one, render.Rect{X: 0, Y: 0, W: 10, H: 10})
		one.Paint(cv)
		if single := pxColorAt(data, 10, 4, 4); single != got {
			t.Errorf("single fader pixel = %#08x, want nested result %#08x", single, got)
		}
	})

	t.Run("layout passes through to the child", func(t *testing.T) {
		probe := newPaintProbe(render.RGB(1, 2, 3))
		f := NewFader(probe)
		rect := render.Rect{X: 3, Y: 4, W: 10, H: 10}
		if got := SizeWithin(f, Size{W: 100, H: 100}); got != (Size{W: 10, H: 10}) {
			t.Errorf("Measure = %v, want the child's natural size", got)
		}
		f.Arrange(rect)
		if probe.Bounds() != rect || f.Bounds() != rect {
			t.Errorf("child bounds %v / fader bounds %v, want both %v", probe.Bounds(), f.Bounds(), rect)
		}
		if hit := f.HitTest(Point{X: 5, Y: 5}); hit != Widget(probe) {
			t.Errorf("HitTest = %v, want the child", hit)
		}
	})
}

func TestFaderZeroAlphaSkipsPaint(t *testing.T) {
	cv, _ := faderCanvas()
	probe := newPaintProbe(render.RGB(255, 0, 0))
	f := NewFader(probe)
	arrangeWidget(f, render.Rect{X: 0, Y: 0, W: 10, H: 10})

	f.SetOpacity(0)
	f.Paint(cv)
	if probe.paints != 0 {
		t.Errorf("child painted %d times at zero alpha, want 0", probe.paints)
	}
	if got := cv.Touched(); got != 0 {
		t.Errorf("zero-alpha frame touched %d pixels, want 0", got)
	}

	// Full opacity paints directly; in between, exactly one child paint.
	f.SetOpacity(1)
	f.Paint(cv)
	if probe.paints != 1 || cv.Touched() != 100 {
		t.Errorf("opaque paint: %d paints, %d touched pixels", probe.paints, cv.Touched())
	}
	cv.ResetTouched()
	f.SetOpacity(0.5)
	f.Paint(cv)
	if probe.paints != 2 {
		t.Errorf("partial paint: %d child paints, want 2", probe.paints)
	}
}

func TestSetOpacityInvalidates(t *testing.T) {
	probe := newPaintProbe(render.RGB(1, 2, 3))
	f := NewFader(probe)
	root := NewBox(Column, 0, 0)
	root.Append(f, false)
	arrangeWidget(root, render.Rect{X: 0, Y: 0, W: 40, H: 20})
	warmDamage(root)

	f.SetOpacity(0.3)
	rects, any := CollectDamage(root)
	if !any {
		t.Fatal("SetOpacity produced no damage")
	}
	if got := render.UnionAll(rects); got != f.Bounds() {
		t.Errorf("damage union %v, want the fader bounds %v", got, f.Bounds())
	}
	if _, any := CollectDamage(root); any {
		t.Error("damage flags were not drained")
	}

	// Clamping keeps the factor in [0, 1].
	f.SetOpacity(5)
	if got := f.Alpha(); got != 1 {
		t.Errorf("Alpha after SetOpacity(5) = %v, want 1", got)
	}
	f.SetOpacity(-2)
	if got := f.Alpha(); got != 0 {
		t.Errorf("Alpha after SetOpacity(-2) = %v, want 0", got)
	}
}

// TestFaderTweenDrivesOpacity pins the contract the enter/exit work
// consumes: a tween callback is just SetOpacity, and every landing
// invalidates the subtree for the next frame.
func TestFaderTweenDrivesOpacity(t *testing.T) {
	c := pinAnimClock(t)
	probe := newPaintProbe(render.RGB(1, 2, 3))
	f := NewFader(probe)
	root := NewBox(Column, 0, 0)
	root.Append(f, false)
	arrangeWidget(root, render.Rect{X: 0, Y: 0, W: 40, H: 20})
	warmDamage(root)

	anim.Start(100*time.Millisecond, f.SetOpacity)

	if !c.step() {
		t.Fatal("tween scheduled no frame")
	}
	if got := f.Alpha(); got <= 0 || got >= 1 {
		t.Errorf("mid-flight opacity = %v, want strictly between 0 and 1", got)
	}
	if _, any := CollectDamage(root); !any {
		t.Error("a tween-driven SetOpacity produced no damage")
	}
	c.drive()
	if got := f.Alpha(); got != 1 {
		t.Errorf("final opacity = %v, want 1", got)
	}
	if probe.paints != 0 {
		t.Errorf("tween painted %d times, want 0 (paint happens in Paint)", probe.paints)
	}
}

func TestNewFaderStartsOpaque(t *testing.T) {
	cv, data := faderCanvas()
	probe := newPaintProbe(render.RGB(255, 0, 0))
	f := NewFader(probe)
	if f.Alpha() != 1 {
		t.Fatalf("fresh fader alpha = %v, want 1", f.Alpha())
	}
	arrangeWidget(f, render.Rect{X: 0, Y: 0, W: 10, H: 10})
	f.Paint(cv)
	if probe.paints != 1 {
		t.Fatalf("fresh fader painted its child %d times, want 1", probe.paints)
	}
	if got := pxColorAt(data, 10, 4, 4); got != render.RGB(255, 0, 0) {
		t.Errorf("fresh fader pixel = %#08x, want the child's unmodulated color", got)
	}
}
