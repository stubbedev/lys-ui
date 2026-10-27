package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// inputStub is a leaf with hover/press/click/drag state for router tests.
type inputStub struct {
	node
	nat              Size
	hovered, pressed bool
	clicks           int
	dragTo           []Point
	scroll           int
}

func (s *inputStub) SetHovered(on bool) { s.hovered = on }
func (s *inputStub) SetPressed(on bool) { s.pressed = on }
func (s *inputStub) ClickAt(p Point)    { s.clicks++ }
func (s *inputStub) DragMove(p Point)   { s.dragTo = append(s.dragTo, p) }
func (s *inputStub) ScrollBy(dy int)    { s.scroll += dy }
func (s *inputStub) Measure(axis Axis, forSize int) Sizes {
	return FixedSizes(s.nat.Along(axis))
}
func (s *inputStub) Arrange(r render.Rect) { s.node.Arrange(r) }
func (s *inputStub) Paint(*render.Canvas)  {}
func (s *inputStub) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

func TestRouterHover(t *testing.T) {
	a := &inputStub{nat: Size{W: 20, H: 20}}
	b := &inputStub{nat: Size{W: 20, H: 20}}
	box := NewBox(Row, 0, 0).Append(a, false).Append(b, false)
	SizeWithin(box, Size{W: 100, H: 100})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

	r := &Router{Root: box}
	r.Move(Point{X: 5, Y: 5})

	t.Run("hover turns on the widget under the pointer", func(t *testing.T) {
		if !a.hovered || b.hovered {
			t.Errorf("a.hovered = %v, b.hovered = %v", a.hovered, b.hovered)
		}
		if r.Hovered() != a {
			t.Errorf("Hovered = %v, want a", r.Hovered())
		}
	})

	t.Run("moving off turns hover off", func(t *testing.T) {
		r.Move(Point{X: 25, Y: 5})
		if a.hovered || !b.hovered {
			t.Errorf("a.hovered = %v, b.hovered = %v", a.hovered, b.hovered)
		}
	})

	t.Run("leave clears hover", func(t *testing.T) {
		r.Leave()
		if b.hovered || r.Hovered() != nil {
			t.Errorf("b.hovered = %v, hovered = %v", b.hovered, r.Hovered())
		}
	})
}

func TestRouterClick(t *testing.T) {
	a := &inputStub{nat: Size{W: 20, H: 20}}
	box := NewBox(Row, 0, 0).Append(a, false)
	SizeWithin(box, Size{W: 100, H: 100})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})
	r := &Router{Root: box}

	t.Run("press and release on the same widget clicks it", func(t *testing.T) {
		r.Press(BTNLeft, Point{X: 5, Y: 5}, 0)
		if !a.pressed {
			t.Fatal("press did not set pressed state")
		}
		r.Release(BTNLeft, Point{X: 6, Y: 6})
		if a.clicks != 1 {
			t.Errorf("clicks = %d, want 1", a.clicks)
		}
		if a.pressed {
			t.Error("release did not clear pressed state")
		}
	})

	t.Run("release off the pressed widget does not click", func(t *testing.T) {
		r.Press(BTNLeft, Point{X: 5, Y: 5}, 0)
		r.Release(BTNLeft, Point{X: 500, Y: 500})
		if a.clicks != 1 {
			t.Errorf("clicks = %d, want still 1", a.clicks)
		}
	})

	t.Run("non-primary buttons are ignored", func(t *testing.T) {
		r.Press(BTNLeft+1, Point{X: 5, Y: 5}, 0)
		r.Release(BTNLeft+1, Point{X: 5, Y: 5})
		if a.clicks != 1 || a.pressed {
			t.Errorf("clicks = %d pressed = %v", a.clicks, a.pressed)
		}
	})
}

