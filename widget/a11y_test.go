package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// a11yRoot lays out a widget inside a container so Bounds is filled.
func a11yRoot(t *testing.T, w Widget) Widget {
	t.Helper()
	root := NewBox(Column, 0, 0).Append(w, false)
	root.Measure(Constraints{Max: Size{W: 400, H: 200}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 200})
	return root
}

func TestRoles(t *testing.T) {
	face := entryFace(t)
	for _, c := range []struct {
		name string
		w    Widget
		want Role
	}{
		{"button", NewButton(NewLabel(face, 12, "go", render.RGB(0, 0, 0)), 4, 4), RoleButton},
		{"label", NewLabel(face, 12, "go", render.RGB(0, 0, 0)), RoleLabel},
		{"entry", NewEntry(face, 14, render.RGB(0, 0, 0)), RoleEntry},
		{"text area", NewTextArea(face, 13, render.RGB(0, 0, 0)), RoleTextArea},
		{"slider", NewSlider(0, 1, 0.1, 0), RoleSlider},
		{"switch", NewSwitch(false), RoleSwitch},
		{"checkbox", NewCheckButton(false), RoleCheckBox},
		{"progress", NewProgressBar(0.5), RoleProgressBar},
		{"scroll", NewScroll(NewLabel(face, 12, "row", render.RGB(0, 0, 0))), RoleScrollArea},
		{"menu", NewMenu(face, 12, MenuItem{Label: "act"}), RoleMenu},
		{"notebook", NewNotebook(face), RoleTabList},
	} {
		if got := RoleOf(c.w); got != c.want {
			t.Errorf("%s: role = %v, want %v", c.name, got, c.want)
		}
		if s := c.want.String(); s == "" || s == "none" {
			t.Errorf("%s: role name %q is useless", c.name, s)
		}
	}
	if RoleOf(NewBox(Row, 0, 0)) != RoleNone {
		t.Error("plain box classified as a role")
	}
	if RoleOf(nil) != RoleNone {
		t.Error("nil classified as a role")
	}
}

