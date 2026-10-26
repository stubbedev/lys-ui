package wlsession

import (
	"os"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// cursor-shape-v1: when the compositor offers it, cursors are its to
// draw - a shape name per pointer enter, no theme lookup, no buffer,
// animation compositor-side. The client theme path stays the fallback
// for compositors without it, a hidden cursor, and names the protocol
// does not carry.

// shapeAPI is the shape device's request side; tests substitute a
// recorder.
type shapeAPI interface {
	SetShape(serial, shape uint32) error
	Destroy() error
}

// cursorShapes maps the cursor vocabulary - the CSS names, the
// toolkit's resize_* spellings, and the classic xcursor names - onto
// the protocol's shapes.
var cursorShapes = map[string]uint32{
	"default": wlr.WpCursorShapeDeviceV1ShapeDefault, "left_ptr": wlr.WpCursorShapeDeviceV1ShapeDefault, "arrow": wlr.WpCursorShapeDeviceV1ShapeDefault,
	"context-menu": wlr.WpCursorShapeDeviceV1ShapeContextMenu,
	"help":         wlr.WpCursorShapeDeviceV1ShapeHelp, "question_arrow": wlr.WpCursorShapeDeviceV1ShapeHelp,
	"pointer": wlr.WpCursorShapeDeviceV1ShapePointer, "hand1": wlr.WpCursorShapeDeviceV1ShapePointer, "hand2": wlr.WpCursorShapeDeviceV1ShapePointer,
	"progress": wlr.WpCursorShapeDeviceV1ShapeProgress, "left_ptr_watch": wlr.WpCursorShapeDeviceV1ShapeProgress,
	"wait": wlr.WpCursorShapeDeviceV1ShapeWait, "watch": wlr.WpCursorShapeDeviceV1ShapeWait,
	"cell":      wlr.WpCursorShapeDeviceV1ShapeCell,
	"crosshair": wlr.WpCursorShapeDeviceV1ShapeCrosshair, "cross": wlr.WpCursorShapeDeviceV1ShapeCrosshair,
	"text": wlr.WpCursorShapeDeviceV1ShapeText, "xterm": wlr.WpCursorShapeDeviceV1ShapeText,
	"vertical-text": wlr.WpCursorShapeDeviceV1ShapeVerticalText,
	"alias":         wlr.WpCursorShapeDeviceV1ShapeAlias,
	"copy":          wlr.WpCursorShapeDeviceV1ShapeCopy,
	"move":          wlr.WpCursorShapeDeviceV1ShapeMove, "fleur": wlr.WpCursorShapeDeviceV1ShapeMove,
	"no-drop":     wlr.WpCursorShapeDeviceV1ShapeNoDrop,
	"not-allowed": wlr.WpCursorShapeDeviceV1ShapeNotAllowed, "crossed_circle": wlr.WpCursorShapeDeviceV1ShapeNotAllowed,
	"grab":     wlr.WpCursorShapeDeviceV1ShapeGrab,
	"grabbing": wlr.WpCursorShapeDeviceV1ShapeGrabbing,
	"e-resize": wlr.WpCursorShapeDeviceV1ShapeEResize, "resize_e": wlr.WpCursorShapeDeviceV1ShapeEResize, "right_side": wlr.WpCursorShapeDeviceV1ShapeEResize,
	"n-resize": wlr.WpCursorShapeDeviceV1ShapeNResize, "resize_n": wlr.WpCursorShapeDeviceV1ShapeNResize, "top_side": wlr.WpCursorShapeDeviceV1ShapeNResize,
	"ne-resize": wlr.WpCursorShapeDeviceV1ShapeNeResize, "resize_ne": wlr.WpCursorShapeDeviceV1ShapeNeResize, "top_right_corner": wlr.WpCursorShapeDeviceV1ShapeNeResize,
	"nw-resize": wlr.WpCursorShapeDeviceV1ShapeNwResize, "resize_nw": wlr.WpCursorShapeDeviceV1ShapeNwResize, "top_left_corner": wlr.WpCursorShapeDeviceV1ShapeNwResize,
	"s-resize": wlr.WpCursorShapeDeviceV1ShapeSResize, "resize_s": wlr.WpCursorShapeDeviceV1ShapeSResize, "bottom_side": wlr.WpCursorShapeDeviceV1ShapeSResize,
	"se-resize": wlr.WpCursorShapeDeviceV1ShapeSeResize, "resize_se": wlr.WpCursorShapeDeviceV1ShapeSeResize, "bottom_right_corner": wlr.WpCursorShapeDeviceV1ShapeSeResize,
	"sw-resize": wlr.WpCursorShapeDeviceV1ShapeSwResize, "resize_sw": wlr.WpCursorShapeDeviceV1ShapeSwResize, "bottom_left_corner": wlr.WpCursorShapeDeviceV1ShapeSwResize,
	"w-resize": wlr.WpCursorShapeDeviceV1ShapeWResize, "resize_w": wlr.WpCursorShapeDeviceV1ShapeWResize, "left_side": wlr.WpCursorShapeDeviceV1ShapeWResize,
	"ew-resize": wlr.WpCursorShapeDeviceV1ShapeEwResize, "resize_ew": wlr.WpCursorShapeDeviceV1ShapeEwResize, "sb_h_double_arrow": wlr.WpCursorShapeDeviceV1ShapeEwResize,
	"ns-resize": wlr.WpCursorShapeDeviceV1ShapeNsResize, "resize_ns": wlr.WpCursorShapeDeviceV1ShapeNsResize, "sb_v_double_arrow": wlr.WpCursorShapeDeviceV1ShapeNsResize,
	"nesw-resize": wlr.WpCursorShapeDeviceV1ShapeNeswResize, "resize_nesw": wlr.WpCursorShapeDeviceV1ShapeNeswResize, "bd_double_arrow": wlr.WpCursorShapeDeviceV1ShapeNeswResize,
	"nwse-resize": wlr.WpCursorShapeDeviceV1ShapeNwseResize, "resize_nwse": wlr.WpCursorShapeDeviceV1ShapeNwseResize, "fd_double_arrow": wlr.WpCursorShapeDeviceV1ShapeNwseResize,
	"col-resize": wlr.WpCursorShapeDeviceV1ShapeColResize,
	"row-resize": wlr.WpCursorShapeDeviceV1ShapeRowResize,
	"all-scroll": wlr.WpCursorShapeDeviceV1ShapeAllScroll,
	"zoom-in":    wlr.WpCursorShapeDeviceV1ShapeZoomIn,
	"zoom-out":   wlr.WpCursorShapeDeviceV1ShapeZoomOut,
}

// bindCursorShapeManager binds the shape manager global, unless
// LYS_UI_NO_CURSOR_SHAPE=1 keeps cursors on the client theme (debugging
// a compositor's cursors, and the headless test of the fallback).
func (s *Session) bindCursorShapeManager(ev wl.RegistryGlobalEvent) {
	if os.Getenv("LYS_UI_NO_CURSOR_SHAPE") == "1" {
		debug.Log("shell", "cursor-shape-v1 disabled by LYS_UI_NO_CURSOR_SHAPE")
		return
	}
	ctx := s.registry.Context()
	mgr := wlr.NewWpCursorShapeManagerV1(ctx)
	if !s.bindOptional(ev, 1, mgr) {
		return
	}
	s.shapeMgr = mgr
	debug.Log("shell", "cursor-shape-v1 bound")
	s.ensureCursorShape()
}

// ensureCursorShape gets the pointer's shape device once both the
// manager and the pointer exist.
func (s *Session) ensureCursorShape() {
	if s.shapeMgr == nil || s.pointer == nil || s.shapeDevice != nil {
		return
	}
	raw, ok := s.pointer.(interface{ wlPointer() *wl.Pointer })
	if !ok || raw.wlPointer() == nil {
		return
	}
	if dev, err := s.shapeMgr.GetPointer(raw.wlPointer()); err == nil {
		s.shapeDevice = dev
	}
}

// dropCursorShape destroys the shape device with its pointer.
func (s *Session) dropCursorShape() {
	if s.shapeDevice != nil {
		_ = s.shapeDevice.Destroy()
		s.shapeDevice = nil
	}
}

// applyShapeLocked shows desired through the shape device; false when
// the protocol cannot (no device, a hidden cursor, an unmapped name),
// leaving the client theme path to draw it.
func (s *Session) applyShapeLocked(desired string) bool {
	if s.shapeDevice == nil || desired == CursorHidden {
		return false
	}
	shape, ok := cursorShapes[effectiveCursor(desired)]
	if !ok {
		return false
	}
	if err := s.shapeDevice.SetShape(s.pointerEnterSerial, shape); err != nil {
		return false
	}
	debug.Log("input", "cursor shape %q -> %d", desired, shape)
	return true
}
