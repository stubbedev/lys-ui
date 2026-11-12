package animclock

import (
	"math"
	"testing"
	"time"
)

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
