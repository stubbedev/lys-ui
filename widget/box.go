package widget

import (
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Axis is the main axis a Box lays its children along.
type Axis uint8

const (
	// Row lays children out left to right.
	Row Axis = iota
	// Column lays children out top to bottom.
	Column
)

// childEntry is one Box child: the widget, whether it grows into leftover
// main-axis space, and its last natural measurement.
type childEntry struct {
	w      Widget
	expand bool
	// cross places the child across the box (AppendAligned);
	// AlignFill stretches it.
	cross Align
}

// crossAlign is the child's placement across the box: its align-self
// where a stylesheet declares one, else AppendAligned's.
func (c *childEntry) crossAlign() Align {
	v := styleOf(c.w)
	if !v.Has(style.PropAlignSelf) {
		return c.cross
	}
	switch v.Layout.AlignSelf.Kind {
	case style.BoxNormal, style.BoxStretch:
		return AlignFill
	case style.BoxStart, style.BoxFlexStart, style.BoxSelfStart, style.BoxLeft:
		return AlignStart
	case style.BoxEnd, style.BoxFlexEnd, style.BoxSelfEnd, style.BoxRight:
		return AlignEnd
	case style.BoxCenter:
		return AlignCenter
	case style.BoxBaseline, style.BoxLastBaseline:
		return AlignBaseline
	}
	return c.cross
}

// Box is a container that lays its children along one axis with fixed
// spacing and padding. The cross axis stretches children to the inner
// height (Row) or width (Column) of the box, unless a child was added
// with AppendAligned.
//
// Every child gets its minimum; the room up to the naturals goes to the
// children furthest below theirs last, and what is left past every
// natural is shared equally among the expanding children (GTK's box
// allocation). Short of the minimums the children overflow the box and
// the painter's clip decides what is visible.
//
// A row's start/end semantics mirror with the box's direction: an RTL
// row flows from the right edge, so the first child sits rightmost,
// where an RTL reader starts. Columns are unaffected.
//
// A stylesheet display declaration lays the children out by CSS
// instead — normal flow or flexbox (docs/css.md's Layout) — with the
// settings above standing in wherever the stylesheet is silent.
type Box struct {
	node
	axis    Axis
	dir     Direction
	spacing int
	padding render.Insets
	child   []*childEntry
	// homogeneous gives every child the largest child's main-axis size
	// (gtk_box_layout's homogeneous): a segmented control's buttons all
	// measure as wide as the widest label.
	homogeneous bool
	// stack is the children's paint order by their CSS stacking keys
	// (z-index, position, floats, order), indexes into child; empty
	// means tree order. Arrange computes it, so paint and hit testing
	// agree.
	stack []int
	// cssBase is the first baseline the last CSS layout pass found,
	// relative to the content box, when cssHasBase.
	cssBase    int
	cssHasBase bool
	// cssDefH is whether the parent's CSS pass sized the box's height
	// definitely (a flex or grid item, a flow item with a definite
	// height) for the arrange it is about to run, which resolves the
	// items' percentage heights against it.
	cssDefH bool
	// departing are the removed children still painting their way out
	// (departure.go).
	departing []*departure
}

// Children exposes the box's children in append order for focus
// traversal.
func (b *Box) Children() []Widget {
	return b.appendChildren(make([]Widget, 0, len(b.child)))
}

// appendChildren appends the box's children in append order, matching
// Children — the snapshot the per-frame damage walk buffers instead of
// a fresh slice per container per frame.
func (b *Box) appendChildren(buf []Widget) []Widget {
	for _, c := range b.child {
		buf = append(buf, c.w)
	}
	return buf
}

// SetDirection selects the base direction the box's main axis flows
// along: an RTL row lays its children out right to left, mirroring
// start/end placement. Columns ignore it. Changing the direction
// re-arranges.
func (b *Box) SetDirection(d Direction) {
	if b.dir == d {
		return
	}
	b.dir = d
	b.InvalidateLayout()
}

// Direction returns the base direction the main axis flows along.
func (b *Box) Direction() Direction { return b.dir }

// SetHomogeneous equalizes the children's main-axis sizes to the
// largest child's (gtk_box_set_homogeneous). Changing it relayouts.
func (b *Box) SetHomogeneous(on bool) {
	if b.homogeneous == on {
		return
	}
	b.homogeneous = on
	b.InvalidateLayout()
}

// Homogeneous reports whether the children share the largest child's
// main-axis size.
func (b *Box) Homogeneous() bool { return b.homogeneous }

// SetAxis lays the children along a instead (GTK's orientable): a
// field turning horizontal when it gets wide.
func (b *Box) SetAxis(a Axis) {
	if b.axis == a {
		return
	}
	b.axis = a
	b.InvalidateLayout()
}

// Axis returns the axis the children lie along.
func (b *Box) Axis() Axis { return b.axis }

// NewBox returns an empty box along axis with the given spacing between
// children and padding on every side.
func NewBox(axis Axis, spacing, padding int) *Box {
	return &Box{axis: axis, spacing: spacing, padding: render.UniformInsets(padding)}
}

// Append adds a widget to the box and reports whether it should expand
// into leftover main-axis space. It returns the box for chaining. The
// box's measure cache drops so the next frame sees the new child.
func (b *Box) Append(w Widget, expand bool) *Box {
	b.child = append(b.child, &childEntry{w: w, expand: expand})
	b.InvalidateLayout()
	restyleChildren(b)
	return b
}

// detachChild drops the child at index i: the removal hook fires while
// the child is still linked to the box (so a router can move a removed
// focus to its traversal neighbor), then the entry goes, the child's
// parent link clears, and the box reflows.
func (b *Box) detachChild(i int) {
	w := b.child[i].w
	notifyRemoved(w)
	b.child = slices.Delete(b.child, i, i+1)
	if !b.depart(w) {
		clearParents(w)
	}
	b.InvalidateLayout()
	restyleChildren(b)
}

// Remove detaches w, found by identity, and reports whether it was a
// child. The removed widget's parent link clears, so it can be
// appended elsewhere without a double parent; removing a widget that
// is not (or no longer) a child reports false and changes nothing.
// The measure cache drops so the next frame reflows without it.
func (b *Box) Remove(w Widget) bool {
	for i, c := range b.child {
		if c.w == w {
			b.detachChild(i)
			return true
		}
	}
	return false
}

// RemoveAt detaches the child at index i; an out-of-range index is a
// no-op.
func (b *Box) RemoveAt(i int) {
	if i < 0 || i >= len(b.child) {
		return
	}
	b.detachChild(i)
}

// Clear detaches every child at once — the wholesale rebuild path.
// Each child's parent link clears and the removal hook fires per
// child; clearing an empty box does nothing.
func (b *Box) Clear() {
	if len(b.child) == 0 {
		return
	}
	ws := make([]Widget, len(b.child))
	for i, c := range b.child {
		ws[i] = c.w
	}
	for _, w := range ws {
		notifyRemoved(w)
	}
	b.child = nil
	b.endDepartures()
	clearParents(ws...)
	b.InvalidateLayout()
}

// AppendAligned adds a child placed across the box by cross, GTK's
// valign in a row and halign in a column: AlignFill stretches it
// (Append), the others keep its natural cross size pinned to the
// start, center or end.
func (b *Box) AppendAligned(w Widget, expand bool, cross Align) *Box {
	b.child = append(b.child, &childEntry{w: w, expand: expand, cross: cross})
	b.InvalidateLayout()
	restyleChildren(b)
	return b
}

// InsertAt puts w at index i with Append's expand meaning, so dynamic
// UIs can reorder without a rebuild. Out-of-range indexes clamp to the
// ends; the measure cache drops like every other mutation.
func (b *Box) InsertAt(i int, w Widget, expand bool) { b.InsertAligned(i, w, expand, AlignFill) }

// InsertAligned is InsertAt with AppendAligned's cross placement.
func (b *Box) InsertAligned(i int, w Widget, expand bool, cross Align) {
	i = min(max(i, 0), len(b.child))
	b.child = slices.Insert(b.child, i, &childEntry{w: w, expand: expand, cross: cross})
	b.InvalidateLayout()
	restyleChildren(b)
}

// Move moves the child at index from to index to (both clamped), in
// place: the child stays attached, so its focus and state survive.
func (b *Box) Move(from, to int) {
	if from < 0 || from >= len(b.child) {
		return
	}
	to = min(max(to, 0), len(b.child)-1)
	if from == to {
		return
	}
	c := b.child[from]
	b.child = slices.Insert(slices.Delete(b.child, from, from+1), to, c)
	b.InvalidateLayout()
	restyleChildren(b)
}

func (b *Box) expands(c *childEntry) bool { return c.expand || WantsExpand(c.w, b.axis) }

// SetEnabled turns the box's subtree on or off: the per-query enable
// walk (IsEnabled) folds the box's flag into every descendant, whose
// own flag stays untouched — re-enabling the box never resurrects a
// child the app disabled on purpose. The walk marks the whole subtree
// for repaint, since each descendant's effective state changed.
func (b *Box) SetEnabled(enabled bool) {
	b.node.SetEnabled(enabled)
	invalidateTree(b)
}

// Measure reports the box's request inside its CSS box (padding,
// border, margin, min sizes): along its axis the children's requests
// end to end with the spacing between, every child the largest one's
// when homogeneous; across, the largest child's at the share of the
// main extent each would get, which Arrange hands out the same way.
// The result is cached until an InvalidateLayout anywhere in the
// subtree, so a static tree costs no recursion on later frames.
func (b *Box) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := b.measureHit(axis, forSize); ok {
		return sz
	}
	v := b.style(b)
	if mode := layoutModeOf(v); mode != modeBox {
		return b.measureStore(axis, forSize, measureBox(v, b.box(v), axis, forSize, func(axis Axis, forSize int) Sizes {
			return b.cssMeasure(v, mode, axis, forSize)
		}))
	}
	return b.measureStore(axis, forSize, measureBox(v, b.box(v), axis, forSize, func(axis Axis, forSize int) Sizes {
		if axis == b.axis {
			return b.mainRequest(v, forSize)
		}
		return b.crossRequest(v, forSize)
	}))
}

