package anim

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// TestInstantKeepsDelayedLandings pins the reduced-motion contract for
// timing skeletons: a tween behind a delay lands once, on schedule,
// even when instant mode collapses its duration — only the motion goes
// away, never the wait.
func TestInstantKeepsDelayedLandings(t *testing.T) {
	restore := SetInstant(true)
	defer restore()
	t0 := time.Unix(1750000000, 0)
	pinClock(t, t0)
	animclock.Reset()
	fired := 0
	cancel := Play(Sequence(
		Delay(50*time.Millisecond),
		Animate(0, func(p float64) {
			if p >= 1 {
				fired++
			}
		}),
	))
	defer cancel()
	if fired != 0 {
		t.Fatalf("fired %d times at launch, want 0", fired)
	}
	animclock.Tick(t0.Add(20 * time.Millisecond)) // mid-delay: nothing lands
	if fired != 0 {
		t.Fatalf("fired %d times mid-delay, want 0", fired)
	}
	step := func() bool {
		wake, ok := animclock.Next(animclock.Timer)
		if !ok {
			return false
		}
		animclock.Tick(wake)
		return true
	}
	for range 10 {
		if !step() {
			break
		}
	}
	if fired != 1 {
		t.Errorf("fired %d times after the delay, want exactly 1", fired)
	}
	if _, ok := animclock.Next(animclock.Timer); ok {
		t.Error("the landed step kept scheduling")
	}
}
