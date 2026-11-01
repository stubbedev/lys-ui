package anim

import (
	"math"
	"time"
)

// Easing maps linear progress t in [0, 1] to eased progress. Most
// curves keep the result inside [0, 1]; springs may overshoot past 1
// (or dip below 0) on their way to settling, so consumers must clamp
// where a value cannot overshoot (alpha, positions in bounds).
type Easing func(t float64) float64

// Linear runs at constant speed.
func Linear(t float64) float64 { return t }

// EaseInQuad starts slow and accelerates.
func EaseInQuad(t float64) float64 { return t * t }

// EaseOutQuad starts fast and decelerates.
func EaseOutQuad(t float64) float64 { return 1 - float64((1-t)*(1-t)) }

// EaseInOutQuad accelerates through the first half, decelerates
// through the second.
func EaseInOutQuad(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	u := float64(-2*t) + 2
	return 1 - float64(u*u)/2
}

// EaseInCubic starts slow and accelerates hard.
func EaseInCubic(t float64) float64 { return t * t * t }

// EaseOutCubic is lys-ui's default curve: fast start, gentle landing.
func EaseOutCubic(t float64) float64 { return 1 - float64((1-t)*(1-t)*(1-t)) }

// EaseInOutCubic eases both ends of the tween.
func EaseInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := float64(-2*t) + 2
	return 1 - float64(u*u*u)/2
}

// EaseOutBack overshoots the target by a fixed fraction and settles
// back - the classic popover landing. The overshoot peaks around 1.10
// near t = 0.7.
func EaseOutBack(t float64) float64 {
	const c1 = 1.70158
	const c3 = c1 + 1
	if t <= 0 || t >= 1 {
		// Clamp the polynomial: its endpoints only land on 0 and 1 up
		// to float noise, and the contract is exact.
		return min(max(t, 0), 1)
	}
	u := t - 1
	return 1 + float64(c3*u*u*u) + float64(c1*u*u)
}

// CubicBezier returns the CSS cubic-bezier(x1, y1, x2, y2) timing
// function; see CubicBezierAt.
func CubicBezier(x1, y1, x2, y2 float64) Easing {
	return func(t float64) float64 { return CubicBezierAt(x1, y1, x2, y2, t) }
}

// CubicBezierAt evaluates the CSS cubic-bezier(x1, y1, x2, y2) timing
// function at t: x is time, y progress, the curve's parameter solved
// for x (Newton where it converges, bisection where it does not). x1
// and x2 clamp to [0, 1], the range CSS admits, so the curve stays a
// function of time; the y controls are free. Outside (0, 1) the
// output holds at the end it passed.
func CubicBezierAt(x1, y1, x2, y2, t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t >= 1:
		return 1
	}
	x1, x2 = min(max(x1, 0), 1), min(max(x2, 0), 1)
	cx, cy := 3*x1, 3*y1
	bx, by := 3*(x2-x1)-cx, 3*(y2-y1)-cy
	ax, ay := 1-cx-bx, 1-cy-by
	sampleX := func(u float64) float64 { return ((ax*u+bx)*u + cx) * u }
	u := t
	for range 8 {
		dx := sampleX(u) - t
		if dx > -1e-7 && dx < 1e-7 {
			return ((ay*u+by)*u + cy) * u
		}
		d := (3*ax*u+2*bx)*u + cx
		if d < 1e-6 && d > -1e-6 {
			break
		}
		u -= dx / d
	}
	lo, hi := 0.0, 1.0
	u = t
	for range 40 {
		if sampleX(u) < t {
			lo = u
		} else {
			hi = u
		}
		u = (lo + hi) / 2
	}
	return ((ay*u+by)*u + cy) * u
}

// StepPos is where a steps() curve jumps: at the start of each
// interval, the end, both edges, or neither.
type StepPos uint8

// The steps() jump positions. JumpEnd is the CSS default (and the
// legacy `end` keyword); JumpStart is the legacy `start`.
const (
	JumpEnd StepPos = iota
	JumpStart
	JumpNone
	JumpBoth
)

// Steps returns the steps(n, jump) timing function; see StepsAt.
func Steps(n int, jump StepPos) Easing {
	return func(t float64) float64 { return StepsAt(n, jump, t, false) }
}

// StepsAt evaluates steps(n, jump) at t (CSS Easing 2 §2.3.1): the
// progress held in n equal intervals, jumping at each interval's start
// (JumpStart), end (JumpEnd), both edges (JumpBoth, n+1 jumps) or
// neither (JumpNone, n-1 jumps). before is the before flag: sampled
// from the before phase, a jump exactly at t is not yet taken. n reads
// as at least 1, and as at least 2 under JumpNone, the counts CSS
// admits.
func StepsAt(n int, jump StepPos, t float64, before bool) float64 {
	steps := max(n, 1)
	if jump == JumpNone {
		steps = max(n, 2)
	}
	cur := math.Floor(t * float64(steps))
	if jump == JumpStart || jump == JumpBoth {
		cur++
	}
	if before && math.Mod(t*float64(steps), 1) == 0 {
		cur--
	}
	if t >= 0 && cur < 0 {
		cur = 0
	}
	jumps := float64(steps)
	switch jump {
	case JumpNone:
		jumps--
	case JumpBoth:
		jumps++
	}
	if t <= 1 && cur > jumps {
		cur = jumps
	}
	return cur / jumps
}

