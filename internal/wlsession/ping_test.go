package wlsession

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"
	"github.com/stubbedev/lys-lib/wltest"
)

// TestPingIsAnsweredWithPong plays the compositor on a loopback socket:
// an xdg_wm_base.ping must come back as xdg_wm_base.pong with the same
// serial, unprompted by any application hook. A client that never
// pongs gets Hyprland's not-responding dialog mapped over it.
func TestPingIsAnsweredWithPong(t *testing.T) {
	ln, _ := wltest.LoopbackDisplay(t, "wayland-ping")
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

	s := &Session{Display: d, wmBase: xdg.NewShell(d.Context())}
	s.HandleWmBasePing(xdg.WmBasePingEvent{Serial: 42})

	const pongOpcode = 3
	opcode, serial := readRequestFor(t, srv, uint32(s.wmBase.Id()))
	if opcode != pongOpcode || serial != 42 {
		t.Fatalf("xdg_wm_base got opcode %d serial %d, want pong (%d) of serial 42", opcode, serial, pongOpcode)
	}
}

// readRequestFor reads wire requests until one addressed to object id
// arrives, returning its opcode and first uint32 argument.
func readRequestFor(t *testing.T, c net.Conn, id uint32) (opcode, arg uint32) {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var head [8]byte
	for {
		if _, err := io.ReadFull(c, head[:]); err != nil {
			t.Fatalf("no request for object %d: %v", id, err)
		}
		obj := binary.NativeEndian.Uint32(head[0:4])
		word := binary.NativeEndian.Uint32(head[4:8])
		size := word >> 16
		body := make([]byte, size-8)
		if _, err := io.ReadFull(c, body); err != nil {
			t.Fatalf("short request body: %v", err)
		}
		if obj != id {
			continue
		}
		if len(body) >= 4 {
			arg = binary.NativeEndian.Uint32(body[0:4])
		}
		return word & 0xffff, arg
	}
}
