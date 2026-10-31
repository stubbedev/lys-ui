package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/text"
)

type countingFont struct {
	render.Font
	shapes *int
}

func (f countingFont) ShapeDir(s string, px float64, d text.Direction) *render.ShapedText {
	*f.shapes++
	return f.Font.ShapeDir(s, px, d)
}

func TestEntrySteadyArrangeShapesNothing(t *testing.T) {
	shapes := 0
	e := NewEntry(countingFont{Font: entryFace(t), shapes: &shapes}, 14, Current().Text)
	e.SetText("a line long enough to pan inside a narrow field")
	host := NewBox(Row, 0, 0).Append(e, true)
	frame(t, host, 120, 40)
	pan := e.ScrollX()
	shapes = 0
	host.Arrange(render.Rect{W: 120, H: 40})
	if shapes != 0 {
		t.Errorf("a steady arrange shaped the text %d times, want 0", shapes)
	}
	if e.ScrollX() != pan {
		t.Errorf("a steady arrange moved the pan %d -> %d", pan, e.ScrollX())
	}
	frame(t, host, 60, 40)
	if shapes == 0 {
		t.Error("a narrower field did not re-pan")
	}
	e.SetText("short")
	frame(t, host, 60, 40)
	if e.ScrollX() != 0 {
		t.Errorf("short text kept a pan of %d", e.ScrollX())
	}
}
