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
// region bounds rects, the clip the frame paints in. rects is reused
// by the next Plan.
func (r *Region) Plan(b *buffer.Buffer, num int) (rects []render.Rect, changed, region render.Rect) {
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
	return rects, changed, render.UnionAll(rects)
}
