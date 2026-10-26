package style

// CSS animations (css-animations-1, web-animations-1): @keyframes
// rules, the animation-* longhands and their shorthand. A keyframes
// rule declares stops over an iteration's progress, each a set of
// computed declarations; a node's animation-name list points it at
// rules, the other longhands' lists give each one its duration, delay,
// timing, iteration count, direction, fill, play state and
// composition. Every longhand a stop declares animates: smoothly where
// it interpolates, discretely otherwise (interp.go). The widget layer
// owns the timeline and samples the pure functions here.

import (
	"math"
	"math/bits"
	"slices"
	"sort"
)

// Keyframes is one @keyframes rule: its stops in offset order, and the
// longhands any of them declares.
type Keyframes struct {
	Name   string
	Frames []Keyframe
	Props  PropSet
}

// Keyframe is one stop of a keyframes rule: its offset in the
// iteration, the longhands it declares with their computed values,
// and its own animation-timing-function, which eases the interval
// from it to the next stop (nil: the animation's).
type Keyframe struct {
	Offset float64
	Props  PropSet
	V      *Values
	Timing *Timing
}

// Animation is one computed animation: which @keyframes rule runs, for
// how long after what delay, along which curve, how many times, in
// which direction, what holds outside the active interval, whether it
// plays, and how its effect composes onto the underlying style.
type Animation struct {
	Name     string
	Duration float64 // seconds
	Delay    float64 // seconds; negative starts mid-flight
	Timing   Timing
	// Iterations is the iteration count, +Inf for infinite.
	Iterations float64
	Direction  uint8 // DirNormal..DirAlternateReverse
	Fill       uint8 // FillNone..FillBoth
	Running    bool
	// Composition is animation-composition: how a frame joins the
	// underlying value (CompReplace, CompAdd, CompAccumulate).
	Composition uint8
	Keyframes   *Keyframes
}

// The animation-direction values.
const (
	DirNormal uint8 = iota
	DirReverse
	DirAlternate
	DirAlternateReverse
)

// The animation-fill-mode values.
const (
	FillNone uint8 = iota
	FillForwards
	FillBackwards
	FillBoth
)

// The animation-composition values.
const (
	CompReplace uint8 = iota
	CompAdd
	CompAccumulate
)

// The keyword lists of the one-keyword animation longhands.
var (
	animDirections = map[string]uint8{
		"normal": DirNormal, "reverse": DirReverse,
		"alternate": DirAlternate, "alternate-reverse": DirAlternateReverse,
	}
	animFills = map[string]uint8{
		"none": FillNone, "forwards": FillForwards, "backwards": FillBackwards, "both": FillBoth,
	}
	animCompositions = map[string]uint8{
		"replace": CompReplace, "add": CompAdd, "accumulate": CompAccumulate,
	}
)

// AnimLists are the animation-* longhands as declared, one list each;
// Compute zips them against the names into Values.Animation. A nil
// list is the longhand's initial value. An empty name is `none`.
type AnimLists struct {
	Names        []string
	Durations    []float64
	Timings      []Timing
	Iterations   []float64
	Directions   []uint8
	Running      []bool
	Delays       []float64
	Fills        []uint8
	Compositions []uint8
}

// Equal reports whether two longhand groups hold the same lists.
func (l *AnimLists) Equal(o *AnimLists) bool {
	return slices.Equal(l.Names, o.Names) && slices.Equal(l.Durations, o.Durations) &&
		slices.Equal(l.Timings, o.Timings) && slices.Equal(l.Iterations, o.Iterations) &&
		slices.Equal(l.Directions, o.Directions) && slices.Equal(l.Running, o.Running) &&
		slices.Equal(l.Delays, o.Delays) && slices.Equal(l.Fills, o.Fills) &&
		slices.Equal(l.Compositions, o.Compositions)
}

// animations zips the lists into the computed animations
// (css-animations-1 §3): one per name, each other list matched by
// index and repeating, a name no layer's @keyframes rule defines
// running nothing.
func (l *AnimLists) animations(layers []Layer) []Animation {
	var out []Animation
	for i, name := range l.Names {
		if name == "" {
			continue
		}
		kf := keyframesFor(name, layers)
		if kf == nil {
			continue
		}
		out = append(out, Animation{
			Name: name, Keyframes: kf,
			Duration:    listAt(l.Durations, i, 0),
			Timing:      listAt(l.Timings, i, TimingEase),
			Iterations:  listAt(l.Iterations, i, 1),
			Direction:   listAt(l.Directions, i, DirNormal),
			Running:     listAt(l.Running, i, true),
			Delay:       listAt(l.Delays, i, 0),
			Fill:        listAt(l.Fills, i, FillNone),
			Composition: listAt(l.Compositions, i, CompReplace),
		})
	}
	return out
}

