package widget

import (
	"fmt"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// ColorButton shows the current color as a swatch on an outline icon
// button and opens a SwatchPicker under itself on click: a swatch
// picks and closes, the picker's Custom entry edits any color in place.
// Presets are the swatches offered, DefaultSwatches until set; Custom,
// when set, lists the user's own colors under them.
type ColorButton struct {
	Button
	face   render.Font
	sizePx float64
	swatch *ColorSwatch
	// Presets are the picker's swatches.
	Presets []render.Color
	// Custom, when set, lists colors for the picker's custom row.
	Custom func() []render.Color
	// OnColorSet runs when the user picked or edited a color, not on
	// SetColor.
	OnColorSet func(c render.Color)
	// close dismisses the open picker, nil while closed.
	close func()
}

// NewColorButton returns a button showing c, its picker's text in face
// at sizePx.
func NewColorButton(face render.Font, sizePx float64, c render.Color) *ColorButton {
	face = requireFace("widget.NewColorButton", face)
	sw := NewColorSwatch(c)
	sw.SetHAlign(AlignCenter)
	sw.SetVAlign(AlignCenter)
	b := &ColorButton{Button: *NewButton(sw, 0, 0), face: face, sizePx: sizePx, swatch: sw, Presets: DefaultSwatches()}
	b.AddClass(css.Outline, css.Icon, "colorbutton")
	b.SetAccessibleName(FormatColor(c))
	b.OnClick = b.open
	return b
}

// open shows the picker under the button.
func (b *ColorButton) open() {
	p := NewSwatchPicker(b.face, b.sizePx, b.Presets, b.swatch.color)
	if b.Custom != nil {
		p.SetCustom(b.Custom())
	}
	p.OnPicked = func(c render.Color) {
		b.Choose(c)
		if b.close != nil {
			b.close()
		}
	}
	p.OnEdited = b.Choose
	b.close = OpenPopover(b, b, p, PopoverOptions{Gap: PopoverGap, OnClosed: func() { b.close = nil }})
}

// Color returns the shown color.
func (b *ColorButton) Color() render.Color { return b.swatch.color }

// SetColor shows c without firing OnColorSet.
func (b *ColorButton) SetColor(c render.Color) {
	b.swatch.SetColor(c)
	b.SetAccessibleName(FormatColor(c))
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
