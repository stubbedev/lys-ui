package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

func testBar(t *testing.T, edge BarEdge) (*Bar, *BarButton, *BarSeparator, *SegmentIndicator) {
	t.Helper()
	face := goldenFace(t)
	b := NewBar(edge)
	segs := NewSegmentIndicator(5)
	segs.SetActive(1)
	segs.SetSegmentState(2, SegmentOccupied)
	segs.SetSegmentState(4, SegmentUrgent)
	b.Start().AppendAligned(segs, false, AlignCenter)
	b.Center().Append(NewLabel(face, 14, "12:45", 0), false)
	sep := NewBarSeparator()
	vol := NewBarButton(face, NewSymbol(SymbolCheck, 16), "Volume 40%")
	vol.SetBadge("2")
	b.End().Append(sep, false)
	b.End().Append(vol, false)
	return b, vol, sep, segs
}

// Along a side edge the bar is a column: its buttons show their icons
// alone, its separators run across it, its segments stack; back on
// the top they turn back.
func TestBarFollowsItsEdge(t *testing.T) {
	b, vol, sep, segs := testBar(t, BarTop)
	arrangeTree(t, b, 800, 60)
	if !IsVisible(vol.label) || sep.Orientation() != Vertical || segs.box.Axis() != Row || !b.HasClass("top") {
		t.Fatal("a top bar does not lay out as a row with labelled buttons and upright separators")
	}
	b.SetEdge(BarLeft)
	arrangeTree(t, b, 60, 800)
	arrangeTree(t, b, 60, 800)
	if IsVisible(vol.label) || !vol.HasClass("vertical") || sep.Orientation() != Horizontal || segs.box.Axis() != Column {
		t.Error("a left bar keeps labels, upright separators or a row of segments")
	}
	if !b.HasClass("left") || !b.HasClass("vertical") || b.HasClass("top") || b.start.Axis() != Column {
		t.Errorf("left bar classes or zone axis wrong")
	}
	if got := vol.Bounds(); got.W > 60 {
		t.Errorf("the icon-only button is %d wide in a 60 px bar", got.W)
	}
	b.SetEdge(BarBottom)
	arrangeTree(t, b, 800, 60)
	arrangeTree(t, b, 800, 60)
	if !IsVisible(vol.label) || sep.Orientation() != Vertical {
		t.Error("back on a horizontal edge the label or separator did not return")
	}
}

// A bar button's popover opens away from the bar's edge, the button
// checked while it is open; a second click closes it.
func TestBarButtonPopover(t *testing.T) {
	for _, tc := range []struct {
		edge BarEdge
		side PopoverSide
	}{{BarTop, PopoverBelow}, {BarBottom, PopoverAbove}, {BarLeft, PopoverRight}, {BarRight, PopoverLeft}} {
		b, vol, _, _ := testBar(t, tc.edge)
		host := &fakeSurfaceHost{}
		SetSurfaceHost(b, host)
		arrangeTree(t, b, 800, 800)
		vol.SetPopover(func() Widget { return NewSpacer(100, 100) })
		vol.OnClick()
		if len(host.opts) != 1 || host.opts[0].Side != tc.side || !vol.HasState(StateChecked) {
			t.Fatalf("edge %d: popovers %+v, checked %v; want one on side %d, checked", tc.edge, host.opts, vol.HasState(StateChecked), tc.side)
		}
		vol.OnClick()
		host.opts[0].OnClosed()
		if host.closed != 1 || vol.HasState(StateChecked) {
			t.Errorf("edge %d: a second click closed %d popovers, checked %v", tc.edge, host.closed, vol.HasState(StateChecked))
		}
	}
}

