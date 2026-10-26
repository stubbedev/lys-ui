package wlsession

import (
	"log/slog"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/logutil"
)

// Touch and pointer gestures. The session binds wl_touch when the seat
// advertises it and routes every contact to the surface it went down
// on - each contact holds its own implicit grab, independent of the
// pointer's - and binds wp_pointer_gestures for a touchpad's
// multi-finger swipe, pinch and hold, routed like pointer motion. The
// widget layer turns both into pointer emulation, gestures, and
// scrolling (widget.TouchTracker, Router.Gesture).

// SurfaceTouchHandler is a SurfacePointerHandler that takes touch:
// contact id, surface-local logical coordinates.
type SurfaceTouchHandler interface {
	HandleTouchDown(id int32, x, y float64, serial uint32)
	HandleTouchMotion(id int32, x, y float64)
	HandleTouchUp(id int32)
	// HandleTouchCancel ends every contact on the surface: the
	// compositor took the touch sequence over (a system gesture).
	HandleTouchCancel()
}

// GestureKind is a touchpad gesture's kind.
type GestureKind uint8

// Touchpad gestures.
const (
	GestureSwipe GestureKind = iota
	GesturePinch
	GestureHold
)

// GesturePhase is where a gesture is in its life.
type GesturePhase uint8

// Gesture phases.
const (
	GestureBegin GesturePhase = iota
	GestureUpdate
	GestureEnd
	GestureCancel
)

// PointerGesture is one touchpad gesture event: on update, DX/DY are
// the centroid's motion since the last event, Scale the pinch relative
// to its start, Rotation the degrees clockwise since the last event.
type PointerGesture struct {
	Kind     GestureKind
	Phase    GesturePhase
	Fingers  int
	DX, DY   float64
	Scale    float64
	Rotation float64
}

// SurfaceGestureHandler is a SurfacePointerHandler that takes
// touchpad gestures.
type SurfaceGestureHandler interface {
	HandlePointerGesture(g PointerGesture)
}

// touchAPI is the release side of wl_touch.
type touchAPI interface {
	Release() error
}

// touchSeat is the optional touch side of seatDevices.
type touchSeat interface {
	GetTouch(l wlclient.TouchListener) (touchAPI, error)
}

// GetTouch implements touchSeat.
func (w wireSeat) GetTouch(l wlclient.TouchListener) (touchAPI, error) {
	t, err := w.seat.GetTouch()
	if err != nil {
		return nil, err
	}
	wlclient.TouchAddListener(t, l)
	return wireTouch{t: t}, nil
}

// wireTouch adapts the generated wl_touch proxy.
type wireTouch struct{ t *wl.Touch }

// Release implements touchAPI.
func (w wireTouch) Release() error { return w.t.Release() }

// wlPointer exposes the generated proxy for the gesture objects.
func (w wirePointer) wlPointer() *wl.Pointer { return w.p }

// handleTouchCapability tracks the touch capability like the pointer
// and keyboard: bind on gain, release and cancel every contact on
// loss.
func (s *Session) handleTouchCapability(has bool) {
	if !has && s.touch != nil {
		debug.Log("seat", "touch capability lost")
		if s.seatVersion >= minSeatReleaseVersion {
			_ = s.touch.Release()
		}
		s.touch = nil
		s.cancelTouches()
	}
	ts, ok := s.seatDev.(touchSeat)
	if has && s.touch == nil && ok {
		t, err := ts.GetTouch(s)
		if err != nil {
			logutil.L().Debug("wlsession: get_touch failed; touch input disabled", slog.Any("err", err))
			return
		}
		s.touch = t
		debug.Log("seat", "touch capability gained")
	}
}

// touchHandler is the touch handler registered for surf, if any.
func (s *Session) touchHandler(surf *wl.Surface) SurfaceTouchHandler {
	h, _ := s.surfaceHandlers[surf].(SurfaceTouchHandler)
	return h
}

