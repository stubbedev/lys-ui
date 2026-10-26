package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// newScrollFixture builds a 60x60 scroll wrapping a 60x200 child: a
// vertical range of 140px with a gutter-reserved vertical bar.
func newScrollFixture(t *testing.T) *Scroll {
	t.Helper()
	s := NewScroll(newStub(60, 200))
	s.ShowBars = true
	s.Measure(Constraints{Max: Size{W: 60, H: 60}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
	return s
}

// A scroll's bars keep 1px off the viewport's outer edge and both ends
// of their axis (#318).
func TestScrollBarsInsetFromTheEdges(t *testing.T) {
	s := NewScroll(newStub(200, 200))
	s.ShowBars = true
	s.Measure(Constraints{Max: Size{W: 60, H: 60}})
	s.Arrange(render.Rect{X: 10, Y: 20, W: 60, H: 60})
	v, _ := s.vBarGeometry()
	if v.X+v.W != 10+60-1 || v.Y != 20+1 || v.Y+v.H != 20+s.viewH-1 {
		t.Errorf("vertical track %v, want 1px off the right, top and bottom of the viewport", v)
	}
	h, _ := s.hBarGeometry()
	if h.Y+h.H != 20+60-1 || h.X != 10+1 || h.X+h.W != 10+s.viewW-1 {
		t.Errorf("horizontal track %v, want 1px off the bottom, left and right of the viewport", h)
	}
}

// TestScrollHandleDrag pins the 1:1 handle mapping: grabbing the
// handle and moving the pointer down scales pointer motion by the
// viewport-to-track ratio into content offset.
func TestScrollHandleDrag(t *testing.T) {
	s := newScrollFixture(t)
	track, handle := s.vBarGeometry()
	if track.W == 0 {
		t.Fatal("no vertical bar geometry for overflowing content")
	}

	s.SetPressed(true)
	// First drag event grabs: press inside the handle.
	s.DragMove(Point{X: track.X + 2, Y: handle.Y + 2})
	startOffY := s.offY

	s.DragMove(Point{X: track.X + 2, Y: handle.Y + 2 + 18})
	_, maxY := s.scrollMax()
	denom := track.H - handle.H
	want := min(max(0, startOffY+(18*maxY+denom/2)/denom), maxY)
	if s.offY != want {
		t.Errorf("offset after 18px drag = %d, want %d (1:1 mapping)", s.offY, want)
	}

	s.SetPressed(false)
	if _, maxY := s.scrollMax(); s.offY > maxY {
		t.Errorf("offset %d exceeds range %d (overshoot)", s.offY, maxY)
	}
}

// TestScrollPageClick pins gutter paging: clicking the track below the
// handle pages down by one viewport; clicking above pages up.
func TestScrollPageClick(t *testing.T) {
	s := newScrollFixture(t)
	track, _ := s.vBarGeometry()

	s.ClickAt(Point{X: track.X + 2, Y: track.Y + track.H - 2})
	if _, offY := s.Offset(); offY != 60 {
		t.Errorf("page down offset = %d, want 60 (one viewport)", offY)
	}
	// After the page down the handle sits lower; clicking the track
	// above its new position pages up, clamped back to the top.
	s.ClickAt(Point{X: track.X + 2, Y: track.Y + 2})
	if _, offY := s.Offset(); offY != 0 {
		t.Errorf("page up offset = %d, want clamped 0", offY)
	}
}

// TestHorizontalWheelScrolls pins horizontal axis handling on a child
// wider than the viewport.
func TestHorizontalWheelScrolls(t *testing.T) {
	s := NewScroll(newStub(200, 30))
	s.ShowBars = true
	s.Measure(Constraints{Max: Size{W: 60, H: 60}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
	s.ScrollBy(2, 0)
	if offX, _ := s.Offset(); offX != 2*wheelStep(s.viewW) {
		t.Errorf("horizontal offset = %d, want two steps of %d", offX, wheelStep(s.viewW))
	}
	if _, handle := s.hBarGeometry(); handle.H == 0 {
		t.Error("no horizontal bar for horizontally overflowing content")
	}
}

// TestScrollMaxContentHeight pins the cap: content shorter than it
// measures natural, taller content measures the cap plus the gutter
// it scrolls behind, and no cap is the uncapped natural height.
func TestScrollMaxContentHeight(t *testing.T) {
	unbounded := Constraints{Max: Size{W: 500, H: 500}}
	s := propagating(NewScroll(newStub(60, 200)))
	s.ShowBars = true
	if got := s.Measure(unbounded); got != (Size{W: 60, H: 200}) {
		t.Errorf("uncapped = %v, want the natural 60x200", got)
	}
	s.SetMaxContentHeight(80)
	if got := s.Measure(unbounded); got != (Size{W: 60 + gutter, H: 80}) {
		t.Errorf("capped = %v, want 80 high with the gutter on the width", got)
	}
	s.Arrange(render.Rect{W: 60 + gutter, H: 80})
	if s.viewW != 60 {
		t.Errorf("viewport width = %d, want the content's 60 beside the gutter", s.viewW)
	}
	short := NewScroll(newStub(60, 40))
	short.SetMaxContentHeight(80)
	if got := short.Measure(unbounded); got != (Size{W: 60, H: 40}) {
		t.Errorf("short content = %v, want its natural 60x40", got)
	}
	s.SetMaxContentHeight(-5)
	if s.MaxContentHeight() != 0 || s.Measure(unbounded).H != 200 {
		t.Error("a negative cap is no cap")
	}
}

// TestScrollVerticalOnly pins GTK's hscrollbar-policy never: a
// wrapping label inside wraps at the viewport's width (the bar's
// gutter reserved beside it) and nothing scrolls sideways, where a
// two-way scroll lets it run one line wide.
func TestScrollVerticalOnly(t *testing.T) {
	face := entryFace(t)
	text := "a long sentence that cannot fit on one line of a narrow popover at all"
	label := NewLabel(face, 12, text, render.RGB(255, 255, 255))
	label.SetWrap(true)
	s := NewScroll(label)
	s.VerticalOnly, s.ShowBars = true, true
	if got := s.Measure(Constraints{Max: Size{W: 120, H: 50}}); got.W > 120 {
		t.Errorf("measured %v, wider than the 120 it was given", got)
	}
	s.Arrange(render.Rect{W: 120, H: 50})
	wrapped := func(w int) int { return label.Measure(Constraints{Max: Size{W: w, H: 1 << 20}}).H }
	if lb := label.Bounds(); s.viewW != 120-gutter || lb.W != s.viewW || lb.H != wrapped(s.viewW) || lb.H <= 50 {
		t.Errorf("label %v in a %dpx viewport: want it the viewport's width less the gutter, wrapped there", lb, s.viewW)
	}
	if mx, my := s.scrollMax(); mx != 0 || my == 0 {
		t.Errorf("scroll range %d,%d: want vertical only", mx, my)
	}
	// Arranged narrower than it was measured: it wraps at the width it
	// got, not the one it was offered.
	s.Measure(Constraints{Max: Size{W: 400, H: 50}})
	s.Arrange(render.Rect{W: 90, H: 50})
	if lb := label.Bounds(); lb.W != 90-gutter || lb.H != wrapped(90-gutter) {
		t.Errorf("re-arranged label %v, want %dx%d", lb, 90-gutter, wrapped(90-gutter))
	}

	two := NewScroll(NewLabel(face, 12, text, render.RGB(255, 255, 255)))
	two.Measure(Constraints{Max: Size{W: 120, H: 30}})
	two.Arrange(render.Rect{W: 120, H: 30})
	if mx, _ := two.scrollMax(); mx == 0 {
		t.Error("a two-way scroll stopped scrolling sideways")
	}
}

// Pixel scrolling moves a Scroll and a List by exactly the pixels,
// fractions carried; a step-only scroller gets a step per 40 pixels.
func TestAxisPixels(t *testing.T) {
	tall := NewBox(Column, 0, 0)
	for range 10 {
		tall.Append(newStub(50, 20), false)
	}
	s := NewScroll(tall)
	s.Measure(Constraints{Max: Size{W: 60, H: 60}})
	s.Arrange(render.Rect{W: 60, H: 60})
	r := &Router{Root: s}
	r.Move(Point{X: 10, Y: 10})
	r.AxisPixels(0, 2.6, 0)
	r.AxisPixels(0, 2.6, 0)
	if _, y := s.Offset(); y != 5 {
		t.Errorf("scroll offset %d after 5.2px, want 5", y)
	}
	var rows staticRows
	for range 20 {
		rows = append(rows, newStub(20, 10))
	}
	l := NewList[Widget](rows, 10)
	l.Measure(Constraints{Max: Size{W: 100, H: 50}})
	l.Arrange(render.Rect{W: 100, H: 50})
	l.ScrollPixels(0, 7.5)
	l.ScrollPixels(0, 0.5)
	if l.offY != 8 {
		t.Errorf("list offset %d, want 8", l.offY)
	}
	stub := &stepScroller{}
	r2 := &Router{Root: stub}
	r2.hover = stub
	for range 5 {
		r2.AxisPixels(0, 10, 0)
	}
	if stub.steps != 1 {
		t.Errorf("a step-only scroller got %d steps for 50px, want 1", stub.steps)
	}
}

// stepScroller scrolls in whole steps only.
type stepScroller struct {
	stub
	steps int
}

func (s *stepScroller) ScrollBy(_, dy int) { s.steps += dy }

// One wheel step is GTK's scroll amount: the visible extent raised to
// 2/3, taller pages scrolling further per notch, at least a pixel.
func TestWheelStepIsPageTwoThirds(t *testing.T) {
	tall := NewBox(Column, 0, 0)
	for range 20 {
		tall.Append(newStub(50, 30), false)
	}
	s := NewScroll(tall)
	s.Measure(Constraints{Max: Size{W: 60, H: 100}})
	s.Arrange(render.Rect{W: 60, H: 100})
	s.ScrollBy(0, 1)
	if _, y := s.Offset(); y != wheelStep(s.viewH) {
		t.Errorf("one step scrolled %d, want the page's 2/3 power %d", y, wheelStep(s.viewH))
	}
	short := NewScroll(NewBox(Column, 0, 0))
	short.Measure(Constraints{Max: Size{W: 30, H: 30}})
	short.Arrange(render.Rect{W: 30, H: 30})
	if got := wheelStep(short.viewH); got < 1 {
		t.Errorf("wheel step %d, want at least a pixel", got)
	}
}
