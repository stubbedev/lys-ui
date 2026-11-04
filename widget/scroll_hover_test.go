package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestScrollHoverWithoutAStyleDamagesNothing(t *testing.T) {
	face := entryFace(t)
	col := NewBox(Column, 0, 0)
	for i := range 40 {
		col.Append(NewLabel(face, 14, string(rune('a'+i%26)), 0), false)
	}
	sc := NewScroll(col)
	r := render.Rect{W: 300, H: 160}
	SizeWithin(sc, Size{W: 300, H: 160})
	sc.Arrange(r)
	CollectDamage(sc)
	sc.SetHovered(true)
	sc.Arrange(r)
	rects, _ := CollectDamage(sc)
	if got := DamageArea(rects); got != 0 {
		t.Errorf("hovering a scroll no rule styles damaged %d px (%+v), want 0", got, rects)
	}
	sc.SetHovered(false)
	sc.SetPressed(false)
	sc.Arrange(r)
	rects, _ = CollectDamage(sc)
	if got := DamageArea(rects); got != 0 {
		t.Errorf("leaving and releasing a scroll no rule styles damaged %d px (%+v), want 0", got, rects)
	}
}

func TestScrollHoverRuleStillRepaints(t *testing.T) {
	loadCSS(t, `scrolledwindow:hover { background-color: #123456; }`)
	sc := NewScroll(NewBox(Column, 0, 0))
	r := render.Rect{W: 300, H: 160}
	SizeWithin(sc, Size{W: 300, H: 160})
	sc.Arrange(r)
	CollectDamage(sc)
	sc.hoverChain = true
	sc.SetHovered(true)
	sc.Arrange(r)
	rects, _ := CollectDamage(sc)
	if got := render.UnionAll(rects); got != r {
		t.Errorf("hover rule damaged %+v, want the whole scroll %+v", got, r)
	}
}
