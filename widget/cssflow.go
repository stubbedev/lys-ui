package widget

// The CSS layout of a Box (docs/css.md). A declared display hands the
// box's children to a CSS formatting context in place of the
// programmatic box layout: flow for block, inline, inline-block and
// flow-root, flexbox for flex and inline-flex (cssflex.go), grid for
// grid and inline-grid (cssgrid.go). A
// display:contents child box dissolves into the pass, its children
// laid out as the box's own while staying parented to it. The
// interaction rule: where the stylesheet sets a layout property, CSS
// wins; where it does not, the programmatic setting stands in — the box
// axis for flex-direction, its spacing for gap, a child's expand for
// flex-grow, AppendAligned for align-self.
//
// Every widget is one atomic box to the pass, and every Box laid out by
// CSS roots its own block formatting context (display: block reads as
// flow-root): floats never escape it and count toward its height, and
// margins collapse between adjacent block siblings only, never through
// a parent.

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// layoutMode is how a Box lays out its children.
type layoutMode uint8

const (
	// modeBox is the programmatic box layout.
	modeBox layoutMode = iota
	// modeFlow is CSS normal flow.
	modeFlow
	// modeFlex is CSS flexbox.
	modeFlex
	// modeGrid is CSS grid.
	modeGrid
)

// layoutModeOf maps a Box's computed display to its layout. Without a
// display declaration the programmatic layout runs; contents boxes are
// dissolved by their parent's pass and none boxes are not laid out at
// all, so neither picks a CSS mode of its own.
func layoutModeOf(v *style.Values) layoutMode {
	if !v.Has(style.PropDisplay) {
		return modeBox
	}
	switch v.Layout.Display {
	case style.DisplayFlex, style.DisplayInlineFlex:
		return modeFlex
	case style.DisplayGrid, style.DisplayInlineGrid:
		return modeGrid
	case style.DisplayContents, style.DisplayNone:
		return modeBox
	}
	return modeFlow
}

// displayContents reports a declared display: contents.
func displayContents(v *style.Values) bool {
	return v.Has(style.PropDisplay) && v.Layout.Display == style.DisplayContents
}

// contentsHost is a container that dissolves into its parent's CSS
// pass under display: contents: a Box, and the types embedding one.
type contentsHost interface {
	contentsBox() *Box
}

func (b *Box) contentsBox() *Box { return b }

// flows reports whether a container's child w takes part in its flow:
// visible and not out of flow.
func flows(w Widget) bool { return ownVisible(w) && !outOfFlow(w) }

// cssMeasure is the box's content request along axis at forSize by
// its CSS layout. Every extent a measure is given is definite; none
// else is, so percentages of the height read as auto.
func (b *Box) cssMeasure(v *style.Values, mode layoutMode, axis Axis, forSize int) Sizes {
	p := b.beginPass(v, mode, true)
	defer p.end()
	var req Sizes
	switch mode {
	case modeFlex:
		req = p.flexMeasure(axis, forSize)
	case modeGrid:
		req = p.gridMeasure(axis, forSize)
	default:
		req = p.flowMeasure(axis, forSize)
	}
	b.cssBase, b.cssHasBase = p.base, p.hasBase
	return req
}

// flowMeasure is normal flow's request: across, the widest item's
// minimum and the run's natural width; down, the run's height at the
// width given, or at its natural width.
func (p *cssPass) flowMeasure(axis Axis, forSize int) Sizes {
	if axis == Column {
		width := forSize
		if width == Unknown {
			width = indefinite
		}
		return FixedSizes(p.flow(width, indefinite).H)
	}
	least := 0
	for i := range p.items {
		if it := &p.items[i]; it.kind == itemFlow {
			least = max(least, measureOuter(it.w, Row, Unknown, indefiniteSize).Min)
		}
	}
	return Sizes{Min: least, Nat: max(least, p.flow(indefinite, indefinite).W)}
}

// indefiniteSize is a containing block with neither extent definite.
var indefiniteSize = Size{W: indefinite, H: indefinite}

// definiteOr is n when def holds, else indefinite.
func definiteOr(n int, def bool) int {
	if def {
		return n
	}
	return indefinite
}

