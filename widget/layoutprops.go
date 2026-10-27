package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

type expandSetting uint8

const (
	expandInherit expandSetting = iota
	expandOn
	expandOff
)

type layoutProps struct {
	hexpand, vexpand expandSetting
	halign, valign   Align
	margin           render.Insets
	// request is the size request, a floor on each axis; 0 leaves the
	// axis to the widget.
	request Size
	// maximum caps each axis's border box; 0 leaves the axis unbounded.
	maximum          Size
	expandValid      bool
	expandH, expandV bool
}

// SetHExpand says whether the widget wants extra horizontal space,
// GTK's hexpand: a row Box gives it a share of its leftover width.
// Unset, a widget wants it when a visible child does.
func (n *node) SetHExpand(on bool) { n.setExpand(&n.layout.hexpand, on) }

// SetVExpand is SetHExpand for vertical space.
func (n *node) SetVExpand(on bool) { n.setExpand(&n.layout.vexpand, on) }

func (n *node) setExpand(s *expandSetting, on bool) {
	want := expandOff
	if on {
		want = expandOn
	}
	if *s == want {
		return
	}
	*s = want
	n.InvalidateLayout()
}

// SetHAlign places the widget horizontally inside the space its parent
// gives it, GTK's halign: AlignFill (the default) stretches it, the
// others keep its natural width at the start, center or end.
func (n *node) SetHAlign(a Align) {
	if n.layout.halign != a {
		n.layout.halign = a
		n.InvalidateLayout()
	}
}

// SetVAlign is SetHAlign for the vertical axis.
func (n *node) SetVAlign(a Align) {
	if n.layout.valign != a {
		n.layout.valign = a
		n.InvalidateLayout()
	}
}

// SetMargin sets the space kept clear around the widget inside its
// parent's allocation, GTK's margin-*. It adds to a stylesheet margin.
func (n *node) SetMargin(m render.Insets) {
	if n.layout.margin != m {
		n.layout.margin = m
		n.InvalidateLayout()
	}
}

// SetSizeRequest sets the widget's minimum size, GTK's
// set_size_request: a container never measures it smaller than w x h
// (its border box: padding and border inside, margin outside). A zero
// axis leaves that axis to the widget; negative values are zero.
func (n *node) SetSizeRequest(w, h int) {
	req := Size{W: max(w, 0), H: max(h, 0)}
	if n.layout.request != req {
		n.layout.request = req
		n.InvalidateLayout()
	}
}

// SizeRequest returns the size request (zero axes unset).
func (n *node) SizeRequest() Size { return n.layout.request }

// SetMaximumSize caps the widget's size, the typed max-width and
// max-height: a container never measures it larger than w x h (its
// border box) and holds a wider slot to it by its alignment, fill
// reading as start. What the content paints past the cap is clipped.
// A zero axis is unbounded; negative values are zero. The size
// request wins where the two cross, so equal values pin an exact size.
func (n *node) SetMaximumSize(w, h int) {
	limit := Size{W: max(w, 0), H: max(h, 0)}
	if n.layout.maximum != limit {
		n.layout.maximum = limit
		n.InvalidateLayout()
	}
}

// MaximumSize returns the size cap (zero axes unbounded).
func (n *node) MaximumSize() Size { return n.layout.maximum }

// capped is the maximum with an unbounded axis indefinite and the size
// request raising a cap below it.
func (l *layoutProps) capped() Size {
	axis := func(limit, req int) int {
		if limit == 0 {
			return indefinite
		}
		return max(limit, req)
	}
	return Size{W: axis(l.maximum.W, l.request.W), H: axis(l.maximum.H, l.request.H)}
}

// HAlign returns the horizontal placement.
func (n *node) HAlign() Align { return n.layout.halign }

// VAlign returns the vertical placement.
func (n *node) VAlign() Align { return n.layout.valign }

// Margin returns the programmatic margin.
func (n *node) Margin() render.Insets { return n.layout.margin }

func (n *node) layoutNode() *node { return n }

type layoutCarrier interface{ layoutNode() *node }

func layoutOf(w Widget) *layoutProps {
	if c, ok := w.(layoutCarrier); ok {
		return &c.layoutNode().layout
	}
	return nil
}

// WantsExpand reports whether w wants extra space on axis: its own
// setting when set, else whether any visible child wants it, GTK's
// computed expand. The result is cached until the subtree's layout
// changes.
func WantsExpand(w Widget, axis Axis) bool {
	l := layoutOf(w)
	if l == nil {
		return false
	}
	if !l.expandValid {
		l.expandH = computeExpand(w, l.hexpand, Row)
		l.expandV = computeExpand(w, l.vexpand, Column)
		l.expandValid = true
	}
	if axis == Row {
		return l.expandH
	}
	return l.expandV
}

func computeExpand(w Widget, s expandSetting, axis Axis) bool {
	switch s {
	case expandOn:
		return true
	case expandOff:
		return false
	}
	p, ok := w.(interface{ Children() []Widget })
	if !ok {
		return false
	}
	for _, c := range p.Children() {
		if IsVisible(c) && WantsExpand(c, axis) {
			return true
		}
	}
	return false
}

