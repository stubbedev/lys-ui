// Package animclock is the animation scheduler the application loop
// drives: launched pieces, the frame deadlines the loop wakes on, and
// the tick that advances them. The public timeline API lives in
// package anim; only the loop and tests reach this package.
package animclock

import (
	"slices"
	"sync"
	"time"
)

// FrameInterval is the frame period: the fallback pacing when the
// compositor's frame callbacks stop arriving, and the deadline
// granularity of Next while a piece is mid-flight.
const FrameInterval = time.Second / 60

// Piece is one flattened step of a timeline: fn runs with Easing of
// linear progress over [At, At+Dur) from the launch instant.
type Piece struct {
	At, Dur time.Duration
	Fn      func(t float64)
	Easing  func(t float64) float64
}

type step struct {
	start, end time.Time
	fn         func(t float64)
	easing     func(t float64) float64
	grp        *group
	done       bool
}

type group struct {
	dead bool
}

// driver is an open-ended animation: fn steps it to now and reports
// whether it still runs - a physics spring, which settles when it
// settles, not at a fixed end.
type driver struct {
	fn   func(now time.Time) bool
	dead bool
}

var (
	mu      sync.Mutex
	clock   = time.Now
	active  []*step
	drivers []*driver
	// running is Tick's reused snapshot of the drivers it steps.
	running  []*driver
	lastTick time.Time
)

// Now is the scheduler's current instant: the pinned one under
// SetClock.
func Now() time.Time {
	mu.Lock()
	defer mu.Unlock()
	return clock()
}

// Launch schedules pieces from the current instant and returns the
// cancel that stops every piece not yet finished. A piece with no
// duration and no offset lands at once, outside the schedule.
func Launch(pieces []Piece) (cancel func()) {
	g := &group{}
	cancel = func() {
		mu.Lock()
		g.dead = true
		mu.Unlock()
	}
	now := Now()
	launch := make([]*step, 0, len(pieces))
	for _, p := range pieces {
		if p.Dur <= 0 && p.At <= 0 {
			p.Fn(1)
			continue
		}
		start := now.Add(p.At)
		launch = append(launch, &step{start: start, end: start.Add(p.Dur), fn: p.Fn, easing: p.Easing, grp: g})
	}
	if len(launch) > 0 {
		mu.Lock()
		active = append(active, launch...)
		mu.Unlock()
	}
	return cancel
}

// Drive runs fn once per frame with the frame's instant until it
// reports false (settled) or the returned cancel runs.
func Drive(fn func(now time.Time) bool) (cancel func()) {
	d := &driver{fn: fn}
	mu.Lock()
	drivers = append(drivers, d)
	mu.Unlock()
	return func() {
		mu.Lock()
		d.dead = true
		mu.Unlock()
	}
}

// Active reports whether any piece is still running or scheduled.
func Active() bool {
	mu.Lock()
	defer mu.Unlock()
	for _, d := range drivers {
		if !d.dead {
			return true
		}
	}
	for _, s := range active {
		if !s.done && !s.grp.dead {
			return true
		}
	}
	return false
}

// Next returns the earliest instant the loop must wake to keep the
// clock running: one frame past the last tick while a piece is
// mid-flight, or a later piece's start while a timeline holds in a
// delay. False means nothing runs and the loop may park.
func Next() (time.Time, bool) {
	mu.Lock()
	defer mu.Unlock()
	now := clock()
	if lastTick.After(now) {
		now = lastTick
	}
	if lastTick.IsZero() {
		lastTick = now
	}
	nextFrame := lastTick.Add(FrameInterval)
	if nextFrame.Before(now) {
		nextFrame = now
	}
	var next time.Time
	found := false
	for _, d := range drivers {
		if !d.dead {
			next, found = nextFrame, true
			break
		}
	}
	for _, s := range active {
		if s.done || s.grp.dead {
			continue
		}
		at := s.start
		if !now.Before(s.start) {
			at = s.end
			if nextFrame.Before(at) {
				at = nextFrame
			}
		}
		if !found || at.Before(next) {
			next, found = at, true
		}
	}
	return next, found
}

// Tick advances every running piece to now and drops finished or
// cancelled ones. Callbacks run after the pass settles with the lock
// released, so they may launch or cancel freely; a callback cancelling
// its own timeline still stops that timeline's later pieces in the
// same tick. It reports whether any callback ran.
func Tick(now time.Time) bool {
	type firing struct {
		fn    func(float64)
		eased float64
		grp   *group
	}
	mu.Lock()
	var due []firing
	keep := active[:0]
	for _, s := range active {
		if s.done || s.grp.dead {
			continue
		}
		if now.Before(s.start) {
			keep = append(keep, s)
			continue
		}
		p := 1.0
		if span := s.end.Sub(s.start); span > 0 {
			p = min(float64(now.Sub(s.start))/float64(span), 1)
		}
		s.done = p >= 1
		due = append(due, firing{fn: s.fn, eased: s.easing(p), grp: s.grp})
		if !s.done {
			keep = append(keep, s)
		}
	}
	clear(active[len(keep):])
	active = keep
	lastTick = now
	running = append(running[:0], drivers...)
	mu.Unlock()
	ran := false
	for _, f := range due {
		mu.Lock()
		dead := f.grp.dead
		mu.Unlock()
		if dead {
			continue
		}
		f.fn(f.eased)
		ran = true
	}
	for _, d := range running {
		mu.Lock()
		dead := d.dead
		mu.Unlock()
		if dead {
			continue
		}
		ran = true
		if !d.fn(now) {
			mu.Lock()
			d.dead = true
			mu.Unlock()
		}
	}
	clear(running)
	mu.Lock()
	drivers = slices.DeleteFunc(drivers, func(d *driver) bool { return d.dead })
	mu.Unlock()
	return ran
}

// SetClock replaces the time source launches and deadlines read and
// returns the restore function; nil restores time.Now.
func SetClock(fn func() time.Time) (restore func()) {
	if fn == nil {
		fn = time.Now
	}
	mu.Lock()
	old := clock
	clock = fn
	mu.Unlock()
	return func() {
		mu.Lock()
		clock = old
		mu.Unlock()
	}
}

// Reset drops every scheduled piece and driver.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	active = nil
	drivers = nil
	lastTick = clock()
}
