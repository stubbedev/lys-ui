package anim

import (
	"math"
	"time"

	"github.com/stubbedev/lys-lib/motion"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// SpringParams shapes a physics spring the way SwiftUI's springs are
// shaped: Response is the period of the undamped oscillation, how long
// one swing takes; Damping is the damping fraction - 1 settles without
// overshoot, below 1 bounces (smaller bounces more), above 1 creeps.
type SpringParams struct {
	Response time.Duration
	Damping  float64
}

// The named springs.
var (
	// SpringSmooth settles without overshoot: layout and size changes.
	SpringSmooth = SpringParams{Response: 350 * time.Millisecond, Damping: 1}
	// SpringSnappy overshoots a touch and lands quickly: popovers,
	// menus, toggles.
	SpringSnappy = SpringParams{Response: 300 * time.Millisecond, Damping: 0.86}
	// SpringBouncy overshoots visibly before it settles: playful
	// entrances, toasts.
	SpringBouncy = SpringParams{Response: 450 * time.Millisecond, Damping: 0.68}
)

// springPrecision is the default settle precision, in value units: a
// hundredth, right for pixels and color channels.
const springPrecision = 0.01

// Spring moves any number of components (a rect's x, y, w, h; a
// radius; a color's channels) toward their targets with a damped
// harmonic spring, one per component. It carries position and
// velocity, so retargeting it mid-flight continues from where and how
// fast it is moving instead of restarting from rest - an interrupted
// open turning into a close flows rather than jerks. Steps are the
// closed-form solution, exact for any frame interval. It runs on the
// application loop: onFrame sees every frame's position, and must not
// keep the slice.
type Spring struct {
	params  SpringParams
	pos     []float64
	vel     []float64
	target  []float64
	onFrame func(pos []float64)
	last    time.Time
	cancel  func()
	// precision is how close to its target, in value units per
	// component, the spring must be before it counts as settled and
	// snaps there; its velocity must be under precision times the
	// angular frequency, the speed of a swing that small.
	precision float64
}

// NewSpring returns a spring at rest at initial, which reports its
// position to onFrame on every frame it moves.
func NewSpring(p SpringParams, initial []float64, onFrame func(pos []float64)) *Spring {
	n := len(initial)
	return &Spring{
		params:  p,
		pos:     append(make([]float64, 0, n), initial...),
		vel:     make([]float64, n),
		target:  append(make([]float64, 0, n), initial...),
		onFrame: onFrame,

		precision: springPrecision,
	}
}

// SetPrecision sets how close, in the components' units, counts as
// arrived: the default hundredth suits pixels; a 0-to-1 fraction wants
// a thousandth, or a small overshoot would read as settled.
func (s *Spring) SetPrecision(p float64) { s.precision = p }

// SetParams reshapes the spring; the motion in flight continues under
// the new shape from its current position and velocity.
func (s *Spring) SetParams(p SpringParams) { s.params = p }

// SetTarget retargets the spring, keeping its position and velocity,
// and runs it until it settles. Under reduced motion (Instant) it lands
// on the target at once, with one onFrame. target has one value per
// component; a different count is a caller bug and panics.
func (s *Spring) SetTarget(target ...float64) {
	if len(target) != len(s.pos) {
		panic("anim: Spring.SetTarget: component count differs from the spring's")
	}
	copy(s.target, target)
	if Instant() {
		s.Snap(target...)
		return
	}
	if s.cancel != nil {
		return // running: the next frame steps toward the new target
	}
	s.last = animclock.Now()
	s.cancel = animclock.Drive(s.step)
}

// Snap puts the spring at rest at v, stopping any motion, with one
// onFrame.
func (s *Spring) Snap(v ...float64) {
	if len(v) != len(s.pos) {
		panic("anim: Spring.Snap: component count differs from the spring's")
	}
	s.Stop()
	copy(s.pos, v)
	copy(s.target, v)
	clear(s.vel)
	if s.onFrame != nil {
		s.onFrame(s.pos)
	}
}

// Stop halts the spring where it is.
func (s *Spring) Stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	clear(s.vel)
}

// Position and Velocity are the spring's current state, per
// component; the slices are the spring's own.
func (s *Spring) Position() []float64 { return s.pos }

// Velocity is the spring's current velocity in units per second.
func (s *Spring) Velocity() []float64 { return s.vel }

// Running reports whether the spring is in motion.
func (s *Spring) Running() bool { return s.cancel != nil }

// step advances to now; it reports false once every component has
// settled, which ends the drive.
func (s *Spring) step(now time.Time) bool {
	dt := now.Sub(s.last).Seconds()
	s.last = now
	settled := s.advance(dt)
	if s.onFrame != nil {
		s.onFrame(s.pos)
	}
	if settled {
		s.cancel = nil
	}
	return !settled
}

// advance moves every component dt seconds along its spring and snaps
// the settled ones to their target. It reports whether all settled.
func (s *Spring) advance(dt float64) bool {
	all := true
	omega := springOmega(s.params)
	vEps := s.precision * omega
	for i := range s.pos {
		x, v := motion.SpringStep(omega, s.params.Damping, s.pos[i]-s.target[i], s.vel[i], dt)
		if math.Abs(x) < s.precision && math.Abs(v) < vEps {
			x, v = 0, 0
		} else {
			all = false
		}
		s.pos[i], s.vel[i] = s.target[i]+x, v
	}
	return all
}

// springOmega is the spring's undamped angular frequency.
func springOmega(p SpringParams) float64 {
	return 2 * math.Pi / math.Max(p.Response.Seconds(), 1e-3)
}