// mainRequest is the content's request along the axis at cross, its
// extent across: the flowing children end to end with the spacing.
func (b *Box) mainRequest(v *style.Values, cross int) Sizes {
	kinds := b.flows()
	defer boxFlows.pop()
	var sum, largest Sizes
	shown := 0
	for i, c := range b.child {
		if kinds[i] != flowIn {
			continue
		}
		r := measureOuter(c.w, b.axis, cross, indefiniteSize)
		sum, largest = sum.plus(r), largest.widest(r)
		shown++
	}
	if shown == 0 {
		return Sizes{}
	}
	if b.homogeneous {
		sum = Sizes{Min: largest.Min * shown, Nat: largest.Nat * shown}
	}
	return sum.grow(b.gap(v) * (shown - 1))
}

// crossRequest is the content's request across at main, its extent
// along the axis (Unknown for none): the largest child's at its share,
// and a row's baseline group's span.
func (b *Box) crossRequest(v *style.Values, main int) Sizes {
	kinds := b.flows()
	defer boxFlows.pop()
	shares := boxShares.push(len(b.child))
	defer boxShares.pop()
	b.split(v, main, Unknown, kinds, shares)
	cross := otherAxis(b.axis)
	var req Sizes
	above, below, grouped := 0, 0, false
	for i, c := range b.child {
		if kinds[i] != flowIn {
			continue
		}
		r := measureOuter(c.w, cross, shares[i], indefiniteSize)
		req = req.widest(r)
		if b.axis == Row && c.crossAlign() == AlignBaseline {
			if base, ok := baselineOf(c.w); ok {
				above, below, grouped = max(above, base), max(below, r.Nat-base), true
			}
		}
	}
	if grouped {
		req = req.atLeast(above + below)
	}
	return req
}

