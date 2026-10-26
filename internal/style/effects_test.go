package style

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// The rendering-effect property set (#159): every new longhand parsed
// and computed, the filter function list, the 3-D transform functions
// and the backface flag they compose.

func TestFilterParsesFunctionList(t *testing.T) {
	s := parseOne(t, `a { filter: blur(4px) grayscale(50%) brightness(1.2) drop-shadow(1px 2px 3px rgba(0,0,0,.5)); }`)
	v := computeTree(s, el("a"))
	if !v.Has(PropFilter) || v.Filter.N != 4 {
		t.Fatalf("filter list: %d ops", v.Filter.N)
	}
	want := []render.FilterKind{render.FilterBlur, render.FilterGrayscale, render.FilterBrightness, render.FilterDropShadow}
	for i, k := range want {
		if v.Filter.Ops[i].Kind != k {
			t.Errorf("op %d = %v, want %v", i, v.Filter.Ops[i].Kind, k)
		}
	}
	if v.Filter.Ops[0].Amount != 4 || v.Filter.Ops[1].Amount != 0.5 {
		t.Errorf("amounts: %+v", v.Filter.Ops[:2])
	}
	sh := v.Filter.Ops[3].Shadow
	if sh.DX != 1 || sh.DY != 2 || sh.Blur != 3 || sh.Color.A() != 128 {
		t.Errorf("drop-shadow: %+v", sh)
	}
	if v.Brightness != 1.2 {
		t.Errorf("brightness product = %v", v.Brightness)
	}
	if b, ok := v.Filter.BrightnessOnly(); ok {
		t.Errorf("a mixed list claimed brightness-only: %v %v", b, ok)
	}
}

func TestFilterNoneAndLegacyBrightness(t *testing.T) {
	s := parseOne(t, `a { filter: none; } b { filter: brightness(2); }`)
	v := computeTree(s, el("a"))
	if !v.Has(PropFilter) || !v.Filter.Empty() || v.Brightness != 1 {
		t.Errorf("filter none: %+v", v.Filter)
	}
	v = computeTree(s, el("b"))
	if v.Brightness != 2 {
		t.Errorf("brightness(2) = %v", v.Brightness)
	}
	if b, ok := v.Filter.BrightnessOnly(); !ok || b != 2 {
		t.Errorf("brightness-only: %v %v", b, ok)
	}
}

func TestBackdropFilterParses(t *testing.T) {
	s := parseOne(t, `a { backdrop-filter: blur(8px) saturate(1.5); }`)
	v := computeTree(s, el("a"))
	if !v.Has(PropBackdropFilter) || v.Backdrop.N != 2 || v.Backdrop.Ops[0].Kind != render.FilterBlur || v.Backdrop.Ops[0].Amount != 8 {
		t.Errorf("backdrop: %+v", v.Backdrop)
	}
	if v.Has(PropFilter) {
		t.Error("backdrop-filter leaked into filter")
	}
}

// -lys-refract(<bezel> <displacement>) is lys-ui's liquid-glass bend:
// two non-negative lengths, nothing else (#197).
func TestRefractFilterParses(t *testing.T) {
	s := parseOne(t, `a { backdrop-filter: -lys-refract(14px 6px) blur(4px); }`)
	v := computeTree(s, el("a"))
	if op := v.Backdrop.Ops[0]; v.Backdrop.N != 2 || op.Kind != render.FilterRefract || op.Bezel != 14 || op.Amount != 6 {
		t.Errorf("refract: %+v", v.Backdrop)
	}
	for _, bad := range []string{`-lys-refract(14px)`, `-lys-refract(14px 6px 2px)`, `-lys-refract(-1px 6px)`, `-lys-refract(14px red)`} {
		s := parseWarns(t, `a { backdrop-filter: `+bad+`; }`)
		if v := computeTree(s, el("a")); v.Has(PropBackdropFilter) {
			t.Errorf("%s parsed: %+v", bad, v.Backdrop)
		}
	}
}

