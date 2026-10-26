package style

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func computeMedia(s *Sheet, n *node, m Media) Values {
	var chain []*node
	for p := n; p != nil; p = p.parent {
		chain = append([]*node{p}, chain...)
	}
	var sc Scratch
	var parent *Values
	var v Values
	for _, c := range chain {
		v = Values{}
		Compute([]Layer{{Sheet: s, Priority: PriorityApplication}}, c, parent, nil, Env{Rem: 16, Media: m}, &sc, &v)
		pv := v
		parent = &pv
	}
	return v
}

func mediaColor(t *testing.T, css string, m Media) (render.Color, bool) {
	t.Helper()
	v := computeMedia(parseOne(t, css), el("a"), m)
	return v.Color, v.Has(PropColor)
}

func TestMediaSizeConditions(t *testing.T) {
	m := Media{Width: 600, Height: 400, DPI: 96}
	for _, tc := range []struct {
		css  string
		want bool
	}{
		{`@media (min-width: 500px) { a { color: red; } }`, true},
		{`@media (min-width: 700px) { a { color: red; } }`, false},
		{`@media (max-width: 600px) { a { color: red; } }`, true},
		{`@media (max-width: 599px) { a { color: red; } }`, false},
		{`@media (width: 600px) { a { color: red; } }`, true},
		{`@media (width: 601px) { a { color: red; } }`, false},
		{`@media (min-height: 400px) { a { color: red; } }`, true},
		{`@media (max-height: 399px) { a { color: red; } }`, false},
		{`@media screen { a { color: red; } }`, true},
		{`@media print { a { color: red; } }`, false},
		{`@media only screen { a { color: red; } }`, true},
		{`@media (min-width: 500px) and (max-width: 700px) { a { color: red; } }`, true},
		{`@media (min-width: 700px) and (max-width: 900px) { a { color: red; } }`, false},
		{`@media (min-width: 700px), (max-width: 700px) { a { color: red; } }`, true},
		{`@media not (min-width: 700px) { a { color: red; } }`, true},
		{`@media not (min-width: 500px) { a { color: red; } }`, false},
		{`@media (bogus-feature: 3) { a { color: red; } }`, false},
		// An unknown feature is unknown, and not keeps it unknown,
		// which a query reads as false (mediaqueries-4 §3).
		{`@media not (bogus-feature: 3) { a { color: red; } }`, false},
		{`@media not print { a { color: red; } }`, true},
		{`@media ((min-width: 700px) or (max-width: 600px)) { a { color: red; } }`, true},
		{`@media (min-width: 700px) or (max-width: 600px) { a { color: red; } }`, true},
		{`@media not ((min-width: 700px) or (max-width: 599px)) { a { color: red; } }`, true},
		{`@media (bogus-feature: 3) or (width: 600px) { a { color: red; } }`, true},
	} {
		if _, ok := mediaColor(t, tc.css, m); ok != tc.want {
			t.Errorf("%s applied = %v, want %v", tc.css, ok, tc.want)
		}
	}
}

func TestMediaRangeSyntax(t *testing.T) {
	m := Media{Width: 600, Height: 400}
	for _, tc := range []struct {
		css  string
		want bool
	}{
		{`@media (400px <= width <= 800px) { a { color: red; } }`, true},
		{`@media (601px <= width <= 800px) { a { color: red; } }`, false},
		{`@media (width >= 500px) { a { color: red; } }`, true},
		{`@media (width > 600px) { a { color: red; } }`, false},
		{`@media (700px >= width) { a { color: red; } }`, true},
		{`@media (500px > width) { a { color: red; } }`, false},
		{`@media (width = 600px) { a { color: red; } }`, true},
	} {
		if _, ok := mediaColor(t, tc.css, m); ok != tc.want {
			t.Errorf("%s applied = %v, want %v", tc.css, ok, tc.want)
		}
	}
}

