package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// shapedText is the text the label paints after its last fit.
func shapedText(l *RichLabel) string {
	var b strings.Builder
	for _, r := range l.shaped {
		b.WriteString(r.sh.Text())
	}
	return b.String()
}

// TestRichLabelEllipsize pins the truncation: a line wider than its box
// trades runes for an ellipsis at the end, start, or middle until it
// fits, the ellipsis taking the style beside it; a line that fits, or
// one with truncation off, paints whole; the natural width stays the
// whole line's.
func TestRichLabelEllipsize(t *testing.T) {
	face := entryFace(t)
	white := render.RGB(255, 255, 255)
	markup := "<b>Firefox</b> web browser for everyone"
	for _, tc := range []struct {
		mode          EllipsizeMode
		prefix, suffx string
	}{
		{EllipsizeEnd, "Firefox", "…"},
		{EllipsizeStart, "…", "everyone"},
		{EllipsizeMiddle, "Fir", "one"},
	} {
		l := NewRichLabel(face, 13, markup, white)
		natural := l.Measure(Constraints{Max: Size{W: 2000, H: 100}})
		l.SetEllipsize(tc.mode)
		if got := l.Measure(Constraints{Max: Size{W: 2000, H: 100}}); got != natural {
			t.Errorf("mode %d: natural %v changed to %v", tc.mode, natural, got)
		}
		l.Arrange(render.Rect{W: natural.W / 2, H: natural.H})
		text := shapedText(l)
		if !strings.HasPrefix(text, tc.prefix) || !strings.HasSuffix(text, tc.suffx) || !strings.Contains(text, "…") {
			t.Errorf("mode %d: painted %q", tc.mode, text)
		}
		if l.advance > float64(natural.W/2) {
			t.Errorf("mode %d: %v wide in a %d box", tc.mode, l.advance, natural.W/2)
		}
		if l.Text() != "Firefox web browser for everyone" {
			t.Errorf("Text = %q, want the whole line", l.Text())
		}
		// Room again: the whole line comes back.
		l.Arrange(render.Rect{W: natural.W, H: natural.H})
		if shapedText(l) != l.Text() {
			t.Errorf("mode %d: a fitting line painted %q", tc.mode, shapedText(l))
		}
	}

	// The end ellipsis inside a bold run is bold.
	l := NewRichLabel(face, 13, "<b>bold bold bold bold</b>", white)
	l.SetEllipsize(EllipsizeEnd)
	n := l.Measure(Constraints{Max: Size{W: 2000, H: 100}})
	l.Arrange(render.Rect{W: n.W / 2, H: n.H})
	if last := l.shaped[len(l.shaped)-1]; !last.style.Bold {
		t.Error("the ellipsis lost the bold run's style")
	}

	// Truncation off: the line overflows whole.
	off := NewRichLabel(face, 13, markup, white)
	m := off.Measure(Constraints{Max: Size{W: 2000, H: 100}})
	off.Arrange(render.Rect{W: m.W / 2, H: m.H})
	if strings.Contains(shapedText(off), "…") {
		t.Error("an unset ellipsize truncated")
	}
}
