package widget

import (
	"math"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

const (
	// spinnerPeriod is the time per full revolution.
	spinnerPeriod = time.Second
	// spinnerArc is the painted sweep, lucide's Loader2: four fifths of
	// the circle, the gap leading the turn.
	spinnerArc = 2 * math.Pi * 0.8
	// spinnerRadius and spinnerWidth are Loader2's 9 and 2 in its 24px
	// box, as fractions of the spinner's size.
	spinnerRadius = 9.0 / 24
	spinnerWidth  = 2.0 / 24
)

// Spinner is an indeterminate activity indicator (lucide's Loader2): a
// rotating open ring in the text color, the `spinner` element's color.
// The rotation is animation-clock driven — one frame invalidation per
// step, and no timer at all unless the spinner is both spinning and
// visible. A hidden or stopped spinner schedules no wake, so an idle
// app with a parked spinner costs nothing.
type Spinner struct {
	node
	size int
	// angle is the arc's leading edge in radians, wrapped to [0, 2π).
	angle float64
	spin  loop
}

// NewSpinner returns a spinner sized size×size pixels.
func NewSpinner(size int) *Spinner {
	s := &Spinner{size: size}
	s.spin = loop{period: spinnerPeriod, owner: s, step: func(t float64) {
		s.angle = 2 * math.Pi * t
		s.Invalidate()
	}}
	return s
}

// Spinning reports whether the rotation is on.
func (s *Spinner) Spinning() bool { return s.spin.on }

// SetSpinning turns the rotation on or off. Stopping cancels the tween
// at once — the frozen angle stays where it stopped and no wake
// remains scheduled. Starting takes effect once the spinner is on
// screen; until then nothing is scheduled.
func (s *Spinner) SetSpinning(on bool) { s.spin.set(on) }

// Arrange records the rect and syncs the rotation with visibility.
func (s *Spinner) Arrange(r render.Rect) {
	s.node.Arrange(r)
	s.spin.arranged()
}

// Measure wants a size×size square.
func (s *Spinner) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := s.measureHit(axis, forSize); ok {
		return sz
	}
	return s.measureStore(axis, forSize, FixedSizes(s.size))
}

// Paint draws the open ring at the current angle.
func (s *Spinner) Paint(cv *render.Canvas) {
	b := s.bounds
	if b.Empty() {
		return
	}
	d := float64(min(b.W, b.H))
	col := pickc(0, s.style(s), style.PropColor, Current().Text)
	cv.Arc(float64(b.X)+float64(b.W)/2, float64(b.Y)+float64(b.H)/2, d*spinnerRadius, d*spinnerWidth, s.angle, spinnerArc, col)
}

// HitTest returns the spinner when p is inside its bounds.
func (s *Spinner) HitTest(p Point) Widget { return s.HitLeaf(s, p) }
