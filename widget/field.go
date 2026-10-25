package widget

import (
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// FieldOrientation is how a Field lays its label out against its
// control.
type FieldOrientation uint8

// The field orientations.
const (
	// FieldVertical stacks the label, the control, the description and
	// the error.
	FieldVertical FieldOrientation = iota
	// FieldHorizontal puts the label, description and error beside the
	// control: a switch or a checkbox with its text.
	FieldHorizontal
	// FieldResponsive is horizontal from fieldResponsiveMin wide, and
	// vertical below.
	FieldResponsive
)

// fieldResponsiveMin is where a responsive field turns horizontal: a
// 448px container.
const fieldResponsiveMin = 448

// fieldTextSize is the label and help text size.
const fieldTextSize = 14

// Field is a labeled form control: a label, the
// control, an optional description and an error message, which turns
// the field invalid - the control takes the .error class and the label
// the destructive color. The label also names the control for
// assistive technology unless it was named already. It styles as the
// `field` element (.invalid while it shows an error) with .title,
// .description and .error labels.
type Field struct {
	composite
	box     *Box
	content *Box
	control Widget
	title   *Label
	desc    *Label
	err     *Label
	orient  FieldOrientation
	wide    bool
	bps     breakpoints
	// built and shownHorizontal record the structure layout last
	// built.
	built, shownHorizontal bool
	// named reports that the field set the control's accessible name,
	// so a relabel keeps it in step.
	named bool
}

// NewField returns a vertical field labeled label around control.
func NewField(face render.Font, label string, control Widget) *Field {
	face = requireFace("widget.NewField", face)
	f := &Field{box: NewBox(Column, 12, 0), content: NewBox(Column, 6, 0), control: control}
	f.SetElement("field")
	f.title = NewLabel(face, fieldTextSize, label, 0)
	f.title.AddClass("title")
	f.title.SetWrap(true)
	f.desc = NewLabel(face, fieldTextSize, "", 0)
	f.desc.AddClass("description")
	f.desc.SetWrap(true)
	f.err = NewLabel(face, fieldTextSize, "", 0)
	f.err.AddClass(css.Error)
	f.err.SetWrap(true)
	f.bps.add(Breakpoint{Min: fieldResponsiveMin, Apply: func() { f.setWide(true) }, Unapply: func() { f.setWide(false) }})
	f.initComposite(f, f.box)
	f.fillWidth = true
	f.boxed = true
	if n := nodeOf(control); n != nil && n.accName == "" {
		n.accName, f.named = label, true
	}
	f.layout()
	return f
}

// SetLabel replaces the label.
func (f *Field) SetLabel(s string) {
	f.title.SetText(s)
	if f.named {
		nodeOf(f.control).accName = s
	}
}

// Label returns the label.
func (f *Field) Label() string { return f.title.Text() }

// SetDescription sets the help text under the label; empty hides it.
func (f *Field) SetDescription(s string) {
	f.desc.SetText(s)
	f.layout()
}

// Description returns the help text.
func (f *Field) Description() string { return f.desc.Text() }

// SetError shows msg and marks the field invalid; empty clears both.
func (f *Field) SetError(msg string) {
	f.err.SetText(msg)
	invalid := msg != ""
	if invalid {
		f.AddClass("invalid")
	} else {
		f.RemoveClass("invalid")
	}
	if c, ok := f.control.(interface {
		AddClass(...string)
		RemoveClass(...string)
	}); ok {
		if invalid {
			c.AddClass(css.Error)
		} else {
			c.RemoveClass(css.Error)
		}
	}
	f.layout()
}

// Error returns the error message, empty while the field is valid.
func (f *Field) Error() string { return f.err.Text() }

// Control returns the control.
func (f *Field) Control() Widget { return f.control }

// SetOrientation switches the layout.
func (f *Field) SetOrientation(o FieldOrientation) {
	f.orient = o
	f.layout()
}

// Orientation returns the layout.
func (f *Field) Orientation() FieldOrientation { return f.orient }

// setWide records a responsive field's width class.
func (f *Field) setWide(wide bool) {
	f.wide = wide
	if f.orient == FieldResponsive {
		f.layout()
	}
}

// horizontal reports the layout the orientation asks for now.
func (f *Field) horizontal() bool {
	return f.orient == FieldHorizontal || f.orient == FieldResponsive && f.wide
}

// layout arranges the parts for the orientation, showing the
// description and error only while they have text. The control is
// moved only when the orientation changes, so a field gaining an
// error keeps a focused control focused.
func (f *Field) layout() {
	if h := f.horizontal(); !f.built || h != f.shownHorizontal {
		f.restructure(h)
	}
	text := f.box
	if f.shownHorizontal {
		text = f.content
	}
	text.Remove(f.desc)
	text.Remove(f.err)
	if f.desc.Text() != "" {
		text.Append(f.desc, false)
	}
	if f.err.Text() != "" {
		text.Append(f.err, false)
	}
	f.InvalidateLayout()
}

// restructure lays the label and the control out vertically or side
// by side.
func (f *Field) restructure(horizontal bool) {
	f.built, f.shownHorizontal = true, horizontal
	f.box.Clear()
	f.content.Clear()
	if horizontal {
		f.box.SetAxis(Row)
		f.content.Append(f.title, false)
		f.box.Append(f.content, true)
		f.box.AppendAligned(f.control, false, AlignCenter)
		return
	}
	f.box.SetAxis(Column)
	f.box.Append(f.title, false)
	f.box.Append(f.control, false)
}

// Arrange evaluates the responsive breakpoint against the width, then
// lays the parts out.
func (f *Field) Arrange(r render.Rect) {
	f.bps.evaluate(r.W)
	f.composite.Arrange(r)
}

// FieldSet groups related fields under a legend: the legend, an
// optional description, and the fields. It styles as the `fieldset`
// element with a .legend label.
type FieldSet struct {
	composite
	box    *Box
	desc   *Label
	fields *Box
}

// NewFieldSet returns an empty set under legend.
func NewFieldSet(face render.Font, legend string) *FieldSet {
	face = requireFace("widget.NewFieldSet", face)
	s := &FieldSet{box: NewBox(Column, 8, 0), fields: NewFieldGroup()}
	s.SetElement("fieldset")
	title := NewLabel(face, 16, legend, 0)
	title.AddClass("legend")
	s.desc = NewLabel(face, fieldTextSize, "", 0)
	s.desc.AddClass("description")
	s.desc.SetWrap(true)
	s.fields.SetPadding(render.Insets{Top: 16})
	s.box.Append(title, false).Append(s.fields, false)
	s.initComposite(s, s.box)
	s.fillWidth = true
	s.boxed = true
	return s
}

// SetDescription sets the text under the legend; empty hides it.
func (s *FieldSet) SetDescription(text string) {
	s.desc.SetText(text)
	s.box.Remove(s.desc)
	if text != "" {
		s.box.InsertAt(1, s.desc, false)
	}
	s.InvalidateLayout()
}

// Add appends a field (or any widget) to the set.
func (s *FieldSet) Add(w Widget) { s.fields.Append(w, false) }

// NewFieldGroup returns the column fields stack in, styled as the
// `fieldgroup` element.
func NewFieldGroup() *Box {
	g := NewBox(Column, 28, 0)
	g.SetElement("fieldgroup")
	return g
}
