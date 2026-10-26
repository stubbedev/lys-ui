package widget

import "github.com/stubbedev/lys-lib/render"

// SetColor sets the label's programmatic ink, the dynamic counterpart
// of NewLabel's color argument: modules that restyle per state (battery
// thresholds, workspace focus) call it instead of rebuilding the label.
// A stylesheet color declaration still outranks it, per the cascade;
// 0 is the no-color value and falls through to the theme.
func (l *Label) SetColor(c render.Color) {
	if l.color == c {
		return
	}
	l.color = c
	l.Invalidate()
}

// Color returns the label's current programmatic ink, or 0 when the
// label inherits.
func (l *Label) Color() render.Color { return l.color }

// inkOr is a text widget's ink when nothing in the cascade colors it:
// its programmatic color, else the theme's text color at paint time,
// so a text widget built without a color follows theme switches.
func inkOr(c render.Color) render.Color {
	if c != 0 {
		return c
	}
	return Current().Text
}
