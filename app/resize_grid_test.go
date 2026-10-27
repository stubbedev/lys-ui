package app

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget"
)

// TestResizeSqueezesGrid is the window-level pin of the grid deficit
// negotiation (#67): a compositor-driven resize shrinking a window
// relayouts the grid into the smaller configure, tracks squeeze
// proportionally, and a floored column rests at its floor instead of
// collapsing - the same one-relayout path every configure takes.
func TestResizeSqueezesGrid(t *testing.T) {
	floored := widget.NewLabel(testFace(t), 12, "keeps its widest token", widget.Current().Text)
	floored.SetWrap(true)
	free := widget.NewLabel(testFace(t), 12, "shrinks", widget.Current().Text)
	free.SetEllipsize(widget.EllipsizeEnd)
	grid := widget.NewGrid(8, 0)
	grid.Attach(floored, 0, 0, 1, 1)
	grid.Attach(free, 1, 0, 1, 1)
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(grid, false)
	h := newPaintHarness(root, 400, 100)
	host := h.wnd.host.(*fakeHost)

	h.frame()
	natural := grid.Bounds().W
	floor := widget.MeasureAsChild(floored, widget.Row, widget.Unknown).Min
	if natural <= floor {
		t.Fatalf("natural grid width %d not above the label floor %d", natural, floor)
	}

	// Shrink the window below the natural width but leave the
	// ellipsizing column room: the configure relayouts, the floored
	// column rests exactly at its token floor, and the ellipsizing
	// column takes the remaining width.
	narrow := floor + 8 + 12
	if narrow >= natural {
		t.Fatalf("test fixture too small: narrow %d >= natural %d", narrow, natural)
	}
	host.resizeTo(narrow, 100)
	if !h.wnd.syncSize() {
		t.Fatal("the shrinking configure was not picked up")
	}
	h.frame()
	if got := floored.Bounds().W; got != floor {
		t.Errorf("floored column = %d wide in a %d window, want its %d token floor", got, narrow, floor)
	}
	if got := free.Bounds(); got.X != floor+8 || got.W != 12 {
		t.Errorf("free column = %+v, want x=%d w=12 (the remainder past the floor and spacing)",
			got, floor+8)
	}
	if got := grid.Bounds().W; got != narrow {
		t.Errorf("grid bounds = %d, want the full %d configure", got, narrow)
	}

	// Shrinking past every floor overflows rather than collapsing: the
	// floored column stays at its floor in an impossible rect.
	host.resizeTo(floor/2, 100)
	if !h.wnd.syncSize() {
		t.Fatal("the second configure was not picked up")
	}
	h.frame()
	if got := floored.Bounds().W; got != floor {
		t.Errorf("floored column collapsed to %d below its floor; overflow is the verdict, not collapse", got)
	}
}
