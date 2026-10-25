package wlsession

import (
	"testing"

	"github.com/neurlang/wayland/wl"
)

// Every move of the keyboard focus reaches OnKeyboardFocus: an enter, a
// leave, and the focused surface unregistered by its own client, which
// the compositor never answers with a leave. Unregistering a surface
// that does not hold the keyboard moves nothing.
func TestKeyboardFocusMovesReachTheHook(t *testing.T) {
	focused, other := &wl.Surface{}, &wl.Surface{}
	s := &Session{surfaceHandlers: map[*wl.Surface]SurfacePointerHandler{}}
	moves := 0
	s.OnKeyboardFocus = func() { moves++ }

	s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: focused})
	if moves != 1 || s.KeyboardFocus() != focused {
		t.Fatalf("enter: %d moves, focus %p, want 1 and the entered surface", moves, s.KeyboardFocus())
	}
	s.SetSurfaceInput(other, nil)
	if moves != 1 || s.KeyboardFocus() != focused {
		t.Fatalf("unregistering an unfocused surface: %d moves, focus %p", moves, s.KeyboardFocus())
	}
	s.SetSurfaceInput(focused, nil)
	if moves != 2 || s.KeyboardFocus() != nil {
		t.Fatalf("unregistering the focused surface: %d moves, focus %p, want 2 and none", moves, s.KeyboardFocus())
	}
	s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: other})
	s.HandleKeyboardLeave(wl.KeyboardLeaveEvent{Surface: other})
	if moves != 4 || s.KeyboardFocus() != nil {
		t.Fatalf("enter then leave: %d moves, focus %p, want 4 and none", moves, s.KeyboardFocus())
	}
}
