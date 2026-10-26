package style

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// scrollRun computes css against one box and returns its values.
func scrollRun(t *testing.T, css string) Values {
	t.Helper()
	var sc Scratch
	s := parseOne(t, css)
	var v Values
	Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &v)
	return v
}

// The scroll-container properties compute: scroll-behavior, the
// scroll-padding sides, scroll-snap-type's axes and strictness,
// overscroll-behavior's axis pair, and the scrollbar trio.
func TestScrollContainerProps(t *testing.T) {
	v := scrollRun(t, `box {
		scroll-behavior: smooth;
		scroll-padding: 1px 2px 3px;
		scroll-snap-type: y mandatory;
		overscroll-behavior: contain none;
		scrollbar-color: #ff0000 #0000ff;
		scrollbar-width: thin;
	}`)
	if !v.ScrollSmooth {
		t.Error("scroll-behavior: smooth did not compute")
	}
	if want := (Sides{Top: 1, Right: 2, Bottom: 3, Left: 2}); v.ScrollPad != want {
		t.Errorf("scroll-padding = %v, want %v", v.ScrollPad, want)
	}
	if !v.Snap.Vertical() || v.Snap.Horizontal() || !v.Snap.Mandatory {
		t.Errorf("scroll-snap-type = %+v, want y mandatory", v.Snap)
	}
	if v.Overscroll[0] != OverscrollContain || v.Overscroll[1] != OverscrollNone {
		t.Errorf("overscroll-behavior = %v, want contain none", v.Overscroll)
	}
	if v.BarThumb != render.RGB(255, 0, 0) || v.BarTrack != render.RGB(0, 0, 255) || v.BarColorAuto {
		t.Errorf("scrollbar-color = %x %x auto=%v, want red over blue", v.BarThumb, v.BarTrack, v.BarColorAuto)
	}
	if v.BarWidth != BarWidthThin {
		t.Errorf("scrollbar-width = %v, want thin", v.BarWidth)
	}

	v = scrollRun(t, `box { scroll-snap-type: inline proximity; scrollbar-color: auto; }`)
	if !v.Snap.Horizontal() || v.Snap.Vertical() || v.Snap.Mandatory {
		t.Errorf("scroll-snap-type inline = %+v, want inline only, proximity", v.Snap)
	}
	if !v.BarColorAuto {
		t.Error("scrollbar-color: auto did not compute")
	}
	v = scrollRun(t, `box { scroll-snap-type: both; overscroll-behavior-y: none; }`)
	if !v.Snap.Vertical() || !v.Snap.Horizontal() || v.Snap.Mandatory {
		t.Errorf("scroll-snap-type both = %+v, want both axes, proximity", v.Snap)
	}
	if v.Overscroll[1] != OverscrollNone || v.Overscroll[0] != OverscrollAuto {
		t.Errorf("overscroll-behavior-y none = %v, want auto none", v.Overscroll)
	}
}

// The scrolled-child properties compute: the scroll-margin sides
// (negatives allowed), scroll-snap-align's one- and two-value forms,
// and scroll-snap-stop.
func TestScrollChildProps(t *testing.T) {
	v := scrollRun(t, `box {
		scroll-margin: -4px 0 2px;
		scroll-snap-align: center;
		scroll-snap-stop: always;
	}`)
	if want := (Sides{Top: -4, Right: 0, Bottom: 2, Left: 0}); v.ScrollMargin != want {
		t.Errorf("scroll-margin = %v, want %v", v.ScrollMargin, want)
	}
	if v.SnapAlign.Block != SnapCenter || v.SnapAlign.Inline != SnapCenter {
		t.Errorf("scroll-snap-align = %+v, want center both", v.SnapAlign)
	}
	if !v.SnapStopAlways {
		t.Error("scroll-snap-stop: always did not compute")
	}
	v = scrollRun(t, `box { scroll-snap-align: start end; }`)
	if v.SnapAlign.Block != SnapStart || v.SnapAlign.Inline != SnapEnd {
		t.Errorf("scroll-snap-align start end = %+v, want block start inline end", v.SnapAlign)
	}
	v = scrollRun(t, `box { scroll-margin-top: 6px; }`)
	if v.ScrollMargin.Top != 6 || v.ScrollMargin.Bottom != 0 {
		t.Errorf("scroll-margin-top = %v, want top only", v.ScrollMargin)
	}
}

