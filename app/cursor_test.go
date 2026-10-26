package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// caretBox is a plain box that asks for the text caret while hovered,
// the way Entry and TextArea do.
type caretBox struct{ *widget.Box }

func (c caretBox) CursorName() string { return "xterm" }

// HitTest returns the caret box itself so the outer CursorNamer is
// reachable.
func (c caretBox) HitTest(p widget.Point) widget.Widget {
	if c.Bounds().Contains(p.X, p.Y) {
		return c
	}
	return nil
}

// newCursorTestInput builds a surface input over a tree whose left
// half asks for a caret and the right half is plain chrome. The
// session is nil: only the shape bookkeeping is under test here, the
// wire half lives in internal/wlsession.
func newCursorTestInput() *surfaceInput {
	root := widget.NewBox(widget.Row, 0, 0)
	root.Append(caretBox{Box: widget.NewBox(widget.Column, 0, 0)}, true)
	root.Append(widget.NewBox(widget.Column, 0, 0), true)
	root.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
	return &surfaceInput{
		router:  &widget.Router{Root: root},
		request: func() {},
	}
}

// Regression for xcursor stickiness: after a leave, the next enter
// must re-apply the hovered widget's shape even though the session
// already restored the arrow - a stale shape cache would skip the
// SetCursor and leave the arrow stuck over a text field.
func TestSurfaceInputLeaveForgetsShape(t *testing.T) {
	in := newCursorTestInput()

	in.HandlePointerEnter(10, 10)
	if in.lastCursor != "xterm" {
		t.Fatalf("hover over the caret box cached %q, want xterm", in.lastCursor)
	}
	in.HandlePointerLeave()
	if in.lastCursor != "" {
		t.Errorf("leave kept the shape cache %q; re-entry would skip the re-apply", in.lastCursor)
	}
	in.HandlePointerEnter(10, 10)
	if in.lastCursor != "xterm" {
		t.Errorf("re-entry did not re-apply the shape (cache %q)", in.lastCursor)
	}

	// The cache reset also runs while a modal blocks the window: the
	// session restores the arrow on leave regardless of the block.
	in.HandlePointerLeave()
	in.lastCursor = "xterm" // simulate the pre-leave cache surviving a block
	in.HandlePointerLeave()
	if in.lastCursor != "" {
		t.Errorf("a blocked leave kept the stale cache %q", in.lastCursor)
	}
}

// pinCursor is the gesture-facing capability: resize edges and chrome
// drags pin a shape by name, hover changes do not clobber it, and
// clearing it returns to the hover-derived shape.
func TestSurfaceInputPinCursor(t *testing.T) {
	in := newCursorTestInput()

	in.HandlePointerEnter(10, 10) // hovering the caret box
	in.pinCursor("resize_e")
	if in.lastCursor != "resize_e" {
		t.Fatalf("pin applied %q, want resize_e", in.lastCursor)
	}

	// Motion over plain chrome keeps the pinned shape.
	in.move(90, 90)
	if in.lastCursor != "resize_e" {
		t.Errorf("hover clobbered the pin: %q", in.lastCursor)
	}

	// Clearing the pin returns to the hovered widget's request.
	in.pinCursor("")
	in.move(10, 10) // back over the caret box
	if in.lastCursor != "xterm" {
		t.Errorf("clearing the pin left %q, want the hover shape xterm", in.lastCursor)
	}

	// The pin survives leave - an active grab must keep its cursor -
	// and re-applies on re-entry.
	in.pinCursor("grabbing")
	in.HandlePointerLeave()
	in.move(10, 10)
	if in.lastCursor != "grabbing" {
		t.Errorf("the pinned shape did not re-apply after leave: %q", in.lastCursor)
	}
}

// Widget cursor requests stay name-based end to end: whatever a
// CursorNamer returns goes to the session unchanged.
func TestSurfaceInputCursorPassThrough(t *testing.T) {
	in := newCursorTestInput()
	in.HandlePointerEnter(50, 50)
	if got := in.cursorShape(); got != "" {
		t.Errorf("plain chrome asks for %q, want no shape", got)
	}
	if in.lastCursor != "" {
		t.Errorf("applying no shape cached %q", in.lastCursor)
	}
}

// SetCursorName asks for a shape from a plain box, and an explicit
// CursorName implementation still outranks it.
func TestSetCursorName(t *testing.T) {
	b := widget.NewBox(widget.Row, 0, 0)
	if got := cursorFor(b); got != "" {
		t.Errorf("fresh box cursor = %q, want the arrow", got)
	}
	b.SetCursorName("pointer")
	if got := cursorFor(b); got != "pointer" {
		t.Errorf("SetCursorName cursor = %q, want pointer", got)
	}
	if got := widget.CursorNameOf(widget.NewSpacer(4, 4)); got != "" {
		t.Errorf("spacer cursor = %q, want empty", got)
	}
}

// The stylesheet's cursor outranks the widget's built-in shape, and
// inline styles carry it: cursorFor reads the computed cascade first.
func TestCursorForCSS(t *testing.T) {
	b := widget.NewBox(widget.Row, 0, 0)
	b.SetInlineStyle("cursor: crosshair")
	if got := cursorFor(b); got != "crosshair" {
		t.Errorf("css cursor = %q, want crosshair", got)
	}
	face, err := render.NewFixtureTypeface()
	if err != nil {
		t.Fatalf("fixture face: %v", err)
	}
	e := widget.NewEntry(face, 14, render.RGB(255, 255, 255))
	if got := cursorFor(e); got != "xterm" {
		t.Fatalf("entry cursor = %q, want its own xterm", got)
	}
	e.SetInlineStyle("cursor: pointer")
	if got := cursorFor(e); got != "pointer" {
		t.Errorf("css cursor over the widget shape = %q, want pointer", got)
	}
}
