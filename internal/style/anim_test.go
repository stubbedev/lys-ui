package style

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// affineNear compares two transforms' matrices entrywise within tol.
func affineNear(a, b Xform, tol float64) bool {
	return affineM(a.M, b.M, tol)
}

func affineM(a, b render.Affine, tol float64) bool {
	for _, pair := range [][2]float64{{a.A, b.A}, {a.B, b.B}, {a.C, b.C}, {a.D, b.D}, {a.E, b.E}, {a.F, b.F}} {
		if d := pair[0] - pair[1]; d > tol || d < -tol {
			return false
		}
	}
	return true
}

// first returns the shorthand's first animation.
func first(v Values) Animation {
	if len(v.Animation) == 0 {
		return Animation{}
	}
	return v.Animation[0]
}

// A @keyframes rule compiles into the sheet: stops in offset order,
// from/to mapped, percentage groups expanded, each stop's declarations
// computed, and the animation longhands dropped.
func TestKeyframesParse(t *testing.T) {
	s := parseOne(t, `@keyframes pulse {
		from { opacity: 1; }
		50%, 60% { opacity: 0.5; -gtk-icon-transform: rotate(90deg); }
		to { opacity: 0.25; -gtk-icon-transform: rotate(0.5turn); }
	}`)
	kf := s.Keyframes("pulse")
	if kf == nil {
		t.Fatal("the sheet lost the pulse keyframes")
	}
	if len(kf.Frames) != 4 {
		t.Fatalf("frames %d, want 4 (the 50/60 group expands)", len(kf.Frames))
	}
	if kf.Frames[0].Offset != 0 || kf.Frames[3].Offset != 1 {
		t.Errorf("endpoints %v %v, want from 0 and to 1", kf.Frames[0].Offset, kf.Frames[3].Offset)
	}
	if !kf.Frames[0].Props.Has(PropOpacity) || kf.Frames[0].V.Opacity != 1 {
		t.Error("the from stop lost its opacity")
	}
	if !kf.Frames[1].Props.Has(PropIconTransform) || !affineNear(kf.Frames[1].V.IconXform, Xform{M: render.Rotate(90)}, 1e-9) {
		t.Error("the 50% stop icon transform, want 90deg")
	}
	if !kf.Frames[3].Props.Has(PropIconTransform) || !affineNear(kf.Frames[3].V.IconXform, Xform{M: render.Rotate(180)}, 1e-9) {
		t.Error("the to stop icon transform, want 0.5turn = 180")
	}
	if kf.Props != SetOf(PropOpacity, PropIconTransform) {
		t.Errorf("the rule animates %v, want opacity and the icon transform only", kf.Props)
	}
	if s.Keyframes("absent") != nil {
		t.Error("an unknown name resolved")
	}
}

// The animation shorthand parses in the orders the stylesheet writes
// and resolves its keyframes through the layers; the longhand lists
// zip against the names, and `none` clears the group.
func TestAnimationShorthand(t *testing.T) {
	var sc Scratch
	run := func(css string) Values {
		s := Parse(`@keyframes a1 { to { opacity: 0; } } @keyframes a2 { to { opacity: 1; } } ` + css)
		var root, v Values
		Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &root)
		Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &v)
		return v
	}
	v := run(`@keyframes spin { from { -gtk-icon-transform: rotate(0deg); } to { -gtk-icon-transform: rotate(360deg); } }
		box { animation: spin var(--d, 1s) steps(20) infinite alternate; }`)
	a := first(v)
	if a.Name != "spin" || a.Keyframes == nil || len(a.Keyframes.Frames) != 2 {
		t.Fatalf("animation %+v, want spin resolved to its keyframes", a)
	}
	if a.Duration != 1 || !math.IsInf(a.Iterations, 1) || a.Direction != DirAlternate || a.Timing.Steps != 20 || !a.Running {
		t.Errorf("shorthand fields %+v, want 1s steps(20) infinite alternate", a)
	}

	// The defaults: ease, one iteration, normal, no fill, running.
	if a := first(run(`box { animation: a1 1s; }`)); a.Timing != TimingEase || a.Iterations != 1 ||
		a.Direction != DirNormal || a.Fill != FillNone || !a.Running {
		t.Errorf("shorthand defaults %+v", a)
	}

	// A comma list runs both entries.
	v = run(`box { animation: a1 1s linear, a2 2s reverse; }`)
	if len(v.Animation) != 2 || v.Animation[0].Name != "a1" || v.Animation[1].Name != "a2" ||
		v.Animation[1].Direction != DirReverse {
		t.Errorf("the comma list %+v, want a1 and a2 reverse", v.Animation)
	}

	// The shorthand's second time is the delay; the longhand takes
	// negatives.
	if d := first(run(`box { animation: a1 1s 0.5s; }`)).Delay; d != 0.5 {
		t.Errorf("shorthand delay %v, want 0.5s", d)
	}
	if d := first(run(`box { animation: a1 1s; animation-delay: -0.25s; }`)).Delay; d != -0.25 {
		t.Errorf("delay longhand %v, want -0.25s", d)
	}
	// Fill modes and directions parse.
	if f := first(run(`box { animation: a1 1s both; }`)).Fill; f != FillBoth {
		t.Errorf("fill %v, want both", f)
	}
	if d := first(run(`box { animation: a1 1s alternate-reverse; }`)).Direction; d != DirAlternateReverse {
		t.Errorf("direction %v, want alternate-reverse", d)
	}

	v = run(`box { animation: a1 2s linear infinite; animation-play-state: paused; }`)
	if first(v).Duration != 2 || first(v).Running {
		t.Errorf("play-state did not merge: %+v", first(v))
	}

	if v := run(`box { animation: a1 1s infinite; animation: none; }`); len(v.Animation) != 0 {
		t.Errorf("animation: none left %+v", v.Animation)
	}
	if v := run(`box { animation: nosuch 1s infinite; }`); len(v.Animation) != 0 {
		t.Errorf("an unknown keyframes name resolved: %+v", v.Animation)
	}

	// The longhand lists zip against the names, a shorter one
	// repeating; a delay declared before the shorthand is reset by it.
	v = run(`box { animation-delay: 5s; animation: a1 1s, a2 2s, a1 3s; animation-duration: 4s, 6s;
		animation-iteration-count: 2; animation-timing-function: step-end; }`)
	if len(v.Animation) != 3 || v.Animation[2].Duration != 4 || v.Animation[1].Duration != 6 ||
		v.Animation[1].Delay != 0 || v.Animation[2].Iterations != 2 || v.Animation[0].Timing.Steps != 1 {
		t.Errorf("zipped lists %+v", v.Animation)
	}
}

