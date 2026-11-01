package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func arrangesUnder(root Widget, act func()) int {
	counts := func() int {
		total := 0
		walkWidgets(root, func(w Widget) bool {
			if c, ok := w.(interface{ arrangeCount() int }); ok {
				total += c.arrangeCount()
			}
			return true
		})
		return total
	}
	before := counts()
	act()
	return counts() - before
}

func settleArrange(root Widget, r render.Rect) {
	for range 3 {
		root.Arrange(r)
	}
}

func TestASettledTreeArrangesNothing(t *testing.T) {
	root := settingsTree(t)
	r := render.Rect{W: 1605, H: 1391}
	settleArrange(root, r)
	if n := arrangesUnder(root, func() { root.Arrange(r) }); n != 0 {
		t.Errorf("an arrange with nothing changed reached %d widgets", n)
	}
}

func TestARepaintReArrangesOnlyItsOwnBranch(t *testing.T) {
	face := goldenFace(t)
	left, right := NewBox(Column, 0, 0), NewBox(Column, 0, 0)
	changed := NewLabel(face, 14, "changed", 0)
	left.Append(changed, false)
	for range 20 {
		right.Append(NewLabel(face, 14, "still", 0), false)
	}
	root := NewBox(Row, 0, 0)
	root.Append(left, true)
	root.Append(right, true)
	r := render.Rect{W: 400, H: 600}
	settleArrange(root, r)
	changed.Invalidate()
	n := arrangesUnder(root, func() { root.Arrange(r) })
	if n == 0 || n > 4 {
		t.Errorf("a repaint of one label arranged %d widgets, want its branch and the sibling box", n)
	}
}

func TestAMovedBoxStillPlacesItsChildren(t *testing.T) {
	face := goldenFace(t)
	inner := NewBox(Column, 0, 0)
	label := NewLabel(face, 14, "moves", 0)
	inner.Append(label, false)
	settleArrange(inner, render.Rect{W: 200, H: 100})
	inner.Arrange(render.Rect{X: 30, Y: 40, W: 200, H: 100})
	if b := label.Bounds(); b.X != 30 || b.Y != 40 {
		t.Errorf("label at %v after its box moved", b)
	}
}

func TestALabelGrowingRePlacesItsSettledSiblings(t *testing.T) {
	face := goldenFace(t)
	first, second := NewLabel(face, 14, "a", 0), NewLabel(face, 14, "b", 0)
	row := NewBox(Row, 0, 0)
	row.Append(first, false)
	row.Append(second, false)
	r := render.Rect{W: 800, H: 40}
	settleArrange(row, r)
	x := second.Bounds().X
	first.SetText("a much longer text than it was")
	row.Arrange(r)
	if second.Bounds().X <= x {
		t.Errorf("sibling stayed at x=%d after the label before it grew", second.Bounds().X)
	}
}

func TestAStolenChildIsTakenBackBySettledBox(t *testing.T) {
	face := goldenFace(t)
	label := NewLabel(face, 14, "shared", 0)
	home, other := NewBox(Column, 0, 0), NewBox(Column, 0, 0)
	home.Append(label, false)
	r := render.Rect{W: 300, H: 100}
	settleArrange(home, r)
	setParents(other, label)
	label.Arrange(render.Rect{X: 5, Y: 5, W: 10, H: 10})
	home.Arrange(r)
	if parentOf(label) != Widget(home) {
		t.Error("the settled box did not re-link the child another container took")
	}
	if b := label.Bounds(); b.X != 0 || b.W != 300 {
		t.Errorf("child left at %v", b)
	}
}

func TestAnOutOfFlowChildFollowsAnAncestorThroughASettledBox(t *testing.T) {
	loadCSS(t, `.anchor { position: relative; } .pin { position: absolute; right: 0; top: 0; width: 10px; height: 10px; }`)
	face := goldenFace(t)
	pin := NewLabel(face, 14, "", 0)
	pin.AddClass("pin")
	mid := NewBox(Column, 0, 0)
	mid.SetSizeRequest(100, 50)
	mid.Append(pin, false)
	anchor := NewBox(Row, 0, 0)
	anchor.AddClass("anchor")
	anchor.Append(mid, false)
	settleArrange(anchor, render.Rect{W: 400, H: 100})
	if b := pin.Bounds(); b.X != 390 {
		t.Fatalf("pin at %v, want x=390", b)
	}
	anchor.Arrange(render.Rect{W: 600, H: 100})
	if b := pin.Bounds(); b.X != 590 {
		t.Errorf("pin at %v after its containing block widened, want x=590", b)
	}
}

func TestASpinnerBackOnScreenResumesUnderASettledBox(t *testing.T) {
	spinner := NewSpinner(16)
	spinner.SetSpinning(true)
	inner := NewBox(Column, 0, 0)
	inner.Append(spinner, false)
	outer := NewBox(Column, 0, 0)
	outer.Append(inner, true)
	unhost := Host(outer)
	defer unhost()
	r := render.Rect{W: 100, H: 100}
	settleArrange(outer, r)
	if spinner.spin.cancel == nil {
		t.Fatal("the spinner never started")
	}
	outer.SetVisible(false)
	spinner.spin.arranged()
	if spinner.spin.cancel != nil {
		t.Fatal("the spinner kept running off screen")
	}
	outer.SetVisible(true)
	outer.Arrange(r)
	if spinner.spin.cancel == nil {
		t.Error("the spinner did not resume back on screen")
	}
}
