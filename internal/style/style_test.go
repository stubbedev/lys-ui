package style

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// capture swaps the warning sink for one test and returns the messages.
func capture(tb testing.TB) *[]string {
	tb.Helper()
	var warns []string
	prev := parseWarn
	parseWarn = func(msg string) { warns = append(warns, msg) }
	tb.Cleanup(func() { parseWarn = prev })
	return &warns
}

// node is a synthetic widget tree for matcher tests.
type node struct {
	target Target
	parent *node
	kids   []*node
	hidden bool
	// cont/hasCont stand in for the widget layer's query container
	// resolution (the nearest container-type ancestor).
	cont    Container
	hasCont bool
}

// StyleContainers reports the node's stand-in query container.
func (n *node) StyleContainers(buf []Container) []Container {
	if n.hasCont {
		buf = append(buf, n.cont)
	}
	return buf
}

func (n *node) StyleTarget(t *Target) { *t = n.target }

func (n *node) StyleParent() Node {
	if n.parent == nil {
		return nil
	}
	return n.parent
}

// siblings returns the parent's visible children.
func (n *node) siblings() []*node {
	if n.parent == nil {
		return []*node{n}
	}
	var out []*node
	for _, k := range n.parent.kids {
		if !k.hidden {
			out = append(out, k)
		}
	}
	return out
}

func (n *node) StylePrev() Node {
	sib := n.siblings()
	for i, k := range sib {
		if k == n && i > 0 {
			return sib[i-1]
		}
	}
	return nil
}

func (n *node) StylePosition() (int, int) {
	sib := n.siblings()
	for i, k := range sib {
		if k == n {
			return i + 1, len(sib)
		}
	}
	return 1, 1
}

func (n *node) StyleTypedPosition(element string) (int, int) {
	sib := n.siblings()
	idx, cnt := 0, 0
	for _, k := range sib {
		if k.target.Element == element {
			cnt++
			if k == n {
				idx = cnt
			}
		}
	}
	return idx, cnt
}

func (n *node) StyleChildren() []Node {
	var out []Node
	for _, k := range n.kids {
		if !k.hidden {
			out = append(out, k)
		}
	}
	return out
}

func el(element string, classes ...string) *node {
	return &node{target: Target{Element: element, Classes: classes}}
}

func (n *node) add(element string, classes ...string) *node {
	c := el(element, classes...)
	c.parent = n
	n.kids = append(n.kids, c)
	return c
}

// parseOne parses css and fails the test when any warning fired.
func parseOne(tb testing.TB, css string) *Sheet {
	tb.Helper()
	warns := capture(tb)
	s := Parse(css)
	if len(*warns) != 0 {
		tb.Fatalf("Parse(%q) warned: %v", css, *warns)
	}
	return s
}

// computeTree computes n and its ancestors root-first and returns n's
// values, so inheritance and var() see a real parent chain.
func computeTree(s *Sheet, n *node) Values {
	return computeLayers([]Layer{{Sheet: s, Priority: PriorityApplication}}, n)
}

func computeLayers(layers []Layer, n *node) Values {
	var chain []*node
	for p := n; p != nil; p = p.parent {
		chain = append([]*node{p}, chain...)
	}
	var sc Scratch
	var parent *Values
	var v Values
	for _, c := range chain {
		v = Values{}
		Compute(layers, c, parent, nil, Env{Rem: 16}, &sc, &v)
		pv := v
		parent = &pv
	}
	return v
}

func TestTokenizer(t *testing.T) {
	ts := tokenize(`a.b:hover > #c { width: calc(1.5rem * -2); color: rgba(1, 2, 3, 50%); content: "x\"y" /* c */ }`)
	var kinds []tokKind
	for _, tk := range ts {
		kinds = append(kinds, tk.kind)
	}
	want := []tokKind{
		tkIdent, tkDelim, tkIdent, tkColon, tkIdent, tkWS, tkDelim, tkWS, tkHash, tkWS, tkLBrace, tkWS,
		tkIdent, tkColon, tkWS, tkFunc, tkDim, tkWS, tkDelim, tkWS, tkNumber, tkRParen, tkSemi, tkWS,
		tkIdent, tkColon, tkWS, tkFunc, tkNumber, tkComma, tkWS, tkNumber, tkComma, tkWS, tkNumber, tkComma, tkWS, tkPercent, tkRParen, tkSemi, tkWS,
		tkIdent, tkColon, tkWS, tkString, tkWS, tkRBrace,
	}
	if !slices.Equal(kinds, want) {
		t.Fatalf("kinds\n got %v\nwant %v", kinds, want)
	}
	if ts[16].num != 1.5 || ts[16].s != "rem" || ts[20].num != -2 {
		t.Errorf("dimension/number lexed as %+v / %+v", ts[16], ts[20])
	}
	if s := ts[44].s; s != `x"y` {
		t.Errorf("string = %q, want the escape resolved", s)
	}
}

