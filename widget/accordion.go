package widget

import "github.com/stubbedev/lys-ui/render"

// AccordionMode is how many of an accordion's items may be open.
type AccordionMode uint8

// The accordion modes.
const (
	// AccordionSingle keeps at most one item open: opening one closes
	// the others.
	AccordionSingle AccordionMode = iota
	// AccordionMultiple lets any number be open.
	AccordionMultiple
)

// accordionDuration is the content's reveal.
const accordionDuration = defaultRevealDuration

// Accordion is a vertical stack of collapsible sections: each item a
// trigger row - the title, and a chevron that turns over as the item
// opens - above content whose height animates open and closed. In
// AccordionSingle mode opening an item closes the open one, and
// SetCollapsible decides whether the open item may close leaving none
// open. Up and Down move focus between the triggers, Home and End to
// the first and last. It styles as the `accordion` element, each item
// an .item (:checked while open) holding a button.trigger and its
// .content.
type Accordion struct {
	composite
	column      *Box
	mode        AccordionMode
	collapsible bool
	items       []*AccordionItem
}

// AccordionItem is one section of an Accordion (NewAccordionItem).
type AccordionItem struct {
	composite
	owner   *Accordion
	column  *Box
	trigger *Button
	title   *Label
	reveal  *Revealer

	// OnToggled fires after every open-state change, including ones the
	// accordion made to keep a single item open.
	OnToggled func(open bool)
}

// NewAccordion returns an empty accordion in mode; a single-mode one
// starts not collapsible.
func NewAccordion(mode AccordionMode) *Accordion {
	a := &Accordion{column: NewBox(Column, 0, 0), mode: mode}
	a.SetElement("accordion")
	a.initComposite(a, a.column)
	a.fillWidth = true
	a.boxed = true
	return a
}

// SetCollapsible lets the open item of a single-mode accordion close,
// leaving none open; a multiple-mode accordion always may.
func (a *Accordion) SetCollapsible(on bool) { a.collapsible = on }

// Append adds it as the last item. In single mode an open item closes
// the open one. An item belongs to one accordion; appending it to a
// second is a caller bug and panics.
func (a *Accordion) Append(it *AccordionItem) {
	if it.owner != nil {
		panic("widget: Accordion.Append: the item already belongs to an accordion")
	}
	it.owner = a
	a.items = append(a.items, it)
	a.column.Append(it, false)
	if it.Open() {
		a.closeOthers(it)
	}
}

// closeOthers closes every item but it in single mode.
func (a *Accordion) closeOthers(it *AccordionItem) {
	if a.mode != AccordionSingle {
		return
	}
	for _, other := range a.items {
		if other != it {
			other.setOpen(false)
		}
	}
}

// Items returns the items in order.
func (a *Accordion) Items() []*AccordionItem { return a.items }

// NavigateFocus implements FocusNavigator: Up and Down step between
// the item triggers, Home and End go to the first and last.
func (a *Accordion) NavigateFocus(target Widget, k KeyAction) Widget {
	triggers := make([]Widget, 0, len(a.items))
	at := -1
	for _, it := range a.items {
		if !isFocusable(it.trigger) {
			continue
		}
		if Widget(it.trigger) == target {
			at = len(triggers)
		}
		triggers = append(triggers, it.trigger)
	}
	if at < 0 {
		return nil
	}
	n := len(triggers)
	switch k {
	case KeyDown:
		return triggers[(at+1)%n]
	case KeyUp:
		return triggers[(at+n-1)%n]
	case KeyHome:
		return triggers[0]
	case KeyEnd:
		return triggers[n-1]
	}
	return nil
}

// NewAccordionItem returns a closed item titled title over content,
// for Accordion.Append.
func NewAccordionItem(face render.Font, title string, content Widget) *AccordionItem {
	face = requireFace("widget.NewAccordionItem", face)
	it := &AccordionItem{column: NewBox(Column, 0, 0)}
	it.AddClass("item")
	it.title = NewLabel(face, 14, title, 0)
	chevron := NewBox(Column, 0, 0)
	chevron.AddClass("chevron")
	chevron.Append(NewSymbol(SymbolChevronDown, 16), true)
	row := NewBox(Row, 16, 0)
	row.AppendAligned(it.title, true, AlignCenter)
	row.AppendAligned(chevron, false, AlignCenter)
	it.trigger = NewButton(row, 0, 0)
	it.trigger.AddClass("trigger")
	it.trigger.OnClick = func() { it.SetOpen(!it.Open()) }
	body := NewBox(Column, 0, 0)
	body.AddClass("content")
	body.Append(content, false)
	it.reveal = NewRevealer(body)
	it.reveal.SetTransition(RevealSlideDown)
	it.reveal.SetCollapse(true)
	it.reveal.SetDuration(accordionDuration)
	it.column.Append(it.trigger, false)
	it.column.Append(it.reveal, false)
	it.initComposite(it, it.column)
	it.fillWidth = true
	it.boxed = true
	return it
}

// Open reports whether the item is open (the target state, while its
// content may still be animating).
func (it *AccordionItem) Open() bool { return it.reveal.Revealed() }

// SetOpen opens or closes the item under its accordion's mode: opening
// one in single mode closes the open one, and closing the open one of
// a single-mode accordion that is not collapsible does nothing. An
// item not yet appended just opens or closes.
func (it *AccordionItem) SetOpen(open bool) {
	a := it.owner
	if a == nil {
		it.setOpen(open)
		return
	}
	if open == it.Open() || !open && a.mode == AccordionSingle && !a.collapsible {
		return
	}
	if open {
		a.closeOthers(it)
	}
	it.setOpen(open)
}

// setOpen moves the item to open, unconditionally.
func (it *AccordionItem) setOpen(open bool) {
	if open == it.Open() {
		return
	}
	it.reveal.SetRevealed(open)
	it.SetState(StateChecked, open)
	if it.OnToggled != nil {
		it.OnToggled(open)
	}
}

// SetTitle replaces the trigger's title.
func (it *AccordionItem) SetTitle(s string) { it.title.SetText(s) }

// Title returns the trigger's title.
func (it *AccordionItem) Title() string { return it.title.Text() }
