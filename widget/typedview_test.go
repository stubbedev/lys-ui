package widget

import (
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestTypedItemsCRUD pins the typed item model: append/insert/remove/
// clear/get, insertion in sort order, and the two change hooks.
func TestTypedItemsCRUD(t *testing.T) {
	m := NewTypedItems("c", "a")
	m.Append("d")
	m.Insert(1, "b")
	if m.Len() != 4 || m.Get(0) != "c" || m.Get(1) != "b" || m.Get(3) != "d" {
		t.Fatalf("after insert: %v", m.items)
	}
	m.RemoveAt(0)
	if m.Len() != 3 || m.Get(0) != "b" {
		t.Fatalf("after remove: %v", m.items)
	}

	sorted := NewTypedItems("b", "d", "e")
	if i := sorted.InsertSorted("c", strings.Compare); i != 1 || sorted.Get(1) != "c" {
		t.Fatalf("insert sorted landed at %d: %v", i, sorted.items)
	}
	if i := sorted.InsertSorted("a", strings.Compare); i != 0 {
		t.Fatalf("head insert landed at %d: %v", i, sorted.items)
	}
	if i := sorted.InsertSorted("z", strings.Compare); i != 5 {
		t.Fatalf("tail insert landed at %d: %v", i, sorted.items)
	}

	whole, edited := 0, -1
	m.SetRefresh(func() { whole++ })
	m.SetItemRefresh(func(i int) { edited = i })
	m.Append("x")
	if whole != 1 {
		t.Errorf("append notified %d times", whole)
	}
	m.Set(1, "B")
	if edited != 1 || m.Get(1) != "B" {
		t.Errorf("in-place edit: hook %d, item %q", edited, m.Get(1))
	}
	if whole != 1 {
		t.Errorf("an in-place edit ran the wholesale hook")
	}
	m.Clear()
	if m.Len() != 0 || whole != 2 {
		t.Fatalf("clear left %d items after %d notifies", m.Len(), whole)
	}
}

// recycleSpy counts the factory's three calls and tracks which item
// every built row is bound to.
type recycleSpy struct {
	setups, binds, unbinds int
	badUnbinds             int
	items                  map[Widget]int
}

func newRecycleSpy() *recycleSpy {
	return &recycleSpy{items: map[Widget]int{}}
}

// spread is 0..n-1 as a slice.
func spread(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func (s *recycleSpy) factory() TypedFactory[int] {
	return TypedFactory[int]{
		Setup: func() Widget {
			s.setups++
			return newStub(60, 20)
		},
		Bind: func(item int, row Widget) {
			s.binds++
			s.items[row] = item
		},
		Unbind: func(item int, row Widget) {
			s.unbinds++
			if s.items[row] != item {
				s.badUnbinds++
			}
		},
	}
}

func newTypedTestList(spy *recycleSpy, n int) (*TypedList[int], *TypedItems[int]) {
	rows := make([]int, n)
	for i := range rows {
		rows[i] = i
	}
	items := NewTypedItems(rows...)
	l := NewTypedList[int](items, spy.factory(), 20)
	items.SetRefresh(l.Refresh)
	return l, items
}

// TestTypedListRecyclesRows pins the recycling claim: Setup builds only
// a viewport's worth of rows ever, Bind runs per item as rows scroll,
// Unbind hears the item on the way out, and only the visible rows are
// instantiated across a ten-thousand-item model.
func TestTypedListRecyclesRows(t *testing.T) {
	spy := newRecycleSpy()
	l, _ := newTypedTestList(spy, 10_000)
	l.Measure(Constraints{Max: Size{W: 200, H: 1 << 20}})
	l.Arrange(render.Rect{W: 200, H: 60})
	cv := render.New(make([]uint8, 200*60*4), 200*4, 200, 60)
	l.Paint(cv)
	if spy.setups != 4 || spy.binds != 4 || spy.unbinds != 0 {
		t.Fatalf("first paint: %d setups, %d binds, %d unbinds", spy.setups, spy.binds, spy.unbinds)
	}
	if len(l.core.rows.live) != 4 {
		t.Fatalf("%d rows live, want the 4 visible", len(l.core.rows.live))
	}

	l.setOffset(40)
	l.Paint(cv)
	if spy.setups != 4 || spy.binds != 6 || spy.unbinds != 2 {
		t.Fatalf("after a scroll: %d setups, %d binds, %d unbinds", spy.setups, spy.binds, spy.unbinds)
	}

	l.setOffset(9940 * 20)
	l.Paint(cv)
	if spy.setups > 8 || spy.binds != spy.setups+spy.unbinds {
		t.Fatalf("deep scroll: %d setups, %d binds, %d unbinds", spy.setups, spy.binds, spy.unbinds)
	}
	if len(l.core.rows.live) != 4 {
		t.Fatalf("%d rows live after scrolling, want 4", len(l.core.rows.live))
	}
	if spy.binds <= spy.setups {
		t.Fatalf("rows were not reused: %d binds over %d setups", spy.binds, spy.setups)
	}
	if spy.badUnbinds != 0 {
		t.Fatalf("%d unbinds heard the wrong item", spy.badUnbinds)
	}
}

// TestTypedListItemChangedRebinds pins the in-place hook: an edited
// item rebinds the row showing it, without Setup or Unbind.
func TestTypedListItemChangedRebinds(t *testing.T) {
	spy := newRecycleSpy()
	l, items := newTypedTestList(spy, 3)
	items.SetItemRefresh(l.ItemChanged)
	l.Measure(Constraints{Max: Size{W: 200, H: 1 << 20}})
	l.Arrange(render.Rect{W: 200, H: 60})
	cv := render.New(make([]uint8, 200*60*4), 200*4, 200, 60)
	l.Paint(cv)
	row := l.core.rows.live[1].w
	setups, binds := spy.setups, spy.binds

	items.Set(1, 99)
	if spy.setups != setups || spy.binds != binds+1 {
		t.Fatalf("edit: %d setups (was %d), %d binds (was %d)", spy.setups, setups, spy.binds, binds)
	}
	if l.core.rows.live[1].w != row || spy.items[row] != 99 {
		t.Fatal("the edited row was not rebound in place")
	}
}

// TestTypedListFiltersCompose pins the filter stack: several filters
// compose, a toggled-off one stops narrowing, pop drops the newest,
// clear drops them all.
func TestTypedListFiltersCompose(t *testing.T) {
	spy := newRecycleSpy()
	l, _ := newTypedTestList(spy, 10)
	names := func() []int {
		out := make([]int, 0, l.Rows())
		for i := range l.Rows() {
			out = append(out, l.Row(i))
		}
		return out
	}
	odd := l.AddFilter(func(i int) bool { return i%2 == 1 })
	big := l.AddFilter(func(i int) bool { return i > 5 })
	if got := names(); !slices.Equal(got, []int{7, 9}) {
		t.Fatalf("composed filters kept %v", got)
	}
	l.SetFilterStatus(odd, false)
	if got := names(); !slices.Equal(got, []int{6, 7, 8, 9}) {
		t.Fatalf("toggled-off filter still narrowed: %v", got)
	}
	if odd.Enabled() || !big.Enabled() {
		t.Fatalf("filter status: odd %v big %v", odd.Enabled(), big.Enabled())
	}
	l.SetFilterStatus(odd, true)
	l.PopFilter() // drops big
	if got := names(); !slices.Equal(got, []int{1, 3, 5, 7, 9}) {
		t.Fatalf("after pop: %v", got)
	}
	l.ClearFilters()
	if got := names(); len(got) != 10 || got[0] != 0 {
		t.Fatalf("after clear: %v", got)
	}
}

// TestTypedListSortAppliedOnInsertAndDemand pins sorting: SetSort
// orders on demand, a later insert lands where the sort puts it, and
// nil returns to the source order.
func TestTypedListSortAppliedOnInsertAndDemand(t *testing.T) {
	spy := newRecycleSpy()
	l, items := newTypedTestList(spy, 5)
	desc := func(a, b int) int { return b - a }
	l.SetSort(desc)
	if got := l.Row(0); got != 4 || l.Rows() != 5 {
		t.Fatalf("sorted row 0 = %d of %d", got, l.Rows())
	}
	items.Append(9)
	if got := l.Row(0); got != 9 {
		t.Fatalf("an appended item landed at %d under the sort, row 0 = %d", 0, got)
	}
	items.Insert(0, 0)
	if got := l.Row(0); got != 9 {
		t.Fatalf("a source-head insert kept the sort: row 0 = %d", got)
	}
	l.SetSort(nil)
	if got := l.Row(0); got != 0 {
		t.Fatalf("source order not restored: row 0 = %d", got)
	}
	if items.Get(0) != 0 {
		t.Fatalf("the source model was reordered: %v", items.items)
	}
}

// TestTypedListTypedSelection pins the typed selection: it reports
// items, follows them through a sort, and loses the items a filter
// hides - reporting the change once.
func TestTypedListTypedSelection(t *testing.T) {
	spy := newRecycleSpy()
	l, _ := newTypedTestList(spy, 4)
	l.SetSelectionMode(SelectionMultiple)
	var reported [][]int
	l.OnSelectionChanged = func(sel []int) { reported = append(reported, sel) }

	l.Select(1) // visible row 1: item 1
	if sel := l.SelectedItems(); !slices.Equal(sel, []int{1}) {
		t.Fatalf("selected %v, want [1]", sel)
	}
	if n := len(reported); n != 1 || !slices.Equal(reported[0], []int{1}) {
		t.Fatalf("reported %v", reported)
	}

	l.SetSort(func(a, b int) int { return b - a })
	if sel := l.SelectedItems(); !slices.Equal(sel, []int{1}) {
		t.Fatalf("after a sort selected %v, want [1]", sel)
	}
	if rows := l.Selection(); !slices.Equal(rows, []int{2}) {
		t.Fatalf("the row selection did not follow the sort: %v", rows)
	}

	hide := l.AddFilter(func(i int) bool { return i != 1 })
	if sel := l.SelectedItems(); len(sel) != 0 {
		t.Fatalf("a hidden item stayed selected: %v", sel)
	}
	if n := len(reported); n != 2 || len(reported[1]) != 0 {
		t.Fatalf("the drop was reported %v", reported)
	}
	l.SetFilterStatus(hide, false)
	if sel := l.SelectedItems(); len(sel) != 0 {
		t.Fatalf("a dropped selection came back: %v", sel)
	}

	l.SetSelectionMode(SelectionSingle)
	l.Select(0)
	if item, ok := l.SelectedItem(); !ok || item != 3 {
		t.Fatalf("single selection: %d %v, want 3", item, ok)
	}
	if _, ok := l.SelectedItem(); !ok {
		t.Fatal("SelectedItem lost the selection")
	}
}

// TestTypedGridFlowsAndStyles pins the grid: cells flow into columns
// of the cell width and style as gridview children, typed selection
// and all.
func TestTypedGridFlowsAndStyles(t *testing.T) {
	spy := newRecycleSpy()
	g := NewTypedGrid[int](NewTypedItems(spread(100)...), spy.factory(), 50, 40)
	g.Measure(Constraints{Max: Size{W: 250, H: 200}})
	g.Arrange(render.Rect{W: 250, H: 200})
	cv := render.New(make([]uint8, 250*200*4), 250*4, 250, 200)
	g.Paint(cv)
	if g.Columns() != 5 {
		t.Fatalf("columns = %d, want 5", g.Columns())
	}
	if g.Element() != "gridview" {
		t.Errorf("element %q, want gridview", g.Element())
	}
	if live := len(g.core.rows.live); live == 0 || live > 30 {
		t.Fatalf("%d cells live for a 5x5 viewport over 100 items", live)
	}
	for _, r := range g.rows {
		if r.Element() != "child" {
			t.Errorf("cell element %q, want child", r.Element())
		}
	}
}
