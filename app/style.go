package app

import (
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// App-level CSS surface names (docs/css.md): the element-name list's
// app half. Each surface names its card for the stylesheet in its
// constructor; toast and the tooltip card are widgets and name
// themselves. The dialog's
// card is the floating plate (Elevation) when client shadows paint and
// the plain root box when they do not.
const (
	elemDialog  = "dialog"
	elemPopover = "popover"
)

// popoverClass is the style class GtkPopover's node carries; a
// stylesheet styling `popover.background` takes over the popover's
// plate.
const popoverClass = "background"

// nameSurfaceElement names w's card for the stylesheet. Content a
// caller hands over (a popover's, a tooltip's) is node-backed in
// practice; anything else simply never matches surface selectors.
func nameSurfaceElement(w widget.Widget, surface string) {
	if c, ok := w.(interface{ SetElement(string) }); ok {
		c.SetElement(surface)
	}
}

// dialogCard builds the dialog's card from its content box: with client
// shadows on, a floating Elevation (theme shadow, rounded plate, on
// transparent window margins so the falloff blends over whatever is
// behind); shadows off, the content edge to edge on the window
// background. Hit-testing stays on the card — the gutter is never a
// hit. Either way the card is named for the stylesheet's `dialog`
// element (docs/css.md).
func dialogCard(root *widget.Box) (widget.Widget, render.Color) {
	background := widget.Current().Bg
	if gutter := widget.Current().ShadowGutter(); gutter > 0 {
		card := widget.NewElevation(root).
			WithRadius(widget.Current().Radius).
			WithPlate(background)
		nameSurfaceElement(card, elemDialog)
		margin := widget.NewBox(widget.Column, 0, gutter)
		margin.Append(card, true)
		return margin, 0
	}
	nameSurfaceElement(root, elemDialog)
	return root, background
}
