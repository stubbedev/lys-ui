package widget

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

// The paint probes' colors.
const (
	layRed   = 0xffff0000
	layGreen = 0xff008000
	layBlue  = 0xff0000ff
	layWhite = 0xffffffff
)

// layScroll scrolls the Scroll named id to y.
func layScroll(id string, y int) func(f *confFixture) {
	return func(f *confFixture) { f.get(id).(*Scroll).SetOffset(0, y) }
}

// layImage is an Image of a 40x20 picture, its left half red and its
// right half blue: the object-fit and object-position probes read
// which half lands where.
func layImage(sel string) confNode {
	return confNode{sel: sel, make: func(*confFixture, []Widget) Widget {
		pic := image.NewRGBA(image.Rect(0, 0, 40, 20))
		draw.Draw(pic, image.Rect(0, 0, 20, 20), image.NewUniform(color.RGBA{R: 0xff, A: 0xff}), image.Point{}, draw.Src)
		draw.Draw(pic, image.Rect(20, 0, 40, 20), image.NewUniform(color.RGBA{B: 0xff, A: 0xff}), image.Point{}, draw.Src)
		return NewImage(pic)
	}}
}

// layTop expects id's arranged top edge at y: the Scroll centers a
// narrower child, a toolkit choice the sticky case does not test.
func layTop(id string, y int) confCheck {
	return wantProbe("top of #"+id, func(f *confFixture) any { return boundsOf(f.get(id)).Y }, y)
}

