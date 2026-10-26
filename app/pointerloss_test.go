package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/widget"
)

// newPointerLossTestInput builds a surface input over a single button,
// the minimal tree for gesture teardown: hover, press, click state.
func newPointerLossTestInput(t *testing.T) (*surfaceInput, *widget.Button) {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	btn := widget.NewButton(widget.NewLabel(face, 13, "drag me", render.RGB(255, 255, 255)), 8, 4)
	root := widget.NewBox(widget.Row, 0, 0).Append(btn, false)
	root.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
	in := &surfaceInput{
		router:  &widget.Router{Root: root},
		request: func() {},
	}
	return in, btn
}

// The session reports a leave when the pointer truly left or the
// grabbing device went away; both must end the whole gesture, not just
// the hover. Regression for the unplug-mid-drag hole: the pressed
// widget used to stay pressed forever after wl_seat dropped the
// pointer capability.
func TestSurfaceInputLeaveEndsGesture(t *testing.T) {
	in, btn := newPointerLossTestInput(t)

	in.HandlePointerEnter(5, 5)
	in.HandlePointerButton(widget.BTNLeft, 1, 7)
	if !btn.Pressed || in.router.Pressed() == nil {
		t.Fatal("setup: the press did not land on the button")
	}

	in.HandlePointerLeave()
	if btn.Pressed || in.router.Pressed() != nil {
		t.Error("the gesture survived the pointer leave: the widget stays pressed with no device to release it")
	}
	if in.router.Hovered() != nil {
		t.Error("hover survived the pointer leave")
	}

	// The vanished device cannot finish its own gesture: no click.
	clicks := 0
	btn.OnClick = func() { clicks++ }
	in.HandlePointerButton(widget.BTNLeft, 0, 9)
	if clicks != 0 {
		t.Errorf("clicks = %d after the loss, want 0", clicks)
	}

	// A replugged device works normally: enter, press, release, click.
	in.HandlePointerEnter(5, 5)
	in.HandlePointerButton(widget.BTNLeft, 1, 11)
	in.HandlePointerButton(widget.BTNLeft, 0, 12)
	if clicks != 1 {
		t.Errorf("clicks = %d after replug, want 1", clicks)
	}
}
