package widget

import (
	"testing"
)

// CenterBox centers the middle slot however wide the sides run: the
// start slot hugs the left edge, the end slot the right, the center
// slot sits at the allocation's midpoint.
func TestCenterBoxCenters(t *testing.T) {
	left := NewLabel(entryFace(t), 10, "left", 0)
	mid := NewBox(Row, 0, 0)
	mid.Append(NewLabel(entryFace(t), 10, "mid", 0), false)
	right := NewLabel(entryFace(t), 10, "right!", 0)
	cb := NewCenterBox(left, mid, right)
	arrangeTree(t, cb, 200, 30)
	lb, mb, rb := left.Bounds(), mid.Bounds(), right.Bounds()
	if lb.X != 0 {
		t.Errorf("start slot at %d, want the left edge", lb.X)
	}
	if rb.X+rb.W != 200 {
		t.Errorf("end slot ends at %d, want the right edge", rb.X+rb.W)
	}
	got := float64(mb.X) + float64(mb.W)/2
	if got < 99 || got > 101 {
		t.Errorf("center slot midpoint %v, want the allocation center", got)
	}
}

// Nil slots measure and arrange without incident.
func TestCenterBoxNilSlots(t *testing.T) {
	mid := NewLabel(entryFace(t), 10, "mid", 0)
	cb := NewCenterBox(nil, mid, nil)
	arrangeTree(t, cb, 80, 20)
	mb := mid.Bounds()
	if got := float64(mb.X) + float64(mb.W)/2; got < 39 || got > 41 {
		t.Errorf("center slot midpoint %v, want 40", got)
	}
	if SizeWithin(cb, Size{W: 100, H: 40}).W == 0 {
		t.Error("the box measured to nothing")
	}
}

// A side long enough to reach the middle pushes the center slot just
// clear of it, as GtkCenterLayout does; short sides leave it centered.
func TestCenterBoxCenterClearsALongSide(t *testing.T) {
	for _, tc := range []struct {
		size, start, center, end, want int
	}{
		{200, 20, 40, 20, 80},
		{200, 120, 40, 20, 120},
		{200, 20, 40, 130, 30},
		{100, 60, 40, 60, 60},
	} {
		if got := centerPos(tc.size, tc.start, tc.center, tc.end); got != tc.want {
			t.Errorf("centerPos(%d, %d, %d, %d) = %d, want %d", tc.size, tc.start, tc.center, tc.end, got, tc.want)
		}
	}
	long := NewSpacer(120, 10)
	mid := NewSpacer(40, 10)
	cb := NewCenterBox(long, mid, NewSpacer(10, 10))
	arrangeTree(t, cb, 200, 30)
	if mb := mid.Bounds(); mb.X != 120 {
		t.Errorf("center slot at %d, want just clear of the 120px start", mb.X)
	}
}

// A Column center box runs top to bottom: start at the top, end at the
// bottom, the center at the vertical middle, each spanning the width.
func TestCenterBoxColumn(t *testing.T) {
	top, mid, bottom := NewSpacer(10, 20), NewSpacer(10, 30), NewSpacer(10, 40)
	cb := NewCenterBox(top, mid, bottom)
	cb.SetOrientation(Column)
	if got := SizeWithin(cb, Size{W: 500, H: 500}); got != (Size{W: 10, H: 90}) {
		t.Errorf("column measures %v, want 10x90", got)
	}
	arrangeTree(t, cb, 50, 300)
	if tb, mb, bb := top.Bounds(), mid.Bounds(), bottom.Bounds(); tb.Y != 0 || mb.Y != 135 || bb.Y != 260 || mb.W != 50 {
		t.Errorf("top %v mid %v bottom %v, want 0, 135 and 260 across the 50px width", tb, mb, bb)
	}
}

// A padded CenterBox measures its content plus its insets, so the
// children get their natural size inside (#275).
func TestCenterBoxCSSInsets(t *testing.T) {
	loadCSS(t, `.details { padding: 8px 0px; }`)
	mid := NewSpacer(30, 34)
	cb := NewCenterBox(nil, mid, nil)
	cb.AddClass("details")
	host := NewBox(Column, 0, 0)
	host.Append(cb, false)
	arrangeTree(t, host, 300, 200)
	if got := cb.Bounds().H; got != 50 {
		t.Errorf("padded center box %d tall, want the 34px child plus 16px", got)
	}
	if got := mid.Bounds().H; got != 34 {
		t.Errorf("the child got %d px, want its natural 34", got)
	}
}
