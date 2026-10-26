package wlsession

import (
	"testing"

	text "github.com/stubbedev/lys-lib/third_party/wayland/unstable/text-input-v3"
)

func TestTextInputIsOptional(t *testing.T) {
	// The text-input manager must stay out of the required globals so
	// compositors without the protocol keep working: today's behavior
	// is the no-protocol path.
	for _, g := range requiredGlobals {
		if g == "zwp_text_input_manager_v3" {
			t.Errorf("%q is required; text input must stay feature-detected", g)
		}
	}
	have := make(map[string]bool)
	for _, g := range requiredGlobals {
		have[g] = true
	}
	if got := missingGlobals(have); len(got) != 0 {
		t.Errorf("missing globals with every required one bound = %v, want none", got)
	}
}

func TestIMEWithoutProtocol(t *testing.T) {
	// A session that never saw the manager global offers no text
	// input, and every text-input call is a harmless no-op.
	s := &Session{}
	if s.TextInputAvailable() {
		t.Error("TextInputAvailable = true without the manager global")
	}
	s.UpdateIME(IMEState{
		Enabled:     true,
		Surrounding: "abc",
		Cursor:      3,
		Anchor:      3,
		CursorRect:  Rect{X: 1, Y: 2, W: 3, H: 4},
		CauseOther:  true,
	})
	s.HandleZwpInputV3Done(text.ZwpInputV3DoneEvent{Serial: 1}) // no listener: dropped
}

func TestIMEDoneAssembly(t *testing.T) {
	s := &Session{}
	var got []IMEEvent
	s.OnIME = func(ev IMEEvent) { got = append(got, ev) }

	s.HandleZwpInputV3PreeditString(text.ZwpInputV3PreeditStringEvent{
		Text: "かんじ", CursorBegin: 3, CursorEnd: 6,
	})
	s.HandleZwpInputV3CommitString(text.ZwpInputV3CommitStringEvent{Text: "漢字"})
	s.HandleZwpInputV3DeleteSurroundingText(text.ZwpInputV3DeleteSurroundingTextEvent{
		BeforeLength: 2, AfterLength: 4,
	})
	s.HandleZwpInputV3Done(text.ZwpInputV3DoneEvent{Serial: 0})

	if len(got) != 1 {
		t.Fatalf("done events delivered = %d, want 1", len(got))
	}
	ev := got[0]
	if ev.Commit != "漢字" {
		t.Errorf("commit = %q, want 漢字", ev.Commit)
	}
	if ev.Preedit != "かんじ" || ev.PreeditCursorBegin != 3 || ev.PreeditCursorEnd != 6 {
		t.Errorf("preedit = %q %d..%d, want かんじ 3..6", ev.Preedit, ev.PreeditCursorBegin, ev.PreeditCursorEnd)
	}
	if ev.DeleteBefore != 2 || ev.DeleteAfter != 4 {
		t.Errorf("delete = %d..%d, want 2..4", ev.DeleteBefore, ev.DeleteAfter)
	}
	if !ev.Current {
		t.Error("serial 0 with no commits sent must count as current")
	}

	// A done event applies the pending batch exactly once; a second
	// done carries the (empty) initial state.
	s.HandleZwpInputV3Done(text.ZwpInputV3DoneEvent{Serial: 0})
	if len(got) != 2 || got[1].Commit != "" || got[1].Preedit != "" {
		t.Errorf("second batch = %+v, want empty commit and preedit", got[1])
	}
}

func TestIMEDoneStaleSerial(t *testing.T) {
	s := &Session{tiSerial: 1}
	var got IMEEvent
	s.OnIME = func(ev IMEEvent) { got = ev }

	s.HandleZwpInputV3CommitString(text.ZwpInputV3CommitStringEvent{Text: "stale"})
	s.HandleZwpInputV3Done(text.ZwpInputV3DoneEvent{Serial: 0})
	if got.Commit != "stale" {
		t.Fatalf("stale batch must still apply, got %+v", got)
	}
	if got.Current {
		t.Error("a done serial behind our commits must not count as current")
	}
}

func TestIMEFocusInvalidates(t *testing.T) {
	s := &Session{}
	focusEvents := 0
	s.OnIMEFocus = func() { focusEvents++ }
	s.HandleZwpInputV3PreeditString(text.ZwpInputV3PreeditStringEvent{Text: "x"})
	s.HandleZwpInputV3Enter(text.ZwpInputV3EnterEvent{})
	if focusEvents != 1 {
		t.Errorf("focus callbacks after enter = %d, want 1", focusEvents)
	}
	// Enter invalidates the pending state; a done now reports nothing.
	var got IMEEvent
	s.OnIME = func(ev IMEEvent) { got = ev }
	s.HandleZwpInputV3Done(text.ZwpInputV3DoneEvent{Serial: 0})
	if got.Commit != "" || got.Preedit != "" {
		t.Errorf("enter must reset pending state, got %+v", got)
	}
	s.HandleZwpInputV3Leave(text.ZwpInputV3LeaveEvent{})
	if focusEvents != 2 {
		t.Errorf("focus callbacks after leave = %d, want 2", focusEvents)
	}
}
