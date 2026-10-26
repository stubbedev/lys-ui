package widget

import (
	"math"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Baseliner reports a widget's first text baseline (GTK's baseline):
// the distance from the top of the rect its parent arranges it in, at
// its natural height, down to the baseline its first line of text sits
// on. ok is false for a widget without text. A row Box lines up the
// children it holds with AlignBaseline on it, so labels, entries, and
// buttons of different sizes read as one line.
type Baseliner interface {
	Baseline() (int, bool)
}

// baselineOf is w's baseline, ok false when it has none.
func baselineOf(w Widget) (int, bool) {
	if b, ok := w.(Baseliner); ok {
		return b.Baseline()
	}
	return 0, false
}

// lineBaseline is where a line of sh sits when centered in a box h
// tall - the rule every text draw uses (drawAligned), so a reported
// baseline is the painted one.
func lineBaseline(h int, sh *render.ShapedText) int {
	return int(math.Round((float64(h)-float64(sh.LineHeight()))/2 + sh.Ascent()))
}

// Baseline implements Baseliner: the first line inside the CSS box,
// raised or lowered by vertical-align (sub, super, a length, or a
// percentage resolved against the line-height at compute time).
func (l *Label) Baseline() (int, bool) {
	if l.shaped == nil {
		return 0, false
	}
	v := l.style(l)
	lineBox := l.lineBox(v)
	base := boxOf(v, render.Insets{}).outer().Top + lineBaseline(lineBox, l.shaped)
	if v.Has(style.PropVerticalAlign) {
		shift := v.VerticalAlignPx
		if v.VerticalAlignPct {
			shift *= float64(lineBox)
		}
		switch v.VerticalAlign {
		case style.VerticalAlignSub:
			base += int(math.Round(0.25 * fontPx(v, l.sizePx)))
		case style.VerticalAlignSuper:
			base -= int(math.Round(0.33 * fontPx(v, l.sizePx)))
		case style.VerticalAlignLength:
			base -= int(math.Round(shift))
		}
	}
	return base, true
}

// Baseline implements Baseliner: the text line inside the field's
// insets, under the margin.
func (e *Entry) Baseline() (int, bool) {
	lg := e.face.Shape("lg", e.fontPx())
	line := int(lg.Ascent() + lg.Descent() + 0.5)
	return marginOf(e.style(e)).Top + e.textInsets().Top + lineBaseline(line, lg), true
}

// Baseline implements Baseliner: the child's, inside the button's box.
func (b *Button) Baseline() (int, bool) {
	base, ok := baselineOf(b.child)
	return b.box(b.style(b)).outer().Top + base, ok
}

// Baseline implements Baseliner: the root's, laid over the whole rect.
func (c *composite) Baseline() (int, bool) { return baselineOf(c.root) }
