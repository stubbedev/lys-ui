package style

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// pseudoCompute is computeTree for one pseudo-element of n, inheriting
// from n's own computed style.
func pseudoCompute(s *Sheet, n *node, name string) Values {
	v := computeTree(s, n)
	var sc Scratch
	var out Values
	ComputePseudo([]Layer{{Sheet: s, Priority: PriorityApplication}}, n, &v, Env{Rem: 16}, &sc, &out, name)
	return out
}

// counterTree computes the tree around n in tree order, each node
// over its parent's values, with the counter inputs the widget layer
// supplies: the previous sibling's set and the set of the node
// computed just before it.
func counterTree(s *Sheet, n *node) Values {
	var sc Scratch
	layers := []Layer{{Sheet: s, Priority: PriorityApplication}}
	got := map[*node]*Values{}
	var last *node
	var walk func(c *node, pv *Values)
	walk = func(c *node, pv *Values) {
		var prev, src *CounterScope
		if p := c.parent; p != nil {
			if i := slices.Index(p.kids, c); i > 0 {
				prev, src = got[p.kids[i-1]].Counters, got[last].Counters
			}
		}
		out := &Values{}
		sc.SetPrevCounters(prev, src)
		Compute(layers, c, pv, nil, Env{Rem: 16}, &sc, out)
		got[c], last = out, c
		for _, k := range c.kids {
			walk(k, out)
		}
	}
	walk(rootOf(n), nil)
	return *got[n]
}

func rootOf(n *node) *node {
	for ; n.parent != nil; n = n.parent {
	}
	return n
}

func TestAttributeSelectors(t *testing.T) {
	s := parseOne(t, `
		[data-x] { color: #000001; }
		[data-x="b"] { color: #000002; }
		[data-x~="c"] { color: #000003; }
		[data-x|="en"] { color: #000004; }
		[data-x^="pre"] { color: #000005; }
		[data-x$="fix"] { color: #000006; }
		[data-x*="mid"] { color: #000007; }
		[data-x="B" i] { color: #000008; }
		[data-x="B" s] { color: #000009; }
		[id="save"] { color: #00000a; }
		[class~="urgent"] { color: #00000b; }
	`)
	n := func(v string) *node {
		c := el("button")
		if v != "" {
			c.target.Attrs = []Attr{{Name: "data-x", Value: v}}
		}
		return c
	}
	if v := computeTree(s, n("")); v.Has(PropColor) {
		t.Error("[data-x] matched without the attribute")
	}
	if got := computeTree(s, n("a")).Color; got != render.RGB(0, 0, 1) {
		t.Errorf("presence: %v", got)
	}
	if got := computeTree(s, n("b")).Color; got != render.RGB(0, 0, 8) {
		t.Errorf("[=] with the later case-insensitive rule at equal specificity: %v", got)
	}
	if got := computeTree(s, n("a c d")).Color; got != render.RGB(0, 0, 3) {
		t.Errorf("[~=] word: %v", got)
	}
	if v := computeTree(s, n("ac")); v.Has(PropColor) && v.Color == render.RGB(0, 0, 3) {
		t.Error("[~=] matched a substring")
	}
	if got := computeTree(s, n("en-US")).Color; got != render.RGB(0, 0, 4) {
		t.Errorf("[|=] prefix-dash: %v", got)
	}
	if got := computeTree(s, n("prefix-x")).Color; got != render.RGB(0, 0, 5) {
		t.Errorf("[^=]: %v", got)
	}
	if got := computeTree(s, n("suffix")).Color; got != render.RGB(0, 0, 6) {
		t.Errorf("$=: %v", got)
	}
	if got := computeTree(s, n("amidz")).Color; got != render.RGB(0, 0, 7) {
		t.Errorf("*=: %v", got)
	}
	s2 := parseOne(t, `
		[data-x="B" i] { color: #000008; }
		[data-x="b"] { color: #000002; }
	`)
	if got := computeTree(s2, n("B")).Color; got != render.RGB(0, 0, 8) {
		t.Errorf("i flag: %v", got)
	}
	if got := computeTree(s2, n("b")).Color; got != render.RGB(0, 0, 2) {
		t.Errorf("i flag lost to the later exact rule at equal specificity: %v", got)
	}
	s3 := parseOne(t, `[data-x="B" s] { color: #000009; } [data-x="b"] { color: #000002; }`)
	if v := computeTree(s3, n("b")); v.Color == render.RGB(0, 0, 9) {
		t.Error("s flag matched the wrong case")
	}
	if got := computeTree(s3, n("B")).Color; got != render.RGB(0, 0, 9) {
		t.Errorf("s flag exact: %v", got)
	}
	idn := el("button")
	idn.target.ID = "save"
	if got := computeTree(s, idn).Color; got != render.RGB(0, 0, 0xa) {
		t.Errorf("[id=]: %v", got)
	}
	clsn := el("button", "urgent")
	if got := computeTree(s, clsn).Color; got != render.RGB(0, 0, 0xb) {
		t.Errorf("[class~=]: %v", got)
	}
	rolen := el("button")
	rolen.target.Role = "button"
	sRole := parseOne(t, `[role="button"] { color: #00000c; }`)
	if got := computeTree(sRole, rolen).Color; got != render.RGB(0, 0, 0xc) {
		t.Errorf("[role=]: %v", got)
	}
}

