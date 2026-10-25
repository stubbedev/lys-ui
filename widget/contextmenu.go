package widget

import (
	"slices"

	"github.com/stubbedev/lys-ui/render"
)

// ContextMenu gives a region a context menu (shadcn/ui's Context
// Menu): a right click anywhere on its child, a long press, or the
// Menu key (Shift+F10) while something inside it has the focus opens
// items as a menu at the pointer - submenus, check and radio items and
// shortcuts included, as Menu renders them. It styles as the
// `contextmenu` element.
type ContextMenu struct {
	composite
	face   render.Font
	sizePx float64
	items  []MenuItem
}

// NewContextMenu wraps child with a context menu of items.
func NewContextMenu(face render.Font, child Widget, items ...MenuItem) *ContextMenu {
	c := &ContextMenu{face: requireFace("widget.NewContextMenu", face), sizePx: 14, items: slices.Clone(items)}
	c.SetElement("contextmenu")
	c.initComposite(c, child)
	c.boxed = true
	return c
}

// SetItems replaces the menu's items.
func (c *ContextMenu) SetItems(items ...MenuItem) { c.items = slices.Clone(items) }

// Items returns the menu's items.
func (c *ContextMenu) Items() []MenuItem { return slices.Clone(c.items) }

// OpenContextMenu implements ContextMenuOpener: the menu opens at at.
func (c *ContextMenu) OpenContextMenu(at Point) {
	if len(c.items) == 0 {
		return
	}
	OpenMenu(c, pointAnchor{X: at.X, Y: at.Y, W: 1, H: 1}, c.face, c.sizePx, c.items, nil)
}

// Gesture implements GestureHandler: a long press opens the menu where
// the finger is.
func (c *ContextMenu) Gesture(g Gesture) bool {
	if g.Kind != GestureLongPress {
		return false
	}
	c.OpenContextMenu(g.At)
	return true
}

// pointAnchor is a rect a popup opens beside: the pointer's pixel.
type pointAnchor render.Rect

// Bounds implements Boundser.
func (a pointAnchor) Bounds() render.Rect { return render.Rect(a) }
