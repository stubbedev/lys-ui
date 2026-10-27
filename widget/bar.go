package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

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
	if !edge.Vertical() {
		b.SetHExpand(true)
	}
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
	b.SetClass("vertical", edge.Vertical())
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
	iconAt   IconPosition
	label    *Label
	badge    *Badge
	vertical bool
	popover  func() Widget
	close    func()
}

// IconPosition is which side of a BarButton's label its icon sits on.
type IconPosition uint8

// The icon positions.
const (
	IconStart IconPosition = iota
	IconEnd
)

// NewBarButton returns a bar button of icon (nil for none) and label
// (empty for none) in face. Its parts sit border-spacing apart, a
// theme rule a stylesheet sets either way.
func NewBarButton(face render.Font, icon Widget, label string) *BarButton {
	face = requireFace("widget.NewBarButton", face)
	row := NewBox(Row, 0, 0)
	b := &BarButton{Button: *NewButton(row, 0, 0), face: face, row: row, icon: icon}
	b.label = NewLabel(face, Current().TextSize, label, 0)
	b.AddClass("bar-button")
	b.restack()
	b.OnClick = b.toggle
	return b
}

// SetIcon replaces the icon; nil removes it.
func (b *BarButton) SetIcon(w Widget) {
	b.icon = w
	b.restack()
}

// SetIconPosition puts the icon before the label (IconStart, the
// default) or after it.
func (b *BarButton) SetIconPosition(p IconPosition) {
	b.iconAt = p
	b.restack()
}

// SetLabel replaces the label text; empty hides it.
func (b *BarButton) SetLabel(s string) {
	b.label.SetText(s)
	b.syncLabel()
}

// Label is the button's label, for its text, classes and limits.
func (b *BarButton) Label() *Label { return b.label }

// SetBadge shows text in a badge after the label; empty removes it.
func (b *BarButton) SetBadge(text string) {
	switch {
	case text == "":
		b.badge = nil
	case b.badge == nil:
		b.badge = NewBadge(b.face, text)
	default:
		b.badge.SetText(text)
		return
	}
	b.restack()
}

// restack lays the parts out in order: the icon on its side of the
// label, then the badge.
func (b *BarButton) restack() {
	b.row.Clear()
	parts := []Widget{b.icon, b.label}
	if b.iconAt == IconEnd {
		parts[0], parts[1] = b.label, b.icon
	}
	for _, w := range append(parts, b.badgeWidget()) {
		if w != nil {
			b.row.AppendAligned(w, false, AlignCenter)
		}
	}
	b.syncLabel()
}

// badgeWidget is the badge as a Widget, nil without one.
func (b *BarButton) badgeWidget() Widget {
	if b.badge == nil {
		return nil
	}
	return b.badge
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
		b.SetClass("vertical", v)
	}
	b.Button.Arrange(r)
}

// BarSeparator is a short rule across a bar between its items, inset
// from both of the bar's edges by the theme's padding. The stylesheet
// styles it: its color is the rule's, min-width the thickness of a
// rule across a horizontal bar and min-height its length (filling the
// bar when unset), swapped across a vertical bar.
type BarSeparator struct {
	Separator
}

// NewBarSeparator returns a bar separator.
func NewBarSeparator() *BarSeparator {
	s := &BarSeparator{orientation: Horizontal}
	s.SetElement("separator")
	s.AddClass("bar-separator")
	return s
}

// rule is the rule's thickness and length (0 to fill) from the
// stylesheet, for its orientation.
func (s *BarSeparator) rule() (thick, length int) {
	v := s.style(s)
	thickProp, lengthProp := style.PropMinWidth, style.PropMinHeight
	if s.orientation == Horizontal {
		thickProp, lengthProp = lengthProp, thickProp
	}
	return max(1, picki(v, thickProp, sepThickness)), picki(v, lengthProp, 0)
}

