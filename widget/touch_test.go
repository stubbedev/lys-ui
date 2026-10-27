package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// gestureProbe is a leaf that records and claims gestures of one kind.
type gestureProbe struct {
	node
	kind GestureKind
	got  []Gesture
}

func (g *gestureProbe) Measure(axis Axis, forSize int) Sizes {
	return FixedSizes(Size{W: 100, H: 100}.Along(axis))
}
func (g *gestureProbe) HitTest(p Point) Widget { return g.HitLeaf(g, p) }
func (g *gestureProbe) Paint(*render.Canvas)   {}
func (g *gestureProbe) Gesture(e Gesture) bool {
	if e.Kind != g.kind {
		return false
	}
	g.got = append(g.got, e)
	return true
}

// touchFixture lays root out at 200x200 behind a tracker.
func touchFixture(root Widget) *TouchTracker {
	SizeWithin(root, Size{W: 200, H: 200})
	root.Arrange(render.Rect{W: 200, H: 200})
	return &TouchTracker{Router: &Router{Root: root}}
}

// A tap clicks, a drag past the slop moves without long-pressing, and
// a cancelled contact never clicks.
func TestTouchTapAndCancel(t *testing.T) {
	face := goldenFace(t)
	b := NewButton(NewLabel(face, 14, "Tap", DarkTheme().Text), 8, 4)
	clicks := 0
	b.OnClick = func() { clicks++ }
	tr := touchFixture(b)
	tr.Down(1, Point{X: 10, Y: 10})
	tr.Up(1)
	if clicks != 1 {
		t.Fatalf("a tap clicked %d times", clicks)
	}
	tr.Down(2, Point{X: 10, Y: 10})
	tr.Cancel()
	tr.Up(2)
	if clicks != 1 {
		t.Error("a cancelled contact clicked")
	}
}

// Holding still past the delay offers a long press; a claimer cancels
// the click, a moved contact never long-presses.
func TestTouchLongPress(t *testing.T) {
	c := pinAnimClock(t)
	probe := &gestureProbe{kind: GestureLongPress}
	tr := touchFixture(probe)
	tr.Down(1, Point{X: 50, Y: 50})
	c.set(c.now().Add(longPressDelay + time.Millisecond))
	c.drive()
	tr.Up(1)
	if len(probe.got) != 1 || probe.got[0].At != (Point{X: 50, Y: 50}) {
		t.Fatalf("long presses = %+v", probe.got)
	}
	tr.Down(2, Point{X: 50, Y: 50})
	tr.Motion(2, Point{X: 80, Y: 50})
	c.drive()
	tr.Up(2)
	if len(probe.got) != 1 {
		t.Error("a moving contact long-pressed")
	}
}

// Two contacts pinch for a claimer: scale and rotation relative to the
// start, the gesture ending with the first lift.
func TestTouchPinch(t *testing.T) {
	probe := &gestureProbe{kind: GesturePinch}
	tr := touchFixture(probe)
	tr.Down(1, Point{X: 40, Y: 50})
	tr.Down(2, Point{X: 60, Y: 50})
	tr.Motion(2, Point{X: 80, Y: 50})
	tr.Up(1)
	tr.Up(2)
	g := probe.got
	if len(g) != 3 || g[0].Phase != GestureBegin || g[2].Phase != GestureEnd {
		t.Fatalf("pinch events: %+v", g)
	}
	if g[1].Scale != 2 || g[1].Rotation != 0 || g[1].At != (Point{X: 60, Y: 50}) {
		t.Errorf("pinch update: %+v", g[1])
	}
}

// Without a pinch claimer two fingers scroll: content follows the
// fingers, and the lift hands the gesture to the kinetic glide.
func TestTouchTwoFingerScroll(t *testing.T) {
	pinAnimClock(t)
	s := NewScroll(NewSpacer(200, 2000))
	tr := touchFixture(s)
	s.SetOffset(0, 500)
	tr.Down(1, Point{X: 80, Y: 100})
	tr.Down(2, Point{X: 120, Y: 100})
	tr.Motion(1, Point{X: 80, Y: 140})
	tr.Motion(2, Point{X: 120, Y: 140})
	if _, y := s.Offset(); y >= 500 {
		t.Errorf("dragging down scrolled to %d, want toward the top", y)
	}
	tr.Up(1)
	if tr.Router.pixelTarget != nil {
		t.Error("the scroll gesture did not end on the lift")
	}
}

// A touchpad swipe pans a carousel and snaps.
func TestCarouselSwipe(t *testing.T) {
	pinAnimClock(t)
	face := goldenFace(t)
	car := NewCarousel(face, 14)
	for range 3 {
		car.Append(NewSpacer(200, 100))
	}
	r := &Router{Root: car}
	SizeWithin(car, Size{W: 200, H: 100})
	car.Arrange(render.Rect{W: 200, H: 100})
	at := Point{X: 100, Y: 50}
	r.Gesture(Gesture{Kind: GestureSwipe, Phase: GestureBegin, At: at, Fingers: 3})
	r.Gesture(Gesture{Kind: GestureSwipe, Phase: GestureUpdate, At: at, DX: -150})
	r.Gesture(Gesture{Kind: GestureSwipe, Phase: GestureEnd, At: at})
	if car.Page() != 1 {
		t.Errorf("swiped to page %d, want 1", car.Page())
	}
}
