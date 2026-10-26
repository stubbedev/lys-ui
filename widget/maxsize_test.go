package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// spill is a widget whose ink runs 60 px past its bounds on each axis.
type spill struct{ Spacer }

func (s *spill) Paint(cv *render.Canvas) {
	b := s.Bounds()
	cv.FillRect(render.Rect{X: b.X, Y: b.Y, W: b.W + 60, H: b.H + 60}, render.RGBA(255, 0, 0, 255))
}

// A maximum caps the measure and the slot, the slot keeping its start
// under fill alignment and moving under center.
func TestMaximumSizeCapsMeasureAndSlot(t *testing.T) {
	face := goldenFace(t)
	label := NewLabel(face, 14, strings.Repeat("wide ", 40), 0)
	label.SetMaximumSize(50, 0)
	if sz := MeasureAsChild(label, Constraints{Max: Size{W: 600, H: 100}}); sz.W != 50 {
		t.Errorf("measured %d wide, want the 50 cap", sz.W)
	}
	col := NewBox(Column, 0, 0)
	col.Append(label, false)
	centered := NewSpacer(10, 10)
	centered.SetMaximumSize(40, 0)
	centered.SetHAlign(AlignCenter)
	col.Append(centered, false)
	arrangeTree(t, col, 300, 200)
	if b := label.Bounds(); b.X != 0 || b.W != 50 {
		t.Errorf("filling label arranged at x %d width %d, want 0 and 50", b.X, b.W)
	}
	if b := centered.Bounds(); b.W != 10 || b.X != 145 {
		t.Errorf("centered spacer at x %d width %d, want its natural 10 centered", b.X, b.W)
	}
	label.SetMaximumSize(0, 0)
	arrangeTree(t, col, 300, 200)
	if b := label.Bounds(); b.W != 300 {
		t.Errorf("uncapped label is %d wide, want the 300 slot", b.W)
	}
}

// A request and a maximum of the same size pin an exact size, and a
// request above the maximum wins.
func TestMaximumSizeWithRequestPinsASize(t *testing.T) {
	big := NewSpacer(500, 500)
	big.SetSizeRequest(80, 40)
	big.SetMaximumSize(80, 40)
	small := NewSpacer(0, 0)
	small.SetSizeRequest(60, 30)
	small.SetMaximumSize(20, 10)
	row := NewBox(Row, 0, 0)
	row.Append(big, false)
	row.Append(small, false)
	arrangeTree(t, row, 400, 300)
	if b := big.Bounds(); b.W != 80 || b.H != 40 {
		t.Errorf("pinned spacer is %dx%d, want 80x40", b.W, b.H)
	}
	if b := small.Bounds(); b.W != 60 || b.H != 30 {
		t.Errorf("request above the cap gave %dx%d, want the 60x30 request", b.W, b.H)
	}
}

// What a capped widget paints past its bounds is clipped.
func TestMaximumSizeClipsTheOverflow(t *testing.T) {
	s := &spill{}
	s.SetSizeRequest(20, 20)
	s.SetMaximumSize(20, 20)
	free := &spill{}
	free.SetSizeRequest(20, 20)
	for _, tc := range []struct {
		w       *spill
		clipped bool
	}{{s, true}, {free, false}} {
		col := NewBox(Column, 0, 0)
		col.Append(tc.w, false)
		col.SetHAlign(AlignStart)
		root := NewBox(Column, 0, 0)
		root.Append(col, false)
		arrangeTree(t, root, 80, 80)
		data := make([]byte, render.Stride(80)*80)
		PaintChild(render.New(data, render.Stride(80), 80, 80), root)
		if in := pixelAt(data, 10, 10); in.R() != 255 {
			t.Errorf("clipped %v: inside pixel %v, want red", tc.clipped, in)
		}
		if out := pixelAt(data, 40, 40); (out.R() == 255) == tc.clipped {
			t.Errorf("clipped %v: pixel past the bounds is %v", tc.clipped, out)
		}
	}
}
