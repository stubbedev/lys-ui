package wlsession

import (
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wltest"
)

// syncCompositor plays the compositor's half of wl_display.sync on c:
// every sync is answered with its callback's done and the display's
// delete_id. It counts the syncs it answered and returns when c closes.
func syncCompositor(c net.Conn, answered *atomic.Int32) {
	var head [8]byte
	for {
		if _, err := io.ReadFull(c, head[:]); err != nil {
			return
		}
		obj := binary.NativeEndian.Uint32(head[0:4])
		word := binary.NativeEndian.Uint32(head[4:8])
		body := make([]byte, word>>16-8)
		if _, err := io.ReadFull(c, body); err != nil {
			return
		}
		if obj != 1 || word&0xffff != displaySyncOpcode {
			continue
		}
		id := binary.NativeEndian.Uint32(body[0:4])
		answered.Add(1)
		const eventSize, doneOpcode, deleteIDOpcode = 12, 0, 1
		var ev [2 * eventSize]byte
		binary.NativeEndian.PutUint32(ev[0:], id)
		binary.NativeEndian.PutUint32(ev[4:], eventSize<<16+doneOpcode)
		binary.NativeEndian.PutUint32(ev[8:], 1)
		binary.NativeEndian.PutUint32(ev[12:], 1)
		binary.NativeEndian.PutUint32(ev[16:], eventSize<<16+deleteIDOpcode)
		binary.NativeEndian.PutUint32(ev[20:], id)
		if _, err := c.Write(ev[:]); err != nil {
			return
		}
	}
}

// newSyncSession connects a session to a loopback compositor that
// answers syncs.
func newSyncSession(t *testing.T) (*Session, net.Conn, *atomic.Int32) {
	t.Helper()
	ln, _ := wltest.LoopbackDisplay(t, "wayland-wake")
	t.Cleanup(func() { _ = ln.Close() })
	d, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	answered := &atomic.Int32{}
	go syncCompositor(srv, answered)
	s := &Session{Display: d}
	t.Cleanup(s.Close)
	return s, srv, answered
}

func stepWithin(t *testing.T, s *Session, limit time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- s.Step() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("step: %v", err)
		}
	case <-time.After(limit):
		t.Fatalf("a loop parked in Step did not wake within %v", limit)
	}
	return time.Since(start)
}

func TestWakeAfterWakesTheLoopWithoutTheCompositor(t *testing.T) {
	s, _, answered := newSyncSession(t)
	s.WakeAfter(10 * time.Millisecond)
	if got := stepWithin(t, s, 5*time.Second); got < 9*time.Millisecond {
		t.Errorf("woke after %v, before the 10ms it asked for", got)
	}
	if n := answered.Load(); n != 0 {
		t.Errorf("a wake sent the compositor %d syncs", n)
	}
}

func TestAWakeReachesAStepAlreadyParked(t *testing.T) {
	s, _, _ := newSyncSession(t)
	done := make(chan error, 1)
	go func() { done <- s.Step() }()
	time.Sleep(5 * time.Millisecond)
	s.WakeAfter(0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a parked Step never woke")
	}
}

func TestEveryPendingWakeFiresInOrder(t *testing.T) {
	s, _, _ := newSyncSession(t)
	s.WakeAfter(60 * time.Millisecond)
	s.WakeAfter(5 * time.Millisecond)
	if got := stepWithin(t, s, 5*time.Second); got >= 60*time.Millisecond {
		t.Errorf("the nearer wake came after %v", got)
	}
	start := time.Now()
	stepWithin(t, s, 5*time.Second)
	if time.Since(start) < 20*time.Millisecond {
		t.Error("the later wake fired with the earlier one")
	}
}

func TestADueWakeDoesNotSpinTheLoop(t *testing.T) {
	s, _, answered := newSyncSession(t)
	s.WakeAfter(0)
	stepWithin(t, s, 5*time.Second)
	done := make(chan error, 1)
	go func() { done <- s.Step() }()
	select {
	case err := <-done:
		t.Fatalf("Step returned with nothing to do: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := s.AfterSync(func() {}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("compositor events no longer end a park; %d syncs answered", answered.Load())
	}
}

func TestManyWakesRaceWithADispatchingLoop(t *testing.T) {
	s, _, _ := newSyncSession(t)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			if err := s.Step(); err != nil {
				done <- err
				return
			}
		}
	}()
	for i := range 200 {
		s.WakeAfter(time.Duration(i%3) * time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	close(stop)
	s.WakeAfter(0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the loop stayed parked past its wakes")
	}
}

// A WakeAfter after Close neither panics nor writes.
func TestWakeAfterCloseIsSilent(t *testing.T) {
	s, _, answered := newSyncSession(t)
	s.Close()
	s.WakeAfter(0)
	time.Sleep(20 * time.Millisecond)
	if answered.Load() != 0 {
		t.Errorf("a wake after Close sent %d syncs", answered.Load())
	}
	if len(s.wakes) != 0 {
		t.Errorf("a wake after Close stayed pending: %v", s.wakes)
	}
}

func TestSetDeadlineReplacesTheLast(t *testing.T) {
	s, _, _ := newSyncSession(t)
	s.SetDeadline(time.Now().Add(time.Hour))
	s.SetDeadline(time.Now().Add(5 * time.Millisecond))
	stepWithin(t, s, 5*time.Second)
	if !s.deadline.IsZero() {
		t.Errorf("a fired deadline stayed set: %v", s.deadline)
	}
	s.SetDeadline(time.Now().Add(5 * time.Millisecond))
	s.SetDeadline(time.Now().Add(time.Hour))
	done := make(chan error, 1)
	go func() { done <- s.Step() }()
	select {
	case err := <-done:
		t.Fatalf("a replaced deadline still fired: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	s.SetDeadline(time.Time{})
	s.WakeAfter(0)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !s.armed.IsZero() {
		t.Errorf("a cleared timer left %v armed", s.armed)
	}
}
