package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// cssCursorRequest maps the computed cursor onto the request the app
// hands the session: keywords by their CSS spelling, url() with a
// hotspot into a file request, auto into nothing.
func TestCSSCursorRequest(t *testing.T) {
	words := []string{
		"default", "pointer", "crosshair", "text", "wait", "progress",
		"help", "move", "n-resize", "s-resize", "e-resize", "w-resize",
		"ne-resize", "nw-resize", "se-resize", "sw-resize",
		"ns-resize", "ew-resize", "nesw-resize", "nwse-resize",
		"zoom-in", "zoom-out", "grab", "grabbing", "none",
		"context-menu", "cell", "vertical-text", "alias", "copy",
		"no-drop", "not-allowed", "col-resize", "row-resize",
		"all-scroll",
	}
	for _, w := range words {
		if got := cssCursorRequest(style.CursorSpec{Name: w}); got != w {
			t.Errorf("cursor %q requested %q", w, got)
		}
	}
	if got := cssCursorRequest(style.CursorSpec{}); got != "" {
		t.Errorf("auto requested %q, want the widget default", got)
	}
	if got := cssCursorRequest(style.CursorSpec{URL: "/c.cur", HotX: 2, HotY: 4}); got != "file:/c.cur#2,4" {
		t.Errorf("url cursor request %q", got)
	}
	if got := cssCursorRequest(style.CursorSpec{Name: "pointer", URL: "/c.cur", HotX: 2, HotY: 4}); got != "file:/c.cur#2,4,pointer" {
		t.Errorf("url cursor request with a fallback %q", got)
	}
}

// The stylesheet's cursor reaches the hovered widget and inherits
// down a container, overriding the widget's built-in shape.
func TestCSSCursorOf(t *testing.T) {
	face := entryFace(t)
	loadCSS(t, `
		entry { cursor: pointer; }
		box { cursor: crosshair; }
	`)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	if got := CSSCursorOf(e); got != "pointer" {
		t.Errorf("entry css cursor = %q, want pointer", got)
	}
	inner := NewLabel(face, 14, "inherited", render.RGB(255, 255, 255))
	box := NewBox(Row, 0, 0)
	box.Append(inner, true)
	box.Measure(Constraints{Max: Size{W: 100, H: 30}})
	box.Arrange(render.Rect{W: 100, H: 30})
	if got := CSSCursorOf(inner); got != "crosshair" {
		t.Errorf("inherited css cursor = %q, want crosshair", got)
	}
	plain := NewSpacer(4, 4)
	if got := CSSCursorOf(plain); got != "" {
		t.Errorf("unstyled cursor = %q, want none", got)
	}
}

// pointer-events: none takes a widget out of hit testing - a
// container's declaration covers its children, and the press falls
// through to whatever is behind.
func TestPointerEventsNone(t *testing.T) {
	face := entryFace(t)
	ghost := NewButton(NewLabel(face, 12, "go", render.RGB(255, 255, 255)), 4, 4)
	ghost.AddClass("ghost")
	behind := NewButton(NewLabel(face, 12, "ok", render.RGB(255, 255, 255)), 4, 4)
	root := NewStack()
	root.Add("behind", behind)
	root.Add("ghost", ghost)
	root.Measure(Constraints{Max: Size{W: 100, H: 40}})
	root.Arrange(render.Rect{W: 100, H: 40})
	if hit := root.HitTest(Point{X: 50, Y: 20}); hit == nil {
		t.Fatal("unstyled stack hit nothing")
	}
	loadCSS(t, `.ghost { pointer-events: none; }`)
	hit := root.HitTest(Point{X: 50, Y: 20})
	if hit == ghost {
		t.Fatal("the ghost still took the hit")
	}
	if hit == nil {
		t.Fatal("the hit fell through to nothing")
	}
	for w := hit; w != nil; w = parentOf(w) {
		if w == behind {
			return
		}
	}
	t.Errorf("the hit landed on %T, want the button behind", hit)
}

