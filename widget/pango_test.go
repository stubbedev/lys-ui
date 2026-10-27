package widget

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// spanStyle parses one span and returns its run's style.
func spanStyle(t *testing.T, markup string) TextStyle {
	t.Helper()
	runs, ok := ParseMarkup(markup)
	if !ok || len(runs) != 1 {
		t.Fatalf("ParseMarkup(%q) = %v, %v", markup, runs, ok)
	}
	return runs[0].Style
}

// Each Pango span attribute parses into the run's style (#259).
func TestPangoSpanAttributes(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if st := spanStyle(t, `<span foreground="red">x</span>`); st.Color != render.RGB(0xff, 0, 0) {
		t.Errorf("foreground red = %#x", uint32(st.Color))
	}
	if st := spanStyle(t, `<span fgcolor="#00ff0080">x</span>`); st.Color != render.RGBA(0, 0xff, 0, 0x80) {
		t.Errorf("fgcolor #rrggbbaa = %#x", uint32(st.Color))
	}
	if st := spanStyle(t, `<span background="#0000ff">x</span>`); st.Background != render.RGB(0, 0, 0xff) {
		t.Errorf("background = %#x", uint32(st.Background))
	}
	if st := spanStyle(t, `<span alpha="50%">x</span>`); !near(st.Alpha, 0.5) {
		t.Errorf("alpha 50%% = %v", st.Alpha)
	}
	if st := spanStyle(t, `<span fgalpha="65535">x</span>`); !near(st.Alpha, 1) {
		t.Errorf("fgalpha 65535 = %v", st.Alpha)
	}
	if st := spanStyle(t, `<span weight="light">x</span>`); st.Weight != 300 || st.Bold {
		t.Errorf("weight light = %d bold %v", st.Weight, st.Bold)
	}
	if st := spanStyle(t, `<span weight="650">x</span>`); st.Weight != 650 || !st.Bold {
		t.Errorf("weight 650 = %d bold %v", st.Weight, st.Bold)
	}
	if st := spanStyle(t, `<span style="oblique">x</span>`); !st.Italic {
		t.Error("style oblique is not italic")
	}
	if st := spanStyle(t, `<span size="small">x</span>`); !near(st.Scale, 1/1.2) {
		t.Errorf("size small scale %v", st.Scale)
	}
	if st := spanStyle(t, `<span size="12pt">x</span>`); !near(st.SizePx, 16) {
		t.Errorf("size 12pt = %vpx, want 16", st.SizePx)
	}
	if st := spanStyle(t, `<span size="12288">x</span>`); !near(st.SizePx, 16) {
		t.Errorf("size 12288 = %vpx, want 16", st.SizePx)
	}
	if st := spanStyle(t, `<span size="150%">x</span>`); !near(st.Scale, 1.5) {
		t.Errorf("size 150%% scale %v", st.Scale)
	}
	if st := spanStyle(t, `<span font_family="Monospace">x</span>`); st.Family != "Monospace" {
		t.Errorf("font_family = %q", st.Family)
	}
	if st := spanStyle(t, `<span underline="double">x</span>`); !st.Underline || st.UnderlineStyle != render.DecorationDouble {
		t.Errorf("underline double = %v %v", st.Underline, st.UnderlineStyle)
	}
	if st := spanStyle(t, `<span strikethrough="true">x</span>`); !st.Strikethrough {
		t.Error("strikethrough true not set")
	}
	if st := spanStyle(t, `<span letter_spacing="1024">x</span>`); !near(st.LetterSpacing, pointPx) {
		t.Errorf("letter_spacing 1024 = %v", st.LetterSpacing)
	}
	if st := spanStyle(t, `<span font_features="tnum, -liga, smcp=1, dlig on">x</span>`); st.Features != "tnum=1;liga=0;smcp=1;dlig=1" {
		t.Errorf("font_features = %q", st.Features)
	}
}

