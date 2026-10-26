package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// stackAt is a 40x20 stack at (20, 20) with a red "a" and a blue "b"
// page.
func stackAt(t StackTransition) *Stack {
	s := NewStack()
	s.Add("a", &solidLeaf{sz: Size{W: 40, H: 20}, col: render.RGB(255, 0, 0)})
	s.Add("b", &solidLeaf{sz: Size{W: 40, H: 20}, col: render.RGB(0, 0, 255)})
	s.SetTransition(t, 100*time.Millisecond)
	s.Measure(Constraints{Max: Size{W: 80, H: 60}})
	s.Arrange(render.Rect{X: 20, Y: 20, W: 40, H: 20})
	return s
}

func paintStack(s *Stack) shot {
	data := make([]byte, render.Stride(80)*60)
	cv := render.New(data, render.Stride(80), 80, 60)
	cv.Clear(cv.Rect(), revealBG)
	s.Paint(cv)
	return data
}

func blueAt(data shot, x, y int) uint8 { return data[y*render.Stride(80)+x*4] }

func TestStackWithoutTransitionSwitchesAtOnce(t *testing.T) {
	c := pinAnimClock(t)
	s := stackAt(StackNone)
	s.Show("b")
	if s.Switching() || c.step() {
		t.Error("a plain stack animated")
	}
	if px := paintStack(s); blueAt(px, 30, 30) != 255 || redAt(px, 30, 30) != 0 {
		t.Error("the new page is not shown at once")
	}
}

func TestStackCrossfade(t *testing.T) {
	c := pinAnimClock(t)
	s := stackAt(StackCrossfade)
	s.Show("b")
	if !s.Switching() {
		t.Fatal("no switch running")
	}
	advance(c, 30*time.Millisecond)
	px := paintStack(s)
	if r, b := redAt(px, 30, 30), blueAt(px, 30, 30); r == 0 || b == 0 || int(r)+int(b) < 250 {
		t.Errorf("mid crossfade red %d blue %d: want both, summing to full", r, b)
	}
	c.drive()
	if s.Switching() {
		t.Error("still switching after the duration")
	}
	if px := paintStack(s); blueAt(px, 30, 30) != 255 || redAt(px, 30, 30) != 0 {
		t.Error("the crossfade did not land on the new page")
	}
}

func TestStackSlideFollowsPageOrder(t *testing.T) {
	c := pinAnimClock(t)
	s := stackAt(StackSlideLeftRight)
	s.Show("b") // a later page comes in from the right
	advance(c, 30*time.Millisecond)
	px := paintStack(s)
	if redAt(px, 21, 30) != 255 || blueAt(px, 58, 30) != 255 {
		t.Errorf("forward slide: left red %d, right blue %d", redAt(px, 21, 30), blueAt(px, 58, 30))
	}
	if got := lit(px, render.Rect{X: 60, Y: 20, W: 20, H: 20}); got != 0 {
		t.Errorf("%d px painted outside the stack", got)
	}
	c.drive()
	s.Show("a") // an earlier page comes in from the left
	advance(c, 30*time.Millisecond)
	px = paintStack(s)
	if redAt(px, 21, 30) != 255 || blueAt(px, 58, 30) != 255 {
		t.Errorf("backward slide: left red %d, right blue %d", redAt(px, 21, 30), blueAt(px, 58, 30))
	}
	s.Remove("b")
	if s.Switching() {
		t.Error("removing the leaving page left the switch running on nothing")
	}
}
