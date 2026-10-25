package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
)

// SidebarCollapse is how a Sidebar collapses on a wide window.
type SidebarCollapse uint8

// The collapse modes, shadcn's collapsible.
const (
	// SidebarOffcanvas slides the sidebar away entirely.
	SidebarOffcanvas SidebarCollapse = iota
	// SidebarIcon narrows it to a rail of the items' icons.
	SidebarIcon
	// SidebarNone keeps it expanded.
	SidebarNone
)

const (
	// sidebarWidth, sidebarRail and sidebarMobile are shadcn's
	// 16rem, 3rem and the off-canvas 18rem.
	sidebarWidth  = 256
	sidebarRail   = 48
	sidebarMobile = 288
	// sidebarNarrow is the width below which the sidebar is off-canvas,
	// shadcn's md breakpoint.
	sidebarNarrow = 768
	// sidebarSlide is the expand and collapse.
	sidebarSlide = 200 * time.Millisecond
)

// SidebarItem is one entry of a Sidebar group.
type SidebarItem struct {
	Label string
	// ID names the item for SetActive; empty uses its Label.
	ID string
	// Icon leads the row and is what the icon rail shows; nil shows
	// none.
	Icon Widget
	// Badge is a count or a note at the row's end; "" shows none.
	Badge string
	// Active marks the item as the current page.
	Active bool
	// OnActivate runs when the item is picked.
	OnActivate func()
	// Items are a submenu, shown under the item as it is toggled open.
	Items []SidebarItem
}

// sidebarRow is an item's row and the parts collapsing hides; parent
// is the submenu holding it, nil at a group's top level.
type sidebarRow struct {
	id     string
	button *Button
	label  *Label
	badge  *Label
	sub    *Box
	parent *Box
	// group is the collapsible group's items box holding the row, nil
	// in a plain group.
	group *Box
}

// sidebarHandle is the resize handle's half width either side of the
// panel's edge.
const sidebarHandle = 3

// Sidebar is an application sidebar (shadcn/ui's Sidebar) beside
// content: a header, groups of items with icons, badges and submenus,
// and a footer. On a wide window it docks beside the content and
// Toggle collapses it - off canvas or to an icon rail - with a slide;
// below shadcn's md width it hides and Toggle opens it as a sheet
// over the content. Picking an item makes it the active one. Bind
// ToggleAction to Ctrl+B, shadcn's shortcut, with an accelerator; the
// Trigger button toggles it too. It styles as the `sidebar` element:
// the .panel, .group-label labels, button.item rows (:checked while
// active), .badge labels and .sub submenus.
type Sidebar struct {
	composite
	face     render.Font
	collapse SidebarCollapse
	expanded bool
	narrow   bool
	width    float64
	slide    anim.Cancel

	sheet   *Sheet
	sheetIn *Box
	dock    *sizedCell
	panel   *Box
	header  *Box
	groups  *Box
	footer  *Box
	rows    []*sidebarRow
	labels  []Widget
	trigger *Button
	bps     breakpoints

	// openWidth is the expanded width (SetWidth); minWidth and maxWidth
	// bound the drag that resizes it, zero when it does not resize.
	openWidth          int
	minWidth, maxWidth int
	resize             dragGesture
	resizeFrom         int

	// OnWidthChanged fires with the expanded width after a drag
	// resized it, for the app to keep.
	OnWidthChanged func(width int)
}

