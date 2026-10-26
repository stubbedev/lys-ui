package style

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// approx reports whether got is within tol of want.
func approx(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol
}

func TestColorConversions(t *testing.T) {
	red := rgbaf{r: 1, a: 1}
	white := rgbaf{r: 1, g: 1, b: 1, a: 1}
	l, a, b := linearToOklab(rgbaf{r: 1})
	if !approx(l, 0.627955, 1e-4) || !approx(a, 0.224863, 1e-4) || !approx(b, 0.125846, 1e-4) {
		t.Errorf("oklab red: %v %v %v", l, a, b)
	}
	if l, a, b = linearToOklab(white); !approx(l, 1, 1e-6) || !approx(a, 0, 1e-6) || !approx(b, 0, 1e-6) {
		t.Errorf("oklab white: %v %v %v", l, a, b)
	}
	l2, c, h := labToLch(linearToOklab(rgbaf{r: 1}))
	if !approx(l2, 0.627955, 1e-4) || !approx(c, 0.257683, 1e-4) || !approx(h, 29.2338, 1e-2) {
		t.Errorf("oklch red: %v %v %v", l2, c, h)
	}
	L, A, B := xyzToLab(rgbToXYZ(red))
	if !approx(L, 54.29, 0.02) || !approx(A, 80.80, 0.05) || !approx(B, 69.91, 0.05) {
		t.Errorf("lab red (D50): %v %v %v", L, A, B)
	}
	if L, A, B = xyzToLab(rgbToXYZ(white)); !approx(L, 100, 0.02) || !approx(A, 0, 0.05) || !approx(B, 0, 0.05) {
		t.Errorf("lab white: %v %v %v", L, A, B)
	}
	L2, C, H := labToLch(xyzToLab(rgbToXYZ(red)))
	if !approx(L2, 54.29, 0.02) || !approx(C, 106.84, 0.05) || !approx(H, 40.86, 0.02) {
		t.Errorf("lch red: %v %v %v", L2, C, H)
	}
}

func TestColorSpaceRoundTrips(t *testing.T) {
	colors := []rgbaf{
		{r: 1, a: 1},
		{g: 1, a: 1},
		{b: 1, a: 1},
		{r: 0.5, g: 0.25, b: 0.75, a: 1},
		{r: 0.9, g: 0.1, b: 0.4, a: 0.5},
	}
	spaces := []space{spSrgb, spSrgbLinear, spLab, spLch, spOklab, spOklch, spHsl, spHwb}
	for _, c := range colors {
		for _, sp := range spaces {
			out := fromComps(toComps(c, sp), sp)
			for _, ch := range []float64{out.r, out.g, out.b, out.a} {
				if math.IsNaN(ch) {
					t.Fatalf("space %v: NaN from %+v", sp, c)
				}
			}
			if !approx(out.r, c.r, 1e-5) || !approx(out.g, c.g, 1e-5) || !approx(out.b, c.b, 1e-5) || !approx(out.a, c.a, 1e-5) {
				t.Errorf("space %v: %+v -> %+v", sp, c, out)
			}
		}
	}
}

