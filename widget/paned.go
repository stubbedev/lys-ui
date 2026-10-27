package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// panedHandleW is the divider's width along the paned's axis, a
// one-pixel handle; panedHandleSlop is how far beyond it a
// press still grabs, so the handle is easy to hit without being easy
// to see.
const (
	panedHandleW    = 1
	panedHandleSlop = 6
)

// Paned is a two-pane container with a draggable divider: children are
// arranged along the axis (Row side by side, Column stacked), the
// divider position is pixels from the start edge, and dragging,
// keyboard nudges, or SetPosition move it — the children relayout
// live, one frame per position change.
//
// The position is a request the arrangement clamps: each pane holds at
// least its minimum, so the
// divider never starves a pane past what it declared it needs; when a
// resize makes the request impossible, the position re-normalizes to
// the nearest clamp — GTK's keep-child-one semantics. Position reports
// the arranged position; SetPosition only asks.
//
// The handle owns a generous hit zone around the divider (the slop
// overlaps the panes' edges); presses there drag, and the Paned is
// keyboard-focusable: arrows nudge along the axis (by a twentieth of
// the span, one pixel minimum), Home and End pin to the extremes.
// The pane children keep their own input: hits outside the handle zone
// reach them, and they join focus traversal through Children.
type Paned struct {
	node
	axis     Axis
	start    Widget
	end      Widget
	position int
	// positionSet records that a position was requested — SetPosition,
	// a drag, or a keyboard nudge. Until one arrives, the arrangement
	// opens at the start pane's natural size (GTK's default split).
	positionSet bool
	arranged    int
	arrangedSet bool
	hovered     bool
	pressed     bool
	// maxPos caps the position (SetMaxPosition); zero is no cap.
	maxPos int
	// sep is the divider's `separator` node (`paned > separator`): its
	// min size slots the divider and its margin shifts it inside the
	// slot; its box layers paint it.
	sep stylePart

	// OnPositionChanged fires after every applied position change —
	// drag, keyboard, SetPosition, or a resize that re-normalized the
	// request — with the arranged position.
	OnPositionChanged func(position int)
}

// NewPaned returns a paned along axis with the two children; either
// may be nil, in which case the other takes the whole rect and no
// divider is drawn.
func NewPaned(axis Axis, start, end Widget) *Paned {
	p := &Paned{axis: axis, start: start, end: end}
	p.sep.SetElement("separator")
	setParents(p, &p.sep)
	return p
}

// appendStyleKids is the separator and the two panes (styleKids).
func (p *Paned) appendStyleKids(buf []Widget) []Widget {
	buf = append(buf, &p.sep)
	for _, pane := range [2]Widget{p.start, p.end} {
		if pane != nil {
			buf = append(buf, pane)
		}
	}
	return buf
}

// sepSlot is the divider's slot along the axis: the separator's min
// size plus its margins, at least 0 (negative margins can shrink the
// slot to nothing — a divider painted over the pane edge).
func (p *Paned) sepSlot() int {
	sv := p.sep.style(&p.sep)
	prop := style.PropMinWidth
	if p.axis == Column {
		prop = style.PropMinHeight
	}
	m := marginOf(sv)
	slot := picki(sv, prop, panedHandleW)
	if p.axis == Column {
		slot += m.Top + m.Bottom
	} else {
		slot += m.Left + m.Right
	}
	return max(0, slot)
}

// Position returns the divider's arranged position in pixels from the
// start edge — the clamped truth, never the unarranged request. Before
// the first arrangement it is zero.
func (p *Paned) Position() int { return p.arranged }

// SetPosition asks for a divider position in pixels from the start
// edge; the next arrangement clamps it between the panes' minimums
// and under SetMaxPosition, and fires
// OnPositionChanged when the arranged position moved.
func (p *Paned) SetPosition(pos int) {
	p.position, p.positionSet = pos, true
	p.Invalidate()
}

// SetMaxPosition caps the divider at pos pixels from the start edge
// (the start pane's largest extent), above the start pane's floor;
// zero removes the cap. Drags, keys and SetPosition all stop there.
func (p *Paned) SetMaxPosition(pos int) {
	if p.maxPos == pos {
		return
	}
	p.maxPos = max(pos, 0)
	p.InvalidateLayout()
}

// availMain is the dividable extent inside r: the main size minus the
// divider slot.
func (p *Paned) availMain(r render.Rect) int {
	return max(0, Size{W: r.W, H: r.H}.Along(p.axis)-p.sepSlot())
}

// clampPos clamps a requested position between the panes' minimums
// inside r.
func (p *Paned) clampPos(pos int, r render.Rect) int {
	avail := p.availMain(r)
	lo := 0
	if p.start != nil {
		lo = min(p.paneRequest(p.start).Min, avail)
	}
	hi := avail
	if p.end != nil {
		hi = max(lo, avail-min(p.paneRequest(p.end).Min, avail))
	}
	if p.maxPos > 0 {
		hi = max(lo, min(hi, p.maxPos))
	}
	return min(max(pos, lo), hi)
}

