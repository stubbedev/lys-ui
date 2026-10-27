package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func listBoxRows(n, w, h int) (*ListBox, []*solidLeaf) {
	l := NewListBox()
	leaves := make([]*solidLeaf, n)
	for i := range n {
		leaves[i] = &solidLeaf{sz: Size{W: w, H: h}}
		l.Append(leaves[i])
	}
	return l, leaves
}

func layoutListBox(l *ListBox, w int) {
	nat := SizeWithin(l, Size{W: w, H: 10000})
	l.Arrange(render.Rect{W: w, H: nat.H})
}

func TestListBoxRowCRUDAndIndex(t *testing.T) {
	l, leaves := listBoxRows(3, 40, 20)
	if l.Len() != 3 || l.RowAt(2).Child() != Widget(leaves[2]) || l.RowAt(3) != nil {
		t.Fatal("Append / RowAt")
	}
	if l.RowAt(1).Index() != 1 || len(l.Rows()) != 3 {
		t.Fatal("Index / Rows")
	}
	inserted := &solidLeaf{sz: Size{W: 10, H: 10}}
	l.Insert(1, inserted)
	if l.RowAt(1).Child() != Widget(inserted) || l.Len() != 4 || parentOf(inserted) != Widget(l.RowAt(1)) || l.RowAt(2).Index() != 2 {
		t.Fatal("Insert")
	}
	l.Select(2)
	removed := l.RowAt(2).Child()
	l.RemoveAt(2)
	if l.Len() != 3 || l.Selected() != -1 || l.RowAt(1).Index() != 1 || parentOf(removed) != nil || l.RowAt(1).Child() != Widget(inserted) {
		t.Fatal("RemoveAt")
	}
	l.Move(2, 0)
	if l.RowAt(0).Child() != Widget(leaves[2]) || l.RowAt(1).Child() != Widget(leaves[0]) || l.RowAt(2).Child() != Widget(inserted) {
		t.Fatal("Move")
	}
	if l.RowAt(2).Index() != 2 || l.RowAt(2).Index() != slices.Index(l.Rows(), Widget(l.RowAt(2))) {
		t.Fatal("post-move Index")
	}
}

func TestListBoxSelectionModes(t *testing.T) {
	l, _ := listBoxRows(3, 40, 20)
	var activated []int
	var changes [][]int
	l.OnActivate = func(i int) { activated = append(activated, i) }
	l.OnSelectionChanged = func(s []int) { changes = append(changes, s) }
	l.RowAt(1).ClickAt(Point{})
	if l.Selected() != 1 || !l.RowAt(1).HasState(StateSelected) || !slices.Equal(activated, []int{1}) {
		t.Fatalf("click: selected=%d activated=%v", l.Selected(), activated)
	}
	l.Select(-1)
	if l.Selected() != -1 {
		t.Fatalf("single mode clears: %d", l.Selected())
	}

	l.SetSelectionMode(SelectionBrowse)
	l.Select(0)
	l.Select(-1)
	if l.Selected() != 0 {
		t.Fatalf("browse mode pins the first row: %d", l.Selected())
	}
	l.SetSelectionMode(SelectionBrowse)
	layoutListBox(l, 100)
	l.RowAt(2).DragMove(Point{X: 2, Y: 42})
	if l.Selected() != 2 {
		t.Fatalf("browse motion selects: %d", l.Selected())
	}
	if len(changes) == 0 || !slices.Equal(changes[len(changes)-1], []int{2}) {
		t.Fatalf("notifications %v", changes)
	}

	l.SetSelectionMode(SelectionMultiple)
	l.Select(0)
	l.RowAt(1).ClickAt(Point{})
	if got := l.Selection(); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("multiple toggle: %v", got)
	}
	l.KeyAction(KeySpace, ModCtrl)
	if got := l.Selection(); !slices.Equal(got, []int{0}) {
		t.Fatalf("ctrl+space toggles the cursor row: %v", got)
	}
	activated = nil
	l.KeyAction(KeyEnter, 0)
	if !slices.Equal(activated, []int{0}) {
		t.Fatalf("Enter activates the selection: %v", activated)
	}

	l.SetSelectionMode(SelectionNone)
	l.Select(0)
	l.RowAt(2).ClickAt(Point{})
	if len(l.Selection()) != 0 {
		t.Fatalf("none mode selects: %v", l.Selection())
	}
}

func TestListBoxRowFlags(t *testing.T) {
	l, _ := listBoxRows(3, 40, 20)
	var activated []int
	l.OnActivate = func(i int) { activated = append(activated, i) }
	l.RowAt(1).SetSelectable(false)
	l.RowAt(1).ClickAt(Point{})
	if l.Selected() != -1 || !slices.Equal(activated, []int{1}) {
		t.Fatalf("a non-selectable row activates without selecting: %d %v", l.Selected(), activated)
	}
	l.KeyAction(KeyDown, 0)
	l.KeyAction(KeyDown, 0)
	if l.Selected() == 1 {
		t.Fatalf("keyboard navigation stepped onto a non-selectable row: %d", l.Selected())
	}

	activated = nil
	l.RowAt(0).SetActivatable(false)
	l.RowAt(0).ClickAt(Point{})
	if l.Selected() != 0 || activated != nil {
		t.Fatalf("a non-activatable row selects without activating: %d %v", l.Selected(), activated)
	}
	l.KeyAction(KeyEnter, 0)
	if activated != nil {
		t.Fatalf("Enter activated a non-activatable row: %v", activated)
	}
}

