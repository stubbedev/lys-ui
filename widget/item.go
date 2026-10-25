package widget

import (
	"slices"

	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// ItemVariant picks an Item's look.
type ItemVariant uint8

// The item variants.
const (
	// ItemDefault is a plain row.
	ItemDefault ItemVariant = iota
	// ItemOutline draws a border around it.
	ItemOutline
	// ItemMuted gives it the muted fill.
	ItemMuted
)

// itemClasses are the variants' classes on the item element.
var itemClasses = [...]string{ItemDefault: "", ItemOutline: css.Outline, ItemMuted: css.Muted}

// Item is a content row: media - an icon on a muted
// tile, an avatar - then a title over a description, then actions, for
// lists and cards. Its text and media update in place, so a live list
// reconciles rows instead of rebuilding them, and SetOnActivate makes
// the whole row a control (click, Enter, Space). It styles as the
// `item` element with the variant's class (.outline, .muted), .small
// for the small size, .activatable while it activates, its .media
// tile, .title and .description.
type Item struct {
	composite
	row      *Box
	text     *Box
	media    *Box
	title    *Label
	desc     *Label
	actions  *Box
	variant  ItemVariant
	activate func()
}

// NewItem returns a default item; an empty description shows the
// title alone.
func NewItem(face render.Font, title, description string) *Item {
	face = requireFace("widget.NewItem", face)
	it := &Item{row: NewBox(Row, 16, 0), text: NewBox(Column, 4, 0), actions: NewBox(Row, 8, 0)}
	it.SetElement("item")
	it.title = NewLabel(face, 14, title, 0)
	it.title.AddClass("title")
	it.title.SetWrap(true)
	it.text.Append(it.title, false)
	it.desc = NewLabel(face, 14, description, 0)
	it.desc.AddClass("description")
	it.desc.SetWrap(true)
	it.desc.SetVisible(description != "")
	it.text.Append(it.desc, false)
	it.row.AppendAligned(it.text, true, AlignCenter)
	it.row.AppendAligned(it.actions, false, AlignCenter)
	it.initComposite(it, it.row)
	it.fillWidth = true
	it.boxed = true
	return it
}

// SetMedia leads the row with w on a muted 32px tile (an icon), or as
// it is when bare (an avatar, an image); nil removes it.
func (it *Item) SetMedia(w Widget, bare bool) {
	if it.media != nil {
		it.row.Remove(it.media)
		it.media = nil
	}
	if w == nil {
		return
	}
	it.media = NewBox(Column, 0, 0)
	if !bare {
		it.media.AddClass("media")
		it.media.SetSizeRequest(32, 32)
	}
	it.media.Append(NewSpacer(0, 0), true).AppendAligned(w, false, AlignCenter).Append(NewSpacer(0, 0), true)
	it.row.InsertAligned(0, it.media, false, AlignCenter)
}

// SetTitle replaces the title.
func (it *Item) SetTitle(title string) { it.title.SetText(title) }

// SetDescription replaces the description; empty hides its line.
func (it *Item) SetDescription(description string) {
	it.desc.SetText(description)
	it.desc.SetVisible(description != "")
}

// SetEllipsize keeps the title and the description on one line each,
// ending in an ellipsis when the row is too narrow, instead of
// wrapping: a one-token name (a device's address) then no longer sets
// the row's minimum width and pushes its actions out.
func (it *Item) SetEllipsize(on bool) {
	mode := EllipsizeNone
	if on {
		mode = EllipsizeEnd
	}
	for _, l := range [...]*Label{it.title, it.desc} {
		l.SetWrap(!on)
		l.SetEllipsize(mode)
	}
}

// SetOnActivate makes the whole row a control running fn on a click
// outside its actions, Enter or Space, with the hover fill and the
// focus ring; nil makes it a plain row again.
func (it *Item) SetOnActivate(fn func()) {
	it.activate = fn
	if fn == nil {
		it.RemoveClass(css.Activatable)
		it.SetCursorName("")
		return
	}
	it.AddClass(css.Activatable)
	it.SetCursorName("pointer")
}

// focusable implements focusOptOut: only an activatable item takes the
// focus.
func (it *Item) focusable() bool { return it.activate != nil }

// HitTest resolves an activatable item to itself, but for its action
// buttons; a plain item hits its parts.
func (it *Item) HitTest(p Point) Widget {
	if it.activate == nil {
		return it.composite.HitTest(p)
	}
	if hit := it.actions.HitTest(p); hit != nil && hit != Widget(it.actions) {
		return hit
	}
	return it.HitLeaf(it, p)
}

// ClickAt runs the activation.
func (it *Item) ClickAt(Point) {
	if it.activate != nil {
		it.activate()
	}
}

// KeyAction runs the activation on Enter or Space.
func (it *Item) KeyAction(a KeyAction, _ Mods) {
	if it.activate != nil && (a == KeyEnter || a == KeySpace) {
		it.activate()
	}
}

// AddAction appends w (a button) to the row's end.
func (it *Item) AddAction(w Widget) { it.actions.AppendAligned(w, false, AlignCenter) }

// SetVariant switches the item's look.
func (it *Item) SetVariant(v ItemVariant) {
	if c := itemClasses[it.variant]; c != "" {
		it.RemoveClass(c)
	}
	if c := itemClasses[v]; c != "" {
		it.AddClass(c)
	}
	it.variant = v
}

// SetSmall switches to the small size.
func (it *Item) SetSmall(on bool) {
	it.SetClass(css.Small, on)
}

// ButtonGroup joins buttons into one control: side by side or stacked,
// only the outer corners rounded and the inner borders shared, with
// SetSeparator lines between and groups nesting in groups. It styles as
// the `buttongroup` element, .vertical when stacked.
type ButtonGroup struct {
	composite
	box *Box
}

// NewButtonGroup joins buttons side by side.
func NewButtonGroup(buttons ...Widget) *ButtonGroup {
	g := &ButtonGroup{box: NewBox(Row, 0, 0)}
	g.SetElement("buttongroup")
	g.initComposite(g, g.box)
	for _, b := range buttons {
		g.Append(b)
	}
	g.boxed = true
	return g
}

// SetVertical stacks the buttons.
func (g *ButtonGroup) SetVertical(on bool) {
	if on {
		g.box.SetAxis(Column)
		g.AddClass("vertical")
		return
	}
	g.box.SetAxis(Row)
	g.RemoveClass("vertical")
}

// Append adds w (a button, a separator, a nested group) at the end.
// Nested groups sit apart.
func (g *ButtonGroup) Append(w Widget) {
	if _, ok := w.(*ButtonGroup); ok {
		g.box.SetSpacing(8)
	}
	g.box.Append(w, false)
}

// ItemGroup stacks Items into one list, a
// separator between each item and the next. It styles as the
// `itemgroup` element.
type ItemGroup struct {
	composite
	box   *Box
	items int
}

// NewItemGroup returns a group of items, in order.
func NewItemGroup(items ...Widget) *ItemGroup {
	g := &ItemGroup{box: NewBox(Column, 0, 0)}
	g.SetElement("itemgroup")
	g.initComposite(g, g.box)
	for _, it := range items {
		g.Append(it)
	}
	g.boxed = true
	return g
}

// Append adds w at the end, after a separator when the group already
// holds an item.
func (g *ItemGroup) Append(w Widget) {
	if g.items > 0 {
		g.box.Append(NewSeparator(Horizontal), false)
	}
	g.box.Append(w, false)
	g.items++
}

// Remove drops item w with the separator that set it apart (the one
// before it, or after it for the first), reporting whether w was in
// the group.
func (g *ItemGroup) Remove(w Widget) bool {
	kids := g.box.Children()
	i := slices.Index(kids, w)
	if i < 0 {
		return false
	}
	g.box.Remove(w)
	switch {
	case i > 0:
		g.box.Remove(kids[i-1])
	case len(kids) > 1:
		g.box.Remove(kids[1])
	}
	g.items--
	return true
}

// Measure shows a separator only between two shown items, as a GTK
// boxed list does, then measures the stack: a hidden item takes the
// separators beside it with it.
func (g *ItemGroup) Measure(con Constraints) Size {
	g.syncSeparators()
	return g.composite.Measure(con)
}

// syncSeparators shows the separator before an item when the item and
// an item above it are shown. The stack alternates item, separator,
// item: the separator before item k sits just before it.
func (g *ItemGroup) syncSeparators() {
	kids := g.box.Children()
	above := false
	for i := 0; i < len(kids); i += 2 {
		shown := ownVisible(kids[i])
		if i > 0 {
			if sep, ok := kids[i-1].(*Separator); ok {
				sep.SetVisible(shown && above)
			}
		}
		above = above || shown
	}
}

// Clear drops every item.
func (g *ItemGroup) Clear() {
	g.box.Clear()
	g.items = 0
}