// boxShares holds split's main-axis shares per layout depth: a box
// keeps its level while it measures or arranges its children, whose
// own splits run a level deeper.
var (
	boxShares  depthStack[int]
	boxReqs    depthStack[Sizes]
	boxExpands depthStack[bool]
	boxFlows   depthStack[flow]
)

func (b *Box) flows() []flow {
	kinds := boxFlows.push(len(b.child))
	for i, c := range b.child {
		setParents(b, c.w)
		kinds[i] = flowOf(c.w)
	}
	return kinds
}

// split hands out main, the content's extent along the axis, among the
// flowing children as Arrange does, writing each one's share to out
// (Unknown when main is): homogeneous, an equal share each; otherwise
// allocate's split of their requests at cross, the extent across, the
// expanders sharing what is left past every natural.
func (b *Box) split(v *style.Values, main, cross int, kinds []flow, out []int) {
	shown := 0
	for _, k := range kinds {
		if k == flowIn {
			shown++
		}
	}
	if main == Unknown || shown == 0 {
		for i := range out {
			out[i] = Unknown
		}
		return
	}
	avail := max(0, main-b.gap(v)*(shown-1))
	if b.homogeneous {
		k := 0
		for i := range b.child {
			if kinds[i] == flowIn {
				out[i] = avail/shown + btoi(k < avail%shown)
				k++
			}
		}
		return
	}
	reqs := boxReqs.push(shown)
	defer boxReqs.pop()
	expand := boxExpands.push(shown)
	defer boxExpands.pop()
	shares := boxShares.push(shown)
	defer boxShares.pop()
	k := 0
	for i, c := range b.child {
		if kinds[i] == flowIn {
			reqs[k] = measureOuter(c.w, b.axis, cross, indefiniteSize)
			expand[k] = b.expands(c)
			k++
		}
	}
	allocate(reqs, expand, avail, shares)
	k = 0
	for i := range b.child {
		if kinds[i] == flowIn {
			out[i] = shares[k]
			k++
		}
	}
}

