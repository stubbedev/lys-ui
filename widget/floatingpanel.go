package widget

import "github.com/stubbedev/lys-ui/render"

// InputRegioner is a window root that takes input only inside its
// region: outside it, pointer input passes through the surface to
// whatever is beneath, as on an overlay layer. An empty region takes
// input everywhere. The window applies it after every layout.
type InputRegioner interface {
	InputRegion() []render.Rect
}

// panelGripPx is the side of a floating panel's corner resize grip.
const panelGripPx = 16

// FloatingPanel is a movable, resizable panel over an area, an overlay
// or layer surface: a card centered until moved, dragged by its header
// (whose own controls keep their presses), resized from its bottom
// right corner within its size limits, and kept inside the area. A
// press outside the panel runs OnOutside; without it, as a window's
// root, the panel takes input only over itself (InputRegioner) and the
// rest passes through. The area is the floatingpanel element and the
// card its panel child, with the theme's card surface and a shadow.
type FloatingPanel struct {
	node
	panel  *Box
	header *panelHandle
	grip   *panelGrip
	// offset is the panel's top left in the area once placed (moved
	// or sized); until then it centers.
	offset   Point
	placed   bool
	size     Size
	min, max Size
	// rect is the panel's last arranged rect; settling marks a gesture
	// that ended, reported once the next arrange placed the panel.
	rect     render.Rect
	settling bool
	// OnOutside runs on a click outside the panel; nil passes those
	// through.
	OnOutside func()
	// OnGeometryChanged hears the panel's rect, in the area's
	// coordinates, once a move or resize gesture ended and the panel
	// was laid out there.
	OnGeometryChanged func(r render.Rect)
}

// NewFloatingPanel returns a panel of header over body; a nil header
// leaves the panel without a drag region.
func NewFloatingPanel(header, body Widget) *FloatingPanel {
	f := &FloatingPanel{panel: NewBox(Column, 0, 0)}
	f.SetElement("floatingpanel")
	f.panel.SetElement("panel")
	if header != nil {
		f.header = &panelHandle{child: header, f: f}
		f.panel.Append(f.header, false)
	}
	f.panel.Append(body, true)
	f.grip = &panelGrip{f: f}
	f.grip.SetCursorName("se-resize")
	return f
}

// SetSizeLimits bounds the panel's size: never below min, nor above
// max on an axis where it is not zero, nor beyond the area.
func (f *FloatingPanel) SetSizeLimits(min, max Size) {
	f.min, f.max = min, max
	f.InvalidateLayout()
}

// SetPanelSize sizes the panel w x h, within its limits; zero on an
// axis returns it to the content's natural size.
func (f *FloatingPanel) SetPanelSize(w, h int) {
	f.size = Size{W: max(w, 0), H: max(h, 0)}
	f.InvalidateLayout()
}

// SetPosition places the panel's top left at p in the area, kept
// inside it.
func (f *FloatingPanel) SetPosition(p Point) {
	f.offset, f.placed = p, true
	f.InvalidateLayout()
}

// Recenter centers the panel again, as before it was first moved.
func (f *FloatingPanel) Recenter() {
	f.placed = false
	f.InvalidateLayout()
}

// PanelRect is the panel's last arranged rect, in the area's
// coordinates.
func (f *FloatingPanel) PanelRect() render.Rect {
	r := f.rect
	r.X, r.Y = r.X-f.bounds.X, r.Y-f.bounds.Y
	return r
}

// limit holds s within the size limits and the area.
func (f *FloatingPanel) limit(s Size, area Size) Size {
	axis := func(n, lo, hi, room int) int {
		if hi > 0 {
			n = min(n, hi)
		}
		return min(max(n, lo), room)
	}
	return Size{W: axis(s.W, f.min.W, f.max.W, area.W), H: axis(s.H, f.min.H, f.max.H, area.H)}
}

// Measure takes the area offered, the panel's natural size where the
// offer is unbounded.
func (f *FloatingPanel) Measure(con Constraints) Size {
	nat := MeasureAsChild(f.panel, Constraints{Max: con.Max})
	w, h := con.Max.W, con.Max.H
	if w >= unboundedPx {
		w = nat.W
	}
	if h >= unboundedPx {
		h = nat.H
	}
	return clampSize(Size{W: w, H: h}, con)
}

// Arrange sizes the panel (its chosen size, else its natural one,
// within the limits), centers it until placed, keeps it inside the
// area, and pins the grip to its corner.
func (f *FloatingPanel) Arrange(r render.Rect) {
	f.node.Arrange(r)
	area := Size{W: r.W, H: r.H}
	want := MeasureAsChild(f.panel, Constraints{Max: area})
	if f.size.W > 0 {
		want.W = f.size.W
	}
	if f.size.H > 0 {
		want.H = f.size.H
	}
	sz := f.limit(want, area)
	if !f.placed {
		f.offset = Point{X: (area.W - sz.W) / 2, Y: (area.H - sz.H) / 2}
	}
	f.offset = Point{X: min(max(f.offset.X, 0), area.W-sz.W), Y: min(max(f.offset.Y, 0), area.H-sz.H)}
	f.rect = render.Rect{X: r.X + f.offset.X, Y: r.Y + f.offset.Y, W: sz.W, H: sz.H}
	MeasureAsChild(f.panel, Constraints{Min: sz, Max: sz})
	ArrangeAsChild(f.panel, f.rect)
	setParents(f, f.panel)
	f.grip.Arrange(render.Rect{X: f.rect.X + f.rect.W - panelGripPx, Y: f.rect.Y + f.rect.H - panelGripPx, W: panelGripPx, H: panelGripPx})
	setParents(f, f.grip)
	if f.settling {
		f.settling = false
		if f.OnGeometryChanged != nil {
			f.OnGeometryChanged(f.PanelRect())
		}
	}
}

