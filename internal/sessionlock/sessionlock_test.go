package sessionlock

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// fakeSurface records one lock surface's requests.
type fakeSurface struct {
	acks      []uint32
	destroyed int
	ackErr    error
}

func (f *fakeSurface) AckConfigure(serial uint32) error {
	if f.ackErr != nil {
		return f.ackErr
	}
	f.acks = append(f.acks, serial)
	return nil
}

func (f *fakeSurface) Destroy() error { f.destroyed++; return nil }

// fakeLock records the lock object's requests.
type fakeLock struct {
	surfaces  []*fakeSurface
	outputs   []*wl.Output
	unlocks   int
	destroys  int
	createErr error
}

func (f *fakeLock) GetLockSurface(_ *wl.Surface, out *wl.Output) (surfaceAPI, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	s := &fakeSurface{}
	f.surfaces = append(f.surfaces, s)
	f.outputs = append(f.outputs, out)
	return s, nil
}

func (f *fakeLock) UnlockAndDestroy() error { f.unlocks++; return nil }
func (f *fakeLock) Destroy() error          { f.destroys++; return nil }

func newTestLock(t *testing.T) (*Lock, *fakeLock, *int, *int) {
	t.Helper()
	fake := &fakeLock{}
	var locked, finished int
	l := newLock(fake, Hooks{
		Locked:   func() { locked++ },
		Finished: func() { finished++ },
	})
	return l, fake, &locked, &finished
}

// TestUnlockOnlyAfterLocked pins the invalid_unlock rule: a pending
// lock refuses Unlock without touching the wire, and a confirmed lock
// unlocks with unlock_and_destroy - never destroy.
func TestUnlockOnlyAfterLocked(t *testing.T) {
	l, fake, locked, _ := newTestLock(t)
	if err := l.Unlock(); !errors.Is(err, ErrNotLocked) {
		t.Fatalf("Unlock while pending = %v, want ErrNotLocked", err)
	}
	if fake.unlocks != 0 || fake.destroys != 0 {
		t.Fatalf("refused unlock reached the wire: unlocks=%d destroys=%d", fake.unlocks, fake.destroys)
	}
	if l.State() != Pending {
		t.Fatalf("refused unlock moved the state to %v", l.State())
	}

	l.onLocked()
	if *locked != 1 || l.State() != Locked {
		t.Fatalf("locked event: hook calls=%d state=%v", *locked, l.State())
	}
	if err := l.Unlock(); err != nil {
		t.Fatalf("Unlock after locked: %v", err)
	}
	if fake.unlocks != 1 || fake.destroys != 0 {
		t.Errorf("unlock requests = unlock_and_destroy x%d, destroy x%d; want 1 and 0", fake.unlocks, fake.destroys)
	}
	if l.State() != Unlocked {
		t.Errorf("state after unlock = %v, want unlocked", l.State())
	}
	if err := l.Unlock(); !errors.Is(err, ErrEnded) {
		t.Errorf("second Unlock = %v, want ErrEnded", err)
	}
	if fake.unlocks != 1 {
		t.Errorf("second unlock reached the wire: %d requests", fake.unlocks)
	}
}