func TestColorConstructors(t *testing.T) {
	s := parseOne(t, `
		a { color: oklch(0.627955 0.257683 29.2338); }
		b { color: oklab(0.627955 0.224863 0.125846); }
		c { color: lab(54.29 80.80 69.91); }
		d { color: lch(54.29% 106.84 40.86); }
		e { color: hwb(0 0% 0%); background-color: hwb(0 30% 30%); }
		f { color: oklch(100% 0 0); }
		g { color: color(srgb 1 0 0); background-color: color(srgb-linear 1 0 0 / 50%); }
		h { color: color(display-p3 1 0 0); }
		i { color: rgb(100%, 0%, 0%); background-color: hsl(120, 100%, 50%); outline-color: rgba(255, 0, 0, 0); }
	`)
	red := render.RGB(0xff, 0, 0)
	if v := computeTree(s, el("a")); !colorNear(v.Color, red, 2) {
		t.Errorf("oklch red: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("b")); !colorNear(v.Color, red, 2) {
		t.Errorf("oklab red: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("c")); !colorNear(v.Color, red, 2) {
		t.Errorf("lab red: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("d")); !colorNear(v.Color, red, 2) {
		t.Errorf("lch red: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("e")); v.Color != red || !colorNear(v.Background, render.RGB(0xb3, 0x4d, 0x4d), 1) {
		t.Errorf("hwb: %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	if v := computeTree(s, el("f")); !colorNear(v.Color, render.RGB(0xff, 0xff, 0xff), 1) {
		t.Errorf("oklch percent lightness: %08x", uint32(v.Color))
	}
	v := computeTree(s, el("g"))
	if !colorNear(v.Color, red, 1) || v.Background.A() != 128 {
		t.Errorf("color(srgb): %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	// CSS Color 4 §13.2: the out-of-gamut P3 red keeps its OKLCH
	// lightness and hue and loses chroma until clipping is invisible,
	// rgb(255 11 12) — not the clipped sRGB red.
	if v := computeTree(s, el("h")); !colorNear(v.Color, render.RGB(0xff, 0x0b, 0x0c), 0) {
		t.Errorf("color(display-p3) out-of-gamut red gamut-maps to rgb(255 11 12): %08x", uint32(v.Color))
	}
	v = computeTree(s, el("i"))
	if v.Color != red || v.Background != render.RGB(0, 0xff, 0) || v.OutlineColor != 0 {
		t.Errorf("legacy comma rgb/hsl: %08x %08x %08x", uint32(v.Color), uint32(v.Background), uint32(v.OutlineColor))
	}
}

// colorNear compares two colors channel by channel within tol.
func colorNear(a, b render.Color, tol int) bool {
	d := func(x, y uint8) int {
		v := int(x) - int(y)
		if v < 0 {
			return -v
		}
		return v
	}
	return d(a.R(), b.R()) <= tol && d(a.G(), b.G()) <= tol && d(a.B(), b.B()) <= tol
}

func TestRelativeColorSyntax(t *testing.T) {
	s := parseOne(t, `
		:root { --src: #ff0000; }
		a { color: rgb(from #ff0000 r g b); }
		b { color: rgb(from #00ff00 0 g b / 50%); }
		c { color: rgb(from var(--src) calc(r / 2) 0 0); }
		d { color: hsl(from #ff0000 h s 50%); }
		e { color: oklch(from red l c 120deg); }
		f { color: rgb(from currentColor r g b); }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0xff, 0, 0) {
		t.Errorf("identity: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("b")); v.Color != render.RGBA(0, 0xff, 0, 128) {
		t.Errorf("channel swap with alpha: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("c")); !colorNear(v.Color, render.RGB(0x80, 0, 0), 1) {
		t.Errorf("calc over a channel: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("d")); v.Color != render.RGB(0xff, 0, 0) {
		t.Errorf("hsl relative: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("e")); !colorNear(v.Color, oklchColor(0.627955, 0.257683, 120), 2) {
		t.Errorf("oklch relative: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("f")); v.Color != render.RGB(0, 0, 0) {
		t.Errorf("currentColor origin (color unset at parse): %08x", uint32(v.Color))
	}
}

// oklchColor builds the render color of an oklch triple, for tests.
func oklchColor(l, c, h float64) render.Color {
	return fromComps([4]float64{l, c, h, 1}, spOklch).color()
}

func TestLightDark(t *testing.T) {
	s := parseOne(t, `a { color: light-dark(#112233, #445566); }`)
	light := computeEnv(s, el("a"), Env{Rem: 16})
	if light.Color != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("light: %08x", uint32(light.Color))
	}
	dark := computeEnv(s, el("a"), Env{Rem: 16, Media: Media{Dark: true}})
	if dark.Color != render.RGB(0x44, 0x55, 0x66) {
		t.Errorf("dark: %08x", uint32(dark.Color))
	}
}

func TestSystemColors(t *testing.T) {
	custom := SystemColors{
		Canvas: render.RGB(0x10, 0x20, 0x30), CanvasText: render.RGB(0x30, 0x20, 0x10),
		Highlight: render.RGB(0x0f, 0x0e, 0x0d), GrayText: render.RGB(0x77, 0x77, 0x77),
		ButtonFace: render.RGB(0x01, 0x02, 0x03),
	}
	SetSystemColors(custom)
	t.Cleanup(func() { SetSystemColors(SystemColors{}) })
	s := parseOne(t, `
		a { color: Canvas; background-color: Highlight; }
		b { color: ButtonFace; border-color: GrayText; }
	`)
	v := computeTree(s, el("a"))
	if v.Color != custom.Canvas || v.Background != custom.Highlight {
		t.Errorf("system colors: %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	v = computeTree(s, el("b"))
	if v.Color != custom.ButtonFace || v.BorderColor[0] != custom.GrayText {
		t.Errorf("system colors 2: %08x %08x", uint32(v.Color), uint32(v.BorderColor[0]))
	}
}

func TestColorMixSpaces(t *testing.T) {
	red, blue := render.RGB(0xff, 0, 0), render.RGB(0, 0, 0xff)
	spaces := []string{"srgb", "srgb-linear", "lab", "oklab", "lch", "oklch", "hsl", "hwb"}
	for _, sp := range spaces {
		css := "x { color: color-mix(in " + sp + ", #ff0000, #ff0000); }"
		s := parseOne(t, css)
		if v := computeTree(s, el("x")); v.Color != red {
			t.Errorf("mix same color in %s: %08x", sp, uint32(v.Color))
		}
		css = "x { color: color-mix(in " + sp + ", #ff0000 100%, #0000ff); }"
		s = parseOne(t, css)
		if v := computeTree(s, el("x")); !colorNear(v.Color, red, 1) {
			t.Errorf("mix 100%% first in %s: %08x", sp, uint32(v.Color))
		}
		css = "x { color: color-mix(in " + sp + ", #ff0000 0%, #0000ff); }"
		s = parseOne(t, css)
		if v := computeTree(s, el("x")); !colorNear(v.Color, blue, 1) {
			t.Errorf("mix 0%% first in %s: %08x", sp, uint32(v.Color))
		}
	}
	s := parseOne(t, `
		a { color: color-mix(in srgb, #ff0000, #0000ff); }
		b { color: color-mix(in srgb-linear, #ff0000, #0000ff); }
		c { color: color-mix(in oklab, #ff0000, #0000ff); }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x80, 0, 0x80) {
		t.Errorf("srgb mix: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("b")); !colorNear(v.Color, render.RGB(0xbc, 0, 0xbc), 1) {
		t.Errorf("srgb-linear mix: %08x", uint32(v.Color))
	}
	// The oklab midpoint of red and blue is a desaturated purple.
	if v := computeTree(s, el("c")); !colorNear(v.Color, render.RGB(0x8a, 0x51, 0xa0), 4) {
		t.Errorf("oklab mix: %08x", uint32(v.Color))
	}
}

func TestColorMixHueMethods(t *testing.T) {
	s := parseOne(t, `
		a { color: color-mix(in hsl shorter hue, hsl(0 100% 50%), hsl(120 100% 50%)); }
		b { color: color-mix(in hsl longer hue, hsl(0 100% 50%), hsl(120 100% 50%)); }
		c { color: color-mix(in hsl increasing hue, hsl(120 100% 50%), hsl(0 100% 50%)); }
		d { color: color-mix(in hsl decreasing hue, hsl(120 100% 50%), hsl(0 100% 50%)); }
	`)
	if v := computeTree(s, el("a")); !colorNear(v.Color, render.RGB(0xff, 0xff, 0), 1) {
		t.Errorf("shorter: %08x, want yellow", uint32(v.Color))
	}
	if v := computeTree(s, el("b")); !colorNear(v.Color, render.RGB(0, 0, 0xff), 1) {
		t.Errorf("longer: %08x, want blue", uint32(v.Color))
	}
	if v := computeTree(s, el("c")); !colorNear(v.Color, render.RGB(0, 0, 0xff), 1) {
		t.Errorf("increasing: %08x, want blue", uint32(v.Color))
	}
	if v := computeTree(s, el("d")); !colorNear(v.Color, render.RGB(0xff, 0xff, 0), 1) {
		t.Errorf("decreasing: %08x, want yellow", uint32(v.Color))
	}
}

func TestCurrentColorInColorFunctions(t *testing.T) {
	s := parseOne(t, `
		a { color: #ff0000; border: 1px solid light-dark(currentColor, #00ff00); outline: 1px solid color-mix(in oklch, currentColor 50%, #0000ff); }
	`)
	v := computeTree(s, el("a"))
	if v.BorderColor[0] != render.RGB(0xff, 0, 0) {
		t.Errorf("currentColor through light-dark (light scheme): %08x", uint32(v.BorderColor[0]))
	}
	if v.OutlineColor == 0 {
		t.Errorf("currentColor through color-mix: %08x", uint32(v.OutlineColor))
	}
}
