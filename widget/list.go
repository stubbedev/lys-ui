package widget

import (
	"maps"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
)

// ListModel supplies rows to a List. Row may return a fresh widget per
// call; the List caches the ones it is currently showing. Len and Row
// are re-queried on List.Changed.
type ListModel[W Widget] interface {
	Len() int
	Row(i int) W
}

// SelectionMode is how pointer and keyboard gestures turn into a
// selected row set on a List.
type SelectionMode uint8

const (
	// SelectionSingle keeps zero or one selected row: a click or a
	// plain motion key selects exactly that row, Enter activates it.
	// The zero value, so a List is single-select unless told otherwise.
	SelectionSingle SelectionMode = iota
	// SelectionNone ignores every selection gesture and Select call;
	// rows keep only their own widget behavior. Keyboard selection
	// keys do nothing.
	SelectionNone
	// SelectionBrowse is single-select that never sits empty once it
	// has selected: pointer motion over a row selects it (focus
	// follows the pointer, GTK's browse mode) and gestures cannot
	// clear the selection, only move it.
	SelectionBrowse
	// SelectionMultiple keeps any set of rows. Clicks toggle rows,
	// drags rubber-band select, shift+arrows extend from the anchor,
	// ctrl+arrows move the keyboard cursor without selecting,
	// ctrl+space toggles the cursor row, ctrl+a selects everything.
	SelectionMultiple
)

// List is a virtualized list view: only the rows inside the viewport
// are requested from the model, measured, and painted, so ten
// thousand rows cost about the same as ten. Selection is pointer and
// keyboard driven, clamps at the ends, and Enter activates.
//
// The selection model is SetSelectionMode: single (the default),
// none, browse, or multiple. Selection reports the whole selected set
// and OnSelectionChanged fires once per gesture with it; OnSelect
// keeps its single-row meaning in single and browse mode and stays
// silent in multiple mode.
//
// Pointer gestures land on a row proxy the List keeps over every
// visible row: the proxy forwards what the row widget itself
// implements (clicks, hover and press tracking, tooltips, cursor
// names) while the List owns selection. In selectable modes the
// proxy's drag is the List's (rubber-band in multiple, motion-select
// in single and browse); rows that need their own drag behavior
// belong in a SelectionNone list. In multiple mode a row that
// contains a CheckButton cedes the check to the selection model: the
// check reflects row membership and clicking it toggles membership
// instead of the check's own state.
type List struct {
	node
	selection
	model listModel
	rowH  int
	// autoH derives rowH from a styled row (a zero rowHeight): probe is
	// that row, parented below the list so the stylesheets reach it.
	autoH bool
	probe *listRow
	// maxH caps the natural height (SetMaxHeight).
	maxH int
	// pxFrac is pixel scrolling below a whole pixel (ScrollPixels);
	// kin is the touchpad glide.
	pxFrac float64
	kin    kinetic
	// singleClick activates a row on a plain click (SetSingleClickActivate).
	singleClick bool
	// cellW is the grid mode's minimum cell width (0 is a plain
	// list); cols is how many cells the arranged width fits.
	cellW int
	cols  int
	// minCols and maxCols bound a grid's column count; zero leaves the
	// side open.
	minCols, maxCols int
	// colSpacing and rowSpacing are the gaps between a grid's columns
	// and between lines, before the stylesheet's border-spacing.
	colSpacing, rowSpacing int
	// rowElement names the row proxies' CSS node when the list is not
	// a plain listview (a GridView's cells are `child`).
	rowElement string

	// view is the content box, the scroll viewport inside the list's
	// CSS box, where the rows lay out.
	view  render.Rect
	offY  int
	hover int
	rows  map[int]*listRow

	// pointer gesture state: dragging while a press is down, the row
	// the press anchored on (-1 until a point resolves it), the last
	// drag point, and whether the band ever left the anchor row (a
	// click after that ends the gesture instead of toggling). The
	// selection holds its notification for the gesture.
	dragging   bool
	gestureRow int
	dragPoint  Point
	dragMoved  bool
	// dragOut reports a gesture that began on a selected row of a list
	// with DragContent: it offers the selection to other apps instead
	// of moving the selection.
	dragOut bool

	// drag auto-scroll: direction and the running tween's cancel.
	scrollDir  int
	scrollAnim anim.Cancel

	OnSelect           func(i int)
	OnActivate         func(i int)
	OnSelectionChanged func(rows []int)
	// OnPointer sees every button press on the list first, with the
	// row under it (-1 below the last row); a true return consumes the
	// press, so no selection, activation or drag follows.
	OnPointer func(row int, e PointerEvent) bool
	// OnScroll sees every wheel step over the list first, with the row
	// under the pointer (-1 below the last row); a true return
	// consumes the step instead of scrolling.
	OnScroll func(row int, e ScrollEvent) bool

	// DragContent, when set, lets the selection be dragged out to other
	// applications, the way a file manager's is: a press on a selected
	// row that moves past the drag threshold offers DragContent of the
	// rows dragged - the whole selection in multiple mode, the pressed
	// row otherwise - instead of moving the selection. A press on an
	// unselected row still selects, rubber-bands and motion-selects;
	// nil (or a nil return) drags nothing.
	DragContent func(rows []int) *DragContent
}