// -lys-grain(<opacity>) is a background image of noise: a number or
// percentage in (0, 1]; a border image has nothing to slice in it.
func TestGrainImageParses(t *testing.T) {
	s := parseOne(t, `a { background-image: -lys-grain(0.05); } b { background-image: -lys-grain(8%); }`)
	if v := computeTree(s, el("a")); v.Bg.Layer(0).Image.Grain != 0.05 {
		t.Errorf("grain = %v", v.Bg.Layer(0).Image)
	}
	if v := computeTree(s, el("b")); v.Bg.Layer(0).Image.Grain != 0.08 {
		t.Errorf("percent grain = %v", v.Bg.Layer(0).Image)
	}
	for _, bad := range []string{`background-image: -lys-grain(0)`, `background-image: -lys-grain(2)`, `background-image: -lys-grain(red)`, `border-image-source: -lys-grain(0.1)`} {
		s := parseWarns(t, `a { `+bad+`; }`)
		if v := computeTree(s, el("a")); v.Has(PropBackgroundImage) || v.Has(PropBorderImageSource) {
			t.Errorf("%s parsed", bad)
		}
	}
}

func TestMixBlendModeAndIsolation(t *testing.T) {
	s := parseOne(t, `a { mix-blend-mode: color-dodge; } b { mix-blend-mode: screen; isolation: isolate; }`)
	v := computeTree(s, el("a"))
	if v.Blend != render.BlendColorDodge {
		t.Errorf("blend = %v", v.Blend)
	}
	v = computeTree(s, el("b"))
	if v.Blend != render.BlendScreen || !v.Isolate {
		t.Errorf("blend/isolate = %v %v", v.Blend, v.Isolate)
	}
}

func TestClipPathShapes(t *testing.T) {
	s := parseOne(t, `
		a { clip-path: inset(4px 8px round 6px); }
		b { clip-path: circle(50% at 25% 75%); }
		c { clip-path: ellipse(40px 20px at center); }
		d { clip-path: polygon(0 0, 100% 0, 50% 100%); }
		e { clip-path: path("M 0 0 L 10 0 L 0 10 Z"); }
		f { clip-path: none; }
	`)
	v := computeTree(s, el("a"))
	if v.Clip.Kind != ClipInset || !v.Clip.Inset[1].Pct && v.Clip.Inset[1].V != 8 || v.Clip.InsetRad[0] != 6 {
		t.Errorf("inset: %+v", v.Clip)
	}
	v = computeTree(s, el("b"))
	if v.Clip.Kind != ClipCircle || !v.Clip.Rad.Pct || v.Clip.Rad.V != 50 || v.Clip.CenterX.V != 25 {
		t.Errorf("circle: %+v", v.Clip)
	}
	v = computeTree(s, el("c"))
	if v.Clip.Kind != ClipEllipse || v.Clip.RadX.V != 40 || v.Clip.RadY.V != 20 {
		t.Errorf("ellipse: %+v", v.Clip)
	}
	v = computeTree(s, el("d"))
	if v.Clip.Kind != ClipPolygon || len(v.Clip.Poly) != 6 || !v.Clip.Poly[4].Pct || v.Clip.Poly[4].V != 50 {
		t.Errorf("polygon: %+v", v.Clip)
	}
	v = computeTree(s, el("e"))
	if v.Clip.Kind != ClipPathData || v.Clip.Path != "M 0 0 L 10 0 L 0 10 Z" {
		t.Errorf("path: %+v", v.Clip)
	}
	v = computeTree(s, el("f"))
	if !v.Clip.Empty() {
		t.Errorf("none: %+v", v.Clip)
	}
}

