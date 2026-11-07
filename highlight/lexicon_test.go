package highlight

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-ui/widget"
)

func lang(t *testing.T, name string) widget.Highlighter {
	t.Helper()
	h, ok := Lookup(name)
	if !ok {
		t.Fatalf("no language %q", name)
	}
	return h
}

// Each language styles its comments, strings, numbers and words; what
// only looks like one of them (a#b in a shell, a Rust lifetime, len not
// called in Python) stays plain.
func TestLanguageLines(t *testing.T) {
	for _, tc := range []struct{ lang, line, want string }{
		{"c", `#include <stdio.h>`, `preprocessor:#include <stdio.h>`},
		{"c", `int main(void) { char c = '\n'; return 0x1F + 3.5f; } // done`, `type:int type:void type:char character:'\n' keyword:return base-n-integer:0x1F floating-point:3.5f comment:// done`},
		{"cpp", `std::string s = "a\tb"; auto p = nullptr; template <typename T> class X {};`, `string:"a special-char:\t string:b" keyword:auto special-constant:nullptr keyword:template keyword:typename keyword:class`},
		{"rust", `#[derive(Debug)] fn f<'a>(x: &'a str) -> Option<u32> { println!("{}", 1_000u32); r#"raw"# }`, `preprocessor:#[derive(Debug)] keyword:fn type:str type:Option type:u32 preprocessor:println! string:"{}" decimal:1_000u32 string:r#"raw"#`},
		{"rust", `let c = 'x'; let b = b"bytes"; let t = true;`, `keyword:let character:'x' keyword:let string:b"bytes" keyword:let boolean:true`},
		{"javascript", "const x = `t ${y}` + 'a' + \"b\"; if (x === null) return undefined; // c", "keyword:const string:`t ${y}` string:'a' string:\"b\" keyword:if special-constant:null keyword:return special-constant:undefined comment:// c"},
		{"typescript", `interface A { n: number; s?: string } @Component export type B = keyof A;`, `keyword:interface type:number type:string preprocessor:@Component keyword:export keyword:type keyword:keyof`},
		{"java", `@Override public static void main(String[] args) { int x = 0; }`, `preprocessor:@Override keyword:public keyword:static type:void type:String type:int decimal:0`},
		{"csharp", `public class A { string s = @"C:\x"; bool b = true; }`, `keyword:public keyword:class type:string string:@"C:\x" type:bool boolean:true`},
		{"python", `@app.route("/") def f(self, x=None): return len(x) if True else print`, `preprocessor:@app.route string:"/" keyword:def special-constant:self special-constant:None keyword:return builtin:len keyword:if boolean:True keyword:else`},
		{"python", `x = """doc""" + 'it\'s' # comment`, `string:"""doc""" string:'it special-char:\' string:s' comment:# comment`},
		{"python", `len = 3`, `decimal:3`},
		{"sh", `if [ -n "$HOME" ]; then echo ${PATH} $1 $(pwd) 'lit' # c`, `keyword:if string:"$HOME" keyword:then builtin:echo identifier:${PATH} identifier:$1 identifier:$(pwd) string:'lit' comment:# c`},
		{"sh", `echo a#b ${#arr} $#`, `builtin:echo identifier:${#arr} identifier:$#`},
		{"nix", `{ pkgs ? import <nixpkgs> {} }: let x = "s${y}"; in rec { a = true; b = null; } # c`, `builtin:import keyword:let string:"s${y}" keyword:in keyword:rec boolean:true special-constant:null comment:# c`},
		{"ruby", `def greet(name) puts "hi #{name}" end # c`, `keyword:def builtin:puts string:"hi #{name}" keyword:end comment:# c`},
		{"lua", `local function f(x) return nil, "s", [[long]] end -- c`, `keyword:local keyword:function keyword:return special-constant:nil string:"s" string:[[long]] keyword:end comment:-- c`},
		{"lua", `--[[ block ]] x = 1`, `comment:--[[ block ]] decimal:1`},
		{"sql", `SELECT id FROM users WHERE age > 18 AND x IS NULL; -- c`, `keyword:SELECT keyword:FROM keyword:WHERE decimal:18 keyword:AND keyword:IS special-constant:NULL comment:-- c`},
		{"sql", `select 'it''s'`, `keyword:select string:'it' string:'s'`},
		{"makefile", `build: main.o $(OBJS) # c`, `function:build identifier:$(OBJS) comment:# c`},
		{"makefile", `CC := gcc`, ``},
		{"dockerfile", `FROM golang:1.22 AS build`, `keyword:FROM floating-point:1.22 keyword:AS`},
		{"html", `<!DOCTYPE html><a href="x.html" class='c'>A &amp; B</a> <!-- c -->`, `preprocessor:<!DOCTYPE html> keyword:<a type:href string:"x.html" type:class string:'c' keyword:> special-char:&amp; keyword:</a keyword:> comment:<!-- c -->`},
		{"xml", `<?xml version="1.0"?><item id="1"/>`, `preprocessor:<?xml version="1.0"?> keyword:<item type:id string:"1" keyword:/>`},
		{"html", `a < b && c`, ``},
		{"css", `.btn:hover, #id > a { color: #fff; margin: 0 1.5em !important; background: url("x.png"); }`, `type:.btn keyword:hover type:#id keyword:a type:color constant:#fff type:margin decimal:0 decimal:1.5em keyword:!important type:background builtin:url string:"x.png"`},
		{"ini", `[Unit]`, `keyword:[Unit]`},
		{"ini", `Description = My service`, `type:Description`},
		{"ini", `Enabled=true`, `type:Enabled boolean:true`},
		{"ini", `Port = 8080`, `type:Port decimal:8080`},
		{"ini", `; comment`, `comment:; comment`},
		{"diff", `@@ -1,3 +1,4 @@ func`, `preprocessor:@@ -1,3 +1,4 @@ func`},
		{"diff", `+added`, `inserted:+added`},
		{"diff", `-removed`, `deleted:-removed`},
		{"diff", `--- a/x.go`, `keyword:--- a/x.go`},
		{"diff", ` context`, ``},
	} {
		spans, end := lang(t, tc.lang).Highlight([]rune(tc.line), 0)
		if got := show(tc.line, spans); got != tc.want {
			t.Errorf("%s: %s\n got %s\nwant %s", tc.lang, tc.line, got, tc.want)
		}
		if end != 0 {
			t.Errorf("%s: %s ends in state %d, want 0", tc.lang, tc.line, end)
		}
	}
}

