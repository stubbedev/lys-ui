package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestVerticalSlider pins the stood-up slider: it measures tall and
// thin, maps the pointer by y (minimum at the top, at the bottom when
// inverted), and the arrows move the knob the way they point.
func TestVerticalSlider(t *testing.T) {
	s := NewSlider(0, 100, 0, 0)
	s.SetAxis(Column)
	if !s.hasClass("vertical") || s.hasClass("horizontal") {
		t.Error("orientation classes not swapped")
	}
	sz := s.Measure(Constraints{Max: Size{W: 500, H: 500}})
	if sz.W != 18 || sz.H != 200 {
		t.Fatalf("vertical natural = %v, want 18x200", sz)
	}
	s.Arrange(render.Rect{W: 18, H: 208})
	if v := s.ValueAt(Point{Y: 4}); v != 0 {
		t.Errorf("top = %v, want the minimum", v)
	}
	if v := s.ValueAt(Point{Y: 104}); v != 50 {
		t.Errorf("middle = %v, want 50", v)
	}
	s.SetInverted(true)
	if v := s.ValueAt(Point{Y: 4}); v != 100 {
		t.Errorf("inverted top = %v, want the maximum", v)
	}
	s.SetValue(50)
	s.KeyAction(KeyUp, 0)
	if s.Value() != 60 {
		t.Errorf("inverted Up = %v, want more (the knob rises)", s.Value())
	}
	s.SetInverted(false)
	s.KeyAction(KeyUp, 0)
	if s.Value() != 50 {
		t.Errorf("Up = %v, want less (toward the top minimum)", s.Value())
	}
}

// TestSpinSteppers pins the drawn steppers: a press on the upper half
// steps up, the lower half down, and a read-only spin ignores both.
func TestSpinSteppers(t *testing.T) {
	face := chromeFace(t)
	s := NewSpinButton(face, 14, DarkTheme().Text, 0, 10, 1, 0)
	s.Measure(Constraints{Max: Size{W: 200, H: 100}})
	s.Arrange(render.Rect{W: 120, H: 30})
	r := s.trailingRect()
	if r.Empty() || r.X+r.W > 120 {
		t.Fatalf("stepper rect = %+v", r)
	}
	s.ClickAt(Point{X: r.X + 2, Y: r.Y + 1})
	s.ClickAt(Point{X: r.X + 2, Y: r.Y + 1})
	s.ClickAt(Point{X: r.X + 2, Y: r.Y + r.H - 1})
	if s.Value() != 1 {
		t.Errorf("up, up, down = %v, want 1", s.Value())
	}
	s.SetReadOnly(true)
	s.ClickAt(Point{X: r.X + 2, Y: r.Y + 1})
	if s.Value() != 1 {
		t.Error("a read-only spin stepped")
	}
}

// TestGoldenOrientation pins a vertical slider beside a spin button
// with its steppers.
func TestGoldenOrientation(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	v := NewSlider(0, 100, 0, 30)
	v.SetAxis(Column)
	v.SetInverted(true)
	spin := NewSpinButton(face, 14, th.Text, 0, 10, 1, 0)
	spin.SetValue(7)
	row := NewBox(Row, 12, 0)
	row.Append(v, false)
	row.AppendAligned(spin, false, AlignStart)
	NewGolden(t, row, "orientation", goldenTheme(th), goldenFrame(180, 210))
}