// Segments carry the active, occupied and urgent classes, and a click
// reports its index.
func TestSegmentIndicator(t *testing.T) {
	_, _, _, segs := testBar(t, BarTop)
	var picked []int
	segs.OnSelect = func(i int) { picked = append(picked, i) }
	segs.segs[3].OnClick()
	if len(picked) != 1 || picked[0] != 3 {
		t.Errorf("picked %v, want 3", picked)
	}
	if !segs.segs[1].HasClass(segmentActive) || !segs.segs[2].HasClass("occupied") || !segs.segs[4].HasClass("urgent") {
		t.Error("segments lack their state classes")
	}
	segs.SetActive(3)
	segs.SetSegmentState(4, SegmentEmpty)
	segs.SetCount(4)
	if segs.segs[1].HasClass(segmentActive) || !segs.segs[3].HasClass(segmentActive) || segs.Count() != 4 || segs.Active() != 3 {
		t.Error("moving the active segment or trimming the count left stale state")
	}
	segs.SetCount(2)
	if segs.Active() != -1 {
		t.Errorf("active %d after its segment went", segs.Active())
	}
}

// TestGoldenBar pins a top bar and a left bar in both palettes.
func TestGoldenBar(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			top, _, _, _ := testBar(t, BarTop)
			left, _, _, _ := testBar(t, BarLeft)
			left.SetSizeRequest(0, 260)
			row := NewBox(Row, 16, 8)
			row.AppendAligned(left, false, AlignStart)
			row.AppendAligned(top, true, AlignStart)
			NewGolden(t, row, "bar-"+tc.name, goldenTheme(tc.th), goldenFrame(520, 280), goldenAfterArrange(func() {
				row.Measure(Constraints{Max: Size{W: 520, H: 280}})
				row.Arrange(render.Rect{W: 520, H: 280})
			}))
		})
	}
}

// A bar button's icon goes either side of its label, the gap between
// them is the stylesheet's to set either way, and its label is reachable.
func TestBarButtonIconPositionAndGap(t *testing.T) {
	face := goldenFace(t)
	icon := NewSymbol(SymbolCheck, 16)
	b := NewBarButton(face, icon, "42%")
	arrangeTree(t, NewBox(Row, 0, 0).Append(b, false), 300, 40)
	if gap := b.label.Bounds().X - (icon.Bounds().X + icon.Bounds().W); gap != 6 {
		t.Errorf("theme gap %d, want 6", gap)
	}
	b.SetIconPosition(IconEnd)
	loadCSS(t, `button.bar-button > box { border-spacing: 2px; }`)
	arrangeTree(t, NewBox(Row, 0, 0).Append(b, false), 300, 40)
	if gap := icon.Bounds().X - (b.label.Bounds().X + b.label.Bounds().W); gap != 2 {
		t.Errorf("icon at end %v after label %v, gap %d; want the stylesheet's 2", icon.Bounds(), b.label.Bounds(), gap)
	}
	if b.Label() != b.label || b.Label().Text() != "42%" {
		t.Error("the label is not reachable")
	}
}

// A bar separator takes its color, thickness and length from CSS.
func TestBarSeparatorStylesFromCSS(t *testing.T) {
	b, _, sep, _ := testBar(t, BarTop)
	loadCSS(t, `separator.bar-separator { color: #ff0000; min-width: 3px; min-height: 10px; }`)
	arrangeTree(t, b, 800, 60)
	arrangeTree(t, b, 800, 60)
	sb := sep.Bounds()
	if sb.W != 3 {
		t.Errorf("separator %v, want 3 px thick", sb)
	}
	const w, h = 800, 60
	data := make([]byte, render.Stride(w)*h)
	sep.Paint(render.New(data, render.Stride(w), w, h))
	at := func(x, y int) render.Color { return render.ColorFromBytes(data[y*render.Stride(w)+x*4:]) }
	cx, cy := sb.X+1, sb.Y+sb.H/2
	if c := at(cx, cy); c.R() != 0xff || c.G() != 0 {
		t.Errorf("rule painted %v, want the CSS red", c)
	}
	if at(cx, cy-8).A() != 0 || at(cx, cy+8).A() != 0 {
		t.Error("the rule is longer than its 10 px")
	}
}
