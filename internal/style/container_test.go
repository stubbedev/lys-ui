package style

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// computeContainer computes n's chain like computeTree, with the
// subject's stand-in query container.
func computeContainer(s *Sheet, n *node, c Container) Values {
	n.cont, n.hasCont = c, true
	return computeTree(s, n)
}

func TestContainerQuerySizeConditions(t *testing.T) {
	for _, tc := range []struct {
		css  string
		c    Container
		want bool
	}{
		{`@container (min-width: 300px) { a { color: red; } }`, Container{W: 400}, true},
		{`@container (min-width: 300px) { a { color: red; } }`, Container{W: 200}, false},
		{`@container (max-width: 300px) { a { color: red; } }`, Container{W: 200}, true},
		{`@container (width: 400px) { a { color: red; } }`, Container{W: 400}, true},
		{`@container (min-inline-size: 300px) { a { color: red; } }`, Container{W: 400}, true},
		{`@container (min-height: 300px) { a { color: red; } }`, Container{W: 400, H: 500, Size: true}, true},
		{`@container (min-height: 300px) { a { color: red; } }`, Container{W: 400, H: 500}, false},
		{`@container (orientation: landscape) { a { color: red; } }`, Container{W: 400, H: 200, Size: true}, true},
		{`@container (orientation: landscape) { a { color: red; } }`, Container{W: 200, H: 400, Size: true}, false},
		{`@container (400px <= width <= 800px) { a { color: red; } }`, Container{W: 500}, true},
		{`@container (401px <= width <= 800px) { a { color: red; } }`, Container{W: 400}, false},
		{`@container not (min-width: 300px) { a { color: red; } }`, Container{W: 200}, true},
		{`@container (min-width: 100px) and (max-width: 300px) { a { color: red; } }`, Container{W: 200}, true},
		{`@container (min-width: 100px), (min-width: 900px) { a { color: red; } }`, Container{W: 200}, true},
	} {
		v := computeContainer(parseOne(t, tc.css), el("a"), tc.c)
		if v.Has(PropColor) != tc.want {
			t.Errorf("%s at %v applied = %v, want %v", tc.css, tc.c, v.Has(PropColor), tc.want)
		}
	}
}

func TestContainerQueryNoContainer(t *testing.T) {
	s := parseOne(t, `@container (min-width: 300px) { a { color: red; } }`)
	n := el("a")
	n.hasCont = false
	if v := computeTree(s, n); v.Has(PropColor) {
		t.Error("a query without a query container must not apply")
	}
}

func TestContainerQueryNameFilter(t *testing.T) {
	s := parseOne(t, `
		@container (min-width: 300px) { a { color: #020202; } }
		@container card (min-width: 300px) { a { color: #010101; } }
	`)
	v := computeContainer(s, el("a"), Container{W: 400, Names: "sidebar card"})
	if v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("named query matched against the name list: %08x", uint32(v.Color))
	}
	v = computeContainer(s, el("a"), Container{W: 400, Names: "sidebar"})
	if v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("a name filter that misses falls to the unnamed query: %08x", uint32(v.Color))
	}
}

func TestContainerQueryUnits(t *testing.T) {
	s := parseOne(t, `
		a {
			padding-top: 10cqw;
			padding-right: 10cqh;
			padding-bottom: 10cqi;
			padding-left: 10cqb;
		}
		b { margin: 10cqmin 10cqmax; }
	`)
	c := Container{W: 200, H: 100, Size: true}
	v := computeContainer(s, el("a"), c)
	if v.Padding != (Sides{Top: 20, Right: 10, Bottom: 20, Left: 10}) {
		t.Errorf("container units: %v", v.Padding)
	}
	v = computeContainer(s, el("b"), c)
	if v.Margin != (Sides{Top: 10, Right: 20, Bottom: 10, Left: 20}) {
		t.Errorf("cqmin/cqmax: %v", v.Margin)
	}
	if v := computeTree(s, el("b")); v.Margin != (Sides{}) {
		t.Errorf("no container resolves to zero: %v", v.Margin)
	}
}

func TestContainerTypeAndNameProperties(t *testing.T) {
	s := parseOne(t, `
		.box { container-type: inline-size; container-name: card main; }
		.sized { container-type: size; }
	`)
	if v := computeTree(s, el("div", "box")); v.ContainerType != ContainerInlineSize || v.ContainerNames != "card main" {
		t.Errorf("container-type/name: %v %q", v.ContainerType, v.ContainerNames)
	}
	if v := computeTree(s, el("div", "sized")); v.ContainerType != ContainerSize {
		t.Errorf("container-type: size: %v", v.ContainerType)
	}
	warns := capture(t)
	bad := Parse(`.bad { container-type: nonsense; }`)
	if len(*warns) != 1 {
		t.Fatalf("a bad container-type warns: %v", *warns)
	}
	if v := computeTree(bad, el("div", "bad")); v.Has(PropContainerType) {
		t.Error("a bad container-type drops the declaration")
	}
}

func TestContainerNestingIntersects(t *testing.T) {
	s := parseOne(t, `
		@container (min-width: 300px) {
			@container (orientation: landscape) { a { color: #010101; } }
		}
	`)
	if v := computeContainer(s, el("a"), Container{W: 400, H: 200, Size: true}); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("nested @container: %08x", uint32(v.Color))
	}
	if v := computeContainer(s, el("a"), Container{W: 400, H: 900, Size: true}); v.Has(PropColor) {
		t.Error("the inner condition must gate")
	}
	if v := computeContainer(s, el("a"), Container{W: 100, H: 200, Size: true}); v.Has(PropColor) {
		t.Error("the outer condition must gate")
	}
}
