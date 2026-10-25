package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget/css"
)

// A field shows its description and error only while it has them, an
// error marks the field and its control invalid, and the label names
// the control.
func TestField(t *testing.T) {
	face := goldenFace(t)
	e := NewEntry(face, 14, 0)
	f := NewField(face, "Email", e)
	if got := len(f.box.Children()); got != 2 {
		t.Fatalf("a bare field shows %d parts, want the label and the control", got)
	}
	f.SetDescription("We never share it.")
	f.SetError("Enter a valid email.")
	if got := len(f.box.Children()); got != 4 {
		t.Fatalf("shows %d parts, want label, control, description and error", got)
	}
	if !e.HasClass(css.Error) || !f.HasClass("invalid") {
		t.Error("an error did not mark the field and control invalid")
	}
	f.SetError("")
	if e.HasClass(css.Error) || f.HasClass("invalid") || len(f.box.Children()) != 3 {
		t.Error("clearing the error kept the invalid state or the message")
	}
	if Describe(e).Name != "Email" {
		t.Errorf("control named %q, want the label", Describe(e).Name)
	}
	f.SetLabel("Work email")
	if Describe(e).Name != "Work email" {
		t.Error("relabeling did not rename the control")
	}
}

// A responsive field is horizontal from fieldResponsiveMin wide.
func TestFieldResponsive(t *testing.T) {
	face := goldenFace(t)
	f := NewField(face, "Notifications", NewSwitch(true))
	f.SetOrientation(FieldResponsive)
	arrangeTree(t, f, fieldResponsiveMin-1, 100)
	if f.box.Axis() != Column {
		t.Error("a narrow responsive field is not vertical")
	}
	arrangeTree(t, f, fieldResponsiveMin, 100)
	if f.box.Axis() != Row {
		t.Error("a wide responsive field is not horizontal")
	}
}

// TestGoldenField pins a field set with a valid, an invalid and
// a horizontal field, in both palettes.
func TestGoldenField(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			set := NewFieldSet(face, "Profile")
			set.SetDescription("This is how others see you.")
			name := NewField(face, "Name", NewEntry(face, 14, 0))
			name.SetDescription("Your public display name.")
			set.Add(name)
			email := NewField(face, "Email", NewEntry(face, 14, 0))
			email.SetError("Enter a valid email address.")
			set.Add(email)
			news := NewField(face, "Newsletter", NewSwitch(true))
			news.SetDescription("Monthly product updates.")
			news.SetOrientation(FieldHorizontal)
			set.Add(news)
			NewGolden(t, set, "field-"+tc.name, goldenTheme(tc.th), goldenFrame(400, 380))
		})
	}
}

// An error appearing on a focused field keeps its control focused.
func TestFieldErrorKeepsFocus(t *testing.T) {
	face := goldenFace(t)
	e := NewEntry(face, 14, 0)
	f := NewField(face, "Email", e)
	arrangeTree(t, f, 300, 200)
	r := &Router{Root: f}
	r.SetFocus(e)
	f.SetError("Enter a valid email.")
	f.SetDescription("Work address.")
	if r.Focused() != Widget(e) || parentOf(e) == nil {
		t.Error("the control lost its focus or its parent when the field changed")
	}
}