// baselineGroup spans a row's baseline-aligned children at the
// heights tall gives them: the most any reaches above the shared
// baseline and below it.
func (b *Box) baselineGroup(kinds []flow, tall []int) (above, below int, ok bool) {
	if b.axis != Row {
		return 0, 0, false
	}
	for i, c := range b.child {
		if kinds[i] != flowIn || c.crossAlign() != AlignBaseline {
			continue
		}
		if base, has := baselineOf(c.w); has {
			above, below, ok = max(above, base), max(below, tall[i]-base), true
		}
	}
	return above, below, ok
}

// crossExtents writes each flowing child's natural extent across at
// its share of main, the content's extent along the axis, to out.
func (b *Box) crossExtents(v *style.Values, main int, kinds []flow, out []int) {
	shares := boxShares.push(len(b.child))
	defer boxShares.pop()
	b.split(v, main, Unknown, kinds, shares)
	for i, c := range b.child {
		if kinds[i] == flowIn {
			out[i] = measureOuter(c.w, otherAxis(b.axis), shares[i], indefiniteSize).Nat
		}
	}
}

// Baseline implements Baseliner. A row's is its baseline group's,
// centered across the row as Arrange places it, else its first child
// with a baseline where its alignment puts it; a column's is its first
// child's. The children sit at their shares of the box's arranged
// width, or of its natural width before it is arranged.
func (b *Box) Baseline() (int, bool) {
	v := b.style(b)
	box := b.box(v)
	top := box.outer().Top
	if layoutModeOf(v) != modeBox {
		return top + b.cssBase, b.cssHasBase
	}
	// bounds is the border box, the measure the margin box.
	main := b.bounds.W - insetsAlong(box.border, Row) - insetsAlong(box.padding, Row)
	if b.bounds.Empty() {
		main = b.Measure(Row, Unknown).Nat - insetsAlong(box.outer(), Row)
	}
	main = max(0, main)
	if b.axis == Column {
		main = Unknown
	}
	kinds := b.flows()
	defer boxFlows.pop()
	tall := boxShares.push(len(b.child))
	defer boxShares.pop()
	b.crossExtents(v, main, kinds, tall)
	cross := 0
	for i := range b.child {
		if kinds[i] == flowIn {
			cross = max(cross, tall[i])
		}
	}
	if above, below, ok := b.baselineGroup(kinds, tall); ok {
		cross = max(cross, above+below)
		return top + (cross-above-below)/2 + above, true
	}
	for i, c := range b.child {
		if kinds[i] != flowIn {
			continue
		}
		base, ok := baselineOf(c.w)
		if b.axis == Column {
			return top + base, ok
		}
		if !ok {
			continue
		}
		switch c.crossAlign() {
		case AlignStart:
		case AlignEnd:
			base += cross - tall[i]
		default: // stretched or centered text sits centered
			base += (cross - tall[i]) / 2
		}
		return top + base, true
	}
	return 0, false
}

