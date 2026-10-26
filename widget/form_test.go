package widget

import (
	"slices"
	"testing"
)

// focusHost is a SurfaceHost that focuses through a router.
type focusHost struct {
	fakeSurfaceHost
	r *Router
}

func (h *focusHost) Focus(w Widget) error { return h.r.SetFocus(w) }

// newTestForm builds a form of two required entries and a third,
// unchecked one, hosted and arranged.
func newTestForm(t *testing.T) (*Form, *Router, []*Entry) {
	t.Helper()
	face := goldenFace(t)
	form := NewForm()
	var entries []*Entry
	for i, name := range []string{"Name", "Email", "Notes"} {
		e := NewEntry(face, 14, 0)
		entries = append(entries, e)
		var validate func() string
		if i < 2 {
			validate = func() string {
				if e.Text() == "" {
					return name + " is required."
				}
				return ""
			}
		}
		form.Add(NewField(face, name, e), validate)
	}
	r := &Router{Root: form}
	SetSurfaceHost(form, &focusHost{r: r})
	arrangeTree(t, form, 400, 400)
	return form, r, entries
}

// A field shows its error once the focus leaves it, not before.
func TestFormValidatesOnBlur(t *testing.T) {
	form, r, entries := newTestForm(t)
	r.SetFocus(entries[0])
	if form.fields[0].field.Error() != "" {
		t.Fatal("a field shows an error before the focus left it")
	}
	r.SetFocus(entries[2])
	if form.fields[0].field.Error() != "Name is required." || form.fields[1].field.Error() != "" {
		t.Errorf("errors %q %q, want only the left field's", form.fields[0].field.Error(), form.fields[1].field.Error())
	}
}

// Submit shows every error, focuses the first invalid field, and runs
// OnSubmit only once all are valid; Enter in an entry submits.
func TestFormSubmit(t *testing.T) {
	form, r, entries := newTestForm(t)
	var submits int
	form.OnSubmit = func() { submits++ }
	entries[0].SetText("Ada")
	r.SetFocus(entries[2])
	form.Submit()
	if submits != 0 || r.Focused() != Widget(entries[1]) {
		t.Fatalf("submits %d, focus on %T; want none and the invalid email", submits, r.Focused())
	}
	if got := []string{form.fields[0].field.Error(), form.fields[1].field.Error()}; !slices.Equal(got, []string{"", "Email is required."}) {
		t.Errorf("errors %q", got)
	}
	entries[1].SetText("ada@example.com")
	r.KeyAction(KeyEnter, 0)
	if submits != 1 || form.fields[1].field.Error() != "" {
		t.Errorf("Enter: submits %d, email error %q; want one submit and no error", submits, form.fields[1].field.Error())
	}
}

// Validation focuses the first invalid field that can take focus: a
// disabled invalid one is passed over, not left holding nothing.
func TestFormValidateSkipsUnfocusableFields(t *testing.T) {
	form, r, entries := newTestForm(t)
	entries[0].SetEnabled(false)
	r.SetFocus(entries[2])
	if form.Validate() {
		t.Fatal("two empty required fields validated")
	}
	if r.Focused() != Widget(entries[1]) {
		t.Errorf("focus on %T, want the invalid email past the disabled name", r.Focused())
	}
}
