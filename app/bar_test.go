package app

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget"
)

// A bar's popover opens away from its edge, the side mapped to the
// popup gravity a PopoverConfig takes.
func TestGravityOfABarEdge(t *testing.T) {
	for edge, want := range map[widget.BarEdge]Gravity{
		widget.BarTop: GravityBottom, widget.BarBottom: GravityTop,
		widget.BarLeft: GravityRight, widget.BarRight: GravityLeft,
	} {
		if got := GravityOf(edge.PopoverSide()); got != want {
			t.Errorf("edge %d opens on gravity %d, want %d", edge, got, want)
		}
	}
}
