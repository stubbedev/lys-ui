package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// newTestPanel is a panel with a header holding a title and a close
// button over a 200x100 body, on an 800x600 area.
func newTestPanel(t *testing.T) (*FloatingPanel, *Button, *Router) {
	t.Helper()
	face := goldenFace(t)
	closeBtn := NewButton(NewLabel(face, 14, "Close", 0), 4, 4)
	header := NewBox(Row, 8, 8)
	header.Append(NewLabel(face, 14, "Files", 0), true)
	header.Append(closeBtn, false)
	f := NewFloatingPanel(header, NewSpacer(200, 100))
	arrangeTree(t, f, 800, 600)
	return f, closeBtn, &Router{Root: f}
}

func drag(r *Router, from, to Point) {
	r.Move(from)
	r.Press(BTNLeft, from, 0)
	r.Move(to)
	r.Release(BTNLeft, to)
}

// The panel centers until moved, its header drags it, and it stays
// inside the area; the header's button keeps its click.
func TestFloatingPanelMoves(t *testing.T) {
	f, closeBtn, r := newTestPanel(t)
	p := f.PanelRect()
	if p.X != (800-p.W)/2 || p.Y != (600-p.H)/2 {
		t.Fatalf("panel at %v, want centered", p)
	}
	var settled []render.Rect
	f.OnGeometryChanged = func(r render.Rect) { settled = append(settled, r) }
	grab := Point{X: p.X + 30, Y: p.Y + 10}
	drag(r, grab, Point{X: grab.X - 100, Y: grab.Y + 50})
	arrangeTree(t, f, 800, 600)
	if got := f.PanelRect(); got.X != p.X-100 || got.Y != p.Y+50 {
		t.Errorf("dragged to %v, want moved by -100,+50 from %v", got, p)
	}
	if len(settled) != 1 || settled[0] != f.PanelRect() {
		t.Errorf("geometry reports %v, want the one move's end", settled)
	}
	drag(r, Point{X: f.PanelRect().X + 30, Y: f.PanelRect().Y + 10}, Point{X: -5000, Y: 9000})
	arrangeTree(t, f, 800, 600)
	if got := f.PanelRect(); got.X != 0 || got.Y+got.H != 600 {
		t.Errorf("dragged far to %v, want it kept at the area's bottom left", got)
	}
	clicked := 0
	closeBtn.OnClick = func() { clicked++ }
	b := closeBtn.Bounds()
	at := f.PanelRect()
	drag(r, Point{X: b.X + 2, Y: b.Y + 2}, Point{X: b.X + 3, Y: b.Y + 2})
	arrangeTree(t, f, 800, 600)
	if clicked != 1 || f.PanelRect() != at {
		t.Errorf("the header button clicked %d times and the panel went %v→%v", clicked, at, f.PanelRect())
	}
}

// The corner grip resizes within the limits, the top left fixed.
func TestFloatingPanelResizes(t *testing.T) {
	f, _, r := newTestPanel(t)
	f.SetSizeLimits(Size{W: 150, H: 120}, Size{W: 500, H: 0})
	arrangeTree(t, f, 800, 600)
	p := f.PanelRect()
	corner := Point{X: p.X + p.W - 4, Y: p.Y + p.H - 4}
	drag(r, corner, Point{X: corner.X + 1000, Y: corner.Y + 40})
	arrangeTree(t, f, 800, 600)
	got := f.PanelRect()
	if got.X != p.X || got.Y != p.Y || got.W != 500 || got.H != p.H+40 {
		t.Errorf("resized to %v from %v, want 500 wide (the max) and 40 taller, the corner fixed", got, p)
	}
	corner = Point{X: got.X + got.W - 4, Y: got.Y + got.H - 4}
	drag(r, corner, Point{X: corner.X - 1000, Y: corner.Y - 1000})
	arrangeTree(t, f, 800, 600)
	if got := f.PanelRect(); got.W != 150 || got.H != 120 {
		t.Errorf("shrunk to %v, want the 150x120 minimum", got)
	}
}

// Outside the panel, presses pass through (an input region of the
// panel alone) until OnOutside takes them.
func TestFloatingPanelOutside(t *testing.T) {
	f, _, r := newTestPanel(t)
	out := Point{X: 5, Y: 5}
	if f.HitTest(out) != nil {
		t.Error("a pass-through panel took a press outside it")
	}
	if reg := f.InputRegion(); len(reg) != 1 || reg[0] != f.rect {
		t.Errorf("input region %v, want the panel %v", reg, f.rect)
	}
	outside := 0
	f.OnOutside = func() { outside++ }
	r.Move(out)
	r.Press(BTNLeft, out, 0)
	r.Release(BTNLeft, out)
	if outside != 1 || len(f.InputRegion()) != 0 {
		t.Errorf("outside clicks %d, region %v; want one and the whole surface", outside, f.InputRegion())
	}
}

// TestGoldenFloatingPanel pins the panel's card and grip in both
// palettes.
func TestGoldenFloatingPanel(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			face := goldenFace(t)
			header := NewBox(Row, 8, 12)
			header.Append(NewLabel(face, 14, "Open File", 0), true)
			body := NewBox(Column, 0, 12)
			body.Append(NewLabel(face, 14, "Documents", 0), false)
			f := NewFloatingPanel(header, body)
			f.SetPanelSize(220, 120)
			NewGolden(t, f, "floatingpanel-"+tc.name, goldenTheme(tc.th), goldenFrame(300, 180))
		})
	}
}
