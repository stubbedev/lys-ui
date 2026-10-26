// Subtree opacity: the embeddable state and the PushAlpha discipline
// around child paint. A fade today would mean threading an alpha factor
// through every widget color by hand; an Opacity-carrying widget pushes
// the factor onto the canvas instead (render.Canvas.PushAlpha), so every
// color its subtree produces — fills, borders, text, image blits — is
// modulated in one place, and nested carriers multiply.
package widget

import "github.com/stubbedev/lys-lib/render"

// Opacity carries a widget's subtree opacity: a factor in [0, 1] the
// widget applies around its children's paint. Widgets that wrap children
// embed it beside node, call bindOpacity from their constructor, and
// route child painting through paintChild. The factor lands on the
// canvas's PushAlpha stack, so nested carriers compose multiplicatively
// and the modulation reaches every blended primitive by construction.
//
// The type has no Opacity getter on purpose: an accessor named like the
// embedded field would shadow the embedding itself (toast.Opacity()
// would collide with the promoted Opacity field). Alpha is the getter.
type Opacity struct {
	alpha float64
	// host is the embedding widget; SetOpacity invalidates its bounds.
	host interface{ Invalidate() }
}

// bindOpacity records the embedding widget so SetOpacity can invalidate
// it. Call once from the embedding widget's constructor.
func (o *Opacity) bindOpacity(host interface{ Invalidate() }) { o.host = host }

// SetOpacity sets the subtree opacity, clamped to [0, 1] — a spring
// tween's overshoot saturates instead of over-brightening. A change
// invalidates the host's bounds, so the next frame repaints the subtree
// under the new factor. Zero is a state, not a special case: painting
// it skips the subtree entirely, which is what makes tween endpoints
// free. It composes with the animation clock directly — a tween
// callback is just SetOpacity.
func (o *Opacity) SetOpacity(a float64) {
	a = min(1, max(0, a))
	if o.alpha == a {
		return
	}
	o.alpha = a
	if o.host != nil {
		o.host.Invalidate()
	}
}

// Alpha returns the subtree opacity in [0, 1]. The zero value is fully
// transparent, so embedding constructors start it at 1.
func (o *Opacity) Alpha() float64 { return o.alpha }

// paintChild paints the wrapped subtree under this opacity. At zero it
// paints nothing — not even a touch of the canvas's pixel counter,
// which is the paint-count harness's skip proof. At one it paints
// directly, no push. In between it pushes the factor onto the canvas
// for the subtree's paint and pops it after, so every color the
// subtree produces is modulated and nothing outside it is.
func (o *Opacity) paintChild(cv *render.Canvas, child Widget) {
	switch {
	case o.alpha <= 0:
		return
	case o.alpha >= 1:
		PaintChild(cv, child)
	default:
		prev := cv.PushAlpha(o.alpha)
		PaintChild(cv, child)
		cv.PopAlpha(prev)
	}
}
