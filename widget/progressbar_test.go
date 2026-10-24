package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// The trough and fill style as `progressbar > trough > progress`: the
// trough's min sizes size the bar and its background paints under the
// fill; the progress node paints the filled fraction.
func TestProgressBarNodes(t *testing.T) {
	loadCSS(t, `progressbar trough { min-height: 8; background-color: #010203; } progressbar trough progress { background-color: #0a0b0c; }`)
	p := NewProgressBar(0.5)
	host := NewBox(Column, 0, 0)
	host.Append(p, false)
	frame(t, host, 200, 40)
	if got := p.Measure(Constraints{Max: Size{W: 200, H: 40}}); got.H != 8 {
		t.Errorf("bar height %d, want the trough's min-height 8", got.H)
	}
	tv := p.trough.style(&p.trough)
	if got := tv.Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("trough background %v, want the trough rule", got)
	}
	pv := p.trough.fill.style(&p.trough.fill)
	if got := pv.Background; got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("progress background %v, want the progress rule", got)
	}
	if fr := p.trough.fill.Bounds(); fr.W*2 != p.trough.Bounds().W || fr.H != 8 {
		t.Errorf("fill rect %+v, want half the trough wide at min-height 8", fr)
	}
}

// Unstyled, the bar is the theme's: a 160x10 surface trough with the
// accent fill, unchanged.
func TestProgressBarUnstyledDefaults(t *testing.T) {
	p := NewProgressBar(0.5)
	arrangeTree(t, p, 200, 40)
	if got := p.Measure(Constraints{Max: Size{W: 200, H: 40}}); got.W != 160 || got.H != 10 {
		t.Errorf("bar %v, want the 160x10 default", got)
	}
	th := Current()
	if got := p.troughFill(th.Accent); got != progressTrack(th.Accent) {
		t.Errorf("trough fallback %v, want the faded accent", got)
	}
	if got := p.fillFill(th); got != th.Accent {
		t.Errorf("progress fallback %v, want the theme accent", got)
	}
	p.Fill, p.Trough = render.RGB(1, 2, 3), render.RGB(4, 5, 6)
	if got := p.fillFill(th); got != render.RGB(1, 2, 3) {
		t.Errorf("programmatic fill %v, want the set color", got)
	}
	if got := p.troughFill(th.Accent); got != render.RGB(4, 5, 6) {
		t.Errorf("programmatic trough %v, want the set color", got)
	}
}

// The trough's min-height sizes the bar from the first layout and after
// an ancestor's class change, and a bar arranged taller than its trough
// paints only the trough's band, centered (#260).
func TestProgressBarTroughHeight(t *testing.T) {
	if got := NewProgressBar(0.5).Measure(Constraints{Max: Size{W: 400, H: 400}}); got != (Size{W: 160, H: 10}) {
		t.Errorf("unstyled bar %v, want the 160x10 default", got)
	}
	loadCSS(t, `progressbar.x trough { min-height: 30px; } .osd trough { min-height: 8px; }`)
	styled := NewProgressBar(0.5)
	styled.AddClass("x")
	if got := MeasureAsChild(styled, Constraints{Max: Size{W: 400, H: 400}}); got.H != 30 {
		t.Errorf("30px trough rule measures %v", got)
	}

	bar := NewProgressBar(1)
	host := NewBox(Column, 0, 0)
	host.Append(bar, false)
	arrangeTree(t, host, 160, 40)
	host.AddClass("osd")
	arrangeTree(t, host, 160, 40)
	if got := bar.Bounds().H; got != 8 {
		t.Fatalf("after the ancestor's class the bar is %d tall, want the 8px trough", got)
	}
	bar.Arrange(render.Rect{W: 160, H: 40})
	buf := make([]byte, render.Stride(160)*40)
	bar.Paint(render.New(buf, render.Stride(160), 160, 40))
	painted := func(y int) bool { return render.ColorFromBytes(buf[y*render.Stride(160)+80*4:]).A() > 0 }
	if painted(15) || !painted(16) || !painted(23) || painted(24) {
		t.Errorf("a 40px bar painted rows 15/16/23/24 = %v/%v/%v/%v, want only the centered 8px band 16..23", painted(15), painted(16), painted(23), painted(24))
	}
}