func TestMediaRatioOrientationResolution(t *testing.T) {
	for _, tc := range []struct {
		css  string
		m    Media
		want bool
	}{
		{`@media (orientation: landscape) { a { color: red; } }`, Media{Width: 800, Height: 600}, true},
		{`@media (orientation: portrait) { a { color: red; } }`, Media{Width: 800, Height: 600}, false},
		{`@media (orientation: portrait) { a { color: red; } }`, Media{Width: 600, Height: 800}, true},
		{`@media (aspect-ratio: 4/3) { a { color: red; } }`, Media{Width: 800, Height: 600}, true},
		{`@media (min-aspect-ratio: 2/1) { a { color: red; } }`, Media{Width: 800, Height: 600}, false},
		{`@media (max-aspect-ratio: 2/1) { a { color: red; } }`, Media{Width: 800, Height: 600}, true},
		{`@media (min-resolution: 144dpi) { a { color: red; } }`, Media{DPI: 192}, true},
		{`@media (min-resolution: 144dpi) { a { color: red; } }`, Media{DPI: 96}, false},
		{`@media (resolution: 2x) { a { color: red; } }`, Media{DPI: 192}, true},
	} {
		if _, ok := mediaColor(t, tc.css, tc.m); ok != tc.want {
			t.Errorf("%s at %v applied = %v, want %v", tc.css, tc.m, ok, tc.want)
		}
	}
}

func TestMediaPreferences(t *testing.T) {
	for _, tc := range []struct {
		css  string
		m    Media
		want bool
	}{
		{`@media (prefers-color-scheme: dark) { a { color: red; } }`, Media{Dark: true}, true},
		{`@media (prefers-color-scheme: dark) { a { color: red; } }`, Media{}, false},
		{`@media (prefers-color-scheme: light) { a { color: red; } }`, Media{}, true},
		{`@media (prefers-contrast: more) { a { color: red; } }`, Media{HighContrast: true}, true},
		{`@media (prefers-contrast: more) { a { color: red; } }`, Media{}, false},
		{`@media (prefers-contrast: no-preference) { a { color: red; } }`, Media{}, true},
		{`@media (prefers-reduced-motion: reduce) { a { color: red; } }`, Media{ReducedMotion: true}, true},
		{`@media (prefers-reduced-motion: reduce) { a { color: red; } }`, Media{}, false},
		{`@media (prefers-reduced-motion: no-preference) { a { color: red; } }`, Media{}, true},
	} {
		if _, ok := mediaColor(t, tc.css, tc.m); ok != tc.want {
			t.Errorf("%s at %v applied = %v, want %v", tc.css, tc.m, ok, tc.want)
		}
	}
}

func TestMediaNestingIntersects(t *testing.T) {
	s := parseOne(t, `
		@media (min-width: 500px) {
			@media (orientation: landscape) { a { color: #010101; } }
			@media (orientation: portrait) { a { color: #020202; } }
		}
	`)
	v := computeMedia(s, el("a"), Media{Width: 600, Height: 400})
	if !v.Has(PropColor) || v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("landscape branch: %+v", v)
	}
	v = computeMedia(s, el("a"), Media{Width: 600, Height: 900})
	if !v.Has(PropColor) || v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("portrait branch: %+v", v)
	}
	v = computeMedia(s, el("a"), Media{Width: 100, Height: 400})
	if v.Has(PropColor) {
		t.Error("outer condition must gate the inner branches")
	}
}

func TestMediaInsideRule(t *testing.T) {
	s := parseOne(t, `
		a {
			color: #010101;
			@media (min-width: 500px) { color: #020202; }
		}
	`)
	v := computeMedia(s, el("a"), Media{Width: 600})
	if !v.Has(PropColor) || v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("media-scoped declaration: %+v", v)
	}
	v = computeMedia(s, el("a"), Media{})
	if !v.Has(PropColor) || v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("base declaration: %+v", v)
	}
}

func TestMediaBadQueryWarns(t *testing.T) {
	warns := capture(t)
	Parse("@media (min-width) { a { color: red; } } a { color: blue; }")
	if len(*warns) != 1 || !strings.Contains((*warns)[0], "@media") {
		t.Fatalf("warns = %v", *warns)
	}
}

func TestSupports(t *testing.T) {
	for _, tc := range []struct {
		css  string
		want bool
	}{
		{`@supports (color: red) { a { color: red; } }`, true},
		{`@supports (background-color: #fff) { a { color: red; } }`, true},
		{`@supports (background-color: nonsense) { a { color: red; } }`, false},
		{`@supports (display: run-in) { a { color: red; } }`, false},
		{`@supports (order: 1.5) { a { color: red; } }`, false},
		{`@supports not (display: run-in) { a { color: red; } }`, true},
		{`@supports (color: red) and (display: run-in) { a { color: red; } }`, false},
		{`@supports (color: red) and (opacity: 0.5) { a { color: red; } }`, true},
		{`@supports (display: run-in) or (color: red) { a { color: red; } }`, true},
		{`@supports ((display: run-in) or (color: red)) and (opacity: 0.5) { a { color: red; } }`, true},
		{`@supports selector(button:hover) { a { color: red; } }`, true},
		{`@supports selector(button::after) { a { color: red; } }`, true},
		{`@supports selector(button::first-line) { a { color: red; } }`, true},
		{`@supports selector(button::spelling-error) { a { color: red; } }`, false},
	} {
		if _, ok := mediaColor(t, tc.css, Media{}); ok != tc.want {
			t.Errorf("%s applied = %v, want %v", tc.css, ok, tc.want)
		}
	}
}

