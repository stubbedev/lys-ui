package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// Clipping containers clip to their logical bounds on a scaled
// canvas: at 2x and 1.5x a scroll, a list and an entry's viewport
// cover their whole device extent, not the top-left part a logical
// rect read as device pixels would.
func TestClipsFollowTheDeviceScale(t *testing.T) {
	for _, scale := range []struct{ num, denom int }{{2, 1}, {3, 2}} {
		for name, build := range map[string]func() Widget{
			"scroll": func() Widget {
				return NewScroll(&solidLeaf{sz: Size{W: 40, H: 40}, col: revealFill})
			},
			"list": func() Widget {
				return NewList[Widget](staticRows{&solidLeaf{sz: Size{W: 40, H: 40}, col: revealFill}}, 40)
			},
			"revealer": func() Widget {
				r := NewRevealer(&solidLeaf{sz: Size{W: 40, H: 40}, col: revealFill})
				r.SetTransition(RevealSlideDown)
				r.SetCollapse(true)
				r.SetDuration(0)
				r.SetRevealed(true)
				return r
			},
		} {
			w := build()
			SizeWithin(w, Size{W: 40, H: 40})
			w.Arrange(render.Rect{W: 40, H: 40})
			dev := 40 * scale.num / scale.denom
			data := make([]byte, render.Stride(dev)*dev)
			cv := render.NewScaled(data, render.Stride(dev), dev, dev, scale.num, scale.denom)
			cv.ClearDevice(cv.Rect(), revealBG)
			w.Paint(cv)
			// The far device corner belongs to the logical 40x40.
			x, y := dev-2, dev-2
			if got := data[y*render.Stride(dev)+x*4+2]; got != 255 {
				t.Errorf("%s at %d/%d: device (%d, %d) red %d, want the child painted there", name, scale.num, scale.denom, x, y, got)
			}
		}
	}
}
