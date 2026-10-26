// Interactive resize: edge hit-testing against the window's own
// bounds, the resize_* cursor shapes for edge hover, and the
// xdg_toplevel.resize grab on edge presses. Layer surfaces have no
// edges; server-decorated windows leave the edges to the compositor.
package app

import (
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/wlcursor"
	"github.com/stubbedev/lys-lib/third_party/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/window"
)

// resizer is the interactive-resize slice of a toplevel host; layer
// surfaces have no edges and never implement it.
// A real toplevel must be one: a method renamed out of the interface
// would silently drop both grabs (it did, from #91 to #115).
var _ resizer = (*window.Window)(nil)

type resizer interface {
	// Resize starts an interactive compositor-driven resize.
	Resize(seat *wl.Seat, serial uint32, edges uint32) error
	// Move starts an interactive compositor-driven move - the CSD
	// header grab (#91).
	Move(seat *wl.Seat, serial uint32) error
}

// sizeLimiter exposes a host's min/max size constraints; layer
// surfaces are compositor-sized and never implement it.
type sizeLimiter interface {
	SizeLimits() (minW, minH, maxW, maxH int)
}

// serverDecorated hosts report compositor-owned decorations: the
// compositor draws borders and handles resize there, so the client
// keeps its own edge handles passive.
type serverDecorated interface {
	ServerDecorated() bool
}

// stateReporter exposes a host's compositor-confirmed toplevel state;
// layer surfaces are compositor-sized and never implement it.
type stateReporter interface {
	State() window.State
}

// resizeBorder is how many logical pixels along each edge act as a
// resize handle, in line with the usual toolkit hit areas.
const resizeBorder = 6

// resizeEdge maps a pointer position (logical surface pixels) to the
// xdg_toplevel.resize_edge bits under it: within resizeBorder pixels
// of an edge, corners combining both axes' bits. An axis too small for
// two disjoint handles matches no edge on that axis - the combined
// left|right value is not a protocol edge.
func resizeEdge(w, h int, x, y float64) uint32 {
	var edges uint32
	if w > 2*resizeBorder {
		switch {
		case x < float64(resizeBorder):
			edges |= xdg.ToplevelResizeEdgeLeft
		case x >= float64(w-resizeBorder):
			edges |= xdg.ToplevelResizeEdgeRight
		}
	}
	if h > 2*resizeBorder {
		switch {
		case y < float64(resizeBorder):
			edges |= xdg.ToplevelResizeEdgeTop
		case y >= float64(h-resizeBorder):
			edges |= xdg.ToplevelResizeEdgeBottom
		}
	}
	return edges
}

// resizeCursor maps resize_edge bits to the xcursor shape for the
// direction; a no-edge or invalid combination (left|right and friends)
// yields "" and the widget cursors apply.
func resizeCursor(edges uint32) string {
	switch edges {
	case xdg.ToplevelResizeEdgeTop:
		return wlcursor.TopSide
	case xdg.ToplevelResizeEdgeBottom:
		return wlcursor.BottomSide
	case xdg.ToplevelResizeEdgeLeft:
		return wlcursor.LeftSide
	case xdg.ToplevelResizeEdgeRight:
		return wlcursor.RightSide
	case xdg.ToplevelResizeEdgeTopLeft:
		return wlcursor.TopLeftCorner
	case xdg.ToplevelResizeEdgeTopRight:
		return wlcursor.TopRightCorner
	case xdg.ToplevelResizeEdgeBottomLeft:
		return wlcursor.BottomLeftCorner
	case xdg.ToplevelResizeEdgeBottomRight:
		return wlcursor.BottomRightCorner
	}
	return ""
}

// resizeEdgeAt is surfaceInput's edge probe: the resize_edge bits under
// the pointer, 0 when the point is interior. A server-decorated window
// reports none (the compositor owns the handles), and so does a
// compositor-confirmed maximized or fullscreen window: a window filling
// the screen edge-to-edge has no edges to grab, whether the compositor
// kept its decorations or dropped them in fullscreen.
func (w *hostWindow) resizeEdgeAt(x, y float64) uint32 {
	if w.decorated != nil && w.decorated() {
		return 0
	}
	if w.state != nil {
		if st := w.state(); st.Maximized || st.Fullscreen {
			return 0
		}
	}
	bw, bh := w.layoutSize()
	return resizeEdge(bw, bh, x, y)
}