func (l *layoutProps) dropExpand() { l.expandValid = false }

func layoutMarginOf(w Widget) render.Insets {
	if l := layoutOf(w); l != nil {
		return l.margin
	}
	return render.Insets{}
}

// measureWithMargin measures a container's child as a flow participant
// along axis at forSize, its margin-box extent across: its margin box,
// CSS sizing included, and nothing for an absolutely or fixed
// positioned child, which leaves the flow (csspos.go). A plain measure
// knows no containing block, so percentage sizes read as auto.
func measureWithMargin(w Widget, axis Axis, forSize int) Sizes {
	if outOfFlow(w) {
		return Sizes{}
	}
	return measureOuter(w, axis, forSize, indefiniteSize)
}

// MeasureAsChild measures w as a container measures its child: its
// size request, programmatic margin and CSS sizes apply. A host that
// owns a tree's top (a popover around its content) measures through it
// so the top's own size request counts, as GTK honors a popover's.
func MeasureAsChild(w Widget, axis Axis, forSize int) Sizes {
	return measureWithMargin(w, axis, forSize)
}

// SizeWithinAsChild is SizeWithin for w measured as a child: a host
// sizing a tree's top by MeasureAsChild.
func SizeWithinAsChild(w Widget, avail Size) Size {
	return sizeWithin(func(axis Axis, forSize int) Sizes { return MeasureAsChild(w, axis, forSize) }, avail)
}

// ArrangeAsChild arranges w in r as a container arranges its child:
// inside its margin, held to its CSS bounds. MeasureAsChild's pair.
func ArrangeAsChild(w Widget, r render.Rect) { arrangeChild(w, r) }

// measureMargin is the widget's own Measure inside its programmatic
// margin, held to its size request and maximum, with no CSS sizing
// around it. forSize is the margin-box extent across.
func measureMargin(w Widget, axis Axis, forSize int) Sizes {
	l := layoutOf(w)
	if l == nil || (l.margin.Zero() && l.request == (Size{}) && l.maximum == (Size{})) {
		return w.Measure(axis, forSize)
	}
	along, across := insetsAlong(l.margin, axis), insetsAlong(l.margin, otherAxis(axis))
	hi := l.capped()
	if forSize != Unknown {
		forSize = min(max(0, forSize-across), hi.Along(otherAxis(axis)))
	}
	s := w.Measure(axis, forSize)
	if limit := hi.Along(axis); finite(limit) {
		s = Sizes{Min: min(s.Min, limit), Nat: min(s.Nat, limit)}
	}
	return s.atLeast(l.request.Along(axis)).grow(along)
}

// parentBlock is the containing block w's percentages resolve against
// at arrange: its parent's content box, indefinite without a parent.
func parentBlock(w Widget) Size {
	p := parentOf(w)
	if p == nil {
		return indefiniteSize
	}
	box := contentBoxOf(p)
	return Size{W: box.W, H: box.H}
}

// insetsAlong is m's total along axis.
func insetsAlong(m render.Insets, axis Axis) int {
	if axis == Row {
		return m.Left + m.Right
	}
	return m.Top + m.Bottom
}

// arrangeChild arranges a container's child inside r with its margin
// and alignment: the margin shrinks r, and a non-fill alignment keeps
// the child's natural size, never more than r, at the start, center or
// end of what is left. CSS applies around it: an out-of-flow child is
// placed against its containing block from r's corner (its static
// position), a sized one holds r to its sizes against the parent's
// content box, and a relative or sticky one shifts by its offsets.
func arrangeChild(w Widget, r render.Rect) {
	n := nodeOf(w)
	var v *style.Values
	if n != nil {
		v = n.style(w)
		if v.Layout.Position.OutOfFlow() && v.Has(style.PropPosition) {
			placeOutOfFlow(w, v, Point{X: r.X, Y: r.Y}, ownContainingBlock(w, v))
			return
		}
	}
	l := layoutOf(w)
	h, va := AlignFill, AlignFill
	if l != nil {
		h, va = l.halign, l.valign
	}
	if v != nil && cssSized(v) {
		r = holdSlot(w, v, r, parentBlock(w), h, va)
	}
	if l == nil || (l.margin.Zero() && l.maximum == (Size{}) && l.halign == AlignFill && l.valign == AlignFill) {
		arrangeAt(w, v, r)
		return
	}
	r = l.margin.Shrink(r)
	if l.maximum != (Size{}) {
		r = fitSlot(r, Size{}, l.capped(), h, va)
	}
	if h != AlignFill || va != AlignFill {
		cb, mw, mh := parentBlock(w), insetsAlong(l.margin, Row), insetsAlong(l.margin, Column)
		nat := Size{W: r.W, H: r.H}
		if h != AlignFill {
			nat.W = min(measureOuter(w, Row, Unknown, cb).Nat-mw, r.W)
		}
		if va != AlignFill {
			nat.H = min(measureOuter(w, Column, nat.W+mw, cb).Nat-mh, r.H)
		}
		if h == AlignBaseline {
			h = AlignStart
		}
		if va == AlignBaseline {
			va = AlignStart
		}
		r = alignRect(r, nat, h, va)
	}
	arrangeAt(w, v, r)
}
