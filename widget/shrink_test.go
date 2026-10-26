package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// layoutColumn measures and arranges a column in a w x h rect.
func layoutColumn(b *Box, w, h int) {
	b.Measure(Constraints{Max: Size{W: w, H: h}})
	b.Arrange(render.Rect{W: w, H: h})
}

// TestColumnShrinksItsScroll pins GTK's give-way: a column short of
// room takes the shortfall from its expanding Scroll, so the header and
// footer around it stay inside the box and the scroll scrolls.
// Regression: a Box never shrank a child, so a form in a fixed-height
// popover ran its buttons off the bottom.
func TestColumnShrinksItsScroll(t *testing.T) {
	header, footer := newStub(50, 20), newStub(50, 30)
	scroll := propagating(NewScroll(newStub(50, 200)))
	col := NewBox(Column, 0, 0)
	col.Append(header, false)
	col.Append(scroll, true)
	col.Append(footer, false)
	layoutColumn(col, 60, 150)
	if got := scroll.Bounds().H; got != 100 {
		t.Errorf("scroll height = %d, want 100 (150 less the 50 around it)", got)
	}
	if b := footer.Bounds(); b.Y+b.H != 150 {
		t.Errorf("footer ends at %d, want inside the 150 box", b.Y+b.H)
	}

	// Never below the floor: past it the column overflows as before.
	layoutColumn(col, 60, 60)
	if got := scroll.Bounds().H; got != scrollFloor {
		t.Errorf("squeezed scroll = %d, want its floor %d", got, scrollFloor)
	}

	// Room to spare: natural plus the extra, as before.
	layoutColumn(col, 60, 400)
	if got := scroll.Bounds().H; got != 350 {
		t.Errorf("roomy scroll = %d, want 350", got)
	}
}

// TestEveryShrinkerGivesWay pins GTK's distribution: a column short
// of room takes the shortfall from every child that can give it, not
// only the expanding ones, each never below its floor.
func TestEveryShrinkerGivesWay(t *testing.T) {
	fixed := propagating(NewScroll(newStub(50, 100)))
	plain := newStub(50, 100)
	col := NewBox(Column, 0, 0)
	col.Append(fixed, false)
	col.Append(plain, true)
	layoutColumn(col, 60, 120)
	if fixed.Bounds().H >= 100 || plain.Bounds().H != 100 {
		t.Errorf("heights %d, %d: want the scroll short of natural and the stub untouched", fixed.Bounds().H, plain.Bounds().H)
	}
	if col.Shrinkable() == 0 {
		t.Error("shrinkable = 0, want the fixed scroll's capacity")
	}
}

// TestShrinkReachesANestedScroll pins the propagation: a stack of
// pages and the boxes inside it report what their scrolls can give,
// so the outer column shrinks the page that holds one.
func TestShrinkReachesANestedScroll(t *testing.T) {
	inner := propagating(NewScroll(newStub(50, 300)))
	page := NewBox(Column, 0, 0)
	page.Append(newStub(50, 40), false)
	page.Append(inner, true)
	other := newStub(50, 80)
	stack := NewStack()
	stack.Add("form", page)
	stack.Add("list", other)
	outer := NewBox(Column, 0, 0)
	outer.Append(newStub(50, 20), false)
	outer.Append(stack, true)
	layoutColumn(outer, 60, 200)
	if b := stack.Bounds(); b.H != 180 {
		t.Errorf("stack height = %d, want 180", b.H)
	}
	if got := inner.Bounds().H; got != 140 {
		t.Errorf("nested scroll = %d, want 140 (the page's 180 less its header)", got)
	}
	// The stack shrinks only as far as its least shrinkable page.
	stack.Measure(Constraints{Max: Size{W: 60, H: 1000}})
	if got, want := stack.Shrinkable(), 340-max(scrollFloor+40, 80); got != want {
		t.Errorf("stack shrinkable = %d, want %d", got, want)
	}
	row := NewBox(Row, 0, 0)
	row.Append(propagating(NewScroll(newStub(10, 100))), false)
	row.Append(newStub(10, 60), false)
	row.Measure(Constraints{Max: Size{W: 100, H: 500}})
	if got := row.Shrinkable(); got != 40 {
		t.Errorf("row shrinkable = %d, want 40 (down to its 60 child)", got)
	}
}

// TestShrinkSplitsByCapacity pins the split: two scrolls give in
// proportion to what each can, and the rounding lands somewhere.
func TestShrinkSplitsByCapacity(t *testing.T) {
	a := propagating(NewScroll(newStub(50, 132))) // can give 100
	b := propagating(NewScroll(newStub(50, 232))) // can give 200
	col := NewBox(Column, 0, 0)
	col.Append(a, true)
	col.Append(b, true)
	layoutColumn(col, 60, 232)
	if a.Bounds().H+b.Bounds().H != 232 {
		t.Errorf("total = %d, want the box's 232", a.Bounds().H+b.Bounds().H)
	}
	if ga, gb := 132-a.Bounds().H, 232-b.Bounds().H; ga != 44 || gb != 88 {
		t.Errorf("gave %d and %d, want 44 and 88, in proportion to 100 and 200", ga, gb)
	}
}

