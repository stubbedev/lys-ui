package wlsession

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/wltest"
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

	select {
	case err := <-inner:
		if err != nil {
			t.Fatalf("inner roundtrip: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the roundtrip run from inside a dispatch never returned")
	}
	select {
	case err := <-outer:
		if err != nil {
			t.Fatalf("outer roundtrip: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the outer roundtrip never returned: the nested one consumed its done")
	}
}
