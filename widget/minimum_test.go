package widget

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// minWidth is w's minimum width as its container sees it.
func minWidth(w Widget) int { return MeasureAsChild(w, Row, Unknown).Min }

// natWidth is w's natural width as its container sees it.
func natWidth(w Widget) int { return MeasureAsChild(w, Row, Unknown).Nat }

// A column's minimum is its widest child's minimum: a wrapping label
// goes down to its widest word, so a short label sets it (#252).
func TestMinWidthWrappingColumn(t *testing.T) {
	face := testFace(t)
	long := NewLabel(face, 14, "No treeman-managed worktrees are registered, or the treeman daemon is not running.", 0)
	long.SetWrap(true)
	short := NewLabel(face, 14, "Worktrees and more", 0)
	col := NewBox(Column, 8, 0)
	col.Append(long, false).Append(short, false)
	shortW := natWidth(short)
	if got := minWidth(col); got != shortW {
		t.Errorf("column minimum %d, want the short label's %d", got, shortW)
	}
	if got := natWidth(long); got <= shortW {
		t.Fatalf("the long label measures %d, not wider than the short one", got)
	}
}

// Without wrap the long label keeps its natural width as its minimum.
func TestMinWidthNonWrappingColumn(t *testing.T) {
	face := testFace(t)
	long := NewLabel(face, 14, "Built-in Audio Analog Stereo Output Device", 0)
	col := NewBox(Column, 8, 0)
	col.Append(long, false).Append(NewLabel(face, 14, "Short", 0), false)
	if got, want := minWidth(col), natWidth(long); got != want {
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
	if got := minWidth(ell); got != dots {
		t.Errorf("ellipsizing label minimum %d, want the ellipsis %d", got, dots)
	}
	fixed := NewSpacer(30, 10)
	row := NewBox(Row, 6, 0)
	row.Append(ell, true).Append(fixed, false)
	if got := minWidth(row); got != dots+6+30 {
		t.Errorf("row minimum %d, want %d", got, dots+6+30)
	}
	row.SetSizeRequest(200, 0)
	if got := minWidth(row); got != 200 {
		t.Errorf("requested row minimum %d, want the 200px request", got)
	}
	if got := minWidth(NewScroll(NewSpacer(500, 10))); got != 0 {
		t.Errorf("sideways scroll minimum %d, want 0", got)
	}
}

// A label's natural width is its text on one line, an ellipsizing or a
// wrapping one's too; max-width-chars holds a wrapping one's to that
// many characters, above its widest word.
func TestLabelNaturalWidth(t *testing.T) {
	face := testFace(t)
	const text = "No treeman-managed worktrees are registered, or the treeman daemon is not running."
	line := int(face.Shape(text, 14).Advance() + 0.999)
	ell := NewLabel(face, 14, text, 0)
	ell.SetEllipsize(EllipsizeEnd)
	if got := natWidth(ell); got != line {
		t.Errorf("ellipsizing label wants %d, want its line %d", got, line)
	}
	wrap := NewLabel(face, 14, text, 0)
	wrap.SetWrap(true)
	if got := natWidth(wrap); got != line {
		t.Errorf("wrapping label wants %d, want its line %d", got, line)
	}
	if least := minWidth(wrap); least >= line || least <= 0 {
		t.Errorf("wrapping label needs %d, want its widest word, under the line %d", least, line)
	}
	capped := NewLabel(face, 14, text, 0)
	capped.SetWrap(true)
	capped.SetMaxWidthChars(20)
	if got := natWidth(capped); got != capped.charsWidth(20) || got <= minWidth(wrap) {
		t.Errorf("wrapping label with max-width-chars 20 wants %d, want its 20 characters %d", got, capped.charsWidth(20))
	}
}

// naturalSize is w's natural size: its natural width, and its natural
// height at that width.
func naturalSize(w Widget) Size { return SizeWithin(w, Size{W: math.MaxInt, H: math.MaxInt}) }

// widthGive is how much of its natural width w can give up as a child.
func widthGive(w Widget) int {
	r := MeasureAsChild(w, Row, Unknown)
	return r.Nat - r.Min
}

// heightGive is how much of its natural height w can give up as a
// child, at its natural width.
func heightGive(w Widget) int {
	r := MeasureAsChild(w, Column, Unknown)
	return r.Nat - r.Min
}

// CharsWithin's count of characters, as max-width-chars, holds a
// wrapping label's natural width to the pixels it was asked for, and
// within one character of them.
func TestCharsWithinCapsTheNaturalWidth(t *testing.T) {
	face := testFace(t)
	const text = "No treeman-managed worktrees are registered, or the treeman daemon is not running."
	for _, px := range []int{120, 200, 333} {
		l := NewLabel(face, 14, text, 0)
		l.SetWrap(true)
		l.SetMaxWidthChars(l.CharsWithin(px))
		r := NewRichLabel(face, 14, text, 0)
		r.SetWrap(true)
		r.SetMaxWidthChars(r.CharsWithin(px))
		char := int(approxCharPx(face, 14)) + 1
		for name, w := range map[string]Widget{"Label": l, "RichLabel": r} {
			if got := natWidth(w); got > px || got < px-char {
				t.Errorf("%s capped for %dpx wants %d, want within a character under it", name, px, got)
			}
		}
	}
}
