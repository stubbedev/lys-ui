package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// Shrinker is a widget that can be arranged shorter than its natural
// height and still do its job: a Scroll scrolls what no longer fits.
// A Box short of room takes the shortfall from every child that can
// give it — GTK's distribution, each to its floor — and whatever
// still does not fit overflows as before. Boxes and stacks report
// what their children can give, so the shrink reaches a Scroll nested
// inside them.
type Shrinker interface {
	// Shrinkable is how many pixels of its last measured height the
	// widget can give up.
	Shrinkable() int
}

// shrinkableOf is w's Shrinkable, 0 for a widget that cannot shrink,
// never past its floor.
func shrinkableOf(w Widget) int {
	if s, ok := w.(Shrinker); ok {
		return floored(w, max(0, s.Shrinkable()), Column)
	}
	return 0
}

// floored caps what w can give up on axis at its floor: a widget is
// never squeezed below its size request or its CSS min (a definite CSS
// size included), whatever its own content could shrink to.
func floored(w Widget, give int, axis Axis) int {
	n := nodeOf(w)
	if n == nil || give == 0 {
		return give
	}
	floor, last := n.layout.request.H, n.measureOut.H
	if axis == Row {
		floor, last = n.layout.request.W, n.measureOut.W
	}
	if v := n.style(w); cssSized(v) {
		if axis == Row {
			floor = max(floor, n.cssMin.W)
		} else {
			floor = max(floor, n.cssMin.H)
		}
	}
	if floor == 0 {
		return give
	}
	return min(give, max(0, last-floor))
}

// Shrinkable implements Shrinker: a column gives what its children
// can; a row is as short as its least shrinkable child lets it be; a
// grid down to its min-content height.
func (b *Box) Shrinkable() int {
	if layoutModeOf(b.style(b)) == modeGrid {
		return b.gridShrinkable(Column)
	}
	if b.flowAxis() == Column {
		total := 0
		for _, c := range b.child {
			if flows(c.w) {
				total += shrinkableOf(c.w)
			}
		}
		return total
	}
	tallest, floor := 0, 0
	for _, c := range b.child {
		if !flows(c.w) {
			continue
		}
		tallest = max(tallest, c.nat.H)
		floor = max(floor, c.nat.H-shrinkableOf(c.w))
	}
	return tallest - floor
}

// WidthShrinker is Shrinker's horizontal counterpart: a widget that
// can be arranged narrower than its natural width and still read — an
// ellipsizing label truncates. A Row Box short of room takes the
// shortfall from every child that can give it, as GTK's box gives way
// down to an ellipsized label's minimum, never below each one's
// floor; whatever still does not fit overflows as before.
type WidthShrinker interface {
	// ShrinkableWidth is how many pixels of its last measured width the
	// widget can give up.
	ShrinkableWidth() int
}

// shrinkableWidthOf is w's ShrinkableWidth, 0 for a widget that cannot
// narrow.
func shrinkableWidthOf(w Widget) int {
	if s, ok := w.(WidthShrinker); ok {
		return floored(w, max(0, s.ShrinkableWidth()), Row)
	}
	return 0
}

// ShrinkableWidth implements WidthShrinker: a row gives what its
// children can; a column is as narrow as its least shrinkable child
// lets it be; a grid down to its min-content width; and the CSS
// layouts that break lines between items down to their widest item.
func (b *Box) ShrinkableWidth() int {
	v := b.style(b)
	switch mode := layoutModeOf(v); mode {
	case modeGrid:
		return b.gridShrinkable(Row)
	case modeFlow:
		return b.lineShrinkable(v, mode)
	case modeFlex:
		if main, _ := b.flexDirection(v); main == Column || v.Layout.FlexWrap != style.FlexNowrap {
			return b.lineShrinkable(v, mode)
		}
	}
	if b.flowAxis() == Row {
		total := 0
		for _, c := range b.child {
			if flows(c.w) {
				total += shrinkableWidthOf(c.w)
			}
		}
		return total
	}
	widest, floor := 0, 0
	for _, c := range b.child {
		if !flows(c.w) {
			continue
		}
		widest = max(widest, c.nat.W)
		floor = max(floor, c.nat.W-shrinkableWidthOf(c.w))
	}
	return widest - floor
}

// lineShrinkable is ShrinkableWidth for a CSS layout whose min-content
// width is its widest item's min-content contribution: normal flow,
// whose lines break between inline items, a multi-line flex row and a
// column flex container (css-sizing-3 §5.1, css-flexbox-1 §9.9.1). It
// is what the last measure's content width gives up to reach it.
func (b *Box) lineShrinkable(v *style.Values, mode layoutMode) int {
	o := b.box(v).outer()
	last := b.measureOut.W - o.Left - o.Right
	p := b.beginPass(v, mode, true)
	defer p.end()
	widest := 0
	for i := range p.items {
		if it := &p.items[i]; it.kind == itemFlow {
			nat := measureOuter(it.w, Constraints{Max: Size{W: indefinite, H: indefinite}})
			widest = max(widest, minContentOf(it.w, Row, nat))
		}
	}
	return max(0, last-widest)
}

