package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// flowGrid is a flow box of n 20x20 children, three per 70px line.
func flowGrid(n int) *FlowBox {
	f := NewFlowBox(5, 5)
	for range n {
		f.Append(NewSpacer(20, 20))
	}
	f.Measure(Constraints{Max: Size{W: 70, H: 200}})
	f.Arrange(render.Rect{W: 70, H: 200})
	return f
}

// TestFlowBoxSelection pins the shared model on a flow box: a click
// selects and activates, a button inside a child keeps its click,
// arrows move by item and by line, and multiple mode rubber-bands with
// one notification per gesture.
func TestFlowBoxSelection(t *testing.T) {
	f := flowGrid(7)
	var activated []int
	var changes [][]int
	f.OnActivate = func(i int) { activated = append(activated, i) }
	f.OnSelectionChanged = func(s []int) { changes = append(changes, s) }
	f.ChildAt(4).ClickAt(Point{})
	if f.Selected() != 4 || !f.ChildAt(4).HasState(StateSelected) || !slices.Equal(activated, []int{4}) {
		t.Fatalf("click: selected=%d activated=%v", f.Selected(), activated)
	}
	f.KeyAction(KeyUp, 0)
	if f.Selected() != 1 {
		t.Errorf("Up from the middle of line two = %d, want 1", f.Selected())
	}
	f.KeyAction(KeyDown, 0)
	f.KeyAction(KeyDown, 0)
	if f.Selected() != 6 {
		t.Errorf("Down twice from 1 = %d, want 6 (the last line's only child)", f.Selected())
	}
	f.KeyAction(KeyLeft, 0)
	if f.Selected() != 5 {
		t.Errorf("Left = %d", f.Selected())
	}

	f.SetSelectionMode(SelectionMultiple)
	changes = nil
	c0 := f.ChildAt(0)
	c0.SetPressed(true)
	c0.DragMove(Point{X: f.ChildAt(4).bounds.X + 2, Y: f.ChildAt(4).bounds.Y + 2})
	c0.DragMove(Point{X: f.ChildAt(5).bounds.X + 2, Y: f.ChildAt(5).bounds.Y + 2})
	c0.ClickAt(Point{})
	c0.PressEnd()
	if got := f.Selection(); !slices.Equal(got, []int{0, 1, 2, 3, 4, 5}) || len(changes) != 1 {
		t.Errorf("band = %v, notifications = %d", got, len(changes))
	}

	btn := NewButton(NewSpacer(10, 10), 0, 0)
	clicked := 0
	btn.OnClick = func() { clicked++ }
	child := f.Append(btn)
	f.Measure(Constraints{Max: Size{W: 70, H: 200}})
	f.Arrange(render.Rect{W: 70, H: 200})
	b := btn.Bounds()
	if hit := f.HitTest(Point{X: b.X + 1, Y: b.Y + 1}); hit == Widget(child) {
		t.Error("the child wrapper swallowed its button's press")
	}
}

// TestFlowBoxSelectionShifts pins the indices across mutation: an
// insert before the selection shifts it, removing the selected child
// clears it and notifies.
func TestFlowBoxSelectionShifts(t *testing.T) {
	f := flowGrid(4)
	f.Select(2)
	f.Insert(0, NewSpacer(20, 20))
	if f.Selected() != 3 || !f.ChildAt(3).HasState(StateSelected) || f.ChildAt(2).HasState(StateSelected) {
		t.Errorf("after insert: selected=%d", f.Selected())
	}
	notified := 0
	f.OnSelectionChanged = func([]int) { notified++ }
	f.RemoveAt(3)
	if f.Selected() != -1 || notified != 1 {
		t.Errorf("after removing the selection: selected=%d notified=%d", f.Selected(), notified)
	}
	if NewWrapBox(0, 0, JustifyStart).SelectionMode() != SelectionNone {
		t.Error("a WrapBox selects")
	}
}

// TestGoldenFlowBoxSelection pins the selection tint on two children.
func TestGoldenFlowBoxSelection(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	f := NewFlowBox(6, 6)
	f.SetSelectionMode(SelectionMultiple)
	for _, s := range []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon"} {
		c := f.Append(NewLabel(face, 14, s, th.Text))
		c.AddClass("pad")
	}
	f.toggle(1)
	f.toggle(3)
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet(`flowboxchild.pad { padding: 4px 8px; }`, StylePriorityUser))
	root.Append(f, false)
	NewGolden(t, root, "flowbox-selection", goldenTheme(th), goldenFrame(200, 70))
}
