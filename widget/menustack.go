package widget

import (
	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
)

// MenuStack presents a Menu tree with sliding submenus: activating a
// row with nested Items replaces the visible level with that submenu,
// headed by a back row that returns to the parent (so does the Left
// arrow). This is GTK's default
// PopoverMenu presentation, and it fits a single popup surface: the
// stack measures to the largest level of the whole tree, so a popover
// sized at open never clips a submenu it slides to.
//
// Every level is a real Menu, so rows keep all of Menu's behaviors
// (check/radio state, icons, disabled rows, accelerators, mnemonics).
// OnDismiss fires when a leaf row activates or a level asks to close
// (a click outside the rows, Esc); navigating between levels does not
// dismiss.
type MenuStack struct {
	node
	face   render.Font
	sizePx float64
	// path is the visible chain, root first; the last level paints.
	path []*Menu
	// OnDismiss runs when the stack as a whole should close.
	OnDismiss func()

	// Per-dispatch outcome: a level's back row or dismissal records
	// here, and the dispatcher resolves it once the level's handler
	// returns (Menu dismisses before it runs a row's action, so the
	// back row's intent is only known afterwards).
	popRequested bool
	dismissed    bool
}

// BackLabelPrefix leads the back row's label (the parent row's label
// follows).
const BackLabelPrefix = "‹ "

// NewMenuStack returns a stack showing items at its root, painted with
// face at sizePx.
func NewMenuStack(face render.Font, sizePx float64, items ...MenuItem) *MenuStack {
	s := &MenuStack{face: face, sizePx: sizePx}
	s.SetItems(items...)
	return s
}

// SetItems replaces the whole tree and returns to its root level - the
// shape a live model update takes (a tray application re-publishing
// its menu while it is open).
func (s *MenuStack) SetItems(items ...MenuItem) {
	s.path = []*Menu{s.level(items, "")}
	s.InvalidateLayout()
}

// Depth is the number of visible levels: 1 at the root.
func (s *MenuStack) Depth() int { return len(s.path) }

// Top returns the visible level.
func (s *MenuStack) Top() *Menu { return s.path[len(s.path)-1] }

// level builds one Menu; a non-empty parent label heads it with the
// back row.
func (s *MenuStack) level(items []MenuItem, parent string) *Menu {
	rows := items
	if parent != "" {
		rows = make([]MenuItem, 0, len(items)+1)
		rows = append(rows, MenuItem{Label: BackLabelPrefix + parent, OnClick: func() { s.popRequested = true }})
		rows = append(rows, items...)
	}
	m := NewMenu(s.face, s.sizePx, rows...)
	m.OnDismiss = func() { s.dismissed = true }
	m.OnSubmenu = func(index int, sub []MenuItem) {
		if open := m.items[index].OnOpen; open != nil {
			open()
		}
		s.path = append(s.path, s.level(sub, m.items[index].Label))
		s.InvalidateLayout()
	}
	return m
}

// dispatch runs one input on the visible level, then resolves what it
// asked for: a back row pops (and swallows the dismissal Menu issued
// before running it), a leaf activation or close request dismisses.
func (s *MenuStack) dispatch(fn func(m *Menu)) {
	s.popRequested, s.dismissed = false, false
	fn(s.Top())
	switch {
	case s.popRequested:
		s.pop()
	case s.dismissed && s.OnDismiss != nil:
		s.OnDismiss()
	}
}

// pop returns to the parent level; a no-op at the root.
func (s *MenuStack) pop() {
	if len(s.path) <= 1 {
		return
	}
	s.path = s.path[:len(s.path)-1]
	s.InvalidateLayout()
}

// Measure wants the largest level of the tree, so the stack's box
// holds every submenu it can slide to.
func (s *MenuStack) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	root := s.path[0]
	return s.measureStore(con, s.measureTree(root.items, "", con))
}

// measureTree is the max size over one level and all of its submenus.
func (s *MenuStack) measureTree(items []MenuItem, parent string, con Constraints) Size {
	sz := s.level(items, parent).Measure(con)
	for _, it := range items {
		if len(it.Items) == 0 {
			continue
		}
		sub := s.measureTree(it.Items, it.Label, con)
		sz.W, sz.H = max(sz.W, sub.W), max(sz.H, sub.H)
	}
	return sz
}

// Arrange lays the visible level into the stack's box.
func (s *MenuStack) Arrange(r render.Rect) {
	s.bounds = r
	top := s.Top()
	setParents(s, top)
	top.Arrange(r)
}

// Paint draws the visible level.
func (s *MenuStack) Paint(cv *render.Canvas) { s.Top().Paint(cv) }

// Children exposes the visible level to the tree walks (damage,
// accessibility).
func (s *MenuStack) Children() []Widget { return []Widget{s.Top()} }

// Role implements Roleer.
func (s *MenuStack) Role() Role { return RoleMenu }

// HitTest returns the stack itself inside its box: it is the input
// leaf and forwards to the visible level.
func (s *MenuStack) HitTest(p Point) Widget { return s.HitLeaf(s, p) }

// SetHovered forwards hover loss to the visible level.
func (s *MenuStack) SetHovered(on bool) { s.Top().SetHovered(on) }

// HoverMove forwards row tracking.
func (s *MenuStack) HoverMove(p Point) { s.Top().HoverMove(p) }

// DragMove forwards press-drag tracking.
func (s *MenuStack) DragMove(p Point) { s.Top().DragMove(p) }

// ClickAt activates a row of the visible level.
func (s *MenuStack) ClickAt(p Point) {
	s.dispatch(func(m *Menu) { m.ClickAt(p) })
}

// KeyAction drives the visible level; Left in a submenu goes back a
// level instead of closing the stack.
func (s *MenuStack) KeyAction(a KeyAction, mods Mods) {
	if a == KeyLeft && len(s.path) > 1 {
		s.pop()
		return
	}
	s.dispatch(func(m *Menu) { m.KeyAction(a, mods) })
}

// ActivateMnemonic fires a mnemonic of the visible level.
func (s *MenuStack) ActivateMnemonic(sym xkb.Keysym) bool {
	fired := false
	s.dispatch(func(m *Menu) { fired = m.ActivateMnemonic(sym) })
	return fired
}

// RawKey implements RawKeyHandler through the visible level.
func (s *MenuStack) RawKey(code uint32, mods Mods, sym xkb.Keysym) bool {
	consumed := false
	s.dispatch(func(m *Menu) { consumed = m.RawKey(code, mods, sym) })
	return consumed
}
