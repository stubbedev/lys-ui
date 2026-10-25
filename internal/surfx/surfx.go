// Package surfx is the surface-animation coordinator: one state
// machine every transient surface (menus, tooltips, toasts, dropdown
// lists, dialogs, layer overlays) drives its enter and exit tweens
// through, instead of one ad-hoc implementation per surface kind.
//
// The core idea is that dismissal is a state, not a syscall. A
// surface's teardown splits in two:
//
//   - Dismiss flips the logical state: the surface's input region goes
//     empty (clicks "through" the dying surface land beneath, the
//     GTK/fuzzel behavior), the app's OnClosed callback fires exactly
//     once, and — for a compositor-dismissed popup — the seat grab has
//     already dropped with popup_done. The surface stays mapped.
//   - The exit tween paints the way out; only when it lands does the
//     driver's Destroy run, which is the real wayland teardown.
//
// Double dismissal is a no-op: Dismiss returns false on every call
// after the first, so another outside click mid-exit cannot re-enter
// the machine, re-seal input, or re-fire callbacks.
//
// wlroots popup lifetime rules shaped the ordering: a grabbed popup
// whose grab dropped while mapped is legal (that is exactly the
// outside-click case — the compositor ends the grab before sending
// popup_done, and the surface stays mapped for the exit tween), while
// the client cannot drop the grab itself without destroying the popup
// role object, which unmaps the surface. Destroying mid-grab is
// therefore the one thing Dismiss defers: the grab lives until Destroy
// at tween end — or dies on the first outside press, which the empty
// input region turns into a popup_done that Dismiss absorbs as a
// no-op. That ordering is what makes an exit tween on a grabbed popup
// wire-legal.
package surfx

import (
	"sync"
	"time"

	"github.com/stubbedev/lys-ui/anim"
)

// Kind selects a surface's animation profile. The zero value is the
// menu/popover profile, the default for xdg popups.
type Kind uint8

// Surface kinds. KindMenu is the zero value — the popover default.
const (
	KindMenu Kind = iota
	KindTooltip
	KindToast
	KindDialog
	KindOverlay
)

// Spec is one motion's shape: a tween's duration and curve, or a
// physics spring (Spring set), which runs until it settles and may
// overshoot the rest state - the surface swells past its size and
// settles back.
type Spec struct {
	Duration time.Duration
	Easing   anim.Easing
	Spring   *anim.SpringParams
}

// moves reports whether the spec animates at all.
func (s Spec) moves() bool { return s.Duration > 0 || s.Spring != nil }

// revealPrecision is a reveal spring's settle precision: a reveal is a
// 0-to-1 fraction, so a thousandth.
const revealPrecision = 0.001

// Rise runs a rising motion from start to rest (1), calling fn with
// every frame's reveal: the spec's spring when it has one - from
// wherever start is, overshoot included, until it settles - else its
// tween over the rest of the way. A spec that does not move lands 1 at
// once.
func Rise(spec Spec, start float64, fn func(reveal float64)) anim.Cancel {
	if spec.Spring != nil {
		s := anim.NewSpring(*spec.Spring, []float64{start}, func(pos []float64) { fn(pos[0]) })
		s.SetPrecision(revealPrecision)
		s.SetTarget(1)
		return anim.Cancel(s.Stop)
	}
	tween := anim.Animate(spec.Duration, func(p float64) { fn(start + (1-start)*p) })
	if spec.Easing != nil {
		tween.Easing(spec.Easing)
	}
	return anim.Play(tween)
}

// Style is a kind's enter and exit motion. Morph surfaces grow out of
// the side facing their anchor - scaling up and unrolling a rounded
// clip as they fade in - and shrink back into it on the way out;
// without it they only fade.
type Style struct {
	Enter, Exit Spec
	Morph       bool
}

