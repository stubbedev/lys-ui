package widget

import "testing"

// A widget's popover sat flush against its trigger, its border on the
// trigger's: the combobox, the date picker, the dropdown and the hover
// card keep PopoverGap from what opened them (#281).
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
		if len(opened.host.opts) != 1 || opened.host.opts[0].Gap != PopoverGap {
			t.Errorf("the %s's popover options %+v, want one popover %dpx from its trigger", opened.name, opened.host.opts, PopoverGap)
		}
	}
}

// A menu opened from a trigger keeps PopoverGap from it, as a popover
// does; a context menu opens at the pointer itself, with none.
func TestMenusKeepTheirGapFromATrigger(t *testing.T) {
	face := chromeFace(t)
	bar := NewMenuBar(face, 14, MenuBarMenu{Label: "File", Items: []MenuItem{{Label: "New Tab", OnClick: func() {}}}})
	barHost := &fakeSurfaceHost{}
	SetSurfaceHost(bar, barHost)
	bar.buttons[0].OnClick()
	if len(barHost.menuOpts) != 1 || barHost.menuOpts[0].Gap != PopoverGap {
		t.Errorf("the menubar's menu options %+v, want one menu %dpx from its root", barHost.menuOpts, PopoverGap)
	}

	c, inner, ctxHost := newTestContextMenu(t)
	b := inner.Bounds()
	(&Router{Root: c}).Press(BTNRight, Point{X: b.X + 4, Y: b.Y + 4})
	if len(ctxHost.menuOpts) != 1 || ctxHost.menuOpts[0].Gap != 0 {
		t.Errorf("the context menu's options %+v, want one menu at the pointer", ctxHost.menuOpts)
	}
}
