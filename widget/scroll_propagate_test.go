package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// propagate-natural-height, as GTK's: without it a scroll asks for no
// height and scrolls in what its holder gives it; with it, it asks for
// its content's height and gives way when its holder is short.
func TestScrollPropagateNaturalHeight(t *testing.T) {
	con := Constraints{Max: Size{W: 200, H: 4000}}
	build := func(propagate bool) (*Box, *Scroll) {
		card := NewBox(Column, 0, 0)
		card.Append(NewSpacer(100, 50), false)
		sc := NewScroll(NewSpacer(100, 2000))
		sc.VerticalOnly, sc.FillY, sc.PropagateNaturalHeight = true, true, propagate
		card.Append(sc, true)
		card.SetSizeRequest(-1, 512)
		return NewBox(Column, 0, 0).Append(card, true), sc
	}

	// GTK's scrolled window without propagate-natural-height asks for
	// no height: a card sized 512 stays 512 and its list scrolls.
	col, sc := build(false)
	if got := col.Measure(con).H; got != 512 {
		t.Errorf("a 512 card holding a plain scroll measures %d tall, want 512", got)
	}
	if sc.Shrinkable() != 0 {
		t.Errorf("a plain scroll offers %d px to give way, asking for none", sc.Shrinkable())
	}
	arrangeTree(t, col, 200, 512)
	if got := sc.Bounds().H; got != 462 {
		t.Errorf("the plain scroll's viewport is %d tall, want the 462 below the header", got)
	}

	// Propagating, it asks for its content's height and gives way down
	// to its floor when its holder is short.
	col, sc = build(true)
	if got := col.Measure(con).H; got != 2050 {
		t.Errorf("a card holding a propagating scroll measures %d tall, want the content's 2050", got)
	}
	if got := sc.Shrinkable(); got != 2000-scrollFloor {
		t.Errorf("a propagating scroll offers %d px to give way, want all above its floor, %d", got, 2000-scrollFloor)
	}
	sc.SetMaxContentHeight(300)
	if got := col.Measure(con).H; got != 512 {
		t.Errorf("a card holding a scroll capped at 300 measures %d, want its 512 request", got)
	}
}

// A content-height cap applies only to a propagated natural height, so
// setting one turns propagation on.
func TestScrollMaxContentHeightPropagates(t *testing.T) {
	sc := NewScroll(NewSpacer(100, 900))
	sc.SetMaxContentHeight(250)
	if !sc.PropagateNaturalHeight {
		t.Fatal("SetMaxContentHeight left PropagateNaturalHeight off")
	}
	if got := sc.Measure(Constraints{Max: Size{W: 400, H: 4000}}).H; got != 250 {
		t.Errorf("a capped scroll measures %d tall, want 250", got)
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

// propagating is s propagating its natural height: a scroll as tall as
// its content that gives way when its holder is short.
func propagating(s *Scroll) *Scroll {
	s.PropagateNaturalHeight = true
	return s
}
