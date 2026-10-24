package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// The check styles as `checkbutton > check`: its box layers paint the
// indicator, its min sizes size the checkbox, and :checked and
// :indeterminate reach it through the checkbox's state.
func TestCheckButtonCheckNode(t *testing.T) {
	loadCSS(t, `checkbutton check { min-width: 24; min-height: 26; border-width: 3; border-style: solid; border-radius: 5; background-color: #010203; }`)
	c := NewCheckButton(false)
	arrangeTree(t, c, 100, 40)
	if got := c.Measure(Constraints{Max: Size{W: 100, H: 40}}); got.W != 24 || got.H != 26 {
		t.Errorf("checkbox %v, want the check's 24x26", got)
	}
	kv := c.check.style(&c.check)
	if got := kv.Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("check background %v, want the `checkbutton check` rule", got)
	}
	if kv.Radius.TopLeft != 5 {
		t.Errorf("check radius %d, want 5", kv.Radius.TopLeft)
	}
	if got := kv.EffBorder().Top; got != 3 {
		t.Errorf("check border %d, want 3", got)
	}

	loadCSS(t, `checkbutton check { background-color: #010203; } checkbutton:checked check { background-color: #0a0b0c; } checkbutton:indeterminate check { background-color: #0d0e0f; }`)
	c = NewCheckButton(false)
	arrangeTree(t, c, 100, 40)
	if got := c.check.style(&c.check).Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("resting check background %v, want the unchecked rule", got)
	}
	c.SetChecked(true)
	if got := c.check.style(&c.check).Background; got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("checked check background %v, want the :checked rule", got)
	}
	c.SetChecked(false)
	c.SetInconsistent(true)
	if got := c.check.style(&c.check).Background; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("inconsistent check background %v, want the :indeterminate rule", got)
	}
	if !c.Inconsistent() || c.Inconsistent() == c.Checked() && c.Checked() {
		t.Errorf("inconsistent %v, checked %v", c.Inconsistent(), c.Checked())
	}
}

// The mark's ink is the check's color, the way a GTK theme sets
// `-gtk-icon-source` and color together.
func TestCheckButtonCheckMarkColor(t *testing.T) {
	loadCSS(t, `checkbutton check { color: #112233; }`)
	c := NewCheckButton(true)
	arrangeTree(t, c, 100, 40)
	if got := pickc(0, c.check.style(&c.check), style.PropColor, 0); got != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("mark color %v, want the check's color rule", got)
	}
}

// A child sits after the check inside the checkbox's box, styled as
// `checkbutton > label`, and a click on it toggles the checkbox; the
// check keeps its size, centered across. Without a child the check
// fills the content box, and SetChild(nil) returns to that.
func TestCheckButtonChild(t *testing.T) {
	loadCSS(t, `checkbutton { padding: 3px; }
		checkbutton check { min-width: 16px; min-height: 16px; }
		checkbutton label { margin-left: 6px; color: #123456; }`)
	c := NewCheckButton(false)
	l := NewLabel(testFace(t), 24, "Allow", 0)
	c.SetChild(l)
	host := NewBox(Row, 0, 0)
	host.Append(c, false)
	frame(t, host, 300, 60)

	b, lb := c.Bounds(), l.Bounds()
	if lb.X != b.X+3+16+6 || lb.X+lb.W != b.X+b.W-3 {
		t.Errorf("label at %v in checkbox %v, want after the 16px check and its 6px margin", lb, b)
	}
	if c.Child() != Widget(l) || l.Parent() != Widget(c) {
		t.Errorf("child %v, label parent %v", c.Child(), l.Parent())
	}
	if want := (render.Rect{X: b.X + 3, Y: b.Y + 3 + (b.H-6-16)/2, W: 16, H: 16}); c.indicator != want || b.H <= 22 {
		t.Errorf("check at %v in %v, want %v (the label sets the height)", c.indicator, b, want)
	}
	if got := pickc(0, l.style(l), style.PropColor, 0); got != render.RGB(0x12, 0x34, 0x56) {
		t.Errorf("label color %v, want the checkbutton label rule", got)
	}
	p := Point{X: lb.X + lb.W/2, Y: lb.Y + lb.H/2}
	if hit := c.HitTest(p); hit != Widget(c) {
		t.Fatalf("hit on the label = %v, want the checkbox", hit)
	}
	c.ClickAt(p)
	if !c.Checked() {
		t.Error("a click on the label did not toggle")
	}

	c.SetChild(nil)
	frame(t, host, 300, 60)
	b = c.Bounds()
	nat := c.Measure(Constraints{Max: Size{W: 300, H: 60}})
	if l.Parent() != nil || nat != (Size{W: 22, H: 22}) || c.indicator != (render.Rect{X: b.X + 3, Y: b.Y + 3, W: b.W - 6, H: b.H - 6}) {
		t.Errorf("childless: label parent %v, natural %v, check %v in %v", l.Parent(), nat, c.indicator, b)
	}
}

// The theme spaces the child 8px after the check, shadcn's gap-2, and
// the first measure already sees `checkbutton > check` rules: the check
// is linked when the checkbox is built, not when it is arranged.
func TestCheckButtonGapAndFirstMeasure(t *testing.T) {
	c := NewCheckButton(false)
	l := NewLabel(testFace(t), 14, "Accept", 0)
	c.SetChild(l)
	host := NewBox(Row, 0, 0)
	host.Append(c, false)
	frame(t, host, 300, 60)
	if gap := l.Bounds().X - (c.indicator.X + c.indicator.W); gap != 8 {
		t.Errorf("label %d px after the check, want 8", gap)
	}

	loadCSS(t, `checkbutton > check { min-width: 30px; min-height: 30px; }`)
	if got := NewCheckButton(false).Measure(Constraints{Max: Size{W: 100, H: 100}}); got.W != 30 {
		t.Errorf("first measure %v, want the 30px check rule", got)
	}
}
