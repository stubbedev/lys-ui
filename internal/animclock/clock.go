// Package animclock is the animation scheduler the application loop
// drives: launched pieces, the frame deadlines the loop wakes on, and
// the tick that advances them. Each application owns a Clock; the
// package-level calls act on the current one. The public timeline API
// lives in package anim; only the loop and tests reach this package.
package animclock

import (
	"slices"
	"sync"
	"sync/atomic"
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
	dead atomic.Bool
}

// driver is an open-ended animation: fn steps it to now and reports
// whether it still runs - a physics spring, which settles when it
// settles, not at a fixed end.
type driver struct {
	fn   func(now time.Time) bool
	dead atomic.Bool
}

// Clock is one schedule of launched pieces and drivers, with the frame
// deadlines and the tick that advance them. An application owns one and
// its loop alone ticks it; Install makes it the clock launches land on.
// The zero Clock is empty and ready.
type Clock struct {
	mu       sync.Mutex
	active   []*step
	drivers  []*driver
	running  []*driver
	firings  []firing
	lastTick time.Time
	launched time.Time
}

var (
	source   atomic.Pointer[func() time.Time]
	detached Clock
	current  atomic.Pointer[Clock]
)

type firing struct {
	fn    func(float64)
	eased float64
	grp   *group
}

// Now is the scheduler's current instant: the pinned one under
// SetClock.
func Now() time.Time {
	if fn := source.Load(); fn != nil {
		return (*fn)()
	}
	return time.Now()
}

// Current is the clock launches land on: the one Install made current,
// or, with none, the detached clock, which no application ticks: tests
// drive it with Tick, and the next Install takes what waits on it.
func Current() *Clock {
	if c := current.Load(); c != nil {
		return c
	}
	return &detached
}

// Install makes c the clock launches land on, moves onto it what waits
// on the detached clock (launched while no clock was current), and
// returns its release, which drops every piece and driver still on c
// and, unless a later Install replaced c, leaves the detached clock
// current again.
func Install(c *Clock) (release func()) {
	current.Store(c)
	c.adopt(&detached)
	return func() {
		current.CompareAndSwap(c, nil)
		c.Reset()
	}
}

// Launch schedules pieces on the current clock: Clock.Launch.
func Launch(pieces []Piece) (cancel func()) { return Current().Launch(pieces) }

// Drive runs fn on the current clock: Clock.Drive.
func Drive(fn func(now time.Time) bool) (cancel func()) { return Current().Drive(fn) }

// Active reports whether the current clock runs anything: Clock.Active.
func Active() bool { return Current().Active() }

// Next is the current clock's next deadline: Clock.Next.
func Next(pace Pace) (time.Time, bool) { return Current().Next(pace) }

// Due reports whether the current clock's deadline has come: Clock.Due.
func Due(now time.Time, pace Pace) bool { return Current().Due(now, pace) }

// Tick advances the current clock: Clock.Tick.
func Tick(now time.Time) bool { return Current().Tick(now) }

// Reset empties the current clock: Clock.Reset.
func Reset() { Current().Reset() }

// SetClock replaces the time source launches and deadlines read and
// returns the restore function; nil restores time.Now. Both rewind the
// current clock's last tick and launch.
func SetClock(fn func() time.Time) (restore func()) {
	var next *func() time.Time
	if fn != nil {
		next = &fn
	}
	old := source.Swap(next)
	Current().rewind()
	return func() {
		source.Store(old)
		Current().rewind()
	}
}

func (c *Clock) adopt(from *Clock) {
	from.mu.Lock()
	active, drivers, launched := from.active, from.drivers, from.launched
	from.active, from.drivers = nil, nil
	from.mu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = append(c.active, active...)
	c.drivers = append(c.drivers, drivers...)
	if launched.After(c.launched) {
		c.launched = launched
	}
}

func (c *Clock) rewind() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastTick, c.launched = time.Time{}, time.Time{}
}

// Launch schedules pieces from the current instant and returns the
// cancel that stops every piece not yet finished. A piece with no
// duration and no offset lands at once, outside the schedule.
func (c *Clock) Launch(pieces []Piece) (cancel func()) {
	g := &group{}
	cancel = func() { g.dead.Store(true) }
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
		c.mu.Lock()
		c.active = append(c.active, launch...)
		c.launched = now
		c.mu.Unlock()
	}
	return cancel
}

// Drive runs fn once per frame with the frame's instant until it
// reports false (settled) or the returned cancel runs.
func (c *Clock) Drive(fn func(now time.Time) bool) (cancel func()) {
	d := &driver{fn: fn}
	c.mu.Lock()
	c.drivers = append(c.drivers, d)
	c.mu.Unlock()
	return func() { d.dead.Store(true) }
}

// Active reports whether any piece is still running or scheduled.
func (c *Clock) Active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.drivers {
		if !d.dead.Load() {
			return true
		}
	}
	for _, s := range c.active {
		if !s.done && !s.grp.dead.Load() {
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
func (c *Clock) Next(pace Pace) (time.Time, bool) {
	now := Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastTick.After(now) {
		now = c.lastTick
	}
	if c.lastTick.IsZero() {
		c.lastTick = now
	}
	next, found := c.nextDeadline(now, pace)
	if found && next.Before(now) {
		next = now
	}
	return next, found
}

// Due reports whether the clock's deadline at pace has come by now, so a
// pass that was not woken by a frame callback ticks it; a pass woken by
// anything else leaves running pieces where they are.
func (c *Clock) Due(now time.Time, pace Pace) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	next, found := c.nextDeadline(now, pace)
	return found && !next.After(now)
}

func (c *Clock) nextDeadline(now time.Time, pace Pace) (time.Time, bool) {
	frame := c.lastTick.Add(pace.interval())
	var next time.Time
	found := false
	for _, d := range c.drivers {
		if !d.dead.Load() {
			next, found = frame, true
			break
		}
	}
	for _, s := range c.active {
		if s.done || s.grp.dead.Load() {
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
func (c *Clock) Tick(now time.Time) bool {
	c.mu.Lock()
	if now.Before(c.lastTick) {
		now = c.lastTick
	}
	if now.Before(c.launched) {
		now = c.launched
	}
	due := c.firings
	c.firings = nil
	keep := c.active[:0]
	for _, s := range c.active {
		if s.done || s.grp.dead.Load() {
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
	clear(c.active[len(keep):])
	c.active = keep
	c.lastTick = now
	stepped := append(c.running[:0], c.drivers...)
	c.running = nil
	c.mu.Unlock()
	ran := false
	for _, f := range due {
		if f.grp.dead.Load() {
			continue
		}
		f.fn(f.eased)
		ran = true
	}
	clear(due)
	for _, d := range stepped {
		if d.dead.Load() {
			continue
		}
		ran = true
		if !d.fn(now) {
			d.dead.Store(true)
		}
	}
	clear(stepped)
	c.mu.Lock()
	c.firings, c.running = due[:0], stepped[:0]
	c.drivers = slices.DeleteFunc(c.drivers, func(d *driver) bool { return d.dead.Load() })
	c.mu.Unlock()
	return ran
}

// Reset drops every scheduled piece and driver.
func (c *Clock) Reset() {
	now := Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active = nil
	c.drivers = nil
	c.lastTick = now
	c.launched = time.Time{}
}
