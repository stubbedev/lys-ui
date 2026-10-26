package widget

import (
	"image"
	"image/color"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func solidImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestImageScaleDownKeepsASmallImageNatural(t *testing.T) {
	resetImageCache(t)
	src := rampImage(2, 2)
	im := NewImage(src)
	im.SetScale(ImageScaleDown)
	_, missesBefore := cacheStats()
	data, dev := paintImage(im, 6, 6, 1, 1)
	// Centered at 2,2 one-to-one; the rest of the box is untouched.
	for y := range 2 {
		for x := range 2 {
			c := src.NRGBAAt(x, y)
			if got := px(data, dev, 2+x, 2+y); got != render.RGBA(c.R, c.G, c.B, c.A) {
				t.Errorf("pixel %d,%d = %v, want source %v", 2+x, 2+y, got, c)
			}
		}
	}
	if got := px(data, dev, 0, 0); got != render.RGB(0, 0, 0) {
		t.Errorf("corner = %v, the small image was enlarged", got)
	}
	if _, misses := cacheStats(); misses != missesBefore {
		t.Error("a natural-size paint went through the resample cache")
	}
}

func TestImageScaleDownShrinksALargeImage(t *testing.T) {
	resetImageCache(t)
	im := NewImage(solidImage(16, 8, color.NRGBA{R: 0x20, G: 0x90, B: 0x40, A: 0xff}))
	im.SetScale(ImageScaleDown)
	data, dev := paintImage(im, 4, 4, 1, 1)
	// 16x8 into 4x4 fits to 4x2 centered: rows 1-2 painted, 0 and 3 not.
	if got := px(data, dev, 0, 1); got != render.RGB(0x20, 0x90, 0x40) {
		t.Errorf("fitted row = %v", got)
	}
	if got := px(data, dev, 0, 0); got != render.RGB(0, 0, 0) {
		t.Errorf("letterbox row = %v, want untouched", got)
	}
}

func TestImageStretchFillsTheBox(t *testing.T) {
	resetImageCache(t)
	im := NewImage(solidImage(8, 2, color.NRGBA{R: 0x80, G: 0x10, B: 0xc0, A: 0xff}))
	im.SetScale(ImageStretch)
	data, dev := paintImage(im, 4, 4, 1, 1)
	for _, at := range [][2]int{{0, 0}, {3, 0}, {0, 3}, {3, 3}} {
		if got := px(data, dev, at[0], at[1]); got != render.RGB(0x80, 0x10, 0xc0) {
			t.Errorf("pixel %v = %v (stretch must cover the box)", at, got)
		}
	}
}
