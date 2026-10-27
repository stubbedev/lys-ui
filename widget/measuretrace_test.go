package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"
)

// A lysdebug build names each measure-cache miss by reason and widget
// type, and a release build reports nothing (#388).
func TestMeasureMissReport(t *testing.T) {
	MeasureMissReport()
	inner := NewBox(Column, 0, 0)
	inner.Append(newStub(10, 10), false)
	root := NewBox(Column, 0, 0)
	root.Append(inner, false)
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	MeasureMissReport()
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	if report := MeasureMissReport(); report != "" {
		t.Errorf("a cached layout reported %q", report)
	}
	inner.Append(newStub(10, 10), false)
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	report := MeasureMissReport()
	if !debug.Enabled {
		if report != "" {
			t.Errorf("a release build reported %q", report)
		}
		return
	}
	if !strings.Contains(report, "uncached *widget.Box") {
		t.Errorf("report %q, want the box invalidated by its new child", report)
	}
	if MeasureMissReport() != "" {
		t.Error("the report did not clear")
	}
}

// computeCounter counts the measures it actually computes, past the
// cache.
type computeCounter struct {
	node
	computed int
}

func (c *computeCounter) Measure(con Constraints) Size {
	if s, ok := c.measureHit(con); ok {
		return s
	}
	c.computed++
	return c.measureStore(con, Size{W: min(con.Max.W, 50), H: c.computed})
}
func (c *computeCounter) Paint(*render.Canvas) {}
func (c *computeCounter) HitTest(Point) Widget { return nil }

// A page in a vertical scroll beside a sidebar meets four constraint
// sets each frame (the row's whole width and the width the sidebar
// leaves, each with and without the scrollbar gutter): all four stay
// cached, and a fifth evicts the least recently used (#397).
func TestMeasureCacheHoldsFourConstraintSets(t *testing.T) {
	c := &computeCounter{}
	sets := []Constraints{
		{Max: Size{W: 1812, H: unboundedPx}},
		{Max: Size{W: 1820, H: unboundedPx}},
		{Max: Size{W: 1556, H: unboundedPx}},
		{Max: Size{W: 1564, H: unboundedPx}},
	}
	for range 3 {
		for _, con := range sets {
			c.Measure(con)
		}
	}
	if c.computed != 4 {
		t.Fatalf("three passes over four constraint sets computed %d times, want 4", c.computed)
	}
	c.Measure(Constraints{Max: Size{W: 900, H: 600}})
	c.Measure(sets[3])
	c.Measure(sets[2])
	c.Measure(sets[1])
	if c.computed != 5 {
		t.Errorf("computed %d times, want 5: the most recent sets kept", c.computed)
	}
	c.Measure(sets[0])
	if c.computed != 6 {
		t.Errorf("computed %d times, want 6: the least recently used set evicted", c.computed)
	}
}

// After a style change the first re-measure must not revalidate the
// sizes the other widths were measured at under the old style (#391).
func TestMeasureCacheDropsEveryEntryOnAStyleChange(t *testing.T) {
	c := &computeCounter{}
	a, b := Constraints{Max: Size{W: 100, H: 100}}, Constraints{Max: Size{W: 200, H: 100}}
	c.Measure(a)
	c.Measure(b)
	styleGen++
	c.Measure(a)
	if got := c.Measure(b); got.H != 4 {
		t.Errorf("width b after the style change returned the old size %v, want a fresh measure", got)
	}
}
