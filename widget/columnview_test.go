package widget

import (
	"strconv"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// viewFace builds the test font for the table widgets.
func viewFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// tableRow is the demo/test row shape.
type tableRow struct {
	Name  string
	Count int
}

func tableCols(face render.Font) []TableColumn[tableRow] {
	th := DarkTheme()
	return []TableColumn[tableRow]{
		{
			Title: "Name", Width: 90, Expand: true,
			Cell: func(r tableRow) Widget { return NewLabel(face, 14, r.Name, th.Text) },
			Sort: func(a, b tableRow) int { return strings.Compare(a.Name, b.Name) },
		},
		{
			Title: "Count", Width: 60,
			Cell: func(r tableRow) Widget { return NewLabel(face, 14, strconv.Itoa(r.Count), th.TextMuted) },
			Sort: func(a, b tableRow) int { return a.Count - b.Count },
		},
	}
}

// TestColumnViewSortAndFilter pins the model layer: the permutation is
// stable (equal rows keep source order), sorting is descending on the
// second cycle, -1 returns to source order, and the filter narrows
// without touching the source indices' data.
func TestColumnViewSortAndFilter(t *testing.T) {
	face := viewFace(t)
	data := SliceModel[tableRow]{{"bravo", 2}, {"alpha", 1}, {"alpine", 1}, {"charlie", 3}}
	v := NewColumnView(face, 14, data, tableCols(face))

	if v.Rows() != 4 {
		t.Fatalf("rows = %d", v.Rows())
	}
	v.SortBy(1, false) // count ascending, stable within equal counts
	if got := v.Row(0).Name; got != "alpha" {
		t.Errorf("first ascending row = %q, want alpha (source order among equals)", got)
	}
	if got := v.Row(1).Name; got != "alpine" {
		t.Errorf("second ascending row = %q, want alpine (stability)", got)
	}
	v.SortBy(1, true) // descending
	if got := v.Row(0).Name; got != "charlie" {
		t.Errorf("first descending row = %q", got)
	}
	v.SetFilter(func(r tableRow) bool { return r.Count == 1 })
	if v.Rows() != 2 {
		t.Fatalf("filtered rows = %d, want 2", v.Rows())
	}
	if got := v.Row(0).Name; got != "alpine" && got != "alpha" {
		t.Errorf("filtered row = %q", got)
	}
	v.SetFilter(nil)
	v.SortBy(-1, false)
	if got := v.Row(0).Name; got != "bravo" {
		t.Errorf("source order not restored: %q", got)
	}
}

// TestColumnViewFilterStack pins the shared filter stack: the old
// SetFilter slot and filters added alongside it compose, toggle, pop
// and clear together.
func TestColumnViewFilterStack(t *testing.T) {
	face := viewFace(t)
	data := SliceModel[tableRow]{{"a", 1}, {"b", 2}, {"ab", 1}, {"c", 3}}
	v := NewColumnView(face, 14, data, tableCols(face))
	v.SetFilter(func(r tableRow) bool { return r.Count != 3 })
	if v.Rows() != 3 {
		t.Fatalf("set filter left %d rows, want 3", v.Rows())
	}
	a := v.AddFilter(func(r tableRow) bool { return strings.HasPrefix(r.Name, "a") })
	if v.Rows() != 2 {
		t.Fatalf("stacked filters left %d rows, want 2", v.Rows())
	}
	v.SetFilterStatus(a, false)
	if v.Rows() != 3 {
		t.Fatalf("a toggled-off filter still narrowed to %d rows", v.Rows())
	}
	v.SetFilterStatus(a, true)
	v.PopFilter()
	if v.Rows() != 3 {
		t.Fatalf("pop touched the SetFilter slot: %d rows", v.Rows())
	}
	v.SetFilter(nil)
	if v.Rows() != 4 {
		t.Fatalf("clearing the slot left %d rows, want 4", v.Rows())
	}
	v.SetFilter(func(r tableRow) bool { return r.Count == 1 })
	v.ClearFilters()
	if v.Rows() != 4 {
		t.Fatalf("clear filters left %d rows, want 4", v.Rows())
	}
	v.SetFilter(func(r tableRow) bool { return r.Count == 2 })
	if v.Rows() != 1 || v.Row(0).Name != "b" {
		t.Fatalf("the slot came back after a clear: %d rows", v.Rows())
	}
}

// TestColumnViewCycleSort pins the header walk: a sortable column
// cycles ascending, descending, off; a sortless column clears.
func TestColumnViewCycleSort(t *testing.T) {
	face := viewFace(t)
	v := NewColumnView(face, 14, SliceModel[tableRow]{{"b", 2}, {"a", 1}}, tableCols(face))

	v.cycleSort(0)
	if col, up := v.SortedBy(); col != 0 || !up {
		t.Errorf("first cycle = %d %v, want 0 ascending", col, up)
	}
	v.cycleSort(0)
	if col, up := v.SortedBy(); col != 0 || up {
		t.Errorf("second cycle = %d %v, want 0 descending", col, up)
	}
	v.cycleSort(0)
	if col, _ := v.SortedBy(); col != -1 {
		t.Errorf("third cycle = %d, want off", col)
	}
	v.SortBy(0, false)
	v.cycleSort(5) // a column that does not exist clears
	if col, _ := v.SortedBy(); col != -1 {
		t.Errorf("unknown column cycle = %d, want off", col)
	}
}

// TestColumnViewVirtualizationBounds pins the virtualization claim at
// the model seam: a hundred-thousand-row table builds only the rows
// the List asks for - the adapter never prebuilds.
func TestColumnViewVirtualizationBounds(t *testing.T) {
	face := viewFace(t)
	big := make(SliceModel[tableRow], 100_000)
	for i := range big {
		big[i] = tableRow{Name: "row" + strconv.Itoa(i), Count: i}
	}
	v := NewColumnView(face, 14, big, tableCols(face))
	if v.Rows() != 100_000 {
		t.Fatalf("rows = %d", v.Rows())
	}
	built := 0
	for i := range 25 { // a viewport's worth
		if v.rows.Row(i) != nil {
			built++
		}
	}
	if built != 25 || len(v.rows.built) > 25 {
		t.Errorf("adapter built %d widgets for 25 asked (cache %d)", built, len(v.rows.built))
	}
}

// TestViewModel pins the shared sort/filter layer directly: identity
// and stability, sorting, one filter, and a toggleable stack of them.
func TestViewModel(t *testing.T) {
	var vm viewModel[tableRow]
	vm.setModel(SliceModel[tableRow]{{"b", 2}, {"a", 1}, {"c", 1}})
	vm.rebuild()
	if vm.len() != 3 || vm.source(2) != 2 {
		t.Fatalf("identity order = %v", vm.order)
	}
	vm.setSort(func(a, b tableRow) int { return a.Count - b.Count })
	vm.rebuild()
	if vm.source(0) != 1 || vm.source(1) != 2 { // a before c: stable
		t.Fatalf("sorted order = %v", vm.order)
	}
	odd := vm.addFilter(func(r tableRow) bool { return r.Count%2 == 1 })
	vm.rebuild()
	if vm.len() != 2 || vm.visible(0) != -1 {
		t.Fatalf("filtered order = %v", vm.order)
	}
	vm.setFilterStatus(odd, false)
	big := vm.addFilter(func(r tableRow) bool { return r.Count > 1 })
	vm.rebuild()
	if vm.len() != 1 { // odd off: only big narrows
		t.Fatalf("toggled-off filter still narrowed: %v", vm.order)
	}
	vm.setFilterStatus(big, true)
	vm.setFilterStatus(odd, true)
	vm.rebuild()
	if vm.len() != 0 {
		t.Fatalf("composed filters kept %d rows", vm.len())
	}
	vm.popFilter()
	vm.rebuild()
	if vm.len() != 2 || vm.at(0).Name != "a" {
		t.Fatalf("after pop order = %v", vm.order)
	}
	vm.clearFilters()
	vm.setSort(nil)
	vm.rebuild()
	if vm.len() != 3 || vm.at(0).Name != "b" {
		t.Fatalf("source order not restored: %v", vm.order)
	}
}

// TestFlatTree pins the tree flattening: pre-order visible rows with
// depths, the all-expanded default, collapse hiding descendants,
// re-expand restoring them, and leaves refusing to toggle.
func TestFlatTree(t *testing.T) {
	roots := []TreeRow[string]{
		{Value: "a", Children: []TreeRow[string]{
			{Value: "a1"},
			{Value: "a2", Children: []TreeRow[string]{{Value: "a2x"}}},
		}},
		{Value: "b"},
	}
	f := NewFlatTree(roots)
	if f.Len() != 5 {
		t.Fatalf("expanded rows = %d, want 5", f.Len())
	}
	if f.Row(1).Value != "a1" || f.Row(1).Depth != 1 || f.Row(2).Value != "a2" || f.Row(3).Depth != 2 {
		t.Fatalf("pre-order flattening wrong: %+v %+v", f.Row(1), f.Row(2))
	}

	f.Toggle(2) // collapse a2
	if f.Len() != 4 {
		t.Fatalf("after collapse rows = %d, want 4", f.Len())
	}
	if f.Row(3).Value != "b" {
		t.Errorf("collapse did not hide descendants: %+v", f.Row(3))
	}
	f.Toggle(2)
	if f.Len() != 5 || f.Row(3).Value != "a2x" {
		t.Errorf("re-expand lost the subtree: %d rows, row3 = %+v", f.Len(), f.Row(3))
	}
	f.Toggle(3) // a leaf
	if f.Len() != 5 {
		t.Error("a leaf toggle changed the tree")
	}
}

// TestFlatTreeNotify pins the refresh hook: the expander's toggle runs
// the installed refresh, and the rows re-flattened underneath it.
func TestFlatTreeNotify(t *testing.T) {
	f := NewFlatTree([]TreeRow[int]{{Value: 1, Children: []TreeRow[int]{{Value: 2}}}})
	fired := 0
	f.SetRefresh(func() {
		fired++
		if f.Len() != 1 {
			t.Errorf("refresh saw %d rows", f.Len())
		}
	})
	f.Toggle(0)
	f.Notify()
	if fired != 1 {
		t.Errorf("refresh fired %d times", fired)
	}
}

// TestGoldenColumnView pins the table's painted look: header with the
// sort arrow, aligned columns, and the tree column's indent guides.
func TestGoldenColumnView(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	data := SliceModel[tableRow]{{"bravo", 2}, {"alpha", 1}}
	v := NewColumnView(face, 14, data, tableCols(face))
	v.SortBy(0, false)
	NewGolden(t, v, "columnview-sorted", goldenTheme(th), goldenFrame(220, 120))

	tree := NewFlatTree([]TreeRow[string]{
		{Value: "home", Children: []TreeRow[string]{{Value: "docs"}, {Value: "pics"}}},
		{Value: "etc"},
	})
	tv := NewColumnView(face, 14, tree, []TableColumn[FlatRow[string]]{{
		Title:  "Name",
		Expand: true,
		Cell: TreeCell(tree, 14, func(s string) Widget {
			return NewLabel(face, 14, s, th.Text)
		}),
	}})
	NewGolden(t, tv, "columnview-tree", goldenTheme(th), goldenFrame(220, 150))
}

// TestColumnViewHeaderAlignsWithRows pins the column contract: every
// header cell and the matching cell of a row share an x and a width,
// fixed and expanding columns alike - the sized cell both use.
func TestColumnViewHeaderAlignsWithRows(t *testing.T) {
	face := viewFace(t)
	v := NewColumnView(face, 14, SliceModel[tableRow]{{"alpha", 1}, {"bravo", 2}}, tableCols(face))
	v.Measure(Constraints{Max: Size{W: 300, H: 200}})
	v.Arrange(render.Rect{W: 300, H: 200})
	// List arranges its visible rows as it paints them.
	v.Paint(render.New(make([]byte, render.Stride(300)*200), render.Stride(300), 300, 200))
	row, ok := v.rows.Row(0).(*Box)
	if !ok {
		t.Fatalf("row 0 is %T", v.rows.Row(0))
	}
	heads, cells := v.header.Children(), row.Children()
	if len(heads) != len(cells) {
		t.Fatalf("%d header cells, %d row cells", len(heads), len(cells))
	}
	for i := range heads {
		h, c := heads[i].(Boundser).Bounds(), cells[i].(Boundser).Bounds()
		if h.X != c.X || h.W != c.W {
			t.Errorf("column %d: header %+v, row %+v", i, h, c)
		}
	}
}

// A row's cells pad their content 8px above and below and inset it
// 8px, under the column title: the reference table's p-2 cells. The
// cells sat flush on their text, and the titles were 13px and muted
// where the reference heads its columns in the text color (#281).
func TestColumnViewCellsAndTitles(t *testing.T) {
	face := viewFace(t)
	v := NewColumnView(face, 14, SliceModel[tableRow]{{"alpha", 1}}, tableCols(face))
	arrangeTree(t, v, 300, 200)
	row := v.buildRow(0).(*Box)
	cell := row.Children()[0].(*sizedCell).root.(*Box)
	if pad := cell.Padding(); pad.Top != 8 || pad.Bottom != 8 {
		t.Errorf("cell padding %+v, want 8 above and below", pad)
	}
	if w := cell.Children()[0].Measure(unbounded).W; w != headerInset || headerInset != 8 {
		t.Errorf("cell inset %d, want 8", w)
	}
	head := v.header.Children()[0].(*sizedCell).root.(*Button)
	title, ok := head.child.(*Box).Children()[0].(*Label)
	if !ok {
		t.Fatal("the column head holds no title label")
	}
	if title.SizePx() != 14 || HasClass(title, css.DimLabel) {
		t.Errorf("column title %vpx, dimmed %v: want 14px in the text color", title.SizePx(), HasClass(title, css.DimLabel))
	}
}

// A row is as tall as its padded cells and its rule: a fixed row
// height clipped the cells' bottom padding, so each row's text sat on
// the rule under it.
func TestColumnViewRowsFitTheirCells(t *testing.T) {
	face := viewFace(t)
	v := NewColumnView(face, 14, SliceModel[tableRow]{{"a", 1}, {"b", 2}}, tableCols(face))
	arrangeTree(t, v, 300, 200)
	cell := NewLabel(face, 14, "a", 0).Measure(Constraints{Max: Size{W: 300, H: 200}})
	if want := cell.H + 2*cellInsetY + 1; v.list.rowH < want {
		t.Errorf("rows are %d tall, want at least %d for the padded cell and its rule", v.list.rowH, want)
	}
}

// The model's last row carries no rule under it, as a table's last row
// draws none: its rule ran across the table's rounded bottom border.
func TestColumnViewLastRowHasNoRule(t *testing.T) {
	face := viewFace(t)
	v := NewColumnView(face, 14, SliceModel[tableRow]{{"a", 1}, {"b", 2}}, tableCols(face))
	arrangeTree(t, v, 300, 200)
	paintTree(v, 300, 200)
	first, last := v.list.rows[0], v.list.rows[1]
	if CascadeBorder(first).Bottom != 1 || CascadeBorder(last).Bottom != 0 {
		t.Errorf("rules under the rows %d and %d, want 1 then 0", CascadeBorder(first).Bottom, CascadeBorder(last).Bottom)
	}
}
