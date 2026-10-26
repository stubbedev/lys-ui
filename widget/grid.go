package widget

import (
	"slices"

	"github.com/stubbedev/lys-lib/render"
)

// Align says how a grid child sits inside its cell along one axis when
// the cell is larger than the child's natural size.
type Align uint8

const (
	// AlignFill stretches the child across the whole cell. It is the
	// default, so plain Attach-ed children behave like Box children.
	AlignFill Align = iota
	// AlignStart pins the child to the cell's leading edge at its
	// natural size.
	AlignStart
	// AlignCenter centers the child in the cell at its natural size.
	AlignCenter
	// AlignEnd pins the child to the cell's trailing edge at its
	// natural size.
	AlignEnd
	// AlignBaseline lines a row Box's child up on the row's shared text
	// baseline at its natural height (GTK's valign baseline); children
	// with no baseline center, and across a column it pins to the start.
	AlignBaseline
)

// gridChild is one Grid child: the widget, the cell it is attached to,
// how many cells it spans, and how it sits inside the spanned run.
type gridChild struct {
	w                Widget
	col, row         int
	colSpan, rowSpan int
	hAlign, vAlign   Align
	// nat is the child's last measured natural size; the align pass
	// reads it the way Box reads childEntry.nat.
	nat Size
}

// Grid is a rows-and-columns container. Children are attached to cells -
// optionally spanning several - and the grid derives its tracks from the
// children: a column or row is as wide or tall as the tallest child
// claiming it alone, plus what spanning children need.
//
// Sizing per axis: natural mode keeps every track at its own maximum;
// homogeneous mode gives every track the largest track's extent.
//
// When the arranged rect is larger than the natural size, the surplus is
// split equally among the tracks (earlier tracks take the remainder), so
// a spanning child grows by the share of the tracks it spans. When it is
// smaller, the deficit is negotiated — the reason a grid exists where a
// Box only overflows: tracks shrink in proportion to their extents down
// to per-track floors, and only when every floor is reached does the
// content overflow and the painter's clip decide visibility. A floor is
// the largest MinSize among the track's single-cell children (spanning
// children are not floor sources — their need spreads, it does not pin);
// widgets without a MinSize have none, and the grid itself reports the
// sum of its floors so nested grids squeeze coherently.
//
// Children carry no expand flags: surplus and deficit both spread
// uniformly over the tracks (earlier tracks take the remainder), the one
// deterministic rule — a child that must claim the extra belongs in a
// Box, whose expand flag is that negotiation.
//
// A child smaller than its shrunken span keeps its natural size under
// AlignStart/Center/End (overflowing the span, exactly like a squeezed
// Box child keeps its natural size); AlignFill takes the shrunken span
// as given, so the two containers agree visually.
//
// Children paint in attach order, so a later attach overlapping an
// earlier span paints on top.
type Grid struct {
	node
	colSpacing int
	rowSpacing int
	colHomog   bool
	rowHomog   bool
	child      []*gridChild

	// colW/rowH are the per-track extents Measure computed; colMin/
	// rowMin the per-track floors (see the type comment); Arrange
	// expands or squeezes the extents into the final rect. Box keeps
	// the same measure-then-arrange contract through childEntry.nat.
	colW, rowH     []int
	colMin, rowMin []int
}

// NewGrid returns an empty grid with the given spacing between columns
// and between rows.
func NewGrid(colSpacing, rowSpacing int) *Grid {
	return &Grid{colSpacing: colSpacing, rowSpacing: rowSpacing}
}

// SetColumnSpacing sets the gap between columns and returns the grid for
// chaining. The measure cache drops so the next frame reflows.
func (g *Grid) SetColumnSpacing(n int) *Grid {
	g.colSpacing = n
	g.InvalidateLayout()
	return g
}

// SetRowSpacing sets the gap between rows and returns the grid for
// chaining.
func (g *Grid) SetRowSpacing(n int) *Grid {
	g.rowSpacing = n
	g.InvalidateLayout()
	return g
}

