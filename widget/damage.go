// Damage tracking turns per-widget invalidation flags into the repaint
// rects one frame needs. Widgets mark themselves through Invalidate,
// InvalidateRect, and InvalidateLayout; CollectDamage drains the flags
// into a rect list the app clips painting to and sends as
// wl_surface.damage_buffer.
package widget

import (
	"sync/atomic"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// maxDamageRects caps the rect list handed to the frame. Past the cap
// the remaining rects collapse into the running bounding box: more wire
// rects than this stops paying for itself.
const maxDamageRects = 32

// damageNoder is node's drain hook; every widget that embeds node has it.
type damageNoder interface {
	takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool)
}

// childser exposes a widget's children for tree walks (the same
// interface focus traversal uses).
type childser interface {
	Children() []Widget
}

// childBuf is the damage walk's child source: the same snapshot
// Children() returns, appended into a buffer the walk reuses across
// the whole traversal instead of a fresh slice per container per
// frame. Implementations must match Children() in order and membership;
// TestChildWalkMatchesChildren pins that over every container.
type childBuf interface {
	appendChildren(buf []Widget) []Widget
}

// walkStack is the per-depth snapshot buffers the damage collector
// reuses across frames. Tree walks are loop-goroutine work (the same
// discipline as every widget mutation), so one stack serves the
// process; slots hold their capacity for the next frame, and a walk
// never re-enters a depth it is still iterating, so a level's snapshot
// lives until its subtree is done. The stack reaches the deepest tree
// the process has walked and stays there.
var walkStack [][]Widget

// backdropReach collects, per walk, the area each backdrop-filtered box
// reads: its bounds grown by the filter's extent. Reused across walks.
var backdropReach []render.Rect

// damageGen counts every mark that can turn a tree dirty: a widget
// invalidation, a style or theme generation move, a stale cascade.
// Draw owners compare it against the generation they last drained at
// and skip the walk when nothing anywhere was marked since, so pump
// work that touched no tree costs no CollectDamage pass. The +1 in
// DamageGeneration keeps zero meaning "never walked".
var damageGen atomic.Uint64

// damageWalks counts CollectDamage calls; tests use it to pin that a
// clean tree's walk is skipped.
var damageWalks atomic.Uint64

// DamageGeneration returns the current damage generation; it is never
// zero.
func DamageGeneration() uint64 { return damageGen.Load() + 1 }

// DamageWalks counts the damage walks the process has run; tests use
// it to pin the idle skip.
func DamageWalks() uint64 { return damageWalks.Load() }

// markDamage bumps the generation every drain-source flip owes, so no
// window can skip past a dirty tree.
func markDamage() { damageGen.Add(1) }

// CollectDamage drains every pending invalidation in the tree and
// returns the repaint rects in root coordinates plus whether anything
// is owed. Rects may overlap; the caller can clip painting to their
// bounding box and damage each rect on the wire. Draining clears the
// flags, so an empty result means the tree paints identically to the
// last frame.
func CollectDamage(root Widget) (rects []render.Rect, any bool) {
	return CollectDamageInto(nil, root)
}

// CollectDamageInto is CollectDamage reusing buf's storage for the
// rects, so a frame loop that keeps its buffer drains without
// allocating.
func CollectDamageInto(buf []render.Rect, root Widget) (rects []render.Rect, any bool) {
	rects = buf[:0]
	if root == nil {
		return rects, false
	}
	damageWalks.Add(1)
	backdropReach = backdropReach[:0]
	bbox := collectInto(root, &rects, 0)
	bbox = bbox.Union(widenForBackdrops(&rects))
	if len(rects) > maxDamageRects {
		rects = rects[:0]
		if !bbox.Empty() {
			rects = append(rects, bbox)
		}
	}
	return rects, len(rects) > 0
}

// collectInto walks w, appending drained rects and returning the
// bounding box of everything it appended (empty when nothing). The
// children iterate from the walk's own snapshot buffer for depth, so a
// container that mutates mid-drain changes the next frame, not this
// walk.
func collectInto(w Widget, rects *[]render.Rect, depth int) render.Rect {
	// A style-marked widget recomputes before it drains: pre-order, so
	// a parent's inherited change marks and damages its subtree within
	// this same walk, and a class toggle lands in one frame.
	if n := nodeOf(w); n != nil && n.styleDirty {
		n.restyle(w)
	}
	bbox := render.Rect{}
	if n := nodeOf(w); n != nil {
		if v := n.style(w); v.Has(style.PropBackdropFilter) && !v.Backdrop.Empty() {
			if bs, ok := w.(Boundser); ok {
				backdropReach = append(backdropReach, v.Backdrop.Extent().Grow(bs.Bounds()))
			}
		}
	}
	if dn, ok := w.(damageNoder); ok {
		var why string
		if debug.Enabled {
			why = damageReasons(nodeOf(w))
		}
		bounds, extras, dirty := dn.takeDamage()
		if debug.Enabled && dirty && !bounds.Empty() {
			noteDamage(w, bounds, why)
		}
		if dirty {
			if !bounds.Empty() {
				*rects = append(*rects, bounds)
				bbox = bbox.Union(bounds)
			}
			for _, r := range extras {
				if r.Empty() {
					continue
				}
				*rects = append(*rects, r)
				bbox = bbox.Union(r)
			}
		}
	}
	kids := snapshotKids(w, depth)
	for _, k := range kids {
		if k == nil {
			continue
		}
		bbox = bbox.Union(collectInto(k, rects, depth+1))
	}
	return bbox
}

// widenForBackdrops adds every backdrop reach the damage touches: a
// backdrop filter reads the backdrop around its box, and outside the
// damage the canvas still holds last frame's filtered pixels, so a box
// repaints whole (and the reach around it) whenever any of it does.
// Added reaches can touch further boxes, so it runs to a fixed point.
// It returns the bounding box of what it added.
func widenForBackdrops(rects *[]render.Rect) render.Rect {
	added := render.Rect{}
	for grew := true; grew; {
		grew = false
		for i := 0; i < len(backdropReach); {
			reach := backdropReach[i]
			if !touches(*rects, reach) {
				i++
				continue
			}
			*rects = append(*rects, reach)
			added = added.Union(reach)
			backdropReach[i] = backdropReach[len(backdropReach)-1]
			backdropReach = backdropReach[:len(backdropReach)-1]
			grew = true
		}
	}
	return added
}

// touches reports whether any rect overlaps r.
func touches(rects []render.Rect, r render.Rect) bool {
	for _, d := range rects {
		if !d.Intersect(r).Empty() {
			return true
		}
	}
	return false
}

// snapshotKids returns w's children in walkStack's slot for depth: a
// Children-shaped snapshot, buffered per depth and reused across
// frames. Widgets without childBuf fall back to the Children copy.
func snapshotKids(w Widget, depth int) []Widget {
	var buf []Widget
	if cb, ok := w.(childBuf); ok {
		for len(walkStack) <= depth {
			walkStack = append(walkStack, nil)
		}
		walkStack[depth] = walkStack[depth][:0]
		buf = cb.appendChildren(walkStack[depth])
		walkStack[depth] = buf
		return buf
	}
	if cs, ok := w.(childser); ok {
		return cs.Children()
	}
	return nil
}

// DamageArea returns the total pixel area the rects cover, counting
// overlaps once per rect. Tests use it to pin how much of a frame a
// damage-restricted repaint touches.
func DamageArea(rects []render.Rect) int {
	area := 0
	for _, r := range rects {
		if !r.Empty() {
			area += r.W * r.H
		}
	}
	return area
}
