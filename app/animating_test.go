package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/widget"
)

func idleSecondApp(t *testing.T) (*Application, *paintHarness) {
	t.Helper()
	second := idleBudgetApp(t)
	idle := newPaintHarness(widget.NewBox(widget.Row, 0, 0), 200, 100)
	idle.wnd.anim = &second.anim
	idle.frame()
	second.windows = []*hostWindow{idle.wnd}
	return second, idle
}

func assertNothingStepped(t *testing.T, second *Application, idle *paintHarness, steps *int) {
	t.Helper()
	stepped, gen, commits := *steps, widget.DamageGeneration(), idle.surf.commits
	now := time.Now()
	for range 10 {
		now = now.Add(anim.FrameInterval)
		if err := second.tick(second.step, now); err != nil {
			t.Fatal(err)
		}
	}
	if *steps != stepped {
		t.Errorf("the second application stepped the first one's animation %d times", *steps-stepped)
	}
	if got := widget.DamageGeneration(); got != gen {
		t.Errorf("damage generation moved %d -> %d with nothing the second application started", gen, got)
	}
	if drawn := idle.surf.commits - commits; drawn != 0 {
		t.Errorf("the second application's idle window drew %d frames", drawn)
	}
	if second.Animating() {
		t.Error("the second application reports an animation it never started")
	}
}

func TestAQuitApplicationsAnimationsStepNothingInTheNext(t *testing.T) {
	sess, _ := deadPeerSession(t)
	first, wnd, h := disconnectApp(t, sess)
	fading := widget.NewBox(widget.Row, 0, 0)
	wnd.router.Root.(*widget.Box).Append(fading, false)
	h.frame()
	steps := 0
	anim.Start(fading, time.Minute, func(float64) {
		steps++
		fading.Invalidate()
	})
	if !first.Animating() {
		t.Fatal("an animation of a widget the first application shows is not its own")
	}
	if err := first.Run(); err == nil {
		t.Fatal("Run over a dead socket returned no error")
	}
	if first.Animating() {
		t.Error("the first application still animates after Run returned")
	}
	second, idle := idleSecondApp(t)
	assertNothingStepped(t, second, idle, &steps)
}

func TestAClosedApplicationsWidgetLaunchesNothingOnTheNext(t *testing.T) {
	for name, closeWindow := range map[string]bool{
		"its window still open": false,
		"its window released":   true,
	} {
		t.Run(name, func(t *testing.T) {
			first := idleBudgetApp(t)
			late := widget.NewBox(widget.Row, 0, 0)
			root := widget.NewBox(widget.Column, 0, 0).Append(late, false)
			h := newPaintHarness(root, 200, 100)
			h.wnd.anim = &first.anim
			first.windows = append(first.windows, h.wnd)
			h.frame()
			first.Close()
			if closeWindow {
				h.wnd.release()
			}

			steps := 0
			anim.Start(late, time.Minute, func(float64) {
				steps++
				late.Invalidate()
			})
			anim.NewSpring(late, anim.SpringSnappy, []float64{0}, func([]float64) {
				steps++
				late.Invalidate()
			}).SetTarget(1)
			if first.Animating() {
				t.Error("a widget of the closed application launched onto its clock")
			}

			second, idle := idleSecondApp(t)
			assertNothingStepped(t, second, idle, &steps)
		})
	}
}

func TestAWidgetBuiltBeforeItsApplicationAnimatesOnceItsWindowShowsIt(t *testing.T) {
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	early := widget.NewBox(widget.Row, 0, 0)
	root := widget.NewBox(widget.Column, 0, 0).Append(early, false)
	landed := 0.0
	anim.Play(early, anim.Animate(100*time.Millisecond, func(p float64) { landed = p }).Easing(anim.Linear))

	a := idleBudgetApp(t)
	if a.Animating() {
		t.Fatal("the application took an animation of a widget no window of its shows")
	}
	h := newPaintHarness(root, 200, 100)
	h.wnd.anim = &a.anim
	a.windows = []*hostWindow{h.wnd}
	if err := a.tick(a.step, now); err != nil {
		t.Fatal(err)
	}
	if !a.Animating() || landed != 0 {
		t.Fatalf("after the window's first frame: Animating = %v at progress %v, want the animation started from 0", a.Animating(), landed)
	}
	frames := 0
	for ; a.Animating() && frames < 100; frames++ {
		now = now.Add(anim.FrameInterval)
		if err := a.tick(a.step, now); err != nil {
			t.Fatal(err)
		}
	}
	if landed != 1 {
		t.Errorf("after %d frames the animation stands at %v, want it landed", frames, landed)
	}
	if frames < 6 {
		t.Errorf("the animation ran %d frames, want its whole 100ms from the window's first frame", frames)
	}
}

func TestAnimatingHoldsForTheAnimationAndNoLonger(t *testing.T) {
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	a := idleBudgetApp(t)
	if a.Animating() {
		t.Fatal("a fresh application reports an animation")
	}
	landed := 0.0
	anim.Play(a, anim.Animate(100*time.Millisecond, func(p float64) { landed = p }).Easing(anim.Linear))
	if !a.Animating() {
		t.Fatal("Animating is false with a tween just launched")
	}
	frames := 0
	for ; a.Animating() && frames < 100; frames++ {
		now = now.Add(anim.FrameInterval)
		if err := a.tick(a.step, now); err != nil {
			t.Fatal(err)
		}
		if a.Animating() != (landed < 1) {
			t.Fatalf("frame %d: Animating = %v at progress %v", frames, a.Animating(), landed)
		}
	}
	if a.Animating() || landed != 1 {
		t.Fatalf("after %d frames: Animating = %v, progress %v; want the tween landed and nothing running", frames, a.Animating(), landed)
	}
	if frames < 2 {
		t.Errorf("the tween ended in %d frame(s), want it running between launch and end", frames)
	}
}

func TestClosingAnApplicationEndsItsAnimations(t *testing.T) {
	a := idleBudgetApp(t)
	ran := false
	anim.Start(a, time.Minute, func(float64) { ran = true })
	a.Close()
	if a.Animating() {
		t.Error("a closed application still animates")
	}
	anim.Start(a, time.Minute, func(float64) { ran = true })
	if a.Animating() {
		t.Error("a closed application took a later launch")
	}
	a.anim.Tick(time.Now().Add(time.Second))
	if ran {
		t.Error("a closed application's tween ran")
	}
}

func TestClosingAnEarlierApplicationLeavesTheLaterOnesAnimations(t *testing.T) {
	first := idleBudgetApp(t)
	second := idleBudgetApp(t)
	anim.Start(second, time.Minute, func(float64) {})
	first.Close()
	if !second.Animating() {
		t.Fatal("closing the first application ended the second one's animation")
	}
	if first.Animating() {
		t.Error("the second application's launch landed on the first")
	}
}
