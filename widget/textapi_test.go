package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// Entry and TextArea take caret and selection in A11yState's rune
// offsets, and report per-character boxes that OffsetAt inverts.
func TestTextSelectorAndGeometry(t *testing.T) {
	face := testFace(t)
	e := NewEntry(face, 14, render.RGB(0, 0, 0))
	e.SetText("héllo")
	SizeWithin(e, Size{W: 300, H: 40})
	e.Arrange(render.Rect{W: 300, H: 40})
	ta := NewTextArea(face, 14, render.RGB(0, 0, 0))
	ta.SetText("ab\ncdé")
	SizeWithin(ta, Size{W: 300, H: 200})
	ta.Arrange(render.Rect{Y: 50, W: 300, H: 200})

	for _, c := range []struct {
		name string
		w    interface {
			Widget
			TextSelector
			TextGeometry
		}
		n int
	}{{"entry", e, 5}, {"textarea", ta, 6}} {
		t.Run(c.name, func(t *testing.T) {
			c.w.Select(1, 3)
			st := Describe(c.w)
			if !st.HasSelection || st.SelStart != 1 || st.SelEnd != 3 || st.Caret != 3 {
				t.Errorf("Select(1,3) = %d..%d caret %d", st.SelStart, st.SelEnd, st.Caret)
			}
			c.w.SetCaret(99)
			if st := Describe(c.w); st.HasSelection || st.Caret != c.n {
				t.Errorf("SetCaret past the end = caret %d sel %v, want %d", st.Caret, st.HasSelection, c.n)
			}
			prev := render.Rect{}
			for i := range c.n {
				r, ok := c.w.CharExtents(i)
				if !ok || r.H <= 0 {
					t.Fatalf("CharExtents(%d) = %+v %v", i, r, ok)
				}
				if r.W > 0 {
					if got := c.w.OffsetAt(Point{X: r.X + r.W/2, Y: r.Y + r.H/2}); got != i {
						t.Errorf("OffsetAt(center of %d) = %d", i, got)
					}
				}
				if i > 0 && r.Y == prev.Y && r.X < prev.X {
					t.Errorf("char %d at x %d left of char %d at %d", i, r.X, i-1, prev.X)
				}
				prev = r
			}
			if _, ok := c.w.CharExtents(c.n); ok {
				t.Error("CharExtents past the end reported a box")
			}
			if got := c.w.OffsetAt(Point{X: -5, Y: -5}); got != -1 {
				t.Errorf("OffsetAt outside = %d, want -1", got)
			}
		})
	}
	// The TextArea's second line sits below its first.
	a, _ := ta.CharExtents(0)
	c, _ := ta.CharExtents(3)
	if c.Y <= a.Y {
		t.Errorf("line 2 char at y %d, not below line 1's %d", c.Y, a.Y)
	}
}
