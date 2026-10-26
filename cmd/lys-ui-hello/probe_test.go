package main

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/component/componenttest"
	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

// arrangedShowcase builds the showcase tree laid out at the demo window
// size, ready for hit tests and router input.
func arrangedShowcase(t *testing.T) *showcase {
	t.Helper()
	face, err := app.UIFont(14)
	if err != nil {
		t.Skip("no system font:", err)
	}
	show := &showcase{env: ui.Env{Face: face, Size: 14}}
	var loop componenttest.Loop
	component.Launch(&loop, show)
	const (
		w, h = 640, 470
	)
	show.root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	return show
}

func center(w widget.Widget) widget.Point {
	b := w.(interface {
		Bounds() render.Rect
	}).Bounds()
	return widget.Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
}

func TestShowcaseControlsReceivePresses(t *testing.T) {
	show := arrangedShowcase(t)
	router := &widget.Router{Root: show.root}

	t.Run("hit tests reach every control", func(t *testing.T) {
		for _, c := range []struct {
			name string
			w    widget.Widget
		}{
			{"button", show.button},
			{"slider", show.slider},
			{"switch", show.sw},
			{"checkbox", show.check},
			{"entry", show.entry},
			{"text area", show.area},
			{"scroll", show.scrolled},
		} {
			over := show.root.HitTest(center(c.w))
			if over == nil {
				t.Errorf("%s: hit test at %v missed the tree entirely", c.name, center(c.w))
				continue
			}
			if !widget.IsInteractive(over) {
				t.Errorf("%s: hit %T is not interactive; a press would move the window", c.name, over)
			}
		}
	})

	t.Run("press and release click the button", func(t *testing.T) {
		before := 0
		show.button.OnClick = func() { before++ }
		p := center(show.button)
		router.Move(p)
		router.Press(widget.BTNLeft, p, 0)
		router.Release(widget.BTNLeft, p)
		if before != 1 {
			t.Errorf("button clicks = %d, want 1", before)
		}
	})

	t.Run("press and release toggle the switch", func(t *testing.T) {
		want := !show.sw.On()
		p := center(show.sw)
		router.Move(p)
		router.Press(widget.BTNLeft, p, 0)
		router.Release(widget.BTNLeft, p)
		if show.sw.On() != want {
			t.Errorf("switch = %v, want toggled to %v", show.sw.On(), want)
		}
	})

	t.Run("wheel over the list scrolls it", func(t *testing.T) {
		before, _ := show.scrolled.Offset()
		p := center(show.scrolled)
		if over := show.root.HitTest(p); !widget.IsInteractive(over) {
			t.Fatalf("list content hit %T is not interactive", over)
		}
		router.Move(p)
		router.Axis(0, 3, 0)
		afterX, afterY := show.scrolled.Offset()
		t.Logf("raw after Axis: x=%d y=%d (before=%d,%d)", afterX, afterY, before, 0)
		if afterY <= before {
			t.Errorf("scroll offsetY = %d, want > %d after wheel down", afterY, before)
		}
	})
}