// keyframesFor is the @keyframes rule called name in the
// highest-priority layer that declares one.
func keyframesFor(name string, layers []Layer) *Keyframes {
	for _, layer := range slices.Backward(layers) {
		if layer.Sheet == nil {
			continue
		}
		if kf := layer.Sheet.Keyframes(name); kf != nil {
			return kf
		}
	}
	return nil
}

// Sample is the animation's state at local time t, the seconds since
// it was applied (web-animations-1 §4.6-§4.9): its directed progress
// within the current iteration and whether it was sampled from the
// before phase, the before flag the step functions read. ok is false
// where the animation has no effect — before or after its active
// interval without a fill covering that side. The timing function is
// not applied here: CSS animations ease each keyframe interval.
func (a *Animation) Sample(t float64) (progress float64, before, ok bool) {
	active := 0.0
	if a.Duration > 0 && a.Iterations > 0 {
		active = a.Duration * a.Iterations
	}
	var at float64
	switch {
	case t < a.Delay:
		if a.Fill != FillBackwards && a.Fill != FillBoth {
			return 0, false, false
		}
		before = true
	case t < a.Delay+active:
		at = t - a.Delay
	default:
		if a.Fill != FillForwards && a.Fill != FillBoth {
			return 0, false, false
		}
		at = active
	}
	overall := 0.0
	switch {
	case a.Duration > 0:
		overall = at / a.Duration
	case !before:
		overall = a.Iterations
	}
	simple := overall - math.Floor(overall)
	if math.IsInf(overall, 1) {
		simple = 0
	}
	iter := math.Floor(overall)
	if simple == 0 && overall != 0 && at == active && !before {
		simple, iter = 1, iter-1
	}
	reverse := false
	switch a.Direction {
	case DirReverse:
		reverse = true
	case DirAlternate:
		reverse = math.Mod(iter, 2) == 1
	case DirAlternateReverse:
		reverse = math.Mod(iter, 2) == 0
	}
	if reverse {
		simple = 1 - simple
	}
	return simple, before, true
}

// End is the local time the animation's active interval ends at,
// +Inf for an infinite one.
func (a *Animation) End() float64 {
	if a.Duration <= 0 || a.Iterations <= 0 {
		return a.Delay
	}
	return a.Delay + a.Duration*a.Iterations
}

// Apply writes the rule's effect at iteration progress into dst for
// every longhand the rule animates, eased per keyframe interval
// (css-animations-1 §3.2) and composed onto under — the value beneath
// the animation — by comp. A longhand missing from the 0% or 100% stop
// takes under's value there.
func (kf *Keyframes) Apply(dst, under *Values, progress float64, timing *Timing, before bool, comp uint8) {
	for i, word := range kf.Props.words() {
		for ; word != 0; word &= word - 1 {
			p := propAt(i, bits.TrailingZeros64(word))
			kf.applyProp(dst, under, p, progress, timing, before)
			if comp != CompReplace {
				Compose(dst, under, p)
			}
		}
	}
}

// kfStop is one stop of a longhand's property-specific keyframes; a
// nil v is a stop synthesized from the underlying value.
type kfStop struct {
	off float64
	v   *Values
	tm  *Timing
}

// applyProp interpolates one longhand over its property-specific
// keyframes (web-animations-1 §5.3.3): the stops declaring it, with
// under synthesized at 0 and 1 where none does; the interval is the
// last stop at or before progress and the one after it.
func (kf *Keyframes) applyProp(dst, under *Values, p Prop, progress float64, timing *Timing, before bool) {
	var buf [16]kfStop
	stops := buf[:0]
	for i := range kf.Frames {
		f := &kf.Frames[i]
		if !f.Props.Has(p) {
			continue
		}
		tm := timing
		if f.Timing != nil {
			tm = f.Timing
		}
		stops = append(stops, kfStop{off: f.Offset, v: f.V, tm: tm})
	}
	if len(stops) == 0 || stops[0].off != 0 {
		stops = slices.Insert(stops, 0, kfStop{off: 0, tm: timing})
	}
	if stops[len(stops)-1].off != 1 {
		stops = append(stops, kfStop{off: 1, tm: timing})
	}
	i := 0
	for k := range len(stops) - 1 {
		if stops[k].off <= progress {
			i = k
		}
	}
	a, b := stops[i], stops[i+1]
	local := 1.0
	if b.off > a.off {
		local = (progress - a.off) / (b.off - a.off)
	}
	av, bv := a.v, b.v
	if av == nil {
		av = under
	}
	if bv == nil {
		bv = under
	}
	Interpolate(dst, av, bv, p, a.tm.At(local, before))
}

