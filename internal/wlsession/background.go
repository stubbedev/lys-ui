package wlsession

import (
	"time"

	"github.com/stubbedev/lys-ui/internal/worker"
)

const backgroundQueue = 128

const backgroundStop = 2 * time.Second

func (s *Session) background() *worker.Queue {
	if s.bg == nil {
		s.bg = worker.New(backgroundQueue)
	}
	return s.bg
}

func (s *Session) postToLoop(fn func()) {
	s.postedMu.Lock()
	s.posted = append(s.posted, fn)
	s.postedMu.Unlock()
	s.hasPosted.Store(true)
	s.WakeAfter(0)
}

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

func (s *Session) stopBackground() {
	if s.bg != nil {
		s.bg.Stop(backgroundStop)
	}
}