// scrollbar-color inherits; scrollbar-width and the container group
// do not (CSS Scrollbars 1).
func TestScrollbarPropsInherit(t *testing.T) {
	s := parseOne(t, `box { scrollbar-color: #101010 #202020; scrollbar-width: thin; scroll-behavior: smooth; }`)
	var sc Scratch
	var root, child Values
	Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &root)
	Compute([]Layer{{Sheet: s}}, el("kid"), &root, nil, Env{}, &sc, &child)
	if !child.Has(PropScrollbarColor) || child.BarThumb != render.RGB(0x10, 0x10, 0x10) {
		t.Error("scrollbar-color did not inherit")
	}
	if child.Has(PropScrollbarWidth) {
		t.Error("scrollbar-width inherited")
	}
	if child.Has(PropScrollBehavior) {
		t.Error("scroll-behavior inherited")
	}
}

// steps() takes its jump position, and linear() parses its point list
// with optional percent stops, wherever a timing function goes.
func TestTimingStepsAndLinear(t *testing.T) {
	timing := func(v Values) Timing { return listAt(v.Transition.Timings, 0, TimingEase) }
	v := scrollRun(t, `box { transition: opacity 1s steps(3, jump-start); }`)
	if tm := timing(v); tm.Steps != 3 || tm.StepPos != anim.JumpStart {
		t.Errorf("steps timing = %+v, want steps(3, jump-start)", tm)
	}
	v = scrollRun(t, `box { transition-timing-function: steps(2, jump-both); }`)
	if tm := timing(v); tm.Steps != 2 || tm.StepPos != anim.JumpBoth {
		t.Errorf("jump-both timing = %+v", tm)
	}
	v = scrollRun(t, `box { transition-timing-function: steps(4); }`)
	if tm := timing(v); tm.StepPos != anim.JumpEnd {
		t.Errorf("bare steps = %+v, want the jump-end default", tm)
	}
	v = scrollRun(t, `box { transition: opacity 1s linear(0, 0.5 25%, 1); }`)
	tm := timing(v)
	if tm.LinN != 3 || tm.Lin[0].Pos != 0 || tm.Lin[1].Pos != 0.25 || tm.Lin[2].Pos != 1 || tm.Lin[2].Value != 1 {
		t.Errorf("linear() timing = %+v, want 0@0%%, 0.5@25%%, 1@100%%", tm)
	}
	v = scrollRun(t, `@keyframes k { to { opacity: 1; } } box { animation: k 1s linear(1 10%, 0 90%, 1); }`)
	if first(v).Timing.LinN != 3 || first(v).Timing.Lin[0].Pos != 0.1 || first(v).Timing.Lin[1].Pos != 0.9 {
		t.Errorf("animation linear() = %+v", first(v).Timing)
	}
}

// transition-behavior parses alone and inside the shorthand.
func TestTransitionBehavior(t *testing.T) {
	discrete := func(v Values) bool {
		tt, ok := v.Transition.For(PropOpacity)
		return ok && tt.Discrete
	}
	v := scrollRun(t, `box { transition-duration: 100ms; transition-behavior: allow-discrete; }`)
	if !discrete(v) {
		t.Error("transition-behavior: allow-discrete did not compute")
	}
	v = scrollRun(t, `box { transition: opacity 100ms allow-discrete; }`)
	if tt, _ := v.Transition.For(PropOpacity); !tt.Discrete || tt.Duration != 0.1 {
		t.Errorf("shorthand = %+v, want 100ms allow-discrete", v.Transition)
	}
	v = scrollRun(t, `box { transition: opacity 100ms allow-discrete; transition-behavior: normal; }`)
	if discrete(v) {
		t.Error("the longhand did not reset the shorthand")
	}
}

