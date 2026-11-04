// The surfx coordinator: the dismissal state machine's contract.
// Dismiss flips logical state exactly once and seals input before the
// first exit frame; exit frames repaint until the tween lands and only
// then destroys; double dismissal and teardown interruptions cannot
// double-fire or double-destroy; the enter's first frame sits at
// progress 0; per-kind defaults differ and every instant path
// (reduced motion) runs the same machine with zero durations.
package surfx

import (
	"fmt"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
)

// recDriver is a fake Driver recording the call order, so tests can
// assert that sealing precedes the first exit frame and that the
// destroy follows the last.
type recDriver struct {
	events    []string
	reveal    float64
	frames    int
	sealed    bool
	destroyed int
	morph     bool
}

func (d *recDriver) ApplyVisual(reveal float64, morph bool) {
	d.morph = morph
	d.reveal = reveal
	d.frames++
	d.events = append(d.events, fmt.Sprintf("frame %.3f", reveal))
}

func (d *recDriver) MarkFrame() {}

func (d *recDriver) SealInput() {
	d.sealed = true
	d.events = append(d.events, "seal")
}

func (d *recDriver) Destroy() {
	d.destroyed++
	d.events = append(d.events, "destroy")
}

// last, without the trailing element.
func last[T any](s []T) T { return s[len(s)-1] }

func TestDismissFiresOnceAndDestroysAfterLastFrame(t *testing.T) {
	restore := animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) })
	defer restore()
	animclock.Reset()

	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, func() bool { return true })
	closed := 0
	fx.SetOnDismissed(func() { closed++ })

	fx.Enter()
	if d.frames != 1 || d.reveal != 0 {
		t.Fatalf("Enter's first frame: reveal = %v frames = %d, want one frame at 0", d.reveal, d.frames)
	}

	if !fx.Dismiss() {
		t.Fatal("first Dismiss reported already dismissed")
	}
	if !d.sealed {
		t.Fatal("Dismiss did not seal input")
	}
	// Seal strictly before the first exit frame.
	if d.events[0] != "frame 0.000" || d.events[1] != "seal" {
		t.Fatalf("event order before exit frames: %v", d.events)
	}

	// A second dismissal mid-exit — another outside click — is a no-op.
	if fx.Dismiss() {
		t.Error("second Dismiss re-entered the machine")
	}

	// Drive the exit: every tick repaints, the destroy comes only
	// after the last frame, and nothing fires twice.
	framesBefore := d.frames
	now := time.Unix(1750000000, 0)
	for steps := 0; ; steps++ {
		if _, ok := animclock.Next(animclock.Timer); !ok {
			break
		}
		steps++
		if steps > 1000 {
			t.Fatal("exit tween never landed")
		}
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
	}
	if d.frames == framesBefore {
		t.Error("exit tween painted no frames")
	}
	if last(d.events) != "destroy" {
		t.Errorf("final event = %q, want the destroy after the last frame", last(d.events))
	}
	if d.destroyed != 1 {
		t.Errorf("destroyed %d times, want exactly 1", d.destroyed)
	}
	if closed != 1 {
		t.Errorf("OnClosed fired %d times, want exactly 1", closed)
	}
	if !fx.Dismissed() || !fx.Destroyed() {
		t.Error("Dismissed/Destroyed flags not set after the exit landed")
	}
}

func TestTeardownInterruptsTweenWithoutLeaks(t *testing.T) {
	restore := animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) })
	defer restore()
	animclock.Reset()

	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, func() bool { return true })
	closed := 0
	fx.SetOnDismissed(func() { closed++ })

	fx.Enter()
	fx.Dismiss()
	// Teardown mid-exit: cancel the tween, fire once, destroy once.
	fx.Teardown()
	if closed != 1 {
		t.Errorf("OnClosed fired %d times after teardown, want exactly 1", closed)
	}
	if d.destroyed != 1 {
		t.Errorf("destroyed %d times after teardown, want exactly 1", d.destroyed)
	}
	// A tick after teardown runs nothing: the surface is gone.
	if _, ok := animclock.Next(animclock.Timer); ok {
		t.Error("canceled tween still holds a wake deadline")
	}
	// Teardown is idempotent.
	fx.Teardown()
	if d.destroyed != 1 {
		t.Errorf("second teardown destroyed again (%d)", d.destroyed)
	}
}

