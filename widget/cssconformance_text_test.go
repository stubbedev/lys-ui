package widget

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// The Text group's fixture metrics. The suite's labels shape the
// bundled Cantarell face at 16px; like WPT's Ahem, the face is test
// input, and the expectations below are derived from these advances
// (pinned by the "fixture face metrics" case): x is 8.1875px, a space
// 4.375px, the ellipsis 13.4375px. So "xxxxx" is 40.9375px, "xxxxx
// xxxxx" 86.25px, "xxxxxx" 49.125px and "xxxxx…" 54.375px.
const txtLine = 20

// txtLabel is id's Label.
func txtLabel(f *confFixture, id string) *Label {
	f.t.Helper()
	l, ok := f.get(id).(*Label)
	if !ok {
		f.t.Fatalf("#%s is a %T, not a Label", id, f.get(id))
	}
	return l
}

// txtAdvance is the fixture face's advance of s at the suite's size.
func txtAdvance(f *confFixture, s string) float64 {
	return f.face.Shape(s, confFaceSize).Advance()
}

// txtRows expects id's laid-out rows at its content width to be rows.
func txtRows(id string, rows ...string) confCheck {
	return wantProbe("rows of #"+id, func(f *confFixture) any {
		l := txtLabel(f, id)
		return strings.Join(l.wrapped(float64(boundsOf(l).W)), "|")
	}, strings.Join(rows, "|"))
}

// txtWidthOf expects id's border box to be as wide as s's rounded-up
// advance, the label's natural width for that text.
func txtWidthOf(id, s string) confCheck {
	return wantProbe(fmt.Sprintf("width of #%s against the advance of %q", id, s), func(f *confFixture) any {
		return boundsOf(f.get(id)).W - int(math.Ceil(txtAdvance(f, s)))
	}, 0)
}

// txtInkSpan is the horizontal ink extent of a frame's band [y0, y1):
// the first and last columns holding a non-white pixel, -1 when none.
func txtInkSpan(data []byte, stride, w, y0, y1 int) (int, int) {
	lo, hi := -1, -1
	for y := max(y0, 0); y < y1 && y*stride < len(data); y++ {
		for x := range w {
			c := render.ColorFromBytes(data[y*stride+x*4:])
			if c.R() < 250 || c.G() < 250 || c.B() < 250 {
				if lo < 0 || x < lo {
					lo = x
				}
				hi = max(hi, x)
			}
		}
	}
	return lo, hi
}

// txtRow checks one painted row against a reference rendering: row's
// text drawn with the fixture face at the spec's pen position (from
// the content box's left edge), plus gap px after every space when
// justified. The ink spans must agree within a pixel.
type txtRow struct {
	id       string
	row      int
	text     string
	pen, gap float64
}

// txtRowAt expects row to paint text with its pen at pen.
func txtRowAt(id string, row int, text string, pen float64) confCheck {
	return txtRow{id: id, row: row, text: text, pen: pen}
}

// txtJustified expects row to paint text from pen with gap px of
// justification after each space.
func txtJustified(id string, row int, text string, pen, gap float64) confCheck {
	return txtRow{id: id, row: row, text: text, pen: pen, gap: gap}
}

func (c txtRow) what() string {
	return fmt.Sprintf("ink of #%s row %d (%q at pen %.2f)", c.id, c.row, c.text, c.pen)
}

func (c txtRow) probe(f *confFixture) (string, string, bool) {
	b := boundsOf(f.get(c.id))
	stride := render.Stride(f.vp.W)
	y0 := b.Y + c.row*txtLine
	lo, hi := txtInkSpan(f.frame, stride, f.vp.W, y0, y0+txtLine)
	ref := make([]byte, stride*2*txtLine)
	cv := render.New(ref, stride, f.vp.W, 2*txtLine)
	cv.Clear(cv.Rect(), render.RGB(0xff, 0xff, 0xff))
	sh := f.face.Shape(c.text, confFaceSize)
	pen := b.X + int(math.Round(c.pen))
	if c.gap > 0 {
		sh.DrawJustified(cv, pen, txtLine, render.RGB(0, 0, 0), c.gap)
	} else {
		sh.Draw(cv, pen, txtLine, render.RGB(0, 0, 0))
	}
	rlo, rhi := txtInkSpan(ref, stride, f.vp.W, 0, 2*txtLine)
	ok := lo >= 0 && abs(lo-rlo) <= 1 && abs(hi-rhi) <= 1
	return fmt.Sprintf("[%d,%d]", lo, hi), fmt.Sprintf("[%d,%d]", rlo, rhi), ok
}

// txtBaselineShift expects id's baseline to sit shift px above ref's,
// two labels laid out alike but for vertical-align (Baseline reports
// it from the label's own top).
func txtBaselineShift(id, ref string, shift int) confCheck {
	return wantProbe(fmt.Sprintf("baseline of #%s above #%s", id, ref), func(f *confFixture) any {
		a, _ := txtLabel(f, id).Baseline()
		b, _ := txtLabel(f, ref).Baseline()
		return b - a
	}, shift)
}

