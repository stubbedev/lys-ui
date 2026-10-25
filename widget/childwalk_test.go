package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// TestChildWalkMatchesChildren pins the contract the frame's damage
// walk leans on: appendChildren (the buffered snapshot CollectDamage
// takes) must produce exactly what Children() returns, in order, for
// every container and every visibility state that changes the answer.
// A drift would send the damage walk and the public traversal order
// out of sync.
func TestChildWalkMatchesChildren(t *testing.T) {
	face := entryFace(t)
	label := func(s string) *Label { return NewLabel(face, 12, s, Current().Text) }

	build := func() []Widget {
		box := NewBox(Column, 2, 0)
		box.Append(label("a"), false)
		box.Append(label("b"), false)

		stack := NewStack()
		stack.Add("one", label("1"))
		stack.Add("two", label("2"))
		stack.Show("one")

		overlay := NewOverlay()
		overlay.Append(label("o1"))
		overlay.Append(label("o2"))

		grid := NewGrid(2, 4)
		grid.Attach(label("g1"), 1, 0, 1, 1)
		grid.Attach(label("g2"), 0, 1, 1, 1)
		grid.Attach(label("g3"), 0, 0, 1, 1)

		notebook := NewNotebook(face)
		notebook.AppendTab("tab1", label("t1"))
		notebook.AppendTab("tab2", label("t2"))
		notebook.SelectTab("tab2")

		paned := NewPaned(Row, label("p1"), label("p2"))

		expanded := NewExpander(face, "open", label("x"))
		expanded.SetOpen(true)
		closed := NewExpander(face, "closed", label("y"))

		scrolled := NewScroll(box)
		empty := NewScroll(nil)

		cal := NewCalendar(face, 12, Today())

		chooser := NewColorChooser(face, 12, render.RGB(1, 2, 3))

		elevation := NewElevation(label("e"))

		return []Widget{
			box, stack, overlay, grid, notebook, paned,
			expanded, closed, scrolled, empty,
			cal, chooser, elevation,
		}
	}

	check := func(stage string, ws []Widget) {
		t.Helper()
		for _, w := range ws {
			want := w.(childser).Children()
			got := w.(childBuf).appendChildren(nil)
			if len(got) != len(want) {
				t.Errorf("%s: %T append visited %d children, Children has %d", stage, w, len(got), len(want))
				continue
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("%s: %T append[%d] = %p, Children[%d] = %p", stage, w, i, got[i], i, want[i])
				}
			}
		}
	}

	ws := build()
	check("initial", ws)
	// State flips change both views together: stack page, notebook tab,
	// expander fold, overlay additions.
	ws[1].(*Stack).Show("two")
	ws[4].(*Notebook).SelectTab("tab1")
	ws[6].(*Expander).SetOpen(false)
	ws[7].(*Expander).SetOpen(true)
	ws[2].(*Overlay).Append(label("o3"))
	check("after flips", ws)
}
