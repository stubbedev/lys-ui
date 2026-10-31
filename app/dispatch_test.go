package app

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

type burstEvent struct {
	handled *int
	wnd     *hostWindow
	target  *widget.Box
}

func (e burstEvent) HandleCallbackDone(wl.CallbackDoneEvent) {
	*e.handled++
	e.target.Invalidate()
	e.wnd.dirty = true
}

func readSyncID(t *testing.T, c net.Conn) uint32 {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var msg [12]byte
	if _, err := io.ReadFull(c, msg[:]); err != nil {
		t.Fatalf("read a wl_display.sync: %v", err)
	}
	if obj, op := binary.NativeEndian.Uint32(msg[0:4]), binary.NativeEndian.Uint32(msg[4:8])&0xffff; obj != 1 || op != 0 {
		t.Fatalf("request to object %d opcode %d, want wl_display.sync", obj, op)
	}
	return binary.NativeEndian.Uint32(msg[8:12])
}

func callbackDones(ids []uint32) []byte {
	out := make([]byte, 0, 12*len(ids))
	for _, id := range ids {
		var msg [12]byte
		binary.NativeEndian.PutUint32(msg[0:4], id)
		binary.NativeEndian.PutUint32(msg[4:8], uint32(len(msg))<<16)
		out = append(out, msg[:]...)
	}
	return out
}

func TestEventBurstCostsOneTickAndOneDraw(t *testing.T) {
	ln, _ := wltest.LoopbackDisplay(t, "wayland-burst")
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
	sess := &wlsession.Session{Display: d}
	a, wnd, h := disconnectApp(t, sess)
	target := wnd.router.Root.(*widget.Box)
	a.wake = func(time.Duration) {}

	const burst = 24
	handled := 0
	ids := make([]uint32, 0, burst)
	for range burst {
		cb, err := d.Sync()
		if err != nil {
			t.Fatal(err)
		}
		cb.AddDoneHandler(burstEvent{handled: &handled, wnd: wnd, target: target})
		ids = append(ids, readSyncID(t, srv))
	}
	wake, err := d.Sync()
	if err != nil {
		t.Fatal(err)
	}
	wakeID := readSyncID(t, srv)

	tick := func() {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- a.tick(a.stepFn(), time.Now()) }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("tick: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("tick parked in dispatch with events pending")
		}
	}

	if _, err := srv.Write(callbackDones(ids)); err != nil {
		t.Fatal(err)
	}
	tick()
	if handled != burst {
		t.Fatalf("one tick dispatched %d of a %d-event burst, want all of it", handled, burst)
	}
	commits := h.surf.commits
	wnd.frameReady = true
	wake.AddDoneHandler(burstEvent{handled: new(int), wnd: wnd, target: target})
	if _, err := srv.Write(callbackDones([]uint32{wakeID})); err != nil {
		t.Fatal(err)
	}
	tick()
	if got := h.surf.commits - commits; got != 1 {
		t.Fatalf("the burst cost %d redraws, want 1", got)
	}
}
