package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// panedFrame measures and arranges a paned into w x h and returns it.
func panedFrame(p *Paned, w, h int) *Paned {
	SizeWithin(p, Size{W: w, H: h})
	p.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	return p
}

// TestPanedDragMovesDividerAndClamps pins the pointer half of #71: a
// drag inside the handle's hit zone (slop included) moves the divider
// live, the panes relayout around it, and the minimums clamp
// both ends — never starving a pane past its declared minimum.
func TestPanedDragMovesDividerAndClamps(t *testing.T) {
	start := &flooredStub{floor: Size{W: 40, H: 0}}
	start.nat = Size{W: 120, H: 30}
	end := &flooredStub{floor: Size{W: 50, H: 0}}
	end.nat = Size{W: 120, H: 30}
	var positions []int
	p := NewPaned(Row, start, end)
	p.OnPositionChanged = func(pos int) { positions = append(positions, pos) }
	panedFrame(p, 300, 60)

	// The handle sits at the initial (clamped) position with room on
	// both sides; a press in the slop zone grabs it.
	if got := p.Position(); got != 120 {
		t.Fatalf("initial position = %d, want the natural 120", got)
	}
	handleX := 120 + panedHandleW/2
	if p.HitTest(Point{X: handleX - panedHandleSlop, Y: 30}) != Widget(p) {
		t.Fatal("a press in the slop zone missed the handle")
	}

	// Dragging to the middle moves the divider and both panes.
	p.SetPressed(true)
	p.DragMove(Point{X: 150, Y: 30})
	if got := p.Position(); got != 150-panedHandleW/2 {
		t.Fatalf("position after drag = %d, want pointer-centered %d", got, 150-panedHandleW/2)
	}
	if start.Bounds().W != p.Position() || end.Bounds().X != p.Position()+panedHandleW {
		t.Errorf("panes = %+v %+v, want them relaid around the divider", start.Bounds(), end.Bounds())
	}

	// The end pane's floor clamps the far end.
	p.DragMove(Point{X: 299, Y: 30})
	if got := p.Position(); got != 300-panedHandleW-50 {
		t.Fatalf("position past the end floor = %d, want the clamp %d", got, 300-panedHandleW-50)
	}
	// The start pane's floor clamps the near end.
	p.DragMove(Point{X: 0, Y: 30})
	if got := p.Position(); got != 40 {
		t.Fatalf("position past the start floor = %d, want the clamp 40", got)
	}
	// A disabled pane never drags.
	p.SetEnabled(false)
	p.DragMove(Point{X: 150, Y: 30})
	if got := p.Position(); got != 40 {
		t.Fatalf("disabled drag moved the divider to %d", got)
	}
	p.SetEnabled(true)

	// The hook fired once per applied change, always with the clamped
	// truth.
	if len(positions) < 3 {
		t.Errorf("position changes = %v, want one per applied drag", positions)
	}
	for _, pos := range positions {
		if pos < 40 || pos > 300-panedHandleW-50 {
			t.Errorf("a change reported %d, outside the clamps", pos)
		}
	}
}

// TestPanedKeyboardNudges pins the keyboard half: arrows step along
// the axis (a twentieth of the span), Home and End pin to the
// extremes, the cross-axis arrows do nothing, and the floors own the
// extremes when they must.
func TestPanedKeyboardNudges(t *testing.T) {
	start := &flooredStub{floor: Size{W: 40, H: 0}}
	start.nat = Size{W: 100, H: 30}
	end := &flooredStub{floor: Size{W: 50, H: 0}}
	end.nat = Size{W: 100, H: 30}
	p := NewPaned(Row, start, end)
	panedFrame(p, 306, 60)
	step := (306 - panedHandleW) / 20

	p.KeyAction(KeyRight, 0)
	if got := p.Position(); got != 100+step {
		t.Fatalf("nudge = %d, want %d (one step from the natural)", got, step+100)
	}
	p.KeyAction(KeyLeft, 0)
	if got := p.Position(); got != 100 {
		t.Fatalf("back nudge = %d, want the natural 100 back", got)
	}
	// The cross-axis arrows are not the handle's.
	p.KeyAction(KeyDown, 0)
	p.KeyAction(KeyUp, 0)
	if got := p.Position(); got != 100 {
		t.Fatalf("cross-axis arrows moved the divider: %d", got)
	}
	p.KeyAction(KeyEnd, 0)
	if got := p.Position(); got != 306-panedHandleW-50 {
		t.Fatalf("end = %d, want the clamped extreme", got)
	}
	p.KeyAction(KeyHome, 0)
	if got := p.Position(); got != 40 {
		t.Fatalf("home = %d, want the start floor", got)
	}
}

