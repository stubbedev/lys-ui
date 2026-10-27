package widget

import (
	"math"
	"slices"
)

// Sizes is a widget's size request along one axis, GTK's minimum and
// natural size: Min is the least it lays out at without clipping or
// overflowing what it shows, Nat the size it wants. 0 <= Min <= Nat.
type Sizes struct {
	Min, Nat int
}

// Unknown is a forSize the measuring container does not know yet: a
// width asked before any height is decided.
const Unknown = -1

// FixedSizes is a request that wants exactly n: a widget of one size
// along an axis.
func FixedSizes(n int) Sizes { return Sizes{Min: n, Nat: n} }

// grow is s with n added to both sizes: a box's insets, a gap.
func (s Sizes) grow(n int) Sizes { return Sizes{Min: s.Min + n, Nat: s.Nat + n} }

// atLeast is s floored at n on both sizes: a size request.
func (s Sizes) atLeast(n int) Sizes { return Sizes{Min: max(s.Min, n), Nat: max(s.Nat, n)} }

// widest is the larger of s and o on each size: children sharing one
// extent, a column's widths.
func (s Sizes) widest(o Sizes) Sizes { return Sizes{Min: max(s.Min, o.Min), Nat: max(s.Nat, o.Nat)} }

// plus is s and o laid end to end: children along a row.
func (s Sizes) plus(o Sizes) Sizes { return Sizes{Min: s.Min + o.Min, Nat: s.Nat + o.Nat} }

// Within is n held to the request: never below Min, never above Nat.
func (s Sizes) Within(n int) int { return max(s.Min, min(n, s.Nat)) }

// SizeWithin is the size w takes in a host that offers at most avail:
// its natural width capped to avail.W, then its natural height for that
// width capped to avail.H, neither below its minimum. A window sizes its
// root with it, a popover its content. The result exceeds avail where
// the widget's minimum does: w overflows rather than squeezing below
// what it needs.
func SizeWithin(w Widget, avail Size) Size { return sizeWithin(w.Measure, avail) }

// sizeWithin is SizeWithin over any measure.
func sizeWithin(measure func(Axis, int) Sizes, avail Size) Size {
	width := measure(Row, Unknown).Within(avail.W)
	return Size{W: width, H: measure(Column, width).Within(avail.H)}
}

// childWithin is SizeWithin for child as parent's child.
func childWithin(parent, child Widget, avail Size) Size {
	return sizeWithin(func(axis Axis, forSize int) Sizes { return measureChild(parent, child, axis, forSize) }, avail)
}

// naturalChild is child's natural size as parent's child: its natural
// width, then its natural height at that width.
func naturalChild(parent, child Widget) Size {
	return childWithin(parent, child, Size{W: math.MaxInt, H: math.MaxInt})
}

// allocate splits extent along one axis among children with the
// given requests, writing each child's share to out, GTK's box
// allocation: every child gets its minimum; the room up to the sum of
// the naturals goes to the children furthest below theirs last, each
// step an equal share of what is left capped at the child's gap
// (gtk_distribute_natural_allocation); and what remains past every
// natural goes in equal shares to the children expand marks. Short of
// the sum of the minimums, each child keeps its minimum and the content
// overflows.
func allocate(req []Sizes, expand []bool, extent int, out []int) {
	extra := extent
	for i, r := range req {
		out[i] = r.Min
		extra -= r.Min
	}
	if extra <= 0 {
		return
	}
	order := allocateOrder[:0]
	for i := range req {
		order = append(order, i)
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return (req[a].Nat - req[a].Min) - (req[b].Nat - req[b].Min)
	})
	allocateOrder = order
	for k, i := range order {
		give := min(req[i].Nat-req[i].Min, extra/(len(order)-k))
		out[i] += give
		extra -= give
	}
	expanders := 0
	for _, e := range expand {
		if e {
			expanders++
		}
	}
	if expanders == 0 || extra == 0 {
		return
	}
	share, rest := extra/expanders, extra%expanders
	for i, e := range expand {
		if !e {
			continue
		}
		out[i] += share
		if rest > 0 {
			out[i]++
			rest--
		}
	}
}

// allocateOrder is allocate's reusable sort buffer (loop-goroutine
// only, like the other layout scratch).
var allocateOrder []int
