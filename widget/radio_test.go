package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestRadioCheckButtons pins the radio contract: checking one unchecks
// its peers (each firing its own OnChanged), a click on the checked
// one keeps it checked, and joining while checked releases the others.
func TestRadioCheckButtons(t *testing.T) {
	a, b, c := NewCheckButton(true), NewCheckButton(false), NewCheckButton(false)
	b.SetGroup(a)
	c.SetGroup(a)
	var changes []bool
	a.OnChanged = func(on bool) { changes = append(changes, on) }
	b.ClickAt(Point{})
	if a.Checked() || !b.Checked() || c.Checked() {
		t.Fatalf("after checking b: a=%v b=%v c=%v", a.Checked(), b.Checked(), c.Checked())
	}
	if len(changes) != 1 || changes[0] {
		t.Errorf("a's OnChanged = %v, want one uncheck", changes)
	}
	b.ClickAt(Point{})
	if !b.Checked() {
		t.Error("clicking the checked radio unchecked it")
	}
	d := NewCheckButton(true)
	d.SetGroup(a)
	if b.Checked() || !d.Checked() {
		t.Errorf("a checked joiner did not take over: b=%v d=%v", b.Checked(), d.Checked())
	}
}

// TestToggleButton pins the standalone and grouped toggle: a click
// flips a free toggle both ways; grouped, activation is exclusive and
// sticky; OnToggled fires on every flip.
func TestToggleButton(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	free := NewToggleButton(NewLabel(face, 14, "Bold", th.Text), 6, 4)
	var flips []bool
	free.OnToggled = func(on bool) { flips = append(flips, on) }
	free.ClickAt(Point{})
	free.ClickAt(Point{})
	if free.Active() || len(flips) != 2 || !flips[0] || flips[1] {
		t.Errorf("free toggle: active=%v flips=%v", free.Active(), flips)
	}

	left := NewToggleButton(NewLabel(face, 14, "L", th.Text), 6, 4)
	right := NewToggleButton(NewLabel(face, 14, "R", th.Text), 6, 4)
	right.SetGroup(left)
	left.ClickAt(Point{})
	right.ClickAt(Point{})
	if left.Active() || !right.Active() || !right.HasState(StateChecked) {
		t.Errorf("grouped: left=%v right=%v", left.Active(), right.Active())
	}
	right.ClickAt(Point{})
	if !right.Active() {
		t.Error("clicking the active grouped toggle released it")
	}
}

// TestGoldenRadio pins the round radio indicator, checked and not.
func TestGoldenRadio(t *testing.T) {
	th := DarkTheme()
	on, off := NewCheckButton(true), NewCheckButton(false)
	off.SetGroup(on)
	row := NewBox(Row, 8, 0)
	row.Append(on, false)
	row.Append(off, false)
	NewGolden(t, row, "radio", goldenTheme(th))
}

// A checked toggle's fill is the stylesheet's :checked rule when one
// exists (no programmatic color outranks it), else the theme's
// selected shade; an inactive one rests on the plain surface. It reads
// as a pressed toggle button to assistive technology.
func TestToggleButtonCheckedFillAndA11y(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	prev := Current()
	SetTheme(th)
	t.Cleanup(func() { SetTheme(prev) })
	paintAt := func(b *ToggleButton) render.Color {
		b.Measure(Constraints{Max: Size{W: 60, H: 30}})
		b.Arrange(render.Rect{W: 60, H: 30})
		stride := render.Stride(60)
		data := make([]byte, stride*30)
		cv := render.New(data, stride, 60, 30)
		b.Paint(cv)
		o := 15*stride + 2*4
		return render.RGB(data[o+2], data[o+1], data[o])
	}
	tg := NewToggleButton(NewLabel(face, 14, "", th.Text), 6, 0)
	if got := paintAt(tg); got != th.Surface {
		t.Errorf("inactive fill %#08x, want the surface %#08x", got, th.Surface)
	}
	tg.SetActive(true)
	if got := paintAt(tg); got != th.HoverSurface() {
		t.Errorf("active fill %#08x, want the selected shade %#08x", got, th.HoverSurface())
	}
	st := Describe(tg)
	if st.Role != RoleToggleButton || !st.Pressed {
		t.Errorf("a11y = %s pressed %v, want a pressed toggle button", st.Role, st.Pressed)
	}
	loadCSS(t, `button:checked { background-color: #ff0000; }`)
	if got := paintAt(tg); got != render.RGB(0xff, 0, 0) {
		t.Errorf("styled active fill %#08x, want the :checked rule's red", got)
	}
	tg.SetActive(false)
	if Describe(tg).Pressed {
		t.Error("an inactive toggle reads pressed")
	}
	if got := paintAt(tg); got == render.RGB(0xff, 0, 0) {
		t.Error("the :checked rule painted an inactive toggle")
	}
}
