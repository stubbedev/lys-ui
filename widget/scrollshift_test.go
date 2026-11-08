package widget

import (
	"fmt"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/testbuild"
)

func arrSum(ws []*Label) int {
	s := 0
	for _, w := range ws {
		s += w.arranges
	}
	return s
}

func longScroll(t *testing.T, rows int) (*Scroll, []*Label) {
	t.Helper()
	face := entryFace(t)
	col := NewBox(Column, 0, 0)
	ls := make([]*Label, 0, rows)
	for i := range rows {
		l := NewLabel(face, 14, fmt.Sprintf("row %d", i), 0)
		ls = append(ls, l)
		col.Append(l, false)
	}
	return NewScroll(col), ls
}

func TestScrollStepArrangesOnlyWhatShows(t *testing.T) {
	sc, ls := longScroll(t, 2000)
	r := render.Rect{W: 800, H: 600}
	SizeWithin(sc, Size{W: 800, H: 600})
	sc.Arrange(r)
	CollectDamage(sc)
	rowH := ls[0].Bounds().H
	visible := (600+2*scrollOverscan+2*(2*14+FocusRingWidth))/rowH + 3
	for step := range 20 {
		sc.SetOffset(0, (step+1)*10)
		before := arrSum(ls)
		sc.Arrange(r)
		if got := arrSum(ls) - before; got > visible {
			t.Fatalf("step %d arranged %d of 2000 rows, want at most %d", step, got, visible)
		}
		CollectDamage(sc)
	}
}

func TestScrollDamageStaysInTheViewport(t *testing.T) {
	sc, _ := longScroll(t, 2000)
	sc.SetMaxContentHeight(200)
	win := render.Rect{W: 800, H: 600}
	root := NewBox(Column, 0, 0).Append(sc, false).Append(NewSpacer(10, 10), false)
	SizeWithin(root, Size{W: 800, H: 600})
	root.Arrange(win)
	CollectDamage(root)
	sc.SetOffset(0, 50)
	root.Arrange(win)
	rects, _ := CollectDamage(root)
	view := sc.Bounds()
	if len(rects) == 0 {
		t.Fatal("a scroll step owed nothing")
	}
	if got := render.UnionAll(rects); got.Intersect(view) != got {
		t.Errorf("damage box %+v escapes the viewport %+v", got, view)
	}
	if got, limit := DamageArea(rects), view.W*view.H; got > limit {
		t.Errorf("damaged %d px, the viewport is %d", got, limit)
	}
}

func TestScrollStepOwesOnlyTheStripWhenItCanMove(t *testing.T) {
	sc, _ := longScroll(t, 2000)
	r := render.Rect{W: 800, H: 600}
	SizeWithin(sc, Size{W: 800, H: 600})
	sc.Arrange(r)
	CollectDamage(sc)
	sc.SetOffset(0, 30)
	sc.Arrange(r)
	rects, moves, _ := CollectFrameDamageInto(nil, nil, sc)
	if len(moves) != 1 || moves[0].DY != -30 || moves[0].DX != 0 {
		t.Fatalf("moves %+v, want the view moved up by 30", moves)
	}
	if got, want := DamageArea(rects), 800*30; got != want {
		t.Errorf("owed %d px (%+v), want the 30 px strip %d", got, rects, want)
	}
}

func TestScrollStepAllocatesNothing(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	sc, _ := longScroll(t, 2000)
	r := render.Rect{W: 800, H: 600}
	SizeWithin(sc, Size{W: 800, H: 600})
	sc.Arrange(r)
	cv := render.New(make([]byte, render.Stride(800)*600), render.Stride(800), 800, 600)
	var rects []render.Rect
	var moves []ScrollMove
	rects, moves, _ = CollectFrameDamageInto(rects, moves, sc)
	sc.Paint(cv)
	y := 0
	step := func() {
		y = (y + 7) % 500
		sc.SetOffset(0, y)
		sc.Arrange(r)
		rects, moves, _ = CollectFrameDamageInto(rects, moves, sc)
		prev := cv.PushClipDevice(render.Rect{Y: 560, W: 800, H: 40})
		sc.Paint(cv)
		cv.PopClip(prev)
	}
	for range 200 {
		step()
	}
	const budget = 0
	if allocs := testing.AllocsPerRun(200, step); allocs > budget {
		t.Errorf("a scroll step allocates %v, budget %d", allocs, budget)
	}
}

