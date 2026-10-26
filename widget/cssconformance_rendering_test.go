package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// The rendering group's probes paint over the suite's white canvas;
// every expected pixel follows from the spec's formula over the
// colors involved. renBase is the backdrop (#336699) and renTop the
// source (#cc8844) of the blend and filter cases.
const (
	renWhite render.Color = 0xffffffff
	renRed   render.Color = 0xffff0000
	renBlue  render.Color = 0xff0000ff
	renBase  render.Color = 0xff336699
	renTop   render.Color = 0xffcc8844
)

// renLayers stacks #layers' children at its top-left corner, the
// later one painted over the earlier, by absolute positioning.
const renLayers = `#layers { position: relative; width: 100px; height: 100px; }
	#layers > * { position: absolute; left: 0; top: 0; }
	`

// renBlend is a 60x60 backdrop box under a 60x60 source box blended
// with mode.
func renBlend(mode string) confCase {
	return confCase{
		name: "mix-blend-mode " + mode,
		wpt:  "css/compositing/mix-blend-mode/mix-blend-mode-" + mode + ".html",
		spec: "compositing-1 §10",
		css: renLayers + `#under, #over { width: 60px; height: 60px; }
			#under { background-color: #336699; }
			#over { background-color: #cc8844; mix-blend-mode: ` + mode + `; }`,
		tree: nBox("#layers", nBox("#under"), nBox("#over")),
	}
}

// renWithPixel is c expecting the pixel at (30, 30).
func renWithPixel(c confCase, want render.Color) confCase {
	c.want = []confCheck{wantPixel(30, 30, want)}
	return c
}