// TestCancelOnlyWhilePending pins the invalid_destroy rule: a locked
// session cannot be withdrawn with destroy, only a pending one can.
func TestCancelOnlyWhilePending(t *testing.T) {
	t.Run("pending lock cancels with destroy", func(t *testing.T) {
		l, fake, _, _ := newTestLock(t)
		if err := l.Cancel(); err != nil {
			t.Fatalf("Cancel while pending: %v", err)
		}
		if fake.destroys != 1 || fake.unlocks != 0 {
			t.Errorf("cancel requests = destroy x%d, unlock x%d; want 1 and 0", fake.destroys, fake.unlocks)
		}
		if l.State() != Cancelled {
			t.Errorf("state = %v, want cancelled", l.State())
		}
	})
	t.Run("locked lock refuses cancel", func(t *testing.T) {
		l, fake, _, _ := newTestLock(t)
		l.onLocked()
		if err := l.Cancel(); !errors.Is(err, ErrLocked) {
			t.Fatalf("Cancel while locked = %v, want ErrLocked", err)
		}
		if fake.destroys != 0 {
			t.Errorf("refused cancel sent destroy (invalid_destroy)")
		}
		if l.State() != Locked {
			t.Errorf("refused cancel moved the state to %v", l.State())
		}
	})
	t.Run("a locked that crosses a cancel is ignored", func(t *testing.T) {
		l, _, locked, _ := newTestLock(t)
		_ = l.Cancel()
		l.onLocked()
		if *locked != 0 || l.State() != Cancelled {
			t.Errorf("late locked revived a cancelled lock: hook=%d state=%v", *locked, l.State())
		}
	})
}

// TestFinishedReleasesByState pins the finished handling: before
// locked the lock is destroyed, after locked it is released with
// unlock_and_destroy, the surfaces close, and the hook fires once.
func TestFinishedReleasesByState(t *testing.T) {
	t.Run("denied lock destroys", func(t *testing.T) {
		l, fake, locked, finished := newTestLock(t)
		s, err := l.NewSurface(&wl.Surface{}, &wl.Output{})
		if err != nil {
			t.Fatal(err)
		}
		l.onFinished()
		if fake.destroys != 1 || fake.unlocks != 0 {
			t.Errorf("denied lock: destroy x%d, unlock x%d; want 1 and 0", fake.destroys, fake.unlocks)
		}
		if *finished != 1 || *locked != 0 {
			t.Errorf("hooks: finished=%d locked=%d, want 1 and 0", *finished, *locked)
		}
		if !s.Closed() || fake.surfaces[0].destroyed != 1 {
			t.Error("finished must destroy the lock surfaces")
		}
		if l.State() != Finished {
			t.Errorf("state = %v, want finished", l.State())
		}
	})
	t.Run("ended locked session unlocks", func(t *testing.T) {
		l, fake, _, finished := newTestLock(t)
		l.onLocked()
		l.onFinished()
		if fake.unlocks != 1 || fake.destroys != 0 {
			t.Errorf("finished after locked: unlock x%d, destroy x%d; want 1 and 0", fake.unlocks, fake.destroys)
		}
		l.onFinished()
		if *finished != 1 || fake.unlocks != 1 {
			t.Errorf("a repeated finished must be ignored: hook=%d unlocks=%d", *finished, fake.unlocks)
		}
	})
	t.Run("finished after unlock is ignored", func(t *testing.T) {
		l, fake, _, finished := newTestLock(t)
		l.onLocked()
		_ = l.Unlock()
		l.onFinished()
		if *finished != 0 || fake.unlocks != 1 || fake.destroys != 0 {
			t.Errorf("finished on an unlocked lock: hook=%d unlocks=%d destroys=%d", *finished, fake.unlocks, fake.destroys)
		}
	})
}

