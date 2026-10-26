package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// recordingHandler captures the pointer callbacks one surface received.
type recordingHandler struct {
	enters, motions, presses, releases, axes, leaves int
	lastX, lastY                                     float64
}

func (h *recordingHandler) HandlePointerEnter(x, y float64) {
	h.enters++
	h.lastX, h.lastY = x, y
}

func (h *recordingHandler) HandlePointerMotion(x, y float64) {
	h.motions++
	h.lastX, h.lastY = x, y
}

func (h *recordingHandler) HandlePointerButton(button, state, serial uint32) {
	if state == 1 {
		h.presses++
	} else {
		h.releases++
	}
}

func (h *recordingHandler) HandlePointerAxis(dx, dy float64) { h.axes++ }

func (h *recordingHandler) HandlePointerLeave() { h.leaves++ }

// newRoutingSession builds a session with routing state but no live
// connection; the routing state machine is pure client logic.
func newRoutingSession() *Session {
	return &Session{surfaceHandlers: make(map[*wl.Surface]SurfacePointerHandler)}
}

func TestPointerRouting(t *testing.T) {
	s := newRoutingSession()
	s1, s2 := &wl.Surface{}, &wl.Surface{}
	h1, h2 := &recordingHandler{}, &recordingHandler{}
	s.SetSurfaceInput(s1, h1)
	s.SetSurfaceInput(s2, h2)

	t.Run("events follow the focused surface", func(t *testing.T) {
		s.HandlePointerEnter(wl.PointerEnterEvent{Surface: s1, SurfaceX: 5, SurfaceY: 7})
		s.HandlePointerMotion(wl.PointerMotionEvent{SurfaceX: 9, SurfaceY: 11})
		if h1.enters != 1 || h1.motions != 1 || h1.lastX != 9 {
			t.Errorf("s1 got enter=%d motion=%d last=(%.0f,%.0f)", h1.enters, h1.motions, h1.lastX, h1.lastY)
		}
		if h2.enters+h2.motions != 0 {
			t.Errorf("s2 received events while s1 had focus")
		}
	})

	t.Run("re-entering another surface switches focus", func(t *testing.T) {
		s.HandlePointerEnter(wl.PointerEnterEvent{Surface: s2, SurfaceX: 1, SurfaceY: 2})
		s.HandlePointerMotion(wl.PointerMotionEvent{SurfaceX: 3, SurfaceY: 4})
		if h2.enters != 1 || h2.motions != 1 {
			t.Errorf("s2 enter=%d motion=%d, want 1/1", h2.enters, h2.motions)
		}
		if h1.motions != 1 {
			t.Errorf("s1 received %d motions, want no new ones", h1.motions)
		}
	})

	t.Run("leave notifies only the left surface", func(t *testing.T) {
		s.HandlePointerLeave(wl.PointerLeaveEvent{Surface: s2})
		if h2.leaves != 1 {
			t.Errorf("s2 leaves = %d, want 1", h2.leaves)
		}
		s.HandlePointerMotion(wl.PointerMotionEvent{})
		if h1.motions+h2.motions != 2 {
			t.Errorf("motion routed after leave with no focus")
		}
	})

	t.Run("events for unregistered surfaces are dropped", func(t *testing.T) {
		orph := &wl.Surface{}
		s.HandlePointerEnter(wl.PointerEnterEvent{Surface: orph})
		s.HandlePointerLeave(wl.PointerLeaveEvent{Surface: orph})
		if h1.enters+h2.enters != 2 {
			t.Errorf("unregistered surface leaked events")
		}
	})
}

func TestImplicitGrab(t *testing.T) {
	s := newRoutingSession()
	s1, s2 := &wl.Surface{}, &wl.Surface{}
	h1, h2 := &recordingHandler{}, &recordingHandler{}
	s.SetSurfaceInput(s1, h1)
	s.SetSurfaceInput(s2, h2)

	t.Run("press grabs, so focus changes do not steal a drag", func(t *testing.T) {
		s.HandlePointerEnter(wl.PointerEnterEvent{Surface: s1})
		s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 1})
		s.HandlePointerEnter(wl.PointerEnterEvent{Surface: s2})
		s.HandlePointerMotion(wl.PointerMotionEvent{SurfaceX: 30, SurfaceY: 40})
		if h1.motions != 1 {
			t.Errorf("grabbed surface lost motion: s1 motions=%d", h1.motions)
		}
		if h2.motions != 0 {
			t.Errorf("s2 received %d motions during s1 grab", h2.motions)
		}
	})

	t.Run("release ends the grab", func(t *testing.T) {
		s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 0})
		s.HandlePointerMotion(wl.PointerMotionEvent{SurfaceX: 1, SurfaceY: 1})
		if h2.motions != 1 {
			t.Errorf("s2 motions=%d after release, want routing to resume", h2.motions)
		}
	})

	t.Run("button release without press is safe", func(t *testing.T) {
		s.grabSurface = nil
		s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 0})
		if h1.releases+h2.releases != 2 {
			t.Errorf("stray release was not routed")
		}
	})
}

func TestPointerCapabilityLoss(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	h := &recordingHandler{}
	s.SetSurfaceInput(surf, h)

	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})
	s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 1})
	s.pointerLost()

	if h.leaves != 1 {
		t.Errorf("focused surface notified %d times on capability loss, want 1", h.leaves)
	}
	if s.pointerFocus != nil || s.grabSurface != nil {
		t.Errorf("routing state survived capability loss: focus=%v grab=%v", s.pointerFocus, s.grabSurface)
	}
	s.HandlePointerMotion(wl.PointerMotionEvent{SurfaceX: 5, SurfaceY: 5})
	s.HandlePointerButton(wl.PointerButtonEvent{Button: 0x110, State: 0})
	if h.motions != 0 || h.releases != 0 {
		t.Errorf("dead pointer still routed events")
	}
}

func TestSetSurfaceInputUnregisters(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	h := &recordingHandler{}
	s.SetSurfaceInput(surf, h)
	s.SetSurfaceInput(surf, nil)
	if len(s.surfaceHandlers) != 0 {
		t.Errorf("handler still registered after unregister")
	}
	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})
	if h.enters != 0 {
		t.Errorf("unregistered handler received %d enters", h.enters)
	}
}
