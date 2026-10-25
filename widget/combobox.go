package widget

import (
	"fmt"
	"slices"
	"strings"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// ComboboxOption is one choice of a Combobox: the typed value it picks
// and the label it shows and is searched by.
type ComboboxOption[T comparable] struct {
	Value T
	Label string
}

// comboboxWidth is the trigger's and list's least width.
const comboboxWidth = 200

// Combobox picks typed values from a searchable list: an outline button
// showing the selection that opens a Command of the options in a
// popover, a check trailing each selected one. A single-choice combobox
// closes on a pick, and picking the selected option clears it; a
// multiple-choice one toggles the picked option and stays open. It
// styles as the `combobox` element: the button.outline trigger and its
// .placeholder text.
type Combobox[T comparable] struct {
	composite
	face        render.Font
	trigger     *Button
	text        *Label
	placeholder string
	options     []ComboboxOption[T]
	selected    []T
	multiple    bool
	checks      []*Box
	close       func()

	// OnChanged fires with the selection after the user changed it.
	OnChanged func(selected []T)
}

// NewCombobox returns a single-choice combobox over options showing
// placeholder while nothing is selected.
func NewCombobox[T comparable](face render.Font, placeholder string, options ...ComboboxOption[T]) *Combobox[T] {
	face = requireFace("widget.NewCombobox", face)
	c := &Combobox[T]{face: face, placeholder: placeholder, options: slices.Clone(options)}
	c.SetElement("combobox")
	c.text = NewLabel(face, 14, "", 0)
	row := NewBox(Row, 8, 0)
	row.AppendAligned(c.text, true, AlignCenter)
	row.AppendAligned(NewSymbol(SymbolChevronsUpDown, 16), false, AlignCenter)
	c.trigger = NewButton(row, 0, 0)
	c.trigger.AddClass(css.Outline)
	c.trigger.SetSizeRequest(comboboxWidth, 0)
	c.trigger.OnClick = c.open
	c.initComposite(c, c.trigger)
	c.boxed = true
	c.sync()
	return c
}

// SetMultiple lets the user pick any number of options.
func (c *Combobox[T]) SetMultiple(on bool) { c.multiple = on }

// Selected returns the selected values in option order.
func (c *Combobox[T]) Selected() []T { return slices.Clone(c.selected) }

// SetSelected selects values, which must all be options' values, or
// the call panics. It reports nothing.
func (c *Combobox[T]) SetSelected(values ...T) {
	for _, v := range values {
		if c.index(v) < 0 {
			panic(fmt.Sprintf("widget: Combobox.SetSelected: %v is no option's value", v))
		}
	}
	c.selected = c.selected[:0]
	for _, o := range c.options {
		if slices.Contains(values, o.Value) {
			c.selected = append(c.selected, o.Value)
		}
	}
	c.sync()
}

// index is the option holding v, -1 for none.
func (c *Combobox[T]) index(v T) int {
	return slices.IndexFunc(c.options, func(o ComboboxOption[T]) bool { return o.Value == v })
}

// sync shows the selection on the trigger and the checks.
func (c *Combobox[T]) sync() {
	var labels []string
	for _, v := range c.selected {
		labels = append(labels, c.options[c.index(v)].Label)
	}
	if len(labels) == 0 {
		c.text.SetText(c.placeholder)
		c.text.AddClass("placeholder")
	} else {
		c.text.SetText(strings.Join(labels, ", "))
		c.text.RemoveClass("placeholder")
	}
	for i, check := range c.checks {
		setCommandCheck(check, slices.Contains(c.selected, c.options[i].Value))
	}
}

// open shows the options in a Command in a popover under the trigger.
func (c *Combobox[T]) open() {
	c.checks = c.checks[:0]
	items := make([]CommandItem, len(c.options))
	for i, o := range c.options {
		check := newCommandCheck(false)
		c.checks = append(c.checks, check)
		items[i] = CommandItem{Label: o.Label, Trailing: check, OnSelect: func() { c.pick(o.Value) }}
	}
	list := NewCommand(c.face, "Search...", CommandGroup{Items: items})
	list.SetSizeRequest(max(comboboxWidth, c.trigger.Bounds().W), 0)
	c.sync()
	c.close = OpenPopover(c, c.trigger, list, PopoverOptions{Focus: list.FocusTarget(), Gap: popoverGap})
}

// pick applies the user's choice of v.
func (c *Combobox[T]) pick(v T) {
	at := slices.Index(c.selected, v)
	switch {
	case at >= 0:
		c.selected = slices.Delete(c.selected, at, at+1)
	case c.multiple:
		c.SetSelected(append(slices.Clone(c.selected), v)...)
	default:
		c.selected = append(c.selected[:0], v)
	}
	c.sync()
	if !c.multiple && c.close != nil {
		c.close()
		c.close = nil
	}
	if c.OnChanged != nil {
		c.OnChanged(c.Selected())
	}
}