func TestTeardownWithoutDismissFiresOnce(t *testing.T) {
	animclock.Reset()
	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, nil)
	closed := 0
	fx.SetOnDismissed(func() { closed++ })
	// The error path: never opened, never dismissed, surface dies.
	fx.Teardown()
	if closed != 1 || d.destroyed != 1 {
		t.Errorf("teardown: closed = %d destroyed = %d, want 1/1", closed, d.destroyed)
	}
}

func TestEnterFirstFrameAtZeroAndRestAtCompletion(t *testing.T) {
	restore := animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) })
	defer restore()
	animclock.Reset()

	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, func() bool { return true })
	fx.Enter()
	// The first frame is at progress 0 — hidden or offset — applied
	// synchronously, so the surface's first paint can never flash the
	// finished widget.
	if d.reveal != 0 {
		t.Fatalf("first frame reveal = %v, want 0", d.reveal)
	}
	// Enter is idempotent.
	fx.Enter()
	if d.frames != 1 {
		t.Fatalf("second Enter replayed a frame (frames = %d)", d.frames)
	}
	// Drive to rest.
	now := time.Unix(1750000000, 0)
	for steps := 0; ; steps++ {
		if _, ok := animclock.Next(animclock.Timer); !ok {
			break
		}
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
		if steps > 1000 {
			t.Fatal("enter tween never landed")
		}
	}
	if d.reveal != 1 {
		t.Errorf("completion reveal = %v, want 1 (rest state)", d.reveal)
	}
	if d.destroyed != 0 {
		t.Error("a completed enter destroyed the surface")
	}
}

func TestInstantPathsRunTheSameMachine(t *testing.T) {
	type instantCase struct {
		name   string
		motion func() bool
		setup  func(t *testing.T)
	}
	for _, tc := range []instantCase{
		{
			name:   "LYS_UI_NO_ANIM equivalent (anim.SetInstant)",
			motion: func() bool { return true },
			setup:  func(t *testing.T) { t.Cleanup(anim.SetInstant(true)) },
		},
		{
			name:   "package toggle (SetEnabled(false))",
			motion: func() bool { return true },
			setup:  func(t *testing.T) { t.Cleanup(SetEnabled(false)) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			animclock.Reset()
			tc.setup(t)

			d := &recDriver{}
			fx := NewCoordinator(KindMenu, d, tc.motion)
			closed := 0
			fx.SetOnDismissed(func() { closed++ })

			fx.Enter()
			if d.reveal != 1 {
				t.Fatalf("instant enter reveal = %v, want 1 at once", d.reveal)
			}
			fx.Dismiss()
			if closed != 1 {
				t.Errorf("instant path fired OnClosed %d times, want exactly 1", closed)
			}
			if d.destroyed != 1 {
				t.Errorf("instant path destroyed %d times, want exactly 1", d.destroyed)
			}
			if !d.sealed {
				t.Error("instant path skipped the input seal")
			}
			// The instant surface holds no wake deadlines: the loop
			// parks immediately after.
			if _, ok := animclock.Next(animclock.Timer); ok {
				t.Error("instant path left a tween scheduled")
			}
		})
	}
}

func TestPerKindDefaults(t *testing.T) {
	menu, tooltip, dialog := Lookup(KindMenu), Lookup(KindTooltip), Lookup(KindDialog)
	// Menus and dialogs morph out of their anchor on springs; tooltips
	// stay a quick fade.
	if menu.Enter.Spring == nil || !menu.Morph || dialog.Enter.Spring == nil || !dialog.Morph {
		t.Errorf("menu %+v / dialog %+v: want spring enters that morph", menu, dialog)
	}
	if tooltip.Enter.Spring != nil || tooltip.Enter.Duration <= 0 || tooltip.Morph {
		t.Errorf("tooltip %+v: want a short tween fade", tooltip)
	}
	if menu.Exit.Duration <= 0 || menu.Exit.Spring != nil {
		t.Errorf("menu exit %+v: want a short tween", menu.Exit)
	}
	// Reduced motion (the theme's switch) is a short fade, no movement;
	// the package toggle and LYS_UI_NO_ANIM are instant.
	if got := Plan(KindMenu, false); got.Morph || got.Enter.Spring != nil || got.Enter.Duration <= 0 || got.Enter.Duration > 100*time.Millisecond {
		t.Errorf("reduced-motion plan = %+v, want a short plain fade", got)
	}
	t.Cleanup(anim.SetInstant(true))
	if got := Plan(KindMenu, true); got.Enter.moves() || got.Exit.moves() {
		t.Errorf("instant plan = %+v, want no motion", got)
	}
}