// confLayout is the #159 Layout group: display, flexbox, grid, sizing,
// positioning, overflow, float and visibility.
var confLayout = []confCase{
	// Flexbox.
	{
		name: "flex-grow distributes free space",
		wpt:  "css/css-flexbox/flex-grow-001.xht",
		spec: "css-flexbox-1 §9.7",
		css: `#c { display: flex; width: 240px; height: 60px; }
			#a { flex-grow: 1; } #b { flex-grow: 2; }`,
		tree: nBox("#c", nLeaf("#a", 20, 20), nLeaf("#b", 20, 20), nLeaf("#z", 20, 20)),
		want: []confCheck{
			wantRect("a", 0, 0, 80, 60),
			wantRect("b", 80, 0, 140, 60),
			wantRect("z", 220, 0, 20, 60),
		},
	},
	{
		name: "flex-shrink scales by the base size",
		spec: "css-flexbox-1 §9.7 step 4c",
		css: `#c { display: flex; width: 300px; height: 20px; }
			#a { flex: 0 1 200px; } #b { flex: 0 3 200px; }`,
		tree: nBox("#c", nLeaf("#a", 20, 20), nLeaf("#b", 20, 20)),
		want: []confCheck{wantRect("a", 0, 0, 175, 20), wantRect("b", 175, 0, 125, 20)},
	},
	{
		name: "flex-basis percentage and auto",
		spec: "css-flexbox-1 §7.3.3, §9.2 step 3",
		css: `#c { display: flex; width: 400px; height: 20px; }
			#p { flex: 0 0 25%; } #w { flex: none; width: 120px; }`,
		tree: nBox("#c", nLeaf("#p", 10, 20), nLeaf("#w", 10, 20)),
		want: []confCheck{wantRect("p", 0, 0, 100, 20), wantRect("w", 100, 0, 120, 20)},
	},
	{
		name: "flex: 1 is a zero basis",
		spec: "css-flexbox-1 §7.1.1",
		css:  `#c { display: flex; width: 300px; height: 20px; } #c > * { flex: 1; }`,
		tree: nBox("#c", nLeaf("#a", 50, 20), nLeaf("#b", 100, 20)),
		want: []confCheck{wantRect("a", 0, 0, 150, 20), wantRect("b", 150, 0, 150, 20)},
	},
	{
		name: "automatic minimum size floors shrinking",
		wpt:  "css/css-flexbox/flex-minimum-width-flex-items-001.xht",
		spec: "css-flexbox-1 §4.5",
		css: `#c, #z { display: flex; width: 100px; height: 20px; }
			#z > * { min-width: 0; }`,
		tree: nCol("#o",
			nBox("#c", nLeaf("#a", 80, 20), nLeaf("#b", 80, 20)),
			nBox("#z", nLeaf("#a0", 80, 20), nLeaf("#b0", 80, 20))),
		want: []confCheck{
			wantRect("a", 0, 0, 80, 20), wantRect("b", 80, 0, 80, 20),
			wantRect("a0", 0, 20, 50, 20), wantRect("b0", 50, 20, 50, 20),
		},
	},
	{
		name: "max-width freezes an item and the rest share",
		spec: "css-flexbox-1 §9.7 step 4d-e",
		css: `#c { display: flex; width: 300px; height: 20px; }
			#c > * { flex: 1 1 0px; } #m { max-width: 50px; }`,
		tree: nBox("#c", nLeaf("#m", 10, 20), nLeaf("#i", 10, 20), nLeaf("#j", 10, 20)),
		want: []confCheck{wantRect("m", 0, 0, 50, 20), wantRect("i", 50, 0, 125, 20), wantRect("j", 175, 0, 125, 20)},
	},
	{
		name: "flex-wrap breaks lines with gaps",
		spec: "css-flexbox-1 §9.3, css-align-3 §8",
		css:  `#c { display: flex; flex-wrap: wrap; gap: 10px; width: 200px; height: 100px; align-content: flex-start; }`,
		tree: nBox("#c", nLeaf("#a", 80, 20), nLeaf("#b", 80, 20), nLeaf("#d", 80, 20), nLeaf("#e", 80, 20), nLeaf("#f", 80, 20)),
		want: []confCheck{
			wantRect("a", 0, 0, 80, 20), wantRect("b", 90, 0, 80, 20),
			wantRect("d", 0, 30, 80, 20), wantRect("e", 90, 30, 80, 20),
			wantRect("f", 0, 60, 80, 20),
		},
	},
	{
		name: "wrap-reverse stacks lines from the cross end",
		wpt:  "css/css-flexbox/flexbox_wrap-reverse.html",
		spec: "css-flexbox-1 §5.2",
		css:  `#c { display: flex; flex-wrap: wrap-reverse; gap: 10px; width: 200px; height: 100px; align-content: flex-start; }`,
		tree: nBox("#c", nLeaf("#a", 80, 20), nLeaf("#b", 80, 20), nLeaf("#d", 80, 20)),
		want: []confCheck{wantRect("a", 0, 80, 80, 20), wantRect("b", 90, 80, 80, 20), wantRect("d", 0, 50, 80, 20)},
	},
	{
		name: "justify-content center",
		wpt:  "css/css-flexbox/flexbox_justifycontent-center.html",
		spec: "css-flexbox-1 §8.2",
		css:  `#c { display: flex; justify-content: center; width: 300px; height: 50px; }`,
		tree: nBox("#c", nLeaf("#a", 50, 50), nLeaf("#b", 50, 50), nLeaf("#d", 50, 50)),
		want: []confCheck{wantRect("a", 75, 0, 50, 50), wantRect("b", 125, 0, 50, 50), wantRect("d", 175, 0, 50, 50)},
	},
	{
		name: "justify-content space-between",
		wpt:  "css/css-flexbox/flexbox_justifycontent-spacebetween.html",
		spec: "css-flexbox-1 §8.2",
		css:  `#c { display: flex; justify-content: space-between; width: 300px; height: 50px; }`,
		tree: nBox("#c", nLeaf("#a", 50, 50), nLeaf("#b", 50, 50), nLeaf("#d", 50, 50)),
		want: []confCheck{wantRect("a", 0, 0, 50, 50), wantRect("b", 125, 0, 50, 50), wantRect("d", 250, 0, 50, 50)},
	},
	{
		name: "justify-content space-around and space-evenly",
		wpt:  "css/css-flexbox/flexbox_justifycontent-spacearound.html",
		spec: "css-align-3 §5.3",
		css: `#c, #e { display: flex; width: 300px; height: 50px; } #c { justify-content: space-around; }
			#e { width: 350px; justify-content: space-evenly; }`,
		tree: nCol("#o",
			nBox("#c", nLeaf("#a", 50, 50), nLeaf("#b", 50, 50), nLeaf("#d", 50, 50)),
			nBox("#e", nLeaf("#a2", 50, 50), nLeaf("#b2", 50, 50), nLeaf("#d2", 50, 50))),
		want: []confCheck{
			wantRect("a", 25, 0, 50, 50), wantRect("b", 125, 0, 50, 50), wantRect("d", 225, 0, 50, 50),
			wantRect("a2", 50, 50, 50, 50), wantRect("b2", 150, 50, 50, 50), wantRect("d2", 250, 50, 50, 50),
		},
	},
	{
		name: "flex-direction row-reverse",
		wpt:  "css/css-flexbox/flexbox_direction-row-reverse.html",
		spec: "css-flexbox-1 §5.1",
		css:  `#c { display: flex; flex-direction: row-reverse; width: 300px; height: 50px; }`,
		tree: nBox("#c", nLeaf("#a", 50, 50), nLeaf("#b", 50, 50), nLeaf("#d", 50, 50)),
		want: []confCheck{wantRect("a", 250, 0, 50, 50), wantRect("b", 200, 0, 50, 50), wantRect("d", 150, 0, 50, 50)},
	},
	{
		name: "flex-direction column grows the main axis",
		spec: "css-flexbox-1 §5.1, §9.7",
		css: `#c { display: flex; flex-direction: column; width: 100px; height: 200px; }
			#a { flex-grow: 1; }`,
		tree: nBox("#c", nLeaf("#a", 20, 20), nLeaf("#b", 20, 20)),
		want: []confCheck{wantRect("a", 0, 0, 100, 180), wantRect("b", 0, 180, 100, 20)},
	},
	{
		name: "flex-direction column-reverse",
		spec: "css-flexbox-1 §5.1",
		css:  `#c { display: flex; flex-direction: column-reverse; width: 50px; height: 300px; }`,
		tree: nBox("#c", nLeaf("#a", 50, 50), nLeaf("#b", 50, 50), nLeaf("#d", 50, 50)),
		want: []confCheck{wantRect("a", 0, 250, 50, 50), wantRect("b", 0, 200, 50, 50), wantRect("d", 0, 150, 50, 50)},
	},
	{
		name: "align-items center and align-self",
		wpt:  "css/css-flexbox/flexbox_align-items-center.html",
		spec: "css-flexbox-1 §8.3",
		css: `#c { display: flex; align-items: center; width: 300px; height: 100px; }
			#e { align-self: flex-end; } #s { align-self: flex-start; }`,
		tree: nBox("#c", nLeaf("#a", 20, 20), nLeaf("#b", 20, 40), nLeaf("#e", 20, 20), nLeaf("#s", 20, 20)),
		want: []confCheck{
			wantRect("a", 0, 40, 20, 20), wantRect("b", 20, 30, 20, 40),
			wantRect("e", 40, 80, 20, 20), wantRect("s", 60, 0, 20, 20),
		},
	},
	{
		name: "align-items baseline",
		wpt:  "css/css-flexbox/flexbox_align-items-baseline.html",
		spec: "css-flexbox-1 §8.3, §9.4 step 8",
		css:  `#c { display: flex; align-items: baseline; width: 300px; height: 100px; }`,
		tree: nBox("#c", nBase("#a", 20, 20, 10), nBase("#b", 20, 40, 30)),
		want: []confCheck{wantRect("a", 0, 20, 20, 20), wantRect("b", 20, 0, 20, 40)},
	},
	{
		name: "align-items stretch honors max-height",
		wpt:  "css/css-flexbox/flexbox_align-items-stretch.html",
		spec: "css-flexbox-1 §9.4 step 11",
		css: `#c { display: flex; width: 300px; height: 100px; }
			#m { max-height: 30px; }`,
		tree: nBox("#c", nLeaf("#a", 20, 20), nLeaf("#m", 20, 20)),
		want: []confCheck{wantRect("a", 0, 0, 20, 100), wantRect("m", 20, 0, 20, 30)},
	},
	{
		name: "align-content center and space-between",
		wpt:  "css/css-flexbox/flexbox_align-content-center.html",
		spec: "css-flexbox-1 §8.4",
		css: `#c, #s { display: flex; flex-wrap: wrap; width: 100px; height: 200px; }
			#c { align-content: center; } #s { align-content: space-between; }`,
		tree: nBox("#o",
			nBox("#c", nLeaf("#a", 60, 20), nLeaf("#b", 60, 20), nLeaf("#d", 60, 20)),
			nBox("#s", nLeaf("#a2", 60, 20), nLeaf("#b2", 60, 20), nLeaf("#d2", 60, 20))),
		want: []confCheck{
			wantRect("a", 0, 70, 60, 20), wantRect("b", 0, 90, 60, 20), wantRect("d", 0, 110, 60, 20),
			wantRect("a2", 100, 0, 60, 20), wantRect("b2", 100, 90, 60, 20), wantRect("d2", 100, 180, 60, 20),
		},
	},
	{
		name: "order reorders layout and paint",
		wpt:  "css/css-flexbox/order_value.html",
		spec: "css-flexbox-1 §5.4",
		css: `#c { display: flex; width: 300px; height: 50px; }
			#a { order: 2; background-color: #ff0000; width: 50px; }
			#b { order: -1; background-color: #0000ff; width: 50px; }
			#d { width: 50px; background-color: #008000; }`,
		tree: nBox("#c", nBox("#a"), nBox("#b"), nBox("#d")),
		want: []confCheck{
			wantRect("b", 0, 0, 50, 50), wantRect("d", 50, 0, 50, 50), wantRect("a", 100, 0, 50, 50),
			wantPixel(25, 25, layBlue), wantPixel(125, 25, layRed),
		},
	},
	{
		name: "auto margins take the free space first",
		wpt:  "css/css-flexbox/flexbox_margin-auto.html",
		spec: "css-flexbox-1 §8.1",
		css: `#c, #m { display: flex; width: 300px; height: 100px; justify-content: center; }
			#b { margin-left: auto; } #k { margin: auto; }`,
		tree: nCol("#o",
			nBox("#c", nLeaf("#a", 50, 50), nLeaf("#b", 50, 50)),
			nBox("#m", nLeaf("#k", 50, 50))),
		want: []confCheck{wantRect("a", 0, 0, 50, 100), wantRect("b", 250, 0, 50, 100), wantRect("k", 125, 125, 50, 50)},
	},
	{
		name: "column-gap between items",
		spec: "css-align-3 §8.1",
		css:  `#c { display: flex; column-gap: 30px; width: 300px; height: 20px; } #c > * { flex: 1; }`,
		tree: nBox("#c", nLeaf("#a", 10, 20), nLeaf("#b", 10, 20), nLeaf("#d", 10, 20)),
		want: []confCheck{wantRect("a", 0, 0, 80, 20), wantRect("b", 110, 0, 80, 20), wantRect("d", 220, 0, 80, 20)},
	},
	{
		name: "flex-flow column wrap",
		spec: "css-flexbox-1 §5.3, §9.3",
		css:  `#c { display: flex; flex-flow: column wrap; align-content: flex-start; width: 200px; height: 100px; }`,
		tree: nBox("#c", nLeaf("#a", 40, 40), nLeaf("#b", 40, 40), nLeaf("#d", 40, 40)),
		want: []confCheck{wantRect("a", 0, 0, 40, 40), wantRect("b", 0, 40, 40, 40), wantRect("d", 40, 0, 40, 40)},
	},
	{
		name: "inline-flex shrinks to its content",
		spec: "css-flexbox-1 §3, §9.9.1",
		css: `#o { display: block; width: 300px; font-size: 0; }
			#c { display: inline-flex; }`,
		tree: nBox("#o", nBox("#c", nLeaf("#a", 30, 20), nLeaf("#b", 40, 20))),
		want: []confCheck{wantRect("c", 0, 0, 70, 20), wantRect("b", 30, 0, 40, 20)},
	},
	{
		name: "visibility collapse keeps a strut",
		wpt:  "css/css-flexbox/flexbox_visibility-collapse.html",
		spec: "css-flexbox-1 §4.4",
		css: `#c { display: flex; align-items: flex-start; }
			#b { visibility: collapse; }`,
		tree: nBox("#c", nLeaf("#a", 50, 20), nLeaf("#b", 50, 80), nLeaf("#d", 50, 20)),
		want: []confCheck{wantRect("a", 0, 0, 50, 20), wantRect("d", 50, 0, 50, 20), wantRect("c", 0, 0, 400, 80)},
	},
	{
		name: "safe and unsafe overflow alignment",
		spec: "css-align-3 §4.4",
		css: `#s, #u { display: flex; width: 100px; height: 20px; } #s > *, #u > * { flex-shrink: 0; }
			#s { justify-content: safe center; } #u { justify-content: unsafe center; }`,
		tree: nCol("#o",
			nBox("#s", nLeaf("#a", 80, 20), nLeaf("#b", 80, 20)),
			nBox("#u", nLeaf("#a2", 80, 20), nLeaf("#b2", 80, 20))),
		want: []confCheck{
			wantRect("a", 0, 0, 80, 20), wantRect("b", 80, 0, 80, 20),
			wantRect("a2", -30, 20, 80, 20), wantRect("b2", 50, 20, 80, 20),
		},
	},
	{
		name: "z-index orders flex items without position",
		wpt:  "css/css-flexbox/flexbox-paint-ordering-001.xhtml",
		spec: "css-flexbox-1 §4.3",
		css: `#c { display: flex; width: 200px; height: 50px; } #c > * { width: 50px; }
			#a { z-index: 1; background-color: #ff0000; }
			#b { margin-left: -25px; background-color: #0000ff; }`,
		tree: nBox("#c", nBox("#a"), nBox("#b")),
		want: []confCheck{wantRect("b", 25, 0, 50, 50), wantPixel(35, 25, layRed), wantPixel(60, 25, layBlue), wantHit(35, 25, "a")},
	},

	// Grid.
	{
		name: "grid fixed, fr and auto tracks",
		wpt:  "css/css-grid/grid-definition/fr-unit.html",
		spec: "css-grid-2 §7.2.4, §12.7",
		css:  `#g { display: grid; grid-template-columns: 100px 1fr 2fr auto; height: 200px; }`,
		tree: nBox("#g", nLeaf("#a", 30, 20), nLeaf("#b", 30, 20), nLeaf("#c", 30, 20), nLeaf("#d", 30, 20)),
		want: []confCheck{
			wantRect("a", 0, 0, 100, 200), wantRect("b", 100, 0, 90, 200),
			wantRect("c", 190, 0, 180, 200), wantRect("d", 370, 0, 30, 200),
		},
	},
	{
		name: "grid row and column gaps",
		spec: "css-align-3 §8, css-grid-2 §10.1",
		css:  `#g { display: grid; width: 340px; grid-template-columns: repeat(3, 1fr); gap: 10px 20px; }`,
		tree: nBox("#g", nLeaf("#a", 30, 20), nLeaf("#b", 30, 20), nLeaf("#c", 30, 20), nLeaf("#d", 30, 20)),
		want: []confCheck{
			wantRect("a", 0, 0, 100, 20), wantRect("b", 120, 0, 100, 20),
			wantRect("c", 240, 0, 100, 20), wantRect("d", 0, 30, 100, 20),
		},
	},
	{
		name: "repeat(auto-fill) fits the definite width",
		wpt:  "css/css-grid/grid-definition/grid-auto-fill-columns-001.html",
		spec: "css-grid-2 §7.2.3.2",
		css:  `#g { display: grid; width: 350px; grid-template-columns: repeat(auto-fill, 100px); }`,
		tree: nBox("#g", nLeaf("#a", 10, 20), nLeaf("#b", 10, 20), nLeaf("#c", 10, 20), nLeaf("#d", 10, 20)),
		want: []confCheck{wantRect("c", 200, 0, 100, 20), wantRect("d", 0, 20, 100, 20)},
	},
	{
		name: "repeat(auto-fit) collapses empty tracks",
		wpt:  "css/css-grid/grid-definition/grid-auto-fit-columns-001.html",
		spec: "css-grid-2 §7.2.3.2",
		css: `#f, #l { display: grid; width: 400px; justify-content: center; }
			#f { grid-template-columns: repeat(auto-fit, 100px); }
			#l { grid-template-columns: repeat(auto-fill, 100px); }`,
		tree: nCol("#o",
			nBox("#f", nLeaf("#a", 10, 20), nLeaf("#b", 10, 20)),
			nBox("#l", nLeaf("#a2", 10, 20), nLeaf("#b2", 10, 20))),
		want: []confCheck{
			wantRect("a", 100, 0, 100, 20), wantRect("b", 200, 0, 100, 20),
			wantRect("a2", 0, 20, 100, 20), wantRect("b2", 100, 20, 100, 20),
		},
	},
	{
		name: "repeat(auto-fill, minmax()) shares the leftover",
		spec: "css-grid-2 §7.2.3.2, §12.7",
		css:  `#g { display: grid; width: 360px; grid-template-columns: repeat(auto-fill, minmax(100px, 1fr)); }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10), nLeaf("#c", 10, 10), nLeaf("#d", 10, 10)),
		want: []confCheck{wantRect("b", 120, 0, 120, 10), wantRect("c", 240, 0, 120, 10), wantRect("d", 0, 10, 120, 10)},
	},
	{
		name: "grid-template-areas places named areas",
		spec: "css-grid-2 §7.3",
		css: `#g { display: grid; grid-template-columns: 50px 50px 50px; grid-template-rows: 20px 30px;
				grid-template-areas: "a a b" "c d d"; }
			#a { grid-area: a; } #b { grid-area: b; } #c { grid-area: c; } #d { grid-area: d; }`,
		tree: nBox("#g", nLeaf("#d", 10, 10), nLeaf("#c", 10, 10), nLeaf("#b", 10, 10), nLeaf("#a", 10, 10)),
		want: []confCheck{
			wantRect("a", 0, 0, 100, 20), wantRect("b", 100, 0, 50, 20),
			wantRect("c", 0, 20, 50, 30), wantRect("d", 50, 20, 100, 30),
		},
	},
	{
		name: "line numbers, negative lines and spans",
		spec: "css-grid-2 §8.3",
		css: `#g { display: grid; grid-template-columns: repeat(4, 50px); grid-template-rows: 20px; }
			#s { grid-column: 2 / span 2; } #n { grid-column: 1 / -1; grid-row: 2; }`,
		tree: nBox("#g", nLeaf("#s", 10, 10), nLeaf("#n", 10, 10)),
		want: []confCheck{wantRect("s", 50, 0, 100, 20), wantRect("n", 0, 20, 200, 10)},
	},
	{
		name: "named grid lines",
		wpt:  "css/css-grid/placement/grid-placement-using-named-grid-lines-001.html",
		spec: "css-grid-2 §7.2.2, §8.3",
		css: `#g { display: grid; grid-template-columns: [left] 50px [mid] 100px [right]; grid-template-rows: 20px; }
			#m { grid-column: mid / right; } #l { grid-column: left; grid-row: 2; }`,
		tree: nBox("#g", nLeaf("#m", 10, 10), nLeaf("#l", 10, 10)),
		want: []confCheck{wantRect("m", 50, 0, 100, 20), wantRect("l", 0, 20, 50, 10)},
	},
	{
		name: "sparse auto-placement never backtracks",
		wpt:  "css/css-grid/placement/grid-auto-flow-sparse-001.html",
		spec: "css-grid-2 §8.5",
		css: `#g { display: grid; grid-template-columns: repeat(3, 50px); grid-auto-rows: 50px; }
			#a, #b { grid-column: span 2; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10), nLeaf("#c", 10, 10)),
		want: []confCheck{wantRect("a", 0, 0, 100, 50), wantRect("b", 0, 50, 100, 50), wantRect("c", 100, 50, 50, 50)},
	},
	{
		name: "dense auto-placement fills holes",
		wpt:  "css/css-grid/placement/grid-auto-flow-dense-001.html",
		spec: "css-grid-2 §7.7, §8.5",
		css: `#g { display: grid; grid-auto-flow: row dense; grid-template-columns: repeat(3, 50px); grid-auto-rows: 50px; }
			#a, #b { grid-column: span 2; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10), nLeaf("#c", 10, 10)),
		want: []confCheck{wantRect("a", 0, 0, 100, 50), wantRect("b", 0, 50, 100, 50), wantRect("c", 100, 0, 50, 50)},
	},
	{
		name: "grid-auto-flow column with implicit columns",
		spec: "css-grid-2 §7.6, §7.7",
		css:  `#g { display: grid; grid-auto-flow: column; grid-template-rows: repeat(2, 30px); grid-auto-columns: 40px; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10), nLeaf("#c", 10, 10)),
		want: []confCheck{wantRect("a", 0, 0, 40, 30), wantRect("b", 0, 30, 40, 30), wantRect("c", 40, 0, 40, 30)},
	},
	{
		name: "grid-auto-rows sizes implicit rows",
		spec: "css-grid-2 §7.6",
		css:  `#g { display: grid; grid-template-columns: 50px 50px; grid-auto-rows: 30px; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10), nLeaf("#c", 10, 10)),
		want: []confCheck{wantRect("b", 50, 0, 50, 30), wantRect("c", 0, 30, 50, 30)},
	},
	{
		name: "justify-items center and align-self end",
		spec: "css-align-3 §6.1, §6.2",
		css: `#g { display: grid; grid-template-columns: 100px; grid-template-rows: 50px; justify-items: center; }
			#a { align-self: end; }`,
		tree: nBox("#g", nLeaf("#a", 30, 20)),
		want: []confCheck{wantRect("a", 35, 30, 30, 20)},
	},
	{
		name: "place-items center",
		spec: "css-align-3 §6.3",
		css:  `#g { display: grid; grid-template-columns: 100px; grid-template-rows: 50px; place-items: center; }`,
		tree: nBox("#g", nLeaf("#a", 30, 20)),
		want: []confCheck{wantRect("a", 35, 15, 30, 20)},
	},
	{
		name: "justify-content distributes tracks",
		wpt:  "css/css-grid/alignment/grid-content-distribution-001.html",
		spec: "css-align-3 §5.3, css-grid-2 §10.5",
		css:  `#g { display: grid; width: 300px; grid-template-columns: 50px 50px; justify-content: space-between; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10)),
		want: []confCheck{wantRect("a", 0, 0, 50, 10), wantRect("b", 250, 0, 50, 10)},
	},
	{
		name: "minmax() tracks maximize equally",
		spec: "css-grid-2 §7.2.4, §12.6",
		css:  `#g { display: grid; width: 250px; grid-template-columns: minmax(100px, 200px) minmax(100px, 200px); }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10)),
		want: []confCheck{wantRect("a", 0, 0, 125, 10), wantRect("b", 125, 0, 125, 10)},
	},
	{
		name: "max-content and fit-content() tracks",
		spec: "css-grid-2 §7.2.4, §12.5",
		css: `#m, #f, #k { display: grid; width: 400px; }
			#m { grid-template-columns: max-content 1fr; }
			#f, #k { grid-template-columns: fit-content(50px) 1fr; }`,
		tree: nCol("#o",
			nBox("#m", nLeaf("#m1", 70, 10), nLeaf("#m2", 10, 10)),
			nBox("#f", nLeaf("#f1", 30, 10), nLeaf("#f2", 10, 10)),
			nBox("#k", nLeaf("#k1", 80, 10), nLeaf("#k2", 10, 10))),
		want: []confCheck{
			wantRect("m1", 0, 0, 70, 10), wantRect("m2", 70, 0, 330, 10),
			wantRect("f1", 0, 10, 30, 10), wantRect("f2", 30, 10, 370, 10),
			wantRect("k1", 0, 20, 80, 10), wantRect("k2", 80, 20, 320, 10),
		},
	},
	{
		name: "spanning items grow intrinsic tracks",
		spec: "css-grid-2 §12.5.1 step 3",
		css: `#g { display: grid; grid-template-columns: auto auto; justify-content: start; }
			#s { grid-column: 1 / span 2; } #b { grid-row: 2; grid-column: 1; } #c { grid-row: 2; grid-column: 2; }`,
		tree: nBox("#g", nLeaf("#s", 200, 10), nLeaf("#b", 30, 10), nLeaf("#c", 30, 10)),
		want: []confCheck{wantRect("s", 0, 0, 200, 10), wantRect("b", 0, 10, 100, 10), wantRect("c", 100, 10, 100, 10)},
	},
	{
		name: "order modifies auto-placement",
		spec: "css-grid-2 §8.5, css-display-3 §3",
		css:  `#g { display: grid; grid-template-columns: 50px 50px; } #a { order: 1; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10)),
		want: []confCheck{wantRect("b", 0, 0, 50, 10), wantRect("a", 50, 0, 50, 10)},
	},
	{
		name: "absolute child resolves its grid area",
		wpt:  "css/css-grid/abspos/positioned-grid-items-001.html",
		spec: "css-grid-2 §9.1",
		css: `#g { display: grid; position: relative; grid-template-columns: 50px 100px; grid-template-rows: 40px; }
			#p { position: absolute; grid-column: 2 / 3; grid-row: 1 / 2; top: 0; right: 0; bottom: 0; left: 0; }`,
		tree: nBox("#g", nLeaf("#x", 10, 10), nLeaf("#p", 10, 10)),
		want: []confCheck{wantRect("p", 50, 0, 100, 40), wantRect("x", 0, 0, 50, 40)},
	},
	{
		name: "rtl grid runs columns from the right",
		spec: "css-grid-2 §7.1, css-writing-modes-4 §2.1",
		css:  `#g { display: grid; direction: rtl; width: 300px; grid-template-columns: 50px 100px; grid-template-rows: 20px; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10)),
		want: []confCheck{wantRect("a", 250, 0, 50, 20), wantRect("b", 150, 0, 100, 20)},
	},
	{
		name: "grid-template shorthand",
		spec: "css-grid-2 §7.4",
		css: `#g { display: grid; grid-template: "a b" 30px "c d" 40px / 60px 80px; }
			#a { grid-area: a; } #d { grid-area: d; }`,
		tree: nBox("#g", nLeaf("#d", 10, 10), nLeaf("#a", 10, 10)),
		want: []confCheck{wantRect("a", 0, 0, 60, 30), wantRect("d", 60, 30, 80, 40)},
	},
	{
		name: "grid shorthand with auto-flow",
		spec: "css-grid-2 §7.8",
		css:  `#g { display: grid; grid: auto-flow 30px / 50px 50px; }`,
		tree: nBox("#g", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10), nLeaf("#c", 10, 10)),
		want: []confCheck{wantRect("b", 50, 0, 50, 30), wantRect("c", 0, 30, 50, 30)},
	},
	{
		name: "inline-grid shrinks to max-content",
		spec: "css-grid-2 §5.2, css-display-3 §2",
		css: `#o { display: block; width: 300px; font-size: 0; }
			#g { display: inline-grid; grid-template-columns: auto auto; }`,
		tree: nBox("#o", nBox("#g", nLeaf("#a", 30, 20), nLeaf("#b", 40, 20))),
		want: []confCheck{wantRect("g", 0, 0, 70, 20), wantRect("b", 30, 0, 40, 20)},
	},
	{
		name: "place-content and place-self",
		spec: "css-align-3 §5.4, §6.3",
		css: `#g { display: grid; width: 300px; height: 100px; grid-template-columns: 100px; grid-template-rows: 40px;
				place-content: end center; }
			#a { place-self: end start; }`,
		tree: nBox("#g", nLeaf("#a", 30, 20)),
		want: []confCheck{wantRect("a", 100, 80, 30, 20)},
	},
	{
		name: "grid-area with four lines and row spans",
		spec: "css-grid-2 §8.3, §8.4",
		css: `#g { display: grid; grid-template-columns: repeat(4, 25px); grid-template-rows: repeat(3, 10px); }
			#a { grid-area: 2 / 2 / 4 / 5; } #b { grid-row: span 2; grid-column: 1; }`,
		tree: nBox("#g", nLeaf("#a", 5, 5), nLeaf("#b", 5, 5)),
		want: []confCheck{wantRect("a", 25, 10, 75, 20), wantRect("b", 0, 0, 25, 20)},
	},
	{
		name: "grid items share a row baseline",
		spec: "css-grid-2 §10.6, css-align-3 §9",
		css:  `#g { display: grid; grid-template-columns: 50px 50px; align-items: baseline; }`,
		tree: nBox("#g", nBase("#a", 20, 20, 10), nBase("#b", 20, 40, 30)),
		want: []confCheck{wantRect("a", 0, 20, 50, 20), wantRect("b", 50, 0, 50, 40)},
	},

	// Display.
	{
		name: "display none leaves layout, paint and hits",
		spec: "css-display-3 §2.5",
		css: `#c { display: flex; width: 300px; height: 50px; background-color: #0000ff; }
			#c > * { width: 50px; background-color: #ff0000; } #n { display: none; }`,
		tree: nBox("#c", nBox("#a"), nBox("#n"), nBox("#d")),
		want: []confCheck{
			wantRect("d", 50, 0, 50, 50),
			wantPixel(75, 25, layRed), wantPixel(125, 25, layBlue),
			wantHit(75, 25, "d"),
		},
	},
	{
		name: "display contents dissolves a box into its parent",
		spec: "css-display-3 §2.5",
		css: `#c { display: flex; width: 300px; height: 50px; }
			#w { display: contents; background-color: #ff0000; }`,
		tree: nBox("#c", nLeaf("#a", 50, 50), nCol("#w", nLeaf("#b", 50, 50), nLeaf("#d", 50, 50))),
		want: []confCheck{
			wantRect("b", 50, 0, 50, 50), wantRect("d", 100, 0, 50, 50),
			wantPixel(75, 25, layWhite),
		},
	},
	{
		name: "display block stacks a row box's children",
		spec: "css-display-3 §2.1, CSS2 §9.4.1",
		css:  `#c { display: block; width: 100px; }`,
		tree: nBox("#c", nLeaf("#a", 30, 20), nLeaf("#b", 30, 20)),
		want: []confCheck{wantRect("a", 0, 0, 100, 20), wantRect("b", 0, 20, 100, 20), wantRect("c", 0, 0, 100, 40)},
	},

	// Normal flow.
	{
		name: "adjacent sibling margins collapse",
		wpt:  "css/CSS2/margin-padding-clear/margin-collapse-001.xht",
		spec: "CSS2 §8.3.1",
		css: `#c { display: block; width: 200px; } #c > * { height: 20px; }
			#a { margin-bottom: 20px; } #b { margin-top: 30px; margin-bottom: 20px; } #d { margin-top: -5px; }`,
		tree: nBox("#c", nBox("#a"), nBox("#b"), nBox("#d")),
		want: []confCheck{wantRect("b", 0, 50, 200, 20), wantRect("d", 0, 85, 200, 20), wantRect("c", 0, 0, 200, 105)},
	},
	{
		name: "auto horizontal margins center a block",
		spec: "CSS2 §10.3.3",
		css: `#c { display: block; width: 300px; }
			#a { width: 100px; margin: 0 auto; } #b { width: 100px; margin-left: auto; }`,
		tree: nBox("#c", nLeaf("#a", 10, 20), nLeaf("#b", 10, 20)),
		want: []confCheck{wantRect("a", 100, 0, 100, 20), wantRect("b", 200, 20, 100, 20)},
	},
	{
		name: "inline-blocks fill line boxes",
		spec: "CSS2 §9.4.2, css-text-3 §7.1",
		css: `#c { display: block; width: 100px; font-size: 0; text-align: center; }
			#c > * { display: inline-block; }`,
		tree: nBox("#c", nLeaf("#a", 40, 20), nLeaf("#b", 40, 20), nLeaf("#d", 40, 20)),
		want: []confCheck{wantRect("a", 10, 0, 40, 20), wantRect("b", 50, 0, 40, 20), wantRect("d", 30, 20, 40, 20)},
	},
	{
		name: "inline-blocks align on the baseline",
		spec: "CSS2 §10.8.1",
		css: `#c { display: block; width: 200px; font-size: 0; }
			#c > * { display: inline-block; }`,
		tree: nBox("#c", nBase("#a", 20, 20, 15), nBase("#b", 20, 40, 30)),
		want: []confCheck{wantRect("a", 0, 15, 20, 20), wantRect("b", 20, 0, 20, 40)},
	},
	{
		name: "floats pull to the sides and blocks flow beside",
		wpt:  "css/CSS2/floats/floats-001.xht",
		spec: "CSS2 §9.5.1",
		css: `#c { display: block; width: 200px; }
			#l { float: left; width: 50px; height: 50px; } #r { float: right; width: 50px; height: 50px; }
			#l2 { float: left; width: 30px; height: 30px; }`,
		tree: nBox("#c", nLeaf("#l", 10, 10), nLeaf("#r", 10, 10), nLeaf("#l2", 10, 10), nLeaf("#b", 10, 20)),
		want: []confCheck{
			wantRect("l", 0, 0, 50, 50), wantRect("r", 150, 0, 50, 50), wantRect("l2", 50, 0, 30, 30),
			wantRect("b", 80, 0, 70, 20), wantRect("c", 0, 0, 200, 50),
		},
	},
	{
		name: "clear moves a block below the floats",
		wpt:  "css/CSS2/floats-clear/clear-001.xht",
		spec: "CSS2 §9.5.2",
		css: `#c { display: block; width: 200px; }
			#l { float: left; width: 50px; height: 50px; } #r { float: right; width: 50px; height: 70px; }
			#b { clear: left; } #k { clear: both; }`,
		tree: nBox("#c", nLeaf("#l", 10, 10), nLeaf("#r", 10, 10), nLeaf("#b", 10, 10), nLeaf("#k", 10, 10)),
		want: []confCheck{wantRect("b", 0, 50, 150, 10), wantRect("k", 0, 70, 200, 10)},
	},
	{
		name: "logical float sides follow the direction",
		wpt:  "css/css-logical/logical-values-float-clear-1.html",
		spec: "css-logical-1 §3.1",
		css: `#c { display: block; direction: rtl; width: 200px; }
			#s { float: inline-start; width: 50px; height: 50px; } #e { float: inline-end; width: 30px; height: 30px; }`,
		tree: nBox("#c", nLeaf("#s", 10, 10), nLeaf("#e", 10, 10)),
		want: []confCheck{wantRect("s", 150, 0, 50, 50), wantRect("e", 0, 0, 30, 30)},
	},

	// Sizing.
	{
		name: "box-sizing picks the box width measures",
		wpt:  "css/css-ui/box-sizing-001.html",
		spec: "css-sizing-3 §4.1 (box-sizing)",
		css: `#c, #b { width: 100px; height: 40px; padding: 10px; border: 5px solid #000000; }
			#b { box-sizing: border-box; }`,
		tree: nCol("#o", nBox("#c"), nBox("#b")),
		want: []confCheck{wantRect("c", 0, 0, 130, 70), wantRect("b", 0, 70, 100, 40)},
	},
	{
		name: "min-width wins over max-width and width",
		spec: "CSS2 §10.4",
		css:  `#a { width: 100px; max-width: 50px; min-width: 80px; height: 10px; } #b { width: 100px; max-width: 60px; height: 10px; }`,
		tree: nCol("#o", nBox("#a"), nBox("#b")),
		want: []confCheck{wantRect("a", 0, 0, 80, 10), wantRect("b", 0, 10, 60, 10)},
	},
	{
		name: "percentage and calc() widths",
		spec: "css-values-4 §10.1, CSS2 §10.2",
		css: `#c { display: block; width: 300px; height: 100px; }
			#a { width: 50%; height: 25%; } #b { width: calc(100% - 20px); height: 10px; }`,
		tree: nBox("#c", nBox("#a"), nBox("#b")),
		want: []confCheck{wantRect("a", 0, 0, 150, 25), wantRect("b", 0, 25, 280, 10)},
	},
	{
		name: "aspect-ratio transfers a definite width",
		wpt:  "css/css-sizing/aspect-ratio/block-aspect-ratio-001.html",
		spec: "css-sizing-4 §2",
		css:  `#a { width: 100px; aspect-ratio: 2 / 1; } #b { width: 100px; padding: 10px; aspect-ratio: 1; }`,
		tree: nCol("#o", nBox("#a"), nBox("#b")),
		want: []confCheck{wantRect("a", 0, 0, 100, 50), wantRect("b", 0, 50, 120, 120)},
	},
	{
		name: "min-content, max-content and fit-content widths",
		spec: "css-sizing-3 §3.2, §5.1",
		css: `#mn, #mx, #ft { display: block; height: 10px; }
			#mn { width: min-content; } #mx { width: max-content; } #ft { width: fit-content; }`,
		tree: nCol("#o",
			nBox("#mn", nLeaf("#a", 60, 5), nLeaf("#b", 90, 5)),
			nBox("#mx", nLeaf("#c", 60, 5), nLeaf("#d", 90, 5)),
			nBox("#ft", nLeaf("#e", 60, 5), nLeaf("#f", 90, 5))),
		want: []confCheck{wantRect("mn", 0, 0, 90, 10), wantRect("mx", 0, 10, 90, 10), wantRect("ft", 0, 20, 90, 10)},
	},
	{
		name: "aspect-ratio transfers a definite height",
		spec: "css-sizing-4 §2",
		css:  `#a { height: 50px; aspect-ratio: 2; }`,
		tree: nBox("#a"),
		want: []confCheck{wantRect("a", 0, 0, 100, 50)},
	},
	{
		name: "sizing applies to every widget",
		spec: "css-sizing-3 §3.1",
		css: `#b { width: 120px; height: 40px; } #t { width: 90px; height: 30px; }
			#e { width: 150px; min-height: 50px; } #m { max-width: 30px; max-height: 15px; }`,
		tree: nCol("#o", nButton("#b", nText("#bl", "go")), nText("#t", "label"), nEntry("#e", "x"), nLeaf("#m", 60, 60)),
		want: []confCheck{
			wantRect("b", 0, 0, 120, 40), wantRect("t", 0, 40, 90, 30),
			wantRect("e", 0, 70, 150, 50), wantRect("m", 0, 120, 30, 15),
		},
	},
	{
		name: "content keywords on a wrapping flex container",
		spec: "css-sizing-3 §5.1, css-flexbox-1 §9.9.1",
		css: `#o > * { display: flex; flex-wrap: wrap; height: 10px; }
			#mn { width: min-content; } #mx { width: max-content; } #fc { width: fit-content(100px); }`,
		tree: nCol("#o",
			nBox("#mn", nLeaf("#a", 60, 5), nLeaf("#b", 90, 5)),
			nBox("#mx", nLeaf("#c", 60, 5), nLeaf("#d", 90, 5)),
			nBox("#fc", nLeaf("#e", 60, 5), nLeaf("#f", 90, 5))),
		want: []confCheck{wantRect("mn", 0, 0, 90, 10), wantRect("mx", 0, 10, 150, 10), wantRect("fc", 0, 20, 100, 10)},
	},
	{
		name: "min-content of inline content breaks between items",
		spec: "css-sizing-3 §5.1, CSS2 §10.3.5",
		css: `#c { display: block; width: min-content; font-size: 0; } #c > * { display: inline-block; }
			#k { display: block; width: max-content; font-size: 0; } #k > * { display: inline-block; }`,
		tree: nCol("#o",
			nBox("#c", nLeaf("#a", 60, 10), nLeaf("#b", 90, 10)),
			nBox("#k", nLeaf("#d", 60, 10), nLeaf("#e", 90, 10))),
		want: []confCheck{
			wantRect("c", 0, 0, 90, 20), wantRect("b", 0, 10, 90, 10),
			wantRect("k", 0, 20, 150, 10), wantRect("e", 60, 20, 90, 10),
		},
	},

	// Positioning.
	{
		name: "relative offsets leave siblings in place",
		wpt:  "css/CSS2/positioning/relpos-calcs-001.xht",
		spec: "CSS2 §9.4.3",
		css: `#c { display: block; width: 200px; }
			#a { position: relative; left: 10px; top: 20px; right: 50px; } #b { position: relative; bottom: 5px; }`,
		tree: nBox("#c", nLeaf("#a", 10, 20), nLeaf("#b", 10, 20), nLeaf("#d", 10, 20)),
		want: []confCheck{wantRect("a", 10, 20, 200, 20), wantRect("b", 0, 15, 200, 20), wantRect("d", 0, 40, 200, 20)},
	},
	{
		name: "absolute insets against the padding box",
		spec: "CSS2 §10.1, §10.3.7",
		css: `#c { display: block; position: relative; width: 200px; height: 100px; border: 5px solid #000000; }
			#a { position: absolute; top: 10px; left: 20px; width: 30px; height: 40px; }
			#b { position: absolute; right: 10px; bottom: 10px; width: 30px; height: 30px; }`,
		tree: nBox("#c", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10)),
		want: []confCheck{wantRect("a", 25, 15, 30, 40), wantRect("b", 165, 65, 30, 30)},
	},
	{
		name: "absolute with both insets stretches or centers",
		wpt:  "css/css-position/position-absolute-center-001.html",
		spec: "CSS2 §10.3.7, §10.6.4",
		css: `#c { display: block; position: relative; width: 200px; height: 100px; }
			#s { position: absolute; left: 10px; right: 10px; top: 0; bottom: 0; }
			#m { position: absolute; left: 0; right: 0; top: 0; bottom: 0; width: 50px; height: 20px; margin: auto; }`,
		tree: nBox("#c", nLeaf("#s", 10, 10), nLeaf("#m", 10, 10)),
		want: []confCheck{wantRect("s", 10, 0, 180, 100), wantRect("m", 75, 40, 50, 20)},
	},
	{
		name: "inset shorthand and percentage insets",
		spec: "css-position-3 §3.1",
		css: `#c { display: block; position: relative; width: 200px; height: 100px; }
			#a { position: absolute; inset: 10px 20px; }
			#b { position: absolute; left: 50%; top: 50%; width: 10px; height: 10px; }`,
		tree: nBox("#c", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10)),
		want: []confCheck{wantRect("a", 20, 10, 160, 80), wantRect("b", 100, 50, 10, 10)},
	},
	{
		name: "fixed positions against the viewport",
		spec: "css-position-3 §2.1",
		css: `#c { display: block; position: relative; width: 100px; height: 100px; margin-left: 50px; }
			#f { position: fixed; top: 0; right: 0; width: 40px; height: 30px; }`,
		tree: nBox("#c", nLeaf("#f", 10, 10)),
		want: []confCheck{wantRect("f", 360, 0, 40, 30)},
	},
	{
		name: "z-index orders paint and hit testing",
		spec: "CSS2 §9.9.1, Appendix E",
		css: `#c { display: block; position: relative; width: 200px; height: 100px; }
			#c > * { position: absolute; top: 0; width: 60px; height: 60px; }
			#a { left: 0; z-index: 2; background-color: #008000; }
			#b { left: 30px; z-index: 1; background-color: #ff0000; }`,
		tree: nBox("#c", nBox("#a"), nBox("#b")),
		want: []confCheck{
			wantPixel(45, 30, layGreen), wantPixel(75, 30, layRed),
			wantHit(45, 30, "a"), wantHit(75, 30, "b"),
		},
	},
	{
		name: "negative z-index paints under in-flow content",
		spec: "CSS2 Appendix E.2",
		css: `#c { display: block; position: relative; width: 200px; }
			#n { position: absolute; top: 0; left: 0; width: 100px; height: 50px; z-index: -1; background-color: #ff0000; }
			#i { width: 50px; height: 50px; background-color: #008000; }`,
		tree: nBox("#c", nBox("#n"), nBox("#i")),
		want: []confCheck{wantPixel(25, 25, layGreen), wantPixel(75, 25, layRed), wantHit(25, 25, "i")},
	},
	{
		name: "sticky stays in the scrollport",
		spec: "css-position-3 §3.4",
		css:  `#k { position: sticky; top: 0; }`,
		vp:   Size{W: 100, H: 100},
		tree: nScroll("#s", nCol("#col", nLeaf("#h", 80, 20), nLeaf("#k", 80, 20), nLeaf("#b", 80, 300))),
		act:  layScroll("s", 50),
		want: []confCheck{layTop("k", 0), layTop("h", -50), layTop("b", -10)},
	},
	{
		name: "static ignores insets; an absolute box keeps its static position",
		spec: "CSS2 §9.3.1, §10.3.7, §10.6.4",
		css: `#c { display: block; position: relative; width: 200px; }
			#s { position: static; left: 30px; top: 30px; height: 20px; }
			#a { position: absolute; width: 40px; height: 10px; }`,
		tree: nBox("#c", nBox("#s"), nLeaf("#a", 10, 10), nBox("#t")),
		want: []confCheck{wantRect("s", 0, 0, 200, 20), wantRect("a", 0, 20, 40, 10), wantRect("c", 0, 0, 200, 20)},
	},
	{
		name: "absolute without a positioned ancestor uses the initial containing block",
		spec: "CSS2 §10.1",
		css: `#c { display: block; width: 100px; height: 100px; margin-left: 50px; }
			#a { position: absolute; right: 0; bottom: 0; width: 40px; height: 30px; }`,
		tree: nBox("#c", nLeaf("#a", 10, 10)),
		want: []confCheck{wantRect("a", 360, 270, 40, 30)},
	},

	// Overflow and visibility.
	{
		name: "overflow hidden clips paint and hits",
		spec: "css-overflow-3 §3",
		css: `#c { display: block; width: 50px; height: 50px; overflow: hidden; }
			#k { width: 100px; height: 100px; background-color: #ff0000; }`,
		tree: nBox("#c", nBox("#k")),
		want: []confCheck{wantPixel(25, 25, layRed), wantPixel(75, 25, layWhite), wantHit(25, 25, "k"), wantHit(75, 25, "root")},
	},
	{
		name: "overflow-x clips one axis",
		spec: "css-overflow-3 §3.1",
		css: `#c { display: block; width: 50px; height: 50px; overflow-x: clip; }
			#k { width: 100px; height: 100px; background-color: #ff0000; }`,
		tree: nBox("#c", nBox("#k")),
		want: []confCheck{wantPixel(75, 25, layWhite), wantPixel(25, 75, layRed)},
	},
	{
		name: "visibility hidden keeps the space",
		spec: "CSS2 §11.2",
		css: `#c { display: flex; width: 300px; height: 50px; background-color: #0000ff; }
			#c > * { width: 50px; background-color: #ff0000; } #h { visibility: hidden; }
			#h > * { visibility: visible; width: 20px; height: 20px; background-color: #008000; }`,
		tree: nBox("#c", nBox("#h", nBox("#v")), nBox("#d")),
		want: []confCheck{
			wantRect("d", 50, 0, 50, 50),
			wantPixel(30, 30, layBlue), wantPixel(10, 10, layGreen), wantPixel(75, 25, layRed),
			wantHit(30, 30, "c"), wantHit(10, 10, "v"),
		},
	},

	// object-fit and object-position on an Image's 40x20 picture, in a
	// 100x100 box.
	{
		name: "object-fit fill stretches the picture",
		wpt:  "css/css-images/object-fit-fill-png-001i.html",
		spec: "css-images-3 §5.5",
		css:  `#i { width: 100px; height: 100px; object-fit: fill; }`,
		tree: layImage("#i"),
		want: []confCheck{wantRect("i", 0, 0, 100, 100), wantPixel(25, 10, layRed), wantPixel(75, 90, layBlue)},
	},
	{
		name: "object-fit contain letterboxes",
		wpt:  "css/css-images/object-fit-contain-png-001i.html",
		spec: "css-images-3 §5.5",
		css:  `#i { width: 100px; height: 100px; object-fit: contain; }`,
		tree: layImage("#i"),
		want: []confCheck{wantPixel(25, 10, layWhite), wantPixel(25, 50, layRed), wantPixel(75, 50, layBlue), wantPixel(75, 90, layWhite)},
	},
	{
		name: "object-fit cover crops by object-position",
		wpt:  "css/css-images/object-fit-cover-png-001i.html",
		spec: "css-images-3 §5.5, §5.6",
		css: `#o > * { width: 100px; height: 100px; object-fit: cover; }
			#l { object-position: left; } #r { object-position: right; }`,
		tree: nBox("#o", layImage("#c"), layImage("#l"), layImage("#r")),
		want: []confCheck{
			wantPixel(25, 10, layRed), wantPixel(75, 90, layBlue),
			wantPixel(125, 10, layRed), wantPixel(175, 90, layRed),
			wantPixel(225, 10, layBlue), wantPixel(275, 90, layBlue),
		},
	},
	{
		name: "object-fit none and scale-down keep the natural size",
		wpt:  "css/css-images/object-fit-none-png-001i.html",
		spec: "css-images-3 §5.5",
		css:  `#o > * { width: 100px; height: 100px; } #n { object-fit: none; } #s { object-fit: scale-down; }`,
		tree: nBox("#o", layImage("#n"), layImage("#s")),
		want: []confCheck{
			wantPixel(35, 50, layRed), wantPixel(65, 50, layBlue), wantPixel(25, 50, layWhite), wantPixel(50, 35, layWhite),
			wantPixel(135, 50, layRed), wantPixel(165, 50, layBlue), wantPixel(125, 50, layWhite),
		},
	},
	{
		name: "object-position keywords, lengths and edge offsets",
		wpt:  "css/css-images/object-position-png-001i.html",
		spec: "css-images-3 §5.6, css-values-4 §8.3",
		css: `#o > * { width: 100px; height: 100px; object-fit: none; }
			#k { object-position: bottom right; } #p { object-position: 10px 20px; }
			#e { object-position: right 10px bottom 20px; }`,
		tree: nBox("#o", layImage("#k"), layImage("#p"), layImage("#e")),
		want: []confCheck{
			wantPixel(65, 90, layRed), wantPixel(95, 90, layBlue), wantPixel(55, 90, layWhite), wantPixel(95, 75, layWhite),
			wantPixel(115, 30, layRed), wantPixel(145, 30, layBlue), wantPixel(105, 30, layWhite), wantPixel(115, 45, layWhite),
			wantPixel(255, 70, layRed), wantPixel(285, 70, layBlue), wantPixel(295, 70, layWhite), wantPixel(255, 85, layWhite),
		},
	},
	{
		name: "a programmatic container lets an oversized child overflow",
		spec: "CSS2 §10.2, css-overflow-3 §2",
		css:  `#c { width: 50px; height: 50px; } #k { width: 100px; height: 100px; }`,
		tree: nBox("#c", nBox("#k")),
		want: []confCheck{wantRect("k", 0, 0, 100, 100)},
	},
}