func TestIsWhereSpecificity(t *testing.T) {
	s := parseOne(t, `
		:is(#hi, .a) { color: #000001; }
		.b { color: #000002; }
		:where(#hi, .a) { color: #000003; }
	`)
	n := el("x", "a", "b")
	n.target.ID = "hi"
	// :is(#hi, .a) carries the id's specificity and beats .b.
	if got := computeTree(s, n).Color; got != render.RGB(0, 0, 1) {
		t.Errorf(":is specificity: %v", got)
	}
	n2 := el("x", "a", "b")
	// :where(...) counts nothing, so the class rule beats it.
	sWhere := parseOne(t, `
		:where(#hi, .a) { color: #000003; }
		.b { color: #000002; }
	`)
	if got := computeTree(sWhere, n2).Color; got != render.RGB(0, 0, 2) {
		t.Errorf(":where specificity: %v", got)
	}
	s2 := parseOne(t, `
		:is(.p) :is(.q) { color: #000004; }
		x.y { color: #000005; }
	`)
	root := el("box", "p")
	kid := root.add("x", "q", "y")
	if got := computeTree(s2, kid).Color; got != render.RGB(0, 0, 4) {
		t.Errorf(":is with combinators inside: %v", got)
	}
}

func TestHasRelational(t *testing.T) {
	s := parseOne(t, `
		.card:has(img) { color: #000001; }
		.card:has(> img) { background-color: #000002; }
		.card:has(+ img) { margin-top: 1px; }
		.card:has(~ img) { margin-bottom: 2px; }
		.card:has(.deep .mark) { padding-top: 3px; }
		.card:has(> .body .mark) { padding-bottom: 4px; }
	`)
	card := func(build func(*node)) *node {
		c := el("box", "card")
		build(c)
		return c
	}
	if got := computeTree(s, card(func(c *node) { c.add("label") })).Color; got != 0 {
		t.Error(":has(img) matched without an img")
	}
	if got := computeTree(s, card(func(c *node) { c.add("label").add("img") })).Color; got != render.RGB(0, 0, 1) {
		t.Errorf(":has descendant: %v", got)
	}
	if v := computeTree(s, card(func(c *node) { c.add("label").add("img") })); v.Has(PropBackgroundColor) {
		t.Error(":has(> img) matched a non-child img")
	}
	if got := computeTree(s, card(func(c *node) { c.add("img") })).Background; got != render.RGB(0, 0, 2) {
		t.Errorf(":has child: %v", got)
	}
	row := el("box")
	first := row.add("box", "card")
	row.add("img")
	if got := computeTree(s, first).Margin.Top; got != 1 {
		t.Errorf(":has next sibling: %d", got)
	}
	if got := computeTree(s, first).Margin.Bottom; got != 2 {
		t.Errorf(":has following sibling: %d", got)
	}
	deep := card(func(c *node) { c.add("div", "deep").add("span", "mark") })
	if got := computeTree(s, deep).Padding.Top; got != 3 {
		t.Errorf(":has complex descendant: %d", got)
	}
	body := card(func(c *node) { c.add("div").add("span", "body").add("i", "mark") })
	if v := computeTree(s, body); v.Padding.Bottom != 0 {
		t.Error(":has(> .body .mark) matched a .body that is not a child")
	}
	body2 := card(func(c *node) { c.add("div", "body").add("i", "mark") })
	if got := computeTree(s, body2).Padding.Bottom; got != 4 {
		t.Errorf(":has(> .body .mark): %d", got)
	}
}