// Defaults are the per-kind profiles. Menus and popovers morph out of
// their anchor on a snappy spring, toasts on a bouncy one, dialogs and
// overlays (launchers, panel reveals) on a smooth one; tooltips stay a
// quick, quiet fade. Every exit is a short emphasized-accelerate tween
// back into the anchor.
var Defaults = map[Kind]Style{
	KindMenu: {
		Enter: Spec{Spring: &anim.SpringSnappy},
		Exit:  Spec{Duration: anim.DurationShort, Easing: anim.EmphasizedAccelerate},
		Morph: true,
	},
	KindTooltip: {
		Enter: Spec{Duration: anim.DurationShort * 3 / 4, Easing: anim.EmphasizedDecelerate},
		Exit:  Spec{Duration: anim.DurationShort / 2, Easing: anim.EmphasizedAccelerate},
	},
	KindToast: {
		Enter: Spec{Spring: &anim.SpringBouncy},
		Exit:  Spec{Duration: anim.DurationMedium, Easing: anim.EmphasizedAccelerate},
		Morph: true,
	},
	KindDialog: {
		Enter: Spec{Spring: &anim.SpringSmooth},
		Exit:  Spec{Duration: anim.DurationShort, Easing: anim.EmphasizedAccelerate},
		Morph: true,
	},
	KindOverlay: {
		Enter: Spec{Spring: &anim.SpringSmooth},
		Exit:  Spec{Duration: anim.DurationMedium, Easing: anim.EmphasizedAccelerate},
		Morph: true,
	},
}

// reducedMotion is the profile every kind degrades to when the theme
// turns motion off: a short fade, no movement.
var reducedMotion = Style{
	Enter: Spec{Duration: anim.DurationShort * 3 / 4, Easing: anim.Linear},
	Exit:  Spec{Duration: anim.DurationShort / 2, Easing: anim.Linear},
}

var (
	mu        sync.Mutex
	overrides = map[Kind]Style{}
	enabled   = true
)

// SetStyle overrides a kind's profile; apps tune curves, springs and
// durations here instead of forking the state machine. A style that
// moves neither way clears the override.
func SetStyle(k Kind, s Style) {
	mu.Lock()
	defer mu.Unlock()
	if !s.Enter.moves() && !s.Exit.moves() {
		delete(overrides, k)
		return
	}
	overrides[k] = s
}

// Lookup returns the effective profile for k: the override when one
// is set, else the default.
func Lookup(k Kind) Style {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := overrides[k]; ok {
		return s
	}
	return Defaults[k]
}

// SetEnabled turns surface animations on or off — the API toggle beside
// LYS_UI_NO_ANIM. It returns the restore function. Disabled animations
// collapse the durations to zero; the state machine still runs, so
// dismissal, sealing, callbacks, and destroy all behave identically.
func SetEnabled(on bool) (restore func()) {
	mu.Lock()
	old := enabled
	enabled = on
	mu.Unlock()
	return func() {
		mu.Lock()
		enabled = old
		mu.Unlock()
	}
}

// Enabled reports whether surface animations are on.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// Plan resolves a kind's effective profile. The package toggle and
// LYS_UI_NO_ANIM collapse it to zero-duration tweens, which deliver
// their end state once at launch, so the state machine's shape is
// identical either way. A caller's reduced-motion source such as the
// theme (motion false) degrades it to a short fade with no movement -
// which is why callers pass their theme state here instead of
// consulting it twice.
func Plan(k Kind, motion bool) Style {
	switch {
	case !Enabled() || anim.Instant():
		return Style{}
	case !motion:
		return reducedMotion
	}
	return Lookup(k)
}

// Driver is the surface-side half the coordinator drives. The surface
// wrapper (popup.Popup, a hostWindow) implements it; the coordinator
// never touches wire objects itself.
type Driver interface {
	// ApplyVisual applies the reveal fraction, 0 hidden through 1 at
	// rest - a spring enter may overshoot past 1 before it settles.
	// morph says whether the surface grows out of its anchor (scale and
	// clip with the fade) or only fades.
	ApplyVisual(reveal float64, morph bool)
	// MarkFrame schedules a repaint of the current visual and wakes the
	// surface's loop if it is parked. Called once per tween frame.
	MarkFrame()
	// SealInput kills the surface's input: an empty input region,
	// committed, so clicks during the exit land beneath. Called once,
	// at Dismiss, before the first exit frame.
	SealInput()
	// Destroy is the real wayland teardown. Called exactly once, when
	// the exit tween lands — or immediately by Teardown.
	Destroy()
}