// SetColumnHomogeneous toggles whether every column takes the widest
// column's extent (true) or its own maximum (false, the default).
func (g *Grid) SetColumnHomogeneous(h bool) *Grid {
	g.colHomog = h
	g.InvalidateLayout()
	return g
}

// SetRowHomogeneous toggles whether every row takes the tallest row's
// extent (true) or its own maximum (false, the default).
func (g *Grid) SetRowHomogeneous(h bool) *Grid {
	g.rowHomog = h
	g.InvalidateLayout()
	return g
}

// ColumnSpacing returns the gap between columns.
func (g *Grid) ColumnSpacing() int { return g.colSpacing }

// RowSpacing returns the gap between rows.
func (g *Grid) RowSpacing() int { return g.rowSpacing }

// ColumnHomogeneous reports whether every column takes the widest
// column's extent.
func (g *Grid) ColumnHomogeneous() bool { return g.colHomog }

// RowHomogeneous reports whether every row takes the tallest row's
// extent.
func (g *Grid) RowHomogeneous() bool { return g.rowHomog }

// Attach puts w at (col, row) with the given spans and returns the grid
// for chaining. Spans below one and negative coordinates clamp to one
// and zero respectively.
//
// Two conflict rules, both last-wins:
//   - attaching a widget that is already attached moves it to the new
//     cell (a widget sits in the grid once);
//   - after the dust settles, whoever occupies the target cell that is
//     not the attached widget is detached - including a widget the move
//     lands on top of.
//
// Overlapping a span without sharing its origin cell is not a conflict:
// both children stay, the later attach paints and hits on top. Every
// attach drops the measure cache.
func (g *Grid) Attach(w Widget, col, row, colSpan, rowSpan int) *Grid {
	col, row = max(0, col), max(0, row)
	colSpan, rowSpan = max(1, colSpan), max(1, rowSpan)
	moved := false
	for _, c := range g.child {
		if c.w == w {
			c.col, c.row, c.colSpan, c.rowSpan = col, row, colSpan, rowSpan
			moved = true
			break
		}
	}
	if !moved {
		g.child = append(g.child, &gridChild{
			w: w, col: col, row: row, colSpan: colSpan, rowSpan: rowSpan,
		})
	}
	// A displaced child leaves the tree entirely: detach it like
	// Grid.Remove — hook while still linked, then clear its parent.
	var displaced []Widget
	for _, c := range g.child {
		if c.w != w && c.col == col && c.row == row {
			displaced = append(displaced, c.w)
		}
	}
	for _, d := range displaced {
		notifyRemoved(d)
	}
	g.child = slices.DeleteFunc(g.child, func(c *gridChild) bool {
		return c.w != w && c.col == col && c.row == row
	})
	clearParents(displaced...)
	g.InvalidateLayout()
	return g
}

// Place moves an attached child to a new cell in place, without
// displacing whoever sits there, so a batch of moves can pass through
// overlapping states: the widget stays attached and keeps its focus.
// Unknown widgets are ignored.
func (g *Grid) Place(w Widget, col, row, colSpan, rowSpan int) *Grid {
	for _, c := range g.child {
		if c.w == w {
			c.col, c.row = max(0, col), max(0, row)
			c.colSpan, c.rowSpan = max(1, colSpan), max(1, rowSpan)
			g.InvalidateLayout()
			return g
		}
	}
	return g
}

// SetAlign selects how w sits inside its spanned run along each axis.
// Unknown widgets are ignored.
func (g *Grid) SetAlign(w Widget, h, v Align) *Grid {
	for _, c := range g.child {
		if c.w == w {
			c.hAlign, c.vAlign = h, v
			g.Invalidate()
			return g
		}
	}
	return g
}

