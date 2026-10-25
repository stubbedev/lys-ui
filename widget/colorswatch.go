package widget

import (
	"image"
	"image/color"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

const (
	// swatchSize is a swatch's side when min-width and min-height
	// leave it unset.
	swatchSize = 24
	// swatchColumns is how many presets a SwatchPicker row holds.
	swatchColumns = 8
	// checkerTile is the side of one checkerboard square under a
	// translucent swatch.
	checkerTile = 4
)

// ColorSwatch is a chip of one color: the colorswatch element, sized
// by its min-width and min-height, with the theme's radius and border,
// a checkerboard under a translucent color, and a ring while selected
// (the :selected state).
type ColorSwatch struct {
	node
	color render.Color
}

// NewColorSwatch returns a swatch of c.
func NewColorSwatch(c render.Color) *ColorSwatch {
	s := &ColorSwatch{color: c}
	s.SetElement("colorswatch")
	return s
}

// Color returns the shown color.
func (s *ColorSwatch) Color() render.Color { return s.color }

// SetColor shows c.
func (s *ColorSwatch) SetColor(c render.Color) {
	if s.color != c {
		s.color = c
		s.Invalidate()
	}
}

// SetSelected rings the swatch as the current pick.
func (s *ColorSwatch) SetSelected(on bool) { s.SetState(StateSelected, on) }

// Selected reports whether the swatch is ringed.
func (s *ColorSwatch) Selected() bool { return s.HasState(StateSelected) }

// Measure wants the min-width by min-height border box.
func (s *ColorSwatch) Measure(con Constraints) Size {
	v := s.style(s)
	return clampSize(Size{W: picki(v, style.PropMinWidth, swatchSize), H: picki(v, style.PropMinHeight, swatchSize)}, con)
}

// Paint draws the box, then the color over a checkerboard inside the
// border.
func (s *ColorSwatch) Paint(cv *render.Canvas) {
	v := s.style(s)
	radii, bw := radiusOr(v, 0), borderOf(v)
	paintBoxBehind(cv, v, s.bounds, radii, bw, 0)
	inner, innerRadii := bw.Shrink(s.bounds), insetCorners(radii, bw)
	if s.color.A() < 0xff {
		paintChecker(cv, inner, innerRadii)
	}
	cv.RoundedRectCorners(inner, innerRadii, s.color)
}

// paintChecker fills r's rounded outline with the transparency
// checkerboard.
func paintChecker(cv *render.Canvas, r render.Rect, radii render.Corners) {
	cv.PaintImageTiles(checkerboard(), render.Tile{
		X: float64(r.X), Y: float64(r.Y), W: 2 * checkerTile, H: 2 * checkerTile, RepeatX: true, RepeatY: true,
	}, r, radii)
}

// checkerboard is one tile of the transparency checkerboard, two
// squares a side, in the theme's surface and border shades.
func checkerboard() image.Image {
	th := Current()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			c := th.Surface
			if (x+y)%2 == 0 {
				c = th.Border
			}
			img.Set(x, y, color.RGBA{R: c.R(), G: c.G(), B: c.B(), A: c.A()})
		}
	}
	return img
}

// HitTest returns the swatch inside its bounds.
func (s *ColorSwatch) HitTest(p Point) Widget { return s.HitLeaf(s, p) }

// a11yName is the color as hex.
func (s *ColorSwatch) a11yName() string { return FormatColor(s.color) }

// DefaultSwatches is the preset palette a ColorButton offers: neutrals
// then the hues around the wheel.
func DefaultSwatches() []render.Color {
	return []render.Color{
		render.RGB(0x0a, 0x0a, 0x0a), render.RGB(0x73, 0x73, 0x73), render.RGB(0xd4, 0xd4, 0xd4), render.RGB(0xff, 0xff, 0xff),
		render.RGB(0xef, 0x44, 0x44), render.RGB(0xf9, 0x73, 0x16), render.RGB(0xf5, 0x9e, 0x0b), render.RGB(0x84, 0xcc, 0x16),
		render.RGB(0x22, 0xc5, 0x5e), render.RGB(0x14, 0xb8, 0xa6), render.RGB(0x06, 0xb6, 0xd4), render.RGB(0x3b, 0x82, 0xf6),
		render.RGB(0x63, 0x66, 0xf1), render.RGB(0x8b, 0x5c, 0xf6), render.RGB(0xd9, 0x46, 0xef), render.RGB(0xec, 0x48, 0x99),
	}
}

