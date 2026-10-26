package app

import (
	"errors"
	"testing"

	"github.com/neurlang/wayland/xdg"
	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// AttachHeader offers minimize, maximize, and the double-press until
// the compositor's wm_capabilities say otherwise, and follows each
// change.
func TestAttachHeaderFollowsCapabilities(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	a := accelApp()
	win := &Window{app: a, win: &window.Window{}}
	bar := widget.NewHeaderBar(face, 14)
	var heard int
	win.OnCapabilities(func(WMCapabilities) { heard++ }) // a hook AttachHeader must keep
	a.AttachHeader(win, bar)
	buttons := func() int {
		n := 0
		cl, mn, mx := bar.Controls().Shown()
		for _, on := range []bool{cl, mn, mx} {
			if on {
				n++
			}
		}
		return n
	}
	if bar.OnDoubleClick == nil || buttons() != 3 {
		t.Fatalf("unknown caps: double-press %v, %d buttons", bar.OnDoubleClick != nil, buttons())
	}
	win.win.HandleToplevelWmCapabilities(xdg.ToplevelWmCapabilitiesEvent{
		Capabilities: []int32{xdg.ToplevelWmCapabilitiesMinimize},
	})
	if bar.OnDoubleClick != nil || buttons() != 2 {
		t.Errorf("minimize only: double-press %v, %d buttons", bar.OnDoubleClick != nil, buttons())
	}
	win.win.HandleToplevelWmCapabilities(xdg.ToplevelWmCapabilitiesEvent{})
	if buttons() != 1 {
		t.Errorf("nothing offered: %d buttons, want close alone", buttons())
	}
	if heard != 2 {
		t.Errorf("earlier hook heard %d changes, want 2", heard)
	}
}

// Without the protocols the surface hints report unavailable, and the
// error bell finds no window to ring and stays harmless.
func TestSurfaceHintsWithoutProtocol(t *testing.T) {
	a := accelApp()
	a.sess = &wlsession.Session{}
	win := &Window{app: a, win: &window.Window{}}
	if err := win.SetContentType(ContentGame); !errors.Is(err, wlsession.ErrProtocolUnavailable) {
		t.Errorf("content type: %v", err)
	}
	if err := win.SetOpacity(0.5); !errors.Is(err, wlsession.ErrProtocolUnavailable) {
		t.Errorf("opacity: %v", err)
	}
	if err := a.Bell(nil); !errors.Is(err, wlsession.ErrProtocolUnavailable) {
		t.Errorf("bell: %v", err)
	}
	a.ringFor(widget.NewBox(widget.Row, 0, 0))
}
