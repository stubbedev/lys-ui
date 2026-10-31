package widget

import (
	"math"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

func TestSpinnerNeedsSpinningAndVisible(t *testing.T) {
	c := pinAnimClock(t)
	s := NewSpinner(24)
	con := Size{W: 100, H: 100}

	t.Run("spinning but unarranged holds no timer", func(t *testing.T) {
		s.SetSpinning(true)
		if animActiveNow(t) {
			t.Error("hidden spinner scheduled rotation work")
		}
	})

	t.Run("visible and spinning ticks", func(t *testing.T) {
		SizeWithin(s, con)
		s.Arrange(render.Rect{X: 0, Y: 0, W: 24, H: 24})
		if !animActiveNow(t) {
			t.Fatal("visible spinning spinner scheduled nothing")
		}
		before := s.angle
		if !c.step() {
			t.Fatal("no frame scheduled")
		}
		if s.angle <= before || s.angle >= 2*math.Pi {
			t.Errorf("angle = %v after one frame, want forward and wrapped", s.angle)
		}
	})

	t.Run("SetSpinning(false) stops cleanly", func(t *testing.T) {
		s.SetSpinning(false)
		if s.Spinning() {
			t.Error("Spinning() still true")
		}
		if animActiveNow(t) {
			t.Error("stopped spinner kept its tween")
		}
		frozen := s.angle
		c.set(c.now().Add(time.Second))
		animclock.Tick(c.now())
		if s.angle != frozen {
			t.Errorf("angle moved after stop: %v -> %v", frozen, s.angle)
		}
	})

	t.Run("restarting resumes", func(t *testing.T) {
		s.SetSpinning(true)
		if !animActiveNow(t) {
			t.Fatal("restarted spinner scheduled nothing")
		}
		if !c.step() {
			t.Fatal("no frame scheduled after restart")
		}
	})

	t.Run("hiding stops the rotation", func(t *testing.T) {
		s.Arrange(render.Rect{})
		if animActiveNow(t) {
			t.Error("hidden spinner kept its tween")
		}
		// Back on screen, still spinning: it resumes without a
		// SetSpinning round-trip.
		s.Arrange(render.Rect{X: 0, Y: 0, W: 24, H: 24})
		if !animActiveNow(t) {
			t.Error("re-shown spinner did not resume")
		}
	})
}

func TestSpinnerAngleWraps(t *testing.T) {
	c := pinAnimClock(t)
	s := NewSpinner(24)
	s.SetSpinning(true)
	SizeWithin(s, Size{W: 100, H: 100})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 24, H: 24})

	// Drive a little past one revolution: the angle must have wrapped
	// while staying inside [0, 2π) throughout.
	prev := s.angle
	wrapped := false
	for range 62 { // 62 frames > 1s at 60Hz
		if !c.step() {
			t.Fatal("rotation stopped mid-spin")
		}
		if s.angle < 0 || s.angle >= 2*math.Pi {
			t.Fatalf("angle out of range: %v", s.angle)
		}
		if s.angle < prev {
			wrapped = true
		}
		prev = s.angle
	}
	if !wrapped {
		t.Error("angle never wrapped over one revolution")
	}
}

func TestSpinnerPaint(t *testing.T) {
	pinAnimClock(t)
	cv := render.New(make([]byte, render.Stride(24)*24), render.Stride(24), 24, 24)

	t.Run("at rest and mid-rotation", func(t *testing.T) {
		s := NewSpinner(24)
		SizeWithin(s, Size{W: 100, H: 100})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 24, H: 24})
		s.Paint(cv)
		s.angle = math.Pi
		s.Paint(cv)
	})

	t.Run("degenerate sizes do not panic", func(t *testing.T) {
		s := NewSpinner(2)
		SizeWithin(s, Size{W: 100, H: 100})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 2, H: 2})
		s.Paint(cv)
		s.Arrange(render.Rect{})
		s.Paint(cv)
	})
}

// TestSpinnerRing pins Loader2's open ring: the stroke runs clockwise
// from the angle in the spinner's color, and the fifth of the turn
// before it is the gap.
func TestSpinnerRing(t *testing.T) {
	loadCSS(t, `spinner { color: #ff0000; }`)
	s := NewSpinner(24)
	data := paintPixels(t, s, 24, 24)
	at := func(a float64) Color {
		return pxAt(data, 24, int(12+9*math.Cos(a)), int(12+9*math.Sin(a)))
	}
	if got := at(math.Pi / 2); got.A() < 0xf0 || got.R() != got.A() || got.G() != 0 {
		t.Errorf("ring at six o'clock %#x, want the spinner's red", uint32(got))
	}
	if got := at(-math.Pi / 5); got.A() != 0 {
		t.Errorf("gap before the angle painted %#x", uint32(got))
	}
}

func TestGoldenSpinner(t *testing.T) {
	for _, shot := range bothPalettes("spinner") {
		NewGolden(t, NewSpinner(24), shot.name, goldenTheme(shot.th))
	}
}

type visibler interface {
	Widget
	SetVisible(visible bool)
}

func TestHiddenIndicatorsScheduleNoFrames(t *testing.T) {
	spinner := func() visibler {
		s := NewSpinner(24)
		s.SetSpinning(true)
		return s
	}
	pulsing := func() visibler {
		p := NewProgressBar(0)
		p.SetPulsing(true)
		return p
	}
	animated := func() visibler {
		l := NewLabel(testFace(t), 12, "x", 0)
		l.AddClass("pulse")
		return l
	}
	inOverlay := func(w visibler) (Widget, visibler) {
		return NewOverlay().Append(NewBox(Column, 0, 0)).AppendAligned(w, AlignCenter, AlignCenter), w
	}
	underFixed := func(w visibler) (Widget, visibler) {
		card := NewBox(Column, 0, 0).Append(w, false)
		return NewFixed().Put(card, 0, 0), card
	}
	inHiddenBox := func(w visibler) (Widget, visibler) {
		card := NewBox(Column, 0, 0).Append(NewOverlay().Append(w), false)
		return NewBox(Column, 0, 0).Append(card, false), card
	}
	cases := []struct {
		name      string
		indicator func() visibler
		place     func(visibler) (host Widget, hide visibler)
	}{
		{"spinner hidden in an overlay", spinner, inOverlay},
		{"pulsing bar hidden in an overlay", pulsing, inOverlay},
		{"css animation hidden in an overlay", animated, inOverlay},
		{"spinner under a hidden fixed child", spinner, underFixed},
		{"css animation under a hidden fixed child", animated, underFixed},
		{"spinner in a hidden box", spinner, inHiddenBox},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loadCSS(t, `@keyframes beat { from { opacity: 1; } to { opacity: 0.5; } }
				.pulse { animation: beat 1s infinite; }`)
			c := pinAnimClock(t)
			host, hide := tc.place(tc.indicator())
			frame(t, host, 200, 100)
			if !c.step() {
				t.Fatal("visible indicator scheduled no frame")
			}
			hide.SetVisible(false)
			frame(t, host, 200, 100)
			CollectDamage(host)
			gen := DamageGeneration()
			for range 3 {
				c.step()
			}
			if animActiveNow(t) {
				t.Error("hidden indicator still schedules frames")
			}
			if DamageGeneration() != gen {
				t.Error("hidden indicator still damages the tree")
			}
			hide.SetVisible(true)
			frame(t, host, 200, 100)
			if !animActiveNow(t) {
				t.Error("shown again, the indicator did not resume")
			}
		})
	}
}
