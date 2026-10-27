package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A size request floors the measure a container takes (the border box,
// margin outside), an aligned child keeps at least the request, and a
// zero axis or a larger natural size is untouched.
func TestSizeRequest(t *testing.T) {
	child := NewDrawingArea(40, 20)
	child.SetSizeRequest(100, 0)
	box := NewBox(Row, 0, 0)
	box.Append(child, false)
	if got := SizeWithin(box, Size{W: 500, H: 500}); got != (Size{W: 100, H: 20}) {
		t.Errorf("box measured %v, want the 100 request and the natural 20", got)
	}
	child.SetMargin(render.Insets{Left: 5, Right: 5})
	if got := SizeWithin(box, Size{W: 500, H: 500}); got.W != 110 {
		t.Errorf("with margins %v, want 110 (the request plus the margin)", got)
	}
	child.SetMargin(render.Insets{})
	child.SetHAlign(AlignCenter)
	SizeWithin(box, Size{W: 300, H: 20})
	box.Arrange(render.Rect{W: 300, H: 20})
	if b := child.Bounds(); b.W != 100 || b.X != 0 {
		t.Errorf("aligned child at %+v, want the request's 100 wide", b)
	}
	child.SetSizeRequest(10, 5)
	if got := SizeWithin(box, Size{W: 500, H: 500}); got != (Size{W: 40, H: 20}) {
		t.Errorf("a request under the natural size measured %v, want the natural 40x20", got)
	}
	child.SetSizeRequest(1000, 0)
	if got := SizeWithin(box, Size{W: 300, H: 500}); got.W != 1000 {
		t.Errorf("a request over the space measured %v, want the 1000 it needs", got)
	}
	if child.SizeRequest() != (Size{W: 1000}) {
		t.Errorf("SizeRequest = %v", child.SizeRequest())
	}
}

// A squeezed row takes the shortfall from children that can give it,
// never past a size request or a CSS width, and an aligned child with
// a CSS size keeps it even with no content of its own.
func TestFloorsHoldInASqueeze(t *testing.T) {
	face := goldenFace(t)
	long := NewLabel(face, 13, "a very long name that cannot possibly fit in this row at all", 0)
	long.SetEllipsize(EllipsizeEnd)
	requested := NewLabel(face, 13, "requested cell text", 0)
	requested.SetEllipsize(EllipsizeEnd)
	requested.SetSizeRequest(90, 0)
	sized := NewLabel(face, 13, "css-sized cell text", 0)
	sized.SetEllipsize(EllipsizeEnd)
	sized.SetInlineStyle("width: 80px;")
	row := NewBox(Row, 0, 0)
	row.Append(long, true).Append(requested, false).Append(sized, false)
	SizeWithin(row, Size{W: 300, H: 30})
	row.Arrange(render.Rect{W: 300, H: 30})
	if w := requested.Bounds().W; w < 90 {
		t.Errorf("the requested cell squeezed to %d, want at least 90", w)
	}
	if w := sized.Bounds().W; w != 80 {
		t.Errorf("the css-sized cell squeezed to %d, want 80", w)
	}
	if w := long.Bounds().W; w > 300-90-80 {
		t.Errorf("the long name kept %d, want it to give way", w)
	}

	img := NewImage(nil)
	img.SetInlineStyle("width: 24px; height: 24px;")
	img.SetHAlign(AlignCenter)
	img.SetVAlign(AlignCenter)
	box := NewBox(Row, 0, 0)
	box.Append(img, false)
	SizeWithin(box, Size{W: 100, H: 100})
	box.Arrange(render.Rect{W: 100, H: 100})
	if b := img.Bounds(); b.W != 24 || b.H != 24 {
		t.Errorf("aligned css-sized image at %+v, want 24x24", b)
	}
}