// widthGiver is a stub that can be arranged narrower than its natural
// width down to floor, an ellipsizing label's contract.
type widthGiver struct {
	stub
	floor int
}

func (g *widthGiver) ShrinkableWidth() int { return g.nat.W - g.floor }

// TestNestedShortRowsKeepTheirTakes pins the per-depth shrink buffers:
// a row short of room narrows its child rows, which are short of room
// themselves and split their own deficits while the outer row is still
// reading its takes. Regression: the inner split ran in the outer
// row's buffer and rewrote the outer takes mid-loop, so the second row
// kept most of its width and the pair overflowed the outer row.
func TestNestedShortRowsKeepTheirTakes(t *testing.T) {
	inner := func() *Box {
		r := NewBox(Row, 0, 0)
		r.Append(&widthGiver{nat: Size{W: 80, H: 10}, floor: 10}, false)
		r.Append(&widthGiver{nat: Size{W: 80, H: 10}, floor: 10}, false)
		return r
	}
	a, b := inner(), inner()
	outer := NewBox(Row, 0, 0)
	outer.Append(a, false)
	outer.Append(b, false)
	outer.Measure(Constraints{Max: Size{W: 100, H: 10}})
	outer.Arrange(render.Rect{W: 100, H: 10})
	if wa, wb := a.Bounds().W, b.Bounds().W; wa != 50 || wb != 50 {
		t.Errorf("inner rows = %d and %d wide, want 50 and 50: the 220px deficit split by equal capacity", wa, wb)
	}
}

// A composite that fills its offer (an Item) took the whole offer as
// its natural width, so beside a fixed sibling in a row it was laid out
// at the row's full width and overflowed it: a 256px dock and an
// ItemGroup page in a 900px window got 900 for the page. Such a
// composite gives the fill back down to its content's natural width,
// through a scrolled page in a stack in an overlay too.
func TestAFillingCompositeGivesWayInARow(t *testing.T) {
	face := goldenFace(t)
	layout := func(content Widget) {
		dock := NewBox(Column, 0, 0)
		dock.SetSizeRequest(256, 0)
		row := NewBox(Row, 0, 0)
		row.Append(dock, false).Append(content, true)
		row.Measure(Constraints{Min: Size{W: 900, H: 100}, Max: Size{W: 900, H: 100}})
		row.Arrange(render.Rect{W: 900, H: 100})
	}
	item := NewItem(face, "x", "")
	layout(item)
	if got, want := item.Bounds(), (render.Rect{X: 256, W: 644, H: 100}); got != want {
		t.Errorf("the item beside the dock = %v, want the row's remaining %v", got, want)
	}

	group := NewItemGroup()
	group.Append(NewItem(face, "x", "y"))
	page := NewBox(Column, 0, 0)
	page.Append(group, false)
	scroll := NewScroll(page)
	scroll.VerticalOnly = true
	stack := NewStack()
	stack.Add("page", scroll)
	overlay := NewOverlay().Append(stack)
	layout(overlay)
	if got := overlay.Bounds(); got.X != 256 || got.W != 644 {
		t.Errorf("the page beside the dock = %v, want x 256 and the 644 left", got)
	}
	if got := group.Bounds(); got.X+got.W > 900 {
		t.Errorf("the item group ends at %d, past the 900px row", got.X+got.W)
	}
}

// An overlay gives way as its main child would: a column short of room
// shrinks the scroll under it, the overlaid child stays inside the
// shrunk box, and the footer stays inside the column. Regression: an
// Overlay was no Shrinker, so a file chooser's body took its full
// height and pushed the buttons out of the panel.
func TestOverlayShrinksItsScroll(t *testing.T) {
	scroll := propagating(NewScroll(newStub(50, 300)))
	badge := newStub(10, 10)
	over := NewOverlay()
	over.Append(scroll)
	over.AppendAligned(badge, AlignEnd, AlignEnd)
	footer := newStub(50, 30)
	col := NewBox(Column, 0, 0)
	col.Append(newStub(50, 20), false)
	col.Append(over, true)
	col.Append(footer, false)
	layoutColumn(col, 60, 200)
	if got := over.Bounds().H; got != 150 {
		t.Errorf("overlay height = %d, want 150 (200 less the 50 around it)", got)
	}
	if got := scroll.Bounds().H; got != 150 {
		t.Errorf("scroll height = %d, want the overlay's 150", got)
	}
	if b, o := badge.Bounds(), over.Bounds(); b.Y+b.H != o.Y+o.H {
		t.Errorf("badge ends at %d, want the shrunk overlay's bottom %d", b.Y+b.H, o.Y+o.H)
	}
	if b := footer.Bounds(); b.Y+b.H != 200 {
		t.Errorf("footer ends at %d, want inside the 200 box", b.Y+b.H)
	}
	// Only as far as the child that needs the most height lets it.
	over.Measure(Constraints{Max: Size{W: 60, H: 1000}})
	if got, want := over.Shrinkable(), 300-scrollFloor; got != want {
		t.Errorf("shrinkable = %d, want %d", got, want)
	}
	over.Append(newStub(50, 100))
	over.Measure(Constraints{Max: Size{W: 60, H: 1000}})
	if got, want := over.Shrinkable(), 300-100; got != want {
		t.Errorf("shrinkable over a 100px stub = %d, want %d", got, want)
	}
}
