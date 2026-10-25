package widget

import "github.com/stubbedev/lys-ui/render"

// BarEdge is the screen edge a Bar runs along.
type BarEdge uint8

// The bar edges.
const (
	BarTop BarEdge = iota
	BarBottom
	BarLeft
	BarRight
)

// Vertical reports a bar along the left or right edge, which lays out
// as a column.
func (e BarEdge) Vertical() bool { return e == BarLeft || e == BarRight }

// class is the edge's style class on the bar.
func (e BarEdge) class() string {
	return [...]string{"top", "bottom", "left", "right"}[e]
}

// PopoverSide is the side a popover opens on from a bar on this edge:
// away from the edge, into the screen.
func (e BarEdge) PopoverSide() PopoverSide {
	return [...]PopoverSide{PopoverBelow, PopoverAbove, PopoverRight, PopoverLeft}[e]
}

// barEdges are every edge class, for swapping one for another.
var barEdges = []string{"top", "bottom", "left", "right"}

// Bar is an edge panel: start, center and end zones along its edge,
// the center kept centered as a CenterBox keeps it. Along a left or
// right edge it lays out as a column (the vertical class), its
// BarButtons drop their labels for their icons, its BarSeparators turn
// across it and its SegmentIndicators stack. It styles as the bar
// element with its edge's class (top, bottom, left, right).
type Bar struct {
	composite
	box                *CenterBox
	start, center, end *Box
	edge               BarEdge
}

// NewBar returns an empty bar along edge.
func NewBar(edge BarEdge) *Bar {
	sp := Current().Spacing
	b := &Bar{start: NewBox(Row, sp, 0), center: NewBox(Row, sp, 0), end: NewBox(Row, sp, 0)}
	b.start.AddClass("start")
	b.center.AddClass("center")
	b.end.AddClass("end")
	b.box = NewCenterBox(b.start, b.center, b.end)
	b.SetElement("bar")
	b.initComposite(b, b.box)
	b.boxed = true
	b.SetEdge(edge)
	return b
}

// Start is the zone at the bar's start, left or top.
func (b *Bar) Start() *Box { return b.start }

// Center is the centered zone.
func (b *Bar) Center() *Box { return b.center }

// End is the zone at the bar's end, right or bottom.
func (b *Bar) End() *Box { return b.end }

// Edge is the edge the bar runs along.
func (b *Bar) Edge() BarEdge { return b.edge }

// SetEdge moves the bar to edge: the zones turn with it, and so do the
// bar parts inside them on their next layout.
func (b *Bar) SetEdge(edge BarEdge) {
	b.edge = edge
	// A horizontal bar claims the width it is offered, its zones
	// spread along it; a vertical one is as wide as its content.
	b.fillWidth = !edge.Vertical()
	axis := Row
	if edge.Vertical() {
		axis = Column
	}
	b.box.SetOrientation(axis)
	for _, z := range []*Box{b.start, b.center, b.end} {
		z.SetAxis(axis)
	}
	b.RemoveClass(barEdges...)
	b.AddClass(edge.class())
	if edge.Vertical() {
		b.AddClass("vertical")
	} else {
		b.RemoveClass("vertical")
	}
	walkWidgets(b, func(w Widget) bool {
		if l, ok := w.(interface{ InvalidateLayout() }); ok {
			l.InvalidateLayout()
		}
		return true
	})
}

// barEdgeOf is the edge of the bar w sits in, BarTop outside one.
func barEdgeOf(w Widget) BarEdge {
	for p := parentOf(w); p != nil; p = parentOf(p) {
		if b, ok := p.(*Bar); ok {
			return b.edge
		}
	}
	return BarTop
}

