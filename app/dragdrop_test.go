package app

import (
	"errors"
	"io"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"

	"github.com/stubbedev/lys-ui/internal/dragdrop"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// mimeLysUITile is the custom type the test drag content offers.
const mimeLysUITile = "application/x-lys-ui-tile"

// fakeDnD records what the surface input drives through the
// dragController seam.
type fakeDnD struct {
	starts       []dragdrop.StartConfig
	startErr     error
	dragging     bool
	payload      []byte
	payloadErr   error
	lastReadMime string
}

func (f *fakeDnD) StartDrag(cfg dragdrop.StartConfig) error {
	f.starts = append(f.starts, cfg)
	return f.startErr
}

func (f *fakeDnD) Dragging() bool { return f.dragging }

func (f *fakeDnD) ReadPayload(mime string) ([]byte, error) {
	f.lastReadMime = mime
	return f.payload, f.payloadErr
}

// dragStub is a clickable widget that declares draggable content.
type dragStub struct {
	bounds  render.Rect
	clicks  int
	refuse  bool
	pressed bool
}

func (s *dragStub) Measure(con widget.Constraints) widget.Size {
	return clampWidgetSize(widget.Size{W: 20, H: 20}, con)
}

func (s *dragStub) Arrange(r render.Rect) { s.bounds = r }

func (s *dragStub) Paint(*render.Canvas) {}

func (s *dragStub) HitTest(p widget.Point) widget.Widget {
	if s.bounds.Contains(p.X, p.Y) {
		return s
	}
	return nil
}

func (s *dragStub) ClickAt(widget.Point) { s.clicks++ }

func (s *dragStub) SetPressed(on bool) { s.pressed = on }

func (s *dragStub) DragContent() *widget.DragContent {
	if s.refuse {
		return nil
	}
	return &widget.DragContent{
		Mimes: []string{mimeLysUITile},
		Write: func(mime string, w io.Writer) error {
			_, err := io.WriteString(w, "stub")
			return err
		},
	}
}

// dropZone plays the drop target: it decides by mime, tracks the
// highlight, and records the drop.
type dropZone struct {
	bounds   render.Rect
	mime     string
	dragOver bool

	enters       int
	enteredMimes []string
	enterPoint   widget.Point
	drops        int
	dropMime     string
	dropData     []byte
	dropPoint    widget.Point
	lastHover    widget.Point
	hovers       int
}

func (z *dropZone) Measure(con widget.Constraints) widget.Size {
	return clampWidgetSize(widget.Size{W: 20, H: 20}, con)
}

func (z *dropZone) Arrange(r render.Rect) { z.bounds = r }

func (z *dropZone) Paint(*render.Canvas) {}

func (z *dropZone) HitTest(p widget.Point) widget.Widget {
	if z.bounds.Contains(p.X, p.Y) {
		return z
	}
	return nil
}

func (z *dropZone) DragEnter(mimes []string, p widget.Point) string {
	z.enters++
	z.enteredMimes = mimes
	z.enterPoint = p
	return z.mime
}

func (z *dropZone) DragHover(p widget.Point) {
	z.hovers++
	z.lastHover = p
}

func (z *dropZone) DragLeave() {}

func (z *dropZone) SetDragOver(on bool) { z.dragOver = on }

func (z *dropZone) Drop(mime string, data []byte, p widget.Point) {
	z.drops++
	z.dropMime = mime
	z.dropData = data
	z.dropPoint = p
}

// Compile-time shapes the drag path relies on.
var (
	_ widget.DragSource  = (*dragStub)(nil)
	_ widget.Clicker     = (*dragStub)(nil)
	_ widget.DragEnterer = (*dropZone)(nil)
	_ widget.Dropper     = (*dropZone)(nil)
)

// clampWidgetSize pins s to con for the stubs.
func clampWidgetSize(s widget.Size, con widget.Constraints) widget.Size {
	if s.W > con.Max.W {
		s.W = con.Max.W
	}
	if s.H > con.Max.H {
		s.H = con.Max.H
	}
	return s
}

// newDragTestInput builds a surface input around a stub tree.
func newDragTestInput(root widget.Widget, dnd dragController) *surfaceInput {
	return &surfaceInput{
		router:  &widget.Router{Root: root},
		surf:    &wl.Surface{},
		dnd:     dnd,
		request: func() {},
	}
}

// rowOf lays one widget alone on a row at the origin.
func rowOf(w widget.Widget) widget.Widget {
	root := widget.NewBox(widget.Row, 0, 0).Append(w, false)
	root.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 20})
	return root
}

