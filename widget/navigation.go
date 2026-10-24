package widget

import (
	"strconv"
	"time"

	"github.com/stubbedev/lys-ui/render"
)

// The navigation vocabulary of #94: NavigationView (a push/pop page
// stack with back), NavigationSplitView (sidebar beside content that
// collapses to navigation below a breakpoint), and OverlaySplitView
// (the sidebar overlaying content when collapsed - the Flap analog).
// Gesture support waits for the touch ticket; the back button, the
// back key (Alt+Left and Backspace, GTK's mappings), and pointer
// drag on Carousel work today.

// NavigationPage is one page: its title and its widget.
type NavigationPage struct {
	Title string
	W     Widget
}

// NavigationView is the push/pop shell: a header (back button when
// depth allows, the current page's title) over a Stack that slides
// pages in from the right on push and back on pop - the Stack's own
// transition doing the animating.
type NavigationView struct {
	composite
	face   render.Font
	sizePx float64

	stack *Stack
	pages []*NavigationPage
	bar   *Box
	back  *Button
	title *Label

	// OnPop fires after a pop delivered the previous page (nil at the
	// root).
	OnPop func(popped, now *NavigationPage)
}

// NewNavigationView returns a view showing root.
func NewNavigationView(face render.Font, sizePx float64, root *NavigationPage) *NavigationView {
	face = requireFace("widget.NewNavigationView", face)
	v := &NavigationView{face: face, sizePx: sizePx}
	v.stack = NewStack()
	v.stack.SetTransition(StackSlideLeftRight, navigationDuration)
	v.back = NewButton(NewSymbol(SymbolChevronLeft, int(sizePx)), 8, 4)
	v.back.OnClick = func() { v.Pop() }
	v.title = NewLabel(face, sizePx, "", 0)
	v.bar = NewBox(Row, 6, 6)
	v.bar.Append(v.back, false)
	v.bar.Append(v.title, false)
	column := NewBox(Column, 0, 0)
	column.Append(v.bar, false)
	column.Append(v.stack, true)
	v.initComposite(v, column)
	v.stack.Add("page0", root.W)
	v.pages = append(v.pages, root)
	v.syncBar()
	return v
}

// navigationDuration is the slide's length.
const navigationDuration = 220 * time.Millisecond

// Push shows page over the current one, sliding in from the right.
func (v *NavigationView) Push(p *NavigationPage) {
	v.pages = append(v.pages, p)
	v.stack.Add(v.pageName(len(v.pages)-1), p.W)
	v.stack.Show(v.pageName(len(v.pages) - 1))
	v.syncBar()
}

// Pop drops the top page (true when something popped) and slides
// back.
func (v *NavigationView) Pop() bool {
	if len(v.pages) <= 1 {
		return false
	}
	popped := v.pages[len(v.pages)-1]
	v.pages = v.pages[:len(v.pages)-1]
	v.stack.Show(v.pageName(len(v.pages) - 1))
	v.syncBar()
	if v.OnPop != nil {
		now := v.pages[len(v.pages)-1]
		v.OnPop(popped, now)
	}
	return true
}

// Depth reports the pushed page count (the root included).
func (v *NavigationView) Depth() int { return len(v.pages) }

// Page returns the top page.
func (v *NavigationView) Page() *NavigationPage {
	if len(v.pages) == 0 {
		return nil
	}
	return v.pages[len(v.pages)-1]
}

// PopToRoot unwinds to the root page.
func (v *NavigationView) PopToRoot() {
	for v.Pop() {
	}
}

// pageName is page i's stack key.
func (v *NavigationView) pageName(i int) string { return "page" + strconv.Itoa(i) }

// syncBar refreshes the back button's visibility and the title.
func (v *NavigationView) syncBar() {
	v.back.SetVisible(len(v.pages) > 1)
	if p := v.Page(); p != nil {
		v.title.SetText(p.Title)
	}
	v.InvalidateLayout()
}

// KeyAction implements the back keys: Alt+Left and Backspace pop.
func (v *NavigationView) KeyAction(a KeyAction, mods Mods) {
	if (a == KeyLeft && mods&ModAlt != 0) || a == KeyBackspace && mods == 0 {
		v.Pop()
	}
}

// Paint draws the bar's surface then the column.
func (v *NavigationView) Paint(cv *render.Canvas) {
	v.paintSurface(cv, v.bar.Bounds(), Current().Surface)
	PaintChild(cv, v.root)
}

// collapsingSplit is the engine both split views share: a wide layout
// and a narrow one over the same page widgets, a breakpoint choosing
// between them, and every widget pass routed to whichever is active.
// The page widgets live in both layouts' containers, so a mode switch
// clears their parent links: the next Arrange links them to the
// layout actually showing them as a first link, which re-cascades
// their subtrees - a plain re-link would keep the style resolved under
// the hidden layout's ancestors.
type collapsingSplit struct {
	composite
	wide      Widget
	narrow    Widget
	shared    []Widget
	bps       breakpoints
	collapsed bool
	// onCollapse runs after a mode switch (the flap closes its overlay
	// when going wide).
	onCollapse func(collapsed bool)
}

