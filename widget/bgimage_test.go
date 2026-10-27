package widget

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// testPNG encodes a 4x3 opaque image of the given NRGBA color as PNG
// bytes.
func testPNG(t *testing.T, col color.NRGBA) []byte {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := range 3 {
		for x := range 4 {
			src.SetNRGBA(x, y, col)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeTemp writes data to a temp file and returns its path.
func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// paintPixels paints w at size and returns the pixel buffer.
func paintPixels(t *testing.T, w Widget, width, height int) []byte {
	t.Helper()
	for range 2 {
		SizeWithin(w, Size{W: width, H: height})
		w.Arrange(render.Rect{W: width, H: height})
	}
	data := make([]byte, render.Stride(width)*height)
	w.Paint(render.New(data, render.Stride(width), width, height))
	return data
}

// pxAt reads one logical pixel of a width-wide buffer.
func pxAt(data []byte, w, x, y int) render.Color {
	i := (y*w + x) * 4
	return render.ColorFromBytes(data[i:])
}

// isRed reports a mostly-red opaque pixel.
func isRed(c render.Color) bool {
	return c.A() > 128 && c.R() > 128 && c.G() < 64 && c.B() < 64
}

// A stylesheet url() paints the file's pixels inside the box — tiled
// at its natural size, CSS's initial background-size and -repeat — so
// the center of the box shows the image's color, and a failed path
// paints the background color alone.
func TestBackgroundImageURLPaints(t *testing.T) {
	path := writeTemp(t, "avatar.png", testPNG(t, color.NRGBA{R: 255, A: 255}))
	loadCSS(t, `box.photo { background-color: #0000ff; background-image: url("`+path+`"); }`)
	b := NewBox(Column, 0, 0)
	b.AddClass("photo")
	data := paintPixels(t, b, 40, 30)
	if c := pxAt(data, 40, 20, 15); !isRed(c) {
		t.Errorf("the box center is %#08x, want the image's red", uint32(c))
	}

	loadCSS(t, `box.photo { background-color: #0000ff; background-image: url("/nonexistent/bg.png"); }`)
	b2 := NewBox(Column, 0, 0)
	b2.AddClass("photo")
	data2 := paintPixels(t, b2, 40, 30)
	if c := pxAt(data2, 40, 20, 15); isRed(c) {
		t.Errorf("a missing file painted %#08x", uint32(c))
	}
}

// file:// URLs and bare paths parse alike; none clears; a gradient and
// a URL replace each other.
func TestBackgroundImageURLForms(t *testing.T) {
	path := writeTemp(t, "art.png", testPNG(t, color.NRGBA{G: 200, A: 255}))
	for _, decl := range []string{
		`background-image: url("` + path + `")`,
		`background-image: url("file://` + path + `")`,
	} {
		loadCSS(t, `box.photo { `+decl+`; }`)
		b := NewBox(Column, 0, 0)
		b.AddClass("photo")
		if got := b.style(b).Bg.Layer(0).Image.URL; got != path {
			t.Errorf("%s parsed to %q", decl, got)
		}
	}
	loadCSS(t, `box.photo { background-image: linear-gradient(to bottom, #010203, #040506); }`)
	b := NewBox(Column, 0, 0)
	b.AddClass("photo")
	if img := b.style(b).Bg.Layer(0).Image; img.URL != "" || img.Gradient == nil {
		t.Errorf("a gradient left url %q", img.URL)
	}
	loadCSS(t, `box.photo { background-image: none; }`)
	b2 := NewBox(Column, 0, 0)
	b2.AddClass("photo")
	v := b2.style(b2)
	if v.Bg.Painted() {
		t.Error("none cleared nothing")
	}
}
