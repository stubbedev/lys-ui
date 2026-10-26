package style

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func importFiles(t *testing.T, files map[string]string, entry string) (*Sheet, []string) {
	t.Helper()
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
	return ParseImporting(files[entry], entry, resolve)
}

func TestImportLayerQualifier(t *testing.T) {
	files := map[string]string{
		"/t/main.css": "@import \"a.css\" layer(base);\na { color: #010101; }",
		"/t/a.css":    "a { color: #020202; }",
	}
	s, warns := importFiles(t, files, "/t/main.css")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("the importing sheet's unlayered rule outranks the imported layer: %08x", uint32(v.Color))
	}
	files = map[string]string{
		"/t/main.css": "@import \"a.css\" layer;\na { color: #010101; }",
		"/t/a.css":    "a { color: #020202; }",
	}
	s, _ = importFiles(t, files, "/t/main.css")
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("a bare layer() qualifier anonymizes: %08x", uint32(v.Color))
	}
}

func TestImportSupportsQualifier(t *testing.T) {
	files := map[string]string{
		"/t/main.css": "@import \"a.css\" supports(color: red);\nb { color: #010101; }",
		"/t/a.css":    "a { color: #020202; } b { color: #030303; }",
	}
	s, warns := importFiles(t, files, "/t/main.css")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("a true supports() import applies: %08x", uint32(v.Color))
	}
	files = map[string]string{
		"/t/main.css": "@import \"a.css\" supports(display: run-in);\na { color: #010101; }",
		"/t/a.css":    "a { color: #020202; }",
	}
	s, warns = importFiles(t, files, "/t/main.css")
	if len(warns) != 1 || !strings.Contains(warns[0], "supports") {
		t.Fatalf("a false supports() warns and drops: %v", warns)
	}
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("a dropped import leaves the sheet's own rules: %08x", uint32(v.Color))
	}
}

