package widget

import (
	"math"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// unboundedPx is a constraint wide enough that nothing measured under
// it clamps: the natural size a width query falls back to.
const unboundedPx = 1 << 20

// unbounded is the constraint a natural measure runs under.
var unbounded = Constraints{Max: Size{W: unboundedPx, H: unboundedPx}}

// widthQuery picks the width a tree reports: its minimum, or the width
// it prefers when its host sizes from it.
type widthQuery uint8

const (
	// widthMin is GTK's minimum width: the narrowest the widget lays
	// out at without clipping or overflowing.
	widthMin widthQuery = iota
	// widthPreferred is the natural width, except that wrapping text
	// counts at its widest word (or its max-width-chars), the width a
	// GTK popover sizes its content to.
	widthPreferred
)

// queryWidther is a widget whose answer to a width query differs from
// its natural width: the width of its box, CSS insets included.
type queryWidther interface {
	queryWidth(q widthQuery) int
}

// MinWidth is w's min-content width as a container sees it, GTK's
// minimum width: the narrowest it lays out at without clipping or
// overflowing its content. A wrapping label goes down to its widest
// word, an ellipsizing one to the ellipsis, a horizontally scrolling
// view to nothing, and anything else keeps its natural width; rows add
// their children's minimums, columns and stacks take the widest, and
// the size request, margins and CSS min-width floor it.
func MinWidth(w Widget) int { return widthAsChild(w, widthMin) }

// PreferredWidth is the width w asks a host that sizes from it for, as
// a container sees it: its natural width, except that a wrapping label
// counts at its widest word, or at its max-width-chars when set, so the
// text wraps inside the host's request rather than widening it to one
// line. A long label that cannot wrap, even an ellipsizing one, keeps
// its natural width and widens the host. Size a popover as
// max(request, PreferredWidth(content)) and lay the content out at
// that width, as GTK sizes its popovers.
func PreferredWidth(w Widget) int { return widthAsChild(w, widthPreferred) }

// widthAsChild answers q for w with its size request and margins, as
// its container sees it; out of flow it takes no width.
func widthAsChild(w Widget, q widthQuery) int {
	if outOfFlow(w) {
		return 0
	}
	width := contentWidth(w, q)
	if l := layoutOf(w); l != nil {
		width = max(width, l.request.W) + l.margin.Left + l.margin.Right
	}
	return width
}

// contentWidth is w's own answer to q, its natural width unless it
// answers otherwise.
func contentWidth(w Widget, q widthQuery) int {
	if m, ok := w.(queryWidther); ok {
		return m.queryWidth(q)
	}
	return w.Measure(unbounded).W
}

// boxMinWidth wraps a content width in the CSS box: the min-width
// floors the content box (the border box under box-sizing:
// border-box), and the insets add back, as measureBox sizes.
func boxMinWidth(v *style.Values, b cssInsets, content int) int {
	o := b.outer()
	minW := picki(v, style.PropMinWidth, 0)
	if v.Layout.BoxSizing == style.BoxSizingBorder {
		minW -= b.border.Left + b.border.Right + b.padding.Left + b.padding.Right
	}
	return max(content, minW) + o.Left + o.Right
}

// queryWidth: a wrapping label answers its widest word, or its
// max-width-chars when it prefers; an ellipsizing one's minimum is the
// ellipsis; otherwise the natural width.
func (l *Label) queryWidth(q widthQuery) int {
	v := l.style(l)
	switch {
	case l.wrapOf(v) && (q == widthMin || l.maxChars <= 0):
		return l.MinSize().W
	case q == widthMin && l.ellOf(v) != EllipsizeNone:
		face, px := l.effStyle()
		return boxMinWidth(v, boxOf(v, render.Insets{}), int(math.Ceil(face.Shape(render.Ellipsis, px).Advance())))
	}
	return l.Measure(unbounded).W
}

// queryWidth: a wrapping rich label answers its widest word, or its
// max-width-chars when it prefers; otherwise the natural width.
func (l *RichLabel) queryWidth(q widthQuery) int {
	if l.wrap && (q == widthMin || l.maxChars <= 0) {
		return l.MinSize().W
	}
	return l.Measure(unbounded).W
}

// queryWidth adds a row's children's answers and their spacing, a
// homogeneous row giving each the widest one's, and takes a column's
// widest; the CSS layouts answer their natural width, less what their
// items can give up for a minimum.
func (b *Box) queryWidth(q widthQuery) int {
	v := b.style(b)
	if layoutModeOf(v) != modeBox {
		w := b.Measure(unbounded).W
		if q == widthMin {
			w -= b.ShrinkableWidth()
		}
		return w
	}
	sum, widest, shown := 0, 0, 0
	for _, c := range b.child {
		if !flows(c.w) {
			continue
		}
		m := widthAsChild(c.w, q)
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

// queryWidth is the widest page's answer, or the visible page's for a
// stack sized to it.
func (s *Stack) queryWidth(q widthQuery) int {
	content := 0
	if s.heterogeneous {
		if w, ok := s.kids[s.visible]; ok {
			content = widthAsChild(w, q)
		}
	} else {
		for _, name := range s.order {
			content = max(content, widthAsChild(s.kids[name], q))
		}
	}
	v := s.style(s)
	return boxMinWidth(v, boxOf(v, render.Insets{}), content)
}

// queryWidth is nothing for a view that scrolls sideways, and its
// child's answer beside the scrollbar gutter for one that only scrolls
// up and down.
func (s *Scroll) queryWidth(q widthQuery) int {
	content := 0
	if s.VerticalOnly && s.child != nil {
		content = widthAsChild(s.child, q) + gutter
	}
	v := s.style(s)
	return boxMinWidth(v, boxOf(v, render.Insets{}), content)
}

// queryWidth is the widest child's answer.
func (o *Overlay) queryWidth(q widthQuery) int {
	content := 0
	for _, k := range o.kids {
		if IsVisible(k) {
			content = max(content, widthAsChild(k, q))
		}
	}
	v := o.style(o)
	return boxMinWidth(v, boxOf(v, render.Insets{}), content)
}

// queryWidth is the root's answer inside the composite's box, or the
// composite's fixed width.
func (c *composite) queryWidth(q widthQuery) int {
	if c.fixedWidth > 0 {
		return c.fixedWidth
	}
	o := c.insets().outer()
	return widthAsChild(c.root, q) + o.Left + o.Right
}
