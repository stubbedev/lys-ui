package anim

import (
	"math"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/motion"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// fakeClock pins the time animations read and returns a clock with the
// step that moves that time on and ticks the clock there.
func fakeClock(t *testing.T) (*Clock, func(d time.Duration)) {
	t.Helper()
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	c := &Clock{}
	return c, func(d time.Duration) {
		now = now.Add(d)
		c.Tick(now)
	}
}

// A spring runs until it settles, lands exactly on its target and
// stops; an underdamped one overshoots on the way, a critically damped
// one never does.
func TestSpringSettles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		p         SpringParams
		overshoot bool
	}{{"smooth", SpringSmooth, false}, {"snappy", SpringSnappy, true}, {"bouncy", SpringBouncy, true}} {
		t.Run(tc.name, func(t *testing.T) {
			c, step := fakeClock(t)
			frames, peak := 0, 0.0
			s := NewSpring(c, tc.p, []float64{0}, func(pos []float64) { frames++; peak = max(peak, pos[0]) })
			s.SetTarget(100)
			for range 600 {
				if !s.Running() {
					break
				}
				step(FrameInterval)
			}
			if s.Running() || s.Position()[0] != 100 || s.Velocity()[0] != 0 {
				t.Fatalf("after 10s: running %v at %v moving %v", s.Running(), s.Position(), s.Velocity())
			}
			if frames < 5 {
				t.Errorf("%d frames: the spring did not animate", frames)
			}
			if over := peak > 100.01; over != tc.overshoot {
				t.Errorf("peak %.2f: overshoot %v, want %v", peak, over, tc.overshoot)
			}
		})
	}
}

// The closed form is exact: one 120ms step lands where twelve 10ms
// steps do, for every damping regime.
func TestSpringStepIsExact(t *testing.T) {
	for _, p := range []SpringParams{SpringBouncy, SpringSmooth, {Response: 300 * time.Millisecond, Damping: 1.6}} {
		w := springOmega(p)
		x1, v1 := motion.SpringStep(w, p.Damping, -80, 40, 0.12)
		x, v := -80.0, 40.0
		for range 12 {
			x, v = motion.SpringStep(w, p.Damping, x, v, 0.01)
		}
		if math.Abs(x-x1) > 1e-9 || math.Abs(v-v1) > 1e-9 {
			t.Errorf("%+v: one step (%v, %v), twelve steps (%v, %v)", p, x1, v1, x, v)
		}
	}
}

// Retargeting mid-flight keeps the position and the velocity: the
// motion bends toward the new target instead of restarting from rest.
func TestSpringRetargetKeepsMotion(t *testing.T) {
	c, step := fakeClock(t)
	s := NewSpring(c, SpringSnappy, []float64{0, 0}, nil)
	s.SetTarget(100, 50)
	for range 6 {
		step(FrameInterval)
	}
	pos := append([]float64(nil), s.Position()...)
	vel := append([]float64(nil), s.Velocity()...)
	if vel[0] <= 0 {
		t.Fatalf("not moving toward the first target: velocity %v", vel)
	}
	s.SetTarget(-100, 50)
	if p, v := s.Position(), s.Velocity(); p[0] != pos[0] || v[0] != vel[0] || p[1] != pos[1] {
		t.Fatalf("retarget jumped: position %v -> %v, velocity %v -> %v", pos, p, vel, v)
	}
	// A spring restarted from rest at the same spot is what a jerk
	// looks like; the retargeted one is carried past it by its
	// momentum.
	rest := NewSpring(c, SpringSnappy, pos, nil)
	rest.SetTarget(-100, 50)
	step(FrameInterval)
	if s.Position()[0] <= rest.Position()[0] {
		t.Errorf("retargeted spring at %v, one restarted from rest at %v: the momentum was lost", s.Position()[0], rest.Position()[0])
	}
}

// Under reduced motion a spring lands on its target at once.
func TestSpringInstant(t *testing.T) {
	c, _ := fakeClock(t)
	t.Cleanup(SetInstant(true))
	got := 0.0
	s := NewSpring(c, SpringBouncy, []float64{0}, func(pos []float64) { got = pos[0] })
	s.SetTarget(42)
	if s.Running() || got != 42 || s.Position()[0] != 42 {
		t.Errorf("instant spring: running %v, frame %v, position %v", s.Running(), got, s.Position())
	}
}
