package widget

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
)

// ChartKind is how a Chart draws its data.
type ChartKind uint8

// The chart kinds.
const (
	// ChartBar draws a bar per series per category, grouped, or
	// stacked with SetStacked.
	ChartBar ChartKind = iota
	// ChartArea fills each series' line down to the baseline, stacked
	// with SetStacked.
	ChartArea
	// ChartLine strokes each series' line.
	ChartLine
	// ChartPie draws the first series as wedges per category; a donut
	// with SetDonut.
	ChartPie
	// ChartRadar draws each series as a polygon over the categories'
	// spokes.
	ChartRadar
	// ChartRadial draws the first series as concentric bars per
	// category, each around a track.
	ChartRadial
)

// String names the kind, for the accessible summary.
func (k ChartKind) String() string {
	return [...]string{"Bar chart", "Area chart", "Line chart", "Pie chart", "Radar chart", "Radial chart"}[k]
}

// ChartSeries is one series of a Chart: a name and a value per
// category; Color 0 takes the theme's next chart color.
type ChartSeries struct {
	Name   string
	Values []float64
	Color  render.Color
}

// chartDuration is a data change's transition.
const chartDuration = 400 * time.Millisecond

// Chart plots categories of data: bars grouped or
// stacked, areas, lines, a pie or donut, a radar, or radial bars, in
// the theme's chart colors (@chart_1 to @chart_5). The cartesian kinds
// draw a value axis on rounded ticks, horizontal grid lines and the
// category labels; every kind but the radial one has a legend of the
// series (pie: of the categories). Hovering a category or a wedge shows
// a tooltip card with its values. Changing the data (SetData) eases the
// shown values to the new ones. Assistive technology reads a summary
// of the data. It styles as the `chart` element.
type Chart struct {
	node
	face    render.Font
	kind    ChartKind
	labels  []string
	series  []ChartSeries
	stacked bool
	donut   bool
	legend  bool

	// shown are the values drawn now, per series per category: the
	// data, or mid-transition between the old data and the new.
	shown  [][]float64
	from   [][]float64
	cancel anim.Cancel
	hover  int
}

// NewChart returns a chart of kind over the categories' labels.
func NewChart(face render.Font, kind ChartKind, labels []string, series ...ChartSeries) *Chart {
	c := &Chart{face: requireFace("widget.NewChart", face), kind: kind, legend: true, hover: -1}
	c.SetElement("chart")
	c.labels, c.series = labels, series
	c.shown = values(series)
	return c
}

// values copies the series' values.
func values(series []ChartSeries) [][]float64 {
	out := make([][]float64, len(series))
	for i, s := range series {
		out[i] = append([]float64(nil), s.Values...)
	}
	return out
}

// SetData replaces the categories and the series, easing the shown
// values to the new ones.
func (c *Chart) SetData(labels []string, series ...ChartSeries) {
	c.from = c.shown
	c.labels, c.series = labels, series
	to := values(series)
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel = anim.Start(chartDuration, func(t float64) {
		t = easeOut(t)
		c.shown = make([][]float64, len(to))
		for i, row := range to {
			c.shown[i] = make([]float64, len(row))
			for j, v := range row {
				var was float64
				if i < len(c.from) && j < len(c.from[i]) {
					was = c.from[i][j]
				}
				c.shown[i][j] = was + (v-was)*t
			}
		}
		c.Invalidate()
	})
}

// SetStacked stacks a bar or area chart's series.
func (c *Chart) SetStacked(on bool) {
	c.stacked = on
	c.Invalidate()
}

// SetDonut cuts a pie chart's middle out.
func (c *Chart) SetDonut(on bool) {
	c.donut = on
	c.Invalidate()
}

// SetLegend shows or hides the legend.
func (c *Chart) SetLegend(on bool) {
	c.legend = on
	c.InvalidateLayout()
}

