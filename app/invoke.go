package app

import (
	"slices"
	"sync"
	"time"

	"github.com/stubbedev/lys-ui/internal/mailbox"
)

// This file is the bridge between goroutines and the loop goroutine,
// the toolkit half of the threading contract in docs/threading.md:
//
//   - Invoke(fn) queues fn for the loop goroutine and kicks the parked
//     loop with one coalesced wake, so a background goroutine can move
//     data onto the screen without ever touching a widget itself.
//   - Every(d, fn) runs fn on the loop goroutine at a fixed cadence -
//     the shape of a poller - riding the loop's timer-wake computation
//     instead of spawning a ticker goroutine that would fight the park.
//
// Both are safe to call from any goroutine; the fn they run is loop
// code, subject to the same rules as event callbacks.

// loopQueues is the application's off-loop -> on-loop plumbing: the
// invoke queue, the periodic timers, and the flag marking the loop
// dead. The invoke queue is the untyped messenger - a mailbox.Box[func()]
// - so Invoke closures and the typed messengers of message.go share
// one armed-wake queue shape; the timers and the dead flag ride the
// same mutex the timer work already uses.
type loopQueues struct {
	mu     sync.Mutex
	timers []*loopTimer
	dead   bool
	box    mailbox.Box[func()]
}

// enqueue appends fn and reports whether this call armed the wake. A
// single armed wake covers every enqueue until the loop drains: under
// an invoke storm the queue grows but the wake count does not, and the
// parked loop wakes once per pass at most.
func (q *loopQueues) enqueue(fn func()) bool {
	return q.box.Put(fn)
}

// drain takes the queued fns and disarms, so the next enqueue arms a
// fresh wake. The caller runs the fns on the loop goroutine.
func (q *loopQueues) drain() []func() {
	return q.box.Take()
}

// pending counts queued fns (tests).
func (q *loopQueues) pending() int {
	return q.box.Pending()
}

// addTimer registers one periodic timer.
func (q *loopQueues) addTimer(t *loopTimer) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.dead {
		t.stopped = true
		return
	}
	q.timers = append(q.timers, t)
}

// killTimer marks a timer stopped; the loop reaps it on its next pass.
func (q *loopQueues) killTimer(t *loopTimer) {
	q.mu.Lock()
	defer q.mu.Unlock()
	t.stopped = true
}

// timerWork reaps stopped timers and returns those due by now. The
// caller fires them outside the lock: a timer fn may cancel itself,
// cancel its peers, or add timers (which fire no earlier than the next
// pass). Ownership of the returned timers passes to the loop
// goroutine, which alone touches their next field afterwards.
func (q *loopQueues) timerWork(now time.Time) (due []*loopTimer) {
	q.mu.Lock()
	q.timers = slices.DeleteFunc(q.timers, func(t *loopTimer) bool { return t.stopped })
	for _, t := range q.timers {
		if !t.next.After(now) {
			due = append(due, t)
		}
	}
	q.mu.Unlock()
	return due
}

// nextDeadline returns the earliest pending timer deadline for the
// loop's wake computation; zero when nothing is pending, so an idle
// application parks.
func (q *loopQueues) nextDeadline() time.Time {
	q.mu.Lock()
	defer q.mu.Unlock()
	var next time.Time
	for _, t := range q.timers {
		if t.stopped {
			continue
		}
		if next.IsZero() || t.next.Before(next) {
			next = t.next
		}
	}
	return next
}

// timerCount counts live (unstopped, unreaped) timers (tests).
func (q *loopQueues) timerCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, t := range q.timers {
		if !t.stopped {
			n++
		}
	}
	return n
}

// shutdown marks the queues dead: queued fns are discarded, every
// timer stops, and later Invokes and Every timers are dropped. Run
// calls it on exit.
func (q *loopQueues) shutdown() {
	q.mu.Lock()
	q.dead = true
	for _, t := range q.timers {
		t.stopped = true
	}
	q.timers = nil
	q.mu.Unlock()
	q.box.Stop()
}

// loopTimer is one Every ticker: fn every d, first fire after d.
type loopTimer struct {
	every time.Duration
	fn    func()
	// next is the loop goroutine's field: written when the timer fires
	// and read for the wake computation, both on the loop.
	next time.Time
	// stopped is read and written under loopQueues.mu, because cancel
	// handles are handed out to arbitrary goroutines.
	stopped bool
}

// Invoke schedules fn to run on the event-loop goroutine: the only
// sanctioned way for another goroutine to touch widgets or any other
// loop-owned state (docs/threading.md). The call itself is safe from
// any goroutine; it queues fn and wakes the parked loop, which runs it
// at the top of its next pass with the same guarantees as an event
// callback.
//
// The wake coalesces: under a storm of Invokes the loop still wakes
// once per pass, not once per call. After Run returns, Invoke drops fn
// - there is no loop left to run it on - so shutdown sequences should
// signal workers through channels, not widget state.
func (a *Application) Invoke(fn func()) {
	if fn == nil {
		return
	}
	if a.queues.enqueue(fn) && a.wake != nil {
		a.wake(0)
	}
}

// Every schedules fn to run on the event-loop goroutine once per d,
// first fire after d - the shape of a poller (a wayle refresh loop,
// a clock, a status sampler). The timer rides the loop's wake
// computation: while one is pending the parked loop wakes exactly at
// its deadline and holds no CPU in between, the same way animation and
// key-repeat deadlines do. Ticks anchor at fire time, so a slow frame
// delays the next tick instead of bursting to catch up.
//
// Every is safe to call from any goroutine; fn runs on the loop and
// must follow the loop's rules. The returned cancel stops the timer
// (idempotent, safe from any goroutine); the timer also stops on its
// own when Run returns. A non-positive d panics: a zero interval would
// spin the loop.
func (a *Application) Every(d time.Duration, fn func()) (cancel func()) {
	if d <= 0 {
		panic("app: Every interval must be positive")
	}
	if fn == nil {
		return func() {}
	}
	t := &loopTimer{every: d, fn: fn, next: time.Now().Add(d)}
	a.queues.addTimer(t)
	return sync.OnceFunc(func() { a.queues.killTimer(t) })
}

// pump runs the loop goroutine's queued work at the top of a pass:
// Invoke fns from other goroutines, then the periodic timers due by
// now. This is the single point where off-loop code crosses onto the
// loop goroutine, so everything it runs has event-callback guarantees.
// Run calls it with the pass's clock reading.
// pump reports whether any work ran: the loop then owes every
// surface a damage check, since an Invoke or timer may have touched any
// widget tree.
func (a *Application) pump(now time.Time) (ran bool) {
	for _, fn := range a.queues.drain() {
		fn()
		ran = true
	}
	for _, t := range a.queues.timerWork(now) {
		ran = true
		t.fn()
		// Anchor the next tick at fire time: no catch-up bursts after
		// a stall, one tick maximum per pass.
		t.next = now.Add(t.every)
	}
	return ran
}
