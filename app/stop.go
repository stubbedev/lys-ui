package app

import (
	"slices"
	"sync"
)

type stopSet struct {
	mu    sync.Mutex
	next  int
	stops []stopEntry
	dead  bool
}

type stopEntry struct {
	id int
	fn func()
}

func (s *stopSet) add(fn func()) (id int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return 0, false
	}
	s.next++
	s.stops = append(s.stops, stopEntry{id: s.next, fn: fn})
	return s.next, true
}

func (s *stopSet) remove(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops = slices.DeleteFunc(s.stops, func(e stopEntry) bool { return e.id == id })
}

func (s *stopSet) shutdown() {
	s.mu.Lock()
	stops := s.stops
	s.stops, s.dead = nil, true
	s.mu.Unlock()
	for _, e := range slices.Backward(stops) {
		e.fn()
	}
}

// OnStop registers fn to run when the application closes (Close, which
// Run calls when it returns), after the loop's last
// pass. Hooks run in reverse registration order, like deferred calls.
// After the application closed, fn runs at once. The returned cancel
// unregisters fn without running it.
func (a *Application) OnStop(fn func()) (cancel func()) {
	id, ok := a.watchers.add(fn)
	if !ok {
		fn()
		return func() {}
	}
	return sync.OnceFunc(func() { a.watchers.remove(id) })
}
