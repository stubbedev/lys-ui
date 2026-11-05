package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

func TestKeyRepeater(t *testing.T) {
	t.Run("no repeat before the initial delay", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, wlsession.ModShift)
		if _, _, ok := r.tick(); ok {
			t.Error("tick fired before the delay elapsed")
		}
	})

	t.Run("one repeat after the delay, then at the rate", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.next = time.Now().Add(-time.Millisecond)
		code, _, ok := r.tick()
		if !ok || code != 30 {
			t.Errorf("tick = %d, %v, want 30, true", code, ok)
		}
		if _, _, ok := r.tick(); ok {
			t.Error("second tick fired immediately after the first repeat")
		}
		r.next = time.Now().Add(-time.Millisecond)
		if _, _, ok = r.tick(); !ok {
			t.Error("tick did not fire at the repeat rate")
		}
	})

	t.Run("release stops repeats", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.release(30)
		r.next = time.Now().Add(-time.Millisecond)
		if _, _, ok := r.tick(); ok {
			t.Error("tick fired after release")
		}
	})

	t.Run("a different key release does not stop the held key", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.release(31)
		r.next = time.Now().Add(-time.Millisecond)
		if _, _, ok := r.tick(); !ok {
			t.Error("unrelated release cancelled the held key's repeat")
		}
	})

	t.Run("a new press re-arms the delay", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, 0)
		r.next = time.Now().Add(-time.Millisecond)
		r.press(30, 0)
		if _, _, ok := r.tick(); ok {
			t.Error("re-press fired immediately instead of re-arming the delay")
		}
	})

	t.Run("zeros fall back to usable defaults", func(t *testing.T) {
		r := newKeyRepeater(0, 0)
		if r.rate <= 0 || r.delay <= 0 {
			t.Errorf("defaults invalid: rate %v delay %v", r.rate, r.delay)
		}
	})

	t.Run("late handling keeps the configured rate", func(t *testing.T) {
		clock := time.Unix(1000, 0)
		r := newKeyRepeater(20, 400)
		r.now = func() time.Time { return clock }
		start := clock
		r.press(30, 0)
		const n = 10
		for i := range n {
			due, _ := r.nextDeadline()
			clock = due.Add(7 * time.Millisecond)
			if _, _, ok := r.tick(); !ok {
				t.Fatalf("repeat %d did not fire", i)
			}
		}
		due, _ := r.nextDeadline()
		if want := start.Add(r.delay + n*r.rate); !due.Equal(want) {
			t.Errorf("after %d late repeats the next is due %v after the press, want %v", n, due.Sub(start), want.Sub(start))
		}
	})

	t.Run("repeats that fell behind are skipped, not burst", func(t *testing.T) {
		clock := time.Unix(1000, 0)
		r := newKeyRepeater(20, 400)
		r.now = func() time.Time { return clock }
		start := clock
		r.press(30, 0)
		clock = start.Add(r.delay + 3*r.rate + r.rate/2)
		if _, _, ok := r.tick(); !ok {
			t.Fatal("an overdue repeat did not fire")
		}
		if _, _, ok := r.tick(); ok {
			t.Error("the repeats that fell behind fired in a burst")
		}
		due, _ := r.nextDeadline()
		if want := start.Add(r.delay + 4*r.rate); !due.Equal(want) {
			t.Errorf("next repeat due %v after the press, want %v", due.Sub(start), want.Sub(start))
		}
	})

	t.Run("repeats carry the mods held at press time", func(t *testing.T) {
		r := newKeyRepeater(20, 400)
		r.press(30, wlsession.ModCtrl)
		r.next = time.Now().Add(-time.Millisecond)
		_, mods, ok := r.tick()
		if !ok || mods != wlsession.ModCtrl {
			t.Errorf("repeat mods = %v, %v, want ctrl", mods, ok)
		}
	})
}
