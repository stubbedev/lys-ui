package widget

import (
	"slices"

	"github.com/stubbedev/lys-ui/render"
)

// Pile defaults: the cards peeking behind the front, the step each
// shows past the previous one, the per-side inset each level of depth
// narrows by, and the gap between cards expanded.
const (
	pilePeek        = 2
	pilePeekOffset  = 8
	pilePeekInset   = 8
	pileSpacing     = 8
	pileFrontClass  = "front"
	pilePeekClass   = "peek"
	pileBuriedClass = "buried"
	pileOpenClass   = "expanded"
)

// Pile stacks its children as cards: collapsed, the front card (index
// 0) shows whole and the next few peek out past one edge of it, each a
// step further and narrower, showing only their own box (background,
// border, shadow) and never their content; expanded, the cards lay out
// as a column with a gap. The change between the two moves the cards
// on their layout motion, which a pile opts its children into
// (smooth) unless the stylesheet's -lys-layout-motion says otherwise.
//
// The pile carries the `expanded` class while expanded; its children
// carry `front`, `peek` or `buried` (deeper than the peek, neither
// painted nor hit). Collapsed, only the front card takes the pointer.
type Pile struct {
	node
	kids     []Widget
	expanded bool
	peek     int
	offset   int
	inset    int
	spacing  int
	edge     Edge
	// nat are the children's last measured sizes.
	nat []Size
}

// NewPile returns an empty, collapsed pile peeking two cards past its
// bottom edge.
func NewPile() *Pile {
	p := &Pile{peek: pilePeek, offset: pilePeekOffset, inset: pilePeekInset, spacing: pileSpacing, edge: EdgeBottom}
	p.SetElement("pile")
	return p
}

// Append adds w behind the other cards.
func (p *Pile) Append(w Widget) { p.Insert(len(p.kids), w) }

// Insert adds w at index i (clamped): 0 is the front.
func (p *Pile) Insert(i int, w Widget) {
	i = min(max(i, 0), len(p.kids))
	p.kids = slices.Insert(p.kids, i, w)
	if n := nodeOf(w); n != nil && n.layoutMotion == LayoutMotionNone {
		n.SetLayoutMotion(LayoutMotionSmooth)
	}
	setParents(p, w)
	p.restack()
}

// Remove detaches w; a widget not in the pile is left alone.
func (p *Pile) Remove(w Widget) {
	i := slices.Index(p.kids, w)
	if i < 0 {
		return
	}
	p.kids = slices.Delete(p.kids, i, i+1)
	notifyRemoved(w)
	clearParents(w)
	p.restack()
}

// Children returns the cards, front first.
func (p *Pile) Children() []Widget { return slices.Clone(p.kids) }

// appendChildren is Children into buf.
func (p *Pile) appendChildren(buf []Widget) []Widget { return append(buf, p.kids...) }

// SetExpanded lays the cards out as a column (true) or stacks them.
func (p *Pile) SetExpanded(on bool) {
	if p.expanded == on {
		return
	}
	p.expanded = on
	p.restack()
}

// Expanded reports whether the cards lay out as a column.
func (p *Pile) Expanded() bool { return p.expanded }

// SetPeek sets how many cards peek behind the front collapsed; below 0
// it is 0.
func (p *Pile) SetPeek(n int) {
	if n = max(n, 0); n != p.peek {
		p.peek = n
		p.restack()
	}
}

// SetPeekOffset sets the step, in logical pixels, each peeking card
// shows past the one in front of it.
func (p *Pile) SetPeekOffset(px int) {
	if px != p.offset {
		p.offset = px
		p.InvalidateLayout()
	}
}

// SetPeekInset sets how far, in logical pixels, each level of depth
// narrows a peeking card on either side.
func (p *Pile) SetPeekInset(px int) {
	if px != p.inset {
		p.inset = px
		p.InvalidateLayout()
	}
}

// SetSpacing sets the gap between expanded cards.
func (p *Pile) SetSpacing(px int) {
	if px != p.spacing {
		p.spacing = px
		p.InvalidateLayout()
	}
}

// SetPeekEdge sets the edge the cards peek past: EdgeBottom for a pile
// hanging from the top of the screen, EdgeTop for one standing on the
// bottom, whose front card then sits lowest. Expanded, the front card
// stays at that same end of the column.
func (p *Pile) SetPeekEdge(e Edge) {
	if e != EdgeTop {
		e = EdgeBottom
	}
	if e != p.edge {
		p.edge = e
		p.InvalidateLayout()
	}
}

// shownPeeks is how many cards peek behind the front, collapsed.
func (p *Pile) shownPeeks() int {
	return max(0, min(p.peek, len(p.visible())-1))
}

