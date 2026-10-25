package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// propagate-natural-height: the scroll stops offering its shortfall —
// the panel-reserve math (measure less Shrinkable, wayle's
// dropdown panel) then grows the surface to the content's natural
// height instead of scrolling at the base height. A plain scroll gives
// the shortfall back and scrolls.
func TestScrollPropagateNaturalHeight(t *testing.T) {
	build := func() (*Box, *Scroll) {
		col := NewBox(Column, 0, 0)
		sc := NewScroll(NewSpacer(100, 400))
		sc.VerticalOnly = true
		col.Append(sc, true)
		return col, sc
	}
	con := Constraints{Max: Size{W: 200, H: 350}}

	col, sc := build()
	arrangeTree(t, col, 200, 350)
	if sc.Shrinkable() <= 0 {
		t.Fatal("a plain scroll cannot shrink; the test needs the floor below the content")
	}
	if got := sc.Bounds().H; got != 350 {
		t.Errorf("plain scroll took %d of a 350 column, want the whole room", got)
	}
	// The wayle reserve: measured height less what it gives up.
	if grow := sc.Measure(con).H - sc.Shrinkable(); grow != scrollFloor {
		t.Errorf("plain reserve %d, want just the scroll floor", grow)
	}

	col2, sc2 := build()
	sc2.PropagateNaturalHeight = true
	arrangeTree(t, col2, 200, 350)
	if sc2.Shrinkable() != 0 {
		t.Error("a propagating scroll still offers to shrink")
	}
	if grow := sc2.Measure(con).H - sc2.Shrinkable(); grow != 350 {
		t.Errorf("propagating reserve %d, want the whole measured 350 (the panel grows past it via the natural, capped by its own ceiling)", grow)
	}
}

// propagate-natural-width: a VerticalOnly scroll asks for its child's
// minimum width, as GTK's never-policy scroll does, so a long
// ellipsizing line does not widen it; propagating asks for the full
// natural width.
func TestScrollPropagateNaturalWidth(t *testing.T) {
	build := func(propagate bool) *Scroll {
		label := NewLabel(entryFace(t), 12, "a long line that would widen any panel it sits in", 0)
		label.SetEllipsize(EllipsizeEnd)
		sc := NewScroll(label)
		sc.VerticalOnly = true
		sc.PropagateNaturalWidth = propagate
		return sc
	}
	con := Constraints{Max: Size{W: 2000, H: 300}}
	natural := NewLabel(entryFace(t), 12, "a long line that would widen any panel it sits in", 0).Measure(con).W
	if w := build(false).Measure(con).W; w >= natural/2 {
		t.Errorf("never-policy scroll asks %dpx for a %dpx line, want its minimum", w, natural)
	}
	if w := build(true).Measure(con).W; w < natural {
		t.Errorf("propagating scroll asks %dpx, want the %dpx natural width", w, natural)
	}
}

// A wrapping rich label gives up width down to its widest token, so a
// column of them inside a never-policy scroll asks for little more
// than that token; a single-line one keeps its width.
func TestWrappingRichLabelMinContent(t *testing.T) {
	text := "words that wrap onto several lines when the panel is narrow"
	con := Constraints{Max: Size{W: 2000, H: 300}}
	wrapping := NewRichLabel(entryFace(t), 12, text, 0)
	wrapping.SetWrap(true)
	full := wrapping.Measure(con).W
	if got, want := wrapping.ShrinkableWidth(), full-wrapping.MinSize().W; got != want || got <= 0 {
		t.Errorf("wrapping shrinkable %d, want %d down to the widest token", got, want)
	}
	single := NewRichLabel(entryFace(t), 12, text, 0)
	single.Measure(con)
	if single.ShrinkableWidth() != 0 {
		t.Error("a single-line rich label offered to shrink")
	}
	col := NewBox(Column, 0, 0)
	body := NewRichLabel(entryFace(t), 12, text, 0)
	body.SetWrap(true)
	col.Append(body, false)
	sc := NewScroll(col)
	sc.VerticalOnly = true
	if w := sc.Measure(con).W; w >= full/2 {
		t.Errorf("never-policy scroll asks %dpx for a %dpx wrapping line, want about its widest token", w, full)
	}
}

// A vertical-only scroll reserves its bar's column only while the
// content overflows: content that fits keeps the viewport's whole
// width, and content that overflows gives the bar its gutter (#281).
func TestVerticalScrollGutterOnlyWhenOverflowing(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows int
		want int
	}{{"fits", 2, 200}, {"overflows", 40, 200 - gutter}} {
		content := NewBox(Column, 0, 0)
		for range tc.rows {
			content.Append(NewSpacer(10, 20), false)
		}
		s := NewScroll(content)
		s.VerticalOnly = true
		s.Measure(Constraints{Max: Size{W: 200, H: 100}})
		s.Arrange(render.Rect{W: 200, H: 100})
		if got := content.Bounds().W; got != tc.want {
			t.Errorf("%s: content %dpx wide, want %d", tc.name, got, tc.want)
		}
	}
}
