package widget

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// animatedGIF encodes a 3-frame 8x8 GIF, 100ms per frame, playing
// loops times (0: forever).
func animatedGIF(t *testing.T, loops int) []byte {
	pal := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}}
	// GIF's loop count: 0 forever, -1 once, n: n more times.
	count := loops - 1
	switch loops {
	case 0:
		count = 0
	case 1:
		count = -1
	}
	g := &gif.GIF{LoopCount: count}
	for i := range 3 {
		p := image.NewPaletted(image.Rect(0, 0, 8, 8), pal)
		for k := range p.Pix {
			p.Pix[k] = uint8(i)
		}
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestImagePlayback pins the frame timeline: frames advance on their
// deadlines only while the image is on screen, a finite animation
// rests on its last frame, and reduced motion shows the first frame.
func TestImagePlayback(t *testing.T) {
	c := pinAnimClock(t)
	im := NewBytesImage(animatedGIF(t, 0), "anim.gif")
	if _, ok := im.img.(*render.Animation); !ok {
		t.Fatalf("source %T is not an animation", im.img)
	}
	c.step()
	if im.Frame() != 0 {
		t.Error("an unarranged image advanced")
	}
	im.Arrange(render.Rect{W: 8, H: 8})
	im.invalid = false
	c.step()
	if im.Frame() != 1 || !im.invalid {
		t.Errorf("after one deadline: frame %d, invalidated %v", im.Frame(), im.invalid)
	}
	c.step()
	c.step()
	if im.Frame() != 0 {
		t.Errorf("an endless animation did not wrap: frame %d", im.Frame())
	}
	im.Arrange(render.Rect{})
	if c.step() {
		t.Error("a hidden image kept a deadline")
	}

	once := NewBytesImage(animatedGIF(t, 1), "once.gif")
	once.Arrange(render.Rect{W: 8, H: 8})
	c.drive()
	if once.Frame() != 2 {
		t.Errorf("a one-shot animation rests on frame %d, want the last", once.Frame())
	}

	t.Cleanup(anim.SetInstant(true))
	still := NewBytesImage(animatedGIF(t, 0), "still.gif")
	still.Arrange(render.Rect{W: 8, H: 8})
	if c.step() || still.Frame() != 0 {
		t.Error("reduced motion animated")
	}
}

// TestGoldenDecodedImages pins a still WebP beside an animated GIF's
// first frame (goldens run under reduced motion).
func TestGoldenDecodedImages(t *testing.T) {
	webp, err := os.ReadFile("testdata/images/blue-purple-pink.lossy.webp")
	if err != nil {
		t.Fatal(err)
	}
	row := NewBox(Row, 8, 0)
	w := NewBytesImage(webp, "webp")
	g := NewBytesImage(animatedGIF(t, 0), "gif")
	for _, im := range []*Image{w, g} {
		if im.Err() != nil {
			t.Fatal(im.Err())
		}
		row.Append(newSizedCell(NewAspectFrame(im, 1), 64), false)
	}
	NewGolden(t, row, "decoded-images", goldenTheme(DarkTheme()), goldenFrame(140, 64))
}
