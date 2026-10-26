package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// longWord is an unbreakable path far wider than 200 px.
var longWord = "/home/user/Pictures/" + strings.Repeat("Screenshot_2026-10-25_16-04-33_", 3) + "extra_long_name.png" + " " + strings.Repeat("x", 120)

// A wrapping, ellipsizing, two-line label holding a word wider than its
// box breaks the word across its lines and ellipsizes the last, never
// measuring or painting past its box: a notification body with a long
// path ran past its card, unwrapped and unellipsized.
func TestWrappingLabelsBreakAnOverlongWord(t *testing.T) {
	face := goldenFace(t)
	plain := NewLabel(face, 14, longWord, 0xff000000)
	plain.SetWrap(true)
	plain.SetEllipsize(EllipsizeEnd)
	plain.SetMaxLines(2)
	rich := NewRichLabel(face, 14, longWord, 0xff000000)
	rich.SetWrap(true)
	rich.SetEllipsize(EllipsizeEnd)
	rich.SetMaxLines(2)
	for name, l := range map[string]Widget{"Label": plain, "RichLabel": rich} {
		t.Run(name, func(t *testing.T) {
			col := NewBox(Column, 0, 0)
			col.Append(l, false)
			col.SetSizeRequest(200, 0)
			col.SetMaximumSize(200, 0)
			if got := l.Measure(Constraints{Max: Size{W: 200, H: 400}}); got.W > 200 {
				t.Errorf("measures %d wide in 200", got.W)
			}
			const w, h = 400, 120
			arrangeTree(t, col, w, h)
			b := l.(Boundser).Bounds()
			data := make([]byte, render.Stride(w)*h)
			l.Paint(render.New(data, render.Stride(w), w, h))
			for y := range h {
				for x := b.X + b.W; x < w; x++ {
					if data[y*render.Stride(w)+x*4+3] != 0 {
						t.Fatalf("ink at %d,%d past the label's right edge %d", x, y, b.X+b.W)
					}
				}
			}
			if b.H < 30 {
				t.Errorf("label is %d tall, want two lines", b.H)
			}
		})
	}
}

// A label capped at two ellipsized lines carries on its last line from
// where that line starts: the merge took the second line's end, minus
// the spaces wrapping drops, so it showed "r five six" after "one two".
func TestRichLabelLastLineStartsWhereItShould(t *testing.T) {
	l := NewRichLabel(goldenFace(t), 14, "one two three four five six seven eight nine ten eleven", 0)
	l.SetWrap(true)
	l.SetEllipsize(EllipsizeEnd)
	l.SetMaxLines(2)
	rows, starts := l.wrapSpans(80)
	if len(rows) < 3 || []rune(l.Text())[starts[1]] != 't' || []rune(l.Text())[starts[2]] != 'f' {
		t.Fatalf("rows start at %v", starts)
	}
	var merged strings.Builder
	for _, r := range sliceSpans(l.runs, starts[1], l.runes) {
		merged.WriteString(r.Text)
	}
	if !strings.HasPrefix(merged.String(), "three four five") {
		t.Errorf("the last line carries on from %q, want three four five", merged.String())
	}
}
