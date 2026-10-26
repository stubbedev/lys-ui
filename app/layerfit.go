package app

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/layersurface"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// unboundedFit bounds a content-sized axis when no output reports a
// logical size yet.
const unboundedFit = 1 << 14

// fitAxes says which axes of a layer surface follow its content.
type fitAxes struct{ w, h bool }

func (f fitAxes) any() bool { return f.w || f.h }

// contentAxes is gtk4-layer-shell's sizing rule: an automatic (zero)
// axis anchored on both of its edges stretches across the output, and
// one that is not takes the window content's natural size.
func contentAxes(width, height uint32, a layersurface.Anchor) fitAxes {
	return fitAxes{
		w: width == 0 && a&(layersurface.AnchorLeft|layersurface.AnchorRight) != layersurface.AnchorLeft|layersurface.AnchorRight,
		h: height == 0 && a&(layersurface.AnchorTop|layersurface.AnchorBottom) != layersurface.AnchorTop|layersurface.AnchorBottom,
	}
}

// fitBounds is the space a content-sized axis may take: the surface's
// output's logical size; for a surface the compositor places, the
// largest output it could pick; unboundedFit while none reports a size.
func fitBounds(out *wlsession.Output, all []*wlsession.Output) widget.Size {
	if out != nil {
		all = []*wlsession.Output{out}
	}
	bound := widget.Size{}
	for _, o := range all {
		bound.W, bound.H = max(bound.W, int(o.LogicalW)), max(bound.H, int(o.LogicalH))
	}
	if bound.W <= 0 || bound.H <= 0 {
		return widget.Size{W: unboundedFit, H: unboundedFit}
	}
	return bound
}

// naturalSize measures root within limit, arranging and re-measuring
// while the tree settles: a subtree first measured without its
// ancestors resolves its scoped stylesheet only once arranged.
func naturalSize(root widget.Widget, limit widget.Size) widget.Size {
	con := widget.Constraints{Max: limit}
	size := root.Measure(con)
	for range maxLayoutPasses {
		root.Arrange(render.Rect{W: size.W, H: size.H})
		if !widget.LayoutPending(root) {
			break
		}
		size = root.Measure(con)
	}
	return size
}

// measureOnly is a settled tree's natural size: its styles resolved in
// an earlier frame, so one cached Measure answers without disturbing
// the arrangement the frame is about to make.
func measureOnly(root widget.Widget, limit widget.Size) widget.Size {
	return root.Measure(widget.Constraints{Max: limit})
}

// fittedSize resolves the requested size against the content: each
// content-sized axis takes root's natural extent (at least one pixel,
// the protocol's floor for a non-automatic axis), measured with the
// other axis at its current size when that one is fixed.
func fittedSize(root widget.Widget, fit fitAxes, width, height uint32, cur [2]int, out *wlsession.Output, all []*wlsession.Output, measure func(widget.Widget, widget.Size) widget.Size) (uint32, uint32) {
	limit := fitBounds(out, all)
	if !fit.w && cur[0] > 0 {
		limit.W = cur[0]
	}
	if !fit.h && cur[1] > 0 {
		limit.H = cur[1]
	}
	size := measure(root, limit)
	if fit.w {
		width = uint32(max(size.W, 1))
	}
	if fit.h {
		height = uint32(max(size.H, 1))
	}
	return width, height
}

// refit runs before each frame of a layer surface with content-sized
// axes: when the content's natural size moved, the surface asks for
// the new size, which lands with this frame's commit; the compositor's
// configure then resizes the buffers. It reports whether it asked, so
// the frame commits even with nothing damaged.
func (l *LayerWindow) refit(root widget.Widget) bool {
	if !l.fit.any() || l.ls.Closed() {
		return false
	}
	sc := l.ls.Config()
	w, h := l.ls.Size()
	nw, nh := fittedSize(root, l.fit, sc.Width, sc.Height, [2]int{w, h}, l.cfg.Output, l.app.sess.Outputs(), measureOnly)
	if nw == sc.Width && nh == sc.Height {
		return false
	}
	return l.ls.SetSize(nw, nh) == nil
}

// defaultSize resolves a toplevel's initial size: each zero axis takes
// root's natural extent within the first output (GTK's default size),
// at least one pixel, so a compositor that leaves the size to the
// client (sway's first configure is 0x0) still gets a buffer to map.
func defaultSize(root widget.Widget, width, height uint32, outputs []*wlsession.Output) (uint32, uint32) {
	if width != 0 && height != 0 || root == nil {
		return width, height
	}
	var first []*wlsession.Output
	if len(outputs) > 0 {
		first = outputs[:1]
	}
	widget.SetStyleRoot(root)
	return fittedSize(root, fitAxes{w: width == 0, h: height == 0}, width, height, [2]int{int(width), int(height)}, nil, first, naturalSize)
}
