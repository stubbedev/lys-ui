package widget

import (
	"math"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// SymbolKind names one of the vector glyphs the toolkit draws for its
// own chrome - disclosure arrows, sort indicators, window controls -
// instead of text glyphs, which fonts (the bundled fixture face among
// them) often lack and which then render as missing-glyph boxes.
type SymbolKind uint8

// Symbol kinds.
const (
	// SymbolChevronRight is a right-pointing chevron (collapsed, next).
	SymbolChevronRight SymbolKind = iota
	// SymbolChevronDown is a down-pointing chevron (expanded, descending).
	SymbolChevronDown
	// SymbolChevronLeft is a left-pointing chevron (back).
	SymbolChevronLeft
	// SymbolChevronUp is an up-pointing chevron (ascending).
	SymbolChevronUp
	// SymbolClose is a diagonal cross.
	SymbolClose
	// SymbolMinimize is a low horizontal bar.
	SymbolMinimize
	// SymbolMaximize is an outlined square.
	SymbolMaximize
	// SymbolCrosshair is a picker's crosshair.
	SymbolCrosshair
	// SymbolDoubleLeft is two left chevrons (a bigger step back).
	SymbolDoubleLeft
	// SymbolDoubleRight is two right chevrons (a bigger step forward).
	SymbolDoubleRight
	// SymbolEllipsis is three dots in a row (more, collapsed items).
	SymbolEllipsis
	// SymbolMinus is a centered horizontal bar (a separator, less).
	SymbolMinus
	// SymbolCheck is a check mark (selected, done).
	SymbolCheck
	// SymbolChevronsUpDown is an up chevron over a down one (a picker
	// that opens a list).
	SymbolChevronsUpDown
	// SymbolSearch is a magnifying glass.
	SymbolSearch
	// SymbolCalendar is a calendar page (a date field).
	SymbolCalendar
	// SymbolPanelLeft is a window with a left pane (a sidebar toggle).
	SymbolPanelLeft
	// SymbolArrowLeft is a left-pointing arrow (previous).
	SymbolArrowLeft
	// SymbolArrowRight is a right-pointing arrow (next).
	SymbolArrowRight
	// SymbolUser is a head over shoulders (a person, an account).
	SymbolUser
)

// chevronAngle is each chevron kind's rotation from right-pointing.
var chevronAngle = map[SymbolKind]float64{
	SymbolChevronRight: 0,
	SymbolChevronDown:  math.Pi / 2,
	SymbolChevronLeft:  math.Pi,
	SymbolChevronUp:    -math.Pi / 2,
}

// Symbol paints one SymbolKind in a size x size box, stroked in the
// cascade's color over the theme's muted text. Styled as `image`.
type Symbol struct {
	node
	kind SymbolKind
	size int
}

// NewSymbol returns a symbol of kind in a size-pixel box.
func NewSymbol(kind SymbolKind, size int) *Symbol {
	s := &Symbol{kind: kind, size: size}
	s.SetElement("image")
	return s
}

// SetKind changes the glyph (a disclosure turning, a sort flipping).
func (s *Symbol) SetKind(kind SymbolKind) {
	if s.kind == kind {
		return
	}
	s.kind = kind
	s.Invalidate()
}

// Kind reports the glyph.
func (s *Symbol) Kind() SymbolKind { return s.kind }

// Measure wants the symbol's box.
func (s *Symbol) Measure(con Constraints) Size {
	return clampSize(Size{W: s.size, H: s.size}, con)
}

// Arrange records the rect.
func (s *Symbol) Arrange(r render.Rect) { s.node.Arrange(r) }

// Paint strokes the glyph centered in the bounds.
func (s *Symbol) Paint(cv *render.Canvas) {
	paintSymbol(cv, s.kind, s.bounds, s.size, pickc(0, s.style(s), style.PropColor, Current().TextMuted))
}

// paintSymbol strokes kind in col, centered in b at most size wide.
func paintSymbol(cv *render.Canvas, kind SymbolKind, b render.Rect, size int, col render.Color) {
	cx, cy := b.X+b.W/2, b.Y+b.H/2
	r := min(b.W, b.H, size) / 3
	if ang, ok := chevronAngle[kind]; ok {
		strokeChevron(cv, cx, cy, r, ang, col)
		return
	}
	switch kind {
	case SymbolDoubleLeft, SymbolDoubleRight:
		ang := 0.0
		if kind == SymbolDoubleLeft {
			ang = math.Pi
		}
		strokeChevron(cv, cx-r/2, cy, r, ang, col)
		strokeChevron(cv, cx+r/2, cy, r, ang, col)
	case SymbolClose:
		strokeCross(cv, cx, cy, r, col)
	case SymbolMinimize:
		cv.Line(cx-r, cy+r, cx+r, cy+r, 1, col)
	case SymbolMaximize:
		cv.Line(cx-r, cy-r, cx+r, cy-r, 1, col)
		cv.Line(cx+r, cy-r, cx+r, cy+r, 1, col)
		cv.Line(cx+r, cy+r, cx-r, cy+r, 1, col)
		cv.Line(cx-r, cy+r, cx-r, cy-r, 1, col)
	case SymbolCrosshair:
		cv.Line(cx-r, cy, cx+r, cy, 1, col)
		cv.Line(cx, cy-r, cx, cy+r, 1, col)
	case SymbolCheck:
		strokeCheck(cv, cx, cy, r, col)
	case SymbolChevronsUpDown:
		strokeChevron(cv, cx, cy-r/2-1, r/2, -math.Pi/2, col)
		strokeChevron(cv, cx, cy+r/2+1, r/2, math.Pi/2, col)
	case SymbolSearch:
		d := r * 3 / 2
		lens := render.Rect{X: cx - r, Y: cy - r, W: d, H: d}
		cv.RoundedBorder(lens, render.UniformCorners(d/2), render.UniformInsets(1), col)
		cv.Line(cx-r+d-1, cy-r+d-1, cx+r, cy+r, 1, col)
	case SymbolCalendar:
		page := render.Rect{X: cx - r, Y: cy - r + r/3, W: 2 * r, H: 2*r - r/3}
		cv.RoundedBorder(page, render.UniformCorners(2), render.UniformInsets(1), col)
		cv.Line(cx-r, cy-r/3+r/3, cx+r, cy-r/3+r/3, 1, col)
		cv.Line(cx-r/2, cy-r, cx-r/2, cy-r+2*r/3, 1, col)
		cv.Line(cx+r/2, cy-r, cx+r/2, cy-r+2*r/3, 1, col)
	case SymbolPanelLeft:
		frame := render.Rect{X: cx - r, Y: cy - r, W: 2 * r, H: 2 * r}
		cv.RoundedBorder(frame, render.UniformCorners(2), render.UniformInsets(1), col)
		cv.Line(cx-r/3, cy-r, cx-r/3, cy+r, 1, col)
	case SymbolMinus:
		cv.Line(cx-r, cy, cx+r, cy, 1, col)
	case SymbolArrowLeft, SymbolArrowRight:
		tip, back := cx+r, -1
		if kind == SymbolArrowLeft {
			tip, back = cx-r, 1
		}
		cv.Line(cx-r, cy, cx+r, cy, 1, col)
		cv.Line(tip, cy, tip+back*2*r/3, cy-2*r/3, 1, col)
		cv.Line(tip, cy, tip+back*2*r/3, cy+2*r/3, 1, col)
	case SymbolUser:
		fx, fy, fr := float64(cx), float64(cy), float64(r)
		cv.Arc(fx, fy-fr/2, fr/2, 1, 0, 2*math.Pi, col)
		cv.Arc(fx, fy+1.2*fr, 0.95*fr, 1, math.Pi, math.Pi, col)
	case SymbolEllipsis:
		d := max(2, size/8)
		for _, x := range [...]int{cx - r, cx, cx + r} {
			cv.RoundedRect(render.Rect{X: x - d/2, Y: cy - d/2, W: d, H: d}, d, col)
		}
	}
}

// HitTest resolves inside the bounds.
func (s *Symbol) HitTest(p Point) Widget { return s.HitLeaf(s, p) }

// strokeCross draws a diagonal cross of half-span r centered on
// (cx, cy): the close glyph Symbol and the notebook's tab close share.
func strokeCross(cv *render.Canvas, cx, cy, r int, col render.Color) {
	cv.Line(cx-r, cy-r, cx+r, cy+r, 1, col)
	cv.Line(cx-r, cy+r, cx+r, cy-r, 1, col)
}

// strokeCheck draws a check mark of half-span r centered on (cx, cy):
// SymbolCheck and a menu's checked row share it.
func strokeCheck(cv *render.Canvas, cx, cy, r int, col render.Color) {
	cv.Line(cx-r, cy, cx-r/3, cy+2*r/3, 1, col)
	cv.Line(cx-r/3, cy+2*r/3, cx+r, cy-2*r/3, 1, col)
}

// strokeChevron draws a v-shaped chevron centered on (cx, cy) with arm
// half-span r, rotated ang radians from right-pointing - the stroke the
// dropdown face and every Symbol chevron share.
func strokeChevron(cv *render.Canvas, cx, cy, r int, ang float64, col render.Color) {
	c, s := math.Cos(ang), math.Sin(ang)
	rot := func(px, py float64) (int, int) {
		return cx + int(math.Round(px*c-py*s)), cy + int(math.Round(px*s+py*c))
	}
	h := float64(r)
	tx, ty := rot(h/2, 0)
	ax, ay := rot(-h/2, -h)
	bx, by := rot(-h/2, h)
	cv.Line(ax, ay, tx, ty, 1, col)
	cv.Line(tx, ty, bx, by, 1, col)
}
