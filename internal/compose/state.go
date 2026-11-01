package compose

import "github.com/stubbedev/lys-lib/xkb"

// Result classifies what one Feed did to the sequence in flight.
type Result uint8

const (
	// None means the keysym took no part in a sequence: the caller
	// routes it through normal text handling. A sequence that was in
	// flight is cancelled by the time None returns.
	None Result = iota
	// Pending means the keysym joined a sequence that is still waiting
	// for more keysyms; nothing has been emitted.
	Pending
	// Done means a sequence completed and text is its committed string.
	Done
)

// State is one compose state machine: feed it the committed keysyms of
// key presses and it answers pending, done-with-text, or none. Not
// safe for concurrent use; it lives on the input thread. The nil State
// is the disabled machine: every method is a no-op.
type State struct {
	table *Table
	cs    *xkb.ComposeState
	// path holds the keysyms of the in-flight sequence, all of them
	// sequence prefixes — the first terminal keysym would have ended
	// the sequence with Done. Backspace unwinds one level by replaying
	// the surviving prefix through a fresh machine; compose sequences
	// are two to five keysyms long, so the replay is trivially cheap.
	path []xkb.Keysym
}

// Feed advances the machine by one committed keysym. A keysym that
// matches no continuation cancels the sequence and reports None, so
// the key itself still goes through normal text handling; the dead-key
// prefix is dropped (there is no replay of cancelled prefixes).
// Modifier keysyms are not fed and never cancel: they cannot appear in
// sequences, and tapping shift mid-sequence must not lose it.
func (s *State) Feed(sym xkb.Keysym) (Result, string) {
	if s == nil || xkb.KeysymIsModifier(sym) {
		return None, ""
	}
	if s.cs.Feed(sym) == xkb.ComposeFeedIgnored {
		return None, ""
	}
	switch s.cs.Status() {
	case xkb.ComposeComposing:
		s.path = append(s.path, sym)
		return Pending, ""
	case xkb.ComposeComposed:
		text := s.cs.UTF8()
		s.replay(nil)
		return Done, text
	default: // this keysym cancelled the sequence
		s.replay(nil)
		return None, ""
	}
}

// Backspace unwinds one level of an in-flight sequence and reports
// whether anything was pending. Outside a sequence it reports false —
// the caller must then let backspace through as the widget's delete
// action. Backspace itself never emits text either way.
func (s *State) Backspace() bool {
	if s == nil || len(s.path) == 0 {
		return false
	}
	s.replay(append([]xkb.Keysym(nil), s.path[:len(s.path)-1]...))
	return true
}

// Cancel drops any in-flight sequence.
func (s *State) Cancel() {
	if s == nil {
		return
	}
	s.replay(nil)
}

// Composing reports whether a sequence is in flight.
func (s *State) Composing() bool { return s != nil && len(s.path) > 0 }

// replay rebuilds the machine over path — always a prefix of an
// unfinished sequence, so the machine ends pending again; replay(nil)
// is a plain reset. The method takes a copy of the path it is handed
// before touching s.path.
func (s *State) replay(path []xkb.Keysym) {
	s.cs = s.table.xkb.NewState()
	s.path = append(s.path[:0], path...)
	for _, sym := range path {
		s.cs.Feed(sym)
	}
}
