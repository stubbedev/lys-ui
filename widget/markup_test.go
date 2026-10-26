package widget

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stubbedev/lys-lib/render"
)

// mustColor decodes an allowlisted hex color, failing the test on a
// bug in the decoder itself.
func mustColor(tb testing.TB, hex string) render.Color {
	tb.Helper()
	col, ok := parseHexColor(hex)
	if !ok {
		tb.Fatalf("parseHexColor(%q) failed", hex)
	}
	return col
}

// runTexts flattens runs to their text for compact assertions.
func runTexts(runs []MarkupRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.Text
	}
	return out
}

func TestParseMarkup(t *testing.T) {
	t.Run("plain text is a single run", func(t *testing.T) {
		runs, ok := ParseMarkup("just text")
		if !ok {
			t.Fatal("plain text rejected")
		}
		if got := runTexts(runs); len(got) != 1 || got[0] != "just text" {
			t.Errorf("runs = %q, want [just text]", got)
		}
		if runs[0].Style != (TextStyle{}) {
			t.Errorf("style = %+v, want zero", runs[0].Style)
		}
	})

	t.Run("empty markup parses to no runs", func(t *testing.T) {
		runs, ok := ParseMarkup("")
		if !ok || len(runs) != 0 {
			t.Errorf("empty markup = %v, %v", runs, ok)
		}
	})

	t.Run("bold and italic split runs", func(t *testing.T) {
		runs, ok := ParseMarkup("a<b>b</b>c<i>d</i>")
		if !ok {
			t.Fatal("rejected")
		}
		if got := runTexts(runs); strings.Join(got, "|") != "a|b|c|d" {
			t.Fatalf("runs = %q", got)
		}
		if !runs[1].Style.Bold || runs[1].Style.Italic {
			t.Errorf("second run style = %+v", runs[1].Style)
		}
		if !runs[3].Style.Italic || runs[3].Style.Bold {
			t.Errorf("fourth run style = %+v", runs[3].Style)
		}
		if runs[0].Style.Bold || runs[2].Style.Bold || runs[2].Style.Italic {
			t.Errorf("plain runs inherited a style: %+v %+v", runs[0].Style, runs[2].Style)
		}
	})

	t.Run("nested tags combine and colors inherit", func(t *testing.T) {
		runs, ok := ParseMarkup(`<b>one <span color="#00ff00"><i>two</i></span></b>`)
		if !ok {
			t.Fatal("rejected")
		}
		if got := runTexts(runs); strings.Join(got, "|") != "one |two" {
			t.Fatalf("runs = %q", got)
		}
		if !runs[0].Style.Bold {
			t.Errorf("first run style = %+v", runs[0].Style)
		}
		st := runs[1].Style
		if !st.Bold || !st.Italic || st.Color != mustColor(t, "#00ff00") {
			t.Errorf("nested run style = %+v", st)
		}
	})

	t.Run("span without attributes is a no-op", func(t *testing.T) {
		runs, ok := ParseMarkup("<span>x</span>")
		if !ok || len(runs) != 1 || runs[0].Style != (TextStyle{}) {
			t.Errorf("runs = %+v ok = %v", runs, ok)
		}
	})

	t.Run("span colors", func(t *testing.T) {
		for _, c := range []string{"#f00", "#12AB34", "#123456", "#12345678"} {
			runs, ok := ParseMarkup(`<span color="` + c + `">x</span>`)
			if !ok {
				t.Fatalf("%s rejected", c)
			}
			want, ok := parseHexColor(c)
			if !ok {
				t.Fatalf("parseHexColor(%q) rejected", c)
			}
			if runs[0].Style.Color != want {
				t.Errorf("%s: color = %v, want %v", c, runs[0].Style.Color, want)
			}
		}
	})

	t.Run("rgb hex expands like CSS", func(t *testing.T) {
		if col := mustColor(t, "#0aF"); col != mustColor(t, "#00aaff") {
			t.Errorf("#0aF = %v, want the #00aaff it expands to", col)
		}
	})

	t.Run("links carry href with decoded entities", func(t *testing.T) {
		runs, ok := ParseMarkup(`<a href="https://example.com/a&amp;b">x</a>`)
		if !ok {
			t.Fatal("rejected")
		}
		if runs[0].Style.Href != "https://example.com/a&b" {
			t.Errorf("href = %q", runs[0].Style.Href)
		}
	})

	t.Run("escaping", func(t *testing.T) {
		for _, c := range []struct{ in, want string }{
			{"&lt;b&gt;x&amp;y", "<b>x&y"},
			{"&quot;q&quot;", `"q"`},
			{"&apos;a&apos;", "'a'"},
		} {
			runs, ok := ParseMarkup(c.in)
			if !ok {
				t.Fatalf("%q rejected", c.in)
			}
			if got := strings.Join(runTexts(runs), ""); got != c.want {
				t.Errorf("%q decoded to %q, want %q", c.in, got, c.want)
			}
			for _, r := range runs {
				if r.Style.Bold || r.Style.Italic {
					t.Errorf("%q decoded to styled runs: %+v", c.in, r.Style)
				}
			}
		}
	})

	t.Run("attribute punctuation is tolerated", func(t *testing.T) {
		if _, ok := ParseMarkup(`<span color = '#fff' >x</span>`); !ok {
			t.Error("spaces around = and > rejected")
		}
		runs, ok := ParseMarkup("<b>x</b >")
		if !ok || len(runs) != 1 {
			t.Errorf("trailing space in close tag rejected: %v %v", runs, ok)
		}
	})

	t.Run("malformed inputs fail", func(t *testing.T) {
		for _, in := range []string{
			"<b>unclosed",                   // unclosed tag
			"<b>x</i>",                      // mismatched close
			"<b>x</b></b>",                  // stray close
			"</b>x",                         // close without open
			"<foo>x</foo>",                  // unknown tag
			"<B>x</B>",                      // uppercase tag
			"< b>x",                         // space before name
			"<b",                            // truncated tag
			"<",                             // lone bracket
			`<span nope="1">x</span>`,       // unknown attribute
			`<span color>x</span>`,          // valueless attribute
			`<span color="#zzz">x</span>`,   // bad color
			`<span color="#12345">x</span>`, // bad color length
			`<a>x</a>`,                      // missing href
			`<a href="">x</a>`,              // empty href
			`<a url="x">y</a>`,              // wrong attribute
			"<b/>x",                         // self-closing
			"a < b",                         // bare <
			"AT&T",                          // bare &
			"&amp",                          // unterminated entity
			"&nbsp;",                        // unknown entity
			"&#38;",                         // numeric entity
			"&;",                            // empty entity
		} {
			if runs, ok := ParseMarkup(in); ok {
				t.Errorf("%q parsed to %q", in, runTexts(runs))
			}
		}
	})
}

