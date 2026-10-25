package widget

import (
	"testing"
	"time"
)

func sizedStack() *Stack {
	return NewStack().Add("short", newStub(40, 50)).Add("tall", newStub(60, 150))
}

func measureH(s *Stack) int { return s.Measure(Constraints{Max: Size{W: 500, H: 500}}).H }

// TestStackSizesToItsVisiblePage pins GTK's homogeneous off: the stack
// measures its visible page, the default its largest, and PageFloor
// the most any page needs either way.
func TestStackSizesToItsVisiblePage(t *testing.T) {
	s := sizedStack()
	if got := measureH(s); got != 150 {
		t.Errorf("homogeneous = %d, want the tallest page's 150", got)
	}
	s.SetHomogeneous(false)
	if got := measureH(s); got != 50 {
		t.Errorf("sized to the visible page = %d, want 50", got)
	}
	s.Show("tall")
	if got := measureH(s); got != 150 {
		t.Errorf("after switching = %d, want 150 (a switch relayouts)", got)
	}
	if got := s.PageFloor(Constraints{Max: Size{W: 500, H: 500}}); got != 150 {
		t.Errorf("PageFloor = %d, want the tallest page's 150", got)
	}
	s.SetHomogeneous(true)
	s.Show("short")
	if got := measureH(s); got != 150 {
		t.Errorf("homogeneous again = %d, want 150", got)
	}
}

// TestStackInterpolatesItsSizeThroughASwitch pins interpolate-size: the
// size tweens between the pages on the switch's easing and lands on
// the new page; without it, it jumps at once.
func TestStackInterpolatesItsSizeThroughASwitch(t *testing.T) {
	c := pinAnimClock(t)
	s := sizedStack()
	s.SetHomogeneous(false)
	s.SetInterpolateSize(true)
	s.SetTransition(StackCrossfade, 200*time.Millisecond)
	measureH(s)
	s.Show("tall")
	c.step()
	c.step()
	if got := measureH(s); got <= 50 || got >= 150 {
		t.Errorf("mid-switch = %d, want between 50 and 150", got)
	}
	c.drive()
	if got := measureH(s); got != 150 {
		t.Errorf("after the switch = %d, want 150", got)
	}

	jump := sizedStack()
	jump.SetHomogeneous(false)
	jump.SetTransition(StackCrossfade, 200*time.Millisecond)
	measureH(jump)
	jump.Show("tall")
	c.step()
	if got := measureH(jump); got != 150 {
		t.Errorf("without interpolate-size = %d mid-switch, want the jump to 150", got)
	}
}

// TestStackFloorsFollowWhatPagesGiveUp pins the floors: a page that
// scrolls needs only the scroll's floor, the visible page's give
// is the stack's (sized to it), and PageFloor is the most any page
// needs.
func TestStackFloorsFollowWhatPagesGiveUp(t *testing.T) {
	s := NewStack().Add("list", propagating(NewScroll(newStub(60, 400)))).Add("form", newStub(60, 150))
	s.SetHomogeneous(false)
	con := Constraints{Max: Size{W: 500, H: 1000}}
	if got := s.Measure(con).H; got != 400 {
		t.Fatalf("list page = %d, want its natural 400", got)
	}
	if got := s.Shrinkable(); got != 400-scrollFloor {
		t.Errorf("list page gives %d, want down to the scroll floor (%d)", got, 400-scrollFloor)
	}
	if got := s.PageFloor(con); got != 150 {
		t.Errorf("PageFloor = %d, want the form's 150 (the list scrolls)", got)
	}
	s.Show("form")
	s.Measure(con)
	if got := s.Shrinkable(); got != 0 {
		t.Errorf("the form gives %d, want 0", got)
	}
	homog := NewStack().Add("list", propagating(NewScroll(newStub(60, 400)))).Add("form", newStub(60, 150))
	homog.Measure(con)
	if got := homog.Shrinkable(); got != 400-150 {
		t.Errorf("homogeneous gives %d, want down to the form's 150", got)
	}
}

// The size settings read back as set.
func TestStackSizeSettingsReadBack(t *testing.T) {
	s := sizedStack()
	if !s.Homogeneous() || s.InterpolateSize() {
		t.Errorf("defaults: homogeneous %v interpolate %v, want true/false", s.Homogeneous(), s.InterpolateSize())
	}
	s.SetHomogeneous(false)
	s.SetInterpolateSize(true)
	if s.Homogeneous() || !s.InterpolateSize() {
		t.Errorf("set: homogeneous %v interpolate %v, want false/true", s.Homogeneous(), s.InterpolateSize())
	}
}
