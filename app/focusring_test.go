package app

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// The ring marks keyboard focus only: a click focuses without one
// (GTK's :focus-visible), Tab traversal shows it.
func TestFocusRingFollowsFocusVisible(t *testing.T) {
	a, b := widget.NewSwitch(false), widget.NewSwitch(false)
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(a, false).Append(b, false)
	root.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 100}})
	root.Arrange(render.Rect{W: 100, H: 100})
	w := &hostWindow{router: &widget.Router{Root: root}}

	w.router.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5}, 0)
	w.router.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	if w.router.Focused() != widget.Widget(a) {
		t.Fatalf("a click focused %v, want the first switch", w.router.Focused())
	}
	if f := w.focusRingWidget(); f != nil {
		t.Errorf("pointer focus shows a ring around %v", f)
	}
	w.router.FocusNext()
	if f := w.focusRingWidget(); f != widget.Widget(b) || widget.FocusVisible(a) || !widget.FocusVisible(b) {
		t.Errorf("keyboard focus ring around %v (a visible %v, b visible %v)", f, widget.FocusVisible(a), widget.FocusVisible(b))
	}
}