// LinearPt is one point of a linear() easing: the output value and,
// when HasPos, its input position as a fraction of the timeline.
type LinearPt struct {
	Value float64
	Pos   float64
	// HasPos marks an explicit percentage stop; the rest spread
	// evenly between their bracketing explicit stops.
	HasPos bool
}

// CanonicalLinear resolves a linear() point list's positions in place
// (CSS Easing 2 §2.1.1): the first point defaults to 0 and the last to
// 1, a position below an earlier one rises to it, and the points left
// without one spread evenly between their positioned neighbours. Every
// point has HasPos set afterwards.
func CanonicalLinear(pts []LinearPt) {
	n := len(pts)
	if n == 0 {
		return
	}
	if !pts[0].HasPos {
		pts[0].Pos, pts[0].HasPos = 0, true
	}
	if !pts[n-1].HasPos {
		pts[n-1].Pos, pts[n-1].HasPos = 1, true
	}
	largest := pts[0].Pos
	for i := range pts {
		if pts[i].HasPos {
			pts[i].Pos = max(pts[i].Pos, largest)
			largest = pts[i].Pos
		}
	}
	for i := 1; i < n; {
		if pts[i].HasPos {
			i++
			continue
		}
		j := i
		for !pts[j].HasPos {
			j++
		}
		lo, hi := pts[i-1].Pos, pts[j].Pos
		for k := i; k < j; k++ {
			pts[k].Pos = lo + (hi-lo)*float64(k-i+1)/float64(j-i+1)
			pts[k].HasPos = true
		}
		i = j
	}
}

// Piecewise returns the linear(...) easing over a copy of pts made
// canonical; see PiecewiseAt.
func Piecewise(pts []LinearPt) Easing {
	c := append([]LinearPt(nil), pts...)
	CanonicalLinear(c)
	return func(t float64) float64 { return PiecewiseAt(c, t) }
}

// PiecewiseAt evaluates the linear(...) easing over canonical points
// at t (CSS Easing 2 §2.1.2): interpolated between the last point at
// or before t and the next, extended past the outermost ones along
// their segment, and the later point's value where two share a
// position. One point is a constant; none is the identity.
func PiecewiseAt(pts []LinearPt, t float64) float64 {
	switch len(pts) {
	case 0:
		return t
	case 1:
		return pts[0].Value
	}
	a := 0
	for i := range pts {
		if pts[i].Pos <= t {
			a = i
		}
	}
	if a == len(pts)-1 {
		a--
	}
	pa, pb := pts[a], pts[a+1]
	if pa.Pos == pb.Pos {
		return pb.Value
	}
	return pa.Value + (t-pa.Pos)/(pb.Pos-pa.Pos)*(pb.Value-pa.Value)
}

// SpringEasing returns a damped-spring easing: the value leaves 0 with
// zero velocity and settles at 1, oscillating on the way when
// underdamped. zeta is the damping ratio - below 1 overshoots (0.3 to
// 0.6 give a lively wobble, 1 is critically damped with none), above 1
// is overdamped and sluggish. omega is the undamped angular frequency
// in radians across the normalized tween: 2*pi crosses one full
// oscillation over the duration, so popovers want roughly 6 to 12. It
// is the fixed-duration curve; Spring is the physics spring that keeps
// its velocity across retargets.
func SpringEasing(zeta, omega float64) Easing {
	return func(t float64) float64 { return SpringEasingAt(zeta, omega, t) }
}

// SpringEasingAt evaluates SpringEasing(zeta, omega) at t without
// building the curve.
func SpringEasingAt(zeta, omega, t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t >= 1:
		return 1
	}
	x, _ := springStep(omega, zeta, -1, 0, t)
	return 1 + x
}

// BounceSpring is the damping ratio and angular frequency of a
// duration-filling spring with the given bounce (0 none, toward 1 a
// lot): damping 1 - bounce, and a frequency whose envelope has decayed
// to a thousandth by the end of the duration, so the curve lands.
func BounceSpring(bounce float64) (zeta, omega float64) {
	zeta = max(0.05, 1-bounce)
	return zeta, 7 / zeta
}

// Bezier is a cubic-bezier curve's control points (x1, y1, x2, y2), as
// data: the theme hands the same curves to stylesheets.
type Bezier [4]float64

// Easing is the curve as an easing function.
func (b Bezier) Easing() Easing { return CubicBezier(b[0], b[1], b[2], b[3]) }

// The emphasized curves, the Material 3 motion pair, and its standard
// curve: an entrance decelerates hard into place, an exit accelerates
// away, and a transform between two resting states eases both ends.
var (
	BezierEmphasizedDecelerate = Bezier{0.05, 0.7, 0.1, 1}
	BezierEmphasizedAccelerate = Bezier{0.3, 0, 0.8, 0.15}
	BezierStandard             = Bezier{0.2, 0, 0, 1}

	// EmphasizedDecelerate brings content in fast and settles it long.
	EmphasizedDecelerate = BezierEmphasizedDecelerate.Easing()
	// EmphasizedAccelerate starts slow and leaves fast: exits.
	EmphasizedAccelerate = BezierEmphasizedAccelerate.Easing()
	// Standard eases a change between two resting states.
	Standard = BezierStandard.Easing()
)

// The motion durations every surface and control share: short for
// state changes (hover, press, a toggle), medium for surfaces opening
// and closing, long for large moves across the screen.
const (
	DurationShort  = 120 * time.Millisecond
	DurationMedium = 220 * time.Millisecond
	DurationLong   = 360 * time.Millisecond
)
