package widget

// The CSS feature set the transform and animation rework landed:
// transitions over every animatable channel, keyframes over the
// transform and color channels, the animation directions, fills, and
// delays, and the transform's full primitive set painting through the
// affine.

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A rotate transition turns the box's subtree: mid-flight the
// composite is neither the identity nor the settled turn, and the
// tween's primitive walk keeps the shape a square.
func TestTransformTransitionRotates(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `box { transition: transform 100ms linear; }
		box.hot { transform: rotate(90deg); }`)
	b := NewBox(Column, 0, 0)
	b.AddClass("pane")
	frame(t, b, 40, 40)
	b.AddClass("hot")
	b.style(b)
	c.step()
	x := b.style(b).Transform
	if x.M == render.Identity {
		t.Fatal("mid-flight rotate is the identity")
	}
	// Still a turn: the columns stay orthonormal, part-way round.
	if math.Abs(math.Hypot(x.M.A, x.M.B)-1) > 1e-6 {
		t.Errorf("mid-flight matrix %v lost the rotation", x.M)
	}
	for c.step() {
	}
	if !affineEq(b.style(b).Transform.M, render.Rotate(90)) {
		t.Errorf("settled matrix %v, want rotate(90deg)", b.style(b).Transform.M)
	}
}

// A dropping declaration eases back to the identity, the implicit to;
// once settled the recompute holds the undeclared zero, which paints
// as the identity.
func TestTransformTransitionRevertsToIdentity(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `box { transform: scale(2); transition: transform 50ms linear; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 40, 40)
	loadCSS(t, `box { transition: transform 50ms linear; }`)
	// The restyle starts the transition at its start value.
	if m := b.style(b).Transform.M; !affineEq(m, render.Scale(2, 2)) {
		t.Fatalf("the transition starts at %v, want the scale(2) it leaves", m)
	}
	for c.step() {
	}
	m := b.style(b).Transform.M
	if m != render.Identity && m != (render.Affine{}) {
		t.Errorf("after the drop the transform is %v, want the identity", m)
	}
}

// Keyframes animate the background-color channel.
func TestKeyframesAnimateBackground(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes wash {
		from { background-color: #000000; }
		to { background-color: #ffffff; }
	}
	box { animation: wash 100ms linear infinite; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 40, 40)
	c.step()
	got := b.style(b).Background
	if got == 0xff000000 || got == 0xffffffff {
		t.Errorf("mid-flight background %v, want a blend", got)
	}
}

// Fill-mode forwards holds the final frame after the iterations end;
// without it the cascade returns.
func TestAnimationFillForwardsHolds(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes dim { from { opacity: 1; } to { opacity: 0.25; } }
		box { opacity: 1; animation: dim 50ms linear 1 forwards; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 40, 40)
	for c.step() {
	}
	if got := b.style(b).Opacity; got != 0.25 {
		t.Fatalf("filled opacity %v, want the final 0.25", got)
	}
	// Dropping the animation hands the channel back to the cascade.
	loadCSS(t, `@keyframes dim { from { opacity: 1; } to { opacity: 0.25; } }
		box { opacity: 1; }`)
	_ = b.style(b)
	if got := b.style(b).Opacity; got != 1 {
		t.Errorf("after the drop the opacity is %v, want the cascade's 1", got)
	}
}

// A negative delay starts the animation mid-flight.
func TestAnimationNegativeDelayStartsMidFlight(t *testing.T) {
	pinAnimClock(t)
	loadCSS(t, `@keyframes dim { from { opacity: 1; } to { opacity: 0; } }
		box { animation: dim 100ms linear -50ms; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 40, 40)
	if got := b.style(b).Opacity; got != 0.5 {
		t.Errorf("opacity after a -50ms delay %v, want the midpoint 0.5", got)
	}
}

// The reverse direction plays the keyframes backwards: one frame in,
// the phase reads from the far end.
func TestAnimationReverseDirection(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes dim { from { opacity: 1; } to { opacity: 0; } }
		box { animation: dim 100ms linear reverse; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 40, 40)
	c.step()
	if got := b.style(b).Opacity; mathAbs(got-0.1666) > 0.01 {
		t.Errorf("reversed opacity after one frame %v, want ~0.167", got)
	}
}

// affineEq compares two affines entrywise.
func affineEq(a, b render.Affine) bool {
	for _, pair := range [][2]float64{{a.A, b.A}, {a.B, b.B}, {a.C, b.C}, {a.D, b.D}, {a.E, b.E}, {a.F, b.F}} {
		if d := pair[0] - pair[1]; d > 1e-9 || d < -1e-9 {
			return false
		}
	}
	return true
}

func mathAbs(x float64) float64 {
	return math.Abs(x)
}
