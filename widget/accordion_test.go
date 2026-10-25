package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/anim"
)

// newTestAccordion builds a three-item accordion in mode, arranged so
// its triggers can take focus.
func newTestAccordion(t *testing.T, mode AccordionMode) (*Accordion, []*Button) {
	t.Helper()
	t.Cleanup(anim.SetInstant(true))
	face := goldenFace(t)
	a := NewAccordion(mode)
	var inner []*Button
	for _, title := range []string{"One", "Two", "Three"} {
		b := NewButton(NewLabel(face, 14, "Inside "+title, 0), 4, 4)
		inner = append(inner, b)
		a.Append(NewAccordionItem(face, title, b))
	}
	arrangeTree(t, a, 300, 400)
	return a, inner
}

func openStates(a *Accordion) []bool {
	out := make([]bool, len(a.Items()))
	for i, it := range a.Items() {
		out[i] = it.Open()
	}
	return out
}

// A single-mode accordion keeps at most one item open; until it is
// collapsible the open item stays open.
func TestAccordionSingle(t *testing.T) {
	a, _ := newTestAccordion(t, AccordionSingle)
	items := a.Items()
	var toggles []bool
	items[0].OnToggled = func(open bool) { toggles = append(toggles, open) }
	items[0].SetOpen(true)
	items[1].SetOpen(true)
	if got := openStates(a); got[0] || !got[1] || got[2] {
		t.Fatalf("open = %v, want only the second", got)
	}
	if len(toggles) != 2 || !toggles[0] || toggles[1] {
		t.Errorf("the first item's toggles = %v, want open then closed", toggles)
	}
	items[1].trigger.OnClick()
	if !items[1].Open() {
		t.Error("a non-collapsible accordion closed its open item")
	}
	a.SetCollapsible(true)
	items[1].trigger.OnClick()
	if got := openStates(a); got[0] || got[1] || got[2] {
		t.Errorf("open = %v, want none after collapsing", got)
	}
}

// A multiple-mode accordion opens and closes each item on its own, and
// an open item carries the checked state its chevron turns on.
func TestAccordionMultiple(t *testing.T) {
	a, _ := newTestAccordion(t, AccordionMultiple)
	items := a.Items()
	items[0].trigger.OnClick()
	items[2].trigger.OnClick()
	if got := openStates(a); !got[0] || got[1] || !got[2] {
		t.Fatalf("open = %v, want the first and third", got)
	}
	if !items[0].HasState(StateChecked) || items[1].HasState(StateChecked) {
		t.Error("the checked state does not follow the open state")
	}
	items[0].trigger.OnClick()
	if items[0].Open() {
		t.Error("a multiple-mode item did not close")
	}
}

// Up, Down, Home and End move focus between the triggers, wrapping;
// Tab skips a closed item's content and reaches an open one's.
func TestAccordionKeyboard(t *testing.T) {
	a, inner := newTestAccordion(t, AccordionMultiple)
	items := a.Items()
	r := &Router{Root: a}
	r.SetFocus(items[0].trigger)
	for _, step := range []struct {
		key  KeyAction
		want int
	}{{KeyDown, 1}, {KeyDown, 2}, {KeyDown, 0}, {KeyUp, 2}, {KeyHome, 0}, {KeyEnd, 2}} {
		r.KeyAction(step.key, 0)
		if r.Focused() != Widget(items[step.want].trigger) {
			t.Fatalf("after %v focus is on %T, want trigger %d", step.key, r.Focused(), step.want)
		}
	}
	r.SetFocus(items[0].trigger)
	r.FocusNext()
	if r.Focused() != Widget(items[1].trigger) {
		t.Fatal("Tab from a closed item's trigger did not skip its content")
	}
	items[0].SetOpen(true)
	arrangeTree(t, a, 300, 400)
	r.SetFocus(items[0].trigger)
	r.FocusNext()
	if r.Focused() != Widget(inner[0]) {
		t.Error("Tab from an open item's trigger did not reach its content")
	}
}

// TestGoldenAccordion pins the accordion with its middle item
// open, in both palettes.
func TestGoldenAccordion(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(anim.SetInstant(true))
			SetTheme(tc.th)
			defer SetTheme(nil)
			a := NewAccordion(AccordionSingle)
			a.SetCollapsible(true)
			for _, s := range [][2]string{
				{"Is it accessible?", "Yes. It adheres to the WAI-ARIA design pattern."},
				{"Is it styled?", "Yes. It comes with default styles that match the other components' aesthetic."},
				{"Is it animated?", "Yes. It's animated by default, but you can disable it if you prefer."},
			} {
				body := NewLabel(face, 14, s[1], 0)
				body.SetWrap(true)
				a.Append(NewAccordionItem(face, s[0], body))
			}
			a.Items()[1].SetOpen(true)
			NewGolden(t, a, "accordion-"+tc.name, goldenTheme(tc.th), goldenFrame(400, 200))
		})
	}
}

// Items opened before they are appended keep the single-mode rule: the
// last open one appended stays open.
func TestAccordionAppendOpen(t *testing.T) {
	face := goldenFace(t)
	t.Cleanup(anim.SetInstant(true))
	a := NewAccordion(AccordionSingle)
	for range 2 {
		it := NewAccordionItem(face, "x", NewLabel(face, 14, "y", 0))
		it.SetOpen(true)
		a.Append(it)
	}
	if got := openStates(a); got[0] || !got[1] {
		t.Errorf("open = %v, want only the last appended", got)
	}
}
