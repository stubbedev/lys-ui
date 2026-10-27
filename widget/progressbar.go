package widget

import (
	"math"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

const (
	// pulseBlock is the activity block's share of the trough (GTK's
	// five activity blocks).
	pulseBlock = 0.2
	// pulsePeriod is one continuous pulse sweep, there and back.
	pulsePeriod = 1500 * time.Millisecond
)

// ProgressBar paints a read-only fill over a trough, as GTK's node
// tree: `progressbar > trough > progress` - the bar's box, the
// trough's, and the fill's, each styled by the cascade. Besides the
// fraction it has GTK's activity mode for work of unknown length: Pulse
// steps a bouncing block, SetPulsing animates it continuously, and
// SetValue returns to the fraction.
type ProgressBar struct {
	meter
	// Fill and Trough color the bar; zero is the theme's accent (or
	// the accent-color) and, for the trough, the fill at a fifth of
	// its opacity, under the trough and progress rules.
	Fill, Trough render.Color
	// PulseStep is how far one Pulse moves the block (GTK pulse-step).
	PulseStep float64

	activity bool
	pos, dir float64
	pulse    loop
}

// NewProgressBar returns a progress bar at value clamped to [0, 1].
func NewProgressBar(value float64) *ProgressBar {
	p := &ProgressBar{PulseStep: 0.1, dir: 1}
	p.initMeter(p, value, "progress")
	p.roundTrough = true
	p.troughFill = func(fill Color) Color { return orColor(p.Trough, progressTrack(fill)) }
	p.fillFill = func(t *Theme) Color { return orColor(p.Fill, t.Accent) }
	p.pulse = loop{period: pulsePeriod, step: func(t float64) {
		// A triangle wave: across and back once per period.
		p.pos = (1 - pulseBlock) * (1 - math.Abs(2*t-1))
		p.Invalidate()
	}}
	return p
}

// progressTrack is the trough's fallback: the fill at a fifth of its
// opacity.
func progressTrack(fill Color) Color { return scaleAlpha(fill, 0.2) }

// orColor is c, or def when c is unset.
func orColor(c, def Color) Color {
	if c != 0 {
		return c
	}
	return def
}

// SetValue sets the fraction, leaving activity mode.
func (p *ProgressBar) SetValue(v float64) {
	if p.activity {
		p.setActivity(false)
		p.Invalidate()
	}
	p.setValue(v)
}

// Activity reports whether the bar is in activity mode.
func (p *ProgressBar) Activity() bool { return p.activity }

// Pulse enters activity mode and moves the block one PulseStep,
// bouncing off the trough's ends.
func (p *ProgressBar) Pulse() {
	p.setActivity(true)
	p.pos += p.dir * p.PulseStep
	if p.pos >= 1-pulseBlock {
		p.pos, p.dir = 1-pulseBlock, -1
	} else if p.pos <= 0 {
		p.pos, p.dir = 0, 1
	}
	p.Invalidate()
}

// SetPulsing animates the activity block continuously while the bar
// is on screen; off freezes it in place (SetValue leaves the mode).
func (p *ProgressBar) SetPulsing(on bool) {
	if on {
		p.setActivity(true)
	}
	p.pulse.set(on)
}

// Pulsing reports whether the continuous animation is on.
func (p *ProgressBar) Pulsing() bool { return p.pulse.on }

// setActivity switches mode; the fill carries .pulse in activity mode
// (GTK's progress.pulse), and leaving it stops the animation.
func (p *ProgressBar) setActivity(on bool) {
	if p.activity == on {
		return
	}
	p.activity = on
	if on {
		p.trough.fill.AddClass("pulse")
		return
	}
	p.trough.fill.RemoveClass("pulse")
	p.pulse.set(false)
}

// Measure wants the 160x10 trough inside the bar's CSS box.
func (p *ProgressBar) Measure(axis Axis, forSize int) Sizes { return p.measureMeter(p, axis, forSize) }

// Arrange lays the bar out and syncs the pulse with visibility.
func (p *ProgressBar) Arrange(r render.Rect) {
	p.meter.Arrange(r)
	p.pulse.arranged(r)
}

// Paint draws the trough and the fill: the fraction, or the block.
func (p *ProgressBar) Paint(cv *render.Canvas) {
	if p.activity {
		p.paintMeter(cv, p.pos, p.pos+pulseBlock)
		return
	}
	p.paintMeter(cv, 0, p.value)
}

// Role implements Roleer.
func (p *ProgressBar) Role() Role { return RoleProgressBar }

// HitTest returns the bar when p is inside its bounds.
func (p *ProgressBar) HitTest(pt Point) Widget { return p.hitMeter(p, pt) }
