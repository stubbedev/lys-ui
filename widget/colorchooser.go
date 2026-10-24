package widget

import (
	"strings"

	"github.com/stubbedev/lys-ui/render"
)

// colorStep is the keyboard step for saturation, value, and hue: one
// square- or strip-width per 64 presses.
const colorStep = 1.0 / 64

// ColorChooser edits one color end to end: an SV square with a hue
// strip and an alpha strip beside it, a hex entry, and two palette rows
// (theme-derived presets and custom picks). OnChanged fires with the
// straight (non-premultiplied) color on every applied change; the SV
// square is two stacked linear gradients — white to hue, transparent to
// black — composited through the canvas's exact premultiplied blending,
// a shader-less SV square.
//
// Pointer: drags on the square and both strips track the pointer; a
// click on a swatch picks it; the hex entry parses on change (#RGB,
// #RRGGBB, #RRGGBBAA, with or without the #). Keyboard: arrows move
// saturation (horizontal) and value (vertical), shift+arrows move the
// hue, PageUp/PageDown step alpha, and the entry types hex directly -
// the chooser is fully operable without the pointer.
type ColorChooser struct {
	node
	face   render.Font
	sizePx float64

	// hue, sat, val are the color's HSV coordinates in [0,1]; alpha its
	// opacity in [0,1].
	hue, sat, val, alpha float64

	hex       *Entry
	mirroring bool // the entry's text is our own mirror, not typing
	addTo     *Button
	// eyedrop, when SetEyedropper wired a screen-pick source, runs on
	// the pick-from-screen button; pickFrom is that button.
	eyedrop  func()
	pickFrom *Button
	presets  []render.Color
	custom   func() []render.Color
	addPick  func(render.Color)

	// svRect/hueRect/alphaRect are the control rects inside the last
	// arranged bounds.
	sv, hueStrip, alphaStrip render.Rect

	// OnChanged fires after every applied color change, whatever
	// produced it.
	OnChanged func(render.Color)
}

// NewColorChooser returns a chooser editing initial (its alpha comes
// along), preset swatches derived from the current theme.
func NewColorChooser(face render.Font, sizePx float64, initial render.Color) *ColorChooser {
	face = requireFace("widget.NewColorChooser", face)
	c := &ColorChooser{face: face, sizePx: sizePx}
	c.hue, c.sat, c.val = hsvOf(initial)
	c.alpha = float64(initial.A()) / 255
	th := Current()
	c.presets = []render.Color{th.Accent, th.Text, th.TextMuted, th.Border, th.Surface, th.Bg}
	c.hex = NewEntry(face, sizePx, th.Text)
	c.hex.SetText(c.hexText())
	c.hex.OnChanged = func(s string) { c.applyHex(s) }
	c.addTo = NewButton(NewLabel(face, sizePx, "+", 0), 6, 4)
	return c
}

// SetPaletteSource wires the chooser's custom-palette view: colors
// lists the session's picks and add records a new one (the chooser
// calls it from the + button). Without a source the custom row stays
// empty and the button is hidden.
func (c *ColorChooser) SetPaletteSource(colors func() []render.Color, add func(render.Color)) {
	c.custom, c.addPick = colors, add
	c.InvalidateLayout()
}

// SetEyedropper wires the pick-from-screen button: start begins a
// screen color pick (the application's captured-screen flow, #84);
// without it the button stays hidden - swatches-only.
func (c *ColorChooser) SetEyedropper(start func()) {
	c.eyedrop = start
	if start != nil && c.pickFrom == nil {
		c.pickFrom = NewButton(NewSymbol(SymbolCrosshair, int(c.sizePx)), 6, 4)
		c.pickFrom.SetCursorName("crosshair")
	}
	c.InvalidateLayout()
}

// Color returns the current color with its alpha.
func (c *ColorChooser) Color() render.Color {
	return hsvColor(c.hue, c.sat, c.val, c.alpha)
}

// SetColor applies col (firing OnChanged once) — the programmatic path.
func (c *ColorChooser) SetColor(col render.Color) {
	c.hue, c.sat, c.val = hsvOf(col)
	c.alpha = float64(col.A()) / 255
	c.colorChanged(false)
}

// colorChanged records a settled color: the hex entry mirrors it (when
// the change did not come from the entry itself) and OnChanged fires.
func (c *ColorChooser) colorChanged(fromHex bool) {
	if !fromHex {
		// Mirror the settled color into the entry without feeding back:
		// the guard keeps applyHex from re-deriving (and re-quantizing)
		// the HSV from its own mirror text.
		c.mirroring = true
		c.hex.SetText(c.hexText())
		c.mirroring = false
	}
	c.Invalidate()
	if c.OnChanged != nil {
		c.OnChanged(c.Color())
	}
}

