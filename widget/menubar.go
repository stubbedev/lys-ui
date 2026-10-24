package widget

import (
	"github.com/stubbedev/lys-ui/render"
)

// MenuBar is the in-window primary navigation (#91): a horizontal bar
// of root buttons, each opening its menu below through the existing
// popover machinery - the PopoverMenuBar shape. The bar never opens
// popovers itself (that is app plumbing); it reports the activation.
//
// Keyboard: F10 focuses the bar's first button when the app routes the
// key to it - MenuBar implements KeyActionHandler for exactly that
// handoff.
type MenuBar struct {
	composite
	face   render.Font
	sizePx float64

	// OnRoot fires when root i activates (click or keyboard): the app
	// opens the menu popover anchored at that root's button.
	OnRoot func(i int, anchor Boundser)

	buttons []*Button
}

// NewMenuBar returns a bar with one button per root label.
func NewMenuBar(face render.Font, sizePx float64, roots ...string) *MenuBar {
	face = requireFace("widget.NewMenuBar", face)
	m := &MenuBar{face: face, sizePx: sizePx}
	m.SetElement("menubar")
	row := NewBox(Row, 2, 4)
	for i, label := range roots {
		which := i
		btn := NewButton(NewLabel(face, sizePx, label, 0), 10, 5)
		btn.OnClick = func() { m.fire(which) }
		m.buttons = append(m.buttons, btn)
		row.Append(btn, false)
	}
	m.initComposite(m, row)
	m.fillWidth = true
	m.surface = surfaceFill
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

func (m *MenuBar) fire(i int) {
	if m.OnRoot != nil {
		m.OnRoot(i, m.buttons[i])
	}
}

// KeyAction handles the F10 handoff: it activates the first root,
// the way GTK menu bars open on F10.
func (m *MenuBar) KeyAction(a KeyAction, mods Mods) {
	if a != KeyEnter && a != KeySpace || len(m.buttons) == 0 {
		return
	}
	m.fire(0)
}