// Edge bands that engage auto-scroll during a drag, and the time one
// rubber-band auto-scroll step takes per row.
const (
	listEdgeScroll = 24
	listScrollStep = 90 * time.Millisecond
)

// listModel erases the model's type parameter; the List only needs
// widgets out of it.
type listModel interface {
	len() int
	row(i int) Widget
}

// rowUnbinder is a model that recycles: the List tells it when a row's
// index left the viewport, so it can unbind the row's item and pool
// the widget.
type rowUnbinder interface {
	unbind(i int)
}

// modelAdapter binds a typed model to the erased interface.
type modelAdapter[W Widget] struct {
	m ListModel[W]
}

func (a modelAdapter[W]) len() int { return a.m.Len() }

func (a modelAdapter[W]) row(i int) Widget { return a.m.Row(i) }

func (a modelAdapter[W]) unbind(i int) {
	if u, ok := any(a.m).(rowUnbinder); ok {
		u.unbind(i)
	}
}

// NewList wraps a model with a uniform row height in pixels; zero
// derives the height from measuring the first row.
func NewList[W Widget](model ListModel[W], rowHeight int) *List {
	l := &List{}
	initList(l, model, rowHeight)
	return l
}

func initList[W Widget](l *List, model ListModel[W], rowHeight int) {
	l.model = modelAdapter[W]{m: model}
	l.rowH = rowHeight
	l.autoH = rowHeight <= 0
	l.hover = -1
	l.rows = make(map[int]*listRow)
	l.initSelection(l, SelectionSingle)
}

// SetCellWidth switches the list into a grid: items flow left to
// right in as many equal columns of at least w pixels as the width
// fits, every line the row height tall, still virtualized by line. Up
// and down move a line, left and right one item. Zero is a plain list.
func (l *List) SetCellWidth(w int) {
	if w == l.cellW {
		return
	}
	l.cellW = max(0, w)
	l.InvalidateLayout()
}

// columns is how many items share a line.
func (l *List) columns() int { return max(1, l.cols) }

// lines is how many lines n items take.
func (l *List) lines(n int) int {
	c := l.columns()
	return (n + c - 1) / c
}

// spacing is the gap between columns and between lines: the
// programmatic spacing plus the stylesheet's border-spacing, as GTK's
// list widgets take theirs from CSS.
func (l *List) spacing() (col, row int) {
	col, row = l.colSpacing, l.rowSpacing
	if v := l.style(l); v.Has(style.PropBorderSpacing) {
		col += v.BorderSpacingH
		row += v.BorderSpacingV
	}
	return col, row
}

// pitch is the distance from one line's top to the next's.
func (l *List) pitch() int {
	_, row := l.spacing()
	return l.rowH + row
}

// contentH is the height lines of rows take, gaps between them.
func (l *List) contentH(lines int) int {
	_, row := l.spacing()
	return lines*l.rowH + max(0, lines-1)*row
}

// fitColumns is how many cells of the grid's cell width, with the
// column gaps between them, fit in w.
func (l *List) fitColumns(w int) int {
	if l.cellW <= 0 {
		return 1
	}
	gap, _ := l.spacing()
	c := max(1, (w+gap)/(l.cellW+gap), l.minCols)
	if l.maxCols > 0 {
		c = min(c, max(l.maxCols, l.minCols, 1))
	}
	return c
}

// stride is a cell's width and the distance from one column's left
// edge to the next's.
func (l *List) stride() (w, step int) {
	gap, _ := l.spacing()
	c := l.columns()
	w = max(1, (l.view.W-(c-1)*gap)/c)
	return w, w + gap
}

// cellRect is item i's rect in root coordinates; the last column
// takes the width's remainder.
func (l *List) cellRect(i int) render.Rect {
	c := l.columns()
	w, step := l.stride()
	x := l.view.X + (i%c)*step
	if i%c == c-1 {
		w = l.view.X + l.view.W - x
	}
	return render.Rect{X: x, Y: l.view.Y + (i/c)*l.pitch() - l.offY, W: w, H: l.rowH}
}

