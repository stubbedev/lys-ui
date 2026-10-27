package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// kineticScroll is a 100x100 Scroll over a 100x2000 column.
func kineticScroll() *Scroll {
	s := NewScroll(NewSpacer(100, 2000))
	SizeWithin(s, Size{W: 100, H: 100})
	s.Arrange(render.Rect{W: 100, H: 100})
	return s
}

// flick feeds a finger scroll of n frames of dy pixels, 10ms apart.
func flick(c *animClock, s PixelScroller, n int, dy float64) {
	for range n {
		c.set(c.now().Add(10 * time.Millisecond))
		s.ScrollPixels(0, dy)
	}
}

// TestKineticGlide pins the glide: a fast flick travels on after the
// fingers lift, decelerating to a stop; a slow drag does not glide;
// a wheel step catches a glide.
func TestKineticGlide(t *testing.T) {
	c := pinAnimClock(t)
	s := kineticScroll()
	flick(c, s, 5, 10) // 1000 px/s
	_, lifted := s.Offset()
	s.ScrollEnd()
	var steps []int
	for c.step() {
		_, y := s.Offset()
		steps = append(steps, y)
	}
	_, end := s.Offset()
	// v*tau = 1000 * 0.325 = 325px, less the settle tail.
	if travel := end - lifted; travel < 250 || travel > 330 {
		t.Errorf("glide travelled %d px, want about 325", travel)
	}
	if len(steps) < 3 || steps[1]-steps[0] <= steps[len(steps)-1]-steps[len(steps)-2] {
		t.Errorf("glide does not decelerate: %v", steps)
	}

	slow := kineticScroll()
	flick(c, slow, 5, 0.3) // 30 px/s
	_, y0 := slow.Offset()
	slow.ScrollEnd()
	c.drive()
	if _, y := slow.Offset(); y != y0 {
		t.Errorf("a slow drag glided %d px", y-y0)
	}

	caught := kineticScroll()
	flick(c, caught, 5, 10)
	caught.ScrollEnd()
	c.step()
	caught.ScrollBy(0, 0)
	_, held := caught.Offset()
	c.drive()
	if _, y := caught.Offset(); y != held {
		t.Error("a wheel step did not catch the glide")
	}
}

// TestOvershootGlow pins the edge glow: scrolling past the top builds
// it at the start edge, the opposite push restarts it at the end, and
// lifting fades it out.
func TestOvershootGlow(t *testing.T) {
	c := pinAnimClock(t)
	s := kineticScroll()
	s.ScrollPixels(0, -80)
	if s.overshoot[1] != -0.5 {
		t.Fatalf("overshoot past the top = %v, want -0.5", s.overshoot[1])
	}
	s.SetOffset(0, 1900)
	s.ScrollPixels(0, 40)
	if s.overshoot[1] != 0.25 {
		t.Errorf("overshoot past the bottom = %v, want 0.25 (restarted)", s.overshoot[1])
	}
	s.ScrollEnd()
	c.drive()
	if s.overshoot != [2]float64{} {
		t.Errorf("released glow = %v, want faded out", s.overshoot)
	}
}

// TestRouterAxisEnd pins the routing: the scroll end reaches the
// scroller that took the gesture's pixels, even with the pointer moved
// off it, and only once.
func TestRouterAxisEnd(t *testing.T) {
	c := pinAnimClock(t)
	s := kineticScroll()
	r := &Router{Root: s}
	r.Move(Point{X: 50, Y: 50})
	for range 5 {
		c.set(c.now().Add(10 * time.Millisecond))
		r.AxisPixels(0, 10, 0)
	}
	r.Leave()
	_, before := s.Offset()
	r.AxisEnd()
	c.drive()
	if _, y := s.Offset(); y <= before {
		t.Error("AxisEnd did not reach the gesture's scroller")
	}
	if r.pixelTarget != nil {
		t.Error("the target outlived its gesture")
	}
}

// TestGoldenOvershootGlow pins the start edge's glow after a push past
// the top.
func TestGoldenOvershootGlow(t *testing.T) {
	th := DarkTheme()
	s := NewScroll(NewSpacer(100, 400))
	s.ShowBars = false
	SizeWithin(s, Size{W: 120, H: 80})
	s.Arrange(render.Rect{W: 120, H: 80})
	s.ScrollPixels(0, -120)
	NewGolden(t, s, "overshoot", goldenTheme(th), goldenFrame(120, 80))
}
