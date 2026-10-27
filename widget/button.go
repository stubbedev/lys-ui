package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Button is a clickable region with a rounded background and one child,
// usually a Box of an icon and a label. Input arrives in M4; the hover and
// pressed states are plain fields so tests and future input code drive
// them directly.
type Button struct {
	node
	child   Widget
	padding int
	radius  int

	// Background colors, one per visual state. A zero color means
	// "unset": the stylesheet's background-color, else the theme
	// surface for that state.
	Bg        render.Color
	BgHover   render.Color
	BgPressed render.Color

	// BgExplicit makes Bg, BgHover, and BgPressed literal: a zero color
	// is transparent instead of unset, so a button can rest on
	// whatever sits behind it and fill only on hover. Like any
	// programmatic color it outranks the stylesheet. A transparent
	// fill under a stylesheet border strokes just the rounded ring
	// (render.Canvas.RoundedBorder).
	BgExplicit bool

	// Hovered and Pressed select the painted state; Pressed wins.
	Hovered, Pressed bool

	// OnClick fires from Click, the hook M4 input will call.
	OnClick func()
}

// NewButton returns a button wrapping child with the given inner padding
// and corner radius. The pointer turns to the hand over it: every
// button is clickable, the way wayle's button primitives ask.
func NewButton(child Widget, padding, radius int) *Button {
	b := &Button{child: child, padding: padding, radius: radius}
	b.cursorName = "pointer"
	return b
}

// Measure pads the child's natural size inside the CSS box: the
// constructor padding (or the stylesheet's, per side), border, margin,
// and min sizes.
func (b *Button) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := b.measureHit(axis, forSize); ok {
		return sz
	}
	v := b.style(b)
	return b.measureStore(axis, forSize, measureBox(v, b.box(v), axis, forSize, func(axis Axis, forSize int) Sizes {
		return measureChild(b, b.child, axis, forSize)
	}))
}

// box is the button's resolved CSS box.
func (b *Button) box(v *style.Values) cssInsets {
	return boxOf(v, render.UniformInsets(b.padding))
}

// Arrange insets the child by the box inside r, the margin box; Bounds
// records the border box.
func (b *Button) Arrange(r render.Rect) {
	border, inner := boxRects(b.box(b.style(b)), r)
	b.ArrangeRoot(border)
	arrangeChild(b.child, inner)
	setParents(b, b.child)
}

// ArrangeRoot records the button's own rect.
func (b *Button) ArrangeRoot(r render.Rect) {
	b.node.Arrange(r)
}

// Paint draws the state's background, then the child. A zero Bg family
// falls back to the theme. Disabled, the fill is the derived disabled
// surface and the child paints through the shared disabled fade, so
// whatever a button wraps (label, icon) mutes without the child
// knowing about the state.
//
// The stylesheet layers between the programmatic colors and the theme
// per state slot; its shadows, background image, border ring (rounded,
// per side), and outline paint around the fill, and opacity and filter
// wrap the whole button.
func (b *Button) Paint(cv *render.Canvas) {
	t := Current()
	v := b.style(b)
	var bg, prog render.Color
	switch {
	case !IsEnabled(b):
		bg, prog = t.DisabledSurface(), b.Bg
	case b.Pressed:
		bg, prog = t.PressedSurface(), b.BgPressed
	case b.Hovered:
		bg, prog = t.HoverSurface(), b.BgHover
	case b.HasState(StateChecked):
		// A checked toggle rests in the selected shade unless a
		// :checked rule paints it.
		bg, prog = t.HoverSurface(), b.Bg
	default:
		bg, prog = t.Surface, b.Bg
	}
	fill := pickc(prog, v, style.PropBackgroundColor, bg)
	if b.BgExplicit {
		fill = prog
	}
	radii := radiusOr(v, b.radius)
	fx := pushEffects(cv, v, b.bounds, radii)
	cv = fx.cv
	paintBoxBehind(cv, v, b.bounds, radii, borderOf(v), fill)
	if IsEnabled(b) {
		PaintChild(cv, b.child)
	} else {
		a := cv.PushAlpha(disabledFade)
		PaintChild(cv, b.child)
		cv.PopAlpha(a)
	}
	paintOutline(cv, v, b.bounds, radii)
	fx.pop()
}

// Role implements Roleer.
func (b *Button) Role() Role { return RoleButton }

// HitTest returns the button when p is inside its bounds.
func (b *Button) HitTest(p Point) Widget {
	return b.HitLeaf(b, p)
}

// ClickAt fires the OnClick hook, ignoring the release point. It is a
// no-op without one, and a no-op while disabled: a dead button never
// fires.
func (b *Button) ClickAt(p Point) {
	b.click()
}

// click fires the hook from ClickAt and KeyAction. Disabled buttons
// swallow clicks, keys, and activation runes alike.
func (b *Button) click() {
	if !IsEnabled(b) {
		return
	}
	if b.OnClick != nil {
		b.OnClick()
	}
}

// SetHovered implements HoverSetter; the hover shade repaints.
func (b *Button) SetHovered(on bool) {
	if b.Hovered == on {
		return
	}
	b.Hovered = on
	b.invalidateState(style.Hover)
}

// SetPressed implements PressSetter; the pressed shade repaints.
func (b *Button) SetPressed(on bool) {
	if b.Pressed == on {
		return
	}
	b.Pressed = on
	b.invalidateState(style.Active)
}

// KeyAction implements KeyActionHandler: Enter activates the button
// when focused, which is what makes it reachable by Tab at all - the
// traversal only collects KeyActionHandler implementations.
func (b *Button) KeyAction(a KeyAction, _ Mods) {
	if a == KeyEnter {
		b.click()
	}
}

// InsertRune implements RuneHandler: Space activates the button, the
// other half of the GTK activation pair. Text never reaches a focused
// button, so a blank rune can only mean "activate".
func (b *Button) InsertRune(r rune) {
	if r == ' ' {
		b.click()
	}
}

// Children returns the button's content.
func (b *Button) Children() []Widget { return b.appendChildren(nil) }

// appendChildren is Children into buf, for the walks that reuse one.
func (b *Button) appendChildren(buf []Widget) []Widget {
	if b.child == nil {
		return buf
	}
	return append(buf, b.child)
}

// appendStyleKids is the content the button's style passes down to
// (styleKids); a method so an embedding type keeps it.
func (b *Button) appendStyleKids(buf []Widget) []Widget { return append(buf, b.child) }
