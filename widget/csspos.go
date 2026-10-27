package widget

// CSS positioning (docs/css.md): relative and sticky shift a widget
// from where its container's layout put it, never reflowing the
// siblings; absolute and fixed take it out of the flow — the container
// measures nothing for it — and place it against its containing block,
// the padding box of the nearest positioned ancestor (absolute) or the
// tree root's box, the window (fixed, and absolute with no positioned
// ancestor). z-index and position order the children a Box paints and
// hit-tests, one stacking context per container.

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// positioned reports a declared, non-static position.
func positioned(v *style.Values) bool {
	return v.Has(style.PropPosition) && v.Layout.Position != style.PositionStatic
}

// outOfFlow reports whether w is absolutely or fixed positioned.
func outOfFlow(w Widget) bool {
	n := nodeOf(w)
	if n == nil {
		return false
	}
	v := n.style(w)
	return v.Has(style.PropPosition) && v.Layout.Position.OutOfFlow()
}

// insetOf resolves one top/right/bottom/left inset against basis; ok is
// false for auto.
func insetOf(v *style.Values, side int, basis int) (int, bool) {
	s := v.Layout.Inset[side]
	if s.Kind != style.SizingLength {
		return 0, false
	}
	return s.Len.Resolve(basis), true
}

// arrangeAt arranges w in r, shifted by its relative or sticky offsets.
// v is w's computed style, nil for a widget without a node.
func arrangeAt(w Widget, v *style.Values, r render.Rect) {
	if v != nil && v.Has(style.PropPosition) {
		switch v.Layout.Position {
		case style.PositionRelative:
			r = relativeRect(w, v, r)
		case style.PositionSticky:
			r = stickyRect(w, v, r)
		}
	}
	w.Arrange(r)
}

// contentBoxOf is the content box of w's last arranged bounds: the
// containing block its in-flow children's percentages resolve against.
func contentBoxOf(w Widget) render.Rect {
	n := nodeOf(w)
	if n == nil {
		return render.Rect{}
	}
	b := cssBoxOf(w, n.style(w))
	return b.padding.Shrink(b.border.Shrink(n.bounds))
}

// relativeRect shifts r by the relative offsets, left beating right and
// top beating bottom, percentages against the parent's content box.
func relativeRect(w Widget, v *style.Values, r render.Rect) render.Rect {
	var cb render.Rect
	if p := parentOf(w); p != nil {
		cb = contentBoxOf(p)
	}
	if dx, ok := insetOf(v, 3, cb.W); ok {
		r.X += dx
	} else if dx, ok := insetOf(v, 1, cb.W); ok {
		r.X -= dx
	}
	if dy, ok := insetOf(v, 0, cb.H); ok {
		r.Y += dy
	} else if dy, ok := insetOf(v, 2, cb.H); ok {
		r.Y -= dy
	}
	return r
}

// stickyRect applies the sticky constraint: inside the nearest Scroll's
// visible viewport inset by the declared sides, the widget keeps its
// flow position until scrolling would carry it past an inset, then
// travels with the viewport — never out of its parent's content box.
// Without a Scroll above there is nothing to stick to, and it stays put.
func stickyRect(w Widget, v *style.Values, r render.Rect) render.Rect {
	var port render.Rect
	for p := parentOf(w); p != nil; p = parentOf(p) {
		if s, ok := p.(*Scroll); ok {
			port = render.Rect{X: s.view.X, Y: s.view.Y, W: s.viewW, H: s.viewH}
			break
		}
	}
	if port.Empty() {
		return r
	}
	var cb render.Rect
	if p := parentOf(w); p != nil {
		cb = contentBoxOf(p)
	}
	stick := func(pos, size, portPos, portSize, cbPos, cbSize, startSide, endSide int) int {
		if start, ok := insetOf(v, startSide, portSize); ok && pos < portPos+start {
			return min(portPos+start, max(pos, cbPos+cbSize-size))
		}
		if end, ok := insetOf(v, endSide, portSize); ok && pos+size > portPos+portSize-end {
			return max(portPos+portSize-end-size, min(pos, cbPos))
		}
		return pos
	}
	r.Y = stick(r.Y, r.H, port.Y, port.H, cb.Y, cb.H, 0, 2)
	r.X = stick(r.X, r.W, port.X, port.W, cb.X, cb.W, 3, 1)
	return r
}

