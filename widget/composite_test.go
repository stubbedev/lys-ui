package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// A composite arranges its root as it measures it, as its parent: the
// root's margin, counted in the measure, insets it in the arrange too.
func TestCompositeArrangesTheRootInsideItsMargin(t *testing.T) {
	root := NewBox(Row, 0, 0)
	root.Append(NewSpacer(20, 10), false)
	root.SetMargin(render.Insets{Left: 5, Top: 3, Right: 5, Bottom: 3})
	c := &composite{}
	c.initComposite(c, root)
	if got := c.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 30, H: 16}) {
		t.Fatalf("measure %v, want the root plus its margin", got)
	}
	c.Arrange(render.Rect{W: 30, H: 16})
	if got := root.Bounds(); got != (render.Rect{X: 5, Y: 3, W: 20, H: 10}) {
		t.Errorf("root at %v, want inside its margin", got)
	}
}