// What spans lines - block comments, long strings, a tag's attributes,
// a CSS block - carries over through the line state and closes where
// the language closes it.
func TestLanguageStateCarries(t *testing.T) {
	for _, tc := range []struct {
		lang      string
		doc, want []string
	}{
		{"c", []string{"/* a", "b */ int x;"}, []string{"comment:/* a", "comment:b */ type:int"}},
		{"python", []string{`s = """one`, `two""" + x`}, []string{`string:"""one`, `string:two"""`}},
		{"rust", []string{`let s = "one`, `two";`}, []string{`keyword:let string:"one`, `string:two"`}},
		{"javascript", []string{"a = `one", "two` // c"}, []string{"string:`one", "string:two` comment:// c"}},
		{"lua", []string{"x = [[one", "two]] -- c", "--[[ a", "b ]] y"}, []string{"string:[[one", "string:two]] comment:-- c", "comment:--[[ a", "comment:b ]]"}},
		{"nix", []string{"x = ''", "  text ${y}", "'';"}, []string{"string:''", "string:  text ${y}", "string:''"}},
		{"sh", []string{`echo "one`, `two" # c`}, []string{`builtin:echo string:"one`, `string:two" comment:# c`}},
		{"html", []string{`<a href="x"`, `   class="y">t</a>`, `<!-- c`, `--> <b>`}, []string{`keyword:<a type:href string:"x"`, `type:class string:"y" keyword:> keyword:</a keyword:>`, `comment:<!-- c`, `comment:--> keyword:<b keyword:>`}},
		{"html", []string{`<a title="one`, `two">`}, []string{`keyword:<a type:title string:"one`, `string:two" keyword:>`}},
		{"css", []string{"a {", "  color: red;", "  /* c", "  */ margin: 0;", "}", "p { }"}, []string{"keyword:a", "type:color", "comment:/* c", "comment:  */ type:margin decimal:0", "", "keyword:p"}},
	} {
		checkDoc(t, lang(t, tc.lang), tc.doc, tc.want)
	}
}

// Spans are in order and never overlap or run empty, whatever a
// language meets.
func TestSpansStayOrdered(t *testing.T) {
	junk := []string{`"'` + "`" + `\`, `/*/*/ */ */`, `#[#![`, `$${$(`, `<a <b c="`, `@@@`, `''''`, `--[[]]`, `0x 1e 1.e5 .5`, `é ü 漢字 "ß"`}
	for _, name := range Languages() {
		h := lang(t, name)
		for _, line := range junk {
			state := 0
			for range 2 {
				var spans []widget.TextSpan
				spans, state = h.Highlight([]rune(line), state)
				prev := 0
				for _, sp := range spans {
					if sp.Start < prev || sp.End <= sp.Start || sp.End > len([]rune(line)) {
						t.Fatalf("%s: %q: spans %+v", name, line, spans)
					}
					prev = sp.End
				}
			}
		}
	}
}

// Detect finds a language by file name, then by the #! line's
// interpreter, then by a vim or Emacs modeline at either end; text that
// says none of these has none.
func TestDetect(t *testing.T) {
	long := strings.Repeat("filler\n", 20)
	for _, tc := range []struct {
		name, text string
		want       widget.Highlighter
	}{
		{"/src/lib.rs", "", langRust},
		{"Makefile", "", langMake},
		{"x.sh", "#!/usr/bin/env python3\n", langShell},
		{"", "#!/bin/bash\necho\n", langShell},
		{"", "#!/usr/bin/env python3\nprint()\n", langPython},
		{"", "#!/usr/bin/python3.12\n", langPython},
		{"", "#!/usr/bin/env -S node --no-warnings\n", langJavaScript},
		{"", "#!/usr/bin/env LANG=C ruby\n", langRuby},
		{"script", "# vim: set ft=lua :\nx = 1\n", langLua},
		{"", long + "// vim: ft=cpp\n", langCPP},
		{"", "# -*- mode: python; coding: utf-8 -*-\n", langPython},
		{"", "-*- nix -*-\n", langNix},
		{"notes.txt", "just some words\n", nil},
		{"", "#!/usr/bin/env frobnicate\n", nil},
		{"", long + "vim: ft=cobol\n", nil},
		{"", "", nil},
	} {
		h, ok := Detect(tc.name, tc.text)
		if ok != (tc.want != nil) || h != tc.want {
			t.Errorf("Detect(%q, %q) = %v %v, want %v", tc.name, tc.text, h, ok, tc.want)
		}
	}
}