func TestNotComplex(t *testing.T) {
	s := parseOne(t, `
		item:not(.skip, box > .flag) { color: #000001; }
	`)
	list := el("list")
	plain := list.add("item")
	skip := list.add("item", "skip")
	flagged := el("list").add("box").add("item", "flag")
	if got := computeTree(s, plain).Color; got != render.RGB(0, 0, 1) {
		t.Errorf(":not complex, plain: %v", got)
	}
	if v := computeTree(s, skip); v.Has(PropColor) {
		t.Error(":not(.skip) matched a .skip")
	}
	if v := computeTree(s, flagged); v.Has(PropColor) {
		t.Error(":not(box > .flag) matched a child of a box")
	}
}

func TestOfTypeStructural(t *testing.T) {
	s := parseOne(t, `
		p:nth-of-type(2) { color: #000001; }
		p:nth-last-of-type(1) { background-color: #000002; }
		p:first-of-type { margin-top: 1px; }
		p:last-of-type { margin-bottom: 2px; }
		p:only-of-type { margin-left: 3px; }
	`)
	root := el("box")
	p1 := root.add("p")
	root.add("h1")
	p2 := root.add("p")
	root.add("h1")
	p3 := root.add("p")
	if v := computeTree(s, p1); v.Has(PropColor) || v.Has(PropBackgroundColor) || v.Has(PropMarginBottom) || v.Margin.Left != 0 {
		t.Errorf("p1: %+v", v)
	}
	if v := computeTree(s, p2); v.Color != render.RGB(0, 0, 1) || v.Has(PropBackgroundColor) {
		t.Errorf("p2 (nth-of-type over h1s): color %v", v.Color)
	}
	if v := computeTree(s, p3); v.Has(PropColor) || v.Background != render.RGB(0, 0, 2) || v.Margin.Bottom != 2 {
		t.Errorf("p3: %+v", v)
	}
	only := el("box").add("p")
	if v := computeTree(s, only); v.Margin.Left != 3 {
		t.Errorf(":only-of-type: %d", v.Margin.Left)
	}
	if v := computeTree(s, only); v.Margin.Top != 1 || v.Margin.Bottom != 2 {
		t.Errorf("first/last-of-type on the only p: %+v", v.Margin)
	}
}

func TestEmptyAndEnabledPseudos(t *testing.T) {
	s := parseOne(t, `
		box:empty { color: #000001; }
		button:enabled { background-color: #000002; }
		entry:read-only { margin-top: 1px; }
		entry:read-write { margin-bottom: 2px; }
		entry:placeholder-shown { padding-top: 4px; }
	`)
	if got := computeTree(s, el("box")).Color; got != render.RGB(0, 0, 1) {
		t.Errorf(":empty: %v", got)
	}
	full := el("box")
	full.add("label")
	if v := computeTree(s, full); v.Has(PropColor) {
		t.Error(":empty matched a parent")
	}
	b := el("button")
	if got := computeTree(s, b).Background; got != render.RGB(0, 0, 2) {
		t.Errorf(":enabled: %v", got)
	}
	b.target.State |= Disabled
	if v := computeTree(s, b); v.Has(PropBackgroundColor) {
		t.Error(":enabled matched a disabled button")
	}
	ro := el("entry")
	ro.target.State = ReadOnly
	if got := computeTree(s, ro).Margin.Top; got != 1 {
		t.Errorf(":read-only: %d", got)
	}
	if v := computeTree(s, ro); v.Has(PropMarginBottom) {
		t.Error(":read-write matched a read-only entry")
	}
	rw := el("entry")
	if got := computeTree(s, rw).Margin.Bottom; got != 2 {
		t.Errorf(":read-write: %d", got)
	}
	ph := el("entry")
	ph.target.State = PlaceholderShown
	if got := computeTree(s, ph).Padding.Top; got != 4 {
		t.Errorf(":placeholder-shown: %d", got)
	}
}

func TestPseudoElementBeforeAfter(t *testing.T) {
	s := parseOne(t, `
		label::before { content: "«"; color: #000001; }
		label::after { content: " »"; }
		label::before { content: counter(item) " of " attr(data-kind); color: #000002; }
	`)
	n := el("label")
	n.target.Attrs = []Attr{{Name: "data-kind", Value: "many"}}
	v := pseudoCompute(s, n, "before")
	if !v.HasContent || v.Content != "0 of many" {
		t.Errorf("::before content = %q has=%v, want %q", v.Content, v.HasContent, "0 of many")
	}
	if got := v.Color; got != render.RGB(0, 0, 2) {
		t.Errorf("::before later rule wins: %v", got)
	}
	after := pseudoCompute(s, n, "after")
	if !after.HasContent || after.Content != " »" {
		t.Errorf("::after content = %q has=%v", after.Content, after.HasContent)
	}
	if after.Has(PropColor) {
		t.Error("::after inherited the ::before rule")
	}
	if plain := computeTree(s, n); plain.HasContent || plain.Content != "" {
		t.Error("the element's own compute picked up a pseudo rule")
	}
}

