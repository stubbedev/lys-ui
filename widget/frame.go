package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// frameRadius and framePad are the Adwaita frame outline's corner and
// inner padding.
const (
	frameRadius = 8
	framePad    = 8
)

// Frame is a labeled outline around a child (GTK Frame): the optional
// label sits above a rounded, bordered box holding the child. The
// outline is the frame's `border` node, so a stylesheet restyles it
// like any surface; with no label the frame is the outline alone.
type Frame struct {
	composite
	column *Box
	label  *Label
	border *frameBorder
	inner  *Box
}

// frameBorder is the outline: an unfilled composite surface with the
// default ring.
type frameBorder struct{ composite }

// NewFrame returns a frame around child titled label ("" for none),
// the label painted with face.
func NewFrame(face render.Font, label string, child Widget) *Frame {
	face = requireFace("widget.NewFrame", face)
	f := &Frame{column: NewBox(Column, 6, 0), inner: NewBox(Column, 0, framePad)}
	f.SetElement("frame")
	f.label = NewLabel(face, 14, label, 0)
	f.label.AddClass(css.Heading)
	f.border = &frameBorder{}
	f.border.SetElement("border")
	f.border.initComposite(f.border, f.inner)
	f.border.surface, f.border.surfaceRadius, f.border.surfaceRing = surfaceNone, frameRadius, 1
	f.SetChild(child)
	f.SetLabel(label)
	f.initComposite(f, f.column)
	return f
}

// Label reports the title.
func (f *Frame) Label() string { return f.label.Text() }

// SetLabel retitles the frame; "" drops the label row.
func (f *Frame) SetLabel(s string) {
	f.label.SetText(s)
	f.column.Clear()
	if s != "" {
		f.column.Append(f.label, false)
	}
	f.column.Append(f.border, true)
	f.InvalidateLayout()
}

// SetChild replaces the framed widget.
func (f *Frame) SetChild(w Widget) {
	f.inner.Clear()
	f.inner.Append(w, true)
	f.InvalidateLayout()
}

// AspectFrame keeps its child at a width:height ratio (GTK
// AspectFrame): it takes what its parent gives and arranges the child
// in the largest rect of that ratio, centered. A ratio of zero obeys
// the child's own natural ratio.
type AspectFrame struct {
	composite
	ratio float64
}

// NewAspectFrame returns a frame holding child at ratio (W/H).
func NewAspectFrame(child Widget, ratio float64) *AspectFrame {
	a := &AspectFrame{ratio: max(ratio, 0)}
	a.SetElement("aspectframe")
	a.initComposite(a, child)
	return a
}

// Ratio reports the ratio (zero: the child's).
func (a *AspectFrame) Ratio() float64 { return a.ratio }

// SetRatio changes the ratio.
func (a *AspectFrame) SetRatio(r float64) {
	a.ratio = max(r, 0)
	a.InvalidateLayout()
}

// effective resolves the ratio, falling back to the child's natural
// one (and to square for a child with no height).
func (a *AspectFrame) effective(nat Size) float64 {
	switch {
	case a.ratio > 0:
		return a.ratio
	case nat.W > 0 && nat.H > 0:
		return float64(nat.W) / float64(nat.H)
	}
	return 1
}

// Measure grows the child's natural size along one axis to the ratio.
func (a *AspectFrame) Measure(con Constraints) Size {
	if sz, ok := a.measureHit(con); ok {
		return sz
	}
	nat := measureChild(a, a.root, con)
	r := a.effective(nat)
	sz := nat
	if float64(nat.W) < float64(nat.H)*r {
		sz.W = int(float64(nat.H)*r + 0.5)
	} else {
		sz.H = int(float64(nat.W)/r + 0.5)
	}
	return a.measureStore(con, clampSize(sz, con))
}

// Arrange fits the largest rect of the ratio inside r, centered.
func (a *AspectFrame) Arrange(r render.Rect) {
	a.node.Arrange(r)
	ratio := a.effective(measureChild(a, a.root, Constraints{Max: Size{W: r.W, H: r.H}}))
	w, h := r.W, int(float64(r.W)/ratio+0.5)
	if h > r.H {
		w, h = int(float64(r.H)*ratio+0.5), r.H
	}
	arrangeChild(a.root, render.Rect{X: r.X + (r.W-w)/2, Y: r.Y + (r.H-h)/2, W: w, H: h})
}

// SizeGroupMode picks the axes a SizeGroup equalizes.
type SizeGroupMode uint8

const (
	// SizeGroupHorizontal equalizes widths.
	SizeGroupHorizontal SizeGroupMode = 1 << iota
	// SizeGroupVertical equalizes heights.
	SizeGroupVertical
	// SizeGroupBoth equalizes both.
	SizeGroupBoth = SizeGroupHorizontal | SizeGroupVertical
)

// SizeGroup makes widgets in different containers measure alike (GTK
// SizeGroup): every member requests the largest natural size of the
// group along the mode's axes - labels in separate rows line up. Add
// returns the wrapper to place in the tree in the member's stead; a
// member's size change relayouts every peer.
type SizeGroup struct {
	mode    SizeGroupMode
	members []*sizeMember
}

// NewSizeGroup returns an empty group equalizing mode's axes.
func NewSizeGroup(mode SizeGroupMode) *SizeGroup { return &SizeGroup{mode: mode} }

// Add joins w and returns the wrapper to place in the tree.
func (g *SizeGroup) Add(w Widget) Widget {
	m := &sizeMember{group: g}
	m.initComposite(m, w)
	g.members = append(g.members, m)
	for _, o := range g.members {
		o.InvalidateLayout()
	}
	return m
}

// sizeMember is one wrapped member.
type sizeMember struct {
	composite
	group   *SizeGroup
	lastCon Constraints
	seen    bool
}

// Measure is the group's largest natural size along the mode's axes,
// each peer measured under its own last constraints.
func (m *sizeMember) Measure(con Constraints) Size {
	if sz, ok := m.measureHit(con); ok {
		return sz
	}
	m.lastCon, m.seen = con, true
	sz := measureChild(m, m.root, con)
	for _, o := range m.group.members {
		if o == m {
			continue
		}
		oc := con
		if o.seen {
			oc = o.lastCon
		}
		os := measureChild(o, o.root, oc)
		if m.group.mode&SizeGroupHorizontal != 0 {
			sz.W = max(sz.W, os.W)
		}
		if m.group.mode&SizeGroupVertical != 0 {
			sz.H = max(sz.H, os.H)
		}
	}
	return m.measureStore(con, clampSize(sz, con))
}

// markMeasureDirty relayouts every peer with this member: their size
// depends on its natural size.
func (m *sizeMember) markMeasureDirty() bool {
	was := m.node.markMeasureDirty()
	for _, o := range m.group.members {
		if o != m && !o.measureDirty {
			o.InvalidateLayout()
		}
	}
	return was
}