// Malformed values and unknown attributes fail the parse.
func TestPangoSpanRejects(t *testing.T) {
	for _, m := range []string{
		`<span foreground="notacolor">x</span>`,
		`<span alpha="0">x</span>`,
		`<span alpha="120%">x</span>`,
		`<span weight="heavyish">x</span>`,
		`<span weight="50">x</span>`,
		`<span style="slanted">x</span>`,
		`<span size="huge">x</span>`,
		`<span size="-3pt">x</span>`,
		`<span underline="triple">x</span>`,
		`<span strikethrough="yes">x</span>`,
		`<span letter_spacing="wide">x</span>`,
		`<span font_features="ligatures">x</span>`,
		`<span font_family="">x</span>`,
		`<span gravity="south">x</span>`,
	} {
		if _, ok := ParseMarkup(m); ok {
			t.Errorf("ParseMarkup(%q) accepted it", m)
		}
	}
}

// The convenience tags set Pango's shorthands, relative sizes
// compounding as they nest.
func TestPangoConvenienceTags(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	if st := spanStyle(t, `<big><big>x</big></big>`); !near(st.Scale, 1.44) {
		t.Errorf("big big scale %v, want 1.44", st.Scale)
	}
	if st := spanStyle(t, `<small>x</small>`); !near(st.Scale, 1/1.2) {
		t.Errorf("small scale %v", st.Scale)
	}
	if st := spanStyle(t, `<tt>x</tt>`); st.Family != "monospace" {
		t.Errorf("tt family %q", st.Family)
	}
	if st := spanStyle(t, `<sup>x</sup>`); st.Shift <= 0 || st.Scale >= 1 {
		t.Errorf("sup shift %v scale %v, want raised and smaller", st.Shift, st.Scale)
	}
	if st := spanStyle(t, `<sub>x</sub>`); st.Shift >= 0 || st.Scale >= 1 {
		t.Errorf("sub shift %v scale %v, want lowered and smaller", st.Shift, st.Scale)
	}
	if st := spanStyle(t, `<b>x</b>`); !st.Bold || st.Weight != weightBold {
		t.Errorf("b = bold %v weight %d", st.Bold, st.Weight)
	}
}

// The Rust shell's drun display format parses into an italic, light,
// small run, and the label shapes it smaller than the name.
func TestPangoDrunFormat(t *testing.T) {
	runs, ok := ParseMarkup(`Alacritty [<span weight='light' size='small'><i>(Terminal)</i></span>]`)
	if !ok || len(runs) != 3 {
		t.Fatalf("drun format parsed %v, %v", runs, ok)
	}
	st := runs[1].Style
	if runs[1].Text != "(Terminal)" || !st.Italic || st.Weight != 300 || math.Abs(st.Scale-1/1.2) > 1e-9 {
		t.Errorf("generic run %+v", runs[1])
	}
	l := NewRichLabel(testFace(t), 14, `<span size='small'>(Terminal)</span>`, 0)
	full := NewRichLabel(testFace(t), 14, `(Terminal)`, 0)
	if small, big := naturalSize(l).W, naturalSize(full).W; small >= big {
		t.Errorf("a small run measures %d, not narrower than the full-size %d", small, big)
	}
}

// A run's background fills behind it and a superscript paints above
// the baseline's text.
func TestPangoRunPaint(t *testing.T) {
	face := testFace(t)
	ink := func(markup string, want func(render.Color) bool) (top int, found bool) {
		l := NewRichLabel(face, 20, markup, render.RGB(0xff, 0xff, 0xff))
		buf := paintPixels(t, l, 120, 40)
		for y := range 40 {
			for x := range 120 {
				if want(pxAt(buf, 120, x, y)) {
					return y, true
				}
			}
		}
		return 0, false
	}
	blue := func(c render.Color) bool { return c.B() > 200 && c.R() < 40 && c.A() == 0xff }
	if _, ok := ink(`<span background="#0000ff">x</span>`, blue); !ok {
		t.Error("no background painted behind the run")
	}
	any := func(c render.Color) bool { return c.A() > 0x80 }
	plain, _ := ink(`x`, any)
	sup, _ := ink(`x<sup>x</sup>`, any)
	if sup >= plain {
		t.Errorf("superscript ink starts at row %d, not above the plain text's %d", sup, plain)
	}
}