func TestSelectorsMatch(t *testing.T) {
	s := parseOne(t, `
		button { color: #000001; }
		.x.y { color: #000002; }
		#id { color: #000003; }
		box > label { color: #000004; }
		window label.deep { color: #000005; }
		.a > .b .c { color: #000006; }
		label + label { background-color: #000007; }
		.first ~ .later { background-color: #000008; }
		item:first-child { padding-top: 1px; }
		item:last-child { padding-bottom: 2px; }
		item:only-child { padding-left: 3px; }
		item:nth-child(2n+1) { padding-right: 4px; }
		item:not(.skip):not(:hover) { margin-top: 5px; }
		:root { margin-left: 6px; }
		button:checked:focus-visible { margin-bottom: 7px; }
	`)
	if got := computeTree(s, el("button")).Color; got != render.RGB(0, 0, 1) {
		t.Errorf("element: %v", got)
	}
	if got := computeTree(s, el("x", "y", "x")).Color; got != render.RGB(0, 0, 2) {
		t.Errorf("compound classes: %v", got)
	}
	if v := computeTree(s, el("q", "x")); v.Has(PropColor) {
		t.Error(".x.y matched a node with only .x")
	}
	n := el("q")
	n.target.ID = "id"
	if got := computeTree(s, n).Color; got != render.RGB(0, 0, 3) {
		t.Errorf("id: %v", got)
	}

	box := el("box")
	lbl := box.add("label")
	if got := computeTree(s, lbl).Color; got != render.RGB(0, 0, 4) {
		t.Errorf("child combinator: %v", got)
	}
	win := el("window")
	deep := win.add("box").add("box").add("label", "deep")
	if got := computeTree(s, deep).Color; got != render.RGB(0, 0, 5) {
		t.Errorf("descendant combinator: %v", got)
	}

	// `.a > .b .c` needs backtracking: the nearest .b is not a child of
	// .a, a farther one is.
	a := el("x", "a")
	c := a.add("x", "b").add("x", "b").add("x", "c")
	if got := computeTree(s, c).Color; got != render.RGB(0, 0, 6) {
		t.Errorf("backtracking descendant: %v", got)
	}
	notA := el("x", "z")
	c2 := notA.add("x", "b").add("x", "c")
	if v := computeTree(s, c2); v.Has(PropColor) {
		t.Error(".a > .b .c matched without .a")
	}

	row := el("box")
	l1 := row.add("label")
	l2 := row.add("label")
	if v := computeTree(s, l1); v.Has(PropBackgroundColor) {
		t.Error("label + label matched the first label")
	}
	if got := computeTree(s, l2).Background; got != render.RGB(0, 0, 7) {
		t.Errorf("adjacent sibling: %v", got)
	}
	l1.hidden = true
	if v := computeTree(s, l2); v.Has(PropBackgroundColor) {
		t.Error("a hidden sibling still counted for +")
	}

	sib := el("box")
	sib.add("x", "first")
	sib.add("x")
	later := sib.add("x", "later")
	if got := computeTree(s, later).Background; got != render.RGB(0, 0, 8) {
		t.Errorf("general sibling: %v", got)
	}

	list := el("list")
	i1 := list.add("item")
	i2 := list.add("item", "skip")
	i3 := list.add("item")
	v1, v2, v3 := computeTree(s, i1), computeTree(s, i2), computeTree(s, i3)
	if v1.Padding.Top != 1 || v2.Has(PropPaddingTop) || v3.Has(PropPaddingTop) {
		t.Errorf(":first-child: %v %v %v", v1.Padding, v2.Padding, v3.Padding)
	}
	if v3.Padding.Bottom != 2 || v1.Has(PropPaddingBottom) {
		t.Errorf(":last-child: %v %v", v1.Padding, v3.Padding)
	}
	if v1.Has(PropPaddingLeft) {
		t.Error(":only-child matched one of three")
	}
	if v1.Padding.Right != 4 || v2.Has(PropPaddingRight) || v3.Padding.Right != 4 {
		t.Errorf(":nth-child(2n+1): %v %v %v", v1.Padding, v2.Padding, v3.Padding)
	}
	if v1.Margin.Top != 5 || v2.Has(PropMarginTop) {
		t.Errorf(":not(.skip): %v %v", v1.Margin, v2.Margin)
	}
	i1.target.State = Hover
	if v := computeTree(s, i1); v.Has(PropMarginTop) {
		t.Error(":not(:hover) matched a hovered item")
	}
	only := el("list").add("item")
	if v := computeTree(s, only); v.Padding.Left != 3 {
		t.Errorf(":only-child: %v", v.Padding)
	}

	if v := computeTree(s, el("anything")); v.Margin.Left != 6 {
		t.Errorf(":root: %v", v.Margin)
	}
	if v := computeTree(s, el("box").add("anything")); v.Has(PropMarginLeft) {
		t.Error(":root matched a child")
	}

	b := el("button")
	b.target.State = Checked | FocusVisible
	if v := computeTree(s, b); v.Margin.Bottom != 7 {
		t.Errorf(":checked:focus-visible: %v", v.Margin)
	}
	b.target.State = Checked | Focus
	if v := computeTree(s, b); v.Has(PropMarginBottom) {
		t.Error(":focus-visible matched plain focus")
	}
}

func TestNthMicrosyntax(t *testing.T) {
	cases := []struct {
		in   string
		a, b int
		ok   bool
	}{
		{"odd", 2, 1, true},
		{"even", 2, 0, true},
		{"3", 0, 3, true},
		{"n", 1, 0, true},
		{"-n+3", -1, 3, true},
		{"2n - 1", 2, -1, true},
		{"+3n+2", 3, 2, true},
		{"x", 0, 0, false},
		{"2n+", 0, 0, false},
	}
	for _, c := range cases {
		x, ok := parseNth(c.in)
		if ok != c.ok || ok && (x.a != c.a || x.b != c.b) {
			t.Errorf("parseNth(%q) = %+v %v, want %d %d %v", c.in, x, ok, c.a, c.b, c.ok)
		}
	}
	x, _ := parseNth("-n+3")
	for pos, want := range map[int]bool{1: true, 3: true, 4: false} {
		if x.matches(pos, 5) != want {
			t.Errorf("-n+3 at %d: want %v", pos, want)
		}
	}
}

func TestCascadeSpecificityOrderAndPriority(t *testing.T) {
	s := parseOne(t, `
		button { color: #010101; }
		.x { color: #020202; }
		button.x { color: #030303; }
		#i { color: #040404; }
		button:not(#i).x { background-color: #050505; }
		button.x.y.z { background-color: #060606; }
	`)
	n := el("button", "x")
	if got := computeTree(s, n).Color; got != render.RGB(3, 3, 3) {
		t.Errorf("button.x lost to lower specificity: %v", got)
	}
	n.target.ID = "i"
	if got := computeTree(s, n).Color; got != render.RGB(4, 4, 4) {
		t.Errorf("id lost to class+element: %v", got)
	}
	// :not(#i) carries id specificity: it outranks three classes.
	n2 := el("button", "x", "y", "z")
	if got := computeTree(s, n2).Background; got != render.RGB(5, 5, 5) {
		t.Errorf(":not() specificity: %v", got)
	}

	s = parseOne(t, "button { color: #101010; color: #303030; } button { color: #202020; }")
	if got := computeTree(s, el("button")).Color; got != render.RGB(0x20, 0x20, 0x20) {
		t.Errorf("source order tie-break: %v", got)
	}
	s = parseOne(t, "button { color: #101010; color: #303030; }")
	if got := computeTree(s, el("button")).Color; got != render.RGB(0x30, 0x30, 0x30) {
		t.Errorf("in-rule tie-break: %v", got)
	}

	// Priority beats specificity, GTK's provider model.
	hi := parseOne(t, "* { color: #0000aa; }")
	lo := parseOne(t, "#i.x.y button { color: #aa0000; } button { color: #aa0000; }")
	layers := []Layer{{Sheet: hi, Priority: PriorityUser}, {Sheet: lo, Priority: PriorityApplication}}
	if got := computeLayers(layers, el("button")).Color; got != render.RGB(0, 0, 0xaa) {
		t.Errorf("higher-priority universal rule lost: %v", got)
	}
	// Equal priority: the later layer wins ties.
	a := parseOne(t, "button { color: #0000aa; }")
	b := parseOne(t, "button { color: #00aa00; }")
	layers = []Layer{{Sheet: a, Priority: PriorityUser}, {Sheet: b, Priority: PriorityUser}}
	if got := computeLayers(layers, el("button")).Color; got != render.RGB(0, 0xaa, 0) {
		t.Errorf("later layer lost the tie: %v", got)
	}
}

