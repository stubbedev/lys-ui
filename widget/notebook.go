package widget

import (
	"math"
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Notebook is a tabbed container: a tab bar on top and exactly one
// visible page beneath it, the GtkNotebook model. Tabs carry a name,
// painted as the tab's label unless SetTabLabel gave it a widget; the
// header, the tab row, and each tab are CSS boxes. Selecting a tab
// shows its page and fires OnSelect. With Closable
// set, each tab paints a close box and a click on it fires OnTabClose
// instead of selecting - hosts remove the page themselves. With
// Reorderable set, dragging a tab along the strip moves it (OnReorder
// reports the new index). Tabs that overflow the strip scroll: arrows
// appear at its ends, the wheel over it scrolls, and the selected tab
// is kept in view.
type Notebook struct {
	node
	face     render.Font
	tabs     []notebookTab
	selected int
	header   notebookHeader
	strip    notebookStrip

	// scroll is the tab strip's horizontal offset; arrows are the
	// overflow chevrons; overHeader tracks the pointer for the wheel.
	scroll     int
	arrows     [2]*Symbol
	overHeader bool
	// revealing asks the next arranged frame to bring the selected tab
	// into view (a selection before the first arrange has no strip).
	revealing bool
	// drag is a tab reorder in flight: the dragged tab's index.
	drag    dragGesture
	dragTab int

	OnSelect    func(name string)
	OnTabClose  func(name string)
	OnReorder   func(name string, index int)
	Closable    bool
	Reorderable bool
}

// The tab bar's metrics: an overflow arrow's width, a closable tab's
// close box at its right edge, the painted name's size, and the floor
// of a tab whose stylesheet sets no min-width or min-height (the arrows
// and the wheel also step by tabWidth).
const (
	tabArrowW    = 20
	tabCloseW    = 20
	tabTextPx    = 14
	tabWidth     = 96
	tabBarHeight = 26
)

// notebookHeader is the tab bar's node tree (`notebook > header >
// tabs > tab`): the header strip, the tab row, one tab part per page
// (its :checked marks the selected one).
type notebookHeader struct {
	stylePart
	tabs notebookTabs
}

type notebookTabs struct {
	stylePart
	tab []*notebookTabPart
}

func (t *notebookTabs) appendStyleKids(buf []Widget) []Widget {
	for _, p := range t.tab {
		buf = append(buf, p)
	}
	return buf
}

// notebookTabPart is one tab's node: its label widget when the host
// set one, and the pointer's hover over it.
type notebookTabPart struct {
	stylePart
	label   Widget
	hovered bool
}

func (t *notebookTabPart) appendStyleKids(buf []Widget) []Widget {
	if t.label != nil {
		buf = append(buf, t.label)
	}
	return buf
}

// setHovered moves the part's :hover.
func (t *notebookTabPart) setHovered(on bool) {
	if t.hovered != on {
		t.hovered = on
		t.invalidateState(style.Hover)
	}
}

type notebookTab struct {
	name string
	w    Widget
	part *notebookTabPart
	// expand shares the strip's leftover width with the other expanding
	// tabs (GTK's tab-expand).
	expand bool
}

// notebookStrip is the arranged tab bar: the header's and the tab
// row's border boxes, the row's content box the tabs scroll in, the
// bar's height above the pages, and each tab's margin box as an
// offset and width along the unscrolled row.
type notebookStrip struct {
	header, tabs, row render.Rect
	height            int
	tabX, tabW        []int
	total             int
	// natural is the strip's natural width: the tabs and the boxes
	// around them.
	natural int
}

// NewNotebook returns an empty notebook with tab labels painted in the
// given face; a render.Chain adds mixed-script fallback. A nil face
// panics here (see requireFace) instead of failing later, in shaping.
func NewNotebook(face render.Font) *Notebook {
	n := &Notebook{face: requireFace("widget.NewNotebook", face)}
	n.arrows = [2]*Symbol{NewSymbol(SymbolChevronLeft, 14), NewSymbol(SymbolChevronRight, 14)}
	n.header.SetElement("header")
	n.header.tabs.SetElement("tabs")
	n.syncTabs()
	return n
}

// syncTabs lists the tab parts in page order, links the header chain,
// down to the tab labels, below the notebook, and restyles the tabs
// for their new positions.
func (n *Notebook) syncTabs() {
	n.header.tabs.tab = n.header.tabs.tab[:0]
	for _, t := range n.tabs {
		n.header.tabs.tab = append(n.header.tabs.tab, t.part)
		if t.part.label != nil {
			setParents(t.part, t.part.label)
		}
	}
	setParents(n, &n.header)
	setParents(&n.header, &n.header.tabs)
	setParents(&n.header.tabs, styleKids(&n.header.tabs)...)
	restyleChildren(&n.header.tabs)
	n.syncSelectedTab()
}

// syncSelectedTab mirrors the selection into the tab parts' :checked.
func (n *Notebook) syncSelectedTab() {
	for i, p := range n.header.tabs.tab {
		p.SetState(StateChecked, i == n.selected)
	}
}

// appendStyleKids is the header and the visible page (styleKids).
func (n *Notebook) appendStyleKids(buf []Widget) []Widget {
	buf = append(buf, &n.header)
	if n.selected < len(n.tabs) {
		buf = append(buf, n.tabs[n.selected].w)
	}
	return buf
}

// AppendTab adds a page under name; the first page added becomes the
// selected one.
func (n *Notebook) AppendTab(name string, w Widget) { n.InsertTab(len(n.tabs), name, w) }

// InsertTab adds a page under name at index i (clamped); the selected
// page stays selected.
func (n *Notebook) InsertTab(i int, name string, w Widget) {
	i = min(max(i, 0), len(n.tabs))
	if len(n.tabs) > 0 && i <= n.selected {
		n.selected++
	}
	part := &notebookTabPart{}
	part.SetElement("tab")
	n.tabs = slices.Insert(n.tabs, i, notebookTab{name: name, w: w, part: part})
	n.syncTabs()
	n.InvalidateLayout()
}

// SetTabLabel shows w in the tab of the page under name instead of its
// painted name (gtk_notebook_set_tab_label); nil restores the name.
// The label styles as the tab's child (`tab > label`) and fills the
// tab's content box. Unknown names are a no-op.
func (n *Notebook) SetTabLabel(name string, w Widget) {
	i := n.tabIndex(name)
	if i < 0 {
		return
	}
	part := n.tabs[i].part
	if part.label != nil {
		notifyRemoved(part.label)
		clearParents(part.label)
	}
	part.label = w
	n.syncTabs()
	n.InvalidateLayout()
}

// TabLabel is the label widget SetTabLabel gave the page under name
// (gtk_notebook_get_tab_label); nil for a painted name or an unknown
// page.
func (n *Notebook) TabLabel(name string) Widget {
	i := n.tabIndex(name)
	if i < 0 {
		return nil
	}
	return n.tabs[i].part.label
}

// stripRange is the tab strip as a scroll range: the offset into the
// tabs' total width, of which the viewport between the arrows shows.
func (n *Notebook) stripRange() ScrollRange {
	total, page := n.strip.total, n.strip.row.W
	if total > page {
		page -= 2 * tabArrowW
	}
	return ScrollRange{Offset: n.scroll, Page: max(page, 0), Total: total}
}

// tabViewport is the strip area tabs paint in: inside the arrows while
// they overflow.
func (n *Notebook) tabViewport() render.Rect {
	r := n.strip.row
	if n.stripRange().Overflows() {
		r.X += tabArrowW
		r.W -= 2 * tabArrowW
	}
	return r
}

// scrollStrip moves the strip to offset, clamped.
func (n *Notebook) scrollStrip(offset int) {
	offset = min(max(offset, 0), n.stripRange().Max())
	if offset != n.scroll {
		n.scroll = offset
		n.Invalidate()
	}
}

// applyReveal brings the selected tab into view once there is a strip.
func (n *Notebook) applyReveal() {
	if n.revealing && n.bounds.W > 0 && n.selected < len(n.tabs) {
		n.revealing = false
		n.revealTab(n.selected)
	}
}

// revealTab scrolls the strip so tab i is whole in view.
func (n *Notebook) revealTab(i int) {
	vp, rect := n.tabViewport(), n.tabRect(i)
	switch {
	case rect.X < vp.X:
		n.scrollStrip(n.scroll - (vp.X - rect.X))
	case rect.X+rect.W > vp.X+vp.W:
		n.scrollStrip(n.scroll + rect.X + rect.W - vp.X - vp.W)
	}
}

// MoveTab moves tab from to index to, the selection following its
// page. It is the programmatic move: OnReorder reports only the user's
// drags, so a handler that mirrors them into a model cannot loop.
func (n *Notebook) MoveTab(from, to int) { n.moveTab(from, to, false) }

func (n *Notebook) moveTab(from, to int, dragged bool) {
	if from == to || from < 0 || to < 0 || from >= len(n.tabs) || to >= len(n.tabs) {
		return
	}
	selected := n.tabs[n.selected].name
	t := n.tabs[from]
	n.tabs = append(n.tabs[:from], n.tabs[from+1:]...)
	n.tabs = append(n.tabs[:to], append([]notebookTab{t}, n.tabs[to:]...)...)
	for i, k := range n.tabs {
		if k.name == selected {
			n.selected = i
		}
	}
	n.syncTabs()
	n.InvalidateLayout()
	if dragged && n.OnReorder != nil {
		n.OnReorder(t.name, to)
	}
}

// SelectedTab returns the visible page's name, empty when none.
func (n *Notebook) SelectedTab() string {
	if n.selected < len(n.tabs) {
		return n.tabs[n.selected].name
	}
	return ""
}

// SelectTab shows the page under name; unknown names are a no-op.
func (n *Notebook) SelectTab(name string) {
	for i, t := range n.tabs {
		if t.name == name {
			n.selectIndex(i)
			return
		}
	}
}

// selectIndex switches pages (revealing the tab), invalidates the
// notebook, and fires OnSelect.
func (n *Notebook) selectIndex(i int) {
	changed := i != n.selected
	n.selected = i
	n.revealing = true
	n.applyReveal()
	if !changed {
		return
	}
	n.syncSelectedTab()
	n.Invalidate()
	if n.OnSelect != nil {
		n.OnSelect(n.tabs[i].name)
	}
}

// CloseTab removes the page under name; the following page becomes
// visible. Unknown names are a no-op. Reports whether a page was
// removed. The closed page detaches like every tree mutation: parent
// link cleared, removal hook fired.
func (n *Notebook) CloseTab(name string) bool {
	i := n.tabIndex(name)
	if i < 0 {
		return false
	}
	n.CloseTabAt(i)
	return true
}

// CloseTabAt removes the page at index i, like CloseTab; an index out
// of range is a no-op.
func (n *Notebook) CloseTabAt(i int) {
	if i < 0 || i >= len(n.tabs) {
		return
	}
	t := n.tabs[i]
	notifyRemoved(t.w)
	if t.part.label != nil {
		notifyRemoved(t.part.label)
		clearParents(t.part.label)
	}
	n.tabs = slices.Delete(n.tabs, i, i+1)
	if n.selected > i || n.selected >= len(n.tabs) {
		n.selected = max(n.selected-1, 0)
	}
	n.scrollStrip(n.scroll)
	clearParents(t.w)
	n.syncTabs()
	n.InvalidateLayout()
}

// Children exposes only the visible page, so keyboard traversal skips
// hidden pages.
func (n *Notebook) Children() []Widget {
	if n.selected < len(n.tabs) {
		return []Widget{n.tabs[n.selected].w}
	}
	return nil
}

// appendChildren appends the selected tab, matching Children.
func (n *Notebook) appendChildren(buf []Widget) []Widget {
	if n.selected < len(n.tabs) {
		return append(buf, n.tabs[n.selected].w)
	}
	return buf
}

// SetEnabled turns the notebook's subtree on or off through the
// per-query enable walk, like Box. Like Stack, hidden pages stay
// untouched: they are not painted, so they need no repaint.
func (n *Notebook) SetEnabled(enabled bool) {
	n.node.SetEnabled(enabled)
	invalidateTree(n)
}

// Measure reports the tab bar height plus the largest page.
func (n *Notebook) Measure(con Constraints) Size {
	if sz, ok := n.measureHit(con); ok {
		return sz
	}
	best := Size{}
	for _, t := range n.tabs {
		s := measureChild(n, t.w, con)
		best.W = max(best.W, s.W)
		best.H = max(best.H, s.H)
	}
	strip := n.layoutStrip(render.Rect{W: con.Max.W})
	best.W = max(best.W, strip.natural)
	best.H += strip.height
	return n.measureStore(con, clampSize(best, con))
}

// Arrange lays out the tab strip and the visible page beneath it.
func (n *Notebook) Arrange(r render.Rect) {
	n.node.Arrange(r)
	n.strip = n.layoutStrip(r)
	n.header.Arrange(n.strip.header)
	n.header.tabs.Arrange(n.strip.tabs)
	n.scrollStrip(n.scroll) // a wider strip may have less to scroll
	n.applyReveal()
	h := n.strip.height
	page := render.Rect{X: r.X, Y: r.Y + h, W: r.W, H: max(0, r.H-h)}
	if n.selected < len(n.tabs) {
		arrangeChild(n.tabs[n.selected].w, page)
		setParents(n, n.tabs[n.selected].w)
	}
}

// layoutStrip lays the tab bar out across r's width: each tab at its
// natural margin box (floored to tabWidth x tabBarHeight where the
// stylesheet sets no min size), in a row as tall as the tallest, inside
// the tabs box, inside the header box spanning the notebook (GTK's tabs
// expand across the header).
func (n *Notebook) layoutStrip(r render.Rect) notebookStrip {
	s := notebookStrip{tabX: make([]int, len(n.tabs)), tabW: make([]int, len(n.tabs))}
	rowH := 0
	for i, t := range n.tabs {
		tv := t.part.style(t.part)
		b := boxOf(tv, render.Insets{})
		sz := measureBox(tv, b, unbounded, func(c Constraints) Size { return n.tabContent(t, c) })
		if !tv.Has(style.PropMinWidth) {
			sz.W = max(sz.W, tabWidth+b.margin.Left+b.margin.Right)
		}
		if !tv.Has(style.PropMinHeight) {
			sz.H = max(sz.H, tabBarHeight+b.margin.Top+b.margin.Bottom)
		}
		s.tabX[i], s.tabW[i] = s.total, sz.W
		s.total += sz.W
		rowH = max(rowH, sz.H)
	}
	hb := boxOf(n.header.style(&n.header), render.Insets{})
	tb := boxOf(n.header.tabs.style(&n.header.tabs), render.Insets{})
	ho, to := hb.outer(), tb.outer()
	s.height = ho.Top + ho.Bottom + to.Top + to.Bottom + rowH
	s.natural = ho.Left + ho.Right + to.Left + to.Right + s.total
	var inHeader render.Rect
	s.header, inHeader = boxRects(hb, render.Rect{X: r.X, Y: r.Y, W: r.W, H: s.height})
	s.tabs, s.row = boxRects(tb, inHeader)
	if !n.anyTabExpands() && s.row.W > s.total {
		// Without an expanding tab the list fits its tabs, the header
		// still spanning the notebook.
		s.tabs.W -= s.row.W - s.total
		s.row.W = s.total
	}
	n.expandTabs(&s)
	return s
}

// anyTabExpands reports whether a tab shares the strip's leftover
// width (tabExpands).
func (n *Notebook) anyTabExpands() bool { return slices.ContainsFunc(n.tabs, n.tabExpands) }

// expandTabs shares the row's leftover width among the expanding tabs,
// GTK's tab-expand: a tab set to expand, or one whose label wants
// horizontal space. The others keep their width; a strip that
// overflows has no leftover.
func (n *Notebook) expandTabs(s *notebookStrip) {
	expanding := 0
	for _, t := range n.tabs {
		if n.tabExpands(t) {
			expanding++
		}
	}
	free := s.row.W - s.total
	if expanding == 0 || free <= 0 {
		return
	}
	x, k := 0, 0
	for i, t := range n.tabs {
		s.tabX[i] = x
		if n.tabExpands(t) {
			s.tabW[i] += expandShare(free, expanding, k)
			k++
		}
		x += s.tabW[i]
	}
	s.total = x
}

// tabExpands reports whether t shares the strip's leftover width.
func (n *Notebook) tabExpands(t notebookTab) bool {
	return t.expand || (t.part.label != nil && WantsExpand(t.part.label, Row))
}

// SetTabExpand makes the named tab share the strip's leftover width
// with the other expanding tabs (gtk_notebook_set_tab_expand); a tab
// whose label wants horizontal space expands without it.
func (n *Notebook) SetTabExpand(name string, on bool) {
	if i := n.tabIndex(name); i >= 0 && n.tabs[i].expand != on {
		n.tabs[i].expand = on
		n.InvalidateLayout()
	}
}

// tabIndex is the named tab's index, -1 for none.
func (n *Notebook) tabIndex(name string) int {
	return slices.IndexFunc(n.tabs, func(t notebookTab) bool { return t.name == name })
}

// tabContent is a tab's content size: its label widget's natural size,
// else its painted name's, plus the close box when Closable.
func (n *Notebook) tabContent(t notebookTab, con Constraints) Size {
	var sz Size
	if t.part.label != nil {
		sz = measureChild(t.part, t.part.label, con)
	} else {
		sh := n.face.Shape(t.name, tabTextPx)
		sz = Size{W: int(math.Ceil(sh.Advance())), H: sh.LineHeight()}
	}
	if n.Closable {
		sz.W += tabCloseW
	}
	return clampSize(sz, con)
}

// tabRect returns tab i's border box, scrolled.
func (n *Notebook) tabRect(i int) render.Rect {
	vp := n.tabViewport()
	part := n.tabs[i].part
	m := marginOf(part.style(part))
	return m.Shrink(render.Rect{X: vp.X + n.strip.tabX[i] - n.scroll, Y: vp.Y, W: n.strip.tabW[i], H: vp.H})
}

// tabLabelRect is the box a tab's label fills: its content box, less
// the close box.
func (n *Notebook) tabLabelRect(i int, border render.Rect) render.Rect {
	part := n.tabs[i].part
	tv := part.style(part)
	r := paddingOr(tv, render.Insets{}).Shrink(borderOf(tv).Shrink(border))
	if n.Closable {
		r.W = max(0, r.W-tabCloseW)
	}
	return r
}

// tabAt is the tab under p, -1 off the visible strip.
func (n *Notebook) tabAt(p Point) int {
	if !n.tabViewport().Contains(p.X, p.Y) {
		return -1
	}
	for i := range n.tabs {
		if n.tabRect(i).Contains(p.X, p.Y) {
			return i
		}
	}
	return -1
}

// arrowRects are the overflow arrows' strips, empty without overflow.
func (n *Notebook) arrowRects() [2]render.Rect {
	if !n.stripRange().Overflows() {
		return [2]render.Rect{}
	}
	row := n.strip.row
	return [2]render.Rect{
		{X: row.X, Y: row.Y, W: tabArrowW, H: row.H},
		{X: row.X + row.W - tabArrowW, Y: row.Y, W: tabArrowW, H: row.H},
	}
}

// Paint draws the tab bar through its nodes — the notebook's box, the
// header and tab-row boxes, and each tab (the selected one :checked:
// its own background and label color, the accent fill and underline as
// the theme's fallback where the stylesheet paints none) — and the
// visible page.
func (n *Notebook) Paint(cv *render.Canvas) {
	th := Current()
	v := n.style(n)
	radii := radiusOr(v, th.Radius)
	fx := pushEffects(cv, v, n.bounds, radii)
	cv = fx.cv
	paintBoxBehind(cv, v, n.bounds, radii, borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Surface))
	paintOutline(cv, v, n.bounds, radii)
	fx.pop()

	paintPart(cv, n.header.style(&n.header), n.strip.header, 0)
	paintPart(cv, n.header.tabs.style(&n.header.tabs), n.strip.tabs, 0)

	strip := cv.PushClip(n.tabViewport())
	for i, t := range n.tabs {
		rect := n.tabRect(i)
		tp := t.part
		tp.Arrange(rect)
		tv := tp.style(tp)
		var fill render.Color
		if i == n.selected {
			fill = th.SurfaceHover
		}
		paintPart(cv, tv, rect, fill)
		if i == n.selected && !tv.Has(style.PropBoxShadow) && !tv.Has(style.PropBackgroundColor) {
			cv.FillRect(render.Rect{X: rect.X, Y: rect.Y + rect.H - 3, W: rect.W, H: 3}, th.Accent)
		}
		label := n.tabLabelRect(i, rect)
		if tp.label != nil {
			arrangeChild(tp.label, label)
			PaintChild(cv, tp.label)
		} else {
			color := th.TextMuted
			if i == n.selected {
				color = th.Text
			}
			n.face.DrawAligned(cv, t.name, label, tabTextPx, pickc(0, tv, style.PropColor, color), render.AlignCenter)
		}
		if n.Closable {
			strokeCross(cv, rect.X+rect.W-13, rect.Y+rect.H/2, 4, th.TextMuted)
		}
	}
	cv.PopClip(strip)
	for k, r := range n.arrowRects() {
		if !r.Empty() {
			n.arrows[k].Arrange(r)
			setParents(n, n.arrows[k])
			PaintChild(cv, n.arrows[k])
		}
	}
	if n.selected < len(n.tabs) {
		h := n.strip.height
		prev := cv.PushClip(render.Rect{
			X: n.bounds.X, Y: n.bounds.Y + h,
			W: n.bounds.W, H: max(0, n.bounds.H-h),
		})
		PaintChild(cv, n.tabs[n.selected].w)
		cv.PopClip(prev)
	}
}

