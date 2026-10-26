package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/widget"
)

// repNext adapts the repeater's deadline for nextWake, collapsing the
// not-held case to the zero time.
func repNext(rep *keyRepeater) time.Time {
	t, ok := rep.nextDeadline()
	if !ok {
		return time.Time{}
	}
	return t
}

// animNext adapts animclock.Next the same way.
func animNext() time.Time {
	t, ok := animclock.Next()
	if !ok {
		return time.Time{}
	}
	return t
}

func TestNextWake(t *testing.T) {
	// The clock is global: tweens other tests left running (a switch's
	// slide) must not count as this loop's (#323).
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	never := time.Time{}
	now := time.Now()

	t.Run("a fully idle loop never wakes", func(t *testing.T) {
		if _, ok := nextWake(repNext(newKeyRepeater(0, 0)), animNext(), never, never, now); ok {
			t.Errorf("idle loop scheduled a wakeup")
		}
	})

	t.Run("a held key wakes at its repeat deadline", func(t *testing.T) {
		rep := newKeyRepeater(0, 0)
		rep.press(30, 0)
		wake, ok := nextWake(repNext(rep), animNext(), never, never, now)
		if !ok {
			t.Fatal("held key did not schedule a wakeup")
		}
		if d := time.Until(wake); d < 300*time.Millisecond || d > 500*time.Millisecond {
			t.Errorf("wake in %v, want ~400ms repeat delay", d)
		}
	})

	t.Run("a released key stops waking", func(t *testing.T) {
		rep := newKeyRepeater(0, 0)
		rep.press(30, 0)
		rep.release(30)
		if _, ok := nextWake(repNext(rep), animNext(), never, never, now); ok {
			t.Errorf("released key still scheduled wakeups")
		}
	})

	t.Run("a running animation wakes at its end", func(t *testing.T) {
		animclock.Reset()
		anim.Start(250*time.Millisecond, func(float64) {})
		wake, ok := nextWake(repNext(newKeyRepeater(0, 0)), animNext(), never, never, now)
		if !ok {
			t.Fatal("animation did not schedule a wakeup")
		}
		if d := time.Until(wake); d > 260*time.Millisecond {
			t.Errorf("wake in %v, want ~250ms tween end", d)
		}
	})

	t.Run("the earliest deadline wins", func(t *testing.T) {
		animclock.Reset()
		anim.Start(20*time.Millisecond, func(float64) {})
		rep := newKeyRepeater(0, 0)
		rep.press(30, 0)
		wake, ok := nextWake(repNext(rep), animNext(), never, never, now)
		if !ok {
			t.Fatal("no wakeup scheduled")
		}
		if d := time.Until(wake); d > 60*time.Millisecond {
			t.Errorf("wake in %v, want the ~20ms animation deadline", d)
		}
	})

	t.Run("already-due deadlines do not wake", func(t *testing.T) {
		if _, ok := nextWake(never, never, never, now.Add(-time.Second), now); ok {
			t.Errorf("past deadline scheduled a zero-duration wake")
		}
	})
}

// TestIdleLoopDoesNotWake drives the loop's wake decision through a
// 200ms idle window: with no key held, no animation, and no tooltip
// pending, zero wakeups may be scheduled. This is the acceptance hook
// for the parked event loop; the pre-park loop burned a core here.
func TestIdleLoopDoesNotWake(t *testing.T) {
	animclock.Reset()
	rep := newKeyRepeater(0, 0)
	wakes := 0
	start := time.Now()
	for time.Since(start) < 200*time.Millisecond {
		if _, ok := nextWake(repNext(rep), animNext(), time.Time{}, time.Time{}, time.Now()); ok {
			wakes++
		}
		// A real loop parks in Session.Step here, waking on events.
	}
	if wakes != 0 {
		t.Errorf("idle window scheduled %d wakeups, want 0", wakes)
	}
}