// The timing functions evaluate per CSS Easing 2: the keywords, steps
// with every jump position and the before flag, and linear().
func TestTimingAt(t *testing.T) {
	for _, c := range []struct {
		css    string
		at     float64
		before bool
		want   float64
	}{
		{"ease", 0.5, false, 0.8024033877399112},
		{"step-start", 0, false, 1},
		{"step-end", 0.99, false, 0},
		{"steps(4, jump-start)", 0, true, 0},
		{"steps(4, jump-start)", 0, false, 0.25},
		{"steps(4, jump-none)", 0.5, false, 2.0 / 3},
		{"steps(4, jump-both)", 0.3, false, 0.4},
		{"linear(0, 0.25, 1)", 0.25, false, 0.125},
		{"linear(0, 0.5 25% 75%, 1)", 0.5, false, 0.5},
		{"linear(0, 1)", 1.5, false, 1.5},
	} {
		var cx ctx
		tm, ok := timingOf(tokenize(c.css), &cx)
		if !ok {
			t.Errorf("%s did not parse", c.css)
			continue
		}
		if got := tm.At(c.at, c.before); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s at %v (before %v) = %v, want %v", c.css, c.at, c.before, got, c.want)
		}
	}
	for _, bad := range []string{"steps(1, jump-none)", "steps(2.5)", "steps(0)", "cubic-bezier(1.5, 0, 0, 1)", "linear(0.5)", "bogus"} {
		var cx ctx
		if _, ok := timingOf(tokenize(bad), &cx); ok {
			t.Errorf("%s parsed", bad)
		}
	}
}

// animation-composition parses as its own longhand list, zips onto the
// group, and stays out of the shorthand, which neither takes nor
// resets it (css-animations-2 §3).
func TestAnimationComposition(t *testing.T) {
	const kf = `@keyframes k { to { opacity: 0; } } `
	v := scrollRun(t, kf+`box { animation-composition: add; animation: k 1s; }`)
	if first(v).Composition != CompAdd {
		t.Errorf("composition before the shorthand = %v, want add kept", first(v).Composition)
	}
	v = scrollRun(t, kf+`box { animation: k 1s, k 2s; animation-composition: accumulate, replace; }`)
	if v.Animation[0].Composition != CompAccumulate || v.Animation[1].Composition != CompReplace {
		t.Errorf("longhand compositions = %+v, want accumulate, replace", v.Animation)
	}
	warns := capture(t)
	Parse(kf + `box { animation: k 1s add; }`)
	if len(*warns) != 1 {
		t.Errorf("a composition keyword in the shorthand: %d warnings, want 1", len(*warns))
	}
}

// @starting-style parses inside a rule and computes as the entry
// transition's values; a rule without one computes none.
func TestStartingStyle(t *testing.T) {
	v := scrollRun(t, `box {
		opacity: 1;
		transition: opacity 200ms;
		@starting-style { opacity: 0; background-color: #00ff00; }
	}`)
	if v.Starting == nil {
		t.Fatal("the @starting-style block did not compute")
	}
	if !v.Starting.Has(PropOpacity) || v.Starting.Opacity != 0 {
		t.Error("the starting opacity did not compute")
	}
	if !v.Starting.Has(PropBackgroundColor) || v.Starting.Background != render.RGB(0, 255, 0) {
		t.Error("the starting background did not compute")
	}
	if v.Starting.Has(PropColor) {
		t.Error("the starting block declared color")
	}
	v = scrollRun(t, `box { opacity: 1; }`)
	if v.Starting != nil {
		t.Error("a rule without @starting-style computed a starting style")
	}
}
