package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// Regression: presses on a label inside a control used to be classified
// as chrome by demos checking the deepest hovered widget, so clicks on
// buttons-over-text, list rows, and scroll areas started an interactive
// window move instead of reaching the widget. IsInteractive must walk
// the parent chain.
func TestIsInteractive(t *testing.T) {
	face := entryFace(t)

	t.Run("nil is not interactive", func(t *testing.T) {
		if IsInteractive(nil) {
			t.Error("nil classified as interactive")
		}
	})

	t.Run("a bare label is chrome", func(t *testing.T) {
		l := NewLabel(face, 12, "chrome", render.RGB(255, 255, 255))
		l.Arrange(render.Rect{X: 0, Y: 0, W: 50, H: 20})
		if IsInteractive(l) {
			t.Error("bare label classified as interactive")
		}
	})

	t.Run("a label inside a button is interactive", func(t *testing.T) {
		lbl := NewLabel(face, 15, "click me", render.RGB(255, 255, 255))
		btn := NewButton(NewBox(Row, 8, 0).Append(lbl, false), 10, 8)
		root := NewBox(Column, 0, 0).Append(btn, false)
		root.Measure(Constraints{Max: Size{W: 200, H: 100}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 100})
		bb := btn.Bounds()
		over := root.HitTest(Point{X: bb.X + bb.W/2, Y: bb.Y + bb.H/2})
		if _, ok := over.(*Button); !ok {
			t.Fatalf("button center hit %T, want the button", over)
		}
		if !IsInteractive(over) {
			t.Error("button not classified as interactive")
		}
	})

	t.Run("a row inside a scroll is interactive", func(t *testing.T) {
		list := NewBox(Column, 4, 0)
		for range 8 {
			list.Append(NewLabel(face, 12, "server-01.example", render.RGB(255, 255, 255)), false)
		}
		sc := NewScroll(list)
		root := NewBox(Column, 0, 0).Append(sc, true)
		root.Measure(Constraints{Max: Size{W: 200, H: 120}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 120})
		b := sc.Bounds()
		over := root.HitTest(Point{X: b.X + 5, Y: b.Y + 5})
		if !IsInteractive(over) {
			t.Errorf("list row hit %T classified as chrome; a press would move the window", over)
		}
	})
}

// Regression: Switch and CheckButton used to expose Click(), which the
// Router never invokes, so they were dead to every click.
func TestTogglesReceiveRouterClicks(t *testing.T) {
	sw := NewSwitch(true)
	root := NewBox(Column, 0, 0).Append(sw, false)
	root.Measure(Constraints{Max: Size{W: 60, H: 40}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 40})
	r := &Router{Root: root}
	p := Point{X: sw.Bounds().X + 5, Y: sw.Bounds().Y + 5}
	r.Move(p)
	r.Press(BTNLeft, p, 0)
	r.Release(BTNLeft, p)
	if sw.On() {
		t.Error("click did not toggle the switch off")
	}
	r.Press(BTNLeft, p, 0)
	r.Release(BTNLeft, p)
	if !sw.On() {
		t.Error("second click did not toggle the switch back on")
	}

	check := NewCheckButton(false)
	root2 := NewBox(Column, 0, 0).Append(check, false)
	root2.Measure(Constraints{Max: Size{W: 60, H: 40}})
	root2.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 40})
	r2 := &Router{Root: root2}
	p2 := Point{X: check.Bounds().X + 5, Y: check.Bounds().Y + 5}
	r2.Move(p2)
	r2.Press(BTNLeft, p2, 0)
	r2.Release(BTNLeft, p2)
	if !check.Checked() {
		t.Error("click did not check the checkbox")
	}
}
