package anim

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

func linearPiece(at, dur time.Duration, fn func(float64)) []piece {
	return []piece{{at: at, dur: dur, fn: fn, easing: Linear}}
}

func TestTickKeepsNoDriverReachableFromItsSnapshot(t *testing.T) {
	var c Clock
	ticks := 0
	cancel := c.drive(func(time.Time) bool {
		ticks++
		return ticks < 2
	})
	defer cancel()
	now := time.Unix(100, 0)
	if !c.Tick(now) {
		t.Fatal("a live driver did not run")
	}
	if !c.Active() {
		t.Error("a driver that asked for another frame stopped")
	}
	c.Tick(now.Add(time.Millisecond))
	if ticks != 2 {
		t.Errorf("driver ran %d times, want 2", ticks)
	}
	if c.Active() {
		t.Error("a settled driver is still active")
	}
	for i, d := range c.running[:cap(c.running)] {
		if d != nil {
			t.Errorf("snapshot slot %d still holds a driver after the tick", i)
		}
	}
}

func TestTickTimeNeverRunsBack(t *testing.T) {
	base := time.Unix(1000, 0)
	pinClock(t, base)
	var c Clock
	var seen []float64
	c.launch(linearPiece(0, time.Second, func(p float64) { seen = append(seen, p) }))
	c.Tick(base.Add(500 * time.Millisecond))
	c.Tick(base.Add(400 * time.Millisecond))
	if len(seen) != 2 || seen[1] < seen[0] {
		t.Fatalf("progress ran back on an older instant: %v", seen)
	}
}

func TestAPieceFiresOnAFrameTimestampBehindItsLaunch(t *testing.T) {
	base := time.Unix(1000, 0)
	pinClock(t, base)
	var c Clock
	fired := false
	c.launch(linearPiece(0, time.Second, func(float64) { fired = true }))
	c.Tick(base.Add(-5 * time.Millisecond))
	if !fired {
		t.Fatal("a tick with a timestamp just behind the launch left the piece unstarted")
	}
}

func TestPaceSetsTheDeadline(t *testing.T) {
	base := time.Unix(1000, 0)
	pinClock(t, base)
	var c Clock
	c.launch(linearPiece(0, 10*time.Second, func(float64) {}))
	c.Tick(base)
	if at, ok := c.Next(PaceTimer); !ok || !at.Equal(base.Add(FrameInterval)) {
		t.Errorf("Next(PaceTimer) = %v %v, want a frame on", at, ok)
	}
	if at, ok := c.Next(PaceFallback); !ok || !at.Equal(base.Add(FallbackInterval)) {
		t.Errorf("Next(PaceFallback) = %v %v, want the fallback", at, ok)
	}
	if c.Due(base.Add(FrameInterval/2), PaceTimer) {
		t.Error("due before a frame passed")
	}
	if !c.Due(base.Add(FrameInterval), PaceTimer) {
		t.Error("not due a frame on at PaceTimer")
	}
	if c.Due(base.Add(FrameInterval), PaceFallback) {
		t.Error("due a frame on while a callback paces the clock")
	}
	if !c.Due(base.Add(FallbackInterval), PaceFallback) {
		t.Error("the fallback did not come due")
	}
}

func TestADelayedPieceWakesAtItsStartWhateverThePace(t *testing.T) {
	base := time.Unix(1000, 0)
	pinClock(t, base)
	var c Clock
	c.launch(linearPiece(5*time.Second, time.Second, func(float64) {}))
	c.Tick(base)
	for _, pace := range []Pace{PaceTimer, PaceFallback} {
		if at, ok := c.Next(pace); !ok || !at.Equal(base.Add(5*time.Second)) {
			t.Errorf("pace %d: Next = %v %v, want the start", pace, at, ok)
		}
	}
}

func TestAStoppedClockDropsWhatRunsAndRefusesLaterLaunches(t *testing.T) {
	var c Clock
	fired := 0
	Start(&c, time.Second, func(float64) { fired++ })
	Drive(&c, func(time.Time) bool { fired++; return true })
	if !c.Active() {
		t.Fatal("what was launched on the clock does not run on it")
	}
	c.Stop()
	if c.Active() {
		t.Error("the stopped clock still runs what was launched on it")
	}
	Start(&c, time.Second, func(float64) { fired++ })
	NewSpring(&c, SpringSnappy, []float64{0}, func([]float64) { fired++ }).SetTarget(1)
	if c.Active() {
		t.Error("the stopped clock took a later launch")
	}
	c.Tick(animclock.Now().Add(time.Hour))
	if fired != 0 {
		t.Errorf("%d callbacks ran on a stopped clock", fired)
	}
}

type heldOwner struct{ held []*Launch }

func (o *heldOwner) ScheduleAnimation(l *Launch) { o.held = append(o.held, l) }

func TestAnAnimationWaitsForItsOwnersClockAndStartsFromThere(t *testing.T) {
	base := time.Unix(1000, 0)
	now := base
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	var owner heldOwner
	var c Clock
	var seen []float64
	Play(&owner, Animate(time.Second, func(p float64) { seen = append(seen, p) }).Easing(Linear))
	if len(owner.held) != 1 || c.Active() {
		t.Fatalf("the owner holds %d launches and the clock runs %v, want the launch held off every clock", len(owner.held), c.Active())
	}
	now = base.Add(time.Minute)
	owner.held[0].Start(&c)
	owner.held[0].Start(&c)
	c.Tick(now.Add(500 * time.Millisecond))
	if len(seen) != 1 || seen[0] != 0.5 {
		t.Errorf("progress %v half a second after the owner started it, want one step at 0.5", seen)
	}
}

func TestACancelledLaunchNeverStarts(t *testing.T) {
	var owner heldOwner
	var c Clock
	cancel := Start(&owner, time.Second, func(float64) {})
	cancel()
	if !owner.held[0].Cancelled() {
		t.Error("the cancelled launch does not report it")
	}
	owner.held[0].Start(&c)
	if c.Active() {
		t.Error("a launch cancelled while it waited started on the clock")
	}
}

func TestAnInstantStepLandsWithoutTheOwnersClock(t *testing.T) {
	var owner heldOwner
	landed := 0.0
	Play(&owner, Animate(0, func(p float64) { landed = p }))
	if landed != 1 || len(owner.held) != 0 {
		t.Errorf("an instant step landed at %v with %d launches handed to the owner, want 1 inside Play and none", landed, len(owner.held))
	}
}
