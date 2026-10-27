package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestSeparatorMeasure(t *testing.T) {
	t.Run("both orientations want one pixel", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			sep  *Separator
		}{
			{"horizontal", NewSeparator(Horizontal)},
			{"vertical", NewSeparator(Vertical)},
		} {
			got := SizeWithin(tc.sep, Size{W: 200, H: 50})
			if got.W != 1 || got.H != 1 {
				t.Errorf("%s natural size = %v, want 1x1", tc.name, got)
			}
		}
	})

	t.Run("orientation round-trips", func(t *testing.T) {
		if got := NewSeparator(Horizontal).Orientation(); got != Horizontal {
			t.Errorf("orientation = %v, want horizontal", got)
		}
		if got := NewSeparator(Vertical).Orientation(); got != Vertical {
			t.Errorf("orientation = %v, want vertical", got)
		}
	})
}

func TestSeparatorPaint(t *testing.T) {
	bg := Current().Bg

	t.Run("horizontal paints its center row", func(t *testing.T) {
		w, h := 60, 11
		data := make([]byte, render.Stride(w)*h)
		cv := render.New(data, render.Stride(w), w, h)
		cv.Clear(cv.Rect(), bg)
		sep := NewSeparator(Horizontal)
		SizeWithin(sep, Size{W: w, H: h})
		sep.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
		sep.Paint(cv)

		mid := 5 * render.Stride(w)
		for x := range w {
			if c := render.ColorFromBytes(data[mid+x*4:]); c == bg {
				t.Fatalf("horizontal rule missing at x=%d", x)
			}
		}
		for _, y := range []int{0, 3, 7, 10} {
			if c := render.ColorFromBytes(data[y*render.Stride(w)+20*4:]); c != bg {
				t.Fatalf("horizontal rule bled onto row %d", y)
			}
		}
	})

	t.Run("vertical paints its center column", func(t *testing.T) {
		w, h := 11, 60
		data := make([]byte, render.Stride(w)*h)
		cv := render.New(data, render.Stride(w), w, h)
		cv.Clear(cv.Rect(), bg)
		sep := NewSeparator(Vertical)
		SizeWithin(sep, Size{W: w, H: h})
		sep.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
		sep.Paint(cv)

		for y := range h {
			if c := render.ColorFromBytes(data[y*render.Stride(w)+5*4:]); c == bg {
				t.Fatalf("vertical rule missing at y=%d", y)
			}
		}
		for _, x := range []int{0, 3, 7, 10} {
			if c := render.ColorFromBytes(data[20*render.Stride(w)+x*4:]); c != bg {
				t.Fatalf("vertical rule bled onto column %d", x)
			}
		}
	})

	t.Run("degenerate rects do not panic", func(t *testing.T) {
		cv := render.New(make([]byte, render.Stride(4)*4), render.Stride(4), 4, 4)
		for _, o := range []Orientation{Horizontal, Vertical} {
			sep := NewSeparator(o)
			SizeWithin(sep, Size{W: 4, H: 4})
			sep.Arrange(render.Rect{X: 1, Y: 1, W: 1, H: 1})
			sep.Paint(cv)
			sep.Arrange(render.Rect{})
			sep.Paint(cv)
		}
	})
}
