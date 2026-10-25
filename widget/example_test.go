// Package widget_test holds the godoc examples for the widget package.
// Entry, TextArea, and Menu are plain in-memory objects, so their
// examples really run under go test; the Output blocks pin the API's
// behavior, not just its shape.
package widget_test

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

func face() *render.Typeface {
	tf, err := render.LoadFont(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
	return tf
}

func ExampleEntry() {
	// An entry is an editing state machine: callbacks see every
	// change, whatever produced it (typing, editing keys, clipboard,
	// SetText).
	name := widget.NewEntry(face(), 14, widget.Current().Text)
	name.SetPlaceholder("who goes there?")
	name.OnChanged = func(s string) { fmt.Println("changed:", s) }

	name.InsertRune('h')
	name.InsertRune('i')
	fmt.Println("text:", name.Text())
	// Output:
	// changed: h
	// changed: hi
	// text: hi
}

func ExampleTextArea() {
	// A text area holds logical lines and soft-wraps them at the
	// offered width by default (SetWrap(false) restores one row per
	// line). Editing stays in logical coordinates; Text reports them.
	notes := widget.NewTextArea(face(), 14, widget.Current().Text)
	notes.SetText("wrap me softly\nline two")

	fmt.Println(notes.Text())
	fmt.Println("--")

	// Place the caret on the second line, after "two", and type.
	notes.SetCursor(1, 8)
	notes.InsertRune('!')
	fmt.Println(notes.Text())
	// Output:
	// wrap me softly
	// line two
	// --
	// wrap me softly
	// line two!
}

func ExampleMenu() {
	// Menus are vertical action lists for popups: rows carry an
	// accelerator label (display-only), check and radio state, and
	// separators that keyboard motion skips.
	wrap := false
	menu := widget.NewMenu(face(), 13,
		widget.MenuItem{Label: "Cut", Accel: "Ctrl+X", OnClick: func() { fmt.Println("cut") }},
		widget.MenuItem{Label: "Copy", Accel: "Ctrl+C", OnClick: func() { fmt.Println("copy") }},
		widget.MenuSeparator(),
		widget.MenuItem{Label: "Word wrap", Kind: widget.ItemCheck, OnClick: func() { wrap = !wrap }},
	)

	// Keyboard: the first Down lands on the first selectable row, the
	// second walks past the separator, Enter activates. The host
	// closes the popup through OnDismiss; Esc maps to KeyDismiss.
	menu.KeyAction(widget.KeyDown, 0)
	menu.KeyAction(widget.KeyDown, 0)
	menu.KeyAction(widget.KeyDown, 0)
	menu.KeyAction(widget.KeyEnter, 0)
	fmt.Println("wrap:", wrap)
	// Output:
	// wrap: true
}

// popoverHost is a stand-in for the application's surface host: it
// hands every popover's keys to the widget the popover focuses.
type popoverHost struct{ focus widget.KeyActionHandler }

func (h *popoverHost) OpenMenu(widget.Boundser, render.Font, float64, []widget.MenuItem, func()) {}

func (h *popoverHost) OpenPopover(_ widget.Boundser, _ widget.Widget, opts widget.PopoverOptions) func() {
	h.focus = opts.Focus.(widget.KeyActionHandler)
	return func() {}
}

func (h *popoverHost) Focus(widget.Widget) {}

func ExampleDropdown() {
	// A dropdown is a closed face showing the selection; Enter, Space,
	// or Down opens the item list in a popover beneath it while
	// focused, through the surface host of its tree. The list holds the
	// focus: arrows move the highlight and Enter picks. Here the list
	// opens from the keyboard, Down walks to the second row, and Enter
	// commits it exactly once.
	dd := widget.NewDropdown(face(), 13, []string{"Light", "Dark", "System"}, 0)
	host := &popoverHost{}
	widget.SetSurfaceHost(dd, host)

	var picked string
	dd.OnSelect = func(int) { picked = dd.Selection() }
	dd.KeyAction(widget.KeyEnter, 0)
	host.focus.KeyAction(widget.KeyDown, 0)
	host.focus.KeyAction(widget.KeyEnter, 0)
	fmt.Println("picked:", picked)

	// DropdownOf carries typed values behind the labels, so handlers
	// see the value, not an index to look up.
	outputs := []widget.DropdownItem[string]{
		{Label: "eDP-1", Value: "/dev/drm0"},
		{Label: "HDMI-A-1", Value: "/dev/drm1"},
	}
	selector := widget.NewDropdownOf(face(), 13, outputs, 0)
	selector.OnSelect = func(i int, dev string) {
		fmt.Println("output", i, "->", dev)
	}
	selector.SetSelected(1)
	fmt.Println("value:", selector.Value())
	// Output:
	// picked: Dark
	// output 1 -> /dev/drm1
	// value: /dev/drm1
}
