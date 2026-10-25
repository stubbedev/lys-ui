package widget

import (
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
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

// measureWithMargin measures a container's child as a flow participant:
// its margin box, CSS sizing included, and nothing for an absolutely
// or fixed positioned child, which leaves the flow (csspos.go).
func measureWithMargin(w Widget, con Constraints) Size {
	if outOfFlow(w) {
		return Size{}
	}
	return measureOuter(w, con)
}

// MeasureAsChild measures w as a container measures its child: its
// size request, programmatic margin and CSS sizes apply. A host that
// owns a tree's top (a popover around its content) measures through it
// so the top's own size request counts, as GTK honors a popover's.
func MeasureAsChild(w Widget, con Constraints) Size { return measureWithMargin(w, con) }

// ArrangeAsChild arranges w in r as a container arranges its child:
// inside its margin, held to its CSS bounds. MeasureAsChild's pair.
func ArrangeAsChild(w Widget, r render.Rect) { arrangeChild(w, r) }

// measureMargin is the widget's own Measure inside its programmatic
// margin, with no CSS sizing around it.
func measureMargin(w Widget, con Constraints) Size {
	var m render.Insets
	var req Size
	hi := Size{W: indefinite, H: indefinite}
	if l := layoutOf(w); l != nil {
		m, req, hi = l.margin, l.request, l.capped()
	}
	if m.Zero() && req == (Size{}) && hi == (Size{W: indefinite, H: indefinite}) {
		return w.Measure(con)
	}
	mw, mh := m.Left+m.Right, m.Top+m.Bottom
	maxW, maxH := min(max(0, con.Max.W-mw), hi.W), min(max(0, con.Max.H-mh), hi.H)
	inner := Constraints{
		Min: Size{W: min(max(0, con.Min.W-mw, min(req.W, con.Max.W-mw)), maxW), H: min(max(0, con.Min.H-mh, min(req.H, con.Max.H-mh)), maxH)},
		Max: Size{W: maxW, H: maxH},
	}
	s := w.Measure(inner)
	s = Size{W: min(max(s.W, min(req.W, inner.Max.W)), maxW), H: min(max(s.H, min(req.H, inner.Max.H)), maxH)}
	return Size{W: min(s.W+mw, con.Max.W), H: min(s.H+mh, con.Max.H)}
}

// arrangeChild arranges a container's child inside r with its margin
// and alignment: the margin shrinks r, and a non-fill alignment keeps
// the child's natural size at the start, center or end of what is left.
// CSS applies around it: an out-of-flow child is placed against its
// containing block from r's corner (its static position), a sized one
// holds r to its definite size and min/max bounds, and a relative or
// sticky one shifts by its offsets.
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
		r = holdSlot(n, r, h, va)
	}
	if l == nil || (l.margin.Zero() && l.maximum == (Size{}) && l.halign == AlignFill && l.valign == AlignFill) {
		// A size request only floors the measure: a fill child takes
		// its slot, which the container sized from that measure.
		arrangeAt(w, v, r)
		return
	}
	r = l.margin.Shrink(r)
	if l.maximum != (Size{}) {
		r = fitSlot(r, Size{}, l.capped(), h, va)
	}
	if h != AlignFill || va != AlignFill {
		nat := w.Measure(Constraints{Max: Size{W: r.W, H: r.H}})
		nat = Size{W: max(nat.W, min(l.request.W, r.W)), H: max(nat.H, min(l.request.H, r.H))}
		if v != nil && cssSized(v) {
			// The CSS size the measure resolved, not the content's.
			nat = Size{W: max(min(nat.W, n.cssMax.W), n.cssMin.W), H: max(min(nat.H, n.cssMax.H), n.cssMin.H)}
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