// TestOneSurfacePerOutput pins duplicate_output: a second surface for
// a covered output is refused locally, a closed one frees the output
// again, and an ended lock creates nothing.
func TestOneSurfacePerOutput(t *testing.T) {
	l, fake, _, _ := newTestLock(t)
	outA, outB := &wl.Output{}, &wl.Output{}
	a, err := l.NewSurface(&wl.Surface{}, outA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.NewSurface(&wl.Surface{}, outA); !errors.Is(err, ErrDuplicateOutput) {
		t.Fatalf("second surface on one output = %v, want ErrDuplicateOutput", err)
	}
	if len(fake.surfaces) != 1 {
		t.Fatalf("refused surface reached the wire: %d get_lock_surface", len(fake.surfaces))
	}
	if _, err := l.NewSurface(&wl.Surface{}, outB); err != nil {
		t.Fatalf("surface on a second output: %v", err)
	}
	if got, ok := l.SurfaceFor(outA); !ok || got != a {
		t.Error("SurfaceFor must return the output's surface")
	}

	a.Close()
	if _, ok := l.SurfaceFor(outA); ok {
		t.Error("a closed surface must not cover its output")
	}
	if _, err := l.NewSurface(&wl.Surface{}, outA); err != nil {
		t.Errorf("replugged output must accept a fresh surface: %v", err)
	}

	if _, err := l.NewSurface(nil, outB); err == nil {
		t.Error("a nil surface must be rejected")
	}

	l.onLocked()
	_ = l.Unlock()
	if _, err := l.NewSurface(&wl.Surface{}, &wl.Output{}); !errors.Is(err, ErrEnded) {
		t.Errorf("surface on an unlocked lock = %v, want ErrEnded", err)
	}
	for i, s := range fake.surfaces {
		if s.destroyed != 1 {
			t.Errorf("surface %d destroyed %d times after unlock, want 1", i, s.destroyed)
		}
	}
}

// TestSurfaceConfigureGate pins commit_before_first_ack and
// dimensions_mismatch: the surface is unusable until a configure was
// acked, takes the configured size exactly (zero included), and a
// closed surface neither acks nor becomes usable.
func TestSurfaceConfigureGate(t *testing.T) {
	l, fake, _, _ := newTestLock(t)
	s, err := l.NewSurface(&wl.Surface{}, &wl.Output{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureUsable(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("EnsureUsable before configure = %v, want ErrNotConfigured", err)
	}
	s.configure(7, 1920, 1080)
	if err := s.EnsureUsable(); err != nil {
		t.Fatalf("EnsureUsable after ack: %v", err)
	}
	if w, h := s.Size(); w != 1920 || h != 1080 {
		t.Errorf("size = %dx%d, want 1920x1080", w, h)
	}
	if got := fake.surfaces[0].acks; len(got) != 1 || got[0] != 7 {
		t.Errorf("acks = %v, want [7]", got)
	}
	s.configure(8, 2560, 1440)
	if w, h := s.Size(); w != 2560 || h != 1440 {
		t.Errorf("reconfigured size = %dx%d, want 2560x1440", w, h)
	}

	s.Close()
	s.configure(9, 800, 600)
	if got := fake.surfaces[0].acks; len(got) != 2 {
		t.Errorf("closed surface acked a late configure: %v", got)
	}
	if err := s.EnsureUsable(); !errors.Is(err, ErrClosed) {
		t.Errorf("EnsureUsable after close = %v, want ErrClosed", err)
	}
	s.Close()
	if fake.surfaces[0].destroyed != 1 {
		t.Errorf("double close sent %d destroys, want 1", fake.surfaces[0].destroyed)
	}
}

// TestFailedAckKeepsSurfaceGated: an ack that failed on the wire must
// not unlock drawing, or the next commit is commit_before_first_ack.
func TestFailedAckKeepsSurfaceGated(t *testing.T) {
	l, fake, _, _ := newTestLock(t)
	s, err := l.NewSurface(&wl.Surface{}, &wl.Output{})
	if err != nil {
		t.Fatal(err)
	}
	fake.surfaces[0].ackErr = errors.New("broken pipe")
	s.configure(1, 100, 100)
	if err := s.EnsureUsable(); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("EnsureUsable after a failed ack = %v, want ErrNotConfigured", err)
	}
}

func TestStateString(t *testing.T) {
	for s, want := range map[State]string{
		Pending: "pending", Locked: "locked", Unlocked: "unlocked",
		Cancelled: "cancelled", Finished: "finished", State(99): "State(99)",
	} {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", uint8(s), got, want)
		}
	}
	if Pending.Ended() || Locked.Ended() {
		t.Error("pending and locked are not ended")
	}
	if !Unlocked.Ended() || !Cancelled.Ended() || !Finished.Ended() {
		t.Error("unlocked, cancelled, and finished are ended")
	}
}
