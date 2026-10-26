package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func drained(t *testing.T, root Widget) {
	t.Helper()
	root.Measure(Constraints{Max: Size{W: 300, H: 200}})
	root.Arrange(render.Rect{W: 300, H: 200})
	CollectDamage(root)
	if _, any := CollectDamage(root); any {
		t.Fatal("damage left after draining")
	}
}

func covers(rects []render.Rect, r render.Rect) bool {
	for _, d := range rects {
		if d.Intersect(r) == r {
			return true
		}
	}
	return false
}

func TestInvalidationBelowAHidingWidgetIsRepainted(t *testing.T) {
	icon := NewThemeIcon("media-playback-start", 16)
	inner := NewBox(Row, 0, 0)
	inner.Append(icon, false)
	button := NewButton(inner, 4, 4)
	root := NewBox(Column, 0, 0)
	root.Append(button, false)
	drained(t, root)

	icon.SetThemeName("media-playback-pause")
	if rects, any := CollectDamage(root); !any || !covers(rects, icon.Bounds()) {
		t.Fatalf("an icon swap inside a button damaged %v, want its bounds %v", rects, icon.Bounds())
	}
	icon.SetThemeName("media-playback-start")
	if rects, any := CollectDamage(root); !any || !covers(rects, icon.Bounds()) {
		t.Fatalf("a second swap damaged %v: a stale inner flag stopped the walk", rects)
	}
}

func TestInvalidationBelowACompositeIsRepainted(t *testing.T) {
	label := NewLabel(testFace(t), 13, "a", render.RGB(0, 0, 0))
	row := NewBox(Row, 0, 0)
	row.Append(label, false)
	title := NewWindowTitle(testFace(t), 13, "t", "")
	title.setRoot(row)
	root := NewBox(Column, 0, 0)
	root.Append(title, false)
	drained(t, root)

	label.Invalidate()
	if rects, any := CollectDamage(root); !any || !covers(rects, label.Bounds()) {
		t.Fatalf("a repaint inside a composite damaged %v, want %v", rects, label.Bounds())
	}
}
