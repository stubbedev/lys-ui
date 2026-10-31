package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// TestProgressBarPulse pins activity mode: Pulse steps and bounces the
// block, the fill carries .pulse, SetPulsing animates only while on
// screen, and SetValue returns to the fraction.
func TestProgressBarPulse(t *testing.T) {
	p := NewProgressBar(0.3)
	t.Cleanup(Host(p))
	p.PulseStep = 0.3
	p.Pulse()
	if !p.Activity() || p.pos != 0.3 || !p.trough.fill.hasClass("pulse") {
		t.Fatalf("first pulse: activity=%v pos=%v", p.Activity(), p.pos)
	}
	p.Pulse()
	p.Pulse() // 0.9 clamps to the end and turns back
	if p.pos != 1-pulseBlock || p.dir != -1 {
		t.Errorf("bounce: pos=%v dir=%v", p.pos, p.dir)
	}

	p.SetPulsing(true)
	if p.pulse.cancel != nil {
		t.Error("an unarranged bar scheduled the pulse")
	}
	p.Arrange(render.Rect{W: 160, H: 10})
	if p.pulse.cancel == nil {
		t.Error("a visible pulsing bar scheduled nothing")
	}
	p.SetValue(0.5)
	if p.Activity() || p.Pulsing() || p.pulse.cancel != nil || p.trough.fill.hasClass("pulse") {
		t.Error("SetValue left activity mode running")
	}
}

// TestLevelBarOffsets pins the classification: the block carries the
// lowest offset at or above the value, and offsets move and drop.
func TestLevelBarOffsets(t *testing.T) {
	b := NewLevelBar(0.1)
	cases := []struct {
		v    float64
		want string
	}{{0.1, "low"}, {0.25, "low"}, {0.5, "high"}, {1, "full"}}
	for _, c := range cases {
		b.SetValue(c.v)
		if b.Offset() != c.want || !b.trough.fill.hasClass(c.want) {
			t.Errorf("value %v: offset %q, want %q", c.v, b.Offset(), c.want)
		}
	}
	b.AddOffset("mid", 0.95)
	b.SetValue(0.9)
	if b.Offset() != "mid" || b.trough.fill.hasClass("high") {
		t.Errorf("custom offset: %q", b.Offset())
	}
	b.RemoveOffset("mid")
	if b.Offset() != "full" || b.trough.fill.hasClass("mid") {
		t.Errorf("after removal: %q", b.Offset())
	}
}

// TestGoldenMeters pins the offset-tinted level bars and a pulse block.
func TestGoldenMeters(t *testing.T) {
	th := DarkTheme()
	col := NewBox(Column, 8, 0)
	for _, v := range []float64{0.2, 0.6, 1} {
		col.Append(NewLevelBar(v), false)
	}
	p := NewProgressBar(0)
	p.PulseStep = 0.4
	p.Pulse()
	col.Append(p, false)
	NewGolden(t, col, "meters", goldenTheme(th))
}

// TestLoopUnderReducedMotion pins the parked loop: an endless
// indicator under reduced motion lands once and schedules nothing,
// instead of relaunching itself at every landing.
func TestLoopUnderReducedMotion(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	p := NewProgressBar(0)
	p.SetPulsing(true)
	p.Arrange(render.Rect{W: 160, H: 10})
	s := NewSpinner(24)
	s.SetSpinning(true)
	s.Arrange(render.Rect{W: 24, H: 24})
	if p.pulse.cancel != nil || s.spin.cancel != nil || p.pos != 0 || s.angle != 0 {
		t.Error("a reduced-motion loop kept a timer or moved")
	}
}
