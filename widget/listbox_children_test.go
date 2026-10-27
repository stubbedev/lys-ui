package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// Tree walks reach a ListBox's rows and placeholder, and a row's label
// inherits the stylesheet's color through the list.
func TestListBoxChildren(t *testing.T) {
	l := NewListBox()
	a, b := NewLabel(goldenFace(t), 13, "a", 0), NewLabel(goldenFace(t), 13, "b", 0)
	l.Append(a)
	l.Append(b)
	ph := NewLabel(goldenFace(t), 13, "none", 0)
	l.SetPlaceholder(ph)
	kids := l.Children()
	if len(kids) != 3 || kids[2] != Widget(ph) {
		t.Fatalf("children %v, want the two rows and the placeholder", kids)
	}
	found := map[string]bool{}
	walkWidgets(l, func(w Widget) bool {
		if lb, ok := w.(*Label); ok {
			found[lb.Text()] = true
		}
		return true
	})
	if !found["a"] || !found["b"] {
		t.Errorf("walk found %v, want the rows' labels", found)
	}
	root := NewBox(Column, 0, 0)
	root.AttachStylesheet(NewStylesheet("list label { color: #102030; }", StylePriorityUser))
	root.Append(l, false)
	SetStyleRoot(root)
	SizeWithin(root, Size{W: 200, H: 200})
	root.Arrange(render.Rect{W: 200, H: 200})
	if c := CascadeColor(a); c != render.RGB(0x10, 0x20, 0x30) {
		t.Errorf("row label color %08x, want the list rule's", uint32(c))
	}
}
