package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// moduleButton is a widget from outside the kit: it embeds Base and
// returns itself from HitTest, and records what its pointer and scroll
// hooks see.
type moduleButton struct {
	Base
	btn     *Button
	middle  int
	right   int
	scrolls []int
}

func newModuleButton(t *testing.T) *moduleButton {
	t.Helper()
	m := &moduleButton{btn: NewButton(NewLabel(testFace(t), 12, "m", 0xFF000000), 0, 0)}
	m.SetOnPointer(func(e PointerEvent) bool {
		switch e.Button {
		case BTNMiddle:
			m.middle++
		case BTNRight:
			m.right++
		default:
			return false
		}
		return true
	})
	m.SetOnScroll(func(e ScrollEvent) bool {
		if e.DY == 0 {
			return false
		}
		m.scrolls = append(m.scrolls, e.DY)
		return true
	})
	return m
}

func (m *moduleButton) Measure(con Constraints) Size { return m.btn.Measure(con) }

func (m *moduleButton) Arrange(r render.Rect) {
	m.node.Arrange(r)
	SetParents(m, m.btn)
	m.btn.Arrange(r)
}

func (m *moduleButton) Paint(cv *render.Canvas) { m.btn.Paint(cv) }

func (m *moduleButton) HitTest(p Point) Widget { return m.HitLeaf(m, p) }

// An outside widget's pointer hook sees the buttons it claims, while
// the primary button it leaves keeps its own protocol, and a disabled
// widget hears nothing.
func TestRouterPointerHooksOnAnOutsideWidget(t *testing.T) {
	mb := newModuleButton(t)
	root := NewBox(Row, 0, 0)
	root.Append(mb, false)
	r := Router{Root: root}
	root.Measure(Constraints{Max: Size{W: 100, H: 40}})
	root.Arrange(render.Rect{W: 100, H: 40})

	r.Press(BTNRight, Point{X: 2, Y: 2}, 0)
	r.Press(BTNMiddle, Point{X: 2, Y: 2}, 0)
	if mb.right != 1 || mb.middle != 1 {
		t.Errorf("right=%d middle=%d, want one hook each", mb.right, mb.middle)
	}
	r.Press(BTNLeft, Point{X: 2, Y: 2}, 0)
	r.Release(BTNLeft, Point{X: 2, Y: 2})
	if mb.right != 1 || mb.middle != 1 {
		t.Error("left press counted as another button")
	}
	mb.SetEnabled(false)
	r.Press(BTNRight, Point{X: 2, Y: 2}, 0)
	if mb.right != 1 {
		t.Error("disabled widget's hook saw a press")
	}
}

// A scroll hook takes the steps it claims and lets the rest through.
func TestRouterScrollHookTakesPrecedence(t *testing.T) {
	mb := newModuleButton(t)
	root := NewBox(Row, 0, 0)
	root.Append(mb, false)
	r := Router{Root: root}
	root.Measure(Constraints{Max: Size{W: 100, H: 40}})
	root.Arrange(render.Rect{W: 100, H: 40})
	r.Move(Point{X: 2, Y: 2})
	if got := r.Hovered(); got != Widget(mb) {
		t.Fatalf("hovered = %T, want the module button", got)
	}

	r.Axis(0, 120, 0)
	if len(mb.scrolls) != 1 || mb.scrolls[0] != 120 {
		t.Errorf("scrolls = %v, want one 120 step", mb.scrolls)
	}

	mb.scrolls = nil
	r.Axis(0, 0, 0)
	if len(mb.scrolls) != 0 {
		t.Errorf("declined step still consumed: %v", mb.scrolls)
	}
}

// A wheel scroll moved the tree under a resting pointer, but hover
// stayed on the widget the pointer last moved over until the pointer
// moved again: its tooltip opened for a control no longer under the
// pointer, at that control's new place. Rehover re-picks after a
// layout; it leaves a press in progress and an absent pointer alone.
func TestRehoverFollowsTheTreeUnderARestingPointer(t *testing.T) {
	face := testFace(t)
	a := NewButton(NewLabel(face, 12, "a", 0), 0, 0)
	b := NewButton(NewLabel(face, 12, "b", 0), 0, 0)
	col := NewBox(Column, 0, 0)
	col.Append(a, false)
	col.Append(b, false)
	layout := func(y int) {
		col.Measure(Constraints{Max: Size{W: 100, H: 400}})
		col.Arrange(render.Rect{Y: y, W: 100, H: 400})
	}
	layout(0)
	r := &Router{Root: col}
	p := Point{X: 10, Y: a.Bounds().H + 2}
	r.Move(p)
	if r.Hovered() != b {
		t.Fatalf("sanity: hovering %v, want b", r.Hovered())
	}
	layout(a.Bounds().H)
	if !r.Rehover() || r.Hovered() != a || !a.Hovered || b.Hovered {
		t.Errorf("after the tree moved: hovering %v, want a", r.Hovered())
	}
	if r.Rehover() {
		t.Error("a Rehover with nothing moved changed the hover")
	}

	r.Press(BTNLeft, p, 0)
	layout(0)
	if r.Rehover() || r.Hovered() != a {
		t.Error("a Rehover moved the hover off a press in progress")
	}
	r.Release(BTNLeft, p)

	r.Leave()
	if r.Rehover() || r.Hovered() != nil {
		t.Error("a Rehover hovered with the pointer gone")
	}
}
