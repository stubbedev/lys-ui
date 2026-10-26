package style

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestTextDecoration pins the shorthand: lines in any combination, a
// style, a color, any order; none clears.
func TestTextDecoration(t *testing.T) {
	s := parseOne(t, `
		a { text-decoration: underline; }
		b { text-decoration: wavy underline line-through #ff0000; }
		c { text-decoration: overline dotted; }
		d { text-decoration: none; }
	`)
	dec := func(n string) render.Decoration { return computeTree(s, el(n)).Decoration }
	if d := dec("a"); d.Lines != render.Underline || d.Style != render.DecorationSolid || d.Color != 0 {
		t.Errorf("underline: %+v", d)
	}
	if d := dec("b"); d.Lines != render.Underline|render.LineThrough || d.Style != render.DecorationWavy || d.Color != render.RGB(0xff, 0, 0) {
		t.Errorf("wavy pair: %+v", d)
	}
	if d := dec("c"); d.Lines != render.Overline || d.Style != render.DecorationDotted {
		t.Errorf("overline dotted: %+v", d)
	}
	if vd := computeTree(s, el("d")); vd.Decoration.Lines != 0 || !vd.Has(PropTextDecorationLine) {
		t.Errorf("none: %+v", vd.Decoration)
	}
	if d := dec("d"); d.Lines != 0 {
		t.Errorf("none: %+v", d)
	}
	for _, bad := range []string{"none underline", "sparkly", "underline red blue"} {
		var v Values
		if parseTextDecoration(tokenize(bad), &ctx{}, &v) {
			t.Errorf("%q parsed", bad)
		}
	}
}