func TestImport(t *testing.T) {
	files := map[string]string{
		"/t/main.css":  "@import \"a.css\";\na { color: #010101; }",
		"/t/a.css":     "@import \"sub/b.css\";\na { color: #020202; }",
		"/t/sub/b.css": "a { color: #030303; }",
	}
	resolve := func(ref, base string) (string, string, error) {
		p := ref
		if !filepath.IsAbs(ref) {
			p = filepath.Join(filepath.Dir(base), ref)
		}
		c, ok := files[p]
		if !ok {
			return p, "", &osErr{p}
		}
		return p, c, nil
	}
	s, warns := ParseImporting(files["/t/main.css"], "/t/main.css", resolve)
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	v := computeMedia(s, el("a"), Media{})
	if v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("imported rules come first; the sheet's own win: %08x", uint32(v.Color))
	}
	s, _ = ParseImporting("@import url(a.css);", "/t/main.css", resolve)
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("url() import: %08x", uint32(v.Color))
	}
	s, _ = ParseImporting("@import \"a.css\" (min-width: 500px);", "/t/main.css", resolve)
	if v := computeMedia(s, el("a"), Media{Width: 600}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("conditional import applied: %08x", uint32(v.Color))
	}
	if v := computeMedia(s, el("a"), Media{Width: 100}); v.Has(PropColor) {
		t.Error("conditional import must not apply below its query")
	}
	_, warns = ParseImporting("@import \"missing.css\";", "/t/main.css", resolve)
	if len(warns) != 1 || !strings.Contains(warns[0], "/t/missing.css") {
		t.Errorf("missing import warns = %v", warns)
	}
}

type osErr struct{ path string }

func (e *osErr) Error() string { return "open " + e.path + ": no such file" }

func TestImportCycle(t *testing.T) {
	files := map[string]string{
		"/t/a.css": "@import \"b.css\";\na { color: #010101; }",
		"/t/b.css": "@import \"a.css\";\na { color: #020202; }",
	}
	resolve := func(ref, base string) (string, string, error) {
		p := filepath.Join(filepath.Dir(base), ref)
		c, ok := files[p]
		if !ok {
			return p, "", &osErr{p}
		}
		return p, c, nil
	}
	s, warns := ParseImporting(files["/t/a.css"], "/t/a.css", resolve)
	if len(warns) != 1 || !strings.Contains(warns[0], "@import cycle: /t/b.css -> /t/a.css -> /t/b.css") {
		t.Fatalf("cycle warning = %v", warns)
	}
	if v := computeMedia(s, el("a"), Media{}); !v.Has(PropColor) {
		t.Error("the valid remainder of a cyclic import must still apply")
	}
}

