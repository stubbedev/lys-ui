// Package repaint keeps one surface's repaint bookkeeping between
// frames: the logical rects its widget tree owes, and, at paint time,
// the device rects a buffer from the surface's rotating pool must
// repaint, the buffer's own staleness included. Windows and popups
// draw through it alike.
package repaint

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/buffer"
	"github.com/stubbedev/lys-ui/internal/scale"
	"github.com/stubbedev/lys-ui/widget"
)

// Region is the damage a surface owes its next frame. The zero value
// owes nothing and has drained no tree yet.
type Region struct {
	// pending accumulates drained invalidations, in logical pixels,
	// until a frame paints them. It survives a frame the pool defers,
	// since the tree's flags are already drained.
	pending []render.Rect
	// seen is the widget damage generation the tree was last drained
	// at: a tree nothing invalidated since is not walked.
	seen uint64
	// dev is the scratch the device rects of a frame are built in.
	dev []render.Rect
	// regions is the scratch the frame's paint regions are built in.
	regions []render.Rect
	// drained is the scratch a tree's damage drains into.
	drained []render.Rect
}

// Collect drains root's invalidations into the region, clipped to
// bounds, the surface's logical rect: a widget may owe pixels outside
// it (an overflowing scroll child), and those must not keep a frame
// alive. It reports whether root owed anything, inside bounds or not.
func (r *Region) Collect(root widget.Widget, bounds render.Rect) bool {
	gen := widget.DamageGeneration()
	if gen == r.seen {
		return false
	}
	r.seen = gen
	var damaged bool
	r.drained, damaged = widget.CollectDamageInto(r.drained, root)
	r.Add(bounds, r.drained...)
	return damaged
}

// Add owes rects, clipped to bounds, besides what the tree drained: a
// focus ring that moved, or the whole surface.
func (r *Region) Add(bounds render.Rect, rects ...render.Rect) {
	for _, d := range rects {
		if d = d.Intersect(bounds); !d.Empty() {
			r.pending = append(r.pending, d)
		}
	}
}

// Full owes the whole surface, bounds: a rescale, a renderer change,
// an overlay toggled over everything.
func (r *Region) Full(bounds render.Rect) { r.Add(bounds, bounds) }

// Empty reports whether nothing is owed: the frame can skip its
// buffer entirely.
func (r *Region) Empty() bool { return len(r.pending) == 0 }

// Plan maps the owed rects onto b at device scale num/scale.Denom and
// clears them. rects is what the frame repaints and damages on the
// wire: the owed rects plus whatever b is stale in. changed is only
// what differs on screen after the commit, the owed rects without b's
// staleness: a fresh buffer's first paint covers the whole surface
// but changes nothing the screen did not already show, so the pool
// must not count it against the other buffers (buffer.Pool.Presented).
// regions covers rects, the clips the frame paints in. Both slices are
// reused by the next Plan.
func (r *Region) Plan(b *buffer.Buffer, num int) (rects []render.Rect, changed render.Rect, regions []render.Rect) {
	bounds := render.Rect{W: b.Width, H: b.Height}
	rects = r.dev[:0]
	for _, d := range r.pending {
		if d = render.MapRect(d, num, scale.Denom).Intersect(bounds); !d.Empty() {
			rects = append(rects, d)
		}
	}
	changed = render.UnionAll(rects)
	if stale := b.Stale.Intersect(bounds); !stale.Empty() {
		rects = append(rects, stale)
	}
	r.pending = r.pending[:0]
	r.dev = rects
	r.regions = coalesce(r.regions[:0], rects)
	return rects, changed, r.regions
}

// Covers reports whether one owed rect, at device scale num/scale.Denom,
// holds all of stale: the frame repaints it anyway, so copying it forward
// first is work thrown away.
func (r *Region) Covers(stale render.Rect, num int) bool {
	if stale.Empty() {
		return false
	}
	for _, d := range r.pending {
		if d = render.MapRect(d, num, scale.Denom); d.Intersect(stale) == stale {
			return true
		}
	}
	return false
}

// maxRegions bounds how many times a frame walks the tree.
const maxRegions = 4

// area is a rect's pixels.
func area(r render.Rect) int { return r.W * r.H }

// Area is the pixels of regions.
func Area(regions []render.Rect) int {
	total := 0
	for _, r := range regions {
		total += area(r)
	}
	return total
}

// worthMerging reports whether painting two rects' box beats painting them
// apart: they touch, or the box is little more than they are. A walk of the
// tree costs something too, so a box of up to twice their pixels wins.
func worthMerging(a, b render.Rect) bool {
	return !a.Intersect(b).Empty() || area(a.Union(b)) <= 2*(area(a)+area(b))
}

// coalesce appends to dst the regions covering rects: disjoint damage stays
// apart unless its box is hardly bigger, and at most maxRegions come out.
func coalesce(dst, rects []render.Rect) []render.Rect {
	for _, r := range rects {
		for i := 0; i < len(dst); {
			if worthMerging(dst[i], r) {
				r = r.Union(dst[i])
				dst[i] = dst[len(dst)-1]
				dst = dst[:len(dst)-1]
				i = 0
				continue
			}
			i++
		}
		if len(dst) == maxRegions {
			at := 0
			for i := 1; i < len(dst); i++ {
				if area(dst[i].Union(r)) < area(dst[at].Union(r)) {
					at = i
				}
			}
			r = r.Union(dst[at])
			dst[at] = dst[len(dst)-1]
			dst = dst[:len(dst)-1]
		}
		dst = append(dst, r)
	}
	return dst
}
