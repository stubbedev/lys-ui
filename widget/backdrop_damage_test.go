package widget

import (
	"bytes"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A damage-restricted repaint under a backdrop filter paints exactly
// what a full repaint does: the filter reads the backdrop around the
// damage too, so damage reaching a backdrop-filtered box repaints the
// box and the filter's reach, never re-filtering pixels the previous
// frame already filtered.
func TestBackdropFilterUnderPartialDamage(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `.frost { background-color: rgba(255,255,255,0.3); backdrop-filter: blur(6px) saturate(1.5); border-radius: 12px; }`)
	const w, h = 220, 120
	build := func(text string) (Widget, *Label) {
		lbl := NewLabel(face, 14, text, render.RGB(0xe0, 0x55, 0x61))
		backdrop := NewBox(Column, 0, 8)
		backdrop.Append(lbl, false)
		card := NewBox(Column, 0, 0)
		card.AddClass("frost")
		return NewOverlay().Append(backdrop).AppendAligned(card, AlignFill, AlignFill), lbl
	}
	root, lbl := build("before")
	got := make([]byte, render.Stride(w)*h)
	cv := render.New(got, render.Stride(w), w, h)
	paintFrame(cv, root, w, h, Current().Bg, render.Rect{W: w, H: h})
	lbl.SetText("after, longer")
	paintFrame(cv, root, w, h, Current().Bg, render.Rect{})

	fresh, _ := build("after, longer")
	want := make([]byte, render.Stride(w)*h)
	paintFrame(render.New(want, render.Stride(w), w, h), fresh, w, h, Current().Bg, render.Rect{W: w, H: h})
	if !bytes.Equal(got, want) {
		t.Error("the partial repaint under the glass differs from a full repaint")
	}
}