// TestPanedResizeRenormalizes pins GTK's keep-child-one semantics: a
// resize keeps the requested position while it fits and re-normalizes
// it to the nearest clamp when it does not — growing never stretches
// the start pane back.
func TestPanedResizeRenormalizes(t *testing.T) {
	start := squeezable(100, 30)
	end := squeezable(100, 30)
	p := NewPaned(Row, start, end)
	panedFrame(p, 400, 60)

	p.SetPosition(150)
	p.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 60})
	if got := p.Position(); got != 150 {
		t.Fatalf("position = %d, want the request 150", got)
	}

	// Shrinking below the request: the position clamps, and the
	// shrunken position is what survives when the window grows again.
	p.Arrange(render.Rect{X: 0, Y: 0, W: 120, H: 60})
	if got := p.Position(); got != 120-panedHandleW {
		t.Fatalf("position in a 120 rect = %d, want the clamp %d", got, 120-panedHandleW)
	}
	p.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 60})
	if got := p.Position(); got != 120-panedHandleW {
		t.Fatalf("position regrown = %d, want the re-normalized value kept", got)
	}
}

// TestPanedNestingAndNilPanes pins the sizing model: a paned inside a
// paned lays out through the same arrangement, and a missing pane
// collapses to the other taking everything.
func TestPanedNestingAndNilPanes(t *testing.T) {
	inner := NewPaned(Column, squeezable(50, 40), squeezable(50, 40))
	outer := NewPaned(Row, inner, squeezable(80, 100))
	panedFrame(outer, 300, 100)

	innerRect := inner.Bounds()
	if innerRect.W != outer.Position() || innerRect.H != 100 {
		t.Fatalf("inner paned = %+v, want the start share of 300x100", innerRect)
	}
	innerStart := inner.Children()[0].(interface{ Bounds() render.Rect }).Bounds()
	if innerStart.H != inner.Position() || innerStart.W != innerRect.W {
		t.Fatalf("inner start pane = %+v, want the column split", innerStart)
	}

	only := NewPaned(Row, squeezable(30, 30), nil)
	panedFrame(only, 200, 50)
	if got := only.Children()[0].(interface{ Bounds() render.Rect }).Bounds(); got != (render.Rect{W: 200, H: 50}) {
		t.Errorf("lone pane = %+v, want the whole 200x50", got)
	}
	if got := only.Position(); got != 0 {
		t.Errorf("a missing pane still positions at %d", got)
	}
}

// TestPanedChildrenKeepTheirInput pins that the handle owns only its
// zone: hits in the panes reach the panes (their own interactive
// widgets), and the Paned joins focus traversal through Children.
func TestPanedChildrenKeepTheirInput(t *testing.T) {
	start := NewButton(squeezable(20, 20), 4, 4)
	end := squeezable(40, 40)
	p := NewPaned(Row, start, end)
	panedFrame(p, 300, 60)

	if hit := p.HitTest(Point{X: 10, Y: 30}); hit != start.HitTest(Point{X: 10, Y: 30}) {
		t.Errorf("hit in the start pane = %T, want the pane's own widget", hit)
	}
	if hit := p.HitTest(Point{X: 250, Y: 30}); hit != Widget(end) {
		t.Errorf("hit in the end pane = %T, want the end pane", hit)
	}
	if got := len(p.Children()); got != 2 {
		t.Errorf("Children = %d entries, want both panes", got)
	}
	if p.CursorName() != "col-resize" {
		t.Errorf("row cursor = %q, want col-resize", p.CursorName())
	}
	if v := NewPaned(Column, nil, nil).CursorName(); v != "row-resize" {
		t.Errorf("column cursor = %q, want row-resize", v)
	}
}

// SetMaxPosition caps every way the divider moves, never under the
// start pane's floor; zero lifts the cap.
func TestPanedMaxPosition(t *testing.T) {
	start := &flooredStub{floor: Size{W: 40}}
	start.nat = Size{W: 120, H: 30}
	end := &flooredStub{}
	end.nat = Size{W: 120, H: 30}
	p := NewPaned(Row, start, end)
	p.SetMaxPosition(100)
	panedFrame(p, 400, 60)
	if got := p.Position(); got != 100 {
		t.Fatalf("natural split = %d, want capped at 100", got)
	}
	p.SetPosition(300)
	panedFrame(p, 400, 60)
	if got := p.Position(); got != 100 {
		t.Errorf("SetPosition past the cap = %d, want 100", got)
	}
	p.SetPressed(true)
	p.DragMove(Point{X: 350, Y: 30})
	if got := p.Position(); got != 100 {
		t.Errorf("drag past the cap = %d, want 100", got)
	}
	p.SetPressed(false)
	p.SetPosition(60)
	panedFrame(p, 400, 60)
	if got := p.Position(); got != 60 {
		t.Errorf("under the cap = %d, want 60 kept", got)
	}
	p.SetMaxPosition(10) // under the floor: the floor wins
	p.SetPosition(0)
	panedFrame(p, 400, 60)
	if got := p.Position(); got != 40 {
		t.Errorf("cap under the floor = %d, want the floor 40", got)
	}
	p.SetMaxPosition(0)
	p.SetPosition(300)
	panedFrame(p, 400, 60)
	if got := p.Position(); got != 300 {
		t.Errorf("uncapped = %d, want 300", got)
	}
}

