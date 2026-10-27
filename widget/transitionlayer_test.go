package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A transition paints through layers, a full-canvas buffer each, and
// lets them go on its first frame at rest: a Stack after its switch, a
// Revealer after its slide, a Morph after its reveal (#411).
func TestTransitionLayersGoAtRest(t *testing.T) {
	const w, h = 120, 60
	cv := render.New(make([]byte, render.Stride(w)*h), render.Stride(w), w, h)
	paint := func(wd Widget) {
		SizeWithin(wd, Size{W: w, H: h})
		wd.Arrange(render.Rect{W: w, H: h})
		wd.Paint(cv)
	}

	stack := NewStack()
	stack.Add("a", newStub(w, h))
	stack.Add("b", newStub(w, h))
	stack.Show("a")
	stack.prev, stack.visible, stack.progress = "a", "b", 0.5
	paint(stack)
	if stack.layers[0] == nil || stack.layers[1] == nil {
		t.Fatal("a switching Stack painted without its layers")
	}
	stack.prev = ""
	paint(stack)
	if stack.layers != [2]*render.Layer{} {
		t.Error("a Stack at rest kept its switch's layers")
	}

	rev := NewRevealer(newStub(w, h))
	rev.SetTransition(RevealSlideDown)
	rev.collapse = false
	rev.progress = 0.5
	paint(rev)
	if rev.layer == nil {
		t.Fatal("a sliding Revealer painted without its layer")
	}
	rev.progress = 1
	paint(rev)
	if rev.layer != nil {
		t.Error("a Revealer at rest kept its slide's layer")
	}

	m := NewMorph(newStub(w, h))
	m.SetReveal(0.5, true)
	paint(m)
	if m.layer == nil {
		t.Fatal("a morphing Morph painted without its layer")
	}
	m.SetReveal(1, true)
	paint(m)
	if m.layer != nil || m.inner != nil {
		t.Error("a Morph at rest kept its reveal's layers")
	}
}