// NewSidebar returns an expanded sidebar beside content, collapsing off
// canvas.
func NewSidebar(face render.Font, content Widget) *Sidebar {
	s := &Sidebar{face: requireFace("widget.NewSidebar", face), expanded: true, width: sidebarWidth, openWidth: sidebarWidth}
	s.SetElement("sidebar")
	s.header = NewBox(Column, 8, 0)
	s.groups = NewBox(Column, 8, 0)
	s.footer = NewBox(Column, 8, 0)
	scroll := NewScroll(s.groups)
	scroll.FillX = true
	s.panel = NewBox(Column, 8, 8)
	s.panel.AddClass("panel")
	s.panel.Append(s.header, false).Append(scroll, true).Append(s.footer, false)
	s.dock = newSizedCell(s.panel, sidebarWidth)
	row := NewBox(Row, 0, 0)
	row.Append(s.dock, false).Append(content, true)
	s.sheetIn = NewBox(Column, 0, 0)
	s.sheet = NewSheet(row, s.sheetIn, SheetLeft)
	s.sheet.SetWidth(sidebarMobile)
	s.bps.add(Breakpoint{Max: sidebarNarrow - 1, Apply: func() { s.setNarrow(true) }, Unapply: func() { s.setNarrow(false) }})
	s.initComposite(s, s.sheet)
	s.boxed = true
	return s
}

// SetHeader puts w at the panel's top.
func (s *Sidebar) SetHeader(w Widget) {
	s.header.Clear()
	s.header.Append(w, false)
}

// SetFooter puts w at the panel's bottom.
func (s *Sidebar) SetFooter(w Widget) {
	s.footer.Clear()
	s.footer.Append(w, false)
}

// AddGroup appends a group of items under label ("" for none).
func (s *Sidebar) AddGroup(label string, items ...SidebarItem) {
	g := NewBox(Column, 2, 0)
	if label != "" {
		l := NewLabel(s.face, 12, label, 0)
		l.AddClass("group-label")
		g.Append(l, false)
		s.labels = append(s.labels, l)
	}
	for _, it := range items {
		g.Append(s.item(it, nil), false)
	}
	s.groups.Append(g, false)
	s.applyCollapse()
}

// AddCollapsibleGroup appends a group whose label folds its items
// (shadcn's collapsible SidebarGroup): clicking the label, or its
// chevron, shows and hides them, open to start with when open.
func (s *Sidebar) AddCollapsibleGroup(label string, open bool, items ...SidebarItem) {
	body := NewBox(Column, 2, 0)
	first := len(s.rows)
	for _, it := range items {
		body.Append(s.item(it, nil), false)
	}
	for _, r := range s.rows[first:] {
		r.group = body
	}
	body.SetVisible(open)
	chevron := NewSymbol(SymbolChevronRight, 12)
	if open {
		chevron.SetKind(SymbolChevronDown)
	}
	line := NewBox(Row, 4, 0)
	line.AppendAligned(NewLabel(s.face, 12, label, 0), true, AlignCenter)
	line.AppendAligned(chevron, false, AlignCenter)
	head := NewButton(line, 0, 0)
	head.AddClass("group-label")
	head.SetAccessibleName(label)
	head.OnClick = func() {
		body.SetVisible(!body.Visible())
		if body.Visible() {
			chevron.SetKind(SymbolChevronDown)
		} else {
			chevron.SetKind(SymbolChevronRight)
		}
	}
	g := NewBox(Column, 2, 0)
	g.Append(head, false).Append(body, false)
	s.groups.Append(g, false)
	s.labels = append(s.labels, head)
	s.applyCollapse()
}

// item builds an item's row under parent (a submenu, nil at a
// group's top level), its own submenu under it.
func (s *Sidebar) item(it SidebarItem, parent *Box) Widget {
	id := it.ID
	if id == "" {
		id = it.Label
	}
	r := &sidebarRow{id: id, label: NewLabel(s.face, 14, it.Label, 0), parent: parent}
	line := NewBox(Row, 8, 0)
	if it.Icon != nil {
		line.AppendAligned(it.Icon, false, AlignCenter)
	}
	line.AppendAligned(r.label, true, AlignCenter)
	if it.Badge != "" {
		r.badge = NewLabel(s.face, 12, it.Badge, 0)
		r.badge.AddClass("badge")
		line.AppendAligned(r.badge, false, AlignCenter)
	}
	r.button = NewButton(line, 0, 0)
	r.button.AddClass("item")
	r.button.SetAccessibleName(it.Label)
	r.button.SetTooltip(it.Label)
	r.button.SetState(StateChecked, it.Active)
	s.rows = append(s.rows, r)
	if len(it.Items) == 0 {
		r.button.OnClick = func() { s.activate(r, it.OnActivate) }
		return r.button
	}
	r.sub = NewBox(Column, 2, 0)
	r.sub.AddClass("sub")
	for _, child := range it.Items {
		r.sub.Append(s.item(child, r.sub), false)
	}
	r.sub.SetVisible(it.Active)
	r.button.OnClick = func() { r.sub.SetVisible(!r.sub.Visible()) }
	col := NewBox(Column, 2, 0)
	col.Append(r.button, false).Append(r.sub, false)
	return col
}

