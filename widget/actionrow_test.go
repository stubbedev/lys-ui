package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"
)

// rowFace builds the test font.
func rowFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestActionRowActivation pins the row contract: activatable rows
// fire OnActivate on whole-row clicks and Enter/Space, inactivatable
// ones never do, and the selected/pressed states stick.
func TestActionRowActivation(t *testing.T) {
	face := rowFace(t)
	r := NewActionRow(face, 14, "Notifications", "When they arrive")
	activated := 0
	r.OnActivate = func() { activated++ }
	r.Measure(Constraints{Max: Size{W: 320, H: 200}})
	r.Arrange(render.Rect{X: 0, Y: 0, W: 320, H: 40})

	r.ClickAt(Point{X: 3, Y: 2})
	r.KeyAction(KeyEnter, 0)
	if activated != 0 {
		t.Errorf("an inactivatable row activated %d times", activated)
	}

	r.SetActivatable(true)
	if !r.Activatable() {
		t.Error("SetActivatable did not stick")
	}
	r.ClickAt(Point{X: 3, Y: 2})
	r.KeyAction(KeyEnter, 0)
	r.KeyAction(KeySpace, 0)
	r.KeyAction(KeyHome, 0)
	if activated != 3 {
		t.Errorf("activatable row fired %d times, want 3", activated)
	}

	r.SetSelected(true)
	if !r.Selected() {
		t.Error("selection did not stick")
	}
	r.SetPressed(true)
	r.SetPressed(false)
}

// TestActionRowPacksAndText pins the vocabulary: title and subtitle
// setters, the packs, and the chevron.
func TestActionRowPacksAndText(t *testing.T) {
	face := rowFace(t)
	th := DarkTheme()
	r := NewActionRow(face, 14, "Title", "Sub")
	r.SetTitle("Changed")
	r.SetSubtitle("Also changed")
	if r.Title() != "Changed" {
		t.Errorf("title = %q", r.Title())
	}
	r.PackStart(NewLabel(face, 14, "s", th.Text))
	r.PackEnd(NewLabel(face, 14, "e", th.Text))
	r.ShowChevron(true)
	r.ShowChevron(true)
	if got := len(r.start.Children()) + len(r.end.Children()); got != 3 {
		t.Errorf("packs hold %d children, want 3 (incl. chevron)", got)
	}
	r.ShowChevron(false)
	if got := len(r.end.Children()); got != 1 {
		t.Errorf("chevron toggle left %d end children", got)
	}
}

// TestActionRowRTLMirroring pins the parity: the packs swap sides
// under an RTL row.
func TestActionRowRTLMirroring(t *testing.T) {
	face := rowFace(t)
	th := DarkTheme()
	r := NewActionRow(face, 14, "row", "")
	r.PackStart(NewLabel(face, 14, "start", th.Text))
	r.PackEnd(NewLabel(face, 14, "end", th.Text))
	r.Measure(Constraints{Max: Size{W: 320, H: 100}})
	r.Arrange(render.Rect{X: 0, Y: 0, W: 320, H: 40})
	start := r.start.Bounds()
	end := r.end.Bounds()
	if start.X >= end.X {
		t.Fatalf("LTR row packs not ordered: start=%v end=%v", start, end)
	}
	r.SetDirection(DirectionRTL)
	r.InvalidateLayout()
	r.Measure(Constraints{Max: Size{W: 320, H: 100}})
	r.Arrange(render.Rect{X: 0, Y: 0, W: 320, H: 40})
	if s, e := r.start.Bounds(), r.end.Bounds(); s.X <= e.X {
		t.Errorf("RTL row packs did not mirror: start=%v end=%v", s, e)
	}
}

// TestControlRows pin each control row's wiring: switch flips fire
// the hook both ways, spin values commit silently when set
// programmatically, combo selections carry through, and the entry
// row's text rides its field.
func TestControlRows(t *testing.T) {
	face := rowFace(t)

	flips := 0
	sw := NewSwitchRow(face, 14, "Dark mode", "", func(bool) { flips++ })
	if sw.On() {
		t.Error("switch starts on")
	}
	sw.sw.SetOn(true)
	if !sw.On() || flips != 1 {
		t.Errorf("flip: on=%v fired=%d", sw.On(), flips)
	}

	values := 0
	spin := NewSpinRow(face, 14, "Volume", "", 0, 100, 1, 0, func(float64) { values++ })
	spin.SetValue(50)
	if spin.Value() != 50 || values != 0 {
		t.Errorf("programmatic set: value=%v fired=%d", spin.Value(), values)
	}

	choices := 0
	combo := NewComboRow(face, 14, "Style", "", []string{"A", "B"}, 0, func(int) { choices++ })
	combo.SetSelected(1)
	if combo.Selected() != 1 || choices != 1 {
		t.Errorf("selection: %d fired=%d", combo.Selected(), choices)
	}

	entry := NewEntryRow(face, 14, "Name", "Ada")
	if entry.Text() != "Ada" {
		t.Errorf("entry row text = %q", entry.Text())
	}
	entry.SetText("Grace")
	if entry.entry.Text() != "Grace" {
		t.Error("SetText did not reach the field")
	}

	clicks := 0
	btn := NewButtonRow(face, 14, "Reset", func() { clicks++ })
	btn.ClickAt(Point{X: 1, Y: 1})
	if clicks != 1 {
		t.Errorf("button row fired %d times", clicks)
	}
}