// paintPart paints a tab bar node's box at its border box: effects,
// background (fill where the stylesheet sets none), border, outline.
func paintPart(cv *render.Canvas, v *style.Values, border render.Rect, fill render.Color) {
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, border, radii)
	paintBoxBehind(fx.cv, v, border, radii, borderOf(v), pickc(0, v, style.PropBackgroundColor, fill))
	paintOutline(fx.cv, v, border, radii)
	fx.pop()
}

// Role implements Roleer.
func (n *Notebook) Role() Role { return RoleTabList }

// HitTest resolves the tab strip into the notebook itself (clicks
// select or close) and otherwise delegates to the visible page.
func (n *Notebook) HitTest(p Point) Widget {
	if !n.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if n.overStrip(p) {
		return n
	}
	if n.selected < len(n.tabs) {
		if hit := HitChild(n.tabs[n.selected].w, p); hit != nil {
			return hit
		}
	}
	return n
}

// ClickAt selects the tab under the point, or fires OnTabClose when
// the close box of a tab was pressed.
func (n *Notebook) ClickAt(p Point) {
	arrows := n.arrowRects()
	switch {
	case arrows[0].Contains(p.X, p.Y):
		n.scrollStrip(n.scroll - tabWidth)
		return
	case arrows[1].Contains(p.X, p.Y):
		n.scrollStrip(n.scroll + tabWidth)
		return
	}
	i := n.tabAt(p)
	if i < 0 {
		return
	}
	if rect := n.tabRect(i); n.Closable && p.X > rect.X+rect.W-20 {
		if n.OnTabClose != nil {
			n.OnTabClose(n.tabs[i].name)
		}
		return
	}
	n.selectIndex(i)
}

