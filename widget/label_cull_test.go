package widget

import (
	"bytes"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestALabelFarFromTheClipPaintsNothing(t *testing.T) {
	face := goldenFace(t)
	label := NewLabel(face, 14, "far away", 0)
	SizeWithin(label, Size{W: 200, H: 40})
	label.Arrange(render.Rect{W: 200, H: 40})
	cv := render.New(make([]byte, render.Stride(400)*400), render.Stride(400), 400, 400)
	prev := cv.PushClip(render.Rect{X: 300, Y: 300, W: 50, H: 50})
	defer cv.PopClip(prev)
	if !label.outsideClip(cv) {
		t.Error("a label far outside the clip was not culled")
	}
	cv.PopClip(prev)
	cv.PushClip(render.Rect{X: 201, Y: 0, W: 10, H: 10})
	if label.outsideClip(cv) {
		t.Error("a clip within the label's ink reach culled it")
	}
}

func TestPartialRepaintMatchesAFullPaintAroundLabels(t *testing.T) {
	face := goldenFace(t)
	const w, h = 300, 200
	for _, tc := range []struct {
		name, css, text string
		width           int
	}{
		{"plain", "", "plain text", 200},
		{"overflowing", "", "a line far wider than the box it was given to draw in", 40},
		{"shadow", "label { text-shadow: 60px 60px 0 red; }", "shadow", 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loadCSS(t, tc.css)
			label := NewLabel(face, 14, tc.text, Current().Text)
			paint := func(cv *render.Canvas, clip render.Rect) {
				prev := cv.PushClip(clip)
				cv.Clear(clip, Current().Bg)
				label.Paint(cv)
				cv.PopClip(prev)
			}
			full := render.Rect{W: w, H: h}
			SizeWithin(label, Size{W: w, H: h})
			label.Arrange(render.Rect{X: 20, Y: 40, W: tc.width, H: 20})
			want := make([]byte, render.Stride(w)*h)
			paint(render.New(want, render.Stride(w), w, h), full)
			got := bytes.Clone(want)
			cv := render.New(got, render.Stride(w), w, h)
			for y := 0; y < h; y += 16 {
				for x := 0; x < w; x += 16 {
					paint(cv, render.Rect{X: x, Y: y, W: 16, H: 16})
				}
			}
			if !bytes.Equal(got, want) {
				t.Error("tile-by-tile repaint differs from one full paint")
			}
		})
	}
}

func TestCombiningMarksKeepALabelFromBeingCulled(t *testing.T) {
	label := NewLabel(goldenFace(t), 14, "Z͑͑͑͑a", 0)
	SizeWithin(label, Size{W: 200, H: 40})
	label.Arrange(render.Rect{W: 200, H: 40})
	cv := render.New(make([]byte, render.Stride(400)*400), render.Stride(400), 400, 400)
	prev := cv.PushClip(render.Rect{X: 300, Y: 300, W: 50, H: 50})
	defer cv.PopClip(prev)
	if label.outsideClip(cv) {
		t.Error("a stack of combining marks can reach past any fixed margin, yet it was culled")
	}
}
