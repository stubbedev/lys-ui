package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// The fill and trough colors paint when set and fall back to the
// theme's accent and the faded accent track when zero.
func TestProgressBarColors(t *testing.T) {
	const w, h = 20, 4
	paint := func(p *ProgressBar) []byte {
		SizeWithin(p, Size{W: w, H: h})
		p.Arrange(render.Rect{W: w, H: h})
		buf := make([]byte, render.Stride(w)*h)
		p.Paint(render.New(buf, render.Stride(w), w, h))
		return buf
	}
	at := func(buf []byte, x int) render.Color { return render.ColorFromBytes(buf[2*render.Stride(w)+x*4:]) }
	red, blue := render.RGB(255, 0, 0), render.RGB(0, 0, 255)
	p := NewProgressBar(0.5)
	p.Fill, p.Trough = red, blue
	buf := paint(p)
	if at(buf, 5) != red || at(buf, 15) != blue {
		t.Errorf("fill %#x trough %#x, want the set colors", uint32(at(buf, 5)), uint32(at(buf, 15)))
	}
	theme := Current()
	buf = paint(NewProgressBar(0.5))
	if at(buf, 5) != theme.Accent || at(buf, 15) != progressTrack(theme.Accent) {
		t.Error("unset colors did not fall back to the theme")
	}
	loadCSS(t, `progressbar { accent-color: #ff0000; }`)
	buf = paint(NewProgressBar(0.5))
	if at(buf, 5) != red || at(buf, 15) != progressTrack(red) {
		t.Errorf("accent-color fill %#x trough %#x, want the accent and its faded track", uint32(at(buf, 5)), uint32(at(buf, 15)))
	}
}
