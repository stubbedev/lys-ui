package widget

import (
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
)

// The caret blinks as GTK's does: on for half of a gtk-cursor-blink-time
// cycle, off for the other half, restarting solid on every edit, caret
// move and focus gain, and staying solid once the field has sat idle
// for gtk-cursor-blink-timeout.
const (
	caretBlinkTime    = 1200 * time.Millisecond
	caretBlinkTimeout = 10 * time.Second
)

// caretBlinker is a text field whose caret blinks. The router restarts
// it on every input the focused field takes and as focus arrives.
type caretBlinker interface {
	restartBlink()
	stopBlink()
}

const (
	caretBlinkHalf  = caretBlinkTime / 2
	caretBlinkFlips = int((caretBlinkTimeout - 1) / caretBlinkHalf)
)

// caretBlink is a field's blink phase and the one pending piece toggling it.
type caretBlink struct {
	hidden bool
	cancel anim.Cancel
	dirty  func()
	land   func(float64)
	due    time.Time
	flips  int
}

// restart shows the caret solid and runs the blink: every half cycle until
// the timeout, the phase flips and dirty repaints the caret's rect. Typing
// only moves the next flip, so a keystroke launches nothing. Reduced motion
// keeps the caret solid.
func (b *caretBlink) restart() {
	if anim.Instant() {
		b.stop()
		return
	}
	b.solid()
	b.flips = 0
	b.due = animclock.Now().Add(caretBlinkHalf)
	if b.cancel == nil {
		b.arm(caretBlinkHalf)
	}
}

func (b *caretBlink) arm(after time.Duration) {
	if b.land == nil {
		b.land = b.landed
	}
	b.cancel = anim.Play(anim.Delay(after), anim.Animate(0, b.land))
}

func (b *caretBlink) landed(float64) {
	now := animclock.Now()
	if now.Before(b.due) {
		b.arm(b.due.Sub(now))
		return
	}
	b.flips++
	b.hidden = !b.hidden
	b.dirty()
	if b.flips >= caretBlinkFlips {
		b.cancel = nil
		b.solid()
		return
	}
	b.due = now.Add(caretBlinkHalf)
	b.arm(caretBlinkHalf)
}

func (b *caretBlink) solid() {
	if b.hidden {
		b.hidden = false
		b.dirty()
	}
}

// stop ends the blink with the caret solid.
func (b *caretBlink) stop() {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.solid()
}

// caretExtents is the caret's vertical span on a line drawn at
// baseline: the font's ascent above it and descent below, GTK's
// cursor height.
func caretExtents(sh *render.ShapedText, baseline int) (y, h int) {
	asc := int(sh.Ascent() + 0.5)
	return baseline - asc, asc + int(sh.Descent()+0.5)
}