// A pane's subtree styles from the stylesheet above the Paned: the
// Paned arranges its panes before its own parent records it, and the
// descendants styled meanwhile restyle once the chain completes.
func TestPanedPanesTakeTheStylesheetAbove(t *testing.T) {
	root := NewBox(Column, 0, 0)
	hdr := NewBox(Row, 0, 0)
	hdr.AddClass("x")
	hdr.Append(squeezable(12, 17), false)
	side := NewBox(Column, 0, 0)
	side.Append(hdr, false)
	root.Append(NewPaned(Row, side, NewBox(Row, 0, 0)), true)
	root.AttachStylesheet(NewStylesheet(".x { padding: 20px; }", StylePriorityUser))
	for range 2 {
		SizeWithin(root, Size{W: 900, H: 650})
		root.Arrange(render.Rect{W: 900, H: 650})
	}
	if got := SizeWithin(hdr, Size{W: 900, H: 650}); got != (Size{W: 52, H: 57}) {
		t.Errorf("styled pane child measures %v, want the 20px padding around 12x17", got)
	}
}

// The panes paint, each in its own rect, and a lone pane still does.
func TestPanedPaintsItsPanes(t *testing.T) {
	loadCSS(t, `.a { background-color: #ff0000; } .b { background-color: #0000ff; }`)
	a, b := NewBox(Row, 0, 0), NewBox(Row, 0, 0)
	a.AddClass("a")
	b.AddClass("b")
	p := panedFrame(NewPaned(Row, a, b), 100, 20)
	p.SetPosition(40)
	panedFrame(p, 100, 20)
	data := paintTree(p, 100, 20)
	if got := render.ColorFromBytes(data[10*render.Stride(100)+10*4:]); got != render.RGB(0xff, 0, 0) {
		t.Errorf("start pane pixel = %#08x, want red", uint32(got))
	}
	if got := render.ColorFromBytes(data[10*render.Stride(100)+90*4:]); got != render.RGB(0, 0, 0xff) {
		t.Errorf("end pane pixel = %#08x, want blue", uint32(got))
	}
	lone := panedFrame(NewPaned(Row, a, nil), 100, 20)
	if got := render.ColorFromBytes(paintTree(lone, 100, 20)[10*render.Stride(100)+90*4:]); got != render.RGB(0xff, 0, 0) {
		t.Errorf("lone pane pixel = %#08x, want red across", uint32(got))
	}
}

// A label inside a button under a Paned styles from the sheet above
// too, and follows the button's inherited color when it changes.
func TestButtonContentRestylesWithItsButton(t *testing.T) {
	loadCSS(t, `.b { color: #ff0000; } .b.hot { color: #00ff00; }`)
	face := testFace(t)
	label := NewLabel(face, 12, "x", 0)
	type wrapped struct{ *Button }
	btn := wrapped{NewButton(label, 0, 0)}
	btn.AddClass("b")
	side := NewBox(Column, 0, 0)
	side.Append(btn, false)
	root := NewBox(Column, 0, 0)
	root.Append(NewPaned(Row, side, NewBox(Row, 0, 0)), true)
	frame := func() {
		SizeWithin(root, Size{W: 200, H: 100})
		root.Arrange(render.Rect{W: 200, H: 100})
	}
	frame()
	frame()
	if got := label.style(label).Color; got != render.RGB(0xff, 0, 0) {
		t.Fatalf("label color %#08x, want the button's red", uint32(got))
	}
	btn.AddClass("hot")
	frame()
	if got := label.style(label).Color; got != render.RGB(0, 0xff, 0) {
		t.Errorf("label color %#08x after the button turned hot, want green", uint32(got))
	}
}

// The divider styles as `paned > separator`: its min size slots it
// between the panes, its start margin shifts it inside the slot, and
// its background paints it.
func TestPanedSeparatorNode(t *testing.T) {
	loadCSS(t, `paned > separator { min-width: 3; margin-left: -3; background-color: #010203; }`)
	p := NewPaned(Row, squeezable(30, 20), squeezable(30, 20))
	host := NewBox(Row, 0, 0)
	host.Append(p, true)
	frame(t, host, 100, 40)

	if got := SizeWithin(p, Size{W: 100, H: 40}); got.W != 60 {
		t.Errorf("paned width %d, want the panes' 60 (the negative margin emptied the slot)", got.W)
	}
	sv := p.sep.style(&p.sep)
	if got := sv.Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("separator background %v, want the rule", got)
	}
	if r := p.sepRect(); r.X != 27 || r.W != 3 {
		t.Errorf("separator rect %+v, want 3 wide at x 27 (60/2 - 3)", r)
	}
}

// Unstyled, the divider is the 6px theme handle it always was.
func TestPanedSeparatorDefaults(t *testing.T) {
	p := NewPaned(Column, squeezable(30, 20), squeezable(30, 20))
	host := NewBox(Column, 0, 0)
	host.Append(p, true)
	frame(t, host, 100, 100)
	if got := p.sepSlot(); got != panedHandleW {
		t.Errorf("slot %d, want the %dpx handle", got, panedHandleW)
	}
	if r := p.sepRect(); r.H != panedHandleW {
		t.Errorf("separator rect %+v, want %d tall", r, panedHandleW)
	}
}
