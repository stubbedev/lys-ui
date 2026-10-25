package widget

import "testing"

// A widget's popover sat flush against its trigger, its border on the
// trigger's: the combobox, the date picker, the dropdown and the hover
// card keep popoverGap from what opened them (#281).
func TestWidgetPopoversKeepTheirGap(t *testing.T) {
	combobox, comboboxHost := newTestCombobox(t)
	combobox.trigger.OnClick()
	picker, pickerHost := newTestPicker(t)
	picker.trigger.OnClick()
	dropdown, dropdownHost := newDropdown(t, "a", "b")
	dropdown.Open()
	card, trigger, cardHost := newTestHoverCard(t)
	b := trigger.Bounds()
	(&Router{Root: card}).Move(Point{X: b.X + 2, Y: b.Y + 2})
	for _, opened := range []struct {
		name string
		host *fakeSurfaceHost
	}{{"combobox", comboboxHost}, {"date picker", pickerHost}, {"dropdown", dropdownHost}, {"hover card", cardHost}} {
		if len(opened.host.opts) != 1 || opened.host.opts[0].Gap != popoverGap {
			t.Errorf("the %s's popover options %+v, want one popover %dpx from its trigger", opened.name, opened.host.opts, popoverGap)
		}
	}
}