func TestListBoxKeyboard(t *testing.T) {
	l, _ := listBoxRows(3, 40, 20)
	var activated []int
	l.OnActivate = func(i int) { activated = append(activated, i) }
	l.KeyAction(KeyDown, 0)
	if l.Selected() != 0 {
		t.Fatalf("Down on nothing selects the first row: %d", l.Selected())
	}
	l.KeyAction(KeyDown, 0)
	if l.Selected() != 1 {
		t.Fatalf("Down: %d", l.Selected())
	}
	l.KeyAction(KeyUp, 0)
	if l.Selected() != 0 {
		t.Fatalf("Up: %d", l.Selected())
	}
	l.KeyAction(KeyUp, 0)
	if l.Selected() != 0 {
		t.Fatalf("Up clamps at the top: %d", l.Selected())
	}
	l.KeyAction(KeyEnd, 0)
	if l.Selected() != 2 {
		t.Fatalf("End: %d", l.Selected())
	}
	l.KeyAction(KeyEnter, 0)
	if !slices.Equal(activated, []int{2}) {
		t.Fatalf("Enter activates: %v", activated)
	}
}

func cmpStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestListBoxFilterSortHeaders(t *testing.T) {
	l, leaves := listBoxRows(3, 40, 20)
	keys := []string{"b", "a", "c"}
	l.SetSortFunc(func(a, b *ListBoxRow) int {
		return cmpStrings(keys[a.Index()], keys[b.Index()])
	})
	layoutListBox(l, 100)
	if tops := []int{l.RowAt(0).Bounds().Y, l.RowAt(1).Bounds().Y, l.RowAt(2).Bounds().Y}; !slices.Equal(tops, []int{20, 0, 40}) {
		t.Fatalf("sorted Y order %v", tops)
	}
	if leaves[1].bounds.Y != 0 || leaves[0].bounds.Y != 20 || leaves[2].bounds.Y != 40 {
		t.Fatalf("sorted rows paint a c b: a at %v, b at %v, c at %v", leaves[1].bounds, leaves[0].bounds, leaves[2].bounds)
	}
	keys = []string{"c", "b", "a"}
	l.InvalidateSort()
	layoutListBox(l, 100)
	if leaves[2].bounds.Y != 0 || leaves[1].bounds.Y != 20 || leaves[0].bounds.Y != 40 {
		t.Fatalf("InvalidateSort re-ran: c at %v, a at %v", leaves[2].bounds, leaves[1].bounds)
	}

	l.Select(0)
	runs := 0
	l.SetFilterFunc(func(r *ListBoxRow) bool {
		runs++
		return r.Index() != 0
	})
	if l.Selected() != -1 {
		t.Fatalf("the hidden selected row dropped: %d", l.Selected())
	}
	if l.RowAt(0).ChildVisible() || !l.RowAt(1).ChildVisible() || !l.RowAt(2).ChildVisible() {
		t.Fatalf("child visibility %v %v %v, want the filtered row alone hidden",
			l.RowAt(0).ChildVisible(), l.RowAt(1).ChildVisible(), l.RowAt(2).ChildVisible())
	}
	layoutListBox(l, 100)
	if sz := SizeWithin(l, Size{W: 100, H: 1000}); sz.H != 40 {
		t.Fatalf("a filtered row still measures: %v", sz)
	}
	runs = 0
	l.InvalidateFilter()
	if runs != 3 {
		t.Fatalf("InvalidateFilter re-ran the filter %d times", runs)
	}

	headers := 0
	l.SetHeaderFunc(func(r *ListBoxRow) Widget {
		headers++
		if r.Index() == 1 {
			return NewSpacer(40, 5)
		}
		return nil
	})
	if headers != 3 || len(l.RowAt(1).Children()) != 2 || l.RowAt(0).Children()[0] != Widget(leaves[0]) {
		t.Fatalf("headers: runs=%d children=%d", headers, len(l.RowAt(1).Children()))
	}
	headers = 0
	l.InvalidateHeaders()
	if headers != 3 {
		t.Fatalf("InvalidateHeaders re-ran the header function %d times", headers)
	}
	if sz := SizeWithin(l, Size{W: 100, H: 1000}); sz.H != 20+5+20 {
		t.Fatalf("a header grows its row: %v", sz)
	}
}

func TestListBoxMinHeightFloorsFilteredMeasure(t *testing.T) {
	l, _ := listBoxRows(6, 40, 20)
	l.SetMinHeight(150)
	full := SizeWithin(l, Size{W: 100, H: 1000})
	l.SetFilterFunc(func(*ListBoxRow) bool { return false })
	filtered := SizeWithin(l, Size{W: 100, H: 1000})
	if filtered.H != 150 || full.H != max(full.H, 150) {
		t.Fatalf("min height %d does not floor the measure: full %v filtered %v", 150, full, filtered)
	}
}

func TestListBoxPlaceholderWhenEmpty(t *testing.T) {
	l, _ := listBoxRows(2, 40, 20)
	ph := NewSpacer(30, 10)
	l.SetPlaceholder(ph)
	layoutListBox(l, 100)
	if ph.Bounds() != (render.Rect{}) {
		t.Fatalf("the placeholder shows beside rows: %v", ph.Bounds())
	}
	l.SetFilterFunc(func(*ListBoxRow) bool { return false })
	if sz := SizeWithin(l, Size{W: 100, H: 1000}); sz != (Size{W: 30, H: 10}) {
		t.Fatalf("with every row filtered out: %v", sz)
	}
	layoutListBox(l, 100)
	if got := ph.Bounds(); got != (render.Rect{W: 100, H: 10}) {
		t.Fatalf("the placeholder fills the width: %v", got)
	}
	if l.HitTest(Point{X: 5, Y: 5}) != Widget(ph) {
		t.Fatalf("hit test over the placeholder: %v", l.HitTest(Point{X: 5, Y: 5}))
	}
}