// box is the box's resolved CSS box: the stylesheet's padding where
// set, else the programmatic padding.
func (b *Box) box(v *style.Values) cssInsets { return boxOf(v, b.padding) }

// gap is the spacing between children: the constructor spacing plus
// the stylesheet's border-spacing along the main axis, as GTK 4's
// GtkBoxLayout adds the CSS spacing to its spacing property
// (gtkboxlayout.c get_spacing). A stylesheet's blanket
// `* { border-spacing: 0 }` therefore keeps a box's own spacing.
func (b *Box) gap(v *style.Values) int { return b.spacingAlong(v, b.axis) }

// spacingAlong is the box spacing between children laid along a: the
// constructor spacing plus the stylesheet's border-spacing on that
// axis.
func (b *Box) spacingAlong(v *style.Values, a Axis) int {
	if !v.Has(style.PropBorderSpacing) {
		return b.spacing
	}
	if a == Row {
		return b.spacing + v.BorderSpacingH
	}
	return b.spacing + v.BorderSpacingV
}

// SetPadding sets the box's programmatic padding per side; the
// stylesheet's padding still wins where it sets a side. The box
// relayouts.
func (b *Box) SetPadding(p render.Insets) {
	if b.padding == p {
		return
	}
	b.padding = p
	b.InvalidateLayout()
}

// Padding returns the programmatic padding.
func (b *Box) Padding() render.Insets { return b.padding }

// SetSpacing sets the programmatic gap between children.
func (b *Box) SetSpacing(spacing int) {
	if b.spacing == spacing {
		return
	}
	b.spacing = spacing
	b.InvalidateLayout()
}

// Spacing returns the programmatic gap between children.
func (b *Box) Spacing() int { return b.spacing }

// Arrange positions the children inside the content box — by the CSS
// layout its display declares, or the programmatic box layout — and
// orders them for paint and hit testing by their stacking keys. r is
// the margin box; Bounds records the border box.
func (b *Box) Arrange(r render.Rect) {
	v := b.style(b)
	border, inner := boxRects(b.box(v), r)
	b.ArrangeRoot(border)
	mode := layoutModeOf(v)
	switch {
	case inner.Empty():
		// Nothing flows in an empty box, but an out-of-flow child still
		// takes its place against its containing block.
		for _, c := range b.child {
			setParents(b, c.w)
			if flowOf(c.w) == flowOut {
				arrangeChild(c.w, render.Rect{X: inner.X, Y: inner.Y})
			} else {
				c.w.Arrange(render.Rect{})
			}
		}
	case mode != modeBox:
		b.cssArrange(v, mode, inner)
	default:
		b.arrangeBox(v, inner)
	}
	b.restack(mode)
}

// arrangeBox is the programmatic box layout: the children split the
// main axis as split hands it out (gtk_box_size_allocate), each stretched
// across the cross axis unless its alignment keeps its natural extent
// there. Out-of-flow children take no space; they are placed from where
// the flow reached.
func (b *Box) arrangeBox(v *style.Values, inner render.Rect) {
	spacing := b.gap(v)
	main, cross := inner.W, inner.H
	if b.axis == Column {
		main, cross = inner.H, inner.W
	}
	kinds := b.flows()
	defer boxFlows.pop()
	sizes := boxShares.push(len(b.child))
	defer boxShares.pop()
	b.split(v, main, cross, kinds, sizes)
	tall := boxArranged.push(len(b.child))
	defer boxArranged.pop()
	if b.axis == Row {
		for i, c := range b.child {
			if kinds[i] == flowIn {
				tall[i] = min(measureOuter(c.w, Column, sizes[i], indefiniteSize).Nat, inner.H)
			}
		}
	}

	pos := 0
	rtl := b.axis == Row && b.dir == DirectionRTL
	above, below, grouped := b.baselineGroup(kinds, tall)
	groupTop := inner.Y + (inner.H-above-below)/2
	for i, c := range b.child {
		if kinds[i] != flowIn {
			if kinds[i] == flowOut {
				at := Point{X: inner.X + pos, Y: inner.Y}
				if b.axis == Column {
					at = Point{X: inner.X, Y: inner.Y + pos}
				} else if rtl {
					at.X = inner.X + inner.W - pos
				}
				arrangeChild(c.w, render.Rect{X: at.X, Y: at.Y})
			} else {
				c.w.Arrange(render.Rect{})
			}
			continue
		}
		size := sizes[i]
		var rect render.Rect
		if b.axis == Row {
			x := inner.X + pos
			if rtl {
				x = inner.X + inner.W - pos - size // the row flows from the right edge
			}
			rect = render.Rect{X: x, Y: inner.Y, W: size, H: inner.H}
			natural := Size{W: size, H: tall[i]}
			switch al := c.crossAlign(); al {
			case AlignFill:
			case AlignBaseline:
				if base, ok := baselineOf(c.w); grouped && ok {
					rect.Y, rect.H = groupTop+above-base, natural.H
				} else {
					rect = alignRect(rect, natural, AlignFill, AlignCenter)
				}
			default:
				rect = alignRect(rect, natural, AlignFill, al)
			}
		} else {
			rect = render.Rect{X: inner.X, Y: inner.Y + pos, W: inner.W, H: size}
			if al := c.crossAlign(); al != AlignFill {
				rect = alignRect(rect, Size{W: min(measureOuter(c.w, Row, size, indefiniteSize).Nat, inner.W), H: size}, al, AlignFill)
			}
		}
		arrangeChild(c.w, rect)
		pos += size + spacing
	}
}

