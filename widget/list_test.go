package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// countingModel is a spy model: it records every Row request so tests
// can pin virtualization.
type countingModel struct {
	rows   int
	called map[int]int
	w      Widget
}

func newCountingModel(rows int) *countingModel {
	return &countingModel{rows: rows, called: make(map[int]int), w: newStub(60, 20)}
}

func (m *countingModel) Len() int { return m.rows }

func (m *countingModel) Row(i int) Widget {
	m.called[i]++
	return m.w
}

func (m *countingModel) distinct() int { return len(m.called) }

func TestListVirtualization(t *testing.T) {
	model := newCountingModel(10000)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 200, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60}) // 3 rows of 20px

	cv := render.New(make([]uint8, 200*60*4), 200*4, 200, 60)
	l.Paint(cv)

	if got := model.distinct(); got > 5 {
		t.Errorf("painting 3 visible rows requested %d distinct rows, want <= 5", got)
	}

	// Scroll far away: only the new viewport's rows are requested.
	l.ScrollBy(0, 40)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60})
	l.Paint(cv)
	if got := model.distinct(); got > 8 {
		t.Errorf("after scrolling, %d distinct rows were requested cumulatively, want <= 8", got)
	}
}

// TestListSelectionClamps pins the choice that selection motion clamps
// at the ends instead of wrapping, and Enter activates once per press.
func TestListSelectionClamps(t *testing.T) {
	model := newCountingModel(5)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})

	t.Run("motion clamps at the ends", func(t *testing.T) {
		l.Select(0)
		l.KeyAction(KeyUp, 0)
		if got := l.Selected(); got != 0 {
			t.Errorf("up from top = %d, want clamped 0", got)
		}
		l.Select(4)
		l.KeyAction(KeyDown, 0)
		l.KeyAction(KeyDown, 0)
		if got := l.Selected(); got != 4 {
			t.Errorf("down past bottom = %d, want clamped 4", got)
		}
	})

	t.Run("home and end jump", func(t *testing.T) {
		l.Select(2)
		l.KeyAction(KeyHome, 0)
		if got := l.Selected(); got != 0 {
			t.Errorf("home = %d, want 0", got)
		}
		l.KeyAction(KeyEnd, 0)
		if got := l.Selected(); got != 4 {
			t.Errorf("end = %d, want 4", got)
		}
	})

	t.Run("enter activates exactly once per press", func(t *testing.T) {
		activations := 0
		l.OnActivate = func(i int) { activations++ }
		l.Select(2)
		l.KeyAction(KeyEnter, 0)
		l.KeyAction(KeyEnter, 0)
		if activations != 2 {
			t.Errorf("activations = %d after two Enters, want 2", activations)
		}
		l.KeyAction(KeyRight, 0)
		if activations != 2 {
			t.Errorf("non-Enter key activated (%d)", activations)
		}
	})
}

// TestListChanged pins model updates: Changed re-queries the length,
// drops caches past the new end, and keeps the selection clamped.
func TestListChanged(t *testing.T) {
	model := newCountingModel(10)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	l.Select(8)

	model.rows = 3
	l.Changed()
	if got := l.Selected(); got != 2 {
		t.Errorf("selection = %d after shrink, want clamped 2", got)
	}
	if len(l.rows) > 3 {
		t.Errorf("row cache holds %d entries past the new length", len(l.rows))
	}

	// A row widget for a surviving index is reused, not rebuilt.
	model.rows = 12
	l.Select(1)
	before := l.rows[1]
	l.Changed()
	if l.rows[1] != before {
		t.Error("Changed rebuilt a widget that survived the update")
	}
}

// TestListRowHit pins that hit-testing a row returns the row widget
// itself, so per-row tooltips and hover styling resolve through the
// standard tooltip machine. The loop paints before any hit can happen;
// painting populates the visible-row cache.
func TestListRowHit(t *testing.T) {
	model := newCountingModel(6)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	cv := render.New(make([]uint8, 100*60*4), 100*4, 100, 60)
	l.Paint(cv)

	got := l.HitTest(Point{X: 50, Y: 45})
	row := l.rowAt(Point{X: 50, Y: 45})
	if got != l.rows[row] {
		t.Errorf("hit widget is not the cached row %d", row)
	}
}

