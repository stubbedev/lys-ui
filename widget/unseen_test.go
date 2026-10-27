package widget

import "testing"

// testDot is a small SVG icon.
func testDot() *Icon {
	return NewSVGIcon([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><circle cx="4" cy="4" r="4"/></svg>`), 8)
}

// A widget built off screen and decorated owes no frame and stales no
// other style: a class on a fresh icon bumped the global style
// generation and the damage, so every window restyled and painted again.
func TestDecoratingAnUnseenWidgetIsFree(t *testing.T) {
	ic := testDot()
	gen, dmg := styleGen, DamageGeneration()
	ic.AddClass("token-accent")
	ic.SetState(StateSelected, true)
	ic.Invalidate()
	if styleGen != gen || DamageGeneration() != dmg {
		t.Errorf("style generation %d→%d, damage %d→%d; want both unmoved", gen, styleGen, dmg, DamageGeneration())
	}
	_ = ic.style(ic)
	ic.AddClass("other")
	if DamageGeneration() == dmg {
		t.Error("a styled widget's class change owed no frame")
	}
}

// A shown widget still damages on a class change.
func TestDecoratingAShownWidgetDamages(t *testing.T) {
	box := NewBox(Column, 0, 0)
	lbl := NewLabel(goldenFace(t), 14, "x", 0)
	box.Append(lbl, false)
	arrangeTree(t, box, 100, 40)
	dmg := DamageGeneration()
	lbl.AddClass("picked")
	if DamageGeneration() == dmg {
		t.Error("a class on an arranged label owed no frame")
	}
}

// A dropdown builds its rows' leading icons once per row set, not on
// every measure: each fresh icon is styled anew.
func TestDropdownBuildsSlotIconsOnce(t *testing.T) {
	built := 0
	dot := func() Widget { built++; return testDot() }
	rows := []DropdownRow{{Label: "a", Leading: dot}, {Label: "b", Leading: dot}}
	d := NewDropdownRows(testFace(t), 13, rows, 0)
	for range 5 {
		d.InvalidateLayout()
		SizeWithin(d, Size{W: 300, H: 40})
	}
	slot := built
	if slot > len(rows)+1 {
		t.Errorf("five measures built %d icons, want one per row and the face's", slot)
	}
	d.SetRows(rows, 0)
	SizeWithin(d, Size{W: 300, H: 40})
	if built == slot {
		t.Error("new rows did not rebuild the slot icons")
	}
}
