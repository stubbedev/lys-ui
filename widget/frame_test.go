package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestFrameLabel pins the label row: present when titled, dropped on
// "", the child always inside the outline.
func TestFrameLabel(t *testing.T) {
	face := chromeFace(t)
	child := NewSpacer(40, 20)
	f := NewFrame(face, "Options", child)
	if n := len(f.column.Children()); n != 2 {
		t.Fatalf("titled frame rows = %d", n)
	}
	f.SetLabel("")
	if n := len(f.column.Children()); n != 1 || f.column.Children()[0] != Widget(f.border) {
		t.Errorf("untitled frame rows = %d", n)
	}
	sz := f.Measure(Constraints{Max: Size{W: 200, H: 200}})
	if sz.W != 40+2*framePad || sz.H != 20+2*framePad {
		t.Errorf("untitled size = %v, want the child plus padding", sz)
	}
}

// TestAspectFrame pins the fit: the child gets the largest centered
// rect of the ratio, and ratio zero follows the child's own.
func TestAspectFrame(t *testing.T) {
	child := NewSpacer(10, 10)
	a := NewAspectFrame(child, 2)
	if sz := a.Measure(Constraints{Max: Size{W: 500, H: 500}}); sz != (Size{W: 20, H: 10}) {
		t.Errorf("natural = %v, want 20x10", sz)
	}
	a.Arrange(render.Rect{W: 300, H: 100})
	if b := child.Bounds(); b != (render.Rect{X: 50, W: 200, H: 100}) {
		t.Errorf("wide offer: child = %+v", b)
	}
	a.Arrange(render.Rect{W: 100, H: 300})
	if b := child.Bounds(); b != (render.Rect{Y: 125, W: 100, H: 50}) {
		t.Errorf("tall offer: child = %+v", b)
	}
	a.SetRatio(0)
	a.Arrange(render.Rect{W: 100, H: 300})
	if b := child.Bounds(); b.W != 100 || b.H != 100 {
		t.Errorf("child ratio: child = %+v", b)
	}
}

// TestSizeGroup pins the equalization across containers: both members
// measure the wider natural width, and a member's change relayouts its
// peer's wrapper.
func TestSizeGroup(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	short, long := NewLabel(face, 14, "ab", th.Text), NewLabel(face, 14, "a much longer label", th.Text)
	g := NewSizeGroup(SizeGroupHorizontal)
	ws, wl := g.Add(short), g.Add(long)
	r1, r2 := NewBox(Row, 0, 0), NewBox(Row, 0, 0)
	r1.Append(ws, false)
	r2.Append(wl, false)
	col := NewBox(Column, 0, 0)
	col.Append(r1, false)
	col.Append(r2, false)
	con := Constraints{Max: Size{W: 400, H: 400}}
	col.Measure(con)
	col.Arrange(render.Rect{W: 400, H: 400})
	a, b := ws.(Boundser).Bounds(), wl.(Boundser).Bounds()
	if a.W != b.W || a.W != long.Measure(con).W {
		t.Errorf("widths %d / %d, want both the long label's", a.W, b.W)
	}
	if ha := short.Measure(con).H; a.H != ha {
		t.Errorf("horizontal group changed the height: %d vs %d", a.H, ha)
	}
	long.SetText("x")
	if !nodeOf(ws).measureDirty {
		t.Error("a member's change left its peer's measure cached")
	}
	col.Measure(con)
	col.Arrange(render.Rect{W: 400, H: 400})
	if a, b := ws.(Boundser).Bounds(), wl.(Boundser).Bounds(); a.W != b.W || a.W != short.Measure(con).W {
		t.Errorf("after shrink: widths %d / %d", a.W, b.W)
	}
}

// TestGoldenFrame pins the outline with its title.
func TestGoldenFrame(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	f := NewFrame(face, "Network", NewLabel(face, 14, "Wired connected", th.Text))
	NewGolden(t, f, "frame", goldenTheme(th), goldenFrame(200, 90))
}

// The frame measures its child through its layout properties: a child
// with a width request and no content of its own still sets the
// frame's width and so its height.
func TestAspectFrameHonorsTheChildSizeRequest(t *testing.T) {
	inner := NewBox(Column, 0, 0)
	inner.SetSizeRequest(360, 0)
	a := NewAspectFrame(inner, 16.0/9)
	host := NewBox(Column, 0, 0)
	host.AppendAligned(a, false, AlignStart)
	arrangeTree(t, host, 600, 600)
	if got := inner.Bounds(); got.W != 360 || got.H != 203 {
		t.Errorf("child %v, want 360x203 from its width request", got)
	}
}
