package app_test

import (
	"sync/atomic"
	"testing"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/internal/waittest"
	"github.com/stubbedev/lys-ui/widget"
)

const (
	xdgSurfaceGetPopup  = 2
	xdgSurfaceAck       = 4
	popupConfigureEvent = 0
)

type popupPeer struct {
	f          *fakeCompositor
	surfaces   map[uint32]uint32
	xdgSurface atomic.Uint32
	popup      atomic.Uint32
	surface    atomic.Uint32
	attaches   atomic.Int64
	acks       atomic.Int64
	created    chan struct{}
	buffered   chan struct{}
}

func (p *popupPeer) request(iface string, obj, opcode uint32, body []byte) {
	switch {
	case iface == "xdg_wm_base" && opcode == wmBaseGetXdgSurface:
		p.surfaces[word(body, 0)] = word(body, 1)
	case iface == "xdg_surface" && opcode == xdgSurfaceGetPopup:
		p.xdgSurface.Store(obj)
		p.surface.Store(p.surfaces[obj])
		p.popup.Store(word(body, 0))
		close(p.created)
	case iface == "xdg_surface" && opcode == xdgSurfaceAck && obj == p.xdgSurface.Load():
		p.acks.Add(1)
	case iface == "wl_surface" && opcode == surfaceAttach && obj == p.surface.Load() && word(body, 0) != 0:
		p.attaches.Add(1)
		select {
		case p.buffered <- struct{}{}:
		default:
		}
	}
}

func TestPopoverOpensWithoutWaitingForItsConfigure(t *testing.T) {
	p := &popupPeer{surfaces: map[uint32]uint32{}, created: make(chan struct{}), buffered: make(chan struct{}, 1)}
	f, a := connectFakeWith(t, kioskGlobals, func(f *fakeCompositor) {
		p.f = f
		f.onRequest = p.request
	})
	root := widget.NewDrawingArea(fakeSurfaceW, fakeSurfaceH)
	win, err := a.NewWindow(app.WindowConfig{Title: "t", Width: fakeSurfaceW, Height: fakeSurfaceH, Root: root})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	watch := watchLoop(a)
	runUntilCleanup(t, a)
	f.awaitMapped(t, "the toplevel")

	syncsBefore := f.syncs.Load()
	opened := make(chan error, 1)
	a.Invoke(func() {
		_, err := a.OpenPopover(win, app.PopoverConfig{Anchor: root, Content: square(), Dismissal: app.PopoverSticky})
		opened <- err
	})
	if err := waittest.For(t, opened, "OpenPopover to return before the configure has come"); err != nil {
		t.Fatalf("OpenPopover: %v", err)
	}
	waittest.For(t, p.created, "get_popup")
	watch.await(t, pendingTicks)
	if got := f.syncs.Load() - syncsBefore; got != 0 {
		t.Errorf("opening the popover cost %d wl_display.sync, want 0", got)
	}
	if got := p.attaches.Load(); got != 0 {
		t.Errorf("%d buffers reached the surface before its configure", got)
	}

	f.send(p.popup.Load(), popupConfigureEvent, 3, 4, 20, 20)
	f.send(p.xdgSurface.Load(), configureEvent, 77)
	waittest.For(t, p.buffered, "the first frame after the configure")
	if got := p.acks.Load(); got != 1 {
		t.Errorf("%d acks of the configure, want 1", got)
	}
}
