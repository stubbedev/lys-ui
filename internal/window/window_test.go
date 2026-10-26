package window

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"
)

func TestConfigureHandshake(t *testing.T) {
	t.Run("surface configure marks configured and records nothing alone", func(t *testing.T) {
		w := &Window{}
		w.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{Serial: 7})
		if !w.configured {
			t.Error("surface configure must set the configured flag")
		}
		if gotW, gotH := w.Size(); gotW != 0 || gotH != 0 {
			t.Errorf("size = %dx%d, want 0x0 (surface configure carries no size)", gotW, gotH)
		}
	})

	t.Run("toplevel configure records the size", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 320, Height: 200})
		gotW, gotH := w.Size()
		if gotW != 320 || gotH != 200 {
			t.Errorf("size = %dx%d, want 320x200", gotW, gotH)
		}
	})

	t.Run("zero axes keep the previous size", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 320, Height: 200})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{})
		gotW, gotH := w.Size()
		if gotW != 320 || gotH != 200 {
			t.Errorf("zero configure clobbered size: %dx%d", gotW, gotH)
		}
	})

	t.Run("leaving fullscreen returns a zero configure to the unconstrained size", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 420, Height: 280})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 1280, Height: 800, States: []int32{xdg.ToplevelStateFullscreen}})
		if gotW, gotH := w.Size(); gotW != 1280 || gotH != 800 {
			t.Fatalf("fullscreen size = %dx%d", gotW, gotH)
		}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{})
		if gotW, gotH := w.Size(); gotW != 420 || gotH != 280 {
			t.Errorf("after fullscreen = %dx%d, want the floating 420x280", gotW, gotH)
		}
	})

	t.Run("a zero configure inside a dictated state keeps the size", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 420, Height: 280})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 1280, Height: 800, States: []int32{xdg.ToplevelStateMaximized}})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{States: []int32{xdg.ToplevelStateMaximized, xdg.ToplevelStateActivated}})
		if gotW, gotH := w.Size(); gotW != 1280 || gotH != 800 {
			t.Errorf("still maximized = %dx%d, want 1280x800", gotW, gotH)
		}
	})

	t.Run("a concrete size after a dictated state wins", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 420, Height: 280})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 1280, Height: 800, States: []int32{xdg.ToplevelStateFullscreen}})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 500, Height: 300})
		if gotW, gotH := w.Size(); gotW != 500 || gotH != 300 {
			t.Errorf("size = %dx%d, want the configured 500x300", gotW, gotH)
		}
	})

	t.Run("a second configure resizes", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 320, Height: 200})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 640, Height: 400})
		gotW, gotH := w.Size()
		if gotW != 640 || gotH != 400 {
			t.Errorf("size = %dx%d, want 640x400", gotW, gotH)
		}
	})
}

