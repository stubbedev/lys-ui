package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// layoutRoot measures and arranges root at w x h, settling pending
// layout the way a window frame does.
func layoutRoot(root Widget, w, h int) {
	for range 3 {
		root.Measure(Constraints{Max: Size{W: w, H: h}})
		root.Arrange(render.Rect{W: w, H: h})
		if !LayoutPending(root) {
			return
		}
	}
}

func TestRevealRectScrollsEachViewport(t *testing.T) {
	leaf := &solidLeaf{sz: Size{W: 50, H: 400}}
	inner := NewScroll(leaf)
	inner.VerticalOnly = true
	box := NewBox(Column, 0, 0)
	box.Append(&solidLeaf{sz: Size{W: 50, H: 300}}, false)
	holder := NewBox(Column, 0, 0)
	holder.Append(inner, false)
	box.Append(holder, false)
	box.Append(&solidLeaf{sz: Size{W: 50, H: 500}}, false)
	outer := NewScroll(box)
	outer.VerticalOnly = true
	inner.SetMaxContentHeight(50)
	layoutRoot(outer, 100, 100)

	// Already showing: nothing moves.
	RevealRect(leaf, render.Rect{X: 0, Y: 10, W: 10, H: 10})
	if _, y := outer.Offset(); y != 0 {
		t.Fatalf("a visible rect scrolled the outer view to %d", y)
	}
	// Row 200 of the leaf: the inner view brings it to its bottom edge,
	// the outer one brings the inner view to its own.
	target := render.Rect{X: leaf.bounds.X, Y: leaf.bounds.Y + 200, W: 10, H: 10}
	RevealRect(leaf, target)
	if !LayoutPending(outer) {
		t.Error("a reveal left the layout to wait for another frame")
	}
	layoutRoot(outer, 100, 100)
	if _, y := inner.Offset(); y != 160 {
		t.Errorf("inner offset %d, want 160 (row 200 at the 50px view's bottom)", y)
	}
	// The inner view now shows the row at 340-350: the outer one
	// scrolls that to its bottom, not where the row sat before.
	if _, y := outer.Offset(); y != 250 {
		t.Errorf("outer offset %d, want 250", y)
	}
	inView := leaf.bounds.Y + 200
	if ob := outer.Bounds(); inView < ob.Y || inView+10 > ob.Y+ob.H {
		t.Errorf("row at %d outside the outer viewport %v", inView, ob)
	}
	// Above the view: back to its top edge.
	RevealRect(leaf, render.Rect{X: leaf.bounds.X, Y: leaf.bounds.Y + 20, W: 10, H: 10})
	layoutRoot(outer, 100, 100)
	if _, y := inner.Offset(); y != 20 {
		t.Errorf("inner offset %d, want 20 (row 20 at the top)", y)
	}
	// Taller than the view: its top shows.
	RevealRect(leaf, render.Rect{X: leaf.bounds.X, Y: leaf.bounds.Y + 100, W: 10, H: 80})
	layoutRoot(outer, 100, 100)
	if _, y := inner.Offset(); y != 100 {
		t.Errorf("inner offset %d, want 100 (a tall rect's top)", y)
	}
	// The nearest alone.
	outer.SetOffset(0, 0)
	layoutRoot(outer, 100, 100)
	_, before := outer.Offset()
	reveal(leaf, render.Rect{X: leaf.bounds.X, Y: leaf.bounds.Y + 390, W: 10, H: 10}, false)
	if _, y := outer.Offset(); y != before {
		t.Errorf("revealing in the nearest scroll moved the outer one %d -> %d", before, y)
	}
}

