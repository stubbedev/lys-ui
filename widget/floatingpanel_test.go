package widget

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stubbedev/lys-lib/render"
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

// The corner grip resizes within the limits, the top left fixed,
// pressed at the panel's very corner or on its painted mark.
func TestFloatingPanelResizes(t *testing.T) {
	f, _, r := newTestPanel(t)
	f.SetSizeLimits(Size{W: 150, H: 120}, Size{W: 500, H: 0})
	arrangeTree(t, f, 800, 600)
	p := f.PanelRect()
	corner := Point{X: p.X + p.W - 1, Y: p.Y + p.H - 1}
	drag(r, corner, Point{X: corner.X + 1000, Y: corner.Y + 40})
	arrangeTree(t, f, 800, 600)
	got := f.PanelRect()
	if got.X != p.X || got.Y != p.Y || got.W != 500 || got.H != p.H+40 {
		t.Errorf("resized to %v from %v, want 500 wide (the max) and 40 taller, the corner fixed", got, p)
	}
	m := f.grip.mark
	mark := Point{X: m.X + m.W/2, Y: m.Y + m.H/2}
	drag(r, mark, Point{X: mark.X - 1000, Y: mark.Y - 1000})
	arrangeTree(t, f, 800, 600)
	if got := f.PanelRect(); got.W != 150 || got.H != 120 {
		t.Errorf("shrunk to %v, want the 150x120 minimum", got)
	}
}

// The panel asks for its chosen size, else its content's natural one,
// within its limits: a window sized to it opened at the content's size
// whatever the panel was set to.
func TestFloatingPanelMeasuresItsSize(t *testing.T) {
	measure := func(set, lo, hi Size) Size {
		t.Helper()
		f := NewFloatingPanel(nil, NewSpacer(200, 100))
		f.SetSizeLimits(lo, hi)
		f.SetPanelSize(set.W, set.H)
		arrangeTree(t, f, 2000, 2000)
		return SizeWithin(f, Size{W: 2000, H: 2000})
	}
	nat := measure(Size{}, Size{}, Size{})
	if nat.W < 200 || nat.H < 100 {
		t.Fatalf("natural %v, want at least the 200x100 content", nat)
	}
	for _, tc := range []struct {
		name        string
		set, lo, hi Size
		want        Size
	}{
		{"set", Size{W: 760, H: 520}, Size{W: 480, H: 320}, Size{W: 1600, H: 1100}, Size{W: 760, H: 520}},
		{"set above the max", Size{W: 760, H: 520}, Size{W: 480, H: 320}, Size{W: 700, H: 0}, Size{W: 700, H: 520}},
		{"set below the floor", Size{W: 300, H: 200}, Size{W: 480, H: 320}, Size{}, Size{W: 480, H: 320}},
		{"set on one axis", Size{W: 760}, Size{}, Size{}, Size{W: 760, H: nat.H}},
		{"natural", Size{}, Size{W: 10, H: 10}, Size{W: 1000, H: 1000}, nat},
		{"natural below the floor", Size{}, Size{W: 480, H: 320}, Size{}, Size{W: 480, H: 320}},
		{"natural above the max", Size{}, Size{}, Size{W: 150, H: 80}, Size{W: 150, H: 80}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := measure(tc.set, tc.lo, tc.hi); got != tc.want {
				t.Errorf("measured %v, want %v", got, tc.want)
			}
		})
	}
}

// insideRounded reports whether (x, y) lies in r with every corner
// rounded by radius.
func insideRounded(r render.Rect, radius int, x, y float64) bool {
	left, top := float64(r.X), float64(r.Y)
	right, bottom := left+float64(r.W), top+float64(r.H)
	if x < left || x > right || y < top || y > bottom {
		return false
	}
	rad := float64(radius)
	cx := min(max(x, left+rad), right-rad)
	cy := min(max(y, top+rad), bottom-rad)
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= rad*rad
}

