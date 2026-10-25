package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// The WPT pass and fail colors: a case's subject computes green when
// the selector behaves as the spec says, red otherwise.
const (
	selGreen = render.Color(0xff008000)
	selRed   = render.Color(0xffff0000)
)

// selAttr is a stub carrying the app-set attributes kv (name, value
// pairs).
func selAttr(sel string, kv ...string) confNode {
	return confNode{sel: sel, make: func(*confFixture, []Widget) Widget {
		s := newStub(10, 10)
		for i := 0; i+1 < len(kv); i += 2 {
			s.SetAttr(kv[i], kv[i+1])
		}
		return s
	}}
}

// selElem is a stub named element for the cascade (SetElement), the
// way a widget's sub-nodes carry GTK's node names.
func selElem(sel, element string) confNode {
	return confNode{sel: sel, make: func(*confFixture, []Widget) Widget {
		s := newStub(10, 10)
		s.SetElement(element)
		return s
	}}
}

// selPlaceholder is an empty Entry showing the placeholder ph.
func selPlaceholder(sel, ph string) confNode {
	return confNode{sel: sel, make: func(f *confFixture, _ []Widget) Widget {
		e := NewEntry(f.face, confFaceSize, render.RGB(0, 0, 0))
		e.SetPlaceholder(ph)
		return e
	}}
}

// selPseudo reads a pseudo-element's computed content string.
func selPseudo(id, pseudo string) func(f *confFixture) any {
	return func(f *confFixture) any { return PseudoContent(f.get(id), pseudo) }
}

// selGreenFor expects each id to compute color green.
func selGreenFor(ids ...string) []confCheck {
	out := make([]confCheck, len(ids))
	for i, id := range ids {
		out[i] = wantComputed(id, cpColor, selGreen)
	}
	return out
}

// selRedFor expects each id to compute color red: the selector must
// not have matched it.
func selRedFor(ids ...string) []confCheck {
	out := make([]confCheck, len(ids))
	for i, id := range ids {
		out[i] = wantComputed(id, cpColor, selRed)
	}
	return out
}

