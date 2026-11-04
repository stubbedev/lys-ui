// The window-level surface animations: a layer overlay's exit tween
// keeps the surface mapped and the loop's wake schedule alive until the
// last tween frame lands, seals input at Dismiss, fires nothing twice,
// and only then reaches the real destroy — driven by the injected
// animation clock, no wall time.
package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/widget"
)

// animClock is the injected animation clock for these tests: launches
// and deadlines read a test-controlled instant, and ticks take their
// instants from the same hand (the pinAnimClock pattern, driven from
// outside the anim package).
type animClock struct {
	cur time.Time
}

func pinAnimClock(t *testing.T) *animClock {
	t.Helper()
	c := &animClock{cur: time.Unix(1750000000, 0)}
	t.Cleanup(animclock.SetClock(func() time.Time { return c.cur }))
	t.Cleanup(animclock.Reset)
	animclock.Reset()
	return c
}

// step advances one animation wake — one frame deadline — and ticks
// there. False when nothing is scheduled: the loop may park.
func (c *animClock) step() bool {
	wake, ok := animclock.Next(animclock.Timer)
	if !ok {
		return false
	}
	c.cur = wake
	animclock.Tick(wake)
	return true
}

// drive drains the schedule so a tween lands before assertions.
func (c *animClock) drive() {
	for range 10000 {
		if !c.step() {
			return
		}
	}
	panic("animation schedule did not drain")
}

// closableHost is a fakeHost whose Closed flag flips — what the real
// destroy closure does when the coordinator's exit lands.
type closableHost struct {
	fakeHost
	closed bool
}

func (f *closableHost) Closed() bool { return f.closed }

// exitHarness couples a wire-free hostWindow with the animation state
// newHostWindow installs for animated kinds, over a closable host.
type exitHarness struct {
	ph        *paintHarness
	host      *closableHost
	destroyed int
}

func newExitHarness(t *testing.T, kind surfx.Kind) *exitHarness {
	t.Helper()
	// The caller must have pinned the animation clock first, so tween
	// launches land on the test's hand.
	host := &closableHost{}
	host.w, host.h = 200, 100
	root := widget.NewBox(widget.Row, 4, 4)
	ph := newPaintHarness(root, 200, 100)
	ph.wnd.host = host
	ph.wnd.lastW, ph.wnd.lastH = 200, 100
	h := &exitHarness{ph: ph, host: host}
	if kind == surfx.KindMenu {
		// Plain toplevels: no animation state at all.
		return h
	}
	// What newHostWindow does for animated kinds, minus the session:
	// morph over the tree, coordinator driving this window.
	ph.wnd.morph = widget.NewMorph(root)
	ph.wnd.router.Root = ph.wnd.morph
	ph.wnd.fx = surfx.NewCoordinator(kind, ph.wnd, func() bool { return true })
	ph.wnd.destroy = func() {
		h.destroyed++
		host.closed = true
	}
	return h
}

func TestLayerOverlayExitKeepsLoopParkedBetweenFrames(t *testing.T) {
	c := pinAnimClock(t)
	h := newExitHarness(t, surfx.KindOverlay)
	hw := h.ph.wnd

	// The enter starts on the first usable pass; its first visual is
	// progress 0 — the window's first frame paints nothing.
	hw.enterIfDue()
	if hw.morph == nil {
		t.Fatal("animated kind built no morph")
	}
	if got := hw.morph.Reveal(); got != 0 {
		t.Fatalf("enter first frame alpha = %v, want 0 (no final-frame flash)", got)
	}
	c.step()
	if got := hw.morph.Reveal(); got <= 0 || got >= 1 {
		t.Fatalf("mid-enter alpha = %v, want a fraction", got)
	}

	// Enter lands: alpha rests at exactly 1, and nothing was destroyed.
	c.drive()
	if got := hw.morph.Reveal(); got != 1 {
		t.Fatalf("rest alpha = %v, want 1", got)
	}
	if h.destroyed != 0 {
		t.Fatal("a completed enter destroyed the surface")
	}

	// Close: the logical state flips at once, the surface stays mapped
	// (host not closed — the loop cannot reap it), and the exit holds a
	// wake deadline so the parked loop ticks exactly one frame at a
	// time. The destroy lands only after the final tick, and afterwards
	// the schedule is empty — the loop parks again.
	if !hw.beginExit() {
		t.Fatal("beginExit reported nothing to animate")
	}
	if !hw.exiting {
		t.Error("exiting flag not set at Dismiss")
	}
	if h.host.closed {
		t.Fatal("beginExit destroyed the surface immediately; no exit window")
	}

	frames := 0
	for c.step() {
		frames++
		if frames > 1000 {
			t.Fatal("exit tween never landed")
		}
		if h.destroyed > 1 {
			t.Fatalf("destroyed %d times mid-exit", h.destroyed)
		}
	}
	if frames == 0 {
		t.Error("exit tween held no wake deadlines; the exit would never paint")
	}
	if h.destroyed != 1 {
		t.Errorf("destroyed %d times, want exactly 1 after the last frame", h.destroyed)
	}
	if !h.host.closed {
		t.Error("raw close did not flip the host's closed flag")
	}
	if _, ok := animclock.Next(animclock.Timer); ok {
		t.Error("tween schedule not empty after the exit landed")
	}
}

