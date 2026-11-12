package wlsession

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/internal/waittest"
)

type roundtripOnDone struct {
	s     *Session
	inner chan error
}

func (h roundtripOnDone) HandleCallbackDone(wl.CallbackDoneEvent) { h.inner <- h.s.Roundtrip() }

func sendCallbackDone(t *testing.T, c net.Conn, id uint32) {
	t.Helper()
	var msg [12]byte
	binary.NativeEndian.PutUint32(msg[0:4], id)
	binary.NativeEndian.PutUint32(msg[4:8], uint32(len(msg))<<16)
	if _, err := c.Write(msg[:]); err != nil {
		t.Fatalf("send wl_callback.done to %d: %v", id, err)
	}
}

func readSync(t *testing.T, c net.Conn) uint32 {
	t.Helper()
	const displayID, syncOpcode = 1, 0
	opcode, id := readRequestFor(t, c, displayID)
	if opcode != syncOpcode {
		t.Fatalf("wl_display request opcode %d, want sync", opcode)
	}
	return id
}

func TestRoundtripSurvivesARoundtripInsideItsDispatch(t *testing.T) {
	ln, _ := wltest.LoopbackDisplay(t, "wayland-nested")
	defer ln.Close()
	d, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Context().Close() })
	srv, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	s := &Session{Display: d}
	trigger, err := d.Sync()
	if err != nil {
		t.Fatal(err)
	}
	inner := make(chan error, 1)
	trigger.AddDoneHandler(roundtripOnDone{s: s, inner: inner})
	outer := make(chan error, 1)
	go func() { outer <- s.Roundtrip() }()

	triggerID, outerID := readSync(t, srv), readSync(t, srv)
	sendCallbackDone(t, srv, triggerID)
	sendCallbackDone(t, srv, outerID)
	sendCallbackDone(t, srv, readSync(t, srv))

	if err := waittest.For(t, inner, "the roundtrip run from inside a dispatch to return"); err != nil {
		t.Fatalf("inner roundtrip: %v", err)
	}
	if err := waittest.For(t, outer, "the outer roundtrip to return, its done not consumed by the nested one"); err != nil {
		t.Fatalf("outer roundtrip: %v", err)
	}
}

func TestRoundtripFlushesABufferedSync(t *testing.T) {
	s, _, answered := newSyncSession(t)
	s.Display.Context().SetBuffered(true)
	done := make(chan error, 1)
	go func() { done <- s.Roundtrip() }()
	if err := waittest.For(t, done, "a roundtrip on a buffered sync to return"); err != nil {
		t.Fatal(err)
	}
	if answered.Load() != 1 {
		t.Fatalf("%d syncs reached the compositor, want 1", answered.Load())
	}
}