// txtShadows is a computed shadow list of layers.
func txtShadows(layers ...style.Shadow) style.Shadows {
	var s style.Shadows
	s.N = copy(s.Layers[:], layers)
	return s
}

// The Text group's computed values.
var (
	cpTxtFeatures      = confProp{"the font feature list", func(v *style.Values) any { return faceFeatureSpec(v) }}
	cpTxtLetterSpacing = confProp{"letter-spacing", func(v *style.Values) any { return v.LetterSpacing }}
	cpTxtWordSpacing   = confProp{"word-spacing", func(v *style.Values) any { return v.WordSpacing }}
	cpTxtLineHeight    = confProp{"line-height", func(v *style.Values) any { return v.LineHeight }}
	cpTxtItalic        = confProp{"font-style italic", func(v *style.Values) any { return v.Italic }}
	cpTxtFamily        = confProp{"font-family", func(v *style.Values) any { return v.FontFamily }}
	cpTxtDecoration    = confProp{"text-decoration", func(v *style.Values) any { return cssDecoration(v) }}
	cpTxtSynthesis     = confProp{"font-synthesis", func(v *style.Values) any { return v.Synthesis }}
	cpTxtOpsz          = confProp{"the opsz axis", func(v *style.Values) any { return fmt.Sprint(cascadeVariations(v, 0, int(v.FontSize))) }}
	cpTxtUnicodeBidi   = confProp{"unicode-bidi", func(v *style.Values) any { return v.UnicodeBidi }}
	cpTxtWritingMode   = confProp{"writing-mode", func(v *style.Values) any { return v.WritingMode }}
	cpTxtVerticalAlign = confProp{"vertical-align", func(v *style.Values) any { return v.VerticalAlign }}
	cpTxtIndent        = confProp{"text-indent", func(v *style.Values) any { return v.TextIndent }}
	cpTxtLineClamp     = confProp{"line-clamp", func(v *style.Values) any { return v.LineClamp }}
	cpTxtShadow        = confProp{"text-shadow", func(v *style.Values) any { return v.TextShadow }}
	cpTxtPalette       = confProp{"font-palette", func(v *style.Values) any { return v.Palette }}
)

