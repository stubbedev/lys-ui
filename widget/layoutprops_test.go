package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func arrange(root Widget, w, h int) {
	SizeWithin(root, Size{W: w, H: h})
	root.Arrange(render.Rect{W: w, H: h})
}

func TestHExpandTakesARowsLeftoverSpace(t *testing.T) {
	a, b := NewSpacer(20, 10), NewSpacer(20, 10)
	b.SetHExpand(true)
	row := NewBox(Row, 0, 0)
	row.Append(a, false).Append(b, false)
	arrange(row, 200, 10)
	if got := b.Bounds().W; got != 180 {
		t.Errorf("expanding child width = %d, want 180", got)
	}
	if got := a.Bounds().W; got != 20 {
		t.Errorf("plain child width = %d, want its natural 20", got)
	}
}

func TestExpandPropagatesUpUntilSetExplicitly(t *testing.T) {
	leaf := NewSpacer(10, 10)
	leaf.SetHExpand(true)
	inner := NewBox(Row, 0, 0)
	inner.Append(leaf, false)
	other := NewSpacer(10, 10)
	outer := NewBox(Row, 0, 0)
	outer.Append(inner, false).Append(other, false)
	arrange(outer, 100, 10)
	if !WantsExpand(inner, Row) || inner.Bounds().W != 90 {
		t.Errorf("inner box: wants=%v width=%d, want the propagated expand and 90", WantsExpand(inner, Row), inner.Bounds().W)
	}
	if WantsExpand(inner, Column) {
		t.Error("horizontal expand leaked into the vertical axis")
	}
	inner.SetHExpand(false)
	arrange(outer, 100, 10)
	if WantsExpand(inner, Row) || inner.Bounds().W != 10 {
		t.Errorf("an explicit false did not stop the propagation: width %d", inner.Bounds().W)
	}
	leaf.SetHExpand(false)
	inner.SetHExpand(true)
	if !WantsExpand(outer, Row) {
		t.Error("the cached expand did not follow a change below")
	}
}

func TestAlignAndMarginPlaceTheChildInItsAllocation(t *testing.T) {
	child := NewSpacer(40, 10)
	child.SetHAlign(AlignCenter)
	child.SetMargin(render.Insets{Top: 5, Right: 0, Bottom: 5, Left: 0})
	col := NewBox(Column, 0, 0)
	col.Append(child, false)
	sz := SizeWithin(col, Size{W: 200, H: 100})
	if sz.H != 20 {
		t.Errorf("measured height %d, want 10 plus the 5+5 margin", sz.H)
	}
	arrange(col, 200, 100)
	if got := child.Bounds(); got != (render.Rect{X: 80, Y: 5, W: 40, H: 10}) {
		t.Errorf("child bounds %+v, want centered at its natural width inside the margin", got)
	}
	end := NewSpacer(30, 30)
	end.SetHAlign(AlignEnd)
	end.SetVAlign(AlignEnd)
	btn := NewButton(end, 0, 0)
	SizeWithin(btn, Size{W: 100, H: 100})
	btn.Arrange(render.Rect{W: 100, H: 100})
	if b := end.Bounds(); b.X+b.W != btn.Bounds().X+btn.Bounds().W || b.Y+b.H != btn.Bounds().Y+btn.Bounds().H {
		t.Errorf("end-aligned child %+v inside button %+v", b, btn.Bounds())
	}
}
