package widget

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// The values group reads lengths through letter-spacing (a float px
// slot that takes any sign), numbers through opacity, and angles
// through a rotate() transform's matrix.
// A declaration the engine rejected reads as "<undeclared>" there, so
// an invalid value never passes for a zero.
var (
	cpValSpacing = confProp{"letter-spacing", valDeclared(style.PropLetterSpacing, func(v *style.Values) any { return v.LetterSpacing })}
	cpValOpacity = confProp{"opacity", valDeclared(style.PropOpacity, func(v *style.Values) any { return v.Opacity })}
	cpValPadTop  = confProp{"padding-top", func(v *style.Values) any { return v.Padding.Top }}
	cpValBorderC = confProp{"border-top-color", func(v *style.Values) any { return v.BorderColor[0] }}
	cpValOutline = confProp{"outline-color", func(v *style.Values) any { return v.OutlineColor }}
	cpValRotate  = confProp{"rotate() angle (deg)", func(v *style.Values) any {
		m := v.Transform.M
		return math.Atan2(m.B, m.A) * 180 / math.Pi
	}}
)

// valDeclared reads p through read when the cascade set it.
func valDeclared(p style.Prop, read func(v *style.Values) any) func(v *style.Values) any {
	return func(v *style.Values) any {
		if !v.Has(p) {
			return "<undeclared>"
		}
		return read(v)
	}
}

// valColor expects id's computed prop color within one count per
// channel: CSS Color 4 fixes the conversion math, not the 8-bit
// rounding of each intermediate step.
func valColor(id string, prop confProp, want render.Color) confCheck {
	return wantProbe("computed "+prop.name+" of #"+id+" (±1)", func(f *confFixture) any {
		got, _ := prop.read(f.values(id)).(render.Color)
		if valNear(got, want, 1) {
			return want
		}
		return got
	}, want)
}

// valNear reports whether every channel of a and b is within tol.
func valNear(a, b render.Color, tol int) bool {
	for _, sh := range []uint{0, 8, 16, 24} {
		if d := int(uint8(a>>sh)) - int(uint8(b>>sh)); d < -tol || d > tol {
			return false
		}
	}
	return true
}

// valLeaves is n stubs #a, #b, ... for the per-node value cases.
func valLeaves(ids ...string) []confNode {
	out := make([]confNode, len(ids))
	for i, id := range ids {
		out[i] = nLeaf("#"+id, 10, 10)
	}
	return out
}