// driveClock steps the pinned animation clock until nothing runs.
func driveClock(t *testing.T, onFrame func()) {
	t.Helper()
	now := time.Unix(1750000000, 0)
	for steps := 0; ; steps++ {
		if _, ok := animclock.Next(animclock.Timer); !ok {
			return
		}
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
		if onFrame != nil {
			onFrame()
		}
		if steps > 2000 {
			t.Fatal("the motion never settled")
		}
	}
}

// A menu's spring enter swells past rest, settles exactly at it, and
// morphs all the way; reduced motion only fades.
func TestSpringEnterOvershootsAndSettles(t *testing.T) {
	t.Cleanup(animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) }))
	animclock.Reset()
	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, func() bool { return true })
	fx.Enter()
	peak := 0.0
	driveClock(t, func() { peak = max(peak, d.reveal) })
	if peak <= 1 || d.reveal != 1 || !d.morph {
		t.Errorf("peak %v, rest %v, morph %v: want an overshoot settling at exactly 1, morphing", peak, d.reveal, d.morph)
	}

	animclock.Reset()
	d = &recDriver{}
	fx = NewCoordinator(KindMenu, d, func() bool { return false })
	fx.Enter()
	driveClock(t, nil)
	if d.reveal != 1 || d.morph || d.frames < 3 {
		t.Errorf("reduced motion: rest %v, morph %v over %d frames; want a plain fade over a few frames", d.reveal, d.morph, d.frames)
	}
}

// Dismissing mid-enter reverses from where the surface is: the exit's
// first frame starts at the enter's last reveal, not at rest.
func TestDismissMidEnterReverses(t *testing.T) {
	t.Cleanup(animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) }))
	animclock.Reset()
	d := &recDriver{}
	fx := NewCoordinator(KindDialog, d, func() bool { return true })
	fx.Enter()
	now := time.Unix(1750000000, 0)
	for range 4 {
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
	}
	mid := d.reveal
	if mid <= 0 || mid >= 1 {
		t.Fatalf("mid-enter reveal %v, want a fraction", mid)
	}
	fx.Dismiss()
	now = now.Add(animclock.FrameInterval)
	animclock.Tick(now)
	if d.reveal >= mid || d.reveal <= 0 {
		t.Errorf("first exit frame at %v from %v: want it heading down from the mid-enter reveal", d.reveal, mid)
	}
	driveClock(t, nil)
	if d.destroyed != 1 {
		t.Errorf("destroyed %d times, want once", d.destroyed)
	}
}

func TestSetStyleOverridesAndClears(t *testing.T) {
	defer SetStyle(KindDialog, Style{})
	SetStyle(KindDialog, Style{Enter: Spec{Duration: 5 * time.Millisecond, Easing: anim.Linear}, Exit: Spec{Duration: 5 * time.Millisecond}})
	got := Lookup(KindDialog)
	if got.Enter.Duration != 5*time.Millisecond || got.Enter.Easing == nil {
		t.Errorf("override not applied: %+v", got)
	}
	// Zero durations clear the override, restoring the defaults.
	SetStyle(KindDialog, Style{})
	want := Defaults[KindDialog]
	got = Lookup(KindDialog)
	if got.Enter.Duration != want.Enter.Duration || got.Exit.Duration != want.Exit.Duration || got.Enter.Easing != nil {
		t.Errorf("cleared override = %+v, want the default durations back", got)
	}
}
