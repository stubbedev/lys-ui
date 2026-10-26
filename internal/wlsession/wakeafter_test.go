package wlsession

import (
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"
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
		answered.Add(1)
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

// A loop parked in Step returns once a WakeAfter fires, and many short
// wakes against a dispatching loop are race-free (run under -race).
func TestWakeAfterWakesTheLoop(t *testing.T) {
	s, _, _ := newSyncSession(t)
	const wakes = 50
	stepped := make(chan error, wakes)
	go func() {
		for range wakes {
			stepped <- s.Step()
		}
	}()
	start := time.Now()
	for i := range wakes {
		s.WakeAfter(time.Duration(i%3) * time.Millisecond)
	}
	for i := range wakes {
		select {
		case err := <-stepped:
			if err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("the loop parked after %d of %d wakes", i, wakes)
		}
	}
	if time.Since(start) > 5*time.Second {
		t.Error("the wakes took too long")
	}
}

// A WakeAfter that fires after Close neither panics nor writes.
func TestWakeAfterCloseIsSilent(t *testing.T) {
	s, _, answered := newSyncSession(t)
	s.Close()
	s.WakeAfter(0)
	time.Sleep(20 * time.Millisecond)
	if answered.Load() != 0 {
		t.Errorf("a wake after Close sent %d syncs", answered.Load())
	}
}
