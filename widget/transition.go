package widget

// Transitions (css-transitions-1): a rule's transition-* lists turn a
// changed computed value into a tween over the style cache, every
// other property still applying at once. Each changed longhand the
// property list names transitions with its own entry's duration, delay
// and timing function — the longhands sharing an entry share one
// tween — and interpolates through the one routine the animations use
// (style.Interpolate): smoothly where the value interpolates, and for
// a discrete longhand only under transition-behavior: allow-discrete,
// flipping halfway.

import (
	"math"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

// trGroup is the longhands one transition entry carries.
type trGroup struct {
	timing style.TransitionTiming
	props  style.PropSet
	layout bool
}

// transitionValues starts the transitions a restyle asks for. A
// transition starts from whatever the cache holds — the settled
// values, or an earlier transition's interpolated ones — so a change
// mid-flight turns smoothly. A first style applies at once (nothing
// settled to move from, and an unarranged widget paints nothing yet)
// unless an @starting-style block names the values it starts from.
func (n *node) transitionValues(old, v *style.Values, firstStyle bool) {
	if n.trCancel != nil {
		n.trCancel()
		n.trCancel = nil
	}
	from := old
	fresh := firstStyle
	if fresh && v.Starting != nil {
		from, fresh = v.Starting, false
	}
	if !v.Transition.Active() || anim.Instant() || fresh || (n.bounds.Empty() && v.Starting == nil) {
		return
	}
	n.startTransitions(*from, *v)
}

// startTransitions launches the tweens carrying every changed longhand
// v's transition lists name, from start to end; it takes copies, which
// the tweens keep.
func (n *node) startTransitions(start, end style.Values) {
	var groups []trGroup
	start.Set.Or(end.Set).Each(func(p style.Prop) {
		changed, known := style.Changed(&start, &end, p)
		if !known || !changed {
			return
		}
		tt, ok := end.Transition.For(p)
		if !ok || !style.Smooth(&start, &end, p) && !tt.Discrete {
			return
		}
		layout := layoutKeyProps.Has(p) || paddingMarginBorder.Has(p)
		for i := range groups {
			if groups[i].timing == tt {
				groups[i].props = groups[i].props.With(p)
				groups[i].layout = groups[i].layout || layout
				return
			}
		}
		groups = append(groups, trGroup{timing: tt, props: style.SetOf(p), layout: layout})
	})
	if len(groups) == 0 {
		return
	}
	steps := make([]anim.Step, len(groups))
	for i, g := range groups {
		apply := func(e float64) {
			g.props.Each(func(p style.Prop) { style.Interpolate(&n.cs, &start, &end, p, e) })
			if g.layout {
				n.InvalidateLayout()
			} else {
				n.Invalidate()
			}
		}
		apply(0)
		steps[i] = transitionStep(g.timing, apply)
	}
	n.trCancel = anim.Play(n, anim.Parallel(steps...))
}

// paddingMarginBorder are the box longhands whose change remeasures.
var paddingMarginBorder = style.SetOf(
	style.PropPaddingTop, style.PropPaddingRight, style.PropPaddingBottom, style.PropPaddingLeft,
	style.PropMarginTop, style.PropMarginRight, style.PropMarginBottom, style.PropMarginLeft,
	style.PropBorderTopWidth, style.PropBorderRightWidth, style.PropBorderBottomWidth, style.PropBorderLeftWidth)

// transitionStep is one entry's tween over its delay and duration:
// the timing function maps raw progress over the whole span, held at
// the start through the delay, the CSS fill-before.
func transitionStep(tt style.TransitionTiming, apply func(e float64)) anim.Step {
	dur := time.Duration(tt.Duration * float64(time.Second))
	delay := time.Duration(max(0, tt.Delay) * float64(time.Second))
	total := dur + delay
	skip := time.Duration(max(0, -tt.Delay) * float64(time.Second))
	return anim.Animate(total-min(skip, total), apply).Easing(func(raw float64) float64 {
		if dur <= 0 {
			return 1
		}
		at := float64(raw*float64(total-min(skip, total))) - float64(delay) + float64(skip)
		return tt.Timing.At(math.Min(math.Max(at/float64(dur), 0), 1), false)
	})
}