// HandleTouchDown implements wl.TouchDownHandler: the contact belongs
// to the surface it landed on until it lifts.
func (s *Session) HandleTouchDown(ev wl.TouchDownEvent) {
	if s.touchFocus == nil {
		s.touchFocus = map[int32]*wl.Surface{}
	}
	s.touchFocus[ev.Id] = ev.Surface
	s.inputSerial = ev.Serial
	if h := s.touchHandler(ev.Surface); h != nil {
		h.HandleTouchDown(ev.Id, float64(ev.X), float64(ev.Y), ev.Serial)
	}
}

// HandleTouchMotion implements wl.TouchMotionHandler.
func (s *Session) HandleTouchMotion(ev wl.TouchMotionEvent) {
	if h := s.touchHandler(s.touchFocus[ev.Id]); h != nil {
		h.HandleTouchMotion(ev.Id, float64(ev.X), float64(ev.Y))
	}
}

// HandleTouchUp implements wl.TouchUpHandler.
func (s *Session) HandleTouchUp(ev wl.TouchUpEvent) {
	surf := s.touchFocus[ev.Id]
	delete(s.touchFocus, ev.Id)
	if h := s.touchHandler(surf); h != nil {
		h.HandleTouchUp(ev.Id)
	}
}

// HandleTouchCancel implements wl.TouchCancelHandler.
func (s *Session) HandleTouchCancel(wl.TouchCancelEvent) { s.cancelTouches() }

// cancelTouches ends every contact on every surface holding one.
func (s *Session) cancelTouches() {
	seen := map[*wl.Surface]bool{}
	for _, surf := range s.touchFocus {
		if seen[surf] {
			continue
		}
		seen[surf] = true
		if h := s.touchHandler(surf); h != nil {
			h.HandleTouchCancel()
		}
	}
	clear(s.touchFocus)
}

// HandleTouchFrame implements wl.TouchFrameHandler: contacts are
// handled as their events arrive.
func (s *Session) HandleTouchFrame(wl.TouchFrameEvent) {}

// HandleTouchShape implements wl.TouchShapeHandler.
func (s *Session) HandleTouchShape(wl.TouchShapeEvent) {}

// HandleTouchOrientation implements wl.TouchOrientationHandler.
func (s *Session) HandleTouchOrientation(wl.TouchOrientationEvent) {}

// maxPointerGesturesVersion is the newest wp_pointer_gestures the
// session speaks (v3 adds hold).
const maxPointerGesturesVersion = 3

// bindPointerGestures binds the touchpad gesture manager.
func (s *Session) bindPointerGestures(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpGesturesV1(ctx)
	if !s.bindOptional(ev, maxPointerGesturesVersion, mgr) {
		return
	}
	s.gestures = mgr
	s.gesturesVersion = min(ev.Version, maxPointerGesturesVersion)
	debug.Log("shell", "pointer-gestures-v1 bound")
	s.ensurePointerGestures()
}

// ensurePointerGestures creates the gesture objects for the bound
// pointer once both the manager and the pointer exist.
func (s *Session) ensurePointerGestures() {
	if s.gestures == nil || s.pointer == nil || s.swipe != nil {
		return
	}
	raw, ok := s.pointer.(interface{ wlPointer() *wl.Pointer })
	if !ok || raw.wlPointer() == nil {
		return
	}
	p := raw.wlPointer()
	if sw, err := s.gestures.GetSwipeGesture(p); err == nil {
		s.swipe = sw
		sw.AddBeginHandler(gestureBegin{s, GestureSwipe})
		sw.AddUpdateHandler(swipeUpdate{s})
		sw.AddEndHandler(gestureEnd{s, GestureSwipe})
	}
	if pi, err := s.gestures.GetPinchGesture(p); err == nil {
		s.pinch = pi
		pi.AddBeginHandler(gestureBegin{s, GesturePinch})
		pi.AddUpdateHandler(pinchUpdate{s})
		pi.AddEndHandler(gestureEnd{s, GesturePinch})
	}
	if s.gesturesVersion >= 3 {
		if ho, err := s.gestures.GetHoldGesture(p); err == nil {
			s.hold = ho
			ho.AddBeginHandler(gestureBegin{s, GestureHold})
			ho.AddEndHandler(gestureEnd{s, GestureHold})
		}
	}
}

