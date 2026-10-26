package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestBoxSkipsInvisibleChildren(t *testing.T) {
	row := NewBox(Row, 0, 0)
	a := NewLabel(testFace(t), 12, "aa", 0xFF000000)
	b := NewLabel(testFace(t), 12, "bb", 0xFF000000)
	row.Append(a, false)
	row.Append(b, false)
	row.Measure(Constraints{Max: Size{W: 200, H: 40}})
	row.Arrange(render.Rect{W: 100, H: 40})
	if row.Bounds().W < 40 {
		t.Fatalf("row = %+v, want both labels sized", row.Bounds())
	}

	// Hiding the first label collapses its slot; the sibling slides in.
	a.SetVisible(false)
	row.Measure(Constraints{Max: Size{W: 200, H: 40}})
	row.Arrange(render.Rect{W: 100, H: 40})
	if got := b.Bounds(); got.X != row.Bounds().X {
		t.Errorf("hidden label left a gap: b at %+v", got)
	}
	if IsVisible(a) {
		t.Error("a reports visible after SetVisible(false)")
	}
	if !IsVisible(b) {
		t.Error("b lost visibility")
	}

	// An invisible child drops out of hit testing.
	if hit := row.HitTest(Point{X: 2, Y: 2}); hit == a {
		t.Error("hit test returned the hidden label")
	}

	// Showing it again restores the layout.
	a.SetVisible(true)
	row.Measure(Constraints{Max: Size{W: 200, H: 40}})
	row.Arrange(render.Rect{W: 100, H: 40})
	if got := b.Bounds(); got.X == row.Bounds().X {
		t.Errorf("b never moved back: %+v", got)
	}
}

func TestIsVisibleFoldsAncestors(t *testing.T) {
	root := NewBox(Row, 0, 0)
	inner := NewBox(Row, 0, 0)
	leaf := NewLabel(testFace(t), 12, "x", 0xFF000000)
	root.Append(inner, false)
	inner.Append(leaf, false)
	root.Measure(Constraints{Max: Size{W: 200, H: 40}})
	root.Arrange(render.Rect{W: 100, H: 40})

	// The leaf's own flag stays true while only its ancestor is hidden;
	// folding happens through IsVisible.
	inner.SetVisible(false)
	if !leaf.Visible() {
		t.Error("leaf's own flag followed the ancestor")
	}
	if IsVisible(leaf) {
		t.Error("IsVisible did not fold the hidden ancestor")
	}
}
