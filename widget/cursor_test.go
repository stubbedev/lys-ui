package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// Regression: the client never told the compositor which cursor to
// show, so hover surfaces fell back to whatever the compositor picks -
// on Hyprland a text caret over the entire window. Widgets now declare
// their pointer shape and the app applies it on hover.
func TestCursorNames(t *testing.T) {
	t.Run("text fields ask for the caret", func(t *testing.T) {
		for _, w := range []CursorNamer{
			NewEntry(entryFace(t), 14, render.RGB(255, 255, 255)),
			NewTextArea(entryFace(t), 13, render.RGB(255, 255, 255)),
		} {
			if got := w.CursorName(); got != "xterm" {
				t.Errorf("%T cursor = %q, want xterm", w, got)
			}
		}
	})

	t.Run("plain widgets do not ask for a shape", func(t *testing.T) {
		if _, wants := Widget(NewSpacer(4, 4)).(CursorNamer); wants {
			t.Error("spacer requests a cursor shape")
		}
	})

	// The clickable primitives ask for the hand, the way wayle's
	// shared widgets set set_cursor_from_name(Some("pointer")) on
	// every control.
	t.Run("clickable primitives ask for the pointer", func(t *testing.T) {
		for _, w := range []Widget{
			NewButton(NewLabel(entryFace(t), 12, "go", render.RGB(255, 255, 255)), 4, 4),
			NewSwitch(false),
			NewCheckButton(false),
			NewSlider(0, 1, 0, 0.5),
		} {
			if got := CursorNameOf(w); got != "pointer" {
				t.Errorf("%T cursor = %q, want pointer", w, got)
			}
		}
	})
}
