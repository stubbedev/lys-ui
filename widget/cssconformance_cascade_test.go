package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// The computed values the cascade cases read beside the shared ones.
var (
	cpCasPaddingTop     = confProp{"padding-top", func(v *style.Values) any { return v.Padding.Top }}
	cpCasBorderTopColor = confProp{"border-top-color", func(v *style.Values) any { return v.BorderColor[0] }}
)

// casSheet installs css as its own stylesheet at priority for the
// rest of the case.
func casSheet(f *confFixture, css string, priority int) {
	s := AddStylesheet(css, priority)
	f.t.Cleanup(s.Remove)
}

// casFace reports whether family resolves to a face at weight 400.
func casFace(family string) func(*confFixture) any {
	return func(*confFixture) any {
		_, ok := resolveFace(family, 400, nil)
		return ok
	}
}

// confCascade is the #159 At-rules and cascade group: the cascade
// order (importance, priority, layers, specificity, scope proximity,
// order), the CSS-wide keywords, custom properties, and the at-rules.
var confCascade = []confCase{
	{
		name: "specificity outranks order and order breaks ties",
		spec: "css-cascade-5 §6.1",
		css: `.t.t { color: #008000; } .t { color: #ff0000; }
			.u { color: #ff0000; } .u { color: #008000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10), nLeaf("#u.u", 10, 10)),
		want: selGreenFor("t", "u"),
	},
	{
		name: "important outranks a more specific normal declaration",
		spec: "css-cascade-5 §6.1",
		css:  `#t { color: #ff0000; } .t { color: #008000 !important; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "important declarations rank by specificity then order",
		spec: "css-cascade-5 §6.1",
		css: `.t.t { color: #008000 !important; } .t { color: #ff0000 !important; }
			.u { color: #ff0000 !important; } .u { color: #008000 !important; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10), nLeaf("#u.u", 10, 10)),
		want: selGreenFor("t", "u"),
	},
	{
		name: "inline style outranks every selector",
		spec: "css-style-attr-1 §3",
		css:  `#t.t.t { color: #ff0000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		act:  func(f *confFixture) { nodeOf(f.get("t")).SetInlineStyle("color: #008000") },
		want: selGreenFor("t"),
	},
	{
		name: "a higher provider priority outranks specificity",
		spec: "css-cascade-5 §6.1 (GTK provider priority)",
		css:  `#t.t.t { color: #ff0000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		act:  func(f *confFixture) { casSheet(f, `stub, .t { color: #008000; }`, StylePriorityUser) },
		want: selGreenFor("t"),
	},
	{
		name: "unlayered styles outrank layered ones whatever their specificity",
		wpt:  "css/css-cascade/layer-basic.html",
		spec: "css-cascade-5 §6.4",
		css:  `@layer a { #t.t { color: #ff0000; } } .t { color: #008000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "layer order comes from first declaration and outranks specificity",
		wpt:  "css/css-cascade/layer-statement-before-import.html",
		spec: "css-cascade-5 §6.4.3",
		css: `@layer b, a;
			@layer a { .t { color: #008000; } }
			@layer b { #t.t.t { color: #ff0000; } }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "a layer's own styles outrank its nested layers",
		spec: "css-cascade-5 §6.4.2",
		css: `@layer outer { .t { color: #008000; } @layer inner { #t { color: #ff0000; } } }
			@layer x.y { #u { color: #ff0000; } }
			@layer x { .u { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10), nLeaf("#u.u", 10, 10)),
		want: selGreenFor("t", "u"),
	},
	{
		name: "anonymous layers order by appearance",
		spec: "css-cascade-5 §6.4.1",
		css:  `@layer { #t { color: #ff0000; } } @layer { .t { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "important declarations reverse the layer order",
		wpt:  "css/css-cascade/layer-important.html",
		spec: "css-cascade-5 §6.4",
		css: `@layer a { .t { color: #008000 !important; } }
			@layer b { #t { color: #ff0000 !important; } }
			.u { color: #ff0000 !important; }
			@layer a { .u { color: #008000 !important; } }
			@layer a { .v { color: #008000 !important; } }
			#v { color: #ff0000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10), nLeaf("#u.u", 10, 10), nLeaf("#v.v", 10, 10)),
		want: selGreenFor("t", "u", "v"),
	},
	{
		name: "revert-layer rolls back to the layer below",
		wpt:  "css/css-cascade/revert-layer-001.html",
		spec: "css-cascade-5 §7.4",
		css: `@layer base { .t { color: #008000; } .u { color: #008000; } }
			@layer mid { .u { color: revert-layer; } }
			@layer top { .t { color: revert-layer; } .u { color: revert-layer; } }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10), nLeaf("#u.u", 10, 10)),
		want: selGreenFor("t", "u"),
	},
	{
		name: "revert rolls back to the user agent origin",
		spec: "css-cascade-5 §7.3",
		css: `.p { color: #008000; }
			.k { color: #ff0000; background-color: #ff0000; }
			.k { color: revert; background-color: revert; }`,
		tree: nCol("#p.p", nLeaf("#k.k", 10, 10)),
		want: []confCheck{
			wantComputed("k", cpColor, selGreen),
			wantComputed("k", cpBackground, render.Color(0)),
		},
	},
	{
		name: "revert rolls back to the lower-priority theme sheets",
		spec: "css-cascade-5 §7.3 (GTK provider priority)",
		css:  `.t { color: #ff0000; } .t { color: revert; } .u { color: revert; }`,
		tree: nCol("#c", nLeaf("#t.t", 1, 1), nLeaf("#u.u", 1, 1)),
		act: func(f *confFixture) {
			casSheet(f, `.t { color: #008000; } .u { color: #ff0000; } .u { color: revert; }`, StylePriorityTheme)
			casSheet(f, `.u { color: #008000; }`, StylePriorityFallback)
		},
		want: selGreenFor("t", "u"),
	},
	{
		name: "inherit and unset",
		spec: "css-cascade-5 §7.2",
		css: `.p { color: #008000; background-color: #008000; }
			.i { color: #ff0000; background-color: inherit; }
			.i { color: unset; }
			.u { background-color: #ff0000; }
			.u { background-color: unset; }`,
		tree: nCol("#p.p", nLeaf("#i.i", 10, 10), nLeaf("#u.u", 10, 10)),
		want: []confCheck{
			wantComputed("i", cpColor, selGreen),
			wantComputed("i", cpBackground, selGreen),
			wantComputed("u", cpBackground, render.Color(0)),
		},
	},
	{
		name: "custom properties inherit and var() takes fallbacks",
		wpt:  "css/css-variables/variable-definition-cascading.html",
		spec: "css-variables-1 §2, §3",
		css: `.p { --c: #008000; }
			.k { color: var(--c); background-color: var(--missing, #008000); }`,
		tree: nCol("#p.p", nCol("#mid", nLeaf("#k.k", 10, 10))),
		want: []confCheck{
			wantComputed("k", cpColor, selGreen),
			wantComputed("k", cpBackground, selGreen),
		},
	},
	{
		name: "a custom property cycle is invalid and var() falls back",
		wpt:  "css/css-variables/variable-cycles.html",
		spec: "css-variables-1 §2.3",
		css:  `.t { --a: var(--b); --b: var(--a); color: var(--a, #008000); }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "var() inside shorthands",
		spec: "css-variables-1 §3.2",
		css: `.t { --w: 3px; --c: #008000; --p: 7px;
			border: var(--w) solid var(--c); padding: var(--p) 0; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: []confCheck{
			wantComputed("t", cpCasBorderTopColor, selGreen),
			wantComputed("t", cpCasPaddingTop, 7),
		},
	},
	{
		name: "media size features and range syntax",
		wpt:  "css/mediaqueries/mq-range-001.html",
		spec: "mediaqueries-4 §4.3, §4.4",
		css: `* { color: #ff0000; }
			@media (min-width: 400px) { #a { color: #008000; } }
			@media (width > 400px) { #b { color: #008000; } }
			@media not (width > 400px) { #b { background-color: #008000; } }
			@media (300px < width <= 400px) { #c1 { color: #008000; } }
			@media (height: 300px) { #d { color: #008000; } }
			@media (max-width: 399px) { #e { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#a", 1, 1), nLeaf("#b", 1, 1), nLeaf("#c1", 1, 1), nLeaf("#d", 1, 1), nLeaf("#e", 1, 1)),
		want: selBoth(selGreenFor("a", "c1", "d"), selRedFor("b", "e"),
			[]confCheck{wantComputed("b", cpBackground, selGreen)}),
	},
	{
		name: "media aspect-ratio orientation and resolution",
		wpt:  "css/mediaqueries/aspect-ratio-001.html",
		spec: "mediaqueries-4 §4.2, §5.1, §5.2",
		css: `* { color: #ff0000; }
			@media (aspect-ratio: 4/3) { #a { color: #008000; } }
			@media (min-aspect-ratio: 2/1) { #b { color: #008000; } }
			@media (orientation: landscape) { #o { color: #008000; } }
			@media (orientation: portrait) { #p { color: #008000; } }
			@media (resolution: 1dppx) { #r { color: #008000; } }
			@media (min-resolution: 2dppx) { #s { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#a", 1, 1), nLeaf("#b", 1, 1), nLeaf("#o", 1, 1), nLeaf("#p", 1, 1), nLeaf("#r", 1, 1), nLeaf("#s", 1, 1)),
		want: selBoth(selGreenFor("a", "o", "r"), selRedFor("b", "p", "s")),
	},
	{
		name: "media query lists and boolean combinators",
		wpt:  "css/mediaqueries/media-queries-level-4-or.html",
		spec: "mediaqueries-4 §3",
		css: `* { color: #ff0000; }
			@media (min-width: 900px), (max-height: 300px) { #list { color: #008000; } }
			@media ((min-width: 900px) or (max-width: 450px)) { #or { color: #008000; } }
			@media screen and (min-width: 100px) { #and { color: #008000; } }
			@media print { #print { color: #008000; } }
			@media not print { #notprint { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#list", 1, 1), nLeaf("#or", 1, 1), nLeaf("#and", 1, 1), nLeaf("#print", 1, 1), nLeaf("#notprint", 1, 1)),
		want: selBoth(selGreenFor("list", "or", "and", "notprint"), selRedFor("print")),
	},
	{
		name: "media re-evaluates on a viewport change",
		spec: "mediaqueries-4 §2",
		css: `* { color: #ff0000; }
			@media (min-width: 600px) { #t { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#t", 1, 1)),
		act:  func(*confFixture) { SetMediaViewport(800, 600) },
		want: selGreenFor("t"),
	},
	{
		name: "prefers-color-scheme follows the theme",
		wpt:  "css/mediaqueries/prefers-color-scheme.html",
		spec: "mediaqueries-5 §11.5",
		css: `* { color: #ff0000; }
			@media (prefers-color-scheme: dark) { #d { color: #008000; } }
			@media (prefers-color-scheme: light) { #l { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#d", 1, 1), nLeaf("#l", 1, 1)),
		want: selBoth(selGreenFor("d"), selRedFor("l")),
	},
	{
		name: "prefers-color-scheme light after a theme switch",
		spec: "mediaqueries-5 §11.5",
		css: `* { color: #ff0000; }
			@media (prefers-color-scheme: dark) { #d { color: #008000; } }
			@media (prefers-color-scheme: light) { #l { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#d", 1, 1), nLeaf("#l", 1, 1)),
		act:  func(*confFixture) { SetTheme(LightTheme()) },
		want: selBoth(selGreenFor("l"), selRedFor("d")),
	},
	{
		name: "prefers-reduced-motion and prefers-contrast",
		wpt:  "css/mediaqueries/prefers-reduced-motion.html",
		spec: "mediaqueries-5 §11.2, §11.3",
		css: `* { color: #ff0000; }
			@media (prefers-reduced-motion: reduce) { #m { color: #008000; } }
			@media (prefers-contrast: more) { #k { color: #008000; } }
			@media (prefers-contrast: no-preference) { #n { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#m", 1, 1), nLeaf("#k", 1, 1), nLeaf("#n", 1, 1)),
		act: func(*confFixture) {
			th := DarkTheme()
			th.Animations = false
			SetTheme(th)
			SetMediaContrast(true)
		},
		want: selBoth(selGreenFor("m", "k"), selRedFor("n")),
	},
	{
		name: "container size queries against the nearest container",
		wpt:  "css/css-contain/container-queries/query-evaluation.html",
		spec: "css-contain-3 §6.1",
		css: `* { color: #ff0000; }
			.box { container-type: inline-size; width: 300px; }
			@container (min-width: 250px) { #a { color: #008000; } }
			@container (min-width: 350px) { #b { color: #008000; } }
			@container (min-height: 1px) { #h { color: #008000; } }
			@container not (min-height: 1px) { #h { background-color: #ff0000; } }`,
		tree: nCol("#box.box", nLeaf("#a", 1, 1), nLeaf("#b", 1, 1), nLeaf("#h", 1, 1)),
		want: selBoth(selGreenFor("a"), selRedFor("b", "h"),
			[]confCheck{wantComputed("h", cpBackground, render.Color(0))}),
	},
	{
		name: "named and size containers",
		wpt:  "css/css-contain/container-queries/container-name.html",
		spec: "css-contain-3 §5.2, §6.1",
		css: `* { color: #ff0000; }
			.outer { container: card / size; width: 300px; height: 200px; }
			.inner { container-type: inline-size; width: 100px; }
			@container card (min-width: 250px) { #a { color: #008000; } }
			@container (min-width: 250px) { #b { color: #008000; } }
			@container card (min-height: 150px) { #c1 { color: #008000; } }`,
		tree: nCol("#outer.outer", nCol("#inner.inner", nLeaf("#a", 1, 1), nLeaf("#b", 1, 1), nLeaf("#c1", 1, 1))),
		want: selBoth(selGreenFor("a", "c1"), selRedFor("b")),
	},
	{
		name: "container query units",
		wpt:  "css/css-contain/container-queries/container-units-basic.html",
		spec: "css-contain-3 §8",
		css: `.box { container-type: size; width: 300px; height: 200px; }
			.t { width: 50cqw; height: 10cqh; padding-top: 10cqmin; }`,
		tree: nCol("#box.box", nLeaf("#t.t", 1, 1)),
		want: []confCheck{
			wantRect("t", 0, 0, 150, 40),
			wantComputed("t", cpCasPaddingTop, 20),
		},
	},
	{
		name: "container condition lists select a container each",
		spec: "css-contain-3 §6.1",
		css: `* { color: #ff0000; }
			.outer { container: card / inline-size; width: 300px; }
			.inner { container: side / inline-size; width: 100px; }
			@container card (min-width: 500px), side (max-width: 150px) { #a { color: #008000; } }
			@container card (min-width: 250px) { @container (max-width: 150px) { #b { color: #008000; } } }
			@container side (min-width: 250px) { #c1 { color: #008000; } }`,
		tree: nCol("#outer.outer", nCol("#inner.inner", nLeaf("#a", 1, 1), nLeaf("#b", 1, 1), nLeaf("#c1", 1, 1))),
		want: selBoth(selGreenFor("a", "b"), selRedFor("c1")),
	},
	{
		name: "container units without a container fall back to the viewport",
		spec: "css-contain-3 §8",
		css: `.t { width: 10cqw; height: 10cqh; }
			.box { container-type: inline-size; width: 200px; }
			.u { width: 10cqw; height: 10cqh; }`,
		tree: nCol("#c", nLeaf("#t.t", 1, 1), nCol("#box.box", nLeaf("#u.u", 1, 1))),
		want: []confCheck{
			wantRect("t", 0, 0, 40, 30),
			wantProbe("size of #u", func(f *confFixture) any {
				b := boundsOf(f.get("u"))
				return [2]int{b.W, b.H}
			}, [2]int{20, 30}),
		},
	},
	{
		name: "supports gates on the engine's support",
		wpt:  "css/css-conditional/at-supports-001.html",
		spec: "css-conditional-3 §6, css-conditional-4 §3",
		css: `* { color: #ff0000; }
			@supports (display: grid) { #a { color: #008000; } }
			@supports (display: no-such-display) { #b { color: #008000; } }
			@supports not (no-such-property: 1) { #c1 { color: #008000; } }
			@supports (color: red) and ((display: flex) or (display: nope)) { #d { color: #008000; } }
			@supports selector(:has(> .x)) { #e { color: #008000; } }
			@supports selector(:no-such-pseudo) { #f { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#a", 1, 1), nLeaf("#b", 1, 1), nLeaf("#c1", 1, 1), nLeaf("#d", 1, 1), nLeaf("#e", 1, 1), nLeaf("#f", 1, 1)),
		want: selBoth(selGreenFor("a", "c1", "d", "e"), selRedFor("b", "f")),
	},
	{
		name: "import with layer supports and media qualifiers",
		wpt:  "css/css-cascade/layer-import.html",
		spec: "css-cascade-5 §2",
		css: `@import "testdata/conformance/import-layered.css" layer(base);
			@import "testdata/conformance/import-supported.css" supports(display: grid);
			@import "testdata/conformance/import-unsupported.css" supports(display: no-such-display);
			@import "testdata/conformance/import-wide.css" (min-width: 500px);
			.t, #n, #m { color: #008000; }
			#s { color: #ff0000; }
			@layer base { #s { color: #ff0000; } }`,
		tree: nCol("#c", nLeaf("#t.t", 1, 1), nLeaf("#s", 1, 1), nLeaf("#n", 1, 1), nLeaf("#m", 1, 1)),
		want: selBoth(selGreenFor("t", "n", "m"), selRedFor("s")),
	},
	{
		name: "an import after a rule is ignored",
		spec: "css-cascade-5 §2",
		css: `#late { color: #008000; }
			@import "testdata/conformance/import-late.css";`,
		tree: nCol("#c", nLeaf("#late", 1, 1)),
		want: selGreenFor("late"),
	},
	{
		name: "font-face loads an app font",
		wpt:  "css/css-fonts/font-face-unicode-range.html",
		spec: "css-fonts-4 §4",
		css: `@font-face { font-family: "Conformance Face";
			src: url("missing.woff2") format("woff2"), url("testdata/Cantarell-Regular.ttf") format("truetype"); }`,
		tree: nCol("#c", nLeaf("#t", 1, 1)),
		want: []confCheck{
			wantProbe("Conformance Face resolves", casFace("Conformance Face"), true),
			wantProbe("an undeclared family declines", casFace("Undeclared Face"), false),
		},
	},
	{
		name: "property registers typed custom properties",
		wpt:  "css/css-properties-values-api/at-property.html",
		spec: "css-properties-values-api-1 §3",
		css: `@property --len { syntax: "<length>"; inherits: true; initial-value: 4px; }
			@property --tint { syntax: "<color>"; inherits: false; initial-value: #008000; }
			.p { --tint: #ff0000; --len: 9px; }
			.k { color: var(--tint); padding-top: var(--len); }
			.bad { --len: red; padding-top: var(--len); }`,
		tree: nCol("#c", nCol("#p.p", nLeaf("#k.k", 1, 1), nLeaf("#badin.bad", 1, 1)), nLeaf("#bad.bad", 1, 1)),
		want: []confCheck{
			wantComputed("k", cpColor, selGreen),
			wantComputed("k", cpCasPaddingTop, 9),
			wantComputed("badin", cpCasPaddingTop, 9),
			wantComputed("bad", cpCasPaddingTop, 4),
		},
	},
	{
		name: "property with an invalid initial value does not register",
		wpt:  "css/css-properties-values-api/at-property.html",
		spec: "css-properties-values-api-1 §3",
		css: `@property --x { syntax: "<length>"; inherits: false; initial-value: red; }
			.t { color: var(--x, #008000); }`,
		tree: nCol("#c", nLeaf("#t.t", 1, 1)),
		want: selGreenFor("t"),
	},
	{
		name: "scope roots and limits",
		wpt:  "css/css-cascade/scope-evaluation.html",
		spec: "css-cascade-6 §2.5",
		css: `* { color: #ff0000; }
			@scope (.card) to (.limit) { .t { color: #008000; } }`,
		tree: nCol("#c",
			nCol("#card.card", nLeaf("#in.t", 1, 1), nCol("#limit.limit", nLeaf("#past.t", 1, 1))),
			nLeaf("#out.t", 1, 1)),
		want: selBoth(selGreenFor("in"), selRedFor("past", "out")),
	},
	{
		name: "scope proximity prefers the nearer root",
		wpt:  "css/css-cascade/scope-proximity.html",
		spec: "css-cascade-6 §6.1",
		css: `@scope (.light) { .t { color: #008000; } }
			@scope (.dark) { .t { color: #ff0000; } }`,
		tree: nCol("#dark.dark", nCol("#light.light", nLeaf("#t.t", 1, 1))),
		want: selGreenFor("t"),
	},
	{
		name: "scope proximity ranks below specificity",
		wpt:  "css/css-cascade/scope-specificity.html",
		spec: "css-cascade-6 §6.1",
		css: `@scope (.far) { .t.t { color: #008000; } }
			@scope (.near) { .t { color: #ff0000; } }`,
		tree: nCol("#far.far", nCol("#near.near", nLeaf("#t.t", 1, 1))),
		want: selGreenFor("t"),
	},
	{
		name: "a scoped rule outranks an unscoped one at equal specificity",
		wpt:  "css/css-cascade/scope-proximity.html",
		spec: "css-cascade-6 §6.1",
		css: `@scope (.a) { .t { color: #008000; } }
			.t { color: #ff0000; }`,
		tree: nCol("#a.a", nLeaf("#t.t", 1, 1)),
		want: selGreenFor("t"),
	},
	{
		name: "the scope root adds no specificity",
		wpt:  "css/css-cascade/scope-specificity.html",
		spec: "css-cascade-6 §2.5.2",
		css: `@scope (#a) { .t { color: #ff0000; } }
			.t.t { color: #008000; }`,
		tree: nCol("#a", nLeaf("#t.t", 1, 1)),
		want: selGreenFor("t"),
	},
	{
		name: "scope pseudo-class and root lists",
		spec: "css-cascade-6 §2.5.2",
		css: `* { color: #ff0000; }
			@scope (.a, .b) { :scope { color: #008000; } :scope > .t { color: #008000; } }`,
		tree: nCol("#c", nCol("#a.a", nLeaf("#t1.t", 1, 1)), nCol("#b.b", nLeaf("#t2.t", 1, 1)), nLeaf("#t3.t", 1, 1)),
		want: selBoth(selGreenFor("a", "b", "t1", "t2"), selRedFor("t3")),
	},
	{
		name: "nested scopes and the nesting selector inside a scope",
		spec: "css-cascade-6 §2.5.2",
		css: `* { color: #ff0000; }
			@scope (.card) {
				@scope (.body) { .t { color: #008000; } }
				& > .head { color: #008000; }
			}`,
		tree: nCol("#c",
			nCol("#card.card", nLeaf("#head.head", 1, 1), nCol("#body.body", nLeaf("#t.t", 1, 1))),
			nCol("#loose.body", nLeaf("#u.t", 1, 1))),
		want: selBoth(selGreenFor("head", "t"), selRedFor("u")),
	},
	{
		name: "nesting desugars the nesting selector",
		wpt:  "css/css-nesting/nesting-basic.html",
		spec: "css-nesting-1 §2, §3",
		css: `.p { color: #ff0000;
				& > .t { color: #008000; }
				.u { color: #008000; }
				&.x { background-color: #008000; }
				.q & { background-color: #008000; }
			}`,
		tree: nCol("#q.q", nCol("#p.p.x", nLeaf("#t.t", 1, 1), nCol("#w", nLeaf("#u.u", 1, 1)))),
		want: selBoth(selGreenFor("t", "u"), selRedFor("p"),
			[]confCheck{wantComputed("p", cpBackground, selGreen)}),
	},
	{
		name: "the nesting selector has the specificity of is()",
		wpt:  "css/css-nesting/nesting-specificity.html",
		spec: "css-nesting-1 §4",
		css: `.a, #b { & .t { color: #008000; } }
			.a .t.t.t { color: #ff0000; }`,
		tree: nCol("#a.a", nLeaf("#t.t", 1, 1)),
		want: selGreenFor("t"),
	},
	{
		name: "the nesting selector keeps is() semantics inside a compound",
		spec: "css-nesting-1 §3",
		css: `* { color: #ff0000; }
			.a .b { .c & { color: #008000; } }`,
		tree: nCol("#a.a", nCol("#c.c", nLeaf("#b.b", 1, 1))),
		want: selGreenFor("b"),
	},
	{
		name: "nesting repeats and joins the parent",
		spec: "css-nesting-1 §3",
		css: `* { color: #ff0000; }
			.a { & + & { color: #008000; } }`,
		tree: nCol("#c", nLeaf("#a1.a", 1, 1), nLeaf("#a2.a", 1, 1)),
		want: selBoth(selGreenFor("a2"), selRedFor("a1")),
	},
	{
		name: "nested conditional rules",
		wpt:  "css/css-nesting/conditional-rules.html",
		spec: "css-nesting-1 §3.2",
		css: `.t { color: #ff0000;
				@media (min-width: 300px) { color: #008000; }
				@supports (display: grid) { background-color: #008000; }
			}`,
		tree: nCol("#c", nLeaf("#t.t", 1, 1)),
		want: []confCheck{
			wantComputed("t", cpColor, selGreen),
			wantComputed("t", cpBackground, selGreen),
		},
	},
	{
		name: "an invalid nested selector drops only the nested rule",
		wpt:  "css/css-nesting/invalid-inner-rules.html",
		spec: "css-nesting-1 §2",
		css: `.t { color: #008000;
				&:no-such-pseudo { color: #ff0000; }
				background-color: #008000;
			}`,
		tree: nCol("#c", nLeaf("#t.t", 1, 1)),
		want: []confCheck{
			wantComputed("t", cpColor, selGreen),
			wantComputed("t", cpBackground, selGreen),
		},
	},
}

func TestCSSConformanceCascade(t *testing.T) { runConformance(t, confCascade) }

// confExceptionsCascade are the At-rules and cascade group's deliberate
// deviations, each asserting the documented behavior.
var confExceptionsCascade = []confException{
	{
		name: "a registered custom property does not animate",
		wpt:  "css/css-properties-values-api/animation/custom-property-transition.html",
		spec: "css-properties-values-api-1 §2",
		css: `@property --o { syntax: "<number>"; inherits: false; initial-value: 0; }
			.t { --o: 0; opacity: var(--o); transition: --o 1000ms linear; }
			.t.on { --o: 1; }`,
		tree: nCol("#c", nLeaf("#t.t", 1, 1)),
		act: func(f *confFixture) {
			nodeOf(f.get("t")).AddClass("on")
			f.settle()
			f.clock.step()
		},
		want:   []confCheck{wantComputed("t", cpOpacity, 1.0)},
		reason: "the transition engine keys on the longhand table, which custom properties sit outside",
		doc:    "Animating a registered property is not supported",
	},
	{
		name: "an import may follow define-color",
		spec: "css-cascade-5 §2 (GTK @define-color)",
		css: `@define-color accent #008000;
			@import "testdata/conformance/import-late.css";
			#late { background-color: @accent; }`,
		tree: nCol("#c", nLeaf("#late", 1, 1)),
		want: []confCheck{
			wantComputed("late", cpColor, selRed),
			wantComputed("late", cpBackground, selGreen),
		},
		reason: "GTK accepts @define-color ahead of its @import rules, and GTK stylesheets load as they are",
		doc:    "`@define-color` may precede an `@import`",
	},
}

func TestCSSConformanceCascadeExceptions(t *testing.T) {
	runConformanceExceptions(t, confExceptionsCascade)
}
