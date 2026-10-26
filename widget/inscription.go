package widget

import (
	"math"
	"strings"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Inscription is GTK's Inscription: text sized by character and line
// counts instead of by its content, for cells in big lists and grids.
// Measure shapes nothing but one reference glyph: the minimum and
// natural widths are MinChars and NatChars of the face's average
// advance, the heights MinLines and NatLines of its line height. Paint
// draws as many lines as fit, each ellipsized at the end.
type Inscription struct {
	node
	face     render.Font
	sizePx   float64
	text     string
	color    render.Color
	align    render.Alignment
	minChars int
	natChars int
	minLines int
	natLines int
	charW    float64
	lineH    int
}

// NewInscription returns an inscription of text: 3 characters minimum,
// 10 natural, one line.
func NewInscription(face render.Font, sizePx float64, text string, color render.Color) *Inscription {
	requireFace("NewInscription", face)
	in := &Inscription{face: face, sizePx: sizePx, text: text, color: color, minChars: 3, natChars: 10, minLines: 1, natLines: 1}
	ref := face.Shape("m", sizePx)
	in.charW, in.lineH = ref.Advance(), ref.LineHeight()
	return in
}

// Text returns the text.
func (in *Inscription) Text() string { return in.text }

// SetText replaces the text. The size does not depend on it, so only
// a repaint follows.
func (in *Inscription) SetText(s string) {
	if in.text != s {
		in.text = s
		in.Invalidate()
	}
}

// SetMinChars sets the minimum width in characters.
func (in *Inscription) SetMinChars(n int) { in.setSize(&in.minChars, n) }

// SetNatChars sets the natural width in characters.
func (in *Inscription) SetNatChars(n int) { in.setSize(&in.natChars, n) }

// SetMinLines sets the minimum height in lines.
func (in *Inscription) SetMinLines(n int) { in.setSize(&in.minLines, n) }

// SetNatLines sets the natural height in lines.
func (in *Inscription) SetNatLines(n int) { in.setSize(&in.natLines, n) }

// SetAlignment sets the horizontal text alignment.
func (in *Inscription) SetAlignment(a render.Alignment) {
	if in.align != a {
		in.align = a
		in.Invalidate()
	}
}

// SetColor sets the ink.
func (in *Inscription) SetColor(c render.Color) {
	if in.color != c {
		in.color = c
		in.Invalidate()
	}
}

func (in *Inscription) setSize(field *int, n int) {
	n = max(0, n)
	if *field != n {
		*field = n
		in.InvalidateLayout()
	}
}

// MinSize is the minimum character and line counts' extent.
func (in *Inscription) MinSize() Size {
	return Size{W: int(math.Ceil(float64(in.minChars) * in.charW)), H: in.minLines * in.lineH}
}

// Measure claims the natural character and line counts' extent, never
// below the minimum.
func (in *Inscription) Measure(con Constraints) Size {
	if sz, ok := in.measureHit(con); ok {
		return sz
	}
	m := in.MinSize()
	w := max(int(math.Ceil(float64(max(in.natChars, in.minChars))*in.charW)), m.W)
	h := max(max(in.natLines, in.minLines)*in.lineH, m.H)
	return in.measureStore(con, Size{W: min(max(w, con.Min.W), con.Max.W), H: min(max(h, con.Min.H), con.Max.H)})
}

// Arrange records the bounds.
func (in *Inscription) Arrange(r render.Rect) { in.node.Arrange(r) }

// Paint draws the lines that fit, each ellipsized to the width.
func (in *Inscription) Paint(cv *render.Canvas) {
	if in.lineH <= 0 {
		return
	}
	r := in.bounds
	ink := pickc(in.color, in.style(in), style.PropColor, inkOr(in.color))
	lines := strings.Split(in.text, "\n")
	fit := max(1, r.H/in.lineH)
	for i, line := range lines {
		if i == fit {
			break
		}
		if i == fit-1 && i < len(lines)-1 {
			line += "…"
		}
		if in.face.Shape(line, in.sizePx).Advance() > float64(r.W) {
			line = render.EllipsizeText(in.face, line, render.EllipsizeEnd, float64(r.W), in.sizePx)
		}
		box := render.Rect{X: r.X, Y: r.Y + i*in.lineH, W: r.W, H: in.lineH}
		in.face.DrawAlignedDir(cv, line, box, in.sizePx, ink, in.align, DirectionAuto)
	}
}

// HitTest returns the inscription when p is inside it.
func (in *Inscription) HitTest(p Point) Widget { return in.HitLeaf(in, p) }

// Role reads as a label.
func (in *Inscription) Role() Role { return RoleLabel }
