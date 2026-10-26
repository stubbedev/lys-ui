package widget

import "fmt"

// Form validates a column of fields: each field
// added with a validator - a closure that reads its control's value
// and returns the error message, or "" while it is valid. A field
// shows its error once the focus leaves it, and Submit validates every
// field, moves the focus to the first invalid one, and runs OnSubmit
// only when all are valid. Enter in a text entry of the form submits.
// It styles as the `form` element.
type Form struct {
	composite
	group  *Box
	fields []formField

	// OnSubmit runs when Submit finds every field valid.
	OnSubmit func()
}

// formField is a field and its validator.
type formField struct {
	field    *Field
	validate func() string
}

// NewForm returns an empty form.
func NewForm() *Form {
	f := &Form{group: NewFieldGroup()}
	f.SetElement("form")
	f.initComposite(f, f.group)
	f.fillWidth = true
	f.boxed = true
	return f
}

// Add appends field, checked by validate; nil checks nothing.
func (f *Form) Add(field *Field, validate func() string) {
	f.fields = append(f.fields, formField{field: field, validate: validate})
	f.group.Append(field, false)
}

// Append appends w, not a field: a fieldset's legend, the actions row.
func (f *Form) Append(w Widget) { f.group.Append(w, false) }

// check validates one field and shows the result, reporting validity.
func (ff formField) check() bool {
	msg := ""
	if ff.validate != nil {
		msg = ff.validate()
	}
	ff.field.SetError(msg)
	return msg == ""
}

// Validate checks every field, shows each one's result, and moves the
// focus to the first invalid field whose control can take it. It
// reports whether all are valid.
func (f *Form) Validate() bool {
	valid := true
	var first Widget
	for _, ff := range f.fields {
		if ff.check() {
			continue
		}
		valid = false
		if c := ff.field.Control(); first == nil && isFocusable(c) {
			first = c
		}
	}
	if first != nil {
		if err := GrabFocus(first); err != nil {
			panic(fmt.Sprintf("widget: Form.Validate: %v", err))
		}
	}
	return valid
}

// Submit runs OnSubmit when every field is valid.
func (f *Form) Submit() {
	if f.Validate() && f.OnSubmit != nil {
		f.OnSubmit()
	}
}

// FocusMoved implements FocusObserver: the field the focus left shows
// its validation.
func (f *Form) FocusMoved(from, to Widget) {
	for _, ff := range f.fields {
		if inSubtree(from, ff.field) && !inSubtree(to, ff.field) {
			ff.check()
		}
	}
}

// InterceptKey implements KeyInterceptor: Enter in a text entry of the
// form submits it.
func (f *Form) InterceptKey(target Widget, a KeyAction, _ Mods) bool {
	if _, ok := target.(*Entry); !ok || a != KeyEnter {
		return false
	}
	f.Submit()
	return true
}
