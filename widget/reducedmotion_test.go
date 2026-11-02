package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/surfx"
)

func TestReducedMotionStopsMotionAndKeepsFades(t *testing.T) {
	t.Cleanup(func() { SetReducedMotion(false) })
	theme := DarkTheme().WithAnimations(true)
	if !theme.Moves() || anim.Instant() {
		t.Fatalf("moves %v, instant %v before the preference", theme.Moves(), anim.Instant())
	}
	moving := surfx.Plan(surfx.KindToast, theme.Moves())
	SetReducedMotion(true)
	if theme.Moves() {
		t.Error("a theme with Animations still moves")
	}
	if short, medium, long := theme.MotionDurations(); short != 0 || medium != 0 || long != 0 {
		t.Errorf("motion tokens %v %v %v", short, medium, long)
	}
	if !anim.Instant() {
		t.Error("springs and spinners keep moving")
	}
	if anim.NoFades() {
		t.Error("fades collapse with motion")
	}
	if !envNow().Media.ReducedMotion {
		t.Error("prefers-reduced-motion does not match")
	}
	plan := surfx.Plan(surfx.KindToast, theme.Moves())
	if plan.Enter.Duration <= 0 || plan.Enter.Duration > 500*time.Millisecond || plan.Enter.Spring != nil {
		t.Errorf("reduced enter %+v, want a short plain fade", plan.Enter)
	}
	if plan.Morph {
		t.Error("a surface still grows out of its anchor")
	}
	if moving.Enter.Spring == nil {
		t.Error("a toast did not spring to begin with")
	}
	SetReducedMotion(false)
	if !theme.Moves() || anim.Instant() {
		t.Errorf("moves %v, instant %v after the preference went", theme.Moves(), anim.Instant())
	}
}

func TestNoAnimCollapsesFadesToo(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	if plan := surfx.Plan(surfx.KindToast, true); plan.Enter.Duration != 0 || plan.Enter.Spring != nil {
		t.Errorf("enter %+v under LYS_UI_NO_ANIM", plan.Enter)
	}
}

func TestReducedMotionTurnsSlidesIntoFades(t *testing.T) {
	t.Cleanup(func() { SetReducedMotion(false) })
	r := NewRevealer(NewStack())
	r.SetTransition(RevealSlideDown)
	s := NewStack()
	s.SetTransition(StackSlideLeftRight, time.Second)
	if r.shownTransition() != RevealSlideDown || s.shownTransition() != StackSlideLeftRight {
		t.Fatalf("before the preference: %v, %v", r.shownTransition(), s.shownTransition())
	}
	SetReducedMotion(true)
	if r.shownTransition() != RevealFade || r.Transition() != RevealSlideDown {
		t.Errorf("revealer draws %v, reports %v", r.shownTransition(), r.Transition())
	}
	if s.shownTransition() != StackCrossfade {
		t.Errorf("stack draws %v", s.shownTransition())
	}
	none := NewRevealer(NewStack())
	none.SetTransition(RevealNone)
	if none.shownTransition() != RevealNone {
		t.Errorf("RevealNone draws %v", none.shownTransition())
	}
	SetReducedMotion(false)
	if r.shownTransition() != RevealSlideDown {
		t.Errorf("after the preference: %v", r.shownTransition())
	}
}
