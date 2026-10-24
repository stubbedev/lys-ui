package widget

import (
	"slices"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/transfer"
	"github.com/stubbedev/lys-ui/widget/css"
)

// ColumnView is the virtualized table (#90): typed columns over a
// ColumnModel, a header row with sort indicators, sort and filter in
// one stable permutation, tree indentation through the tree helpers,
// and optional row drag reordering. List underneath does what List is
// for - virtualization, selection, keyboard - through the same model
// adapter, so a million rows cost about what twenty do.
//
// The model layer is the GIO list-model analog, shared with the
// typed views (#166): sorting and a stack of toggleable filters never
// touch the source model, they live in a permutation of its row
// indices - stable (equal rows keep source order), recomputed on
// SortBy/SetFilter/SetModel/AddFilter/SetFilterStatus/PopFilter/
// ClearFilters, and the source's own order is one call away.
type ColumnView[T any] struct {
	composite
	face   render.Font
	sizePx float64

	cols   []TableColumn[T]
	vm     viewModel[T]
	search *ViewFilter[T]
	sortBy int
	sortUp bool

	header *Box
	list   *List
	rows   *columnRows[T]

	// OnRowReorder, when set, receives drag-and-drop row moves: from
	// and to are visible indices (post sort/filter), fired on the loop
	// goroutine. The application performs the move in its model - the
	// view never mutates data it does not own.
	OnRowReorder func(from, to int)
	dragRows     bool

	// winOff and winLen are the window of the sorted, filtered rows the
	// view shows (SetWindow); winLen 0 shows them all.
	winOff, winLen int
	// onChanged hears every recompute: a data table resyncing its
	// selection and pager after a sort.
	onChanged func()
	// recomputing reports a recompute in progress: the list's reset
	// clears its selection then, which is not the user's doing.
	recomputing bool
}

// TableColumn is one typed table column: how a cell renders, how wide
// it sits, and whether the header sorts by it.
type TableColumn[T any] struct {
	Title string
	// Width is the cell's minimum width (the header's too, so columns
	// line up); Expand shares leftover table width across the
	// expanding columns, Box-style.
	Width  int
	Expand bool
	// Cell builds the cell's widget for one row value; nil renders the
	// zero label.
	Cell func(T) Widget
	// Sort, when non-nil, makes the header clickable and orders rows:
	// negative puts a first. Equal values keep source order (stable).
	Sort func(a, b T) int
	// Head, when set, is the header cell instead of Title's button: a
	// select-all check.
	Head Widget
}

// ColumnModel is the table's data source, ListModel's typed sibling.
type ColumnModel[T any] interface {
	Len() int
	Row(i int) T
}

// SliceModel adapts a slice as a ColumnModel.
type SliceModel[T any] []T

// Len implements ColumnModel.
func (s SliceModel[T]) Len() int { return len(s) }

// Row implements ColumnModel.
func (s SliceModel[T]) Row(i int) T { return s[i] }

// NewColumnView returns a view over model with the given columns.
func NewColumnView[T any](face render.Font, sizePx float64, model ColumnModel[T], cols []TableColumn[T]) *ColumnView[T] {
	face = requireFace("widget.NewColumnView", face)
	v := &ColumnView[T]{
		face: face, sizePx: sizePx,
		cols: cols, sortBy: -1,
	}
	v.vm.setModel(model)
	v.rows = &columnRows[T]{view: v}
	v.list = NewList(v.rows, int(sizePx)+12)
	v.header = NewBox(Row, 0, 0)
	v.header.AddClass("header")
	column := NewBox(Column, 0, 0)
	column.Append(v.header, false)
	column.Append(v.list, true)
	v.initComposite(v, column)
	v.boxed = true
	v.SetElement("columnview")
	v.buildHeader()
	v.recompute()
	return v
}

// List exposes the underlying list - selection, activation, keyboard,
// scrolling - the same surface a plain List gives.
func (v *ColumnView[T]) List() *List { return v.list }

// Row returns the data of visible row i (post sort, filter and
// window).
func (v *ColumnView[T]) Row(i int) T {
	return v.vm.at(i + v.winOff)
}

// Rows reports the visible row count: the window's share of the
// matching rows.
func (v *ColumnView[T]) Rows() int {
	n := max(0, v.vm.len()-v.winOff)
	if v.winLen > 0 {
		n = min(n, v.winLen)
	}
	return n
}

// Matching reports how many rows pass the filters, the window aside.
func (v *ColumnView[T]) Matching() int { return v.vm.len() }

