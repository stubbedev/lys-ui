package animclock

import (
	"math"
	"testing"
	"time"
)

func TestTickKeepsNoDriverReachableFromItsSnapshot(t *testing.T) {
	ticks := 0
	cancel := Drive(func(time.Time) bool {
		ticks++
		return ticks < 2
	})
	defer cancel()
	now := time.Unix(100, 0)
	if !Tick(now) {
		t.Fatal("a live driver did not run")
	}
	if !Active() {
		t.Error("a driver that asked for another frame stopped")
	}
	Tick(now.Add(time.Millisecond))
	if ticks != 2 {
		t.Errorf("driver ran %d times, want 2", ticks)
	}
	if Active() {
		t.Error("a settled driver is still active")
	}
	for i, d := range running[:cap(running)] {
		if d != nil {
			t.Errorf("snapshot slot %d still holds a driver after the tick", i)
		}
	}
}

func TestFrameClockConvertsAndWraps(t *testing.T) {
	base := time.Unix(1000, 0)
	var c FrameClock
	if got := c.Instant(100, base); !got.Equal(base) {
		t.Fatalf("first instant = %v, want the local now", got)
	}
	now := base.Add(20 * time.Millisecond)
	if got, want := c.Instant(116, now), base.Add(16*time.Millisecond); !got.Equal(want) {
		t.Errorf("instant = %v, want the callback's own %v", got, want)
	}
	var w FrameClock
	w.Instant(math.MaxUint32-4, base)
	if got, want := w.Instant(11, base.Add(18*time.Millisecond)), base.Add(16*time.Millisecond); !got.Equal(want) {
		t.Errorf("across the wrap: %v, want %v", got, want)
	}
}

func TestFrameClockNeverLeadsNorTrailsFar(t *testing.T) {
	base := time.Unix(1000, 0)
	var c FrameClock
	c.Instant(0, base)
	ahead := base.Add(10 * time.Millisecond)
	if got := c.Instant(50, ahead); !got.Equal(ahead) {
		t.Errorf("a timestamp ahead of the clock = %v, want now", got)
	}
	late := base.Add(10 * time.Second)
	if got := c.Instant(60, late); !got.Equal(late) {
		t.Errorf("a timestamp seconds behind = %v, want now", got)
	}
	if got, want := c.Instant(76, late.Add(17*time.Millisecond)), late.Add(16*time.Millisecond); !got.Equal(want) {
		t.Errorf("after reanchoring = %v, want %v", got, want)
	}
}

func TestTickTimeNeverRunsBack(t *testing.T) {
	base := time.Unix(1000, 0)
	defer SetClock(func() time.Time { return base })()
	Reset()
	var seen []float64
	Launch([]Piece{{Dur: time.Second, Fn: func(p float64) { seen = append(seen, p) }, Easing: func(p float64) float64 { return p }}})
	Tick(base.Add(500 * time.Millisecond))
	Tick(base.Add(400 * time.Millisecond))
	if len(seen) != 2 || seen[1] < seen[0] {
		t.Fatalf("progress ran back on an older instant: %v", seen)
	}
	Reset()
}

func TestAPieceFiresOnAFrameTimestampBehindItsLaunch(t *testing.T) {
	base := time.Unix(1000, 0)
	defer SetClock(func() time.Time { return base })()
	Reset()
	fired := false
	Launch([]Piece{{Dur: time.Second, Fn: func(float64) { fired = true }, Easing: func(p float64) float64 { return p }}})
	Tick(base.Add(-5 * time.Millisecond))
	if !fired {
		t.Fatal("a tick with a timestamp just behind the launch left the piece unstarted")
	}
	Reset()
}

func TestPaceSetsTheDeadline(t *testing.T) {
	base := time.Unix(1000, 0)
	now := base
	defer SetClock(func() time.Time { return now })()
	Reset()
	Launch([]Piece{{Dur: 10 * time.Second, Fn: func(float64) {}, Easing: func(p float64) float64 { return p }}})
	Tick(base)
	if at, ok := Next(Timer); !ok || !at.Equal(base.Add(FrameInterval)) {
		t.Errorf("Next(Timer) = %v %v, want a frame on", at, ok)
	}
	if at, ok := Next(Fallback); !ok || !at.Equal(base.Add(FallbackInterval)) {
		t.Errorf("Next(Fallback) = %v %v, want the fallback", at, ok)
	}
	if Due(base.Add(FrameInterval/2), Timer) {
		t.Error("due before a frame passed")
	}
	if !Due(base.Add(FrameInterval), Timer) {
		t.Error("not due a frame on at Timer")
	}
	if Due(base.Add(FrameInterval), Fallback) {
		t.Error("due a frame on while a callback paces the clock")
	}
	if !Due(base.Add(FallbackInterval), Fallback) {
		t.Error("the fallback did not come due")
	}
	Reset()
}

func TestADelayedPieceWakesAtItsStartWhateverThePace(t *testing.T) {
	base := time.Unix(1000, 0)
	defer SetClock(func() time.Time { return base })()
	Reset()
	Launch([]Piece{{At: 5 * time.Second, Dur: time.Second, Fn: func(float64) {}, Easing: func(p float64) float64 { return p }}})
	Tick(base)
	for _, pace := range []Pace{Timer, Fallback} {
		if at, ok := Next(pace); !ok || !at.Equal(base.Add(5*time.Second)) {
			t.Errorf("pace %d: Next = %v %v, want the start", pace, at, ok)
		}
	}
	Reset()
}
