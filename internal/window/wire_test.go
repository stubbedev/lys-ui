// Wire-level coverage over a loopback unix socket: window.New and the
// resize/limit requests are pinned by their actual bytes, and the
// close/configure events are driven through the real proxy dispatch
// path. No compositor is involved - the far end only drains frames.
package window

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/headlesstest"
)

// xdg_toplevel request opcodes the tests match on.
const (
	opResize          = 6
	opSetMaxSize      = 7
	opSetMinSize      = 8
	opSetMaximized    = 9
	opUnsetMaximized  = 10
	opSetFullscreen   = 11
	opUnsetFullscreen = 12
	opSetMinimized    = 13
)

// newWireWindow builds a real window over the loopback connection.
func newWireWindow(t *testing.T, cfg Config) *Window {
	t.Helper()
	disp, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disp.Context().Close() })
	wmBase := xdg.NewWmBase(disp.Context())
	surf := wl.NewSurface(disp.Context())
	// The fake compositor sends no registry burst, so these two proxies
	// have nothing that would carry their ids to the wire - which would
	// leave the next request waiting on the connection's ordering rule
	// forever. Unregister drops them from the bookkeeping (and clears
	// the wait); the requests below still go out with their ids.
	wmBase.Unregister()
	surf.Unregister()
	w, err := New(wmBase, surf, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestSizeLimitRequests(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{
		Title: "t", AppID: "t",
		MinWidth: 320, MinHeight: 200,
		MaxWidth: 800, MaxHeight: 600,
	})

	t.Run("set_min_size carries the configured minimum", func(t *testing.T) {
		f := headlesstest.WaitForFrame(t, frames, "set_min_size", func(f headlesstest.WireFrame) bool {
			return f.Opcode == opSetMinSize
		})
		if len(f.Body) != 8 || f.Word(0) != 320 || f.Word(1) != 200 {
			t.Errorf("set_min_size payload %v, want (320, 200)", f.Body)
		}
	})

	t.Run("set_max_size carries the configured maximum", func(t *testing.T) {
		f := headlesstest.WaitForFrame(t, frames, "set_max_size", func(f headlesstest.WireFrame) bool {
			return f.Opcode == opSetMaxSize
		})
		if len(f.Body) != 8 || f.Word(0) != 800 || f.Word(1) != 600 {
			t.Errorf("set_max_size payload %v, want (800, 600)", f.Body)
		}
	})

	t.Run("the limits are tracked client-side", func(t *testing.T) {
		minW, minH, maxW, maxH := w.SizeLimits()
		if minW != 320 || minH != 200 || maxW != 800 || maxH != 600 {
			t.Errorf("SizeLimits = %d,%d,%d,%d, want 320,200,800,600", minW, minH, maxW, maxH)
		}
	})
}

func TestNoLimitRequestsWhenUnconstrained(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})
	drain := func() {
		for {
			select {
			case <-frames:
			default:
				return
			}
		}
	}
	drain()
	time.Sleep(20 * time.Millisecond) // settle: nothing else sends here
	drain()

	// Live limit updates go out on the wire and land in the tracked state.
	if err := w.SetMinSize(120, 80); err != nil {
		t.Fatal(err)
	}
	f := headlesstest.WaitForFrame(t, frames, "set_min_size", func(f headlesstest.WireFrame) bool {
		return f.Opcode == opSetMinSize
	})
	if len(f.Body) != 8 || f.Word(0) != 120 || f.Word(1) != 80 {
		t.Errorf("set_min_size payload %v, want (120, 80)", f.Body)
	}
	if err := w.SetMaxSize(0, 0); err != nil {
		t.Fatal(err)
	}
	f = headlesstest.WaitForFrame(t, frames, "set_max_size", func(f headlesstest.WireFrame) bool {
		return f.Opcode == opSetMaxSize
	})
	if len(f.Body) != 8 || f.Word(0) != 0 || f.Word(1) != 0 {
		t.Errorf("set_max_size payload %v, want (0, 0)", f.Body)
	}
	if minW, minH, _, _ := w.SizeLimits(); minW != 120 || minH != 80 {
		t.Errorf("min limits = %dx%d, want 120x80", minW, minH)
	}
}

