package widget

import "testing"

func TestRemeasureBeforeArrangeSeesChanges(t *testing.T) {
	face := testFace(t)
	con := Size{W: 400, H: 400}

	row := NewActionRow(face, 13, "Title", "")
	before := SizeWithin(row, con)
	row.SetSubtitle("a subtitle line")
	if after := SizeWithin(row, con); after.H <= before.H {
		t.Errorf("a composite kept its cached size after a change below it: %v then %v", before, after)
	}

	hidden := NewSpacer(10, 30)
	hidden.SetVisible(false)
	box := NewBox(Column, 0, 0)
	box.Append(NewSpacer(10, 10), false).Append(hidden, false)
	before = SizeWithin(box, con)
	hidden.SetVisible(true)
	if after := SizeWithin(box, con); after.H != before.H+30 {
		t.Errorf("showing a hidden child did not reach the box: %v then %v", before, after)
	}
}
