package atspi

import (
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// The calls that reach widgets: AT-driven focus, the caret and
// selection setters, and the per-character geometry. Setters queue on
// the loop and answer at once from the snapshot (the next sample
// reports the outcome, as for DoAction); the geometry is shaping-time
// state no snapshot carries, so its probes run on the loop and wait.

// loopTimeout bounds a probe waiting on the loop: a stalled loop
// answers the zero value rather than hanging the AT.
const loopTimeout = 500 * time.Millisecond

// onLoop runs fn on the loop goroutine and waits for it, reporting
// whether it ran in time. fn's results are read only after a true.
func (b *Bridge) onLoop(fn func()) bool {
	done := make(chan struct{})
	b.scene.Invoke(func() {
		fn()
		close(done)
	})
	select {
	case <-done:
		return true
	case <-time.After(loopTimeout):
		return false
	case <-b.stop:
		return false
	}
}

// GrabFocus moves keyboard focus to a focusable widget through its
// window's router, the same path a click takes.
func (o *componentIface) GrabFocus() bool {
	n := o.b.node(o.id)
	if n == nil || !n.st.Focusable {
		return false
	}
	w := n.w
	o.b.scene.Invoke(func() { o.b.scene.SetFocus(w) })
	return true
}

// selector is the node's widget as a TextSelector, nil when it has
// none (a label's text is read-only).
func (o *textIface) selector() (widget.TextSelector, *anode) {
	n := o.b.node(o.id)
	if n == nil || !n.st.Enabled {
		return nil, n
	}
	s, _ := n.w.(widget.TextSelector)
	return s, n
}

// SetCaretOffset moves the caret (Text.SetCaretOffset).
func (o *textIface) SetCaretOffset(offset int32) bool {
	s, _ := o.selector()
	if s == nil {
		return false
	}
	o.b.scene.Invoke(func() { s.SetCaret(int(offset)) })
	return true
}

// SetSelection replaces selection num (lys-ui widgets hold one).
func (o *textIface) SetSelection(num, start, end int32) bool {
	s, _ := o.selector()
	if s == nil || num != 0 {
		return false
	}
	o.b.scene.Invoke(func() { s.Select(int(start), int(end)) })
	return true
}

// AddSelection selects start..end when nothing is selected; a second
// selection is refused (one per widget).
func (o *textIface) AddSelection(start, end int32) bool {
	s, n := o.selector()
	if s == nil || n.st.HasSelection {
		return false
	}
	o.b.scene.Invoke(func() { s.Select(int(start), int(end)) })
	return true
}

// RemoveSelection collapses the selection onto the caret.
func (o *textIface) RemoveSelection(num int32) bool {
	s, n := o.selector()
	if s == nil || num != 0 || !n.st.HasSelection {
		return false
	}
	caret := n.st.Caret
	o.b.scene.Invoke(func() { s.SetCaret(caret) })
	return true
}

// geometry is the node's widget as TextGeometry.
func (o *textIface) geometry() widget.TextGeometry {
	n := o.b.node(o.id)
	if n == nil {
		return nil
	}
	g, _ := n.w.(widget.TextGeometry)
	return g
}

// GetCharacterExtents is the character's box, window-relative like
// every Component coordinate.
func (o *textIface) GetCharacterExtents(offset int32, coordType uint32) (int32, int32, int32, int32) {
	r := o.rangeExtents(offset, offset+1)
	return int32(r.X), int32(r.Y), int32(r.W), int32(r.H)
}

// GetRangeExtents is the union of the range's character boxes.
func (o *textIface) GetRangeExtents(start, end int32, coordType uint32) (int32, int32, int32, int32) {
	r := o.rangeExtents(start, end)
	return int32(r.X), int32(r.Y), int32(r.W), int32(r.H)
}

// rangeExtents unions the boxes of start..end on the loop.
func (o *textIface) rangeExtents(start, end int32) render.Rect {
	g := o.geometry()
	if g == nil || end <= start {
		return render.Rect{}
	}
	var out render.Rect
	if !o.b.onLoop(func() {
		for i := start; i < end; i++ {
			if r, ok := g.CharExtents(int(i)); ok {
				out = unionRect(out, r)
			}
		}
	}) {
		return render.Rect{}
	}
	return out
}

// GetOffsetAtPoint is the character under a window-relative point.
func (o *textIface) GetOffsetAtPoint(x, y int32, coordType uint32) int32 {
	g := o.geometry()
	if g == nil {
		return -1
	}
	off := -1
	if !o.b.onLoop(func() { off = g.OffsetAt(widget.Point{X: int(x), Y: int(y)}) }) {
		return -1
	}
	return int32(off)
}

// unionRect is the smallest rect holding both; an empty a is b.
func unionRect(a, b render.Rect) render.Rect {
	if a.W == 0 && a.H == 0 && a.X == 0 && a.Y == 0 {
		return b
	}
	x0, y0 := min(a.X, b.X), min(a.Y, b.Y)
	x1, y1 := max(a.X+a.W, b.X+b.W), max(a.Y+a.H, b.Y+b.H)
	return render.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}
