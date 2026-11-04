// Package repaint keeps one surface's repaint bookkeeping between
// frames: the logical rects its widget tree owes, and, at paint time,
// the device rects a buffer from the surface's rotating pool must
// repaint, the buffer's own staleness included. Windows and popups
// draw through it alike.
package repaint

import (
	"image"

	"github.com/stubbedev/lys-lib/gpu"
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
	// blitted is the logical views the last frame moved, owed again if the
	// GPU refuses that frame.
	blitted []render.Rect
	// carries and carryRects are a GPU frame's copies, built by Blit.
	carries    []gpu.Carry
	carryRects []image.Rectangle
	cutScratch []image.Rectangle
}

type blit struct {
	view   render.Rect
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
// front, on the wire without being painted, or, for a front of another
// size or kind or a move landing between device pixels, owed whole so the
// frame paints it. Call it ahead of DropCovered and CopyForward, and Blit
// after them.
func (r *Region) PlanBlits(b, front *buffer.Buffer, num int) {
	r.blits, r.moved, r.blitted = r.blits[:0], r.moved[:0], r.blitted[:0]
	copies := front != nil && front.Width == b.Width && front.Height == b.Height &&
		(b.Data != nil && front.Data != nil || b.Target != nil && front.Target != nil)
	for _, m := range r.moves {
		if !copies || m.DX*num%scale.Denom != 0 || m.DY*num%scale.Denom != 0 {
			r.pending = append(r.pending, m.View)
			continue
		}
		r.blits = append(r.blits, blit{m.View, render.MapRect(m.View, num, scale.Denom), m.DX * num / scale.Denom, m.DY * num / scale.Denom})
	}
	r.moves = r.moves[:0]
}

// Blit makes the moves PlanBlits kept. Shared memory copies here, and nil
// comes back. A GPU target copies inside the submission that draws it: the
// carries come back for the frame, b's staleness from carryFrom first, when
// not nil (buffer.Pool.CopyForward), then each move from front. The carries
// are reused by the next Blit.
func (r *Region) Blit(b, front, carryFrom *buffer.Buffer) []gpu.Carry {
	for _, c := range r.blits {
		r.moved = append(r.moved, c.area)
		r.blitted = append(r.blitted, c.view)
	}
	if b.Target == nil {
		for _, c := range r.blits {
			buffer.MoveRect(b, front, c.area, c.dx, c.dy)
		}
		return nil
	}
	r.carryRects, r.carries = r.carryRects[:0], r.carries[:0]
	if carryFrom != nil {
		for _, s := range b.Stale.Rects() {
			r.carryRects = append(r.carryRects, deviceRect(s))
		}
		for _, c := range r.blits {
			r.carryRects = cutOut(r.carryRects, deviceRect(buffer.MovedArea(c.area, c.dx, c.dy)), &r.cutScratch)
		}
	}
	stale := len(r.carryRects)
	for _, c := range r.blits {
		r.carryRects = append(r.carryRects, deviceRect(buffer.MovedArea(c.area, c.dx, c.dy)))
	}
	if carryFrom != nil {
		r.carries = append(r.carries, gpu.Carry{From: carryFrom.Target, Rects: r.carryRects[:stale]})
	}
	for i, c := range r.blits {
		at := stale + i
		r.carries = append(r.carries, gpu.Carry{From: front.Target, Rects: r.carryRects[at : at+1], Offset: image.Pt(-c.dx, -c.dy)})
	}
	return r.carries
}

func deviceRect(r render.Rect) image.Rectangle { return image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H) }

// cutOut is rs without cut: a stale rect a move then overwrites is not worth
// copying.
func cutOut(rs []image.Rectangle, cut image.Rectangle, scratch *[]image.Rectangle) []image.Rectangle {
	out := (*scratch)[:0]
	for _, r := range rs {
		in := r.Intersect(cut)
		if in.Empty() {
			out = append(out, r)
			continue
		}
		for _, piece := range [4]image.Rectangle{
			{Min: r.Min, Max: image.Pt(r.Max.X, in.Min.Y)},
			{Min: image.Pt(r.Min.X, in.Max.Y), Max: r.Max},
			{Min: image.Pt(r.Min.X, in.Min.Y), Max: image.Pt(in.Min.X, in.Max.Y)},
			{Min: image.Pt(in.Max.X, in.Min.Y), Max: image.Pt(r.Max.X, in.Max.Y)},
		} {
			if !piece.Empty() {
				out = append(out, piece)
			}
		}
	}
	*scratch = rs[:0]
	return out
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

// Refuse owes again what the last Plan took, the views it moved whole: the
// GPU refused the frame, having done nothing, or drew it without everything
// in it.
func (r *Region) Refuse() {
	r.pending = append(r.pending, r.planned...)
	r.pending = append(r.pending, r.blitted...)
	r.blitted = r.blitted[:0]
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