// SwatchPicker picks a color from a grid of preset swatches, one click
// each, the current one ringed; an optional custom row lists the
// user's own colors under the presets, and a Custom entry opens the
// full ColorChooser in place. OnPicked hears a swatch pick, OnEdited
// every change the chooser makes.
type SwatchPicker struct {
	composite
	face    render.Font
	sizePx  float64
	column  *Box
	presets *Box
	custom  *Box
	more    *Button
	chooser *ColorChooser
	current render.Color
	// OnPicked runs when a swatch is clicked.
	OnPicked func(c render.Color)
	// OnEdited runs on every change in the expanded chooser.
	OnEdited func(c render.Color)
}

// NewSwatchPicker returns a picker over presets with current ringed.
func NewSwatchPicker(face render.Font, sizePx float64, presets []render.Color, current render.Color) *SwatchPicker {
	face = requireFace("widget.NewSwatchPicker", face)
	th := Current()
	p := &SwatchPicker{
		face: face, sizePx: sizePx, current: current,
		column: NewBox(Column, th.Spacing, 2*th.Padding), presets: NewBox(Column, th.Spacing/2, 0), custom: NewBox(Column, th.Spacing/2, 0),
	}
	p.SetElement("swatchpicker")
	p.fillGrid(p.presets, presets)
	p.column.Append(p.presets, false)
	label := NewLabel(face, sizePx, "Custom…", 0)
	p.more = NewButton(label, 0, 0)
	p.more.AddClass(css.Ghost, css.Small)
	p.more.OnClick = p.expand
	p.column.AppendAligned(p.more, false, AlignStart)
	p.initComposite(p, p.column)
	p.mark()
	return p
}

// SetCustom shows colors as a row under the presets; none hides it.
func (p *SwatchPicker) SetCustom(colors []render.Color) {
	shown := len(p.custom.Children()) > 0
	p.fillGrid(p.custom, colors)
	switch {
	case len(colors) > 0 && !shown:
		p.column.InsertAligned(1, p.custom, false, AlignFill)
	case len(colors) == 0 && shown:
		p.column.Remove(p.custom)
	}
	p.mark()
}

// Color returns the current color.
func (p *SwatchPicker) Color() render.Color { return p.current }

// SetColor makes c current, ringing its swatch.
func (p *SwatchPicker) SetColor(c render.Color) {
	p.current = c
	p.mark()
}

// Expanded reports whether the full chooser is open.
func (p *SwatchPicker) Expanded() bool { return p.chooser != nil }

// fillGrid lays colors into grid, swatchColumns to a row, each a
// button over its swatch.
func (p *SwatchPicker) fillGrid(grid *Box, colors []render.Color) {
	grid.Clear()
	var row *Box
	for i, c := range colors {
		if i%swatchColumns == 0 {
			row = NewBox(Row, Current().Spacing/2, 0)
			grid.Append(row, false)
		}
		sw := NewColorSwatch(c)
		b := NewButton(sw, 0, 0)
		b.AddClass("swatch")
		b.SetAccessibleName(FormatColor(c))
		b.OnClick = func() { p.pick(c) }
		row.Append(b, false)
	}
}

// mark rings the swatches of the current color.
func (p *SwatchPicker) mark() {
	for _, grid := range []*Box{p.presets, p.custom} {
		walkWidgets(grid, func(w Widget) bool {
			if sw, ok := w.(*ColorSwatch); ok {
				sw.SetSelected(sw.color == p.current)
			}
			return true
		})
	}
}

// pick makes c current and reports it.
func (p *SwatchPicker) pick(c render.Color) {
	p.SetColor(c)
	if p.chooser != nil {
		p.chooser.SetColor(c)
	}
	if p.OnPicked != nil {
		p.OnPicked(c)
	}
}

// expand swaps the Custom entry for the full chooser.
func (p *SwatchPicker) expand() {
	if p.chooser != nil {
		return
	}
	p.chooser = NewColorChooser(p.face, p.sizePx, p.current)
	p.chooser.OnChanged = func(c render.Color) {
		p.SetColor(c)
		if p.OnEdited != nil {
			p.OnEdited(c)
		}
	}
	p.column.Remove(p.more)
	p.column.Append(p.chooser, false)
}
