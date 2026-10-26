package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A levelbar carries GTK's node tree - levelbar > trough > block.filled
// - so the stylesheet's gauge selectors reach it: the block paints from
// the cascade, the variant class on the bar tints it, and the value
// sizes the block.
func TestLevelBarPaintsFromTheCascade(t *testing.T) {
	loadCSS(t, `
		levelbar.battery-gauge { all: unset; min-height: 24px; border-radius: 6px; background: #101018; }
		levelbar.battery-gauge trough { all: unset; min-height: 24px; border-radius: 6px; background: #101018; }
		levelbar.battery-gauge block.filled { border-radius: 6px; }
		levelbar.battery-gauge.good block.filled { background: #00ff00; }
		levelbar.battery-gauge.crit block.filled { background: #ff0000; }
	`)
	root := NewBox(Column, 0, 0)
	bar := NewLevelBar(0.5)
	bar.AddClass("battery-gauge", "good")
	root.Append(bar, false)
	arrangeTree(t, root, 200, 100)

	data := make([]byte, render.Stride(200)*100)
	cv := render.NewScaled(data, render.Stride(200), 200, 100, 1, 1)
	cv.Clear(cv.Rect(), 0)
	PaintChild(cv, root)
	// The block is half the 200px trough, pure green, starting at x=0.
	count := func(col func(c render.Color) bool) int {
		n := 0
		for x := range 200 {
			c := render.ColorFromBytes(data[(5*200+x)*4:])
			if col(c) {
				n++
			}
		}
		return n
	}
	green := count(func(c render.Color) bool { return c.R() == 0 && c.G() == 255 })
	if green < 98 || green > 100 {
		t.Errorf("the good block covers %dpx at half value, want 100", green)
	}
	track := count(func(c render.Color) bool { return c.R() == 0x10 && c.B() == 0x18 })
	if track < 95 {
		t.Errorf("the trough track covers %dpx, want the full 200 minus the block", track)
	}

	// The variant class drives the fill: crit is red, no Go color set.
	bar.RemoveClass("good")
	bar.AddClass("crit")
	bar.SetValue(1)
	arrangeTree(t, root, 200, 100)
	data = make([]byte, render.Stride(200)*100)
	cv = render.NewScaled(data, render.Stride(200), 200, 100, 1, 1)
	cv.Clear(cv.Rect(), 0)
	PaintChild(cv, root)
	red := count(func(c render.Color) bool { return c.R() == 255 && c.G() == 0 })
	if red < 198 || red > 200 {
		t.Errorf("the crit block covers %dpx at full value, want 200", red)
	}
}

// A canvas-drawn widget reads its ink and stroke from the cascade:
// CascadeColor is the computed color (state classes change it), and
// CascadeBorder is the computed border widths.
func TestCascadeReaders(t *testing.T) {
	face := goldenFace(t)
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(`
		.ring { border: 3px solid transparent; color: #00aaff; }
		.ring.error { color: #ff0044; }
	`, StylePriorityUser))
	ring := NewLabel(face, 10, "", 0)
	ring.AddClass("ring")
	root.Append(ring, false)
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	root.Arrange(render.Rect{W: 100, H: 20})

	if got := CascadeColor(ring); got != render.RGB(0x00, 0xaa, 0xff) {
		t.Errorf("cascade color = %v, want #00aaff", got)
	}
	if got := CascadeBorder(ring); got.Top != 3 {
		t.Errorf("cascade border top = %d, want 3", got.Top)
	}
	ring.AddClass("error")
	if got := CascadeColor(ring); got != render.RGB(0xff, 0x00, 0x44) {
		t.Errorf("error state color = %v, want #ff0044", got)
	}
}

// PaintBoxLayers paints a custom widget's own cascade at its bounds:
// the background lands, and a :hover rule drives it once the widget is
// hovered.
func TestPaintBoxLayers(t *testing.T) {
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(`
		.row { background: #202030; }
		.row.available:hover { background: #00ff00; }
	`, StylePriorityUser))
	row := newHoverRow()
	row.AddClass("row", "available")
	row.Append(NewSpacer(10, 10), false)
	root.Append(row, false)
	sz := root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	root.Arrange(render.Rect{W: 100, H: sz.H})

	paint := func() render.Color {
		data := make([]byte, render.Stride(100)*100)
		cv := render.NewScaled(data, render.Stride(100), 100, 100, 1, 1)
		cv.Clear(cv.Rect(), 0)
		PaintChild(cv, root)
		return render.ColorFromBytes(data[(5*100+5)*4:])
	}
	if got := paint(); got != render.RGB(0x20, 0x20, 0x30) {
		t.Errorf("row background = %v, want #202030", got)
	}
	setHoverChain(nil, row)
	if got := paint(); got != render.RGB(0, 255, 0) {
		t.Errorf("hovered row background = %v, want the :hover green", got)
	}
}

// hoverRow is a custom widget whose Paint defers its chrome to
// PaintBoxLayers - the bluetooth row's shape: a node of its own over
// an inner box of content.
type hoverRow struct {
	node
	inner *Box
}

func newHoverRow() *hoverRow {
	r := &hoverRow{inner: NewBox(Row, 0, 0)}
	setParents(r, r.inner)
	return r
}

func (r *hoverRow) Append(w Widget, _ bool) { r.inner.Append(w, false) }

func (r *hoverRow) Measure(con Constraints) Size { return r.inner.Measure(con) }

func (r *hoverRow) Arrange(rect render.Rect) {
	r.node.Arrange(rect)
	r.inner.Arrange(rect)
}

func (r *hoverRow) Paint(cv *render.Canvas) {
	PaintBoxLayers(cv, r)
	PaintChild(cv, r.inner)
}

func (r *hoverRow) HitTest(pt Point) Widget { return r.HitLeaf(r, pt) }