// phase is the coordinator's position in the enter/exit arc.
type phase uint8

const (
	phaseIdle phase = iota // before Enter
	phaseOpen              // at rest (Enter done or never wanted)
	phaseExit              // tween in flight
	phaseGone              // destroyed
)

// Coordinator is the dismissal state machine for one surface. Embed or
// hold it; the surface keeps its own wire objects behind the Driver.
// All methods are safe from any goroutine: tween callbacks land on
// whichever loop ticked the animation clock, while Dismiss usually
// comes from an input path.
type Coordinator struct {
	kind   Kind
	drv    Driver
	motion func() bool // nil: package toggle only

	mp     sync.Mutex
	ph     phase
	reveal float64

	// destroyed guards the exactly-once Destroy (and, with fire()'s
	// nil-ing of onDismissed, the exactly-once callback). Touched only
	// under mp.
	destroyed bool

	cancelEnter anim.Cancel
	cancelExit  anim.Cancel
	onDismissed func()
	// morph is the running style's Morph, applied with every frame.
	morph bool
}

// NewCoordinator builds the machine for one surface of kind k. motion,
// when non-nil, is consulted at every launch so a live theme change
// applies to the next tween; callers typically pass their theme's
// animation flag there.
func NewCoordinator(k Kind, drv Driver, motion func() bool) *Coordinator {
	return &Coordinator{kind: k, drv: drv, motion: motion}
}

// SetOnDismissed registers the exactly-once dismissal callback. It
// fires inside the first Dismiss (the logical close), and from
// Teardown when teardown beats any dismissal — a surface interrupted
// mid-flight still tells its app it went away, once.
func (c *Coordinator) SetOnDismissed(f func()) {
	c.mp.Lock()
	c.onDismissed = f
	c.mp.Unlock()
}

// Dismissed reports whether the logical close happened.
func (c *Coordinator) Dismissed() bool {
	c.mp.Lock()
	defer c.mp.Unlock()
	return c.ph >= phaseExit
}

// Destroyed reports whether the wire teardown happened.
func (c *Coordinator) Destroyed() bool {
	c.mp.Lock()
	defer c.mp.Unlock()
	return c.destroyed
}

// Reveal returns the current visual fraction for a paint pass.
func (c *Coordinator) Reveal() float64 {
	c.mp.Lock()
	defer c.mp.Unlock()
	return c.reveal
}

// Visual returns the current frame: the reveal fraction and whether
// the motion morphs, for a driver building its view after frames ran.
func (c *Coordinator) Visual() (reveal float64, morph bool) {
	c.mp.Lock()
	defer c.mp.Unlock()
	return c.reveal, c.morph
}

// motionOn resolves the motion decision for a launch.
func (c *Coordinator) motionOn() bool {
	return c.motion == nil || c.motion()
}

// Enter starts the enter tween: the first frame sits at progress 0
// (applied synchronously, so a surface's very first paint is hidden or
// offset — never a flash of the final frame), the tween then raises
// the reveal to rest. Idempotent; a surface that skips Enter opens at
// rest.
//
// Locking note: the launch happens OUTSIDE c.mp. A zero-duration tween
// (reduced motion) fires its callback synchronously inside Play, and
// that callback takes the lock — launching under the lock would
// self-deadlock. The phase flip stays inside so concurrent Enters stay
// single-shot.
func (c *Coordinator) Enter() {
	c.mp.Lock()
	if c.destroyed || c.ph != phaseIdle {
		c.mp.Unlock()
		return
	}
	c.ph = phaseOpen
	st := Plan(c.kind, c.motionOn())
	c.morph = st.Morph
	c.mp.Unlock()
	// The first frame applies synchronously at progress 0, whatever the
	// duration, so the driver's very first paint is the hidden/offset
	// one — never a flash of the finished surface. (Reduced motion then
	// lands 1 inside play, on the same call stack.)
	c.drv.ApplyVisual(0, st.Morph)
	cancel := c.play(st.Enter, true, 0)
	c.mp.Lock()
	if c.ph == phaseOpen && !c.destroyed {
		c.cancelEnter = cancel
	}
	c.mp.Unlock()
}

