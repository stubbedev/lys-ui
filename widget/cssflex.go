package widget

// The CSS flexbox layout (CSS Flexible Box Layout §9) over a Box's
// items: flex base and hypothetical main sizes with the automatic
// minimum size, line breaking, the flexible-length resolution with its
// freeze loop, cross sizes with stretch and baseline groups,
// align-content over the lines, and the main-axis distribution of auto
// margins and justify-content, in every direction and wrap mode.

import (
	"math"
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// flexItem is a cssItem's flexbox scratch. Main sizes are outer
// (margin-box) pixels, kept fractional until the line is placed.
type flexItem struct {
	base, hyp, target float64
	minMain, maxMain  float64
	// inner is the content-box flex base size the shrink factor
	// scales by.
	inner        float64
	grow, shrink float64
	// least is the item's main extent at its narrowest: its minimum
	// when it shrinks, its hypothetical size when it does not.
	least     float64
	viol      float64
	frozen    bool
	collapsed bool
	// main and mainPos are the placed main size and its offset from
	// the line's main start; cross and crossPos the same across.
	main, mainPos   int
	cross, crossPos int
	crossSizes      axisSizes
	align           style.BoxAlign
	baseline        int
	hasBaseline     bool
}

// flexLine is one line of a flex layout: its item range in the pass,
// its cross extent and offset, and its baseline group.
type flexLine struct {
	start, end   int
	cross, pos   int
	above, below int
	used         float64
}

// flexLines are the flex passes' line buffers, one per nesting level.
var flexLines depthStack[flexLine]

// flexAxes is a flex container's resolved geometry.
type flexAxes struct {
	main, cross Axis
	// reverse is row-reverse or column-reverse; mainFlip lays the
	// main axis from the physical end (a reversed row, or an RTL row
	// that is not reversed); crossFlip the cross axis likewise (wrap-
	// reverse, or a column's inline axis in RTL).
	reverse, mainFlip, crossFlip bool
	wrap, wrapReverse            bool
	gapMain, gapCross            int
}

// flexAxesOf resolves the container's axes for a content box of size:
// flex-direction where declared and the box axis otherwise, and the
// gaps — row-gap and column-gap where declared, the box spacing
// otherwise.
func (p *cssPass) flexAxesOf(size Size) flexAxes {
	l := &p.v.Layout
	var f flexAxes
	f.main, f.reverse = p.b.flexDirection(p.v)
	f.cross = otherAxis(f.main)
	f.wrap = p.v.Has(style.PropFlexWrap) && l.FlexWrap != style.FlexNowrap
	f.wrapReverse = f.wrap && l.FlexWrap == style.FlexWrapReverse
	if f.main == Row {
		f.mainFlip, f.crossFlip = p.rtl != f.reverse, f.wrapReverse
	} else {
		f.mainFlip, f.crossFlip = f.reverse, p.rtl != f.wrapReverse
	}
	colGap, rowGap := p.gapAlong(Row, size.Along(Row)), p.gapAlong(Column, size.Along(Column))
	f.gapMain, f.gapCross = colGap, rowGap
	if f.main == Column {
		f.gapMain, f.gapCross = rowGap, colGap
	}
	return f
}

// gapAlong is the gap between the items laid along a — column-gap
// between the columns of a row, row-gap between the rows: the declared
// gap, a percentage of the content box's basis extent (zero when that
// is indefinite), else the box spacing.
func (p *cssPass) gapAlong(a Axis, basis int) int {
	prop, l := style.PropColumnGap, p.v.Layout.ColumnGap
	if a == Column {
		prop, l = style.PropRowGap, p.v.Layout.RowGap
	}
	if !p.v.Has(prop) {
		return p.b.spacingAlong(p.v, a)
	}
	if !finite(basis) {
		basis = 0
	}
	return l.Resolve(basis)
}

// flexDirection is the box's flex main axis and whether it runs
// reversed: flex-direction where declared, the box axis otherwise.
func (b *Box) flexDirection(v *style.Values) (main Axis, reverse bool) {
	if !v.Has(style.PropFlexDirection) {
		return b.axis, false
	}
	switch v.Layout.FlexDirection {
	case style.FlexRowReverse:
		return Row, true
	case style.FlexColumn:
		return Column, false
	case style.FlexColumnReverse:
		return Column, true
	}
	return Row, false
}

// boxAlignOf maps a programmatic alignment onto the CSS one.
func boxAlignOf(a Align) (style.BoxAlign, bool) {
	switch a {
	case AlignStart:
		return style.BoxAlign{Kind: style.BoxStart}, true
	case AlignCenter:
		return style.BoxAlign{Kind: style.BoxCenter}, true
	case AlignEnd:
		return style.BoxAlign{Kind: style.BoxEnd}, true
	case AlignBaseline:
		return style.BoxAlign{Kind: style.BoxBaseline}, true
	}
	return style.BoxAlign{}, false
}

// alignSelfOf resolves a flex item's cross alignment: selfAlignOf's
// along the cross axis, AppendAligned placing it; normal stretches.
func (p *cssPass) alignSelfOf(it *cssItem, cross Axis) style.BoxAlign {
	a := p.selfAlignOf(it, cross, true, style.PropAlignSelf, style.PropAlignItems)
	if a.Kind == style.BoxNormal {
		a.Kind = style.BoxStretch
	}
	return a
}

// selfAlignOf resolves an item's self alignment along a: its self
// property (align-self or justify-self) where declared and not auto,
// the container's items property (align-items or justify-items), then
// the programmatic placement — SetHAlign/SetVAlign along a, and
// AppendAligned's cross alignment where appended says it places along
// a — and normal, the initial value, last.
func (p *cssPass) selfAlignOf(it *cssItem, a Axis, appended bool, self, items style.Prop) style.BoxAlign {
	switch own := boxAlignField(&it.v.Layout, self); {
	case it.v.Has(self) && own.Kind != style.BoxAuto:
		return own
	case p.v.Has(items):
		return boxAlignField(&p.v.Layout, items)
	}
	prog := AlignFill
	if l := layoutOf(it.w); l != nil {
		prog = l.valign
		if a == Row {
			prog = l.halign
		}
	}
	if appended && it.entry.cross != AlignFill {
		prog = it.entry.cross
	}
	if m, ok := boxAlignOf(prog); ok {
		return m
	}
	return style.BoxAlign{Kind: style.BoxNormal}
}

// boxAlignField is the computed value of one self or items alignment
// longhand.
func boxAlignField(l *style.Layout, p style.Prop) style.BoxAlign {
	switch p {
	case style.PropAlignSelf:
		return l.AlignSelf
	case style.PropAlignItems:
		return l.AlignItems
	case style.PropJustifySelf:
		return l.JustifySelf
	}
	return l.JustifyItems
}

// flexFactors are an item's grow and shrink factors: flex-grow where
// declared, else 1 for a child that expands along the main axis
// (Append's expand, SetHExpand/SetVExpand); flex-shrink where
// declared, else CSS's 1.
func flexFactors(it *cssItem, main Axis) (grow, shrink float64) {
	l := &it.v.Layout
	shrink = 1
	if it.v.Has(style.PropFlexShrink) {
		shrink = l.FlexShrink
	}
	switch {
	case it.v.Has(style.PropFlexGrow):
		grow = l.FlexGrow
	case it.entry.expand || WantsExpand(it.w, main):
		grow = 1
	}
	return grow, shrink
}

// clipsMain reports whether the item's overflow clips along the main
// axis, which drops its automatic minimum size to zero.
func clipsMain(it *cssItem, main Axis) bool {
	if main == Row {
		return it.v.Has(style.PropOverflowX) && it.v.Layout.OverflowX.Clips()
	}
	return it.v.Has(style.PropOverflowY) && it.v.Layout.OverflowY.Clips()
}

// flexMeasure is a flex container's request along axis at forSize:
// along the main axis, the line's hypothetical sizes, at least every
// item at its narrowest end to end (one line) or the narrowest widest
// item (wrapping); across, the lines' extent at the main size given, or
// at the natural one.
func (p *cssPass) flexMeasure(axis Axis, forSize int) Sizes {
	main, _ := p.b.flexDirection(p.v)
	size := indefiniteSize
	if forSize != Unknown {
		size = withExtent(size, otherAxis(axis), forSize)
	}
	if axis != main {
		return FixedSizes(p.flex(size, forSize != Unknown, false).Along(axis))
	}
	nat := p.flex(size, false, forSize != Unknown).Along(axis)
	f := p.flexAxesOf(size)
	least, shown := 0.0, 0
	for i := range p.items {
		it := &p.items[i]
		if it.kind != itemFlow || it.collapsed {
			continue
		}
		if f.wrap {
			least = math.Max(least, it.least)
		} else {
			least += it.least
		}
		shown++
	}
	if !f.wrap {
		least += float64(f.gapMain * max(0, shown-1))
	}
	return Sizes{Min: min(int(math.Ceil(least)), nat), Nat: nat}
}

// flex lays the items out as a flex container whose content box is
// size; mainDef and crossDef say whether those extents are definite
// (an arrange always is; a measure where it is given the extent). It returns the content size the layout needs: the widest line's
// hypothetical sizes along the main axis, the lines and their gaps
// across.
func (p *cssPass) flex(size Size, mainDef, crossDef bool) Size {
	f := p.flexAxesOf(size)
	mainSize, crossSize := size.Along(f.main), size.Along(f.cross)
	mainBasis, crossBasis := definiteOr(mainSize, mainDef), definiteOr(crossSize, crossDef)
	cb := withExtent(withExtent(Size{}, f.main, mainBasis), f.cross, crossBasis)
	flows, ordered := 0, false
	for i := range p.items {
		if it := &p.items[i]; it.kind == itemFlow {
			p.flexBase(it, f, cb, crossSize)
			flows++
			ordered = ordered || it.v.Layout.Order != 0
		}
	}
	if ordered {
		slices.SortStableFunc(p.items, func(a, b cssItem) int { return a.v.Layout.Order - b.v.Layout.Order })
	}

	lines := flexLines.push(max(1, flows))
	defer flexLines.pop()
	lines = p.flexBreak(lines, f, mainDef, mainSize)
	natMain := 0.0
	for li := range lines {
		ln := &lines[li]
		gaps := float64(f.gapMain * max(0, p.flowCount(ln)-1))
		ln.used = gaps
		for i := ln.start; i < ln.end; i++ {
			if it := &p.items[i]; it.kind == itemFlow {
				ln.used += it.hyp
			}
		}
		natMain = max(natMain, ln.used)
		if mainDef {
			p.resolveFlexible(ln, float64(mainSize), gaps)
		} else {
			for i := ln.start; i < ln.end; i++ {
				p.items[i].target = p.items[i].hyp
			}
		}
	}
	if !mainDef {
		mainSize = int(math.Ceil(natMain))
	}
	for li := range lines {
		p.flexMainPlace(&lines[li], f, mainSize, mainDef)
		p.flexCrossSize(&lines[li], f, cb, crossSize, crossDef)
	}
	cross := p.flexLinesPlace(lines, f, crossSize, crossDef)
	for li := range lines {
		p.flexCrossPlace(&lines[li], f)
	}
	for i := range p.items {
		if it := &p.items[i]; it.kind == itemFlow {
			main, crossPos := it.mainPos, it.crossPos
			if f.mainFlip {
				main = mainSize - it.mainPos - it.main
			}
			it.rect = rectOn(f.main, main, crossPos, it.main, it.cross)
			if !p.hasBase && it.hasBaseline {
				p.base, p.hasBase = it.rect.Y+it.baseline, true
			}
		}
	}
	return withExtent(withExtent(Size{}, f.main, int(math.Ceil(natMain))), f.cross, cross)
}

// rectOn builds a rect from main and cross coordinates.
func rectOn(main Axis, mainPos, crossPos, mainSize, crossSize int) render.Rect {
	if main == Row {
		return render.Rect{X: mainPos, Y: crossPos, W: mainSize, H: crossSize}
	}
	return render.Rect{X: crossPos, Y: mainPos, W: crossSize, H: mainSize}
}

// flowCount counts a line's items that take main space.
func (p *cssPass) flowCount(ln *flexLine) int {
	n := 0
	for i := ln.start; i < ln.end; i++ {
		if it := &p.items[i]; it.kind == itemFlow && !it.collapsed {
			n++
		}
	}
	return n
}

// flexBase resolves one item's flex base size and hypothetical main
// size (§9.2) in a container whose containing block is cb: a definite
// flex-basis, else a definite main size property, else the content's
// max-content size, measured at its cross size — a definite one, the
// stretched line of a single-line container's definite cross size, or
// its natural cross size within crossAvail; then the min/max main
// sizes, the automatic minimum (the content's min-content size, capped
// by a definite main size) standing in for min auto unless the item
// clips its overflow.
func (p *cssPass) flexBase(it *cssItem, f flexAxes, cb Size, crossAvail int) {
	mainBasis, crossBasis := cb.Along(f.main), cb.Along(f.cross)
	it.flexItem = flexItem{}
	it.collapsed = it.v.Layout.Visibility == style.VisibilityCollapse
	ms := resolveAxis(it.w, it.v, f.main, mainBasis)
	cs := resolveAxis(it.w, it.v, f.cross, crossBasis)
	it.crossSizes = cs
	it.align = p.alignSelfOf(it, f.cross)
	it.grow, it.shrink = flexFactors(it, f.main)
	across := Unknown
	switch {
	case cs.definite:
		across = min(cs.clamp(cs.size), crossAvail)
	case !f.wrap && finite(crossBasis) && it.align.Kind == style.BoxStretch && !hasAutoCross(it, f.cross):
		across = cs.clamp(crossBasis)
	case f.cross == Row && finite(crossAvail):
		across = measureOuter(it.w, Row, Unknown, cb).Within(crossAvail)
	}
	req := measureOuter(it.w, f.main, across, cb)
	basis := style.Sizing{}
	if it.v.Has(style.PropFlexBasis) {
		basis = it.v.Layout.FlexBasis
	}
	base := req.Nat
	switch basis.Kind {
	case style.SizingLength:
		if !basis.Len.Relative() || finite(mainBasis) {
			base = max(0, basis.Len.Resolve(mainBasis)) + ms.ax.out()
		}
	case style.SizingAuto:
		if ms.definite {
			base = ms.size
		}
	case style.SizingMinContent:
		base = req.Min
	case style.SizingFitContent:
		if finite(mainBasis) {
			base = req.Within(mainBasis)
		}
	}
	lo := ms.min
	_, minSize, _ := sizingOn(&it.v.Layout, f.main)
	if minSize.Kind == style.SizingAuto && !clipsMain(it, f.main) {
		lo = min(req.Min, ms.max)
		if ms.definite {
			lo = min(lo, ms.clamp(ms.size))
		}
	}
	it.base = float64(base)
	it.inner = float64(max(0, ms.ax.content(base)))
	it.minMain, it.maxMain = float64(lo), float64(max(ms.max, lo))
	it.hyp = math.Max(math.Min(it.base, it.maxMain), it.minMain)
	it.least = it.hyp
	if it.shrink > 0 {
		it.least = math.Min(it.hyp, it.minMain)
	}
	if it.collapsed {
		it.cross = measureOuter(it.w, f.cross, int(math.Round(it.hyp)), cb).Nat
		it.base, it.hyp, it.inner, it.least = 0, 0, 0, 0
	}
}

// hasAutoCross reports an auto margin across.
func hasAutoCross(it *cssItem, cross Axis) bool {
	s, e := autoMargins(it, cross)
	return s || e
}

// flexBreak collects the items into lines (§9.3): one line unless the
// container wraps with a definite main size, else as many items per
// line as their hypothetical sizes and the gaps between them fit, at
// least one.
func (p *cssPass) flexBreak(lines []flexLine, f flexAxes, mainDef bool, mainSize int) []flexLine {
	n := 0
	lines[0] = flexLine{start: 0}
	used, count := 0.0, 0
	for i := range p.items {
		it := &p.items[i]
		if !f.wrap || !mainDef || it.kind != itemFlow || it.collapsed {
			continue
		}
		add := it.hyp
		if count > 0 {
			add += float64(f.gapMain)
		}
		if count > 0 && used+add > float64(mainSize) {
			lines[n].end = i
			n++
			lines[n] = flexLine{start: i}
			used, count, add = 0, 0, it.hyp
		}
		used += add
		count++
	}
	lines[n].end = len(p.items)
	return lines[:n+1]
}

// resolveFlexible resolves the flexible lengths of one line (§9.7):
// the items grow into positive free space by flex-grow or shrink out of
// negative free space by flex-shrink scaled by their inner base sizes,
// a factor sum below one distributing only that fraction, and every
// round freezes the items its min/max clamping violated until none is
// left flexible.
func (p *cssPass) resolveFlexible(ln *flexLine, avail, gaps float64) {
	items := p.items[ln.start:ln.end]
	growing := ln.used < avail
	factor := func(it *cssItem) float64 {
		if growing {
			return it.grow
		}
		return it.shrink
	}
	initial := avail - gaps
	for k := range items {
		it := &items[k]
		if it.kind != itemFlow {
			continue
		}
		it.target, it.frozen = it.base, it.collapsed
		if it.collapsed || factor(it) == 0 || growing && it.base > it.hyp || !growing && it.base < it.hyp {
			it.target, it.frozen = it.hyp, true
		}
		if it.frozen {
			initial -= it.target
		} else {
			initial -= it.base
		}
	}
	for {
		free, sum, scaled, open := avail-gaps, 0.0, 0.0, 0
		for k := range items {
			it := &items[k]
			if it.kind != itemFlow {
				continue
			}
			if it.frozen {
				free -= it.target
				continue
			}
			free -= it.base
			sum += factor(it)
			scaled += it.shrink * it.inner
			open++
		}
		if open == 0 {
			return
		}
		if sum < 1 && math.Abs(initial*sum) < math.Abs(free) {
			free = initial * sum
		}
		total := 0.0
		for k := range items {
			it := &items[k]
			if it.kind != itemFlow || it.frozen {
				continue
			}
			t := it.base
			switch {
			case growing && sum > 0:
				t += free * it.grow / sum
			case !growing && scaled > 0:
				t += free * it.shrink * it.inner / scaled
			}
			c := math.Max(math.Min(t, it.maxMain), it.minMain)
			it.target, it.viol = c, c-t
			total += c - t
		}
		const eps = 1e-9
		for k := range items {
			it := &items[k]
			if it.kind != itemFlow || it.frozen {
				continue
			}
			if math.Abs(total) < eps || total > 0 && it.viol > eps || total < 0 && it.viol < -eps {
				it.frozen = true
			}
		}
	}
}

// distribute splits free space for a justify-content or align-content
// value over n subjects: the offset before the first and the extra
// between neighbors, measured from the axis's flow start. startAtFlow
// reports whether the writing mode's start (start, self-start) lies at
// the flow start, leftAtFlow whether the physical left does.
// Distributed values fall back as CSS says when the space is negative
// or there is only one subject, and safe falls back to the flow start
// on overflow.
func distribute(a style.BoxAlign, free float64, n int, startAtFlow, leftAtFlow bool) (offset, between float64) {
	if free < 0 && a.Safe {
		return 0, 0
	}
	switch a.Kind {
	case style.BoxFlexEnd:
		return free, 0
	case style.BoxCenter:
		return free / 2, 0
	case style.BoxStart, style.BoxSelfStart:
		if startAtFlow {
			return 0, 0
		}
		return free, 0
	case style.BoxEnd, style.BoxSelfEnd:
		if startAtFlow {
			return free, 0
		}
		return 0, 0
	case style.BoxLeft:
		if leftAtFlow {
			return 0, 0
		}
		return free, 0
	case style.BoxRight:
		if leftAtFlow {
			return free, 0
		}
		return 0, 0
	case style.BoxSpaceBetween:
		if free > 0 && n > 1 {
			return 0, free / float64(n-1)
		}
	case style.BoxSpaceAround:
		if free <= 0 || n == 0 {
			return free / 2, 0
		}
		return free / float64(n) / 2, free / float64(n)
	case style.BoxSpaceEvenly:
		if free <= 0 {
			return free / 2, 0
		}
		return free / float64(n+1), free / float64(n+1)
	}
	return 0, 0
}

// flexMainPlace places one line's items along the main axis (§9.5):
// auto margins take the free space first, else justify-content
// distributes it. Positions run from the line's main start and round at
// the edges, so the sizes always sum to the placed total.
func (p *cssPass) flexMainPlace(ln *flexLine, f flexAxes, mainSize int, mainDef bool) {
	used := float64(f.gapMain * max(0, p.flowCount(ln)-1))
	autos := 0
	for i := ln.start; i < ln.end; i++ {
		it := &p.items[i]
		if it.kind != itemFlow || it.collapsed {
			continue
		}
		used += it.target
		s, e := autoMargins(it, f.main)
		autos += btoi(s) + btoi(e)
	}
	free := 0.0
	if mainDef {
		free = float64(mainSize) - used
	}
	var offset, between, share float64
	if autos > 0 && free > 0 {
		share = free / float64(autos)
	} else {
		justify := style.BoxAlign{Kind: style.BoxFlexStart}
		if p.v.Has(style.PropJustifyContent) {
			justify = p.v.Layout.JustifyContent
		}
		// start is the inline (row) or block (column) start, which the
		// reverse directions move away from main start; left and right
		// apply to rows only.
		leftAtFlow := !f.mainFlip
		if f.main == Column {
			leftAtFlow = !f.reverse
		}
		offset, between = distribute(justify, free, p.flowCount(ln), !f.reverse, leftAtFlow)
	}
	pos := offset
	for i := ln.start; i < ln.end; i++ {
		it := &p.items[i]
		if it.kind != itemFlow {
			continue
		}
		if it.collapsed {
			it.mainPos, it.main = int(math.Round(pos)), 0
			continue
		}
		s, e := autoMargins(it, f.main)
		if s {
			pos += share
		}
		a, b := int(math.Round(pos)), int(math.Round(pos+it.target))
		it.mainPos, it.main = a, b-a
		pos += it.target + float64(f.gapMain) + between
		if e {
			pos += share
		}
	}
}

// btoi is 1 for true.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// flexCrossSize measures one line's items across at their used main
// sizes and sizes the line (§9.4): a single-line container's definite
// cross size, else its tallest item or its baseline group.
func (p *cssPass) flexCrossSize(ln *flexLine, f flexAxes, cb Size, crossSize int, crossDef bool) {
	ln.cross, ln.above, ln.below = 0, 0, 0
	for i := ln.start; i < ln.end; i++ {
		it := &p.items[i]
		if it.kind != itemFlow {
			continue
		}
		cs := it.crossSizes
		if !it.collapsed {
			if cs.definite {
				it.cross = cs.clamp(cs.size)
			} else {
				it.cross = measureOuter(it.w, f.cross, it.main, cb).Nat
			}
		}
		ln.cross = max(ln.cross, it.cross)
		if f.main == Row && !it.collapsed && (it.align.Kind == style.BoxBaseline || it.align.Kind == style.BoxLastBaseline) {
			if b, ok := baselineIn(it); ok {
				it.baseline, it.hasBaseline = b, true
				ln.above, ln.below = max(ln.above, b), max(ln.below, it.cross-b)
			}
		}
	}
	ln.cross = max(ln.cross, ln.above+ln.below)
	if !f.wrap && crossDef {
		ln.cross = crossSize
	}
}

// flexLinesPlace distributes the lines across the container
// (align-content, §9.4 and §9.6): with a definite cross size the free
// space stretches the lines (normal and stretch) or spaces them; the
// lines stack from cross start, the far edge under wrap-reverse. It
// returns the cross extent the lines span.
func (p *cssPass) flexLinesPlace(lines []flexLine, f flexAxes, crossSize int, crossDef bool) int {
	total := f.gapCross * (len(lines) - 1)
	for _, ln := range lines {
		total += ln.cross
	}
	if !crossDef {
		crossSize = total
	}
	free := float64(crossSize - total)
	align := style.BoxAlign{Kind: style.BoxNormal}
	if p.v.Has(style.PropAlignContent) {
		align = p.v.Layout.AlignContent
	}
	var offset, between float64
	if f.wrap {
		switch align.Kind {
		case style.BoxNormal, style.BoxStretch:
			if free > 0 {
				each, rest := int(free)/len(lines), int(free)%len(lines)
				for li := range lines {
					lines[li].cross += each + btoi(li < rest)
				}
			}
		case style.BoxBaseline, style.BoxLastBaseline:
		default:
			offset, between = distribute(align, free, len(lines), !f.wrapReverse, true)
		}
	}
	pos := offset
	for li := range lines {
		ln := &lines[li]
		ln.pos = int(math.Round(pos))
		if f.crossFlip {
			ln.pos = crossSize - ln.pos - ln.cross
		}
		pos += float64(ln.cross+f.gapCross) + between
	}
	return crossSize
}

// flexCrossPlace aligns one line's items across it (§9.6): stretch
// fills an auto cross size to the line within the min/max bounds, auto
// margins split the leftover, and align-self places the rest — start
// and end by the writing mode, flex-start and flex-end by the line's
// cross start, baseline by the line's baseline group.
func (p *cssPass) flexCrossPlace(ln *flexLine, f flexAxes) {
	for i := ln.start; i < ln.end; i++ {
		it := &p.items[i]
		if it.kind != itemFlow {
			continue
		}
		cs := it.crossSizes
		autoS, autoE := autoMargins(it, f.cross)
		if it.align.Kind == style.BoxStretch && !cs.definite && !autoS && !autoE && !it.collapsed {
			it.cross = cs.clamp(ln.cross)
		}
		free := ln.cross - it.cross
		off := 0
		switch {
		case (autoS || autoE) && free > 0:
			switch {
			case autoS && autoE:
				off = free / 2
			case autoS:
				off = free
			}
		case it.align.Safe && free < 0:
		default:
			switch it.align.Kind {
			case style.BoxCenter:
				off = free / 2
			case style.BoxFlexEnd:
				off = free
			case style.BoxStart, style.BoxSelfStart:
				if f.wrapReverse {
					off = free
				}
			case style.BoxEnd, style.BoxSelfEnd:
				if !f.wrapReverse {
					off = free
				}
			case style.BoxBaseline, style.BoxLastBaseline:
				if it.hasBaseline {
					off = ln.above - it.baseline
				}
			}
		}
		it.crossPos = ln.pos + off
		if f.crossFlip {
			it.crossPos = ln.pos + ln.cross - off - it.cross
		}
	}
}
