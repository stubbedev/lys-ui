package widget

import (
	"slices"
	"testing"
)

func TestBoxMoveKeepsChildrenAttached(t *testing.T) {
	a, b, c := NewSpacer(1, 1), NewSpacer(2, 2), NewSpacer(3, 3)
	box := NewBox(Column, 0, 0)
	box.Append(a, false).Append(b, false).Append(c, false)
	removed := 0
	defer SetRemovedHook(func(Widget) { removed++ })()
	box.Move(0, 2)
	if got := box.Children(); !slices.Equal(got, []Widget{b, c, a}) {
		t.Errorf("after Move(0, 2) children = %v", got)
	}
	box.Move(2, -5)
	if got := box.Children(); !slices.Equal(got, []Widget{a, b, c}) {
		t.Errorf("after a clamped Move children = %v", got)
	}
	if removed != 0 {
		t.Errorf("moving fired the removal hook %d times", removed)
	}
}

func TestFlowBoxMoveCarriesSelection(t *testing.T) {
	f := NewFlowBox(0, 0)
	f.SetSelectionMode(SelectionMultiple)
	ws := []Widget{NewSpacer(1, 1), NewSpacer(1, 1), NewSpacer(1, 1), NewSpacer(1, 1)}
	for _, w := range ws {
		f.Append(w)
	}
	f.Select(2)
	f.Move(0, 3)
	if got := f.ChildAt(3).Child(); got != ws[0] {
		t.Fatalf("index 3 holds %v, want the moved child", got)
	}
	if sel := f.Selection(); !slices.Equal(sel, []int{1}) {
		t.Errorf("selection after Move(0, 3) = %v, want [1]: the selected child shifted up", sel)
	}
	f.Move(1, 3)
	if sel := f.Selection(); !slices.Equal(sel, []int{3}) {
		t.Errorf("selection after moving the selected child = %v, want [3]", sel)
	}
}

func TestNotebookInsertAndMoveTab(t *testing.T) {
	n := NewNotebook(testFace(t))
	n.AppendTab("a", NewSpacer(1, 1))
	n.AppendTab("c", NewSpacer(1, 1))
	n.SelectTab("c")
	n.InsertTab(1, "b", NewSpacer(1, 1))
	if n.SelectedTab() != "c" {
		t.Errorf("inserting before the selection moved it to %q", n.SelectedTab())
	}
	n.MoveTab(2, 0)
	var names []string
	for _, tab := range n.tabs {
		names = append(names, tab.name)
	}
	if !slices.Equal(names, []string{"c", "a", "b"}) || n.SelectedTab() != "c" {
		t.Errorf("tabs %v selected %q, want [c a b] with c selected", names, n.SelectedTab())
	}
}

func TestStackInsertAndMoveReorderPages(t *testing.T) {
	s := NewStack()
	s.Add("a", NewSpacer(1, 1)).Add("c", NewSpacer(1, 1))
	s.Insert(1, "b", NewSpacer(1, 1))
	if got := s.Order(); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("order after Insert = %v", got)
	}
	s.Move("a", 2)
	if got := s.Order(); !slices.Equal(got, []string{"b", "c", "a"}) || s.Visible() != "a" {
		t.Errorf("order after Move = %v visible %q, want [b c a] still showing a", got, s.Visible())
	}
}

func TestGridPlaceMovesWithoutDisplacing(t *testing.T) {
	g := NewGrid(0, 0)
	a, b := NewSpacer(1, 1), NewSpacer(1, 1)
	g.Attach(a, 0, 0, 1, 1).Attach(b, 1, 0, 1, 1)
	removed := 0
	defer SetRemovedHook(func(Widget) { removed++ })()
	g.Place(a, 1, 0, 1, 1)
	g.Place(b, 2, 0, 1, 1)
	if removed != 0 || len(g.Children()) != 2 {
		t.Errorf("Place displaced a child: removed=%d children=%d", removed, len(g.Children()))
	}
}

func TestNotebookCloseTabAtKeepsTheSelectedPage(t *testing.T) {
	n := NewNotebook(testFace(t))
	for _, name := range []string{"a", "b", "c", "d"} {
		n.AppendTab(name, NewSpacer(1, 1))
	}
	n.SelectTab("c")
	n.CloseTab("a")
	if n.SelectedTab() != "c" {
		t.Errorf("closing an earlier tab switched the page to %q", n.SelectedTab())
	}
	n.CloseTabAt(2)
	n.CloseTabAt(1)
	if n.SelectedTab() != "b" {
		t.Errorf("closing the selected last tab selected %q, want b", n.SelectedTab())
	}
}