// dropPointerGestures destroys the gesture objects with their pointer.
func (s *Session) dropPointerGestures() {
	if s.swipe != nil {
		_ = s.swipe.Destroy()
	}
	if s.pinch != nil {
		_ = s.pinch.Destroy()
	}
	if s.hold != nil {
		_ = s.hold.Destroy()
	}
	s.swipe, s.pinch, s.hold = nil, nil, nil
	s.gestureSurface = nil
}

// routeGesture delivers a touchpad gesture to the surface it began on.
func (s *Session) routeGesture(g PointerGesture) {
	if g.Phase == GestureBegin {
		s.gestureSurface = s.pointerFocus
	}
	if h, ok := s.surfaceHandlers[s.gestureSurface].(SurfaceGestureHandler); ok {
		h.HandlePointerGesture(g)
	}
	if g.Phase == GestureEnd || g.Phase == GestureCancel {
		s.gestureSurface = nil
	}
}

// The generated handler interfaces each name one method, so a small
// adapter per event carries the gesture kind.
type (
	gestureBegin struct {
		s    *Session
		kind GestureKind
	}
	gestureEnd struct {
		s    *Session
		kind GestureKind
	}
	swipeUpdate struct{ s *Session }
	pinchUpdate struct{ s *Session }
)

func (h gestureBegin) begin(fingers uint32) {
	h.s.routeGesture(PointerGesture{Kind: h.kind, Phase: GestureBegin, Fingers: int(fingers), Scale: 1})
}

func (h gestureEnd) end(cancelled int32) {
	phase := GestureEnd
	if cancelled != 0 {
		phase = GestureCancel
	}
	h.s.routeGesture(PointerGesture{Kind: h.kind, Phase: phase, Scale: 1})
}

// HandleZwpGestureSwipeV1Begin implements the swipe begin handler.
func (h gestureBegin) HandleZwpGestureSwipeV1Begin(ev wlr.ZwpGestureSwipeV1BeginEvent) {
	h.begin(ev.Fingers)
}

// HandleZwpGesturePinchV1Begin implements the pinch begin handler.
func (h gestureBegin) HandleZwpGesturePinchV1Begin(ev wlr.ZwpGesturePinchV1BeginEvent) {
	h.begin(ev.Fingers)
}

// HandleZwpGestureHoldV1Begin implements the hold begin handler.
func (h gestureBegin) HandleZwpGestureHoldV1Begin(ev wlr.ZwpGestureHoldV1BeginEvent) {
	h.begin(ev.Fingers)
}

// HandleZwpGestureSwipeV1End implements the swipe end handler.
func (h gestureEnd) HandleZwpGestureSwipeV1End(ev wlr.ZwpGestureSwipeV1EndEvent) { h.end(ev.Cancelled) }

// HandleZwpGesturePinchV1End implements the pinch end handler.
func (h gestureEnd) HandleZwpGesturePinchV1End(ev wlr.ZwpGesturePinchV1EndEvent) { h.end(ev.Cancelled) }

// HandleZwpGestureHoldV1End implements the hold end handler.
func (h gestureEnd) HandleZwpGestureHoldV1End(ev wlr.ZwpGestureHoldV1EndEvent) { h.end(ev.Cancelled) }

// HandleZwpGestureSwipeV1Update implements the swipe update handler.
func (h swipeUpdate) HandleZwpGestureSwipeV1Update(ev wlr.ZwpGestureSwipeV1UpdateEvent) {
	h.s.routeGesture(PointerGesture{Kind: GestureSwipe, Phase: GestureUpdate, DX: float64(ev.Dx), DY: float64(ev.Dy), Scale: 1})
}

// HandleZwpGesturePinchV1Update implements the pinch update handler.
func (h pinchUpdate) HandleZwpGesturePinchV1Update(ev wlr.ZwpGesturePinchV1UpdateEvent) {
	h.s.routeGesture(PointerGesture{
		Kind: GesturePinch, Phase: GestureUpdate, DX: float64(ev.Dx), DY: float64(ev.Dy),
		Scale: float64(ev.Scale), Rotation: float64(ev.Rotation),
	})
}
