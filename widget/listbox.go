package widget

import (
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/widget/css"
)

// ListBox is a non-virtualized list of rows: every row is a child
// widget kept in the tree, measured and painted in order, so it is
// for short lists where a List's viewport recycling would be overhead.
// Selection is pointer and keyboard driven through the embedded
// selection model; OnSelectionChanged fires once per gesture, OnActivate
// on row activation (a double-click or a single click when
// SetSingleClickActivate is on).
type ListBox struct {
	node
	selection
	rows        []*ListBoxRow
	filterFn    func(*ListBoxRow) bool
	sortFn      func(a, b *ListBoxRow) int
	headerFn    func(*ListBoxRow) Widget
	order       []int
	hidden      []bool
	ph          Widget
	minHeight   int
	singleClick bool

	OnSelectionChanged func(rows []int)
	OnActivate         func(i int)
}

// ListBoxRow is one row in a ListBox: a child widget with an optional
// header, selectable and activatable flags, and the row's own CSS box.
type ListBoxRow struct {
	node
	box         *ListBox
	child       Widget
	header      Widget
	nat         Size
	headNat     Size
	selectable  bool
	activatable bool
}

// NewListBox returns an empty single-select ListBox styled as a
// boxed list (CSS class boxedList).
func NewListBox() *ListBox {
	l := &ListBox{singleClick: true}
	l.SetElement("list")
	l.AddClass(css.BoxedList)
	l.initSelection(l, SelectionSingle)
	return l
}

// SetSingleClickActivate toggles whether a plain click activates a
// row (on by default); off means activation needs a double-click.
func (l *ListBox) SetSingleClickActivate(on bool) { l.singleClick = on }

func (l *ListBox) count() int { return len(l.rows) }

func (l *ListBox) selectionSync() {
	for i, r := range l.rows {
		r.SetState(StateSelected, l.isSelected(i))
	}
	l.Invalidate()
}

func (l *ListBox) selectionNotify() {
	if l.OnSelectionChanged != nil {
		l.OnSelectionChanged(l.Selection())
	}
}

func (l *ListBox) selectionSingle(int) {}

func (l *ListBox) reveal(i int) {
	if r := l.RowAt(i); r != nil {
		l.revealItem(i, r)
	}
}

// item is row i as a Widget, nil when there is none.
func (l *ListBox) item(i int) Widget {
	if r := l.RowAt(i); r != nil {
		return r
	}
	return nil
}

func (l *ListBox) activate(i int) {
	if r := l.RowAt(i); r != nil && r.activatable && l.OnActivate != nil {
		l.OnActivate(i)
	}
}

// Append adds w as a new row at the end and returns the row.
func (l *ListBox) Append(w Widget) *ListBoxRow { return l.Insert(len(l.rows), w) }

// Insert adds w as a new row at position i (clamped to the ends) and
// returns the row.
func (l *ListBox) Insert(i int, w Widget) *ListBoxRow {
	r := &ListBoxRow{box: l, child: w, selectable: true, activatable: true}
	r.SetElement("row")
	r.AddClass(css.Activatable)
	r.cursorName = "pointer"
	i = min(max(i, 0), len(l.rows))
	l.rows = append(l.rows, nil)
	copy(l.rows[i+1:], l.rows[i:])
	l.rows[i] = r
	l.hidden = append(l.hidden, false)
	copy(l.hidden[i+1:], l.hidden[i:])
	l.hidden[i] = false
	l.inserted(i)
	setParents(l, r)
	setParents(r, w)
	l.recompute()
	restyleChildren(l)
	return r
}

// RemoveAt removes the row at position i, doing nothing when i is
// out of range.
func (l *ListBox) RemoveAt(i int) {
	if i < 0 || i >= len(l.rows) {
		return
	}
	r := l.rows[i]
	l.rows = append(l.rows[:i], l.rows[i+1:]...)
	l.hidden = append(l.hidden[:i], l.hidden[i+1:]...)
	l.removed(i)
	if r.header != nil {
		clearParents(r.header)
	}
	notifyRemoved(r.child)
	clearParents(r, r.child)
	r.box = nil
	l.recompute()
	restyleChildren(l)
}

// Move relocates the row at from to position to (both clamped to the
// row range), adjusting the selection to follow.
func (l *ListBox) Move(from, to int) {
	if from < 0 || from >= len(l.rows) {
		return
	}
	to = min(max(to, 0), len(l.rows)-1)
	if from == to {
		return
	}
	r := l.rows[from]
	l.rows = slices.Insert(slices.Delete(l.rows, from, from+1), to, r)
	h := l.hidden[from]
	l.hidden = slices.Insert(slices.Delete(l.hidden, from, from+1), to, h)
	l.moved(from, to)
	l.recompute()
	restyleChildren(l)
}

