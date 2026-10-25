package widget

import (
	"fmt"
	"slices"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// dataTablePageSize is the default rows per page.
const dataTablePageSize = 10

// dataTableCheckWidth is the selection column's width.
const dataTableCheckWidth = 40

// DataTable is a data table on a ColumnView: a text filter
// over the rows, a Columns menu that hides and shows columns, sorting
// by the sortable columns' headers, row selection with a check column
// (the header check selecting the page), and pages of rows with a
// pager and a count of the selected rows under the table. The rows stay
// virtualized: a page of a million rows costs what a page of ten does.
// Selection is by row of the model, so it survives paging, sorting and
// filtering. It styles as the `datatable` element: the .toolbar row,
// the .table box, and the .footer row.
type DataTable[T any] struct {
	composite
	face     render.Font
	cols     []TableColumn[T]
	hidden   []bool
	view     *ColumnView[T]
	query    *Entry
	match    func(row T, query string) bool
	columns  *Button
	pager    *Pagination
	status   *Label
	headChk  *CheckButton
	pageSize int
	page     int
	selected map[int]bool
	syncing  bool

	// OnSelectionChanged fires with the selected model rows, ascending,
	// after the user changed them.
	OnSelectionChanged func(rows []int)
}

// NewDataTable returns a table over model's rows in cols; match
// decides whether a row passes the filter's text (nil hides the
// filter).
func NewDataTable[T any](face render.Font, model ColumnModel[T], cols []TableColumn[T], match func(row T, query string) bool) *DataTable[T] {
	face = requireFace("widget.NewDataTable", face)
	t := &DataTable[T]{
		face: face, cols: slices.Clone(cols), hidden: make([]bool, len(cols)),
		match: match, pageSize: dataTablePageSize, page: 1, selected: map[int]bool{},
	}
	t.SetElement("datatable")
	t.headChk = NewCheckButton(false)
	t.headChk.SetAccessibleName(Tr("Select all"))
	t.headChk.OnChanged = t.selectPage
	t.view = NewColumnView(face, 14, model, t.visibleColumns())
	t.view.List().SetSelectionMode(SelectionMultiple)
	t.view.List().OnSelectionChanged = t.listSelected
	t.view.onChanged = t.refresh
	t.view.AddClass("table")

	toolbar := NewBox(Row, 8, 0)
	toolbar.AddClass("toolbar")
	if match != nil {
		t.query = NewEntry(face, 14, 0)
		t.query.SetPlaceholder(Tr("Filter..."))
		t.query.SetWidthChars(24, 32)
		t.query.OnChanged = func(q string) { t.filter(q) }
		toolbar.AppendAligned(t.query, false, AlignCenter)
	}
	toolbar.Append(NewSpacer(0, 0), true)
	cline := NewBox(Row, 6, 0)
	cline.AppendAligned(NewLabel(face, 14, Tr("Columns"), 0), false, AlignCenter)
	cline.AppendAligned(NewSymbol(SymbolChevronDown, 14), false, AlignCenter)
	t.columns = NewButton(cline, 0, 0)
	t.columns.AddClass(css.Outline)
	t.columns.OnClick = t.openColumns
	toolbar.AppendAligned(t.columns, false, AlignCenter)

	t.status = NewLabel(face, 14, "", 0)
	t.status.AddClass(css.Muted)
	t.pager = NewPagination(face, 1, 1)
	t.pager.OnPageChanged = func(p int) { t.showPage(p) }
	footer := NewBox(Row, 8, 0)
	footer.AddClass("footer")
	footer.AppendAligned(t.status, true, AlignCenter)
	footer.AppendAligned(t.pager, false, AlignCenter)

	col := NewBox(Column, 16, 0)
	col.Append(toolbar, false).Append(t.view, true).Append(footer, false)
	t.initComposite(t, col)
	t.fillWidth = true
	t.boxed = true
	t.showPage(1)
	return t
}

// visibleColumns is the check column and the shown data columns.
func (t *DataTable[T]) visibleColumns() []TableColumn[T] {
	out := []TableColumn[T]{{
		Width: dataTableCheckWidth,
		Head:  t.headChk,
		Cell:  func(T) Widget { return NewCheckButton(false) },
	}}
	for i, c := range t.cols {
		if !t.hidden[i] {
			out = append(out, c)
		}
	}
	return out
}

// View returns the table's column view.
func (t *DataTable[T]) View() *ColumnView[T] { return t.view }

// SetPageSize shows n rows a page, at least one.
func (t *DataTable[T]) SetPageSize(n int) {
	t.pageSize = max(1, n)
	t.showPage(1)
}

// Page returns the shown page, from 1.
func (t *DataTable[T]) Page() int { return t.page }

// Selected returns the selected model rows, ascending.
func (t *DataTable[T]) Selected() []int {
	out := make([]int, 0, len(t.selected))
	for r := range t.selected {
		out = append(out, r)
	}
	slices.Sort(out)
	return out
}

// SortBy sorts by data column i (in the order the table was given
// them; descending with desc), or returns to the model's order for -1
// or a hidden or unsortable column.
func (t *DataTable[T]) SortBy(i int, desc bool) {
	at := -1
	if i >= 0 && i < len(t.cols) && !t.hidden[i] {
		at = 1
		for j := range i {
			if !t.hidden[j] {
				at++
			}
		}
	}
	t.view.SortBy(at, desc)
}

// SetColumnVisible shows or hides data column i (in the order the
// table was given them).
func (t *DataTable[T]) SetColumnVisible(i int, on bool) {
	t.hidden[i] = !on
	t.view.SetColumns(t.visibleColumns())
}

// ColumnVisible reports data column i shown.
func (t *DataTable[T]) ColumnVisible(i int) bool { return !t.hidden[i] }

// filter narrows the rows to the query and goes back to the first page.
func (t *DataTable[T]) filter(q string) {
	if q == "" {
		t.view.SetFilter(nil)
	} else {
		t.view.SetFilter(func(row T) bool { return t.match(row, q) })
	}
	t.showPage(1)
}

// pages is the page count of the matching rows.
func (t *DataTable[T]) pages() int {
	return max(1, (t.view.Matching()+t.pageSize-1)/t.pageSize)
}

// showPage windows the view onto page p.
func (t *DataTable[T]) showPage(p int) {
	t.page = min(max(1, p), t.pages())
	t.view.SetWindow((t.page-1)*t.pageSize, t.pageSize)
}

// refresh brings the pager, the page's checks and the count up to date
// after the view recomputed.
func (t *DataTable[T]) refresh() {
	if t.pager == nil {
		return
	}
	if n := t.pages(); t.pager.Count() != n || t.pager.Page() != min(t.page, n) {
		t.page = min(t.page, n)
		t.pager.SetCount(n, t.page)
	}
	var rows []int
	all := t.view.Rows() > 0
	for i := range t.view.Rows() {
		if t.selected[t.view.SourceIndex(i)] {
			rows = append(rows, i)
		} else {
			all = false
		}
	}
	t.syncing = true
	t.view.List().SelectRows(rows...)
	t.headChk.SetChecked(all)
	t.syncing = false
	t.status.SetText(fmt.Sprintf(Tr("%d of %d row(s) selected."), len(t.selected), t.view.Matching()))
}

// listSelected takes the user's selection of the page's rows.
func (t *DataTable[T]) listSelected(rows []int) {
	if t.syncing || t.view.recomputing {
		return
	}
	for i := range t.view.Rows() {
		delete(t.selected, t.view.SourceIndex(i))
	}
	for _, i := range rows {
		t.selected[t.view.SourceIndex(i)] = true
	}
	t.refresh()
	if t.OnSelectionChanged != nil {
		t.OnSelectionChanged(t.Selected())
	}
}

// selectPage selects or clears every row of the page.
func (t *DataTable[T]) selectPage(on bool) {
	if t.syncing {
		return
	}
	rows := make([]int, 0, t.view.Rows())
	if on {
		for i := range t.view.Rows() {
			rows = append(rows, i)
		}
	}
	t.view.List().SelectRows(rows...)
}

// openColumns opens the menu of data columns to show or hide.
func (t *DataTable[T]) openColumns() {
	items := make([]MenuItem, 0, len(t.cols))
	for i, c := range t.cols {
		items = append(items, MenuItem{Label: c.Title, Kind: ItemCheck, Checked: !t.hidden[i], OnClick: func() {
			t.SetColumnVisible(i, t.hidden[i])
		}})
	}
	OpenMenu(t, t.columns, t.face, 14, items, MenuOptions{Gap: PopoverGap})
}