// TestParseMarkupRoundTripsPlainText pins the contract the widget
// relies on for plain strings: input with no markup characters parses
// to exactly one run of itself.
func TestParseMarkupRoundTripsPlainText(t *testing.T) {
	for _, in := range []string{"just text", "a \u0301 combining", "日本語 テキスト", "50% done; (ok)"} {
		runs, ok := ParseMarkup(in)
		if !ok || len(runs) != 1 || runs[0].Text != in {
			t.Errorf("%q: runs = %q ok = %v, want one identical run", in, runTexts(runs), ok)
		}
	}
}

// FuzzParseMarkup exercises the parser with arbitrary input. It must
// never panic; whatever it accepts must decode deterministically, keep
// valid UTF-8 valid, and pass markup-free text through untouched.
func FuzzParseMarkup(f *testing.F) {
	for _, seed := range []string{
		"", "plain", "<b>bold</b>", "<i>x</i>", `<span color="#fff">y</span>`,
		`<a href="http://x">z</a>`, "&amp;&lt;&gt;", "<b><i><span>",
		"<b>", "</b>", "&", "&#x1f;", "<span color='#0f0'", "a<b>b</i>c</b>",
		"日本語<b>テキスト</b>", "a\u0301", "\x00\xff\xfe",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		runs, ok := ParseMarkup(in)
		if !ok {
			return
		}
		again, ok2 := ParseMarkup(in)
		if !ok2 || !reflect.DeepEqual(runs, again) {
			t.Fatalf("ParseMarkup(%q) is not deterministic", in)
		}
		valid := utf8.ValidString(in)
		var sb strings.Builder
		for _, r := range runs {
			sb.WriteString(r.Text)
			if valid && !utf8.ValidString(r.Text) {
				t.Fatalf("ParseMarkup(%q) broke UTF-8 in run %q", in, r.Text)
			}
		}
		if !strings.ContainsAny(in, "<&") && sb.String() != in {
			t.Fatalf("ParseMarkup(%q) = %q, want the untouched text", in, sb.String())
		}
	})
}

// <u> and <s> mark their runs underlined and struck, nesting with the
// rest; the plain text around them stays plain.
func TestParseMarkupDecorations(t *testing.T) {
	runs, ok := ParseMarkup("a<u>b<s>c</s></u><s>d</s>")
	if !ok {
		t.Fatal("rejected")
	}
	if got := runTexts(runs); strings.Join(got, "|") != "a|b|c|d" {
		t.Fatalf("runs = %q", got)
	}
	want := []TextStyle{{}, {Underline: true}, {Underline: true, Strikethrough: true}, {Strikethrough: true}}
	for i, w := range want {
		if runs[i].Style != w {
			t.Errorf("run %d style = %+v, want %+v", i, runs[i].Style, w)
		}
	}
	if _, ok := ParseMarkup("<u>x</s>"); ok {
		t.Error("a mismatched decoration close parsed")
	}
}
