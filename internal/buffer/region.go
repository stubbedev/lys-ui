package buffer

import "github.com/stubbedev/lys-lib/render"

// maxRegionRects bounds a Region: how many separate rects a frame walks
// the tree for, and a buffer copies forward, before their boxes merge.
const maxRegionRects = 4

// Region is a bounded set of rects covering what was added, possibly
// more: disjoint rects stay apart unless the box around them is hardly
// bigger, so two thin bars at a window's far edges do not cost the
// window between them. The zero value is empty.
type Region struct {
	rects [maxRegionRects]render.Rect
	n     int
}

// RegionOf is the region covering r.
func RegionOf(r render.Rect) Region {
	var g Region
	g.Add(r)
	return g
}

// Rects is the region's rects, valid until the next change.
func (g *Region) Rects() []render.Rect { return g.rects[:g.n] }

// Empty reports whether the region covers nothing.
func (g *Region) Empty() bool { return g.n == 0 }

// Clear empties the region.
func (g *Region) Clear() { g.n = 0 }

// Area is the pixels of the region's rects.
func (g *Region) Area() int {
	total := 0
	for _, r := range g.Rects() {
		total += rectArea(r)
	}
	return total
}

// Add covers r too, merging it with any rect worth merging, and at the
// bound with whichever grows least.
func (g *Region) Add(r render.Rect) {
	if r.Empty() {
		return
	}
	for i := 0; i < g.n; {
		if worthMerging(g.rects[i], r) {
			r = r.Union(g.rects[i])
			g.Remove(i)
			i = 0
			continue
		}
		i++
	}
	if g.n == maxRegionRects {
		at := 0
		for i := 1; i < g.n; i++ {
			if rectArea(g.rects[i].Union(r)) < rectArea(g.rects[at].Union(r)) {
				at = i
			}
		}
		r = r.Union(g.rects[at])
		g.Remove(at)
	}
	g.rects[g.n] = r
	g.n++
}

// AddRegion covers o too.
func (g *Region) AddRegion(o *Region) {
	for _, r := range o.Rects() {
		g.Add(r)
	}
}

// Remove drops the i-th of Rects, moving the last into its place.
func (g *Region) Remove(i int) {
	g.n--
	g.rects[i] = g.rects[g.n]
}

func rectArea(r render.Rect) int { return r.W * r.H }

// worthMerging reports whether painting two rects' box beats painting
// them apart: they touch, or the box is little more than they are. A
// walk of the tree costs something too, so a box of up to twice their
// pixels wins.
func worthMerging(a, b render.Rect) bool {
	return !a.Intersect(b).Empty() || rectArea(a.Union(b)) <= 2*(rectArea(a)+rectArea(b))
}
