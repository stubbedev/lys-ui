package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// An alert shows its description only while it has one, carries the
// destructive class for that variant, and announces itself by title.
func TestAlert(t *testing.T) {
	a := NewAlert(goldenFace(t), "Heads up!", "")
	if len(a.text.Children()) != 1 {
		t.Fatal("an empty description is shown")
	}
	a.SetDescription("Details")
	a.SetDescription("More details")
	if len(a.text.Children()) != 2 || a.Description() != "More details" {
		t.Fatal("the description is not shown once")
	}
	a.SetDescription("")
	if len(a.text.Children()) != 1 {
		t.Fatal("clearing the description kept it")
	}
	a.SetVariant(AlertDestructive)
	if !a.HasClass(css.Destructive) {
		t.Error("the destructive variant lacks its class")
	}
	a.SetVariant(AlertDefault)
	if a.HasClass(css.Destructive) {
		t.Error("the default variant kept the destructive class")
	}
	if st := Describe(a); st.Role != RoleAlert || st.Name != "Heads up!" {
		t.Errorf("a11y = %v %q, want alert named by its title", st.Role, st.Name)
	}
}

// A class added to the widget the app holds styles it: the element
// sits on the composite, which lays its content inside its padding.
func TestAlertTakesAppClasses(t *testing.T) {
	LoadStylesheet("alert.custom { background-color: #ff0000; }")
	defer LoadStylesheet("")
	a := NewAlert(goldenFace(t), "Heads up!", "")
	a.AddClass("custom")
	arrangeTree(t, a, 300, 100)
	if v := a.style(a); v.Background != render.RGB(0xff, 0, 0) {
		t.Errorf("alert.custom background %v, want red", v.Background)
	}
	if got, b := a.row.Bounds(), a.Bounds(); got.X != b.X+16+1 || got.Y != b.Y+12+1 {
		t.Errorf("content at %v in %v, want inside the 1px border and 12px 16px padding", got, b)
	}
}