// color is series (or, for a pie, category) i's color.
func (c *Chart) color(i int) render.Color {
	if i < len(c.series) && c.series[i].Color != 0 && c.kind != ChartPie && c.kind != ChartRadial {
		return c.series[i].Color
	}
	return Current().ChartColor(i)
}

// value is series s's shown value for category i.
func (c *Chart) value(s, i int) float64 {
	if s < len(c.shown) && i < len(c.shown[s]) {
		return c.shown[s][i]
	}
	return 0
}

// cartesian reports a chart drawn over axes.
func (c *Chart) cartesian() bool {
	return c.kind == ChartBar || c.kind == ChartArea || c.kind == ChartLine
}

// The chart's text and spacing, and its natural size: the narrowest
// plot that still reads, which a filling or expanding slot widens to
// the slot's width.
const (
	chartText   = 12
	chartLegend = 28
	chartTicks  = 4
	chartWidth  = 240
	chartHeight = 260
)

// Measure wants the natural plot size.
func (c *Chart) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	return c.measureStore(con, clampSize(Size{W: chartWidth, H: chartHeight}, con))
}

// niceMax is the axis top for a largest value v: chartTicks steps of
// the round step at or above v/chartTicks - 1, 2, 2.5 or 5 times a
// power of ten - so the ticks land on round numbers with little room
// to spare.
func niceMax(v float64) float64 {
	if v <= 0 {
		return 1
	}
	raw := v / chartTicks
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if raw <= m*p {
			return m * p * chartTicks
		}
	}
	return 10 * p * chartTicks
}

// top is the largest shown value, of the stacks when stacked.
func (c *Chart) top() float64 {
	m := 0.0
	for i := range c.labels {
		sum := 0.0
		for s := range c.series {
			v := c.value(s, i)
			if c.stacked {
				sum += v
			} else {
				m = max(m, v)
			}
		}
		m = max(m, sum)
	}
	return m
}

// plot is the rect the data draws in - the bounds less the legend's
// strip and, for the cartesian kinds, the value axis and the category
// labels.
func (c *Chart) plot() render.Rect {
	b := c.bounds
	if c.legend && c.kind != ChartRadial {
		b.H -= chartLegend
	}
	if !c.cartesian() {
		return b
	}
	axisW := 0
	scale := niceMax(c.top())
	for k := 0; k <= chartTicks; k++ {
		w := int(c.face.Shape(tickLabel(scale*float64(k)/chartTicks), chartText).Advance() + 0.5)
		axisW = max(axisW, w+8)
	}
	labelH := c.face.Shape("lg", chartText).LineHeight() + 6
	return render.Rect{X: b.X + axisW, Y: b.Y + 6, W: max(0, b.W-axisW), H: max(0, b.H-labelH-6)}
}

