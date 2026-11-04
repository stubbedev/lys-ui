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
	// planned is what the last Plan took off pending, kept for a frame
	// the GPU refused.
	planned []render.Rect
	// regions is where the frame's paint regions are built, and changed
	// what they change on screen.
	regions, changed buffer.Region
	// drained is the scratch a tree's damage drains into.
	drained []render.Rect
	moves   []widget.ScrollMove
	blits   []blit
	moved   []render.Rect
}

type blit struct {
	area   render.Rect
	dx, dy int
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
	earlier := len(r.pending) > 0 || len(r.moves) > 0
	if earlier {
		r.demote(bounds)
	}
	var damaged bool
	r.drained, r.moves, damaged = widget.CollectFrameDamageInto(r.drained, r.moves, root)
	if earlier {
		r.demote(bounds)
	}
	r.Add(bounds, r.drained...)
	return damaged
}

func (r *Region) demote(bounds render.Rect) {
	for _, m := range r.moves {
		r.Add(bounds, m.View)
	}
	r.moves = r.moves[:0]
}

// PlanBlits decides how b takes the moves the region holds: copied from
// front, on the wire without being painted, or, for a GPU target or a
// move landing between device pixels, owed whole so the frame paints it.
// Call it ahead of DropCovered and CopyForward, and BlitNow after them.
func (r *Region) PlanBlits(b, front *buffer.Buffer, num int) {
	r.blits, r.moved = r.blits[:0], r.moved[:0]
	copies := b.Data != nil && front != nil && front.Data != nil && front.Width == b.Width && front.Height == b.Height
	for _, m := range r.moves {
		if !copies || m.DX*num%scale.Denom != 0 || m.DY*num%scale.Denom != 0 {
			r.pending = append(r.pending, m.View)
			continue
		}
		r.blits = append(r.blits, blit{render.MapRect(m.View, num, scale.Denom), m.DX * num / scale.Denom, m.DY * num / scale.Denom})
	}
	r.moves = r.moves[:0]
}

// BlitNow makes the copies PlanBlits kept.
func (r *Region) BlitNow(b, front *buffer.Buffer) {
	for _, c := range r.blits {
		buffer.MoveRect(b, front, c.area, c.dx, c.dy)
		r.moved = append(r.moved, c.area)
	}
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
func (r *Region) Empty() bool { return len(r.pending) == 0 && len(r.moves) == 0 }

// Plan maps the owed rects onto b at device scale num/scale.Denom and
// takes them off the region. rects is what the frame repaints and damages on the
// wire: the owed rects plus whatever b is stale in. changed is only
// what differs on screen after the commit, the owed rects without b's
// staleness: a fresh buffer's first paint covers the whole surface
// but changes nothing the screen did not already show, so the pool
// must not count it against the other buffers (buffer.Pool.Presented).
// regions covers rects, the clips the frame paints in. All three are
// reused by the next Plan. carrying says the frame copies b's staleness
// forward (buffer.Pool.CopyForward), so it paints none of it.
func (r *Region) Plan(b *buffer.Buffer, num int, carrying bool) (rects []render.Rect, changed *buffer.Region, regions []render.Rect) {
	bounds := render.Rect{W: b.Width, H: b.Height}
	rects = r.dev[:0]
	r.changed.Clear()
	r.regions.Clear()
	for _, d := range r.pending {
		if d = render.MapRect(d, num, scale.Denom).Intersect(bounds); !d.Empty() {
			rects = append(rects, d)
			r.changed.Add(d)
			r.regions.Add(d)
		}
	}
	for _, d := range r.moved {
		rects = append(rects, d)
		r.changed.Add(d)
	}
	r.moved = r.moved[:0]
	if !carrying {
		for _, stale := range b.Stale.Rects() {
			if stale = stale.Intersect(bounds); !stale.Empty() {
				rects = append(rects, stale)
				r.regions.Add(stale)
			}
		}
	}
	r.pending, r.planned = r.planned[:0], r.pending
	r.dev = rects
	return rects, &r.changed, r.regions.Rects()
}

// Refuse owes again what the last Plan took: the GPU refused the frame,
// having done nothing, or drew it without everything in it.
func (r *Region) Refuse() {
	r.pending = append(r.pending, r.planned...)
	r.planned = r.planned[:0]
}

// DropCovered takes off b's staleness every rect one owed rect, at
// device scale num/scale.Denom, holds whole: the frame repaints those
// anyway, so copying them forward first is work thrown away.
func (r *Region) DropCovered(b *buffer.Buffer, num int) {
	for i := range r.pending {
		owed := render.MapRect(r.pending[i], num, scale.Denom)
		for at := 0; at < len(b.Stale.Rects()); {
			if stale := b.Stale.Rects()[at]; owed.Intersect(stale) == stale {
				b.Stale.Remove(at)
				continue
			}
			at++
		}
	}
}

// Area is the pixels of regions.
func Area(regions []render.Rect) int {
	total := 0
	for _, r := range regions {
		total += r.W * r.H
	}
	return total
}
