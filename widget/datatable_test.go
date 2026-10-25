package widget

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

type payment struct {
	ID     int
	Email  string
	Amount int
}

// paymentCols are the email and amount columns, the amount sortable.
func paymentCols(face render.Font) []TableColumn[payment] {
	return []TableColumn[payment]{
		{Title: "Email", Width: 220, Expand: true, Cell: func(p payment) Widget { return NewLabel(face, 14, p.Email, 0) }},
		{
			Title: "Amount", Width: 100, Cell: func(p payment) Widget { return NewLabel(face, 14, fmt.Sprintf("$%d", p.Amount), 0) },
			Sort: func(a, b payment) int { return a.Amount - b.Amount },
		},
	}
}

func newTestDataTable(t *testing.T, n int) *DataTable[payment] {
	t.Helper()
	face := goldenFace(t)
	rows := make(SliceModel[payment], n)
	for i := range rows {
		rows[i] = payment{ID: i, Email: fmt.Sprintf("user%d@example.com", i), Amount: (i * 37) % 1000}
	}
	tbl := NewDataTable(face, rows, paymentCols(face), func(p payment, q string) bool { return strings.Contains(p.Email, q) })
	host := &fakeSurfaceHost{}
	SetSurfaceHost(tbl, host)
	arrangeTree(t, tbl, 700, 600)
	return tbl
}

// A large table pages its rows, filters them, and keeps a selection by
// model row across pages.
func TestDataTablePagesFiltersAndSelects(t *testing.T) {
	tbl := newTestDataTable(t, 100000)
	if tbl.pager.Count() != 10000 || tbl.view.Rows() != 10 {
		t.Fatalf("%d pages of %d rows", tbl.pager.Count(), tbl.view.Rows())
	}
	tbl.view.List().SelectRows(0, 2)
	tbl.listSelected([]int{0, 2})
	tbl.showPage(5)
	if tbl.view.SourceIndex(0) != 40 {
		t.Fatalf("page 5 starts at row %d, want 40", tbl.view.SourceIndex(0))
	}
	tbl.view.List().SelectRows(1)
	tbl.listSelected([]int{1})
	if got := tbl.Selected(); !slices.Equal(got, []int{0, 2, 41}) {
		t.Fatalf("selected %v, want rows 0, 2 and 41", got)
	}
	tbl.showPage(1)
	if got := tbl.view.List().Selection(); !slices.Equal(got, []int{0, 2}) {
		t.Errorf("page 1 shows %v selected, want its rows 0 and 2", got)
	}
	tbl.filter("user4")
	if tbl.page != 1 || tbl.view.Matching() != 11111 {
		t.Errorf("filtered to %d rows on page %d", tbl.view.Matching(), tbl.page)
	}
	if !strings.Contains(tbl.status.Text(), "3 of 11111") {
		t.Errorf("status %q", tbl.status.Text())
	}
}

// The header check selects the page; a hidden column leaves the view.
func TestDataTableSelectPageAndColumns(t *testing.T) {
	tbl := newTestDataTable(t, 25)
	tbl.selectPage(true)
	if got := len(tbl.Selected()); got != 10 {
		t.Fatalf("the header check selected %d rows, want the page's 10", got)
	}
	tbl.SetColumnVisible(1, false)
	if len(tbl.view.cols) != 2 || tbl.ColumnVisible(1) {
		t.Errorf("hiding the amount left %d columns", len(tbl.view.cols))
	}
}

// TestGoldenDataTable pins the toolbar, the table with a sorted
// column and selected rows, and the footer, in both palettes.
func TestGoldenDataTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			tbl := newTestDataTable(t, 40)
			tbl.SetPageSize(5)
			tbl.SortBy(1, true)
			tbl.view.List().SelectRows(1, 2)
			tbl.listSelected([]int{1, 2})
			NewGolden(t, tbl, "datatable-"+tc.name, goldenTheme(tc.th), goldenFrame(700, 360))
		})
	}
}