func TestFocusTraversalScrollsTheFocusedIntoView(t *testing.T) {
	face := entryFace(t)
	col := NewBox(Column, 0, 0)
	var entries []*Entry
	for range 10 {
		e := NewEntry(face, 14, 0)
		entries = append(entries, e)
		col.Append(e, false)
	}
	scroll := NewScroll(col)
	scroll.VerticalOnly = true
	r := &Router{Root: scroll}
	layoutRoot(scroll, 200, 100)
	for i := range entries {
		r.FocusNext()
		layoutRoot(scroll, 200, 100)
		b := r.Focused().(*Entry).Bounds()
		if b.Y < 0 || b.Y+b.H > 100 {
			t.Fatalf("entry %d focused at %v, outside the 100px view", i, b)
		}
	}
	// Focus set by a click scrolls nothing (the click was in view).
	scroll.SetOffset(0, 0)
	layoutRoot(scroll, 200, 100)
	r.SetFocus(entries[9])
	if _, y := scroll.Offset(); y != 0 {
		t.Errorf("SetFocus scrolled to %d", y)
	}
}

// A Scroll is a CSS box like GtkScrolledWindow: its background and
// border paint, its border and padding inset the viewport, and
// min-height floors it; unstyled it paints nothing of its own.
func TestScrollCSSBox(t *testing.T) {
	loadCSS(t, `.ed { background-color: #ff0000; border: 2px solid #00ff00; padding: 5px; min-height: 80px; }`)
	leaf := &solidLeaf{sz: Size{W: 40, H: 200}, col: render.RGB(0, 0, 0xff)}
	s := propagating(NewScroll(leaf))
	s.VerticalOnly, s.ShowBars = true, true
	s.AddClass("ed")
	if sz := s.Measure(Constraints{Max: Size{W: 100, H: 300}}); sz.H != 214 {
		t.Errorf("height %d, want the content and its insets", sz.H)
	}
	s.SetMaxContentHeight(20)
	if sz := s.Measure(Constraints{Max: Size{W: 100, H: 300}}); sz.H != 94 {
		t.Errorf("capped height %d, want the 80px content min-height and the insets", sz.H)
	}
	s.Arrange(render.Rect{W: 100, H: 80})
	if leaf.bounds.X != 7 || leaf.bounds.Y != 7 || leaf.bounds.W != 100-14-gutter {
		t.Errorf("child at %v, want inset 7 by border and padding, as wide as the viewport less the gutter", leaf.bounds)
	}
	data := make([]byte, render.Stride(100)*80)
	cv := render.New(data, render.Stride(100), 100, 80)
	s.Paint(cv)
	px := func(x, y int) render.Color { return render.ColorFromBytes(data[y*render.Stride(100)+x*4:]) }
	if px(1, 40) != render.RGB(0, 0xff, 0) || px(4, 40) != render.RGB(0xff, 0, 0) || px(20, 40) != render.RGB(0, 0, 0xff) {
		t.Errorf("border %v, padding %v, content %v", px(1, 40), px(4, 40), px(20, 40))
	}
	if px(20, 76) != render.RGB(0xff, 0, 0) {
		t.Errorf("the child painted %v over the bottom padding: the viewport clips inside it", px(20, 76))
	}
	s.ScrollBy(0, 1)
	s.Arrange(render.Rect{W: 100, H: 80})
	if leaf.bounds.Y != 7-wheelStep(s.viewH) {
		t.Errorf("scrolled child at %d, want %d", leaf.bounds.Y, 7-wheelStep(s.viewH))
	}
	// A rect under the bottom padding is not in view.
	s.SetOffset(0, 0)
	s.Arrange(render.Rect{W: 100, H: 80})
	RevealRect(leaf, render.Rect{X: 20, Y: 70, W: 4, H: 4})
	if _, y := s.Offset(); y != 1 {
		t.Errorf("revealing under the padding scrolled %d, want 1", y)
	}
	s.SetOffset(0, scrollStepPx)
	s.Arrange(render.Rect{W: 100, H: 80})
	track, _ := s.vBarGeometry()
	if track.X+track.W != 100-7-barInset || track.Y != 7+barInset {
		t.Errorf("bar track %v, want inside the padding", track)
	}

	plain := NewScroll(&solidLeaf{sz: Size{W: 10, H: 10}, col: render.RGB(0, 0, 0xff)})
	plain.Measure(Constraints{Max: Size{W: 100, H: 80}})
	plain.Arrange(render.Rect{W: 100, H: 80})
	clear(data)
	plain.Paint(cv)
	if px(1, 1) != 0 {
		t.Errorf("an unstyled scroll painted %v at its corner", px(1, 1))
	}
}
