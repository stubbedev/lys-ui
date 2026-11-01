package wlsession

import (
	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/internal/compose"
)

// Compose wiring. The session owns the compose table (resolved once
// from $XCOMPOSEFILE, ~/.XCompose, then the system locale file) and
// the single seat-wide state machine: Wayland keyboard focus is
// seat-wide, so a sequence in flight belongs to the seat, not to a
// surface. With no compose file anywhere the state is nil, and every
// method here degrades to the disabled machine — dead-key layouts
// then behave exactly as before this wiring existed.

// loadCompose resolves and parses the active compose file, opening the
// seat's compose state. Errors (no file, bad file) leave compose
// disabled rather than failing the connection.
func (s *Session) loadCompose() {
	s.comp = compose.Load().Start()
}

// ComposePending reports whether a compose sequence is in flight.
func (s *Session) ComposePending() bool { return s.comp.Composing() }

// FeedCompose advances the compose machine by the keysym of a key
// press, answering pending, done-with-text, or none. None means the
// keysym takes no part in compose and normal text handling should
// proceed; a sequence that was in flight has been cancelled by then.
func (s *Session) FeedCompose(sym xkb.Keysym) (compose.Result, string) {
	return s.comp.Feed(sym)
}

// ComposeBackspace unwinds one level of an in-flight sequence,
// reporting whether anything was pending. The caller consumes the
// backspace either way when a sequence was in flight — the widget must
// not delete text for it — and passes it through when none was.
func (s *Session) ComposeBackspace() bool { return s.comp.Backspace() }