// parseAnimation parses the animation shorthand (css-animations-1
// §3.11), one entry per comma group, each a flexible-order run of a
// duration, a delay (the second time), a timing function, an iteration
// count, a direction, a fill mode, a play state and the name (an ident
// none of the keywords claims). Every longhand list takes one entry per
// group; animation-composition is not part of the shorthand.
func parseAnimation(ts []token, cx *ctx, v *Values) bool {
	var l AnimLists
	for _, grp := range splitTop(ts, tkComma) {
		if !l.addShorthand(grp, cx) {
			return false
		}
	}
	l.Compositions = v.Anim.Compositions
	v.Anim = l
	return true
}

// addShorthand parses one comma group of the shorthand onto every
// list.
func (l *AnimLists) addShorthand(ts []token, cx *ctx) bool {
	var times []float64
	name, named := "", false
	timing, haveT := TimingEase, false
	iters, haveI := 1.0, false
	dir, haveDir := DirNormal, false
	fill, haveFill := FillNone, false
	running, haveRun := true, false
	for _, part := range components(ts) {
		if secs, ok := timeOf(part, cx); ok && len(times) < 2 {
			times = append(times, secs)
			continue
		}
		if tm, ok := timingOf(part, cx); ok && !haveT {
			timing, haveT = tm, true
			continue
		}
		if n, ok := iterationOf(part, cx); ok && !haveI {
			iters, haveI = n, true
			continue
		}
		s, ok := identOf([][]token{part})
		if !ok {
			return false
		}
		if d, ok := animDirections[s]; ok && !haveDir && s != "normal" {
			dir, haveDir = d, true
			continue
		}
		if f, ok := animFills[s]; ok && !haveFill && s != "none" {
			fill, haveFill = f, true
			continue
		}
		if s == "running" || s == "paused" {
			if haveRun {
				return false
			}
			running, haveRun = s == "running", true
			continue
		}
		if named {
			return false
		}
		named = true
		if s != "none" {
			name = s
		}
	}
	if len(times) > 0 && times[0] < 0 {
		return false
	}
	delay := 0.0
	if len(times) == 2 {
		delay = times[1]
	}
	l.Names = append(l.Names, name)
	l.Durations = append(l.Durations, listAt(times, 0, 0))
	l.Delays = append(l.Delays, delay)
	l.Timings = append(l.Timings, timing)
	l.Iterations = append(l.Iterations, iters)
	l.Directions = append(l.Directions, dir)
	l.Fills = append(l.Fills, fill)
	l.Running = append(l.Running, running)
	return true
}

// iterationOf reads one animation-iteration-count value: infinite or
// a non-negative number.
func iterationOf(ts []token, cx *ctx) (float64, bool) {
	if s, ok := identOf([][]token{ts}); ok {
		return math.Inf(1), s == "infinite"
	}
	n, ok := evalNumeric(ts, cx)
	return n.v, ok && n.kind == numNumber && n.v >= 0
}

// parseAnimationIterations parses animation-iteration-count's list.
func parseAnimationIterations(ts []token, cx *ctx, v *Values) bool {
	var out []float64
	for _, item := range splitTop(ts, tkComma) {
		n, ok := iterationOf(trimWS(item), cx)
		if !ok {
			return false
		}
		out = append(out, n)
	}
	v.Anim.Iterations = out
	return true
}

// parseAnimationPlayState parses animation-play-state's list of
// running and paused.
func parseAnimationPlayState(ts []token, _ *ctx, v *Values) bool {
	var out []bool
	for _, item := range splitTop(ts, tkComma) {
		s, ok := identOf(components(item))
		if !ok || s != "running" && s != "paused" {
			return false
		}
		out = append(out, s == "running")
	}
	v.Anim.Running = out
	return true
}

// parseAnimationName parses animation-name's list of names and none.
func parseAnimationName(ts []token, _ *ctx, v *Values) bool {
	var out []string
	for _, item := range splitTop(ts, tkComma) {
		s, ok := identOf(components(item))
		if !ok {
			return false
		}
		if s == "none" {
			s = ""
		}
		out = append(out, s)
	}
	v.Anim.Names = out
	return true
}

// sortFrames orders a keyframes rule's stops by offset, stable so
// equal offsets keep source order (the later wins), and records the
// longhands the stops declare.
func sortFrames(kf *Keyframes) {
	sort.SliceStable(kf.Frames, func(i, j int) bool { return kf.Frames[i].Offset < kf.Frames[j].Offset })
	kf.Props = PropSet{}
	for i := range kf.Frames {
		kf.Props = kf.Props.Or(kf.Frames[i].Props)
	}
}