// syncSelected mirrors membership into the cached row proxies'
// :selected state; SetState restyles only the rows that flipped.
// Virtualization means the rest are seeded by newRow as they scroll in.
func (l *List) syncSelected() {
	for i, r := range l.rows {
		r.SetState(StateSelected, l.isSelected(i))
	}
}

// count implements selectionHost.
func (l *List) count() int { return l.model.len() }

// selectionSync implements selectionHost: the checkbox rows and the
// proxies' :selected state follow membership, and the list repaints.
func (l *List) selectionSync() {
	l.syncChecks()
	l.syncSelected()
	l.Invalidate()
}

// selectionNotify implements selectionHost: OnSelectionChanged.
func (l *List) selectionNotify() {
	if l.OnSelectionChanged != nil {
		l.OnSelectionChanged(l.Selection())
	}
}

// selectionSingle implements selectionHost: OnSelect.
func (l *List) selectionSingle(i int) {
	if l.OnSelect != nil {
		l.OnSelect(i)
	}
}

// reveal implements selectionHost. Before the first layout there is
// no viewport to scroll against, so the reveal waits for Arrange.
func (l *List) reveal(i int) {
	l.kin.stop()
	if l.view.H <= 0 {
		l.revealAt = i + 1
		return
	}
	l.revealAt = 0
	l.scrollTo(i)
}

// evict drops a cached row and, when the model recycles, releases it
// back to the model's pool.
func (l *List) evict(i int) {
	delete(l.rows, i)
	if u, ok := l.model.(rowUnbinder); ok {
		u.unbind(i)
	}
}

// evictAll drops every cached row.
func (l *List) evictAll() {
	for i := range l.rows {
		l.evict(i)
	}
}

// Changed re-queries the model after its data changed, keeping cached
// row widgets whose indices still exist.
func (l *List) Changed() {
	n := l.model.len()
	for i := range l.rows {
		if i >= n {
			l.evict(i)
		}
	}
	l.offY = min(l.offY, l.scrollMax())
	l.clampTo(n)
	l.syncChecks()
	l.InvalidateLayout()
}

// Refresh re-queries every row widget, keeping the selection and the
// scroll: the rows' data changed in place (a column resized, a
// thumbnail arrived), where Changed would keep showing the cached
// widgets.
func (l *List) Refresh() {
	l.evictAll()
	l.probe = nil
	l.Changed()
}

// Reset is for a model whose rows were replaced wholesale (a folder
// listing for another folder): every cached row widget, the selection
// and the scroll go, and OnSelectionChanged fires when a selection was
// dropped.
func (l *List) Reset() {
	l.evictAll()
	l.probe = nil
	l.offY, l.hover = 0, -1
	had := l.clearAll()
	l.Changed()
	if had {
		l.notify()
	}
}

// scrollTo nudges the offset so row i is inside the viewport.
func (l *List) scrollTo(i int) {
	top, bottom := l.offY, l.offY+l.view.H
	y := (i / l.columns()) * l.pitch()
	if y < top {
		l.offY = y
	} else if y+l.rowH > bottom {
		l.offY = y + l.rowH - l.view.H
	}
}

// measureRowH settles the row height: the fixed one, or the first
// row's as styled - measured on a probe row parented below the list, so
// the stylesheet's padding and fonts count (GTK sizes rows by their
// styled natural height), and re-measured as they change.
func (l *List) measureRowH() {
	if !l.autoH {
		return
	}
	if l.model.len() == 0 {
		l.rowH = 1
		return
	}
	if l.probe == nil {
		l.probe = l.newRow(0)
	}
	setParents(l, l.probe)
	w := l.view.W
	if w <= 0 {
		w = unboundedPx // not arranged yet: the natural width
	}
	s := l.probe.Measure(Constraints{Max: Size{W: w, H: unboundedPx}})
	l.rowH = max(1, s.H)
}

// Measure reports the offered width by the full content height inside
// the list's CSS box, clamped to the constraints: the list is the
// viewport, not the content.
func (l *List) Measure(con Constraints) Size {
	if sz, ok := l.measureHit(con); ok {
		return sz
	}
	v := l.style(l)
	return l.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		lines, w := l.model.len(), 120
		if l.cellW > 0 {
			c := l.fitColumns(inner.Max.W)
			lines, w = (lines+c-1)/c, l.cellW
		}
		l.measureRowH()
		h := l.contentH(lines)
		if l.maxH > 0 {
			h = min(h, l.maxH)
		}
		return clampSize(Size{W: w, H: h}, inner)
	}))
}