// txtAligned is a single-line "xxxxx" in a 200px box under css.
func txtAligned(name, wpt, css string, pen float64) confCase {
	return confCase{
		name: name, wpt: wpt, spec: "css-text-3 §7.1",
		css:  `#t { width: 200px; line-height: 20px; ` + css + ` }`,
		tree: nText("#t", "xxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 200, txtLine), txtRowAt("t", 0, "xxxxx", pen)},
	}
}

// confText is the #159 Text group: alignment and indentation, white
// space, breaking and hyphenation, spacing, vertical-align, clamping
// and overflow, the font property families, decoration and shadow,
// and direction.
var confText = []confCase{
	{
		name: "fixture face metrics",
		spec: "css-text-3 (fixture input: the face's advances)",
		tree: nText("#t", "x"),
		want: []confCheck{
			wantProbe("advance of x", func(f *confFixture) any { return txtAdvance(f, "x") }, 8.1875),
			wantProbe("advance of a space", func(f *confFixture) any { return txtAdvance(f, " ") }, 4.375),
			wantProbe("advance of the ellipsis", func(f *confFixture) any { return txtAdvance(f, "…") }, 13.4375),
			wantProbe("advance of xxxxx xxxxx", func(f *confFixture) any { return txtAdvance(f, "xxxxx xxxxx") }, 86.25),
		},
	},
	txtAligned("text-align left", "", `text-align: left;`, 0),
	txtAligned("text-align right", "", `text-align: right;`, 159.0625),
	txtAligned("text-align center", "", `text-align: center;`, 79.53125),
	txtAligned("text-align start ltr", "", `text-align: start;`, 0),
	txtAligned("text-align end ltr", "", `text-align: end;`, 159.0625),
	txtAligned("text-align start rtl", "", `direction: rtl; text-align: start;`, 159.0625),
	txtAligned("text-align end rtl", "", `direction: rtl; text-align: end;`, 0),
	txtAligned("text-align left rtl", "", `direction: rtl; text-align: left;`, 0),
	txtAligned("text-align right rtl", "", `direction: rtl; text-align: right;`, 159.0625),
	{
		name: "text-align justify spreads all but the last row",
		wpt:  "css/css-text/text-align/text-align-justify-001.html",
		spec: "css-text-3 §7.1, §7.3",
		css:  `#t { width: 100px; line-height: 20px; text-align: justify; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx"),
		want: []confCheck{
			wantRect("t", 0, 0, 100, 2*txtLine),
			txtRows("t", "xxxxx xxxxx", "xxxxx"),
			txtJustified("t", 0, "xxxxx xxxxx", 0, 13.75),
			txtRowAt("t", 1, "xxxxx", 0),
		},
	},
	{
		name: "direction inherits into the start edge",
		spec: "css-writing-modes-4 §2.1",
		css:  `#p { direction: rtl; } #t { width: 200px; line-height: 20px; text-align: start; }`,
		tree: nCol("#p", nText("#t", "xxxxx")),
		want: []confCheck{txtRowAt("t", 0, "xxxxx", 159.0625)},
	},
	{
		name: "text-indent shortens the first line",
		spec: "css-text-3 §8.1",
		css:  `#t { width: 100px; line-height: 20px; text-indent: 20px; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx"),
		want: []confCheck{
			wantRect("t", 0, 0, 100, 2*txtLine),
			txtRows("t", "xxxxx", "xxxxx xxxxx"),
			txtRowAt("t", 0, "xxxxx", 20),
			txtRowAt("t", 1, "xxxxx xxxxx", 0),
		},
	},
	{
		name: "text-indent on a single line",
		spec: "css-text-3 §8.1",
		css:  `#t { width: 200px; line-height: 20px; text-indent: 30px; }`,
		tree: nText("#t", "xxxxx"),
		want: []confCheck{txtRowAt("t", 0, "xxxxx", 30)},
	},
	{
		name: "text-indent from the right in rtl",
		spec: "css-text-3 §8.1",
		css:  `#t { width: 200px; line-height: 20px; text-indent: 30px; direction: rtl; }`,
		tree: nText("#t", "xxxxx"),
		want: []confCheck{txtRowAt("t", 0, "xxxxx", 129.0625)},
	},
	{
		name: "white-space normal collapses spaces",
		spec: "css-text-3 §4.1.1",
		css:  `#t { white-space: normal; line-height: 20px; }`,
		tree: nBox("#r", nText("#t", "xxxxx    xxxxx")),
		want: []confCheck{txtWidthOf("t", "xxxxx xxxxx"), txtRowAt("t", 0, "xxxxx xxxxx", 0)},
	},
	{
		name: "white-space normal turns a newline into a space",
		spec: "css-text-3 §4.1.2",
		css:  `#t { white-space: normal; width: 200px; line-height: 20px; }`,
		tree: nText("#t", "xxxxx\nxxxxx"),
		want: []confCheck{
			wantRect("t", 0, 0, 200, txtLine),
			txtRows("t", "xxxxx xxxxx"),
			txtRowAt("t", 0, "xxxxx xxxxx", 0),
		},
	},
	{
		name: "white-space nowrap keeps one line",
		spec: "css-text-3 §3",
		css:  `#t { white-space: nowrap; width: 100px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx"),
		want: []confCheck{
			wantRect("t", 0, 0, 100, txtLine),
			txtRows("t", "xxxxx xxxxx xxxxx"),
		},
	},
	{
		name: "white-space nowrap collapses a newline",
		spec: "css-text-3 §4.1.2",
		css:  `#t { white-space: nowrap; line-height: 20px; }`,
		tree: nBox("#r", nText("#t", "xxxxx\nxxxxx")),
		want: []confCheck{txtWidthOf("t", "xxxxx xxxxx"), txtRowAt("t", 0, "xxxxx xxxxx", 0)},
	},
	{
		name: "white-space pre keeps spaces and newlines",
		spec: "css-text-3 §3, §4.1",
		css:  `#t { white-space: pre; line-height: 20px; }`,
		tree: nBox("#r", nText("#t", "xxxxx  xxxxx\nxxxxx")),
		want: []confCheck{
			txtWidthOf("t", "xxxxx  xxxxx"),
			wantProbe("height of #t", func(f *confFixture) any { return boundsOf(f.get("t")).H }, 2*txtLine),
			txtRows("t", "xxxxx  xxxxx", "xxxxx"),
			txtRowAt("t", 0, "xxxxx  xxxxx", 0),
			txtRowAt("t", 1, "xxxxx", 0),
		},
	},
	{
		name: "white-space pre does not wrap",
		spec: "css-text-3 §3",
		css:  `#t { white-space: pre; width: 100px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 100, txtLine), txtRows("t", "xxxxx xxxxx xxxxx")},
	},
	{
		name: "white-space pre-wrap hangs preserved spaces",
		wpt:  "css/css-text/white-space/pre-wrap-001.html",
		spec: "css-text-3 §4.1.3",
		css:  `#t { white-space: pre-wrap; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx   xxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, 2*txtLine), txtRows("t", "xxxxx   ", "xxxxx")},
	},
	{
		name: "white-space pre-wrap keeps a newline",
		spec: "css-text-3 §4.1",
		css:  `#t { white-space: pre-wrap; width: 200px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx\nxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 200, 2*txtLine), txtRows("t", "xxxxx", "xxxxx")},
	},
	{
		name: "white-space break-spaces wraps between spaces",
		spec: "css-text-3 §3, §5.1",
		css:  `#t { white-space: break-spaces; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx   xxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, 2*txtLine), txtRows("t", "xxxxx  ", " xxxxx")},
	},
	{
		name: "white-space pre-line collapses spaces and keeps newlines",
		spec: "css-text-3 §4.1.1",
		css:  `#t { white-space: pre-line; width: 200px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx    xxxxx\nxxxxx"),
		want: []confCheck{
			wantRect("t", 0, 0, 200, 2*txtLine),
			txtRows("t", "xxxxx xxxxx", "xxxxx"),
			txtRowAt("t", 0, "xxxxx xxxxx", 0),
		},
	},
	{
		name: "word-break break-all breaks inside words",
		wpt:  "css/css-text/word-break/word-break-break-all-001.html",
		spec: "css-text-3 §5.2",
		css:  `#t { word-break: break-all; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxx xxxxxxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, 3*txtLine), txtRows("t", "xxx xx", "xxxxxx", "xx")},
	},
	{
		name: "word-break keep-all leaves Latin breaking alone",
		wpt:  "css/css-text/word-break/word-break-keep-all-001.html",
		spec: "css-text-3 §5.2",
		css:  `#t { word-break: keep-all; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx xxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, 2*txtLine), txtRows("t", "xxxxx", "xxxxx")},
	},
	{
		name: "line-break anywhere breaks at every character",
		wpt:  "css/css-text/line-break/line-break-anywhere-001.html",
		spec: "css-text-3 §5.3",
		css:  `#t { line-break: anywhere; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxxxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, 2*txtLine), txtRows("t", "xxxxxx", "xxxx")},
	},
	{
		name: "overflow-wrap anywhere breaks an overlong word",
		wpt:  "css/css-text/overflow-wrap/overflow-wrap-anywhere-001.html",
		spec: "css-text-3 §5.5",
		css:  `#t { overflow-wrap: anywhere; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxx xxxxxxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, 3*txtLine), txtRows("t", "xxx", "xxxxxx", "xxxx")},
	},
	{
		name: "overflow-wrap break-word breaks an overlong word",
		wpt:  "css/css-text/overflow-wrap/overflow-wrap-break-word-001.html",
		spec: "css-text-3 §5.5",
		css:  `#t { overflow-wrap: break-word; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxx xxxxxxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, 3*txtLine), txtRows("t", "xxx", "xxxxxx", "xxxx")},
	},
	{
		name: "word-wrap is the overflow-wrap alias",
		spec: "css-text-3 §5.5",
		css:  `#t { word-wrap: break-word; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxxxxxxx"),
		want: []confCheck{txtRows("t", "xxxxxx", "xxxx")},
	},
	{
		name: "overflow-wrap normal lets a word overflow",
		spec: "css-text-3 §5.5",
		css:  `#t { overflow-wrap: normal; width: 50px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxxxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 50, txtLine), txtRows("t", "xxxxxxxxxx")},
	},
	{
		name: "overflow-wrap anywhere lowers the min-content width",
		spec: "css-text-3 §5.5",
		css: `#c, #d { display: flex; width: 30px; } #t { line-height: 20px; overflow-wrap: anywhere; }
			#u { line-height: 20px; overflow-wrap: break-word; }`,
		tree: nCol("#col", nBox("#c", nWrap("#t", "xxxxxxxxxx")), nBox("#d", nWrap("#u", "xxxxxxxxxx"))),
		want: []confCheck{
			wantProbe("anywhere shrinks to the flex container", func(f *confFixture) any { return boundsOf(f.get("t")).W }, 30),
			wantProbe("break-word keeps the word's width", func(f *confFixture) any { return boundsOf(f.get("u")).W }, 82),
		},
	},
	{
		name: "hyphens manual breaks at a soft hyphen",
		wpt:  "css/css-text/hyphens/hyphens-manual-001.html",
		spec: "css-text-3 §6.1",
		css:  `#t { hyphens: manual; width: 60px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx\u00adxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 60, 2*txtLine), txtRows("t", "xxxxx-", "xxxxx")},
	},
	{
		name: "hyphens manual hides an unbroken soft hyphen",
		spec: "css-text-3 §6.1",
		css:  `#t { hyphens: manual; width: 200px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx\u00adxxxxx"),
		want: []confCheck{txtRows("t", "xxxxxxxxxx"), txtRowAt("t", 0, "xxxxxxxxxx", 0)},
	},
	{
		name: "hyphens none never breaks at a soft hyphen",
		wpt:  "css/css-text/hyphens/hyphens-none-001.html",
		spec: "css-text-3 §6.1",
		css:  `#t { hyphens: none; width: 60px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx\u00adxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 60, txtLine), txtRows("t", "xxxxxxxxxx")},
	},
	{
		name: "word-spacing widens each space",
		spec: "css-text-3 §8.3",
		css:  `#t { word-spacing: 10px; line-height: 20px; }`,
		tree: nBox("#r", nText("#t", "xxxxx xxxxx")),
		want: []confCheck{
			wantComputed("t", cpTxtWordSpacing, 10.0),
			wantProbe("width of #t", func(f *confFixture) any { return boundsOf(f.get("t")).W }, 97),
		},
	},
	{
		name: "word-spacing and letter-spacing em resolve at the font size",
		spec: "css-text-3 §8.2, §8.3",
		css:  `#t { font-size: 20px; word-spacing: 0.5em; letter-spacing: 0.25em; }`,
		tree: nText("#t", "x"),
		want: []confCheck{
			wantComputed("t", cpTxtWordSpacing, 10.0),
			wantComputed("t", cpTxtLetterSpacing, 5.0),
		},
	},
	{
		name: "vertical-align length raises the baseline",
		spec: "css-inline-3 §4.2 (CSS 2 §10.8.1)",
		css:  `#a, #b { line-height: 20px; } #a { vertical-align: 4px; }`,
		tree: nCol("#c", nText("#a", "x"), nText("#b", "x")),
		want: []confCheck{txtBaselineShift("a", "b", 4)},
	},
	{
		name: "vertical-align percentage of the line height",
		spec: "css-inline-3 §4.2 (CSS 2 §10.8.1)",
		css:  `#a, #b { line-height: 20px; } #a { vertical-align: 50%; }`,
		tree: nCol("#c", nText("#a", "x"), nText("#b", "x")),
		want: []confCheck{txtBaselineShift("a", "b", 10)},
	},
	{
		name: "vertical-align keywords compute",
		spec: "css-inline-3 §4.2",
		css:  `#a { vertical-align: super; } #b { vertical-align: sub; } #c { vertical-align: middle; }`,
		tree: nCol("#p", nText("#a", "x"), nText("#b", "x"), nText("#c", "x")),
		want: []confCheck{
			wantComputed("a", cpTxtVerticalAlign, style.VerticalAlignSuper),
			wantComputed("b", cpTxtVerticalAlign, style.VerticalAlignSub),
			wantComputed("c", cpTxtVerticalAlign, style.VerticalAlignMiddle),
		},
	},
	{
		name: "line-clamp caps the rows and ellipsizes the last",
		spec: "css-overflow-4 §4.1",
		css:  `#t { line-clamp: 2; width: 100px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx xxxxx xxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 100, 2*txtLine), txtRows("t", "xxxxx xxxxx", "xxxxx xxxxx…")},
	},
	{
		name: "-webkit-line-clamp is the legacy alias",
		wpt:  "css/css-overflow/webkit-line-clamp-001.html",
		spec: "css-overflow-4 §4.2",
		css:  `#t { -webkit-line-clamp: 1; width: 100px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx"),
		want: []confCheck{
			wantComputed("t", cpTxtLineClamp, 1),
			wantRect("t", 0, 0, 100, txtLine),
			txtRows("t", "xxxxx xxxxx…"),
		},
	},
	{
		name: "line-clamp none keeps every row",
		spec: "css-overflow-4 §4.1",
		css:  `#t { line-clamp: none; width: 100px; line-height: 20px; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx xxxxx xxxxx"),
		want: []confCheck{wantComputed("t", cpTxtLineClamp, 0), wantRect("t", 0, 0, 100, 3*txtLine)},
	},
	{
		name: "text-overflow ellipsis on a clipped line",
		wpt:  "css/css-ui/text-overflow-001.html",
		spec: "css-overflow-3 §3.1 (text-overflow)",
		css:  `#t { width: 60px; line-height: 20px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }`,
		tree: nText("#t", "xxxxxxxxxxxxxxxxxxxx"),
		want: []confCheck{wantRect("t", 0, 0, 60, txtLine), txtRowAt("t", 0, "xxxxx…", 0)},
	},
	{
		name: "text-overflow clip cuts at the content edge",
		spec: "css-overflow-3 §3.1 (text-overflow)",
		css:  `#t { width: 60px; line-height: 20px; overflow: hidden; white-space: nowrap; text-overflow: clip; }`,
		tree: nText("#t", "xxxxxxxxxxxxxxxxxxxx"),
		want: []confCheck{
			wantProbe("ink right edge of #t", func(f *confFixture) any {
				_, hi := txtInkSpan(f.frame, render.Stride(f.vp.W), f.vp.W, 0, txtLine)
				return hi
			}, 59),
		},
	},
	{
		name: "font-size keywords and relative sizes",
		spec: "css-fonts-4 §2.5",
		css: `#a { font-size: xx-large; } #b { font-size: large; } #c { font-size: xxx-large; }
			#p { font-size: 10px; } #d { font-size: larger; } #e { font-size: 2em; }
			#g { font-size: 150%; } #h { font-size: 1.5rem; }`,
		tree: nCol("#col", nText("#a", "x"), nText("#b", "x"), nText("#c", "x"),
			nCol("#p", nText("#d", "x"), nText("#e", "x"), nText("#g", "x"), nText("#h", "x"))),
		want: []confCheck{
			wantComputed("a", cpFontSize, 32.0),
			wantComputed("b", cpFontSize, 19.2),
			wantComputed("c", cpFontSize, 48.0),
			wantComputed("d", cpFontSize, 12.0),
			wantComputed("e", cpFontSize, 20.0),
			wantComputed("g", cpFontSize, 15.0),
			wantComputed("h", cpFontSize, 24.0),
		},
	},
	{
		name: "font-weight bolder and lighter follow the relative-weight table",
		wpt:  "css/css-fonts/font-weight-bolder-001.xht",
		spec: "css-fonts-4 §2.2 (relative weights)",
		css: `#w100 { font-weight: 100; } #w300 { font-weight: 300; } #w500 { font-weight: 500; }
			#w600 { font-weight: 600; } #w800 { font-weight: 800; }
			.bolder { font-weight: bolder; } .lighter { font-weight: lighter; }`,
		tree: nCol("#col",
			nCol("#w100", nText("#b100.bolder", "x"), nText("#l100.lighter", "x")),
			nCol("#w300", nText("#b300.bolder", "x"), nText("#l300.lighter", "x")),
			nCol("#w500", nText("#b500.bolder", "x"), nText("#l500.lighter", "x")),
			nCol("#w600", nText("#b600.bolder", "x"), nText("#l600.lighter", "x")),
			nCol("#w800", nText("#b800.bolder", "x"), nText("#l800.lighter", "x"))),
		want: []confCheck{
			wantComputed("b100", cpFontWeight, 400), wantComputed("l100", cpFontWeight, 100),
			wantComputed("b300", cpFontWeight, 400), wantComputed("l300", cpFontWeight, 100),
			wantComputed("b500", cpFontWeight, 700), wantComputed("l500", cpFontWeight, 100),
			wantComputed("b600", cpFontWeight, 900), wantComputed("l600", cpFontWeight, 400),
			wantComputed("b800", cpFontWeight, 900), wantComputed("l800", cpFontWeight, 700),
		},
	},
	{
		name: "font shorthand with a line height",
		spec: "css-fonts-4 §2.7",
		css:  `#t { font: italic 700 20px/30px "Fixture Sans", serif; }`,
		tree: nText("#t", "x"),
		want: []confCheck{
			wantComputed("t", cpTxtItalic, true),
			wantComputed("t", cpFontWeight, 700),
			wantComputed("t", cpFontSize, 20.0),
			wantComputed("t", cpTxtLineHeight, 30.0),
			wantComputed("t", cpTxtFamily, "Fixture Sans"),
		},
	},
	{
		name: "font shorthand resets the line height",
		spec: "css-fonts-4 §2.7",
		css:  `#t { line-height: 40px; font: 20px serif; }`,
		tree: nText("#t", "x"),
		want: []confCheck{wantComputed("t", cpTxtLineHeight, 0.0)},
	},
	{
		name: "a line-height number inherits as the number",
		spec: "css-inline-3 §5.1 (line-height: computed value)",
		css:  `#c { line-height: 1.5; font-size: 10px; } #big, #pctbig { font-size: 20px; } #pct { line-height: 150%; font-size: 10px; }`,
		tree: nCol("#c", nText("#big", "x"), nText("#small", "x"), nCol("#pct", nText("#pctbig", "x"))),
		want: []confCheck{
			wantComputed("big", cpTxtLineHeight, 30.0),
			wantComputed("small", cpTxtLineHeight, 15.0),
			wantComputed("pctbig", cpTxtLineHeight, 15.0),
		},
	},
	{
		name: "line-height number and percentage",
		spec: "css-inline-3 §5.1 (line-height)",
		css:  `#a { line-height: 1.5; } #b { line-height: 150%; font-size: 20px; }`,
		tree: nCol("#c", nText("#a", "x"), nText("#b", "x")),
		want: []confCheck{
			wantComputed("a", cpTxtLineHeight, 24.0),
			wantComputed("b", cpTxtLineHeight, 30.0),
			wantProbe("height of #a", func(f *confFixture) any { return boundsOf(f.get("a")).H }, 24),
		},
	},
	{
		name: "wrapped rows paint under a line height below the glyphs",
		spec: "css-inline-3 §5.1 (negative half-leading)",
		css:  `#t { width: 100px; line-height: 10px; }`,
		tree: nWrap("#t", "xxxxx xxxxx xxxxx"),
		want: []confCheck{
			wantRect("t", 0, 0, 100, 20),
			wantProbe("first row has ink", func(f *confFixture) any {
				lo, _ := txtInkSpan(f.frame, render.Stride(f.vp.W), f.vp.W, 0, 10)
				return lo >= 0
			}, true),
		},
	},
	{
		name: "text-transform uppercase and capitalize",
		spec: "css-text-3 §2.1",
		css:  `#a { text-transform: uppercase; } #b { text-transform: capitalize; }`,
		tree: nCol("#c", nBox("#ra", nText("#a", "xxxxx")), nBox("#rb", nText("#b", "ab cd"))),
		want: []confCheck{txtWidthOf("a", "XXXXX"), txtWidthOf("b", "Ab Cd")},
	},
	{
		name: "text-decoration shorthand",
		spec: "css-text-decor-3 §2.4",
		css:  `#t { text-decoration: underline wavy #ff0000; }`,
		tree: nText("#t", "x"),
		want: []confCheck{wantComputed("t", cpTxtDecoration,
			render.Decoration{Lines: render.Underline, Style: render.DecorationWavy, Color: 0xffff0000})},
	},
	{
		name: "text-decoration longhands",
		spec: "css-text-decor-3 §2.1–2.3",
		css: `#t { text-decoration-line: underline line-through; text-decoration-style: dotted;
			text-decoration-color: #0000ff; }`,
		tree: nText("#t", "x"),
		want: []confCheck{wantComputed("t", cpTxtDecoration,
			render.Decoration{Lines: render.Underline | render.LineThrough, Style: render.DecorationDotted, Color: 0xff0000ff})},
	},
	{
		name: "text-shadow paints an offset copy",
		spec: "css-text-decor-3 §4",
		css:  `#t { line-height: 20px; color: #000000; text-shadow: 30px 0 #ff0000; }`,
		tree: nText("#t", "x"),
		want: []confCheck{
			wantComputed("t", cpTxtShadow, txtShadows(style.Shadow{X: 30, Color: 0xffff0000})),
			wantProbe("red shadow ink right of the text", func(f *confFixture) any {
				for y := range txtLine {
					for x := 30; x < 40; x++ {
						if c := f.pixel(x, y); c.R() > 200 && c.G() < 80 && c.B() < 80 {
							return true
						}
					}
				}
				return false
			}, true),
		},
	},
	{
		name: "text-shadow inherits",
		spec: "css-text-decor-3 §4",
		css:  `#p { text-shadow: 1px 2px 3px #00ff00; }`,
		tree: nCol("#p", nText("#t", "x")),
		want: []confCheck{wantComputed("t", cpTxtShadow, txtShadows(style.Shadow{X: 1, Y: 2, Blur: 3, Color: 0xff00ff00}))},
	},
	{
		name: "font-variant-caps small-caps",
		spec: "css-fonts-4 §6.3",
		css:  `#t { font-variant-caps: small-caps; }`,
		tree: nText("#t", "x"),
		want: []confCheck{wantComputed("t", cpTxtFeatures, "smcp=1")},
	},
	{
		name: "font-variant-numeric lists",
		spec: "css-fonts-4 §6.7",
		css:  `#t { font-variant-numeric: tabular-nums slashed-zero; }`,
		tree: nText("#t", "x"),
		want: []confCheck{wantComputed("t", cpTxtFeatures, "tnum=1;zero=1")},
	},
	{
		name: "font-variant-ligatures, east-asian, position and alternates",
		spec: "css-fonts-4 §6.4–6.8",
		css: `#a { font-variant-ligatures: discretionary-ligatures; } #b { font-variant-east-asian: jis04 full-width; }
			#c { font-variant-position: super; } #d { font-variant-alternates: historical-forms; }`,
		tree: nCol("#p", nText("#a", "x"), nText("#b", "x"), nText("#c", "x"), nText("#d", "x")),
		want: []confCheck{
			wantComputed("a", cpTxtFeatures, "dlig=1"),
			wantComputed("b", cpTxtFeatures, "fwid=1;jp04=1"),
			wantComputed("c", cpTxtFeatures, "sups=1"),
			wantComputed("d", cpTxtFeatures, "hist=1"),
		},
	},
	{
		name: "font-variant shorthand",
		spec: "css-fonts-4 §6.11",
		css:  `#t { font-variant: small-caps oldstyle-nums; }`,
		tree: nText("#t", "x"),
		want: []confCheck{wantComputed("t", cpTxtFeatures, "onum=1;smcp=1")},
	},
	{
		name: "font-feature-settings wins over font-variant per tag",
		spec: "css-fonts-4 §7.2 (feature precedence)",
		css:  `#t { font-variant-numeric: tabular-nums; font-feature-settings: "tnum" 0, "kern" 1; }`,
		tree: nText("#t", "x"),
		want: []confCheck{wantComputed("t", cpTxtFeatures, "kern=1;tnum=0")},
	},
	{
		name: "font-synthesis computes and inherits",
		spec: "css-fonts-4 §6.2 (font-synthesis: inherited)",
		css:  `#p { font-synthesis: weight style; } #q { font-synthesis: none; }`,
		tree: nCol("#c", nCol("#p", nText("#a", "x")), nText("#q", "x")),
		want: []confCheck{
			wantComputed("a", cpTxtSynthesis, style.SynthWeight|style.SynthStyle),
			wantComputed("q", cpTxtSynthesis, style.FontSynthesis(0)),
		},
	},
	{
		name: "font-optical-sizing feeds the opsz axis",
		spec: "css-fonts-4 §7.4",
		css:  `#a { font-size: 16px; font-optical-sizing: auto; } #b { font-size: 16px; font-optical-sizing: none; }`,
		tree: nCol("#c", nText("#a", "x"), nText("#b", "x")),
		want: []confCheck{
			wantComputed("a", cpTxtOpsz, "[{opsz 12}]"),
			wantComputed("b", cpTxtOpsz, "[]"),
		},
	},
	{
		name: "font-palette computes and inherits",
		spec: "css-fonts-4 §9.1",
		css:  `#p { font-palette: dark; } #q { font-palette: light; }`,
		tree: nCol("#c", nCol("#p", nText("#a", "x")), nText("#q", "x"), nText("#n", "x")),
		want: []confCheck{
			wantComputed("a", cpTxtPalette, style.PaletteDark),
			wantComputed("q", cpTxtPalette, style.PaletteLight),
			wantComputed("n", cpTxtPalette, style.PaletteNormal),
		},
	},
	{
		name: "unicode-bidi computes and does not inherit",
		spec: "css-writing-modes-4 §2.2",
		css:  `#p { unicode-bidi: bidi-override; }`,
		tree: nCol("#p", nText("#t", "x")),
		want: []confCheck{
			wantComputed("p", cpTxtUnicodeBidi, style.BidiOverride),
			wantComputed("t", cpTxtUnicodeBidi, style.BidiNormal),
		},
	},
	{
		name: "unicode-bidi bidi-override forces the base direction",
		spec: "css-writing-modes-4 §2.2",
		css:  `#t { width: 200px; line-height: 20px; direction: rtl; unicode-bidi: bidi-override; text-align: start; }`,
		tree: nText("#t", "xxxxx"),
		want: []confCheck{txtRowAt("t", 0, "xxxxx", 159.0625)},
	},
	{
		name: "writing-mode computes and inherits",
		spec: "css-writing-modes-4 §3.1",
		css:  `#p { writing-mode: vertical-rl; }`,
		tree: nCol("#p", nText("#t", "x")),
		want: []confCheck{wantComputed("t", cpTxtWritingMode, style.WritingVerticalRL)},
	},
}

// confExceptionsText are the Text group's documented deviations.
var confExceptionsText = []confException{
	{
		name:   "hyphens auto breaks only at soft hyphens",
		spec:   "css-text-3 §6.1",
		css:    `#t { hyphens: auto; width: 50px; line-height: 20px; }`,
		tree:   nWrap("#t", "hyphenation"),
		want:   []confCheck{txtRows("t", "hyphenation")},
		reason: "no hyphenation dictionary ships, so auto has no hyphenation opportunities beyond the soft hyphens manual honors",
		doc:    "`hyphens: auto` (no hyphenation dictionary ships; without one, auto would guess)",
	},
	{
		name:   "writing-mode vertical-rl lays out horizontally",
		spec:   "css-writing-modes-4 §3.1",
		css:    `#t { writing-mode: vertical-rl; line-height: 20px; }`,
		tree:   nBox("#r", nText("#t", "xxxxx")),
		want:   []confCheck{txtWidthOf("t", "xxxxx"), wantProbe("height of #t", func(f *confFixture) any { return boundsOf(f.get("t")).H }, txtLine)},
		reason: "the paint stack, caret math, wrapping and ellipsizing are horizontal; vertical modes parse and compute only",
		doc:    "`writing-mode`'s vertical forms (vertical-rl/vertical-lr parse and compute but paint horizontally",
	},
	{
		name:   "text-indent percentages are invalid",
		wpt:    "css/css-text/text-indent/text-indent-percentage-001.xht",
		spec:   "css-text-3 §8.1",
		css:    `#t { text-indent: 10px; } #t { text-indent: 10%; }`,
		tree:   nText("#t", "x"),
		want:   []confCheck{wantComputed("t", cpTxtIndent, 10.0)},
		reason: "a percentage resolves against the containing block, which the per-declaration parse cache cannot see; the declaration is dropped",
		doc:    "`word-spacing` and `text-indent` percentages",
	},
	{
		name:   "text-overflow ellipsizes without overflow hidden",
		spec:   "css-overflow-3 §3.1 (text-overflow applies to overflow other than visible)",
		css:    `#t { width: 60px; line-height: 20px; white-space: nowrap; text-overflow: ellipsis; }`,
		tree:   nText("#t", "xxxxxxxxxxxxxxxxxxxx"),
		want:   []confCheck{txtRowAt("t", 0, "xxxxx…", 0)},
		reason: "a toolkit label never paints past its box: overflow on a Label is not a scroll or clip switch, so text-overflow decides alone",
		doc:    "`text-overflow` ellipsizes a label whatever its `overflow`",
	},
	{
		name: "font-palette paints the first palette",
		spec: "css-fonts-4 §9.1 (font-palette selects a CPAL palette)",
		css:  `#b { font-palette: dark; }`,
		tree: nCol("", nText("#a", "x"), nText("#b", "x")),
		want: []confCheck{wantProbe("the two labels paint alike", func(f *confFixture) any {
			ra, rb := boundsOf(f.get("a")), boundsOf(f.get("b"))
			for y := range ra.H {
				for x := range ra.W {
					if f.pixel(ra.X+x, ra.Y+y) != f.pixel(rb.X+x, rb.Y+y) {
						return false
					}
				}
			}
			return true
		}, true)},
		reason: "color glyphs paint with the font's first CPAL palette: the font loader exposes the palettes without their light and dark type flags, and @font-palette-values is not parsed",
		doc:    "`font-palette` computes and inherits; color glyphs always paint with the font's first palette",
	},
}

func TestCSSConformanceText(t *testing.T) { runConformance(t, confText) }

func TestCSSConformanceTextExceptions(t *testing.T) {
	runConformanceExceptions(t, confExceptionsText)
}