// TestKeyRepeatLatency pins responsiveness: a press routes immediately
// (the key event itself wakes the loop) and repeats land at the
// compositor's rate afterwards.
func TestKeyRepeatLatency(t *testing.T) {
	rep := newKeyRepeater(10, 100)
	rep.press(30, 0)

	presses := 0
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, _, ok := rep.tick(); ok {
			presses++
		}
		time.Sleep(10 * time.Millisecond)
	}
	if presses < 1 || presses > 3 {
		t.Errorf("repeats in 250ms = %d, want 1-2 at 10/s", presses)
	}
}

// TestShouldDraw pins the frame-pacing gate, especially the resize
// bypass: a configure-driven repaint must go out even with a frame
// still pending, because compositors withhold that callback until the
// surface commits at the configured size - gating it deadlocks the
// resize. The frameOwed rules underneath are pinned in TestFrameOwed.
func TestShouldDraw(t *testing.T) {
	t0 := time.Now()
	t.Run("an idle window never draws", func(t *testing.T) {
		if shouldDraw(&hostWindow{}, false, t0, false) {
			t.Error("undirtied window drew")
		}
	})
	t.Run("a dirty window waits for its frame callback", func(t *testing.T) {
		w := &hostWindow{dirty: true, framePending: true, frameArmedAt: t0}
		if shouldDraw(w, false, t0, false) {
			t.Error("drew while the previous frame was pending")
		}
	})
	t.Run("a returned callback lets the frame go out", func(t *testing.T) {
		if !shouldDraw(&hostWindow{dirty: true}, false, t0, false) {
			t.Error("pacing blocked a ready frame")
		}
	})
	t.Run("a stale animation frame goes out", func(t *testing.T) {
		w := &hostWindow{dirty: true, framePending: true, frameArmedAt: t0}
		if !shouldDraw(w, true, t0.Add(frameStaleAfter), false) {
			t.Error("occluded animation stalled")
		}
	})
	t.Run("a configure resize bypasses pacing", func(t *testing.T) {
		w := &hostWindow{dirty: true, framePending: true, frameArmedAt: t0}
		if !shouldDraw(w, false, t0, true) {
			t.Error("resize repaint blocked by pacing; the resize would deadlock")
		}
	})
}

// TestLoopKickerCoalesces pins the wakeup budget: one outstanding kick
// covers every deadline it spans, and a fired kick rearms. The
// animation loop passes here once per frame - a kicker that re-armed on
// covered deadlines would queue a sync per pass and never park.
func TestLoopKickerCoalesces(t *testing.T) {
	k := &loopKicker{}
	now := time.Now()
	deadline := now.Add(16 * time.Millisecond)

	if k.covers(deadline, now) {
		t.Error("a fresh kicker claims a pending kick")
	}
	k.until = deadline
	if !k.covers(deadline, now) {
		t.Error("the pending kick does not cover its own deadline")
	}
	if !k.covers(deadline.Add(-time.Millisecond), now) {
		t.Error("a deadline 1ms before the pending kick is treated as uncovered")
	}
	if k.covers(deadline.Add(3*time.Millisecond), now) {
		t.Error("a deadline 3ms past the pending kick counts as covered")
	}
	if k.covers(deadline, deadline.Add(time.Millisecond)) {
		t.Error("a fired kick still counts as pending")
	}
}

// TestAnimatingLoopWakesOnFrameDeadlines drives the loop's wake
// decision across a tween's lifetime: an animating tree wakes once per
// animation frame period, each tick rolls the deadline forward, and
// when the tween ends the loop parks again - the acceptance half of
// the timer-driven animation clock.
func TestAnimatingLoopWakesOnFrameDeadlines(t *testing.T) {
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	rep := newKeyRepeater(0, 0)
	never := time.Time{}

	if _, ok := nextWake(repNext(rep), animNext(), never, never, time.Now()); ok {
		t.Fatal("static tree scheduled a wakeup")
	}

	anim.Start(70*time.Millisecond, func(float64) {})
	now := time.Now()
	wakes := 0
	for animclock.Active() {
		wake, ok := nextWake(repNext(rep), animNext(), never, never, now)
		if !ok {
			t.Fatal("animating tree did not schedule a wake")
		}
		if d := wake.Sub(now); d <= 0 || d > animclock.FrameInterval {
			t.Fatalf("wake in %v, want one animation frame period (%v) or less", d, animclock.FrameInterval)
		}
		wakes++
		now = wake
		animclock.Tick(now) // the loop ticks its animation clock on the wake
	}
	if wakes < 3 {
		t.Errorf("wakes over a 70ms tween = %d, want the ~%v frame cadence", wakes, animclock.FrameInterval)
	}
	if _, ok := nextWake(repNext(rep), animNext(), never, never, now); ok {
		t.Error("finished tween still scheduled wakeups")
	}
}

