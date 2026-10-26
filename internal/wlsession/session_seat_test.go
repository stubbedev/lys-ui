package wlsession

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/wlclient"
	"github.com/stubbedev/lys-lib/third_party/wayland/wlcursor"
)

// fakePointer records the requests the capability and cursor paths
// make on one wl_pointer proxy.
type fakePointer struct {
	releases  int
	listeners []wlclient.PointerListener
}

func (f *fakePointer) SetCursor(uint32, *wl.Surface, int32, int32) error { return nil }

func (f *fakePointer) Release() error { f.releases++; return nil }
func (f *fakePointer) AddListener(h wlclient.PointerListener) {
	f.listeners = append(f.listeners, h)
}

// fakeKeyboard records the requests the capability path makes on one
// wl_keyboard proxy.
type fakeKeyboard struct {
	releases  int
	listeners []wlclient.KeyboardListener
}

func (f *fakeKeyboard) Release() error { f.releases++; return nil }

func (f *fakeKeyboard) AddListener(h wlclient.KeyboardListener) {
	f.listeners = append(f.listeners, h)
}

// fakeSeatDev records the acquisition traffic the capability path
// generates and hands back recorder proxies, so unplug-replug cycles
// run without a live connection.
type fakeSeatDev struct {
	getPointers  int
	getKeyboards int
	pointerErr   error
	keyboardErr  error
	pointers     []*fakePointer
	keyboards    []*fakeKeyboard
}

func (f *fakeSeatDev) GetPointer() (pointerAPI, error) {
	f.getPointers++
	if f.pointerErr != nil {
		return nil, f.pointerErr
	}
	p := &fakePointer{}
	f.pointers = append(f.pointers, p)
	return p, nil
}

func (f *fakeSeatDev) GetKeyboard() (keyboardAPI, error) {
	f.getKeyboards++
	if f.keyboardErr != nil {
		return nil, f.keyboardErr
	}
	k := &fakeKeyboard{}
	f.keyboards = append(f.keyboards, k)
	return k, nil
}

// seatFixture is a session with a recorder seat and the cursor wire
// faked, so capability transitions exercise the full loss/gain path
// without a compositor.
type seatFixture struct {
	s      *Session
	seat   *fakeSeatDev
	pushes int // cursor frames pushed to the (fake) wire
}

func newSeatFixture() *seatFixture {
	f := &seatFixture{
		s:    &Session{surfaceHandlers: make(map[*wl.Surface]SurfacePointerHandler)},
		seat: &fakeSeatDev{},
	}
	f.s.seatDev = f.seat
	f.s.seatVersion = 7 // modern default: release requests exist
	f.s.crs.surface = &wl.Surface{}
	f.s.crs.theme = func(desired string) (*wlcursor.Cursor, string, error) {
		return fakeCursor(defaultCursor, 1), defaultCursor, nil
	}
	f.s.crs.push = func(*wlcursor.ImageBuffer) error {
		f.pushes++
		return nil
	}
	return f
}

