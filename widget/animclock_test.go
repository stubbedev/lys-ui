package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// animClock is the injected animation clock the anim-driven widget
// tests share: launches and deadlines read a test-controlled instant,
// and ticks take their instants from the same hand, so schedules are
// exact (the anim package's own pinClock pattern, driven from outside
// the package through animclock.SetClock).
type animClock struct {
	cur time.Time
}

// pinAnimClock stops the animation clock at a fixed epoch and clears
// the tween list for the test's run.
func pinAnimClock(t *testing.T) *animClock {
	t.Helper()
	c := &animClock{cur: time.Unix(1750000000, 0)}
	t.Cleanup(animclock.SetClock(func() time.Time { return c.cur }))
	t.Cleanup(animclock.Reset)
	animclock.Reset()
	return c
}

// now returns the current test instant.
func (c *animClock) now() time.Time { return c.cur }

// set moves the test instant; the next launch lands on it.
func (c *animClock) set(tm time.Time) { c.cur = tm }

// step advances one animation wake — exactly one frame period past
// the last tick for an in-flight tween, a later step's start across a
// delay — and ticks the clock there. False when nothing is scheduled.
func (c *animClock) step() bool {
	wake, ok := animclock.Next(animclock.Timer)
	if !ok {
		return false
	}
	c.cur = wake
	animclock.Tick(wake)
	return true
}

// drive drains the schedule: every tween runs to its end. Bound so a
// misbehaving test fails instead of hanging; a spinner's long-horizon
// rotation never drains and must be stepped by hand.
func (c *animClock) drive() {
	for range 100000 {
		if !c.step() {
			return
		}
	}
	panic("animation schedule did not drain")
}