// BarButton is a bar's compact button: an icon, an optional label and
// an optional trailing badge, sized from the theme. In a vertical bar
// it shows its icon alone. With a popover set, a click opens it away
// from the bar's edge and the button stays :checked while it is open.
// Middle and right clicks and the wheel bind through SetOnPointer and
// SetOnScroll.
type BarButton struct {
	Button
	face     render.Font
	row      *Box
	icon     Widget
	label    *Label
	badge    *Badge
	vertical bool
	popover  func() Widget
	close    func()
}

// NewBarButton returns a bar button of icon (nil for none) and label
// (empty for none) in face.
func NewBarButton(face render.Font, icon Widget, label string) *BarButton {
	face = requireFace("widget.NewBarButton", face)
	row := NewBox(Row, Current().Spacing*3/4, 0)
	b := &BarButton{Button: *NewButton(row, 0, 0), face: face, row: row}
	b.label = NewLabel(face, Current().TextSize, label, 0)
	b.AddClass("bar-button")
	b.SetIcon(icon)
	b.row.AppendAligned(b.label, false, AlignCenter)
	b.syncLabel()
	b.OnClick = b.toggle
	return b
}

// SetIcon replaces the icon; nil removes it.
func (b *BarButton) SetIcon(w Widget) {
	if b.icon != nil {
		b.row.Remove(b.icon)
	}
	b.icon = w
	if w != nil {
		b.row.InsertAligned(0, w, false, AlignCenter)
	}
	b.syncLabel()
}

// SetLabel replaces the label; empty hides it.
func (b *BarButton) SetLabel(s string) {
	b.label.SetText(s)
	b.syncLabel()
}

// Label returns the label text.
func (b *BarButton) Label() string { return b.label.Text() }

// SetBadge shows text in a badge after the label; empty removes it.
func (b *BarButton) SetBadge(text string) {
	switch {
	case text == "" && b.badge != nil:
		b.row.Remove(b.badge)
		b.badge = nil
	case text != "" && b.badge == nil:
		b.badge = NewBadge(b.face, text)
		b.row.AppendAligned(b.badge, false, AlignCenter)
	case text != "":
		b.badge.SetText(text)
	}
}

// SetPopover makes a click open build's widget in a popover away from
// the bar's edge, and a click while it is open close it; nil leaves
// the button to its OnClick.
func (b *BarButton) SetPopover(build func() Widget) { b.popover = build }

// toggle opens or closes the popover.
func (b *BarButton) toggle() {
	switch {
	case b.popover == nil:
	case b.close != nil:
		b.close()
	default:
		b.SetState(StateChecked, true)
		b.close = OpenPopover(b, b, b.popover(), PopoverOptions{Gap: PopoverGap, Side: barEdgeOf(b).PopoverSide(), OnClosed: func() {
			b.close = nil
			b.SetState(StateChecked, false)
		}})
	}
}

// syncLabel shows the label unless it is empty or the bar is vertical
// with an icon to show instead.
func (b *BarButton) syncLabel() {
	b.label.SetVisible(b.label.Text() != "" && (!b.vertical || b.icon == nil))
}

// Arrange follows the bar's orientation, then lays the button out.
func (b *BarButton) Arrange(r render.Rect) {
	if v := barEdgeOf(b).Vertical(); v != b.vertical {
		b.vertical = v
		b.syncLabel()
		if v {
			b.AddClass("vertical")
		} else {
			b.RemoveClass("vertical")
		}
	}
	b.Button.Arrange(r)
}

// BarSeparator is a short rule across a bar between its items, inset
// from both of the bar's edges by the theme's padding.
type BarSeparator struct {
	Separator
}

// NewBarSeparator returns a bar separator.
func NewBarSeparator() *BarSeparator {
	s := &BarSeparator{orientation: Horizontal}
	s.AddClass("bar-separator")
	return s
}