// tickLabel writes an axis tick compactly.
func tickLabel(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// Paint draws the chart, its legend and, on hover, the tooltip.
func (c *Chart) Paint(cv *render.Canvas) {
	plot := c.plot()
	switch c.kind {
	case ChartBar, ChartArea, ChartLine:
		c.paintCartesian(cv, plot)
	case ChartPie:
		c.paintPie(cv, plot)
	case ChartRadar:
		c.paintRadar(cv, plot)
	case ChartRadial:
		c.paintRadial(cv, plot)
	}
	if c.legend && c.kind != ChartRadial {
		c.paintLegend(cv)
	}
	if c.hover >= 0 && c.hover < len(c.labels) {
		c.paintTooltip(cv, plot)
	}
}

// slot is category i's band across the plot.
func (c *Chart) slot(plot render.Rect, i int) (x, w float64) {
	n := max(1, len(c.labels))
	w = float64(plot.W) / float64(n)
	return float64(plot.X) + float64(i)*w, w
}

// yOf maps a value to the plot's height on scale.
func yOf(plot render.Rect, v, scale float64) float64 {
	return float64(plot.Y+plot.H) - v/scale*float64(plot.H)
}

// paintCartesian draws the grid, the value axis, the category labels
// and the bars, areas or lines.
func (c *Chart) paintCartesian(cv *render.Canvas, plot render.Rect) {
	th := Current()
	scale := niceMax(c.top())
	for k := 0; k <= chartTicks; k++ {
		v := scale * float64(k) / chartTicks
		y := int(yOf(plot, v, scale) + 0.5)
		cv.FillRect(render.Rect{X: plot.X, Y: y, W: plot.W, H: 1}, th.Border)
		label := tickLabel(v)
		lh := c.face.Shape("lg", chartText).LineHeight()
		c.face.DrawAligned(cv, label, render.Rect{X: c.bounds.X, Y: y - lh/2, W: plot.X - c.bounds.X - 8, H: lh}, chartText, th.TextMuted, render.AlignEnd)
	}
	labelY := plot.Y + plot.H + 6
	for i, l := range c.labels {
		x, w := c.slot(plot, i)
		c.face.DrawAligned(cv, l, render.Rect{X: int(x), Y: labelY, W: int(w), H: c.face.Shape("lg", chartText).LineHeight()}, chartText, th.TextMuted, render.AlignCenter)
	}
	if c.hover >= 0 && c.kind == ChartBar {
		x, w := c.slot(plot, c.hover)
		cv.FillRect(render.Rect{X: int(x), Y: plot.Y, W: int(w), H: plot.H}, scaleAlpha(th.MutedColor(), 0.5))
	}
	switch c.kind {
	case ChartBar:
		c.paintBars(cv, plot, scale)
	default:
		c.paintLines(cv, plot, scale)
	}
}

// paintBars draws the bars: side by side in a category's band, or one
// stack per category.
func (c *Chart) paintBars(cv *render.Canvas, plot render.Rect, scale float64) {
	n := len(c.series)
	for i := range c.labels {
		x, w := c.slot(plot, i)
		inner := w * 0.7
		base := 0.0
		for s := range n {
			v := c.value(s, i)
			var bx, bw float64
			if c.stacked {
				bx, bw = x+(w-inner)/2, inner
			} else {
				bw = inner / float64(max(1, n))
				bx = x + (w-inner)/2 + float64(s)*bw
			}
			top, bottom := yOf(plot, base+v, scale), yOf(plot, base, scale)
			if c.stacked {
				base += v
			}
			r := render.Rect{X: int(bx + 1), Y: int(top + 0.5), W: max(1, int(bw-2)), H: max(0, int(bottom-top+0.5))}
			radius := 4
			if c.stacked && s < n-1 {
				radius = 0
			}
			cv.RoundedRect(r, min(radius, r.W/2, r.H), c.color(s))
		}
	}
}

// points are series s's line through the categories' centers, on the
// stack below it when stacked.
func (c *Chart) points(plot render.Rect, scale float64, s int) (line, below []float64) {
	for i := range c.labels {
		x, w := c.slot(plot, i)
		base := 0.0
		if c.stacked {
			for t := range s {
				base += c.value(t, i)
			}
		}
		line = append(line, x+w/2, yOf(plot, base+c.value(s, i), scale))
		below = append(below, x+w/2, yOf(plot, base, scale))
	}
	return line, below
}

// paintLines draws the areas or lines.
func (c *Chart) paintLines(cv *render.Canvas, plot render.Rect, scale float64) {
	for s := range c.series {
		line, below := c.points(plot, scale, s)
		if len(line) < 4 {
			continue
		}
		col := c.color(s)
		if c.kind == ChartArea {
			p := &render.Path{}
			p.MoveTo(line[0], line[1])
			for k := 2; k < len(line); k += 2 {
				p.LineTo(line[k], line[k+1])
			}
			for k := len(below) - 2; k >= 0; k -= 2 {
				p.LineTo(below[k], below[k+1])
			}
			p.Close()
			cv.FillPath(p, scaleAlpha(col, 0.4))
		}
		strokePolyline(cv, line, 2, col)
	}
}

// strokePolyline strokes the points (x0, y0, x1, y1, ...) width wide,
// each segment a quad, its joints capped with a dot.
func strokePolyline(cv *render.Canvas, pts []float64, width float64, col render.Color) {
	h := width / 2
	for k := 0; k+3 < len(pts); k += 2 {
		x0, y0, x1, y1 := pts[k], pts[k+1], pts[k+2], pts[k+3]
		dx, dy := x1-x0, y1-y0
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*h, dx/l*h
		p := &render.Path{}
		p.MoveTo(x0+nx, y0+ny)
		p.LineTo(x1+nx, y1+ny)
		p.LineTo(x1-nx, y1-ny)
		p.LineTo(x0-nx, y0-ny)
		p.Close()
		cv.FillPath(p, col)
	}
	for k := 2; k+3 < len(pts); k += 2 {
		fillCircle(cv, pts[k], pts[k+1], h, col)
	}
}

// fillCircle fills a circle of radius r centered on (x, y).
func fillCircle(cv *render.Canvas, x, y, r float64, col render.Color) {
	cv.FillPath(arcPath(x, y, r, 0, 0, 2*math.Pi), col)
}

// arcPath is the ring segment between radii inner and outer from angle
// a0 to a1 (radians, clockwise from twelve o'clock); inner 0 is a
// wedge to the center.
func arcPath(cx, cy, outer, inner, a0, a1 float64) *render.Path {
	steps := max(8, int(math.Abs(a1-a0)*outer/4))
	at := func(r, a float64) (float64, float64) { return cx + r*math.Sin(a), cy - r*math.Cos(a) }
	p := &render.Path{}
	x, y := at(outer, a0)
	p.MoveTo(x, y)
	for k := 1; k <= steps; k++ {
		x, y = at(outer, a0+(a1-a0)*float64(k)/float64(steps))
		p.LineTo(x, y)
	}
	if inner <= 0 {
		p.LineTo(cx, cy)
	} else {
		for k := steps; k >= 0; k-- {
			x, y = at(inner, a0+(a1-a0)*float64(k)/float64(steps))
			p.LineTo(x, y)
		}
	}
	p.Close()
	return p
}

// roundCenter is a round chart's middle and radius and size in the plot.
func roundCenter(plot render.Rect) (cx, cy, r float64) {
	return float64(plot.X) + float64(plot.W)/2, float64(plot.Y) + float64(plot.H)/2, float64(min(plot.W, plot.H))/2 - 8
}

// paintPie draws the first series as wedges.
func (c *Chart) paintPie(cv *render.Canvas, plot render.Rect) {
	cx, cy, r := roundCenter(plot)
	total := 0.0
	for i := range c.labels {
		total += c.value(0, i)
	}
	if total <= 0 {
		return
	}
	inner := 0.0
	if c.donut {
		inner = r * 0.6
	}
	a := 0.0
	for i := range c.labels {
		da := c.value(0, i) / total * 2 * math.Pi
		grow := 0.0
		if i == c.hover {
			grow = 6
		}
		cv.FillPath(arcPath(cx, cy, r+grow, inner, a, a+da), c.color(i))
		a += da
	}
}

// paintRadar draws the spoke grid and each series' polygon.
func (c *Chart) paintRadar(cv *render.Canvas, plot render.Rect) {
	th := Current()
	cx, cy, r := roundCenter(plot)
	r -= float64(c.face.Shape("lg", chartText).LineHeight())
	n := len(c.labels)
	if n < 3 {
		return
	}
	at := func(i int, f float64) (float64, float64) {
		a := float64(i) / float64(n) * 2 * math.Pi
		return cx + f*r*math.Sin(a), cy - f*r*math.Cos(a)
	}
	for ring := 1; ring <= chartTicks; ring++ {
		var pts []float64
		for i := 0; i <= n; i++ {
			x, y := at(i%n, float64(ring)/chartTicks)
			pts = append(pts, x, y)
		}
		strokePolyline(cv, pts, 1, th.Border)
	}
	lh := c.face.Shape("lg", chartText).LineHeight()
	for i, l := range c.labels {
		x, y := at(i, 1.12)
		c.face.DrawAligned(cv, l, render.Rect{X: int(x) - 40, Y: int(y) - lh/2, W: 80, H: lh}, chartText, th.TextMuted, render.AlignCenter)
	}
	scale := niceMax(c.top())
	for s := range c.series {
		p := &render.Path{}
		var pts []float64
		for i := 0; i <= n; i++ {
			x, y := at(i%n, c.value(s, i%n)/scale)
			pts = append(pts, x, y)
			if i == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		p.Close()
		cv.FillPath(p, scaleAlpha(c.color(s), 0.5))
		strokePolyline(cv, pts, 2, c.color(s))
	}
}

// paintRadial draws the first series as concentric bars on tracks.
func (c *Chart) paintRadial(cv *render.Canvas, plot render.Rect) {
	th := Current()
	cx, cy, r := roundCenter(plot)
	n := len(c.labels)
	if n == 0 {
		return
	}
	scale := niceMax(c.top())
	band := r * 0.7 / float64(n)
	for i := range c.labels {
		outer := r - float64(i)*band
		inner := outer - band*0.7
		cv.FillPath(arcPath(cx, cy, outer, inner, 0, 2*math.Pi), scaleAlpha(th.MutedColor(), 0.6))
		cv.FillPath(arcPath(cx, cy, outer, inner, 0, c.value(0, i)/scale*2*math.Pi), c.color(i))
	}
}

// legendEntries are the legend's names: the categories for a pie, the
// series otherwise.
func (c *Chart) legendEntries() []string {
	if c.kind == ChartPie {
		return c.labels
	}
	names := make([]string, len(c.series))
	for i, s := range c.series {
		names[i] = s.Name
	}
	return names
}

// paintLegend draws a swatch and a name per entry, centered under the
// plot.
func (c *Chart) paintLegend(cv *render.Canvas) {
	th := Current()
	entries := c.legendEntries()
	widths := make([]int, len(entries))
	total := 0
	for i, e := range entries {
		widths[i] = 10 + 6 + int(c.face.Shape(e, chartText).Advance()+0.5) + 16
		total += widths[i]
	}
	x := c.bounds.X + (c.bounds.W-total)/2
	y := c.bounds.Y + c.bounds.H - chartLegend/2
	lh := c.face.Shape("lg", chartText).LineHeight()
	for i, e := range entries {
		cv.RoundedRect(render.Rect{X: x, Y: y - 5, W: 10, H: 10}, 2, c.color(i))
		c.face.DrawAligned(cv, e, render.Rect{X: x + 16, Y: y - lh/2, W: widths[i], H: lh}, chartText, th.Text, render.AlignStart)
		x += widths[i]
	}
}

// paintTooltip draws the hovered category's card: its label and each
// series' value, beside the pointer's category.
func (c *Chart) paintTooltip(cv *render.Canvas, plot render.Rect) {
	th := Current()
	lh := c.face.Shape("lg", chartText).LineHeight()
	type row struct {
		name, value string
		col         render.Color
	}
	var rows []row
	if c.kind == ChartPie || c.kind == ChartRadial {
		rows = append(rows, row{c.labels[c.hover], formatValue(c.value(0, c.hover)), c.color(c.hover)})
	} else {
		for s, sr := range c.series {
			rows = append(rows, row{sr.Name, formatValue(c.value(s, c.hover)), c.color(s)})
		}
	}
	w := 120
	for _, r := range rows {
		w = max(w, 28+int(c.face.Shape(r.name, chartText).Advance()+c.face.Shape(r.value, chartText).Advance()+0.5)+24)
	}
	h := 12 + lh + 4 + len(rows)*(lh+4) + 8
	x, slotW := c.slot(plot, c.hover)
	card := render.Rect{X: int(x+slotW) + 8, Y: plot.Y + 8, W: w, H: h}
	if card.X+card.W > c.bounds.X+c.bounds.W {
		card.X = int(x) - w - 8
	}
	if !c.cartesian() {
		card.X, card.Y = c.bounds.X+8, c.bounds.Y+8
	}
	cv.BoxShadow(card, render.UniformCorners(8), render.BoxShadow{Y: 2, Blur: 8, Color: render.RGBA(0, 0, 0, 40)})
	cv.RoundedRect(card, 8, th.PopoverColor())
	cv.RoundedBorder(card, render.UniformCorners(8), render.UniformInsets(1), th.Border)
	y := card.Y + 8
	if c.cartesian() || c.kind == ChartRadar {
		c.face.DrawAligned(cv, c.labels[c.hover], render.Rect{X: card.X + 10, Y: y, W: w - 20, H: lh}, chartText, th.Text, render.AlignStart)
		y += lh + 4
	}
	for _, r := range rows {
		cv.RoundedRect(render.Rect{X: card.X + 10, Y: y + lh/2 - 5, W: 10, H: 10}, 2, r.col)
		c.face.DrawAligned(cv, r.name, render.Rect{X: card.X + 26, Y: y, W: w - 36, H: lh}, chartText, th.TextMuted, render.AlignStart)
		c.face.DrawAligned(cv, r.value, render.Rect{X: card.X + 26, Y: y, W: w - 36, H: lh}, chartText, th.Text, render.AlignEnd)
		y += lh + 4
	}
}

// formatValue writes a value for the tooltip and the summary.
func formatValue(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

// HoverMove picks the category, or the pie's wedge, under the pointer.
func (c *Chart) HoverMove(p Point) {
	h := c.categoryAt(p)
	if h != c.hover {
		c.hover = h
		c.Invalidate()
	}
}

// SetHovered clears the tooltip when the pointer leaves.
func (c *Chart) SetHovered(on bool) {
	if !on && c.hover >= 0 {
		c.hover = -1
		c.Invalidate()
	}
}

// categoryAt is the category under p, -1 for none.
func (c *Chart) categoryAt(p Point) int {
	plot := c.plot()
	switch c.kind {
	case ChartPie:
		cx, cy, r := roundCenter(plot)
		dx, dy := float64(p.X)-cx, float64(p.Y)-cy
		if math.Hypot(dx, dy) > r || c.donut && math.Hypot(dx, dy) < r*0.6 {
			return -1
		}
		a := math.Atan2(dx, -dy)
		if a < 0 {
			a += 2 * math.Pi
		}
		total := 0.0
		for i := range c.labels {
			total += c.value(0, i)
		}
		acc := 0.0
		for i := range c.labels {
			acc += c.value(0, i) / total * 2 * math.Pi
			if a <= acc {
				return i
			}
		}
		return -1
	case ChartRadar, ChartRadial:
		return -1
	}
	if !plot.Contains(p.X, p.Y) {
		return -1
	}
	n := max(1, len(c.labels))
	return min(n-1, (p.X-plot.X)*n/max(1, plot.W))
}

// HitTest takes the pointer over the chart.
func (c *Chart) HitTest(p Point) Widget { return c.HitLeaf(c, p) }

// Role is RoleChart.
func (c *Chart) Role() Role { return RoleChart }

// Summary is the chart's data in words: its kind, then each series'
// values by category.
func (c *Chart) Summary() string {
	var b strings.Builder
	b.WriteString(c.kind.String())
	for _, sr := range c.series {
		fmt.Fprintf(&b, "; %s:", sr.Name)
		for i, l := range c.labels {
			if i < len(sr.Values) {
				fmt.Fprintf(&b, " %s %s", l, formatValue(sr.Values[i]))
				if i < len(c.labels)-1 {
					b.WriteByte(',')
				}
			}
		}
	}
	return b.String()
}

// a11yName is the summary.
func (c *Chart) a11yName() string { return c.Summary() }