// Keyframes interpolate each longhand between the stops that declare
// it, the underlying value standing in at 0 and 1 where no stop does,
// each interval eased by its starting stop's timing function.
func TestKeyframesApply(t *testing.T) {
	s := parseOne(t, `@keyframes k {
		from { opacity: 1; }
		50% { opacity: 0.4; -gtk-icon-transform: rotate(0deg); animation-timing-function: steps(2, jump-start); }
		to { -gtk-icon-transform: rotate(180deg); }
	}`)
	kf := s.Keyframes("k")
	under := initialValues()
	under.Opacity, under.Set = 0.2, SetOf(PropOpacity)
	at := func(p float64) Values {
		v := under
		kf.Apply(&v, &under, p, &TimingLinear, false, CompReplace)
		return v
	}
	if v := at(0.25); math.Abs(v.Opacity-0.7) > 1e-9 {
		t.Errorf("at 0.25: opacity %v, want halfway 1 to 0.4", v.Opacity)
	}
	// The 50% stop's steps(2, jump-start) eases the opacity's interval
	// to the synthesized 100% stop: at 0.6 it already took a step.
	if v := at(0.6); math.Abs(v.Opacity-0.3) > 1e-9 {
		t.Errorf("at 0.6: opacity %v, want 0.4 halfway to the underlying 0.2", v.Opacity)
	}
	// The icon transform runs from its 50% stop under the stop's steps:
	// at 0.6 that is the first step, halfway round.
	if v := at(0.6); !affineNear(v.IconXform, Xform{M: render.Rotate(90)}, 1e-6) {
		t.Errorf("at 0.6: icon transform %v, want a quarter turn", v.IconXform.M)
	}
	// The transform turns, not shrinks: halfway 0deg to 180deg maps
	// (1,0) up, not through a collapsed matrix.
	ts := parseOne(t, `@keyframes r { from { transform: rotate(0deg); } to { transform: rotate(180deg); } }`)
	v := initialValues()
	ts.Keyframes("r").Apply(&v, &v, 0.5, &TimingLinear, false, CompReplace)
	px, py := v.Transform.M.Apply(1, 0)
	if math.Abs(px) > 1e-9 || math.Abs(py-1) > 1e-9 {
		t.Errorf("mid transform maps (1,0) to (%v,%v), want (0,1)", px, py)
	}
}

// Sample walks the timing model: phases and fills, iterations,
// directions, and the end of the active interval.
func TestAnimationSample(t *testing.T) {
	for _, c := range []struct {
		name string
		a    Animation
		t, p float64
		ok   bool
	}{
		{"before without fill", Animation{Duration: 1, Iterations: 1, Delay: 1}, 0.5, 0, false},
		{"before with backwards", Animation{Duration: 1, Iterations: 1, Delay: 1, Fill: FillBackwards}, 0.5, 0, true},
		{"active", Animation{Duration: 2, Iterations: 1}, 0.5, 0.25, true},
		{"negative delay", Animation{Duration: 1, Iterations: 1, Delay: -0.3}, 0, 0.3, true},
		{"second iteration", Animation{Duration: 1, Iterations: 3}, 1.25, 0.25, true},
		{"alternate odd", Animation{Duration: 1, Iterations: 3, Direction: DirAlternate}, 1.25, 0.75, true},
		{"alternate-reverse even", Animation{Duration: 1, Iterations: 3, Direction: DirAlternateReverse}, 0.25, 0.75, true},
		{"after without fill", Animation{Duration: 1, Iterations: 2}, 2.5, 0, false},
		{"after forwards ends at 1", Animation{Duration: 1, Iterations: 2, Fill: FillForwards}, 2.5, 1, true},
		{"after forwards alternate ends reversed", Animation{Duration: 1, Iterations: 2, Fill: FillForwards, Direction: DirAlternate}, 2.5, 0, true},
		{"fractional iterations end mid-way", Animation{Duration: 1, Iterations: 1.5, Fill: FillForwards}, 5, 0.5, true},
		{"infinite", Animation{Duration: 1, Iterations: math.Inf(1)}, 7.25, 0.25, true},
	} {
		p, _, ok := c.a.Sample(c.t)
		if ok != c.ok || ok && math.Abs(p-c.p) > 1e-9 {
			t.Errorf("%s: progress %v ok %v, want %v %v", c.name, p, ok, c.p, c.ok)
		}
	}
}