// cssArrange lays the box's children out in its content box inner by
// its CSS layout. The content box's height is definite when the parent
// pass sized it (cssDefH) or the box's own height says so.
func (b *Box) cssArrange(v *style.Values, mode layoutMode, inner render.Rect) {
	p := b.beginPass(v, mode, false)
	defer p.end()
	defH := b.cssDefH || resolveAxis(b, v, Column, indefinite).definite
	b.cssDefH = false
	switch mode {
	case modeFlex:
		p.flex(Size{W: inner.W, H: inner.H}, true, true)
	case modeGrid:
		p.grid(Size{W: inner.W, H: inner.H}, [2]gridSizing{sizeDefinite, sizeDefinite})
	default:
		p.height = definiteOr(inner.H, defH)
		p.flow(inner.W, p.height)
	}
	p.arrange(Point{X: inner.X, Y: inner.Y})
	b.cssBase, b.cssHasBase = p.base, p.hasBase
}

// plainValues is the empty cascade a widget without a node reads.
var plainValues style.Values

// styleOf is w's computed style, the empty cascade for a widget
// without a node.
func styleOf(w Widget) *style.Values {
	if n := nodeOf(w); n != nil {
		return n.style(w)
	}
	return &plainValues
}

// itemKind is what a pass does with one collected child.
type itemKind uint8

const (
	// itemFlow takes part in the layout.
	itemFlow itemKind = iota
	// itemGone is hidden (SetVisible or display: none): arranged empty.
	itemGone
	// itemOut is absolutely or fixed positioned: placed at its static
	// position after the layout.
	itemOut
	// itemContents is a display:contents box: its children follow it
	// in the pass, and its bounds become their union.
	itemContents
)

// cssItem is one widget a CSS layout pass places: a child of the box,
// or a child of a display:contents box between, which stays parented
// to that box. rect is its margin box, relative to the content box.
type cssItem struct {
	w     Widget
	host  *Box
	entry *childEntry
	v     *style.Values
	kind  itemKind
	nat   Size
	rect  render.Rect
	// static is an out-of-flow item's static position, relative to
	// the content box.
	static Point

	// The flow scratch: the float side (-1 left, 1 right), whether a
	// float is placed yet, and whether it waits for the line to end.
	floatSide int8
	placed    bool
	pending   bool

	// The flex scratch (cssflex.go).
	flexItem
	// grid is the grid scratch (cssgrid.go).
	grid gridItem
}

// cssPass is one CSS layout run over a Box's children; measure and
// arrange share it, arrange placing what measure only sizes.
type cssPass struct {
	b         *Box
	v         *style.Values
	mode      layoutMode
	items     []cssItem
	rtl       bool
	measuring bool
	// height is a flow's content box height when definite, the basis
	// of its items' percentage heights; indefinite otherwise.
	height int
	// base is the run's first baseline, relative to the content box.
	base    int
	hasBase bool
}

// cssItems are the passes' item buffers, one per nesting level.
var cssItems depthStack[cssItem]

// beginPass collects the box's children for a pass; the caller ends it.
func (b *Box) beginPass(v *style.Values, mode layoutMode, measuring bool) cssPass {
	p := cssPass{b: b, v: v, mode: mode, measuring: measuring, rtl: b.rtl(v), height: indefinite}
	p.items = cssItems.push(countItems(b))
	n := 0
	p.collect(b, &n)
	return p
}

// end releases the pass's buffer.
func (p *cssPass) end() { cssItems.pop() }

// dissolves reports whether c dissolves into its parent's pass.
func dissolves(c Widget) (*Box, bool) {
	h, ok := c.(contentsHost)
	if !ok || !ownVisible(c) || !displayContents(styleOf(c)) {
		return nil, false
	}
	return h.contentsBox(), true
}

// countItems counts the items a pass over b collects, parenting the
// children so their cascades resolve under b.
func countItems(b *Box) int {
	n := len(b.child)
	for _, c := range b.child {
		setParents(b, c.w)
		if inner, ok := dissolves(c.w); ok {
			n += countItems(inner)
		}
	}
	return n
}

// collect fills the items from host's children, a display:contents
// child's own children following it.
func (p *cssPass) collect(host *Box, n *int) {
	for _, c := range host.child {
		it := &p.items[*n]
		*n++
		*it = cssItem{w: c.w, host: host, entry: c, v: styleOf(c.w)}
		switch {
		case !ownVisible(c.w):
			it.kind = itemGone
		case it.v.Has(style.PropPosition) && it.v.Layout.Position.OutOfFlow():
			it.kind = itemOut
		default:
			if inner, ok := dissolves(c.w); ok {
				it.kind = itemContents
				p.collect(inner, n)
			}
		}
	}
}

