package widget_test

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// wrapSwitch is the idiom apps use to add behavior to a widget.
type wrapSwitch struct{ *widget.Switch }

// A struct embedding a widget measures and styles as the widget: a
// container holding the wrapper resolves the widget's cascade (#263).
func TestWrappedWidgetStylesAsItself(t *testing.T) {
	measure := func(w widget.Widget) widget.Size {
		root := widget.NewBox(widget.Row, 0, 0)
		root.Append(w, false)
		con := widget.Constraints{Max: widget.Size{W: 300, H: 100}}
		root.Measure(con)
		root.Arrange(render.Rect{W: 300, H: 100})
		return widget.MeasureAsChild(w, con)
	}
	plain := measure(wrapSwitch{widget.NewSwitch(false)})
	if bare := measure(widget.NewSwitch(false)); plain != bare {
		t.Errorf("unstyled wrapped switch %v, want the bare one's %v", plain, bare)
	}
	widget.LoadStylesheet(`switch { min-width: 50px; min-height: 30px; } .setting-control switch { min-width: calc(2rem + 20px); min-height: calc(1rem + 14px); }`)
	t.Cleanup(func() { widget.LoadStylesheet("") })
	if got := measure(widget.NewSwitch(false)); got != (widget.Size{W: 50, H: 30}) {
		t.Errorf("bare switch %v, want 50x30", got)
	}
	if got := measure(wrapSwitch{widget.NewSwitch(false)}); got != (widget.Size{W: 50, H: 30}) {
		t.Errorf("wrapped switch %v, want 50x30", got)
	}
	sw := wrapSwitch{widget.NewSwitch(true)}
	row := widget.NewBox(widget.Row, 0, 0)
	row.AddClass("setting-control")
	row.Append(sw, false)
	measure(row)
	if got := widget.MeasureAsChild(sw, widget.Constraints{Max: widget.Size{W: 300, H: 100}}); got != (widget.Size{W: 52, H: 30}) {
		t.Errorf("wrapped switch in a classed row %v, want the calc() 52x30", got)
	}
}