// Remove detaches w and reports whether it was attached. The removed
// widget's parent link clears and the removal hook fires, like every
// tree mutation; the measure cache drops so the next frame reflows
// without it.
func (g *Grid) Remove(w Widget) bool {
	for i, c := range g.child {
		if c.w == w {
			notifyRemoved(w)
			g.child = slices.Delete(g.child, i, i+1)
			clearParents(w)
			g.InvalidateLayout()
			return true
		}
	}
	return false
}

// Children exposes the children in row-major order - the visual order
// focus traversal follows, so Tab reads the grid left-to-right down the
// rows, a spanning child visited at its origin cell - regardless of the
// order attachments arrived in.
func (g *Grid) Children() []Widget {
	return g.appendChildren(make([]Widget, 0, len(g.child)))
}

// appendChildren appends the children in the same row-major order
// Children builds.
func (g *Grid) appendChildren(buf []Widget) []Widget {
	sorted := slices.Clone(g.child)
	slices.SortStableFunc(sorted, func(a, b *gridChild) int {
		if a.row != b.row {
			return a.row - b.row
		}
		return a.col - b.col
	})
	for _, c := range sorted {
		buf = append(buf, c.w)
	}
	return buf
}

// extents reports the track counts: one past the farthest occupied
// cell. A grid with no children is 0x0 and measures and arranges as
// nothing.
func (g *Grid) extents() (cols, rows int) {
	for _, c := range g.child {
		cols = max(cols, c.col+c.colSpan)
		rows = max(rows, c.row+c.rowSpan)
	}
	return cols, rows
}

// SetEnabled turns the grid's subtree on or off through the per-query
// enable walk, like Box.
func (g *Grid) SetEnabled(enabled bool) {
	g.node.SetEnabled(enabled)
	invalidateTree(g)
}

// Measure measures every child and derives the tracks: single-cell
// children set their track's maximum (and, for MinSizer children, its
// floor), then spanning children grow a run of tracks by whatever their
// natural size exceeds the run's current maxima plus internal spacing -
// the deficit spreads equally over the run, earlier tracks taking the
// remainder. The result is cached until an InvalidateLayout anywhere in
// the subtree or a different constraint arrives.
func (g *Grid) Measure(con Constraints) Size {
	if sz, ok := g.measureHit(con); ok {
		return sz
	}
	cols, rows := g.extents()
	g.colW = make([]int, cols)
	g.rowH = make([]int, rows)
	g.colMin = make([]int, cols)
	g.rowMin = make([]int, rows)
	for _, c := range g.child {
		c.nat = measureChild(g, c.w, Constraints{Max: con.Max})
	}
	for _, c := range g.child {
		if c.colSpan == 1 {
			g.colW[c.col] = max(g.colW[c.col], c.nat.W)
			g.colMin[c.col] = max(g.colMin[c.col], minSizeOf(c.w).W)
		}
		if c.rowSpan == 1 {
			g.rowH[c.row] = max(g.rowH[c.row], c.nat.H)
			g.rowMin[c.row] = max(g.rowMin[c.row], minSizeOf(c.w).H)
		}
	}
	for _, c := range g.child {
		growTracks(g.colW, g.colSpacing, c.col, c.colSpan, c.nat.W)
		growTracks(g.rowH, g.rowSpacing, c.row, c.rowSpan, c.nat.H)
	}
	// A floor is a minimum: a constraint-clamped measurement (a
	// wrapping label measured under a tiny width clips its rows below
	// its own token floor) must not pull a track under it, or the
	// squeeze pass would have nothing to defend.
	for i := range g.colW {
		g.colW[i] = max(g.colW[i], g.colMin[i])
	}
	for i := range g.rowH {
		g.rowH[i] = max(g.rowH[i], g.rowMin[i])
	}
	if g.colHomog {
		evenTracks(g.colW)
		evenTracks(g.colMin)
	}
	if g.rowHomog {
		evenTracks(g.rowH)
		evenTracks(g.rowMin)
	}
	nat := Size{
		W: trackSum(g.colW) + g.colSpacing*(cols-1),
		H: trackSum(g.rowH) + g.rowSpacing*(rows-1),
	}
	return g.measureStore(con, clampSize(nat, con))
}