func TestCulledRowsAnswerWhereTheyAre(t *testing.T) {
	face := entryFace(t)
	col := NewBox(Column, 0, 0)
	var inner []*Label
	var rows []*Box
	for i := range 400 {
		l := NewLabel(face, 14, fmt.Sprintf("row %d", i), 0)
		inner = append(inner, l)
		row := NewBox(Row, 0, 4).Append(l, false)
		rows = append(rows, row)
		col.Append(row, false)
	}
	sc := NewScroll(col)
	r := render.Rect{W: 400, H: 300}
	SizeWithin(sc, Size{W: 400, H: 300})
	sc.Arrange(r)
	CollectDamage(sc)
	start := make([]render.Rect, len(inner))
	for i, l := range inner {
		start[i] = l.Bounds()
	}
	for step := 1; step <= 30; step++ {
		sc.SetOffset(0, step*17)
		sc.Arrange(r)
		CollectDamage(sc)
	}
	if rows[300].culled == false {
		t.Fatal("a row far below the view was arranged")
	}
	for _, i := range []int{0, 5, 200, 399} {
		want := start[i]
		want.Y -= 30 * 17
		if got := inner[i].Bounds(); got != want {
			t.Errorf("row %d's label reads %+v, want %+v", i, got, want)
		}
	}
	mid := Point{X: 20, Y: 150}
	if hit, ok := sc.HitTest(mid).(*Box); ok {
		if !hit.Bounds().Contains(mid.X, mid.Y) {
			t.Errorf("hit a box at %+v that does not hold %+v", hit.Bounds(), mid)
		}
	}
}

type recycledRows struct {
	face render.Font
	made *int
}

func (m recycledRows) Len() int { return 100000 }
func (m recycledRows) Row(i int) *Label {
	*m.made++
	return NewLabel(m.face, 14, fmt.Sprintf("row %d", i), 0)
}

func TestListRebuildsNoRowItJustShowed(t *testing.T) {
	made := 0
	l := NewList[*Label](recycledRows{face: entryFace(t), made: &made}, 24)
	r := render.Rect{W: 600, H: 600}
	SizeWithin(l, Size{W: 600, H: 600})
	l.Arrange(r)
	CollectDamage(l)
	l.setOffset(24 * 3)
	l.Arrange(r)
	before := made
	for range 10 {
		l.setOffset(0)
		l.Arrange(r)
		l.setOffset(24 * 3)
		l.Arrange(r)
	}
	if got := made - before; got != 0 {
		t.Errorf("scrolling back and forth built %d rows, want none", got)
	}
}

func TestListStepOwesItsStripAndKeepsRowsInOrder(t *testing.T) {
	made := 0
	l := NewList[*Label](recycledRows{face: entryFace(t), made: &made}, 24)
	r := render.Rect{W: 600, H: 600}
	SizeWithin(l, Size{W: 600, H: 600})
	l.Arrange(r)
	CollectDamage(l)
	l.setOffset(48)
	l.Arrange(r)
	rects, moves, _ := CollectFrameDamageInto(nil, nil, l)
	if len(moves) != 1 || moves[0].DY != -48 {
		t.Fatalf("moves %+v, want the rows moved up by 48", moves)
	}
	if got := render.UnionAll(rects); got.Intersect(r) != got {
		t.Errorf("damage %+v escapes the list", got)
	}
	last := -1
	for _, k := range l.Children() {
		idx := k.(*listRow).idx
		if idx <= last {
			t.Fatalf("children out of order: %d after %d", idx, last)
		}
		last = idx
	}
}

func TestListStepAllocatesNothingAcrossRowsItHolds(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	made := 0
	l := NewList[*Label](recycledRows{face: entryFace(t), made: &made}, 24)
	r := render.Rect{W: 600, H: 600}
	SizeWithin(l, Size{W: 600, H: 600})
	l.Arrange(r)
	var rects []render.Rect
	var moves []ScrollMove
	y := 0
	step := func() {
		y = 24 - y
		l.setOffset(y)
		l.Arrange(r)
		rects, moves, _ = CollectFrameDamageInto(rects, moves, l)
	}
	for range 20 {
		step()
	}
	const budget = 0
	if allocs := testing.AllocsPerRun(100, step); allocs > budget {
		t.Errorf("a list step over held rows allocates %v, budget %d", allocs, budget)
	}
}

