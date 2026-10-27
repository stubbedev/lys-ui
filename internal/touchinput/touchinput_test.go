package touchinput

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/widget"
)

// A pen drives the pointer and feeds its samples to the widget under
// it: hovering in range, the tip pressing (a stroke stays with the
// widget it began on), and leaving range ending the hover.
func TestTabletBridge(t *testing.T) {
	area := widget.NewDrawingArea(100, 100)
	var samples []widget.Stylus
	area.OnStylus = func(s widget.Stylus) { samples = append(samples, s) }
	row := widget.NewBox(widget.Row, 0, 0)
	row.Append(area, false)
	row.Append(widget.NewSpacer(100, 100), false)
	widget.SizeWithin(row, widget.Size{W: 200, H: 100})
	row.Arrange(render.Rect{W: 200, H: 100})
	r := &widget.Router{Root: row}
	in := Input{Tracker: widget.TouchTracker{Router: r}}
	frame := func(s wlsession.TabletSample) { in.HandleTabletFrame(s) }
	frame(wlsession.TabletSample{ProximityIn: true, X: 10, Y: 10})
	if r.Hovered() != widget.Widget(area) {
		t.Fatal("a pen in range does not hover")
	}
	frame(wlsession.TabletSample{Down: true, X: 10, Y: 10, Pressure: 0.5, Serial: 3})
	frame(wlsession.TabletSample{X: 150, Y: 10, Pressure: 0.9}) // the stroke wanders off the area
	frame(wlsession.TabletSample{Up: true, X: 150, Y: 10})
	frame(wlsession.TabletSample{ProximityOut: true, X: 150, Y: 10})
	if len(samples) != 4 {
		t.Fatalf("the area took %d samples, want 4 (the stroke's, plus hover)", len(samples))
	}
	if !samples[1].Down || samples[1].Pressure != 0.5 || !samples[2].Down || samples[2].At.X != 150 || samples[3].Down {
		t.Errorf("stroke samples: %+v", samples)
	}
	if r.Hovered() != nil || in.Serial != 3 {
		t.Errorf("after leaving range: hover %v serial %d", r.Hovered(), in.Serial)
	}
}