// boxArranged holds arrangeBox's cross extents per layout depth.
var boxArranged depthStack[int]

// ArrangeRoot records the box's own rect.
func (b *Box) ArrangeRoot(r render.Rect) {
	b.node.Arrange(r)
}

// Paint draws the box's CSS layers when the stylesheet gives it any (a
// bare box paints nothing — theme-only boxes are transparent), then the
// children in stacking order, then the outline; opacity and filter wrap
// it all. A transform declaration paints the whole subtree into an
// offscreen layer and composites it back through the affine, so the
// transform applies to the children as one.
func (b *Box) Paint(cv *render.Canvas) {
	v := b.style(b)
	if v.Has(style.PropTransform) && (v.Transform.M != render.Identity || v.Transform.P != render.Identity3) {
		b.paintTransformed(cv, v)
		return
	}
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, b.bounds, radii)
	b.paintBox(fx.cv, v, radii)
	fx.pop()
}

// paintBox paints the box's own layers, its children, and its outline.
// The layers and the outline need a box that generates one (not under
// display: contents) and shows it (visibility: visible); the children
// paint by their own visibility either way, inside the overflow clip.
func (b *Box) paintBox(cv *render.Canvas, v *style.Values, radii render.Corners) {
	own := !displayContents(v) && v.Layout.Visibility == style.VisibilityVisible
	if bg := pickc(0, v, style.PropBackgroundColor, 0); own && (bg != 0 || hasBoxLayers(v)) {
		paintBoxBehind(cv, v, b.bounds, radii, borderOf(v), bg)
	}
	if clip, ok := b.overflowClip(v); ok {
		prev := cv.PushClip(clip)
		b.paintChildren(cv)
		b.paintDepartures(cv)
		cv.PopClip(prev)
	} else {
		b.paintChildren(cv)
		b.paintDepartures(cv)
	}
	if own {
		paintOutline(cv, v, b.bounds, radii)
	}
}

// paintChildren paints the visible children in stacking order.
func (b *Box) paintChildren(cv *render.Canvas) {
	for k := range b.child {
		if c := b.child[b.stackAt(k)]; ownVisible(c.w) {
			PaintChild(cv, c.w)
		}
	}
}

// stackAt is the child index painted k-th.
func (b *Box) stackAt(k int) int {
	if len(b.stack) == len(b.child) {
		return b.stack[k]
	}
	return k
}

// restack orders the children by their CSS stacking keys; with every
// key zero the order is the tree's and the buffer stays empty.
func (b *Box) restack(mode layoutMode) {
	b.stack = b.stack[:0]
	if !slices.ContainsFunc(b.child, func(c *childEntry) bool { return stackKeyOf(c.w, mode) != stackKey{} }) {
		return
	}
	for i := range b.child {
		b.stack = append(b.stack, i)
	}
	slices.SortStableFunc(b.stack, func(i, j int) int {
		return compareStack(stackKeyOf(b.child[i].w, mode), stackKeyOf(b.child[j].w, mode))
	})
}