// paintedList is a 10-row list painted once, so its visible rows are
// cached.
func paintedList(t *testing.T, mode SelectionMode) (*List, *countingModel) {
	t.Helper()
	model := newCountingModel(10)
	l := NewList(model, 20)
	l.SetSelectionMode(mode)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	l.Paint(render.New(make([]uint8, 100*60*4), 100*4, 100, 60))
	return l, model
}

// TestListRefreshRequeriesRowsKeepingState pins Refresh: every visible
// row is asked for again, while the selection and scroll stay.
func TestListRefreshRequeriesRowsKeepingState(t *testing.T) {
	l, model := paintedList(t, SelectionSingle)
	l.ScrollBy(0, 1)
	l.Select(3)
	before := model.called[3]
	l.Refresh()
	l.Paint(render.New(make([]uint8, 100*60*4), 100*4, 100, 60))
	if model.called[3] != before+1 {
		t.Errorf("row 3 asked %d times after Refresh, want %d", model.called[3], before+1)
	}
	if l.Selected() != 3 || l.offY != 40 {
		t.Errorf("Refresh dropped state: selected %d, offset %d", l.Selected(), l.offY)
	}
}

// TestListResetDropsEverything pins Reset: the rows, the selection
// and the scroll go, and the dropped selection is reported once; a
// Reset with nothing selected reports nothing.
func TestListResetDropsEverything(t *testing.T) {
	for _, mode := range []SelectionMode{SelectionSingle, SelectionMultiple} {
		l, model := paintedList(t, mode)
		l.ScrollBy(0, 1)
		l.Select(4)
		var reports [][]int
		l.OnSelectionChanged = func(rows []int) { reports = append(reports, rows) }
		before := model.called[2]
		l.Reset()
		if len(l.rows) != 0 || l.offY != 0 || len(l.Selection()) != 0 {
			t.Errorf("mode %d: Reset kept rows %d, offset %d, selection %v", mode, len(l.rows), l.offY, l.Selection())
		}
		if len(reports) != 1 || len(reports[0]) != 0 {
			t.Errorf("mode %d: reports = %v, want one empty selection", mode, reports)
		}
		l.Paint(render.New(make([]uint8, 100*60*4), 100*4, 100, 60))
		if model.called[2] != before+1 {
			t.Errorf("mode %d: row 2 not re-queried after Reset", mode)
		}
		reports = nil
		l.Reset()
		if len(reports) != 0 {
			t.Errorf("mode %d: a Reset with nothing selected reported %v", mode, reports)
		}
	}
}

// boxModel serves Box rows holding a fixed 30px stub then an expanding
// one, fresh per request.
type boxModel struct{ n int }

func (m boxModel) Len() int { return m.n }

func (m boxModel) Row(int) *Box {
	b := NewBox(Row, 0, 0)
	b.Append(newStub(30, 10), false)
	b.Append(newStub(0, 10), true)
	return b
}

// TestListMeasuresEveryRowBeforeArranging pins that rows lay out from
// a measure: a container row arranges its children from the sizes its
// Measure recorded, and no Measure pass reaches a row except through
// the list. Only row 0 used to be measured (to derive the row height),
// so every later row laid its children out at zero width.
func TestListMeasuresEveryRowBeforeArranging(t *testing.T) {
	l := NewList[*Box](boxModel{5}, 20)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{W: 100, H: 100})
	l.Paint(render.New(make([]uint8, 100*100*4), 100*4, 100, 100))
	for i := range 5 {
		row := l.rows[i].row.(*Box)
		first, rest := row.child[0].w.(*stub).rect, row.child[1].w.(*stub).rect
		if first.W != 30 || first.Y != i*20 || rest.X != 30 || rest.W != 70 {
			t.Errorf("row %d laid out %+v / %+v, want 30px at y=%d then 70px", i, first, rest, i*20)
		}
	}
}

// A row widget styles from the stylesheet above the list, as its
// label inherits the color a rule sets there.
func TestListRowsTakeTheStylesheetAbove(t *testing.T) {
	loadCSS(t, `.host { color: #ff0000; } .host row { padding: 4px; }`)
	label := NewLabel(testFace(t), 12, "x", 0)
	row := NewBox(Row, 0, 0)
	row.Append(label, false)
	l := NewList[Widget](staticRows{row}, 0)
	host := NewBox(Column, 0, 0)
	host.AddClass("host")
	host.Append(l, true)
	for range 2 {
		host.Measure(Constraints{Max: Size{W: 100, H: 100}})
		host.Arrange(render.Rect{W: 100, H: 100})
		data := make([]byte, render.Stride(100)*100)
		host.Paint(render.New(data, render.Stride(100), 100, 100))
	}
	if got := label.style(label).Color; got != render.RGB(0xff, 0, 0) {
		t.Errorf("row label color %#08x, want the inherited red", uint32(got))
	}
	if b := label.Bounds(); b.X != 4 {
		t.Errorf("label at %v, want inside the row's 4px padding", b)
	}
}

