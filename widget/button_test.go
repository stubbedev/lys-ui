package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestButtonMeasure(t *testing.T) {
	t.Run("pads the child on every side", func(t *testing.T) {
		b := NewButton(newStub(10, 6), 4, 2)
		got := SizeWithin(b, Size{W: 100, H: 100})
		if got != (Size{W: 18, H: 14}) {
			t.Errorf("button measure = %v, want 18x14", got)
		}
	})

	t.Run("overflows a smaller host rather than squeeze", func(t *testing.T) {
		b := NewButton(newStub(50, 50), 4, 2)
		if got := SizeWithin(b, Size{W: 20, H: 20}); got != (Size{W: 58, H: 58}) {
			t.Errorf("button measure = %v, want its 58x58 minimum", got)
		}
	})
}

func TestButtonArrange(t *testing.T) {
	child := newStub(10, 6)
	b := NewButton(child, 4, 2)
	SizeWithin(b, Size{W: 100, H: 100})
	b.Arrange(render.Rect{X: 10, Y: 5, W: 30, H: 20})

	if got := b.Bounds(); got != (render.Rect{X: 10, Y: 5, W: 30, H: 20}) {
		t.Errorf("button bounds = %v, want the arranged rect", got)
	}
	if child.rect != (render.Rect{X: 14, Y: 9, W: 22, H: 12}) {
		t.Errorf("child rect = %v, want inset by padding 4", child.rect)
	}
}

func TestButtonPaintStates(t *testing.T) {
	cvSize := 40
	stride := render.Stride(cvSize)
	data := make([]byte, stride*cvSize)
	newPainted := func(pressed, hovered bool) render.Color {
		cv := render.New(data, stride, cvSize, cvSize)
		clear(data)
		b := NewButton(newStub(0, 0), 0, 0)
		b.Bg = render.RGB(10, 10, 10)
		b.BgHover = render.RGB(20, 20, 20)
		b.BgPressed = render.RGB(30, 30, 30)
		b.Pressed = pressed
		b.Hovered = hovered
		SizeWithin(b, Size{W: 100, H: 100})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 40})
		b.Paint(cv)
		return render.ColorFromBytes(data[20*stride+20*4 : 20*stride+20*4+4])
	}

	t.Run("plain state paints Bg", func(t *testing.T) {
		if got := newPainted(false, false); got != render.RGB(10, 10, 10) {
			t.Errorf("plain pixel = %v, want Bg", got)
		}
	})

	t.Run("hovered paints BgHover", func(t *testing.T) {
		if got := newPainted(false, true); got != render.RGB(20, 20, 20) {
			t.Errorf("hover pixel = %v, want BgHover", got)
		}
	})

	t.Run("pressed wins over hovered", func(t *testing.T) {
		if got := newPainted(true, true); got != render.RGB(30, 30, 30) {
			t.Errorf("pressed pixel = %v, want BgPressed", got)
		}
	})
}

func TestButtonHitTest(t *testing.T) {
	b := NewButton(newStub(10, 6), 4, 2)
	SizeWithin(b, Size{W: 100, H: 100})
	b.Arrange(render.Rect{X: 10, Y: 5, W: 30, H: 20})

	t.Run("inside returns the button, not its child", func(t *testing.T) {
		if got := b.HitTest(Point{X: 15, Y: 10}); got != Widget(b) {
			t.Errorf("hit = %v, want the button", got)
		}
	})

	t.Run("outside returns nil", func(t *testing.T) {
		if got := b.HitTest(Point{X: 0, Y: 0}); got != nil {
			t.Errorf("hit = %v, want nil", got)
		}
	})
}

func TestButtonClick(t *testing.T) {
	t.Run("fires OnClick", func(t *testing.T) {
		fired := false
		b := NewButton(newStub(1, 1), 0, 0)
		b.OnClick = func() { fired = true }
		b.ClickAt(Point{X: 0, Y: 0})
		if !fired {
			t.Error("ClickAt did not fire OnClick")
		}
	})

	t.Run("without OnClick it does not panic", func(t *testing.T) {
		b := NewButton(newStub(1, 1), 0, 0)
		b.ClickAt(Point{X: 0, Y: 0})
	})
}

func TestButtonBgExplicit(t *testing.T) {
	cvSize := 40
	stride := render.Stride(cvSize)
	data := make([]byte, stride*cvSize)
	backdrop := render.RGB(200, 0, 0)
	paint := func(setup func(b *Button)) render.Color {
		cv := render.New(data, stride, cvSize, cvSize)
		cv.FillRect(render.Rect{W: cvSize, H: cvSize}, backdrop)
		b := NewButton(newStub(0, 0), 0, 0)
		setup(b)
		SizeWithin(b, Size{W: 100, H: 100})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 40})
		b.Paint(cv)
		return render.ColorFromBytes(data[20*stride+20*4 : 20*stride+20*4+4])
	}

	t.Run("zero Bg is transparent", func(t *testing.T) {
		if got := paint(func(b *Button) { b.BgExplicit = true }); got != backdrop {
			t.Errorf("resting pixel = %v, want the backdrop through a transparent button", got)
		}
	})

	t.Run("hover still fills", func(t *testing.T) {
		got := paint(func(b *Button) {
			b.BgExplicit = true
			b.BgHover = render.RGB(0, 0, 90)
			b.Hovered = true
		})
		if got != render.RGB(0, 0, 90) {
			t.Errorf("hover pixel = %v, want BgHover", got)
		}
	})

	t.Run("the stylesheet does not refill it", func(t *testing.T) {
		LoadStylesheet("button { background-color: #00ff00; }")
		defer LoadStylesheet("")
		if got := paint(func(b *Button) { b.BgExplicit = true }); got != backdrop {
			t.Errorf("styled explicit pixel = %v, want transparent", got)
		}
		if got := paint(func(*Button) {}); got != render.RGB(0, 255, 0) {
			t.Errorf("styled default pixel = %v, want the stylesheet background", got)
		}
	})

	t.Run("without it zero means the theme surface", func(t *testing.T) {
		if got := paint(func(*Button) {}); got != Current().Surface {
			t.Errorf("default pixel = %v, want the theme surface", got)
		}
	})
}
