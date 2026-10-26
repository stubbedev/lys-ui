package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/compose"
)

// sessionWithCompose builds a session whose seat uses the shared
// compose fixture, the way Connect does when a compose file exists.
func sessionWithCompose(t *testing.T) *Session {
	t.Helper()
	tb, err := compose.NewTableFile("../compose/testdata/XCompose")
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return &Session{comp: tb.Start()}
}

func TestSessionComposeDisabledByDefault(t *testing.T) {
	// A zero-value session — no Connect, no compose file — must be
	// inert: every compose call is a no-op, nothing panics.
	s := &Session{}
	if s.ComposePending() || s.ComposeBackspace() {
		t.Fatal("compose should be disabled without a table")
	}
	if res, text := s.FeedCompose('e'); res != compose.None || text != "" {
		t.Fatalf("FeedCompose = %v %q, want none", res, text)
	}
}

func TestSessionComposeFeedAndCommit(t *testing.T) {
	s := sessionWithCompose(t)
	if res, _ := s.FeedCompose(xkb.KeyDeadAcute); res != compose.Pending {
		t.Fatalf("dead_acute = %v, want pending", res)
	}
	if !s.ComposePending() {
		t.Fatal("sequence should be in flight")
	}
	if res, text := s.FeedCompose('e'); res != compose.Done || text != "é" {
		t.Fatalf("dead_acute+e = %v %q, want done é", res, text)
	}
	if s.ComposePending() {
		t.Fatal("sequence should be committed")
	}
}

func TestSessionComposeBackspaceUnwindsOneLevel(t *testing.T) {
	s := sessionWithCompose(t)
	for _, sym := range []xkb.Keysym{'s', 't'} {
		if res, _ := s.FeedCompose(sym); res != compose.Pending {
			t.Fatalf("feed %v = %v, want pending", sym, res)
		}
	}
	if !s.ComposeBackspace() {
		t.Fatal("pending backspace should unwind")
	}
	if !s.ComposePending() {
		t.Fatal("the s prefix should remain pending")
	}
	if res, text := feedComposeSym(s, 't', 'r'); res != compose.Done || text != "★" {
		t.Fatalf("replayed s+t+r = %v %q, want done ★", res, text)
	}
}

// feedComposeSym feeds syms in order, returning the last result.
func feedComposeSym(s *Session, syms ...xkb.Keysym) (compose.Result, string) {
	var res compose.Result
	var text string
	for _, sym := range syms {
		res, text = s.FeedCompose(sym)
	}
	return res, text
}

func TestSessionComposeCancelsOnKeyboardLeave(t *testing.T) {
	s := sessionWithCompose(t)
	if res, _ := s.FeedCompose(xkb.KeyDeadAcute); res != compose.Pending {
		t.Fatalf("dead_acute = %v, want pending", res)
	}
	s.HandleKeyboardLeave(wl.KeyboardLeaveEvent{})
	if s.ComposePending() {
		t.Fatal("keyboard leave should cancel the sequence")
	}
}