// selBoth joins check lists.
func selBoth(lists ...[]confCheck) []confCheck {
	var out []confCheck
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// confSelectors is the #159 Selectors group: attribute selectors, the
// functional and structural pseudo-classes, the state pseudo-classes,
// combinators, specificity and the pseudo-elements.
var confSelectors = []confCase{
	{
		name: "attribute presence and equality",
		wpt:  "css/selectors/attribute-selectors/attribute-case/semantics.html",
		spec: "selectors-4 §6.1",
		css: `* { color: #ff0000; }
			[data-x] { color: #008000; }
			[data-y="a"] { color: #008000; }`,
		tree: nCol("#c",
			selAttr("#present", "data-x", ""),
			selAttr("#eq", "data-y", "a"),
			selAttr("#neq", "data-y", "ab"),
			selAttr("#none")),
		want: selBoth(selGreenFor("present", "eq"), selRedFor("neq", "none")),
	},
	{
		name: "attribute whitespace list and dash match",
		spec: "selectors-4 §6.1",
		css: `* { color: #ff0000; }
			[data-tags~="b"] { color: #008000; }
			[data-lang|="en"] { color: #008000; }`,
		tree: nCol("#c",
			selAttr("#list", "data-tags", "a b c"),
			selAttr("#glued", "data-tags", "ab c"),
			selAttr("#en", "data-lang", "en"),
			selAttr("#enus", "data-lang", "en-US"),
			selAttr("#english", "data-lang", "english")),
		want: selBoth(selGreenFor("list", "en", "enus"), selRedFor("glued", "english")),
	},
	{
		name: "attribute substring operators",
		spec: "selectors-4 §6.2",
		css: `* { color: #ff0000; }
			[data-p^="pre"] { color: #008000; }
			[data-s$="fix"] { color: #008000; }
			[data-m*="mid"] { color: #008000; }`,
		tree: nCol("#c",
			selAttr("#pre", "data-p", "prefix"),
			selAttr("#notpre", "data-p", "xpre"),
			selAttr("#suf", "data-s", "suffix"),
			selAttr("#notsuf", "data-s", "fixed"),
			selAttr("#mid", "data-m", "amidst"),
			selAttr("#notmid", "data-m", "m-i-d")),
		want: selBoth(selGreenFor("pre", "suf", "mid"), selRedFor("notpre", "notsuf", "notmid")),
	},
	{
		name: "attribute substring operators with an empty value match nothing",
		spec: "selectors-4 §6.2",
		css: `* { color: #008000; }
			[data-v^=""], [data-v$=""], [data-v*=""] { color: #ff0000; }`,
		tree: nCol("#c", selAttr("#t", "data-v", "anything")),
		want: selGreenFor("t"),
	},
	{
		name: "attribute case-sensitivity flags",
		wpt:  "css/selectors/attribute-selectors/attribute-case/semantics.html",
		spec: "selectors-4 §6.3",
		css: `* { color: #ff0000; }
			[data-k="ABC" i] { color: #008000; }
			[data-k="ABC" s] { background-color: #ff0000; }
			[data-k="abc" s] { background-color: #008000; }`,
		tree: nCol("#c", selAttr("#t", "data-k", "abc")),
		want: []confCheck{
			wantComputed("t", cpColor, selGreen),
			wantComputed("t", cpBackground, selGreen),
		},
	},
	{
		name: "built-in id class and role attributes",
		spec: "selectors-4 §6.1",
		css: `* { color: #ff0000; }
			[id="i"] { color: #008000; }
			[class~="k"] { color: #008000; }
			[role="button"] { color: #008000; }`,
		tree: nCol("#c",
			nLeaf("#i", 10, 10),
			nLeaf("#k.j.k.l", 10, 10),
			nButton("#b", nLeaf("#inner", 4, 4))),
		want: selGreenFor("i", "k", "b"),
	},
	{
		name: "is() takes its most specific argument's specificity",
		wpt:  "css/selectors/is-specificity.html",
		spec: "selectors-4 §17",
		css: `:is(#nothing, .t) { color: #008000; }
			.t.t.t { color: #ff0000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "where() has zero specificity",
		wpt:  "css/selectors/is-where-basic.html",
		spec: "selectors-4 §4.4",
		css: `* { color: #008000; }
			:where(#t, .t) { color: #ff0000; }
			item { color: #008000; }`,
		tree: nCol("#c", selElem("#t.t", "item")),
		want: selGreenFor("t"),
	},
	{
		name: "is() and where() take complex selector arguments",
		spec: "selectors-4 §4.2",
		css: `* { color: #ff0000; }
			:is(.a > .b) { color: #008000; }
			:where(.a .d) .e { color: #008000; }`,
		tree: nCol("#a.a",
			nLeaf("#b.b", 10, 10),
			nCol("#x", nLeaf("#nb.b", 10, 10)),
			nCol("#d.d", nLeaf("#e.e", 10, 10))),
		want: selBoth(selGreenFor("b", "e"), selRedFor("nb")),
	},
	{
		name: "is() and where() are forgiving of invalid arguments",
		wpt:  "css/selectors/is-where-error-recovery.html",
		spec: "selectors-4 §4.2",
		css: `* { color: #ff0000; }
			:is(.t, :nonsense) { color: #008000; }
			:where(:nonsense, .u) { color: #008000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10), nLeaf("#u.u", 10, 10)),
		want: selGreenFor("t", "u"),
	},
	{
		name: "an invalid selector in a plain list drops the rule",
		spec: "selectors-4 §3.1",
		css: `* { color: #008000; }
			.t, :nonsense { color: #ff0000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "not() with a complex selector list",
		wpt:  "css/selectors/not-complex.html",
		spec: "selectors-4 §4.3",
		css: `* { color: #ff0000; }
			.t:not(.a .t, .skip) { color: #008000; }`,
		tree: nCol("#c",
			nCol("#a.a", nLeaf("#under.t", 10, 10)),
			nLeaf("#free.t", 10, 10),
			nLeaf("#skip.t.skip", 10, 10)),
		want: selBoth(selGreenFor("free"), selRedFor("under", "skip")),
	},
	{
		name: "not() counts its most specific argument",
		wpt:  "css/selectors/not-specificity.html",
		spec: "selectors-4 §17",
		css: `.t:not(.x, #nothing) { color: #008000; }
			.t.t.t.t { color: #ff0000; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: selGreenFor("t"),
	},
	{
		name: "each functional pseudo-class adds its own specificity",
		spec: "selectors-4 §17",
		css: `.t:not(.a):not(.b) { color: #008000; }
			.t.t { color: #ff0000; }
			:is(.u):where(#nothing, .u):is(.u) { background-color: #008000; }
			.u.u.u { background-color: #ff0000; }
			.l > :nth-child(1 of #k1, .x) { padding-top: 3px; }
			.l > .x.x.x.x { padding-top: 9px; }`,
		tree: nCol("#l.l", nLeaf("#k1.x.t.u", 10, 10)),
		want: []confCheck{
			wantComputed("k1", cpColor, selGreen),
			wantComputed("k1", cpBackground, selRed),
			wantComputed("k1", cpCasPaddingTop, 3),
		},
	},
	{
		name: "repeated has() requires every argument list",
		spec: "selectors-4 §4.5",
		css: `* { color: #ff0000; }
			.p:has(.a):has(.b) { color: #008000; }`,
		tree: nCol("#c",
			nCol("#both.p", nLeaf("#a1.a", 1, 1), nLeaf("#b1.b", 1, 1)),
			nCol("#one.p", nLeaf("#a2.a", 1, 1))),
		want: selBoth(selGreenFor("both"), selRedFor("one")),
	},
	{
		name: "an ancestor's class change restyles an is() subject",
		spec: "selectors-4 §4.2",
		css: `* { color: #ff0000; }
			:is(.on > .t) { color: #008000; }`,
		tree: nCol("#p", nLeaf("#t.t", 1, 1)),
		act:  func(f *confFixture) { nodeOf(f.get("p")).AddClass("on") },
		want: selGreenFor("t"),
	},
	{
		name: "has() descendant and child arguments",
		wpt:  "css/selectors/has-basic.html",
		spec: "selectors-4 §4.5",
		css: `* { color: #ff0000; }
			.p:has(.c) { color: #008000; }
			.q:has(> .c) { color: #008000; }`,
		tree: nCol("#root2",
			nCol("#p.p", nCol("#mid", nLeaf("#c1.c", 10, 10))),
			nCol("#q.q", nLeaf("#c2.c", 10, 10)),
			nCol("#qdeep.q", nCol("#wrap", nLeaf("#c3.c", 10, 10))),
			nCol("#pnone.p", nLeaf("#n", 10, 10))),
		want: selBoth(selGreenFor("p", "q"), selRedFor("qdeep", "pnone")),
	},
	{
		name: "has() sibling arguments",
		wpt:  "css/selectors/has-relative-argument.html",
		spec: "selectors-4 §4.5",
		css: `* { color: #ff0000; }
			.a:has(+ .s) { color: #008000; }
			.b:has(~ .s) { color: #008000; }`,
		tree: nCol("#c",
			nLeaf("#a1.a", 10, 10),
			nLeaf("#s1.s", 10, 10),
			nLeaf("#b1.b", 10, 10),
			nLeaf("#gap", 10, 10),
			nLeaf("#a2.a", 10, 10),
			nLeaf("#s2.s", 10, 10),
			nLeaf("#b2.b", 10, 10)),
		want: selBoth(selGreenFor("a1", "a2", "b1"), selRedFor("b2")),
	},
	{
		name: "has() with a complex relative argument",
		wpt:  "css/selectors/has-relative-argument.html",
		spec: "selectors-4 §4.5",
		css: `* { color: #ff0000; }
			.p:has(> .x .y) { color: #008000; }`,
		tree: nCol("#c",
			nCol("#p.p", nCol("#x.x", nCol("#m", nLeaf("#y.y", 10, 10)))),
			nCol("#p2.p", nCol("#w", nCol("#x2.x", nLeaf("#y2.y", 10, 10))))),
		want: selBoth(selGreenFor("p"), selRedFor("p2")),
	},
	{
		name: "has() counts its most specific argument",
		wpt:  "css/selectors/has-specificity.html",
		spec: "selectors-4 §17",
		css: `.p:has(#k, .z) { color: #008000; }
			.p.p.p.p { color: #ff0000; }`,
		tree: nCol("#p.p", nLeaf("#k", 10, 10)),
		want: selGreenFor("p"),
	},
	{
		name: "child descendant and sibling combinators",
		spec: "selectors-4 §16",
		css: `* { color: #ff0000; }
			.a > .child { color: #008000; }
			.a .desc { color: #008000; }
			.first + .adj { color: #008000; }
			.first ~ .gen { color: #008000; }`,
		tree: nCol("#a.a",
			nLeaf("#child.child", 10, 10),
			nCol("#w", nLeaf("#desc.desc", 10, 10), nLeaf("#nochild.child", 10, 10)),
			nLeaf("#first.first", 10, 10),
			nLeaf("#adj.adj.gen", 10, 10),
			nLeaf("#notadj.adj", 10, 10),
			nLeaf("#gen.gen", 10, 10)),
		want: selBoth(selGreenFor("child", "desc", "adj", "gen"), selRedFor("nochild", "notadj")),
	},
	{
		name: "child-indexed pseudo-classes",
		spec: "selectors-4 §14.3",
		css: `* { color: #ff0000; }
			.l > :first-child { color: #008000; }
			.l > :last-child { color: #008000; }
			.l > :nth-child(3n+2) { color: #008000; }
			.l > :nth-last-child(4) { color: #008000; }
			.o > :only-child { color: #008000; }`,
		tree: nCol("#c",
			nCol("#l.l",
				nLeaf("#k1", 10, 10), nLeaf("#k2", 10, 10), nLeaf("#k3", 10, 10),
				nLeaf("#k4", 10, 10), nLeaf("#k5", 10, 10), nLeaf("#k6", 10, 10)),
			nCol("#o.o", nLeaf("#only", 10, 10))),
		want: selBoth(selGreenFor("k1", "k2", "k3", "k5", "k6", "only"), selRedFor("k4")),
	},
	{
		name: "nth-child even odd and negative steps",
		spec: "selectors-4 §14.3",
		css: `* { color: #ff0000; }
			.l > :nth-child(-n+2) { color: #008000; }
			.l > :nth-child(even) { color: #008000; }`,
		tree: nCol("#l.l",
			nLeaf("#k1", 10, 10), nLeaf("#k2", 10, 10), nLeaf("#k3", 10, 10),
			nLeaf("#k4", 10, 10), nLeaf("#k5", 10, 10)),
		want: selBoth(selGreenFor("k1", "k2", "k4"), selRedFor("k3", "k5")),
	},
	{
		name: "nth-child of a selector list",
		wpt:  "css/selectors/nth-child-of-classname.html",
		spec: "selectors-4 §14.3",
		css: `* { color: #ff0000; }
			.l > :nth-child(2 of .x) { color: #008000; }
			.l > :nth-last-child(1 of .x) { background-color: #008000; }`,
		tree: nCol("#l.l",
			nLeaf("#k1.x", 10, 10), nLeaf("#k2", 10, 10), nLeaf("#k3.x", 10, 10),
			nLeaf("#k4", 10, 10), nLeaf("#k5.x", 10, 10)),
		want: selBoth(selGreenFor("k3"), selRedFor("k1", "k2", "k5"),
			[]confCheck{wantComputed("k5", cpBackground, selGreen), wantComputed("k3", cpBackground, render.Color(0))}),
	},
	{
		name: "typed child-indexed pseudo-classes",
		wpt:  "css/selectors/of-type-selectors.xhtml",
		spec: "selectors-4 §14.4",
		css: `* { color: #ff0000; }
			.l > item:first-of-type { color: #008000; }
			.l > box:last-of-type { color: #008000; }
			.l > item:nth-of-type(3) { color: #008000; }
			.l > item:nth-last-of-type(2) { background-color: #008000; }
			.l > label:only-of-type { color: #008000; }`,
		tree: nCol("#l.l",
			nCol("#b1"), selElem("#s1", "item"), nCol("#b2"), selElem("#s2", "item"),
			nText("#t1", "x"), selElem("#s3", "item"), nCol("#b3"), selElem("#s4", "item")),
		want: selBoth(selGreenFor("s1", "b3", "s3", "t1"), selRedFor("b1", "b2", "s2", "s4"),
			[]confCheck{wantComputed("s3", cpBackground, selGreen), wantComputed("s4", cpBackground, render.Color(0))}),
	},
	{
		name: "empty matches only nodes without children or text",
		spec: "selectors-4 §14.2",
		css: `* { color: #ff0000; }
			:empty { color: #008000; }
			.c > * { background-color: #008000; }
			.c > :not(:empty) { background-color: #ff0000; }`,
		tree: nCol("#c.c",
			nCol("#emptybox"),
			nCol("#full", nLeaf("#inside", 10, 10)),
			nLeaf("#leaf", 10, 10),
			nText("#blank", ""),
			nText("#worded", "text")),
		want: selBoth(selGreenFor("emptybox", "leaf", "blank"),
			[]confCheck{
				wantComputed("full", cpBackground, selRed),
				wantComputed("worded", cpBackground, selRed),
				wantComputed("emptybox", cpBackground, selGreen),
				wantComputed("blank", cpBackground, selGreen),
			}),
	},
	{
		name: "root matches the tree root alone",
		spec: "selectors-4 §14.1",
		css: `* { background-color: #ff0000; }
			:root { background-color: #008000; }
			:root * { color: #008000; }`,
		tree: nCol("#c", nLeaf("#t", 10, 10)),
		want: []confCheck{
			wantComputed("root", cpBackground, selGreen),
			wantComputed("c", cpBackground, selRed),
			wantComputed("t", cpColor, selGreen),
		},
	},
	{
		name: "enabled and disabled follow the inherited sensitivity",
		spec: "selectors-4 §13.1",
		css: `* { color: #ff0000; }
			:enabled { color: #008000; }
			.off :disabled { color: #008000; }
			.off { color: #008000; }`,
		tree: nCol("#c",
			nLeaf("#on", 10, 10),
			nCol("#off.off", nLeaf("#inner", 10, 10))),
		act:  func(f *confFixture) { f.get("off").(*Box).SetEnabled(false) },
		want: selGreenFor("on", "off", "inner"),
	},
	{
		name: "read-only read-write and placeholder-shown",
		spec: "selectors-4 §13.2",
		css: `* { color: #ff0000; }
			entry:read-write { color: #008000; }
			#ro:read-only { color: #008000; }
			entry:placeholder-shown { background-color: #008000; }`,
		tree: nCol("#c", nEntry("#rw", "typed"), nEntry("#ro", "fixed"), selPlaceholder("#ph", "hint"), nEntry("#bare", "theme")),
		act:  func(f *confFixture) { f.get("ro").(*Entry).SetReadOnly(true) },
		want: selBoth(selGreenFor("rw", "ro", "ph"), []confCheck{
			wantComputed("ph", cpBackground, selGreen),
			wantComputedLike("rw", "bare", cpBackground),
		}),
	},
	{
		name: "focus focus-within and pointer focus without focus-visible",
		spec: "selectors-4 §9.2",
		css: `* { color: #ff0000; }
			entry:focus { color: #008000; }
			.w:focus-within { color: #008000; }
			entry:focus-visible { background-color: #ff0000; }`,
		tree: nCol("#w.w", nEntry("#e", "x"), nEntry("#other", "y")),
		act:  func(f *confFixture) { (&Router{Root: f.root}).SetFocus(f.get("e")) },
		want: selBoth(selGreenFor("e", "w"), selRedFor("other"),
			[]confCheck{wantComputedLike("e", "other", cpBackground)}),
	},
	{
		name: "keyboard focus matches focus-visible",
		wpt:  "css/selectors/focus-visible-001.html",
		spec: "selectors-4 §9.4",
		css: `* { color: #ff0000; }
			entry:focus-visible { color: #008000; }`,
		tree: nCol("#c", nEntry("#e", "x")),
		act:  func(f *confFixture) { (&Router{Root: f.root}).FocusNext() },
		want: selGreenFor("e"),
	},
	{
		name: "hover matches the hovered widget and its ancestors",
		spec: "selectors-4 §9.1",
		css: `* { color: #ff0000; }
			:hover { color: #008000; }`,
		tree: nCol("#c", nCol("#p", nLeaf("#t", 10, 10)), nLeaf("#sib", 10, 10)),
		act:  func(f *confFixture) { setHoverChain(nil, f.get("t")) },
		want: selBoth(selGreenFor("t", "p", "c"), selRedFor("sib")),
	},
	{
		name: "app-driven states checked selected and target",
		spec: "selectors-4 §8.2",
		css: `* { color: #ff0000; }
			:checked { color: #008000; }
			:selected { color: #008000; }
			:target { color: #008000; }`,
		tree: nCol("#c", nLeaf("#ck", 10, 10), nLeaf("#sl", 10, 10), nLeaf("#tg", 10, 10), nLeaf("#plain", 10, 10)),
		act: func(f *confFixture) {
			nodeOf(f.get("ck")).SetState(StateChecked, true)
			nodeOf(f.get("sl")).SetState(StateSelected, true)
			nodeOf(f.get("tg")).SetState(StateTarget, true)
		},
		want: selBoth(selGreenFor("ck", "sl", "tg"), selRedFor("plain")),
	},
	{
		name: "specificity orders ids over classes over elements",
		spec: "selectors-4 §17",
		css: `#t { color: #008000; }
			.t.t.t.t.t.t.t.t.t.t.t { color: #ff0000; }
			.u { background-color: #008000; }
			box box box box box stub { background-color: #ff0000; }`,
		tree: nCol("#c", nCol("#c2", nCol("#c3", nCol("#c4", nCol("#c5", nLeaf("#t.t.u", 10, 10)))))),
		want: []confCheck{
			wantComputed("t", cpColor, selGreen),
			wantComputed("t", cpBackground, selGreen),
		},
	},
	{
		name: "before and after content strings and attr()",
		spec: "css-content-3 §2",
		css: `.t::before { content: "<" attr(data-tag) ">"; }
			.t::after { content: "end"; }
			.t::after { content: none; }
			.u::after { content: "tail"; }`,
		tree: nCol("#c", selAttr("#t.t", "data-tag", "x"), nLeaf("#u.u", 10, 10)),
		want: []confCheck{
			wantProbe("::before content of #t", selPseudo("t", "before"), "<x>"),
			wantProbe("::after content of #t", selPseudo("t", "after"), ""),
			wantProbe("::after content of #u", selPseudo("u", "after"), "tail"),
		},
	},
	{
		name: "counters count in document order",
		spec: "css-lists-3 §4.4",
		css: `.l { counter-reset: n 4; }
			.i { counter-increment: n 2; }
			.i::before { content: counter(n) "."; }`,
		tree: nCol("#l.l", nLeaf("#a.i", 10, 10), nLeaf("#b.i", 10, 10), nLeaf("#c.i", 10, 10)),
		want: []confCheck{
			wantProbe("::before content of #a", selPseudo("a", "before"), "6."),
			wantProbe("::before content of #b", selPseudo("b", "before"), "8."),
			wantProbe("::before content of #c", selPseudo("c", "before"), "10."),
		},
	},
	{
		name: "nested counters join with counters()",
		spec: "css-lists-3 §4.5",
		css: `.ol { counter-reset: item; }
			.li { counter-increment: item; }
			.li::before { content: counters(item, ".") " "; }`,
		tree: nCol("#outer.ol",
			nLeaf("#one.li", 10, 10),
			nCol("#two.li", nCol("#inner.ol", nLeaf("#twoone.li", 10, 10), nLeaf("#twotwo.li", 10, 10))),
			nLeaf("#three.li", 10, 10)),
		want: []confCheck{
			wantProbe("::before content of #one", selPseudo("one", "before"), "1 "),
			wantProbe("::before content of #two", selPseudo("two", "before"), "2 "),
			wantProbe("::before content of #twoone", selPseudo("twoone", "before"), "2.1 "),
			wantProbe("::before content of #twotwo", selPseudo("twotwo", "before"), "2.2 "),
			wantProbe("::before content of #three", selPseudo("three", "before"), "3 "),
		},
	},
	{
		name: "a counter change re-counts the following elements",
		spec: "css-lists-3 §4.4",
		css: `.l { counter-reset: n; }
			.i { counter-increment: n; }
			.skip { counter-increment: none; }
			.i::before { content: counter(n); }`,
		tree: nCol("#l.l", nLeaf("#a.i", 1, 1), nLeaf("#b.i", 1, 1), nLeaf("#c.i", 1, 1)),
		act:  func(f *confFixture) { nodeOf(f.get("a")).AddClass("skip") },
		want: []confCheck{
			wantProbe("::before content of #a", selPseudo("a", "before"), "0"),
			wantProbe("::before content of #b", selPseudo("b", "before"), "1"),
			wantProbe("::before content of #c", selPseudo("c", "before"), "2"),
		},
	},
	{
		name: "pseudo-elements cascade their own style inheriting the element's",
		spec: "css-pseudo-4 §4",
		css: `.t { color: #008000; font-size: 20px; }
			.t::before { content: "x"; font-size: 10px; }`,
		tree: nCol("#c", nLeaf("#t.t", 10, 10)),
		want: []confCheck{
			wantProbe("::before color of #t", func(f *confFixture) any { return PseudoStyle(f.get("t"), "before").Color }, selGreen),
			wantProbe("::before font-size of #t", func(f *confFixture) any { return PseudoStyle(f.get("t"), "before").FontSize }, 10.0),
		},
	},
	{
		name: "placeholder and selection style the entry parts",
		spec: "css-pseudo-4 §3.2, §4.1",
		css: `entry::placeholder { color: #008000; }
			entry::selection { background-color: #008000; }`,
		tree: nCol("#c", selPlaceholder("#e", "hint")),
		want: []confCheck{
			wantProbe("::placeholder color", func(f *confFixture) any {
				e := f.get("e").(*Entry)
				return e.text.placeholder.style(&e.text.placeholder).Color
			}, selGreen),
			wantProbe("::selection background", func(f *confFixture) any {
				e := f.get("e").(*Entry)
				return e.text.selection.style(&e.text.selection).Background
			}, selGreen),
		},
	},
	{
		name: "marker styles the marker part",
		spec: "css-pseudo-4 §3.3",
		css: `* { color: #ff0000; }
			.li::marker { color: #008000; }`,
		tree: nCol("#li.li", selElem("#m", "marker"), selElem("#other", "check")),
		want: selBoth(selGreenFor("m"), selRedFor("other")),
	},
}

func TestCSSConformanceSelectors(t *testing.T) { runConformance(t, confSelectors) }

// confExceptionsSelectors are the Selectors group's deliberate
// deviations, each asserting the documented behavior.
var confExceptionsSelectors = []confException{
	{
		name: "first-line parses but never matches",
		wpt:  "css/css-pseudo/first-line-basic.html",
		spec: "css-pseudo-4 §2.1",
		css: `* { background-color: #ff0000; }
			@supports selector(label::first-line) { #t { background-color: #008000; } }
			#t { color: #ff0000; }
			#t::first-line { color: #008000; }`,
		tree: nCol("#c", nWrap("#t", "first line")),
		want: []confCheck{
			wantComputed("t", cpBackground, selGreen),
			wantComputed("t", cpColor, selRed),
			wantProbe("::first-line style of #t", func(f *confFixture) any { return PseudoStyle(f.get("t"), "first-line") == nil }, true),
		},
		reason: "line fragmentation happens inside the text shaper at paint time; a widget's text is one shaped run with no per-line box the cascade could address",
		doc:    "`::first-line` parses but never matches",
	},
}

func TestCSSConformanceSelectorsExceptions(t *testing.T) {
	runConformanceExceptions(t, confExceptionsSelectors)
}