func TestResizeRequest(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})

	t.Run("the grab carries the seat, serial, and edges", func(t *testing.T) {
		seat := wl.NewSeat(w.Toplevel.Context())
		if err := w.Resize(seat, 4242, xdg.ToplevelResizeEdgeBottomRight); err != nil {
			t.Fatal(err)
		}
		f := headlesstest.WaitForFrame(t, frames, "resize", func(f headlesstest.WireFrame) bool {
			return f.Opcode == opResize
		})
		if len(f.Body) != 12 {
			t.Fatalf("resize payload %v, want 3 words", f.Body)
		}
		if f.Word(0) != int32(seat.Id()) {
			t.Errorf("seat = %d, want the seat proxy id %d", f.Word(0), seat.Id())
		}
		if f.Word(1) != 4242 {
			t.Errorf("serial = %d, want 4242", f.Word(1))
		}
		if f.Word(2) != int32(xdg.ToplevelResizeEdgeBottomRight) {
			t.Errorf("edges = %d, want %d", f.Word(2), xdg.ToplevelResizeEdgeBottomRight)
		}
	})

	t.Run("a none edge or missing seat is a no-op", func(t *testing.T) {
		if err := w.Resize(nil, 6, xdg.ToplevelResizeEdgeBottomRight); err != nil {
			t.Fatal(err)
		}
		if err := w.Resize(nil, 7, xdg.ToplevelResizeEdgeNone); err != nil {
			t.Fatal(err)
		}
		select {
		case f := <-frames:
			if f.Opcode == opResize {
				t.Error("a guarded resize reached the wire")
			}
		default:
		}
	})
}

// TestConfigureDispatchUpdatesSize drives the handshake through the
// real proxy dispatch path - the same events a compositor delivers -
// and pins that a non-zero configure resizes the window.
func TestConfigureDispatchUpdatesSize(t *testing.T) {
	headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})

	data := make([]byte, 12)
	binary.NativeEndian.PutUint32(data[0:4], 640)
	binary.NativeEndian.PutUint32(data[4:8], 400)
	// states: an empty array (length word only)
	w.Toplevel.Dispatch(&wl.Event{Opcode: 0, Data: data})
	if gw, gh := w.Size(); gw != 640 || gh != 400 {
		t.Errorf("size after configure = %dx%d, want 640x400", gw, gh)
	}

	serial := make([]byte, 4)
	binary.NativeEndian.PutUint32(serial, 9)
	w.XdgSurface.Dispatch(&wl.Event{Opcode: 0, Data: serial})
	if err := w.EnsureUsable(); err != nil {
		t.Errorf("handshake did not complete: %v", err)
	}

	// A zero-axis configure keeps the previous size (the Hyprland quirk).
	zero := make([]byte, 12)
	w.Toplevel.Dispatch(&wl.Event{Opcode: 0, Data: zero})
	if gw, gh := w.Size(); gw != 640 || gh != 400 {
		t.Errorf("zero configure clobbered size: %dx%d", gw, gh)
	}
}

// TestConfigureDispatchParsesStates drives a configure with a state
// array through the real proxy dispatch path - the wire encoding a
// compositor sends - and pins that the array decodes into the
// confirmed state set alongside the size.
func TestConfigureDispatchParsesStates(t *testing.T) {
	headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})

	data := make([]byte, 20)
	binary.NativeEndian.PutUint32(data[0:4], 800)
	binary.NativeEndian.PutUint32(data[4:8], 600)
	// states: array of 32-bit values, byte length first.
	binary.NativeEndian.PutUint32(data[8:12], 8)
	binary.NativeEndian.PutUint32(data[12:16], xdg.ToplevelStateActivated)
	binary.NativeEndian.PutUint32(data[16:20], xdg.ToplevelStateMaximized)
	w.Toplevel.Dispatch(&wl.Event{Opcode: 0, Data: data})

	s := w.State()
	if !s.Activated || !s.Maximized || s.Fullscreen {
		t.Errorf("state = %s, want activated+maximized", s)
	}
	if gw, gh := w.Size(); gw != 800 || gh != 600 {
		t.Errorf("size = %dx%d, want 800x600", gw, gh)
	}
}

