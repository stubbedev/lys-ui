package widget

// The CSS animation runner: the node-side timeline that plays the
// computed animations against the style cache. Each animation counts
// its local time from the instant it was first applied (paused, the
// time holds); every frame samples the pure timing model
// (style.Animation.Sample) and writes the keyframes' effect over the
// cascade's values (style.Keyframes.Apply), the later animation in the
// list on top. A restyle re-syncs the runners — an animation the list
// still names keeps its timeline — and outside an animation's active
// interval without a fill the cascade's own values show.

import (
	"math"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/style"
)

// animDriveSpan bounds one drive tween of an animation whose active
// interval does not end (infinite iterations): the drive relaunches at
// its end.
const animDriveSpan = time.Second

// nodeAnim is a node's animation state, allocated with its first
// animation.
type nodeAnim struct {
	// runs are the animations' timelines in the computed list's order.
	runs []animRunner
	// base is the cascade's values under the animations, refreshed by
	// every restyle: what a frame starts from.
	base style.Values
	// under is the frame scratch: the value beneath the animation
	// being applied.
	under style.Values
	// props are the longhands any running animation writes.
	props style.PropSet
	// drive cancels the tween that ticks the frames; nil when no
	// animation needs another frame.
	drive anim.Cancel
}

// animRunner is one computed animation's timeline.
type animRunner struct {
	a style.Animation
	// start is the instant local time zero sits at; held is the local
	// time a paused animation holds.
	start time.Time
	held  float64
}

// local is the runner's local time at now, in seconds.
func (r *animRunner) local(now time.Time) float64 {
	if !r.a.Running {
		return r.held
	}
	return now.Sub(r.start).Seconds()
}

// syncAnimation aligns the runners with the node's freshly computed
// animation list, matched by name: an animation the list still names
// keeps its timeline (a pause freezes it, a resume continues it), a
// new one starts now, a dropped one stops. It then writes the current
// frame over the cascade's values and drives the next frames.
func (n *node) syncAnimation(v *style.Values) {
	if anim.Instant() || len(v.Animation) == 0 {
		n.stopAnimations()
		return
	}
	now := animclock.Now()
	na := n.anims
	if na == nil {
		na = &nodeAnim{}
		n.anims = na
	}
	runs := make([]animRunner, 0, len(v.Animation))
	na.props = style.PropSet{}
	for _, a := range v.Animation {
		r := animRunner{a: a, start: now}
		if old := na.find(a.Name); old != nil {
			r.start, r.held = old.start, old.held
			switch {
			case old.a.Running && !a.Running:
				r.held = old.local(now)
			case !old.a.Running && a.Running:
				r.start = now.Add(-time.Duration(old.held * float64(time.Second)))
			}
		}
		runs = append(runs, r)
		na.props = na.props.Or(a.Keyframes.Props)
	}
	na.runs = runs
	na.base = *v
	n.animFrame(now)
	n.driveAnimations(now)
}

// find returns the runner playing name, nil when none.
func (na *nodeAnim) find(name string) *animRunner {
	for i := range na.runs {
		if na.runs[i].a.Name == name {
			return &na.runs[i]
		}
	}
	return nil
}

// stopAnimations drops every runner; the caller has just recomputed
// the style cache from the cascade, so nothing needs reverting.
func (n *node) stopAnimations() {
	na := n.anims
	if na == nil {
		return
	}
	if na.drive != nil {
		na.drive()
	}
	n.anims = nil
	n.Invalidate()
}

// animFrame writes the animations' effects at now over the cascade's
// values: each animated longhand starts from base, and every animation
// in effect applies on top of the ones before it, composing onto that
// value.
func (n *node) animFrame(now time.Time) {
	na := n.anims
	na.props.Each(func(p style.Prop) { style.CopyLonghand(&n.cs, &na.base, p) })
	for i := range na.runs {
		r := &na.runs[i]
		progress, before, ok := r.a.Sample(r.local(now))
		if !ok {
			continue
		}
		na.under = n.cs
		r.a.Keyframes.Apply(&n.cs, &na.under, progress, &r.a.Timing, before, r.a.Composition)
	}
	if !na.props.And(style.LayoutProps).Empty() {
		n.InvalidateLayout()
	}
	n.Invalidate()
}

// driveAnimations launches the tween that ticks the frames until the
// last running animation's active interval ends (a span at a time for
// an endless one); paused or finished animations need no frames.
func (n *node) driveAnimations(now time.Time) {
	na := n.anims
	if na.drive != nil {
		na.drive()
		na.drive = nil
	}
	remaining := 0.0
	for i := range na.runs {
		r := &na.runs[i]
		if r.a.Running {
			remaining = max(remaining, r.a.End()-r.local(now))
		}
	}
	if remaining <= 0 {
		return
	}
	span := animDriveSpan
	if !math.IsInf(remaining, 1) {
		span = time.Duration(math.Ceil(remaining * float64(time.Second)))
	}
	na.drive = anim.Play(anim.Animate(span, func(t float64) {
		at := animclock.Now()
		n.animFrame(at)
		if t >= 1 {
			na.drive = nil
			n.driveAnimations(at)
		}
	}).Easing(anim.Linear))
}
