// Package worker runs blocking work off the loop goroutine: one
// goroutine at a time, in posting order, over a bounded queue. The
// goroutine lives only while work is queued, so an idle Queue costs
// nothing and a Stop leaves nothing behind.
package worker

import (
	"sync"
	"time"
)

// Queue runs posted functions in order. Create it with New.
type Queue struct {
	capacity int

	mu      sync.Mutex
	jobs    []func()
	running bool
	stopped bool
	settled chan struct{}
}

// New returns a queue holding at most capacity functions beyond the one
// running.
func New(capacity int) *Queue {
	return &Queue{capacity: capacity}
}

// Post queues fn without blocking and reports whether it was taken: a
// full or stopped queue refuses it.
func (q *Queue) Post(fn func()) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped || len(q.jobs) == q.capacity {
		return false
	}
	q.jobs = append(q.jobs, fn)
	if !q.running {
		q.running = true
		q.settled = make(chan struct{})
		go q.run(q.settled)
	}
	return true
}

func (q *Queue) run(settled chan struct{}) {
	defer close(settled)
	for {
		q.mu.Lock()
		if len(q.jobs) == 0 {
			q.jobs = nil
			q.running = false
			q.mu.Unlock()
			return
		}
		fn := q.jobs[0]
		q.jobs[0] = nil
		q.jobs = q.jobs[1:]
		q.mu.Unlock()
		fn()
	}
}

// Settle waits until everything posted so far ran, at most for limit,
// and reports whether it did.
func (q *Queue) Settle(limit time.Duration) bool {
	q.mu.Lock()
	running, settled := q.running, q.settled
	q.mu.Unlock()
	if !running {
		return true
	}
	t := time.NewTimer(limit)
	defer t.Stop()
	select {
	case <-settled:
		return true
	case <-t.C:
		return false
	}
}

// Stop refuses later posts, lets queued work finish, and waits at most
// for limit, reporting whether the work ended. A function stuck past
// limit keeps its goroutine until it returns; the queue is dead either
// way. Safe to call twice.
func (q *Queue) Stop(limit time.Duration) bool {
	q.mu.Lock()
	q.stopped = true
	q.mu.Unlock()
	return q.Settle(limit)
}
