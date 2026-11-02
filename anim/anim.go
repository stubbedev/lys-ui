// Package anim declares time-based animations: tweens with easing
// curves, combined into timelines with Sequence, Parallel and Delay,
// and launched with Play; and physics springs (Spring), which run until
// they settle and keep their velocity when retargeted. The application loop advances them once per
// frame and parks when none run. Callbacks run on the loop goroutine,
// so a tween may mutate widgets directly.
package anim

import (
	"os"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

var (
	instant atomic.Bool
	reduced atomic.Bool
)

func init() {
	instant.Store(os.Getenv("LYS_UI_NO_ANIM") == "1")
}

// Instant reports whether things that move land at once: springs,
// smooth scrolling, keyframes, spinners. It is true with LYS_UI_NO_ANIM=1
// or SetInstant, under which tweens collapse as well (delays keep their
// durations, so timing survives), and under SetReduced, the desktop's
// reduced-motion preference, under which tweens still run so that a
// surface can fade.
func Instant() bool { return instant.Load() || reduced.Load() }

// NoFades reports whether even fades collapse: LYS_UI_NO_ANIM=1 or
// SetInstant.
func NoFades() bool { return instant.Load() }

// Reduced reports the desktop's reduced-motion preference: SetReduced.
func Reduced() bool { return reduced.Load() }

// SetReduced records the desktop's reduced-motion preference, which
// app.Application follows. Use widget.SetReducedMotion, which also
// restyles, from anything that draws.
func SetReduced(on bool) { reduced.Store(on) }

// SetInstant turns reduced motion on or off for later launches and
// returns the restore function.
func SetInstant(on bool) (restore func()) {
	old := instant.Swap(on)
	return func() { instant.Store(old) }
}

// Cancel stops a launched timeline: every piece not yet finished never
// fires again, and widget state keeps the last value a callback left.
// Calling it again, or after the timeline finished, does nothing.
type Cancel func()

// Step is one timed piece of a timeline: a tween (Animate), a hold
// (Delay), or a composition (Sequence, Parallel).
type Step interface {
	expand(off time.Duration, out *[]animclock.Piece) time.Duration
}

// Tween animates one value: fn runs once per frame with eased progress
// over the tween's duration.
type Tween struct {
	dur    time.Duration
	fn     func(t float64)
	easing Easing
}

// Animate declares a tween over dur with the EaseOutCubic curve. A
// zero or negative dur runs fn once with 1 when it is reached.
func Animate(dur time.Duration, fn func(t float64)) *Tween {
	return &Tween{dur: dur, fn: fn, easing: EaseOutCubic}
}

// Easing replaces the tween's curve and returns the tween.
func (t *Tween) Easing(e Easing) *Tween {
	t.easing = e
	return t
}

func (t *Tween) expand(off time.Duration, out *[]animclock.Piece) time.Duration {
	if t.fn != nil {
		easing := t.easing
		if easing == nil {
			easing = EaseOutCubic
		}
		dur := t.dur
		if instant.Load() {
			dur = 0
		}
		*out = append(*out, animclock.Piece{At: off, Dur: dur, Fn: t.fn, Easing: easing})
	}
	return off + t.dur
}

type delay time.Duration

// Delay holds the timeline for d before its next step.
func Delay(d time.Duration) Step { return delay(d) }

func (d delay) expand(off time.Duration, _ *[]animclock.Piece) time.Duration {
	return off + time.Duration(d)
}

type sequence []Step

// Sequence runs steps back to back, each starting when the previous
// one ends.
func Sequence(steps ...Step) Step { return sequence(steps) }

func (s sequence) expand(off time.Duration, out *[]animclock.Piece) time.Duration {
	for _, st := range s {
		if st != nil {
			off = st.expand(off, out)
		}
	}
	return off
}

type parallel []Step

// Parallel runs steps together and ends when the last of them ends.
func Parallel(steps ...Step) Step { return parallel(steps) }

func (p parallel) expand(off time.Duration, out *[]animclock.Piece) time.Duration {
	end := off
	for _, st := range p {
		if st != nil {
			end = max(end, st.expand(off, out))
		}
	}
	return end
}

// Play launches steps as one timeline, several steps running in
// sequence, and returns its Cancel. The schedule is fixed at launch,
// so ticks land on exact offsets however frames arrive.
func Play(steps ...Step) Cancel {
	var pieces []animclock.Piece
	_ = sequence(steps).expand(0, &pieces)
	return Cancel(animclock.Launch(pieces))
}

// Start runs fn once per frame for dur with EaseOutCubic progress from
// 0 to 1: Play(Animate(dur, fn)).
func Start(dur time.Duration, fn func(t float64)) Cancel {
	return Play(Animate(dur, fn))
}
