package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A floating panel's content takes the stylesheets above the panel on
// its first layout: the panel is parented before it is measured, so
// the first frame is already styled (#383).
func TestFloatingPanelContentIsStyledOnTheFirstLayout(t *testing.T) {
	btn := NewButton(newStub(20, 10), 0, 0)
	btn.AddClass("small")
	header := NewBox(Row, 0, 0)
	header.AddClass("hdr")
	header.Append(btn, false)
	panel := NewFloatingPanel(header, NewBox(Column, 0, 0))
	root := NewBox(Column, 0, 0)
	root.Append(panel, true)
	root.AttachStylesheet(NewStylesheet(`button.small { padding: 0 12px; } .hdr button.small { padding: 0; }`, StylePriorityUser))
	SetStyleRoot(root)
	SizeWithin(root, Size{W: 400, H: 300})
	root.Arrange(render.Rect{W: 400, H: 300})
	if got := btn.Bounds().W; got != 20 {
		t.Errorf("first layout: button %d wide, want 20, its padding-0 width under .hdr", got)
	}
}
