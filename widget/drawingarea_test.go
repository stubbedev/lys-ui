package widget

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestGoldenDrawingArea pins a pressure-sensitive stroke: stylus
// samples feed a drawing area that strokes each segment as wide as the
// pen pressed.
func TestGoldenDrawingArea(t *testing.T) {
	area := NewDrawingArea(220, 80)
	var pts []Stylus
	area.OnStylus = func(s Stylus) {
		if s.Down {
			pts = append(pts, s)
			area.QueueDraw()
		}
	}
	area.OnDraw = func(cv *render.Canvas, r render.Rect) {
		cv.FillRect(r, render.RGB(0x18, 0x18, 0x25))
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			w := 1 + int(math.Round(b.Pressure*8))
			cv.Line(r.X+a.At.X, r.Y+a.At.Y, r.X+b.At.X, r.Y+b.At.Y, w, render.RGB(0xcb, 0xa6, 0xf7))
		}
	}
	r := &Router{Root: area}
	area.Measure(Constraints{Max: Size{W: 220, H: 80}})
	area.Arrange(render.Rect{W: 220, H: 80})
	for i := range 21 {
		x := 10 + i*10
		y := 40 + int(25*math.Sin(float64(i)/3))
		p := 0.5 - 0.5*math.Cos(float64(i)*math.Pi/20) // a stroke that swells and tapers
		r.Move(Point{X: x, Y: y})                      // the bridge moves the pointer first
		r.Stylus(Stylus{At: Point{X: x, Y: y}, Pressure: p, Down: true})
	}
	if len(pts) != 21 {
		t.Fatalf("the area took %d samples", len(pts))
	}
	NewGolden(t, area, "drawingarea-stylus", goldenTheme(DarkTheme()))
}