// applyPos records the clamped position and fires the hook on change.
// A missing pane means there is no divider: the position rests at
// zero and nothing fires.
func (p *Paned) applyPos(pos int, r render.Rect) {
	if p.start == nil || p.end == nil {
		p.position, p.arranged, p.arrangedSet = 0, 0, true
		return
	}
	pos = p.clampPos(pos, r)
	// A resize that made the old position impossible re-normalizes the
	// request to what the arrangement could do (GTK keeps child one
	// until it cannot).
	p.position, p.positionSet = pos, true
	if p.arrangedSet && p.arranged == pos {
		return
	}
	p.arranged, p.arrangedSet = pos, true
	p.Invalidate()
	p.placePanes()
	if p.OnPositionChanged != nil {
		p.OnPositionChanged(pos)
	}
}

// Measure wants the two panes' requests plus the handle along the
// axis, so a Paned nested in a squeezing container (a Grid) keeps both
// panes alive; across, the larger pane's request at its natural extent
// along.
func (p *Paned) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := p.measureHit(axis, forSize); ok {
		return sz
	}
	var req Sizes
	for _, pane := range [2]Widget{p.start, p.end} {
		switch {
		case pane == nil:
		case axis == p.axis:
			req = req.plus(p.paneRequest(pane))
		default:
			req = req.widest(measureChild(p, pane, axis, Unknown))
		}
	}
	if axis == p.axis {
		req = req.grow(p.sepSlot())
	}
	return p.measureStore(axis, forSize, req)
}

// paneRequest is a pane's request along the axis.
func (p *Paned) paneRequest(pane Widget) Sizes { return measureChild(p, pane, p.axis, Unknown) }

// Arrange clamps the requested position, lays the panes around it, and
// keeps the parent links current for the walks.
func (p *Paned) Arrange(r render.Rect) {
	p.node.Arrange(r)
	req := p.position
	if !p.positionSet && p.start != nil {
		// No request yet: open at the start pane's natural size, GTK's
		// default split.
		req = p.paneRequest(p.start).Nat
	}
	p.applyPos(req, r)
	p.placePanes()
}

// placePanes lays the panes around the arranged position: the whole
// rect to whoever is alone, a split with the handle between them
// otherwise. Runs from Arrange and from every applied position change
// (a drag, a nudge), so the panes resize live, one layout per change
// rather than one per frame.
func (p *Paned) placePanes() {
	r := p.bounds
	switch {
	case p.start == nil && p.end == nil:
		return
	case p.start == nil:
		arrangeChild(p.end, r)
		setParents(p, p.end)
		return
	case p.end == nil:
		arrangeChild(p.start, r)
		setParents(p, p.start)
		return
	}
	if p.axis == Column {
		slot := p.sepSlot()
		arrangeChild(p.start, render.Rect{X: r.X, Y: r.Y, W: r.W, H: p.arranged})
		arrangeChild(p.end, render.Rect{X: r.X, Y: r.Y + p.arranged + slot, W: r.W, H: max(0, r.H-p.arranged-slot)})
	} else {
		slot := p.sepSlot()
		arrangeChild(p.start, render.Rect{X: r.X, Y: r.Y, W: p.arranged, H: r.H})
		arrangeChild(p.end, render.Rect{X: r.X + p.arranged + slot, Y: r.Y, W: max(0, r.W-p.arranged-slot), H: r.H})
	}
	setParents(p, p.start, p.end)
}

// Paint draws the panes, then the divider between them through its
// `separator` node: the box layers it resolves, the theme's handle
// looks as the fallback per state, and no divider when a pane is
// missing (the other still paints).
func (p *Paned) Paint(cv *render.Canvas) {
	for _, pane := range [2]Widget{p.start, p.end} {
		if pane != nil && IsVisible(pane) {
			PaintChild(cv, pane)
		}
	}
	if p.start == nil || p.end == nil {
		return
	}
	th := Current()
	bg := th.Border
	switch {
	case !IsEnabled(p):
		bg = th.DisabledText()
	case p.pressed:
		bg = th.Accent
	case p.hovered:
		bg = th.TextMuted
	}
	sv := p.sep.style(&p.sep)
	rect := p.sepRect()
	p.sep.Arrange(rect)
	radii := radiusOr(sv, min(rect.W, rect.H)/2)
	fx := pushEffects(cv, sv, rect, radii)
	cv = fx.cv
	defer fx.pop()
	paintBoxBehind(cv, sv, rect, radii, borderOf(sv), pickc(0, sv, style.PropBackgroundColor, bg))
	paintOutline(cv, sv, rect, radii)
}