// rtl reports whether the box's inline axis runs right to left: the
// CSS direction where declared, the programmatic one otherwise.
func (b *Box) rtl(v *style.Values) bool {
	if v.Has(style.PropDirection) {
		return v.Direction == 2
	}
	return b.dir == DirectionRTL
}

// arrange arranges every item at origin, the content box's corner:
// flow items in their rects, hidden ones empty, out-of-flow ones
// against their containing blocks (a grid item's grid area, where the
// grid resolved one) from their static positions, and
// each display:contents box around what it holds.
func (p *cssPass) arrange(origin Point) {
	for i := range p.items {
		it := &p.items[i]
		switch it.kind {
		case itemGone:
			it.w.Arrange(render.Rect{})
		case itemFlow:
			r := it.rect
			r.X += origin.X
			r.Y += origin.Y
			if h, ok := it.w.(contentsHost); ok {
				h.contentsBox().cssDefH = p.mode != modeFlow || resolveAxis(it.w, it.v, Column, p.height).definite
			}
			arrangeAt(it.w, it.v, layoutMarginOf(it.w).Shrink(r))
		case itemOut:
			cb := ownContainingBlock(it.w, it.v)
			if it.grid.hasCB {
				cb = it.grid.cb
				cb.X += origin.X
				cb.Y += origin.Y
			}
			placeOutOfFlow(it.w, it.v, Point{X: origin.X + it.static.X, Y: origin.Y + it.static.Y}, cb)
		}
		setParents(it.host, it.w)
	}
	// Inner contents boxes come later in the pass: union them first.
	for i := len(p.items) - 1; i >= 0; i-- {
		if it := &p.items[i]; it.kind == itemContents {
			inner, _ := it.w.(contentsHost)
			box := inner.contentsBox()
			var u render.Rect
			for _, c := range box.child {
				if r := boundsOf(c.w); !r.Empty() {
					u = unionRect(u, r)
				}
			}
			box.node.Arrange(u)
		}
	}
}

// unionRect is the smallest rect holding a and b, an empty a holding
// nothing.
func unionRect(a, b render.Rect) render.Rect {
	if a.Empty() {
		return b
	}
	return a.Union(b)
}

// marginsAlong is an item's margins on a's two sides, start then end,
// the stylesheet's and the programmatic ones.
func marginsAlong(it *cssItem, a Axis) (start, end int) {
	lm := layoutMarginOf(it.w)
	m := it.v.Margin
	if a == Row {
		return m.Left + lm.Left, m.Right + lm.Right
	}
	return m.Top + lm.Top, m.Bottom + lm.Bottom
}

// autoMargins reports an item's auto margins on a's start and end.
func autoMargins(it *cssItem, a Axis) (start, end bool) {
	if a == Row {
		return it.v.MarginAuto[3], it.v.MarginAuto[1]
	}
	return it.v.MarginAuto[0], it.v.MarginAuto[2]
}

// baselineIn is an item's baseline below its margin box top.
func baselineIn(it *cssItem) (int, bool) {
	b, ok := baselineOf(it.w)
	return b + layoutMarginOf(it.w).Top, ok
}

// collapseMargins is the CSS adjoining-margin rule: the largest
// positive margin plus the most negative one.
func collapseMargins(a, b int) int {
	switch {
	case a >= 0 && b >= 0:
		return max(a, b)
	case a < 0 && b < 0:
		return min(a, b)
	}
	return a + b
}

// inlineLevel reports an item that sits on line boxes.
func inlineLevel(it *cssItem) bool {
	return it.v.Has(style.PropDisplay) && it.v.Layout.Display.Inline()
}

// floatSideOf resolves an item's float to a physical side: -1 left, 1
// right, 0 for none.
func floatSideOf(it *cssItem, rtl bool) int8 {
	if !it.v.Has(style.PropFloat) {
		return 0
	}
	switch it.v.Layout.Float {
	case style.FloatLeft:
		return -1
	case style.FloatRight:
		return 1
	case style.FloatInlineStart:
		if rtl {
			return 1
		}
		return -1
	case style.FloatInlineEnd:
		if rtl {
			return -1
		}
		return 1
	}
	return 0
}