func TestDottedLayerOrder(t *testing.T) {
	s := parseOne(t, `
		@layer a { a { color: #010101; } }
		@layer a.b { a { color: #020202; } }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("a outranks its nested a.b: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer a.b { a { color: #010101; } }
		@layer c { a { color: #020202; } }
		a { color: #030303; }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x03, 0x03, 0x03) {
		t.Errorf("unlayered outranks every layer, dotted or not: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer a { a { color: #010101; } }
		@layer a.b, a.c { a { color: #020202; } }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("a statement declaring nested names still nests them: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer a.b { a { color: #010101; } }
		@layer a.b { a { color: #020202; } }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("the later declaration in the same dotted layer wins: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer a { @layer b { a { color: #010101; } } a { color: #020202; } }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("a nested @layer block inside @layer a means a.b: %08x", uint32(v.Color))
	}
}

func TestRevertLayerChain(t *testing.T) {
	s := parseOne(t, `
		@layer base { a { color: #010101; } }
		@layer mid { a { color: #020202; } }
		a { color: revert-layer; }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("an unlayered revert-layer rolls into the top layer: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer base { a { color: #010101; } }
		@layer mid { a { color: #020202; } }
		@layer top { a { color: revert-layer; } }
		@layer over { a { color: revert-layer; } }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("two revert-layer steps walk two layers down: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer base { a { color: #010101; } }
		@layer mid { a { color: revert-layer; } }
		@layer top { a { color: revert-layer; } }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("a chain of revert-layers rolls to the bottom layer: %08x", uint32(v.Color))
	}
	s = parseOne(t, `
		@layer base { a { color: #010101; } }
		@layer mid { a { color: revert-layer; } }
		@layer top { a { color: revert-layer; } }
		@layer over { a { color: revert-layer; } }
	`)
	if v := computeTree(s, el("a")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("a revert-layer beneath the last layer stops at base: %08x", uint32(v.Color))
	}
}

func TestNthChildOfSelector(t *testing.T) {
	s := parseOne(t, `
		item:nth-child(2 of .kept) { color: #010101; }
		item:nth-last-child(1 of .kept) { color: #020202; }
	`)
	row := el("row")
	first := row.add("item", "kept")
	row.add("item")
	second := row.add("item", "kept")
	third := row.add("item", "kept")
	if v := computeTree(s, first); v.Has(PropColor) {
		t.Error("the first kept sibling is nth-child 1")
	}
	if v := computeTree(s, second); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("the index counts only matching siblings: %08x", uint32(v.Color))
	}
	if v := computeTree(s, third); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("nth-last-child counts from the matching end: %08x", uint32(v.Color))
	}
	s = parseOne(t, `.a:nth-child(odd of .b) { color: #030303; }`)
	n := el("x", "a")
	el("x", "b").parent = n
	if v := computeTree(s, n.add("y", "b")); v.Has(PropColor) {
		t.Error("a node outside the of-list has no index")
	}
}

func TestContentURLAndQuotes(t *testing.T) {
	s := parseOne(t, `
		.a::before { content: "x" url("img.png"); }
		.b::before { content: open-quote close-quote; }
		.c::before { content: open-quote open-quote "in" close-quote; }
		.d::before { content: no-open-quote "silent"; }
	`)
	a := pseudoCompute(s, el("div", "a"), "before")
	if !a.HasContent || a.Content != "x" || a.ContentURL != "img.png" {
		t.Errorf("content url(): text %q url %q has %v", a.Content, a.ContentURL, a.HasContent)
	}
	b := pseudoCompute(s, el("div", "b"), "before")
	if b.Content != "\u201c\u201d" || b.QuoteDepth != 0 {
		t.Errorf("open/close pair: %q depth %d", b.Content, b.QuoteDepth)
	}
	c := pseudoCompute(s, el("div", "c"), "before")
	if c.Content != "\u201c\u2018in\u2019" || c.QuoteDepth != 1 {
		t.Errorf("nested quotes: %q depth %d", c.Content, c.QuoteDepth)
	}
	d := pseudoCompute(s, el("div", "d"), "before")
	if d.Content != "silent" || d.QuoteDepth != 1 {
		t.Errorf("no-open-quote counts silently: %q depth %d", d.Content, d.QuoteDepth)
	}
	// The quote depth moves to the element's own values when its
	// pseudo-element's content resolves: descendants' open-quote
	// continues the count.
	parent := el("div", "c")
	pv := computeTree(s, parent)
	var sc Scratch
	var out Values
	ComputePseudo([]Layer{{Sheet: s}}, parent, &pv, Env{}, &sc, &out, "before")
	if pv.QuoteDepth != 1 {
		t.Errorf("the element inherits its pseudo's quote depth: %d", pv.QuoteDepth)
	}
}

func TestPropertyRegistration(t *testing.T) {
	s := parseOne(t, `
		@property --gap {
			syntax: "<length>";
			inherits: true;
			initial-value: 8px;
		}
		@property --tint {
			syntax: "<color>";
			inherits: false;
			initial-value: #010203;
		}
		a { padding: var(--gap); }
		b { color: var(--tint); }
		.set { --gap: 20px; }
	`)
	v := computeTree(s, el("a"))
	if v.Padding != (Sides{8, 8, 8, 8}) {
		t.Errorf("an inherited registration's initial value answers: %v", v.Padding)
	}
	if v := computeTree(s, el("window").add("a")); v.Padding != (Sides{8, 8, 8, 8}) {
		t.Errorf("the initial value inherits down the chain: %v", v.Padding)
	}
	if v := computeTree(s, el("b")); v.Color != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("a typed color registration computes: %08x", uint32(v.Color))
	}
	root := el("b", "set")
	root.target.Inline = ParseDeclarations("--tint: #040506")
	if v := computeTree(s, root); v.Color != render.RGB(0x04, 0x05, 0x06) {
		t.Errorf("an own declaration beats the registration: %08x", uint32(v.Color))
	}
	kid := root.add("b")
	if v := computeTree(s, kid); v.Color != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("inherits: false stops the chain at the initial value: %08x", uint32(v.Color))
	}
	if v := computeTree(s, root.add("a")); v.Padding != (Sides{20, 20, 20, 20}) {
		t.Errorf("an own --gap overrides the initial value: %v", v.Padding)
	}
}

func TestPropertyBadRegistration(t *testing.T) {
	warns := capture(t)
	Parse(`@property --x { syntax: "<length>"; inherits: true; initial-value: red; }`)
	if len(*warns) != 1 || !strings.Contains((*warns)[0], "initial-value") {
		t.Fatalf("a mistyped initial value warns: %v", *warns)
	}
	warns = capture(t)
	Parse(`@property --x { inherits: true; initial-value: 1px; }`)
	if len(*warns) != 1 || !strings.Contains((*warns)[0], "syntax") {
		t.Fatalf("a missing syntax warns: %v", *warns)
	}
	warns = capture(t)
	Parse(`@property --x { syntax: "<bogus>"; }`)
	if len(*warns) != 1 || !strings.Contains((*warns)[0], "syntax") {
		t.Fatalf("an unknown syntax warns: %v", *warns)
	}
}

func TestScopeProximity(t *testing.T) {
	s := parseOne(t, `
		@scope (.card) {
			p { color: #010101; }
			@scope (.inner) {
				p { color: #020202; }
			}
		}
	`)
	deep := el("div", "card").add("div", "inner").add("p")
	if v := computeTree(s, deep); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("the inner scope wins at equal specificity: %08x", uint32(v.Color))
	}
	shallow := el("div", "card").add("p")
	if v := computeTree(s, shallow); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf("the outer scope still applies outside .inner: %08x", uint32(v.Color))
	}
	if v := computeTree(s, el("div", "other").add("p")); v.Has(PropColor) {
		t.Error("a rule outside its scope root does not apply")
	}
}

func TestScopePseudoAndUpperBound(t *testing.T) {
	s := parseOne(t, `
		@scope (.card) to (.limit) {
			:scope > .head { color: #010101; }
			p { color: #020202; }
		}
	`)
	card := el("div", "card")
	if v := computeTree(s, card.add("div", "head")); v.Color != render.RGB(0x01, 0x01, 0x01) {
		t.Errorf(":scope anchors the root: %08x", uint32(v.Color))
	}
	limit := card.add("div", "limit")
	if v := computeTree(s, limit.add("p")); v.Has(PropColor) {
		t.Error("the upper bound ends the scope")
	}
	if v := computeTree(s, card.add("p")); v.Color != render.RGB(0x02, 0x02, 0x02) {
		t.Errorf("below the upper bound the scope applies: %08x", uint32(v.Color))
	}
}

func TestFontFaceRule(t *testing.T) {
	files := map[string]string{
		"/t/main.css": "@font-face { font-family: App Sans; src: url(sans.ttf) format(\"truetype\"); } a { color: red; }",
		"/t/sans.ttf": "binary",
	}
	s, warns := importFiles(t, files, "/t/main.css")
	if len(warns) != 0 {
		t.Fatalf("warns = %v", warns)
	}
	faces := s.FontFaces()
	if len(faces) != 1 {
		t.Fatalf("faces = %d, want 1", len(faces))
	}
	f := faces[0]
	if f.Family != "App Sans" || !strings.HasSuffix(f.Src, "sans.ttf") || f.Format != "truetype" || f.WeightLo != 400 || f.WeightHi != 400 {
		t.Errorf("face = %+v", f)
	}
	warnsFF := capture(t)
	s = Parse(`@font-face { font-family: W; font-weight: 300 700; src: url(w.woff2) format("woff2"), url(w.ttf); font-style: italic; }`)
	if len(*warnsFF) != 1 || !strings.Contains((*warnsFF)[0], "format") {
		t.Fatalf("the unsupported src warns and falls through: %v", *warnsFF)
	}
	faces = s.FontFaces()
	if len(faces) != 1 {
		t.Fatalf("faces = %d, want 1", len(faces))
	}
	if faces[0].WeightLo != 300 || faces[0].WeightHi != 700 || !faces[0].Italic || !strings.HasSuffix(faces[0].Src, "w.ttf") {
		t.Errorf("weight range and src fallback: %+v", faces[0])
	}
	badFmt := capture(t)
	Parse(`@font-face { font-family: Bad; src: url(bad.ttf) format("svg"); }`)
	if len(*badFmt) != 2 || !strings.Contains((*badFmt)[0], "format") {
		t.Fatalf("an unsupported format warns and drops the face: %v", *badFmt)
	}
	noFam := capture(t)
	Parse(`@font-face { src: url(x.ttf); }`)
	if len(*noFam) != 1 || !strings.Contains((*noFam)[0], "font-family") {
		t.Fatalf("a missing family warns: %v", *noFam)
	}
	files = map[string]string{
		"/t/main.css": "@font-face { font-family: Gone; src: url(missing.ttf); }",
	}
	_, warns2 := importFiles(t, files, "/t/main.css")
	if len(warns2) != 2 || !strings.Contains(warns2[0], "missing.ttf") {
		t.Fatalf("an unreadable src reports through the stylesheet error path: %v", warns2)
	}
}