// activate makes r the active item and runs its action.
func (s *Sidebar) activate(r *sidebarRow, run func()) {
	for _, o := range s.rows {
		o.button.SetState(StateChecked, o == r)
	}
	if s.narrow {
		s.sheet.SetOpen(false)
	}
	if run != nil {
		run()
	}
}

// SetActive makes the item called id (its ID, else its Label) the
// active one, as a click would without running it, and shows it: its
// submenu and its collapsed group open. It reports whether an item
// has that id.
func (s *Sidebar) SetActive(id string) bool {
	var found *sidebarRow
	for _, r := range s.rows {
		if r.id == id && found == nil {
			found = r
		}
	}
	if found == nil {
		return false
	}
	for _, r := range s.rows {
		r.button.SetState(StateChecked, r == found)
	}
	if found.parent != nil {
		found.parent.SetVisible(true)
	}
	if found.group != nil {
		found.group.SetVisible(true)
	}
	return true
}

// SetWidth sets the expanded width, shadcn's 16rem (256px) by default.
func (s *Sidebar) SetWidth(px int) {
	s.openWidth = max(sidebarRail, px)
	if s.expanded && s.slide == nil {
		s.width = float64(s.openWidth)
		s.dock.fixedWidth = s.openWidth
		s.dock.InvalidateLayout()
	}
}

// Width returns the expanded width.
func (s *Sidebar) Width() int { return s.openWidth }

// SetResizable lets the user drag the docked panel's edge to resize it
// between min and max pixels (OnWidthChanged reports the result); a
// zero max turns it off.
func (s *Sidebar) SetResizable(min, max int) {
	s.minWidth, s.maxWidth = min, max
}

// onHandle reports whether p is on the docked, expanded panel's edge
// while the sidebar resizes.
func (s *Sidebar) onHandle(p Point) bool {
	if s.maxWidth == 0 || s.narrow || !s.expanded || !IsVisible(s.dock) {
		return false
	}
	edge := s.dock.Bounds().X + s.dock.Bounds().W
	b := s.dock.Bounds()
	return p.Y >= b.Y && p.Y < b.Y+b.H && p.X >= edge-sidebarHandle && p.X < edge+sidebarHandle
}

// HitTest takes the resize handle itself, and otherwise the panel and
// content as laid out.
func (s *Sidebar) HitTest(p Point) Widget {
	if s.onHandle(p) {
		return s
	}
	return s.composite.HitTest(p)
}

// CursorName is the resize cursor over the handle.
func (s *Sidebar) CursorName() string { return "col-resize" }

// PressAt starts a resize on the handle.
func (s *Sidebar) PressAt(p Point) {
	if s.onHandle(p) {
		s.resize.begin(p)
		s.resizeFrom = s.openWidth
	}
}

// DragMove resizes the panel with the pointer, within the bounds.
func (s *Sidebar) DragMove(p Point) {
	dx, _, ok := s.resize.travel(p)
	if !ok {
		return
	}
	s.SetWidth(min(s.maxWidth, max(s.minWidth, s.resizeFrom+dx)))
}

// PressEnd ends a resize, reporting the new width.
func (s *Sidebar) PressEnd() {
	if s.resize.end() && s.OnWidthChanged != nil {
		s.OnWidthChanged(s.openWidth)
	}
}

// SetCollapse picks how a wide sidebar collapses.
func (s *Sidebar) SetCollapse(c SidebarCollapse) {
	s.collapse = c
	s.SetExpanded(s.expanded || c == SidebarNone)
}

