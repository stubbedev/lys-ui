package widget

import "github.com/stubbedev/lys-lib/render"

// Orientation is the main direction an oriented widget runs along.
type Orientation uint8

const (
	// Horizontal runs left to right.
	Horizontal Orientation = iota
	// Vertical runs top to bottom.
	Vertical
)

// sepThickness is the rule's width in pixels.
const sepThickness = 1

// Separator is the standalone one-pixel theme rule: horizontal between
// stacked rows, vertical between side-by-side panes. Menus paint the
// same rule inline for their ItemSeparator rows; layouts reach for
// this widget. The natural size is one pixel square along both axes —
// boxes stretch the cross axis, and expand stretches the main axis —
// and the rule paints through whatever it was allocated.
type Separator struct {
	node
	orientation Orientation
}

// NewSeparator returns a separator along orientation.
func NewSeparator(orientation Orientation) *Separator {
	return &Separator{orientation: orientation}
}

// Orientation returns the axis the rule runs along.
func (s *Separator) Orientation() Orientation { return s.orientation }

// Measure wants one pixel; allocation decides the rest.
func (s *Separator) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := s.measureHit(axis, forSize); ok {
		return sz
	}
	return s.measureStore(axis, forSize, FixedSizes(sepThickness))
}

// Paint draws the rule centered in the allocated rect, in the theme
// border color menus use for their inline separators.
func (s *Separator) Paint(cv *render.Canvas) {
	b := s.bounds
	if b.Empty() {
		return
	}
	col := Current().Border
	if s.orientation == Horizontal {
		cv.FillRect(render.Rect{X: b.X, Y: b.Y + max(0, (b.H-sepThickness)/2), W: b.W, H: min(sepThickness, b.H)}, col)
		return
	}
	cv.FillRect(render.Rect{X: b.X + max(0, (b.W-sepThickness)/2), Y: b.Y, W: min(sepThickness, b.W), H: b.H}, col)
}

// HitTest returns the separator when p is inside its bounds.
func (s *Separator) HitTest(p Point) Widget { return s.HitLeaf(s, p) }