func TestDescribeValueAndState(t *testing.T) {
	face := entryFace(t)

	t.Run("slider carries range, value, and step", func(t *testing.T) {
		s := NewSlider(0, 10, 0.5, 3)
		st := Describe(s)
		if st.Min != 0 || st.Max != 10 || st.Value != 3 || st.Step != 0.5 {
			t.Errorf("slider state = %+v", st)
		}
		if !st.Focusable {
			t.Error("slider not marked focusable")
		}
	})

	t.Run("switch and checkbox carry checked state", func(t *testing.T) {
		if !Describe(NewSwitch(true)).Checked {
			t.Error("switch on not reported checked")
		}
		if Describe(NewSwitch(false)).Checked {
			t.Error("switch off reported checked")
		}
		if !Describe(NewCheckButton(true)).Checked {
			t.Error("checkbox checked not reported")
		}
	})

	t.Run("progress carries the fill fraction", func(t *testing.T) {
		st := Describe(NewProgressBar(0.4))
		if st.Min != 0 || st.Max != 1 || st.Value != 0.4 {
			t.Errorf("progress state = %+v", st)
		}
	})

	t.Run("entry carries text, caret, and selection", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(0, 0, 0))
		e.SetText("hello world")
		e.SelectAll()
		st := Describe(e)
		if st.Text != "hello world" {
			t.Errorf("text = %q", st.Text)
		}
		if !st.Editable || st.Multiline {
			t.Errorf("editable = %v, multiline = %v", st.Editable, st.Multiline)
		}
		if !st.HasSelection || st.SelStart != 0 || st.SelEnd != len("hello world") {
			t.Errorf("selection = %d:%d active=%v", st.SelStart, st.SelEnd, st.HasSelection)
		}
		// Collapsing the selection lands the caret at the edge the
		// motion points at; rightward means the selection end.
		e.MoveCursor(1)
		st = Describe(e)
		if st.HasSelection || st.Caret != len("hello world") {
			t.Errorf("after collapse: caret = %d, active selection = %v", st.Caret, st.HasSelection)
		}
	})

	t.Run("text area flattens caret and selection to rune offsets", func(t *testing.T) {
		a := NewTextArea(face, 13, render.RGB(0, 0, 0))
		a.SetText("ab\ncd")
		a.SetCursor(1, 1)
		st := Describe(a)
		// "ab\ncd": line 1 starts at rune offset 3.
		if st.Caret != 4 {
			t.Errorf("caret = %d, want 4", st.Caret)
		}
		if !st.Multiline {
			t.Error("text area not marked multiline")
		}
		a.SetCursor(0, 0)
		a.KeyAction(KeyRight, ModShift) // extends the selection across "ab\nc"
		a.KeyAction(KeyRight, ModShift)
		a.KeyAction(KeyRight, ModShift)
		st = Describe(a)
		if !st.HasSelection || st.SelStart != 0 || st.SelEnd != 3 {
			t.Errorf("selection = %d:%d active=%v", st.SelStart, st.SelEnd, st.HasSelection)
		}
	})

	t.Run("names come from tooltips then the wrapped label", func(t *testing.T) {
		btn := NewButton(NewBox(Row, 4, 0).Append(
			NewLabel(face, 12, "save file", render.RGB(0, 0, 0)), false), 8, 4)
		if got := Describe(btn).Name; got != "save file" {
			t.Errorf("button name = %q, want the wrapped label text", got)
		}
		btn.SetTooltip("saves")
		if got := Describe(btn).Name; got != "saves" {
			t.Errorf("button name = %q, want the tooltip", got)
		}
		lbl := NewLabel(face, 12, "a caption", render.RGB(0, 0, 0))
		if got := Describe(lbl).Name; got != "a caption" {
			t.Errorf("label name = %q, want the text", got)
		}
		// A type embedding a Button keeps the button's name, and its
		// tooltip still wins.
		type toggle struct{ *Button }
		tg := toggle{NewButton(NewBox(Row, 0, 0).Append(
			NewBox(Row, 0, 0), false).Append(
			NewLabel(face, 12, "12:30", render.RGB(0, 0, 0)), false), 0, 0)}
		if got := Describe(tg).Name; got != "12:30" {
			t.Errorf("embedded button name = %q, want the nested label text", got)
		}
		tg.SetTooltip("clock")
		if got := Describe(tg).Name; got != "clock" {
			t.Errorf("embedded button name = %q, want the tooltip", got)
		}
		// A container names nothing by itself.
		if got := Describe(NewBox(Row, 0, 0).Append(lbl, false)).Name; got != "" {
			t.Errorf("box name = %q, want none", got)
		}
	})

	t.Run("focusable mirrors the tab traversal set", func(t *testing.T) {
		face2 := entryFace(t)
		for _, c := range []struct {
			name string
			w    Widget
			want bool
		}{
			{"button", NewButton(NewSpacer(4, 4), 2, 2), true},
			{"slider", NewSlider(0, 1, 0, 0), true},
			{"switch", NewSwitch(false), true},
			{"checkbox", NewCheckButton(false), true},
			{"entry", NewEntry(face2, 14, render.RGB(0, 0, 0)), true},
			{"text area", NewTextArea(face2, 13, render.RGB(0, 0, 0)), true},
			{"label", NewLabel(face2, 12, "x", render.RGB(0, 0, 0)), false},
			{"progress", NewProgressBar(0), false},
			{"scroll", NewScroll(NewSpacer(4, 4)), false},
		} {
			if got := Describe(c.w).Focusable; got != c.want {
				t.Errorf("%s: focusable = %v, want %v", c.name, got, c.want)
			}
		}
	})

	t.Run("bounds are the arranged rect", func(t *testing.T) {
		s := NewSwitch(false)
		a11yRoot(t, s)
		if got, want := Describe(s).Bounds, s.Bounds(); got != want {
			t.Errorf("bounds = %v, want %v", got, want)
		}
	})

	t.Run("describe nil is the zero state", func(t *testing.T) {
		if got := Describe(nil); got != (A11yState{}) {
			t.Errorf("Describe(nil) = %+v", got)
		}
	})
}