// TestConfigureStateTracking pins the confirmed-state model: the state
// array of each configure is the truth, requests are only hints. A
// configure without a state the client asked for reports the request as
// refused - what the compositor confirms wins, never what we asked for.
func TestConfigureStateTracking(t *testing.T) {
	t.Run("the state array decodes into the confirmed set", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			Width: 800, Height: 600,
			States: []int32{xdg.ToplevelStateActivated, xdg.ToplevelStateMaximized},
		})
		s := w.State()
		if !s.Activated || !s.Maximized || s.Fullscreen {
			t.Errorf("state = %s, want activated+maximized", s)
		}
	})

	t.Run("the tiled and suspended bits decode", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			States: []int32{
				xdg.ToplevelStateTiledLeft, xdg.ToplevelStateTiledRight,
				xdg.ToplevelStateTiledTop, xdg.ToplevelStateTiledBottom,
				xdg.ToplevelStateSuspended, xdg.ToplevelStateResizing,
				99, // unknown: newer compositors carry states we do not know
			},
		})
		s := w.State()
		if !s.TiledLeft || !s.TiledRight || !s.TiledTop || !s.TiledBottom || !s.Suspended || !s.Resizing {
			t.Errorf("state = %s, want the tiled, suspended, and resizing bits", s)
		}
	})

	t.Run("each configure replaces the state wholesale", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			States: []int32{xdg.ToplevelStateMaximized},
		})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			States: []int32{xdg.ToplevelStateFullscreen},
		})
		s := w.State()
		if s.Maximized || !s.Fullscreen {
			t.Errorf("state = %s, want fullscreen only: a stale state must not survive", s)
		}
	})

	t.Run("a stateless configure reports the request refused", func(t *testing.T) {
		w := &Window{}
		if err := w.Maximize(); err != nil {
			t.Fatal(err)
		}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			Width: 800, Height: 600,
		}) // no states: the compositor said no
		if w.State() != (State{}) {
			t.Errorf("state = %s, want empty: the confirmed set never carries a request", w.State())
		}
	})

	t.Run("a configure confirms the state only after the request", func(t *testing.T) {
		w := &Window{}
		if err := w.Maximize(); err != nil {
			t.Fatal(err)
		}
		if w.State().Maximized {
			t.Fatal("the request itself leaked into the reported state")
		}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			States: []int32{xdg.ToplevelStateMaximized},
		})
		if !w.State().Maximized {
			t.Errorf("state = %s, want the compositor-confirmed maximized", w.State())
		}
	})

	t.Run("an empty configure withdraws a previously confirmed state", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			States: []int32{xdg.ToplevelStateFullscreen},
		})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{})
		if w.State().Fullscreen {
			t.Errorf("state = %s, want fullscreen withdrawn by the empty configure", w.State())
		}
	})

	t.Run("zero size axes still keep the size across state updates", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 320, Height: 200})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{
			States: []int32{xdg.ToplevelStateFullscreen},
		})
		if gw, gh := w.Size(); gw != 320 || gh != 200 {
			t.Errorf("size = %dx%d, want the previous 320x200", gw, gh)
		}
	})

	t.Run("the state renders in a fixed order", func(t *testing.T) {
		got := parseStates([]int32{
			xdg.ToplevelStateTiledBottom, xdg.ToplevelStateFullscreen,
			xdg.ToplevelStateActivated, xdg.ToplevelStateMaximized,
		}).String()
		if got != "activated+maximized+fullscreen+tiled-bottom" {
			t.Errorf("String = %q, want activated+maximized+fullscreen+tiled-bottom", got)
		}
		if s := (State{}).String(); s != "normal" {
			t.Errorf("empty String = %q, want normal", s)
		}
	})
}

// TestStateRequestsOnWirelessWindow pins the wire-free guards: a
// window without a wire records the request as a no-op instead of
// panicking, exactly like Resize and the size limits.
func TestStateRequestsOnWirelessWindow(t *testing.T) {
	w := &Window{}
	for name, fn := range map[string]func() error{
		"maximize":      w.Maximize,
		"unmaximize":    w.Unmaximize,
		"fullscreen":    func() error { return w.Fullscreen(nil) },
		"unfullscreen":  w.Unfullscreen,
		"set_minimized": w.SetMinimized,
	} {
		if err := fn(); err != nil {
			t.Errorf("wire-free %s = %v, want nil", name, err)
		}
	}
}

func TestEnsureUsable(t *testing.T) {
	t.Run("fresh window is not usable", func(t *testing.T) {
		if err := (&Window{}).EnsureUsable(); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("want ErrNotConfigured, got %v", err)
		}
	})

	t.Run("configured window is usable", func(t *testing.T) {
		w := &Window{}
		w.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{})
		if err := w.EnsureUsable(); err != nil {
			t.Errorf("configured window must be usable, got %v", err)
		}
	})

	t.Run("closed window is not usable even after configure", func(t *testing.T) {
		w := &Window{}
		w.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{})
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if err := w.EnsureUsable(); !errors.Is(err, ErrClosed) {
			t.Errorf("want ErrClosed, got %v", err)
		}
		if !w.Closed() {
			t.Error("Closed must report the close event")
		}
	})
}

