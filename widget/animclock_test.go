package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
)

// animClock is the injected animation clock the anim-driven widget
// tests share: launches and deadlines read a test-controlled instant,
// and ticks take their instants from the same hand, so schedules are
// exact. Its clock runs what the trees hosted on it launch.
type animClock struct {
	cur   time.Time
	clock anim.Clock
}

// pinAnimClock stops the animation time at a fixed epoch and returns a
// fresh clock for the test's run.
func pinAnimClock(t *testing.T) *animClock {
	t.Helper()
	c := &animClock{cur: time.Unix(1750000000, 0)}
	t.Cleanup(animclock.SetClock(func() time.Time { return c.cur }))
	return c
}

func (c *animClock) host(t *testing.T, root Widget) {
	t.Helper()
	t.Cleanup(Host(root, &c.clock))
}

func (c *animClock) frame(t *testing.T, root Widget, w, h int) {
	t.Helper()
	frameOn(t, &c.clock, root, w, h)
}

func (c *animClock) arrangeTree(t *testing.T, root Widget, w, h int) {
	t.Helper()
	arrangeTreeOn(t, &c.clock, root, w, h)
}

func (c *animClock) scheduled() bool {
	if c.clock.Active() {
		return true
	}
	_, ok := c.clock.Next(anim.PaceTimer)
	return ok
}

func (c *animClock) hostTree(t *testing.T, w Widget) {
	t.Helper()
	for p := parentOf(w); p != nil; p = parentOf(p) {
		w = p
	}
	c.host(t, w)
}

// now returns the current test instant.
func (c *animClock) now() time.Time { return c.cur }

// set moves the test instant; the next launch lands on it.
func (c *animClock) set(tm time.Time) { c.cur = tm }

func (c *animClock) active() bool { return c.clock.Active() }

func (c *animClock) tick(tm time.Time) {
	c.cur = tm
	c.clock.Tick(tm)
}

// step advances one animation wake — exactly one frame period past
// the last tick for an in-flight tween, a later step's start across a
// delay — and ticks the clock there. False when nothing is scheduled.
func (c *animClock) step() bool {
	wake, ok := c.clock.Next(anim.PaceTimer)
	if !ok {
		return false
	}
	c.tick(wake)
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