// clearsSide reports whether an item's clear moves it below floats on
// side.
func clearsSide(it *cssItem, side int8, rtl bool) bool {
	if !it.v.Has(style.PropClear) {
		return false
	}
	switch it.v.Layout.Clear {
	case style.ClearBoth:
		return true
	case style.ClearLeft:
		return side < 0
	case style.ClearRight:
		return side > 0
	case style.ClearInlineStart:
		return side < 0 != rtl
	case style.ClearInlineEnd:
		return side > 0 != rtl
	}
	return false
}

// flowState is the running state of one normal-flow layout.
type flowState struct {
	p     *cssPass
	width int
	// height is the content box's height when it is definite, the
	// basis of the items' percentage heights; indefinite otherwise.
	height int
	y      int
	// prevMB is the bottom margin of the last block, which the next
	// block's top margin collapses with while havePrev holds.
	prevMB   int
	havePrev bool
	// floatTop is the highest a later float may start: no float sits
	// above an earlier one.
	floatTop int
	// extent is the widest the content reaches: the natural width.
	extent int
	line   flowLine
}

// flowLine is the line box being filled: its first item, top edge,
// filled width and tallest item so far, and how many items it holds.
type flowLine struct {
	start, y, w, h, n int
}

// flow lays the items out in normal flow at content width width
// (indefinite while measuring under an unbounded width: the natural
// width is found first). Block-level items stack, each as wide as the
// band between the floats beside it unless its width says otherwise,
// with auto margins centering or pushing it; adjacent block margins
// collapse. Inline-level items fill line boxes, aligned by text-align
// and on their baselines by vertical-align. Floats take the side they
// name as high as the earlier content allows, and clear moves an item
// below the floats it names. It returns the content size the run
// needs.
func (p *cssPass) flow(width, height int) Size {
	if !finite(width) {
		width = p.flowNaturalWidth()
	}
	f := flowState{p: p, width: width, height: height}
	for i := range p.items {
		it := &p.items[i]
		switch {
		case it.kind == itemOut:
			it.static = Point{Y: f.y}
			if f.line.n > 0 {
				it.static = Point{X: f.line.w, Y: f.line.y}
			}
			if p.rtl {
				it.static.X = width - it.static.X
			}
		case it.kind != itemFlow:
		case floatSideOf(it, p.rtl) != 0:
			it.floatSide = floatSideOf(it, p.rtl)
			f.float(it)
		case inlineLevel(it):
			f.inline(i)
		default:
			f.block(i)
		}
	}
	f.flushLine(len(p.items))
	return Size{W: f.extent, H: max(f.y, f.clearY(nil))}
}

// flowNaturalWidth is the width the flow wants with no limit: the
// widest block, or the widest unbroken run of inline items between
// blocks, beside every float.
func (p *cssPass) flowNaturalWidth() int {
	widest, run, floats := 0, 0, 0
	for i := range p.items {
		it := &p.items[i]
		if it.kind != itemFlow {
			continue
		}
		w := measureOuter(it.w, Row, Unknown, indefiniteSize).Nat
		switch {
		case floatSideOf(it, p.rtl) != 0:
			floats += w
		case inlineLevel(it):
			run += w
		default:
			widest = max(widest, run, w)
			run = 0
		}
	}
	return max(widest, run) + floats
}

// band is the stretch of the line between the floats overlapping the
// rows top through bottom.
func (f *flowState) band(top, bottom int) (left, right int) {
	left, right = 0, f.width
	bottom = max(bottom, top+1)
	for i := range f.p.items {
		it := &f.p.items[i]
		if !it.placed || it.rect.Y >= bottom || it.rect.Y+it.rect.H <= top {
			continue
		}
		if it.floatSide < 0 {
			left = max(left, it.rect.X+it.rect.W)
		} else {
			right = min(right, it.rect.X)
		}
	}
	return left, right
}

// nextFloatEnd is the first bottom edge below top of the floats
// overlapping the rows top through bottom, or -1 when none overlaps.
func (f *flowState) nextFloatEnd(top, bottom int) int {
	next := -1
	bottom = max(bottom, top+1)
	for i := range f.p.items {
		it := &f.p.items[i]
		end := it.rect.Y + it.rect.H
		if !it.placed || it.rect.Y >= bottom || end <= top {
			continue
		}
		if next < 0 || end < next {
			next = end
		}
	}
	return next
}

