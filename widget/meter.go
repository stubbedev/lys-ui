package widget

import (
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// meter is the trough-and-fill core ProgressBar and LevelBar share:
// GTK's `<bar> > trough > <fill>` node tree, a value in [0, 1], a
// 160x10 trough floored by the trough's min sizes, and the paint of
// the trough with the fill over a span of it - the value from the
// start, or wherever the embedder's span says (a pulse block).
type meter struct {
	node
	value  float64
	trough meterTrough
	// self is the embedding widget, for reading its own cascade
	// (accent-color) where the meter paints the fill.
	self Widget
	// fillFill colors the fill the cascade gives no background, and
	// troughFill the trough from that fill (accent-color applied); nil
	// leaves them cascade-only.
	fillFill   func(*Theme) Color
	troughFill func(fill Color) Color
	// roundTrough defaults the trough's radius to half its height.
	roundTrough bool
}

// meterTrough is the trough node and its fill.
type meterTrough struct {
	stylePart
	fill stylePart
}

func (t *meterTrough) appendStyleKids(buf []Widget) []Widget { return append(buf, &t.fill) }

// initMeter names the nodes and links them under self.
func (m *meter) initMeter(self Widget, value float64, fill string) {
	m.value = math01(value)
	m.self = self
	m.trough.SetElement("trough")
	m.trough.fill.SetElement(fill)
	setParents(self, &m.trough)
	setParents(&m.trough, &m.trough.fill)
}

// appendStyleKids is the trough (styleKids).
func (m *meter) appendStyleKids(buf []Widget) []Widget { return append(buf, &m.trough) }

// Value returns the fill fraction in [0, 1].
func (m *meter) Value() float64 { return m.value }

// setValue clamps v to [0, 1] and reports whether it changed,
// invalidating the bar if so.
func (m *meter) setValue(v float64) bool {
	v = math01(v)
	if v == m.value {
		return false
	}
	m.value = v
	m.Invalidate()
	return true
}

// troughSize is the trough's border box: 160x10, floored by the
// trough's min sizes, GTK's progressbar trough gadget.
func (m *meter) troughSize() Size {
	tv := m.trough.style(&m.trough)
	return Size{W: picki(tv, style.PropMinWidth, 160), H: picki(tv, style.PropMinHeight, 8)}
}

// measureMeter wants the trough inside self's CSS box, clamped to con.
func (m *meter) measureMeter(self Widget, con Constraints) Size {
	if sz, ok := m.measureHit(con); ok {
		return sz
	}
	ts := m.troughSize()
	v := m.style(self)
	return m.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(ts, inner)
	}))
}

// Arrange records the bar's border box and lays the trough across its
// content box at the trough's own height, centered, so a bar given
// more height than its trough keeps a thin trough.
func (m *meter) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(m.style(m.self), render.Insets{}), r)
	m.node.Arrange(border)
	h := min(inner.H, m.troughSize().H)
	m.trough.Arrange(render.Rect{X: inner.X, Y: inner.Y + (inner.H-h)/2, W: inner.W, H: h})
}

// paintMeter draws the trough, then the fill over the [from, to)
// fraction of the trough's content box, each from its cascade.
func (m *meter) paintMeter(cv *render.Canvas, from, to float64) {
	t := Current()
	tv := m.trough.style(&m.trough)
	fx := pushEffects(cv, &m.node, tv, m.bounds, render.Corners{})
	cv = fx.cv
	defer fx.pop()
	box := m.trough.bounds
	tr := 0
	if m.roundTrough {
		tr = box.H / 2
	}
	fill := fallback(m.fillFill, t)
	if m.self != nil {
		fill = accentOf(m.style(m.self), fill)
	}
	trough := Color(0)
	if m.troughFill != nil {
		trough = m.troughFill(fill)
	}
	_, inner := boxRects(boxOf(tv, render.Insets{}), box)
	paintBoxBehind(cv, tv, box, radiusOr(tv, tr), borderOf(tv), pickc(0, tv, style.PropBackgroundColor, trough))
	x0, x1 := int(float64(inner.W)*from), int(float64(inner.W)*to)
	if x1 <= x0 {
		return
	}
	fr := render.Rect{X: inner.X + x0, Y: inner.Y, W: x1 - x0, H: inner.H}
	m.trough.fill.Arrange(fr)
	fv := m.trough.fill.style(&m.trough.fill)
	fxp := pushEffects(cv, &m.trough.fill.node, fv, fr, radiusOr(fv, inner.H/2))
	cv = fxp.cv
	paintBoxBehind(cv, fv, fr, radiusOr(fv, inner.H/2), borderOf(fv), pickc(0, fv, style.PropBackgroundColor, fill))
	fxp.pop()
}

// fallback resolves an optional themed color.
func fallback(fn func(*Theme) Color, t *Theme) Color {
	if fn == nil {
		return 0
	}
	return fn(t)
}

// HitTest is the leaf hit: the whole bar.
func (m *meter) hitMeter(self Widget, p Point) Widget { return m.HitLeaf(self, p) }

func math01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