// init wires the core: self is the embedding widget (the parent the
// layouts' children report), collapseWidth the breakpoint (420, the
// adw default, when not positive).
func (c *collapsingSplit) init(self, wide, narrow Widget, collapseWidth int, shared ...Widget) {
	if collapseWidth <= 0 {
		collapseWidth = 420
	}
	c.initComposite(self, wide)
	c.wide, c.narrow, c.shared = wide, narrow, shared
	c.bps.add(Breakpoint{
		Max:     collapseWidth,
		Apply:   func() { c.setCollapsed(true) },
		Unapply: func() { c.setCollapsed(false) },
	})
}

// setCollapsed switches layouts and re-homes the shared widgets.
func (c *collapsingSplit) setCollapsed(on bool) {
	if c.collapsed == on {
		return
	}
	c.collapsed = on
	if on {
		c.setRoot(c.narrow)
	} else {
		c.setRoot(c.wide)
	}
	clearParents(c.shared...)
	if c.onCollapse != nil {
		c.onCollapse(on)
	}
}

// Collapsed reports which layout the last width chose.
func (c *collapsingSplit) Collapsed() bool { return c.collapsed }

// Arrange evaluates the breakpoint against the allocated width, then
// lays out whichever layout it picked - in the same pass, so a
// collapse never paints one frame of the old layout.
func (c *collapsingSplit) Arrange(r render.Rect) {
	c.bps.evaluate(r.W)
	measureChild(c, c.root, Constraints{Max: Size{W: r.W, H: r.H}})
	c.composite.Arrange(r)
}

// NavigationSplitView puts a sidebar beside content above a
// breakpoint and collapses to a NavigationView below it: the sidebar
// becomes the root page, content the pushed page - the mobile shape.
// ShowContent is the sidebar's activation hook; the back button and
// back keys return to the sidebar through the same view.
type NavigationSplitView struct {
	collapsingSplit
	nav     *NavigationView
	content *NavigationPage
}

// NewNavigationSplitView returns the split over sidebar and content,
// collapsing at or below collapseWidth (420 when not positive).
func NewNavigationSplitView(face render.Font, sizePx float64, sidebar, content *NavigationPage, collapseWidth int) *NavigationSplitView {
	s := &NavigationSplitView{content: content}
	wide := NewBox(Row, 0, 0)
	wide.AppendAligned(sidebar.W, false, AlignFill)
	wide.Append(content.W, true)
	s.nav = NewNavigationView(face, sizePx, sidebar)
	s.init(s, wide, s.nav, collapseWidth, sidebar.W, content.W)
	return s
}

// ShowContent pushes the content page in the collapsed view; above
// the breakpoint the content is already showing and this is a no-op.
func (s *NavigationSplitView) ShowContent() {
	if s.collapsed && s.nav.Depth() < 2 {
		s.nav.Push(s.content)
	}
}

// Navigation returns the collapsed layout's view (its depth, pop hook
// and back keys).
func (s *NavigationSplitView) Navigation() *NavigationView { return s.nav }

// KeyAction forwards the back keys to the collapsed view.
func (s *NavigationSplitView) KeyAction(a KeyAction, mods Mods) {
	if s.collapsed {
		s.nav.KeyAction(a, mods)
	}
}

// OverlaySplitView is the Flap shape: wide, the sidebar beside the
// content; collapsed, the content full-size with the sidebar sliding
// in over its leading edge on ShowSidebar. Gestures wait for the touch
// ticket.
type OverlaySplitView struct {
	collapsingSplit
	reveal *Revealer
}

// NewOverlaySplitView returns the split over content with sidebar;
// collapseWidth is 420 when not positive. Collapsed, the sidebar
// starts hidden.
func NewOverlaySplitView(sidebar, content Widget, collapseWidth int) *OverlaySplitView {
	s := &OverlaySplitView{}
	wide := NewBox(Row, 0, 0)
	wide.AppendAligned(sidebar, false, AlignFill)
	wide.Append(content, true)
	s.reveal = NewRevealer(sidebar)
	s.reveal.SetTransition(RevealSlideRight)
	s.reveal.SetDuration(200 * time.Millisecond)
	s.reveal.SetRevealed(false)
	narrow := NewOverlay()
	narrow.Append(content)
	narrow.AppendAligned(s.reveal, AlignStart, AlignFill)
	s.init(s, wide, narrow, collapseWidth, sidebar, content)
	s.onCollapse = func(collapsed bool) {
		if !collapsed {
			s.reveal.SetRevealed(false)
		}
	}
	return s
}

// ShowSidebar opens or closes the collapsed overlay; above the
// breakpoint the sidebar always shows and this is a no-op.
func (s *OverlaySplitView) ShowSidebar(on bool) {
	if s.collapsed {
		s.reveal.SetRevealed(on)
	}
}

// SidebarShowing reports whether the sidebar is on screen (always,
// wide).
func (s *OverlaySplitView) SidebarShowing() bool {
	return !s.collapsed || s.reveal.Revealed()
}