func TestRouterDrag(t *testing.T) {
	s := NewSlider(0, 100, 0, 0)
	SizeWithin(s, Size{W: 500, H: 100})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 204, H: 18})
	root := NewBox(Row, 0, 0).Append(s, false)
	SizeWithin(root, Size{W: 500, H: 100})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 204, H: 18})

	r := &Router{Root: root}
	r.Press(BTNLeft, Point{X: 100, Y: 9}, 0)
	t.Run("motion while pressed drags the slider", func(t *testing.T) {
		r.Move(Point{X: 150, Y: 9})
		if s.Value() <= 0 || s.Value() >= 100 {
			t.Errorf("value = %v, want mid-drag value", s.Value())
		}
	})
	t.Run("release ends the drag", func(t *testing.T) {
		r.Release(BTNLeft, Point{X: 150, Y: 9})
		v := s.Value()
		r.Move(Point{X: 20, Y: 9})
		if s.Value() != v {
			t.Errorf("value moved after release: %v -> %v", v, s.Value())
		}
	})
}

func TestRouterPointerLost(t *testing.T) {
	a := &inputStub{nat: Size{W: 20, H: 20}}
	box := NewBox(Row, 0, 0).Append(a, false)
	SizeWithin(box, Size{W: 100, H: 100})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 20})
	r := &Router{Root: box}

	r.Move(Point{X: 5, Y: 5})
	r.Press(BTNLeft, Point{X: 5, Y: 5}, 0)
	r.Move(Point{X: 15, Y: 5}) // in-widget drag while pressed
	if !a.hovered || !a.pressed || len(a.dragTo) == 0 {
		t.Fatalf("setup: hovered=%v pressed=%v dragTo=%d", a.hovered, a.pressed, len(a.dragTo))
	}

	t.Run("losing the pointer ends hover, press, and drag", func(t *testing.T) {
		r.PointerLost()
		if a.hovered || r.Hovered() != nil {
			t.Errorf("hover survived the loss: stub=%v router=%v", a.hovered, r.Hovered())
		}
		if a.pressed || r.Pressed() != nil {
			t.Errorf("press survived the loss: stub=%v router=%v", a.pressed, r.Pressed())
		}
	})

	t.Run("the vanished device cannot finish the gesture", func(t *testing.T) {
		drags := len(a.dragTo)
		r.Move(Point{X: 18, Y: 5}) // motion with no pointer: no drag feed
		if len(a.dragTo) != drags {
			t.Error("motion after the loss fed the drag")
		}
		r.Release(BTNLeft, Point{X: 5, Y: 5}) // a release that never came
		if a.clicks != 0 {
			t.Errorf("clicks = %d, want 0: a lost pointer must not click", a.clicks)
		}
	})

	t.Run("the router works again for the replugged device", func(t *testing.T) {
		r.Move(Point{X: 5, Y: 5})
		if !a.hovered {
			t.Fatal("hover did not come back")
		}
		r.Press(BTNLeft, Point{X: 5, Y: 5}, 0)
		r.Release(BTNLeft, Point{X: 5, Y: 5})
		if a.clicks != 1 {
			t.Errorf("clicks = %d, want 1 after replug", a.clicks)
		}
	})

	t.Run("losing the pointer with no gesture is safe", func(t *testing.T) {
		r.PointerLost()
		r.PointerLost() // twice: idempotent
		if a.hovered || a.pressed || r.Hovered() != nil || r.Pressed() != nil {
			t.Error("a repeated loss disturbed clean state")
		}
	})
}

func TestRouterScrollBubbles(t *testing.T) {
	tall := NewBox(Column, 0, 0)
	for range 10 {
		tall.Append(newStub(50, 20), false)
	}
	scroll := NewScroll(tall)
	scroll.ShowBars = true
	root := NewBox(Row, 0, 0).Append(scroll, false)
	SizeWithin(root, Size{W: 100, H: 100})
	SizeWithin(scroll, Size{W: 100, H: 100})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
	scroll.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})

	t.Run("axis reaches the scroll through the parent chain", func(t *testing.T) {
		r := &Router{Root: root}
		r.Move(Point{X: 5, Y: 5})
		r.Axis(0, 2, 0)
		if _, offY := scroll.Offset(); offY != 2*wheelStep(scroll.viewH) {
			t.Errorf("scroll offset = %d, want two steps of %d", offY, wheelStep(scroll.viewH))
		}
	})
}

