// Input method bridge: the session's zwp_text_input_v3 batches feed
// the focused editable widget, and widget edits push surrounding text
// and the caret rectangle back. Everything is a no-op on compositors
// without the protocol.
package app

import (
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// imeWire is the session-facing slice the controller drives; a stub
// stands in for tests.
type imeWire interface {
	TextInputAvailable() bool
	UpdateIME(wlsession.IMEState)
}

// imeSnapshot mirrors IMEState for change detection: state is only
// pushed when it moved since the last push.
type imeSnapshot struct {
	enabled   bool
	multiline bool
	text      string
	cursor    int
	anchor    int
	rect      render.Rect
}

// imeController bridges one session's text input and the focused
// widget of a window's router.
type imeController struct {
	sess imeWire
	sent imeSnapshot
	have bool
}

func newIMEController(sess imeWire) *imeController {
	return &imeController{sess: sess}
}

// sync pushes the focused widget's text-input state when it changed
// since the last push: enable on an editable widget (Entry, TextArea)
// with its surrounding text and caret rectangle, disable on anything
// else. causeOther marks changes made without the input method, asking
// it to drop its composing state. Without the protocol this is a
// no-op, so the keyboard path runs exactly as without IME support.
//
// The widget tree is laid out in logical surface pixels, so the caret
// rect is already surface-local: set_cursor_rectangle takes surface
// coordinates and the rect passes through with no scaling or rounding
// at any device scale.
func (c *imeController) sync(r *widget.Router, causeOther bool) {
	if !c.sess.TextInputAvailable() {
		return
	}
	var next imeSnapshot
	if tr, ok := r.Focused().(widget.IMETracker); ok {
		next.enabled = true
		next.multiline = tr.IMEMultiline()
		next.text, next.cursor, next.anchor = tr.IMESurrounding()
		next.rect = tr.IMECursorRect()
	}
	if c.have && c.sent == next {
		return
	}
	c.sent, c.have = next, true
	st := wlsession.IMEState{
		Enabled:     next.enabled,
		Multiline:   next.multiline,
		Surrounding: next.text,
		Cursor:      next.cursor,
		Anchor:      next.anchor,
		CauseOther:  causeOther,
		CursorRect: wlsession.Rect{
			X: intconv.Clamp[int32](next.rect.X),
			Y: intconv.Clamp[int32](next.rect.Y),
			W: intconv.Clamp[int32](next.rect.W),
			H: intconv.Clamp[int32](next.rect.H),
		},
	}
	c.sess.UpdateIME(st)
}

// deliver applies one compositor batch into the focused widget, in the
// done event's order: delete surrounding bytes, insert the commit
// string, show the next preedit. A batch whose serial still matches
// our commits also re-pushes the state, which the changes just
// invalidated.
func (c *imeController) deliver(r *widget.Router, ev wlsession.IMEEvent) {
	cl, ok := r.Focused().(widget.IMEClient)
	if !ok {
		return
	}
	if ev.DeleteBefore > 0 || ev.DeleteAfter > 0 {
		cl.IMEDelete(int(ev.DeleteBefore), int(ev.DeleteAfter))
	}
	if ev.Commit != "" {
		cl.IMECommit(ev.Commit)
	}
	cl.IMEPreedit(ev.Preedit, ev.PreeditCursorBegin, ev.PreeditCursorEnd)
	if ev.Current {
		c.sync(r, false)
	}
}

// reset forgets the pushed state: text-input focus moved surfaces and
// the compositor invalidated everything, so it must be re-sent.
func (c *imeController) reset() { c.have = false }