// user-select gates the text surfaces' selection gestures: none keeps
// a drag a caret move, all selects everything on a press.
func TestUserSelectEntry(t *testing.T) {
	face := entryFace(t)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	e.SetText("select me")
	e.Arrange(render.Rect{W: 200, H: 30})
	loadCSS(t, `entry { user-select: none; }`)
	e.ClickAt(Point{X: 10, Y: 15})
	e.DragMove(Point{X: 120, Y: 15})
	if start, end, on := e.Selection(); on && start != end {
		t.Errorf("none dragged a selection [%d,%d)", start, end)
	}
	e.DoubleClickAt(Point{X: 30, Y: 15})
	if _, _, on := e.Selection(); on {
		t.Error("none double-clicked a word selection")
	}
	loadCSS(t, `entry { user-select: all; }`)
	e.ClickAt(Point{X: 10, Y: 15})
	start, end, on := e.Selection()
	if !on || start != 0 || end != len("select me") {
		t.Errorf("all selected [%d,%d) on %v", start, end, on)
	}
}

func TestUserSelectTextArea(t *testing.T) {
	face := entryFace(t)
	ta := NewTextArea(face, 14, render.RGB(255, 255, 255))
	ta.SetText("two lines\nof text")
	ta.Arrange(render.Rect{W: 200, H: 60})
	loadCSS(t, `textview { user-select: none; }`)
	ta.ClickAt(Point{X: 10, Y: 10})
	ta.DragMove(Point{X: 150, Y: 40})
	if start, end, on := ta.Selection(); on && start != end {
		t.Errorf("none dragged a selection %v-%v", start, end)
	}
	loadCSS(t, `textview { user-select: all; }`)
	ta.ClickAt(Point{X: 10, Y: 10})
	if start, end, on := ta.Selection(); !on || start.line != 0 || start.col != 0 || end.line != 1 || end.col != len("of text") {
		t.Errorf("all selected %v-%v on %v", start, end, on)
	}
}

func TestUserSelectRichLabel(t *testing.T) {
	face := entryFace(t)
	l := NewRichLabel(face, 14, "rich text here", render.RGB(255, 255, 255))
	l.SetSelectable(true)
	l.Arrange(render.Rect{W: 200, H: 30})
	loadCSS(t, `label { user-select: none; }`)
	l.PressAt(Point{X: 2, Y: 15})
	l.DragMove(Point{X: 150, Y: 15})
	if _, on := l.SelectedText(); on {
		t.Error("none dragged a label selection")
	}
	loadCSS(t, `label { user-select: all; }`)
	l.PressAt(Point{X: 2, Y: 15})
	if got, on := l.SelectedText(); !on || got != "rich text here" {
		t.Errorf("all selected %q on %v", got, on)
	}
}

// touch-action gates the touch scroll path: pan-x keeps the wheel
// free, none stops the gesture outright. The wheel path (Axis) is
// never gated.
func TestTouchActionGatesAxisPixels(t *testing.T) {
	face := entryFace(t)
	newScroll := func() (*Scroll, *Router) {
		tall := NewBox(Column, 0, 0)
		for range 10 {
			tall.Append(newStub(50, 20), false)
		}
		s := NewScroll(tall)
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{W: 60, H: 60})
		return s, &Router{Root: s}
	}
	_ = face
	tall := NewBox(Column, 0, 0)
	for range 10 {
		st := newStub(50, 20)
		st.AddClass("gated")
		tall.Append(st, false)
	}
	s := NewScroll(tall)
	s.Measure(Constraints{Max: Size{W: 60, H: 60}})
	s.Arrange(render.Rect{W: 60, H: 60})
	r := &Router{Root: s}
	loadCSS(t, `.gated { touch-action: pan-x; }`)
	r.Move(Point{X: 10, Y: 10})
	r.AxisPixels(0, 40, 0)
	if _, y := s.Offset(); y != 0 {
		t.Errorf("pan-x scrolled vertically to %d", y)
	}
	loadCSS(t, `.gated { touch-action: none; }`)
	r.Move(Point{X: 10, Y: 10})
	r.AxisPixels(0, 40, 0)
	r.Axis(0, 2, 0)
	if _, y := s.Offset(); y == 0 {
		t.Error("none blocked the wheel path too")
	}
	loadCSS(t, "")
	s2, r2 := newScroll()
	r2.Move(Point{X: 10, Y: 10})
	r2.AxisPixels(0, 40, 0)
	if _, y := s2.Offset(); y == 0 {
		t.Error("the ungated scroll did not move")
	}
}
