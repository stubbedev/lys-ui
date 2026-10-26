// Drag and drop glue: the press+motion gesture that turns a widget's
// declared content into a wl_data_device start_drag, the routing of
// data-device events into the widget tree, and the drag icon snapshot.
// The wire-level state machine lives in internal/dragdrop.
package app

import (
	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/dragdrop"
	"github.com/stubbedev/lys-ui/internal/intconv"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/transfer"
	"github.com/stubbedev/lys-ui/widget"
)

// dragThreshold is the pointer travel, in logical pixels, before a
// press becomes a drag — a click never is. Close to GTK's threshold.
const dragThreshold = 8.0

// maxDragIcon bounds the drag icon snapshot; a source widget larger
// than this is cropped in the icon.
const maxDragIcon = 512

// dragController is the slice of the drag-and-drop controller the
// surface input drives. *dragdrop.Controller satisfies it; tests
// substitute a recording fake.
type dragController interface {
	StartDrag(cfg dragdrop.StartConfig) error
	Dragging() bool
	ReadPayload(mime string) ([]byte, error)
}

// startDrag promotes the active press into a data-device drag when it
// crossed the threshold over a widget.DragSource. Called from the
// pointer-motion path while the implicit grab is live.
func (in *surfaceInput) startDrag() {
	if in.dragStarted || in.dnd == nil || in.dnd.Dragging() {
		return
	}
	src, ok := in.router.Pressed().(widget.DragSource)
	if !ok {
		return
	}
	dx, dy := in.x-in.pressX, in.y-in.pressY
	if dx*dx+dy*dy < dragThreshold*dragThreshold {
		return
	}
	content := src.DragContent()
	if content == nil {
		return
	}
	icon := in.router.Pressed()
	if ic, ok := src.(widget.DragIconer); ok {
		icon = ic.DragIcon()
	}
	err := in.dnd.StartDrag(dragdrop.StartConfig{
		Origin:     in.surf,
		Icon:       renderDragIcon(in.sess, icon, in.deviceScale()),
		GrabSerial: in.pressSerial,
		Content:    *content,
	})
	if err != nil {
		debug.Log("input", "dnd start: %v", err)
		return
	}
	in.dragStarted = true
	// The gesture became a drag: the release must not click, and the
	// pressed widget must not keep following the pointer as a DragMover.
	in.router.CancelPress()
	in.request()
}

// renderDragIcon paints w standalone into a fresh shm-backed surface
// the compositor moves with the pointer during the drag. frac120 is the
// source window's current 120-based device scale; the icon surface
// scales to match. Any failure leaves the icon nil: start_drag takes a
// nil icon and shows a fallback graphic.
func renderDragIcon(sess *wlsession.Session, w widget.Widget, frac120 uint32) *wl.Surface {
	if sess == nil || sess.Compositor() == nil || sess.Shm() == nil || w == nil {
		return nil
	}
	size := w.Measure(widget.Constraints{Max: widget.Size{W: maxDragIcon, H: maxDragIcon}})
	if size.W <= 0 || size.H <= 0 {
		return nil
	}
	b, err := buffer.NewFile(sess.Shm(),
		scale.DeviceSize(size.W, frac120), scale.DeviceSize(size.H, frac120),
		scale.IntegerScale(frac120))
	if err != nil {
		debug.Log("frame", "dnd icon buffer: %v", err)
		return nil
	}
	// The icon paints a live widget of the tree: it is laid out at the
	// icon's origin for the paint and put back where the window has it.
	if bs, ok := w.(widget.Boundser); ok {
		defer w.Arrange(bs.Bounds())
	}
	w.Arrange(render.Rect{X: 0, Y: 0, W: size.W, H: size.H})
	cv := render.NewScaled(b.Data, b.Stride, b.Width, b.Height, int(frac120), scale.Denom)
	w.Paint(cv)
	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		buffer.Cancel(b)
		debug.Log("frame", "dnd icon surface: %v", err)
		return nil
	}
	sc := scale.New(sess, surf, nil)
	_ = sc.Apply(frac120, size.W, size.H)
	attach := func(err error, what string) bool {
		if err != nil {
			debug.Log("frame", "dnd icon %s: %v", what, err)
			_ = surf.Destroy()
			buffer.Cancel(b)
			return false
		}
		return true
	}
	if !attach(surf.Attach(b.WL, 0, 0), "attach") ||
		!attach(surf.DamageBuffer(0, 0, intconv.Int32(b.Width), intconv.Int32(b.Height)), "damage") ||
		!attach(surf.Commit(), "commit") {
		return nil
	}
	// One-shot: the commit was the buffer's only user. Its arena slot
	// returns when the compositor releases the buffer (the icon surface
	// itself is destroyed at drag end).
	return surf
}