// shrinkBufs holds shrinkTakes' caps and takes per layout depth. The
// caller pushes a level, keeps it while its loop reads the takes, and
// pops it after: the loop measures and arranges the children, whose
// own shrinks run one level deeper.
var shrinkBufs depthStack[int]

// shrinkTakes splits a main-axis deficit across the children that can
// give in proportion to what each can give, never more than that:
// takes[i] is what child i gives up, nil when nobody can give. GTK's
// distribution: every child that can give participates, on either
// axis. buf is the caller's shrinkBufs level, two per child long.
func (b *Box) shrinkTakes(buf []int, deficit int) (takes []int) {
	if deficit <= 0 {
		return nil
	}
	give := shrinkableOf
	if b.axis == Row {
		give = shrinkableWidthOf
	}
	n := len(b.child)
	caps, takes := buf[:n], buf[n:2*n]
	total := 0
	for i, c := range b.child {
		caps[i] = 0
		if flows(c.w) {
			caps[i] = give(c.w)
			total += caps[i]
		}
	}
	if total == 0 {
		return nil
	}
	deficit = min(deficit, total)
	given := 0
	for i, c := range caps {
		takes[i] = deficit * c / total
		given += takes[i]
	}
	// The rounding remainder goes to whoever still has room, in order.
	for i := range takes {
		if given == deficit {
			break
		}
		if more := min(caps[i]-takes[i], deficit-given); more > 0 {
			takes[i] += more
			given += more
		}
	}
	return takes
}

// Shrinkable implements Shrinker: every page is arranged in the same
// rect, so the stack can shrink as far as its least shrinkable page
// lets it - or, sized to its visible page, as far as that page lets
// it, tweened with the size through an interpolating switch.
func (s *Stack) Shrinkable() int {
	if s.heterogeneous {
		floor := s.pageFloor(s.visible)
		if s.prev != "" && s.interpolate {
			floor = s.lerp(s.pageFloor(s.prev), floor)
		}
		return max(0, s.visibleSize().H-floor)
	}
	tallest, floor := 0, 0
	for _, name := range s.order {
		tallest = max(tallest, s.measured[name].H)
		floor = max(floor, s.pageFloor(name))
	}
	return tallest - floor
}

// Shrinkable implements Shrinker: every child shares the overlay's
// rect, so it gives way as far as the child that needs the most height
// lets it, a scrolled main child carrying its overlays down with it.
func (o *Overlay) Shrinkable() int {
	tallest, floor := 0, 0
	for i, k := range o.kids {
		if i >= len(o.nats) || !IsVisible(k) {
			continue
		}
		tallest = max(tallest, o.nats[i])
		floor = max(floor, o.nats[i]-shrinkableOf(k))
	}
	return tallest - floor
}

// pageFloor is the least height a page works at: its last measured
// height less what it can give up.
func (s *Stack) pageFloor(name string) int {
	k, ok := s.kids[name]
	if !ok {
		return 0
	}
	return s.measured[name].H - shrinkableOf(k)
}

// PageFloor is the most any page needs at con: its natural height less
// what it can give up (a scrolled list gives way). A holder that cannot
// grow once mapped - a popup - reserves it up front for a stack sized
// to its visible page.
func (s *Stack) PageFloor(con Constraints) int {
	box := boxOf(s.style(s), render.Insets{})
	inner, o := box.innerOf(con), box.outer()
	floor := 0
	for _, name := range s.order {
		s.measured[name] = measureChild(s, s.kids[name], inner)
		floor = max(floor, s.pageFloor(name))
	}
	return floor + o.Top + o.Bottom
}

// scrollFloor is the least height a Scroll gives way to: room for the
// bar to still read as one.
const scrollFloor = 4 * gutter

// Shrinkable implements Shrinker: a scroll gives up everything above
// its floor and scrolls the rest. Only one propagating its natural
// height asks for more than nothing, so only it has any to give: its
// minimum, as GTK's, stays below its content's height.
func (s *Scroll) Shrinkable() int { return max(0, s.measured.H-scrollFloor) }

// Shrinkable implements Shrinker: a list scrolls the rows that no
// longer fit, down to one row (a ListView's scrolled window giving
// way in a popover short of room).
func (l *List) Shrinkable() int { return max(0, l.measureOut.H-l.rowH) }