// renFilter is a 60x60 #336699 box painted through filter.
func renFilter(name, wpt, filter string, want render.Color) confCase {
	return confCase{
		name: "filter " + name,
		wpt:  wpt,
		spec: "filter-effects-1 §13",
		css:  `#a { width: 60px; height: 60px; background-color: #336699; filter: ` + filter + `; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(30, 30, want), wantPixel(70, 30, renWhite)},
	}
}

// confRendering is the #159 Rendering group: filters, backdrop
// filters, blending and isolation, clipping, masking, 3-D and 2-D
// transforms, and the box painting basics.
var confRendering = []confCase{
	renFilter("grayscale", "css/filter-effects/filter-grayscale-001.html", "grayscale(100%)", 0xff5f5f5f),
	renFilter("sepia", "css/filter-effects/filter-sepia-001.html", "sepia(1)", 0xff7f7158),
	renFilter("saturate 0", "css/filter-effects/filter-saturate-001.html", "saturate(0)", 0xff5f5f5f),
	renFilter("saturate 2", "css/filter-effects/filter-saturate-001.html", "saturate(200%)", 0xff076dd3),
	renFilter("hue-rotate", "css/filter-effects/filter-hue_rotate-001.html", "hue-rotate(180deg)", 0xff8b5825),
	renFilter("invert", "css/filter-effects/filter-invert-001.html", "invert(1)", 0xffcc9966),
	renFilter("brightness", "css/filter-effects/filter-brightness-001.html", "brightness(0.5)", 0xff1a334d),
	renFilter("contrast", "css/filter-effects/filter-contrast-001.html", "contrast(2)", 0xff004db3),
	renFilter("opacity", "css/filter-effects/filter-opacity-001.html", "opacity(50%)", 0xff99b3cc),
	renFilter("list order brightness then invert", "", "brightness(0.5) invert(1)", 0xffe6ccb3),
	renFilter("list order invert then brightness", "", "invert(1) brightness(0.5)", 0xff664d33),
	{
		name: "filter blur spreads an edge",
		wpt:  "css/filter-effects/filter-blur-001.html",
		spec: "filter-effects-1 §13.1",
		css:  `#a { width: 100px; height: 60px; margin-left: 20px; background-color: #000000; filter: blur(4px); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(70, 30, 0xff000000),
			wantPixelNear(119, 30, 0xff737373, 16),
			wantPixelNear(120, 30, 0xff8c8c8c, 16),
			wantPixel(140, 30, renWhite),
			wantPixel(2, 30, renWhite),
		},
	},
	{
		name: "filter drop-shadow",
		wpt:  "css/filter-effects/filter-drop-shadow-001.html",
		spec: "filter-effects-1 §13.1",
		css:  `#a { width: 50px; height: 50px; background-color: #ff0000; filter: drop-shadow(10px 10px 0 #0000ff); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(25, 25, renRed),
			wantPixel(55, 55, renBlue),
			wantPixel(55, 5, renWhite),
			wantPixel(5, 55, renWhite),
		},
	},
	{
		name: "backdrop-filter invert",
		wpt:  "css/filter-effects/backdrop-filter-basic.html",
		spec: "filter-effects-2 §3",
		css: renLayers + `#under { width: 100px; height: 100px; background-color: #ff0000; }
			#over { width: 50px; height: 50px; backdrop-filter: invert(1); }`,
		tree: nBox("#layers", nBox("#under"), nBox("#over")),
		want: []confCheck{
			wantPixel(25, 25, 0xff00ffff),
			wantPixel(75, 75, renRed),
			wantPixel(150, 25, renWhite),
		},
	},
	{
		name: "backdrop-filter grayscale under a translucent box",
		spec: "filter-effects-2 §3",
		css: renLayers + `#under { width: 100px; height: 100px; background-color: #ff0000; }
			#over { width: 50px; height: 50px; backdrop-filter: grayscale(1); background-color: rgba(0, 0, 255, 0.5); }`,
		tree: nBox("#layers", nBox("#under"), nBox("#over")),
		want: []confCheck{
			wantPixel(25, 25, 0xff1b1b9a),
			wantPixel(75, 75, renRed),
		},
	},
	renWithPixel(renBlend("multiply"), 0xff293629),
	renWithPixel(renBlend("screen"), 0xffd6b8b4),
	renWithPixel(renBlend("overlay"), 0xff526d69),
	renWithPixel(renBlend("darken"), 0xff336644),
	renWithPixel(renBlend("lighten"), 0xffcc8899),
	renWithPixel(renBlend("color-dodge"), 0xffffdbd1),
	renWithPixel(renBlend("color-burn"), 0xff000000),
	renWithPixel(renBlend("hard-light"), 0xffad7052),
	renWithPixel(renBlend("soft-light"), 0xff596a7c),
	renWithPixel(renBlend("difference"), 0xff992255),
	renWithPixel(renBlend("exclusion"), 0xffad818b),
	renWithPixel(renBlend("luminosity"), 0xff6c9fd2),
	renWithPixel(renBlend("color"), 0xff934f0b),
	{
		name: "isolation isolate blends inside the group",
		wpt:  "css/compositing/isolation/isolation-isolate-blend.html",
		spec: "compositing-1 §3.2",
		css: renLayers + `#under, #group, #kid { width: 60px; height: 60px; }
			#under { background-color: #336699; }
			#group { isolation: isolate; }
			#kid { background-color: #cc8844; mix-blend-mode: multiply; }`,
		tree: nBox("#layers", nBox("#under"), nCol("#group", nBox("#kid"))),
		want: []confCheck{wantPixel(30, 30, renTop)},
	},
	{
		name: "isolation auto blends through the group",
		spec: "compositing-1 §3.2",
		css: renLayers + `#under, #group, #kid { width: 60px; height: 60px; }
			#under { background-color: #336699; }
			#kid { background-color: #cc8844; mix-blend-mode: multiply; }`,
		tree: nBox("#layers", nBox("#under"), nCol("#group", nBox("#kid"))),
		want: []confCheck{wantPixel(30, 30, 0xff293629)},
	},
	{
		name: "clip-path circle",
		wpt:  "css/css-masking/clip-path/clip-path-circle-001.html",
		spec: "css-shapes-1 §3.1",
		css:  `#a { width: 100px; height: 100px; background-color: #ff0000; clip-path: circle(50% at 50% 50%); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(50, 50, renRed),
			wantPixel(50, 6, renRed),
			wantPixel(94, 50, renRed),
			wantPixel(6, 6, renWhite),
			wantPixel(93, 93, renWhite),
		},
	},
	{
		name: "clip-path inset with round",
		wpt:  "css/css-masking/clip-path/clip-path-inset-round-001.html",
		spec: "css-shapes-1 §3.1",
		css:  `#a { width: 100px; height: 100px; background-color: #ff0000; clip-path: inset(10px 20px round 5px); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(15, 50, renWhite),
			wantPixel(25, 50, renRed),
			wantPixel(50, 5, renWhite),
			wantPixel(50, 15, renRed),
			wantPixel(20, 10, renWhite),
			wantPixel(77, 87, renRed),
		},
	},
	{
		name: "clip-path ellipse",
		wpt:  "css/css-masking/clip-path/clip-path-ellipse-001.html",
		spec: "css-shapes-1 §3.1",
		css:  `#a { width: 100px; height: 100px; background-color: #ff0000; clip-path: ellipse(40px 20px at 50% 50%); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(50, 50, renRed),
			wantPixel(85, 50, renRed),
			wantPixel(95, 50, renWhite),
			wantPixel(50, 65, renRed),
			wantPixel(50, 75, renWhite),
		},
	},
	{
		name: "clip-path polygon",
		wpt:  "css/css-masking/clip-path/clip-path-polygon-001.html",
		spec: "css-shapes-1 §3.1",
		css:  `#a { width: 100px; height: 100px; background-color: #ff0000; clip-path: polygon(50% 0, 100% 100%, 0 100%); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(50, 12, renRed),
			wantPixel(20, 12, renWhite),
			wantPixel(10, 95, renRed),
			wantPixel(10, 50, renWhite),
		},
	},
	{
		name: "clip-path path",
		wpt:  "css/css-masking/clip-path/clip-path-path-001.html",
		spec: "css-shapes-2 §5.4",
		css:  `#a { width: 100px; height: 100px; background-color: #ff0000; clip-path: path('M 0 0 L 100 0 L 0 100 Z'); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(10, 10, renRed),
			wantPixel(80, 10, renRed),
			wantPixel(90, 90, renWhite),
		},
	},
	{
		name: "clip-path clips hit testing",
		spec: "css-masking-1 §5",
		css:  `#a { width: 100px; height: 100px; background-color: #ff0000; clip-path: circle(50% at 50% 50%); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantHit(50, 50, "a"),
			wantHit(6, 6, "root"),
		},
	},
	{
		name: "mask-image alpha gradient",
		wpt:  "css/css-masking/mask-image/mask-image-1a.html",
		spec: "css-masking-1 §7.1",
		css:  `#a { width: 100px; height: 40px; background-color: #ff0000; mask-image: linear-gradient(to right, black, transparent); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(0, 20, 0xffff0303),
			wantPixel(50, 20, 0xffff8181),
			wantPixel(99, 20, 0xfffffcfc),
		},
	},
	{
		name: "mask-mode alpha keeps an opaque black layer",
		spec: "css-masking-1 §7.6",
		css: `#a { width: 100px; height: 40px; background-color: #ff0000;
			mask-image: linear-gradient(to right, white 50%, black 50%); mask-mode: alpha; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(25, 20, renRed), wantPixel(75, 20, renRed)},
	},
	{
		name: "mask-mode luminance hides black",
		spec: "css-masking-1 §7.6",
		css: `#a { width: 100px; height: 40px; background-color: #ff0000;
			mask-image: linear-gradient(to right, white 50%, black 50%); mask-mode: luminance; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(25, 20, renRed), wantPixel(75, 20, renWhite)},
	},
	{
		name: "mask-size and no-repeat",
		spec: "css-masking-1 §7.4",
		css: `#a { width: 100px; height: 40px; background-color: #ff0000;
			mask-image: linear-gradient(black, black); mask-size: 50px 20px; mask-repeat: no-repeat; mask-position: 50px 0; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(25, 10, renWhite), wantPixel(75, 10, renRed), wantPixel(75, 30, renWhite)},
	},
	{
		name: "perspective translateZ magnifies",
		wpt:  "css/css-transforms/transform3d-perspective-001.html",
		spec: "css-transforms-2 §6.1",
		css: `#a { width: 50px; height: 50px; margin: 50px; background-color: #ff0000;
			transform: perspective(100px) translateZ(50px); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(30, 30, renRed),
			wantPixel(120, 120, renRed),
			wantPixel(20, 20, renWhite),
			wantPixel(130, 130, renWhite),
		},
	},
	{
		name: "rotateX foreshortens without perspective",
		wpt:  "css/css-transforms/transform3d-rotatex-001.html",
		spec: "css-transforms-2 §13",
		css:  `#a { width: 50px; height: 50px; margin: 20px; background-color: #ff0000; transform: rotateX(60deg); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(45, 45, renRed),
			wantPixel(45, 30, renWhite),
			wantPixel(45, 60, renWhite),
			wantPixel(45, 35, renRed),
		},
	},
	{
		name: "backface-visibility hidden hides a flipped element",
		wpt:  "css/css-transforms/backface-visibility-hidden-001.html",
		spec: "css-transforms-2 §11",
		css:  `#a { width: 50px; height: 50px; background-color: #ff0000; transform: rotateY(180deg); backface-visibility: hidden; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(25, 25, renWhite)},
	},
	{
		name: "backface-visibility visible shows a flipped element",
		spec: "css-transforms-2 §11",
		css:  `#a { width: 50px; height: 50px; background-color: #ff0000; transform: rotateY(180deg); }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(25, 25, renRed)},
	},
	{
		name: "2d transforms translate scale skew",
		wpt:  "css/css-transforms/translate/translate-001.html",
		spec: "css-transforms-1 §13",
		css: `#t, #s, #k { width: 50px; height: 50px; background-color: #ff0000; }
			#t { transform: translate(10px, 20px); }
			#s { margin-top: 30px; transform: scale(0.5); }
			#k { margin-top: 30px; margin-left: 30px; transform: skewX(45deg); }`,
		tree: nCol("", nBox("#t"), nBox("#s"), nBox("#k")),
		want: []confCheck{
			wantPixel(5, 25, renWhite),
			wantPixel(15, 25, renRed),
			wantPixel(10, 112, renWhite),
			wantPixel(20, 112, renRed),
			wantPixel(40, 112, renWhite),
			wantPixel(20, 185, renWhite),
			wantPixel(90, 200, renRed),
			wantPixel(40, 200, renWhite),
		},
	},
	{
		name: "transform-origin moves the pivot",
		wpt:  "css/css-transforms/transform-origin-001.html",
		spec: "css-transforms-1 §8",
		css:  `#a { width: 50px; height: 50px; background-color: #ff0000; transform: scale(2); transform-origin: 0 0; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(90, 90, renRed), wantPixel(105, 50, renWhite)},
	},
	{
		name: "rotate turns about the center",
		spec: "css-transforms-1 §13",
		css:  `#a { width: 50px; height: 50px; margin: 20px; background-color: #ff0000; transform: rotate(45deg); }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(45, 45, renRed),
			wantPixel(23, 23, renWhite),
			wantPixel(45, 12, renRed),
			wantPixel(45, 7, renWhite),
		},
	},
	{
		name: "borders and radius",
		wpt:  "css/css-backgrounds/border-radius-001.html",
		spec: "css-backgrounds-3 §4, §5",
		css: `#a { width: 50px; height: 50px; background-color: #ff0000; border: 5px solid #0000ff; }
			#b { width: 50px; height: 50px; margin-top: 10px; background-color: #ff0000; border-radius: 25px; }`,
		tree: nCol("", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantPixel(2, 2, renBlue),
			wantPixel(30, 30, renRed),
			wantPixel(57, 57, renBlue),
			wantPixel(62, 30, renWhite),
			wantPixel(3, 73, renWhite),
			wantPixel(25, 95, renRed),
		},
	},
	{
		name: "box-shadow outer and inset",
		wpt:  "css/css-backgrounds/box-shadow-001.html",
		spec: "css-backgrounds-3 §7",
		css: `#a { width: 50px; height: 50px; background-color: #ff0000; box-shadow: 10px 10px 0 0 #0000ff; }
			#b { width: 50px; height: 50px; margin-top: 20px; background-color: #ff0000; box-shadow: inset 5px 5px 0 0 #0000ff; }`,
		tree: nCol("", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantPixel(55, 55, renBlue),
			wantPixel(25, 25, renRed),
			wantPixel(55, 5, renWhite),
			wantPixel(2, 72, renBlue),
			wantPixel(40, 110, renRed),
		},
	},
	{
		name: "outline and offset",
		wpt:  "css/css-ui/outline-offset-001.html",
		spec: "css-ui-4 §3",
		css:  `#a { width: 50px; height: 50px; margin: 10px; background-color: #ff0000; outline: 3px solid #0000ff; outline-offset: 2px; }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(30, 30, renRed),
			wantPixel(61, 30, renWhite),
			wantPixel(63, 30, renBlue),
			wantPixel(66, 30, renWhite),
			wantPixel(5, 30, renBlue),
		},
	},
	{
		name: "outline auto draws the focus ring",
		spec: "css-ui-4 §3.3, §3.4 (outline-style and outline-color auto)",
		css: `#a, #b { width: 20px; height: 20px; margin: 10px; background-color: #ff0000; accent-color: #00ff00; }
			#a { outline: auto; }
			#b { outline: 1px auto #0000ff; }`,
		tree: nCol("", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantPixel(15, 15, renRed), wantPixel(10-FocusRingWidth, 20, 0xff00ff00), wantPixel(9, 20, 0xff00ff00), wantPixel(9-FocusRingWidth, 20, renWhite),
			wantPixel(9, 60, renBlue), wantPixel(8, 60, renWhite),
		},
	},
	{
		name: "opacity composites the subtree",
		wpt:  "css/css-color/t422-rgba-onscreen-a.xht",
		spec: "css-color-4 §15.1",
		css: `#a { width: 60px; height: 60px; opacity: 0.5; background-color: #ff0000; }
			#k { width: 30px; height: 30px; background-color: #0000ff; }`,
		tree: nCol("#a", nBox("#k")),
		want: []confCheck{wantPixel(45, 45, 0xffff8080), wantPixel(15, 15, 0xff8080ff)},
	},
	{
		name: "linear and repeating gradients",
		wpt:  "css/css-images/linear-gradient-1.html",
		spec: "css-images-3 §3.1",
		css: `#a { width: 100px; height: 20px; background-image: linear-gradient(to right, #ff0000, #0000ff); }
			#b { width: 100px; height: 20px; background-image: repeating-linear-gradient(to right, #ff0000 0 10px, #0000ff 10px 20px); }
			#c { width: 100px; height: 20px; background-image: linear-gradient(to right, #ff0000 50%, #0000ff 50%); }`,
		tree: nCol("", nBox("#a"), nBox("#b"), nBox("#c")),
		want: []confCheck{
			wantPixel(0, 10, 0xfffe0001),
			wantPixel(50, 10, 0xff7f0080),
			wantPixel(5, 30, renRed),
			wantPixel(15, 30, renBlue),
			wantPixel(25, 30, renRed),
			wantPixel(45, 50, renRed),
			wantPixel(55, 50, renBlue),
		},
	},
	{
		name: "conic and radial gradients",
		wpt:  "css/css-images/conic-gradient-001.html",
		spec: "css-images-4 §3.3",
		css: `#a { width: 100px; height: 100px; background-image: conic-gradient(#ff0000 0 25%, #0000ff 25% 50%, #00ff00 50% 75%, #ffff00 75%); }
			#b { width: 100px; height: 100px; background-image: radial-gradient(circle closest-side, #ff0000 50%, #0000ff 50%); }`,
		tree: nCol("", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantPixel(75, 25, renRed),
			wantPixel(75, 75, renBlue),
			wantPixel(25, 75, 0xff00ff00),
			wantPixel(25, 25, 0xffffff00),
			wantPixel(50, 150, renRed),
			wantPixel(50, 110, renBlue),
			wantPixel(80, 150, renBlue),
		},
	},
	{
		name: "background-repeat tiles the image at its natural size",
		spec: "css-backgrounds-3 §3.4, §3.9 (auto size is the natural size)",
		css:  `#a { width: 40px; height: 20px; background: url("` + renRedBlue + `") repeat-x; }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(2, 5, renRed), wantPixel(7, 5, renBlue), wantPixel(12, 5, renRed), wantPixel(37, 5, renBlue),
			wantPixel(2, 15, renWhite),
		},
	},
	{
		name: "background-position offsets the lone tile",
		spec: "css-backgrounds-3 §3.6",
		css:  `#a { width: 40px; height: 10px; background-image: url("` + renRedBlue + `"); background-repeat: no-repeat; background-position: 5px 0; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(2, 5, renWhite), wantPixel(7, 5, renRed), wantPixel(12, 5, renBlue), wantPixel(17, 5, renWhite)},
	},
	{
		name: "background-position keywords place within the free space",
		spec: "css-backgrounds-3 §3.6 (percentages of the area minus the image)",
		css:  `#a { width: 40px; height: 20px; background: url("` + renRedBlue + `") right bottom no-repeat; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(32, 15, renRed), wantPixel(37, 15, renBlue), wantPixel(27, 15, renWhite), wantPixel(37, 5, renWhite)},
	},
	{
		name: "background-size scales the tile",
		spec: "css-backgrounds-3 §3.9",
		css:  `#a { width: 40px; height: 10px; background: url("` + renRedBlue + `") 0 0 / 20px 10px no-repeat; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(5, 5, renRed), wantPixel(15, 5, renBlue), wantPixel(25, 5, renWhite)},
	},
	{
		name: "background-size cover and contain keep the ratio",
		spec: "css-backgrounds-3 §3.9",
		css: `#a, #b { width: 40px; height: 10px; background-image: url("` + renRedBlue + `"); background-repeat: no-repeat; }
			#a { background-size: cover; } #b { background-size: contain; }`,
		tree: nCol("", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantPixel(10, 5, renRed), wantPixel(30, 5, renBlue),
			wantPixel(2, 15, renRed), wantPixel(7, 15, renBlue), wantPixel(15, 15, renWhite),
		},
	},
	{
		name: "background-repeat round fits a whole count",
		spec: "css-backgrounds-3 §3.4 (round)",
		css:  `#a { width: 24px; height: 10px; background: url("` + renRedBlue + `") round no-repeat; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(3, 5, renRed), wantPixel(9, 5, renBlue), wantPixel(15, 5, renRed), wantPixel(21, 5, renBlue)},
	},
	{
		name: "background-repeat space spreads whole tiles",
		spec: "css-backgrounds-3 §3.4 (space)",
		css:  `#a { width: 35px; height: 10px; background: url("` + renRedBlue + `") space no-repeat; }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(2, 5, renRed), wantPixel(11, 5, renWhite), wantPixel(14, 5, renRed),
			wantPixel(20, 5, renBlue), wantPixel(27, 5, renRed), wantPixel(33, 5, renBlue),
		},
	},
	{
		name: "multiple background layers stack the first on top",
		spec: "css-backgrounds-3 §3.1 (the first layer is closest to the user)",
		css: `#a { width: 40px; height: 10px; background-color: #ff0000;
			background-image: url("` + renRedBlue + `"), linear-gradient(#00ff00, #00ff00);
			background-repeat: no-repeat; background-size: auto, 30px 10px; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(2, 5, renRed), wantPixel(7, 5, renBlue), wantPixel(15, 5, 0xff00ff00), wantPixel(35, 5, renRed)},
	},
	{
		name: "background-clip and background-origin pick the boxes",
		spec: "css-backgrounds-3 §3.7, §3.8",
		css: `#a, #b { width: 20px; height: 20px; padding: 5px; }
			#a { background-color: #ff0000; background-clip: content-box; }
			#b { background: url("` + renRedBlue + `") no-repeat content-box; }`,
		tree: nCol("", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantPixel(2, 2, renWhite), wantPixel(15, 15, renRed),
			wantPixel(4, 32, renWhite), wantPixel(7, 37, renRed), wantPixel(12, 37, renBlue),
		},
	},
	{
		name: "background-attachment fixed positions against the viewport",
		spec: "css-backgrounds-3 §3.5",
		css: `#t { width: 10px; height: 10px; }
			#a, #b { width: 10px; height: 10px; background: url("` + renRedBlue + `") no-repeat; }
			#b { background-attachment: fixed; }`,
		tree: nCol("", nBox("#t"), nBox("#a"), nBox("#b")),
		want: []confCheck{wantPixel(2, 15, renRed), wantPixel(2, 25, renWhite)},
	},
	{
		name: "image-set picks the candidate for the device resolution",
		spec: "css-images-4 §2.2",
		css: `#a { width: 10px; height: 10px; background-image:
			image-set("testdata/conformance/green.png" 1x, "testdata/conformance/yellow.png" 2x); }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(5, 5, 0xff00ff00)},
	},
}

// renRedBlue is the 10x10 fixture, red left of x = 5 and blue right.
// renNine is the 30x30 nine-slice fixture: 10px red corners, green
// edges — the top edge green left of x = 15 and yellow right of it,
// so its tiling shows — and a blue middle.
const (
	renRedBlue = "testdata/conformance/red-blue.png"
	renNine    = "testdata/conformance/nine.png"
)

// renYellow and renGreen are the nine-slice fixture's edge colors.
const (
	renYellow render.Color = 0xffffff00
	renGreen  render.Color = 0xff00ff00
)

// confBorderImage are the border-image cases: a 55px border box (35px
// content, 10px borders) cut at 10px.
var confBorderImage = []confCase{
	{
		name: "border-image slices into nine regions",
		spec: "css-backgrounds-3 §6.2, §6.7",
		css:  `#a { width: 35px; height: 35px; border: 10px solid; border-image: url("` + renNine + `") 10; }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(5, 5, renRed), wantPixel(50, 50, renRed), wantPixel(5, 27, renGreen),
			wantPixel(50, 27, renGreen), wantPixel(27, 50, renGreen), wantPixel(27, 27, renWhite),
		},
	},
	{
		name: "border-image fill draws the middle",
		spec: "css-backgrounds-3 §6.2 (fill)",
		css:  `#a { width: 35px; height: 35px; border: 10px solid; border-image: url("` + renNine + `") 10 fill; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(27, 27, renBlue)},
	},
	{
		name: "border-image-width and border-image-outset",
		spec: "css-backgrounds-3 §6.3, §6.4",
		css: `#a { width: 40px; height: 40px; margin: 10px; border: 10px solid;
			border-image: url("` + renNine + `") 10 / 20px / 5px; }`,
		tree: nBox("#a"),
		want: []confCheck{
			wantPixel(7, 7, renRed), wantPixel(24, 24, renRed), wantPixel(26, 10, renGreen),
			wantPixel(4, 40, renWhite), wantPixel(40, 40, renWhite),
		},
	},
	{
		name: "border-image-repeat stretch repeat round space",
		spec: "css-backgrounds-3 §6.5, §6.7",
		css: `#s, #r, #o, #p { width: 35px; height: 35px; border: 10px solid; }
			#s { border-image: url("` + renNine + `") 10 stretch; }
			#r { border-image: url("` + renNine + `") 10 repeat; }
			#o { border-image: url("` + renNine + `") 10 round; }
			#p { border-image: url("` + renNine + `") 10 space; }`,
		tree: nCol("", nBox("#s"), nBox("#r"), nBox("#o"), nBox("#p")),
		vp:   Size{W: 100, H: 240},
		want: []confCheck{
			wantPixel(20, 5, renGreen), wantPixel(40, 5, renYellow),
			wantPixel(11, 60, renYellow), wantPixel(14, 60, renGreen), wantPixel(20, 60, renYellow),
			wantPixel(11, 115, renGreen), wantPixel(16, 115, renYellow), wantPixel(20, 115, renGreen),
			wantPixel(10, 170, renWhite), wantPixel(12, 170, renGreen), wantPixel(18, 170, renYellow), wantPixel(21, 170, renWhite),
		},
	},
	{
		name: "border-image with an unloadable source keeps the border",
		spec: "css-backgrounds-3 §6.1 (a source that fails to load is none)",
		css: `#a { width: 20px; height: 20px; border: 5px solid #0000ff;
			border-image: url("testdata/conformance/missing.png") 10; }`,
		tree: nBox("#a"),
		want: []confCheck{wantPixel(2, 15, renBlue)},
	},
}

