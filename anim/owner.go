package anim

import (
	"sync/atomic"
	"time"
)

// Owner is what an animation belongs to, and where the clock that runs
// it comes from. Every widget is one: its animations run on the clock of
// the application whose window shows it, wait while it is in no window
// and start once it is, and never run for a window that closed. An
// app.Application is one, for what belongs to no widget, and so is a
// Clock.
type Owner interface {
	// ScheduleAnimation starts l on the owner's clock, now or once the
	// owner has one, or never.
	ScheduleAnimation(l *Launch)
}

// Launch is one animation on its way to a clock: an Owner starts it on
// the clock that runs it. It starts once at most, and not at all once
// cancelled.
type Launch struct {
	start     func(c *Clock) (cancel func())
	started   atomic.Bool
	cancelled atomic.Bool
	stop      atomic.Pointer[func()]
}

// Start runs the animation on c. A second Start, or one after the
// animation was cancelled, does nothing.
func (l *Launch) Start(c *Clock) {
	if l.cancelled.Load() || !l.started.CompareAndSwap(false, true) {
		return
	}
	stop := l.start(c)
	l.stop.Store(&stop)
	if l.cancelled.Load() {
		stop()
	}
}

// Cancelled reports whether the animation was cancelled, so an owner
// holding it for later can let it go.
func (l *Launch) Cancelled() bool { return l.cancelled.Load() }

func (l *Launch) cancel() {
	l.cancelled.Store(true)
	if stop := l.stop.Load(); stop != nil {
		(*stop)()
	}
}

func schedule(o Owner, start func(c *Clock) (cancel func())) Cancel {
	l := &Launch{start: start}
	o.ScheduleAnimation(l)
	return l.cancel
}

// Drive runs fn on o's clock once per frame, with the frame's instant,
// until it reports false or the returned Cancel runs: an animation with
// no fixed end, which settles when it settles.
func Drive(o Owner, fn func(now time.Time) bool) Cancel {
	return schedule(o, func(c *Clock) func() { return c.drive(fn) })
}
