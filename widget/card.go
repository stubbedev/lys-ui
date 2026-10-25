package widget

import "github.com/stubbedev/lys-ui/render"

// cardTitleStep is how far a card's title steps over the text size.
const cardTitleStep = 1.125

// Card is the card anatomy as one widget: a header with the title, an
// optional muted description and an optional trailing action, then the
// content, then a footer of actions, right-aligned unless SetFooterAlign
// says otherwise. The sections space themselves and pad the card from
// the theme (three Spacing between them, three Padding around), so a
// caller sets no margins; the text sizes follow the theme's TextSize,
// the title a step over it. It is the card element, styled as the .card
// class is, and its parts the .card-header,
// .card-title, .card-description, .card-action, .card-content and
// .card-footer classes a stylesheet restyles.
type Card struct {
	composite
	column *Box
	header *Box
	titles *Box
	title  *Label
	desc   *Label
	// actionSlot and contentSlot hold the action and the content, in
	// the tree while they do.
	actionSlot, contentSlot *Box
	footer                  *Box
}

// NewCard returns a card with the title and nothing else.
func NewCard(face render.Font, title string) *Card {
	face = requireFace("widget.NewCard", face)
	th := Current()
	c := &Card{
		column: NewBox(Column, 3*th.Spacing, 3*th.Padding),
		header: NewBox(Row, 2*th.Spacing, 0),
		titles: NewBox(Column, th.Spacing*3/4, 0),
		footer: NewBox(Row, th.Spacing, 0),

		actionSlot:  NewBox(Column, 0, 0),
		contentSlot: NewBox(Column, 0, 0),
	}
	c.title = NewLabel(face, th.TextSize*cardTitleStep, title, 0)
	c.title.SetWrap(true)
	c.desc = NewLabel(face, th.TextSize, "", 0)
	c.desc.SetWrap(true)
	c.header.AddClass("card-header")
	c.title.AddClass("card-title")
	c.desc.AddClass("card-description")
	c.actionSlot.AddClass("card-action")
	c.contentSlot.AddClass("card-content")
	c.footer.AddClass("card-footer")
	c.footer.SetHAlign(AlignEnd)
	c.titles.Append(c.title, false)
	c.header.Append(c.titles, true)
	c.column.Append(c.header, false)
	c.initComposite(c, c.column)
	c.boxed = true
	c.SetElement("card")
	return c
}

// SetTitle replaces the title.
func (c *Card) SetTitle(s string) { c.title.SetText(s) }

// Title returns the title.
func (c *Card) Title() string { return c.title.Text() }

// SetDescription sets the muted line under the title; empty hides it.
func (c *Card) SetDescription(s string) {
	c.desc.SetText(s)
	shown := len(c.titles.Children()) == 2
	switch {
	case s != "" && !shown:
		c.titles.Append(c.desc, false)
	case s == "" && shown:
		c.titles.Remove(c.desc)
	}
}

// Description returns the description.
func (c *Card) Description() string { return c.desc.Text() }

// SetAction puts w at the header's trailing edge, level with the
// title; nil removes it.
func (c *Card) SetAction(w Widget) {
	fillSlot(c.header, c.actionSlot, w, func() { c.header.AppendAligned(c.actionSlot, false, AlignStart) })
}

// Action returns the header's trailing widget, nil when none.
func (c *Card) Action() Widget { return slotted(c.actionSlot) }

// SetContent puts w between the header and the footer; nil removes it.
func (c *Card) SetContent(w Widget) {
	fillSlot(c.column, c.contentSlot, w, func() { c.column.InsertAligned(1, c.contentSlot, false, AlignFill) })
}

// Content returns the content, nil when none.
func (c *Card) Content() Widget { return slotted(c.contentSlot) }

// fillSlot puts w alone in slot, which insert places into parent when
// it was empty; nil empties the slot and takes it out of parent.
func fillSlot(parent, slot *Box, w Widget, insert func()) {
	in := slotted(slot) != nil
	slot.Clear()
	switch {
	case w == nil && in:
		parent.Remove(slot)
	case w != nil:
		slot.Append(w, true)
		if !in {
			insert()
		}
	}
}

// slotted is the widget in a slot, nil when it is empty.
func slotted(slot *Box) Widget {
	if kids := slot.Children(); len(kids) > 0 {
		return kids[0]
	}
	return nil
}

// AppendFooter adds w to the footer, after the ones already there; the
// footer shows once it holds anything.
func (c *Card) AppendFooter(w Widget) {
	if len(c.footer.Children()) == 0 {
		c.column.Append(c.footer, false)
	}
	c.footer.Append(w, false)
}

// ClearFooter empties and hides the footer.
func (c *Card) ClearFooter() {
	if len(c.footer.Children()) == 0 {
		return
	}
	c.footer.Clear()
	c.column.Remove(c.footer)
}

// SetFooterAlign places the footer's actions: AlignEnd (the default),
// AlignStart, AlignCenter, or AlignFill to spread the row across.
func (c *Card) SetFooterAlign(a Align) { c.footer.SetHAlign(a) }

// a11yName is the title, what the card is announced by.
func (c *Card) a11yName() string { return c.title.Text() }
