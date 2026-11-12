package anim

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// FrameInterval is the period the timer paces a running clock at while no
// surface waits on a frame callback.
const FrameInterval = time.Second / 60

// FallbackInterval is the period a running clock ticks at while a surface
// waits on a frame callback, or no surface can show it: the callback paces it
// when the compositor answers, and an occluded window's animations still land.
const FallbackInterval = time.Second / 4

// Pace says what paces a Clock.
type Pace uint8

const (
	// PaceTimer paces the clock at FrameInterval: a surface could draw now.
	PaceTimer Pace = iota
	// PaceFallback leaves pacing to frame callbacks, with FallbackInterval
	// under them: every surface that could draw waits on one.
	PaceFallback
)

func (p Pace) interval() time.Duration {
	if p == PaceFallback {
		return FallbackInterval
	}
	return FrameInterval
}

type piece struct {
	at, dur time.Duration
	fn      func(t float64)
	easing  func(t float64) float64
}

func (p piece) instant() bool { return p.dur <= 0 && p.at <= 0 }

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

type driver struct {
	fn   func(now time.Time) bool
	dead atomic.Bool
}

type firing struct {
	fn    func(float64)
	eased float64
	grp   *group
}

// Clock is the schedule of one application's animations: the timelines,
// springs and drivers launched onto it, the frame deadlines they need and
// the tick that advances them. An app.Application owns one and its loop
// alone ticks it; a widget's animations land on the clock of the
// application whose window shows the widget. A Clock is an Owner of its
// own: what is scheduled on it starts on it at once. Stop ends it for good.
// The zero Clock is empty and ready.
type Clock struct {
	mu       sync.Mutex
	active   []*step
	drivers  []*driver
	running  []*driver
	firings  []firing
	lastTick time.Time
	launched time.Time
	stopped  bool
}

// ScheduleAnimation starts l on c.
func (c *Clock) ScheduleAnimation(l *Launch) { l.Start(c) }

// Stop drops every timeline and driver on c and refuses every later
// launch: the clock of an application that closed runs nothing again.
func (c *Clock) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active, c.drivers = nil, nil
	c.stopped = true
}

func (c *Clock) launch(pieces []piece) (cancel func()) {
	g := &group{}
	now := animclock.Now()
	launch := make([]*step, 0, len(pieces))
	for _, p := range pieces {
		start := now.Add(p.at)
		launch = append(launch, &step{start: start, end: start.Add(p.dur), fn: p.fn, easing: p.easing, grp: g})
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return func() {}
	}
	c.active = append(c.active, launch...)
	c.launched = now
	return func() { g.dead.Store(true) }
}

func (c *Clock) drive(fn func(now time.Time) bool) (cancel func()) {
	d := &driver{fn: fn}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return func() {}
	}
	c.drivers = append(c.drivers, d)
	return func() { d.dead.Store(true) }
}

// Active reports whether anything on c still runs or waits to start.
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

// Next returns the earliest instant the loop must wake to keep c running
// at pace: one interval past the last tick while a timeline is mid-flight,
// or a later step's start while a timeline holds in a delay. At PaceTimer a
// step's end counts too, so its last frame lands on time. False means
// nothing runs and the loop may park.
func (c *Clock) Next(pace Pace) (time.Time, bool) {
	now := animclock.Now()
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

// Due reports whether c's deadline at pace has come by now, so a pass
// that was not woken by a frame callback ticks it; a pass woken by
// anything else leaves running timelines where they are.
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
		case now.Before(s.start) && pace == PaceTimer:
			at = s.start
		case pace == PaceTimer && s.end.Before(at):
			at = s.end
		}
		if !found || at.Before(next) {
			next, found = at, true
		}
	}
	return next, found
}

// Tick advances everything on c to now and drops what finished or was
// cancelled. Time never runs back: a now older than the last tick, or
// than the last launch (a compositor timestamp lags the local clock),
// counts as that instant. Callbacks run after the pass settles with the
// lock released, so they may launch or cancel freely; a callback
// cancelling its own timeline still stops that timeline's later steps in
// the same tick. It reports whether any callback ran.
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