func TestInlineDeclarations(t *testing.T) {
	s := parseOne(t, "button { color: #111111; background-color: #222222; } .pin { background-color: #333333; }")
	n := el("button", "pin")
	n.target.Inline = ParseDeclarations("color: #abcdef; --tint: #010203")
	n.target.InlinePriority = PriorityUser
	v := computeLayers([]Layer{{Sheet: s, Priority: PriorityApplication}}, n)
	if v.Color != render.RGB(0xab, 0xcd, 0xef) {
		t.Errorf("inline at a higher priority lost: %v", v.Color)
	}
	if v.Background != render.RGB(0x33, 0x33, 0x33) {
		t.Errorf("inline block clobbered an undeclared property: %v", v.Background)
	}
	if got, ok := v.Var("--tint"); !ok || got != "#010203" {
		t.Errorf("inline custom property = %q %v", got, ok)
	}
	// At a lower priority than the sheet, inline declarations lose —
	// the per-widget provider under a display-wide bundle.
	n.target.InlinePriority = PriorityApplication - 1
	if v := computeLayers([]Layer{{Sheet: s, Priority: PriorityApplication}}, n); v.Color != render.RGB(0x11, 0x11, 0x11) {
		t.Errorf("lower-priority inline won: %v", v.Color)
	}
	warns := capture(t)
	if b := ParseDeclarations("color: nope; padding: 2px"); b.Len() != 1 || len(*warns) != 1 {
		t.Errorf("bad inline declaration: len %d, %d warnings", b.Len(), len(*warns))
	}
}

func TestShorthandsAndLonghands(t *testing.T) {
	s := parseOne(t, `
		a { padding: 1px; margin: 1px 2px; border-width: 1px 2px 3px; border-radius: 1px 2px 3px 4px; }
		b { padding: 1px 2px 3px 4px; padding-left: 9px; margin: -2px 3px 4px; }
		c { border: 2px solid #ff0000; border-top: 5px dashed; border-left-width: 0; }
		d { border-bottom: 1px solid #00ff00; }
		e { border-width: 3px; }
		f { border-radius: 50%; }
		g { outline: 2px solid #0000ff; outline-offset: -1px; }
		h { border-spacing: 4px 6px; }
		i { border-spacing: 3px; }
		j { border: none; }
	`)
	v := computeTree(s, el("a"))
	if v.Padding != (Sides{1, 1, 1, 1}) || v.Margin != (Sides{1, 2, 1, 2}) {
		t.Errorf("1/2-value: padding %v margin %v", v.Padding, v.Margin)
	}
	if v.BorderWidth != (Sides{1, 2, 3, 2}) {
		t.Errorf("3-value border-width: %v", v.BorderWidth)
	}
	if v.Radius != (Corners{1, 2, 3, 4}) {
		t.Errorf("4-value radius: %v", v.Radius)
	}
	v = computeTree(s, el("b"))
	if v.Padding != (Sides{1, 2, 3, 9}) {
		t.Errorf("longhand after shorthand: %v", v.Padding)
	}
	if v.Margin != (Sides{-2, 3, 4, 3}) {
		t.Errorf("negative margin: %v", v.Margin)
	}
	v = computeTree(s, el("c"))
	if v.EffBorder() != (Sides{5, 2, 2, 0}) {
		t.Errorf("border sides: %v", v.EffBorder())
	}
	if v.BorderStyle[0] != BorderDashed || v.BorderColor[1] != render.RGB(0xff, 0, 0) {
		t.Errorf("border style/color: %v %v", v.BorderStyle, v.BorderColor)
	}
	v = computeTree(s, el("d"))
	if v.EffBorder() != (Sides{Bottom: 1}) || v.BorderColor[2] != render.RGB(0, 0xff, 0) {
		t.Errorf("border-bottom: %v %v", v.EffBorder(), v.BorderColor)
	}
	// A width without a visible style draws nothing (GTK computes it 0).
	v = computeTree(s, el("e"))
	if v.BorderWidth.Top != 3 || v.EffBorder() != (Sides{}) {
		t.Errorf("border-width without style: %v / %v", v.BorderWidth, v.EffBorder())
	}
	if v := computeTree(s, el("f")); v.Radius.TopLeft < 1000 {
		t.Errorf("50%% radius: %v", v.Radius)
	}
	v = computeTree(s, el("g"))
	if v.EffOutline() != 2 || v.OutlineColor != render.RGB(0, 0, 0xff) || v.OutlineOffset != -1 {
		t.Errorf("outline: %d %v %d", v.EffOutline(), v.OutlineColor, v.OutlineOffset)
	}
	if v := computeTree(s, el("h")); v.BorderSpacingH != 4 || v.BorderSpacingV != 6 {
		t.Errorf("border-spacing pair: %d %d", v.BorderSpacingH, v.BorderSpacingV)
	}
	if v := computeTree(s, el("i")); v.BorderSpacingH != 3 || v.BorderSpacingV != 3 {
		t.Errorf("border-spacing single: %d %d", v.BorderSpacingH, v.BorderSpacingV)
	}
	if v := computeTree(s, el("j")); v.EffBorder() != (Sides{}) || !v.Has(PropBorderTopStyle) {
		t.Errorf("border: none: %v", v.EffBorder())
	}
}