func TestDecorateNilManager(t *testing.T) {
	t.Run("a nil decoration manager is a silent no-op", func(t *testing.T) {
		if err := (&Window{}).Decorate(nil); err != nil {
			t.Errorf("Decorate(nil) = %v, want nil", err)
		}
	})
}

// TestLimitAndResizeStateOnWirelessWindow pins the wire-free paths: a
// window without a wire records limits, answers ServerDecorated, and
// treats resize grabs as no-ops instead of panicking.
func TestLimitAndResizeStateOnWirelessWindow(t *testing.T) {
	w := &Window{}
	if err := w.SetMinSize(100, 50); err != nil {
		t.Errorf("wire-free SetMinSize = %v, want nil", err)
	}
	if err := w.SetMaxSize(400, 300); err != nil {
		t.Errorf("wire-free SetMaxSize = %v, want nil", err)
	}
	minW, minH, maxW, maxH := w.SizeLimits()
	if minW != 100 || minH != 50 || maxW != 400 || maxH != 300 {
		t.Errorf("SizeLimits = %d,%d,%d,%d, want 100,50,400,300", minW, minH, maxW, maxH)
	}
	if err := w.Resize(nil, 1, 10); err != nil {
		t.Errorf("wire-free Resize = %v, want nil", err)
	}
	if w.ServerDecorated() {
		t.Error("undecorated window reports server decorations")
	}
}

// TestCloseRequestVeto pins the veto semantics: a nil veto accepts
// every close request, a false return keeps the window open, and a
// true return (or an explicit Close) closes it.
func TestCloseRequestVeto(t *testing.T) {
	t.Run("nil veto accepts the close", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if !w.Closed() {
			t.Error("close request did not close the window")
		}
	})

	t.Run("a vetoing callback keeps the window open", func(t *testing.T) {
		w := &Window{}
		w.SetCloseRequest(func() bool { return false })
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if w.Closed() {
			t.Error("vetoed close request closed the window")
		}
	})

	t.Run("an accepting callback closes", func(t *testing.T) {
		w := &Window{}
		count := 0
		w.SetCloseRequest(func() bool {
			count++
			return count >= 2
		})
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if w.Closed() {
			t.Error("first close should have been vetoed")
		}
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if !w.Closed() {
			t.Error("second close should have been accepted")
		}
	})

	t.Run("explicit Close ignores the veto", func(t *testing.T) {
		w := &Window{}
		w.SetCloseRequest(func() bool { return false })
		w.Close()
		if !w.Closed() {
			t.Error("client-side Close was vetoed")
		}
	})
}

// Until the compositor says, every capability counts as offered; its
// wm_capabilities replaces that and is announced.
func TestWmCapabilities(t *testing.T) {
	w := &Window{}
	if c := w.Capabilities(); c.Known || !c.Maximize || !c.Minimize || !c.Fullscreen || !c.WindowMenu {
		t.Errorf("unknown caps = %+v, want all offered", c)
	}
	var heard []Capabilities
	w.OnCapabilities = func(c Capabilities) { heard = append(heard, c) }
	w.HandleToplevelWmCapabilities(xdg.ToplevelWmCapabilitiesEvent{
		Capabilities: []int32{xdg.ToplevelWmCapabilitiesFullscreen},
	})
	c := w.Capabilities()
	if !c.Known || c.Maximize || c.Minimize || c.WindowMenu || !c.Fullscreen {
		t.Errorf("caps = %+v, want fullscreen only", c)
	}
	if len(heard) != 1 || heard[0] != c {
		t.Errorf("heard %+v", heard)
	}
}

// A wireless window survives the requests that need a toplevel.
func TestWirelessMoveAndParent(t *testing.T) {
	w := &Window{}
	if err := w.Move(nil, 1); err != nil {
		t.Errorf("Move on a wireless window: %v", err)
	}
	w.SetParent(nil)
	w.SetParent(&Window{})
}
