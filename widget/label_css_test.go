package widget

// The CSS text properties on the label surface: white-space and
// line-clamp drive the wrapped row count, text-overflow picks the
// ellipsis, direction forces the base, and vertical-align moves the
// reported baseline.

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func measureLabel(l *Label, w int) Size {
	return SizeWithin(l, Size{W: w, H: 1 << 20})
}

func TestCSSWhiteSpaceNowrapOverridesWrap(t *testing.T) {
	l := NewLabel(testFace(t), 12, "one two three", render.RGB(0, 0, 0))
	l.SetWrap(true)
	loadCSS(t, `label { white-space: nowrap; }`)
	sz := measureLabel(l, 40)
	if rows := sz.H / l.lineBox(l.style(l)); rows != 1 {
		t.Errorf("nowrap left %d rows (size %v), want 1", rows, sz)
	}
}

func TestCSSPreWrapWrapsWithoutCollapse(t *testing.T) {
	l := NewLabel(testFace(t), 12, "aa bb", render.RGB(0, 0, 0))
	l.SetWrap(true)
	loadCSS(t, `label { white-space: pre-wrap; }`)
	if rows := l.wrapped(200); len(rows) != 1 || rows[0] != "aa bb" {
		t.Errorf("pre-wrap rows = %q, want the untouched row", rows)
	}
}

func TestCSSLineClampCapsRowsAndEllipsizes(t *testing.T) {
	l := NewLabel(testFace(t), 12, "one two three four five six", render.RGB(0, 0, 0))
	l.SetWrap(true)
	loadCSS(t, `label { line-clamp: 2; }`)
	sz := measureLabel(l, 60)
	if rows := sz.H / l.lineBox(l.style(l)); rows != 2 {
		t.Errorf("line-clamp 2 left %d rows (size %v)", rows, sz)
	}
	rows := l.wrapped(60)
	if len(rows) != 2 || !endsWithEllipsis(rows[1]) {
		t.Errorf("clamped rows = %q, want 2 with an ellipsized tail", rows)
	}
}

func endsWithEllipsis(s string) bool {
	return len(s) >= len(render.Ellipsis) && s[len(s)-len(render.Ellipsis):] == render.Ellipsis
}

func TestCSSTextOverflowEllipsis(t *testing.T) {
	l := NewLabel(testFace(t), 12, "a very long label", render.RGB(0, 0, 0))
	wide := measureLabel(l, 1<<20).W
	loadCSS(t, `label { text-overflow: ellipsis; }`)
	sz := measureLabel(l, 60)
	if sz.W >= wide || sz.W > 60 {
		t.Errorf("ellipsized width %d, want it capped under the long text %d", sz.W, wide)
	}
}

func TestCSSDirectionForcesBase(t *testing.T) {
	l := NewLabel(testFace(t), 12, "שלום hello", render.RGB(0, 0, 0))
	l.SetDirection(DirectionLTR)
	loadCSS(t, `label { direction: rtl; }`)
	if got := l.effDirection(l.style(l)); got != DirectionRTL {
		t.Errorf("CSS direction rtl = %v", got)
	}
}

func TestCSSVerticalAlignShiftsBaseline(t *testing.T) {
	l := NewLabel(testFace(t), 12, "x", render.RGB(0, 0, 0))
	arrangeTree(t, l, 100, 40)
	base, ok := l.Baseline()
	if !ok {
		t.Fatal("label has no baseline")
	}
	loadCSS(t, `label { vertical-align: 4px; }`)
	l.Invalidate()
	shifted, _ := l.Baseline()
	if shifted != base-4 {
		t.Errorf("vertical-align 4px baseline %d, want %d", shifted, base-4)
	}
}