// SetMaxHeight caps the list's natural height at h pixels (0 lifts the
// cap): a longer model scrolls inside it, still virtualized - GTK's
// list in a scrolled window with a max content height.
func (l *List) SetMaxHeight(h int) {
	if l.maxH == h {
		return
	}
	l.maxH = max(h, 0)
	l.InvalidateLayout()
}

// Arrange records the border box inside r and lays the viewport out in
// its content box.
func (l *List) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(l.style(l), render.Insets{}), r)
	l.node.Arrange(border)
	l.view = inner
	l.cols = l.fitColumns(inner.W)
	l.measureRowH()
	l.offY = min(l.offY, l.scrollMax())
	if i := l.revealAt - 1; i >= 0 && l.view.H > 0 {
		l.revealAt = 0
		if i < l.model.len() {
			l.scrollTo(i)
		}
	}
}

// visible returns the half-open range of rows inside the viewport and
// caches their proxy widgets, evicting everything off-screen.
func (l *List) visible() (first, last int) {
	n, c := l.model.len(), l.columns()
	first = min(l.offY/l.pitch()*c, max(0, n-1))
	last = min(n, ((l.offY+l.view.H)/l.pitch()+1)*c)
	for i := range l.rows {
		if i < first || i >= last {
			l.evict(i)
		}
	}
	for i := first; i < last; i++ {
		r, ok := l.rows[i]
		if !ok {
			r = l.newRow(i)
			l.rows[i] = r
		}
		r.SetClass(listRowLast, i == n-1)
	}
	return first, last
}

// listRowLast marks the model's last row, which the cascade cannot
// tell by position: the virtualized rows are only those in view.
const listRowLast = "last"

// newRow wraps model row i in its proxy, claiming the checkbox of a
// checkbox row in multiple mode and syncing it to membership.
func (l *List) newRow(i int) *listRow {
	r := &listRow{list: l, idx: i, row: l.model.row(i)}
	if l.rowElement != "" {
		r.SetElement(l.rowElement)
	}
	if l.isSelected(i) {
		r.flags = StateSelected // seeded pre-style; syncSelected carries the flips
	}
	if l.mode == SelectionMultiple {
		r.check = findCheckButton(r.row)
		if r.check != nil {
			r.check.SetChecked(l.isSelected(i))
		}
	}
	return r
}

// syncChecks mirrors membership into the checkbox rows currently in
// the cache; virtualization means only visible checks exist to sync,
// and newRow syncs the rest as they scroll in.
func (l *List) syncChecks() {
	if l.mode != SelectionMultiple {
		return
	}
	for i, r := range l.rows {
		if r.check != nil {
			r.check.SetChecked(l.isSelected(i))
		}
	}
}

// findCheckButton returns the first CheckButton at or below w.
func findCheckButton(w Widget) *CheckButton {
	if c, ok := w.(*CheckButton); ok {
		return c
	}
	if c, ok := w.(childser); ok {
		for _, k := range c.Children() {
			if found := findCheckButton(k); found != nil {
				return found
			}
		}
	}
	return nil
}

// Paint draws the viewport background, the cursor ring, and exactly
// the visible rows; the rows paint their own hover and selection fills
// (the `row` node's CSS box).
func (l *List) Paint(cv *render.Canvas) {
	th := Current()
	if bg := pickc(0, l.style(l), style.PropBackgroundColor, th.Surface); bg != 0 {
		cv.FillRect(l.bounds, bg)
	}
	prev := cv.PushClip(l.view)

	first, last := l.visible()
	for i := first; i < last; i++ {
		w := l.rows[i]
		rect := l.cellRect(i)
		// Parent first: the row measures and styles inside Arrange, and
		// its cascade must reach the stylesheets above the list.
		setParents(l, w)
		w.Arrange(rect)
		if l.mode == SelectionMultiple && i == l.cursor {
			cv.BorderRect(rect, 1, th.Accent)
		}
		PaintChild(cv, w)
	}
	cv.PopClip(prev)
}

// rowFill is the theme's band tint behind a hovered or selected row,
// the fallback under a stylesheet's `row:hover` / `row:selected`
// background: translucent over the viewport fill, as the bands always
// painted.
func (l *List) rowFill(i int) render.Color {
	switch {
	case l.isSelected(i):
		return selectedTint()
	case i == l.hover:
		tint := Current().SurfaceHover
		return render.RGBA(tint.R(), tint.G(), tint.B(), 120)
	}
	return 0
}

