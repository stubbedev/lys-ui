package app

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// A key held while keyboard focus moves stops repeating: its release
// goes wherever focus went, so a repeat would keep pressing it into
// the next surface that takes the keyboard (an Enter that confirmed a
// layer surface confirming the next one). A key held on a surface that
// keeps the keyboard repeats on.
func TestHeldKeyStopsRepeatingWhenFocusMoves(t *testing.T) {
	due := func(a *Application) bool {
		a.rep.next = time.Now().Add(-time.Millisecond)
		_, _, ok := a.rep.tick()
		return ok
	}
	for name, move := range map[string]func(s *wlsession.Session, focused *wl.Surface){
		"leave": func(s *wlsession.Session, focused *wl.Surface) {
			s.HandleKeyboardLeave(wl.KeyboardLeaveEvent{Surface: focused})
		},
		"enter": func(s *wlsession.Session, _ *wl.Surface) {
			s.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: &wl.Surface{}})
		},
		"destroy": func(s *wlsession.Session, focused *wl.Surface) { s.SetSurfaceInput(focused, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			sess := &wlsession.Session{}
			a := NewApplication(sess)
			a.wireSession()
			focused := &wl.Surface{}
			sess.HandleKeyboardEnter(wl.KeyboardEnterEvent{Surface: focused})
			a.rep.press(28, 0)
			if !due(a) {
				t.Fatal("a held key on the focused surface does not repeat")
			}
			move(sess, focused)
			if due(a) {
				t.Fatal("the held key repeats after keyboard focus moved")
			}
		})
	}
}

func TestKeyRepeaterStop(t *testing.T) {
	r := newKeyRepeater(20, 400)
	r.press(30, 0)
	r.stop()
	r.next = time.Now().Add(-time.Millisecond)
	if _, _, ok := r.tick(); ok {
		t.Error("tick fired after stop")
	}
	if _, ok := r.nextDeadline(); ok {
		t.Error("a stopped repeater still schedules a wake")
	}
}
