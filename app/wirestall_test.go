package app

import (
	"encoding/binary"
	"io"
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/testbuild"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wltest"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

const wireSurfaceCommit = 6

type wireFixture struct {
	sess *wlsession.Session
	ctx  *wl.Context
	comp *wl.Compositor
	peer *net.UnixConn
}

func newWireFixture(t *testing.T, name string) *wireFixture {
	t.Helper()
	ln, _ := wltest.LoopbackDisplay(t, name)
	t.Cleanup(func() { _ = ln.Close() })
	d, err := wl.Connect("")
	if err != nil {
		t.Fatalf("connect to the loopback display: %v", err)
	}
	peer, err := ln.AcceptUnix()
	if err != nil {
		t.Fatalf("accept the client: %v", err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	sess := &wlsession.Session{Display: d}
	t.Cleanup(sess.Close)
	reg, err := d.GetRegistry()
	if err != nil {
		t.Fatalf("get the registry: %v", err)
	}
	comp := wl.NewCompositor(d.Context())
	if err := reg.Bind(1, "wl_compositor", 4, comp); err != nil {
		t.Fatalf("bind the compositor: %v", err)
	}
	if _, err := d.Context().Flush(); err != nil {
		t.Fatalf("flush the setup: %v", err)
	}
	return &wireFixture{sess: sess, ctx: d.Context(), comp: comp, peer: peer}
}

func (f *wireFixture) window(t *testing.T, a *Application) *paintHarness {
	t.Helper()
	surf, err := f.comp.CreateSurface()
	if err != nil {
		t.Fatalf("create a surface: %v", err)
	}
	h := newPaintHarness(widget.NewDrawingArea(64, 48), 64, 48)
	h.wnd.surf = wireSurface{wl: surf, comp: f.comp, sess: f.sess, sync: &h.wnd.surfSync}
	h.wnd.sess = f.sess
	a.windows = append(a.windows, h.wnd)
	return h
}

func (f *wireFixture) drain(commits *atomic.Int64) {
	var head [8]byte
	for {
		if _, err := io.ReadFull(f.peer, head[:]); err != nil {
			return
		}
		word := binary.NativeEndian.Uint32(head[4:])
		if _, err := io.CopyN(io.Discard, f.peer, int64(word>>16-8)); err != nil {
			return
		}
		if word&0xffff == wireSurfaceCommit {
			commits.Add(1)
		}
	}
}

func sendmsgCalls(t *testing.T, ctx *wl.Context) int64 {
	t.Helper()
	sends := reflect.ValueOf(ctx).Elem().FieldByName("sends")
	if !sends.IsValid() {
		t.Fatal("wl.Context has no sends counter to count sendmsg with")
	}
	return sends.Int()
}

func boundedTick(t *testing.T, a *Application, step func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- a.tick(step, time.Now()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tick: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("a loop pass stayed blocked for 10s with %d bytes unsent", a.sess.Display.Context().Backlog())
	}
}

func TestStalledWireSkipsFramesAndOwesThem(t *testing.T) {
	f := newWireFixture(t, "wayland-stall")
	f.ctx.SetBuffered(true)
	a := NewTestApplication(t, f.sess)
	h := f.window(t, a)
	surf := h.wnd.surf.(wireSurface).wl

	var filled int64
	for pending := false; !pending; filled++ {
		if filled > 1<<20 {
			t.Fatal("the socket took a million commits without filling")
		}
		if err := surf.Commit(); err != nil {
			t.Fatalf("commit %d: %v", filled, err)
		}
		var err error
		if pending, err = f.ctx.Flush(); err != nil {
			t.Fatalf("flush commit %d: %v", filled, err)
		}
	}

	wakeThenStep := func() error {
		f.sess.WakeAfter(0)
		return f.sess.Step()
	}
	if err := wakeThenStep(); err != nil {
		t.Fatalf("step: %v", err)
	}
	backlog := f.ctx.Backlog()
	sends := sendmsgCalls(t, f.ctx)
	const passes = 20000
	host := h.wnd.host.(*fakeHost)
	host.w = 80
	for range passes {
		h.wnd.dirty = true
		boundedTick(t, a, wakeThenStep)
	}
	if h.wnd.framePending {
		t.Error("a frame was committed behind a compositor that is not reading")
	}
	if !h.wnd.dirty {
		t.Error("the skipped frame is no longer owed")
	}
	if got := f.ctx.Backlog(); got != backlog {
		t.Errorf("%d passes moved the backlog from %d to %d bytes, want it untouched", passes, backlog, got)
	}
	perPass := float64(sendmsgCalls(t, f.ctx)-sends) / passes
	t.Logf("stalled pass: %.1f sendmsg tries", perPass)
	if perPass > 4 {
		t.Errorf("a stalled pass tried sendmsg %.1f times, want at most 4", perPass)
	}

	var commits atomic.Int64
	go f.drain(&commits)
	for range 1000 {
		if h.wnd.framePending {
			break
		}
		boundedTick(t, a, func() error {
			if h.wnd.framePending {
				f.sess.WakeAfter(0)
			}
			return f.sess.Step()
		})
	}
	if !h.wnd.framePending || h.wnd.lastW != 80 {
		t.Fatalf("the owed frame never went out once the peer read: pending %v, width %d", h.wnd.framePending, h.wnd.lastW)
	}
	boundedTick(t, a, wakeThenStep)
	for deadline := time.Now().Add(10 * time.Second); commits.Load() != filled+1; {
		if time.Now().After(deadline) {
			t.Fatalf("the peer read %d commits, want %d, with %d bytes unsent", commits.Load(), filled+1, f.ctx.Backlog())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStalledLoopWakesOnlyForWritableSocket(t *testing.T) {
	f := newWireFixture(t, "wayland-stall-park")
	f.ctx.SetBuffered(true)
	surf, err := f.comp.CreateSurface()
	if err != nil {
		t.Fatal(err)
	}
	for pending := false; !pending; {
		if err := surf.Commit(); err != nil {
			t.Fatal(err)
		}
		if pending, err = f.ctx.Flush(); err != nil {
			t.Fatal(err)
		}
	}
	var passes atomic.Int64
	var stop atomic.Bool
	done := make(chan error, 1)
	go func() {
		for !stop.Load() {
			if err := f.sess.Step(); err != nil {
				done <- err
				return
			}
			passes.Add(1)
		}
		done <- nil
	}()
	time.Sleep(100 * time.Millisecond)
	if n := passes.Load(); n > 2 {
		t.Fatalf("a stalled loop made %d passes with the peer not reading, want it parked", n)
	}
	var commits atomic.Int64
	go f.drain(&commits)
	for deadline := time.Now().Add(10 * time.Second); f.ctx.Backlog() != 0; {
		if time.Now().After(deadline) {
			t.Fatalf("a parked loop never flushed after the peer read: %d bytes left after %d passes", f.ctx.Backlog(), passes.Load())
		}
		time.Sleep(time.Millisecond)
	}
	if n := passes.Load(); n > 1000 {
		t.Fatalf("the loop spun %d passes to drain", n)
	}
	stop.Store(true)
	f.sess.WakeAfter(0)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("step: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the loop did not stop on a wake")
	}
}

func TestWireFrameSendmsgPerPass(t *testing.T) {
	for _, tc := range []struct {
		name    string
		windows int
		want    int64
	}{
		{"one window", 1, 1},
		{"three windows", 3, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testbuild.SkipAllocBudget(t)
			f := newWireFixture(t, "wayland-sends")
			f.ctx.SetBuffered(true)
			a := NewTestApplication(t, f.sess)
			var hs []*paintHarness
			for range tc.windows {
				hs = append(hs, f.window(t, a))
			}
			var commits atomic.Int64
			go f.drain(&commits)
			step := func() error {
				f.sess.WakeAfter(0)
				return f.sess.Step()
			}
			frame := func() {
				for _, h := range hs {
					h.wnd.dirty, h.wnd.framePending, h.wnd.frame.Ready = true, false, false
					for _, b := range h.bufs {
						if b.Busy() {
							b.Release()
						}
					}
					h.wnd.router.Root.(*widget.DrawingArea).QueueDraw()
				}
				boundedTick(t, a, step)
			}
			frame()
			frame()
			const frames = 50
			before := sendmsgCalls(t, f.ctx)
			committed := commits.Load()
			for range frames {
				frame()
			}
			got := sendmsgCalls(t, f.ctx) - before
			t.Logf("%d frames over %d windows: %d sendmsg", frames, tc.windows, got)
			if got > frames*tc.want {
				t.Errorf("%d passes took %d sendmsg, want %d per pass", frames, got, tc.want)
			}
			for deadline := time.Now().Add(10 * time.Second); commits.Load()-committed < int64(frames*tc.windows); {
				if time.Now().After(deadline) {
					t.Fatalf("the peer saw %d commits, want %d", commits.Load()-committed, frames*tc.windows)
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}
