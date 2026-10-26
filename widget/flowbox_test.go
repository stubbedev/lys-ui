package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

func flowLeaves(n int) []*solidLeaf {
	out := make([]*solidLeaf, n)
	for i := range out {
		out[i] = &solidLeaf{sz: Size{W: 40, H: 20}, col: render.RGB(0, 0, 0xff)}
	}
	return out
}

func TestFlowBoxWraps(t *testing.T) {
	f := NewFlowBox(4, 6)
	leaves := flowLeaves(5)
	for _, l := range leaves {
		f.Append(l)
	}
	if sz := f.Measure(Constraints{Max: Size{W: 1000, H: 1000}}); sz != (Size{W: 5*40 + 4*4, H: 20}) {
		t.Errorf("wide: %v, want one line", sz)
	}
	// 100px fits two 40px children and their gap per line.
	if sz := f.Measure(Constraints{Max: Size{W: 100, H: 1000}}); sz != (Size{W: 84, H: 3*20 + 2*6}) {
		t.Errorf("narrow: %v, want three lines of two", sz)
	}
	// The gaps count: at 82 two children fit but not their gap, at 126
	// three fit but not both gaps.
	if sz := f.Measure(Constraints{Max: Size{W: 82, H: 1000}}); sz.H != 5*20+4*6 {
		t.Errorf("82px: %v, want one per line", sz)
	}
	if sz := f.Measure(Constraints{Max: Size{W: 126, H: 1000}}); sz.H != 3*20+2*6 {
		t.Errorf("126px: %v, want two per line", sz)
	}
	f.Measure(Constraints{Max: Size{W: 100, H: 1000}})
	f.Arrange(render.Rect{X: 10, Y: 10, W: 100, H: 72})
	if leaves[2].bounds != (render.Rect{X: 10, Y: 36, W: 40, H: 20}) || leaves[3].bounds.X != 54 {
		t.Errorf("third child at %v, fourth at %v", leaves[2].bounds, leaves[3].bounds)
	}
	if f.IndexAt(Point{X: 60, Y: 40}) != 3 || f.IndexAt(Point{X: 52, Y: 40}) != -1 || f.ChildAt(3).Child() != leaves[3] {
		t.Error("IndexAt / ChildAt")
	}
	if f.ChildAt(4).Index() != 4 || f.ChildAt(9) != nil {
		t.Error("Index / out-of-range ChildAt")
	}
	// The per-line cap.
	f.SetMaxChildrenPerLine(2)
	if sz := f.Measure(Constraints{Max: Size{W: 1000, H: 1000}}); sz.H != 3*20+2*6 {
		t.Errorf("two per line: %v", sz)
	}
	f.SetMaxChildrenPerLine(0)
	if sz := f.Measure(Constraints{Max: Size{W: 1000, H: 1000}}); sz.H != 5*20+4*6 {
		t.Errorf("a cap below one is one: %v", sz)
	}
	f.SetMaxChildrenPerLine(7)
	// Hidden children take no slot.
	f.ChildAt(1).SetVisible(false)
	f.Measure(Constraints{Max: Size{W: 100, H: 1000}})
	f.Arrange(render.Rect{W: 100, H: 72})
	if leaves[2].bounds.X != 44 || leaves[2].bounds.Y != 0 {
		t.Errorf("after a hidden child the next sits at %v", leaves[2].bounds)
	}
	// Insert, remove, clear.
	extra := &solidLeaf{sz: Size{W: 10, H: 10}}
	f.Insert(0, extra)
	if f.ChildAt(0).Child() != extra || f.Len() != 6 || parentOf(extra) != Widget(f.ChildAt(0)) {
		t.Error("Insert at 0")
	}
	f.RemoveAt(0)
	if f.Len() != 5 || parentOf(extra) != nil || f.ChildAt(0).Child() != leaves[0] {
		t.Error("RemoveAt")
	}
	f.Clear()
	if f.Len() != 0 || len(f.Children()) != 0 || f.Measure(Constraints{Max: Size{W: 100, H: 100}}) != (Size{}) {
		t.Error("Clear")
	}
}