// sepRect is the divider's painted box: the separator's min size
// inside the slot, shifted by its start margin.
func (p *Paned) sepRect() render.Rect {
	sv := p.sep.style(&p.sep)
	m := marginOf(sv)
	if p.axis == Column {
		return render.Rect{X: p.bounds.X, Y: p.bounds.Y + p.arranged + m.Top, W: p.bounds.W, H: picki(sv, style.PropMinHeight, panedHandleW)}
	}
	return render.Rect{X: p.bounds.X + p.arranged + m.Left, Y: p.bounds.Y, W: picki(sv, style.PropMinWidth, panedHandleW), H: p.bounds.H}
}

// handleRect is the divider's hit zone: the whole slot.
func (p *Paned) handleRect() render.Rect {
	slot := p.sepSlot()
	if p.axis == Column {
		return render.Rect{X: p.bounds.X, Y: p.bounds.Y + p.arranged, W: p.bounds.W, H: slot}
	}
	return render.Rect{X: p.bounds.X + p.arranged, Y: p.bounds.Y, W: slot, H: p.bounds.H}
}

// inHandle reports whether p sits inside the handle's hit zone: the
// handle plus its slop, generously overlapping the panes' edges.
func (p *Paned) inHandle(pt Point) bool {
	h := p.handleRect()
	if p.axis == Column {
		return pt.X >= h.X && pt.X < h.X+h.W &&
			pt.Y >= h.Y-panedHandleSlop && pt.Y < h.Y+h.H+panedHandleSlop
	}
	return pt.Y >= h.Y && pt.Y < h.Y+h.H &&
		pt.X >= h.X-panedHandleSlop && pt.X < h.X+h.W+panedHandleSlop
}

// Role implements Roleer.
func (p *Paned) Role() Role { return RoleSplitter }

// HitTest returns the Paned inside the handle's hit zone — the handle
// owns the press even where the slop overlaps a pane — then whatever
// pane is under p, else nil: the paned's own chrome swallows nothing.
func (p *Paned) HitTest(pt Point) Widget {
	if p.inHandle(pt) {
		return p
	}
	if p.start != nil {
		if hit := p.start.HitTest(pt); hit != nil {
			return hit
		}
	}
	if p.end != nil {
		if hit := p.end.HitTest(pt); hit != nil {
			return hit
		}
	}
	return nil
}

// Children exposes the panes for focus traversal and the damage walk.
func (p *Paned) Children() []Widget {
	var out []Widget
	if p.start != nil {
		out = append(out, p.start)
	}
	if p.end != nil {
		out = append(out, p.end)
	}
	return out
}

// appendChildren appends the panes, matching Children.
func (p *Paned) appendChildren(buf []Widget) []Widget {
	if p.start != nil {
		buf = append(buf, p.start)
	}
	if p.end != nil {
		buf = append(buf, p.end)
	}
	return buf
}

// SetHovered tracks the handle's hover shade.
func (p *Paned) SetHovered(on bool) {
	if p.hovered == on {
		return
	}
	p.hovered = on
	p.invalidateStyle()
}

// SetPressed tracks the drag's pressed shade and reports the press the
// divider follows.
func (p *Paned) SetPressed(on bool) {
	if p.pressed == on {
		return
	}
	p.pressed = on
	p.invalidateStyle()
}

// DragMove drags the divider to the pointer: the position is where the
// pointer sits minus half the handle, clamped by the arrangement.
// Disabled panes do not drag.
func (p *Paned) DragMove(pt Point) {
	if !IsEnabled(p) {
		return
	}
	var main int
	if p.axis == Column {
		main = pt.Y - p.bounds.Y
	} else {
		main = pt.X - p.bounds.X
	}
	p.applyPos(main-panedHandleW/2, p.bounds)
}

// KeyAction implements KeyActionHandler: arrows nudge the divider
// along the axis by a twentieth of the span (one pixel at least),
// Home and End pin it to the extremes; the cross-axis arrows and every
// other action are not the handle's. Disabled panes ignore keys.
func (p *Paned) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(p) {
		return
	}
	var forward, back, home, end bool
	if p.axis == Column {
		forward, back, home, end = a == KeyDown, a == KeyUp, a == KeyHome, a == KeyEnd
	} else {
		forward, back, home, end = a == KeyRight, a == KeyLeft, a == KeyHome, a == KeyEnd
	}
	step := max(1, p.availMain(p.bounds)/20)
	switch {
	case forward:
		p.applyPos(p.position+step, p.bounds)
	case back:
		p.applyPos(p.position-step, p.bounds)
	case home:
		p.applyPos(0, p.bounds)
	case end:
		p.applyPos(p.availMain(p.bounds), p.bounds)
	}
}

// CursorName returns the resize cursor for the axis while the handle
// is the hovered widget (HitTest returns the Paned only inside the
// handle's hit zone, so being hovered is being on the handle).
func (p *Paned) CursorName() string {
	if p.axis == Column {
		return "row-resize"
	}
	return "col-resize"
}
