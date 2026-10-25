package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/anim"
)

var (
	chartMonths = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun"}
	chartSeries = []ChartSeries{
		{Name: "Desktop", Values: []float64{186, 305, 237, 73, 209, 214}},
		{Name: "Mobile", Values: []float64{80, 200, 120, 190, 130, 140}},
	}
)

func TestNiceMax(t *testing.T) {
	for in, want := range map[float64]float64{305: 400, 0: 1, 9: 10, 40: 40, 41: 80, 520: 800, 0.3: 0.4} {
		if got := niceMax(in); got != want {
			t.Errorf("niceMax(%v) = %v, want %v", in, got, want)
		}
	}
}

// Hovering a bar chart picks the category under the pointer, a pie the
// wedge; leaving clears it.
func TestChartHover(t *testing.T) {
	c := NewChart(goldenFace(t), ChartBar, chartMonths, chartSeries...)
	arrangeTree(t, c, 480, 260)
	plot := c.plot()
	c.HoverMove(Point{X: plot.X + plot.W/6*2 + 4, Y: plot.Y + 10})
	if c.hover != 2 {
		t.Fatalf("hover on the third band = %d", c.hover)
	}
	c.SetHovered(false)
	if c.hover != -1 {
		t.Error("leaving kept the tooltip")
	}
	pie := NewChart(goldenFace(t), ChartPie, []string{"a", "b"}, ChartSeries{Name: "v", Values: []float64{3, 1}})
	arrangeTree(t, pie, 300, 300)
	pp := pie.plot()
	cx, cy, r := roundCenter(pp)
	pie.HoverMove(Point{X: int(cx + r/2), Y: int(cy)})
	if pie.hover != 0 {
		t.Errorf("the right of a 3:1 pie is wedge %d, want 0", pie.hover)
	}
	pie.HoverMove(Point{X: int(cx - r/2), Y: int(cy - 2)})
	if pie.hover != 1 {
		t.Errorf("the upper left of a 3:1 pie is wedge %d, want 1", pie.hover)
	}
}

// SetData eases to the new values and the summary reads them.
func TestChartDataAndSummary(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	c := NewChart(goldenFace(t), ChartLine, []string{"Q1", "Q2"}, ChartSeries{Name: "Sales", Values: []float64{1, 2}})
	c.SetData([]string{"Q1", "Q2"}, ChartSeries{Name: "Sales", Values: []float64{5, 7.5}})
	if c.value(0, 0) != 5 || c.value(0, 1) != 7.5 {
		t.Fatalf("shown %v after the transition", c.shown)
	}
	if got, want := Describe(c).Name, "Line chart; Sales: Q1 5, Q2 7.5"; got != want {
		t.Errorf("summary %q, want %q", got, want)
	}
}

// TestGoldenChart pins every kind - bars grouped and stacked with
// a tooltip, areas, lines, a donut, a radar and radial bars - in both
// palettes.
func TestGoldenChart(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			cell := func(c *Chart) Widget { return newSizedCell(c, 360) }
			bar := NewChart(face, ChartBar, chartMonths, chartSeries...)
			stacked := NewChart(face, ChartBar, chartMonths, chartSeries...)
			stacked.SetStacked(true)
			area := NewChart(face, ChartArea, chartMonths, chartSeries...)
			area.SetStacked(true)
			line := NewChart(face, ChartLine, chartMonths, chartSeries...)
			browsers := []string{"Chrome", "Safari", "Firefox", "Edge", "Other"}
			visitors := ChartSeries{Name: "Visitors", Values: []float64{275, 200, 187, 173, 90}}
			donut := NewChart(face, ChartPie, browsers, visitors)
			donut.SetDonut(true)
			radar := NewChart(face, ChartRadar, chartMonths, chartSeries...)
			radial := NewChart(face, ChartRadial, browsers, visitors)
			grid := NewGrid(16, 16)
			for i, c := range []*Chart{bar, stacked, area, line, donut, radar, radial} {
				grid.Attach(cell(c), i%2, i/2, 1, 1)
			}
			NewGolden(t, grid, "chart-"+tc.name, goldenTheme(tc.th),
				goldenAfterArrange(func() {
					plot := bar.plot()
					bar.HoverMove(Point{X: plot.X + plot.W/6 + 4, Y: plot.Y + 20})
				}))
		})
	}
}

// Charts take the width they are given: two expanding side by side
// share a row narrower than two wide plots, inside it.
func TestChartsShareARow(t *testing.T) {
	bar := NewChart(goldenFace(t), ChartBar, chartMonths, chartSeries...)
	pie := NewChart(goldenFace(t), ChartPie, []string{"a", "b"}, ChartSeries{Name: "v", Values: []float64{3, 1}})
	row := NewBox(Row, 16, 0)
	row.Append(bar, true).Append(pie, true)
	arrangeTree(t, row, 680, 300)
	b, p := bar.Bounds(), pie.Bounds()
	if p.X+p.W > 680 || b.W != p.W || b.W < chartWidth {
		t.Errorf("bar %v pie %v, want two equal halves inside 680", b, p)
	}
}