// TestExpanderRow pins the disclosure: collapsed by default, Expand
// toggles it, sub-rows ride along, and activation expands too.
func TestExpanderRow(t *testing.T) {
	face := rowFace(t)
	th := DarkTheme()
	exp := NewExpanderRow(face, 14, "Advanced", "")
	exp.Add(NewActionRow(face, 14, "Sub", ""))
	exp.Add(NewLabel(face, 13, "tail", th.Text))
	if exp.Expanded() {
		t.Error("expander starts expanded")
	}
	exp.Expand()
	if !exp.Expanded() {
		t.Error("Expand did not disclose")
	}
	exp.OnActivate()
	if exp.Expanded() {
		t.Error("activation did not collapse")
	}
}

// TestPreferencesGroupAndPage pin the containers: groups hold their
// rows with the header slots, pages stack groups.
func TestPreferencesGroupAndPage(t *testing.T) {
	face := rowFace(t)
	g := NewPreferencesGroup(face, 14, "General")
	g.SetSuffix("3 of 5")
	g.Add(NewActionRow(face, 14, "One", ""))
	g.Add(NewActionRow(face, 14, "Two", ""))
	if got := len(g.rows.Children()); got != 2 {
		t.Fatalf("group holds %d rows", got)
	}
	g.Measure(Constraints{Max: Size{W: 360, H: 400}})
	g.Arrange(render.Rect{X: 0, Y: 0, W: 360, H: 120})

	p := NewPreferencesPage()
	p.Add(g)
	p.Add(NewPreferencesGroup(face, 14, "More"))
	if got := len(p.column.Children()); got != 2 {
		t.Errorf("page holds %d groups", got)
	}
	p.Measure(Constraints{Max: Size{W: 500, H: 400}})
	p.Arrange(render.Rect{X: 0, Y: 0, W: 500, H: 400})
}

// TestGoldenRows pins the painted row vocabulary: a plain row, a
// switch row, an expanded expander, and a group wrapping both.
func TestGoldenRows(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	plain := NewActionRow(face, 14, "Notifications", "When they arrive")
	plain.SetActivatable(true)
	plain.ShowChevron(true)
	prev, callers := Current(), LightTheme()
	SetTheme(callers)
	NewGolden(t, plain, "actionrow", goldenTheme(th), goldenFrame(320, 48))
	if Current() != callers {
		t.Fatal("NewGolden left its shot's theme installed; the caller's must be back on return")
	}
	SetTheme(prev)

	group := NewPreferencesGroup(face, 14, "General")
	sw := NewSwitchRow(face, 14, "Dark style", "Follow the night", nil)
	sw.Set(true)
	group.Add(sw)
	exp := NewExpanderRow(face, 14, "Advanced", "")
	exp.Add(NewActionRow(face, 14, "Inner", ""))
	exp.Expand()
	group.Add(exp)
	NewGolden(t, group, "preferences-group", goldenTheme(th), goldenFrame(320, 240))
}

// Tab and the accessibility tree reach the widgets a composite is
// built from: a switch packed into an action row.
func TestCompositeContentTakesFocus(t *testing.T) {
	face := goldenFace(t)
	row := NewActionRow(face, 14, "Wi-Fi", "")
	sw := NewSwitch(false)
	row.PackEnd(sw)
	arrangeTree(t, row, 300, 60)
	r := &Router{Root: row}
	r.SetFocus(row)
	r.FocusNext()
	if r.Focused() != Widget(sw) {
		t.Errorf("Tab from the row focused %T, want its switch", r.Focused())
	}
	found := false
	for _, st := range DescribeTree(row) {
		found = found || st.Role == RoleSwitch
	}
	if !found {
		t.Error("the accessibility tree misses the row's switch")
	}
}
