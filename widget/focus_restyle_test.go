package widget

import (
	"strconv"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// focusRowButtons is how many tab stops each row of focusRestyleTree
// holds.
const focusRowButtons = 2

func focusRestyleTree(t *testing.T, rows int) (root *Box, labels []*Label) {
	t.Helper()
	face := testFace(t)
	root = NewBox(Column, 0, 0)
	for i := range rows {
		label := NewLabel(face, 14, "Option "+strconv.Itoa(i), 0)
		row := NewBox(Row, 4, 0)
		row.AddClass("row")
		row.Append(label, true)
		for b := range focusRowButtons {
			row.Append(NewButton(NewLabel(face, 14, strconv.Itoa(b), 0), 4, 2), false)
		}
		root.Append(row, false)
		labels = append(labels, label)
	}
	arrangeTree(t, root, 400, rows*40)
	return root, labels
}

func treeRestyles(root Widget) int {
	total := 0
	walkWidgets(root, func(w Widget) bool {
		if c, ok := w.(interface{ restyleCount() int }); ok {
			total += c.restyleCount()
		}
		return true
	})
	return total
}

// A focus move within a tree restyles the widgets whose state changed
// and what reads them, never the page: the old and new focus with their
// content, and the rows whose :focus-within flipped with their
// children. Ancestors both chains share keep their bit, and a sibling
// combinator that reads no focus state widens nothing. The first focus
// enters the tree, flipping the root's :focus-within, which the sheet
// reads on every ancestor; every later move costs the same on a tree
// of 20 rows as on one of 400.
func TestFocusMoveRestylesOnlyTheWidgetsItChanges(t *testing.T) {
	sheet := AddStylesheet(`
		.row:focus-within > label { color: #ff0000; }
		box:focus-within button:focus label { color: #00ff00; }
		label + button { margin-left: 2px; }
	`, StylePriorityUser)
	t.Cleanup(sheet.Remove)
	red := render.RGB(0xff, 0, 0)
	const (
		moves = 3 * focusRowButtons
		bound = 16
	)
	var most [2]int
	for k, rows := range [2]int{20, 400} {
		root, labels := focusRestyleTree(t, rows)
		r := &Router{Root: root}
		r.FocusNext()
		root.Arrange(render.Rect{W: 400, H: rows * 40})
		for range moves {
			before := treeRestyles(root)
			r.FocusNext()
			root.Arrange(render.Rect{W: 400, H: rows * 40})
			CollectDamage(root)
			most[k] = max(most[k], treeRestyles(root)-before)
		}
		if most[k] > bound {
			t.Errorf("%d rows: a focus move restyled up to %d widgets, bound %d", rows, most[k], bound)
		}
		focusedRow := moves / focusRowButtons
		for i, l := range labels {
			if got := l.style(l).Color; (got == red) != (i == focusedRow) {
				t.Errorf("%d rows: row %d's label is %v, want red on the focused row %d alone", rows, i, got, focusedRow)
			}
		}
	}
	if most[0] != most[1] {
		t.Errorf("a focus move restyled up to %d widgets on 20 rows and %d on 400: the cost grows with the tree", most[0], most[1])
	}
}