// PressAt picks up a tab for reordering.
func (n *Notebook) PressAt(p Point) {
	if i := n.tabAt(p); n.Reorderable && i >= 0 {
		n.drag.begin(p)
		n.dragTab = i
	}
}

// DragMove moves the dragged tab into the slot under the pointer, so
// the strip reorders live as the pointer crosses its neighbors.
func (n *Notebook) DragMove(p Point) {
	if _, _, ok := n.drag.travel(p); !ok {
		return
	}
	vp := n.tabViewport()
	if i := n.tabAt(Point{X: p.X, Y: vp.Y + vp.H/2}); i >= 0 && i != n.dragTab {
		n.moveTab(n.dragTab, i, true)
		n.dragTab = i
	}
}

// PressEnd drops the tab.
func (n *Notebook) PressEnd() { n.drag.end() }

// overStrip reports whether p is on the tab bar.
func (n *Notebook) overStrip(p Point) bool { return p.Y < n.bounds.Y+n.strip.height }

// HoverMove tracks whether the pointer is over the tab strip, and the
// tab under it (its :hover).
func (n *Notebook) HoverMove(p Point) {
	n.overHeader = n.overStrip(p)
	n.hoverTab(n.tabAt(p))
}

// SetHovered forgets the strip when the pointer leaves the notebook
// itself (into a page, or away).
func (n *Notebook) SetHovered(on bool) {
	if !on {
		n.overHeader = false
		n.hoverTab(-1)
	}
}