// selectedTint is the theme's translucent accent behind a selected
// item (a list row, a flow box child) where no stylesheet names one.
func selectedTint() render.Color {
	hl := Current().Accent
	return render.RGBA(hl.R(), hl.G(), hl.B(), 70)
}

// Role implements Roleer.
func (l *List) Role() Role { return RoleList }

// HitTest returns the row proxy under p, or the list itself where no
// item is (see itemAt).
func (l *List) HitTest(p Point) Widget {
	if !l.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if i, ok := l.itemAt(p); ok {
		if w, ok := l.rows[i]; ok {
			return w
		}
	}
	return l
}

func (l *List) claimPointer(e PointerEvent) bool {
	if l.OnPointer != nil && l.OnPointer(l.rowUnder(e.At), e) {
		return true
	}
	return l.node.claimPointer(e)
}

func (l *List) listensScroll() bool { return l.OnScroll != nil || l.node.listensScroll() }

func (l *List) claimScroll(e ScrollEvent) bool {
	if l.OnScroll != nil && l.OnScroll(l.rowUnder(e.At), e) {
		return true
	}
	return l.node.claimScroll(e)
}

// rowUnder is the row under p, -1 when none is.
func (l *List) rowUnder(p Point) int {
	if i, ok := l.itemAt(p); ok {
		return i
	}
	return -1
}

// itemAt maps a point to the item whose border box holds it: past the
// last item, or in a row proxy's margin, no item is under it.
func (l *List) itemAt(p Point) (int, bool) {
	i, ok := l.rowAtExact(p)
	if !ok {
		return 0, false
	}
	box := l.cellRect(i)
	if r, ok := l.rows[i]; ok {
		box, _ = boxRects(boxOf(r.style(r), render.Insets{}), box)
	}
	return i, box.Contains(p.X, p.Y)
}

// rowAt maps a point to a row index, clamped into the model; a gap
// counts toward the item before it.
func (l *List) rowAt(p Point) int {
	line := max((p.Y-l.view.Y+l.offY)/l.pitch(), 0)
	return min(line*l.columns()+l.columnAt(p), l.model.len()-1)
}

// columnAt maps a point to its grid column, clamped.
func (l *List) columnAt(p Point) int {
	_, step := l.stride()
	return min(max((p.X-l.view.X)/step, 0), l.columns()-1)
}

// rowAtExact maps a point to a row index only when one is under it:
// the trailing band below the last row hits no row, where rowAt
// clamps into the last one.
func (l *List) rowAtExact(p Point) (int, bool) {
	y := p.Y - l.view.Y + l.offY
	if y < 0 {
		return 0, false
	}
	i := (y/l.pitch())*l.columns() + l.columnAt(p)
	if i >= l.model.len() {
		return 0, false
	}
	return i, true
}

// gestureStart records the press that begins a pointer gesture; row
// -1 defers the anchor to the first point that resolves one.
func (l *List) gestureStart(row int) {
	l.dragging = true
	l.dragMoved = false
	l.gestureRow = row
	l.dragOut = l.DragContent != nil && l.mode != SelectionNone && row >= 0 && l.isSelected(row)
	l.kin.stop()
	l.hold()
}

// gestureEnd closes a pointer gesture: auto-scroll stops and a held
// notification lands, exactly once, with the full final set.
func (l *List) gestureEnd() {
	if !l.dragging {
		return
	}
	l.dragging = false
	l.stopAutoScroll()
	l.release()
}

// rowClick applies the click half of a pointer gesture on row r.idx.
// A click after the band left the anchor row ends the gesture without
// toggling: the drag already applied its selection. Checkbox rows in
// multiple mode toggle membership through the check and do not reach
// the row's own click handling; every other row forwards to it after
// the selection applies, so interactive rows keep working.
func (l *List) rowClick(r *listRow, p Point) {
	if !IsEnabled(r.row) {
		return
	}
	if l.dragMoved {
		return
	}
	l.click(r.idx)
	if l.singleClick && l.mode != SelectionMultiple {
		l.activate(r.idx)
	}
	if r.check == nil || l.mode != SelectionMultiple {
		if c, ok := r.row.(Clicker); ok {
			c.ClickAt(p)
		}
	}
}

// rowDoubleClick applies the second click of a pair: the first click
// already applied the selection, so the pair activates the row
// (double-click opens, GTK-style) instead of toggling it back off.
func (l *List) rowDoubleClick(r *listRow, p Point) {
	if !IsEnabled(r.row) || l.mode == SelectionNone {
		return
	}
	l.activate(r.idx)
	if d, ok := r.row.(DoubleClicker); ok {
		d.DoubleClickAt(p)
	}
}