// SourceIndex maps visible row i to its row in the model, -1 when out
// of range.
func (v *ColumnView[T]) SourceIndex(i int) int {
	if i < 0 || i >= v.Rows() {
		return -1
	}
	return v.vm.source(i + v.winOff)
}

// SetWindow shows only limit of the sorted, filtered rows from offset
// on - a page of them; limit 0 shows them all.
func (v *ColumnView[T]) SetWindow(offset, limit int) {
	v.winOff, v.winLen = max(0, offset), max(0, limit)
	v.recompute()
}

// SetColumns replaces the columns: a column toggled visible or hidden.
// The sort stays on the column it was on when that column remains.
func (v *ColumnView[T]) SetColumns(cols []TableColumn[T]) {
	if v.sortBy >= len(cols) {
		v.sortBy = -1
	}
	v.cols = cols
	v.buildHeader()
	v.recompute()
}

// SetModel replaces the data source and drops the sort/filter
// permutation back to the source order.
func (v *ColumnView[T]) SetModel(m ColumnModel[T]) {
	v.vm.setModel(m)
	v.sortBy = -1
	v.buildHeader()
	v.recompute()
}

// SortBy sorts by column col (descending with desc); col -1 returns
// to the source order. A column without a Sort function clears the
// sort instead.
func (v *ColumnView[T]) SortBy(col int, desc bool) {
	if col < 0 || col >= len(v.cols) || v.cols[col].Sort == nil {
		v.sortBy = -1
		v.sortUp = false
	} else {
		v.sortBy = col
		v.sortUp = !desc
	}
	v.buildHeader()
	v.recompute()
}

// SortedBy reports the active sort column (-1 when none) and its
// direction.
func (v *ColumnView[T]) SortedBy() (col int, ascending bool) { return v.sortBy, v.sortUp }

// SetFilter narrows the view to rows keep accepts (nil clears); the
// source model is untouched and the sort stays. It is one slot in the
// filter stack: AddFilter/SetFilterStatus/PopFilter/ClearFilters
// manage the rest alongside it.
func (v *ColumnView[T]) SetFilter(keep func(T) bool) {
	switch {
	case keep == nil:
		v.vm.setFilterStatus(v.search, false)
	case v.search == nil:
		v.search = v.vm.addFilter(keep)
	default:
		v.search.keep = keep
		v.search.on = true
	}
	v.recompute()
}

// AddFilter appends a filter to the stack and returns its handle;
// every enabled filter must accept a row for it to show.
func (v *ColumnView[T]) AddFilter(keep func(T) bool) *ViewFilter[T] {
	f := v.vm.addFilter(keep)
	v.recompute()
	return f
}

// SetFilterStatus turns a filter AddFilter returned on or off without
// dropping it.
func (v *ColumnView[T]) SetFilterStatus(f *ViewFilter[T], on bool) {
	v.vm.setFilterStatus(f, on)
	v.recompute()
}

// PopFilter drops the most recently added filter.
func (v *ColumnView[T]) PopFilter() {
	v.vm.popFilter()
	v.recompute()
}

// ClearFilters drops every filter, SetFilter's slot included.
func (v *ColumnView[T]) ClearFilters() {
	v.vm.clearFilters()
	v.search = nil
	v.recompute()
}

// EnableRowDrag turns row drag-and-drop on: rows become drag sources
// offering the internal reorder mime, and a row dropped on another
// fires OnRowReorder with the two visible indices.
func (v *ColumnView[T]) EnableRowDrag(on bool) {
	v.dragRows = on
	v.recompute()
}

// buildHeader rebuilds the header row: one button per column, the
// active sort column carrying its arrow, every cell held to its
// column's width so rows and headers line up.
func (v *ColumnView[T]) buildHeader() {
	v.header.Clear()
	for i, col := range v.cols {
		if col.Head != nil {
			head := NewBox(Row, 0, 0)
			head.Append(NewSpacer(headerInset, 0), false)
			head.AppendAligned(col.Head, false, AlignCenter)
			v.header.AppendAligned(newSizedCell(head, col.Width), col.Expand, AlignFill)
			continue
		}
		title := NewBox(Row, 4, 0)
		name := NewLabel(v.face, v.sizePx-1, col.Title, 0)
		name.AddClass(css.DimLabel)
		title.AppendAligned(name, false, AlignCenter)
		if i == v.sortBy {
			arrow := SymbolChevronDown
			if v.sortUp {
				arrow = SymbolChevronUp
			}
			title.AppendAligned(NewSymbol(arrow, int(v.sizePx)), false, AlignCenter)
		}
		btn := NewButton(title, headerInset, 2)
		if col.Sort != nil {
			which := i
			btn.OnClick = func() { v.cycleSort(which) }
		}
		v.header.AppendAligned(newSizedCell(btn, col.Width), col.Expand, AlignFill)
	}
}

