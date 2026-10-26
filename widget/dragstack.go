package widget

import (
	"strconv"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/sysfont"
)

// dragStackOffset is how far each card behind the front one shows.
const dragStackOffset = 4

// dragStack is the drag icon of several items: the front item over two
// cards offset behind it, and a count badge on its corner.
type dragStack struct {
	node
	front Widget
	count string
}

// newDragStack stacks front for count items.
func newDragStack(front Widget, count int) *dragStack {
	return &dragStack{front: front, count: strconv.Itoa(count)}
}

// Measure is the front item's size plus the offset of the two cards.
func (s *dragStack) Measure(con Constraints) Size {
	sz := s.front.Measure(con)
	return clampSize(Size{W: sz.W + 2*dragStackOffset, H: sz.H + 2*dragStackOffset}, con)
}

// Arrange records the rect; Paint lays the front item out.
func (s *dragStack) Arrange(r render.Rect) { s.node.Arrange(r) }

// Paint draws the two cards, the front item over them, and the count.
// The front item is a live widget of its list: it is laid out at the
// stack's front for the paint and put back where it was.
func (s *dragStack) Paint(cv *render.Canvas) {
	th := Current()
	b := s.bounds
	front := render.Rect{X: b.X, Y: b.Y, W: b.W - 2*dragStackOffset, H: b.H - 2*dragStackOffset}
	for i := 2; i >= 1; i-- {
		card := front
		card.X += i * dragStackOffset
		card.Y += i * dragStackOffset
		cv.RoundedRect(card, th.Radius, th.CardColor())
		cv.RoundedBorder(card, render.UniformCorners(th.Radius), render.UniformInsets(1), th.Border)
	}
	cv.RoundedRect(front, th.Radius, th.CardColor())
	if fb, ok := s.front.(Boundser); ok {
		was := fb.Bounds()
		s.front.Arrange(front)
		PaintChild(cv, s.front)
		s.front.Arrange(was)
	}
	badge := render.Rect{X: front.X + front.W - 22, Y: front.Y + 2, W: 20, H: 18}
	cv.RoundedRect(badge, 9, th.Accent)
	if face, err := sysfont.UI(); err == nil {
		face.DrawAligned(cv, s.count, badge, 11, th.OnAccent, render.AlignCenter)
	}
}

// HitTest is the stack's own: an icon takes no input.
func (s *dragStack) HitTest(p Point) Widget { return s.HitLeaf(s, p) }