func TestSeatCapabilityTransitions(t *testing.T) {
	f := newSeatFixture()
	surf := &wl.Surface{}
	h := &recordingHandler{}
	f.s.SetSurfaceInput(surf, h)

	t.Run("bind-time arrival binds pointer and keyboard", func(t *testing.T) {
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})
		if f.s.pointer == nil || f.s.keyboard == nil {
			t.Fatal("bind-time capabilities did not bind pointer and keyboard")
		}
		if f.seat.getPointers != 1 || f.seat.getKeyboards != 1 {
			t.Errorf("acquisitions = %d/%d, want 1/1", f.seat.getPointers, f.seat.getKeyboards)
		}
		if len(f.seat.pointers[0].listeners) != 1 || len(f.seat.keyboards[0].listeners) != 1 {
			t.Error("bound proxies left without the session's listeners")
		}
	})

	t.Run("enter applies the cursor on the fresh pointer", func(t *testing.T) {
		f.s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf, Serial: 42})
		if f.pushes != 1 {
			t.Errorf("cursor pushes = %d, want 1 on the first enter", f.pushes)
		}
		if f.s.pointerEnterSerial != 42 {
			t.Errorf("enter serial = %d, want 42", f.s.pointerEnterSerial)
		}
	})

	t.Run("unplug stops routing, clears focus and releases the proxy", func(t *testing.T) {
		f.s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 1}) // implicit grab
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capKeyboard})

		if h.leaves != 1 {
			t.Errorf("focused surface got %d leaves on capability loss, want 1", h.leaves)
		}
		if f.s.grabSurface != nil || f.s.pointerFocus != nil {
			t.Errorf("routing survived the loss: grab=%v focus=%v", f.s.grabSurface, f.s.pointerFocus)
		}
		if f.s.pointer != nil {
			t.Error("the dead pointer proxy was not dropped")
		}
		if got := f.seat.pointers[0].releases; got != 1 {
			t.Errorf("wl_pointer.release count = %d, want 1", got)
		}
		if f.s.pointerEnterSerial != 0 {
			t.Errorf("enter serial = %d after loss, want 0", f.s.pointerEnterSerial)
		}

		// A dead device must not route: no motion, no stray release.
		f.s.HandlePointerMotion(wl.PointerMotionEvent{SurfaceX: 5, SurfaceY: 5})
		f.s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 0})
		if h.motions != 0 || h.releases != 0 {
			t.Errorf("dead pointer routed motion=%d release=%d, want 0/0", h.motions, h.releases)
		}
	})

	t.Run("replug rebinds and re-applies the cursor", func(t *testing.T) {
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})
		if f.seat.getPointers != 2 {
			t.Errorf("get_pointer calls = %d, want a fresh bind on replug", f.seat.getPointers)
		}
		f.s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf, Serial: 43})
		if f.pushes != 2 {
			t.Errorf("cursor pushes = %d, want re-apply on the replugged pointer", f.pushes)
		}
		if f.s.pointerEnterSerial != 43 {
			t.Errorf("enter serial = %d, want 43", f.s.pointerEnterSerial)
		}
	})
}

func TestSeatKeyboardCapabilityTransitions(t *testing.T) {
	f := newSeatFixture()
	f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})

	var keys []uint32
	f.s.OnKey = func(keycode uint32, mods Mods) { keys = append(keys, keycode) }

	t.Run("typing works on the first keyboard", func(t *testing.T) {
		f.s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: &wl.Surface{}})
		f.s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: 30, State: 1})
		if len(keys) != 1 || keys[0] != 30 {
			t.Fatalf("keys = %v, want one press of 30", keys)
		}
	})

	t.Run("super is held, the locks are not", func(t *testing.T) {
		// Mod4 (super) with Num Lock (Mod2) and Caps Lock latched.
		f.s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 1<<6 | 1<<4 | 1<<1, Serial: 6})
		if got := f.s.Mods(); got != ModSuper {
			t.Errorf("mods = %v, want super alone", got)
		}
		f.s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 0, Serial: 6})
	})

	t.Run("unplug clears device state and releases the proxy", func(t *testing.T) {
		f.s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 1, Serial: 7})
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer})
		if f.s.keyboard != nil {
			t.Fatal("the dead keyboard proxy was not dropped")
		}
		if got := f.seat.keyboards[0].releases; got != 1 {
			t.Errorf("wl_keyboard.release count = %d, want 1", got)
		}
		if f.s.Mods() != 0 {
			t.Errorf("mods = %v after keyboard loss, want none", f.s.Mods())
		}
		if f.s.KeyboardFocus() != nil {
			t.Error("keyboard focus survived the keyboard loss")
		}
	})

	t.Run("replug keeps typing working", func(t *testing.T) {
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})
		if f.seat.getKeyboards != 2 {
			t.Fatalf("get_keyboard calls = %d, want a fresh bind on replug", f.seat.getKeyboards)
		}
		f.s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: &wl.Surface{}})
		f.s.HandleKeyboardKey(wl.KeyboardKeyEvent{Key: 31, State: 1})
		if len(keys) != 2 || keys[1] != 31 {
			t.Errorf("keys = %v, want the replugged keyboard delivering 31", keys)
		}
	})
}

