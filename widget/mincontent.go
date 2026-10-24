package widget

import (
	"math"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// unboundedPx is a constraint wide enough that nothing measured under
// it clamps: the natural size a min-content query falls back to.
const unboundedPx = 1 << 20

// unbounded is the constraint a natural measure runs under.
var unbounded = Constraints{Max: Size{W: unboundedPx, H: unboundedPx}}

// minContentWidther is a widget whose min-content width differs from
// its natural width: the width of its box, CSS insets included, below
// which it clips or overflows its content.
type minContentWidther interface {
	minContentWidth() int
}

// MinWidth is w's min-content width as a container sees it, GTK's
// minimum width: the narrowest it lays out at without clipping or
// overflowing its content. A wrapping label goes down to its widest
// word, an ellipsizing one to the ellipsis, a horizontally scrolling
// view to nothing, and anything else keeps its natural width; rows add
// their children's minimums, columns and stacks take the widest, and
// the size request, margins and CSS min-width floor it. A host that
// sizes a surface from its content (a popover at its request, widened
// to MinWidth) lays the content out at that width, as GTK does.
func MinWidth(w Widget) int {
	if outOfFlow(w) {
		return 0
	}
	width := contentMinWidth(w)
	if l := layoutOf(w); l != nil {
		width = max(width, l.request.W) + l.margin.Left + l.margin.Right
	}
	return width
}

// contentMinWidth is w's own min-content width, its natural width
// unless it narrows.
func contentMinWidth(w Widget) int {
	if m, ok := w.(minContentWidther); ok {
		return m.minContentWidth()
	}
	return w.Measure(unbounded).W
}

// boxMinWidth wraps a content min-content width in the CSS box: the
// min-width floors the content box (the border box under box-sizing:
// border-box), and the insets add back, as measureBox sizes.
func boxMinWidth(v *style.Values, b cssInsets, content int) int {
	o := b.outer()
	minW := picki(v, style.PropMinWidth, 0)
	if v.Layout.BoxSizing == style.BoxSizingBorder {
		minW -= b.border.Left + b.border.Right + b.padding.Left + b.padding.Right
	}
	return max(content, minW) + o.Left + o.Right
}

// minContentWidth is a wrapping label's widest word, an ellipsizing
// one's ellipsis, and otherwise its natural width.
func (l *Label) minContentWidth() int {
	v := l.style(l)
	switch {
	case l.wrapOf(v):
		return l.MinSize().W
	case l.ellOf(v) != EllipsizeNone:
		face, px := l.effStyle()
		return boxMinWidth(v, boxOf(v, render.Insets{}), int(math.Ceil(face.Shape(render.Ellipsis, px).Advance())))
	}
	return l.Measure(unbounded).W
}

// minContentWidth is a wrapping rich label's widest word, and
// otherwise its natural width.
func (l *RichLabel) minContentWidth() int {
	if l.wrap {
		return l.MinSize().W
	}
	return l.Measure(unbounded).W
}

// minContentWidth adds a row's children's minimums and their spacing,
// a homogeneous row giving each the widest one's, and takes a column's
// widest; the CSS layouts give up what their items can.
func (b *Box) minContentWidth() int {
	v := b.style(b)
	if layoutModeOf(v) != modeBox {
		return b.Measure(unbounded).W - b.ShrinkableWidth()
	}
	sum, widest, shown := 0, 0, 0
	for _, c := range b.child {
		if !flows(c.w) {
			continue
		}
		m := MinWidth(c.w)
		sum += m
		widest = max(widest, m)
		shown++
	}
	content := widest
	if b.axis == Row {
		if b.homogeneous {
			sum = widest * shown
		}
		content = sum + b.gap(v)*max(shown-1, 0)
	}
	return boxMinWidth(v, b.box(v), content)
}

// minContentWidth is the widest page's minimum, or the visible page's
// for a stack sized to it.
func (s *Stack) minContentWidth() int {
	content := 0
	if s.heterogeneous {
		if w, ok := s.kids[s.visible]; ok {
			content = MinWidth(w)
		}
	} else {
		for _, name := range s.order {
			content = max(content, MinWidth(s.kids[name]))
		}
	}
	v := s.style(s)
	return boxMinWidth(v, boxOf(v, render.Insets{}), content)
}

// minContentWidth is nothing for a view that scrolls sideways, and its
// child's minimum beside the scrollbar gutter for one that only
// scrolls up and down.
func (s *Scroll) minContentWidth() int {
	content := 0
	if s.VerticalOnly && s.child != nil {
		content = MinWidth(s.child) + gutter
	}
	v := s.style(s)
	return boxMinWidth(v, boxOf(v, render.Insets{}), content)
}

// minContentWidth is the widest child's minimum.
func (o *Overlay) minContentWidth() int {
	content := 0
	for _, k := range o.kids {
		if IsVisible(k) {
			content = max(content, MinWidth(k))
		}
	}
	v := o.style(o)
	return boxMinWidth(v, boxOf(v, render.Insets{}), content)
}

// minContentWidth is the root's minimum inside the composite's box, or
// the composite's fixed width.
func (c *composite) minContentWidth() int {
	if c.fixedWidth > 0 {
		return c.fixedWidth
	}
	o := c.insets().outer()
	return MinWidth(c.root) + o.Left + o.Right
}