// Measure turns the rule across the bar, then wants its thickness by
// its length.
func (s *BarSeparator) Measure(axis Axis, forSize int) Sizes {
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
	thick, length := s.rule()
	if s.orientation == Vertical {
		return FixedSizes(Size{W: thick, H: length}.Along(axis))
	}
	return FixedSizes(Size{W: length, H: thick}.Along(axis))
}

// Paint draws the rule in its color, centered across its slot and, at
// a set length, along it.
func (s *BarSeparator) Paint(cv *render.Canvas) {
	b := s.bounds
	thick, length := s.rule()
	col := pickc(0, s.style(s), style.PropColor, Current().Border)
	r := render.Rect{X: b.X + (b.W-thick)/2, Y: b.Y, W: thick, H: b.H}
	if length > 0 {
		r.Y, r.H = b.Y+(b.H-min(length, b.H))/2, min(length, b.H)
	}
	if s.orientation == Horizontal {
		r = render.Rect{X: b.X, Y: b.Y + (b.H-thick)/2, W: b.W, H: thick}
		if length > 0 {
			r.X, r.W = b.X+(b.W-min(length, b.W))/2, min(length, b.W)
		}
	}
	cv.FillRect(r, col)
}

// SegmentState marks a segment of a SegmentIndicator.
type SegmentState uint8

// The segment states.
const (
	SegmentEmpty SegmentState = iota
	// SegmentOccupied is a segment with something in it, a workspace
	// with windows.
	SegmentOccupied
)

// The classes a segment's state, activity and urgency set.
const (
	segmentActive   = "active"
	segmentOccupied = "occupied"
	segmentUrgent   = "urgent"
)

// SegmentIndicator is a row of segments, one per workspace or page:
// the active one drawn long and filled, occupied and urgent ones in
// their theme colours. A segment can hold content (SetSegmentContent),
// centred in it,
// and Segment reaches its button. A click on a segment reports it
// through OnSelect. In a vertical bar the segments stack. It styles as the
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
	s.boxed = true
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
		slot := NewBox(Row, 0, 0)
		slot.SetHAlign(AlignCenter)
		slot.SetVAlign(AlignCenter)
		seg := NewButton(slot, 0, 0)
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

// Segment is segment i's button, nil out of range: for its classes,
// inline style, tooltip, and its SetOnPointer and SetOnScroll for the
// other buttons and the wheel.
func (s *SegmentIndicator) Segment(i int) *Button {
	if i < 0 || i >= len(s.segs) {
		return nil
	}
	return s.segs[i]
}

// SetSegmentContent shows w inside segment i (a label, app icons), nil
// for none; out of range does nothing.
func (s *SegmentIndicator) SetSegmentContent(i int, w Widget) {
	seg := s.Segment(i)
	if seg == nil {
		return
	}
	slot, ok := seg.child.(*Box)
	if !ok {
		panic("widget: a segment's child is not its content slot")
	}
	slot.Clear()
	if w != nil {
		slot.AppendAligned(w, false, AlignCenter)
	}
}

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

// SetSegmentState marks segment i empty or occupied; out of range does
// nothing.
func (s *SegmentIndicator) SetSegmentState(i int, st SegmentState) {
	if seg := s.Segment(i); seg != nil {
		seg.SetClass(segmentOccupied, st == SegmentOccupied)
	}
}

// SetSegmentUrgent marks segment i as asking for attention or not,
// whatever its state; out of range does nothing.
func (s *SegmentIndicator) SetSegmentUrgent(i int, urgent bool) {
	if seg := s.Segment(i); seg != nil {
		seg.SetClass(segmentUrgent, urgent)
	}
}

// Measure stacks the segments in a vertical bar, then measures them.
func (s *SegmentIndicator) Measure(axis Axis, forSize int) Sizes {
	along := Row
	if barEdgeOf(s).Vertical() {
		along = Column
	}
	if s.box.Axis() != along {
		s.box.SetAxis(along)
	}
	return s.composite.Measure(axis, forSize)
}
