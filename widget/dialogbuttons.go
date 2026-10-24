package widget

import (
	"fmt"

	"github.com/stubbedev/lys-ui/render"
)

// ColorButton is GTK's ColorDialogButton: a swatch of the current
// color that opens a color chooser on click. OnOpen is the opening
// hook (app.Application.AttachColorButton installs the chooser
// dialog); Choose applies the user's pick and fires OnColorSet.
type ColorButton struct {
	Button
	swatch *colorSwatch
	// OnOpen runs on click to show a chooser starting at the color.
	OnOpen func(current render.Color)
	// OnColorSet runs when the user picked a color (Choose), not on
	// SetColor.
	OnColorSet func(c render.Color)
}

type colorSwatch struct {
	node
	color render.Color
}

func (s *colorSwatch) Measure(Constraints) Size { return Size{W: 32, H: 18} }

func (s *colorSwatch) Paint(cv *render.Canvas) {
	r := s.Bounds()
	cv.RoundedRect(r, 4, Current().Border)
	cv.RoundedRect(render.UniformInsets(1).Shrink(r), 3, s.color)
}

func (s *colorSwatch) HitTest(p Point) Widget { return s.HitLeaf(s, p) }

// NewColorButton returns a button showing c.
func NewColorButton(c render.Color) *ColorButton {
	sw := &colorSwatch{color: c}
	b := &ColorButton{Button: *NewButton(sw, 6, 6), swatch: sw}
	b.SetElement("colorbutton")
	b.OnClick = func() {
		if b.OnOpen != nil {
			b.OnOpen(b.swatch.color)
		}
	}
	return b
}

// Color returns the shown color.
func (b *ColorButton) Color() render.Color { return b.swatch.color }

// SetColor shows c without firing OnColorSet.
func (b *ColorButton) SetColor(c render.Color) {
	if b.swatch.color != c {
		b.swatch.color = c
		b.swatch.Invalidate()
	}
}

// Choose applies a color the user picked: it shows it and fires
// OnColorSet.
func (b *ColorButton) Choose(c render.Color) {
	b.SetColor(c)
	if b.OnColorSet != nil {
		b.OnColorSet(c)
	}
}

// BindColor wires the binding two-way to the shown color: changes show,
// picks write back.
func (b *ColorButton) BindColor(binding *Binding[render.Color]) (unbind func()) {
	return bindWidget(binding, b.SetColor, func(fn func(render.Color)) { b.OnColorSet = fn })
}

// FontChoice is a font family and size, the FontButton's value.
type FontChoice struct {
	Family string
	Size   float64
}

// String renders the choice as the button shows it: "Inter 13".
func (f FontChoice) String() string { return fmt.Sprintf("%s %g", f.Family, f.Size) }

// FontButton is GTK's FontDialogButton: a button naming the current
// family and size that opens a font chooser on click. OnOpen is the
// opening hook (app.Application.AttachFontButton installs the chooser
// dialog); Choose applies the user's pick and fires OnFontSet.
type FontButton struct {
	Button
	label  *Label
	choice FontChoice
	// OnOpen runs on click to show a chooser starting at the choice.
	OnOpen func(current FontChoice)
	// OnFontSet runs when the user picked a font (Choose), not on
	// SetFont.
	OnFontSet func(f FontChoice)
}

// NewFontButton returns a button naming initial, its label in face at
// sizePx.
func NewFontButton(face render.Font, sizePx float64, initial FontChoice) *FontButton {
	label := NewLabel(face, sizePx, initial.String(), 0)
	b := &FontButton{Button: *NewButton(label, 6, 6), label: label, choice: initial}
	b.SetElement("fontbutton")
	b.OnClick = func() {
		if b.OnOpen != nil {
			b.OnOpen(b.choice)
		}
	}
	return b
}

// Font returns the shown choice.
func (b *FontButton) Font() FontChoice { return b.choice }

// SetFont shows f without firing OnFontSet.
func (b *FontButton) SetFont(f FontChoice) {
	b.choice = f
	b.label.SetText(f.String())
}

// Choose applies a font the user picked: it shows it and fires
// OnFontSet.
func (b *FontButton) Choose(f FontChoice) {
	b.SetFont(f)
	if b.OnFontSet != nil {
		b.OnFontSet(f)
	}
}

// BindFont wires the binding two-way to the shown choice.
func (b *FontButton) BindFont(binding *Binding[FontChoice]) (unbind func()) {
	return bindWidget(binding, b.SetFont, func(fn func(FontChoice)) { b.OnFontSet = fn })
}