func TestMaskGroupParses(t *testing.T) {
	s := parseOne(t, `
		a { mask: linear-gradient(to right, black, transparent) 25% 50% / 50% 100% no-repeat luminance; }
		b { mask-image: none; }
	`)
	v := computeTree(s, el("a"))
	if !v.Has(PropMaskImage) || v.Mask.Image.Gradient == nil || v.Mask.Image.Gradient.N != 2 {
		t.Fatalf("mask: %+v", v.Mask)
	}
	if !v.Has(PropMaskPosition) || v.Mask.Pos != [2]AxisPos{{Frac: 0.25}, {Frac: 0.5}} {
		t.Errorf("position: %+v", v.Mask)
	}
	want := LayerSize{Kind: SizeExplicit, W: LayerLength{Len: NumVal{V: 50, Pct: true}}, H: LayerLength{Len: NumVal{V: 100, Pct: true}}}
	if !v.Has(PropMaskSize) || v.Mask.Size != want {
		t.Errorf("size: %+v", v.Mask)
	}
	if !v.Has(PropMaskRepeat) || v.Mask.Repeat != (LayerRepeat{TileNoRepeat, TileNoRepeat}) {
		t.Error("no-repeat")
	}
	if !v.Has(PropMaskMode) || !v.Mask.Luminance {
		t.Error("luminance mode")
	}
	if v = computeTree(s, el("b")); !v.Has(PropMaskImage) || !v.Mask.Image.None() {
		t.Errorf("mask none: %+v", v.Mask)
	}
}

func TestPerspectiveAndTransformStyle(t *testing.T) {
	s := parseOne(t, `a { perspective: 600px; transform-style: preserve-3d; } b { perspective: none; }`)
	v := computeTree(s, el("a"))
	if v.Perspective != 600 || !v.Preserve3D {
		t.Errorf("perspective/style: %v %v", v.Perspective, v.Preserve3D)
	}
	if v = computeTree(s, el("b")); v.Perspective != 0 {
		t.Errorf("perspective none: %v", v.Perspective)
	}
}

func TestBackfaceVisibility(t *testing.T) {
	s := parseOne(t, `a { backface-visibility: hidden; } b { backface-visibility: visible; }`)
	v := computeTree(s, el("a"))
	if !v.BackfaceHidden {
		t.Error("hidden did not parse")
	}
	if v = computeTree(s, el("b")); v.BackfaceHidden {
		t.Error("visible parsed as hidden")
	}
}

func TestTransform3DFunctions(t *testing.T) {
	s := parseOne(t, `a { transform: perspective(400px) rotateY(30deg); } b { transform: rotateY(180deg); } c { transform: translateZ(20px); }`)
	v := computeTree(s, el("a"))
	if len(v.Transform.Parts) != 2 || v.Transform.Parts[0].Op != XformPerspective || v.Transform.Parts[1].Op != XformRotateY {
		t.Fatalf("3-D parts: %+v", v.Transform.Parts)
	}
	if v.Transform.P.Flat() {
		t.Error("a projective composition stayed affine")
	}
	if v.Transform.Backface {
		t.Error("rotateY(30deg) claimed a backface")
	}
	v = computeTree(s, el("b"))
	if !v.Transform.Backface {
		t.Error("rotateY(180deg) missed the backface")
	}
	if v = computeTree(s, el("c")); v.Transform.Parts[0].Op != XformTranslateZ {
		t.Errorf("translateZ: %+v", v.Transform.Parts)
	}
}

func TestLerpXform3D(t *testing.T) {
	s := parseOne(t, `a { transform: rotateY(0deg); }`)
	from := computeTree(s, el("a")).Transform
	s = parseOne(t, `a { transform: rotateY(180deg); }`)
	to := computeTree(s, el("a")).Transform
	mid := LerpXform(from, to, 0.5)
	det := mid.P[0]*mid.P[4] - mid.P[1]*mid.P[3]
	if det <= 0 {
		t.Errorf("the half turn's determinant %v, want positive", det)
	}
	if end := LerpXform(from, to, 1); !end.Backface {
		t.Error("the full turn missed the backface")
	}
}
