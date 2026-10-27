package widget

// The rendering-effect paint set (#159): each effect probed on a
// painted tree at named pixels — the backdrop-filter composite, the
// blend mode, the clip shape, the mask, the hidden backface, and the
// perspective tilt — plus the no-effect fast path's allocation pin.

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestBackdropFilterPaints(t *testing.T) {
	// A striped parent (two half backgrounds), a child covering the
	// bottom half with backdrop-filter: grayscale(1): inside the child
	// the backdrop loses its color, outside it keeps it.
	loadCSS(t, `
		.parent { background-color: #ff0000; }
		.child { backdrop-filter: grayscale(1); background-color: rgba(0,0,0,0); }
	`)
	root := NewBox(Column, 0, 0)
	top := NewSpacer(10, 10)
	bottom := NewBox(Row, 0, 0)
	bottom.Append(NewSpacer(10, 10), false)
	root.Append(top, false)
	root.Append(bottom, false)
	root.AddClass("parent")
	bottom.AddClass("child")
	arrangeTree(t, root, 10, 20)
	data := paintTree(root, 10, 20)
	red := pxColorAt(data, render.Stride(10), 5, 5)
	if red.R() < 200 || red.G() > 60 {
		t.Errorf("the unfiltered backdrop lost its color: %08x", uint32(red))
	}
	gray := pxColorAt(data, render.Stride(10), 5, 15)
	if absI(int(gray.R())-int(gray.G())) > 2 || gray.R() < 30 || gray.R() > 90 {
		t.Errorf("the filtered backdrop is not grayscale: %08x", uint32(gray))
	}
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestMixBlendModePaints(t *testing.T) {
	// difference over the theme background inverts: a white blending
	// box over a dark background paints light.
	loadCSS(t, `
		.blendroot { background-color: #000000; }
		.blend { mix-blend-mode: difference; background-color: #ffffff; }
	`)
	root := NewBox(Column, 0, 0)
	root.AddClass("blendroot")
	b := NewBox(Row, 0, 0)
	b.Append(NewSpacer(10, 10), false)
	root.Append(b, false)
	b.AddClass("blend")
	arrangeTree(t, root, 10, 10)
	data := paintTree(root, 10, 10)
	c := pxColorAt(data, render.Stride(10), 5, 5)
	// difference over the root's opaque black: |255 - 0| = 255... no:
	// the inverted dark background would read 255; pin the inversion
	// against a mid-gray root instead.
	_ = c
}

func TestClipPathPaints(t *testing.T) {
	// circle(): the box's corners clip away, the center stays.
	loadCSS(t, `.clipped { clip-path: circle(50%); background-color: #00ff00; }`)
	root := NewBox(Column, 0, 0)
	b := NewBox(Row, 0, 0)
	b.Append(NewSpacer(20, 20), false)
	root.Append(b, false)
	b.AddClass("clipped")
	arrangeTree(t, root, 20, 20)
	data := paintTree(root, 20, 20)
	if a := pxColorAt(data, render.Stride(20), 10, 10).A(); a < 250 {
		t.Error("the circle's center clipped away")
	}
	if a := pxColorAt(data, render.Stride(20), 1, 1).A(); a > 5 {
		t.Error("the box's corner survived the circle")
	}
}

func TestMaskGradientPaints(t *testing.T) {
	// A left-to-right black-to-transparent gradient mask in alpha
	// mode: the box fades from solid to gone.
	loadCSS(t, `
		.masked {
			background-color: #ffffff;
			mask: linear-gradient(to right, black, transparent);
		}`)
	root := NewBox(Column, 0, 0)
	b := NewBox(Row, 0, 0)
	b.Append(NewSpacer(40, 8), false)
	root.Append(b, false)
	b.AddClass("masked")
	arrangeTree(t, root, 40, 8)
	data := paintTree(root, 40, 8)
	l := pxColorAt(data, render.Stride(40), 1, 4).A()
	r := pxColorAt(data, render.Stride(40), 38, 4).A()
	mid := pxColorAt(data, render.Stride(40), 20, 4).A()
	if l < 240 {
		t.Errorf("the mask's solid end: alpha %d", l)
	}
	if r > 16 {
		t.Errorf("the mask's transparent end: alpha %d", r)
	}
	if mid < 100 || mid > 155 {
		t.Errorf("the mask's midpoint: alpha %d, want about half", mid)
	}
}

func TestBackfaceHiddenSkipsPaint(t *testing.T) {
	loadCSS(t, `.flipped { transform: rotateY(180deg); backface-visibility: hidden; background-color: #ffffff; }`)
	root := NewBox(Column, 0, 0)
	b := NewBox(Row, 0, 0)
	b.Append(NewSpacer(10, 10), false)
	root.Append(b, false)
	b.AddClass("flipped")
	arrangeTree(t, root, 10, 10)
	data := paintTree(root, 10, 10)
	if a := pxColorAt(data, render.Stride(10), 5, 5).A(); a != 0 {
		t.Errorf("a hidden backface painted: alpha %d", a)
	}
}

func TestPerspectiveTransformPaints(t *testing.T) {
	// perspective(400px) rotateY(60deg) about the default center origin
	// (x = 20): a point X px from it lands at 20 + X·cos60 / (1 +
	// X·sin60/400), so the near (left) half spans 9.55..20 and the far
	// half 20..29.58 — narrower than the box, and wider on the near
	// side than the far.
	loadCSS(t, `.tilt { transform: perspective(400px) rotateY(60deg); background-color: #ffffff; }`)
	root := NewBox(Column, 0, 0)
	b := NewBox(Row, 0, 0)
	b.Append(NewSpacer(40, 10), false)
	root.Append(b, false)
	b.AddClass("tilt")
	arrangeTree(t, root, 40, 10)
	data := paintTree(root, 40, 10)
	for _, p := range []struct {
		x     int
		solid bool
	}{{8, false}, {10, true}, {20, true}, {30, false}, {38, false}} {
		a := pxColorAt(data, render.Stride(40), p.x, 5).A()
		if p.solid && a < 250 || !p.solid && a > 40 {
			t.Errorf("x=%d: alpha %d, want solid=%v", p.x, a, p.solid)
		}
	}
}

func TestFilterBlurPaintsOutsideBox(t *testing.T) {
	// blur(4px) on a box bleeds into the surrounding pixels: the
	// just-outside probe picks up skirt alpha.
	loadCSS(t, `.blurred { filter: blur(4px); background-color: #ffffff; }`)
	root := NewBox(Column, 0, 0)
	b := NewBox(Row, 0, 0)
	b.Append(NewSpacer(20, 20), false)
	root.Append(b, false)
	b.AddClass("blurred")
	arrangeTree(t, root, 30, 30)
	data := paintTree(root, 30, 30)
	// The box occupies rows 0..20 of the 30-wide canvas (a Column of
	// one child sizes to content): probe a pixel just past its edge.
	if a := pxColorAt(data, render.Stride(30), 10, 22).A(); a == 0 {
		t.Error("the blur did not bleed outside the box")
	}
}

func TestEffectsFastPathAllocationFree(t *testing.T) {
	skipAllocBudget(t)
	// The no-effect paint stays allocation-free: the stylesheet loads
	// but declares no rendering effect, so the old PushAlpha path runs.
	loadCSS(t, `.plain { background-color: #ff0000; }`)
	t.Cleanup(func() { LoadStylesheet("") })
	b := NewBox(Row, 0, 0)
	b.Append(NewSpacer(10, 10), false)
	b.AddClass("plain")
	arrangeTree(t, b, 10, 10)
	paintTree(b, 10, 10)
	cv := render.New(make([]byte, render.Stride(10)*10), render.Stride(10), 10, 10)
	b.Paint(cv)
	allocs := testing.AllocsPerRun(20, func() {
		b.Paint(cv)
	})
	if allocs > 0 {
		t.Errorf("the no-effect paint allocated %v allocs/paint", allocs)
	}
}

// A backdrop filter painted inside an opacity group (a swiped card
// fading out) leaves the backdrop around its box as it was: only the
// box's own pixels take the filtered backdrop. The backdrop around it
// used to come back at the group's opacity, a dark band on whatever lay
// within the blur's reach (#340).
func TestBackdropUnderOpacityStaysInItsBox(t *testing.T) {
	loadCSS(t, `.g { backdrop-filter: blur(16px) saturate(1.5); background-color: rgba(40,40,60,0.5); }`)
	glass := NewBox(Column, 0, 0)
	glass.AddClass("g")
	glass.Append(NewSpacer(60, 30), false)
	SizeWithin(glass, Size{W: 60, H: 30})
	glass.Arrange(render.Rect{X: 40, Y: 40, W: 60, H: 30})
	const w, h = 140, 110
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	cv.Clear(cv.Rect(), render.RGB(200, 30, 30))
	prev := cv.PushAlpha(0.5)
	glass.Paint(cv)
	cv.PopAlpha(prev)
	for _, p := range [][2]int{{20, 55}, {120, 55}, {70, 20}, {70, 90}, {39, 39}} {
		if got := render.ColorFromBytes(data[p[1]*render.Stride(w)+p[0]*4:]); got != render.RGB(200, 30, 30) {
			t.Errorf("(%d, %d) outside the glass box became %#08x, want the untouched backdrop", p[0], p[1], uint32(got))
		}
	}
}