func TestFlowBoxChildNodes(t *testing.T) {
	loadCSS(t, `flowbox > flowboxchild.mark { background-color: #ff0000; padding: 3px; }`)
	f := NewFlowBox(0, 0)
	leaf := &solidLeaf{sz: Size{W: 10, H: 10}, col: render.RGB(0, 0, 0xff)}
	c := f.Append(leaf)
	c.AddClass("mark")
	sz := f.Measure(Constraints{Max: Size{W: 100, H: 100}})
	if sz != (Size{W: 16, H: 16}) {
		t.Errorf("styled child %v, want the padding around it", sz)
	}
	f.Arrange(render.Rect{W: 16, H: 16})
	data := make([]byte, render.Stride(16)*16)
	cv := render.New(data, render.Stride(16), 16, 16)
	f.Paint(cv)
	px := func(x, y int) render.Color { return render.ColorFromBytes(data[y*render.Stride(16)+x*4:]) }
	if px(1, 1) != render.RGB(0xff, 0, 0) || px(8, 8) != render.RGB(0, 0, 0xff) {
		t.Errorf("padding %v, child %v", px(1, 1), px(8, 8))
	}
	// Selectable, the wrapper takes the bare child's press; with no
	// selection the press reaches the child itself.
	if f.HitTest(Point{X: 8, Y: 8}) != Widget(c) || f.HitTest(Point{X: 1, Y: 1}) != Widget(c) {
		t.Error("selectable hit test")
	}
	f.SetSelectionMode(SelectionNone)
	if f.HitTest(Point{X: 8, Y: 8}) != Widget(leaf) || f.HitTest(Point{X: 1, Y: 1}) != Widget(c) {
		t.Error("hit test")
	}
}

// dropZone is an app's drop target around a FlowBox: it embeds the
// box and adds the drag interfaces.
type dropZone struct {
	*FlowBox
	entered []Point
	dropped string
}

func (z *dropZone) DragEnter(_ []string, p Point) string {
	z.entered = append(z.entered, p)
	return "text/plain"
}

func (z *dropZone) Drop(_ string, data []byte, _ Point) { z.dropped = string(data) }

// A type embedding a container is what its children's parent links
// name, so ancestor walks find the wrapper's interfaces: a drag over a
// chip inside the zone reaches the zone.
func TestEmbeddingWrapperIsTheParent(t *testing.T) {
	zone := &dropZone{FlowBox: NewFlowBox(0, 0)}
	leaf := &solidLeaf{sz: Size{W: 20, H: 20}}
	c := zone.Append(leaf)
	root := NewBox(Column, 0, 0)
	root.Append(zone, false)
	layoutRoot(root, 100, 100)
	if parentOf(c) != Widget(zone) {
		t.Fatalf("the child's parent is %T, want the wrapper", parentOf(c))
	}
	r := &Router{Root: root}
	if mime := r.DragEnter([]string{"text/plain"}, Point{X: 5, Y: 5}); mime != "text/plain" || len(zone.entered) != 1 {
		t.Fatalf("drag over the chip: mime %q, zone entered %d times", mime, len(zone.entered))
	}
	r.Drop("text/plain", []byte("0:left:1"), Point{X: 5, Y: 5})
	if zone.dropped != "0:left:1" {
		t.Errorf("the zone got %q", zone.dropped)
	}
	// Unwrapped, the parent is the container itself.
	plain := NewFlowBox(0, 0)
	pc := plain.Append(&solidLeaf{sz: Size{W: 5, H: 5}})
	if parentOf(pc) != Widget(plain) {
		t.Errorf("a plain box's child names %T", parentOf(pc))
	}
	// A removed child no longer knows its index.
	zone.RemoveAt(0)
	if c.Index() != -1 {
		t.Errorf("a removed child reports index %d", c.Index())
	}
}

