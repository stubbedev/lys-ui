package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// swipeCard is a 200x60 SwipeDismiss over a button filling it, in a
// Router, counting the button's clicks and the dismissals.
type swipeCard struct {
	r       *Router
	s       *SwipeDismiss
	clicks  int
	dismiss []int
}

func newSwipeCard(t *testing.T) *swipeCard {
	t.Helper()
	sc := &swipeCard{}
	btn := NewButton(NewSpacer(200, 60), 0, 0)
	btn.OnClick = func() { sc.clicks++ }
	sc.s = NewSwipeDismiss(btn)
	sc.s.OnDismiss = func(dir int) { sc.dismiss = append(sc.dismiss, dir) }
	arrangeTree(t, sc.s, 200, 60)
	sc.r = &Router{Root: sc.s}
	return sc
}

// drag presses at (x0, 30), moves through xs one frame apart, and
// releases at the last.
func (sc *swipeCard) drag(c *animClock, x0 int, ys int, xs ...int) {
	sc.r.Move(Point{X: x0, Y: 30})
	sc.r.Press(BTNLeft, Point{X: x0, Y: 30}, 0)
	p := Point{X: x0, Y: 30}
	for _, x := range xs {
		c.set(c.now().Add(16 * time.Millisecond))
		p = Point{X: x, Y: 30 + ys}
		sc.r.Move(p)
	}
	sc.r.Release(BTNLeft, p)
}

// settle runs the clock until nothing is scheduled.
func settleClock(c *animClock) {
	for c.step() {
	}
}

// A swipe past 40% of the width sends the card out and fires
// OnDismiss once, its direction given, and the button the press landed
// on never clicks; a shorter one springs back, dismissing nothing
// (#333).
func TestSwipeDismissThroughTheRouter(t *testing.T) {
	c := pinAnimClock(t)
	sc := newSwipeCard(t)
	sc.drag(c, 50, 0, 60, 70, 80, 90, 100)
	settleClock(c)
	if sc.s.Offset() != 0 || len(sc.dismiss) != 0 || sc.clicks != 0 {
		t.Fatalf("a 50px swipe: offset %v, dismissals %v, clicks %d; want it back, none, none", sc.s.Offset(), sc.dismiss, sc.clicks)
	}
	sc.drag(c, 50, 0, 60, 90, 120, 150, 180)
	if sc.s.Offset() == 0 {
		t.Fatal("the card did not follow the pointer")
	}
	settleClock(c)
	if len(sc.dismiss) != 1 || sc.dismiss[0] != 1 || sc.clicks != 0 {
		t.Fatalf("a 130px swipe right: dismissals %v, clicks %d; want [1] and none", sc.dismiss, sc.clicks)
	}
	if sc.s.Offset() != 200 {
		t.Errorf("dismissed card at %v, want out at 200", sc.s.Offset())
	}
	sc.drag(c, 150, 0, 140, 120, 90, 60, 30)
	settleClock(c)
	if len(sc.dismiss) != 1 {
		t.Errorf("a dismissed card dismissed again: %v", sc.dismiss)
	}
}

// A press that never leaves the slop clicks the button; one that
// travels up or down more than sideways is no swipe; a short fast
// flick out left dismisses as a fling.
func TestSwipeDismissLeavesClicksAndVerticalDrags(t *testing.T) {
	c := pinAnimClock(t)
	sc := newSwipeCard(t)
	sc.drag(c, 100, 0, 103, 104)
	settleClock(c)
	if sc.clicks != 1 || sc.s.Offset() != 0 {
		t.Errorf("a press inside the slop: clicks %d, offset %v; want a click and no swipe", sc.clicks, sc.s.Offset())
	}
	sc.drag(c, 100, 25, 105, 110)
	settleClock(c)
	if sc.s.Offset() != 0 || len(sc.dismiss) != 0 {
		t.Errorf("a mostly vertical drag swiped: offset %v, dismissals %v", sc.s.Offset(), sc.dismiss)
	}
	sc.r.Move(Point{X: 100, Y: 30})
	sc.r.Press(BTNLeft, Point{X: 100, Y: 30}, 0)
	c.set(c.now().Add(5 * time.Millisecond))
	sc.r.Move(Point{X: 80, Y: 30})
	c.set(c.now().Add(5 * time.Millisecond))
	sc.r.Move(Point{X: 60, Y: 30})
	sc.r.Release(BTNLeft, Point{X: 60, Y: 30})
	settleClock(c)
	if len(sc.dismiss) != 1 || sc.dismiss[0] != -1 {
		t.Errorf("a 40px flick left at 4000 px/s: dismissals %v, want [-1]", sc.dismiss)
	}
}

// A sideways two-finger scroll drags the card and its end settles it;
// a vertical one is left to the scrollers beneath.
func TestSwipeDismissTakesASidewaysFingerScroll(t *testing.T) {
	c := pinAnimClock(t)
	sc := newSwipeCard(t)
	sc.r.Move(Point{X: 100, Y: 30})
	sc.r.AxisPixels(0, 40, 0)
	sc.r.AxisEnd()
	if sc.s.Offset() != 0 {
		t.Fatalf("a vertical finger scroll moved the card to %v", sc.s.Offset())
	}
	for range 5 {
		c.set(c.now().Add(16 * time.Millisecond))
		sc.r.AxisPixels(-30, 2, 0)
	}
	if sc.s.Offset() != 150 {
		t.Fatalf("five 30px sideways deltas put the card at %v, want 150", sc.s.Offset())
	}
	sc.r.AxisEnd()
	settleClock(c)
	if len(sc.dismiss) != 1 || sc.dismiss[0] != 1 {
		t.Errorf("a 150px finger swipe: dismissals %v, want [1]", sc.dismiss)
	}
}

// The card fades with its offset: halfway out it paints at half
// opacity.
func TestSwipeDismissFades(t *testing.T) {
	loadCSS(t, `.c { background-color: #ffffff; }`)
	inner := NewBox(Column, 0, 0)
	inner.AddClass("c")
	inner.Append(NewSpacer(200, 60), false)
	s := NewSwipeDismiss(inner)
	arrangeTree(t, s, 200, 60)
	s.dx = 100
	s.place()
	data := paintTree(s, 300, 60)
	if got := render.ColorFromBytes(data[30*render.Stride(300)+150*4:]); got.A() < 120 || got.A() > 135 {
		t.Errorf("halfway out the card paints alpha %d, want about half", got.A())
	}
}