// Len returns the number of rows.
func (l *ListBox) Len() int { return len(l.rows) }

// RowAt returns the row at position i, or nil when i is out of range.
func (l *ListBox) RowAt(i int) *ListBoxRow {
	if i < 0 || i >= len(l.rows) {
		return nil
	}
	return l.rows[i]
}

// Children returns the rows in order, then the placeholder: what tree
// walks (the restyle pass, focus traversal, the accessibility bridge)
// descend into, as for every container.
func (l *ListBox) Children() []Widget {
	out := make([]Widget, 0, len(l.rows)+1)
	for _, r := range l.rows {
		out = append(out, r)
	}
	if l.ph != nil {
		out = append(out, l.ph)
	}
	return out
}

// Rows returns every row widget in order.
func (l *ListBox) Rows() []Widget {
	out := make([]Widget, len(l.rows))
	for i, r := range l.rows {
		out[i] = r
	}
	return out
}

// SetMinHeight floors the height the list measures to, in logical
// pixels: a filter that hides rows keeps the box's extent, so the
// layout around it never moves.
func (l *ListBox) SetMinHeight(h int) {
	if h == l.minHeight {
		return
	}
	l.minHeight = h
	l.InvalidateLayout()
}

// SetPlaceholder sets a widget shown when no row is visible, replacing
// any previous placeholder.
func (l *ListBox) SetPlaceholder(w Widget) {
	if l.ph != nil {
		clearParents(l.ph)
	}
	l.ph = w
	if w != nil {
		setParents(l, w)
	}
	l.InvalidateLayout()
}

// SetFilterFunc installs a filter: rows where keep returns false are
// hidden, and the list is recomputed. A nil keep clears the filter.
func (l *ListBox) SetFilterFunc(keep func(*ListBoxRow) bool) {
	l.filterFn = keep
	l.recompute()
}

// SetSortFunc installs a sort comparator and recomputes the row
// order. A nil cmp clears the sort, restoring insertion order.
func (l *ListBox) SetSortFunc(cmp func(a, b *ListBoxRow) int) {
	l.sortFn = cmp
	l.recompute()
}

// SetHeaderFunc installs a header builder: fn is called per row to
// produce its header widget (nil for none), and the headers are
// rerun. A nil fn clears all headers.
func (l *ListBox) SetHeaderFunc(fn func(*ListBoxRow) Widget) {
	l.headerFn = fn
	l.rerunHeaders()
}

// InvalidateFilter re-runs the filter and relayouts.
func (l *ListBox) InvalidateFilter() { l.recompute() }

// InvalidateSort re-runs the sort and relayouts.
func (l *ListBox) InvalidateSort() { l.recompute() }

// InvalidateHeaders reruns every row's header builder and relayouts.
func (l *ListBox) InvalidateHeaders() { l.rerunHeaders() }

func (l *ListBox) recompute() {
	order := make([]int, 0, len(l.rows))
	for i, r := range l.rows {
		keep := l.filterFn == nil || l.filterFn(r)
		l.hidden[i] = !keep
		if keep {
			order = append(order, i)
		}
	}
	if l.sortFn != nil {
		slices.SortStableFunc(order, func(a, b int) int { return l.sortFn(l.rows[a], l.rows[b]) })
	}
	l.order = order
	l.dropHiddenSelection()
	l.InvalidateLayout()
}

func (l *ListBox) dropHiddenSelection() {
	before := l.Selection()
	switch l.mode {
	case SelectionMultiple:
		set := make(map[int]struct{}, len(l.multi))
		for i := range l.multi {
			if !l.hidden[i] {
				set[i] = struct{}{}
			}
		}
		l.applyMulti(set)
	case SelectionBrowse:
		if l.sel >= 0 && l.hidden[l.sel] {
			l.sel = -1
			for _, i := range l.order {
				l.sel = i
				break
			}
		}
	case SelectionSingle:
		if l.sel >= 0 && l.hidden[l.sel] {
			l.sel = -1
		}
	}
	l.selectionSync()
	if after := l.Selection(); !slices.Equal(before, after) {
		l.notify()
	}
}

func (l *ListBox) rerunHeaders() {
	for _, r := range l.rows {
		if r.header != nil {
			clearParents(r.header)
		}
		r.header = nil
		if l.headerFn != nil {
			if h := l.headerFn(r); h != nil {
				r.header = h
				setParents(r, h)
			}
		}
	}
	l.InvalidateLayout()
}

func (l *ListBox) anyVisible() bool {
	for _, i := range l.order {
		if IsVisible(l.rows[i]) {
			return true
		}
	}
	return false
}

func (l *ListBox) placeholderShown() bool {
	return l.ph != nil && !l.anyVisible() && IsVisible(l.ph)
}