// hoverTab moves :hover to tab i, none for -1.
func (n *Notebook) hoverTab(i int) {
	for k, t := range n.tabs {
		t.part.setHovered(k == i)
	}
}

// CursorName is the hovered tab label's cursor (GTK resolves it on the
// label itself), else the notebook's own.
func (n *Notebook) CursorName() string {
	for _, t := range n.tabs {
		if !t.part.hovered || t.part.label == nil {
			continue
		}
		if css := CSSCursorOf(t.part.label); css != "" {
			return css
		}
		if name := CursorNameOf(t.part.label); name != "" {
			return name
		}
	}
	return CursorNameOf(n)
}

func (n *Notebook) listensScroll() bool { return true }

// claimScroll scrolls an overflowing strip under the pointer, a third
// of a tab per wheel step, after the notebook's own scroll hook;
// anywhere else the step passes on to the scrollers outside.
func (n *Notebook) claimScroll(e ScrollEvent) bool {
	if n.node.claimScroll(e) {
		return true
	}
	if !n.overHeader || !n.stripRange().Overflows() {
		return false
	}
	n.scrollStrip(n.scroll + e.DY*tabWidth/3)
	return true
}

// KeyAction implements KeyActionHandler: ctrl+PageUp/PageDown cycle
// through tabs. Other actions are ignored.
func (n *Notebook) KeyAction(a KeyAction, mods Mods) {
	if mods&ModCtrl == 0 || len(n.tabs) == 0 {
		return
	}
	switch a {
	case KeyPriorPage:
		n.selectIndex((n.selected - 1 + len(n.tabs)) % len(n.tabs))
	case KeyNextPage:
		n.selectIndex((n.selected + 1) % len(n.tabs))
	}
}
