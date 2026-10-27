package widget

import "testing"

// set_homogeneous: every child carries the largest child's main-axis
// size, in measure and in arrange (gtk_box_layout's homogeneous).
func TestBoxHomogeneousEqualizes(t *testing.T) {
	b := NewBox(Row, 0, 0)
	b.SetHomogeneous(true)
	narrow := NewLabel(entryFace(t), 10, "ab", 0)
	wide := NewLabel(entryFace(t), 10, "wide!", 0)
	b.Append(narrow, false)
	b.Append(wide, false)
	sz := SizeWithin(b, Size{W: 400, H: 40})
	wn := SizeWithin(narrow, Size{W: 400, H: 40}).W
	ww := SizeWithin(wide, Size{W: 400, H: 40}).W
	if wn >= ww {
		t.Fatalf("the wide label is not the widest (%d vs %d); the test needs distinct naturals", wn, ww)
	}
	if sz.W != 2*ww {
		t.Errorf("homogeneous row %d wide, want twice the widest %d", sz.W, 2*ww)
	}
	arrangeTree(t, b, sz.W, sz.H)
	nb, wb := narrow.Bounds(), wide.Bounds()
	if nb.W != wb.W {
		t.Errorf("child widths %d and %d, want equal", nb.W, wb.W)
	}
	if wb.X != nb.X+nb.W {
		t.Errorf("children not adjacent: %v then %v", nb, wb)
	}
}

// Homogeneous defaults off, and turning it off restores the naturals.
func TestBoxHomogeneousDefaultsOff(t *testing.T) {
	b := NewBox(Row, 0, 0)
	narrow := NewLabel(entryFace(t), 10, "ab", 0)
	wide := NewLabel(entryFace(t), 10, "wide!", 0)
	b.Append(narrow, false)
	b.Append(wide, false)
	if b.Homogeneous() {
		t.Error("a plain box is homogeneous")
	}
	sz := SizeWithin(b, Size{W: 400, H: 40})
	arrangeTree(t, b, sz.W, sz.H)
	wn, ww := narrow.Bounds().W, wide.Bounds().W
	if wn == ww {
		t.Error("a plain box equalized its children")
	}
	b.SetHomogeneous(false)
	if b.Homogeneous() {
		t.Error("SetHomogeneous(false) left it on")
	}
}

// The expanders' share of a width that does not divide evenly goes a
// pixel more to the first of them, so the row ends flush, gaps or not;
// an even width keeps the shares equal (#255).
func TestBoxSpreadsTheRemainder(t *testing.T) {
	for _, tc := range []struct {
		width, spacing int
		want           []int
	}{
		{337, 0, []int{85, 84, 84, 84}},
		{340, 1, []int{85, 84, 84, 84}},
		{336, 0, []int{84, 84, 84, 84}},
	} {
		row := NewBox(Row, tc.spacing, 0)
		row.SetHomogeneous(true)
		kids := make([]*Spacer, 4)
		for i := range kids {
			kids[i] = NewSpacer(10, 10)
			row.Append(kids[i], true)
		}
		arrangeTree(t, row, tc.width, 20)
		for i, k := range kids {
			if k.Bounds().W != tc.want[i] {
				t.Errorf("%dpx with spacing %d: child %d is %d wide, want %d", tc.width, tc.spacing, i, k.Bounds().W, tc.want[i])
			}
		}
		if last := kids[3].Bounds(); last.X+last.W != tc.width {
			t.Errorf("%dpx with spacing %d: the row ends at %d", tc.width, tc.spacing, last.X+last.W)
		}
	}
}