func TestCounters(t *testing.T) {
	s := parseOne(t, `
		list { counter-reset: item; }
		item { counter-increment: item; }
		item::before { content: counters(item, ".") " "; }
		deep { counter-reset: item 9; }
	`)
	list := el("list")
	i1 := list.add("item")
	i2 := list.add("item")
	if got := counterTree(s, i1).Counters.get("item"); got != 1 {
		t.Errorf("first item's counter = %d, want 1", got)
	}
	if got := counterTree(s, i2).Counters.get("item"); got != 2 {
		t.Errorf("second item's counter = %d, want 2 (document order)", got)
	}
	nested := list
	deep := nested.add("deep")
	d1 := deep.add("item")
	c := counterTree(s, d1)
	if got := c.Counters.get("item"); got != 10 {
		t.Errorf("reset counter incremented = %d, want 10", got)
	}
	sJoin := parseOne(t, `
		deep { counter-reset: item 9; }
		item { counter-increment: item; }
		item::before { content: counters(item, ".") " "; }
	`)
	root := el("deep")
	kid := root.add("item")
	pv := counterTree(sJoin, kid)
	var sc Scratch
	var out Values
	ComputePseudo([]Layer{{Sheet: sJoin, Priority: PriorityApplication}}, kid, &pv, Env{Rem: 16}, &sc, &out, "before")
	// The increment bumps deep's counter, the innermost one in scope;
	// it instantiates no nested counter (css-lists-3 §4.4).
	if out.Content != "10 " {
		t.Errorf("counters(item, \".\") = %q, want %q", out.Content, "10 ")
	}
}

func TestPartPseudoElements(t *testing.T) {
	s := parseOne(t, `
		entry::placeholder { color: #000001; }
		entry::selection { background-color: #000002; }
		list ::marker { color: #000003; }
	`)
	entry := el("entry")
	text := entry.add("text")
	placeholder := text.add("placeholder")
	selection := text.add("selection")
	if got := computeTree(s, placeholder).Color; got != render.RGB(0, 0, 1) {
		t.Errorf("::placeholder styles the placeholder part: %v", got)
	}
	if got := computeTree(s, selection).Background; got != render.RGB(0, 0, 2) {
		t.Errorf("::selection styles the selection part: %v", got)
	}
	if v := computeTree(s, entry); v.Has(PropColor) {
		t.Error("::placeholder styled the entry itself")
	}
	list := el("list")
	marker := list.add("marker")
	if got := computeTree(s, marker).Color; got != render.RGB(0, 0, 3) {
		t.Errorf("::marker: %v", got)
	}
}

func TestSelectorFormsAllocateNothingPerMatch(t *testing.T) {
	s := parseOne(t, `
		[data-x="a"]:is(.p, .q):not(.skip):placeholder-shown { color: #000001; }
		.p:empty:enabled:read-write { background-color: #000002; }
	`)
	n := el("item", "p")
	n.target.Attrs = []Attr{{Name: "data-x", Value: "a"}}
	n.target.State = PlaceholderShown
	var sc Scratch
	var v Values
	layers := []Layer{{Sheet: s, Priority: PriorityApplication}}
	Compute(layers, n, nil, nil, Env{Rem: 16}, &sc, &v)
	if !v.Has(PropColor) {
		t.Fatal("the fixture rule did not match")
	}
	if got := testing.AllocsPerRun(100, func() {
		Compute(layers, n, nil, &v, Env{Rem: 16}, &sc, &v)
	}); got != 0 {
		t.Errorf("compute with attribute/:is/:not/of-type selectors allocated %v per run", got)
	}
}

func TestHasBudget(t *testing.T) {
	s := parseOne(t, `.card:has(.mark) { color: #000001; }`)
	card := el("box", "card")
	deep := card
	for range maxHasBudget + 8 {
		deep = deep.add("box")
	}
	deep.add("span", "mark")
	if v := computeTree(s, card); v.Has(PropColor) {
		t.Error(":has matched past the visit budget")
	}
}