func TestCSSConformanceBorderImage(t *testing.T) { runConformance(t, confBorderImage) }

// renHas reads whether #id's cascade sets p.
func renHas(id string, p style.Prop) func(f *confFixture) any {
	return func(f *confFixture) any { return f.values(id).Has(p) }
}

// confExceptionsRendering are the Rendering group's documented
// deviations.
var confExceptionsRendering = []confException{
	{
		name:   "url() filters drop the declaration",
		spec:   "filter-effects-1 §6 (url() references an SVG filter)",
		css:    `#a { width: 60px; height: 60px; background-color: #336699; filter: url("#f") invert(100%); }`,
		tree:   nBox("#a"),
		want:   []confCheck{wantPixel(30, 30, renBase), wantProbe("filter declared", renHas("a", style.PropFilter), false)},
		reason: "an SVG filter graph needs a document loader the engine does not have",
		doc:    "`url()` filters are not supported: they name SVG filter graphs",
	},
	{
		name:   "transform-style preserve-3d paints flat",
		spec:   "css-transforms-2 §6 (3-D rendering contexts)",
		css:    `#a { width: 10px; height: 10px; transform-style: preserve-3d; }`,
		tree:   nBox("#a"),
		want:   []confCheck{wantProbe("preserve-3d computed", func(f *confFixture) any { return f.values("a").Preserve3D }, true)},
		reason: "each subtree composites as one offscreen layer with no z-interaction between siblings, so there is no shared 3-D rendering context",
		doc:    "`transform-style: preserve-3d` parses and computes but paints as `flat`",
	},
	{
		name:   "the general 3-D transform functions are rejected",
		spec:   "css-transforms-2 §13",
		css:    `#a { width: 10px; height: 10px; transform: matrix3d(1,0,0,0, 0,1,0,0, 0,0,1,0, 0,0,0,1); }`,
		tree:   nBox("#a"),
		want:   []confCheck{wantProbe("transform declared", renHas("a", style.PropTransform), false)},
		reason: "only the plane-restricted 3-D functions compose honestly on the flat pipeline",
		doc:    "`matrix3d()`, `rotate3d()`, `scale3d()` and `translate3d()` are rejected",
	},
	{
		name:   "a mask layer list is rejected",
		spec:   "css-masking-1 §7.1 (mask-image takes a layer list)",
		css:    `#a { width: 10px; height: 10px; mask-image: linear-gradient(black, black), linear-gradient(black, black); }`,
		tree:   nBox("#a"),
		want:   []confCheck{wantProbe("mask-image declared", renHas("a", style.PropMaskImage), false)},
		reason: "the effect chain composites one mask layer",
		doc:    "Multiple mask layers are not supported: a comma list rejects the declaration",
	},
	{
		name: "background-blend-mode composites source-over",
		spec: "compositing-1 §9 (background-blend-mode)",
		css: `#a { width: 40px; height: 10px; background-color: #336699;
			background-image: linear-gradient(#cc8844, #cc8844); background-blend-mode: multiply; }`,
		tree:   nBox("#a"),
		want:   []confCheck{wantPixel(20, 5, renTop)},
		reason: "the background layers paint straight onto the canvas; blending one layer over the next needs a layer group per box",
		doc:    "`background-blend-mode` parses and is ignored",
	},
}

func TestCSSConformanceRenderingExceptions(t *testing.T) {
	runConformanceExceptions(t, confExceptionsRendering)
}

func TestCSSConformanceRendering(t *testing.T) { runConformance(t, confRendering) }