// In a row short of room a flow box narrows to what is left and wraps,
// the row growing taller (GtkBox height-for-width); a non-expanding
// entry beside it narrows too, as GTK narrows any child to its minimum.
func TestFlowBoxWrapsInATightRow(t *testing.T) {
	f := NewFlowBox(4, 4)
	for _, l := range flowLeaves(5) {
		f.Append(l)
	}
	label := &solidLeaf{sz: Size{W: 50, H: 20}}
	// The flow box sits in a frame, as an app's zone row holds it: the
	// frame gives what its children can.
	frame := NewBox(Row, 0, 0)
	frame.Append(f, false)
	row := NewBox(Row, 0, 0)
	row.Append(label, false)
	row.Append(frame, false)
	wide := row.Measure(Constraints{Max: Size{W: 1000, H: 500}})
	if wide.H != 20 {
		t.Fatalf("with room: %v, want one line", wide)
	}
	// 150px: 50 for the label, 100 for the chips, two per line.
	narrow := row.Measure(Constraints{Max: Size{W: 150, H: 500}})
	if narrow.H != 3*20+2*4 || narrow.W > 150 {
		t.Errorf("tight: %v, want three lines within 150", narrow)
	}
	row.Arrange(render.Rect{W: 150, H: narrow.H})
	if b := f.Bounds(); b.X != 50 || b.W > 100 {
		t.Errorf("flow box at %v, want the 100px beside the label", b)
	}
	// Never below its widest child.
	f.Measure(Constraints{Max: Size{W: 1000, H: 500}})
	if got := f.ShrinkableWidth(); got != 5*40+4*4-40 {
		t.Errorf("shrinkable %d, want down to one 40px child", got)
	}
	face := entryFace(t)
	e := NewEntry(face, 14, 0)
	e.SetTextWidth(GTKTextWidth)
	row2 := NewBox(Row, 0, 0)
	row2.Append(e, false)
	row2.Measure(Constraints{Max: Size{W: 100, H: 100}})
	row2.Arrange(render.Rect{W: 100, H: 40})
	if e.Bounds().W > 100 {
		t.Errorf("a non-expanding entry kept %dpx in a 100px row", e.Bounds().W)
	}
}

// A list gives up its height down to one row, so a popover that the
// compositor shrank keeps it scrolling inside.
func TestListShrinks(t *testing.T) {
	face := entryFace(t)
	rows := make([]Widget, 20)
	for i := range rows {
		rows[i] = NewLabel(face, 14, "row", 0)
	}
	l := NewList[Widget](staticRows(rows), 0)
	col := NewBox(Column, 0, 0)
	col.Append(l, true)
	full := col.Measure(Constraints{Max: Size{W: 200, H: 5000}}).H
	if l.Shrinkable() != full-l.rowH || l.rowH == 0 {
		t.Fatalf("shrinkable %d of %d (row %d)", l.Shrinkable(), full, l.rowH)
	}
	col.Arrange(render.Rect{W: 200, H: 3 * l.rowH})
	if b := l.Bounds(); b.H != 3*l.rowH {
		t.Errorf("list in a short column: %v, want three rows tall", b)
	}
}

// GtkScale's nodes style the slider: the trough's background, size and
// natural width, the highlight's fill, and a knob hidden until the
// scale is hovered.
func TestSliderScaleNodes(t *testing.T) {
	loadCSS(t, `
		scale { all: unset; }
		scale trough { background-color: #202020; min-height: 8px; min-width: 160px; border-radius: 4px; }
		scale trough highlight { background-color: #0000ff; }
		scale trough slider { background-color: #ffffff; min-width: 16px; min-height: 16px; opacity: 0; }
		scale:hover trough slider { opacity: 1; }
	`)
	s := NewSlider(0, 100, 0, 50)
	if sz := s.Measure(Constraints{Max: Size{W: 1000, H: 100}}); sz != (Size{W: 160, H: 18}) {
		t.Errorf("measured %v, want the trough's 160px by 18", sz)
	}
	s.Arrange(render.Rect{W: 160, H: 20})
	paint := func() []byte {
		data := make([]byte, render.Stride(160)*20)
		cv := render.New(data, render.Stride(160), 160, 20)
		s.Paint(cv)
		return data
	}
	px := func(data []byte, x, y int) render.Color {
		return render.ColorFromBytes(data[y*render.Stride(160)+x*4:])
	}
	data := paint()
	if px(data, 40, 10) != render.RGB(0, 0, 0xff) || px(data, 120, 10) != render.RGB(0x20, 0x20, 0x20) {
		t.Errorf("highlight %v, trough %v", px(data, 40, 10), px(data, 120, 10))
	}
	if px(data, 80, 3) != 0 {
		t.Errorf("the knob shows (%v) before hover", px(data, 80, 3))
	}
	if tr := s.troughRect(); tr != (render.Rect{X: 0, Y: 6, W: 160, H: 8}) {
		t.Errorf("trough %v", tr)
	}
	s.SetHovered(true)
	data = paint()
	if px(data, 80, 10) != render.RGB(0xff, 0xff, 0xff) {
		t.Errorf("hovered knob %v, want it shown", px(data, 80, 10))
	}
	// The value maps across the styled trough's full width.
	if v := s.ValueAt(Point{X: 40}); v != 25 {
		t.Errorf("x=40 maps to %v, want 25", v)
	}
	if !slices.Contains(styleKids(s), Widget(&s.trough)) || parentOf(&s.trough.knob) != Widget(&s.trough) {
		t.Error("the scale's nodes are not walked")
	}
}