// DragEnter implements dragdrop.Target: route through the widget tree
// and report the accepted mime. A modal dialog over this window owns
// the pointer, so its drags are rejected too.
func (in *surfaceInput) DragEnter(mimes []string, x, y float64) string {
	if in.dropInput() {
		return ""
	}
	mime := in.router.DragEnter(mimes, in.dropPoint(x, y))
	in.request()
	return mime
}

// DragMotion implements dragdrop.Target.
func (in *surfaceInput) DragMotion(x, y float64) string {
	if in.dropInput() {
		return ""
	}
	mime := in.router.DragHover(in.dropPoint(x, y))
	in.request()
	return mime
}

// DragLeave implements dragdrop.Target.
func (in *surfaceInput) DragLeave() {
	in.router.DragLeave()
	in.request()
}

// Drop implements dragdrop.Target: fetch the payload for the accepted
// mime and hand it to the widget. Same-process drops resolve inside
// ReadPayload without touching the wire; cross-process ones go through
// the offer pipe. Every ReadPayload error rejects the drop — past
// xfer.MaxPayload or stalled past the transfer deadline, the payload
// is refused (and traced), never handed to the widget truncated.
func (in *surfaceInput) Drop(x, y float64) {
	if in.dropInput() {
		return
	}
	mime := in.router.DragMime()
	if mime == "" {
		return
	}
	data, err := in.dnd.ReadPayload(mime)
	if err != nil {
		debug.Log("input", "dnd drop: %v", err)
		in.reportTransfer(err)
		return
	}
	in.router.Drop(mime, data, in.dropPoint(x, y))
	in.request()
}

// DragAction implements dragdrop.ActionTarget: the drop target's pick
// (widget.DropActionChooser).
func (in *surfaceInput) DragAction(offered transfer.Action) transfer.Action {
	return in.router.DragAction(offered)
}

// cancelDragKey cancels this process's running drag on Escape, GTK's
// drag-cancel. Compositors that keep the keyboard during a drag (most
// cancel on Escape themselves) deliver nothing, which leaves the drag
// to them; one that delivers keys gets the source-side cancel.
func (a *Application) cancelDragKey(keycode uint32) bool {
	return a.dnd != nil && a.dnd.Dragging() && a.sess.KeySym(keycode) == xkb.KeyEscape && a.dnd.CancelDrag()
}

// CancelDrag cancels the drag this application started, if one is
// running; its OnDone hears transfer.ActionNone. Reports whether a
// drag was running.
func (a *Application) CancelDrag() bool { return a.dnd != nil && a.dnd.CancelDrag() }

// SetTransferErrorHandler installs fn to hear clipboard and drag
// transfers that failed - a paste (ctrl+v or middle-click) or a drop
// whose peer sent more than xfer.MaxPayload, stalled past the
// transfer deadline, or broke the pipe - so the app can show a toast.
// An empty clipboard is not an error. fn runs on the loop goroutine.
func (a *Application) SetTransferErrorHandler(fn func(err error)) { a.onTransferError = fn }

// reportTransfer hands a failed transfer to the installed handler.
func (a *Application) reportTransfer(err error) {
	if err = transferFailure(err); err != nil {
		debug.Log("input", "transfer failed: %v", err)
		if a.onTransferError != nil {
			a.onTransferError(err)
		}
	}
}

// deviceScale reports the host window's 120-based device scale for
// one-shot surfaces; 1x when unset (wire-free tests).
func (in *surfaceInput) deviceScale() uint32 {
	if in.frac == nil {
		return scale.Denom
	}
	if f := in.frac(); f != 0 {
		return f
	}
	return scale.Denom
}

// dropPoint maps surface coordinates into router (root) coordinates,
// the same mapping the pointer path applies: both are logical pixels,
// so it truncates to the tree's integer grid.
func (in *surfaceInput) dropPoint(x, y float64) widget.Point {
	return widget.Point{X: int(x), Y: int(y)}
}
