// The enter/exit visual half of a popup: a widget.Morph the
// coordinator drives. The popup grows out of the edge facing its
// anchor - scaling up while a rounded clip unrolls away from the
// anchor and the surface fades in - with its shadow and plate drawn for
// the unrolled shape, and shrinks back into the anchor on the way out.
// Everything stays inside the popup's own surface.
package popup

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// animView is the popup's animated paint wrapper around the content
// root Run was handed.
type animView struct {
	morph *widget.Morph
}

// newAnimView wraps root for motion painting. reveal and morph seed
// the wrapper - the coordinator may already hold a frame from an Enter
// that ran before the content existed (zero-duration enters land
// inside New). The motion grows from the content edge facing the
// anchor; plate paints the shadow and plate for a content rect inside
// the inset gutter.
func newAnimView(root widget.Widget, reveal float64, morph bool, g Gravity, inset int, plate func(cv *render.Canvas, content render.Rect)) *animView {
	m := widget.NewMorph(root)
	m.SetOrigin(originOf(g))
	m.SetRadius(widget.Current().Radius)
	m.SetPlate(inset, plate)
	m.SetReveal(reveal, morph)
	return &animView{morph: m}
}

// originOf is the edge the popup grows from: the one facing its
// anchor. Below its anchor (bottom gravity) it grows down from its top
// edge, above it up from its bottom edge, and so on.
func originOf(g Gravity) (float64, float64) {
	switch g {
	case GravityTop:
		return 0.5, 1
	case GravityLeft:
		return 1, 0.5
	case GravityRight:
		return 0, 0.5
	default:
		return 0.5, 0
	}
}

// setProgress applies one motion frame.
func (a *animView) setProgress(reveal float64, morph bool) {
	a.morph.SetReveal(reveal, morph)
}