// dragApply applies the motion half of a pointer gesture at p:
// rubber-band from the anchor row in multiple mode, motion-select in
// single and browse. The band remembers leaving the anchor row, and
// the edge bands engage auto-scroll.
func (l *List) dragApply(p Point) {
	l.dragPoint = p
	if l.gestureRow < 0 {
		if i, ok := l.itemAt(p); ok {
			l.gestureRow = i
		} else {
			return
		}
	}
	if row := l.rowAt(p); row != l.gestureRow {
		l.dragMoved = true
	}
	l.applyDragSelection()
	l.updateAutoScroll()
}

// applyDragSelection applies the pointer gesture's selection at the
// current drag point: rubber-band from the anchor in multiple mode,
// motion-select in single and browse. Auto-scroll frames call it with
// the scrolled offset already in place, so it never touches the
// scroll state.
func (l *List) applyDragSelection() {
	l.drag(l.gestureRow, l.rowAt(l.dragPoint))
}

// stopAutoScroll drops the drag auto-scroll tween, if any.
func (l *List) stopAutoScroll() {
	if l.scrollAnim != nil {
		l.scrollAnim()
		l.scrollAnim = nil
	}
	l.scrollDir = 0
}

// scrollMax is the largest viewport offset.
func (l *List) scrollMax() int {
	return max(0, l.contentH(l.lines(l.model.len()))-l.view.H)
}

// updateAutoScroll runs or stops the edge auto-scroll for the current
// drag point: one row per step while the pointer sits in an edge band
// (or beyond it), nothing elsewhere or when the offset is already
// pinned at that end. Under reduced motion (anim.Instant) each call
// steps one row discretely instead of animating, so no tween runs.
func (l *List) updateAutoScroll() {
	if !l.dragging {
		l.stopAutoScroll()
		return
	}
	dir := 0
	switch {
	case l.dragPoint.Y < l.view.Y+listEdgeScroll:
		dir = -1
	case l.dragPoint.Y >= l.view.Y+l.view.H-listEdgeScroll:
		dir = 1
	}
	row := l.pitch()
	if dir == 0 || l.offY == min(max(0, l.offY+dir*row), l.scrollMax()) {
		l.stopAutoScroll()
		return
	}
	if anim.Instant() {
		l.offY = min(max(0, l.offY+dir*row), l.scrollMax())
		l.applyDragSelection()
		l.Invalidate()
		return
	}
	if l.scrollAnim != nil && l.scrollDir == dir {
		return
	}
	l.stopAutoScroll()
	l.scrollDir = dir
	base := l.offY
	l.scrollAnim = anim.Play(anim.Animate(listScrollStep, func(t float64) {
		l.offY = min(max(0, base+int(float64(dir*row)*t)), l.scrollMax())
		l.applyDragSelection()
		l.Invalidate()
		if t >= 1 {
			l.scrollAnim = nil
			l.updateAutoScroll()
		}
	}).Easing(anim.Linear))
}

// HoverMove implements HoverMover: tracks the hovered row for styling
// and per-row tooltips, none where no item is under the pointer.
// Browse mode selects the hovered row (focus follows the pointer).
// Disabled lists highlight nothing.
func (l *List) HoverMove(p Point) {
	if !IsEnabled(l) {
		return
	}
	i := l.rowUnder(p)
	if i == l.hover {
		return
	}
	l.hover = i
	l.Invalidate()
	if l.mode == SelectionBrowse && i >= 0 {
		l.selectOne(i)
	}
}

// ScrollBy implements ScrollHandler. Disabled lists do not scroll.
func (l *List) ScrollBy(dx, dy int) {
	if !IsEnabled(l) {
		return
	}
	l.kin.stop()
	l.offY = min(max(0, l.offY+dy*scrollStepPx), l.scrollMax())
	l.Invalidate()
}

// ClickAt applies a press-and-release on the list itself: a row hit
// that no proxy covers (a partially visible last row, say) selects
// that row per the mode.
func (l *List) ClickAt(p Point) {
	if !IsEnabled(l) {
		return
	}
	if i, ok := l.itemAt(p); ok {
		l.rowClick(&listRow{list: l, idx: i, row: l.model.row(i)}, p)
	}
}

// DragMove implements DragMover for presses on the list itself; see
// dragApply.
func (l *List) DragMove(p Point) {
	if !IsEnabled(l) {
		return
	}
	l.dragApply(p)
}

