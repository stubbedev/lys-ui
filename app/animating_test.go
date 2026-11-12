package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/widget"
)

func TestAQuitApplicationsAnimationsStepNothingInTheNext(t *testing.T) {
	sess, _ := deadPeerSession(t)
	first, wnd, _ := disconnectApp(t, sess)
	fading := widget.NewBox(widget.Row, 0, 0)
	wnd.router.Root.(*widget.Box).Append(fading, false)
	steps := 0
	anim.Start(time.Minute, func(float64) {
		steps++
		fading.Invalidate()
	})
	if !first.Animating() {
		t.Fatal("an animation launched under the first application is not its own")
	}
	if err := first.Run(); err == nil {
		t.Fatal("Run over a dead socket returned no error")
	}
	if first.Animating() {
		t.Error("the first application still animates after Run returned")
	}

	second := idleBudgetApp(t)
	idle := newPaintHarness(widget.NewBox(widget.Row, 0, 0), 200, 100)
	idle.wnd.anim = &second.anim
	idle.frame()
	second.windows = []*hostWindow{idle.wnd}
	stepped, gen, commits := steps, widget.DamageGeneration(), idle.surf.commits
	now := time.Now()
	for range 10 {
		now = now.Add(animclock.FrameInterval)
		if err := second.tick(second.step, now); err != nil {
			t.Fatal(err)
		}
	}
	if steps != stepped {
		t.Errorf("the second application stepped the first one's animation %d times", steps-stepped)
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

func TestAnimatingHoldsForTheAnimationAndNoLonger(t *testing.T) {
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	a := idleBudgetApp(t)
	if a.Animating() {
		t.Fatal("a fresh application reports an animation")
	}
	landed := 0.0
	anim.Play(anim.Animate(100*time.Millisecond, func(p float64) { landed = p }).Easing(anim.Linear))
	if !a.Animating() {
		t.Fatal("Animating is false with a tween just launched")
	}
	frames := 0
	for ; a.Animating() && frames < 100; frames++ {
		now = now.Add(animclock.FrameInterval)
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
	anim.Start(time.Minute, func(float64) { ran = true })
	a.Close()
	if a.Animating() {
		t.Error("a closed application still animates")
	}
	animclock.Tick(time.Now().Add(time.Second))
	if ran {
		t.Error("a closed application's tween ran on the clock launches fall back to")
	}
}

func TestClosingAnEarlierApplicationLeavesTheLaterOnesAnimations(t *testing.T) {
	first := idleBudgetApp(t)
	second := idleBudgetApp(t)
	anim.Start(time.Minute, func(float64) {})
	first.Close()
	if !second.Animating() {
		t.Fatal("closing the first application ended the second one's animation")
	}
	anim.Start(time.Minute, func(float64) {})
	if first.Animating() {
		t.Error("a launch after the first application closed landed on it")
	}
}

func TestAnAnimationLaunchedBeforeItsApplicationRunsOnIt(t *testing.T) {
	animclock.Reset()
	landed := false
	anim.Start(100*time.Millisecond, func(p float64) { landed = p == 1 })
	a := idleBudgetApp(t)
	if !a.Animating() {
		t.Fatal("the application did not take an animation launched before it")
	}
	now := time.Now()
	for i := 0; a.Animating() && i < 100; i++ {
		now = now.Add(animclock.FrameInterval)
		if err := a.tick(a.step, now); err != nil {
			t.Fatal(err)
		}
	}
	if !landed {
		t.Error("the animation launched before the application never landed")
	}
}
