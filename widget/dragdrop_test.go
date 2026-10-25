package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/transfer"
)

// dropPanel plays a drop target: it accepts a drag by mime, tracks the
// hover highlight, and records everything the router delivered.
type dropPanel struct {
	node
	nat  Size
	mime string // accepted mime; "" rejects

	dragOver     bool
	enters       int
	enteredMimes []string
	enterPoint   Point
	hovers       []Point
	leaves       int
	drops        int
	dropMime     string
	dropData     []byte
	dropPoint    Point
}

func (p *dropPanel) Measure(con Constraints) Size { return clampSize(p.nat, con) }
func (p *dropPanel) Paint(*render.Canvas)         {}
func (p *dropPanel) HitTest(point Point) Widget   { return p.HitLeaf(p, point) }

func (p *dropPanel) DragEnter(mimes []string, point Point) string {
	p.enters++
	p.enteredMimes = mimes
	p.enterPoint = point
	return p.mime
}

func (p *dropPanel) DragHover(point Point) { p.hovers = append(p.hovers, point) }
func (p *dropPanel) DragLeave()            { p.leaves++ }
func (p *dropPanel) SetDragOver(on bool)   { p.dragOver = on }

func (p *dropPanel) Drop(mime string, data []byte, point Point) {
	p.drops++
	p.dropMime = mime
	p.dropData = data
	p.dropPoint = point
}

// arrangeDropPanels lays two side-by-side drop panels on a row.
func arrangeDropPanels() (root Widget, a, b *dropPanel) {
	a = &dropPanel{nat: Size{W: 20, H: 20}, mime: "application/x-lys-ui-tile"}
	b = &dropPanel{nat: Size{W: 20, H: 20}, mime: ""}
	root = NewBox(Row, 0, 0).Append(a, false).Append(b, false)
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})
	return root, a, b
}

func TestRouterDragEnterByMime(t *testing.T) {
	root, a, b := arrangeDropPanels()
	r := &Router{Root: root}

	t.Run("the panel decides by mime", func(t *testing.T) {
		mime := r.DragEnter([]string{"application/x-lys-ui-tile", "text/plain"}, Point{X: 5, Y: 5})
		if mime != "application/x-lys-ui-tile" {
			t.Fatalf("accepted = %q, want the tile mime", mime)
		}
		if a.enters != 1 || len(a.enteredMimes) != 2 || a.enteredMimes[0] != "application/x-lys-ui-tile" {
			t.Errorf("panel saw enters=%d mimes=%v", a.enters, a.enteredMimes)
		}
		if a.enterPoint != (Point{X: 5, Y: 5}) {
			t.Errorf("enter point %v, want (5,5)", a.enterPoint)
		}
		if !a.dragOver {
			t.Error("accepted drag did not highlight the panel")
		}
	})

	t.Run("an empty mime rejects and skips the highlight", func(t *testing.T) {
		mime := r.DragEnter([]string{"text/plain"}, Point{X: 25, Y: 5})
		if mime != "" {
			t.Fatalf("rejected mime = %q, want empty", mime)
		}
		if b.dragOver {
			t.Error("rejected drag highlighted the panel")
		}
	})
}

// dropRow is a drop target that arranges one leaf child; drop hits are
// resolved on the parent chain when the leaf fills the area.
type dropRow struct {
	dropPanel
	child Widget
}

func (p *dropRow) Arrange(r render.Rect) {
	p.node.Arrange(r)
	p.child.Arrange(r)
	setParents(p, p.child)
}

func (p *dropRow) HitTest(point Point) Widget {
	if hit := p.child.HitTest(point); hit != nil {
		return hit
	}
	return p.HitLeaf(p, point)
}

// plainLeaf is a widget with no drag behavior at all.
type plainLeaf struct {
	node
}

func (l *plainLeaf) Measure(con Constraints) Size { return clampSize(Size{W: 20, H: 20}, con) }
func (l *plainLeaf) Paint(*render.Canvas)         {}
func (l *plainLeaf) HitTest(point Point) Widget   { return l.HitLeaf(l, point) }

// newBubbleRow builds a drop row containing a plain leaf, so hits
// inside the row land on the leaf and must bubble to the panel.
func newBubbleRow() *dropRow {
	p := &dropRow{}
	p.nat = Size{W: 20, H: 20}
	p.mime = "application/x-lys-ui-row"
	p.child = &plainLeaf{}
	return p
}

func TestRouterDragBubblesToAncestor(t *testing.T) {
	panel := newBubbleRow()
	root := NewBox(Column, 0, 0).Append(panel, false)
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 20})

	r := &Router{Root: root}
	// The hit lands on the plain leaf; the drop decision belongs to
	// the panel above it.
	mime := r.DragEnter([]string{"application/x-lys-ui-row"}, Point{X: 10, Y: 10})
	if mime != "application/x-lys-ui-row" {
		t.Fatalf("mime = %q, want the row mime via the ancestor", mime)
	}
	if panel.enters != 1 || !panel.dragOver {
		t.Errorf("panel enters=%d dragOver=%v; the ancestor was not found", panel.enters, panel.dragOver)
	}
}