func TestDragGestureStartsDataDeviceDrag(t *testing.T) {
	card := &dragStub{}
	root := rowOf(card)
	dnd := &fakeDnD{}
	in := newDragTestInput(root, dnd)

	// Press on the card, then move past the threshold.
	in.HandlePointerMotion(5, 5)
	in.HandlePointerButton(widget.BTNLeft, 1, 4242)
	in.HandlePointerMotion(20, 5)

	if len(dnd.starts) != 1 {
		t.Fatalf("start_drag sent %d times, want once", len(dnd.starts))
	}
	cfg := dnd.starts[0]
	if cfg.Origin != in.surf {
		t.Errorf("origin = %v, want the input surface", cfg.Origin)
	}
	// The gesture's press serial must reach start_drag: this is the
	// serial bookkeeping the compositor validates.
	if cfg.GrabSerial != 4242 {
		t.Errorf("grab serial = %d, want the press serial 4242", cfg.GrabSerial)
	}
	if len(cfg.Content.Mimes) != 1 || cfg.Content.Mimes[0] != mimeLysUITile {
		t.Errorf("offered mimes = %v, want the card's content", cfg.Content.Mimes)
	}
	if cfg.Content.Write == nil {
		t.Error("content arrived without a payload provider")
	}
	if !in.dragStarted {
		t.Error("gesture state did not latch")
	}
	if card.pressed {
		t.Error("the drag did not clear the pressed state on the card")
	}

	// The drag owns the gesture: the release must not click the card.
	in.HandlePointerButton(widget.BTNLeft, 0, 4243)
	if card.clicks != 0 {
		t.Errorf("card clicked %d times around a drag gesture", card.clicks)
	}

	// A later motion after the gesture latched must not re-start it.
	in.HandlePointerMotion(40, 5)
	if len(dnd.starts) != 1 {
		t.Errorf("start_drag sent %d times, want one per gesture", len(dnd.starts))
	}
}

func TestDragGestureGuards(t *testing.T) {
	t.Run("below the threshold the click survives", func(t *testing.T) {
		card := &dragStub{}
		dnd := &fakeDnD{}
		in := newDragTestInput(rowOf(card), dnd)

		in.HandlePointerMotion(5, 5)
		in.HandlePointerButton(widget.BTNLeft, 1, 1)
		in.HandlePointerMotion(10, 5) // 5px < the 8px threshold

		if len(dnd.starts) != 0 {
			t.Errorf("start_drag sent %d times below the threshold", len(dnd.starts))
		}
		in.HandlePointerButton(widget.BTNLeft, 0, 2)
		if card.clicks != 1 {
			t.Errorf("card clicks = %d, want the click to survive", card.clicks)
		}
	})

	t.Run("a widget without drag content never starts a drag", func(t *testing.T) {
		card := &dragStub{refuse: true}
		dnd := &fakeDnD{}
		in := newDragTestInput(rowOf(card), dnd)

		in.HandlePointerMotion(5, 5)
		in.HandlePointerButton(widget.BTNLeft, 1, 1)
		in.HandlePointerMotion(30, 5)

		if len(dnd.starts) != 0 {
			t.Errorf("a refusing source started %d drags", len(dnd.starts))
		}
	})

	t.Run("a controller failure degrades to a click", func(t *testing.T) {
		card := &dragStub{}
		dnd := &fakeDnD{startErr: errors.New("no data device")}
		in := newDragTestInput(rowOf(card), dnd)

		in.HandlePointerMotion(5, 5)
		in.HandlePointerButton(widget.BTNLeft, 1, 1)
		in.HandlePointerMotion(15, 5) // past the threshold, still on the card
		if len(dnd.starts) != 1 {
			t.Fatalf("start_drag sent %d times", len(dnd.starts))
		}
		in.HandlePointerButton(widget.BTNLeft, 0, 2)
		if card.clicks != 1 {
			t.Errorf("card clicks = %d, want the failed drag to degrade to a click", card.clicks)
		}
	})
}