type staticRows []Widget

func (s staticRows) Len() int         { return len(s) }
func (s staticRows) Row(i int) Widget { return s[i] }

// frame measures, arranges, and paints host twice, so the list has
// materialized and styled its row proxies.
func frame(t *testing.T, host Widget, w, h int) {
	t.Helper()
	for range 2 {
		host.Measure(Constraints{Max: Size{W: w, H: h}})
		host.Arrange(render.Rect{W: w, H: h})
		data := make([]byte, render.Stride(w)*h)
		host.Paint(render.New(data, render.Stride(w), w, h))
	}
}

// The row proxy is GTK's `row` node: a stylesheet written for GTK
// addresses it directly — `listview > row` styles the box, the row
// widget lands inside its padding, and the auto row height grows by
// it.
func TestListRowIsGtkRowBox(t *testing.T) {
	face := testFace(t)
	loadCSS(t, `listview > row { padding: 4 8; background-color: #010203; border-radius: 7; }`)
	label := NewLabel(face, 12, "x", 0)
	row := NewBox(Row, 0, 0)
	row.Append(label, false)
	l := NewList[Widget](staticRows{row}, 0)
	host := NewBox(Column, 0, 0)
	host.Append(l, true)
	frame(t, host, 100, 100)

	r := l.rows[0]
	if el := r.Element(); el != "row" {
		t.Fatalf("proxy element = %q, want row", el)
	}
	v := r.style(r)
	if got := v.Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("row background %#08x, want the `listview > row` rule", uint32(got))
	}
	if v.Radius.TopLeft != 7 {
		t.Errorf("row radius %d, want 7", v.Radius.TopLeft)
	}
	if b := label.Bounds(); b.X != 8 || b.Y != 4 {
		t.Errorf("label at (%d, %d), want inside the row's 4/8 padding", b.X, b.Y)
	}
	natural := NewLabel(face, 12, "x", 0).Measure(Constraints{Max: Size{W: 100, H: 100}})
	if l.rowH != natural.H+8 {
		t.Errorf("row height %d, want the label's %d plus the padding", l.rowH, natural.H)
	}
}

// Selection rides the row proxy's :selected state; a row the selection
// left behind drops it.
func TestListRowSelectedState(t *testing.T) {
	loadCSS(t, `row:selected { background-color: #0a0b0c; }`)
	l := NewList[Widget](staticRows{newStub(20, 10), newStub(20, 10)}, 0)
	host := NewBox(Column, 0, 0)
	host.Append(l, true)
	frame(t, host, 100, 100)

	if l.rows[0].style(l.rows[0]).Has(style.PropBackgroundColor) {
		t.Fatal("an unselected row matched :selected")
	}
	l.Select(0)
	if got := l.rows[0].style(l.rows[0]).Background; got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("selected row background %v, want the :selected rule", got)
	}
	if l.rows[1].style(l.rows[1]).Has(style.PropBackgroundColor) {
		t.Error("row 1 matched :selected")
	}
	l.Select(1)
	if l.rows[0].style(l.rows[0]).Has(style.PropBackgroundColor) {
		t.Error("row 0 kept :selected after the move")
	}
	if got := l.rows[1].style(l.rows[1]).Background; got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("newly selected row background %v, want the :selected rule", got)
	}
}

// The hover chain the router moves over motion marks the row proxy
// :hover, and clears it when the pointer leaves.
func TestListRowHoverState(t *testing.T) {
	loadCSS(t, `row:hover { background-color: #0d0e0f; }`)
	l := NewList[Widget](staticRows{newStub(20, 10), newStub(20, 10)}, 0)
	host := NewBox(Column, 0, 0)
	host.Append(l, true)
	frame(t, host, 100, 100)

	setHoverChain(nil, l.rows[0])
	if got := l.rows[0].style(l.rows[0]).Background; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("hovered row background %v, want the :hover rule", got)
	}
	setHoverChain(l.rows[0], nil)
	if l.rows[0].style(l.rows[0]).Has(style.PropBackgroundColor) {
		t.Error("row kept :hover after the pointer left")
	}
}

