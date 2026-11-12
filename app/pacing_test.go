package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

type pacingRig struct {
	t      *testing.T
	a      *Application
	h      *paintHarness
	now    time.Time
	steps  int
	stamps []time.Time
	armed  time.Time
	arms   int
}

func newPacingRig(t *testing.T) *pacingRig {
	t.Helper()
	r := &pacingRig{t: t, now: time.Unix(1000, 0)}
	t.Cleanup(animclock.SetClock(func() time.Time { return r.now }))
	area := widget.NewDrawingArea(64, 48)
	r.h = newPaintHarness(area, 64, 48)
	r.h.surf.withholdFrames = true
	r.a = NewTestApplication(t, &wlsession.Session{})
	r.a.step = func() error { return nil }
	r.a.wake = func(time.Duration) {}
	r.a.arm = func(at time.Time) {
		if !at.Equal(r.armed) {
			r.armed = at
			r.arms++
		}
	}
	r.a.windows = []*hostWindow{r.h.wnd}
	r.h.wnd.anim = &r.a.anim
	return r
}

func (r *pacingRig) drive(area *widget.DrawingArea) {
	r.t.Helper()
	cancel := anim.Drive(r.a, func(at time.Time) bool {
		r.steps++
		r.stamps = append(r.stamps, at)
		area.QueueDraw()
		return true
	})
	r.t.Cleanup(cancel)
}

func (r *pacingRig) pass(after time.Duration) {
	r.t.Helper()
	r.now = r.now.Add(after)
	if err := r.a.tick(r.a.step, r.now); err != nil {
		r.t.Fatalf("tick: %v", err)
	}
}

func (r *pacingRig) callback(after time.Duration, ms uint32) {
	r.t.Helper()
	for _, b := range r.h.bufs {
		if b.Busy() {
			b.Release()
		}
	}
	r.h.wnd.frame = wlsession.FrameDone{Ready: true, MS: ms}
	r.pass(after)
}

func (r *pacingRig) area() *widget.DrawingArea {
	return r.h.wnd.router.Root.(*widget.DrawingArea)
}

func TestAnimationTicksOncePerCommitAndArmsOnce(t *testing.T) {
	r := newPacingRig(t)
	r.drive(r.area())
	r.pass(time.Second)
	if !r.h.wnd.framePending {
		t.Fatal("the first pass of an animation committed nothing")
	}
	const frames = 50
	commits, steps, arms := r.h.surf.commits, r.steps, r.arms
	ms := uint32(5000)
	for range frames {
		ms += 16
		r.callback(16*time.Millisecond, ms)
	}
	if got := r.steps - steps; got != frames {
		t.Errorf("%d callbacks stepped the clock %d times, want one each", frames, got)
	}
	if got := r.h.surf.commits - commits; got != frames {
		t.Errorf("%d callbacks made %d commits, want one each", frames, got)
	}
	if got := r.arms - arms; got > frames {
		t.Errorf("%d commits armed %d deadlines, want at most one each", frames, got)
	}
	want := r.now.Add(anim.FallbackInterval)
	if got := r.armed; got.Sub(want) > 20*time.Millisecond || want.Sub(got) > 20*time.Millisecond {
		t.Errorf("deadline armed for %v with a callback pending, want the fallback about %v", got, want)
	}
}

func TestPointerPassesLeaveAnimationsWhereTheyAre(t *testing.T) {
	r := newPacingRig(t)
	r.drive(r.area())
	r.pass(time.Second)
	for name, setup := range map[string]func(){
		"a callback pending":  func() {},
		"no callback pending": func() { r.h.wnd.framePending = false },
	} {
		t.Run(name, func(t *testing.T) {
			setup()
			r.pass(anim.FrameInterval)
			steps, arms := r.steps, r.arms
			for range 5 {
				r.pass(time.Millisecond)
			}
			if got := r.steps - steps; got != 0 {
				t.Errorf("5 motion passes stepped the clock %d times, want 0", got)
			}
			if got := r.arms - arms; got != 0 {
				t.Errorf("5 motion passes armed %d deadlines, want 0", got)
			}
		})
	}
}

func TestOccludedWindowWakesAtTheLowRateAndItsTweenLands(t *testing.T) {
	r := newPacingRig(t)
	r.drive(r.area())
	r.pass(time.Second)
	steps, arms := r.steps, r.arms
	landed := false
	anim.Play(r.a, anim.Animate(600*time.Millisecond, func(p float64) { landed = p >= 1 }).Easing(anim.Linear))
	const wakes = 16
	for range wakes {
		r.pass(r.armed.Sub(r.now))
	}
	if !landed {
		t.Error("a tween on a window the compositor withholds never landed")
	}
	if want := wakes; r.steps-steps != want {
		t.Errorf("%d armed wakes stepped the clock %d times", want, r.steps-steps)
	}
	if got := r.arms - arms; got != wakes {
		t.Errorf("%d wakes armed %d deadlines", wakes, got)
	}
	if got := r.now.Sub(time.Unix(1000, 0).Add(time.Second)); got < 3*time.Second {
		t.Errorf("%d wakes took only %v, want the low rate of %v each", wakes, got, anim.FallbackInterval)
	}
}

func TestAnimationWithNoWindowLandsAtTheLowRate(t *testing.T) {
	r := newPacingRig(t)
	r.a.holds = 1
	r.a.windows = nil
	landed := false
	anim.Play(r.a, anim.Animate(400*time.Millisecond, func(p float64) { landed = p >= 1 }).Easing(anim.Linear))
	r.pass(time.Second)
	for wakes := 0; !landed; wakes++ {
		if wakes > 4 {
			t.Fatalf("a tween with no surface took more than %d wakes", wakes)
		}
		if r.armed.IsZero() {
			t.Fatal("a running tween armed no wake")
		}
		r.pass(r.armed.Sub(r.now))
	}
	if r.a.Animating() {
		t.Error("the landed tween stayed active")
	}
}

func TestAnimationTimeIsTheCompositorsAndNeverRunsBack(t *testing.T) {
	r := newPacingRig(t)
	r.drive(r.area())
	r.pass(time.Second)
	r.callback(16*time.Millisecond, 7000)
	anchored := r.stamps[len(r.stamps)-1]
	r.callback(20*time.Millisecond, 7016)
	if got, want := r.stamps[len(r.stamps)-1], anchored.Add(16*time.Millisecond); !got.Equal(want) {
		t.Errorf("tick at %v, want the callback's timestamp %v, not the wake's", got, want)
	}
	r.h.wnd.framePending = true
	r.pass(anim.FallbackInterval)
	r.callback(time.Millisecond, 7017)
	for i := 1; i < len(r.stamps); i++ {
		if r.stamps[i].Before(r.stamps[i-1]) {
			t.Fatalf("animation time ran back at step %d: %v after %v", i, r.stamps[i], r.stamps[i-1])
		}
	}
}
