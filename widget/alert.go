package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// AlertVariant picks an alert's look.
type AlertVariant uint8

// The alert variants.
const (
	// AlertDefault is the neutral callout.
	AlertDefault AlertVariant = iota
	// AlertDestructive paints the callout in the destructive color.
	AlertDestructive
)

// Alert is an inline callout in the content flow:
// an optional icon beside a title and a wrapped description, on a
// bordered card. Unlike Banner it carries no action and does not slide;
// it is there for as long as the content shows it. It styles as the
// `alert` element, with the .destructive class for that variant, and
// assistive technology announces it as a notification.
type Alert struct {
	composite
	row     *Box
	text    *Box
	icon    Widget
	title   *Label
	desc    *Label
	variant AlertVariant
}

// NewAlert returns a default-variant alert; an empty description
// shows the title alone.
func NewAlert(face render.Font, title, description string) *Alert {
	face = requireFace("widget.NewAlert", face)
	a := &Alert{row: NewBox(Row, 12, 0), text: NewBox(Column, 4, 0)}
	a.SetElement("alert")
	a.title = NewLabel(face, 14, title, 0)
	a.title.AddClass("title")
	a.title.SetWrap(true)
	a.desc = NewLabel(face, 14, description, 0)
	a.desc.AddClass("description")
	a.desc.SetWrap(true)
	a.text.Append(a.title, false)
	a.row.Append(a.text, true)
	a.SetDescription(description)
	a.initComposite(a, a.row)
	a.SetHExpand(true)
	a.boxed = true
	return a
}

// SetTitle replaces the title.
func (a *Alert) SetTitle(s string) { a.title.SetText(s) }

// Title returns the title.
func (a *Alert) Title() string { return a.title.Text() }

// SetDescription replaces the description; empty hides it.
func (a *Alert) SetDescription(s string) {
	a.desc.SetText(s)
	shown := len(a.text.Children()) == 2
	switch {
	case s != "" && !shown:
		a.text.Append(a.desc, false)
	case s == "" && shown:
		a.text.Remove(a.desc)
	}
	a.InvalidateLayout()
}

// Description returns the description.
func (a *Alert) Description() string { return a.desc.Text() }

// SetIcon puts w (an icon, typically 16px) before the text; nil
// removes it.
func (a *Alert) SetIcon(w Widget) {
	if a.icon != nil {
		a.row.Remove(a.icon)
	}
	a.icon = w
	if w != nil {
		a.row.InsertAligned(0, w, false, AlignStart)
	}
	a.InvalidateLayout()
}

// SetVariant switches the alert's look.
func (a *Alert) SetVariant(v AlertVariant) {
	a.RemoveClass(css.Destructive)
	if v == AlertDestructive {
		a.AddClass(css.Destructive)
	}
	a.variant = v
}

// Variant returns the alert's look.
func (a *Alert) Variant() AlertVariant { return a.variant }

// Role is RoleAlert.
func (a *Alert) Role() Role { return RoleAlert }

// a11yName is the title, what the announcement leads with.
func (a *Alert) a11yName() string { return a.title.Text() }