// Without a stylesheet the theme still tints hover and selection — the
// bands the List painted before rows styled themselves.
func TestListRowThemeFallbackFills(t *testing.T) {
	l := NewList[Widget](staticRows{newStub(20, 10), newStub(20, 10), newStub(20, 10)}, 0)
	host := NewBox(Column, 0, 0)
	host.Append(l, true)
	frame(t, host, 100, 100)
	th := Current()

	if got := l.rowFill(0); got != 0 {
		t.Errorf("resting row fill %v, want none", got)
	}
	l.Select(1)
	want := render.RGBA(th.Accent.R(), th.Accent.G(), th.Accent.B(), 70)
	if got := l.rowFill(1); got != want {
		t.Errorf("selected row fill %v, want the theme's accent band %v", got, want)
	}
	l.HoverMove(Point{X: 1, Y: l.rowH*2 + 5})
	want = render.RGBA(th.SurfaceHover.R(), th.SurfaceHover.G(), th.SurfaceHover.B(), 120)
	if got := l.rowFill(2); got != want {
		t.Errorf("hovered row fill %v, want the theme's hover band %v", got, want)
	}
}

// An auto row height is the styled row's: a stylesheet's padding
// grows every row, and a fixed height ignores it.
func TestListAutoRowHeightIsStyled(t *testing.T) {
	loadCSS(t, `.host .r { padding: 10px 0; }`)
	mk := func(fixed int) (*List, *Box) {
		var rows staticRows
		for range 3 {
			b := NewBox(Row, 0, 0)
			b.AddClass("r")
			b.Append(newStub(20, 10), false)
			rows = append(rows, b)
		}
		l := NewList[Widget](rows, fixed)
		host := NewBox(Column, 0, 0)
		host.AddClass("host")
		host.Append(l, true)
		for range 2 {
			host.Measure(Constraints{Max: Size{W: 100, H: 200}})
			host.Arrange(render.Rect{W: 100, H: 200})
		}
		return l, host
	}
	if l, _ := mk(0); l.rowH != 30 || l.Measure(Constraints{Max: Size{W: 100, H: 200}}).H != 90 {
		t.Errorf("auto row height %d (list %v), want the styled 30", l.rowH, l.Measure(Constraints{Max: Size{W: 100, H: 200}}))
	}
	if l, _ := mk(12); l.rowH != 12 {
		t.Errorf("fixed row height %d, want 12", l.rowH)
	}
}

func TestListMaxHeight(t *testing.T) {
	var rows staticRows
	for range 50 {
		rows = append(rows, newStub(20, 10))
	}
	l := NewList[Widget](rows, 10)
	con := Constraints{Max: Size{W: 100, H: 1000}}
	if h := l.Measure(con).H; h != 500 {
		t.Fatalf("natural %d, want 500", h)
	}
	l.SetMaxHeight(120)
	if h := l.Measure(con).H; h != 120 {
		t.Errorf("capped %d, want 120", h)
	}
	l.Arrange(render.Rect{W: 100, H: 120})
	l.ScrollBy(0, 1000)
	if l.offY != 380 {
		t.Errorf("scrolled to %d, want the 380 the cap leaves", l.offY)
	}
	l.SetMaxHeight(0)
	if h := l.Measure(con).H; h != 500 {
		t.Errorf("uncapped %d, want 500", h)
	}
}

func TestListSingleClickActivate(t *testing.T) {
	var rows staticRows
	for range 3 {
		rows = append(rows, newStub(20, 10))
	}
	l := NewList[Widget](rows, 10)
	var acted []int
	l.OnActivate = func(i int) { acted = append(acted, i) }
	l.Measure(Constraints{Max: Size{W: 100, H: 100}})
	l.Arrange(render.Rect{W: 100, H: 100})
	click := func(y int) {
		data := make([]byte, render.Stride(100)*100)
		l.Paint(render.New(data, render.Stride(100), 100, 100))
		if r, ok := l.HitTest(Point{X: 5, Y: y}).(*listRow); ok {
			l.rowClick(r, Point{X: 5, Y: y})
		}
	}
	click(15)
	if len(acted) != 0 || l.Selected() != 1 {
		t.Fatalf("a plain click activated %v (selected %d)", acted, l.Selected())
	}
	l.SetSingleClickActivate(true)
	click(25)
	if len(acted) != 1 || acted[0] != 2 || l.Selected() != 2 {
		t.Errorf("single-click activate: %v selected %d", acted, l.Selected())
	}
}

