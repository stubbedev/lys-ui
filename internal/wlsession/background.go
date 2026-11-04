package wlsession

import (
	"time"

	"github.com/stubbedev/lys-ui/internal/worker"
)

// backgroundQueue bounds the work waiting for the session's background
// goroutine; its closures only read files and compile.
const backgroundQueue = 128

// backgroundStop is how long Close waits for work already running.
const backgroundStop = 2 * time.Second

// background is the session's off-loop goroutine, for what must not stall
// a frame: compiling a keymap, reading cursor files. It runs one closure
// at a time, in posting order, so what it uses needs no lock. Its results
// come back through postToLoop.
func (s *Session) background() *worker.Queue {
	if s.bg == nil {
		s.bg = worker.New(backgroundQueue)
	}
	return s.bg
}

// postToLoop runs fn on the loop goroutine, at the end of the next Step:
// the way back from the background goroutine. Safe from any goroutine.
func (s *Session) postToLoop(fn func()) {
	s.postedMu.Lock()
	s.posted = append(s.posted, fn)
	s.postedMu.Unlock()
	s.hasPosted.Store(true)
	s.WakeAfter(0)
}

// runPosted runs what the background goroutine posted, in order.
func (s *Session) runPosted() {
	if !s.hasPosted.Swap(false) {
		return
	}
	s.postedMu.Lock()
	posted := s.posted
	s.posted = nil
	s.postedMu.Unlock()
	for _, fn := range posted {
		fn()
	}
}

// stopBackground ends the background goroutine.
func (s *Session) stopBackground() {
	if s.bg != nil {
		s.bg.Stop(backgroundStop)
	}
}
