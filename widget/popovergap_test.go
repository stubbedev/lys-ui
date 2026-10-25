package widget

import "testing"

// A widget's popover sat flush against its trigger, its border on the
// trigger's: the combobox, the date picker, the dropdown, the hover
// card and the navigation menu keep PopoverGap from what opened them
// (#281, #294).
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
	nav, navHost, _ := newTestNavMenu(t)
	nav.triggers[0].OnClick()
	for _, opened := range []struct {
		name string
		host *fakeSurfaceHost
	}{{"combobox", comboboxHost}, {"date picker", pickerHost}, {"dropdown", dropdownHost}, {"hover card", cardHost}, {"navigation menu", navHost}} {
		if len(opened.host.opts) != 1 || opened.host.opts[0].Gap != PopoverGap {
			t.Errorf("the %s's popover options %+v, want one popover %dpx from its trigger", opened.name, opened.host.opts, PopoverGap)
		}
	}
}

// A menu opened from a trigger keeps PopoverGap from it, as a popover
// does; a context menu opens at the pointer itself, with none. A
// menubar's menu keeps the gap from the bar's edge under its root: from
// the root, inside the bar's padding, it overlapped the bar (#294).
func TestMenusKeepTheirGapFromATrigger(t *testing.T) {
	face := chromeFace(t)
	bar := NewMenuBar(face, 14, MenuBarMenu{Label: "File", Items: []MenuItem{{Label: "New Tab", OnClick: func() {}}}},
		MenuBarMenu{Label: "Edit", Items: []MenuItem{{Label: "Undo", OnClick: func() {}}}})
	barHost := &fakeSurfaceHost{}
	SetSurfaceHost(bar, barHost)
	arrangeTree(t, bar, 400, 60)
	bar.buttons[1].OnClick()
	if len(barHost.menuOpts) != 1 || barHost.menuOpts[0].Gap != PopoverGap {
		t.Fatalf("the menubar's menu options %+v, want one menu %dpx from its root", barHost.menuOpts, PopoverGap)
	}
	if w := AnchorWidget(barHost.menuAnchorers[0]); w != Widget(bar.buttons[1]) {
		t.Errorf("the menu styles under %v, want its root", w)
	}
	root, edge := bar.buttons[1].Bounds(), bar.Bounds()
	if a := barHost.menuAnchors[0]; a.X != root.X || a.W != root.W || a.Y+a.H != edge.Y+edge.H {
		t.Errorf("the menu anchored on %v, want the root's columns %d+%d down to the bar's edge %d", a, root.X, root.W, edge.Y+edge.H)
	}

	c, inner, ctxHost := newTestContextMenu(t)
	b := inner.Bounds()
	(&Router{Root: c}).Press(BTNRight, Point{X: b.X + 4, Y: b.Y + 4}, 0)
	if len(ctxHost.menuOpts) != 1 || ctxHost.menuOpts[0].Gap != 0 {
		t.Errorf("the context menu's options %+v, want one menu at the pointer", ctxHost.menuOpts)
	}
}