// TestWindowStateRequests pins the state requests by their bytes: the
// set/unset pairs go out as the protocol numbers them, and
// set_fullscreen carries a nil output (compositor's choice).
func TestWindowStateRequests(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})
	drain := func() {
		for {
			select {
			case <-frames:
			default:
				return
			}
		}
	}
	drain()

	requests := []struct {
		name    string
		opcode  uint32
		send    func() error
		words   int
		payload int32
	}{
		{"set_maximized", opSetMaximized, w.Maximize, 0, 0},
		{"unset_maximized", opUnsetMaximized, w.Unmaximize, 0, 0},
		{"set_fullscreen", opSetFullscreen, func() error { return w.Fullscreen(nil) }, 1, 0},
		{"unset_fullscreen", opUnsetFullscreen, w.Unfullscreen, 0, 0},
		{"set_minimized", opSetMinimized, w.SetMinimized, 0, 0},
	}
	for _, req := range requests {
		t.Run(req.name+" carries the protocol opcode", func(t *testing.T) {
			drain()
			if err := req.send(); err != nil {
				t.Fatal(err)
			}
			f := headlesstest.WaitForFrame(t, frames, req.name, func(f headlesstest.WireFrame) bool {
				return f.Opcode == req.opcode
			})
			if len(f.Body) != 4*req.words {
				t.Fatalf("%s payload %v, want %d word(s)", req.name, f.Body, req.words)
			}
			if req.words == 1 && f.Word(0) != req.payload {
				t.Errorf("%s output = %d, want %d (nil: compositor picks)", req.name, f.Word(0), req.payload)
			}
		})
	}

	t.Run("the requests never touch the reported state", func(t *testing.T) {
		if s := w.State(); s != (State{}) {
			t.Errorf("state = %s after requests, want empty until a configure confirms", s)
		}
	})
}

// TestProtocolCloseRoutesThroughVeto pins the veto on the actual
// protocol path: xdg_toplevel.close dispatched through the wire proxy
// honors the installed veto.
func TestProtocolCloseRoutesThroughVeto(t *testing.T) {
	t.Run("a vetoing window survives the close event", func(t *testing.T) {
		headlesstest.WireServer(t, "wayland-test")
		w := newWireWindow(t, Config{Title: "t", AppID: "t"})
		vetoed := false
		w.SetCloseRequest(func() bool {
			vetoed = true
			return false
		})
		w.Toplevel.Dispatch(&wl.Event{Opcode: 1})
		if !vetoed {
			t.Fatal("the close event never reached the veto; it did not route")
		}
		if w.Closed() {
			t.Error("vetoed protocol close destroyed the window")
		}
	})

	t.Run("an unvetted window closes", func(t *testing.T) {
		headlesstest.WireServer(t, "wayland-test")
		w := newWireWindow(t, Config{Title: "t", AppID: "t"})
		w.Toplevel.Dispatch(&wl.Event{Opcode: 1})
		if !w.Closed() {
			t.Error("protocol close did not close the window")
		}
	})
}

