// Text input: optional zwp_text_input_v3 support. The manager global is
// feature-detected — compositors without it keep the session running
// with every text-input call a no-op.
package wlsession

import (
	text "github.com/neurlang/wayland/unstable/text-input-v3"
	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// maxIMESurrounding is the wayland message size cap the protocol puts
// on surrounding text; larger buffers are simply not reported.
const maxIMESurrounding = 4000

// IMEState is one atomic text-input state update. Callers push the
// focused editable widget's state; UpdateIME applies it with a commit
// request.
type IMEState struct {
	// Enabled requests input-method composition because an editable
	// widget holds focus; false disables it.
	Enabled bool
	// Multiline hints the input method at multi-line content.
	Multiline bool
	// Surrounding is the text around the caret (preedit excluded)
	// with Cursor and Anchor as byte offsets into it. Text beyond
	// maxIMESurrounding bytes is not sent, which per protocol means
	// the client is unaware of its surroundings.
	Surrounding    string
	Cursor, Anchor int
	// CursorRect is the caret rectangle in surface-local coordinates,
	// where the input method may place a candidate window.
	CursorRect Rect
	// CauseOther marks a change made without the input method
	// (typing, clipboard, caret motion), asking it to drop whatever
	// it is composing.
	CauseOther bool
}

// Rect is a rectangle in surface-local coordinates.
type Rect struct {
	X, Y, W, H int32
}

// IMEEvent is one applied text-input batch (a done event), carrying the
// double-buffered changes in the order the protocol applies them:
// delete surrounding text, insert the commit string, show the new
// preedit.
type IMEEvent struct {
	// Commit is the text to insert at the caret (may be empty).
	Commit string
	// Preedit is the composing text to show at the caret (may be
	// empty, ending composition).
	Preedit string
	// PreeditCursorBegin and PreeditCursorEnd are byte offsets into
	// Preedit; both -1 hide the composing caret.
	PreeditCursorBegin, PreeditCursorEnd int
	// DeleteBefore and DeleteAfter are byte counts to remove before
	// and after the caret.
	DeleteBefore, DeleteAfter uint32
	// Current reports that the done serial matches the commits we
	// sent, so the pushed state is still known and may be re-pushed.
	// A stale batch still applies — committed text is never dropped —
	// but skips the resync.
	Current bool
}

// tiPending accumulates the double-buffered text-input events until the
// done event applies them.
type tiPending struct {
	commit  string
	preedit string
	begin   int32
	end     int32
	before  uint32
	after   uint32
}

// bindTextInputManager binds the optional text-input manager global.
func (s *Session) bindTextInputManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := text.NewZwpInputManagerV3(ctx)
	if !s.bindOptional(ev, 1, mgr) {
		return
	}
	s.textInputMgr = mgr
	s.ensureTextInput()
}

// ensureTextInput creates the seat's text input once both the manager
// and the seat are bound, whichever arrives first.
func (s *Session) ensureTextInput() {
	if s.textInput != nil || s.textInputMgr == nil || s.seat == nil {
		return
	}
	ti, err := s.textInputMgr.GetInput(s.seat)
	if err != nil {
		return
	}
	s.textInput = ti
	ti.AddEnterHandler(s)
	ti.AddLeaveHandler(s)
	ti.AddPreeditStringHandler(s)
	ti.AddCommitStringHandler(s)
	ti.AddDeleteSurroundingTextHandler(s)
	ti.AddDoneHandler(s)
	debug.Log("input", "text-input-v3 bound")
}

// TextInputAvailable reports whether the compositor advertised
// zwp_text_input_manager_v3 and the seat's text input is bound. When
// false, every text-input call on the session is a no-op and the
// keyboard path runs exactly as without IME support.
func (s *Session) TextInputAvailable() bool { return s.textInput != nil }

// UpdateIME pushes one atomic state update: enable or disable plus the
// surrounding text and caret rectangle, applied atomically by a commit
// request. No-op without the protocol.
func (s *Session) UpdateIME(st IMEState) {
	ti := s.textInput
	if ti == nil {
		return
	}
	if st.Enabled {
		_ = ti.Enable()
		if st.CauseOther {
			_ = ti.SetChangeCause(text.ZwpInputV3ChangeCauseOther)
		}
		hint := uint32(text.ZwpInputV3ContentHintNone)
		if st.Multiline {
			hint = text.ZwpInputV3ContentHintMultiline
		}
		_ = ti.SetContentType(hint, text.ZwpInputV3ContentPurposeNormal)
		if len(st.Surrounding) <= maxIMESurrounding {
			_ = ti.SetSurroundingText(st.Surrounding, intconv.Int32(st.Cursor), intconv.Int32(st.Anchor))
		}
		_ = ti.SetCursorRectangle(st.CursorRect.X, st.CursorRect.Y, st.CursorRect.W, st.CursorRect.H)
	} else {
		_ = ti.Disable()
	}
	if err := ti.Commit(); err == nil {
		s.tiSerial++
	}
}

// HandleZwpInputV3Enter implements text.ZwpInputV3EnterHandler: the
// surface gained text-input focus. Enter (like leave) invalidates all
// committed state, so the host must re-push it for the new surface.
func (s *Session) HandleZwpInputV3Enter(text.ZwpInputV3EnterEvent) {
	s.tiPending = tiPending{}
	if s.OnIMEFocus != nil {
		s.OnIMEFocus()
	}
}

// HandleZwpInputV3Leave implements text.ZwpInputV3LeaveHandler.
func (s *Session) HandleZwpInputV3Leave(text.ZwpInputV3LeaveEvent) {
	s.tiPending = tiPending{}
	if s.OnIMEFocus != nil {
		s.OnIMEFocus()
	}
}

// HandleZwpInputV3PreeditString implements
// text.ZwpInputV3PreeditStringHandler: buffer the composing text until
// done applies it.
func (s *Session) HandleZwpInputV3PreeditString(ev text.ZwpInputV3PreeditStringEvent) {
	s.tiPending.preedit = ev.Text
	s.tiPending.begin, s.tiPending.end = ev.CursorBegin, ev.CursorEnd
}

// HandleZwpInputV3CommitString implements
// text.ZwpInputV3CommitStringHandler.
func (s *Session) HandleZwpInputV3CommitString(ev text.ZwpInputV3CommitStringEvent) {
	s.tiPending.commit = ev.Text
}

// HandleZwpInputV3DeleteSurroundingText implements
// text.ZwpInputV3DeleteSurroundingTextHandler.
func (s *Session) HandleZwpInputV3DeleteSurroundingText(ev text.ZwpInputV3DeleteSurroundingTextEvent) {
	s.tiPending.before, s.tiPending.after = ev.BeforeLength, ev.AfterLength
}

// HandleZwpInputV3Done implements text.ZwpInputV3DoneHandler: the
// pending batch is now current; hand it to the host in application
// order and report whether the serial matched our commits.
func (s *Session) HandleZwpInputV3Done(ev text.ZwpInputV3DoneEvent) {
	p := s.tiPending
	s.tiPending = tiPending{}
	if s.OnIME == nil {
		return
	}
	s.OnIME(IMEEvent{
		Commit:             p.commit,
		Preedit:            p.preedit,
		PreeditCursorBegin: int(p.begin),
		PreeditCursorEnd:   int(p.end),
		DeleteBefore:       p.before,
		DeleteAfter:        p.after,
		Current:            ev.Serial == s.tiSerial,
	})
}