func TestInvalidValuesAreDropped(t *testing.T) {
	warns := capture(t)
	s := Parse(`
		a { padding: 4px; padding: -1px; }
		b { color: #00ff00; color: notacolor; }
		c { margin: 1px 2px 3px 4px 5px; }
		d { font-size: 0; }
		e { opacity: 2; }
	`)
	if len(*warns) != 4 {
		t.Errorf("got %d warnings, want 4: %v", len(*warns), *warns)
	}
	if v := computeTree(s, el("a")); v.Padding.Top != 4 {
		t.Errorf("negative padding replaced the valid one: %v", v.Padding)
	}
	if v := computeTree(s, el("b")); v.Color != render.RGB(0, 0xff, 0) {
		t.Errorf("an invalid color won: %v", v.Color)
	}
	if v := computeTree(s, el("c")); v.Has(PropMarginTop) {
		t.Error("a five-value margin applied")
	}
	if v := computeTree(s, el("d")); v.Has(PropFontSize) {
		t.Error("font-size 0 applied")
	}
	// Opacity clamps into range; it is valid CSS.
	if v := computeTree(s, el("e")); v.Opacity != 1 {
		t.Errorf("opacity clamp: %v", v.Opacity)
	}
}

func TestColors(t *testing.T) {
	s := parseOne(t, `
		a { color: #123; background-color: #11223380; }
		b { color: rgba(255, 0, 0, 0.5); background-color: rgb(0 128 255 / 50%); }
		c { color: red; background-color: transparent; }
		d { color: hsl(120, 100%, 50%); }
		e { color: color-mix(in srgb, #ff0000 25%, #0000ff); background-color: color-mix(in srgb, #ffffff 40%, transparent); }
		f { color: color-mix(in srgb, #ff0000 20%, #0000ff 20%); }
		g { color: #ff0000; border: 1px solid currentColor; box-shadow: 0 0 2px; }
		h { color: alpha(#ff0000, 0.5); background-color: mix(#000000, #ffffff, 0.5); }
	`)
	v := computeTree(s, el("a"))
	if v.Color != render.RGB(0x11, 0x22, 0x33) || v.Background != render.RGBA(0x11, 0x22, 0x33, 0x80) {
		t.Errorf("hex: %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	v = computeTree(s, el("b"))
	if v.Color.A() != 128 || v.Color.R() != 128 || v.Background.A() != 128 || v.Background.B() != 128 {
		t.Errorf("rgb(): %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	v = computeTree(s, el("c"))
	if v.Color != render.RGB(0xff, 0, 0) || !v.Has(PropBackgroundColor) || v.Background != 0 {
		t.Errorf("named: %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	if v := computeTree(s, el("d")); v.Color != render.RGB(0, 0xff, 0) {
		t.Errorf("hsl: %08x", uint32(v.Color))
	}
	v = computeTree(s, el("e"))
	if v.Color != render.RGB(0x40, 0, 0xbf) {
		t.Errorf("color-mix 25%%: %08x", uint32(v.Color))
	}
	// Mixing with transparent keeps the hue and scales alpha
	// (premultiplied interpolation).
	if v.Background != render.RGBA(0xff, 0xff, 0xff, 0x66) {
		t.Errorf("color-mix with transparent: %08x", uint32(v.Background))
	}
	// Percentages summing under 100 scale the result's alpha.
	if v := computeTree(s, el("f")); v.Color.A() != 102 {
		t.Errorf("color-mix under 100%%: %08x", uint32(v.Color))
	}
	v = computeTree(s, el("g"))
	if v.BorderColor[0] != render.RGB(0xff, 0, 0) || v.Shadow.N != 1 || v.Shadow.Layers[0].Color != render.RGB(0xff, 0, 0) {
		t.Errorf("currentColor: border %08x shadow %+v", uint32(v.BorderColor[0]), v.Shadow.List())
	}
	v = computeTree(s, el("h"))
	if v.Color.A() != 128 || v.Background != render.RGB(0x80, 0x80, 0x80) {
		t.Errorf("gtk alpha/mix: %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	warns := capture(t)
	Parse(`x { color: color-mix(in cmyk, red, blue); } y { color: #12345; } z { color: rgb(1, 2); }`)
	if len(*warns) != 3 {
		t.Errorf("invalid colors: %d warnings, want 3", len(*warns))
	}
}

func TestUnitsAndCalc(t *testing.T) {
	s := parseOne(t, `
		a { padding: 1rem; margin: 12pt; min-width: calc(2rem + 4px); min-height: calc((1px + 2px) * 3); }
		b { font-size: 20px; padding: 0.5em; margin-top: calc(-1 * 3px); }
		c { font-size: 1.5em; }
		d { opacity: calc(0.25 * 2); letter-spacing: 0.1em; }
		e { min-width: max(3px, 1rem); min-height: clamp(1px, 50px, 20px); }
	`)
	v := computeTree(s, el("a"))
	if v.Padding.Top != 16 || v.Margin.Top != 16 || v.Layout.MinWidth.Px() != 36 || v.Layout.MinHeight.Px() != 9 {
		t.Errorf("rem/pt/calc: %v %v %d %d", v.Padding, v.Margin, v.Layout.MinWidth.Px(), v.Layout.MinHeight.Px())
	}
	v = computeTree(s, el("b"))
	if v.Padding.Top != 10 || v.Margin.Top != -3 {
		t.Errorf("em/negative calc: %v %v", v.Padding, v.Margin)
	}
	parent := el("b")
	child := parent.add("c")
	if v := computeTree(s, child); v.FontSize != 30 {
		t.Errorf("font-size em against the parent: %v", v.FontSize)
	}
	if v := computeTree(s, el("d")); v.Opacity != 0.5 || v.LetterSpacing != 1.6 {
		t.Errorf("calc number / em spacing: %v %v", v.Opacity, v.LetterSpacing)
	}
	if v := computeTree(s, el("e")); v.Layout.MinWidth.Px() != 16 || v.Layout.MinHeight.Px() != 20 {
		t.Errorf("max/clamp: %d %d", v.Layout.MinWidth.Px(), v.Layout.MinHeight.Px())
	}
	warns := capture(t)
	zs := Parse(`x { padding: calc(1px * 2px); } y { padding: calc(1px + 2); } z { padding: calc(1px / 0); }`)
	if len(*warns) != 2 {
		t.Errorf("type errors: %d warnings, want 2: %v", len(*warns), *warns)
	}
	// Values 4: a division by zero is an infinity, not a type error,
	// and a top-level infinity clamps to the largest value.
	if v := computeTree(zs, el("z")); v.Padding.Top != calcLimit {
		t.Errorf("calc(1px / 0) padding %d, want the clamped %d", v.Padding.Top, calcLimit)
	}
}

func TestCustomPropertiesAndVar(t *testing.T) {
	s := parseOne(t, `
		:root { --fg: #102030; --gap: 4px; --double: calc(var(--gap) * 2); --alias: var(--fg); }
		.card { --gap: 10px; }
		label { color: var(--alias); padding: var(--double); margin: var(--missing, var(--gap)); }
		.broken { color: var(--nope); background-color: var(--nope); }
		.cycle { --a: var(--b); --b: var(--a); padding: var(--a, 3px); }
		.empty { --e: ; margin: var(--e) 2px; }
		.reset { --fg: initial; color: var(--fg, #ffffff); }
	`)
	root := el("window")
	lbl := root.add("label")
	v := computeTree(s, lbl)
	if v.Color != render.RGB(0x10, 0x20, 0x30) {
		t.Errorf("var chain: %08x", uint32(v.Color))
	}
	if v.Padding.Top != 8 || v.Margin.Top != 4 {
		t.Errorf("var in calc / fallback: %v %v", v.Padding, v.Margin)
	}
	// --double resolved on :root against :root's --gap: the value is
	// computed where declared, and a descendant's --gap does not redo it.
	card := el("window")
	inCard := card.add("box", "card").add("label")
	if v := computeTree(s, inCard); v.Padding.Top != 8 || v.Margin.Top != 10 {
		t.Errorf("inherited computed var: padding %v margin %v", v.Padding, v.Margin)
	}

	// An unresolvable var() is invalid at computed-value time: color
	// (inherited) inherits, background-color (not inherited) is initial.
	parent := el("window", "tinted")
	parent.target.Inline = ParseDeclarations("color: #0a0b0c; background-color: #0d0e0f")
	parent.target.InlinePriority = PriorityUser
	broken := parent.add("label", "broken")
	v = computeTree(s, broken)
	if v.Color != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("invalid var color should inherit: %08x", uint32(v.Color))
	}
	if !v.Has(PropBackgroundColor) || v.Background != 0 {
		t.Errorf("invalid var background should be initial: %08x", uint32(v.Background))
	}

	cyc := el("window").add("label", "cycle")
	if v := computeTree(s, cyc); v.Padding.Top != 3 {
		t.Errorf("cycle falls back: %v", v.Padding)
	}
	if cv := computeTree(s, cyc); func() bool { _, ok := cv.Var("--a"); return ok }() {
		t.Error("a cyclic custom property is defined")
	}
	if v := computeTree(s, el("window").add("label", "empty")); v.Margin != (Sides{2, 2, 2, 2}) {
		t.Errorf("empty custom property: %v", v.Margin)
	}
	if v := computeTree(s, el("window").add("label", "reset")); v.Color != render.RGB(0xff, 0xff, 0xff) {
		t.Errorf("--x: initial: %08x", uint32(v.Color))
	}
}

func TestVarsPointerReuse(t *testing.T) {
	s := parseOne(t, ":root { --a: 1px; } .own { --b: 2px; }")
	var sc Scratch
	root := el("window")
	kid := root.add("box")
	own := root.add("box", "own")
	var rv, kv, ov, ov2 Values
	Compute([]Layer{{Sheet: s}}, root, nil, nil, Env{}, &sc, &rv)
	Compute([]Layer{{Sheet: s}}, kid, &rv, nil, Env{}, &sc, &kv)
	if kv.Vars != rv.Vars {
		t.Error("a node declaring no customs must share its parent's environment")
	}
	Compute([]Layer{{Sheet: s}}, own, &rv, nil, Env{}, &sc, &ov)
	Compute([]Layer{{Sheet: s}}, own, &rv, &ov, Env{}, &sc, &ov2)
	if ov2.Vars != ov.Vars || ov2.Background != ov.Background || ov2.Padding != ov.Padding || ov2.Set != ov.Set {
		t.Error("an unchanged environment must keep its pointer across recomputes")
	}
}

func TestAllUnsetAndWideKeywords(t *testing.T) {
	s := parseOne(t, `
		* { all: unset; }
		.p { color: #111111; background-color: #222222; padding: 5px; }
		.inh { background-color: inherit; padding: inherit; }
		.ini { color: initial; }
	`)
	v := computeTree(s, el("button"))
	if !v.Has(PropBackgroundColor) || v.Background != 0 || !v.Has(PropPaddingTop) || v.Opacity != 1 {
		t.Errorf("all: unset must set non-inherited initials: %+v", v)
	}
	if v.Has(PropColor) || v.Has(PropFontSize) {
		t.Error("all: unset set an inherited property at the root")
	}
	if !v.Has(PropBorderTopStyle) || v.EffBorder() != (Sides{}) {
		t.Error("all: unset left border styles unset")
	}
	p := el("box", "p")
	c := p.add("label", "inh")
	v = computeTree(s, c)
	if v.Color != render.RGB(0x11, 0x11, 0x11) || v.Background != render.RGB(0x22, 0x22, 0x22) || v.Padding.Top != 5 {
		t.Errorf("inherit: %08x %08x %v", uint32(v.Color), uint32(v.Background), v.Padding)
	}
	if v := computeTree(s, p.add("label", "ini")); v.Has(PropColor) {
		t.Error("color: initial should fall to the theme (unset)")
	}
}

func TestInheritanceSet(t *testing.T) {
	s := parseOne(t, `
		box { color: #010203; background-color: #090909; font-size: 20px; font-weight: bold;
		      letter-spacing: 1px; text-transform: uppercase; -gtk-icon-size: 24px; padding: 3px; font-family: "Cantarell", sans-serif; font-style: italic; }
		label.own { color: #070707; }
	`)
	b := el("box")
	v := computeTree(s, b.add("label"))
	if v.Color != render.RGB(1, 2, 3) || v.FontSize != 20 || v.FontWeight != 700 || v.LetterSpacing != 1 ||
		v.TextTransform != TransformUppercase || v.IconSize != 24 || v.FontFamily != "Cantarell" || !v.Italic {
		t.Errorf("inherited set: %+v", v)
	}
	if v.Has(PropBackgroundColor) || v.Has(PropPaddingTop) {
		t.Error("non-inherited properties inherited")
	}
	if v.Declares(PropColor) || !v.Own.Empty() {
		t.Errorf("inherited values count as declared: own %b", v.Own)
	}
	if v := computeTree(s, b.add("label", "own")); v.Color != render.RGB(7, 7, 7) || !v.Declares(PropColor) || v.Declares(PropFontSize) {
		t.Errorf("a direct declaration lost to inheritance: %08x (own %b)", uint32(v.Color), v.Own)
	}
	if v := computeTree(s, b); !v.Declares(PropColor) || !v.Declares(PropPaddingTop) {
		t.Errorf("the box's own declarations: own %b", v.Own)
	}
}

// transitionFor reports whether a change of p transitions for dur
// seconds under v.
func transitionFor(v Values, p Prop, dur float64) bool {
	tt, ok := v.Transition.For(p)
	return ok && tt.Duration == dur
}

func TestBoxShadowGradientFilterTransition(t *testing.T) {
	s := parseOne(t, `
		a { box-shadow: inset 0 -2px 0 0 #ff0000, 0 4px 8px rgba(0,0,0,0.5); }
		b { box-shadow: none; }
		c { background: linear-gradient(to right, #000000, #ffffff 25%, #ff0000); }
		d { background-image: linear-gradient(45deg, #000000 10%, #ffffff); background-color: #123456; }
		e { background: #abcdef; }
		f { filter: brightness(1.15) brightness(2); opacity: 50%; }
		g { transition: background-color 150ms ease-in-out, color 0.3s linear 0.1s; }
		h { transition: all 200ms; }
		i { transition-property: opacity; transition-duration: 1s; }
	`)
	v := computeTree(s, el("a"))
	sh := v.Shadow.List()
	if len(sh) != 2 || !sh[0].Inset || sh[0].Y != -2 || sh[0].Color != render.RGB(0xff, 0, 0) ||
		sh[1].Inset || sh[1].Y != 4 || sh[1].Blur != 8 || sh[1].Color.A() != 128 {
		t.Errorf("shadows: %+v", sh)
	}
	if v := computeTree(s, el("b")); !v.Has(PropBoxShadow) || v.Shadow.N != 0 {
		t.Error("box-shadow: none")
	}
	v = computeTree(s, el("c"))
	var buf [MaxResolvedStops]render.GradientStop
	if g := bgGradient(v); g.N != 3 || g.Angle != 90 || g.Resolve(100, buf[:0])[1].Pos != 0.25 || g.Resolve(100, buf[:0])[2].Pos != 1 {
		t.Errorf("gradient to right: %+v", v.Bg)
	}
	if !v.Has(PropBackgroundColor) || v.Background != 0 {
		t.Error("background shorthand must reset the color")
	}
	v = computeTree(s, el("d"))
	if g := bgGradient(v); g.Angle != 45 || g.Stops[0].Pos != 0.1 || v.Background != render.RGB(0x12, 0x34, 0x56) {
		t.Errorf("gradient angle: %+v", v.Bg)
	}
	if v := computeTree(s, el("e")); v.Background != render.RGB(0xab, 0xcd, 0xef) || v.Bg.Painted() {
		t.Errorf("background color: %+v", v)
	}
	if v := computeTree(s, el("f")); v.Brightness != 2.3 || v.Opacity != 0.5 {
		t.Errorf("filter/opacity: %v %v", v.Brightness, v.Opacity)
	}
	v = computeTree(s, el("g"))
	bg, okBg := v.Transition.For(PropBackgroundColor)
	fg, okFg := v.Transition.For(PropColor)
	if _, okOp := v.Transition.For(PropOpacity); !okBg || !okFg || okOp ||
		bg.Duration != 0.15 || bg.Timing != TimingEaseInOut || fg.Duration != 0.3 || fg.Delay != 0.1 || fg.Timing != TimingLinear {
		t.Errorf("transition list: %+v", v.Transition)
	}
	if v := computeTree(s, el("h")); !transitionFor(v, PropOpacity, 0.2) {
		t.Errorf("transition all: %+v", v.Transition)
	}
	if v := computeTree(s, el("i")); !transitionFor(v, PropOpacity, 1) || transitionFor(v, PropColor, 1) {
		t.Errorf("transition longhands: %+v", v.Transition)
	}
}

func TestFonts(t *testing.T) {
	s := parseOne(t, `
		a { font: italic bold 12px "Fira Sans", sans-serif; }
		b { font-family: Noto Sans, serif; font-weight: 600; }
		c { font-weight: bolder; }
		d { font-size: larger; }
	`)
	if v := computeTree(s, el("a")); !v.Italic || v.FontWeight != 700 || v.FontSize != 12 || v.FontFamily != "Fira Sans" {
		t.Errorf("font shorthand: %+v", v)
	}
	if v := computeTree(s, el("b")); v.FontFamily != "Noto Sans" || v.FontWeight != 600 {
		t.Errorf("unquoted family: %q %d", v.FontFamily, v.FontWeight)
	}
	par := el("b")
	if v := computeTree(s, par.add("c")); v.FontWeight != 900 {
		t.Errorf("bolder: %d", v.FontWeight)
	}
	if v := computeTree(s, el("d")); v.FontSize != 16*1.2 {
		t.Errorf("larger: %v", v.FontSize)
	}
}

func TestWarnAndSkipPolicy(t *testing.T) {
	warns := capture(t)
	s := Parse(`
		button { color: #ff0000; bogus-prop: 12px; background-color: #00ff00; }
		button:visited { color: #123123; }
		label::first-line { color: #456456; }
		button { color: #0000ff !important; }
		.nested { a { color: #111111; } }
		@import "x.css";
		@keyframes spin { from { opacity: 0; } to { opacity: 1; } }
		@media screen { button { color: #222222; } }
		button { animation: spin 1s; -gtk-icon-transform: rotate(1deg); }
		entry { padding: 4px; color: #abcdef; }
		button:link { color: #333333; }
		button { background-color: #445566; `)
	want := []string{"unknown property", "bad selector", "@import", "bad selector", "unterminated"}
	if len(*warns) != len(want) {
		t.Fatalf("got %d warnings, want %d: %v", len(*warns), len(want), *warns)
	}
	for i, w := range want {
		if !strings.Contains((*warns)[i], w) {
			t.Errorf("warning %d = %q, want it to mention %q", i, (*warns)[i], w)
		}
	}
	v := computeTree(s, el("button"))
	if v.Color != render.RGB(0x00, 0x00, 0xff) || v.Background != render.RGB(0, 0xff, 0) {
		t.Errorf("!important beats the later normal rules: %08x %08x", uint32(v.Color), uint32(v.Background))
	}
	if v := computeTree(s, el("entry")); v.Color != render.RGB(0xab, 0xcd, 0xef) {
		t.Errorf("rule after the skipped ones: %08x", uint32(v.Color))
	}
	s = parseOne(t, "nosuchwidget { color: #010101; } #nosuchid { color: #020202; }")
	if v := computeTree(s, el("button")); v.Has(PropColor) {
		t.Error("unknown element name matched")
	}
}

func TestSensitivity(t *testing.T) {
	s := parseOne(t, `
		menubutton:hover > button .icon { color: #000001; }
		.bar.top .section:not(.ghost) label { color: #000002; }
		button:active { color: #000003; }
	`)
	sens := s.Sensitivity()
	if sens.AncestorStates != Hover {
		t.Errorf("ancestor states = %b, want hover only (active is rightmost)", sens.AncestorStates)
	}
	for _, c := range []string{"bar", "top", "section", "ghost"} {
		if !sens.AncestorClasses[c] {
			t.Errorf("ancestor class %q missing", c)
		}
	}
	if sens.AncestorClasses["icon"] || sens.Structural || sens.SiblingFacts {
		t.Error("rightmost class or siblings recorded")
	}
	for _, tc := range []struct {
		css               string
		structural, facts bool
	}{
		{"a + b { color: red; }", true, true},
		{"a ~ b { color: red; }", true, true},
		{"a:first-child { color: red; }", true, false},
		{"a:last-child { color: red; }", true, false},
		{"a:nth-child(2n) { color: red; }", true, false},
		{"a:nth-child(2n of .x) { color: red; }", true, true},
	} {
		got := parseOne(t, tc.css).Sensitivity()
		if got.Structural != tc.structural || got.SiblingFacts != tc.facts {
			t.Errorf("%s: structural %v facts %v, want %v %v", tc.css, got.Structural, got.SiblingFacts, tc.structural, tc.facts)
		}
	}
}

func TestParseEmpty(t *testing.T) {
	s := Parse("")
	if s == nil || len(s.rules) != 0 {
		t.Fatal("empty stylesheet must parse to an empty sheet")
	}
	if v := computeTree(s, el("button")); !v.Set.Empty() {
		t.Error("empty sheet set properties")
	}
}

func TestComputeVarFreeAllocatesNothing(t *testing.T) {
	s := parseOne(t, `
		button { color: #111111; background-color: #222222; padding: 8px; }
		button:hover { background-color: #333333; }
		box > button { border-radius: 4px; }
		* { font-weight: 400; }
	`)
	root := el("box")
	b := root.add("button")
	layers := []Layer{{Sheet: s, Priority: PriorityApplication}}
	var sc Scratch
	var pv, v Values
	Compute(layers, root, nil, nil, Env{}, &sc, &pv)
	Compute(layers, b, &pv, nil, Env{}, &sc, &v)
	alloc := testing.AllocsPerRun(100, func() {
		Compute(layers, b, &pv, &v, Env{}, &sc, &v)
	})
	if alloc != 0 {
		t.Errorf("Compute allocated %v ops for a var-free sheet, want 0", alloc)
	}
}

func ExampleParse() {
	s := Parse(`:root { --danger: #a00; } button.destructive { background-color: var(--danger); }`)
	var sc Scratch
	var root, v Values
	layers := []Layer{{Sheet: s, Priority: PriorityApplication}}
	win := el("window")
	btn := win.add("button", "destructive")
	Compute(layers, win, nil, nil, Env{}, &sc, &root)
	Compute(layers, btn, &root, nil, Env{}, &sc, &v)
	fmt.Println(v.Has(PropBackgroundColor), v.Background == render.RGB(0xaa, 0, 0))
	// Output: true true
}

// line-height resolves lengths, unitless numbers, and percentages
// against the computed font-size, inherits, and `normal` clears.
func TestLineHeight(t *testing.T) {
	s := parseOne(t, `
		label { font-size: 20px; line-height: 2; }
		label.wrap { line-height: 150%; }
		label.px { line-height: 45px; }
		label.none { line-height: normal; }
		span { font-size: 12px; }
	`)
	if got := computeTree(s, el("label")).LineHeight; got != 40 {
		t.Errorf("line-height: 2 at 20px = %v, want 40", got)
	}
	if got := computeTree(s, el("label", "wrap")).LineHeight; got != 30 {
		t.Errorf("line-height: 150%% at 20px = %v, want 30", got)
	}
	if got := computeTree(s, el("label", "px")).LineHeight; got != 45 {
		t.Errorf("line-height: 45px = %v, want 45", got)
	}
	if got := computeTree(s, el("label", "none")).LineHeight; got != 0 {
		t.Errorf("line-height: normal = %v, want 0 (the font's own box)", got)
	}
	// Inherited: a unitless number inherits as the number, so the span
	// resolves it against its own 12px (it took the label's 40px); a
	// percentage inherits as the label's computed length. An explicit
	// normal on the parent clears it.
	root := el("label")
	kid := root.add("span")
	if got := computeTree(s, kid).LineHeight; got != 24 {
		t.Errorf("inherited line-height: 2 at 12px = %v, want 24", got)
	}
	pct := el("label", "wrap")
	if got := computeTree(s, pct.add("span")).LineHeight; got != 30 {
		t.Errorf("inherited line-height: 150%% = %v, want the parent's 30", got)
	}
	normal := parseOne(t, "label { line-height: normal; }")
	if got := computeTree(normal, kid).LineHeight; got != 0 {
		t.Errorf("inherited normal line-height = %v, want 0", got)
	}
}

// font-feature-settings parses the tag list into the canonical sorted
// form, defaulting values to 1, and `normal` clears it.
func TestFontFeatureSettings(t *testing.T) {
	s := parseOne(t, `
		label { font-feature-settings: "tnum"; }
		label.zero { font-feature-settings: "kern" 0, tnum off; }
		label.none { font-feature-settings: normal; }
	`)
	if got := computeTree(s, el("label")).Features; got != "tnum=1" {
		t.Errorf("quoted tag with default value = %q, want tnum=1", got)
	}
	if got := computeTree(s, el("label", "zero")).Features; got != "kern=0;tnum=0" {
		t.Errorf("multiple tags sort canonically: %q, want kern=0;tnum=0", got)
	}
	if got := computeTree(s, el("label", "none")).Features; got != "" {
		t.Errorf("normal = %q, want empty", got)
	}
	// Inherited into a child that declares nothing.
	root := el("label")
	if got := computeTree(s, root.add("span")).Features; got != "tnum=1" {
		t.Errorf("inherited features = %q, want tnum=1", got)
	}
}

// transform parses the full 2-D set the stylesheet may use; junk
// fails and warns, and `none` is the identity.
func TestTransform(t *testing.T) {
	s := parseOne(t, `
		label { transform: scale(1.5); }
		label.xy { transform: scale(2, 0.5); }
		label.tx { transform: translateX(var(--slide, 12px)); }
		label.none { transform: none; }
		label.rot { transform: rotate(45deg); }
		label.spin { transform: translate(4px, 6px) rotate(90deg) scale(2); }
		label.mx { transform: matrix(1, 0, 0.5, 1, 10, 20); }
		label.sk { transform: skew(45deg, 0deg); }
	`)
	if v := computeTree(s, el("label")); !affineNear(v.Transform, Xform{M: render.Scale(1.5, 1.5)}, 1e-9) {
		t.Errorf("scale(1.5) = %v", v.Transform)
	}
	if v := computeTree(s, el("label", "xy")); !affineNear(v.Transform, Xform{M: render.Scale(2, 0.5)}, 1e-9) {
		t.Errorf("scale(2, 0.5) = %v", v.Transform)
	}
	if v := computeTree(s, el("label", "tx")); !affineNear(v.Transform, Xform{M: render.Translate(12, 0)}, 1e-9) {
		t.Errorf("translateX(var(--slide, 12px)) = %v, want 12", v.Transform)
	}
	if v := computeTree(s, el("label", "none")); v.Transform.M != render.Identity {
		t.Errorf("none = %v, want the identity", v.Transform)
	}
	if v := computeTree(s, el("label", "rot")); !affineNear(v.Transform, Xform{M: render.Rotate(45)}, 1e-9) {
		t.Errorf("rotate(45deg) = %v", v.Transform)
	}
	// Left-to-right composition: (4,6) shifted, turned, doubled.
	m := computeTree(s, el("label", "spin")).Transform
	px, py := m.M.Apply(1, 0)
	if math.Abs(px-4) > 1e-9 || math.Abs(py-8) > 1e-9 {
		t.Errorf("composed transform maps (1,0) to (%v,%v), want (4,8)", px, py)
	}
	if v := computeTree(s, el("label", "mx")); !affineNear(v.Transform, Xform{M: render.Affine{A: 1, C: 0.5, D: 1, E: 10, F: 20}}, 1e-9) {
		t.Errorf("matrix = %v", v.Transform)
	}
	v := computeTree(s, el("label", "sk"))
	px, py = v.Transform.M.Apply(0, 1)
	if math.Abs(px-1) > 1e-9 || math.Abs(py-1) > 1e-9 {
		t.Errorf("skew(45deg, 0deg) maps (0,1) to (%v,%v), want (1,1)", px, py)
	}
	warns := capture(t)
	Parse("label { transform: nonsense(45deg); }")
	if len(*warns) == 0 {
		t.Error("an unknown transform function parsed without a warning")
	}

	// transform-origin: keywords, lengths, percentages, one value
	// centering the other axis.
	s = parseOne(t, `
		label { transform-origin: left top; }
		label.px { transform-origin: 4px 20%; }
		label.one { transform-origin: 30%; }
	`)
	if v := computeTree(s, el("label")); v.OriginFrac != [2]float64{0, 0} {
		t.Errorf("left top = %v", v.OriginFrac)
	}
	if v := computeTree(s, el("label", "px")); v.OriginPx[0] != 4 || v.OriginFrac[1] != 0.2 {
		t.Errorf("4px 20%% = %v/%v", v.OriginPx, v.OriginFrac)
	}
	if v := computeTree(s, el("label", "one")); v.OriginFrac != [2]float64{0.3, 0.5} {
		t.Errorf("one value = %v, want 0.3 and the centered 0.5", v.OriginFrac)
	}
}

// all: unset computes every non-inherited longhand to its initial —
// and a transform initial of 0 would paint nothing. The scale's
// initial is the identity, like opacity's is 1.
func TestAllUnsetLeavesTheTransformIdentity(t *testing.T) {
	s := parseOne(t, `* { all: unset; } label { color: red; }`)
	v := computeTree(s, el("label"))
	if !v.Has(PropTransform) {
		t.Fatal("all: unset did not compute the transform longhands")
	}
	if v.Transform.M != render.Identity {
		t.Errorf("all: unset transform = %v, want the identity", v.Transform)
	}
	if v.Opacity != 1 {
		t.Errorf("all: unset opacity = %v, want 1", v.Opacity)
	}
}

// A CSS-wide keyword leaves text-overflow and line-clamp undeclared, as
// it does the layout longhands: their initial value is the label's own
// ellipsize and line limit, so `* { all: unset }` keeps them. An
// explicit value still declares them.
func TestWideKeywordsLeaveWidgetTextSettingsUndeclared(t *testing.T) {
	s := parseOne(t, `
		* { all: unset; }
		a { text-overflow: initial; line-clamp: unset; }
		b { text-overflow: clip; line-clamp: 2; }
	`)
	for _, name := range []string{"label", "a"} {
		v := computeTree(s, el(name))
		if v.Has(PropTextOverflow) || v.Has(PropLineClamp) {
			t.Errorf("%s: a wide keyword declared text-overflow %v / line-clamp %v", name, v.Has(PropTextOverflow), v.Has(PropLineClamp))
		}
	}
	if v := computeTree(s, el("b")); !v.Has(PropTextOverflow) || v.Overflow != TextOverflowClip || !v.Has(PropLineClamp) || v.LineClamp != 2 {
		t.Errorf("explicit values: overflow set=%v %v, clamp set=%v %d", v.Has(PropTextOverflow), v.Overflow, v.Has(PropLineClamp), v.LineClamp)
	}
}
