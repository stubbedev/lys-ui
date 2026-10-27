// Window-state coverage at the loop level: a configure that confirms
// fullscreen (or maximize) rides the same relayout path as any other
// configure - the first frame at the new size is already correct - a
// state-only configure relayouts nothing, and the confirmed
// maximized/fullscreen state retires the client's own edge handles,
// keeping the decoration rules intact when a compositor drops its
// server-side decorations in fullscreen.
package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/window"
	"github.com/stubbedev/lys-ui/widget"
)

// stateHost plays the compositor's configure path against a real
// (wire-free) window.Window: configures go through the window's own
// handlers, and Size/State report what the window holds - so stateful
// tests run the real state machine under the real hostWindow pipeline.
type stateHost struct {
	win  *window.Window
	surf wl.Surface
}

func (h *stateHost) EnsureUsable() error      { return h.win.EnsureUsable() }
func (h *stateHost) Closed() bool             { return h.win.Closed() }
func (h *stateHost) Size() (int, int)         { return h.win.Size() }
func (h *stateHost) HostSurface() *wl.Surface { return &h.surf }
func (h *stateHost) State() window.State      { return h.win.State() }

// configure plays one toplevel configure round: the state array and the
// size the compositor confirms, then the ack-carrying surface
// configure.
func (h *stateHost) configure(w, ht int, states ...int32) {
	h.win.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
		Width: int32(w), Height: int32(ht), States: states,
	})
	h.win.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{})
}

// newStateHarness is a paint harness whose host is the real window
// state machine: everything else behaves like newPaintHarness.
func newStateHarness(root widget.Widget, w, hgt int) (*paintHarness, *stateHost) {
	host := &stateHost{win: &window.Window{}}
	host.configure(w, hgt)
	ph := newPaintHarness(root, w, hgt)
	ph.wnd.host = host
	ph.wnd.state = host.State
	return ph, host
}

// TestFullscreenConfigureRelayouts pins the reactive relayout on the
// fullscreen state transition: the output-sized configure that confirms
// fullscreen makes the very first frame measure, arrange, paint, and
// damage at the output size through the one syncSize path.
func TestFullscreenConfigureRelayouts(t *testing.T) {
	progress := widget.NewProgressBar(0)
	root := widget.NewBox(widget.Column, 8, 8)
	root.Append(progress, true)
	h, host := newStateHarness(root, 420, 280)
	h.frame()

	host.configure(1280, 800, xdg.ToplevelStateFullscreen, xdg.ToplevelStateActivated)
	if !host.State().Fullscreen {
		t.Fatalf("state = %s, want the compositor-confirmed fullscreen", host.State())
	}
	if !h.wnd.syncSize() {
		t.Fatal("the fullscreen-size configure was not picked up")
	}
	if !h.wnd.dirty {
		t.Fatal("the configure did not schedule a repaint")
	}
	commits := h.surf.commits
	painted, damage := h.frame()

	if got := render.UnionAll(damage); got != (render.Rect{W: 1280, H: 800}) {
		t.Errorf("first frame damage %+v, want the full 1280x800", got)
	}
	last := h.bufs[len(h.bufs)-1]
	if last.Width != 1280 || last.Height != 800 {
		t.Errorf("buffer = %dx%d, want the 1280x800 output size", last.Width, last.Height)
	}
	if painted < 1280*800*9/10 {
		t.Errorf("painted %d px, want a full repaint at the output size", painted)
	}
	if got := progress.Bounds(); got != (render.Rect{X: 8, Y: 8, W: 1280 - 16, H: 800 - 16}) {
		t.Errorf("bar bounds %+v, want the 1280x800 arrangement", got)
	}
	if h.surf.commits != commits+1 {
		t.Errorf("%d commits for one configure, want exactly one frame", h.surf.commits-commits)
	}

	// Leaving fullscreen restores the window size through the same path.
	host.configure(420, 280)
	if !h.wnd.syncSize() {
		t.Fatal("the restore-size configure was not picked up")
	}
	_, damage = h.frame()
	if got := render.UnionAll(damage); got != (render.Rect{W: 420, H: 280}) {
		t.Errorf("restore damage %+v, want the full 420x280", got)
	}
}

// TestStateOnlyConfigureDoesNotRelayout pins the confirmed-state model
// at the loop level: a configure that only carries states (zero size
// axes) moves the window's reported state but nothing in the loop
// state - the size is unchanged, so no pool resize and no repaint is
// scheduled. States ride the size change; they never drive their own
// second relayout path.
func TestStateOnlyConfigureDoesNotRelayout(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h, host := newStateHarness(root, 420, 280)
	h.frame()
	h.wnd.dirty = false // exactly what the loop looks like after an idle frame

	host.configure(0, 0, xdg.ToplevelStateMaximized)
	if !host.State().Maximized {
		t.Fatalf("state = %s, want the confirmed maximized", host.State())
	}
	if h.wnd.dirty {
		t.Error("a state-only configure scheduled a repaint")
	}
	if h.wnd.syncSize() {
		t.Error("a state-only configure resized the loop state")
	}
}

// TestConfirmedStateRetiresClientEdges pins the decoration rules under
// state changes: a compositor-confirmed maximized or fullscreen window
// has no client edge handles (it fills the screen edge-to-edge),
// whether or not the compositor kept its server-side decorations; a
// server-decorated window is passive anyway; and restoring the state
// brings the client's edges back.
func TestConfirmedStateRetiresClientEdges(t *testing.T) {
	h, host := newStateHarness(widget.NewBox(widget.Row, 0, 0), 420, 280)
	edgeAt := func() uint32 { return h.wnd.resizeEdgeAt(2, 2) }

	if edgeAt() == 0 {
		t.Error("a normal window lost its client edge handles")
	}

	host.configure(1280, 800, xdg.ToplevelStateFullscreen)
	if e := edgeAt(); e != 0 {
		t.Errorf("fullscreen window still reports edges (%d)", e)
	}

	host.configure(1280, 800, xdg.ToplevelStateMaximized)
	if e := edgeAt(); e != 0 {
		t.Errorf("maximized window still reports edges (%d)", e)
	}

	host.configure(420, 280)
	if edgeAt() == 0 {
		t.Error("a restored window did not regain its client edge handles")
	}

	// Server-side decorations keep gating the edges first, state or no
	// state: a compositor that drops them in fullscreen changes nothing
	// client-side.
	h.wnd.decorated = func() bool { return true }
	host.configure(1280, 800, xdg.ToplevelStateFullscreen)
	if e := edgeAt(); e != 0 {
		t.Errorf("server-decorated fullscreen window still reports edges (%d)", e)
	}
}