func TestRouterDragHoverRetargets(t *testing.T) {
	root, a, b := arrangeDropPanels()
	b.mime = "application/x-lys-ui-tile"
	r := &Router{Root: root}

	r.DragEnter([]string{"application/x-lys-ui-tile"}, Point{X: 5, Y: 5})
	// Crossing into the second panel inside one surface: no new enter
	// arrives from the compositor, the router retargets on hover.
	mime := r.DragHover(Point{X: 25, Y: 5})

	if mime != "application/x-lys-ui-tile" {
		t.Fatalf("mime after retarget = %q, want the second panel's mime", mime)
	}
	if b.enters != 1 {
		t.Errorf("second panel got %d enters via hover, want 1", b.enters)
	}
	if a.dragOver || a.leaves != 1 {
		t.Errorf("first panel dragOver=%v leaves=%d, want cleared", a.dragOver, a.leaves)
	}
	if !b.dragOver {
		t.Error("second panel was not highlighted after retarget")
	}
	if len(b.hovers) != 0 {
		t.Errorf("entering panel got %d hovers, want none (hover follows enter)", len(b.hovers))
	}

	// Staying put keeps feeding hover positions to the current target.
	r.DragHover(Point{X: 26, Y: 6})
	if len(b.hovers) != 1 || b.hovers[0] != (Point{X: 26, Y: 6}) {
		t.Errorf("panel hovers = %v, want the motion point", b.hovers)
	}
}

func TestRouterDragLeaveClears(t *testing.T) {
	root, a, _ := arrangeDropPanels()
	r := &Router{Root: root}
	r.DragEnter([]string{"application/x-lys-ui-tile"}, Point{X: 5, Y: 5})

	r.DragLeave()

	if a.dragOver || a.leaves != 1 {
		t.Errorf("panel dragOver=%v leaves=%d after leave, want false/1", a.dragOver, a.leaves)
	}
	if r.DragMime() != "" {
		t.Errorf("DragMime = %q after leave, want empty", r.DragMime())
	}
}

func TestRouterDropDeliversPayload(t *testing.T) {
	root, a, _ := arrangeDropPanels()
	r := &Router{Root: root}
	r.DragEnter([]string{"application/x-lys-ui-tile"}, Point{X: 5, Y: 5})

	r.Drop("application/x-lys-ui-tile", []byte("tile-7"), Point{X: 7, Y: 9})

	if a.drops != 1 {
		t.Fatalf("panel got %d drops, want 1", a.drops)
	}
	if a.dropMime != "application/x-lys-ui-tile" || string(a.dropData) != "tile-7" || a.dropPoint != (Point{X: 7, Y: 9}) {
		t.Errorf("drop = (%q, %q, %v)", a.dropMime, a.dropData, a.dropPoint)
	}
	// The drop ends the hover: no dangling highlight, no stale mime.
	if a.dragOver || r.DragMime() != "" || r.dragTarget != nil {
		t.Error("drop left drag state behind")
	}
}

func TestRouterCancelPressSuppressesClick(t *testing.T) {
	src := &inputStub{nat: Size{W: 20, H: 20}}
	root := NewBox(Row, 0, 0).Append(src, false)
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 20})
	r := &Router{Root: root}
	p := Point{X: 5, Y: 5}
	r.Move(p)
	r.Press(BTNLeft, p, 0)

	if r.Pressed() != src {
		t.Fatalf("Pressed = %v, want the pressed stub", r.Pressed())
	}

	// A drag that started from this press must not click on release.
	r.CancelPress()
	r.Release(BTNLeft, p)

	if src.clicks != 0 {
		t.Errorf("stub clicked %d times after a cancelled press", src.clicks)
	}
	if r.Pressed() != nil {
		t.Error("press survived CancelPress")
	}
	if src.pressed {
		t.Error("CancelPress did not clear the pressed state on the widget")
	}
}

// movePanel is a drop panel that moves when the source lets it.
type movePanel struct{ dropPanel }

func (p *movePanel) HitTest(point Point) Widget { return p.HitLeaf(p, point) }

func (p *movePanel) DropAction(offered transfer.Action) transfer.Action {
	return transfer.Prefer(offered, transfer.ActionMove)
}

// The accepting target's DropActionChooser picks the action; a plain
// target or a rejected drag leaves the default.
func TestRouterDragAction(t *testing.T) {
	m := &movePanel{dropPanel{nat: Size{W: 20, H: 20}, mime: "a/b"}}
	plain := &dropPanel{nat: Size{W: 20, H: 20}, mime: "a/b"}
	root := NewBox(Row, 0, 0).Append(m, false).Append(plain, false)
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	root.Arrange(render.Rect{W: 40, H: 20})
	r := &Router{Root: root}
	both := transfer.ActionCopy | transfer.ActionMove
	r.DragEnter([]string{"a/b"}, Point{X: 5, Y: 5})
	if got := r.DragAction(both); got != transfer.ActionMove {
		t.Errorf("chooser picked %d, want move", got)
	}
	r.DragHover(Point{X: 25, Y: 5})
	if got := r.DragAction(both); got != transfer.ActionNone {
		t.Errorf("plain target picked %d, want the default", got)
	}
	m.mime = ""
	r.DragHover(Point{X: 5, Y: 5})
	if got := r.DragAction(both); got != transfer.ActionNone {
		t.Errorf("rejecting chooser picked %d", got)
	}
}