// Dismiss flips the logical state and starts the exit tween from the
// current reveal. Returns false when the surface is already dismissed
// or gone — the double-dismiss no-op the outside-click path relies on.
// Like Enter, the launch runs outside the lock (see there).
func (c *Coordinator) Dismiss() bool {
	c.mp.Lock()
	if c.destroyed || c.ph >= phaseExit {
		c.mp.Unlock()
		return false
	}
	c.ph = phaseExit
	start := c.reveal
	if c.cancelEnter != nil {
		c.cancelEnter()
		c.cancelEnter = nil
	}
	st := Plan(c.kind, c.motionOn())
	c.morph = st.Morph
	c.mp.Unlock()
	// Input dies before the first exit frame: the seal is the whole
	// click-through guarantee, and for a still-held client-side grab it
	// also turns the next outside press into a popup_done that Dismiss
	// absorbs as its no-op second call.
	c.drv.SealInput()
	cancel := c.play(st.Exit, false, start)
	c.mp.Lock()
	if !c.destroyed {
		c.cancelExit = cancel
	}
	c.mp.Unlock()
	c.fire()
	return true
}

// play launches one motion. Rising ones run reveal from 0 to 1 - a
// spring from wherever it is, through any overshoot, until it settles;
// the exit runs from start down to 0, so an exit interrupting an enter
// reverses from the current state. Zero durations (instant motion)
// land the end state — and for the exit that means Destroy —
// synchronously inside this call.
func (c *Coordinator) play(spec Spec, rising bool, start float64) anim.Cancel {
	fn := func(p float64) {
		c.mp.Lock()
		if c.destroyed {
			// Teardown won the race against this callback: the driver is
			// gone and must not be touched again.
			c.mp.Unlock()
			return
		}
		var reveal float64
		if rising {
			reveal = p
		} else {
			reveal = start * (1 - p)
		}
		c.reveal = reveal
		morph := c.morph
		c.mp.Unlock()
		c.drv.ApplyVisual(reveal, morph)
		c.drv.MarkFrame()
		if !rising && p >= 1 {
			c.finishExit()
		}
	}
	if rising {
		return Rise(spec, start, fn)
	}
	tween := anim.Animate(spec.Duration, fn)
	if spec.Easing != nil {
		tween.Easing(spec.Easing)
	}
	return anim.Play(tween)
}

// finishExit runs when the exit tween landed: the last frame has been
// marked for paint, so the wire teardown follows. Exactly once.
func (c *Coordinator) finishExit() {
	c.mp.Lock()
	if c.destroyed {
		c.mp.Unlock()
		return
	}
	c.destroyed = true
	c.ph = phaseGone
	c.cancelExit = nil
	drv := c.drv
	c.mp.Unlock()
	drv.Destroy()
}

// Teardown cancels any tween, fires the dismissal callback if it never
// fired, and destroys the surface if it is not destroyed. This is the
// error-path and surface-teardown entry (the old Close): no leak, no
// zombie, exactly-once everywhere — and idempotent.
func (c *Coordinator) Teardown() {
	c.mp.Lock()
	first := !c.destroyed
	c.destroyed = true
	c.ph = phaseGone
	if c.cancelEnter != nil {
		c.cancelEnter()
		c.cancelEnter = nil
	}
	if c.cancelExit != nil {
		c.cancelExit()
		c.cancelExit = nil
	}
	drv := c.drv
	c.mp.Unlock()
	c.fire()
	if first {
		drv.Destroy()
	}
}

// fire runs OnDismissed at most once per coordinator.
func (c *Coordinator) fire() {
	c.mp.Lock()
	f := c.onDismissed
	c.onDismissed = nil
	c.mp.Unlock()
	if f != nil {
		f()
	}
}
