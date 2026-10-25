package widget

import (
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// NavigationMenuItem is one entry of a NavigationMenu: a trigger
// opening Content, or - without Content - a link running OnActivate.
type NavigationMenuItem struct {
	Label      string
	Content    Widget
	OnActivate func()
}

// NavigationLink is one link of a navigation panel's grid.
type NavigationLink struct {
	Title, Description string
	OnActivate         func()
}

// The navigation menu's delays: Radix's open delay, and the close
// grace that lets the pointer cross from a trigger to the panel.
const (
	navMenuOpenDelay  = 200 * time.Millisecond
	navMenuCloseDelay = 300 * time.Millisecond
)

// navLinkWidth is a panel link's width; its description wraps in it.
const navLinkWidth = 240

// navMenuIndicator is the caret's half width under the open trigger.
const navMenuIndicator = 6

// NavigationMenu is a site's top navigation: a row of triggers whose
// panels open in one shared viewport under the menu, so moving between
// triggers swaps the panel in place and the viewport resizes to it,
// while a caret under the open trigger slides along. Resting the
// pointer on a trigger opens its panel, and it stays open while the
// pointer is on the menu or the panel; Enter, Space or Down on a
// focused trigger opens it with the keyboard in the panel, Left and
// Right step between the triggers, and Escape closes. It styles as the
// `navigationmenu` element: button.trigger rows (:checked while open),
// the .viewport panel box, and the panels' button.link entries.
type NavigationMenu struct {
	composite
	face     render.Font
	row      *Box
	items    []NavigationMenuItem
	triggers []*Button
	chevrons []*Box
	viewport *navMenuViewport

	active int
	// pointer and inViewport report the pointer on the menu and on the
	// open panel.
	pointer, inViewport bool
	pending             int
	timer               anim.Cancel
	close               func()

	caret *anim.Spring
	at    [2]float64 // the caret's center x and whether it shows (0/1)
}

// navMenuViewport is the shared panel box: it tells the menu while the
// pointer is on it.
type navMenuViewport struct {
	*Box
	m *NavigationMenu
}

// hoverWithin keeps the panel open while the pointer is on it.
func (v *navMenuViewport) hoverWithin(on bool) {
	v.m.inViewport = on
	v.m.settle()
}

// NewNavigationMenu returns a menu of items.
func NewNavigationMenu(face render.Font, items ...NavigationMenuItem) *NavigationMenu {
	m := &NavigationMenu{face: requireFace("widget.NewNavigationMenu", face), row: NewBox(Row, 4, 0), items: items, active: -1, pending: -1}
	m.SetElement("navigationmenu")
	m.viewport = &navMenuViewport{Box: NewBox(Column, 0, 0), m: m}
	m.viewport.AddClass("viewport")
	for i, it := range items {
		line := NewBox(Row, 4, 0)
		line.AppendAligned(NewLabel(m.face, 14, it.Label, 0), false, AlignCenter)
		chevron := NewBox(Column, 0, 0)
		if it.Content != nil {
			chevron.AddClass("chevron")
			chevron.Append(NewSymbol(SymbolChevronDown, 12), true)
			line.AppendAligned(chevron, false, AlignCenter)
		}
		b := NewButton(line, 0, 0)
		b.AddClass("trigger")
		b.OnClick = func() { m.click(i) }
		b.SetOnHoverWithin(func(on bool) {
			if on {
				m.hoverTrigger(i)
			}
		})
		m.triggers = append(m.triggers, b)
		m.chevrons = append(m.chevrons, chevron)
		m.row.AppendAligned(b, false, AlignCenter)
	}
	m.caret = anim.NewSpring(anim.SpringSnappy, m.at[:], func(pos []float64) {
		copy(m.at[:], pos)
		m.Invalidate()
	})
	m.initComposite(m, m.row)
	m.boxed = true
	return m
}

// Active is the open panel's item, -1 for none.
func (m *NavigationMenu) Active() int { return m.active }

// click opens a trigger's panel or runs a link.
func (m *NavigationMenu) click(i int) {
	if m.items[i].Content == nil {
		m.dismiss()
		if run := m.items[i].OnActivate; run != nil {
			run()
		}
		return
	}
	if m.active == i {
		m.dismiss()
		return
	}
	m.show(i, false)
}

// hoverWithin tracks the pointer on the menu.
func (m *NavigationMenu) hoverWithin(on bool) {
	m.pointer = on
	m.settle()
}

// hoverTrigger makes the trigger under the pointer the one to open:
// at once while a panel is open, after the open delay otherwise.
func (m *NavigationMenu) hoverTrigger(i int) {
	if m.items[i].Content == nil {
		return
	}
	if m.active >= 0 {
		m.show(i, false)
		return
	}
	m.pending = i
	m.settle()
}

// settle schedules the panel toward what the pointer asks: the
// pending trigger's open after the open delay, closed after the close
// grace once the pointer left the menu and the panel.
func (m *NavigationMenu) settle() {
	if m.timer != nil {
		m.timer()
		m.timer = nil
	}
	want := m.pointer || m.inViewport
	switch {
	case !want && m.active >= 0:
		m.timer = anim.Start(navMenuCloseDelay, func(t float64) {
			if t >= 1 {
				m.timer = nil
				m.dismiss()
			}
		})
	case want && m.active < 0 && m.pending >= 0:
		i := m.pending
		m.timer = anim.Start(navMenuOpenDelay, func(t float64) {
			if t >= 1 {
				m.timer = nil
				m.show(i, false)
			}
		})
	}
}

// show puts item i's panel in the viewport, opening it under the menu
// - grabbing the keyboard into it when the keyboard opened it.
func (m *NavigationMenu) show(i int, keyboard bool) {
	m.pending = -1
	for j, b := range m.triggers {
		b.SetState(StateChecked, j == i)
	}
	m.active = i
	m.viewport.Clear()
	m.viewport.Append(m.items[i].Content, true)
	b := m.triggers[i].Bounds()
	m.caret.SetTarget(float64(b.X+b.W/2), 1)
	if m.close != nil && !keyboard {
		return
	}
	if m.close != nil {
		m.close()
	}
	opts := PopoverOptions{NoGrab: !keyboard, Gap: PopoverGap, OnClosed: m.closed}
	if keyboard {
		opts.Focus = firstFocusable(m.items[i].Content)
	}
	m.close = OpenPopover(m, m, m.viewport, opts)
}

// firstFocusable is the first widget under w that takes the focus.
func firstFocusable(w Widget) Widget {
	var first Widget
	focusWalker(w, func(x Widget) {
		if _, ok := x.(KeyActionHandler); ok && first == nil {
			first = x
		}
	})
	return first
}

// dismiss closes the open panel.
func (m *NavigationMenu) dismiss() {
	if m.close != nil {
		m.close()
	}
	m.closed()
}

// closed resets the menu once the panel is gone.
func (m *NavigationMenu) closed() {
	m.close = nil
	m.active = -1
	for _, b := range m.triggers {
		b.SetState(StateChecked, false)
	}
	m.caret.SetTarget(m.at[0], 0)
}

// InterceptKey implements KeyInterceptor: on a focused trigger, Enter,
// Space and Down open its panel with the keyboard in it, and Escape
// closes.
func (m *NavigationMenu) InterceptKey(target Widget, a KeyAction, _ Mods) bool {
	i := -1
	for j, b := range m.triggers {
		if Widget(b) == target {
			i = j
		}
	}
	switch {
	case a == KeyDismiss && m.active >= 0:
		m.dismiss()
		return true
	case i < 0 || m.items[i].Content == nil:
		return false
	case a == KeyEnter || a == KeySpace || a == KeyDown:
		m.show(i, true)
		return true
	}
	return false
}

// NavigateFocus implements FocusNavigator: Left and Right step between
// the triggers.
func (m *NavigationMenu) NavigateFocus(target Widget, a KeyAction) Widget {
	for i, b := range m.triggers {
		if Widget(b) != target {
			continue
		}
		switch a {
		case KeyRight:
			return m.triggers[(i+1)%len(m.triggers)]
		case KeyLeft:
			return m.triggers[(i+len(m.triggers)-1)%len(m.triggers)]
		}
	}
	return nil
}

// Paint draws the triggers, then the caret under the open one.
func (m *NavigationMenu) Paint(cv *render.Canvas) {
	m.composite.Paint(cv)
	if m.at[1] <= 0 {
		return
	}
	th := Current()
	x := m.at[0]
	y := float64(m.bounds.Y + m.bounds.H)
	h := float64(navMenuIndicator) * m.at[1]
	p := &render.Path{}
	p.MoveTo(x-navMenuIndicator, y)
	p.LineTo(x, y-h)
	p.LineTo(x+navMenuIndicator, y)
	p.Close()
	cv.FillPath(p, th.Border)
}

// NewNavigationLinks is a navigation panel: a grid of cols columns of
// links, each its title over a muted description.
func NewNavigationLinks(face render.Font, cols int, links ...NavigationLink) *Grid {
	face = requireFace("widget.NewNavigationLinks", face)
	g := NewGrid(8, 8)
	for i, l := range links {
		col := NewBox(Column, 4, 0)
		title := NewLabel(face, 14, l.Title, 0)
		title.AddClass(css.Heading)
		desc := NewLabel(face, 14, l.Description, 0)
		desc.AddClass(css.Muted)
		desc.SetWrap(true)
		col.Append(title, false).Append(desc, false)
		b := NewButton(col, 0, 0)
		b.AddClass("link")
		b.OnClick = l.OnActivate
		g.Attach(newSizedCell(b, navLinkWidth), i%max(cols, 1), i/max(cols, 1), 1, 1)
	}
	return g
}
