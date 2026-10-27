package widget

import (
	"math"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
)

// Swipe thresholds: the travel, in logical pixels, a drag must make
// mostly sideways before it is a swipe; the share of the width past
// which a release dismisses; and the speed, in logical pixels a
// second, a release at which flings the card out wherever it is.
const (
	swipeSlop       = 8
	swipeDismissAt  = 0.4
	swipeFlingSpeed = 800
)

// SwipeDismiss lets its child be swiped away sideways, as a
// notification is: a press that travels sideways past a small slop
// takes the gesture from whatever it landed on (a button under it does
// not click), and the child follows the pointer, fading as it goes. A
// release past 40% of the width, or a fling, sends it out on a spring
// and fires OnDismiss once; any shorter swipe springs it back. A
// release inside the slop is an ordinary click for the child. A
// sideways two-finger touchpad scroll drags it the same way, its end
// settling it.
type SwipeDismiss struct {
	node
	child Widget
	// OnDismiss fires once, as the child leaves: dir is -1 out to the
	// left, 1 to the right.
	OnDismiss func(dir int)

	// dx is the child's offset; grab is where the pointer holds it, so
	// dx follows the pointer 1:1.
	dx, grab float64
	// vel is the swipe's speed and lastX, lastT its last sample.
	vel   float64
	lastX float64
	lastT time.Time
	// swiping marks a swipe in progress; out is the direction the child
	// flies out in, 0 while it may come back; dismissed marks OnDismiss
	// fired.
	swiping   bool
	out       int
	dismissed bool
	spring    *anim.Spring
}

// NewSwipeDismiss wraps child.
func NewSwipeDismiss(child Widget) *SwipeDismiss {
	s := &SwipeDismiss{child: child}
	s.SetElement("swipedismiss")
	s.spring = anim.NewSpring(anim.SpringSnappy, []float64{0}, func(pos []float64) { s.settled(pos[0]) })
	return s
}

// Child is the wrapped widget.
func (s *SwipeDismiss) Child() Widget { return s.child }

// Offset is how far, in logical pixels, the child sits off its place.
func (s *SwipeDismiss) Offset() float64 { return s.dx }

// Children is the child.
func (s *SwipeDismiss) Children() []Widget { return []Widget{s.child} }

// Measure is the child's size.
func (s *SwipeDismiss) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := s.measureHit(axis, forSize); ok {
		return sz
	}
	return s.measureStore(axis, forSize, measureChild(s, s.child, axis, forSize))
}

// Arrange places the child at r, shifted by the swipe.
func (s *SwipeDismiss) Arrange(r render.Rect) {
	s.node.Arrange(r)
	s.place()
}

// place arranges the child at the bounds shifted by the offset.
func (s *SwipeDismiss) place() {
	setParents(s, s.child)
	r := s.bounds
	r.X += int(math.Round(s.dx))
	arrangeChild(s.child, r)
	s.Invalidate()
}

// Paint draws the child, fading with its offset.
func (s *SwipeDismiss) Paint(cv *render.Canvas) {
	if s.dx == 0 {
		PaintChild(cv, s.child)
		return
	}
	a := 1 - min(1, math.Abs(s.dx)/float64(max(1, s.bounds.W)))
	if a <= 0 {
		return
	}
	prev := cv.PushAlpha(a)
	PaintChild(cv, s.child)
	cv.PopAlpha(prev)
}

// HitTest is the child, else the wrapper within its bounds.
func (s *SwipeDismiss) HitTest(p Point) Widget {
	if hit := HitChild(s.child, p); hit != nil {
		return hit
	}
	return s.HitLeaf(s, p)
}

// InterceptDrag implements DragInterceptor: a press that travelled
// past the slop, more sideways than up or down, becomes a swipe.
func (s *SwipeDismiss) InterceptDrag(from, at Point) bool {
	ddx, ddy := math.Abs(float64(at.X-from.X)), math.Abs(float64(at.Y-from.Y))
	if s.dismissed || ddx <= swipeSlop || ddx <= ddy {
		return false
	}
	s.begin(float64(from.X))
	return true
}

// DragMove implements DragMover: the child follows the pointer.
func (s *SwipeDismiss) DragMove(p Point) {
	if s.swiping {
		s.drag(float64(p.X) - s.grab)
	}
}

// PressEnd implements PressEnder: the release settles the swipe.
func (s *SwipeDismiss) PressEnd() {
	if s.swiping {
		s.release()
	}
}

// ClaimPixels implements PixelClaimer: a sideways finger scroll drags
// the child as the pointer does.
func (s *SwipeDismiss) ClaimPixels(dx, dy float64) bool {
	if s.dismissed {
		return false
	}
	if !s.swiping {
		if math.Abs(dx) <= math.Abs(dy) {
			return false
		}
		s.begin(0)
	}
	s.drag(s.dx - dx)
	return true
}

// ScrollEnd implements ScrollEnder: lifting the fingers settles it.
func (s *SwipeDismiss) ScrollEnd() {
	if s.swiping {
		s.release()
	}
}

// begin takes a swipe whose pointer is at x, holding the child where
// it shows.
func (s *SwipeDismiss) begin(x float64) {
	s.spring.Stop()
	s.swiping, s.out = true, 0
	s.grab = x - s.dx
	s.vel, s.lastX, s.lastT = 0, s.dx, animclock.Now()
}

// drag moves the child to offset dx and samples the swipe's speed.
func (s *SwipeDismiss) drag(dx float64) {
	now := animclock.Now()
	if dt := now.Sub(s.lastT).Seconds(); dt > 0 {
		s.vel = (dx - s.lastX) / dt
	}
	s.lastX, s.lastT = dx, now
	s.dx = dx
	s.place()
}

// release sends the child out when it went far or fast enough that
// way, else back.
func (s *SwipeDismiss) release() {
	s.swiping = false
	w := float64(max(1, s.bounds.W))
	dir := 1
	if s.dx < 0 {
		dir = -1
	}
	far := math.Abs(s.dx) > swipeDismissAt*w
	fling := math.Abs(s.vel) > swipeFlingSpeed && math.Signbit(s.vel) == math.Signbit(s.dx) && s.dx != 0
	s.spring.Snap(s.dx)
	if far || fling {
		s.out = dir
		s.spring.SetTarget(float64(dir) * w)
		return
	}
	s.spring.SetTarget(0)
}

// settled takes the spring's position; reaching the far side fires
// OnDismiss, once.
func (s *SwipeDismiss) settled(x float64) {
	s.dx = x
	s.place()
	if s.out != 0 && !s.dismissed && x == float64(s.out)*float64(max(1, s.bounds.W)) {
		s.dismissed = true
		if s.OnDismiss != nil {
			s.OnDismiss(s.out)
		}
	}
}