// confExceptionsLayout are the Layout group's deliberate deviations.
var confExceptionsLayout = []confException{
	{
		name: "subgrid lays out as none",
		spec: "css-grid-2 §9",
		css: `#g { display: grid; grid-template-columns: 50px 100px; }
			#s { grid-column: 1 / -1; display: grid; grid-template-columns: subgrid; }`,
		tree:   nBox("#g", nBox("#s", nLeaf("#a", 10, 10), nLeaf("#b", 10, 10))),
		want:   []confCheck{wantRect("a", 0, 0, 150, 10), wantRect("b", 0, 10, 150, 10)},
		reason: "a subgrid's items size its parent's tracks, and the widget tree measures and arranges each child Box as one widget",
		doc:    "- `subgrid`: `grid-template-columns`/`grid-template-rows: subgrid`",
	},
	{
		name: "display inline is an atomic inline box",
		spec: "CSS2 §10.3.1",
		css: `#o { display: block; width: 300px; font-size: 0; }
			#i { display: inline; width: 100px; }`,
		tree:   nBox("#o", nBox("#i", nLeaf("#k", 20, 20))),
		want:   []confCheck{wantRect("i", 0, 0, 100, 20)},
		reason: "every widget is one box: there is no inline fragmentation, so an inline box sizes like an inline-block",
		doc:    "so `display: inline` lays out as `inline-block`",
	},
	{
		name: "margins do not collapse through a parent",
		spec: "CSS2 §8.3.1",
		css: `#o, #p { display: block; } #o { width: 200px; }
			#x { height: 10px; } #k { height: 20px; margin-top: 20px; }`,
		tree:   nBox("#o", nBox("#x"), nBox("#p", nBox("#k"))),
		want:   []confCheck{wantRect("p", 0, 10, 200, 40), wantRect("k", 0, 30, 200, 20)},
		reason: "every CSS-laid-out box roots its own block formatting context",
		doc:    "margins never collapse through a parent and its first or",
	},
	{
		name: "z-index stacks within the container",
		spec: "CSS2 §9.9.1, Appendix E",
		css: `#c { display: block; position: relative; width: 200px; height: 100px; }
			#a, #b { position: absolute; top: 0; left: 0; width: 100px; height: 100px; }
			#b { background-color: #0000ff; }
			#k { position: relative; z-index: 10; width: 50px; height: 50px; background-color: #ff0000; }`,
		tree:   nBox("#c", nBox("#a", nBox("#k")), nBox("#b")),
		want:   []confCheck{wantPixel(25, 25, layBlue), wantHit(25, 25, "b")},
		reason: "stacking contexts are per container: a widget paints in its parent's pass",
		doc:    "- Stacking contexts are per container: `z-index` orders a widget",
	},
	{
		name: "absolute flex child's static position is the content start",
		wpt:  "css/css-flexbox/abspos/flex-abspos-staticpos-justify-content-001.html",
		spec: "css-flexbox-1 §4.1",
		css: `#c { display: flex; position: relative; justify-content: center; width: 200px; height: 50px; }
			#p { position: absolute; width: 50px; height: 50px; }`,
		tree:   nBox("#c", nLeaf("#p", 10, 10)),
		want:   []confCheck{wantRect("p", 0, 0, 50, 50)},
		reason: "the flex pass does not align out-of-flow children as sole flex items",
		doc:    "- The static position of an absolute child of a flex container is the",
	},
	{
		name:   "min() mixing a percentage with a length is invalid",
		spec:   "css-values-4 §10.2",
		css:    `#c { display: block; width: 400px; } #a { width: min(50%, 100px); height: 10px; }`,
		tree:   nBox("#c", nBox("#a")),
		want:   []confCheck{wantRect("a", 0, 0, 400, 10)},
		reason: "the comparison needs the layout-time basis, which the compute-time evaluator never sees",
		doc:    "`min()`, `max()` and `clamp()` mixing percentages with lengths are",
	},
	{
		name:   "the window root takes the window size",
		spec:   "css-sizing-3 §3.1",
		css:    `#root { width: 100px; height: 50px; }`,
		tree:   nLeaf("#a", 10, 10),
		want:   []confCheck{wantRect("root", 0, 0, 400, 300)},
		reason: "the window's root widget is the viewport, which the window size fixes",
		doc:    "The window's root widget takes the window size; CSS sizes on it do",
	},
}

func TestCSSConformanceLayoutExceptions(t *testing.T) {
	runConformanceExceptions(t, confExceptionsLayout)
}

func TestCSSConformanceLayout(t *testing.T) { runConformance(t, confLayout) }