func (l *ListBox) indexAt(p Point) int {
	for _, i := range l.order {
		if IsVisible(l.rows[i]) && l.rows[i].bounds.Contains(p.X, p.Y) {
			return i
		}
	}
	return -1
}

func (l *ListBox) rowClick(i int) {
	r := l.RowAt(i)
	if r == nil {
		return
	}
	if r.selectable {
		l.click(i)
	}
	if l.singleClick && l.mode != SelectionMultiple && r.activatable {
		l.activate(i)
	}
}

// KeyAction forwards a keyboard action to the selection model when
// the list is enabled.
func (l *ListBox) KeyAction(a KeyAction, mods Mods) {
	if IsEnabled(l) {
		l.key(a, mods, l.navigate)
	}
}

func (l *ListBox) navigate(from int, a KeyAction) (int, bool) {
	if a != KeyUp && a != KeyDown {
		return 0, false
	}
	navigable := func(i int) bool { return !l.hidden[i] && l.rows[i].selectable }
	pos := slices.Index(l.order, from)
	if pos < 0 {
		for _, i := range l.order {
			if navigable(i) {
				return i, true
			}
		}
		return 0, false
	}
	step := -1
	if a == KeyDown {
		step = 1
	}
	for p := pos + step; p >= 0 && p < len(l.order); p += step {
		if navigable(l.order[p]) {
			return l.order[p], true
		}
	}
	return from, true
}

// Measure reports the offered width by the total height of the
// visible rows (or the placeholder), clamped to the constraints.
func (l *ListBox) Measure(con Constraints) Size {
	if sz, ok := l.measureHit(con); ok {
		return sz
	}
	v := l.style(l)
	box := boxOf(v, render.Insets{})
	return l.measureStore(con, measureBox(v, box, con, func(inner Constraints) Size {
		var out Size
		for _, i := range l.order {
			r := l.rows[i]
			r.nat = measureChild(l, r, Constraints{Max: inner.Max})
			if IsVisible(r) {
				out.W = max(out.W, r.nat.W)
				out.H += r.nat.H
			}
		}
		if l.placeholderShown() {
			out = measureChild(l, l.ph, Constraints{Max: inner.Max})
		}
		out.H = max(out.H, l.minHeight)
		return clampSize(out, inner)
	}))
}

// Arrange lays out the visible rows top to bottom inside the box.
func (l *ListBox) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(l.style(l), render.Insets{}), r)
	l.node.Arrange(border)
	y := inner.Y
	for _, i := range l.order {
		r := l.rows[i]
		if !IsVisible(r) {
			continue
		}
		r.Arrange(render.Rect{X: inner.X, Y: y, W: inner.W, H: r.nat.H})
		setParents(l, r)
		y += r.nat.H
	}
	if l.placeholderShown() {
		arrangeChild(l.ph, inner)
	}
	l.revealArranged(l.item)
}

// Paint draws the box background, the row separators, and the
// visible rows in order.
func (l *ListBox) Paint(cv *render.Canvas) {
	paintCSSBox(cv, l.style(l), l.bounds)
	th := Current()
	prev := false
	for _, i := range l.order {
		r := l.rows[i]
		if !IsVisible(r) {
			continue
		}
		if prev && (r.header == nil || !IsVisible(r.header)) {
			b := r.bounds
			cv.FillRect(render.Rect{X: b.X + 8, Y: b.Y, W: b.W - 16, H: 1}, th.Border)
		}
		prev = true
		PaintChild(cv, r)
	}
	if l.placeholderShown() {
		PaintChild(cv, l.ph)
	}
}

// HitTest returns the topmost visible row under p, or the
// placeholder when it is shown, or the list itself.
func (l *ListBox) HitTest(p Point) Widget {
	for _, i := range l.order {
		r := l.rows[i]
		if !IsVisible(r) {
			continue
		}
		if hit := r.HitTest(p); hit != nil {
			return hit
		}
	}
	if l.placeholderShown() {
		return l.HitLeaf(l.ph, p)
	}
	return l.HitLeaf(l, p)
}

// Child returns the row's content widget.
func (r *ListBoxRow) Child() Widget { return r.child }

// Header returns the row's header widget, or nil when it has none.
func (r *ListBoxRow) Header() Widget { return r.header }

// Index returns the row's position in its ListBox, or -1 when
// detached.
func (r *ListBoxRow) Index() int {
	if r.box != nil {
		return slices.Index(r.box.rows, r)
	}
	return -1
}

// ChildVisible reports whether the list shows the row: false while its
// filter hides it (gtk_widget_get_child_visible), true when detached.
func (r *ListBoxRow) ChildVisible() bool {
	i := r.Index()
	return i < 0 || !r.box.hidden[i]
}

// SetSelectable toggles whether the row responds to selection
// gestures.
func (r *ListBoxRow) SetSelectable(on bool) { r.selectable = on }