// hexText renders the current color as #rrggbb (alpha ff omitted).
func (c *ColorChooser) hexText() string { return FormatColor(c.Color()) }

// applyHex parses typed text and applies the color it names (markup's
// #RGB/#RRGGBB/#RRGGBBAA reader, the # optional here); anything
// unparseable is ignored - the entry keeps typing, the color stands.
func (c *ColorChooser) applyHex(s string) {
	if c.mirroring {
		return
	}
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	col, ok := parseHexColor(s)
	if !ok {
		return
	}
	c.hue, c.sat, c.val = hsvOf(col)
	c.alpha = float64(col.A()) / 255
	c.colorChanged(true)
}

// hsvOf converts a straight color to hue, saturation, value in [0,1].
func hsvOf(col render.Color) (h, s, v float64) {
	b := col.Straight()
	r := float64(b[0]) / 255
	g := float64(b[1]) / 255
	bl := float64(b[2]) / 255
	max, min := r, r
	for _, x := range []float64{g, bl} {
		max = maxf(max, x)
		min = minf(min, x)
	}
	v = max
	if max == 0 {
		return 0, 0, 0
	}
	d := max - min
	s = d / max
	switch max {
	case min:
		h = 0
	case r:
		h = (g - bl) / d
		if g < bl {
			h += 6
		}
	case g:
		h = (bl-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h /= 6
	return h, s, v
}

// hsvColor converts HSV in [0,1] plus alpha in [0,1] to a color.
func hsvColor(h, s, v, a float64) render.Color {
	i := int(h*6) % 6
	f := h*6 - float64(int(h*6))
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch i {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return render.RGBA(uint8(r*255+0.5), uint8(g*255+0.5), uint8(b*255+0.5), uint8(a*255+0.5))
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Measure wants the SV square (160) plus the strips (24 + gaps), the
// entry row, and the two palette rows.
func (c *ColorChooser) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	lineH := c.face.Shape("lg", c.sizePx).LineHeight()
	w := 160 + 8 + 24 + 8 + 24 + 16
	if c.pickFrom != nil {
		w += 8 + 24
	}
	h := 160 + 8 + lineH + 12 + 8 + (lineH+10)*2 + 12
	return c.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Arrange records the rect and places the hex entry and the + button.
func (c *ColorChooser) Arrange(r render.Rect) {
	c.node.Arrange(r)
	lineH := c.face.Shape("lg", c.sizePx).LineHeight()
	c.sv = render.Rect{X: r.X + 8, Y: r.Y + 8, W: 160, H: 160}
	c.hueStrip = render.Rect{X: c.sv.X + c.sv.W + 8, Y: c.sv.Y, W: 24, H: 160}
	c.alphaStrip = render.Rect{X: c.hueStrip.X + c.hueStrip.W + 8, Y: c.sv.Y, W: 24, H: 160}
	entryY := c.sv.Y + c.sv.H + 8
	entryH := lineH + 10
	c.hex.Arrange(render.Rect{X: c.sv.X, Y: entryY, W: 160 + 8 + 24, H: entryH})
	btnX := c.hex.Bounds().X + c.hex.Bounds().W + 8
	if c.custom != nil {
		nat := c.addTo.Measure(unbounded)
		c.addTo.Arrange(render.Rect{X: btnX, Y: entryY, W: nat.W, H: entryH})
		c.addTo.OnClick = func() {
			if c.addPick != nil {
				c.addPick(c.Color())
			}
		}
		btnX = c.addTo.Bounds().X + c.addTo.Bounds().W + 8
	}
	if c.eyedrop != nil {
		nat := c.pickFrom.Measure(unbounded)
		c.pickFrom.Arrange(render.Rect{X: btnX, Y: entryY, W: nat.W, H: entryH})
		c.pickFrom.OnClick = c.eyedrop
	}
	setParents(c, c.hex)
	if c.custom != nil {
		setParents(c, c.addTo)
	}
	if c.eyedrop != nil {
		setParents(c, c.pickFrom)
	}
}

// Children exposes the entry (and the + and pick buttons when their
// sources are wired) for focus traversal.
func (c *ColorChooser) Children() []Widget {
	kids := []Widget{c.hex}
	if c.custom != nil {
		kids = append(kids, c.addTo)
	}
	if c.eyedrop != nil {
		kids = append(kids, c.pickFrom)
	}
	return kids
}

// appendChildren appends the entry and optional buttons, matching
// Children.
func (c *ColorChooser) appendChildren(buf []Widget) []Widget {
	buf = append(buf, c.hex)
	if c.custom != nil {
		buf = append(buf, c.addTo)
	}
	if c.eyedrop != nil {
		buf = append(buf, c.pickFrom)
	}
	return buf
}

// Paint draws the SV square as two stacked gradients, both strips, the
// cursors, and the palette rows - the square's arithmetic is the
// canvas's own premultiplied source-over.
func (c *ColorChooser) Paint(cv *render.Canvas) {
	th := Current()

	hueCol := hsvColor(c.hue, 1, 1, 1)
	cv.RoundedRect(c.sv, 4, th.Surface)
	cv.PaintGradient(c.sv, render.Corners{}, render.Linear(90, render.GradientStop{Pos: 0, Color: render.RGBA(255, 255, 255, 255)}, render.GradientStop{Pos: 1, Color: hueCol}))
	cv.PaintGradient(c.sv, render.Corners{}, render.Linear(180, render.GradientStop{Pos: 0, Color: 0}, render.GradientStop{Pos: 1, Color: render.RGBA(0, 0, 0, 255)}))

	// The hue strip: red through violet, top to bottom.
	cv.RoundedRect(c.hueStrip, 4, th.Surface)
	for i := range 160 {
		col := hsvColor(float64(i)/160, 1, 1, 1)
		cv.FillRect(render.Rect{X: c.hueStrip.X, Y: c.hueStrip.Y + i, W: c.hueStrip.W, H: 1}, col)
	}

	// The alpha strip: a checkerboard under the color-to-transparent
	// gradient, the standard visualization.
	cv.RoundedRect(c.alphaStrip, 4, th.Surface)
	for y := 0; y < 160; y += 8 {
		for x := 0; x < c.alphaStrip.W; x += 8 {
			grey := th.Surface
			if ((x/8)+(y/8))%2 == 0 {
				grey = th.SurfaceHover
			}
			cv.FillRect(render.Rect{X: c.alphaStrip.X + x, Y: c.alphaStrip.Y + y, W: 8, H: 8}, grey)
		}
	}
	cv.PaintGradient(c.alphaStrip, render.Corners{}, render.Linear(180, render.GradientStop{Pos: 0, Color: hsvColor(c.hue, c.sat, c.val, 1)}, render.GradientStop{Pos: 1, Color: hsvColor(c.hue, c.sat, c.val, 0)}))

	// Cursors: a ring on the square, bars on the strips.
	sx := c.sv.X + int(c.sat*float64(c.sv.W))
	sy := c.sv.Y + int((1-c.val)*float64(c.sv.H))
	cursorRing(cv, sx, sy)
	cursorBar(cv, c.hueStrip, int(c.hue*float64(c.hueStrip.H)), false)
	cursorBar(cv, c.alphaStrip, int((1-c.alpha)*float64(c.alphaStrip.H)), false)

	PaintChild(cv, c.hex)
	if c.custom != nil {
		PaintChild(cv, c.addTo)
	}

	// Palette rows: theme presets, then the session's picks.
	lineH := c.face.Shape("lg", c.sizePx).LineHeight()
	rowY := c.hex.Bounds().Y + c.hex.Bounds().H + 8
	c.paintSwatches(cv, c.presets, rowY)
	if c.custom != nil {
		c.paintSwatches(cv, c.custom(), rowY+lineH+10)
	}
}

// paintSwatches draws one row of clickable swatches, recording their
// rects for hit testing.
func (c *ColorChooser) paintSwatches(cv *render.Canvas, colors []render.Color, y int) {
	size := 18
	for i, col := range colors {
		r := render.Rect{X: c.sv.X + i*(size+6), Y: y, W: size, H: size}
		cv.RoundedRect(r, 4, col)
		if col == c.Color() {
			cv.BorderRect(r, 2, Current().Accent)
		}
	}
}

// cursorRing paints the SV square's cursor: a white ring with a dark
// edge, visible over any color.
func cursorRing(cv *render.Canvas, x, y int) {
	cv.FillRect(render.Rect{X: x - 4, Y: y - 4, W: 8, H: 8}, render.RGBA(0, 0, 0, 160))
	cv.BorderRect(render.Rect{X: x - 3, Y: y - 3, W: 6, H: 6}, 1, render.RGBA(255, 255, 255, 255))
}

// cursorBar paints a strip cursor: a horizontal marker line.
func cursorBar(cv *render.Canvas, strip render.Rect, pos int, horizontal bool) {
	_ = horizontal
	cv.FillRect(render.Rect{X: strip.X - 2, Y: strip.Y + pos, W: strip.W + 4, H: 2}, render.RGBA(255, 255, 255, 255))
	cv.FillRect(render.Rect{X: strip.X - 2, Y: strip.Y + pos - 1, W: strip.W + 4, H: 1}, render.RGBA(0, 0, 0, 160))
}

// swatchAt maps p to a palette color, if one is under it.
func (c *ColorChooser) swatchAt(p Point) (render.Color, bool) {
	lineH := c.face.Shape("lg", c.sizePx).LineHeight()
	rowY := c.hex.Bounds().Y + c.hex.Bounds().H + 8
	rows := [][]render.Color{c.presets}
	if c.custom != nil {
		rows = append(rows, c.custom())
	}
	for ri, row := range rows {
		y := rowY + ri*(lineH+10)
		for i, col := range row {
			if p.X >= c.sv.X+i*24 && p.X < c.sv.X+i*24+18 && p.Y >= y && p.Y < y+18 {
				return col, true
			}
		}
	}
	return 0, false
}

// Role implements Roleer.
func (c *ColorChooser) Role() Role { return RoleColorChooser }

// HitTest returns the entry or the + button under p, else the chooser
// (the square, strips, and swatches are its own geometry).
func (c *ColorChooser) HitTest(p Point) Widget {
	if hit := c.hex.HitTest(p); hit != nil {
		return hit
	}
	if c.custom != nil {
		if hit := c.addTo.HitTest(p); hit != nil {
			return hit
		}
	}
	if c.bounds.Contains(p.X, p.Y) {
		return c
	}
	return nil
}

// ClickAt applies one press-release: the square and strips set their
// component from the point, a swatch picks its color.
func (c *ColorChooser) ClickAt(p Point) {
	if !IsEnabled(c) {
		return
	}
	// The pixel-inclusive mappings: the last pixel row is the full
	// extent, so the extremes are reachable with integer coordinates.
	switch {
	case c.sv.Contains(p.X, p.Y):
		c.sat = clamp01(float64(p.X-c.sv.X) / float64(c.sv.W-1))
		c.val = 1 - clamp01(float64(p.Y-c.sv.Y)/float64(c.sv.H-1))
		c.colorChanged(false)
	case c.hueStrip.Contains(p.X, p.Y):
		c.hue = clamp01(float64(p.Y-c.hueStrip.Y) / float64(c.hueStrip.H-1))
		c.colorChanged(false)
	case c.alphaStrip.Contains(p.X, p.Y):
		c.alpha = 1 - clamp01(float64(p.Y-c.alphaStrip.Y)/float64(c.alphaStrip.H-1))
		c.colorChanged(false)
	default:
		if col, ok := c.swatchAt(p); ok {
			c.SetColor(col)
		}
	}
}

// DragMove tracks drags across the square and strips exactly like
// ClickAt. Disabled choosers do not drag.
func (c *ColorChooser) DragMove(p Point) {
	if !IsEnabled(c) {
		return
	}
	c.ClickAt(p)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// KeyAction implements KeyActionHandler: arrows move saturation and
// value, shift+arrows the hue, PageUp/PageDown step alpha, Home and End
// pin saturation. Disabled choosers ignore keys.
func (c *ColorChooser) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(c) {
		return
	}
	switch a {
	case KeyLeft, KeyRight:
		d := colorStep
		if a == KeyLeft {
			d = -colorStep
		}
		if mods&ModShift != 0 {
			c.hue = clamp01(c.hue + d)
		} else {
			c.sat = clamp01(c.sat + d)
		}
		c.colorChanged(false)
	case KeyUp, KeyDown:
		d := colorStep
		if a == KeyDown {
			d = -colorStep
		}
		c.val = clamp01(c.val + d)
		c.colorChanged(false)
	case KeyPriorPage:
		c.alpha = clamp01(c.alpha + 1.0/16)
		c.colorChanged(false)
	case KeyNextPage:
		c.alpha = clamp01(c.alpha - 1.0/16)
		c.colorChanged(false)
	case KeyHome:
		c.sat = 0
		c.colorChanged(false)
	case KeyEnd:
		c.sat = 1
		c.colorChanged(false)
	}
}
