package widget

import "github.com/stubbedev/lys-ui/render"

// tooltipTextPx is a tooltip's text size, shadcn's text-xs.
const tooltipTextPx = 12

// NewTooltipCard is a tooltip's card (shadcn/ui's TooltipContent): the
// text, parsed as markup when markup is set, in the small size on the
// primary fill. It styles as the `tooltip` element.
func NewTooltipCard(face render.Font, text string, markup bool) *Box {
	face = requireFace("widget.NewTooltipCard", face)
	var label Widget = NewLabel(face, tooltipTextPx, text, 0)
	if markup {
		label = NewRichLabel(face, tooltipTextPx, text, 0)
	}
	card := NewBox(Row, 0, 0)
	card.SetElement("tooltip")
	card.Append(label, false)
	return card
}