// SetPressed implements PressSetter: true begins a pointer gesture on
// the list itself, false only clears the row's own pressed tracking.
func (l *List) SetPressed(on bool) {
	if on {
		l.gestureStart(-1)
	}
}

// PressEnd implements PressEnder: closes the gesture started by the
// press, landing the held selection notification.
func (l *List) PressEnd() {
	l.gestureEnd()
}

// KeyAction implements KeyActionHandler: selection motion clamps at
// the ends (pinned), PageUp/PageDown move a viewport of rows, and
// Enter activates exactly once per press. Multiple mode adds the
// cursor/anchor gestures; none mode ignores selection keys. Disabled
// lists ignore keys.
func (l *List) KeyAction(a KeyAction, mods Mods) {
	if IsEnabled(l) {
		l.key(a, mods, l.navigate)
	}
}

// navigate maps a motion key to its target row: a line up or down, an
// item sideways in a grid, a viewport of lines per page.
func (l *List) navigate(from int, a KeyAction) (int, bool) {
	line, item := l.steps()
	page := max(1, l.view.H/l.pitch()) * line
	switch a {
	case KeyUp:
		return from - line, true
	case KeyDown:
		return from + line, true
	case KeyLeft:
		return from - item, item > 0
	case KeyRight:
		return from + item, item > 0
	case KeyPriorPage:
		return from - page, true
	case KeyNextPage:
		return from + page, true
	}
	return 0, false
}

// steps is how far up/down and left/right move: a line and nothing in
// a list, a line of cells and one cell in a grid.
func (l *List) steps() (line, item int) {
	if l.cellW > 0 {
		return l.columns(), 1
	}
	return 1, 0
}

// activate fires OnActivate for one row when it exists.
func (l *List) activate(i int) {
	if i >= 0 && i < l.model.len() && l.OnActivate != nil {
		l.OnActivate(i)
	}
}

// listRow is the interactive proxy the List puts over every visible
// row widget: pointer gestures land on it (the List returns it from
// HitTest), and it forwards what the row itself implements while the
// List owns selection.
type listRow struct {
	node
	list  *List
	idx   int
	row   Widget
	check *CheckButton
}

// Measure wraps the row widget in the row's CSS box: margin, border,
// and padding around the row's natural size. The row is parented first
// so it measures as styled.
func (r *listRow) Measure(con Constraints) Size {
	setParents(r, r.row)
	v := r.style(r)
	return measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return r.row.Measure(inner)
	})
}

// Arrange records the proxy's border box and lays the row widget out
// in the content box, linking it below the proxy for the ancestor
// walks. The row is measured first: no Measure pass reaches it (the
// list measures as a viewport), and containers arrange from what they
// measured.
func (r *listRow) Arrange(rect render.Rect) {
	border, inner := boxRects(boxOf(r.style(r), render.Insets{}), rect)
	r.node.Arrange(border)
	setParents(r, r.row)
	r.row.Measure(Constraints{Max: Size{W: inner.W, H: inner.H}})
	r.row.Arrange(inner)
}

// Paint draws the row's CSS box behind the row widget — GTK's `row`:
// the background the theme tints for hover and selection and a
// stylesheet overrides per state, rounded and bordered, with the
// outline above the content.
func (r *listRow) Paint(cv *render.Canvas) {
	v := r.style(r)
	fill := pickc(0, v, style.PropBackgroundColor, r.list.rowFill(r.idx))
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, v, r.bounds, radii)
	cv = fx.cv
	paintBoxBehind(cv, v, r.bounds, radii, borderOf(v), fill)
	PaintChild(cv, r.row)
	paintOutline(cv, v, r.bounds, radii)
	fx.pop()
}

// HitTest returns the proxy while p is inside its band.
func (r *listRow) HitTest(p Point) Widget { return r.HitLeaf(r, p) }

// ClickAt applies the list's click selection, then the row's own.
func (r *listRow) ClickAt(p Point) { r.list.rowClick(r, p) }

// DoubleClickAt activates the row (double-click opens) and forwards
// to the row's own double-click handling. The first click of the pair
// already applied the selection; the second does not toggle it back.
func (r *listRow) DoubleClickAt(p Point) { r.list.rowDoubleClick(r, p) }

// DragMove applies the list's drag selection; in none mode the row's
// own drag handling runs instead.
func (r *listRow) DragMove(p Point) {
	if r.list.dragOut {
		return
	}
	if r.list.mode == SelectionNone {
		if d, ok := r.row.(DragMover); ok {
			d.DragMove(p)
		}
		return
	}
	r.list.dragApply(p)
}

