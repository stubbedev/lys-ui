package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/debug"
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
