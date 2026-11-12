package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestCaretSitsOnTheSideOfTheClusterPastIt(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		dir        Direction
		shape      string
		leftward   bool
	}{
		{"ltr bar", "abc", DirectionLTR, "bar", false},
		{"ltr block", "abc", DirectionLTR, "block", false},
		{"rtl bar", "אבג", DirectionRTL, "bar", true},
		{"rtl block", "אבג", DirectionRTL, "block", true},
		{"rtl underscore", "אבג", DirectionRTL, "underscore", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loadCSS(t, `entry, textview { caret-shape: `+tc.shape+`; }`)
			e := NewEntry(testFace(t), 14, render.RGB(0xff, 0xff, 0xff))
			e.SetDirection(tc.dir)
			e.SetText(tc.text)
			ta := NewTextArea(testFace(t), 14, render.RGB(0xff, 0xff, 0xff))
			ta.SetDirection(tc.dir)
			ta.SetText(tc.text)
			root := NewBox(Column, 0, 0)
			root.Append(e, false).Append(ta, false)
			arrangeTree(t, root, 200, 120)
			r := &Router{Root: root}

			if err := r.SetFocus(e); err != nil {
				t.Fatal(err)
			}
			e.Select(0, 0)
			sh := e.shape(e.displayText())
			c, _ := e.caretRect(sh)
			assertCaretSide(t, "entry", c, e.caretX(sh, 0), e.contentRect(), tc.leftward)

			if err := r.SetFocus(ta); err != nil {
				t.Fatal(err)
			}
			ta.SetCursor(0, 0)
			c, _ = ta.caretRect()
			assertCaretSide(t, "text area", c, ta.caretX(0, 0), ta.textRect(), tc.leftward)
		})
	}
}

func assertCaretSide(t *testing.T, field string, c render.Rect, x int, box render.Rect, leftward bool) {
	t.Helper()
	if leftward && c.X+c.W != x || !leftward && c.X != x {
		t.Errorf("%s caret %v at boundary x %d, want it extending leftward=%v", field, c, x, leftward)
	}
	if c.X < box.X || c.X+c.W > box.X+box.W {
		t.Errorf("%s caret %v leaves the text box %v", field, c, box)
	}
}