// fit finds the first top at or below top where an item w wide and h
// tall fits between the floats, and the band there.
func (f *flowState) fit(top, w, h int) (y, left, right int) {
	for {
		left, right = f.band(top, top+h)
		if right-left >= w {
			return top, left, right
		}
		next := f.nextFloatEnd(top, top+h)
		if next < 0 {
			return top, left, right
		}
		top = next
	}
}

// clearY is the bottom of the floats it clears, every float for nil.
func (f *flowState) clearY(it *cssItem) int {
	y := 0
	for i := range f.p.items {
		fl := &f.p.items[i]
		if fl.placed && (it == nil || clearsSide(it, fl.floatSide, f.p.rtl)) {
			y = max(y, fl.rect.Y+fl.rect.H)
		}
	}
	return y
}

// block places one block-level item.
func (f *flowState) block(i int) {
	it := &f.p.items[i]
	f.flushLine(i)
	if y := f.clearY(it); y > f.y {
		f.y, f.havePrev = y, false
	}
	mt, mb := marginsAlong(it, Column)
	top := f.y
	if f.havePrev {
		top -= f.prevMB + mt - collapseMargins(f.prevMB, mt)
	}
	ws := resolveAxis(it.w, it.v, Row, f.width)
	need := ws.min
	if ws.definite {
		need = ws.clamp(ws.size)
	}
	top, left, right := f.fit(top, need, 0)
	band := right - left
	w := ws.clamp(band)
	if ws.definite {
		w = ws.clamp(ws.size)
	}
	x := left
	autoL, autoR := autoMargins(it, Row)
	switch free := band - w; {
	case free > 0 && autoL && autoR:
		x += free / 2
	case free > 0 && autoL:
		x += free
	case !autoL && !autoR && f.p.rtl:
		x += free
	}
	h := measureOuter(it.w, Column, w, f.basis()).Nat
	it.rect = render.Rect{X: x, Y: top, W: w, H: h}
	natW := w
	if f.p.measuring && !ws.definite {
		natW = measureOuter(it.w, Row, Unknown, f.basis()).Within(band)
	}
	it.nat = Size{W: natW, H: h}
	f.extent = max(f.extent, left+natW+f.width-right)
	if b, ok := baselineIn(it); ok && !f.p.hasBase {
		f.p.base, f.p.hasBase = top+b, true
	}
	f.y = top + h
	f.prevMB, f.havePrev = mb, true
}

// basis is the containing block the items' percentages resolve
// against: the content box's width, and its height when definite (CSS
// 2 §10.5); against an indefinite height they read as auto.
func (f *flowState) basis() Size { return Size{W: f.width, H: f.height} }

// shrinkToFit is an item's size at the line's width: its natural width
// held within it, and its natural height there.
func (f *flowState) shrinkToFit(it *cssItem) Size {
	w := measureOuter(it.w, Row, Unknown, f.basis()).Within(f.width)
	return Size{W: w, H: measureOuter(it.w, Column, w, f.basis()).Nat}
}

// inline adds one inline-level item to the line, ending the line first
// when the item does not fit beside what it holds.
func (f *flowState) inline(i int) {
	it := &f.p.items[i]
	it.nat = f.shrinkToFit(it)
	if f.line.n > 0 {
		left, right := f.band(f.line.y, f.line.y+max(f.line.h, it.nat.H))
		if f.line.w+it.nat.W > right-left {
			f.flushLine(i)
		}
	}
	if f.line.n == 0 {
		top, _, _ := f.fit(f.y, it.nat.W, it.nat.H)
		f.line = flowLine{start: i, y: top}
	}
	f.line.w += it.nat.W
	f.line.h = max(f.line.h, it.nat.H)
	f.line.n++
}

// float places one float: beside the line in progress when it fits
// there, after the line otherwise.
func (f *flowState) float(it *cssItem) {
	it.nat = f.shrinkToFit(it)
	if f.line.n == 0 {
		f.placeFloat(it, f.y)
		return
	}
	left, right := f.band(f.line.y, f.line.y+max(f.line.h, it.nat.H))
	if right-left-f.line.w >= it.nat.W {
		f.placeFloat(it, f.line.y)
		return
	}
	it.pending = true
}