func TestNonAnimatedWindowClosesImmediately(t *testing.T) {
	// Plain toplevels carry no coordinator: beginExit is false and the
	// caller falls through to the raw close, exactly the pre-tween
	// behavior.
	h := newExitHarness(t, surfx.KindMenu)
	if h.ph.wnd.beginExit() {
		t.Fatal("a non-animated kind began an exit")
	}
	if h.ph.wnd.fx != nil || h.ph.wnd.morph != nil {
		t.Error("non-animated kind built animation state")
	}
}

func TestDismissedWindowSealsInputAndIgnoresDoubleClose(t *testing.T) {
	c := pinAnimClock(t)
	h := newExitHarness(t, surfx.KindDialog)
	hw := h.ph.wnd
	hw.enterIfDue()
	c.drive()
	// surfaceInput, the way newHostWindow wires it: its blocked check
	// is what refuses pointer routing while a modal block is up — and
	// now while the window exits.
	hw.input = &surfaceInput{blocked: func() bool { return hw.blocked || hw.exiting }}

	// Dismiss flips the input-dead state. The SealInput wire half (an
	// empty input region, committed, before the first exit frame) is
	// covered by the coordinator test's event ordering.
	hw.beginExit()
	if !hw.input.dropInput() {
		t.Error("an exiting window still accepts pointer input")
	}

	// A second close mid-exit must not re-enter or double-destroy.
	if !hw.beginExit() {
		t.Error("second beginExit reported fresh")
	}
	for c.step() {
	}
	if h.destroyed != 1 {
		t.Errorf("destroyed %d times, want exactly 1", h.destroyed)
	}
	if _, ok := animclock.Next(animclock.Timer); ok {
		t.Error("schedule not drained")
	}
}

// TestExitTweenPaintsFadingFrames drives the paint pipeline through an
// animated close: every exit tick repaints the surface (a commit per
// frame), the alpha falls monotonically to zero, and the destroy waits
// for the last frame.
func TestExitTweenPaintsFadingFrames(t *testing.T) {
	c := pinAnimClock(t)
	h := newExitHarness(t, surfx.KindOverlay)
	hw := h.ph.wnd
	hw.enterIfDue()
	c.drive()

	hw.beginExit()
	alphas := []float64{hw.morph.Reveal()}
	commitsBefore := h.ph.surf.commits
	frames := 0
	for c.step() {
		frames++
		hw.kick.Store(true)
		hw.dirty = true
		// Hand every busy buffer back, the way the compositor's release
		// events would between frames, so the pool keeps rotating.
		for _, b := range h.ph.bufs {
			b.Release()
		}
		if !hw.draw() {
			t.Fatal(hw.drawErr)
		}
		alphas = append(alphas, hw.morph.Reveal())
		if frames > 1000 {
			t.Fatal("exit tween never landed")
		}
	}
	if frames < 3 {
		t.Fatalf("exit produced %d frames, want a real tween", frames)
	}
	if got := h.ph.surf.commits - commitsBefore; got != frames {
		t.Errorf("exit drew %d frames but committed %d buffers", frames, got)
	}
	for i := 1; i < len(alphas); i++ {
		if alphas[i] > alphas[i-1] {
			t.Fatalf("alpha rose mid-exit at frame %d: %v -> %v", i, alphas[i-1], alphas[i])
		}
	}
	if last := alphas[len(alphas)-1]; last != 0 {
		t.Errorf("final alpha = %v, want 0", last)
	}
	if h.destroyed != 1 {
		t.Errorf("destroyed %d times, want 1 after the last frame", h.destroyed)
	}
}