func TestAnOverlappedScrollDoesNotMove(t *testing.T) {
	loadCSS(t, `.over { position: absolute; left: 10px; top: 10px; background-color: #ff0000; }
		.round { border-radius: 12px; background-color: #222222; }
		.grad { background-image: linear-gradient(red, blue); }`)
	for name, build := range map[string]func(sc *Scroll) Widget{
		"an overlapping sibling": func(sc *Scroll) Widget {
			over := NewBox(Column, 0, 0)
			over.AddClass("over")
			over.SetSizeRequest(50, 50)
			return NewBox(Column, 0, 0).Append(sc, true).Append(over, false)
		},
		"rounded corners behind": func(sc *Scroll) Widget {
			b := NewBox(Column, 0, 0).Append(sc, true)
			b.AddClass("round")
			return b
		},
		"a gradient behind": func(sc *Scroll) Widget {
			b := NewBox(Column, 0, 0).Append(sc, true)
			b.AddClass("grad")
			return b
		},
	} {
		sc, _ := longScroll(t, 100)
		root := build(sc)
		r := render.Rect{W: 300, H: 200}
		SizeWithin(root, Size{W: 300, H: 200})
		root.Arrange(r)
		CollectDamage(root)
		sc.SetOffset(0, 40)
		root.Arrange(r)
		rects, moves, _ := CollectFrameDamageInto(nil, nil, root)
		if len(moves) != 0 {
			t.Errorf("%s: moved %+v", name, moves)
		}
		if got := render.UnionAll(rects); got.Intersect(sc.Bounds()) != sc.Bounds() {
			t.Errorf("%s: owes %+v, want the whole view %+v", name, got, sc.Bounds())
		}
	}
}

type baseRow struct {
	Base
	arranges *int
}

func (r *baseRow) Measure(axis Axis, _ int) Sizes {
	if axis == Row {
		return FixedSizes(200)
	}
	return FixedSizes(20)
}

func (r *baseRow) Arrange(rect render.Rect) {
	*r.arranges++
	r.ArrangeSelf(rect)
}

func (r *baseRow) Paint(cv *render.Canvas) { cv.FillRect(r.Bounds(), render.RGB(40, 60, 80)) }

func (r *baseRow) HitTest(p Point) Widget { return r.HitLeaf(r, p) }

func baseRowScroll(declared bool) (*Scroll, *int) {
	arranges := 0
	col := NewBox(Column, 0, 0)
	for range 2000 {
		row := &baseRow{arranges: &arranges}
		if declared {
			row.SetInkBounds(render.Insets{})
		}
		col.Append(row, false)
	}
	return NewScroll(col), &arranges
}

func TestAScrollOfBaseRowsThatDeclareTheirInkMovesAndArrangesWhatShows(t *testing.T) {
	sc, arranges := baseRowScroll(true)
	r := render.Rect{W: 400, H: 300}
	SizeWithin(sc, Size{W: 400, H: 300})
	sc.Arrange(r)
	CollectFrameDamageInto(nil, nil, sc)
	visible := (300+2*scrollOverscan)/20 + 3
	for step := 1; step <= 10; step++ {
		sc.SetOffset(0, step*10)
		before := *arranges
		sc.Arrange(r)
		if got := *arranges - before; got > visible {
			t.Fatalf("step %d arranged %d of 2000 rows, want at most %d", step, got, visible)
		}
		rects, moves, _ := CollectFrameDamageInto(nil, nil, sc)
		if len(moves) != 1 || moves[0].DY != -10 {
			t.Fatalf("step %d: moves %+v, want the view copied up by 10", step, moves)
		}
		if got, want := DamageArea(rects), 400*10; got > want {
			t.Fatalf("step %d owed %d px, want the 10 px strip %d", step, got, want)
		}
	}
}

func TestAScrollOfBaseRowsThatDeclareNothingRepaintsItsView(t *testing.T) {
	sc, _ := baseRowScroll(false)
	r := render.Rect{W: 400, H: 300}
	SizeWithin(sc, Size{W: 400, H: 300})
	sc.Arrange(r)
	CollectFrameDamageInto(nil, nil, sc)
	sc.SetOffset(0, 10)
	sc.Arrange(r)
	rects, moves, _ := CollectFrameDamageInto(nil, nil, sc)
	if len(moves) != 0 {
		t.Errorf("rows that may paint anywhere were copied: %+v", moves)
	}
	if got := render.UnionAll(rects); got.Intersect(sc.Bounds()) != sc.Bounds() {
		t.Errorf("owed %+v, want the whole view %+v", got, sc.Bounds())
	}
}