// Keyboard-first guarantee: every activatable widget responds to Enter
// and Space the same way a pointer click would, and the traversal set
// (KeyActionHandler) matches the interactive controls.
func TestKeyboardActivation(t *testing.T) {
	t.Run("enter and space click a focused button", func(t *testing.T) {
		clicks := 0
		b := NewButton(NewSpacer(8, 8), 2, 2)
		b.OnClick = func() { clicks++ }
		b.KeyAction(KeyEnter, 0)
		b.InsertRune(' ')
		if clicks != 2 {
			t.Errorf("clicks = %d, want 2 (Enter + Space)", clicks)
		}
		b.InsertRune('x')
		if clicks != 2 {
			t.Errorf("typing a rune clicked the button (%d)", clicks)
		}
	})

	t.Run("enter and space toggle a focused switch", func(t *testing.T) {
		s := NewSwitch(false)
		s.KeyAction(KeyEnter, 0)
		if !s.On() {
			t.Error("enter did not toggle the switch on")
		}
		s.InsertRune(' ')
		if s.On() {
			t.Error("space did not toggle the switch off")
		}
	})

	t.Run("enter and space toggle a focused checkbox", func(t *testing.T) {
		c := NewCheckButton(false)
		c.KeyAction(KeyEnter, 0)
		if !c.Checked() {
			t.Error("enter did not check the checkbox")
		}
		c.InsertRune(' ')
		if c.Checked() {
			t.Error("space did not uncheck the checkbox")
		}
	})

	t.Run("the router routes enter to the focused control", func(t *testing.T) {
		clicks := 0
		b := NewButton(NewLabel(entryFace(t), 12, "go", render.RGB(0, 0, 0)), 4, 4)
		b.OnClick = func() { clicks++ }
		r := &Router{Root: a11yRoot(t, b)}
		r.FocusNext()
		if r.Focused() != Widget(b) {
			t.Fatalf("focus = %v, want the button", r.Focused())
		}
		r.KeyAction(KeyEnter, 0)
		r.Type(' ')
		if clicks != 2 {
			t.Errorf("clicks = %d, want 2 through the router", clicks)
		}
	})
}

func TestDescribeTree(t *testing.T) {
	face := entryFace(t)
	btn := NewButton(NewLabel(face, 12, "go", render.RGB(0, 0, 0)), 4, 4)
	sw := NewSwitch(true)
	lbl := NewLabel(face, 12, "hint", render.RGB(0, 0, 0))
	root := NewBox(Column, 2, 0)
	root.Append(btn, false)
	root.Append(NewBox(Row, 2, 0).Append(lbl, false).Append(sw, false), false)
	a11yRoot(t, root)

	states := DescribeTree(root)
	var roles []Role
	for _, st := range states {
		if st.Role != RoleNone {
			roles = append(roles, st.Role)
		}
	}
	// The button lists its label as its child, ahead of the hint.
	want := []Role{RoleButton, RoleLabel, RoleLabel, RoleSwitch}
	if len(roles) != len(want) {
		t.Fatalf("roles = %v, want %v", roles, want)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("roles = %v, want %v", roles, want)
		}
	}
}

// A markup tooltip keeps its markup for the card and its plain text
// as the accessible name; unparsable markup is plain text.
func TestTooltipMarkup(t *testing.T) {
	l := NewLabel(testFace(t), 13, "x", render.RGB(0, 0, 0))
	l.SetTooltipMarkup("<b>Save</b> the file")
	if m, ok := l.TooltipMarkup(); !ok || m != "<b>Save</b> the file" || l.TooltipText() != "Save the file" {
		t.Errorf("markup %q %v, text %q", m, ok, l.TooltipText())
	}
	if Describe(l).Name != "Save the file" {
		t.Errorf("accessible name = %q", Describe(l).Name)
	}
	l.SetTooltip("plain")
	if _, ok := l.TooltipMarkup(); ok || l.TooltipText() != "plain" {
		t.Error("SetTooltip did not clear the markup")
	}
	l.SetTooltipMarkup("<b>broken")
	if _, ok := l.TooltipMarkup(); ok || l.TooltipText() != "<b>broken" {
		t.Error("unparsable markup was not kept as plain text")
	}
}
