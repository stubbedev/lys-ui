package widget

import (
	"github.com/stubbedev/lys-ui/render"
)

// MenuBar is the in-window primary navigation (#91): a bordered,
// rounded strip of root buttons, each opening its menu below through
// the surface's host - the PopoverMenuBar shape. It styles as the
// `menubar` element, its roots as `menubar > box > button`, the root
// whose menu is open :checked.
//
// Keyboard: F10 focuses the bar's first button when the app routes the
// key to it - MenuBar implements KeyActionHandler for exactly that
// handoff.
type MenuBar struct {
	composite
	face   render.Font
	sizePx float64

	menus   []MenuBarMenu
	buttons []*Button
}

// MenuBarMenu is one root of a MenuBar: its label and the menu it
// opens.
type MenuBarMenu struct {
	Label string
	Items []MenuItem
}

// NewMenuBar returns a bar with one root button per menu.
func NewMenuBar(face render.Font, sizePx float64, menus ...MenuBarMenu) *MenuBar {
	face = requireFace("widget.NewMenuBar", face)
	m := &MenuBar{face: face, sizePx: sizePx, menus: menus}
	m.SetElement("menubar")
	row := NewBox(Row, 4, 0)
	for i, menu := range menus {
		which := i
		btn := NewButton(NewLabel(face, sizePx, menu.Label, 0), 0, 0)
		btn.OnClick = func() { m.fire(which) }
		m.buttons = append(m.buttons, btn)
		row.Append(btn, false)
	}
	m.initComposite(m, row)
	m.boxed = true
	return m
}

// Root returns root i's button, the anchor for its popover.
func (m *MenuBar) Root(i int) Boundser {
	if i < 0 || i >= len(m.buttons) {
		return nil
	}
	return m.buttons[i]
}

// Roots reports the root count.
func (m *MenuBar) Roots() int { return len(m.buttons) }

// SetFocused paints the bar's focus state (F10 handoff).
func (m *MenuBar) SetFocused(on bool) {
	m.SetState(StateSelected, on)
	m.Invalidate()
}

// fire opens root i's menu below the bar, under its button, the root
// :checked until the menu closes.
func (m *MenuBar) fire(i int) {
	root := m.buttons[i]
	root.SetState(StateChecked, true)
	OpenMenu(m, barSpan{bar: m, item: root}, m.face, m.sizePx, m.menus[i].Items, MenuOptions{Gap: PopoverGap, OnClosed: func() { root.SetState(StateChecked, false) }})
}

// barSpan is a menu's anchor: its item's columns across the bar's
// rows, so the menu keeps its gap from the bar's edge rather than
// from the item inside the bar's padding. The item is what the menu
// styles under (AnchorWidget).
type barSpan struct {
	bar  Boundser
	item *Button
}

func (s barSpan) Bounds() render.Rect {
	b, it := s.bar.Bounds(), s.item.Bounds()
	return render.Rect{X: it.X, Y: b.Y, W: it.W, H: b.H}
}

// AnchorWidget is the widget a popup opened against anchor styles
// under, nil for a bare rect: the anchor itself, or the item a
// menubar's menu opens beneath.
func AnchorWidget(anchor Boundser) Widget {
	switch a := anchor.(type) {
	case barSpan:
		return a.item
	case Widget:
		return a
	}
	return nil
}

// KeyAction handles the F10 handoff: it activates the first root,
// the way GTK menu bars open on F10.
func (m *MenuBar) KeyAction(a KeyAction, mods Mods) {
	if a != KeyEnter && a != KeySpace || len(m.buttons) == 0 {
		return
	}
	m.fire(0)
}
