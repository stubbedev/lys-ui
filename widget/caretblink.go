package widget

import (
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
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

// caretBlink is a field's blink phase and the timeline toggling it.
type caretBlink struct {
	hidden bool
	cancel anim.Cancel
}

// restart shows the caret solid and runs the blink: every half cycle
// until the timeout, the phase flips and dirty repaints the caret's
// rect. Reduced motion keeps the caret solid.
func (b *caretBlink) restart(dirty func()) {
	b.stop(dirty)
	if anim.Instant() {
		return
	}
	half := caretBlinkTime / 2
	var steps []anim.Step
	for at := half; at < caretBlinkTimeout; at += half {
		steps = append(steps, anim.Delay(half), anim.Animate(0, func(float64) {
			b.hidden = !b.hidden
			dirty()
		}))
	}
	steps = append(steps, anim.Animate(0, func(float64) {
		b.cancel = nil
		if b.hidden {
			b.hidden = false
			dirty()
		}
	}))
	b.cancel = anim.Play(anim.Sequence(steps...))
}

// stop ends the blink with the caret solid.
func (b *caretBlink) stop(dirty func()) {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	if b.hidden {
		b.hidden = false
		dirty()
	}
}

// caretExtents is the caret's vertical span on a line drawn at
// baseline: the font's ascent above it and descent below, GTK's
// cursor height.
func caretExtents(sh *render.ShapedText, baseline int) (y, h int) {
	asc := int(sh.Ascent() + 0.5)
	return baseline - asc, asc + int(sh.Descent()+0.5)
}
