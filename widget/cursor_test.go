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
			if got := HoverCursor(w); got != "pointer" {
				t.Errorf("%T cursor = %q, want pointer", w, got)
			}
		}
	})
}

func TestHoverCursorFollowsWhereAClickLands(t *testing.T) {
	label := func() *Label { return NewLabel(entryFace(t), 12, "x", render.RGB(255, 255, 255)) }

	t.Run("a leaf inside a click-within box asks for the hand", func(t *testing.T) {
		leaf := label()
		box := NewBox(Row, 0, 0)
		box.Append(leaf, false)
		box.SetOnClickWithin(func() {})
		arrangeTree(t, box, 100, 40)
		if got := HoverCursor(leaf); got != "pointer" {
			t.Errorf("leaf cursor = %q, want pointer", got)
		}
	})

	t.Run("a leaf in a plain box keeps the arrow", func(t *testing.T) {
		leaf := label()
		box := NewBox(Row, 0, 0)
		box.Append(leaf, false)
		arrangeTree(t, box, 100, 40)
		if got := HoverCursor(leaf); got != "" {
			t.Errorf("leaf cursor = %q, want the arrow", got)
		}
	})

	t.Run("a nil hover keeps the arrow", func(t *testing.T) {
		if got := HoverCursor(nil); got != "" {
			t.Errorf("nil cursor = %q, want the arrow", got)
		}
	})

	t.Run("a clicker that names nothing asks for the hand", func(t *testing.T) {
		if got := HoverCursor(NewDropdown(entryFace(t), 12, []string{"a"}, 0)); got != "pointer" {
			t.Errorf("dropdown cursor = %q, want pointer", got)
		}
	})

	t.Run("a clicker only clickable in part keeps the arrow", func(t *testing.T) {
		for _, w := range []Widget{NewScroll(label()), NewItem(entryFace(t), "a", ""), NewActionRow(entryFace(t), 12, "a", "")} {
			if got := HoverCursor(w); got != "" {
				t.Errorf("%T cursor = %q, want the arrow", w, got)
			}
		}
	})

	t.Run("a writable field asks for the caret", func(t *testing.T) {
		for _, w := range []Widget{
			NewInputOTP(entryFace(t), OTPDigits, 6),
			NewEditableLabel(entryFace(t), 12, render.RGB(255, 255, 255), "x"),
		} {
			if got := HoverCursor(w); got != "xterm" {
				t.Errorf("%T cursor = %q, want xterm", w, got)
			}
		}
	})
}

func TestEntryIconAsksForTheHand(t *testing.T) {
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	arrangeTree(t, e, 200, 40)
	if got := HoverCursor(e); got != "xterm" {
		t.Errorf("field cursor = %q, want xterm", got)
	}
	e.trailing, e.trailingClick = NewSpacer(16, 16), func(Point) {}
	r := e.trailingRect()
	if r.Empty() {
		t.Fatal("no trailing icon rect")
	}
	e.HoverMove(Point{X: r.X + 1, Y: r.Y + 1})
	if got := HoverCursor(e); got != "pointer" {
		t.Errorf("icon cursor = %q, want pointer", got)
	}
}
