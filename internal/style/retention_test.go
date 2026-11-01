package style

import (
	"runtime"
	"strings"
	"testing"
	"weak"
)

const retentionCSS = `
@define-color accent #336699;
@property --gap { syntax: "<length>"; inherits: false; initial-value: 4px; }
.box > .child:hover, .other .deep .selector { color: @accent; margin: var(--gap) 2px; }
`

func TestASheetDoesNotKeepTheTokenStreamAlive(t *testing.T) {
	sheet, stream := func() (*Sheet, weak.Pointer[token]) {
		toks := tokenize(retentionCSS)
		p := parser{src: retentionCSS, cq: -1}
		p.stylesheet(toks)
		return &p.sheet, weak.Make(&toks[0])
	}()
	runtime.GC()
	if stream.Value() != nil {
		t.Error("the parsed sheet still holds the token stream")
	}
	if len(sheet.rules) == 0 {
		t.Fatal("no rules parsed")
	}
	found := false
	for _, r := range sheet.rules {
		for _, d := range r.block.decls {
			found = found || d.name == "margin"
			if cap(d.val) != len(d.val) {
				t.Errorf("%s value has capacity %d past its %d tokens", d.name, cap(d.val), len(d.val))
			}
		}
	}
	if !found {
		t.Error("the margin declaration was not kept")
	}
	runtime.KeepAlive(sheet)
}

func TestKeptValuesAreIndependent(t *testing.T) {
	var p parser
	a, b := p.keep([]token{{s: "a"}}), p.keep([]token{{s: "b"}})
	a = append(a, token{s: "grown"})
	if b[0].s != "b" || a[0].s != "a" {
		t.Errorf("a growing value overwrote its neighbour: a %v b %v", a, b)
	}
	large := make([]token, keptChunk+1)
	if got := p.keep(large); len(got) != len(large) {
		t.Errorf("a value over one chunk kept %d of %d tokens", len(got), len(large))
	}
}

func TestParsedValuesStillResolveAfterTheStreamIsGone(t *testing.T) {
	sheet := Parse(strings.ReplaceAll(retentionCSS, "@accent", "#112233"))
	runtime.GC()
	for _, r := range sheet.rules {
		for _, d := range r.block.decls {
			if len(d.val) == 0 {
				t.Errorf("%s lost its value", d.name)
			}
		}
	}
}