// containingBlock is the rect an out-of-flow widget resolves against:
// the padding box of its nearest positioned ancestor for absolute, the
// tree root's box for fixed and for an absolute with no positioned
// ancestor.
func containingBlock(w Widget, fixed bool) render.Rect {
	top := w
	for p := parentOf(w); p != nil; p = parentOf(p) {
		top = p
		if fixed {
			continue
		}
		pn := nodeOf(p)
		if pn == nil {
			continue
		}
		if pv := pn.style(p); positioned(pv) {
			return cssBoxOf(p, pv).border.Shrink(pn.bounds)
		}
	}
	if n := nodeOf(top); n != nil && top != w {
		return n.bounds
	}
	return render.Rect{}
}

// ownContainingBlock is containingBlock for w's computed position.
func ownContainingBlock(w Widget, v *style.Values) render.Rect {
	return containingBlock(w, v.Layout.Position == style.PositionFixed)
}

// placeOutOfFlow arranges an absolutely or fixed positioned widget
// against its containing block cb (ownContainingBlock's, or the grid
// area a grid container resolves for it). Each axis resolves CSS's way: a
// definite size wins; with both insets set an auto size stretches
// between them, and auto margins then center a definite one; otherwise
// the widget takes its shrink-to-fit size. A set inset places the box
// from its side, left and top first; with neither set the box keeps its
// static position, the corner the container's flow left for it.
func placeOutOfFlow(w Widget, v *style.Values, static Point, cb render.Rect) {
	ws := resolveAxis(w, v, Row, cb.W)
	hs := resolveAxis(w, v, Column, cb.H)
	left, hasL := insetOf(v, 3, cb.W)
	right, hasR := insetOf(v, 1, cb.W)
	top, hasT := insetOf(v, 0, cb.H)
	bottom, hasB := insetOf(v, 2, cb.H)
	availW := max(0, cb.W-left-right)
	availH := max(0, cb.H-top-bottom)
	basis := Size{W: cb.W, H: cb.H}
	width := ws.clamp(ws.size)
	if !ws.definite {
		if hasL && hasR {
			width = ws.clamp(availW)
		} else {
			width = measureOuter(w, Row, Unknown, basis).Within(availW)
		}
	}
	height := hs.clamp(hs.size)
	if !hs.definite {
		if hasT && hasB {
			height = hs.clamp(availH)
		} else {
			height = measureOuter(w, Column, width, basis).Within(availH)
		}
	}
	auto := v.MarginAuto
	place := func(cbPos, cbSize, size, start, end int, hasStart, hasEnd, autoStart, autoEnd bool, staticPos int) int {
		switch {
		case hasStart && hasEnd && (autoStart || autoEnd):
			free := cbSize - start - end - size
			if autoStart && autoEnd {
				return cbPos + start + free/2
			}
			if autoStart {
				return cbPos + start + free
			}
			return cbPos + start
		case hasStart:
			return cbPos + start
		case hasEnd:
			return cbPos + cbSize - end - size
		}
		return staticPos
	}
	r := render.Rect{
		X: place(cb.X, cb.W, width, left, right, hasL, hasR, auto[3], auto[1], static.X),
		Y: place(cb.Y, cb.H, height, top, bottom, hasT, hasB, auto[0], auto[2], static.Y),
		W: width,
		H: height,
	}
	w.Arrange(layoutMarginOf(w).Shrink(r))
}

// stackKey orders one child in its container's paint: the CSS painting
// layers — negative z-index, in-flow blocks, floats, inline-level
// boxes, positioned boxes at z-index 0, positive z-index — then the
// z-index within a layer and order-modified document order. z-index
// applies to positioned boxes and to flex and grid items. The zero key is an
// in-flow block at z-index 0: tree order.
type stackKey struct {
	layer, z, order int
}

// stackKeyOf computes w's key inside a container laying out in mode.
func stackKeyOf(w Widget, mode layoutMode) stackKey {
	n := nodeOf(w)
	if n == nil {
		return stackKey{}
	}
	v := n.style(w)
	pos := positioned(v)
	var k stackKey
	items := mode == modeFlex || mode == modeGrid
	if items {
		k.order = v.Layout.Order
	}
	if pos || items {
		k.z = v.Layout.ZIndex
	}
	switch {
	case k.z < 0:
		k.layer = -1
	case pos && k.z == 0:
		k.layer = 3
	case k.z > 0:
		k.layer = 4
	case mode == modeFlow && v.Has(style.PropFloat) && v.Layout.Float != style.FloatNone:
		k.layer = 1
	case mode == modeFlow && v.Has(style.PropDisplay) && v.Layout.Display.Inline():
		k.layer = 2
	}
	return k
}

// compareStack orders two keys.
func compareStack(a, b stackKey) int {
	switch {
	case a.layer != b.layer:
		return a.layer - b.layer
	case a.z != b.z:
		return a.z - b.z
	}
	return a.order - b.order
}
