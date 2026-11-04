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

type boxLayersLeaf struct{ node }

func (l *boxLayersLeaf) Measure(Axis, int) Sizes { return FixedSizes(30) }
func (l *boxLayersLeaf) Paint(cv *render.Canvas) { PaintBoxLayers(cv, l) }
func (l *boxLayersLeaf) HitTest(p Point) Widget  { return l.HitLeaf(l, p) }

func redPNG(t *testing.T) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 30, 30))
	for y := range 30 {
		for x := range 30 {
			img.Set(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestStylesheetImageNeverReadsOnTheLoop(t *testing.T) {
	for _, tc := range []struct {
		name, css string
		red       bool
	}{
		{"background", `.i { background-image: url("%s"); }`, true},
		{"border-image", `.i { border: 10px solid; border-image: url("%s") 10 fill; }`, true},
		{"mask", `.i { background-color: #00ff00; mask-image: url("%s"); }`, false},
		{"pseudo content", `.i::before { content: url("%s"); }`, true},
		{"pseudo background", `.i::before { content: ""; background-image: url("%s"); }`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeInvoker(t)
			path := filepath.Join(t.TempDir(), "held.png")
			feed := holdRead(t, path)
			addCSS(t, bytesReplaceOne(tc.css, path), 900)
			w := &boxLayersLeaf{}
			w.AddClass("i")
			SizeWithin(w, Size{W: 30, H: 30})
			w.Arrange(render.Rect{W: 30, H: 30})
			var first []byte
			onTime(t, "a restyle and paint", func() { first = paintTree(w, 30, 30) })
			CollectDamage(w)

			feed(redPNG(t))
			settleAssets(t, f)
			if _, any := CollectDamage(w); !any {
				t.Fatal("the decode's answer repainted nothing")
			}
			settled := paintTree(w, 30, 30)
			if bytes.Equal(first, settled) {
				t.Fatal("the settled frame shows nothing of the file")
			}
			if tc.red {
				if got := render.ColorFromBytes(settled[15*render.Stride(30)+15*4:]); got != render.RGB(255, 0, 0) {
					t.Errorf("center = %v, want the file's red", got)
				}
			}
		})
	}
}

func TestStylesheetImageDecodesOnceForEveryWidget(t *testing.T) {
	f := newFakeInvoker(t)
	path := filepath.Join(t.TempDir(), "shared.png")
	if err := os.WriteFile(path, []byte(redPNG(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	addCSS(t, bytesReplaceOne(`.i { background-image: url("%s"); }`, path), 900)
	var boxes []*Box
	for range 3 {
		b := NewBox(Row, 0, 0)
		b.Append(NewSpacer(10, 10), false)
		b.AddClass("i")
		SizeWithin(b, Size{W: 10, H: 10})
		b.Arrange(render.Rect{W: 10, H: 10})
		paintTree(b, 10, 10)
		CollectDamage(b)
		boxes = append(boxes, b)
	}
	settleAssets(t, f)
	for i, b := range boxes {
		if _, any := CollectDamage(b); !any {
			t.Errorf("box %d was not repainted by the one decode", i)
		}
	}
	if img := cssImages[path]; img == nil || !img.settled || img.icon == nil || len(img.waiters) != 0 {
		t.Errorf("after the decode: %+v", img)
	}
}

func TestStylesheetImageFailureSettlesAndWarns(t *testing.T) {
	f := newFakeInvoker(t)
	path := filepath.Join(t.TempDir(), "gone.png")
	addCSS(t, bytesReplaceOne(`.i { background-image: url("%s"); background-color: #0000ff; }`, path), 900)
	w := NewBox(Row, 0, 0)
	w.Append(NewSpacer(10, 10), false)
	w.AddClass("i")
	SizeWithin(w, Size{W: 10, H: 10})
	w.Arrange(render.Rect{W: 10, H: 10})
	paintTree(w, 10, 10)
	settleAssets(t, f)
	data := paintTree(w, 10, 10)
	if got := render.ColorFromBytes(data[5*render.Stride(10)+5*4:]); got != render.RGB(0, 0, 255) {
		t.Errorf("center = %v, want the background color alone", got)
	}
	if img := cssImages[path]; img == nil || !img.settled {
		t.Error("a file that does not load stays asked for")
	}
}

func bytesReplaceOne(format, path string) string {
	return string(bytes.Replace([]byte(format), []byte("%s"), []byte(path), 1))
}
