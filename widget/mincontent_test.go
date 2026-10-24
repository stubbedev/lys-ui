package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// A column's minimum is its widest child's minimum: a wrapping label
// goes down to its widest word, so a short label sets it (#252).
func TestMinWidthWrappingColumn(t *testing.T) {
	face := testFace(t)
	long := NewLabel(face, 14, "No treeman-managed worktrees are registered, or the treeman daemon is not running.", 0)
	long.SetWrap(true)
	short := NewLabel(face, 14, "Worktrees and more", 0)
	col := NewBox(Column, 8, 0)
	col.Append(long, false).Append(short, false)
	shortW := short.Measure(unbounded).W
	if got := MinWidth(col); got != shortW {
		t.Errorf("column minimum %d, want the short label's %d", got, shortW)
	}
	if got := long.Measure(unbounded).W; got <= shortW {
		t.Fatalf("the long label measures %d, not wider than the short one", got)
	}
}

// Without wrap the long label keeps its natural width as its minimum.
func TestMinWidthNonWrappingColumn(t *testing.T) {
	face := testFace(t)
	long := NewLabel(face, 14, "Built-in Audio Analog Stereo Output Device", 0)
	col := NewBox(Column, 8, 0)
	col.Append(long, false).Append(NewLabel(face, 14, "Short", 0), false)
	if got, want := MinWidth(col), long.Measure(unbounded).W; got != want {
		t.Errorf("column minimum %d, want the long label's natural %d", got, want)
	}
}

// An ellipsizing label's minimum is the ellipsis; a row adds its
// children's minimums and spacing; a size request floors, and a
// sideways scroll gives everything up.
func TestMinWidthEllipsisRowRequestScroll(t *testing.T) {
	face := testFace(t)
	ell := NewLabel(face, 14, "A rather long title that can ellipsize", 0)
	ell.SetEllipsize(EllipsizeEnd)
	dots := int(face.Shape(render.Ellipsis, 14).Advance() + 0.999)
	if got := MinWidth(ell); got != dots {
		t.Errorf("ellipsizing label minimum %d, want the ellipsis %d", got, dots)
	}
	fixed := NewSpacer(30, 10)
	row := NewBox(Row, 6, 0)
	row.Append(ell, true).Append(fixed, false)
	if got := MinWidth(row); got != dots+6+30 {
		t.Errorf("row minimum %d, want %d", got, dots+6+30)
	}
	row.SetSizeRequest(200, 0)
	if got := MinWidth(row); got != 200 {
		t.Errorf("requested row minimum %d, want the 200px request", got)
	}
	if got := MinWidth(NewScroll(NewSpacer(500, 10))); got != 0 {
		t.Errorf("sideways scroll minimum %d, want 0", got)
	}
}