func TestRouterFocusKeys(t *testing.T) {
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	e.SetText("hi")
	box := NewBox(Row, 0, 0).Append(e, false)
	SizeWithin(box, Size{W: 200, H: 100})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 28})

	r := &Router{Root: box}
	t.Run("no focus swallows keys", func(t *testing.T) {
		r.Type('x')
		r.KeyAction(KeyBackspace, 0)
		if e.Text() != "hi" {
			t.Errorf("text = %q, want untouched hi", e.Text())
		}
	})

	t.Run("focus by press routes runes and actions", func(t *testing.T) {
		r.Press(BTNLeft, Point{X: 10, Y: 14}, 0)
		r.Type('!')
		if e.Text() != "hi!" {
			t.Fatalf("text = %q, want hi!", e.Text())
		}
		r.KeyAction(KeyBackspace, 0)
		if e.Text() != "hi" {
			t.Errorf("text = %q, want hi", e.Text())
		}
		if r.Focused() != e {
			t.Errorf("focus = %v, want the entry", r.Focused())
		}
	})
}

func TestParentChain(t *testing.T) {
	leaf := newStub(10, 10)
	box := NewBox(Row, 0, 0).Append(leaf, false)
	SizeWithin(box, Size{W: 100, H: 100})
	box.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})

	t.Run("arranged children know their container", func(t *testing.T) {
		if got := parentOf(leaf); got != Widget(box) {
			t.Errorf("parent = %v, want the box", got)
		}
	})

	t.Run("the root has no parent", func(t *testing.T) {
		if got := parentOf(box); got != nil {
			t.Errorf("root parent = %v, want nil", got)
		}
	})
}

func TestFocusTraversal(t *testing.T) {
	root := NewBox(Column, 4, 0)
	a := newFocusTarget()
	b := newFocusTarget()
	c := newFocusTarget()
	root.Append(NewBox(Row, 4, 0).Append(NewSpacer(2, 2), false).Append(a, false), false)
	root.Append(NewScroll(b), false)
	root.Append(c, false)
	r := &Router{Root: root}

	t.Run("focus next walks paint order across containers", func(t *testing.T) {
		r.FocusNext()
		if r.Focused() != Widget(a) {
			t.Fatalf("first focus = %v, want the first focusable", r.Focused())
		}
		r.FocusNext()
		if r.Focused() != Widget(b) {
			t.Errorf("second focus = %v, want b through the scroll", r.Focused())
		}
		r.FocusNext()
		if r.Focused() != Widget(c) {
			t.Errorf("third focus = %v, want c", r.Focused())
		}
	})

	t.Run("focus next wraps around", func(t *testing.T) {
		r.FocusNext()
		if r.Focused() != Widget(a) {
			t.Errorf("focus = %v, want wrapped to a", r.Focused())
		}
	})

	t.Run("focus prev steps backwards", func(t *testing.T) {
		r.FocusPrev()
		if r.Focused() != Widget(c) {
			t.Errorf("focus = %v, want c", r.Focused())
		}
	})

	t.Run("containers without focusable children are skipped", func(t *testing.T) {
		r2 := &Router{Root: NewBox(Column, 0, 0)}
		r2.FocusNext()
		if r2.Focused() != nil {
			t.Errorf("focus = %v, want nil with no focusable widgets", r2.Focused())
		}
	})
}

// focusTarget is a minimal KeyActionHandler for traversal tests.
type focusTarget struct {
	node
}

func newFocusTarget() *focusTarget { return &focusTarget{} }

func (f *focusTarget) Measure(axis Axis, forSize int) Sizes {
	return FixedSizes(Size{W: 8, H: 8}.Along(axis))
}

func (f *focusTarget) Paint(cv *render.Canvas) {}

func (f *focusTarget) Arrange(r render.Rect) { f.bounds = r }

func (f *focusTarget) HitTest(p Point) Widget { return f.HitLeaf(f, p) }

func (f *focusTarget) KeyAction(a KeyAction, mods Mods) {}
