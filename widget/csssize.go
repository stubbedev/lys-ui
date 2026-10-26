package widget

// The CSS sizing properties on every widget (docs/css.md): width,
// height, their min-* and max-* bounds, box-sizing and aspect-ratio.
// They apply at the container boundary — measureOuter around the
// widget's own Measure, arrangeChild around its Arrange — so a widget
// gets them whatever its type, and a widget whose cascade declares
// none takes the plain path untouched. Percentages resolve against the
// containing block: the content box a CSS layout pass hands its items,
// and elsewhere the space the container offers in the measure
// constraint.

import (
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// indefinite is the available extent of an unbounded axis, the
// constraint Scroll measures its content under. A percentage of it
// resolves as auto, and no layout pass stretches into it.
const indefinite = math.MaxInt

// finite reports whether an available extent is definite.
func finite(n int) bool { return n < indefinite }

// extent is s along a.
func extent(s Size, a Axis) int {
	if a == Row {
		return s.W
	}
	return s.H
}

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
// containing block's extent there (indefinite when unknown); cross is
// the extent available across for the measures the content keywords
// take. Auto, an indefinite percentage and the block-axis (height)
// content keywords leave the size indefinite — CSS reads min-content,
// max-content and fit-content as auto in the block axis.
func resolveAxis(w Widget, v *style.Values, a Axis, basis, cross int) axisSizes {
	size, lo, hi := sizingOn(&v.Layout, a)
	in := intrinsic{w: w, a: a, cross: cross, basis: basis, ax: boxAxisOf(w, v, a)}
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
	cross    int
	ax       boxAxis
	measured bool
	nat      Size
}

// natural is the widget's max-content size: its plain measure, the
// axis unbounded.
func (in *intrinsic) natural() Size {
	if !in.measured {
		in.nat = measureMargin(in.w, Constraints{Max: withExtent(Size{W: in.cross, H: in.cross}, in.a, indefinite)})
		in.measured = true
	}
	return in.nat
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
	nat := in.natural()
	maxc := extent(nat, in.a)
	minc := minContentOf(in.w, in.a, nat)
	switch s.Kind {
	case style.SizingMinContent:
		return minc, true
	case style.SizingMaxContent:
		return maxc, true
	}
	limit := maxc
	if !s.Len.Relative() || finite(in.basis) {
		limit = s.Len.Resolve(in.basis) + in.ax.out()
	}
	return min(maxc, max(minc, limit)), true
}

// minContentOf is w's min-content extent along a, its floor: the
// natural extent less what the widget can give up (Shrinker,
// WidthShrinker), lowered further to a MinSizer's reported floor — a
// wrapping label's widest word. nat is its natural measure.
func minContentOf(w Widget, a Axis, nat Size) int {
	n := extent(nat, a)
	if a == Row {
		n -= shrinkableWidthOf(w)
	} else {
		n -= shrinkableOf(w)
	}
	if m, ok := w.(MinSizer); ok {
		lm := layoutMarginOf(w)
		floor := m.MinSize()
		floor.W += lm.Left + lm.Right
		floor.H += lm.Top + lm.Bottom
		n = min(n, extent(floor, a))
	}
	return max(0, n)
}

// measureOuter measures w's margin box under con, programmatic margin
// and CSS sizing included: the one measure every layout pass takes of
// an in-flow widget.
func measureOuter(w Widget, con Constraints) Size {
	if n := nodeOf(w); n != nil {
		if v := n.style(w); cssSized(v) {
			return sizedMeasure(w, n, v, con)
		}
	}
	return measureMargin(w, con)
}

// sizedMeasure is measureOuter for a widget with CSS sizes: a definite
// size pins its axis, the min/max bounds narrow the constraints the
// widget measures under, and aspect-ratio transfers a definite size —
// or, with both auto, the measured width — to the other axis. The
// bounds are recorded for arrangeChild, which holds a stretched slot
// to them.
func sizedMeasure(w Widget, n *node, v *style.Values, con Constraints) Size {
	ws := resolveAxis(w, v, Row, con.Max.W, con.Max.H)
	hs := resolveAxis(w, v, Column, con.Max.H, con.Max.W)
	ratio := v.Layout.AspectRatio
	transfer := func(from axisSizes, to *axisSizes, scale float64) {
		box := float64(max(0, from.clamp(from.size)-from.ax.out()))
		to.size, to.definite = int(math.Round(box*scale))+to.ax.out(), true
	}
	if ratio > 0 && ws.definite && !hs.definite {
		transfer(ws, &hs, 1/ratio)
	} else if ratio > 0 && hs.definite && !ws.definite {
		transfer(hs, &ws, ratio)
	}
	inner := con
	pin := func(s axisSizes, lo, hi *int) {
		if s.definite {
			*lo = min(max(s.clamp(s.size), *lo), *hi)
			*hi = *lo
			return
		}
		*hi = min(*hi, max(s.max, *lo))
		*lo = min(max(*lo, s.min), *hi)
	}
	pin(ws, &inner.Min.W, &inner.Max.W)
	pin(hs, &inner.Min.H, &inner.Max.H)
	sz := measureMargin(w, inner)
	if ratio > 0 && !ws.definite && !hs.definite {
		box := float64(max(0, sz.W-ws.ax.out()))
		sz.H = min(max(hs.clamp(int(math.Round(box/ratio))+hs.ax.out()), con.Min.H), con.Max.H)
	}
	n.cssMin, n.cssMax = Size{W: ws.min, H: hs.min}, Size{W: ws.max, H: hs.max}
	if ws.definite {
		n.cssMin.W, n.cssMax.W = sz.W, sz.W
	}
	if hs.definite || ratio > 0 {
		n.cssMin.H, n.cssMax.H = sz.H, sz.H
	}
	return clampSize(sz, con)
}

// holdSlot fits a container's slot r for w — its margin box — to the
// bounds the last sized measure recorded: a slot wider than a definite
// width or a max-width narrows to it, one narrower than a min-width
// widens, and the narrowed box sits by the widget's alignment (fill
// reads as start, the CSS block placement).
func holdSlot(n *node, r render.Rect, h, v Align) render.Rect {
	return fitSlot(r, n.cssMin, n.cssMax, h, v)
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