// Measure turns the rule across the bar, then measures it.
func (s *BarSeparator) Measure(con Constraints) Size {
	across := Vertical
	if barEdgeOf(s).Vertical() {
		across = Horizontal
	}
	if across != s.orientation {
		s.orientation = across
		inset := Current().Padding * 3 / 4
		if across == Vertical {
			s.SetMargin(render.Insets{Top: inset, Bottom: inset})
		} else {
			s.SetMargin(render.Insets{Left: inset, Right: inset})
		}
	}
	return s.Separator.Measure(con)
}

// SegmentState marks a segment of a SegmentIndicator.
type SegmentState uint8

// The segment states.
const (
	SegmentEmpty SegmentState = iota
	// SegmentOccupied is a segment with something in it, a workspace
	// with windows.
	SegmentOccupied
	// SegmentUrgent is a segment asking for attention.
	SegmentUrgent
)

// segmentActive is the active segment's class.
const segmentActive = "active"

// segmentClasses are the state classes a segment carries.
var segmentClasses = []string{"occupied", "urgent"}

// SegmentIndicator is a row of segments, one per workspace or page:
// the active one drawn long and filled, occupied and urgent ones in
// their theme colours. A click on a segment reports it through
// OnSelect. In a vertical bar the segments stack. It styles as the
// segments element, each segment a button.segment with the active,
// occupied and urgent classes.
type SegmentIndicator struct {
	composite
	box    *Box
	segs   []*Button
	active int
	// OnSelect hears a clicked segment's index.
	OnSelect func(i int)
}

// NewSegmentIndicator returns n empty segments, none active.
func NewSegmentIndicator(n int) *SegmentIndicator {
	s := &SegmentIndicator{box: NewBox(Row, Current().Spacing/2, 0), active: -1}
	s.SetElement("segments")
	s.initComposite(s, s.box)
	s.SetCount(n)
	return s
}

// SetCount keeps n segments, the states of those kept.
func (s *SegmentIndicator) SetCount(n int) {
	for len(s.segs) > n {
		s.box.Remove(s.segs[len(s.segs)-1])
		s.segs = s.segs[:len(s.segs)-1]
	}
	for i := len(s.segs); i < n; i++ {
		seg := NewButton(NewSpacer(0, 0), 0, 0)
		seg.AddClass("segment")
		seg.SetAccessibleName(Tr("Segment") + " " + itoa(i+1))
		seg.OnClick = func() {
			if s.OnSelect != nil {
				s.OnSelect(i)
			}
		}
		s.box.AppendAligned(seg, false, AlignCenter)
		s.segs = append(s.segs, seg)
	}
	if s.active >= n {
		s.active = -1
	}
}

// Count is the number of segments.
func (s *SegmentIndicator) Count() int { return len(s.segs) }

// SetActive makes segment i the active one; out of range, none is.
func (s *SegmentIndicator) SetActive(i int) {
	if s.active >= 0 && s.active < len(s.segs) {
		s.segs[s.active].RemoveClass(segmentActive)
		s.segs[s.active].SetState(StateChecked, false)
	}
	s.active = -1
	if i >= 0 && i < len(s.segs) {
		s.active = i
		s.segs[i].AddClass(segmentActive)
		s.segs[i].SetState(StateChecked, true)
	}
}

// Active is the active segment, -1 for none.
func (s *SegmentIndicator) Active() int { return s.active }

// SetSegmentState marks segment i; out of range does nothing.
func (s *SegmentIndicator) SetSegmentState(i int, st SegmentState) {
	if i < 0 || i >= len(s.segs) {
		return
	}
	s.segs[i].RemoveClass(segmentClasses...)
	if st != SegmentEmpty {
		s.segs[i].AddClass(segmentClasses[st-1])
	}
}

// Measure stacks the segments in a vertical bar, then measures them.
func (s *SegmentIndicator) Measure(con Constraints) Size {
	axis := Row
	if barEdgeOf(s).Vertical() {
		axis = Column
	}
	if s.box.Axis() != axis {
		s.box.SetAxis(axis)
	}
	return s.composite.Measure(con)
}