// cycleSort walks a column through ascending, descending, off.
func (v *ColumnView[T]) cycleSort(col int) {
	switch {
	case v.sortBy != col:
		v.SortBy(col, false)
	case v.sortUp:
		v.SortBy(col, true)
	default:
		v.SortBy(-1, false)
	}
}

// recompute rebuilds the permutation and tells the List.
func (v *ColumnView[T]) recompute() {
	var cmp func(a, b T) int
	if v.sortBy >= 0 && v.sortBy < len(v.cols) && v.cols[v.sortBy].Sort != nil && v.vm.src != nil {
		less := v.cols[v.sortBy].Sort
		desc := !v.sortUp
		cmp = func(a, b T) int {
			c := less(a, b)
			if desc {
				return -c
			}
			return c
		}
	}
	v.recomputing = true
	defer func() { v.recomputing = false }()
	v.vm.setSort(cmp)
	v.vm.rebuild()
	v.rows.set()
	v.list.Reset()
	v.InvalidateLayout()
	if v.onChanged != nil {
		v.onChanged()
	}
}

// columnRows adapts the view as a ListModel: each visible row builds a
// horizontal Box of cells, draggable when row drag is on.
type columnRows[T any] struct {
	view  *ColumnView[T]
	built []Widget
}

// Len implements ListModel.
func (r *columnRows[T]) Len() int { return r.view.Rows() }

// Row implements ListModel; rows are built once per permutation and
// cached until the next recompute.
func (r *columnRows[T]) Row(i int) Widget {
	if i < 0 || i >= r.view.vm.len() {
		return nil
	}
	for len(r.built) < i+1 {
		r.built = append(r.built, nil)
	}
	if r.built[i] == nil {
		r.built[i] = r.view.buildRow(i)
	}
	return r.built[i]
}

// set drops the row cache on recompute.
func (r *columnRows[T]) set() { r.built = r.built[:0] }

// buildRow renders visible row i: the cells in column order, widths
// held by the same spacer trick the header uses, draggable rows
// wrapped in the reorder source.
func (v *ColumnView[T]) buildRow(i int) Widget {
	val := v.Row(i)
	row := NewBox(Row, 0, 0)
	for _, col := range v.cols {
		var cell Widget = NewLabel(v.face, v.sizePx, "", 0)
		if col.Cell != nil {
			cell = col.Cell(val)
		}
		// The inset matches the header button's padding, so a cell's
		// content starts under its column title.
		inner := NewBox(Row, 0, 0)
		inner.Append(NewSpacer(headerInset, 0), false)
		inner.AppendAligned(cell, false, AlignCenter)
		row.AppendAligned(newSizedCell(inner, col.Width), col.Expand, AlignFill)
	}
	if !v.dragRows {
		return row
	}
	return &dragRow[T]{Box: row, view: v, at: i}
}

// headerInset is the header buttons' padding and the row cells' inset.
const headerInset = 6

// rowReorderMime is the internal drag mime for row moves.
const rowReorderMime = "application/x-lys-ui-row"

// dragRow makes a table row a drag source and drop target for
// reordering: dragging offers the row's visible index, dropping on
// another row fires the view's OnRowReorder.
type dragRow[T any] struct {
	*Box
	view *ColumnView[T]
	at   int
}

// DragContent implements DragSource.
func (d *dragRow[T]) DragContent() *DragContent {
	return &DragContent{
		Content: transfer.Bytes(rowReorderMime, []byte(rowKey(d.at))),
		Actions: transfer.ActionMove,
	}
}

// DragEnter implements DragEnterer.
func (d *dragRow[T]) DragEnter(mimes []string, _ Point) string {
	if slices.Contains(mimes, rowReorderMime) {
		return rowReorderMime
	}
	return ""
}

// Drop implements Dropper: the move lands as visible indices.
func (d *dragRow[T]) Drop(mime string, data []byte, _ Point) {
	if mime != rowReorderMime || d.view.OnRowReorder == nil {
		return
	}
	from := parseRowKey(string(data))
	if from >= 0 {
		d.view.OnRowReorder(from, d.at)
	}
}

// rowKey encodes a visible index as the drag payload.
func rowKey(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [12]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// parseRowKey decodes one; anything else is -1.
func parseRowKey(s string) int {
	if s == "" {
		return -1
	}
	n := 0
	for i := 0; i < len(s); i++ { //nolint:intrange // the index dance is the point here
		if s[i] < '0' || s[i] > '9' {
			return -1
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}
