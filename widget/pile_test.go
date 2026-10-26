package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
)

// pileOf is a pile of n cards, each a stub h tall, the pile 200 wide.
func pileOf(t *testing.T, h ...int) (*Pile, []*stub) {
	t.Helper()
	t.Cleanup(anim.SetInstant(true))
	p := NewPile()
	var cards []*stub
	for _, hh := range h {
		c := newStub(200, hh)
		cards = append(cards, c)
		p.Append(c)
	}
	return p, cards
}

// Collapsed, the front card shows whole at the top and two cards peek
// below it, each a step lower and narrower, at the front's height; the
// rest lie buried under the last peek. Only the front takes the pointer
// (#332).
func TestPileCollapsed(t *testing.T) {
	p, cards := pileOf(t, 40, 60, 30, 50)
	settle(t, p, 200, 400)
	if sz := p.Measure(Constraints{Max: Size{W: 200, H: 400}}); sz != (Size{W: 200, H: 40 + 2*pilePeekOffset}) {
		t.Errorf("collapsed size %v, want the front plus two steps", sz)
	}
	want := []render.Rect{
		{X: 0, Y: 0, W: 200, H: 40},
		{X: 8, Y: 8, W: 184, H: 40},
		{X: 16, Y: 16, W: 168, H: 40},
		{X: 16, Y: 16, W: 168, H: 40},
	}
	for i, c := range cards {
		if c.Bounds() != want[i] {
			t.Errorf("card %d at %v, want %v", i, c.Bounds(), want[i])
		}
	}
	for i, class := range []string{pileFrontClass, pilePeekClass, pilePeekClass, pileBuriedClass} {
		if !cards[i].HasClass(class) {
			t.Errorf("card %d lacks %q", i, class)
		}
	}
	if p.HasClass(pileOpenClass) {
		t.Error("a collapsed pile is expanded")
	}
	if hit := p.HitTest(Point{X: 100, Y: 50}); hit == Widget(cards[1]) || hit == Widget(cards[2]) {
		t.Errorf("a peeking card took the pointer: %T", hit)
	}
	if hit := p.HitTest(Point{X: 100, Y: 10}); hit != Widget(cards[0]) {
		t.Errorf("the front did not take the pointer: %T", hit)
	}
}

// Expanded, the cards lay out as a column at their own heights with
// the gap, every one taking the pointer; peeking past the top edge the
// front sits lowest, collapsed and expanded.
func TestPileExpanded(t *testing.T) {
	p, cards := pileOf(t, 40, 60, 30)
	p.SetExpanded(true)
	settle(t, p, 200, 400)
	if sz := p.Measure(Constraints{Max: Size{W: 200, H: 400}}); sz.H != 40+60+30+2*pileSpacing {
		t.Errorf("expanded height %d", sz.H)
	}
	if b := cards[2].Bounds(); b != (render.Rect{X: 0, Y: 40 + 60 + 2*pileSpacing, W: 200, H: 30}) {
		t.Errorf("third card at %v", b)
	}
	if !p.HasClass(pileOpenClass) || cards[1].HasClass(pilePeekClass) {
		t.Error("expanded classes wrong")
	}
	if hit := p.HitTest(Point{X: 100, Y: 60}); hit != Widget(cards[1]) {
		t.Errorf("an expanded card did not take the pointer: %T", hit)
	}
	p.SetPeekEdge(EdgeTop)
	settle(t, p, 200, 146)
	if b := cards[0].Bounds(); b.Y+b.H != 146 {
		t.Errorf("expanded from the bottom, the front ends at %d, want 146", b.Y+b.H)
	}
	p.SetExpanded(false)
	settle(t, p, 200, 56)
	if f, k := cards[0].Bounds(), cards[1].Bounds(); f.Y != 16 || k.Y != 8 {
		t.Errorf("peeking up: front at %d, first peek at %d; want 16 and 8", f.Y, k.Y)
	}
}

// A peeking card paints only its own box: its background shows past
// the front, its content never does.
func TestPilePeeksPaintTheirBoxOnly(t *testing.T) {
	loadCSS(t, `.card { background-color: #0000ff; }`)
	t.Cleanup(anim.SetInstant(true))
	p := NewPile()
	for range 2 {
		card := NewBox(Column, 0, 0)
		card.AddClass("card")
		card.Append(&solidLeaf{sz: Size{W: 200, H: 40}, col: render.RGB(255, 0, 0)}, false)
		p.Append(card)
	}
	settle(t, p, 200, 60)
	data := paintTree(p, 200, 60)
	at := func(x, y int) render.Color { return render.ColorFromBytes(data[y*render.Stride(200)+x*4:]) }
	if got := at(100, 20); got != render.RGB(255, 0, 0) {
		t.Errorf("front content %#08x, want red", uint32(got))
	}
	if got := at(100, 44); got != render.RGB(0, 0, 0xff) {
		t.Errorf("past the front, the peek shows %#08x, want its blue box, not its red content", uint32(got))
	}
}

// Expanding moves the cards on their layout spring: mid-flight a card
// shows between its stacked and its column place.
func TestPileExpandAnimates(t *testing.T) {
	c := pinAnimClock(t)
	p := NewPile()
	cards := []*stub{newStub(200, 40), newStub(200, 40)}
	for _, card := range cards {
		p.Append(card)
	}
	settle(t, p, 200, 400)
	paintTree(p, 200, 400)
	from := ShownRect(cards[1]).Y
	p.SetExpanded(true)
	settle(t, p, 200, 400)
	paintTree(p, 200, 400)
	c.step()
	c.step()
	paintTree(p, 200, 400)
	to := cards[1].Bounds().Y
	if y := ShownRect(cards[1]).Y; y <= from || y >= to {
		t.Errorf("mid-flight the second card shows at %d, want between %d and %d", y, from, to)
	}
}
