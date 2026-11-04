package widget

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

type countLeaf struct {
	node
	paints *int
}

func (c *countLeaf) ownInk() (Widget, render.Insets, bool) { return c, render.Insets{}, true }
func (c *countLeaf) Measure(Axis, int) Sizes               { return FixedSizes(20) }
func (c *countLeaf) Paint(*render.Canvas)                  { *c.paints++ }
func (c *countLeaf) HitTest(Point) Widget                  { return nil }

type wildLeaf struct {
	Base
	paints *int
}

func (w *wildLeaf) Measure(Axis, int) Sizes { return FixedSizes(20) }
func (w *wildLeaf) Paint(*render.Canvas)    { *w.paints++ }
func (w *wildLeaf) HitTest(Point) Widget    { return nil }

func paintWithin(root Widget, w, h int, clip render.Rect) []byte {
	buf := make([]byte, render.Stride(w)*h)
	cv := render.New(buf, render.Stride(w), w, h)
	prev := cv.PushClipDevice(clip)
	cv.BeginOverlays()
	root.Paint(cv)
	cv.FlushOverlays()
	cv.FinishFocus()
	cv.PopClip(prev)
	return buf
}

func TestPaintVisitsOnlyWhatTheClipTouches(t *testing.T) {
	col := NewBox(Column, 0, 0)
	paints := 0
	for range 1000 {
		col.Append(&countLeaf{paints: &paints}, false)
	}
	SizeWithin(col, Size{W: 200, H: 20000})
	col.Arrange(render.Rect{W: 200, H: 20000})
	CollectDamage(col)
	paintWithin(col, 200, 20000, render.Rect{W: 200, H: 20})
	if paints > 2 {
		t.Errorf("a clip over one of 1000 leaves made %d Paint calls, want the leaf and its neighbour within the focus ring's reach", paints)
	}
}

func TestPaintWalksIntoANestedBoxOnlyWhenItsInkTouches(t *testing.T) {
	root := NewBox(Column, 0, 0)
	paints := 0
	for range 10 {
		row := NewBox(Row, 0, 0)
		for range 10 {
			row.Append(&countLeaf{paints: &paints}, false)
		}
		root.Append(row, false)
	}
	SizeWithin(root, Size{W: 200, H: 200})
	root.Arrange(render.Rect{W: 200, H: 200})
	paintWithin(root, 200, 200, render.Rect{X: 90, Y: 90, W: 10, H: 10})
	if paints > 4 {
		t.Errorf("a clip inside one cell made %d Paint calls, want its neighbours at most", paints)
	}
}

func TestAWidgetOutsideTheKitAlwaysPaints(t *testing.T) {
	col := NewBox(Column, 0, 0)
	paints := 0
	far := NewBox(Column, 0, 0).Append(&wildLeaf{paints: &paints}, false)
	near := &countLeaf{paints: new(int)}
	col.Append(near, false)
	col.Append(NewSpacer(10, 500), false)
	col.Append(far, false)
	SizeWithin(col, Size{W: 200, H: 600})
	col.Arrange(render.Rect{W: 200, H: 600})
	paintWithin(col, 200, 600, render.Rect{W: 10, H: 10})
	if paints != 1 {
		t.Errorf("a Base widget far from the clip painted %d times, want 1: it may paint anywhere", paints)
	}
}

type shadowingBox struct {
	*Box
	paints *int
}

func (s *shadowingBox) Paint(*render.Canvas) { *s.paints++ }

func TestAnEmbeddingTypeIsNotTrustedWithItsFieldsInk(t *testing.T) {
	col := NewBox(Column, 0, 0)
	paints := 0
	col.Append(NewSpacer(10, 500), false)
	col.Append(&shadowingBox{Box: NewBox(Column, 0, 0), paints: &paints}, false)
	SizeWithin(col, Size{W: 200, H: 600})
	col.Arrange(render.Rect{W: 200, H: 600})
	paintWithin(col, 200, 600, render.Rect{W: 10, H: 10})
	if paints != 1 {
		t.Errorf("a type shadowing Box.Paint was skipped: %d paints", paints)
	}
}

func TestAShadowAddedByAStyleExtendsTheInk(t *testing.T) {
	loadCSS(t, `.lifted { box-shadow: 0 0 0 30px #ff0000; }`)
	paints := 0
	inner := NewBox(Column, 0, 0).Append(&countLeaf{paints: &paints}, false)
	col := NewBox(Column, 0, 0).Append(NewSpacer(10, 100), false).Append(inner, false)
	SizeWithin(col, Size{W: 200, H: 200})
	col.Arrange(render.Rect{W: 200, H: 200})
	col.Paint(render.New(make([]byte, render.Stride(200)*200), render.Stride(200), 200, 200))
	clip := render.Rect{W: 200, H: 90}
	inner.AddClass("lifted")
	col.Arrange(render.Rect{W: 200, H: 200})
	CollectDamage(col)
	got := paintWithin(col, 200, 200, clip)
	if got[80*render.Stride(200)+2] != 0xff {
		t.Errorf("pixel in the shadow is %d, want the shadow's red: the box was skipped", got[80*render.Stride(200)+2])
	}
}

