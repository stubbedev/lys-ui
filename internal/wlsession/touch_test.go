package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/wlclient"
)

// touchHandler records touch and gesture traffic for one surface.
type touchRecorder struct {
	recordingHandler
	downs, motions, ups, cancels int
	lastID                       int32
	lastSerial                   uint32
	gestures                     []PointerGesture
}

func (h *touchRecorder) HandleTouchDown(id int32, x, y float64, serial uint32) {
	h.downs++
	h.lastID, h.lastSerial = id, serial
}
func (h *touchRecorder) HandleTouchMotion(id int32, x, y float64) { h.motions++; h.lastID = id }
func (h *touchRecorder) HandleTouchUp(id int32)                   { h.ups++; h.lastID = id }
func (h *touchRecorder) HandleTouchCancel()                       { h.cancels++ }
func (h *touchRecorder) HandlePointerGesture(g PointerGesture) {
	h.gestures = append(h.gestures, g)
}

// Each contact routes to the surface it went down on - two fingers on
// two surfaces stay apart - and a cancel ends every surface's contacts
// once.
func TestTouchRouting(t *testing.T) {
	s := newRoutingSession()
	s1, s2 := &wl.Surface{}, &wl.Surface{}
	h1, h2 := &touchRecorder{}, &touchRecorder{}
	s.SetSurfaceInput(s1, h1)
	s.SetSurfaceInput(s2, h2)
	s.HandleTouchDown(wl.TouchDownEvent{Serial: 7, Surface: s1, Id: 1, X: 10, Y: 10})
	s.HandleTouchDown(wl.TouchDownEvent{Serial: 8, Surface: s2, Id: 2, X: 5, Y: 5})
	s.HandleTouchMotion(wl.TouchMotionEvent{Id: 1, X: 12, Y: 10})
	s.HandleTouchMotion(wl.TouchMotionEvent{Id: 2, X: 6, Y: 5})
	s.HandleTouchUp(wl.TouchUpEvent{Id: 1})
	if h1.downs != 1 || h1.motions != 1 || h1.ups != 1 || h1.lastSerial != 7 {
		t.Errorf("surface 1: %+v", h1)
	}
	if h2.downs != 1 || h2.motions != 1 || h2.ups != 0 || h2.lastID != 2 {
		t.Errorf("surface 2: %+v", h2)
	}
	s.HandleTouchCancel(wl.TouchCancelEvent{})
	if h2.cancels != 1 || h1.cancels != 0 || len(s.touchFocus) != 0 {
		t.Errorf("cancel: s1 %d s2 %d left %d", h1.cancels, h2.cancels, len(s.touchFocus))
	}
}

// fakeTouchSeat adds touch to the recorder seat.
type fakeTouchSeat struct {
	fakeSeatDev
	touches  int
	releases int
}

func (f *fakeTouchSeat) GetTouch(wlclient.TouchListener) (touchAPI, error) {
	f.touches++
	return fakeTouch{f}, nil
}

type fakeTouch struct{ seat *fakeTouchSeat }

func (t fakeTouch) Release() error { t.seat.releases++; return nil }

// The touch capability binds once, releases on loss, and cancels the
// contacts in flight.
func TestTouchCapability(t *testing.T) {
	s := newRoutingSession()
	seat := &fakeTouchSeat{}
	s.seatDev, s.seatVersion = seat, 7
	surf, h := &wl.Surface{}, &touchRecorder{}
	s.SetSurfaceInput(surf, h)
	s.handleTouchCapability(true)
	s.handleTouchCapability(true)
	if seat.touches != 1 || s.touch == nil {
		t.Fatalf("binds = %d", seat.touches)
	}
	s.HandleTouchDown(wl.TouchDownEvent{Surface: surf, Id: 3})
	s.handleTouchCapability(false)
	if seat.releases != 1 || s.touch != nil || h.cancels != 1 {
		t.Errorf("loss: releases %d cancels %d", seat.releases, h.cancels)
	}
}

// A touchpad gesture stays with the surface it began on, even when the
// pointer focus moves mid-gesture.
func TestPointerGestureRouting(t *testing.T) {
	s := newRoutingSession()
	s1, s2 := &wl.Surface{}, &wl.Surface{}
	h1, h2 := &touchRecorder{}, &touchRecorder{}
	s.SetSurfaceInput(s1, h1)
	s.SetSurfaceInput(s2, h2)
	s.pointerFocus = s1
	gestureBegin{s, GesturePinch}.begin(2)
	s.pointerFocus = s2
	s.routeGesture(PointerGesture{Kind: GesturePinch, Phase: GestureUpdate, Scale: 1.5, Rotation: 10})
	gestureEnd{s, GesturePinch}.end(0)
	if len(h1.gestures) != 3 || len(h2.gestures) != 0 || h1.gestures[1].Scale != 1.5 || h1.gestures[2].Phase != GestureEnd {
		t.Errorf("gestures: s1 %+v s2 %+v", h1.gestures, h2.gestures)
	}
	gestureEnd{s, GestureSwipe}.end(1)
	if s.gestureSurface != nil {
		t.Error("an ended gesture kept its surface")
	}
}
