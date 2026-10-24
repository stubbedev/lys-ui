package widget

import (
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// ItemVariant picks an Item's look, shadcn/ui's variants.
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

// Item is a content row (shadcn/ui's Item): media - an icon on a muted
// tile, an avatar - then a title over a description, then actions, for
// lists and cards. It styles as the `item` element with the variant's
// class (.outline, .muted), .small for the small size, its .media tile,
// .title and .description.
type Item struct {
	composite
	row     *Box
	text    *Box
	media   *Box
	title   *Label
	desc    *Label
	actions *Box
	variant ItemVariant
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
	if description != "" {
		it.desc = NewLabel(face, 14, description, 0)
		it.desc.AddClass("description")
		it.desc.SetWrap(true)
		it.text.Append(it.desc, false)
	}
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

// SetSmall switches to the small size, shadcn's size="sm".
func (it *Item) SetSmall(on bool) {
	if on {
		it.AddClass(css.Small)
	} else {
		it.RemoveClass(css.Small)
	}
}

// ButtonGroup joins buttons into one control (shadcn/ui's Button
// Group): side by side or stacked, only the outer corners rounded and
// the inner borders shared, with SetSeparator lines between and groups
// nesting in groups. It styles as the `buttongroup` element, .vertical
// when stacked.
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
// Nested groups sit apart, shadcn's gap-2 between them.
func (g *ButtonGroup) Append(w Widget) {
	if _, ok := w.(*ButtonGroup); ok {
		g.box.SetSpacing(8)
	}
	g.box.Append(w, false)
}

// ItemGroup stacks Items into one list (shadcn/ui's Item Group), a
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