// visible are the children that take part in layout, front first.
func (p *Pile) visible() []Widget {
	out := make([]Widget, 0, len(p.kids))
	for _, w := range p.kids {
		if IsVisible(w) {
			out = append(out, w)
		}
	}
	return out
}

// restack owes the cards a relayout in their new order or state.
func (p *Pile) restack() {
	p.InvalidateLayout()
	restyleChildren(p)
}

// syncClasses moves the state classes onto the pile and its cards: a
// shown card at depth d is the front at 0, collapsed a peek up to the
// peek count and buried past it.
func (p *Pile) syncClasses() {
	p.SetClass(pileOpenClass, p.expanded)
	peeks, d := p.shownPeeks(), 0
	for _, w := range p.kids {
		c, ok := w.(interface{ SetClass(string, bool) })
		shown := IsVisible(w)
		if ok {
			c.SetClass(pileFrontClass, shown && d == 0)
			c.SetClass(pilePeekClass, shown && !p.expanded && d > 0 && d <= peeks)
			c.SetClass(pileBuriedClass, shown && !p.expanded && d > peeks)
		}
		if shown {
			d++
		}
	}
}

// Measure is the front card and its peeks collapsed, the column of
// cards expanded, inside the pile's CSS box.
func (p *Pile) Measure(con Constraints) Size {
	if sz, ok := p.measureHit(con); ok {
		return sz
	}
	v := p.style(p)
	return p.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		vis := p.visible()
		p.nat = p.nat[:0]
		var sz Size
		for i, w := range vis {
			n := measureChild(p, w, Constraints{Max: Size{W: inner.Max.W, H: inner.Max.H}})
			p.nat = append(p.nat, n)
			sz.W = max(sz.W, n.W)
			switch {
			case p.expanded && i > 0:
				sz.H += p.spacing + n.H
			case i == 0:
				sz.H = n.H
			}
		}
		if !p.expanded && len(vis) > 0 {
			sz.H += p.shownPeeks() * p.offset
		}
		return clampSize(sz, inner)
	}))
}

// Arrange places the cards: collapsed, the front at the end away from
// the peek edge and each peek at the front's rect, narrowed and shifted
// toward the edge, the buried ones under the last peek; expanded, a
// column from the front's end.
func (p *Pile) Arrange(r render.Rect) {
	p.syncClasses()
	border, inner := boxRects(boxOf(p.style(p), render.Insets{}), r)
	p.node.Arrange(border)
	vis := p.visible()
	if len(vis) == 0 || len(p.nat) != len(vis) {
		return
	}
	if p.expanded {
		y := inner.Y
		if p.edge == EdgeTop {
			y = inner.Y + inner.H
		}
		for i, w := range vis {
			h := p.nat[i].H
			if p.edge == EdgeTop {
				y -= h
				arrangeChild(w, render.Rect{X: inner.X, Y: y, W: inner.W, H: h})
				y -= p.spacing
				continue
			}
			arrangeChild(w, render.Rect{X: inner.X, Y: y, W: inner.W, H: h})
			y += h + p.spacing
		}
		return
	}
	peeks := p.shownPeeks()
	front := render.Rect{X: inner.X, Y: inner.Y, W: inner.W, H: p.nat[0].H}
	step := p.offset
	if p.edge == EdgeTop {
		front.Y = inner.Y + inner.H - front.H
		step = -step
	}
	for i, w := range vis {
		d := min(i, peeks)
		in := d * p.inset
		arrangeChild(w, render.Rect{X: front.X + in, Y: front.Y + d*step, W: max(0, front.W-2*in), H: front.H})
	}
}

// Paint draws the cards back to front: collapsed, the peeks as their
// own boxes only, then the front whole; expanded, every card.
func (p *Pile) Paint(cv *render.Canvas) {
	paintCSSBox(cv, p.style(p), p.bounds)
	vis := p.visible()
	if p.expanded {
		for _, w := range slices.Backward(vis) {
			PaintChild(cv, w)
		}
		return
	}
	for i := p.shownPeeks(); i > 0; i-- {
		w := vis[i]
		n := nodeOf(w)
		if n == nil {
			continue
		}
		// The card's spring keeps moving while it peeks, so expanding
		// sets out from where it shows.
		n.moving(w)
		paintCSSBox(cv, n.style(w), ShownRect(w))
	}
	if len(vis) > 0 {
		PaintChild(cv, vis[0])
	}
}

// HitTest is the front card collapsed, any card expanded, else the
// pile within its bounds.
func (p *Pile) HitTest(pt Point) Widget {
	vis := p.visible()
	if !p.expanded && len(vis) > 0 {
		vis = vis[:1]
	}
	for _, w := range vis {
		if hit := w.HitTest(pt); hit != nil {
			return hit
		}
	}
	return p.HitLeaf(p, pt)
}