// overflowReach is how far an unclipped axis of an overflow clip
// extends: further than any tree reaches.
const overflowReach = 1 << 24

// overflowClip is the clip a clipping overflow puts on the children;
// see overflowClipOf.
func (b *Box) overflowClip(v *style.Values) (render.Rect, bool) { return overflowClipOf(v, b.bounds) }

// overflowClipOf is the clip a clipping overflow (hidden, clip, and
// scroll and auto, which add no scrollbars outside a Scroll) puts on a
// widget's content: the padding box of bounds on each clipping axis,
// unbounded on a visible one.
func overflowClipOf(v *style.Values, bounds render.Rect) (render.Rect, bool) {
	cx := v.Has(style.PropOverflowX) && v.Layout.OverflowX.Clips()
	cy := v.Has(style.PropOverflowY) && v.Layout.OverflowY.Clips()
	if !cx && !cy {
		return render.Rect{}, false
	}
	r := borderOf(v).Shrink(bounds)
	if !cx {
		r.X, r.W = r.X-overflowReach, r.W+2*overflowReach
	}
	if !cy {
		r.Y, r.H = r.Y-overflowReach, r.H+2*overflowReach
	}
	return r, true
}

// paintTransformed paints the box's subtree offscreen and composites
// it through the CSS transform about the transform-origin: the pivot
// the stylesheet's fractions and offsets name, the box's center by
// default. The layer is the transformed bounds, so a swatch swung past
// its box still lands. The box's own effects — opacity, filters, the
// clip and the mask — apply inside the layer, in the box's local
// space, before the transform maps the result; the backdrop filter
// samples the canvas under the untransformed box first.
func (b *Box) paintTransformed(cv *render.Canvas, v *style.Values) {
	radii := radiusOr(v, 0)
	if backfaceHidden(v) {
		return
	}
	paintBackdropOf(cv, v, b.bounds, radii)
	frac, off := v.Origin()
	ox := float64(b.bounds.X) + frac[0]*float64(b.bounds.W) + off[0]
	oy := float64(b.bounds.Y) + frac[1]*float64(b.bounds.H) + off[1]
	m := v.Transform.M.About(ox, oy)
	region := m.MapBounds(b.bounds)
	region = region.Union(b.bounds)
	var p3 render.M3
	if v.Transform.P != render.Identity3 || v.Perspective > 0 {
		p3 = render.Affine3(render.Translate(ox, oy)).Mul(v.Transform.P).Mul(render.Affine3(render.Translate(-ox, -oy)))
		if v.Perspective > 0 {
			p3 = render.Perspective4(v.Perspective).Plane3().Mul(p3)
		}
		region = p3.MapBounds(b.bounds).Union(region)
	}
	// The layer and the composite work in device pixels: the region
	// maps out, and the transforms - logical, about a logical origin -
	// conjugate by the device scale.
	num, denom := cv.DeviceScale()
	f := float64(num) / float64(denom)
	toDev, toLogical := render.Scale(f, f), render.Scale(1/f, 1/f)
	dev := cv.MapRect(region)
	l := cv.ScratchLayer(dev)
	defer l.Release()
	fx := pushLayerEffects(l.Canvas(), v, b.bounds, radii)
	b.paintBox(fx.cv, v, radii)
	fx.pop()
	if v.Transform.P != render.Identity3 || v.Perspective > 0 {
		cv.CompositeProjective(l, dev, render.Affine3(toDev).Mul(p3).Mul(render.Affine3(toLogical)), 1)
		return
	}
	cv.Composite(l, l.Canvas().Rect(), toDev.Mul(m).Mul(toLogical), 1)
}

// HitTest returns the deepest child under p, topmost in stacking order
// first and only inside the overflow clip, or the box itself when p is
// inside its bounds but over no child (padding, spacing, leftover
// space) — never a display:contents box, which has no area of its own.
func (b *Box) HitTest(p Point) Widget {
	v := b.style(b)
	if clip, ok := b.overflowClip(v); !ok || clip.Contains(p.X, p.Y) {
		for k := range slices.Backward(b.child) {
			c := b.child[b.stackAt(k)]
			if hit := HitChild(c.w, p); hit != nil {
				return hit
			}
		}
	}
	if displayContents(v) {
		return nil
	}
	return b.HitLeaf(b, p)
}