func TestSeatCapabilityRapidCycles(t *testing.T) {
	f := newSeatFixture()
	surf := &wl.Surface{}
	h := &recordingHandler{}
	f.s.SetSurfaceInput(surf, h)

	for i := range 100 {
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})
		f.s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf, Serial: uint32(i) + 1})
		f.s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 1}) // grab
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capKeyboard})
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capTouch})
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard | capTouch})
	}
	// Every proxy but the live one was released exactly once — no
	// acquire/release skew across the hundred cycles.
	if got, want := f.seat.pointers[len(f.seat.pointers)-1].releases, 0; got != want {
		t.Errorf("live pointer releases = %d, want %d", got, want)
	}
	var pointerReleases int
	for _, p := range f.seat.pointers {
		pointerReleases += p.releases
	}
	if got, want := pointerReleases, len(f.seat.pointers)-1; got != want {
		t.Errorf("pointer releases = %d, want %d (one per dead proxy)", got, want)
	}
	var keyboardReleases int
	for _, k := range f.seat.keyboards {
		keyboardReleases += k.releases
	}
	if got, want := keyboardReleases, len(f.seat.keyboards)-1; got != want {
		t.Errorf("keyboard releases = %d, want %d (one per dead proxy)", got, want)
	}
	if f.s.pointer == nil || f.s.keyboard == nil {
		t.Error("the final cycle left the session without input devices")
	}
}

func TestSeatCapabilityEdgePaths(t *testing.T) {
	t.Run("an unchanged set is a no-op", func(t *testing.T) {
		f := newSeatFixture()
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})
		if f.seat.getPointers != 1 || f.seat.getKeyboards != 1 {
			t.Errorf("re-sent set re-acquired: %d/%d", f.seat.getPointers, f.seat.getKeyboards)
		}
	})

	t.Run("a failed rebind stays consistent and retries on the next arrival", func(t *testing.T) {
		f := newSeatFixture()
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer})
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{}) // unplug
		f.seat.pointerErr = errors.New("wire dead")
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer})
		if f.s.pointer != nil {
			t.Error("a failed get_pointer must not leave a half-bound pointer")
		}
		f.seat.pointerErr = nil
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{})
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer})
		if f.s.pointer == nil {
			t.Error("the next arrival did not retry the failed bind")
		}
	})

	t.Run("pre-release seats drop the proxy without a release request", func(t *testing.T) {
		f := newSeatFixture()
		f.s.seatVersion = 4 // wl_pointer.release arrives at seat v5
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capPointer | capKeyboard})
		f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{})
		if f.s.pointer != nil || f.s.keyboard != nil {
			t.Fatal("the proxies were not dropped")
		}
		if got := f.seat.pointers[0].releases + f.seat.keyboards[0].releases; got != 0 {
			t.Errorf("release requests = %d on a pre-v5 seat, want 0", got)
		}
	})
}

// The touch bit must be a deliberate no-op: no acquisition, no panic,
// and no interference with the pointer and keyboard bits it travels
// with. See the capTouch comment for why touch stays out.
func TestSeatTouchCapabilityIgnored(t *testing.T) {
	f := newSeatFixture()
	f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capTouch})
	if f.s.pointer != nil || f.s.keyboard != nil {
		t.Error("touch-only capabilities bound a device")
	}
	if f.seat.getPointers+f.seat.getKeyboards != 0 {
		t.Error("touch-only capabilities reached the wire")
	}

	f.s.HandleSeatCapabilities(wl.SeatCapabilitiesEvent{Capabilities: capTouch | capPointer})
	if f.s.pointer == nil {
		t.Error("the pointer bit was lost when travelling with the touch bit")
	}
}
