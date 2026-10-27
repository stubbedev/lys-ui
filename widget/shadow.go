// Box shadows for floating surfaces. Menus, popovers, tooltips, toasts,
// and dialogs all read their elevation from here, so one theme knob
// (ShadowBlur, ShadowColor) restyles — or disables — every shadow at
// once, and the look never drifts per widget.
//
// Two invariants hold everywhere:
//
//   - The shadow paints OUTSIDE the widget's logical bounds. Layout,
//     hit-testing, and the bounds the damage collector sees are never
//     widened by it: a point in the shadow gutter is not a hit, the
//     one rule that keeps click-away dismissal and padding math
//     honest.
//   - The pixels a shadow owes beyond the bounds are tracked as a
//     damage ring (shadowTracker), invalidated only when the bounds
//     move or the theme's shadow restyles — a hover twitch repaints
//     the card, never the falloff.
package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// defaultShadowColor is the derived shadow paint for themes that set a
// blur but no color: neutral black at 110 alpha.
var defaultShadowColor = render.RGBA(0, 0, 0, 110)

// shadowPaint resolves the shadow's paint at full strength:
// premultiplied color and blur radius in logical pixels. blur 0 means
// disabled — the theme's ShadowBlur is the on/off switch; an unset
// color derives the default. Weaker strengths (the toast's fade)
// scaleAlpha the result.
func (t *Theme) shadowPaint() (render.Color, int) {
	if t.ShadowBlur <= 0 {
		return 0, 0
	}
	return t.or(t.ShadowColor, defaultShadowColor), t.ShadowBlur
}

// DrawShadow paints the theme's box shadow around r — a widget's
// arranged bounds — rounding at the given corner radius. It is the one
// elevation helper every floating surface paints through; disabled or
// fully transparent shadows paint nothing.
func (t *Theme) DrawShadow(cv *render.Canvas, r render.Rect, radius int) {
	col, blur := t.shadowPaint()
	if blur == 0 {
		return
	}
	cv.Shadow(r, radius, blur, col)
}

// ShadowRing returns the rect the shadow around r covers: the damage
// ring a floating widget owes beyond its bounds (the blur plus the
// anti-aliasing rim, matching the raster's box). Empty when the shadow
// is disabled, which callers treat as "no extra pixels owed".
func (t *Theme) ShadowRing(r render.Rect) render.Rect {
	_, blur := t.shadowPaint()
	return ringFor(r, blur)
}

// ringFor is the damage ring for an explicit blur — the
// stylesheet-overridden blur a floating surface paints with.
func ringFor(r render.Rect, blur int) render.Rect {
	if blur <= 0 {
		return render.Rect{}
	}
	return expandRect(r, blur+1)
}

// ShadowGutter reports how much room a surface must reserve around its
// content for the theme's shadow: the blur when shadows paint, else
// zero. Popups grow their surface by it on every side and dialogs
// inset their card by it; content size, placement, and hit-testing
// stay on the un-guttered logical rect.
func (t *Theme) ShadowGutter() int {
	_, blur := t.shadowPaint()
	return blur
}

// expandRect grows r by n on every side.
func expandRect(r render.Rect, n int) render.Rect {
	return render.Rect{X: r.X - n, Y: r.Y - n, W: r.W + 2*n, H: r.H + 2*n}
}

// shadowTracker remembers the ring a floating widget last owed damage
// for and the theme generation it was owed under. Embed it in any
// widget whose Paint draws outside its bounds, and call sync from
// Arrange — which every frame runs before the damage walk.
type shadowTracker struct {
	last render.Rect
	gen  uint64
}

// syncRing is the tracker's entry: re-invalidates the ring when the
// bounds moved or resized (the old pixels must be erased and the new
// painted), the theme's shadow restyled (SetTheme bumps the
// generation), or the stylesheet overrode the blur — callers pass the
// ring the surface effectively paints with. Steady state — same
// bounds, same generation — is a no-op, so hovering a menu never
// re-invalidates the falloff.
func (s *shadowTracker) syncRing(n *node, ring render.Rect) {
	if ring == s.last && s.gen == themeGen {
		return
	}
	if !s.last.Empty() {
		n.InvalidateRect(s.last)
	}
	if !ring.Empty() {
		n.InvalidateRect(ring)
	}
	s.last, s.gen = ring, themeGen
}

// Elevation wraps a widget and paints the theme's box shadow around
// it — the widget-shaped half of the shared elevation helper, for
// floating cards whose content does not paint its own plate. WithPlate
// adds a rounded fill behind the child, so a bare content tree becomes
// a floating card: shadow, plate, content (the dialog's shape).
//
// The wrapper claims exactly the child's rect: Measure and Arrange
// pass through untouched — the shadow lives outside the logical
// bounds — and HitTest resolves through the child alone, so a point in
// the gutter is never a hit.
type Elevation struct {
	node
	child  Widget
	radius int
	plate  Color
	ring   shadowTracker
}

// NewElevation wraps child, rounding at the theme's corner radius.
func NewElevation(child Widget) *Elevation {
	return &Elevation{child: child, radius: Current().Radius}
}

// WithRadius sets the corner radius the shadow and plate round at.
func (e *Elevation) WithRadius(px int) *Elevation { e.radius = px; return e }

// WithPlate sets a rounded fill painted between the shadow and the
// child; zero (the default) paints no plate.
func (e *Elevation) WithPlate(c Color) *Elevation { e.plate = c; return e }

// Child returns the wrapped widget.
func (e *Elevation) Child() Widget { return e.child }

// Measure passes through to the child.
func (e *Elevation) Measure(axis Axis, forSize int) Sizes {
	return measureChild(e, e.child, axis, forSize)
}

// Arrange records the rect, passes it to the child, and keeps the
// shadow ring's damage in step with the effective (stylesheet-aware)
// blur.
func (e *Elevation) Arrange(r render.Rect) {
	e.node.Arrange(r)
	arrangeChild(e.child, r)
	setParents(e, e.child)
	_, blur := effShadow(e, Current())
	e.ring.syncRing(&e.node, ringFor(e.bounds, blur))
}

// Paint draws shadow, optional plate and border, then the child. The
// stylesheet's box-shadow, background-color (the plate a bare content
// tree floats on), border and border-radius override the constructor
// and theme values — the dialog and popover cards style through this.
func (e *Elevation) Paint(cv *render.Canvas) {
	t := Current()
	v := e.style(e)
	radii := radiusOr(v, e.radius)
	if col, blur := effShadow(e, t); blur > 0 {
		cv.Shadow(e.bounds, radii.TopLeft, blur, col)
	}
	paintBoxBehind(cv, v, e.bounds, radii, borderOf(v), pickc(0, v, style.PropBackgroundColor, e.plate))
	PaintChild(cv, e.child)
}

// HitTest resolves through the child alone: the shadow gutter is never
// a hit, so clicks just outside the card pass through.
func (e *Elevation) HitTest(p Point) Widget { return HitChild(e.child, p) }

// Children exposes the child for the damage collector, focus
// traversal, and the accessibility walk.
func (e *Elevation) Children() []Widget { return []Widget{e.child} }

// appendChildren appends the child, matching Children (a nil child is
// appended and skipped by the walk, as the slice form always was).
func (e *Elevation) appendChildren(buf []Widget) []Widget { return append(buf, e.child) }

// SetEnabled turns the elevation's child on or off through the
// per-query enable walk, like Box.
func (e *Elevation) SetEnabled(enabled bool) {
	e.node.SetEnabled(enabled)
	invalidateTree(e)
}
