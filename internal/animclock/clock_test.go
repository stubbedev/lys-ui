package animclock

import (
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