// MinSize implements MinSizer: the sum of the track floors plus
// spacing, so a nested grid squeezes coherently instead of reporting
// no floor and collapsing under its parent's negotiation.
func (g *Grid) MinSize() Size {
	cols, rows := g.extents()
	return Size{
		W: trackSum(g.colMin) + g.colSpacing*max(0, cols-1),
		H: trackSum(g.rowMin) + g.rowSpacing*max(0, rows-1),
	}
}

// minSizeOf reports w's floor, zero for widgets without one.
func minSizeOf(w Widget) Size {
	if m, ok := w.(MinSizer); ok {
		return m.MinSize()
	}
	return Size{}
}

// growTracks widens the run of n tracks starting at i so want fits: the
// deficit beyond the run's current maxima plus internal spacing spreads
// equally over the run, earlier tracks taking the remainder.
func growTracks(tracks []int, spacing, i, n, want int) {
	if n <= 0 {
		return
	}
	run := 0
	for k := i; k < i+n; k++ {
		run += tracks[k]
	}
	run += spacing * (n - 1)
	deficit := want - run
	if deficit <= 0 {
		return
	}
	per, rem := deficit/n, deficit%n
	for k := range n {
		tracks[i+k] += per
		if k < rem {
			tracks[i+k]++
		}
	}
}

// evenTracks gives every track the largest track's extent.
func evenTracks(tracks []int) {
	best := 0
	for _, t := range tracks {
		best = max(best, t)
	}
	for i := range tracks {
		tracks[i] = best
	}
}

// trackSum adds the track extents up.
func trackSum(tracks []int) int {
	total := 0
	for _, t := range tracks {
		total += t
	}
	return total
}

// Arrange assigns every child its spanned run, sized by the fitted
// tracks, aligned inside it per its Align. The surplus beyond the
// natural size splits equally among the tracks (earlier tracks take the
// remainder); a smaller rect squeezes the tracks in proportion to their
// extents down to their floors, overflowing only once every floor is
// reached.
func (g *Grid) Arrange(r render.Rect) {
	g.ArrangeRoot(r)
	cols, rows := g.extents()
	if cols == 0 {
		return
	}
	if len(g.colW) != cols || len(g.rowH) != rows {
		// Arrange follows Measure the way Box reads childEntry.nat;
		// without measured tracks there is nothing sane to place, so
		// the children park at empty rects instead of panicking on
		// the missing slices.
		for _, c := range g.child {
			c.w.Arrange(render.Rect{})
		}
		return
	}
	fitTracks(g.colW, g.colMin, g.colSpacing, r.W)
	fitTracks(g.rowH, g.rowMin, g.rowSpacing, r.H)
	for _, c := range g.child {
		x, cw := trackExtent(g.colW, g.colSpacing, c.col, c.colSpan)
		y, ch := trackExtent(g.rowH, g.rowSpacing, c.row, c.rowSpan)
		cell := render.Rect{X: r.X + x, Y: r.Y + y, W: cw, H: ch}
		arrangeChild(c.w, alignRect(cell, c.nat, c.hAlign, c.vAlign))
	}
	setParents(g, g.Children()...)
}

// ArrangeRoot records the grid's own rect.
func (g *Grid) ArrangeRoot(r render.Rect) {
	g.node.Arrange(r)
}

// fitTracks fits the tracks into avail: a surplus splits equally over
// every track (earlier tracks taking the remainder); a deficit
// squeezes the tracks in proportion to their extents down to their
// floors — shrinkTracks owns that negotiation, and overflow past the
// floors is its verdict, not this one.
func fitTracks(tracks, floors []int, spacing, avail int) {
	if len(tracks) == 0 {
		return
	}
	natural := trackSum(tracks) + spacing*(len(tracks)-1)
	if avail > natural {
		extra := avail - natural
		per, rem := extra/len(tracks), extra%len(tracks)
		for i := range tracks {
			tracks[i] += per
			if i < rem {
				tracks[i]++
			}
		}
		return
	}
	content := max(0, avail-spacing*(len(tracks)-1))
	shrinkTracks(tracks, floors, trackSum(tracks)-content)
}

