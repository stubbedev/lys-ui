package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// surfPx reads one pixel out of a test frame's buffer.
func surfPx(data []byte, w, x, y int) render.Color {
	o := y*render.Stride(w) + x*4
	return render.ColorFromBytes(data[o : o+4])
}

func TestDrawSurfacePixelsPersist(t *testing.T) {
	s := NewDrawSurface(60, 40)
	draws, resizes := 0, 0
	s.OnResize = func(cv *render.Canvas) { resizes++ }
	w, h := 60, 40
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	bg := render.RGB(0x20, 0x20, 0x28)

	paintFrame(cv, s, w, h, bg, render.Rect{})
	if resizes != 1 {
		t.Fatalf("the first paint reallocated %d times, want once", resizes)
	}

	red := render.RGB(0xff, 0, 0)
	s.Draw(render.Rect{X: 10, Y: 10, W: 20, H: 10}, func(cv *render.Canvas) {
		draws++
		cv.FillRect(render.Rect{X: 10, Y: 10, W: 20, H: 10}, red)
	})
	paintFrame(cv, s, w, h, bg, render.Rect{})
	if draws != 1 {
		t.Fatalf("the draw callback ran %d times, want once", draws)
	}

	// A full repaint with no further drawing must show the same pixels.
	s.QueueDraw()
	paintFrame(cv, s, w, h, bg, render.Rect{})
	if got := surfPx(data, w, 15, 15); got != red {
		t.Errorf("pixel (15,15) = %#x after a redraw the app did not draw, want the retained red %#x", got, red)
	}
	if draws != 1 || resizes != 1 {
		t.Errorf("a repaint asked the app for work: %d draws, %d resizes", draws, resizes)
	}
	if got := surfPx(data, w, 5, 5); got != bg {
		t.Errorf("pixel (5,5) = %#x, want the untouched background %#x", got, bg)
	}
}

func TestDrawSurfaceDamageLimitedToRect(t *testing.T) {
	s := NewDrawSurface(100, 80)
	SizeWithin(s, Size{W: 100, H: 80})
	s.Arrange(render.Rect{X: 5, Y: 7, W: 100, H: 80})
	warmDamage(s)

	s.Draw(render.Rect{X: 3, Y: 4, W: 6, H: 5}, func(*render.Canvas) {})
	rects, any := CollectDamage(s)
	if !any {
		t.Fatal("Draw produced no damage")
	}
	want := render.Rect{X: 8, Y: 11, W: 6, H: 5}
	if got := render.UnionAll(rects); got != want {
		t.Errorf("damage union %+v, want the shifted draw rect %+v alone", got, want)
	}
	if _, any := CollectDamage(s); any {
		t.Error("damage flags were not drained by the collect")
	}
}

func TestDrawSurfaceResizeKeepsContent(t *testing.T) {
	s := NewDrawSurface(50, 40)
	resizes := 0
	red := render.RGB(0xff, 0, 0)
	s.OnResize = func(cv *render.Canvas) { resizes++ }
	bg := render.RGB(0x20, 0x20, 0x28)

	data := make([]byte, render.Stride(50)*40)
	cv := render.New(data, render.Stride(50), 50, 40)
	paintFrame(cv, s, 50, 40, bg, render.Rect{})
	s.Draw(render.Rect{W: 50, H: 40}, func(cv *render.Canvas) {
		cv.FillRect(render.Rect{W: 50, H: 40}, red)
	})
	paintFrame(cv, s, 50, 40, bg, render.Rect{})
	if resizes != 1 {
		t.Fatalf("setup reallocated %d times, want once", resizes)
	}

	// Widening reallocates the buffer, keeps the overlap, and leaves the
	// exposed column transparent: the background shows through.
	data2 := make([]byte, render.Stride(80)*40)
	cv2 := render.New(data2, render.Stride(80), 80, 40)
	paintFrame(cv2, s, 80, 40, bg, render.Rect{})
	if resizes != 1 {
		t.Errorf("a same-scale resize fired OnResize %d extra times", resizes-1)
	}
	if got := surfPx(data2, 80, 10, 10); got != red {
		t.Errorf("pixel (10,10) = %#x after resize, want the kept red %#x", got, red)
	}
	if got := surfPx(data2, 80, 75, 10); got != bg {
		t.Errorf("exposed pixel (75,10) = %#x, want the background %#x", got, bg)
	}
}

func TestDrawSurfaceRescaleAsksForRedraw(t *testing.T) {
	s := NewDrawSurface(60, 40)
	resizes := 0
	red := render.RGB(0xff, 0, 0)
	s.OnResize = func(cv *render.Canvas) {
		resizes++
		cv.FillRect(render.Rect{X: 0, Y: 0, W: 30, H: 40}, red)
	}
	bg := render.RGB(0x20, 0x20, 0x28)

	data := make([]byte, render.Stride(60)*40)
	cv := render.New(data, render.Stride(60), 60, 40)
	paintFrame(cv, s, 60, 40, bg, render.Rect{})
	if resizes != 1 {
		t.Fatalf("setup reallocated %d times, want once", resizes)
	}
	s.Draw(render.Rect{X: 40, Y: 10, W: 10, H: 10}, func(cv *render.Canvas) {
		cv.FillRect(render.Rect{X: 40, Y: 10, W: 10, H: 10}, red)
	})
	paintFrame(cv, s, 60, 40, bg, render.Rect{})

	// Doubling the device scale reallocates from transparent; OnResize
	// is the app's one full redraw, at the new scale's logical size.
	data2 := make([]byte, render.Stride(120)*80)
	cv2 := render.NewScaled(data2, render.Stride(120), 120, 80, 2, 1)
	// The rescale repaints the whole frame, like a fresh buffer after a
	// scale change; the logical rect is the frame's stale region.
	paintFrame(cv2, s, 60, 40, bg, render.Rect{W: 60, H: 40})
	if resizes != 2 {
		t.Fatalf("the rescale reallocated %d times in all, want twice", resizes)
	}
	if got := surfPx(data2, 120, 20, 20); got != red {
		t.Errorf("device pixel (20,20) = %#x, want the OnResize red %#x", got, red)
	}
	if got := surfPx(data2, 120, 100, 20); got != bg {
		t.Errorf("undrawn device pixel (100,20) = %#x, want the background %#x", got, bg)
	}
}

func TestDrawSurfaceComposite(t *testing.T) {
	s := NewDrawSurface(40, 30)
	w, h := 40, 30
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	black := render.RGB(0, 0, 0)
	paintFrame(cv, s, w, h, black, render.Rect{})

	// A half-transparent white over black is mid-gray, and the Draw clip
	// confines a stray primitive to the declared rect.
	gray := render.RGB(0x80, 0x80, 0x80)
	s.Draw(render.Rect{X: 10, Y: 10, W: 5, H: 5}, func(cv *render.Canvas) {
		cv.FillRect(render.Rect{W: 40, H: 30}, render.RGBA(0xff, 0xff, 0xff, 0x80))
	})
	paintFrame(cv, s, w, h, black, render.Rect{})
	if got := surfPx(data, w, 12, 12); got != gray {
		t.Errorf("blended pixel (12,12) = %#x, want %#+v", got, gray)
	}
	if got := surfPx(data, w, 2, 2); got != black {
		t.Errorf("pixel (2,2) outside the draw rect = %#x, want the background %#x", got, black)
	}
}