// Dragging several rows out shows a stack with their count, and
// painting the icon leaves the live row where the list put it.
func TestListDragIconStacksTheRows(t *testing.T) {
	l := NewList(newCountingModel(3), 20)
	l.SetSelectionMode(SelectionMultiple)
	l.DragContent = func([]int) *DragContent { return &DragContent{} }
	l.Measure(Constraints{Max: Size{W: 100, H: 60}})
	l.Arrange(render.Rect{W: 100, H: 60})
	l.Paint(render.New(make([]byte, render.Stride(100)*60), render.Stride(100), 100, 60))
	l.SelectAll()
	r := l.rows[1]
	r.SetPressed(true)
	defer r.PressEnd()
	icon, ok := r.DragIcon().(*dragStack)
	if !ok || icon.count != "3" {
		t.Fatalf("icon %T, want a stack of 3", r.DragIcon())
	}
	was := r.Bounds()
	sz := icon.Measure(Constraints{Max: Size{W: 200, H: 200}})
	icon.Arrange(render.Rect{W: sz.W, H: sz.H})
	icon.Paint(render.New(make([]byte, render.Stride(sz.W)*sz.H), render.Stride(sz.W), sz.W, sz.H))
	if r.Bounds() != was {
		t.Errorf("painting the icon moved the row to %v from %v", r.Bounds(), was)
	}
	l.Select(0)
	r.PressEnd()
	r.SetPressed(true)
	if r.DragIcon() != Widget(r) {
		t.Error("a press on an unselected row arms a stacked drag icon")
	}
}

// A viewport whose top and bottom both cut a row builds the bottom one
// too: rows 10px, 29px of view at offset 9 reach row 3 (#370).
func TestListVisibleReachesThePartialBottomRow(t *testing.T) {
	l := NewList(newCountingModel(10), 10)
	l.Measure(Constraints{Max: Size{W: 100, H: 29}})
	l.Arrange(render.Rect{W: 100, H: 29})
	l.offY = 9
	if first, last := l.visible(); first != 0 || last != 4 {
		t.Errorf("visible rows [%d, %d), want [0, 4)", first, last)
	}
}

// A row proxy's margin belongs to no row: a point there hits the list
// (#371).
func TestListRowMarginHitsNoRow(t *testing.T) {
	loadCSS(t, `listview > row { margin: 4px; }`)
	l := NewList[Widget](staticRows{newStub(20, 10), newStub(20, 10)}, 20)
	frame(t, l, 100, 100)
	if got := l.HitTest(Point{X: 50, Y: 2}); got != Widget(l) {
		t.Errorf("a point in row 0's top margin hit %T, want the list", got)
	}
	if got := l.HitTest(Point{X: 2, Y: 10}); got != Widget(l) {
		t.Errorf("a point in row 0's side margin hit %T, want the list", got)
	}
	if got, ok := l.HitTest(Point{X: 50, Y: 10}).(*listRow); !ok || got.idx != 0 {
		t.Errorf("a point inside row 0's border box hit %v, want row 0", got)
	}
}

// :nth-child counts a row's place in the model, not among the rows
// materialized beside it, so zebra stripes stay on the same items as
// the list scrolls, and every run stripes alike (#381).
func TestListRowNthChildIsTheModelIndex(t *testing.T) {
	loadCSS(t, `row:nth-child(even) { background-color: #010203; }`)
	l := NewList(newCountingModel(40), 10)
	host := NewBox(Column, 0, 0)
	host.Append(l, true)
	check := func(when string) {
		t.Helper()
		for i, r := range l.rows {
			striped := r.style(r).Has(style.PropBackgroundColor)
			if want := (i+1)%2 == 0; striped != want {
				t.Errorf("%s: row %d striped %v, want %v", when, i, striped, want)
			}
		}
	}
	frame(t, host, 100, 50)
	check("at the top")
	l.ScrollBy(0, 3)
	frame(t, host, 100, 50)
	if _, first := l.rows[0]; first {
		t.Fatal("the scroll left row 0 materialized; scroll further")
	}
	check("scrolled")
}