// The grip paints inside the panel's border and its rounded corner:
// pinned to the outer corner, it drew on and past the border.
func TestFloatingPanelGripInsideRoundedBorder(t *testing.T) {
	const border = 6
	for _, radius := range []int{4, 40} {
		t.Run(strconv.Itoa(radius), func(t *testing.T) {
			f := NewFloatingPanel(nil, NewSpacer(200, 100))
			root := NewBox(Column, 0, 0)
			root.Append(f, true)
			root.AttachStylesheet(NewStylesheet(fmt.Sprintf("floatingpanel > panel { border: %dpx solid red; border-radius: %dpx; }", border, radius), StylePriorityUser))
			arrangeTree(t, root, 800, 600)
			const w, h = 800, 600
			data := make([]byte, render.Stride(w)*h)
			f.grip.Paint(render.New(data, render.Stride(w), w, h))
			inner := render.UniformInsets(border).Shrink(f.PanelRect())
			painted := 0
			for y := range h {
				for x := range w {
					if data[y*render.Stride(w)+x*4+3] == 0 {
						continue
					}
					painted++
					fx, fy := float64(x), float64(y)
					for _, c := range [][2]float64{{fx, fy}, {fx + 1, fy}, {fx, fy + 1}, {fx + 1, fy + 1}} {
						if !insideRounded(inner, radius-border, c[0], c[1]) {
							t.Fatalf("grip pixel (%d,%d) outside the panel's inner shape %v rounded %d", x, y, inner, radius-border)
						}
					}
				}
			}
			if painted == 0 {
				t.Fatal("the grip painted nothing")
			}
		})
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

// The window finds a panel's input region through the wrappers around
// it: a Revealer hid it, so the overlay swallowed every click.
func TestInputRegionOfLooksThroughWrappers(t *testing.T) {
	f, _, _ := newTestPanel(t)
	rev := NewRevealer(f)
	rev.SetRevealed(true)
	root := NewBox(Column, 0, 0)
	root.Append(rev, true)
	arrangeTree(t, root, 800, 600)
	if reg, ok := InputRegionOf(root); !ok || len(reg) != 1 || reg[0] != f.rect {
		t.Errorf("region %v %v through a box and a revealer, want the panel's %v", reg, ok, f.rect)
	}
	two := NewBox(Column, 0, 0)
	two.Append(NewSpacer(1, 1), false)
	two.Append(NewSpacer(1, 1), false)
	if _, ok := InputRegionOf(two); ok {
		t.Error("a region found under a container of two")
	}
}

// dropTitle is a header label taking file drops.
type dropTitle struct {
	*Label
	dropped []string
}

func (d *dropTitle) DragEnter(mimes []string, _ Point) string {
	if len(mimes) > 0 && mimes[0] == "text/uri-list" {
		return mimes[0]
	}
	return ""
}

func (d *dropTitle) Drop(_ string, data []byte, _ Point) { d.dropped = append(d.dropped, string(data)) }

func (d *dropTitle) HitTest(p Point) Widget {
	if d.Label.HitTest(p) != nil {
		return d
	}
	return nil
}

// A drag over the header reaches the drop target in it: the handle is
// the hit leaf, and it swallowed every drag.
func TestFloatingPanelHeaderTakesDrops(t *testing.T) {
	title := &dropTitle{Label: NewLabel(goldenFace(t), 14, "Open File", 0)}
	header := NewBox(Row, 0, 8)
	header.Append(title, true)
	f := NewFloatingPanel(header, NewSpacer(200, 100))
	arrangeTree(t, f, 800, 600)
	r := &Router{Root: f}
	b := title.Bounds()
	at := Point{X: b.X + 4, Y: b.Y + b.H/2}
	if got := r.DragEnter([]string{"text/uri-list"}, at); got != "text/uri-list" {
		t.Fatalf("drag over the title accepted %q", got)
	}
	r.Drop("text/uri-list", []byte("file:///tmp/a"), at)
	if len(title.dropped) != 1 {
		t.Errorf("the title got %v", title.dropped)
	}
}
