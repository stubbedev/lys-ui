// The CSS interaction surfaces (docs/css.md): cursor, pointer-events,
// user-select, caret-shape, accent-color, appearance, and touch-action
// — the computed values the input path and the painters read.
package widget

import (
	"strconv"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// cursorFilePrefix marks a cursor request that asks for a file cursor
// (CSS cursor: url()): `file:<path>#<x>,<y>`, the hotspot CSS named.
const cursorFilePrefix = "file:"

// CSSCursorOf returns the cursor the stylesheet asks for over w: an
// xcursor shape name the session resolves ("pointer", "ns-resize",
// the hidden "none"), or a file request built from cursor: url() and
// its hotspot. Empty when no cursor declaration reaches w or it is
// auto — the caller falls back to the widget's own shape. The value
// inherits, so one declaration covers a subtree.
func CSSCursorOf(w Widget) string {
	n := nodeOf(w)
	if n == nil {
		return ""
	}
	return cssCursorRequest(n.style(w).Cursor)
}

// cssCursorRequest maps the computed cursor onto the request the app
// hands the session: keywords pass through by their CSS spelling, a
// url() becomes a file request carrying its hotspot and the list's
// fallback keyword (`file:<path>#<x>,<y>[,<fallback>]`), auto asks
// for nothing.
func cssCursorRequest(spec style.CursorSpec) string {
	if spec.URL == "" {
		return spec.Name
	}
	req := cursorFilePrefix + spec.URL + "#" + strconv.Itoa(spec.HotX) + "," + strconv.Itoa(spec.HotY)
	if spec.Name != "" {
		req += "," + spec.Name
	}
	return req
}

// pointerEventsNone reports whether w's computed pointer-events is
// none: it is transparent to hit testing and a press falls through to
// whatever is behind it. The property inherits, so a container's none
// covers its subtree unless a descendant sets auto again.
func pointerEventsNone(w Widget) bool {
	n := nodeOf(w)
	if n == nil {
		return false
	}
	v := n.style(w)
	return v.Has(style.PropPointerEvents) && v.PointerEvents == style.PointerNone
}

// clippedOut reports whether a clip-path on w or an ancestor hides
// the point p: CSS Masking's clipped-out region takes no pointer
// events, so a press there falls through like a press beside the
// widget.
func clippedOut(w Widget, p Point) bool {
	for ; w != nil; w = parentOf(w) {
		n := nodeOf(w)
		if n == nil {
			continue
		}
		v := n.style(w)
		if !v.Has(style.PropClipPath) || v.Clip.Empty() {
			continue
		}
		if path, ok := clipShapePath(&v.Clip, n.bounds, radiusOr(v, 0)); ok && !path.Contains(float64(p.X)+0.5, float64(p.Y)+0.5) {
			return true
		}
	}
	return false
}

// userSelectOf returns w's used user-select (CSS UI 4 §7.1): its own
// value unless that is auto, and for auto contain on an editable text
// field, else the parent's used value when that is none or all, else
// text.
func userSelectOf(w Widget) style.UserSelect {
	n := nodeOf(w)
	if n == nil {
		return style.SelectText
	}
	if v := n.style(w); v.Has(style.PropUserSelect) && v.UserSelect != style.SelectAuto {
		return v.UserSelect
	}
	if editableText(w) {
		return style.SelectContain
	}
	p := parentOf(w)
	if p == nil {
		return style.SelectText
	}
	if up := userSelectOf(p); up == style.SelectNone || up == style.SelectAll {
		return up
	}
	return style.SelectText
}

// accentOf resolves accent-color over the fallback color: the
// sheet's color when it declares one, else def (the theme accent or
// its disabled derivation).
func accentOf(v *style.Values, def render.Color) render.Color {
	if v.Has(style.PropAccentColor) && !v.AccentAuto {
		return v.AccentColor
	}
	return def
}

// touchActionOf intersects the touch-action declarations from w up
// the chain into the pan axes a touch scroll may still move.
func touchActionOf(w Widget) style.TouchAction {
	act := style.TouchAuto
	for ; w != nil; w = parentOf(w) {
		n := nodeOf(w)
		if n == nil {
			continue
		}
		if v := n.style(w); v.Has(style.PropTouchAction) {
			act &= v.TouchAction
		}
	}
	return act
}

// gateTouchAxis masks scroll deltas by the touch-action that reaches
// w: none stops both axes, pan-x and pan-y keep theirs. The wheel
// path is not gated — touch-action speaks to touch manipulation
// only.
func gateTouchAxis(w Widget, dx, dy *float64) {
	switch touchActionOf(w) {
	case style.TouchNone:
		*dx, *dy = 0, 0
	case style.TouchPanX:
		*dy = 0
	case style.TouchPanY:
		*dx = 0
	}
}

// caretWidth is the block and underscore caret's fallback width when
// the caret sits past the text's end.
const caretWidth = 8

// caretShapeRect is the caret's rect in its CSS shape: the 2px bar,
// the block as wide as the cluster past the caret, the underscore
// along the text area's bottom. adv is that cluster's width, 0 past
// the end.
func caretShapeRect(x, y, h, adv int, shape style.CaretShape) render.Rect {
	switch shape {
	case style.CaretBlock:
		return render.Rect{X: x, Y: y, W: max(adv, 2), H: h}
	case style.CaretUnderscore:
		return render.Rect{X: x, Y: y + h - 2, W: max(adv, caretWidth), H: 2}
	default:
		return render.Rect{X: x, Y: y, W: 2, H: h}
	}
}

// editableText reports whether w is a text field the user can edit
// now, user-select auto's contain case.
func editableText(w Widget) bool {
	e, ok := w.(interface{ editable() bool })
	return ok && e.editable()
}
