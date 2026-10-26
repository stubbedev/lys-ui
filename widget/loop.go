package widget

import (
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// loop is a repeating linear phase that runs only while on and on
// screen - the indeterminate motion Spinner and a pulsing ProgressBar
// share. Turning it on takes effect once the owner is arranged into a
// visible rect; going off screen stops it and coming back resumes;
// turning it off freezes the phase where it was. A stopped or hidden
// loop schedules no wake, so an idle app with a parked indicator costs
// nothing.
type loop struct {
	period  time.Duration
	on      bool
	visible bool
	cancel  anim.Cancel
	// step receives each frame's phase in [0, 1); a wrap reports 0.
	step func(phase float64)
}

// set turns the loop on or off.
func (l *loop) set(on bool) {
	if on == l.on {
		return
	}
	l.on = on
	if !on {
		l.stop()
		return
	}
	if l.visible {
		l.start()
	}
}

// arranged syncs the loop with the owner's rect: only the empty/
// non-empty transition starts or stops it, so a no-change Arrange
// never restarts a cycle.
func (l *loop) arranged(r render.Rect) {
	l.visible = !r.Empty()
	switch {
	case !l.visible:
		l.stop()
	case l.on && l.cancel == nil:
		l.start()
	}
}

// start launches one cycle; the landing tick relaunches while the loop
// is still on and visible (anim callbacks may launch - Tick runs them
// with its lock released). Under reduced motion a cycle lands at
// launch, so the loop parks at phase 0 instead of relaunching forever.
func (l *loop) start() {
	if l.cancel != nil {
		l.cancel()
	}
	if anim.Instant() {
		l.cancel = nil
		l.step(0)
		return
	}
	l.cancel = anim.Play(anim.Animate(l.period, func(t float64) {
		if t >= 1 {
			l.step(0)
			l.cancel = nil
			if l.on && l.visible {
				l.start()
			}
			return
		}
		l.step(t)
	}).Easing(anim.Linear))
}

// stop cancels the running cycle.
func (l *loop) stop() {
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
}
