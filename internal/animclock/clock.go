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

// FrameInterval is the period the timer paces a running piece at while no
// surface waits on a frame callback.
const FrameInterval = time.Second / 60

// FallbackInterval is the period a running piece ticks at while a surface
// waits on a frame callback, or no surface can show it: the callback paces it
// when the compositor answers, and an occluded window's animations still land.
const FallbackInterval = time.Second / 4

// Pace says what paces the clock.
type Pace uint8

const (
	// Timer paces the clock at FrameInterval: a surface could draw now.
	Timer Pace = iota
	// Fallback leaves pacing to frame callbacks, with FallbackInterval
	// under them: every surface that could draw waits on one.
	Fallback
)

func (p Pace) interval() time.Duration {
	if p == Fallback {
		return FallbackInterval
	}
	return FrameInterval
}

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
	mu       sync.Mutex
	clock    = time.Now
	active   []*step
	drivers  []*driver
	running  []*driver
	firings  []firing
	lastTick time.Time
	launched time.Time
)

type firing struct {
	fn    func(float64)
	eased float64
	grp   *group
}

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
		launched = now
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

// Next returns the earliest instant the loop must wake to keep the clock
// running at pace: one interval past the last tick while a piece is
// mid-flight, or a later piece's start while a timeline holds in a delay. At
// Timer a piece's end counts too, so its last frame lands on time. False means
// nothing runs and the loop may park.
func Next(pace Pace) (time.Time, bool) {
	mu.Lock()
	defer mu.Unlock()
	now := clock()
	if lastTick.After(now) {
		now = lastTick
	}
	if lastTick.IsZero() {
		lastTick = now
	}
	next, found := nextDeadline(now, pace)
	if found && next.Before(now) {
		next = now
	}
	return next, found
}

// Due reports whether the clock's deadline at pace has come by now, so a
// pass that was not woken by a frame callback ticks it; a pass woken by
// anything else leaves running pieces where they are.
func Due(now time.Time, pace Pace) bool {
	mu.Lock()
	defer mu.Unlock()
	next, found := nextDeadline(now, pace)
	return found && !next.After(now)
}

func nextDeadline(now time.Time, pace Pace) (time.Time, bool) {
	frame := lastTick.Add(pace.interval())
	var next time.Time
	found := false
	for _, d := range drivers {
		if !d.dead {
			next, found = frame, true
			break
		}
	}
	for _, s := range active {
		if s.done || s.grp.dead {
			continue
		}
		at := frame
		switch {
		case now.Before(s.start) && s.start.After(at):
			at = s.start
		case now.Before(s.start) && pace == Timer:
			at = s.start
		case pace == Timer && s.end.Before(at):
			at = s.end
		}
		if !found || at.Before(next) {
			next, found = at, true
		}
	}
	return next, found
}

// Tick advances every running piece to now and drops finished or
// cancelled ones. Time never runs back: a now older than the last tick, or
// than the last launch (a compositor timestamp lags the local clock), counts
// as that instant. Callbacks run after the pass settles with the lock
// released, so they may launch or cancel freely; a callback cancelling
// its own timeline still stops that timeline's later pieces in the
// same tick. It reports whether any callback ran.
func Tick(now time.Time) bool {
	mu.Lock()
	if now.Before(lastTick) {
		now = lastTick
	}
	if now.Before(launched) {
		now = launched
	}
	due := firings
	firings = nil
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
	stepped := append(running[:0], drivers...)
	running = nil
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
	clear(due)
	for _, d := range stepped {
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
	clear(stepped)
	mu.Lock()
	firings, running = due[:0], stepped[:0]
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
	lastTick, launched = time.Time{}, time.Time{}
	mu.Unlock()
	return func() {
		mu.Lock()
		clock = old
		lastTick, launched = time.Time{}, time.Time{}
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
	launched = time.Time{}
}
