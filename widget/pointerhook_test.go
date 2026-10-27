package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func arrangedList(t *testing.T, rows int) *List {
	t.Helper()
	l := NewList(newCountingModel(rows), 20)
	SizeWithin(l, Size{W: 100, H: 1 << 20})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
	l.Paint(render.New(make([]uint8, 100*100*4), 100*4, 100, 100))
	return l
}

// A list's pointer hook sees any button with its row, click count and
// modifiers; what it claims selects nothing, what it leaves selects as
// before.
func TestListPointerHook(t *testing.T) {
	l := arrangedList(t, 3)
	var seen []PointerEvent
	var rows []int
	l.OnPointer = func(row int, e PointerEvent) bool {
		seen, rows = append(seen, e), append(rows, row)
		return e.Button == BTNMiddle || e.Mods&ModCtrl != 0
	}
	r := &Router{Root: l}
	row2 := Point{X: 50, Y: 50}
	r.Move(row2)
	r.Press(BTNMiddle, row2, ModShift)
	r.Release(BTNMiddle, row2)
	r.Press(BTNMiddle, row2, ModShift)
	r.Release(BTNMiddle, row2)
	r.Press(BTNLeft, row2, ModCtrl)
	r.Release(BTNLeft, row2)
	if len(l.Selection()) != 0 {
		t.Fatalf("claimed presses selected %v", l.Selection())
	}
	if len(seen) != 3 || seen[0].Clicks != 1 || seen[1].Clicks != 2 || seen[2].Clicks != 1 {
		t.Fatalf("saw %+v, want middle presses counting 1 and 2, then a fresh left", seen)
	}
	if seen[0].Mods != ModShift || seen[2].Mods != ModCtrl || rows[0] != 2 {
		t.Errorf("saw %+v on rows %v, want shift then ctrl on row 2", seen, rows)
	}
	r.Press(BTNLeft, row2, 0)
	r.Release(BTNLeft, row2)
	if sel := l.Selection(); len(sel) != 1 || sel[0] != 2 {
		t.Errorf("an unclaimed click selected %v, want row 2", sel)
	}
	below := Point{X: 50, Y: 90}
	r.Move(below)
	r.Press(BTNRight, below, 0)
	if rows[len(rows)-1] != -1 {
		t.Errorf("a press below the rows reported row %d, want -1", rows[len(rows)-1])
	}
}

// A list's scroll hook takes the steps it claims; the rest scroll the
// list. Pixel scrolling reaches it in whole steps.
func TestListScrollHook(t *testing.T) {
	l := arrangedList(t, 20)
	var got []ScrollEvent
	l.OnScroll = func(row int, e ScrollEvent) bool {
		got = append(got, e)
		return e.Mods&ModShift != 0
	}
	r := &Router{Root: l}
	r.Move(Point{X: 50, Y: 30})
	r.Axis(0, 1, ModShift)
	if l.offY != 0 {
		t.Fatalf("a claimed step scrolled the list to %d", l.offY)
	}
	r.Axis(0, 1, 0)
	if l.offY == 0 {
		t.Fatal("an unclaimed step did not scroll the list")
	}
	r.AxisPixels(0, 30, ModShift)
	r.AxisPixels(0, 30, ModShift)
	if len(got) != 3 || got[0].DY != 1 || got[2].DY != 1 || got[2].Mods != ModShift {
		t.Errorf("scroll hook saw %+v, want a step each from Axis and a whole step from 60 px", got)
	}
}

// A container's hook sees presses on plain content inside it but not
// on a control, whose own hook sees its presses instead.
func TestPointerHookStopsAtControls(t *testing.T) {
	face := goldenFace(t)
	btn := NewButton(NewLabel(face, 14, "button", 0), 8, 8)
	plain := NewLabel(face, 14, "plain", 0)
	row := NewBox(Row, 0, 0)
	row.Append(btn, false)
	row.Append(plain, false)
	var outer, inner []uint32
	row.SetOnPointer(func(e PointerEvent) bool { outer = append(outer, e.Button); return true })
	btn.SetOnPointer(func(e PointerEvent) bool { inner = append(inner, e.Button); return false })
	clicked := 0
	btn.OnClick = func() { clicked++ }
	arrangeTree(t, row, 300, 40)
	r := &Router{Root: row}
	center := func(w Boundser) Point {
		b := w.Bounds()
		return Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
	}
	r.Press(BTNLeft, center(btn), 0)
	r.Release(BTNLeft, center(btn))
	r.Press(BTNMiddle, center(plain), 0)
	if len(inner) != 1 || len(outer) != 1 || outer[0] != BTNMiddle {
		t.Errorf("button hook saw %v, container hook %v; want the left press and the middle press", inner, outer)
	}
	if clicked != 1 {
		t.Errorf("the button clicked %d times, want its unclaimed press to click once", clicked)
	}
}
