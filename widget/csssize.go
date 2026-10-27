package widget

// The CSS sizing properties on every widget (docs/css.md): width,
// height, their min-* and max-* bounds, box-sizing and aspect-ratio.
// They apply at the container boundary — measureOuter around the
// widget's own Measure, arrangeChild around its Arrange — so a widget
// gets them whatever its type, and a widget whose cascade declares
// none takes the plain path untouched. Percentages resolve against the
// containing block: the content box a CSS layout pass hands its items,
// and the parent's content box at Arrange. A measure that has none
// treats a percentage as auto, CSS's rule for intrinsic contributions.

import (
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// indefinite is the extent of an axis a layout pass does not know. A
// percentage of it resolves as auto, and no layout pass stretches into
// it.
const indefinite = math.MaxInt

// finite reports whether an available extent is definite.
func finite(n int) bool { return n < indefinite }

// withExtent is s with its extent along a set to v.
func withExtent(s Size, a Axis, v int) Size {
	if a == Row {
		s.W = v
	} else {
		s.H = v
	}
	return s
}

// otherAxis is the axis across a.
func otherAxis(a Axis) Axis {
	if a == Row {
		return Column
	}
	return Row
}

// cssBoxer is a widget whose CSS box carries widget-supplied insets (a
// Box's or a Button's programmatic padding); every other widget's box
// is the stylesheet's alone.
type cssBoxer interface {
	box(v *style.Values) cssInsets
}

// cssBoxOf resolves w's CSS box.
func cssBoxOf(w Widget, v *style.Values) cssInsets {
	if b, ok := w.(cssBoxer); ok {
		return b.box(v)
	}
	return boxOf(v, render.Insets{})
}

// sizedProps are the longhands that send a widget through the CSS
// sizing path. A plain pixel min-width or min-height alone stays the
// widget's own floor, GTK's reading, which every widget applies itself.
var sizedProps = style.SetOf(style.PropWidth, style.PropHeight, style.PropMaxWidth,
	style.PropMaxHeight, style.PropBoxSizing, style.PropAspectRatio)

// cssSized reports whether w's cascade sizes it through CSS.
func cssSized(v *style.Values) bool {
	relativeMin := func(s style.Sizing) bool { return s.Kind > style.SizingLength || s.Len.Relative() }
	return !v.Set.And(sizedProps).Empty() || relativeMin(v.Layout.MinWidth) || relativeMin(v.Layout.MinHeight)
}

// boxAxis is one axis of a widget's CSS box: its margins (the
// programmatic ones included), its border and padding, and whether
// box-sizing puts that border and padding inside the sizes.
type boxAxis struct {
	margin, bp int
	borderBox  bool
}

// out is the extent outside the box-sizing box: what a sizing value
// adds to reach the margin box.
func (b boxAxis) out() int {
	if b.borderBox {
		return b.margin
	}
	return b.margin + b.bp
}

// content is the content-box extent of a margin-box extent.
func (b boxAxis) content(outer int) int { return outer - b.margin - b.bp }

// boxAxisOf resolves w's box along a.
func boxAxisOf(w Widget, v *style.Values, a Axis) boxAxis {
	b, lm := cssBoxOf(w, v), layoutMarginOf(w)
	out := boxAxis{borderBox: v.Layout.BoxSizing == style.BoxSizingBorder}
	if a == Row {
		out.margin = b.margin.Left + b.margin.Right + lm.Left + lm.Right
		out.bp = b.border.Left + b.border.Right + b.padding.Left + b.padding.Right
	} else {
		out.margin = b.margin.Top + b.margin.Bottom + lm.Top + lm.Bottom
		out.bp = b.border.Top + b.border.Bottom + b.padding.Top + b.padding.Bottom
	}
	return out
}

// axisSizes is one axis of a widget's resolved CSS sizes, in outer
// (margin-box) pixels: the preferred size when it is definite, and the
// min/max bounds, max indefinite for none.
type axisSizes struct {
	ax       boxAxis
	size     int
	definite bool
	min, max int
}

// clamp holds n inside the bounds, min winning over max as in CSS.
func (s axisSizes) clamp(n int) int { return max(min(n, s.max), s.min) }

// sizingOn picks the width or height sizing values.
func sizingOn(l *style.Layout, a Axis) (size, lo, hi style.Sizing) {
	if a == Row {
		return l.Width, l.MinWidth, l.MaxWidth
	}
	return l.Height, l.MinHeight, l.MaxHeight
}

// resolveAxis resolves w's sizing properties along a against basis, the
// containing block's extent there (indefinite when unknown). Auto, an
// indefinite percentage and the block-axis (height) content keywords
// leave the size indefinite — CSS reads min-content, max-content and
// fit-content as auto in the block axis.
func resolveAxis(w Widget, v *style.Values, a Axis, basis int) axisSizes {
	size, lo, hi := sizingOn(&v.Layout, a)
	in := intrinsic{w: w, a: a, basis: basis, ax: boxAxisOf(w, v, a)}
	out := axisSizes{ax: in.ax, max: indefinite}
	out.size, out.definite = in.resolve(size)
	if n, ok := in.resolve(lo); ok {
		out.min = n
	}
	if n, ok := in.resolve(hi); ok {
		out.max = n
	}
	return out
}

// intrinsic resolves sizing values for one widget axis, measuring the
// widget at most once for the content keywords.
type intrinsic struct {
	w        Widget
	a        Axis
	basis    int
	ax       boxAxis
	measured bool
	req      Sizes
}

// content is the widget's min-content and max-content widths, its
// margin box's: the content keywords read them, the block axis never.
func (in *intrinsic) content() Sizes {
	if !in.measured {
		in.req = measureMargin(in.w, Row, Unknown)
		in.measured = true
	}
	return in.req
}

// resolve returns s in outer pixels, ok false when it leaves the size
// to the content (auto, none, an indefinite percentage).
func (in *intrinsic) resolve(s style.Sizing) (int, bool) {
	switch s.Kind {
	case style.SizingLength:
		if s.Len.Relative() && !finite(in.basis) {
			return 0, false
		}
		return max(0, s.Len.Resolve(in.basis)) + in.ax.out(), true
	case style.SizingMinContent, style.SizingMaxContent, style.SizingFitContent:
		if in.a == Column {
			return 0, false
		}
	default:
		return 0, false
	}
	c := in.content()
	switch s.Kind {
	case style.SizingMinContent:
		return c.Min, true
	case style.SizingMaxContent:
		return c.Nat, true
	}
	limit := c.Nat
	if !s.Len.Relative() || finite(in.basis) {
		limit = s.Len.Resolve(in.basis) + in.ax.out()
	}
	return min(c.Nat, max(c.Min, limit)), true
}

// measureOuter measures w's margin box along axis at forSize, its
// margin-box extent across: programmatic margin and CSS sizing
// included, the one measure every layout pass takes of an in-flow
// widget. basis is the containing block's extent along each axis,
// indefinite where the caller has none.
func measureOuter(w Widget, axis Axis, forSize int, basis Size) Sizes {
	if n := nodeOf(w); n != nil {
		if v := n.style(w); cssSized(v) {
			return sizedMeasure(w, v, axis, forSize, basis)
		}
	}
	return measureMargin(w, axis, forSize)
}

// sizedMeasure is measureOuter for a widget with CSS sizes: a definite
// size pins its axis to one extent, the min/max bounds hold both of the
// widget's sizes, and aspect-ratio transfers a definite size, or the
// extent across when the other axis is auto, to the axis measured.
func sizedMeasure(w Widget, v *style.Values, axis Axis, forSize int, basis Size) Sizes {
	s := resolveAxis(w, v, axis, basis.Along(axis))
	across := resolveAxis(w, v, otherAxis(axis), basis.Along(otherAxis(axis)))
	if across.definite {
		forSize = across.clamp(across.size)
	} else if forSize != Unknown {
		forSize = across.clamp(forSize)
	}
	if ratio := v.Layout.AspectRatio; ratio > 0 && !s.definite {
		scale := ratio
		if axis == Column {
			scale = 1 / ratio
		}
		if forSize != Unknown {
			box := float64(max(0, forSize-across.ax.out()))
			return FixedSizes(s.clamp(int(math.Round(box*scale)) + s.ax.out()))
		}
	}
	if s.definite {
		return FixedSizes(s.clamp(s.size))
	}
	req := measureMargin(w, axis, forSize)
	return Sizes{Min: s.clamp(req.Min), Nat: s.clamp(req.Nat)}
}

// holdSlot fits a container's slot r for w — its margin box — to its
// CSS sizes, resolved against cb, the containing block: a slot wider
// than a definite width or a max-width narrows to it, one narrower than
// a min-width widens, and the narrowed box sits by the widget's
// alignment (fill reads as start, the CSS block placement).
func holdSlot(w Widget, v *style.Values, r render.Rect, cb Size, h, al Align) render.Rect {
	ws := resolveAxis(w, v, Row, cb.W)
	hs := resolveAxis(w, v, Column, cb.H)
	lo, hi := Size{W: ws.min, H: hs.min}, Size{W: ws.max, H: hs.max}
	if ws.definite {
		lo.W = ws.clamp(ws.size)
		hi.W = lo.W
	}
	ratio := v.Layout.AspectRatio
	switch {
	case hs.definite:
		lo.H = hs.clamp(hs.size)
		hi.H = lo.H
		if !ws.definite && ratio > 0 {
			lo.W = ws.clamp(int(math.Round(float64(max(0, lo.H-hs.ax.out()))*ratio)) + ws.ax.out())
			hi.W = lo.W
		}
	case ratio > 0:
		width := ws.clamp(min(max(r.W, lo.W), hi.W))
		lo.H = hs.clamp(int(math.Round(float64(max(0, width-ws.ax.out()))/ratio)) + hs.ax.out())
		hi.H = lo.H
	}
	return fitSlot(r, lo, hi, h, al)
}

// fitSlot holds r between lo and hi on each axis, a narrowed axis
// placed by its alignment (fill reads as start).
func fitSlot(r render.Rect, lo, hi Size, h, v Align) render.Rect {
	fit := func(pos, size, lo, hi int, a Align) (int, int) {
		want := max(min(size, hi), lo)
		switch a {
		case AlignCenter:
			pos += (size - want) / 2
		case AlignEnd:
			pos += size - want
		}
		return pos, want
	}
	r.X, r.W = fit(r.X, r.W, lo.W, hi.W, h)
	r.Y, r.H = fit(r.Y, r.H, lo.H, hi.H, v)
	return r
}