// dragRows are the rows a drag-out from row idx carries: the whole
// selection in multiple mode, the row otherwise.
func (l *List) dragRows(idx int) []int {
	if l.mode == SelectionMultiple {
		return l.Selection()
	}
	return []int{idx}
}

// DragContent implements DragSource: a drag-out of the selection when
// the gesture began on a selected row of a list with DragContent, else
// the row's own content when it is a DragSource, else nothing.
func (r *listRow) DragContent() *DragContent {
	if r.list.dragOut {
		return r.list.DragContent(r.list.dragRows(r.idx))
	}
	if s, ok := r.row.(DragSource); ok {
		return s.DragContent()
	}
	return nil
}

// DragIcon implements DragIconer: a drag of several rows shows the
// pressed row on a stack with their count.
func (r *listRow) DragIcon() Widget {
	if n := len(r.list.dragRows(r.idx)); r.list.dragOut && n > 1 {
		return newDragStack(r, n)
	}
	return r
}

// SetPressed tracks the row's own pressed state and opens the list's
// pointer gesture on the press.
func (r *listRow) SetPressed(on bool) {
	if pr, ok := r.row.(PressSetter); ok {
		pr.SetPressed(on)
	}
	if on {
		r.list.gestureStart(r.idx)
	}
}

// PressEnd closes the list's pointer gesture.
func (r *listRow) PressEnd() { r.list.gestureEnd() }

// SetHovered forwards hover tracking to the row widget.
func (r *listRow) SetHovered(on bool) {
	if h, ok := r.row.(HoverSetter); ok {
		h.SetHovered(on)
	}
}

// HoverMove feeds the list's hover band, then the row's own handling.
func (r *listRow) HoverMove(p Point) {
	r.list.HoverMove(p)
	if h, ok := r.row.(HoverMover); ok {
		h.HoverMove(p)
	}
}

// TooltipText returns the row widget's tooltip, the proxy's own when
// the row carries none.
func (r *listRow) TooltipText() string {
	if t, ok := r.row.(TooltipTexter); ok {
		return t.TooltipText()
	}
	return r.tooltip
}

// CursorName forwards the row widget's cursor shape.
func (r *listRow) CursorName() string {
	if c, ok := r.row.(CursorNamer); ok {
		return c.CursorName()
	}
	return ""
}

// KeyAction forwards keyboard actions into the list: a press on a row
// focuses the proxy, and the list owns the keyboard selection model.
func (r *listRow) KeyAction(a KeyAction, mods Mods) { r.list.KeyAction(a, mods) }

// SelectAll forwards the ctrl+a select-all route into the list.
func (r *listRow) SelectAll() { r.list.SelectAll() }

// styleChildren are the height probe and the materialized rows in
// model order (styleKids), so sibling selectors see a stable order.
func (l *List) appendStyleKids(buf []Widget) []Widget {
	if l.probe != nil {
		buf = append(buf, l.probe)
	}
	for _, i := range slices.Sorted(maps.Keys(l.rows)) {
		buf = append(buf, l.rows[i])
	}
	return buf
}

// modelPosition implements modelPositioner: the row's place in the
// model.
func (r *listRow) modelPosition() (int, int) { return r.idx + 1, r.list.model.len() }

// appendStyleKids is the row widget, inheriting through the proxy.
func (r *listRow) appendStyleKids(buf []Widget) []Widget { return append(buf, r.row) }

// SetSingleClickActivate makes a plain click activate the row
// (OnActivate) as well as select it, GTK's single-click-activate: a
// picker's rows act on one click. Multiple-selection lists keep
// clicks for membership.
func (l *List) SetSingleClickActivate(on bool) { l.singleClick = on }

// ScrollPixels implements PixelScroller: exact vertical deltas, the
// fraction kept for the next one.
func (l *List) ScrollPixels(_, dy float64) {
	if !IsEnabled(l) {
		return
	}
	l.kin.sample(0, dy)
	l.movePixels(dy)
}

// ScrollEnd implements ScrollEnder: the list glides on from the
// gesture's velocity.
func (l *List) ScrollEnd() {
	l.kin.fling(func(_, dy float64) bool { return l.movePixels(dy) }, func() {})
}

// movePixels scrolls by a pixel delta, keeping the sub-pixel remainder;
// reports whether the offset moved (false pinned at an end).
func (l *List) movePixels(dy float64) bool {
	l.pxFrac += dy
	w := int(l.pxFrac)
	l.pxFrac -= float64(w)
	if w == 0 {
		return true
	}
	off := min(max(0, l.offY+w), l.scrollMax())
	if off == l.offY {
		return false
	}
	l.offY = off
	l.Invalidate()
	return true
}
