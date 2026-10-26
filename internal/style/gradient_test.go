package style

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// bgGradient is the gradient of v's top background layer, zero when
// it has none.
func bgGradient(v Values) Gradient {
	if g := v.Bg.Layer(0).Image.Gradient; g != nil {
		return *g
	}
	return Gradient{}
}

// The background longhands and the shorthand compute per layer: the
// lists match by index and repeat, the shorthand's position takes a
// `/ size`, one box names origin and clip, two name each, and the
// color rides the final layer only.
func TestBackgroundLayers(t *testing.T) {
	s := parseOne(t, `
		a { background-image: url("a.png"), linear-gradient(#000000, #ffffff);
			background-size: cover, 10px auto; background-repeat: no-repeat space;
			background-position: right 4px bottom, 25% 75%; background-clip: content-box;
			background-origin: border-box, padding-box; background-attachment: fixed; }
		b { background: url("b.png") center / contain repeat-x content-box, #102030 padding-box border-box; }
	`)
	v := computeTree(s, el("a"))
	if v.Bg.Len() != 2 {
		t.Fatalf("layers %d, want 2", v.Bg.Len())
	}
	top, bottom := v.Bg.Layer(0), v.Bg.Layer(1)
	if top.Image.URL != "a.png" || bottom.Image.Gradient == nil {
		t.Errorf("images %+v %+v", top.Image, bottom.Image)
	}
	if top.Size.Kind != SizeCover || bottom.Size != (LayerSize{Kind: SizeExplicit, W: LayerLength{Len: NumVal{V: 10}}, H: LayerLength{Auto: true}}) {
		t.Errorf("sizes %+v %+v", top.Size, bottom.Size)
	}
	if top.Repeat != (LayerRepeat{TileNoRepeat, TileSpace}) || bottom.Repeat != top.Repeat {
		t.Errorf("repeats %+v %+v, want the one entry repeating", top.Repeat, bottom.Repeat)
	}
	if top.Pos != [2]AxisPos{{Frac: 1, Px: -4}, {Frac: 1}} || bottom.Pos != [2]AxisPos{{Frac: 0.25}, {Frac: 0.75}} {
		t.Errorf("positions %+v %+v", top.Pos, bottom.Pos)
	}
	if top.Clip != BoxContent || top.Origin != BoxBorder || bottom.Origin != BoxPadding || bottom.Attach != AttachFixed {
		t.Errorf("boxes %+v %+v", top, bottom)
	}
	v = computeTree(s, el("b"))
	top, bottom = v.Bg.Layer(0), v.Bg.Layer(1)
	if top.Size.Kind != SizeContain || top.Pos != [2]AxisPos{{Frac: 0.5}, {Frac: 0.5}} ||
		top.Repeat != (LayerRepeat{TileRepeat, TileNoRepeat}) || top.Origin != BoxContent || top.Clip != BoxContent {
		t.Errorf("shorthand top layer %+v", top)
	}
	if !bottom.Image.None() || bottom.Origin != BoxPadding || bottom.Clip != BoxBorder || v.Background != render.RGB(0x10, 0x20, 0x30) {
		t.Errorf("shorthand bottom layer %+v, color %08x", bottom, uint32(v.Background))
	}
	warns := capture(t)
	Parse(`x { background: #ff0000, url("a.png"); }`)
	if len(*warns) != 1 {
		t.Errorf("a color outside the final layer: %d warnings, want 1", len(*warns))
	}
}

// border-image parses its shorthand in any order with the slash-led
// width and outset, the longhands override it, and undeclared
// longhands read their initial values.
func TestBorderImageParse(t *testing.T) {
	s := parseOne(t, `
		a { border-image: round url("n.png") 10% 4 fill / 2 auto 3px / 1px; }
		b { border-image-source: linear-gradient(#000000, #ffffff); border-image-repeat: space repeat; }
		c { border: 1px solid; border-image-source: url("n.png"); border: 2px solid; }
	`)
	va, vb := computeTree(s, el("a")), computeTree(s, el("b"))
	bi := va.UsedBorderImage()
	if bi.Source.URL != "n.png" || !bi.Fill || bi.Slice != [4]NumVal{{V: 10, Pct: true}, {V: 4}, {V: 10, Pct: true}, {V: 4}} {
		t.Errorf("source and slices %+v", bi)
	}
	if bi.Width != [4]BorderImageLen{{V: 2}, {Kind: ImageLenAuto}, {Kind: ImageLenLength, V: 3}, {Kind: ImageLenAuto}} {
		t.Errorf("widths %+v", bi.Width)
	}
	if bi.Outset != [4]BorderImageLen{{Kind: ImageLenLength, V: 1}, {Kind: ImageLenLength, V: 1}, {Kind: ImageLenLength, V: 1}, {Kind: ImageLenLength, V: 1}} {
		t.Errorf("outsets %+v", bi.Outset)
	}
	if bi.Repeat != (LayerRepeat{TileRound, TileRound}) {
		t.Errorf("repeat %+v", bi.Repeat)
	}
	bi = vb.UsedBorderImage()
	if bi.Source.Gradient == nil || bi.Repeat != (LayerRepeat{TileSpace, TileRepeat}) || bi.Slice[0] != (NumVal{V: 100, Pct: true}) || bi.Width[0] != (BorderImageLen{V: 1}) {
		t.Errorf("longhands over the initial values %+v", bi)
	}
	if vc := computeTree(s, el("c")); !vc.UsedBorderImage().Source.None() {
		t.Error("the border shorthand did not reset border-image")
	}
}