// Selectable reports whether the row responds to selection gestures.
func (r *ListBoxRow) Selectable() bool { return r.selectable }

// SetActivatable toggles whether the row can be activated (firing
// the list's OnActivate); it also sets the cursor and activatable CSS
// class accordingly.
func (r *ListBoxRow) SetActivatable(on bool) {
	if r.activatable == on {
		return
	}
	r.activatable = on
	r.cursorName = ""
	if on {
		r.cursorName = "pointer"
		r.AddClass(css.Activatable)
	} else {
		r.RemoveClass(css.Activatable)
	}
	r.Invalidate()
}

// Activatable reports whether the row can be activated.
func (r *ListBoxRow) Activatable() bool { return r.activatable }

// Measure reports the offered width by the header plus child
// natural height, clamped to the constraints.
func (r *ListBoxRow) Measure(con Constraints) Size {
	if sz, ok := r.measureHit(con); ok {
		return sz
	}
	v := r.style(r)
	box := boxOf(v, render.Insets{})
	return r.measureStore(con, measureBox(v, box, con, func(inner Constraints) Size {
		var out Size
		r.headNat = Size{}
		if r.header != nil {
			r.headNat = measureChild(r, r.header, Constraints{Max: inner.Max})
			if !IsVisible(r.header) {
				r.headNat.H = 0
			}
			out.W = max(out.W, r.headNat.W)
			out.H += r.headNat.H
		}
		c := measureChild(r, r.child, inner)
		out.W = max(out.W, c.W)
		out.H += c.H
		return clampSize(out, inner)
	}))
}

// Arrange lays out the header (if any) on top and the child below it.
func (r *ListBoxRow) Arrange(rct render.Rect) {
	border, inner := boxRects(boxOf(r.style(r), render.Insets{}), rct)
	r.node.Arrange(border)
	if r.header != nil && r.headNat.H > 0 {
		arrangeChild(r.header, render.Rect{X: inner.X, Y: inner.Y, W: inner.W, H: r.headNat.H})
		inner.Y += r.headNat.H
		inner.H -= r.headNat.H
	}
	arrangeChild(r.child, inner)
}

// Paint draws the selected tint when the row is selected and the
// stylesheet did not set a background, then the box, header, and child.
func (r *ListBoxRow) Paint(cv *render.Canvas) {
	v := r.style(r)
	if r.HasState(StateSelected) && !v.Declares(style.PropBackgroundColor) {
		cv.RoundedRect(r.bounds, radiusOr(v, 6).TopLeft, selectedTint())
	}
	paintCSSBox(cv, v, r.bounds)
	if r.header != nil && r.headNat.H > 0 {
		PaintChild(cv, r.header)
	}
	PaintChild(cv, r.child)
}

// HitTest returns the child or header widget under p when it can
// handle the hit, or the row itself.
func (r *ListBoxRow) HitTest(p Point) Widget {
	if hit := r.child.HitTest(p); hit != nil && (r.box == nil || r.box.mode == SelectionNone || !r.selectable || interactiveWithin(hit, r)) {
		return hit
	}
	if r.header != nil && r.headNat.H > 0 {
		if hit := r.header.HitTest(p); hit != nil {
			return hit
		}
	}
	return r.HitLeaf(r, p)
}

// ClickAt forwards a click to the list's row click handler.
func (r *ListBoxRow) ClickAt(Point) { r.box.rowClick(r.Index()) }

// DoubleClickAt activates the row when single-click activation is off.
func (r *ListBoxRow) DoubleClickAt(Point) {
	if !r.box.singleClick && r.box.mode != SelectionNone && r.activatable {
		r.box.activate(r.Index())
	}
}

// SetPressed holds the list's selection press while on.
func (r *ListBoxRow) SetPressed(on bool) {
	if on {
		r.box.hold()
	}
}

// DragMove moves the selection to the row under p in single and
// browse mode (pointer-follows-selection).
func (r *ListBoxRow) DragMove(p Point) {
	b := r.box
	if b.mode != SelectionSingle && b.mode != SelectionBrowse {
		return
	}
	if at := b.indexAt(p); at >= 0 && at != b.sel {
		b.selectOne(at)
	}
}

// PressEnd releases the list's selection press.
func (r *ListBoxRow) PressEnd() { r.box.release() }

// KeyAction forwards a keyboard action to the list's selection model.
func (r *ListBoxRow) KeyAction(a KeyAction, mods Mods) { r.box.KeyAction(a, mods) }

// SelectAll selects every row through the list's selection model.
func (r *ListBoxRow) SelectAll() { r.box.SelectAll() }

// Children returns the header (if any) and the child widget.
func (r *ListBoxRow) Children() []Widget {
	if r.header == nil {
		return []Widget{r.child}
	}
	return []Widget{r.header, r.child}
}
