package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
)

// loop is a repeating linear phase that runs only while on and on
// screen - the indeterminate motion Spinner and a pulsing ProgressBar
// share. Turning it on takes effect once the owner is on screen; going
// off screen stops it and coming back resumes; turning it off freezes
// the phase where it was. A stopped or hidden
// loop schedules no wake, so an idle app with a parked indicator costs
// nothing.
type loop struct {
	period time.Duration
	owner  Widget
	on     bool
	cancel anim.Cancel
	// step receives each frame's phase in [0, 1).
	step func(phase float64)
}

// set turns the loop on or off.
func (l *loop) set(on bool) {
	if on == l.on {
		return
	}
	l.on = on
	if n := nodeOf(l.owner); n != nil {
		n.markBranchStale()
	}
	if !on {
		l.stop()
		return
	}
	if onScreen(l.owner) {
		l.start()
	}
}

// arranged syncs the loop with the owner's visibility: only the
// off/on-screen transition starts or stops it, so a no-change Arrange
// never restarts a cycle.
func (l *loop) arranged() {
	if l.on {
		arrangeVolatility++
	}
	switch {
	case !onScreen(l.owner):
		l.stop()
	case l.on && l.cancel == nil:
		l.start()
	}
}

// start runs the phase from 0 until the loop stops or goes off screen.
// Under reduced motion it parks at phase 0.
func (l *loop) start() {
	if l.cancel != nil {
		l.cancel()
	}
	if anim.Instant() {
		l.cancel = nil
		l.step(0)
		return
	}
	started := animclock.Now()
	l.cancel = anim.Cancel(animclock.Drive(func(now time.Time) bool {
		if !onScreen(l.owner) {
			l.cancel = nil
			return false
		}
		l.step(float64(now.Sub(started)%l.period) / float64(l.period))
		return true
	}))
}

// stop cancels the running cycle.
func (l *loop) stop() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
}