// TestFrameOwed pins the loop's frame gate: a frame goes out when the
// compositor answered the previous one, and an animation pushes one
// through only once its callback has gone unheard past the staleness
// bound - the occluded case the animation timer covers.
func TestFrameOwed(t *testing.T) {
	t0 := time.Now()
	w := &hostWindow{framePending: true, frameArmedAt: t0}

	if !(&hostWindow{}).frameOwed(true, t0) {
		t.Error("window without a pending frame was not owed one")
	}
	if w.frameOwed(false, t0.Add(time.Second)) {
		t.Error("pending frame drawn past without the callback")
	}
	if w.frameOwed(true, t0.Add(animclock.FrameInterval)) {
		t.Error("animation drew over a fresh pending frame")
	}
	if !w.frameOwed(true, t0.Add(frameStaleAfter)) {
		t.Error("animation did not take over after the callback went stale")
	}
}

func TestTooltipNext(t *testing.T) {
	target := newTipTarget("hover text")
	target.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
	router := &widget.Router{Root: target}

	t.Run("pending hover schedules its delay", func(t *testing.T) {
		tip := &tooltipCtl{since: time.Now()}
		router.Move(widget.Point{X: 5, Y: 5})
		tip.update(router, time.Now(), func(widget.Widget, string) (tooltipWindow, tooltipPainter) { return nil, nil })
		wake, ok := tip.next()
		if !ok {
			t.Fatal("pending tooltip did not schedule a wakeup")
		}
		if d := time.Until(wake); d < 300*time.Millisecond || d > 500*time.Millisecond {
			t.Errorf("wake in %v, want ~500ms tooltip delay", d)
		}
	})

	t.Run("no hover never wakes", func(t *testing.T) {
		tip := &tooltipCtl{since: time.Now()}
		if _, ok := tip.next(); ok {
			t.Errorf("tooltip without hover scheduled a wakeup")
		}
	})

	t.Run("failed open backs off a full delay", func(t *testing.T) {
		t0 := time.Now()
		router.Move(widget.Point{X: 5, Y: 5})
		attempts := 0
		tip := &tooltipCtl{}
		tip.update(router, t0.Add(-time.Second), func(widget.Widget, string) (tooltipWindow, tooltipPainter) { return nil, nil })
		// The dwell has elapsed; the opener fails (no tooltip face,
		// compositor rejection). The dwell clock must restart, or every
		// loop iteration retries the open - once a live protocol-error
		// storm against a compositor that kept rejecting the popup.
		tip.update(router, t0, func(widget.Widget, string) (tooltipWindow, tooltipPainter) {
			attempts++
			return nil, nil
		})
		if attempts != 1 {
			t.Fatalf("opener ran %d times, want 1", attempts)
		}
		tip.update(router, t0.Add(10*time.Millisecond), func(widget.Widget, string) (tooltipWindow, tooltipPainter) {
			attempts++
			return nil, nil
		})
		if attempts != 1 {
			t.Errorf("failed open retried after 10ms; want a full-delay back-off")
		}
		if wake, ok := tip.next(); !ok {
			t.Error("backed-off tooltip did not schedule a retry")
		} else if d := wake.Sub(t0); d != tooltipDelay {
			t.Errorf("retry due in %v after the failed attempt, want %v", d, tooltipDelay)
		}
	})
}