// Expanded reports whether the docked sidebar is expanded.
func (s *Sidebar) Expanded() bool { return s.expanded }

// SetExpanded expands or collapses the docked sidebar with a slide; a
// SidebarNone sidebar stays expanded.
func (s *Sidebar) SetExpanded(on bool) {
	if s.collapse == SidebarNone {
		on = true
	}
	s.expanded = on
	target := float64(s.openWidth)
	switch {
	case on:
	case s.collapse == SidebarIcon:
		target = sidebarRail
	default:
		target = 0
	}
	if !on {
		s.applyCollapse()
	}
	if s.slide != nil {
		s.slide()
	}
	from := s.width
	s.slide = anim.Start(sidebarSlide, func(t float64) {
		s.width = from + (target-from)*easeOut(t)
		s.dock.fixedWidth = int(s.width + 0.5)
		s.dock.SetVisible(s.width > 0 && !s.narrow)
		s.dock.InvalidateLayout()
		if t >= 1 {
			s.slide = nil
			s.applyCollapse()
		}
	})
}

// railed reports the docked sidebar showing only its icons.
func (s *Sidebar) railed() bool { return !s.expanded && s.collapse == SidebarIcon && !s.narrow }

// applyCollapse shows the labels, badges and submenus unless the rail
// shows only icons.
func (s *Sidebar) applyCollapse() {
	full := !s.railed()
	for _, r := range s.rows {
		r.label.SetVisible(full)
		if r.badge != nil {
			r.badge.SetVisible(full)
		}
	}
	for _, l := range s.labels {
		if v, ok := l.(interface{ SetVisible(bool) }); ok {
			v.SetVisible(full)
		}
	}
	s.InvalidateLayout()
}

// Toggle collapses or expands the docked sidebar, or below the narrow
// width opens or closes it over the content.
func (s *Sidebar) Toggle() {
	if s.narrow {
		s.sheet.SetOpen(!s.sheet.Open())
		return
	}
	s.SetExpanded(!s.expanded)
}

// ToggleAction is Toggle as an action, for an accelerator: shadcn
// binds Ctrl+B.
func (s *Sidebar) ToggleAction() Activatable { return NewAction("sidebar.toggle", s.Toggle) }

// Trigger returns the button that toggles the sidebar, for the
// content's header.
func (s *Sidebar) Trigger() *Button {
	if s.trigger == nil {
		s.trigger = NewButton(NewSymbol(SymbolPanelLeft, 16), 0, 0)
		s.trigger.AddClass("trigger")
		s.trigger.SetAccessibleName(Tr("Toggle sidebar"))
		s.trigger.SetSizeRequest(28, 28)
		s.trigger.OnClick = s.Toggle
	}
	return s.trigger
}

// setNarrow moves the panel between the dock and the sheet as the
// width crosses the narrow breakpoint.
func (s *Sidebar) setNarrow(narrow bool) {
	s.narrow = narrow
	s.sheetIn.Clear()
	if narrow {
		s.dock.setRoot(NewBox(Column, 0, 0))
		s.sheetIn.Append(s.panel, true)
	} else {
		s.sheet.SetOpen(false)
		s.dock.setRoot(s.panel)
	}
	s.dock.SetVisible(!narrow && s.width > 0)
	s.applyCollapse()
}

// SetNarrowWidth moves the breakpoint below which the sidebar is off
// canvas (shadcn's md, 768px, by default): the sidebar measures its
// own width, so one inside a narrower region of a wide window wants a
// lower one.
func (s *Sidebar) SetNarrowWidth(w int) {
	s.bps.points[0].Max = w - 1
	s.bps.seen = false
	s.InvalidateLayout()
}

// Narrow reports the sidebar off canvas below the narrow width.
func (s *Sidebar) Narrow() bool { return s.narrow }

// Arrange evaluates the narrow breakpoint against the width, then lays
// the sidebar out.
func (s *Sidebar) Arrange(r render.Rect) {
	s.bps.evaluate(r.W)
	s.composite.Arrange(r)
}