// confValues is the #159 Values and functions group: units, math
// functions, colors, env(), attr() and var().
var confValues = []confCase{
	{
		name: "calc precedence and nesting",
		spec: "css-values-4 §10.1",
		css: `#a { letter-spacing: calc(2px + 3px * 4 - (10px / 2)); }
			#b { letter-spacing: calc(calc(1px + 2px) * calc(2 + 1)); }
			#c { letter-spacing: calc((1px + 1px) / 4); }`,
		tree: nBox("", valLeaves("a", "b", "c")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 9.0),
			wantComputed("b", cpValSpacing, 9.0),
			wantComputed("c", cpValSpacing, 0.5),
		},
	},
	{
		name: "min max clamp",
		wpt:  "css/css-values/minmax-length-computed.html",
		spec: "css-values-4 §10.2",
		css: `#a { letter-spacing: min(10px, 20px, 5px); }
			#b { letter-spacing: max(1px, 3px, 2px); }
			#c { letter-spacing: clamp(1px, 50px, 5px); }
			#d { letter-spacing: clamp(10px, 1px, 5px); }
			#e { letter-spacing: calc(min(10px, 20px) + max(1px, 3px)); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 5.0),
			wantComputed("b", cpValSpacing, 3.0),
			wantComputed("c", cpValSpacing, 5.0),
			wantComputed("d", cpValSpacing, 10.0),
			wantComputed("e", cpValSpacing, 13.0),
		},
	},
	{
		name: "em and rem in calc",
		spec: "css-values-4 §6.1.1",
		css:  `#a { font-size: 20px; letter-spacing: calc(1em + 1rem); } #b { font-size: 10px; letter-spacing: 2.5em; }`,
		tree: nBox("", valLeaves("a", "b")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 36.0),
			wantComputed("b", cpValSpacing, 25.0),
		},
	},
	{
		name: "round strategies",
		wpt:  "css/css-values/round-mod-rem-computed.html",
		spec: "css-values-4 §10.5.1",
		css: `#a { letter-spacing: round(up, 23px, 10px); }
			#b { letter-spacing: round(down, 27px, 10px); }
			#c { letter-spacing: round(to-zero, -27px, 10px); }
			#d { letter-spacing: round(nearest, 25px, 10px); }
			#e { letter-spacing: round(-25px, 10px); }
			#f { letter-spacing: calc(round(-1.5) * 1px); }
			#g { letter-spacing: calc(round(2.5) * 1px); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f", "g")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 30.0),
			wantComputed("b", cpValSpacing, 20.0),
			wantComputed("c", cpValSpacing, -20.0),
			wantComputed("d", cpValSpacing, 30.0),
			wantComputed("e", cpValSpacing, -20.0),
			wantComputed("f", cpValSpacing, -1.0),
			wantComputed("g", cpValSpacing, 3.0),
		},
	},
	{
		name: "mod and rem signs",
		wpt:  "css/css-values/round-mod-rem-computed.html",
		spec: "css-values-4 §10.5.2",
		css: `#a { letter-spacing: mod(18px, 5px); }
			#b { letter-spacing: mod(-18px, 5px); }
			#c { letter-spacing: rem(-18px, 5px); }
			#d { letter-spacing: mod(18px, -5px); }
			#e { letter-spacing: rem(18px, -5px); }
			#f { transform: rotate(mod(400deg, 360deg)); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 3.0),
			wantComputed("b", cpValSpacing, 2.0),
			wantComputed("c", cpValSpacing, -3.0),
			wantComputed("d", cpValSpacing, -2.0),
			wantComputed("e", cpValSpacing, 3.0),
			wantComputed("f", cpValRotate, 40.0),
		},
	},
	{
		name: "abs and sign",
		wpt:  "css/css-values/signs-abs-computed.html",
		spec: "css-values-4 §10.8",
		css: `#a { letter-spacing: abs(-3px); }
			#b { letter-spacing: calc(sign(-10px) * 1px); }
			#c { letter-spacing: calc(sign(0px) * 1px); }
			#d { opacity: sign(3); }
			#e { letter-spacing: calc(abs(-2) * sign(5%) * 1px); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 3.0),
			wantComputed("b", cpValSpacing, -1.0),
			wantComputed("c", cpValSpacing, 0.0),
			wantComputed("d", cpValOpacity, 1.0),
			wantComputed("e", cpValSpacing, 2.0),
		},
	},
	{
		name: "sin cos tan",
		wpt:  "css/css-values/sin-cos-tan-computed.html",
		spec: "css-values-4 §10.6",
		css: `#a { opacity: sin(30deg); }
			#b { opacity: cos(60deg); }
			#c { opacity: tan(45deg); }
			#d { opacity: calc(sin(0.25turn) - cos(0.5turn) - 1.5); }
			#e { opacity: sin(calc(3.14159265358979 / 6)); }
			#f { opacity: cos(200grad + 200grad); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f")...),
		want: []confCheck{
			wantComputed("a", cpValOpacity, 0.5),
			wantComputed("b", cpValOpacity, 0.5),
			wantComputed("c", cpValOpacity, 1.0),
			wantComputed("d", cpValOpacity, 0.5),
			wantComputed("e", cpValOpacity, 0.5),
			wantComputed("f", cpValOpacity, 1.0),
		},
	},
	{
		name: "asin acos atan atan2",
		wpt:  "css/css-values/acos-asin-atan-atan2-computed.html",
		spec: "css-values-4 §10.7",
		css: `#a { transform: rotate(asin(1)); }
			#b { transform: rotate(acos(0.5)); }
			#c { transform: rotate(atan(1)); }
			#d { transform: rotate(atan2(1px, -1px)); }
			#e { transform: rotate(atan2(-1, -1)); }
			#f { transform: rotate(atan2(1em, 1em)); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f")...),
		want: []confCheck{
			wantComputed("a", cpValRotate, 90.0),
			wantComputed("b", cpValRotate, 60.0),
			wantComputed("c", cpValRotate, 45.0),
			wantComputed("d", cpValRotate, 135.0),
			wantComputed("e", cpValRotate, -135.0),
			wantComputed("f", cpValRotate, 45.0),
		},
	},
	{
		name: "pow sqrt hypot",
		wpt:  "css/css-values/hypot-pow-sqrt-computed.html",
		spec: "css-values-4 §10.9",
		css: `#a { letter-spacing: calc(pow(2, 3) * 1px); }
			#b { letter-spacing: calc(sqrt(16) * 1px); }
			#c { letter-spacing: hypot(3px, 4px); }
			#d { letter-spacing: calc(hypot(6, 8) * 1px); }
			#e { letter-spacing: calc(pow(sqrt(2), 2) * 1px); }
			#f { letter-spacing: hypot(-5px); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 8.0),
			wantComputed("b", cpValSpacing, 4.0),
			wantComputed("c", cpValSpacing, 5.0),
			wantComputed("d", cpValSpacing, 10.0),
			wantComputed("e", cpValSpacing, 2.0),
			wantComputed("f", cpValSpacing, 5.0),
		},
	},
	{
		name: "log and exp",
		wpt:  "css/css-values/exp-log-computed.html",
		spec: "css-values-4 §10.9",
		css: `#a { letter-spacing: calc(log(8, 2) * 1px); }
			#b { opacity: log(1); }
			#c { opacity: exp(0); }
			#d { letter-spacing: calc(log(exp(3)) * 1px); }
			#e { letter-spacing: calc(log(100, 10) * 1px); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 3.0),
			wantComputed("b", cpValOpacity, 0.0),
			wantComputed("c", cpValOpacity, 1.0),
			wantComputed("d", cpValSpacing, 3.0),
			wantComputed("e", cpValSpacing, 2.0),
		},
	},
	{
		name: "numeric constants",
		wpt:  "css/css-values/calc-infinity-nan-computed.html",
		spec: "css-values-4 §10.7.1",
		css: `#a { letter-spacing: calc(pi * 1px); }
			#b { letter-spacing: calc(e * 1px); }
			#c { opacity: calc(infinity); }
			#d { opacity: calc(-infinity); }
			#e { opacity: calc(NaN); }
			#f { opacity: sin(pi / 6); }
			#g { letter-spacing: calc(log(e) * 1px); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f", "g")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, math.Pi),
			wantComputed("b", cpValSpacing, math.E),
			wantComputed("c", cpValOpacity, 1.0),
			wantComputed("d", cpValOpacity, 0.0),
			wantComputed("e", cpValOpacity, 0.0),
			wantComputed("f", cpValOpacity, 0.5),
			wantComputed("g", cpValSpacing, 1.0),
		},
	},
	{
		name: "division by zero is infinite",
		wpt:  "css/css-values/calc-infinity-nan-computed.html",
		spec: "css-values-4 §10.9",
		css: `#a { opacity: calc(1 / 0); }
			#b { opacity: calc(-1 / 0); }
			#c { opacity: calc(0.5 + mod(1, 0)); }`,
		tree: nBox("", valLeaves("a", "b", "c")...),
		want: []confCheck{
			wantComputed("a", cpValOpacity, 1.0),
			wantComputed("b", cpValOpacity, 0.0),
			wantComputed("c", cpValOpacity, 0.0),
		},
	},
	{
		name: "viewport units",
		spec: "css-values-4 §6.1.2",
		vp:   Size{W: 400, H: 300},
		css: `#a { letter-spacing: 10vw; } #b { letter-spacing: 10vh; }
			#c { letter-spacing: 10vmin; } #d { letter-spacing: 10vmax; }
			#e { letter-spacing: calc(50vw - 50vh); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 40.0),
			wantComputed("b", cpValSpacing, 30.0),
			wantComputed("c", cpValSpacing, 30.0),
			wantComputed("d", cpValSpacing, 40.0),
			wantComputed("e", cpValSpacing, 50.0),
		},
	},
	{
		name: "small large dynamic viewport units",
		wpt:  "css/css-values/viewport-units-parsing.html",
		spec: "css-values-4 §6.1.2.1",
		vp:   Size{W: 400, H: 300},
		css: `#a { letter-spacing: 10dvh; } #b { letter-spacing: 10svh; } #c { letter-spacing: 10lvh; }
			#d { letter-spacing: 10dvw; } #e { letter-spacing: 10svmin; } #f { letter-spacing: 10lvmax; }
			#g { letter-spacing: 10vi; } #h { letter-spacing: 10vb; } #i { letter-spacing: 10dvi; }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f", "g", "h", "i")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 30.0),
			wantComputed("b", cpValSpacing, 30.0),
			wantComputed("c", cpValSpacing, 30.0),
			wantComputed("d", cpValSpacing, 40.0),
			wantComputed("e", cpValSpacing, 30.0),
			wantComputed("f", cpValSpacing, 40.0),
			wantComputed("g", cpValSpacing, 40.0),
			wantComputed("h", cpValSpacing, 30.0),
			wantComputed("i", cpValSpacing, 40.0),
		},
	},
	{
		name: "absolute length units",
		spec: "css-values-4 §6.2",
		css: `#a { letter-spacing: 1in; } #b { letter-spacing: 2.54cm; } #c { letter-spacing: 25.4mm; }
			#d { letter-spacing: 72pt; } #e { letter-spacing: 6pc; } #f { letter-spacing: 101.6Q; }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 96.0),
			wantComputed("b", cpValSpacing, 96.0),
			wantComputed("c", cpValSpacing, 96.0),
			wantComputed("d", cpValSpacing, 96.0),
			wantComputed("e", cpValSpacing, 96.0),
			wantComputed("f", cpValSpacing, 96.0),
		},
	},
	{
		name: "font-relative units scale with font-size",
		spec: "css-values-4 §6.1.1",
		css: `#a { font-size: 10px; } #b { font-size: 20px; }
			#a, #b { letter-spacing: calc(1ch + 1ex + 1cap + 1ic); }`,
		tree: nBox("", valLeaves("a", "b")...),
		want: []confCheck{
			wantProbe("20px font's units are twice the 10px font's", func(f *confFixture) any {
				return f.values("b").LetterSpacing - 2*f.values("a").LetterSpacing
			}, 0.0),
		},
	},
	{
		name: "lh and rlh",
		wpt:  "css/css-values/lh-unit-001.html",
		spec: "css-values-4 §6.1.1",
		css: `#root { line-height: 25px; }
			#a { line-height: 30px; letter-spacing: 1lh; }
			#b { line-height: 30px; letter-spacing: 2rlh; }
			#c { font-size: 10px; line-height: 2; letter-spacing: 1lh; }`,
		tree: nBox("", valLeaves("a", "b", "c")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 30.0),
			wantComputed("b", cpValSpacing, 50.0),
			wantComputed("c", cpValSpacing, 20.0),
		},
	},
	{
		name: "angle units",
		spec: "css-values-4 §7.1",
		css: `#a { transform: rotate(0.25turn); } #b { transform: rotate(50grad); }
			#c { transform: rotate(1rad); } #d { transform: rotate(calc(1turn - 270deg)); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d")...),
		want: []confCheck{
			wantComputed("a", cpValRotate, 90.0),
			wantComputed("b", cpValRotate, 45.0),
			wantComputed("c", cpValRotate, 180/math.Pi),
			wantComputed("d", cpValRotate, 90.0),
		},
	},
	{
		name: "lab lch oklab oklch",
		wpt:  "css/css-color/parsing/color-computed-lab.html",
		spec: "css-color-4 §8, §9",
		css: `#a { color: lab(50 20 -30); } #b { color: lch(60 40 120); }
			#c { color: oklab(0.5 0.1 -0.1); } #d { color: oklch(0.7 0.1 200); }
			#e { color: lch(60 40 0.33333333turn); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff856caa),
			valColor("b", cpColor, 0xff7d9a51),
			valColor("c", cpColor, 0xff81459a),
			valColor("d", cpColor, 0xff40b1b7),
			valColor("e", cpColor, 0xff7d9a51),
		},
	},
	{
		name: "percent and none channels",
		wpt:  "css/css-color/parsing/color-computed-lab.html",
		spec: "css-color-4 §8.1, §4.4",
		css: `#a { color: oklch(70% 25% 200); } #b { color: lab(50% 16% -24%); }
			#c { color: oklch(0.7 none 200); }`,
		tree: nBox("", valLeaves("a", "b", "c")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff40b1b7),
			valColor("b", cpColor, 0xff856caa),
			valColor("c", cpColor, 0xff9e9e9e),
		},
	},
	{
		name: "hsl and hwb",
		wpt:  "css/css-color/parsing/color-computed-hwb.html",
		spec: "css-color-4 §7, §8",
		css: `#a { color: hwb(200 20% 30%); } #b { color: hsl(120 50% 40%); }
			#c { color: hwb(210 20% 40%); } #d { color: hwb(0 60% 60%); }
			#e { color: hsl(120deg 100% 50% / 0.25); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff3388b2),
			valColor("b", cpColor, 0xff339933),
			valColor("c", cpColor, 0xff336699),
			valColor("d", cpColor, 0xff808080),
			valColor("e", cpColor, 0x40004000),
		},
	},
	{
		name: "color() predefined spaces",
		wpt:  "css/css-color/parsing/color-computed-color-function.html",
		spec: "css-color-4 §10",
		css: `#a { color: color(srgb-linear 0.2 0.4 0.6); } #b { color: color(display-p3 0.5 0.3 0.2); }
			#c { color: color(rec2020 0.5 0.3 0.2); } #d { color: color(a98-rgb 0.5 0.3 0.2); }
			#e { color: color(prophoto-rgb 0.5 0.3 0.2); } #f { color: color(xyz-d50 0.2 0.3 0.25); }
			#g { color: color(xyz-d65 0.25 0.3 0.35); } #h { color: color(xyz 0.25 0.3 0.35); }
			#i { color: color(srgb 1 0 0 / 50%); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f", "g", "h", "i")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff7caacb),
			valColor("b", cpColor, 0xff88492d),
			valColor("c", cpColor, 0xffa3533d),
			valColor("d", cpColor, 0xff8f4b2e),
			valColor("e", cpColor, 0xffb94d3b),
			valColor("f", cpColor, 0xff26a794),
			valColor("g", cpColor, 0xff749d9a),
			valColor("h", cpColor, 0xff749d9a),
			valColor("i", cpColor, 0x80800000),
		},
	},
	{
		name: "out-of-gamut colors gamut-map to sRGB",
		spec: "css-color-4 §13.2",
		css: `#a { color: color(display-p3 0 1 0); } #b { color: oklch(0.7 0.4 40); }
			#c { color: lab(50 100 0); } #d { color: oklch(1.2 0.2 120); } #e { color: lab(-10 20 20); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff00fb29),
			valColor("b", cpColor, 0xffff5c00),
			valColor("c", cpColor, 0xffee007d),
			valColor("d", cpColor, 0xffffffff),
			valColor("e", cpColor, 0xff000000),
		},
	},
	{
		name: "hex named and transparent",
		spec: "css-color-4 §5, §6",
		css: `#a { color: #f00; } #b { color: #ff000080; } #c { color: #0f08; }
			#d { color: rebeccapurple; } #e { color: transparent; } #f { color: rgb(255 0 0 / 50%); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f")...),
		want: []confCheck{
			valColor("a", cpColor, 0xffff0000),
			valColor("b", cpColor, 0x80800000),
			valColor("c", cpColor, 0x88008800),
			valColor("d", cpColor, 0xff663399),
			valColor("e", cpColor, 0x00000000),
			valColor("f", cpColor, 0x80800000),
		},
	},
	{
		name: "color-mix in srgb",
		wpt:  "css/css-color/parsing/color-computed-color-mix-function.html",
		spec: "css-color-5 §2",
		css: `#a { color: color-mix(in srgb, red, blue); }
			#b { color: color-mix(in srgb, red 25%, blue); }
			#c { color: color-mix(in srgb, red 20%, blue 30%); }
			#d { color: color-mix(in srgb, rgb(255 0 0 / 0.5), blue); }
			#e { color: color-mix(in srgb, red 150%, blue); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff800080),
			valColor("b", cpColor, 0xff4000bf),
			valColor("c", cpColor, 0x8033004d),
			valColor("d", cpColor, 0xbf400080),
			wantComputed("e", confProp{"color (declared)", valDeclared(style.PropColor, func(v *style.Values) any { return v.Color })}, "<undeclared>"),
		},
	},
	{
		name: "color-mix rectangular spaces",
		wpt:  "css/css-color/parsing/color-computed-color-mix-function.html",
		spec: "css-color-5 §2.1",
		css: `#a { color: color-mix(in srgb-linear, #ff8000, #008080); }
			#b { color: color-mix(in lab, #ff8000, #008080); }
			#c { color: color-mix(in oklab, #ff8000, #008080); }
			#d { color: color-mix(in xyz, #ff8000, #008080); }
			#e { color: color-mix(in xyz-d50, #ff8000, #008080); }
			#f { color: color-mix(in display-p3, #ff8000, #008080); }
			#g { color: color-mix(in a98-rgb, #ff8000, #008080); }
			#h { color: color-mix(in rec2020, #ff8000, #008080); }
			#i { color: color-mix(in prophoto-rgb, #ff8000, #008080); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f", "g", "h", "i")...),
		want: []confCheck{
			valColor("a", cpColor, 0xffbc805c),
			valColor("b", cpColor, 0xffa58651),
			valColor("c", cpColor, 0xffa0875f),
			valColor("d", cpColor, 0xffbc805c),
			valColor("e", cpColor, 0xffbc805c),
			valColor("f", cpColor, 0xff968253),
			valColor("g", cpColor, 0xff9f804a),
			valColor("h", cpColor, 0xffa28253),
			valColor("i", cpColor, 0xffac8255),
		},
	},
	{
		name: "color-mix polar spaces and hue methods",
		wpt:  "css/css-color/parsing/color-computed-color-mix-function.html",
		spec: "css-color-4 §12.4",
		css: `#a { color: color-mix(in lch, #ff8000, #008080); }
			#b { color: color-mix(in lch longer hue, #ff8000, #008080); }
			#c { color: color-mix(in oklch increasing hue, #ff8000, #008080); }
			#d { color: color-mix(in oklch decreasing hue, #ff8000, #008080); }
			#e { color: color-mix(in hsl, #ff8000, #008080); }
			#f { color: color-mix(in hwb longer hue, #ff8000, #008080); }
			#g { color: color-mix(in hsl shorter hue, red, blue); }
			#h { color: color-mix(in hsl longer hue, red, blue); }
			#i { color: color-mix(in hsl increasing hue, red, blue); }
			#j { color: color-mix(in hsl decreasing hue, red, blue); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f", "g", "h", "i", "j")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff599b2f),
			valColor("b", cpColor, 0xffa875df),
			valColor("c", cpColor, 0xff7a992a),
			valColor("d", cpColor, 0xff9e74cf),
			valColor("e", cpColor, 0xff30bf00),
			valColor("f", cpColor, 0xff9000c0),
			valColor("g", cpColor, 0xffff00ff),
			valColor("h", cpColor, 0xff00ff00),
			valColor("i", cpColor, 0xff00ff00),
			valColor("j", cpColor, 0xffff00ff),
		},
	},
	{
		name: "color-mix powerless hue takes the other hue",
		wpt:  "css/css-color/parsing/color-computed-color-mix-function.html",
		spec: "css-color-4 §4.4.1, §12.3",
		css: `#a { color: color-mix(in oklch, white, blue); }
			#b { color: color-mix(in hsl, white, blue); }
			#c { color: color-mix(in lch, red, blue); }`,
		tree: nBox("", valLeaves("a", "b", "c")...),
		want: []confCheck{
			valColor("a", cpColor, 0xff74a3ff),
			valColor("b", cpColor, 0xff9f9fdf),
			valColor("c", cpColor, 0xffcd007e),
		},
	},
	{
		name: "relative color syntax",
		wpt:  "css/css-color/parsing/color-computed-relative-color.html",
		spec: "css-color-5 §4",
		css: `#a { color: rgb(from #336699 r g b / 50%); }
			#b { color: rgb(from #336699 b g r); }
			#c { color: hsl(from #336699 h s calc(l * 1.5)); }
			#d { color: hsl(from red calc(h + 120) s l); }
			#e { color: lch(from red l 0 h); }
			#f { color: oklch(from #336699 l c calc(h + 180)); }
			#g { color: lab(from #336699 calc(l + 10) a b); }
			#h { --x: #336699; color: rgb(from var(--x) r g b / 50%); }
			#i { color: hwb(from #336699 h w b / calc(alpha / 4)); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e", "f", "g", "h", "i")...),
		want: []confCheck{
			valColor("a", cpColor, 0x801a334d),
			valColor("b", cpColor, 0xff996633),
			valColor("c", cpColor, 0xff6699cc),
			valColor("d", cpColor, 0xff00ff00),
			valColor("e", cpColor, 0xff828282),
			valColor("f", cpColor, 0xff865815),
			valColor("g", cpColor, 0xff4f7fb4),
			valColor("h", cpColor, 0x801a334d),
			valColor("i", cpColor, 0x400d1a26),
		},
	},
	{
		name: "light-dark under the dark theme",
		spec: "css-color-5 §7",
		css:  `#a { color: light-dark(#111111, #eeeeee); background-color: light-dark(white, black); }`,
		tree: nBox("", valLeaves("a")...),
		want: []confCheck{
			valColor("a", cpColor, 0xffeeeeee),
			valColor("a", cpBackground, 0xff000000),
		},
	},
	{
		name: "light-dark under the light theme",
		spec: "css-color-5 §7",
		css:  `#a { color: light-dark(#111111, #eeeeee); background-color: light-dark(white, black); }`,
		tree: nBox("", valLeaves("a")...),
		act:  func(*confFixture) { SetTheme(LightTheme()) },
		want: []confCheck{
			valColor("a", cpColor, 0xff111111),
			valColor("a", cpBackground, 0xffffffff),
		},
	},
	{
		name: "system colors follow the theme",
		spec: "css-color-4 §6.2",
		css:  `#a { color: CanvasText; background-color: Canvas; border-top-color: Highlight; }`,
		tree: nBox("", valLeaves("a")...),
		act:  func(*confFixture) { SetTheme(LightTheme()) },
		want: []confCheck{
			wantProbe("CanvasText is the theme text", func(f *confFixture) any { return f.values("a").Color == Current().Text }, true),
			wantProbe("Canvas is the theme background", func(f *confFixture) any { return f.values("a").Background == Current().Bg }, true),
			wantProbe("Highlight is the theme accent", func(f *confFixture) any { return f.values("a").BorderColor[0] == Current().Accent }, true),
		},
	},
	{
		name: "currentColor everywhere",
		wpt:  "css/css-color/currentcolor-001.html",
		spec: "css-color-4 §6.4",
		css: `#p { color: #ff0000; --c: currentColor; }
			#a { color: #0000ff; border-top-color: currentColor; outline-color: currentcolor;
				background-color: color-mix(in srgb, currentColor 50%, white); }
			#b { border-top-color: var(--c); }
			#c { color: rgb(from currentColor b g r); }`,
		tree: nBox("#p", valLeaves("a", "b", "c")...),
		want: []confCheck{
			valColor("a", cpValBorderC, 0xff0000ff),
			valColor("a", cpValOutline, 0xff0000ff),
			valColor("a", cpBackground, 0xff8080ff),
			valColor("b", cpValBorderC, 0xffff0000),
			valColor("c", cpColor, 0xff0000ff),
		},
	},
	{
		name: "env() values and fallbacks",
		wpt:  "css/css-env/fallback.html",
		spec: "css-env-1 §3",
		css: `#a { letter-spacing: env(safe-area-inset-top); }
			#b { letter-spacing: env(no-such-variable, 7px); }
			#c { letter-spacing: env(safe-area-inset-left, 7px); }
			#d { letter-spacing: calc(env(keyboard-inset-height) + 3px); }
			#e { letter-spacing: env(no-such-variable); }`,
		tree: nBox("", valLeaves("a", "b", "c", "d", "e")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 0.0),
			wantComputed("b", cpValSpacing, 7.0),
			wantComputed("c", cpValSpacing, 0.0),
			wantComputed("d", cpValSpacing, 3.0),
			wantComputed("e", cpValSpacing, "<undeclared>"),
		},
	},
	{
		name: "typed attr()",
		wpt:  "css/css-values/attr-all-types.html",
		spec: "css-values-5 §7.5",
		css: `#a { letter-spacing: attr(data-n px); color: attr(data-c type(<color>)); }
			#b { letter-spacing: attr(data-l type(<length>)); }
			#c { letter-spacing: attr(data-missing px, 9px); color: attr(data-missing color, #00ff00); }
			#d { letter-spacing: attr(data-l number, 4px); opacity: attr(data-o number); }
			#e { letter-spacing: calc(attr(data-n px) * 2); }`,
		tree: nBox("",
			valAttrLeaf("#a", "data-n", "7", "data-c", "#ff0000"),
			valAttrLeaf("#b", "data-l", "12px"),
			valAttrLeaf("#c"),
			valAttrLeaf("#d", "data-l", "12px", "data-o", "0.25"),
			valAttrLeaf("#e", "data-n", "7"),
		),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 7.0),
			valColor("a", cpColor, 0xffff0000),
			wantComputed("b", cpValSpacing, 12.0),
			wantComputed("c", cpValSpacing, 9.0),
			valColor("c", cpColor, 0xff00ff00),
			wantComputed("d", cpValSpacing, 4.0),
			wantComputed("d", cpValOpacity, 0.25),
			wantComputed("e", cpValSpacing, 14.0),
		},
	},
	{
		name: "var() in shorthands and functions",
		wpt:  "css/css-variables/variable-substitution-shorthands.html",
		spec: "css-variables-1 §3",
		css: `#p { --w: 3px; --c: #00ff00; --p: 1px 2px 3px 4px; --l: 0.7; }
			#a { border: var(--w) solid var(--c); padding: var(--p); }
			#b { color: oklch(var(--l) 0.1 200); letter-spacing: calc(var(--w) * 2); }
			#c { letter-spacing: var(--missing, var(--also-missing, 6px)); }`,
		tree: nBox("#p", valLeaves("a", "b", "c")...),
		want: []confCheck{
			wantProbe("border-top-width of #a", func(f *confFixture) any { return f.values("a").BorderWidth.Top }, 3),
			valColor("a", cpValBorderC, 0xff00ff00),
			wantComputed("a", cpValPadTop, 1),
			wantProbe("padding-left of #a", func(f *confFixture) any { return f.values("a").Padding.Left }, 4),
			valColor("b", cpColor, 0xff40b1b7),
			wantComputed("b", cpValSpacing, 6.0),
			wantComputed("c", cpValSpacing, 6.0),
		},
	},
	{
		name: "var() cycles are invalid at computed-value time",
		wpt:  "css/css-variables/variable-cycles.html",
		spec: "css-variables-1 §2.3",
		css: `#a { --x: var(--y); --y: var(--x); letter-spacing: var(--x, 2px); opacity: 0.5; opacity: var(--y); }
			#b { --s: calc(var(--s) + 1px); letter-spacing: var(--s, 5px); }`,
		tree: nBox("", valLeaves("a", "b")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 2.0),
			wantComputed("a", cpValOpacity, 1.0),
			wantComputed("b", cpValSpacing, 5.0),
		},
	},
	{
		name: "rem reads the root font-size",
		wpt:  "css/css-values/rem-unit-root-element.html",
		spec: "css-values-4 §6.1.1",
		css:  `#root { font-size: 2rem; } #a { letter-spacing: 1rem; } #b { font-size: 10px; letter-spacing: 0.5rem; }`,
		tree: nBox("", valLeaves("a", "b")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 32.0),
			wantComputed("b", cpValSpacing, 16.0),
		},
	},
	{
		name: "resolution units",
		spec: "css-values-4 §7.4",
		css: `@media (min-resolution: 1dppx) { #a { letter-spacing: 1px; } }
			@media (min-resolution: 96dpi) and (max-resolution: 1x) { #b { letter-spacing: 2px; } }
			@media (min-resolution: 37.79dpcm) { #c { letter-spacing: 3px; } }
			@media (min-resolution: 2dppx) { #d { letter-spacing: 4px; } }`,
		tree: nBox("", valLeaves("a", "b", "c", "d")...),
		want: []confCheck{
			wantComputed("a", cpValSpacing, 1.0),
			wantComputed("b", cpValSpacing, 2.0),
			wantComputed("c", cpValSpacing, 3.0),
			wantComputed("d", cpValSpacing, "<undeclared>"),
		},
	},
}

// valAttrLeaf is a stub carrying the data attributes kv (name, value,
// ...) the attr() cases read.
func valAttrLeaf(sel string, kv ...string) confNode {
	n := nLeaf(sel, 10, 10)
	leaf := n.make
	n.make = func(f *confFixture, kids []Widget) Widget {
		w := leaf(f, kids)
		for i := 0; i+1 < len(kv); i += 2 {
			nodeOf(w).SetAttr(kv[i], kv[i+1])
		}
		return w
	}
	return n
}

func TestCSSConformanceValues(t *testing.T) { runConformance(t, confValues) }
