package widget

import (
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// composite is the shared core of widgets built from other widgets: one
// inner root every pass delegates to, an optional painted surface
// behind it, and the embedding widget (self) as the parent the root
// reports and the hit a press on bare background resolves to. An
// embedder overrides only the pass that is genuinely its own - a
// minimum height, a hit rule, an extra decoration - and inherits the
// rest, so the delegation exists exactly once.
type composite struct {
	node
	self Widget
	root Widget
	// fillWidth claims the full offered width (bars, rows, lists);
	// off, the composite wants its root's natural size.
	fillWidth bool
	// fixedWidth, when positive, pins the measured width (table cells
	// that must line up across rows); the root measures within it.
	fixedWidth int
	// minHeight floors the measured height (chrome bars keep their
	// height when their content is short).
	minHeight int
	// surface, when set, fills the bounds behind the root through the
	// cascade: background-color over this theme fallback, border, and
	// border-radius over surfaceRadius.
	surface       func(th *Theme) Color
	surfaceRadius int
	// surfaceRing is the default border width the surface draws (in
	// the theme's border color) until the cascade declares one.
	surfaceRing int
	// measuredW and floorW are the last measured width and the least
	// it works at, for ShrinkableWidth.
	measuredW, floorW int
	// boxed makes the composite its own CSS element: it paints its
	// cascade box over its bounds less the margin, and lays the root
	// out inside its margin, border and padding, as a Box does. The
	// widget's element and classes then sit on the node the app holds.
	boxed bool
}

// insets is the margin, border and padding a boxed composite lays its
// root inside; none otherwise.
func (c *composite) insets() cssInsets {
	if !c.boxed {
		return cssInsets{}
	}
	return boxOf(c.style(c.self), render.Insets{})
}

// surfaceFill is the plain control surface, the fallback most
// composite surfaces paint.
func surfaceFill(th *Theme) Color { return th.Surface }

// surfaceNone is the unfilled surface: only the cascade's background,
// border, and the default ring paint (outlines such as Frame).
func surfaceNone(*Theme) Color { return 0 }

// initComposite wires the core: self is the embedding widget, root the
// tree it is built from.
func (c *composite) initComposite(self, root Widget) {
	c.self, c.root = self, root
}

// setRoot swaps the delegated tree (a mode switch); the layout reflows.
func (c *composite) setRoot(root Widget) {
	c.root = root
	c.InvalidateLayout()
}

// Measure measures the root, widened to the offer under fillWidth.
func (c *composite) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	in := c.insets()
	inner := in.innerOf(con)
	if c.fixedWidth > 0 {
		inner.Max.W = min(inner.Max.W, c.fixedWidth)
	}
	sz := measureChild(c.self, c.root, inner)
	o := in.outer()
	sz.W += o.Left + o.Right
	sz.H += o.Top + o.Bottom
	floor := sz.W - shrinkableWidthOf(c.root)
	switch {
	case c.fixedWidth > 0:
		sz.W = c.fixedWidth
		floor = sz.W
	case c.fillWidth:
		sz.W = con.Max.W
	}
	sz.H = max(sz.H, c.minHeight)
	sz = clampSize(sz, con)
	c.measuredW, c.floorW = sz.W, min(floor, sz.W)
	return c.measureStore(con, sz)
}

// ShrinkableWidth implements WidthShrinker: a composite gives what its
// root can, and one that fills its offer gives the fill back too, down
// to its content's natural width, as a GTK row allocated less than it
// would take still lays its content out.
func (c *composite) ShrinkableWidth() int { return max(0, c.measuredW-c.floorW) }

// Arrange lays the root over the content box, as its parent: inside
// its margins and held to its CSS sizes, matching Measure.
func (c *composite) Arrange(r render.Rect) {
	c.node.Arrange(r)
	setParents(c.self, c.root)
	arrangeChild(c.root, c.insets().outer().Shrink(r))
}

// Paint draws the surface (when configured) then the root.
func (c *composite) Paint(cv *render.Canvas) {
	switch {
	case c.boxed:
		c.paintSurface(cv, c.insets().margin.Shrink(c.bounds), 0)
	case c.surface != nil:
		c.paintSurface(cv, c.bounds, c.surface(Current()))
	}
	PaintChild(cv, c.root)
}

// paintSurface fills rect with the composite's cascade box: the
// stylesheet's background-color over fill, its border, and its radius
// over surfaceRadius. Embedders that paint a surface on a sub-rect or
// with a state-dependent fill call it from their own Paint.
func (c *composite) paintSurface(cv *render.Canvas, rect render.Rect, fill Color) {
	v := c.style(c.self)
	paintBoxBehind(cv, v, rect, radiusOr(v, c.surfaceRadius), ringOr(v, c.surfaceRing), pickc(0, v, style.PropBackgroundColor, fill))
}

// HitTest resolves into the root; bare background is the embedder.
func (c *composite) HitTest(p Point) Widget {
	if !c.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := c.root.HitTest(p); hit != nil {
		return hit
	}
	return c.self
}

// Children is the root, so focus traversal, the damage walk and the
// accessibility tree reach the widgets a composite is built from.
func (c *composite) Children() []Widget { return []Widget{c.root} }

// appendChildren appends the root, matching Children.
func (c *composite) appendChildren(buf []Widget) []Widget { return append(buf, c.root) }

// appendStyleKids is the root (styleKids).
func (c *composite) appendStyleKids(buf []Widget) []Widget { return append(buf, c.root) }

// sizedCell pins its child's width - the table cell every header and
// row of a ColumnView shares, so columns line up by construction.
type sizedCell struct{ composite }

// newSizedCell wraps child at width (zero leaves the natural width).
func newSizedCell(child Widget, width int) *sizedCell {
	c := &sizedCell{}
	c.initComposite(c, child)
	c.fixedWidth = width
	return c
}

// packs is the leading/trailing slot pair bars and rows share
// (HeaderBar, ActionBar, ActionRow): packed widgets keep their natural
// size, centered across the bar - a switch or button never stretches
// to the row's height.
type packs struct {
	owner interface{ InvalidateLayout() }
	start *Box
	end   *Box
}

// initPacks builds the two slots, spaced by gap.
func (p *packs) initPacks(owner interface{ InvalidateLayout() }, gap int) {
	p.owner = owner
	p.start = NewBox(Row, gap, 0)
	p.end = NewBox(Row, gap, 0)
}

// PackStart adds w to the leading slot.
func (p *packs) PackStart(w Widget) {
	p.start.AppendAligned(w, false, AlignCenter)
	p.owner.InvalidateLayout()
}

// PackEnd adds w to the trailing slot.
func (p *packs) PackEnd(w Widget) {
	p.end.AppendAligned(w, false, AlignCenter)
	p.owner.InvalidateLayout()
}