// TestDialogModalityRequests pins the xdg-dialog-v1 wire contract
// (#61): a parented window's SetModal creates one xdg_dialog_v1 object
// and sends set_modal, a later call reuses the object for unset_modal,
// closing the window unsets and destroys it, and a missing manager
// (the compositor without the global) stays silent - the
// application-level block is the floor.
func TestDialogModalityRequests(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-test")
	disp, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disp.Context().Close() })
	ctx := disp.Context()
	// The fake compositor sends no registry burst; unregister every
	// hand-made proxy so requests referencing them do not wait on the
	// connection's ordering rule (see newWireWindow).
	wmBase := xdg.NewWmBase(ctx)
	wmBase.Unregister()
	mgr := wlr.NewWmDialogV1(ctx)
	mgr.Unregister()

	newWin := func(title string) *Window {
		surf := wl.NewSurface(ctx)
		surf.Unregister()
		w, err := New(wmBase, surf, Config{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	parent := newWin("parent")
	dialog := newWin("dialog")
	dialog.SetParent(parent)

	t.Run("set, unset, and close on the wire", func(t *testing.T) {
		if err := dialog.SetModal(mgr, true); err != nil {
			t.Fatal(err)
		}
		if !dialog.ModalHinted() {
			t.Fatal("SetModal(true) left no live hint")
		}
		f := headlesstest.WaitForFrame(t, frames, "get_xdg_dialog", func(f headlesstest.WireFrame) bool {
			return f.Obj == uint32(mgr.Id()) && f.Opcode == 1
		})
		if len(f.Body) != 8 {
			t.Fatalf("get_xdg_dialog payload %v, want 2 words", f.Body)
		}
		headlesstest.WaitForFrame(t, frames, "set_modal", func(f headlesstest.WireFrame) bool {
			return f.Obj == uint32(dialog.dialog.Id()) && f.Opcode == 1
		})

		if err := dialog.SetModal(mgr, false); err != nil {
			t.Fatal(err)
		}
		if dialog.ModalHinted() {
			t.Error("SetModal(false) left the hint live")
		}
		headlesstest.WaitForFrame(t, frames, "unset_modal", func(f headlesstest.WireFrame) bool {
			return f.Obj == uint32(dialog.dialog.Id()) && f.Opcode == 2
		})

		// Close drops the hint and the object: unset_modal, destroy.
		dlgObj := dialog.dialog
		dialog.Close()
		waitFrame := func(name string, opcode uint32) {
			t.Helper()
			headlesstest.WaitForFrame(t, frames, name, func(f headlesstest.WireFrame) bool {
				return f.Obj == uint32(dlgObj.Id()) && f.Opcode == opcode
			})
		}
		waitFrame("unset on close", 2)
		waitFrame("destroy on close", 0)
		if dialog.dialog != nil {
			t.Error("close kept the xdg_dialog_v1 object alive")
		}
	})

	t.Run("a missing manager is silent", func(t *testing.T) {
		other := newWin("other")
		// Drain the window-creation noise through set_app_id (always
		// sent, even empty), so the silence below is about the modality
		// request, not leftovers.
		headlesstest.WaitForFrame(t, frames, "setup", func(f headlesstest.WireFrame) bool {
			return f.Obj == uint32(other.Toplevel.Id()) && f.Opcode == 3
		})
		if err := other.SetModal(nil, true); err != nil {
			t.Fatal(err)
		}
		if other.ModalHinted() {
			t.Error("a nil manager hinted modality")
		}
		select {
		case f := <-frames:
			t.Errorf("request reached the wire without a manager: %+v", f)
		default:
		}
	})
}

func wireString(s string) []byte {
	n := len(s) + 1
	b := make([]byte, 4+(n+3)/4*4)
	binary.NativeEndian.PutUint32(b, uint32(n))
	copy(b[4:], s)
	return b
}

func TestExportRequestsAHandleAndDropsItOnClose(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{Title: "t"})
	exp := wlr.NewZxdgExporterV2(w.WLSurface.Context())
	exp.Unregister()
	if err := w.Export(exp); err != nil {
		t.Fatal(err)
	}
	f := headlesstest.WaitForFrame(t, frames, "export_toplevel", func(f headlesstest.WireFrame) bool {
		return f.Obj == uint32(exp.Id()) && f.Opcode == 1
	})
	if len(f.Body) != 8 || f.Word(1) != int32(w.WLSurface.Id()) {
		t.Fatalf("export_toplevel payload %v, want the new id and the window's wl_surface", f.Body)
	}
	if w.ExportHandle() != "" {
		t.Error("a handle before the compositor sent one")
	}
	w.exported.Dispatch(&wl.Event{Opcode: 0, Data: wireString("f00d-handle")})
	if got := w.ExportHandle(); got != "f00d-handle" {
		t.Errorf("handle %q after the handle event", got)
	}
	first := w.exported
	if err := w.Export(exp); err != nil || w.exported != first {
		t.Error("a second Export replaced the first")
	}
	w.Toplevel.Dispatch(&wl.Event{Opcode: 1})
	headlesstest.WaitForFrame(t, frames, "destroy on close", func(f headlesstest.WireFrame) bool {
		return f.Obj == uint32(first.Id()) && f.Opcode == 0
	})
	if w.ExportHandle() != "" {
		t.Error("a closed window kept its handle")
	}
	other := newWireWindow(t, Config{Title: "u"})
	if err := other.Export(nil); err != nil || other.exported != nil {
		t.Error("a nil exporter exported")
	}
}

// Close destroys the toplevel and then its xdg_surface, so the window
// leaves the screen: the close only marked it closed once, and a
// multi-window application's closed window stayed mapped until the
// process exited (#278). A second Close sends nothing.
func TestCloseDestroysTheRole(t *testing.T) {
	frames := headlesstest.WireServer(t, "wayland-test")
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})
	toplevel, xdgSurface := uint32(w.Toplevel.Id()), uint32(w.XdgSurface.Id())
	w.Close()
	w.Close()
	_ = w.SetTitle("after close")
	if w.Toplevel != nil || w.XdgSurface != nil {
		t.Fatal("the closed window kept its role objects")
	}
	destroyed := func(id uint32) func(headlesstest.WireFrame) bool {
		return func(f headlesstest.WireFrame) bool { return f.Obj == id && f.Opcode == 0 }
	}
	headlesstest.WaitForFrame(t, frames, "xdg_toplevel.destroy", destroyed(toplevel))
	headlesstest.WaitForFrame(t, frames, "xdg_surface.destroy", destroyed(xdgSurface))
	select {
	case f := <-frames:
		t.Errorf("a request followed the teardown: object %d opcode %d", f.Obj, f.Opcode)
	case <-time.After(100 * time.Millisecond):
	}
}
