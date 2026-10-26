package highlight

import (
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/golden"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// lineCase is one line and its expected spans (show's notation).
type lineCase struct{ line, want string }

// checkLines runs single-line cases from state 0, each expected to end
// in state 0.
func checkLines(t *testing.T, h widget.Highlighter, cases []lineCase) {
	t.Helper()
	for _, tc := range cases {
		spans, end := h.Highlight([]rune(tc.line), 0)
		if got := show(tc.line, spans); got != tc.want {
			t.Errorf("%s\n got %s\nwant %s", tc.line, got, tc.want)
		}
		if end != 0 {
			t.Errorf("%s: ends in state %d, want 0", tc.line, end)
		}
	}
}

// checkDoc runs a document line by line through the carried state.
func checkDoc(t *testing.T, h widget.Highlighter, doc, want []string) {
	t.Helper()
	state := 0
	for i, line := range doc {
		var spans []widget.TextSpan
		spans, state = h.Highlight([]rune(line), state)
		if got := show(line, spans); got != want[i] {
			t.Errorf("line %d %q\n got %s\nwant %s", i, line, got, want[i])
		}
	}
	if state != 0 {
		t.Errorf("the document ends in state %d", state)
	}
}

func TestGoLines(t *testing.T) {
	checkLines(t, Go{}, []lineCase{
		{`package main // entry`, `keyword:package comment:// entry`},
		{`//go:build linux`, `preprocessor://go:build linux`},
		{`// +build linux`, `preprocessor:// +build linux`},
		{`func f(s string) error { return nil }`, `keyword:func type:string type:error keyword:return special-constant:nil`},
		{`x := len(a) + cap`, `builtin:len`},
		{`ok := true || false`, `boolean:true boolean:false`},
		{`s := "a\tb\x41\u00e9"`, `string:"a special-char:\t string:b special-char:\x41 special-char:\u00e9 string:"`},
		{"r := '\\n' + 'x'", `character:' special-char:\n character:' character:'x'`},
		{"raw := `C:\\path` // c", "string:`C:\\path` comment:// c"},
		{`n := 42 + 0x_ff + 0o17 + 0b1010 + 1_000`, `decimal:42 base-n-integer:0x_ff base-n-integer:0o17 base-n-integer:0b1010 decimal:1_000`},
		{`f := 3.14 + 1e9 + .5 + 2i + 1.5i`, `floating-point:3.14 floating-point:1e9 floating-point:.5 complex:2i complex:1.5i`},
		{`a /* inline */ b /*/ still */`, `comment:/* inline */ comment:/*/ still */`},
	})
}

func TestGoStateCarries(t *testing.T) {
	checkDoc(t, Go{}, []string{
		"/* a block",
		"   comment */ var x = `raw",
		"still raw`",
		"x++",
	}, []string{
		`comment:/* a block`,
		"comment:   comment */ keyword:var string:`raw",
		"string:still raw`",
		``,
	})
}

func TestJSONLines(t *testing.T) {
	checkLines(t, JSON{}, []lineCase{
		{
			`{"name": "lys-ui", "n": -1.5e3, "ok": true, "none": null}`,
			`type:"name" string:"lys-ui" type:"n" floating-point:-1.5e3 type:"ok" boolean:true type:"none" special-constant:null`,
		},
		{`["a\n", 42, false]`, `string:"a special-char:\n string:" decimal:42 boolean:false`},
		{`{"k" : 1}`, `type:"k" decimal:1`},
		{`{oops: 1}`, `error:o error:o error:p error:s decimal:1`},
	})
}

func TestYAMLLines(t *testing.T) {
	checkLines(t, YAML{}, []lineCase{
		{`# config`, `comment:# config`},
		{`---`, `preprocessor:---`},
		{`name: lys-ui # the toolkit`, `type:name comment:# the toolkit`},
		{`  enabled: yes`, `type:enabled boolean:yes`},
		{`count: 42`, `type:count decimal:42`},
		{`ratio: .5`, `type:ratio floating-point:.5`},
		{`empty: ~`, `type:empty special-constant:~`},
		{`- item`, ``},
		{`- key: "v\t1"`, `type:key string:"v special-char:\t string:1"`},
		{`base: &base {a: 1}`, `type:base identifier:&base type:a decimal:1`},
		{`<<: *base`, `type:<< identifier:*base`},
		{`when: !!timestamp 2001-12-14`, `type:when preprocessor:!!timestamp`},
		{`url: http://x:80/y`, `type:url`},
		{`"quoted key": 'it''s'`, `type:"quoted key" string:'it' string:'s'`},
	})
}

func TestYAMLStateCarries(t *testing.T) {
	checkDoc(t, YAML{}, []string{
		`script: |`,
		`  echo one`,
		``,
		`  echo two`,
		`next: "multi`,
		`  line" # end`,
		`done: true`,
	}, []string{
		`type:script special-char:|`,
		`string:echo one`,
		``,
		`string:echo two`,
		`type:next string:"multi`,
		`string:  line" comment:# end`,
		`type:done boolean:true`,
	})
}

func TestMarkdownLines(t *testing.T) {
	checkLines(t, Markdown{}, []lineCase{
		{`# Title`, `heading:# Title`},
		{`###### Six`, `heading:###### Six`},
		{`#hashtag`, ``},
		{`---`, `thematic-break:---`},
		{`* * *`, `thematic-break:* * *`},
		{`> quoted *it*`, `blockquote-marker:> emphasis:*it*`},
		{`- item with **bold**`, `list-marker:- strong-emphasis:**bold**`},
		{`12. numbered`, `list-marker:12.`},
		{"Use `go test` here", "inline-code:`go test`"},
		{"``code with ` tick``", "inline-code:``code with ` tick``"},
		{`snake_case_name and _em_`, `emphasis:_em_`},
		{`a * b * c`, ``},
		{`see [docs](https://x.y) or <https://z>`, `link-text:[docs] link-destination:(https://x.y) link-destination:<https://z>`},
		{`not \*emphasis\*`, `special-char:\* special-char:\*`},
	})
}

// A fenced block hands its lines to the hinted language - its own
// multi-line state carried inside Markdown's - and an unknown hint is
// preformatted.
func TestMarkdownFences(t *testing.T) {
	checkDoc(t, Markdown{}, []string{
		"Intro *text*",
		"```go",
		"s := `raw",
		"still raw`",
		"```",
		"~~~~ toml",
		`cmd = """first`,
		"~~~",
		`last"""`,
		"~~~~",
		"```",
		"plain *code*",
		"```",
		"**after**",
	}, []string{
		`emphasis:*text*`,
		"preformatted-section:```go",
		"string:`raw",
		"string:still raw`",
		"preformatted-section:```",
		"preformatted-section:~~~~ toml",
		`type:cmd string:"""first`,
		"string:~~~",
		`string:last"""`,
		"preformatted-section:~~~~",
		"preformatted-section:```",
		"preformatted-section:plain *code*",
		"preformatted-section:```",
		`strong-emphasis:**after**`,
	})
}

// TestRegistry pins the language table: lookup by name and alias in
// any case, by file name, listing, and an app's override and addition.
func TestRegistry(t *testing.T) {
	for name, want := range map[string]widget.Highlighter{"GO": Go{}, "golang": Go{}, "yml": YAML{}, "md": Markdown{}, "toml": TOML{}} {
		if h, ok := Lookup(name); !ok || h != want {
			t.Errorf("Lookup(%q) = %v %v", name, h, ok)
		}
	}
	for path, want := range map[string]widget.Highlighter{"/src/main.go": Go{}, "a/b.yml": YAML{}, "Cargo.lock": TOML{}, "README.md": Markdown{}, "x.jsonc": JSON{}} {
		if h, ok := ForFile(path); !ok || h != want {
			t.Errorf("ForFile(%q) = %v %v", path, h, ok)
		}
	}
	if _, ok := Lookup("cobol"); ok {
		t.Error("an unknown language resolved")
	}
	before := Languages()
	t.Cleanup(func() { Register(builtinLanguages[2]) })
	Register(Language{Name: "JSON", Highlighter: TOML{}})
	if h, _ := Lookup("json"); h != (TOML{}) || !slices.Equal(Languages(), before) {
		t.Error("an override did not replace the language in place")
	}
	if !slices.Equal(before, []string{"go", "json", "markdown", "toml", "yaml"}) {
		t.Errorf("Languages() = %v", before)
	}
}

// sample is the document the scheme goldens render: Markdown around a
// fenced Go block.
const sample = "# Highlight\n" +
	"Some *emphasis*, **strong** and `code`.\n" +
	"- see [docs](https://x.y)\n" +
	"```go\n" +
	"// Sum adds.\n" +
	"func Sum(xs []int) int {\n" +
	"\tn := 0 // total\n" +
	"\tfor _, x := range xs { n += x }\n" +
	"\treturn n + len(\"a\\tb\")\n" +
	"}\n" +
	"```"

// TestGoldenSchemes renders the sample through each Adwaita scheme on
// its theme.
func TestGoldenSchemes(t *testing.T) {
	face, err := render.NewFixtureTypeface()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		th   *widget.Theme
	}{{"scheme-adwaita", widget.LightTheme()}, {"scheme-adwaita-dark", widget.DarkTheme()}} {
		prev := widget.Current()
		widget.SetTheme(tc.th)
		ta := widget.NewTextArea(face, 13, tc.th.Text)
		ta.SetText(strings.ReplaceAll(sample, "\t", "    "))
		ta.SetHighlighter(Markdown{}, SchemeFor(tc.th))
		const w, h = 360, 220
		ta.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
		ta.Arrange(render.Rect{W: w, H: h})
		stride := render.Stride(w)
		buf := make([]byte, stride*h)
		cv := render.New(buf, stride, w, h)
		cv.Clear(cv.Rect(), tc.th.Bg)
		ta.Paint(cv)
		widget.SetTheme(prev)
		golden.Check(t, "testdata/golden", tc.name, render.NRGBA(buf, stride, w, h), golden.Tolerance{})
	}
}