func TestLayerOrder(t *testing.T) {
	s := parseOne(t, `
		a { color: #010101; }
		@layer base { a { color: #020202; } }
		@layer theme { a { color: #030303; } }
	`)
	v := computeMedia(s, el("a"), Media{})
	if v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("unlayered must outrank layers: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer base { a { color: #010101; } }
		@layer theme { a { color: #020202; } }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("later layer wins: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer theme, base;
		@layer base { a { color: #010101; } }
		@layer theme { a { color: #020202; } }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("the statement's later layer wins: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer { a { color: #010101; } }
		@layer { a { color: #020202; } }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("second anonymous layer wins: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer base {
			a { color: #020202; }
			@media (min-width: 500px) { a { color: #010101; } }
		}
	`)
	if v := computeMedia(s, el("a"), Media{Width: 600}); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("media inside a layer: %08x", uint32(v.Color))
	}
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("media-off falls to the layer base: %08x", uint32(v.Color))
	}
}

func TestImportant(t *testing.T) {
	s := parseOne(t, `
		a { color: #010101 !important; }
		#a.x a { color: #020202; }
		a { color: #030303 !important; }
	`)
	v := computeMedia(s, el("a"), Media{})
	if v.Color != render.RGB(0x03, 0x03, 0x03) {
		t.Errorf("later important wins, specificity aside: %08x", uint32(v.Color))
	}
	low := parseOne(t, `a { color: #010101 !important; }`)
	high := parseOne(t, `#a.x a { color: #020202; }`)
	v = computeLayers([]Layer{{Sheet: low, Priority: PriorityTheme}, {Sheet: high, Priority: PriorityUser}}, el("a"))
	if v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("important at a lower priority beats normal above it: %08x", uint32(v.Color))
	}
	v = computeLayers([]Layer{
		{Sheet: parseOne(t, `a { color: #010101 !important; }`), Priority: PriorityTheme},
		{Sheet: parseOne(t, `a { color: #020203 !important; }`), Priority: PriorityTheme},
	}, el("a"))
	if v.Color != render.RGB(0x02, 0x02, 0x03) {
		t.Errorf("later important at the same priority wins: %08x", uint32(v.Color))
	}
}

func TestNesting(t *testing.T) {
	s := parseOne(t, `
		button {
			color: #010101;
			&:hover { color: #020202; }
			.icon { color: #030303; }
			& > .direct { color: #040404; }
		}
	`)
	root := el("button")
	if v := computeMedia(s, root, Media{}); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("own declaration: %08x", uint32(v.Color))
	}
	hover := el("button")
	hover.target.State = Hover
	if v := computeMedia(s, hover, Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("&:hover desugars to the parent: %08x", uint32(v.Color))
	}
	icon := root.add("image", "icon")
	if v := computeMedia(s, icon, Media{}); v.Color != render.RGB(0x03, 0x03, 0x03) {
		t.Errorf("bare nested selector is a descendant: %08x", uint32(v.Color))
	}
	direct := root.add("box", "direct")
	if v := computeMedia(s, direct, Media{}); v.Color != render.RGB(0x04, 0x04, 0x04) {
		t.Errorf("& with a combinator: %08x", uint32(v.Color))
	}
}

func TestNestingCommaCrossProduct(t *testing.T) {
	s := parseOne(t, `
		.a, .b {
			&:hover, &:focus { color: #010101; }
			& .c { color: #020202; }
		}
	`)
	for _, cl := range []string{"a", "b"} {
		n := el("button", cl)
		n.target.State = Focus
		if v := computeMedia(s, n, Media{}); v.Color != render.RGB(0x01, 0x01, 0x01) {
			t.Errorf(".%s:focus: %08x", cl, uint32(v.Color))
		}
		if v := computeMedia(s, n.add("image", "c"), Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
			t.Errorf(".%s .c: %08x", cl, uint32(v.Color))
		}
	}
}

func TestNestingNested(t *testing.T) {
	s := parseOne(t, `
		.a {
			.b {
				.c { color: #010101; }
				&:hover { color: #020202; }
			}
		}
	`)
	deep := el("box", "a").add("box", "b").add("label", "c")
	if v := computeMedia(s, deep, Media{}); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("two levels: %08x", uint32(v.Color))
	}
	h := el("box", "a").add("box", "b")
	h.target.State = Hover
	if v := computeMedia(s, h, Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("nested &:hover binds the inner parent: %08x", uint32(v.Color))
	}
}

func TestRevert(t *testing.T) {
	s := parseOne(t, `
		a { color: #010101; }
		b { color: #020202; }
		b a { color: revert; }
	`)
	root := el("b")
	child := root.add("a")
	if v := computeMedia(s, child, Media{}); !v.Has(PropColor) || v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("revert falls to the inherited value: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		a { color: #010101; }
		a { color: revert; }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Has(PropColor) {
		t.Error("revert with nothing beneath behaves as unset")
	}
}

func TestRevertLayer(t *testing.T) {
	s := parseOne(t, `
		@layer base { a { color: #010101; } }
		@layer theme { a { color: #020202; } }
		@layer top { a { color: revert-layer; } }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("revert-layer rolls to the next layer down: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer base { a { color: #010101; } }
		a { color: revert-layer; }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("unlayered revert-layer rolls into the layers: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		a { color: #010101; }
		a { color: revert-layer; }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Has(PropColor) {
		t.Error("revert-layer with nothing beneath behaves as unset")
	}
	s = parseOne(t, `
		@layer base { a { color: #010101; } }
		@layer theme { a { color: revert-layer; } }
		@layer top { a { opacity: 0.5; } }
	`)
	if v := computeMedia(s, el("a"), Media{}); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("revert-layer ignores an unrelated layer above: %08x", uint32(v.Color))
	}
}