func cullingTree(t *testing.T) (Widget, int, int) {
	t.Helper()
	loadCSS(t, `
		.shadow { box-shadow: 4px 6px 12px #80ff0000; }
		.outline { outline: 3px solid #00ff00; outline-offset: 2px; }
		.moved { transform: translate(30px, 40px) rotate(12deg); background-color: #4040ff; }
		.blurred { filter: blur(6px); background-color: #ff00ff; }
		.faded { opacity: 0.5; background-color: #00ffff; }
		.glass { backdrop-filter: blur(4px); background-color: #20ffffff; }
		.clipped { overflow: hidden; }
		.away { position: relative; left: 60px; top: 24px; background-color: #ffff00; }
		.loud { text-shadow: 3px 3px 2px #ff0000; }
		.before::before { content: "<>"; }
	`)
	face := entryFace(t)
	mk := func(class, text string) *Box {
		b := NewBox(Column, 2, 2)
		b.AddClass(class)
		b.Append(NewLabel(face, 14, text, 0), false)
		return b
	}
	root := NewBox(Column, 6, 6)
	for _, c := range []string{"shadow", "outline", "moved", "blurred", "faded", "glass", "clipped", "away"} {
		root.Append(mk(c, c+" box with text"), false)
	}
	squeezed := NewBox(Row, 0, 0)
	squeezed.SetSizeRequest(30, 20)
	long := NewLabel(face, 14, "a label much wider than the room it was given", 0)
	squeezed.Append(long, false)
	root.Append(squeezed, false)
	shadowed := NewLabel(face, 14, "shadowed", 0)
	shadowed.AddClass("loud")
	root.Append(shadowed, false)
	pseudo := NewLabel(face, 14, "pseudo", 0)
	pseudo.AddClass("before")
	root.Append(pseudo, false)
	root.Append(NewSpinner(24), false)
	root.Append(NewButton(NewLabel(face, 14, "button", 0), 4, 4), false)
	root.Append(NewSeparator(Horizontal), false)
	const w, h = 320, 520
	t.Cleanup(Host(root))
	SizeWithin(root, Size{W: w, H: h})
	root.Arrange(render.Rect{W: w, H: h})
	CollectDamage(root)
	return root, w, h
}

func assertCullMatchesFullPaint(t *testing.T, root Widget, w, h int) {
	t.Helper()
	paintWithin(root, w, h, render.Rect{W: w, H: h})
	root.Arrange(render.Rect{W: w, H: h})
	CollectDamage(root)
	regions := []render.Rect{{W: w, H: h}}
	for _, side := range []int{7, 23, 61} {
		for y := 0; y < h; y += side + side/2 {
			for x := 0; x < w; x += side + side/2 {
				regions = append(regions, render.Rect{X: x, Y: y, W: side, H: side})
			}
		}
	}
	for _, r := range regions {
		cullInk = false
		full := paintWithin(root, w, h, r)
		cullInk = true
		culled := paintWithin(root, w, h, r)
		if !bytes.Equal(full, culled) {
			t.Fatalf("%s", cullDiff(full, culled, w, h, r))
		}
	}
}

func cullDiff(full, culled []byte, w, h int, r render.Rect) string {
	stride := render.Stride(w)
	for y := range h {
		for x := range w {
			at := y*stride + x*4
			if !bytes.Equal(full[at:at+4], culled[at:at+4]) {
				return fmt.Sprintf("clip %+v: (%d,%d) is %v culled, %v in full", r, x, y, culled[at:at+4], full[at:at+4])
			}
		}
	}
	return "lengths differ"
}

func TestCulledPaintMatchesFullPaintOverEffects(t *testing.T) {
	root, w, h := cullingTree(t)
	assertCullMatchesFullPaint(t, root, w, h)
}

func TestCulledPaintMatchesFullPaintOverTheShowcase(t *testing.T) {
	show := buildShowcase(t)
	SizeWithin(show.root, Size{W: showW, H: showH})
	show.root.Arrange(render.Rect{W: showW, H: showH})
	CollectDamage(show.root)
	assertCullMatchesFullPaint(t, show.root, showW, showH)
}

func TestPlainLabelsLeaveTheirBoxBounded(t *testing.T) {
	face := entryFace(t)
	col := NewBox(Column, 4, 4)
	var labels []*Label
	for i := range 200 {
		l := NewLabel(face, 14, fmt.Sprintf("row %d", i), 0)
		labels = append(labels, l)
		col.Append(l, false)
	}
	SizeWithin(col, Size{W: 300, H: 5000})
	col.Arrange(render.Rect{W: 300, H: 5000})
	CollectDamage(col)
	for _, l := range labels {
		if !l.inkClosed {
			t.Fatalf("label %q is left unbounded with a plain single line", l.Text())
		}
	}
	if !col.inkClosed {
		t.Error("a box of plain labels is left unbounded")
	}
}