// A press on the trough warps the value there (GTK's
// primary-button-warps-slider), before any drag; disabled, nothing.
func TestSliderWarpsOnPress(t *testing.T) {
	s := NewSlider(0, 100, 0, 0)
	s.Measure(Constraints{Max: Size{W: 208, H: 18}})
	s.Arrange(render.Rect{W: 208, H: 18})
	r := &Router{Root: s}
	r.Press(BTNLeft, Point{X: 104, Y: 9}, 0)
	if v := s.Value(); v != 50 {
		t.Errorf("pressed at the middle: %v, want 50", v)
	}
	r.Release(BTNLeft, Point{X: 104, Y: 9})
	s.SetEnabled(false)
	r.Press(BTNLeft, Point{X: 4, Y: 9}, 0)
	s.PressAt(Point{X: 4, Y: 9})
	if v := s.Value(); v != 50 {
		t.Errorf("a disabled slider warped to %v", v)
	}
}

// Element reports the type's GTK node name once the widget is laid out
// in a tree, before any stylesheet asked for it.
func TestElementReportsTheTypeName(t *testing.T) {
	loadCSS(t, "")
	icon := NewThemeIcon("x", 16)
	box := NewBox(Row, 0, 0)
	box.Append(icon, false)
	box.Measure(Constraints{Max: Size{W: 100, H: 100}})
	if icon.Element() != "image" {
		t.Errorf("a parented icon reports %q", icon.Element())
	}
	icon.SetElement("custom")
	if icon.Element() != "custom" {
		t.Errorf("the override reports %q", icon.Element())
	}
}

// iconGrid is a flow box of n 20x20 cells, eight a line, in a
// vertical scroll capped at three lines.
func iconGrid(n int) (*Scroll, *FlowBox) {
	f := NewFlowBox(0, 0)
	f.SetMaxChildrenPerLine(8)
	for range n {
		f.Append(NewSpacer(20, 20))
	}
	s := NewScroll(f)
	s.VerticalOnly = true
	s.SetMaxContentHeight(60)
	return s, f
}

// shownIn reports whether w lies inside s's viewport.
func shownIn(s *Scroll, w Widget) bool {
	b := boundsOf(w)
	return b.Y >= s.view.Y && b.Y+b.H <= s.view.Y+s.viewH
}

// A flow box selection scrolls into view in the Scrolls above it: one
// made before the first layout once it has a place, a keyboard move
// past the viewport at once; a visible one moves nothing, and a later
// scroll of the user's own stands through the next layout (#326).
func TestFlowBoxRevealsItsSelection(t *testing.T) {
	s, f := iconGrid(80)
	f.Select(50)
	settle(t, s, 160, 60)
	if !shownIn(s, f.ChildAt(50)) {
		t.Fatalf("child 50 at %v outside the viewport after the first layout", f.ChildAt(50).Bounds())
	}
	s.SetOffset(0, 0)
	settle(t, s, 160, 60)
	if _, y := s.Offset(); y != 0 {
		t.Errorf("a later layout scrolled to %d, undoing the user's scroll to 0", y)
	}
	f.Select(10)
	f.KeyAction(KeyDown, 0)
	f.KeyAction(KeyDown, 0)
	settle(t, s, 160, 60)
	if !shownIn(s, f.ChildAt(f.Selected())) {
		t.Errorf("Down to child %d left it outside the viewport", f.Selected())
	}
	visible, g := iconGrid(80)
	g.Select(9)
	settle(t, visible, 160, 60)
	if _, y := visible.Offset(); y != 0 {
		t.Errorf("a visible selection scrolled the box to %d", y)
	}
}

// A list box selection made before the first layout scrolls into view
// once the rows have a place.
func TestListBoxRevealsASelectionMadeBeforeLayout(t *testing.T) {
	l := NewListBox()
	for range 30 {
		l.Append(NewSpacer(100, 20))
	}
	s := NewScroll(l)
	s.VerticalOnly = true
	s.SetMaxContentHeight(60)
	l.Select(25)
	settle(t, s, 100, 60)
	if !shownIn(s, l.RowAt(25)) {
		t.Errorf("row 25 at %v outside the viewport after the first layout", l.RowAt(25).Bounds())
	}
}