// Paint draws the panel, then its grip.
func (f *FloatingPanel) Paint(cv *render.Canvas) {
	PaintChild(cv, f.panel)
	PaintChild(cv, f.grip)
}

// HitTest is the grip, then the panel; outside it the area itself
// when OnOutside takes those presses, else nothing.
func (f *FloatingPanel) HitTest(p Point) Widget {
	if hit := f.grip.HitTest(p); hit != nil {
		return hit
	}
	if hit := f.panel.HitTest(p); hit != nil {
		return hit
	}
	if f.OnOutside != nil && f.bounds.Contains(p.X, p.Y) {
		return f
	}
	return nil
}

// ClickAt is a click outside the panel.
func (f *FloatingPanel) ClickAt(Point) {
	if f.OnOutside != nil {
		f.OnOutside()
	}
}

// Children are the panel and its grip.
func (f *FloatingPanel) Children() []Widget { return []Widget{f.panel, f.grip} }

// InputRegion implements InputRegioner: the panel alone when presses
// outside pass through, everywhere when OnOutside takes them.
func (f *FloatingPanel) InputRegion() []render.Rect {
	if f.OnOutside != nil {
		return nil
	}
	return []render.Rect{f.rect}
}

// moveBy shifts the panel by d from where the drag began.
func (f *FloatingPanel) moveBy(from Point, d Point) {
	f.offset, f.placed = Point{X: from.X + d.X, Y: from.Y + d.Y}, true
	f.InvalidateLayout()
}

// startResize records the size a resize drag starts from.
func (f *FloatingPanel) startResize() Point {
	f.placed = true
	return Point{X: f.rect.W, Y: f.rect.H}
}

// resizeBy grows the panel by d from the size the drag began at, its
// top left fixed.
func (f *FloatingPanel) resizeBy(from Point, d Point) {
	f.size = Size{W: max(from.X+d.X, 1), H: max(from.Y+d.Y, 1)}
	f.InvalidateLayout()
}

// settled marks a finished gesture, its geometry reported by the next
// arrange.
func (f *FloatingPanel) settled() {
	f.settling = true
	f.InvalidateLayout()
}

// panelHandle is a panel's header as its drag region: presses on the
// header's controls stay theirs, the rest drag the panel.
type panelHandle struct {
	node
	child        Widget
	f            *FloatingPanel
	anchor, from Point
}

func (h *panelHandle) Measure(con Constraints) Size { return MeasureAsChild(h.child, con) }

func (h *panelHandle) Arrange(r render.Rect) {
	h.node.Arrange(r)
	ArrangeAsChild(h.child, r)
	setParents(h, h.child)
}

func (h *panelHandle) Paint(cv *render.Canvas) { PaintChild(cv, h.child) }

func (h *panelHandle) Children() []Widget { return []Widget{h.child} }

func (h *panelHandle) HitTest(p Point) Widget {
	if hit := h.child.HitTest(p); hit != nil && interactiveWithin(hit, h) {
		return hit
	}
	return h.HitLeaf(h, p)
}

// PressAt anchors the drag where the press landed.
func (h *panelHandle) PressAt(p Point) { h.anchor, h.from = p, h.f.offset }

// DragMove moves the panel with the pointer.
func (h *panelHandle) DragMove(p Point) {
	h.f.moveBy(h.from, Point{X: p.X - h.anchor.X, Y: p.Y - h.anchor.Y})
}

// PressEnd ends the move.
func (h *panelHandle) PressEnd() { h.f.settled() }

// CursorName is the move cursor.
func (h *panelHandle) CursorName() string { return "grab" }

// panelGrip is a panel's corner resize grip.
type panelGrip struct {
	node
	f            *FloatingPanel
	anchor, from Point
}

func (g *panelGrip) Measure(con Constraints) Size {
	return clampSize(Size{W: panelGripPx, H: panelGripPx}, con)
}

// Paint draws two short diagonal strokes in the corner.
func (g *panelGrip) Paint(cv *render.Canvas) {
	b := g.bounds
	col := Current().TextMuted
	for _, n := range []int{4, 8} {
		for i := range n {
			cv.FillRect(render.Rect{X: b.X + b.W - 4 - i, Y: b.Y + b.H - 4 - n + i, W: 1, H: 1}, col)
		}
	}
}

func (g *panelGrip) HitTest(p Point) Widget { return g.HitLeaf(g, p) }

// PressAt anchors the resize.
func (g *panelGrip) PressAt(p Point) { g.anchor, g.from = p, g.f.startResize() }

// DragMove resizes with the pointer.
func (g *panelGrip) DragMove(p Point) {
	g.f.resizeBy(g.from, Point{X: p.X - g.anchor.X, Y: p.Y - g.anchor.Y})
}

// PressEnd ends the resize.
func (g *panelGrip) PressEnd() { g.f.settled() }