// shrinkTracks distributes a deficit across the tracks in proportion
// to their extents, never taking a track below its floor. Each pass
// makes one exact largest-remainder split over the tracks that still
// have room (integer share plus ranked rounding pixels, ties to the
// earlier track), each share capped by its room; a capped share spills
// into the next pass over the survivors. A deficit that survives every
// pass overflows — the tracks rest at their floors. Deterministic by
// construction: integer math only, and every pass applies at least one
// pixel while anything has room.
func shrinkTracks(tracks, floors []int, deficit int) {
	if deficit <= 0 {
		return
	}
	room := func(i int) int { return max(0, tracks[i]-floors[i]) }
	for deficit > 0 {
		var live []int
		total := 0
		for i := range tracks {
			if room(i) > 0 {
				live = append(live, i)
				total += tracks[i]
			}
		}
		if len(live) == 0 {
			return // every floor reached: overflow
		}
		type claim struct{ i, frac int }
		claims := make([]claim, 0, len(live))
		applied := 0
		for pos, i := range live {
			weight := tracks[i]
			take := min(deficit*weight/total, room(i))
			tracks[i] -= take
			applied += take
			// The fractional claim ranks this track's right to a
			// rounding pixel; the earlier track breaks ties.
			claims = append(claims, claim{i: i, frac: (deficit*weight%total)*len(live) - pos})
		}
		if remainder := deficit - applied; remainder > 0 {
			slices.SortFunc(claims, func(a, b claim) int { return b.frac - a.frac })
			for _, c := range claims {
				if remainder == 0 {
					break
				}
				if room(c.i) > 0 {
					tracks[c.i]--
					remainder--
					applied++
				}
			}
		}
		if applied == 0 {
			return
		}
		deficit -= applied
	}
}

// trackExtent returns the offset and size of the run of n tracks
// starting at i, spacing included inside the run but not before it.
func trackExtent(tracks []int, spacing, i, n int) (off, size int) {
	for k := range i {
		off += tracks[k] + spacing
	}
	for k := range n {
		size += tracks[i+k]
	}
	size += spacing * (n - 1)
	return off, size
}

// alignRect fits a child of natural size nat into its cell per the
// per-axis alignment: fill takes the whole cell, the rest keep the
// natural size and pin to an edge or center - a natural size larger
// than the cell overflows it rather than shrinking.
func alignRect(cell render.Rect, nat Size, h, v Align) render.Rect {
	out := cell
	if h != AlignFill {
		out.W = nat.W
		switch h {
		case AlignCenter:
			out.X = cell.X + (cell.W-nat.W)/2
		case AlignEnd:
			out.X = cell.X + cell.W - nat.W
		}
	}
	if v != AlignFill {
		out.H = nat.H
		switch v {
		case AlignCenter:
			out.Y = cell.Y + (cell.H-nat.H)/2
		case AlignEnd:
			out.Y = cell.Y + cell.H - nat.H
		}
	}
	return out
}

// Paint paints the children in attach order, so overlapping spans
// composite bottom to top.
func (g *Grid) Paint(cv *render.Canvas) {
	for _, c := range g.child {
		PaintChild(cv, c.w)
	}
}

// HitTest returns the topmost child under p (later attaches sit on
// top), or the grid itself when p is inside its bounds but over no
// child.
func (g *Grid) HitTest(p Point) Widget {
	for i := range slices.Backward(g.child) {
		if hit := HitChild(g.child[i].w, p); hit != nil {
			return hit
		}
	}
	return g.HitLeaf(g, p)
}
