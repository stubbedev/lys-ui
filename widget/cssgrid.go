package widget

// The CSS grid layout (CSS Grid Layout Module Level 2, docs/css.md) over
// a Box's items: the explicit grid of the template track lists and
// areas, auto-fill and auto-fit repetitions, line placement with named,
// negative and area lines and spans, auto-placement (§8.5) sparse or
// dense along either flow with implicit tracks sized by grid-auto-*, the
// track sizing algorithm (§11: intrinsic contributions of spanning
// items, maximized tracks, fr expansion, stretched auto tracks), content
// distribution, self alignment with baseline groups, and absolutely
// positioned items against the grid area their lines name. Columns lie
// along the Row axis and rows along the Column axis; an RTL grid lays
// its columns out logically and mirrors them at the end.

import (
	"math"
	"slices"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// gridSizing is how a grid sizes one axis: into a definite extent, or
// under a max-content or min-content constraint, its extent indefinite.
type gridSizing uint8

const (
	sizeDefinite gridSizing = iota
	sizeMaxContent
	sizeMinContent
)

// gridSpan is a run of grid lines, end exclusive of the tracks.
type gridSpan struct{ start, end int }

// size is the number of tracks the span covers.
func (s gridSpan) size() int { return s.end - s.start }

// gridContrib is one item's size contributions along an axis, outer
// pixels: its minimum contribution (the automatic minimum where that
// applies), and its min-content and max-content contributions.
type gridContrib struct {
	minimum, minC, maxC float64
}

// gridItem is a cssItem's grid scratch, indexed by axis: Row for the
// columns, Column for the rows.
type gridItem struct {
	// area is the item's grid area as zero-based lines of the final
	// grid; fixed marks the axes its placement properties fix before
	// auto-placement, and span the track count an auto axis takes.
	area  [2]gridSpan
	fixed [2]bool
	span  [2]int
	// self is the resolved justify-self (Row) and align-self (Column).
	self    [2]style.BoxAlign
	contrib [2]gridContrib
	// flex marks an axis where the item crosses a flexible track.
	flex [2]bool
	// width is the used outer width; height the content height at it.
	width, height int
	// baseline is the item's first baseline below its margin box top
	// (its bottom edge when it has none, hasBaseline false), and shim
	// the offset its baseline-sharing group adds above it (first
	// baseline) or below it (last baseline).
	baseline    int
	hasBaseline bool
	shim        float64
	// cb is an absolutely positioned item's containing block, relative
	// to the content box, when the grid resolved one from its lines.
	cb    render.Rect
	hasCB bool
}

// gridTrack is one track's sizing state. A length bound resolves to
// pixels (a percentage without a definite basis reads as auto), and a
// fit-content() limit sits in maxPx (infinite without a basis).
type gridTrack struct {
	minK, maxK   style.TrackKind
	minPx, maxPx float64
	fr           float64
	base, limit  float64
	// growable marks a limit the intrinsic-maximums step just made
	// finite: the max-content step may still grow it past it.
	growable bool
	// The distribution scratch: an item's increase, the step's planned
	// increase, the frozen flag, and whether any item touched the track.
	incr, planned   float64
	frozen, touched bool
	covered         bool
	collapsed       bool
	// above and below are a row's baseline-sharing groups' extents.
	above, below float64
	// start and end are the placed edges, in content-box pixels.
	start, end int
}

// flexible reports an fr maximum.
func (t *gridTrack) flexible() bool { return t.maxK == style.TrackFr }

// intrinsicMin reports a content-sized minimum.
func (t *gridTrack) intrinsicMin() bool {
	return t.minK == style.TrackAuto || t.minK == style.TrackMinContent || t.minK == style.TrackMaxContent
}

// intrinsicMax reports a content-sized maximum.
func (t *gridTrack) intrinsicMax() bool {
	return t.maxK == style.TrackAuto || t.maxK == style.TrackMinContent || t.maxK == style.TrackMaxContent ||
		t.maxK == style.TrackFitContent
}

// maxContentMax reports a maximum the max-content contributions size:
// max-content, auto, and fit-content() below its limit.
func (t *gridTrack) maxContentMax() bool {
	return t.maxK == style.TrackAuto || t.maxK == style.TrackMaxContent || t.maxK == style.TrackFitContent
}

// gridAxis is one axis of a grid pass.
type gridAxis struct {
	tpl  *style.TrackList
	auto *style.AutoTracks
	// reps is the auto repetition count; tplCount the tracks the
	// template sizes, explicit the explicit grid's (the template's or
	// the areas', whichever is more), offset the implicit tracks before
	// it.
	reps, tplCount, explicit, offset int
	tracks                           []gridTrack
	gap                              int
	sizing                           gridSizing
	// size is the content box extent, read when sizing is definite.
	size int
}

// gaps is the gutter total between n tracks.
func (ax *gridAxis) gaps(n int) float64 { return float64(ax.gap * max(0, n-1)) }

// shown counts the tracks auto-fit did not collapse.
func (ax *gridAxis) shown() int {
	n := 0
	for i := range ax.tracks {
		n += btoi(!ax.tracks[i].collapsed)
	}
	return n
}

// used is the extent the tracks and their gutters take.
func (ax *gridAxis) used() float64 {
	sum := ax.gaps(ax.shown())
	for i := range ax.tracks {
		sum += ax.tracks[i].base
	}
	return sum
}

// areaOf is the placed extent of span sp: the first track's start to
// the last track's end.
func (ax *gridAxis) areaOf(sp gridSpan) (start, size int) {
	return ax.tracks[sp.start].start, ax.tracks[sp.end-1].end - ax.tracks[sp.start].start
}

// gridPass is one grid layout run over a cssPass's items. It holds the
// pass's parts rather than the pass, which stays on its caller's stack.
type gridPass struct {
	items []cssItem
	b     *Box
	v     *style.Values
	rtl   bool
	ax    [2]gridAxis
	flow  style.GridAutoFlow
	areas *style.GridAreas
	// base is the grid's first baseline, when hasBase.
	base    int
	hasBase bool
}

// The grid passes' buffers, one per nesting level: the tracks of both
// axes, the auto-placement occupancy bits, and the sparse cursors of
// the items locked to a row (or column).
var (
	gridTracks  depthStack[gridTrack]
	gridCells   depthStack[uint64]
	gridCursors depthStack[int]
)

var inf = math.Inf(1)

// gridMeasure is a grid container's request along axis at forSize:
// across, the columns sized under a min-content and a max-content
// constraint; down, the rows sized so at the width given (the columns
// sized into it), or at the max-content width.
func (p *cssPass) gridMeasure(axis Axis, forSize int) Sizes {
	size, cols := indefiniteSize, sizeMaxContent
	if axis == Column && forSize != Unknown {
		size.W, cols = forSize, sizeDefinite
	}
	if axis == Row {
		return Sizes{
			Min: p.grid(size, [2]gridSizing{sizeMinContent, sizeMaxContent}).W,
			Nat: p.grid(size, [2]gridSizing{sizeMaxContent, sizeMaxContent}).W,
		}
	}
	return Sizes{
		Min: p.grid(size, [2]gridSizing{cols, sizeMinContent}).H,
		Nat: p.grid(size, [2]gridSizing{cols, sizeMaxContent}).H,
	}
}

// grid lays the items out as a grid container whose content box is
// size, each axis sized as sizing says (size's extent read where it is
// definite). It returns the content size the tracks and gutters take.
func (p *cssPass) grid(size Size, sizing [2]gridSizing) Size {
	l := &p.v.Layout
	g := gridPass{items: p.items, b: p.b, v: p.v, rtl: p.rtl, flow: l.GridAutoFlow, areas: l.GridTemplateAreas}
	for _, a := range [2]Axis{Row, Column} {
		ax := &g.ax[a]
		ax.sizing, ax.size = sizing[a], size.Along(a)
		ax.tpl, ax.auto = l.GridTemplateColumns, l.GridAutoColumns
		areas := 0
		if g.areas != nil {
			areas = g.areas.Columns
		}
		if a == Column {
			ax.tpl, ax.auto = l.GridTemplateRows, l.GridAutoRows
			if g.areas != nil {
				areas = g.areas.Rows
			}
		}
		if ax.tpl != nil && ax.tpl.Subgrid {
			ax.tpl = nil
		}
		ax.gap = p.gapAlong(a, g.basis(a))
		ax.reps = g.repetitions(a)
		if ax.tpl != nil {
			ax.tplCount = ax.tpl.Count(ax.reps)
		}
		ax.explicit = max(ax.tplCount, areas)
	}
	ordered := false
	for i := range p.items {
		it := &p.items[i]
		it.grid = gridItem{}
		ordered = ordered || it.kind == itemFlow && it.v.Layout.Order != 0
	}
	if ordered {
		slices.SortStableFunc(p.items, func(a, b cssItem) int { return a.v.Layout.Order - b.v.Layout.Order })
	}
	cols, rows := g.place()
	tracks := gridTracks.push(cols + rows)
	defer gridTracks.pop()
	g.ax[Row].tracks, g.ax[Column].tracks = tracks[:cols], tracks[cols:]
	g.initTracks(Row)
	g.initTracks(Column)

	for i := range p.items {
		if it := &p.items[i]; it.kind == itemFlow {
			for _, a := range [2]Axis{Row, Column} {
				self, items := style.PropJustifySelf, style.PropJustifyItems
				if a == Column {
					self, items = style.PropAlignSelf, style.PropAlignItems
				}
				it.grid.self[a] = gridSelf(p.selfAlignOf(it, a, a == otherAxis(p.b.axis), self, items), it, a, p.rtl)
			}
			req := measureOuter(it.w, Row, Unknown, indefiniteSize)
			g.contribute(it, Row, req.Nat, req.Min)
		}
	}
	g.sizeTracks(Row)
	width := g.position(Row)
	g.contributeRows()
	g.sizeTracks(Column)
	height := g.position(Column)
	g.placeItems(width)
	g.resolveOutOfFlow(width, height)
	g.firstBaseline()
	p.base, p.hasBase = g.base, g.hasBase
	return Size{W: int(math.Round(g.ax[Row].used())), H: int(math.Round(g.ax[Column].used()))}
}

// basis is the extent percentages resolve against along a: the content
// box when definite, indefinite otherwise.
func (g *gridPass) basis(a Axis) int {
	if g.ax[a].sizing == sizeDefinite {
		return g.ax[a].size
	}
	return indefinite
}

// repetitions is the auto repetition count along a (§7.2.3.2): the most
// repetitions that fit a definite extent or maximum, else the fewest
// that reach a definite minimum, else one; each track counts as its
// fixed bound, and the gutters count too.
func (g *gridPass) repetitions(a Axis) int {
	ax := &g.ax[a]
	t := ax.tpl
	if t == nil || t.Repeat == style.RepeatNone {
		return 0
	}
	space, fill := -1, true
	if ax.sizing == sizeDefinite {
		space = ax.size
	} else {
		s := resolveAxis(g.b, g.v, a, indefinite)
		switch {
		case finite(s.max):
			space = s.ax.content(s.max)
		case s.min > 0:
			space, fill = s.ax.content(s.min), false
		}
	}
	if space < 0 {
		return 1
	}
	fixed := func(r style.TrackRun) (sum int) {
		for _, s := range r.Sizes {
			b := s.Max
			if b.Kind != style.TrackLength {
				b = s.Min
			}
			sum += max(0, b.Len.Resolve(space)) + ax.gap
		}
		return sum
	}
	outside := fixed(t.Head) + fixed(t.Tail) - ax.gap
	per := fixed(t.Auto)
	if per <= 0 {
		return 1
	}
	room := space - outside
	n := room / per
	if !fill && n*per < room {
		n++
	}
	limit := (style.GridLineLimit - len(t.Head.Sizes) - len(t.Tail.Sizes)) / len(t.Auto.Sizes)
	return max(1, min(n, limit))
}

// named reports whether explicit line i along a carries name+suffix:
// from the template's line names or the areas' implicit edge names.
func (g *gridPass) named(a Axis, i int, name, suffix string) bool {
	ax := &g.ax[a]
	if ax.tpl != nil && i <= ax.tplCount && ax.tpl.LineNamed(i, ax.reps, name, suffix) {
		return true
	}
	return g.areas.LineNamed(a == Row, i, name, suffix)
}

// seek finds the n-th line called name from line from in direction dir
// (+1 or -1). Explicit lines count by their names; past the explicit
// grid on the side the search runs to, every implicit line counts as so
// named, as CSS assumes, and the implicit lines behind the search never
// do.
func (g *gridPass) seek(a Axis, name string, from, dir, n int) int {
	e := g.ax[a].explicit
	start := max(from, 0)
	if dir < 0 {
		start = min(from, e)
	}
	for i := start; i >= 0 && i <= e; i += dir {
		if g.named(a, i, name, "") {
			n--
			if n == 0 {
				return i
			}
		}
	}
	if dir > 0 {
		return max(e, from-1) + n
	}
	return min(0, from+1) - n
}

// lineOf resolves a definite placement line along a to an explicit line
// index: a lone identifier names an area edge (<ident>-start on a start
// side, <ident>-end on an end side) or else the first line so named; a
// number counts lines, those called its name when it has one, from the
// end of the explicit grid when negative.
func (g *gridPass) lineOf(a Axis, l style.GridLine, end bool) int {
	e := g.ax[a].explicit
	if l.Kind == style.GridLineIdent {
		suffix := "-start"
		if end {
			suffix = "-end"
		}
		for i := 0; i <= e; i++ {
			if g.named(a, i, l.Name, suffix) {
				return i
			}
		}
		return g.seek(a, l.Name, 0, 1, 1)
	}
	switch {
	case l.Name != "" && l.N > 0:
		return g.seek(a, l.Name, 0, 1, l.N)
	case l.Name != "":
		return g.seek(a, l.Name, e, -1, -l.N)
	case l.N > 0:
		return l.N - 1
	}
	return e + 1 + l.N
}

// spanFrom resolves the side across from a definite line from, dir +1
// for an end side: auto spans one track, span N spans N, and a named
// span reaches the N-th line so called.
func (g *gridPass) spanFrom(a Axis, from int, l style.GridLine, dir int) int {
	switch {
	case l.Kind == style.GridLineSpan && l.Name != "":
		return g.seek(a, l.Name, from+dir, dir, l.N)
	case l.Kind == style.GridLineSpan:
		return from + dir*l.N
	}
	return from + dir
}

// definiteLine reports a line that names a position: an identifier or a
// number.
func definiteLine(l style.GridLine) bool {
	return l.Kind == style.GridLineIdent || l.Kind == style.GridLineNumber
}

// clampLine holds a resolved line inside the lines CSS keeps.
func clampLine(n int) int { return max(-style.GridLineLimit, min(n, style.GridLineLimit)) }

// linesOf is an item's placement lines along a.
func linesOf(it *cssItem, a Axis) (start, end style.GridLine) {
	l := &it.v.Layout
	if a == Row {
		return l.GridColumnStart, l.GridColumnEnd
	}
	return l.GridRowStart, l.GridRowEnd
}

// placeAxis resolves one axis of an item's placement (§8.3): a definite
// line span in explicit line indexes, or an auto position spanning n
// tracks. Two spans drop the end one; a lone named span spans one
// track; lines that cross swap, and equal lines span one track.
func (g *gridPass) placeAxis(a Axis, s, e style.GridLine) (sp gridSpan, fixed bool, n int) {
	if s.Kind == style.GridLineSpan && e.Kind == style.GridLineSpan {
		e = style.GridLine{}
	}
	switch {
	case definiteLine(s) && definiteLine(e):
		sp = gridSpan{g.lineOf(a, s, false), g.lineOf(a, e, true)}
	case definiteLine(s):
		lo := g.lineOf(a, s, false)
		sp = gridSpan{lo, g.spanFrom(a, lo, e, 1)}
	case definiteLine(e):
		hi := g.lineOf(a, e, true)
		sp = gridSpan{g.spanFrom(a, hi, s, -1), hi}
	default:
		n = 1
		switch {
		case s.Kind == style.GridLineSpan && s.Name == "":
			n = s.N
		case e.Kind == style.GridLineSpan && e.Name == "":
			n = e.N
		}
		return gridSpan{}, false, n
	}
	sp.start, sp.end = clampLine(sp.start), clampLine(sp.end)
	if sp.start > sp.end {
		sp.start, sp.end = sp.end, sp.start
	}
	if sp.start == sp.end {
		if sp.start == style.GridLineLimit {
			sp.start--
		}
		sp.end = sp.start + 1
	}
	return sp, true, sp.size()
}

// gridOccupancy is the auto-placement cell map: a bit per cell, rows of
// the major (flow-crossing) axis, stride cells of the minor axis each.
type gridOccupancy struct {
	bits   []uint64
	stride int
}

// free reports whether the cells of the major and minor spans are all
// empty.
func (o gridOccupancy) free(major, minor gridSpan) bool {
	for r := major.start; r < major.end; r++ {
		for c := minor.start; c < minor.end; c++ {
			if i := r*o.stride + c; o.bits[i/64]&(1<<(i%64)) != 0 {
				return false
			}
		}
	}
	return true
}

// mark fills the cells of the major and minor spans.
func (o gridOccupancy) mark(major, minor gridSpan) {
	for r := major.start; r < major.end; r++ {
		for c := minor.start; c < minor.end; c++ {
			i := r*o.stride + c
			o.bits[i/64] |= 1 << (i % 64)
		}
	}
}

// place positions every in-flow item (§8.5) and returns the implicit
// grid's column and row counts, recording each axis's leading implicit
// tracks as its offset. The minor axis is the one auto-placement fills
// (the columns under a row flow), the major the one it grows.
func (g *gridPass) place() (cols, rows int) {
	minor, major := Row, Column
	if g.flow.Column {
		minor, major = Column, Row
	}
	lo := [2]int{}
	hi := [2]int{g.ax[Row].explicit, g.ax[Column].explicit}
	locked, autoMajor, widest := 0, 0, 0
	for i := range g.items {
		it := &g.items[i]
		if it.kind != itemFlow {
			continue
		}
		gi := &it.grid
		for _, a := range [2]Axis{Row, Column} {
			s, e := linesOf(it, a)
			gi.area[a], gi.fixed[a], gi.span[a] = g.placeAxis(a, s, e)
			if gi.fixed[a] {
				lo[a], hi[a] = min(lo[a], gi.area[a].start), max(hi[a], gi.area[a].end)
			}
		}
		if !gi.fixed[minor] {
			widest = max(widest, gi.span[minor])
			if gi.fixed[major] {
				locked += gi.span[minor]
			}
		}
		if !gi.fixed[major] {
			autoMajor += gi.span[major]
		}
	}
	for _, a := range [2]Axis{Row, Column} {
		g.ax[a].offset = -lo[a]
		hi[a] -= lo[a]
	}
	for i := range g.items {
		if it := &g.items[i]; it.kind == itemFlow {
			for _, a := range [2]Axis{Row, Column} {
				if it.grid.fixed[a] {
					it.grid.area[a].start += g.ax[a].offset
					it.grid.area[a].end += g.ax[a].offset
				}
			}
		}
	}
	// A placement never lands past the cells already occupied plus its
	// own span, which bounds the map.
	stride := max(hi[minor]+locked, widest)
	majors := hi[major] + autoMajor
	occ := gridOccupancy{bits: gridCells.push((stride*majors + 63) / 64), stride: stride}
	defer gridCells.pop()
	for i := range g.items {
		if it := &g.items[i]; it.kind == itemFlow && it.grid.fixed[minor] && it.grid.fixed[major] {
			occ.mark(it.grid.area[major], it.grid.area[minor])
		}
	}
	dense := g.flow.Dense
	cursors := gridCursors.push(majors)
	defer gridCursors.pop()
	for i := range g.items {
		it := &g.items[i]
		gi := &it.grid
		if it.kind != itemFlow || !gi.fixed[major] || gi.fixed[minor] {
			continue
		}
		row := gi.area[major]
		c := 0
		if !dense {
			c = cursors[row.start]
		}
		for !occ.free(row, gridSpan{c, c + gi.span[minor]}) {
			c++
		}
		gi.area[minor] = gridSpan{c, c + gi.span[minor]}
		occ.mark(row, gi.area[minor])
		cursors[row.start] = gi.area[minor].end
		hi[minor] = max(hi[minor], gi.area[minor].end)
	}
	count := max(hi[minor], widest)
	curMajor, curMinor := 0, 0
	for i := range g.items {
		it := &g.items[i]
		gi := &it.grid
		if it.kind != itemFlow || gi.fixed[major] {
			continue
		}
		if gi.fixed[minor] {
			c := gi.area[minor].start
			switch {
			case dense:
				curMajor = 0
			case c < curMinor:
				curMajor++
			}
			curMinor = c
			for !occ.free(gridSpan{curMajor, curMajor + gi.span[major]}, gi.area[minor]) {
				curMajor++
			}
		} else {
			if dense {
				curMajor, curMinor = 0, 0
			}
			for {
				if curMinor+gi.span[minor] > count {
					curMajor, curMinor = curMajor+1, 0
					continue
				}
				if occ.free(gridSpan{curMajor, curMajor + gi.span[major]}, gridSpan{curMinor, curMinor + gi.span[minor]}) {
					break
				}
				curMinor++
			}
			gi.area[minor] = gridSpan{curMinor, curMinor + gi.span[minor]}
		}
		gi.area[major] = gridSpan{curMajor, curMajor + gi.span[major]}
		occ.mark(gi.area[major], gi.area[minor])
		hi[major] = max(hi[major], gi.area[major].end)
	}
	hi[minor] = count
	return hi[Row], hi[Column]
}

// sizeOf is the sizing function of track t along a: the template's
// inside it, grid-auto-* for the area-only explicit tracks and the
// implicit ones — counted forward after the explicit grid and backward
// before it.
func (g *gridPass) sizeOf(a Axis, t int) style.TrackSize {
	ax := &g.ax[a]
	k := t - ax.offset
	switch {
	case k < 0:
		return ax.auto.At(k)
	case k < ax.tplCount:
		return ax.tpl.Size(k, ax.reps)
	case k < ax.explicit:
		return ax.auto.At(k - ax.tplCount)
	}
	return ax.auto.At(k - ax.explicit)
}

// initTracks resolves the tracks' sizing functions along a and collapses
// the empty tracks of an auto-fit repetition into fixed zero tracks.
func (g *gridPass) initTracks(a Axis) {
	ax := &g.ax[a]
	basis := g.basis(a)
	for t := range ax.tracks {
		s := g.sizeOf(a, t)
		tr := gridTrack{fr: s.Max.Fr}
		tr.minK, tr.minPx = breadthOn(s.Min, basis)
		tr.maxK, tr.maxPx = breadthOn(s.Max, basis)
		ax.tracks[t] = tr
	}
	for i := range g.items {
		if it := &g.items[i]; it.kind == itemFlow {
			for t := it.grid.area[a].start; t < it.grid.area[a].end; t++ {
				ax.tracks[t].covered = true
			}
		}
	}
	if ax.tpl == nil || ax.tpl.Repeat != style.RepeatFit {
		return
	}
	for t := range ax.tracks {
		if tr := &ax.tracks[t]; !tr.covered && ax.tpl.Repeated(t-ax.offset, ax.reps) {
			*tr = gridTrack{minK: style.TrackLength, maxK: style.TrackLength, collapsed: true}
		}
	}
}

// breadthOn resolves one track bound against basis.
func breadthOn(b style.TrackBreadth, basis int) (style.TrackKind, float64) {
	switch b.Kind {
	case style.TrackLength:
		if b.Len.Relative() && !finite(basis) {
			return style.TrackAuto, 0
		}
		return style.TrackLength, float64(max(0, b.Len.Resolve(basis)))
	case style.TrackFitContent:
		if b.Len.Relative() && !finite(basis) {
			return style.TrackFitContent, inf
		}
		return style.TrackFitContent, float64(max(0, b.Len.Resolve(basis)))
	}
	return b.Kind, 0
}

// gridSelf settles an item's self alignment along a — justify-self and
// justify-items in the row axis, align-self and align-items in the
// column axis, AppendAligned placing across the box — for a grid of
// direction rtl: normal stretches an item without an aspect ratio and
// starts one with it; self-start and self-end follow the item's own
// direction; the row axis has no baseline across it, so a baseline
// there falls back to start (last baseline to end).
func gridSelf(al style.BoxAlign, it *cssItem, a Axis, rtl bool) style.BoxAlign {
	switch al.Kind {
	case style.BoxNormal:
		al.Kind = style.BoxStretch
		if it.v.Has(style.PropAspectRatio) && it.v.Layout.AspectRatio > 0 {
			al.Kind = style.BoxStart
		}
	case style.BoxSelfStart, style.BoxSelfEnd:
		flipped := a == Row && it.v.Has(style.PropDirection) && (it.v.Direction == 2) != rtl
		if (al.Kind == style.BoxSelfStart) != flipped {
			al.Kind = style.BoxStart
		} else {
			al.Kind = style.BoxEnd
		}
	case style.BoxBaseline:
		if a == Row {
			al.Kind = style.BoxStart
		}
	case style.BoxLastBaseline:
		if a == Row {
			al.Kind = style.BoxEnd
		}
	}
	return al
}

// contribute records an item's contributions along a from its
// max-content and min-content outer extents (§11.5): clamped by its
// min and max sizes, a definite size being all three; the minimum
// contribution is its min size, or the automatic minimum where that
// applies — the min-content contribution, capped by a definite maximum
// and by the spanned tracks' fixed maximums.
func (g *gridPass) contribute(it *cssItem, a Axis, maxC, minC int) {
	gi := &it.grid
	sp := gi.area[a]
	tracks := g.ax[a].tracks[sp.start:sp.end]
	autoMin, fixedMax, capSum := false, true, g.ax[a].gaps(sp.size())
	for i := range tracks {
		t := &tracks[i]
		gi.flex[a] = gi.flex[a] || t.flexible()
		autoMin = autoMin || t.minK == style.TrackAuto
		fixedMax = fixedMax && t.maxK == style.TrackLength
		capSum += t.maxPx
	}
	s := resolveAxis(it.w, it.v, a, indefinite)
	c := &gi.contrib[a]
	if s.definite {
		n := float64(s.clamp(s.size))
		*c = gridContrib{minimum: n, minC: n, maxC: n}
		return
	}
	c.maxC, c.minC = float64(s.clamp(maxC)), float64(s.clamp(minC))
	c.minimum = float64(max(s.min, s.ax.margin+s.ax.bp))
	_, lo, _ := sizingOn(&it.v.Layout, a)
	if lo.Kind == style.SizingAuto && !clipsMain(it, a) && autoMin && (sp.size() == 1 || !gi.flex[a]) {
		auto := c.minC
		if fixedMax {
			auto = math.Min(auto, capSum)
		}
		c.minimum = math.Max(c.minimum, auto)
	}
}

// contributeRows measures the items' heights at their used widths in
// the sized columns and records their row contributions, each
// baseline-aligned item's shim within its row's baseline-sharing group
// added.
func (g *gridPass) contributeRows() {
	cols := &g.ax[Row]
	rows := g.ax[Column].tracks
	for i := range g.items {
		it := &g.items[i]
		if it.kind != itemFlow {
			continue
		}
		gi := &it.grid
		_, area := cols.areaOf(gi.area[Row])
		gi.width = g.itemWidth(it, area)
		req := measureOuter(it.w, Column, gi.width, indefiniteSize)
		gi.height = req.Nat
		g.contribute(it, Column, req.Nat, req.Min)
		gi.baseline, gi.hasBaseline = baselineIn(it)
		if !gi.hasBaseline {
			gi.baseline = req.Nat
		}
		switch gi.self[Column].Kind {
		case style.BoxBaseline:
			t := &rows[gi.area[Column].start]
			t.above = math.Max(t.above, float64(gi.baseline))
		case style.BoxLastBaseline:
			t := &rows[gi.area[Column].end-1]
			t.below = math.Max(t.below, float64(req.Nat-gi.baseline))
		}
	}
	for i := range g.items {
		it := &g.items[i]
		if it.kind != itemFlow {
			continue
		}
		gi := &it.grid
		switch gi.self[Column].Kind {
		case style.BoxBaseline:
			gi.shim = rows[gi.area[Column].start].above - float64(gi.baseline)
		case style.BoxLastBaseline:
			gi.shim = rows[gi.area[Column].end-1].below - float64(gi.height-gi.baseline)
		default:
			continue
		}
		c := &gi.contrib[Column]
		c.minimum, c.minC, c.maxC = c.minimum+gi.shim, c.minC+gi.shim, c.maxC+gi.shim
	}
}

// stretches reports whether an item fills its area along a: stretch
// alignment, an auto size, and no auto margin.
func stretches(it *cssItem, a Axis, s axisSizes) bool {
	ms, me := autoMargins(it, a)
	return it.grid.self[a].Kind == style.BoxStretch && !s.definite && !ms && !me
}

// itemWidth is an item's used outer width in a column area area wide: a
// definite width, the area for a stretched one, else fit-content —
// its max-content width within its min-content width and the area —
// all inside its min and max widths.
func (g *gridPass) itemWidth(it *cssItem, area int) int {
	ws := resolveAxis(it.w, it.v, Row, area)
	switch {
	case ws.definite:
		return ws.clamp(ws.size)
	case stretches(it, Row, ws):
		return ws.clamp(area)
	}
	c := &it.grid.contrib[Row]
	return ws.clamp(min(int(c.maxC), max(int(c.minC), area)))
}

// sizeTracks runs the track sizing algorithm along a (§11.4-11.8):
// base sizes and growth limits from the sizing functions, the
// intrinsic sizes the items' contributions call for — single-track
// items, then spanning items by span, then the items crossing flexible
// tracks — the maximized tracks, the flexible tracks, and the stretched
// auto tracks.
func (g *gridPass) sizeTracks(a Axis) {
	ax := &g.ax[a]
	for i := range ax.tracks {
		t := &ax.tracks[i]
		t.base, t.limit, t.growable = 0, inf, false
		if t.minK == style.TrackLength {
			t.base = t.minPx
		}
		if t.maxK == style.TrackLength {
			t.limit = math.Max(t.maxPx, t.base)
		}
	}
	g.fitSingles(a)
	widest := 1
	for i := range g.items {
		if it := &g.items[i]; it.kind == itemFlow && !it.grid.flex[a] {
			widest = max(widest, it.grid.area[a].size())
		}
	}
	for n := 2; n <= widest; n++ {
		g.fitSpanning(a, n, false)
	}
	g.fitSpanning(a, 0, true)
	for i := range ax.tracks {
		if t := &ax.tracks[i]; math.IsInf(t.limit, 1) {
			t.limit = t.base
		}
	}
	g.maximize(a)
	g.expandFlexible(a)
	g.stretchAuto(a)
}

// minimumFor is an item's contribution to an auto minimum: its minimum
// contribution, or under a min-content or max-content constraint its
// limited min-content or max-content contribution — capped by the
// spanned tracks' fixed maximums (a length or a fit-content() limit)
// when they all have one, and floored by the minimum contribution.
func (g *gridPass) minimumFor(it *cssItem, a Axis) float64 {
	ax := &g.ax[a]
	c := &it.grid.contrib[a]
	v := c.maxC
	switch ax.sizing {
	case sizeDefinite:
		return c.minimum
	case sizeMinContent:
		v = c.minC
	}
	sp := it.grid.area[a]
	sum := ax.gaps(sp.size())
	for i := sp.start; i < sp.end; i++ {
		t := &ax.tracks[i]
		if t.maxK != style.TrackLength && (t.maxK != style.TrackFitContent || math.IsInf(t.maxPx, 1)) {
			return v
		}
		sum += t.maxPx
	}
	return math.Max(math.Min(v, sum), c.minimum)
}

// fitSingles sizes the tracks to their non-spanning items (§11.5 step
// 2): a content-sized minimum takes the largest matching contribution,
// a content-sized maximum the largest min-content or max-content
// contribution (a fit-content() one clamped to its limit), every limit
// at least its base size. Flexible tracks wait for the items crossing
// them.
func (g *gridPass) fitSingles(a Axis) {
	tracks := g.ax[a].tracks
	for i := range tracks {
		tracks[i].planned = -1
	}
	for i := range g.items {
		it := &g.items[i]
		sp := it.grid.area[a]
		if it.kind != itemFlow || sp.size() != 1 || tracks[sp.start].flexible() {
			continue
		}
		t := &tracks[sp.start]
		c := &it.grid.contrib[a]
		switch t.minK {
		case style.TrackMinContent:
			t.base = math.Max(t.base, c.minC)
		case style.TrackMaxContent:
			t.base = math.Max(t.base, c.maxC)
		case style.TrackAuto:
			t.base = math.Max(t.base, g.minimumFor(it, a))
		}
		switch t.maxK {
		case style.TrackMinContent:
			t.planned = math.Max(t.planned, c.minC)
		case style.TrackMaxContent, style.TrackAuto, style.TrackFitContent:
			t.planned = math.Max(t.planned, c.maxC)
		}
	}
	for i := range tracks {
		t := &tracks[i]
		if t.planned >= 0 {
			t.limit = t.planned
			if t.maxK == style.TrackFitContent {
				t.limit = math.Min(t.limit, t.maxPx)
			}
		}
		if t.limit < t.base {
			t.limit = t.base
		}
		t.planned = 0
	}
}

// gridStep is one round of §11.5's spanning-item distribution: which
// contribution grows which tracks' base sizes or growth limits.
type gridStep uint8

const (
	// stepIntrinsicMin gives minimum contributions to intrinsic minimums.
	stepIntrinsicMin gridStep = iota
	// stepContentMin gives min-content contributions to min-content and
	// max-content minimums.
	stepContentMin
	// stepMaxContentMin gives max-content contributions to max-content
	// minimums, and to auto ones under a max-content constraint.
	stepMaxContentMin
	// stepIntrinsicMax gives min-content contributions to intrinsic
	// maximums.
	stepIntrinsicMax
	// stepMaxContentMax gives max-content contributions to max-content
	// maximums.
	stepMaxContentMax
)

// limits reports a step that grows growth limits.
func (s gridStep) limits() bool { return s >= stepIntrinsicMax }

// affects reports whether the step grows track t.
func (s gridStep) affects(t *gridTrack, sizing gridSizing) bool {
	switch s {
	case stepIntrinsicMin:
		return t.intrinsicMin()
	case stepContentMin:
		return t.minK == style.TrackMinContent || t.minK == style.TrackMaxContent
	case stepMaxContentMin:
		return t.minK == style.TrackMaxContent || t.minK == style.TrackAuto && sizing == sizeMaxContent
	case stepIntrinsicMax:
		return t.intrinsicMax()
	}
	return t.maxContentMax()
}

// contribution is the item's contribution a step distributes.
func (g *gridPass) contribution(it *cssItem, a Axis, s gridStep) float64 {
	c := &it.grid.contrib[a]
	switch s {
	case stepIntrinsicMin:
		return g.minimumFor(it, a)
	case stepContentMin, stepIntrinsicMax:
		return c.minC
	}
	return c.maxC
}

// affected is the size a step grows on track t: the base size, or the
// growth limit (the base size while that is infinite).
func (s gridStep) affected(t *gridTrack) float64 {
	if s.limits() && !math.IsInf(t.limit, 1) {
		return t.limit
	}
	return t.base
}

// fitSpanning sizes the tracks to the items spanning n tracks without
// crossing a flexible one, or with flex to the items crossing a
// flexible track (n unused), whose space goes to the flexible tracks
// alone by their fr ratios (§11.5 steps 3 and 4).
func (g *gridPass) fitSpanning(a Axis, n int, flex bool) {
	ax := &g.ax[a]
	for s := stepIntrinsicMin; s <= stepMaxContentMax; s++ {
		if flex && s.limits() {
			break
		}
		for i := range ax.tracks {
			ax.tracks[i].planned, ax.tracks[i].touched = 0, false
		}
		for i := range g.items {
			it := &g.items[i]
			if it.kind != itemFlow || it.grid.flex[a] != flex || !flex && it.grid.area[a].size() != n {
				continue
			}
			g.spread(it, a, s, flex)
		}
		for i := range ax.tracks {
			t := &ax.tracks[i]
			if !t.touched {
				continue
			}
			switch {
			case !s.limits():
				t.base += t.planned
			case math.IsInf(t.limit, 1):
				t.limit = t.base + t.planned
				t.growable = s == stepIntrinsicMax
			default:
				t.limit += t.planned
			}
		}
		if s == stepMaxContentMin || s == stepMaxContentMax {
			for i := range ax.tracks {
				t := &ax.tracks[i]
				t.limit = math.Max(t.limit, t.base)
				if s == stepMaxContentMax {
					t.growable = false
				}
			}
		}
	}
}

// spread distributes one item's contribution over its spanned tracks
// for step s (§11.5.1): what the contribution exceeds of the spanned
// sizes and gutters goes equally to the affected tracks up to their
// limits, then past them to the tracks the step prefers, or to the
// flexible tracks by their fr ratios; each track keeps the largest
// increase any item asks of it.
func (g *gridPass) spread(it *cssItem, a Axis, s gridStep, flex bool) {
	ax := &g.ax[a]
	sp := it.grid.area[a]
	tracks := ax.tracks[sp.start:sp.end]
	space := g.contribution(it, a, s) - ax.gaps(sp.size())
	affected, sumFr := 0, 0.0
	for i := range tracks {
		t := &tracks[i]
		space -= s.affected(t)
		t.incr, t.frozen = 0, !s.affects(t, ax.sizing) || flex && !t.flexible()
		if !t.frozen {
			affected++
			sumFr += t.fr
		}
	}
	if space <= 0 || affected == 0 {
		return
	}
	if flex {
		for i := range tracks {
			t := &tracks[i]
			if t.frozen {
				continue
			}
			share := space * (t.fr + (1-sumFr)/float64(affected))
			if sumFr >= 1 {
				share = space * t.fr / sumFr
			}
			t.planned, t.touched = math.Max(t.planned, share), true
		}
		return
	}
	space = fillTracks(tracks, space, func(t *gridTrack) float64 { return s.room(t, false) })
	if space > 0 {
		beyond := 0
		for i := range tracks {
			t := &tracks[i]
			if s.affects(t, ax.sizing) {
				t.frozen = !s.beyond(t)
				beyond += btoi(!t.frozen)
			}
		}
		if beyond == 0 {
			for i := range tracks {
				tracks[i].frozen = !s.affects(&tracks[i], ax.sizing)
			}
		}
		space = fillTracks(tracks, space, func(t *gridTrack) float64 { return s.room(t, true) })
		if space > 0 {
			for i := range tracks {
				tracks[i].frozen = !s.affects(&tracks[i], ax.sizing)
			}
			fillTracks(tracks, space, func(*gridTrack) float64 { return inf })
		}
	}
	for i := range tracks {
		if t := &tracks[i]; s.affects(t, ax.sizing) {
			t.planned, t.touched = math.Max(t.planned, t.incr), true
		}
	}
}

// beyond reports whether space past the limits goes to track t: a track
// with an intrinsic maximum for the minimum and min-content steps, a
// max-content maximum for the max-content minimums, any affected track
// for the growth limits.
func (s gridStep) beyond(t *gridTrack) bool {
	switch s {
	case stepIntrinsicMin, stepContentMin:
		return t.intrinsicMax()
	case stepMaxContentMin:
		return t.maxContentMax()
	}
	return true
}

// room is how far track t may grow in step s: up to its growth limit
// (capped by a fit-content() limit) for a base size, up to a finite
// growth limit not marked growable (else a fit-content() limit) for a
// growth limit — never below the size it already has. Past the limits a
// fit-content() track grows up to its limit, the rest freely.
func (s gridStep) room(t *gridTrack, past bool) float64 {
	fit := inf
	if t.maxK == style.TrackFitContent {
		fit = t.maxPx
	}
	limit := fit
	switch {
	case past && s.limits():
		limit = inf
	case past:
	case !s.limits():
		limit = math.Min(t.limit, fit)
	case !math.IsInf(t.limit, 1) && !t.growable:
		limit = t.limit
	}
	return math.Max(0, limit-s.affected(t))
}

// fillTracks grows the unfrozen tracks' increases equally by space, each
// up to its room, freezing those that reach it, and returns the space
// left once every track froze.
func fillTracks(tracks []gridTrack, space float64, room func(t *gridTrack) float64) float64 {
	const eps = 1e-9
	for space > eps {
		open, step := 0, inf
		for i := range tracks {
			if t := &tracks[i]; !t.frozen {
				open++
				step = math.Min(step, room(t)-t.incr)
			}
		}
		if open == 0 {
			return space
		}
		step = math.Min(math.Max(step, 0), space/float64(open))
		for i := range tracks {
			t := &tracks[i]
			if t.frozen {
				continue
			}
			t.incr += step
			space -= step
			if room(t)-t.incr <= eps {
				t.frozen = true
			}
		}
	}
	return 0
}

// maximize grows the tracks to their growth limits (§11.6): all the way
// under a max-content constraint, by the free space of a definite
// extent shared equally otherwise.
func (g *gridPass) maximize(a Axis) {
	ax := &g.ax[a]
	switch ax.sizing {
	case sizeMaxContent:
		for i := range ax.tracks {
			ax.tracks[i].base = ax.tracks[i].limit
		}
	case sizeDefinite:
		free := float64(ax.size) - ax.used()
		if free <= 0 {
			return
		}
		for i := range ax.tracks {
			t := &ax.tracks[i]
			t.incr, t.frozen = 0, t.collapsed
		}
		fillTracks(ax.tracks, free, func(t *gridTrack) float64 { return t.limit - t.base })
		for i := range ax.tracks {
			ax.tracks[i].base += ax.tracks[i].incr
		}
	}
}

// expandFlexible sizes the flexible tracks (§11.7): by the fr that fills
// a definite extent's free space, or under a max-content constraint by
// the largest fr any flexible track's base size or any item crossing
// flexible tracks calls for; under a min-content constraint, or with no
// free space, they keep their base sizes.
func (g *gridPass) expandFlexible(a Axis) {
	ax := &g.ax[a]
	if !slices.ContainsFunc(ax.tracks, func(t gridTrack) bool { return t.flexible() }) {
		return
	}
	fr := 0.0
	switch ax.sizing {
	case sizeDefinite:
		if float64(ax.size) > ax.used() {
			fr = g.frSize(a, gridSpan{0, len(ax.tracks)}, float64(ax.size))
		}
	case sizeMaxContent:
		for i := range ax.tracks {
			if t := &ax.tracks[i]; t.flexible() {
				fr = math.Max(fr, t.base/math.Max(1, t.fr))
			}
		}
		for i := range g.items {
			if it := &g.items[i]; it.kind == itemFlow && it.grid.flex[a] {
				fr = math.Max(fr, g.frSize(a, it.grid.area[a], it.grid.contrib[a].maxC))
			}
		}
	}
	for i := range ax.tracks {
		if t := &ax.tracks[i]; t.flexible() {
			t.base = math.Max(t.base, fr*t.fr)
		}
	}
}

// frSize finds the size of an fr over the tracks of sp filling space
// (§11.7.1): the space the inflexible tracks and the gutters leave over
// the flexible tracks' factors (at least one in sum), a flexible track
// whose base size exceeds its share counted as inflexible.
func (g *gridPass) frSize(a Axis, sp gridSpan, space float64) float64 {
	ax := &g.ax[a]
	tracks := ax.tracks[sp.start:sp.end]
	n := 0
	for i := range tracks {
		n += btoi(!tracks[i].collapsed)
	}
	left := space - ax.gaps(n)
	for i := range tracks {
		t := &tracks[i]
		t.frozen = !t.flexible()
		if t.frozen {
			left -= t.base
		}
	}
	for {
		sum := 0.0
		for i := range tracks {
			if t := &tracks[i]; !t.frozen {
				sum += t.fr
			}
		}
		hyp := left / math.Max(1, sum)
		again := false
		for i := range tracks {
			if t := &tracks[i]; !t.frozen && hyp*t.fr < t.base {
				t.frozen, again = true, true
				left -= t.base
			}
		}
		if !again {
			return hyp
		}
	}
}

// contentAlign is the grid's content distribution along a:
// justify-content for the columns, align-content for the rows, normal
// where undeclared.
func (g *gridPass) contentAlign(a Axis) style.BoxAlign {
	v := g.v
	if a == Row && v.Has(style.PropJustifyContent) {
		return v.Layout.JustifyContent
	}
	if a == Column && v.Has(style.PropAlignContent) {
		return v.Layout.AlignContent
	}
	return style.BoxAlign{Kind: style.BoxNormal}
}

// stretchAuto shares a definite extent's leftover space among the tracks
// with an auto maximum (§11.8) when the content distribution is normal
// or stretch.
func (g *gridPass) stretchAuto(a Axis) {
	ax := &g.ax[a]
	if k := g.contentAlign(a).Kind; ax.sizing != sizeDefinite || k != style.BoxNormal && k != style.BoxStretch {
		return
	}
	free := float64(ax.size) - ax.used()
	n := 0
	for i := range ax.tracks {
		n += btoi(ax.tracks[i].maxK == style.TrackAuto && !ax.tracks[i].collapsed)
	}
	if free <= 0 || n == 0 {
		return
	}
	for i := range ax.tracks {
		if t := &ax.tracks[i]; t.maxK == style.TrackAuto && !t.collapsed {
			t.base += free / float64(n)
		}
	}
}

// position places the tracks along a by the content distribution over a
// definite extent's free space (§10.5), rounding at the edges, and
// returns the extent the items lay out across: the definite one, else
// the tracks'. Collapsed tracks take no space and no gutters.
func (g *gridPass) position(a Axis) int {
	ax := &g.ax[a]
	extent := int(math.Round(ax.used()))
	free := 0.0
	if ax.sizing == sizeDefinite {
		extent = ax.size
		free = float64(ax.size) - ax.used()
	}
	offset, between := distribute(g.contentAlign(a), free, ax.shown(), true, a == Column || !g.rtl)
	pos, first := offset, true
	for i := range ax.tracks {
		t := &ax.tracks[i]
		if !t.collapsed {
			if !first {
				pos += float64(ax.gap) + between
			}
			first = false
		}
		t.start = int(math.Round(pos))
		if !t.collapsed {
			pos += t.base
		}
		t.end = int(math.Round(pos))
	}
	return extent
}

// placeItems aligns every in-flow item in its grid area (§10.3-10.4):
// auto margins take positive free space first, else the self
// alignment places the item's used size — stretched, definite, or its
// content size — with baseline-sharing items offset by their shims; an
// RTL grid mirrors the columns across width.
func (g *gridPass) placeItems(width int) {
	cols, rows := &g.ax[Row], &g.ax[Column]
	for i := range g.items {
		it := &g.items[i]
		if it.kind != itemFlow {
			continue
		}
		gi := &it.grid
		cs, cw := cols.areaOf(gi.area[Row])
		rs, rh := rows.areaOf(gi.area[Column])
		hs := resolveAxis(it.w, it.v, Column, rh)
		h := gi.height
		switch {
		case hs.definite:
			h = hs.clamp(hs.size)
		case stretches(it, Column, hs):
			h = hs.clamp(rh)
		}
		x := cs + g.alignIn(it, Row, cw, gi.width)
		if g.rtl {
			x = width - x - gi.width
		}
		it.rect = render.Rect{X: x, Y: rs + g.alignIn(it, Column, rh, h), W: gi.width, H: h}
	}
}

// alignIn is an item's offset along a in an area area long for a used
// size of size.
func (g *gridPass) alignIn(it *cssItem, a Axis, area, size int) int {
	free := area - size
	if s, e := autoMargins(it, a); (s || e) && free > 0 {
		switch {
		case s && e:
			return free / 2
		case s:
			return free
		}
		return 0
	}
	al := it.grid.self[a]
	switch al.Kind {
	case style.BoxBaseline:
		return int(math.Round(it.grid.shim))
	case style.BoxLastBaseline:
		return free - int(math.Round(it.grid.shim))
	}
	off, _ := distribute(al, float64(free), 1, true, a == Column || !g.rtl)
	return int(math.Round(off))
}

// resolveOutOfFlow resolves the containing block of each absolutely
// positioned item of a positioned grid container from its grid lines
// (§9.1 of Grid 2, Absolute Positioning): lines resolve as for in-flow
// items but create no implicit tracks, and an auto line, a lone span,
// or a line outside the grid stands for the padding edge. Without a
// positioned grid the item keeps its own containing block; either way
// its static position is the content box's start corner.
func (g *gridPass) resolveOutOfFlow(width, height int) {
	if !positioned(g.v) {
		return
	}
	pad := g.b.box(g.v).padding
	padStart, padEnd := pad.Left, pad.Right
	if g.rtl {
		padStart, padEnd = padEnd, padStart
	}
	for i := range g.items {
		it := &g.items[i]
		if it.kind != itemOut || it.v.Layout.Position != style.PositionAbsolute {
			continue
		}
		x0, x1 := g.absEdges(it, Row, -padStart, width+padEnd)
		y0, y1 := g.absEdges(it, Column, -pad.Top, height+pad.Bottom)
		if g.rtl {
			x0, x1 = width-x1, width-x0
		}
		it.grid.cb, it.grid.hasCB = render.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, true
	}
}

// absEdges resolves an absolutely positioned item's edges along a, lo
// and hi the padding edges an auto line stands for.
func (g *gridPass) absEdges(it *cssItem, a Axis, lo, hi int) (start, end int) {
	ax := &g.ax[a]
	s, e := linesOf(it, a)
	if s.Kind == style.GridLineSpan && e.Kind == style.GridLineSpan {
		e = style.GridLine{}
	}
	var from, to int
	hasFrom, hasTo := definiteLine(s), definiteLine(e)
	switch {
	case hasFrom && hasTo:
		from, to = g.lineOf(a, s, false), g.lineOf(a, e, true)
		if from > to {
			from, to = to, from
		}
		if from == to {
			to = from + 1
		}
	case hasFrom:
		from = g.lineOf(a, s, false)
		to, hasTo = g.spanFrom(a, from, e, 1), e.Kind == style.GridLineSpan
	case hasTo:
		to = g.lineOf(a, e, true)
		from, hasFrom = g.spanFrom(a, to, s, -1), s.Kind == style.GridLineSpan
	}
	n := len(ax.tracks)
	start, end = lo, hi
	if k := from + ax.offset; hasFrom && k >= 0 && k <= n && n > 0 {
		start = ax.tracks[min(k, n-1)].start
		if k == n {
			start = ax.tracks[n-1].end
		}
	}
	if k := to + ax.offset; hasTo && k >= 0 && k <= n && n > 0 {
		end = ax.tracks[max(k-1, 0)].end
		if k == 0 {
			end = ax.tracks[0].start
		}
	}
	return start, end
}

// firstBaseline records the grid's first baseline (§10.8 of Grid 2): in
// the first row holding items, a first-baseline group's shared
// baseline, else the baseline of the first item there in grid order
// that has one.
func (g *gridPass) firstBaseline() {
	first := -1
	for i := range g.items {
		if it := &g.items[i]; it.kind == itemFlow && (first < 0 || it.grid.area[Column].start < first) {
			first = it.grid.area[Column].start
		}
	}
	var pick *cssItem
	for i := range g.items {
		it := &g.items[i]
		gi := &it.grid
		if it.kind != itemFlow || gi.area[Column].start != first || !gi.hasBaseline {
			continue
		}
		if gi.self[Column].Kind == style.BoxBaseline {
			pick = it
			break
		}
		if pick == nil || gi.area[Row].start < pick.grid.area[Row].start {
			pick = it
		}
	}
	if pick != nil {
		g.base, g.hasBase = pick.rect.Y+pick.grid.baseline, true
	}
}