// placeFloat puts a float as high as top, its clearance and the
// earlier floats allow, at the band's edge on its side.
func (f *flowState) placeFloat(it *cssItem, top int) {
	top = max(top, f.floatTop)
	if it.v.Has(style.PropClear) {
		top = max(top, f.clearY(it))
	}
	top, left, right := f.fit(top, it.nat.W, it.nat.H)
	x := left
	if it.floatSide > 0 {
		x = right - it.nat.W
	}
	it.rect = render.Rect{X: x, Y: top, W: it.nat.W, H: it.nat.H}
	it.placed = true
	f.floatTop = top
	if it.floatSide < 0 {
		f.extent = max(f.extent, x+it.nat.W)
	} else {
		f.extent = max(f.extent, f.width-x)
	}
}

// lineAlign is an inline item's vertical-align on its line.
type lineAlign uint8

const (
	onBaseline lineAlign = iota
	onTop
	onMiddle
	onBottom
)

// lineAlignOf maps vertical-align onto the line placement: the baseline
// family (sub, super and lengths shift the widget's own reported
// baseline) or the line's top, middle or bottom.
func lineAlignOf(it *cssItem) lineAlign {
	if !it.v.Has(style.PropVerticalAlign) {
		return onBaseline
	}
	switch it.v.VerticalAlign {
	case style.VerticalAlignTop, style.VerticalAlignTextTop:
		return onTop
	case style.VerticalAlignMiddle:
		return onMiddle
	case style.VerticalAlignBottom, style.VerticalAlignTextBottom:
		return onBottom
	}
	return onBaseline
}

// lineBaselineOf is where an inline item's baseline sits below its top:
// its own baseline, or its bottom margin edge when it has none (CSS's
// inline-block without a line box).
func lineBaselineOf(it *cssItem) int {
	if b, ok := baselineIn(it); ok {
		return b
	}
	return it.nat.H
}

// textAlignOffset is where a line's content starts in its band, free
// being the band's room left over.
func textAlignOffset(v *style.Values, rtl bool, free int) int {
	align := style.AlignStart
	if v.Has(style.PropTextAlign) {
		align = v.TextAlign
	}
	switch align {
	case style.AlignEnd:
		if rtl {
			return 0
		}
		return free
	case style.AlignRight:
		return free
	case style.AlignCenter:
		return free / 2
	case style.AlignLeft:
		return 0
	}
	if rtl {
		return free
	}
	return 0
}

// flushLine places the line's inline items, ending at item end, and
// the floats that waited for it.
func (f *flowState) flushLine(end int) {
	if f.line.n == 0 {
		return
	}
	items := f.p.items[f.line.start:end]
	inLine := func(it *cssItem) bool { return it.kind == itemFlow && it.floatSide == 0 && inlineLevel(it) }
	above, below, tall := 0, 0, 0
	for k := range items {
		it := &items[k]
		if !inLine(it) {
			continue
		}
		if lineAlignOf(it) == onBaseline {
			b := lineBaselineOf(it)
			above, below = max(above, b), max(below, it.nat.H-b)
		} else {
			tall = max(tall, it.nat.H)
		}
	}
	h := max(above+below, tall)
	left, right := f.band(f.line.y, f.line.y+h)
	x := left + textAlignOffset(f.p.v, f.p.rtl, right-left-f.line.w)
	run := 0
	for k := range items {
		it := &items[k]
		if !inLine(it) {
			continue
		}
		ix := x + run
		if f.p.rtl {
			ix = x + f.line.w - run - it.nat.W
		}
		iy := f.line.y
		switch lineAlignOf(it) {
		case onBaseline:
			iy += above - lineBaselineOf(it)
		case onMiddle:
			iy += (h - it.nat.H) / 2
		case onBottom:
			iy += h - it.nat.H
		}
		it.rect = render.Rect{X: ix, Y: iy, W: it.nat.W, H: it.nat.H}
		run += it.nat.W
	}
	f.extent = max(f.extent, left+f.line.w+f.width-right)
	if above > 0 && !f.p.hasBase {
		f.p.base, f.p.hasBase = f.line.y+above, true
	}
	f.y = f.line.y + h
	f.havePrev = false
	f.line = flowLine{}
	for k := range items {
		if it := &items[k]; it.pending {
			it.pending = false
			f.placeFloat(it, f.y)
		}
	}
}
