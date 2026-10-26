package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// A `transition` covering background-color animates the change: the
// color moves over the duration (the paint reads the interpolated
// value from the cache) and settles exactly on the target.
func TestBackgroundTransition(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `button { background-color: #000000; transition: background-color 200ms; } button:hover { background-color: #ffffff; }`)
	b := NewButton(NewSpacer(0, 0), 0, 0)
	arrangeTree(t, b, 100, 40)
	if got := b.style(b).Background; got != render.RGB(0, 0, 0) {
		t.Fatalf("resting background %#08x, want black", uint32(got))
	}

	b.SetHovered(true)
	_ = b.style(b) // the frame's style pass launches the tween
	c.step()
	mid := b.style(b).Background
	if mid == render.RGB(0, 0, 0) || mid == render.RGB(255, 255, 255) {
		t.Fatalf("hover background %#08x mid-flight, want it moving between the ends", uint32(mid))
	}
	for c.step() {
	}
	if got := b.style(b).Background; got != render.RGB(255, 255, 255) {
		t.Fatalf("settled hover background %#08x, want white", uint32(got))
	}

	b.SetHovered(false)
	_ = b.style(b)
	for c.step() {
	}
	if got := b.style(b).Background; got != render.RGB(0, 0, 0) {
		t.Fatalf("settled rest background %#08x, want black", uint32(got))
	}
}

// A change without a transition applies at once, and so does every
// change while animations are off (the reduced-motion switch).
func TestBackgroundTransitionOff(t *testing.T) {
	pinAnimClock(t)
	loadCSS(t, `button { background-color: #000000; } button:hover { background-color: #ffffff; }`)
	b := NewButton(NewSpacer(0, 0), 0, 0)
	arrangeTree(t, b, 100, 40)
	b.SetHovered(true)
	if got := b.style(b).Background; got != render.RGB(255, 255, 255) {
		t.Fatalf("unstyled hover background %#08x, want white at once", uint32(got))
	}

	loadCSS(t, `button { background-color: #000000; transition: background-color 200ms; } button:hover { background-color: #ffffff; }`)
	restore := anim.SetInstant(true)
	defer restore()
	b.SetHovered(false)
	if got := b.style(b).Background; got != render.RGB(0, 0, 0) {
		t.Fatalf("instant-mode rest background %#08x, want black at once", uint32(got))
	}
}