func TestSurfaceInputRoutesDrops(t *testing.T) {
	zone := &dropZone{mime: mimeLysUITile}
	root := rowOf(zone)
	dnd := &fakeDnD{payload: []byte("tile-7")}
	in := newDragTestInput(root, dnd)

	mime := in.DragEnter([]string{mimeLysUITile, "text/plain"}, 5, 6)
	if mime != mimeLysUITile {
		t.Fatalf("accepted = %q, want the tile mime", mime)
	}
	if zone.enters != 1 || len(zone.enteredMimes) != 2 {
		t.Errorf("zone got enters=%d mimes=%v", zone.enters, zone.enteredMimes)
	}
	if !zone.dragOver {
		t.Error("accepted drag did not highlight the zone")
	}

	// Hover within the zone reaches DragHover with router coordinates.
	in.DragMotion(6, 7)
	if zone.hovers != 1 || zone.lastHover != (widget.Point{X: 6, Y: 7}) {
		t.Errorf("zone hovers=%d last=%v", zone.hovers, zone.lastHover)
	}

	in.Drop(7, 8)

	if zone.drops != 1 {
		t.Fatalf("zone got %d drops", zone.drops)
	}
	if dnd.lastReadMime != mimeLysUITile {
		t.Errorf("payload read for %q, want the accepted mime", dnd.lastReadMime)
	}
	if string(zone.dropData) != "tile-7" || zone.dropMime != mimeLysUITile {
		t.Errorf("drop = (%q, %q)", zone.dropMime, zone.dropData)
	}
	if zone.dropPoint != (widget.Point{X: 7, Y: 8}) {
		t.Errorf("drop point %v, want (7,8)", zone.dropPoint)
	}
	if zone.dragOver {
		t.Error("drop left the zone highlighted")
	}
}

func TestSurfaceInputDropCoordinatesAreLogical(t *testing.T) {
	zone := &dropZone{mime: mimeLysUITile}
	root := rowOf(zone)
	dnd := &fakeDnD{payload: []byte("x")}
	in := newDragTestInput(root, dnd)
	in.frac = func() uint32 { return 300 } // 2.5x device scale

	// Drop points are logical surface pixels at any device scale: the
	// widget tree is laid out in the same space the events arrive in.
	if m := in.DragEnter([]string{mimeLysUITile}, 5, 3); m == "" {
		t.Fatal("drag was rejected")
	}
	if zone.enterPoint != (widget.Point{X: 5, Y: 3}) {
		t.Errorf("enter point %v, want the logical (5,3)", zone.enterPoint)
	}
	in.Drop(3, 4)
	if zone.dropPoint != (widget.Point{X: 3, Y: 4}) {
		t.Errorf("drop point %v, want the logical (3,4)", zone.dropPoint)
	}
}

func TestSurfaceInputRejectsAndLeaves(t *testing.T) {
	rejecter := &dropZone{mime: ""}
	root := rowOf(rejecter)
	in := newDragTestInput(root, &fakeDnD{})

	if m := in.DragEnter([]string{mimeLysUITile}, 5, 5); m != "" {
		t.Fatalf("rejecting zone accepted %q", m)
	}
	if rejecter.dragOver {
		t.Error("rejected drag highlighted the zone")
	}

	// An accepted zone that is left again accepts nothing on drop.
	zone := &dropZone{mime: mimeLysUITile}
	in2 := newDragTestInput(rowOf(zone), &fakeDnD{})
	in2.DragEnter([]string{mimeLysUITile}, 5, 5)
	if !zone.dragOver {
		t.Fatal("enter did not highlight the zone")
	}
	in2.DragLeave()
	if zone.dragOver {
		t.Error("leave did not clear the highlight")
	}
	in2.Drop(5, 5)
	if zone.drops != 0 {
		t.Errorf("zone got %d drops after leave", zone.drops)
	}
}

func TestSurfaceInputRejectsDropsWhenBlocked(t *testing.T) {
	zone := &dropZone{mime: mimeLysUITile}
	in := newDragTestInput(rowOf(zone), &fakeDnD{})
	in.blocked = func() bool { return true } // a modal dialog owns the pointer

	if m := in.DragEnter([]string{mimeLysUITile}, 5, 5); m != "" {
		t.Errorf("blocked window accepted %q", m)
	}
	in.Drop(5, 5)
	if zone.drops != 0 {
		t.Errorf("blocked window took %d drops", zone.drops)
	}
}

// TestRenderDragIconToleratesMissingPieces pins the guard path: no
// session, no compositor, or no widget yields no icon instead of a
// crash, because start_drag accepts a nil icon.
func TestRenderDragIconToleratesMissingPieces(t *testing.T) {
	if got := renderDragIcon(nil, &dragStub{}, 1); got != nil {
		t.Error("icon rendered without a session")
	}
	if got := renderDragIcon(&wlsession.Session{}, &dragStub{}, 1); got != nil {
		t.Error("icon rendered without a compositor")
	}
	if got := renderDragIcon(&wlsession.Session{}, nil, 1); got != nil {
		t.Error("icon rendered for a nil widget")
	}
}