// image-set() picks the candidate for the device resolution: the
// lowest at or above it, else the highest.
func TestImageSet(t *testing.T) {
	s := parseOne(t, `a { background-image: image-set("lo.png" 1x, "hi.png" 2x, "x.webp" 3x type("image/webp")); }`)
	var sc Scratch
	pick := func(dpi float64) string {
		var v Values
		Compute([]Layer{{Sheet: s}}, el("a"), nil, nil, Env{Media: Media{DPI: dpi}}, &sc, &v)
		return v.Bg.Layer(0).Image.URL
	}
	if got := pick(96); got != "lo.png" {
		t.Errorf("1x device picked %q", got)
	}
	if got := pick(144); got != "hi.png" {
		t.Errorf("1.5x device picked %q, want the 2x candidate", got)
	}
	if got := pick(384); got != "hi.png" {
		t.Errorf("4x device picked %q, want the highest supported", got)
	}
}

// TestGradientForms pins the radial, conic and repeating parses: shape,
// size keyword, explicit radii, positions, from-angles, angle stops.
func TestGradientForms(t *testing.T) {
	s := parseOne(t, `
		a { background-image: radial-gradient(#000000, #ffffff); }
		b { background-image: radial-gradient(circle closest-side at left 25%, #000000, #ffffff); }
		c { background-image: radial-gradient(20px 10px at center, #000000, #ffffff); }
		d { background-image: repeating-radial-gradient(circle 8px, #000000, #ffffff 50%); }
		e { background-image: conic-gradient(from 90deg at 30% 70%, #ff0000, #0000ff 90deg, #00ff00); }
		f { background-image: repeating-linear-gradient(45deg, #000000, #ffffff 10%); }
		h { background-image: conic-gradient(#ff0000 0%, #0000ff 0.5turn); }
	`)
	img := func(name string) Gradient { return bgGradient(computeTree(s, el(name))) }
	if g := img("a"); g.Kind != render.GradientRadial || g.Circle || g.Size != render.FarthestCorner || g.CenterX != 0.5 || g.N != 2 {
		t.Errorf("default radial: %+v", g)
	}
	if g := img("b"); !g.Circle || g.Size != render.ClosestSide || g.CenterX != 0 || g.CenterY != 0.25 {
		t.Errorf("circle closest-side at left 25%%: %+v", g)
	}
	if g := img("c"); g.Size != render.RadiusExplicit || g.RadiusX != 20 || g.RadiusY != 10 || g.Circle {
		t.Errorf("explicit ellipse: %+v", g)
	}
	if g := img("d"); !g.Repeat || !g.Circle || g.RadiusX != 8 || g.Stops[1].Pos != 0.5 {
		t.Errorf("repeating circle: %+v", g)
	}
	if g := img("e"); g.Kind != render.GradientConic || g.Angle != 90 || g.CenterX != 0.3 || g.CenterY != 0.7 || g.Stops[1].Pos != 0.25 {
		t.Errorf("conic: %+v", g)
	}
	if g := img("f"); g.Kind != render.GradientLinear || !g.Repeat || g.Angle != 45 {
		t.Errorf("repeating linear: %+v", g)
	}
	if g := img("h"); g.N != 2 || g.Stops[1].Pos != 0.5 {
		t.Errorf("conic turn stop: %+v", g)
	}
}

// An unknown radial keyword skips the declaration (and the same path
// accepts a valid form, so the rejections are the keywords').
func TestGradientRejects(t *testing.T) {
	if _, ok := gradientOf(tokenize("radial-gradient(circle 1px, #000000, #ffffff)"), &ctx{}); !ok {
		t.Fatal("the control form did not parse")
	}
	for _, bad := range []string{"radial-gradient(bogus, #000000, #ffffff)", "conic-gradient(from 1px, #000000, #ffffff)", "radial-gradient(circle 1px 2px, #000000, #ffffff)"} {
		var g Gradient
		var ok bool
		toks := tokenize(bad)
		g, ok = gradientOf(toks, &ctx{})
		if ok {
			t.Errorf("%s parsed: %+v", bad, g)
		}
	}
}
