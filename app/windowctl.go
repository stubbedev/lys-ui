package app

import (
	"github.com/neurlang/wayland/xdg"
	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// Window requests beyond state: interactive move and resize begun from
// code, fullscreen on a chosen output, attention, and transient
// parents.

// ResizeEdge is the edge or corner an interactive resize drags.
type ResizeEdge uint32

// Resize edges, the xdg_toplevel resize_edge values.
const (
	EdgeTop         ResizeEdge = xdg.ToplevelResizeEdgeTop
	EdgeBottom      ResizeEdge = xdg.ToplevelResizeEdgeBottom
	EdgeLeft        ResizeEdge = xdg.ToplevelResizeEdgeLeft
	EdgeTopLeft     ResizeEdge = xdg.ToplevelResizeEdgeTopLeft
	EdgeBottomLeft  ResizeEdge = xdg.ToplevelResizeEdgeBottomLeft
	EdgeRight       ResizeEdge = xdg.ToplevelResizeEdgeRight
	EdgeTopRight    ResizeEdge = xdg.ToplevelResizeEdgeTopRight
	EdgeBottomRight ResizeEdge = xdg.ToplevelResizeEdgeBottomRight
)

// BeginMove starts the compositor's interactive move from the press
// that is under way (gtk_window_begin_move_drag): call it from a press
// handler - a custom title area, a drag handle - and the window
// follows the pointer until release. The protocol anchors the grab to
// the serial of the latest input event, which the session supplies;
// without one the compositor ignores the request.
func (w *Window) BeginMove() {
	if hw := w.app.hostWindowOf(w); hw != nil && hw.startMove != nil {
		hw.startMove(w.app.sess.InputSerial())
	}
}

// BeginResize starts an interactive resize from edge, like BeginMove
// (gtk_window_begin_resize_drag) - for app-drawn resize handles; the
// window's own edges already resize.
func (w *Window) BeginResize(edge ResizeEdge) {
	if hw := w.app.hostWindowOf(w); hw != nil && hw.startResize != nil {
		hw.startResize(uint32(edge), w.app.sess.InputSerial())
	}
}

// FullscreenOn asks for fullscreen on output (nil: the compositor's
// pick, as Fullscreen); a presentation on the projector, a video on
// the second monitor. See Maximize for the state-request contract.
func (w *Window) FullscreenOn(output *wlsession.Output) {
	if w.win != nil {
		_ = w.win.Fullscreen(outputWire(output))
	}
}

// RequestAttention asks the compositor to draw the user to the window
// (GTK's urgency hint) - background work finished, a message arrived.
// Wayland has no urgency flag: the request is xdg-activation without a
// user interaction behind it, which compositors answer by marking the
// window urgent or demanding attention instead of raising it (focus
// stealing prevention). A no-op without xdg-activation or once the
// window closed.
func (w *Window) RequestAttention() {
	if w.win == nil || w.closed {
		return
	}
	surf := w.win.HostSurface()
	sess := w.app.sess
	w.app.Invoke(func() {
		sess.RequestActivationToken(surf, 0, func(token string, err error) {
			if err != nil {
				debug.Log("shell", "attention request: %v", err)
				return
			}
			if !w.closed && !w.win.Closed() {
				sess.Activate(surf, token)
			}
		})
	})
}

// SetTransientFor keeps the window above parent and grouped with it -
// a toolbox, an inspector, a find bar window (xdg_toplevel.set_parent,
// the relation dialogs get by construction). nil releases it.
func (w *Window) SetTransientFor(parent *Window) {
	if w.win == nil {
		return
	}
	if parent == nil || parent.win == nil {
		w.win.SetParent(nil)
		return
	}
	w.win.SetParent(parent.win)
}
