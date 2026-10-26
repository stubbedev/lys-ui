package wlsession

import (
	"errors"
	"testing"

	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// relRecorder records relative motion.
type relRecorder struct {
	recordingHandler
	moves [][4]float64
}

func (h *relRecorder) HandleRelativeMotion(dx, dy, rdx, rdy float64) {
	h.moves = append(h.moves, [4]float64{dx, dy, rdx, rdy})
}

// Relative motion goes to the pointer's surface, accelerated and raw.
func TestRelativeMotionRouting(t *testing.T) {
	s := newRoutingSession()
	s1, s2 := &wl.Surface{}, &wl.Surface{}
	h1, h2 := &relRecorder{}, &relRecorder{}
	s.SetSurfaceInput(s1, h1)
	s.SetSurfaceInput(s2, h2)
	s.pointerFocus = s2
	s.HandleZwpRelativePointerV1Motion(wlr.ZwpRelativePointerV1MotionEvent{Dx: 3, Dy: -2, DxUnaccel: 1.5, DyUnaccel: -1})
	if len(h1.moves) != 0 || len(h2.moves) != 1 || h2.moves[0] != [4]float64{3, -2, 1.5, -1} {
		t.Errorf("moves: s1 %v s2 %v", h1.moves, h2.moves)
	}
}

// Without the manager (or a pointer) a constraint is unavailable; a
// constraint reports the compositor's activations, releases cleanly,
// and dies with its surface's input.
func TestPointerConstraintLifecycle(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	s.SetSurfaceInput(surf, &relRecorder{})
	if _, err := s.LockPointer(surf, false); !errors.Is(err, ErrPointerConstraintsUnavailable) {
		t.Errorf("no manager: %v", err)
	}
	c := &PointerConstraint{s: s, surface: surf}
	s.constraints = map[*wl.Surface]*PointerConstraint{surf: c}
	var seen []bool
	c.OnActive = func(on bool) { seen = append(seen, on) }
	c.HandleZwpLockedPointerV1Locked(wlr.ZwpLockedPointerV1LockedEvent{})
	if !c.Active() {
		t.Fatal("locked did not activate")
	}
	if _, err := s.ConfinePointer(surf, nil, true); !errors.Is(err, ErrAlreadyConstrained) {
		t.Errorf("second constraint: %v", err)
	}
	s.SetSurfaceInput(surf, nil)
	if c.Active() || s.constraints[surf] != nil || len(seen) != 2 || seen[1] {
		t.Errorf("surface teardown: active %v seen %v", c.Active(), seen)
	}
}
