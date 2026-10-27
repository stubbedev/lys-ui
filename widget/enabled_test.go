package widget

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// enabledRoot arranges w in a column box so parents, hit tests, and
// traversal all see it, and returns the root.
func enabledRoot(t *testing.T, w Widget) *Box {
	t.Helper()
	root := NewBox(Column, 0, 0).Append(w, false)
	SizeWithin(root, Size{W: 400, H: 200})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 200})
	return root
}

func TestDisabledButtonSwallowsInput(t *testing.T) {
	newButton := func() (*Button, *Router, Point, *int) {
		clicks := 0
		b := NewButton(NewLabel(entryFace(t), 12, "go", render.RGB(255, 255, 255)), 8, 4)
		b.OnClick = func() { clicks++ }
		r := &Router{Root: enabledRoot(t, b)}
		bb := b.Bounds()
		return b, r, Point{X: bb.X + 5, Y: bb.Y + 5}, &clicks
	}

	t.Run("clicks stop firing", func(t *testing.T) {
		b, r, p, clicks := newButton()
		b.SetEnabled(false)
		r.Move(p)
		r.Press(BTNLeft, p, 0)
		r.Release(BTNLeft, p)
		if *clicks != 0 {
			t.Fatalf("clicks = %d, want 0", *clicks)
		}
		if b.Hovered || b.Pressed {
			t.Errorf("hovered = %v, pressed = %v, want both false", b.Hovered, b.Pressed)
		}
	})

	t.Run("a press on a disabled widget leaves focus alone", func(t *testing.T) {
		b, r, p, _ := newButton()
		b.SetEnabled(false)
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		root := b.Parent()
		box, ok := root.(*Box)
		if !ok {
			t.Fatalf("root = %T, want the arranging box", root)
		}
		box.Append(e, false)
		SizeWithin(box, Size{W: 400, H: 200})
		box.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 200})
		ep := Point{X: e.Bounds().X + 5, Y: e.Bounds().Y + 5}
		r.Press(BTNLeft, ep, 0)
		r.Release(BTNLeft, ep)
		if r.Focused() != Widget(e) {
			t.Fatalf("focus = %v, want the entry", r.Focused())
		}
		r.Press(BTNLeft, p, 0)
		r.Release(BTNLeft, p)
		if r.Focused() != Widget(e) {
			t.Error("pressing a disabled button stole focus")
		}
	})

	t.Run("a disable between press and release kills the click", func(t *testing.T) {
		b, r, p, clicks := newButton()
		r.Move(p)
		r.Press(BTNLeft, p, 0)
		b.SetEnabled(false)
		r.Release(BTNLeft, p)
		if *clicks != 0 {
			t.Errorf("clicks = %d, want 0", *clicks)
		}
	})

	t.Run("keys and activation runes stop firing", func(t *testing.T) {
		b, _, p, clicks := newButton()
		b.SetEnabled(false)
		b.KeyAction(KeyEnter, 0)
		b.InsertRune(' ')
		b.ClickAt(p)
		if *clicks != 0 {
			t.Errorf("clicks = %d, want 0", *clicks)
		}
	})

	t.Run("disabled does not mean chrome", func(t *testing.T) {
		b, _, _, _ := newButton()
		b.SetEnabled(false)
		if !IsInteractive(b) {
			t.Error("a disabled button stopped classifying as interactive")
		}
	})

	t.Run("re-enabling restores the click", func(t *testing.T) {
		b, r, p, clicks := newButton()
		b.SetEnabled(false)
		b.SetEnabled(true)
		r.Move(p)
		r.Press(BTNLeft, p, 0)
		r.Release(BTNLeft, p)
		if *clicks != 1 {
			t.Errorf("clicks = %d, want 1 after re-enable", *clicks)
		}
	})
}

func TestTraversalSkipsDisabled(t *testing.T) {
	newTargets := func() (*focusTarget, *focusTarget, *focusTarget, *Router) {
		a, b, c := newFocusTarget(), newFocusTarget(), newFocusTarget()
		root := NewBox(Column, 0, 0).Append(a, false).Append(b, false).Append(c, false)
		SizeWithin(root, Size{W: 100, H: 100})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
		return a, b, c, &Router{Root: root}
	}

	t.Run("tab never lands on a disabled widget", func(t *testing.T) {
		a, b, c, r := newTargets()
		b.SetEnabled(false)
		r.FocusNext()
		if r.Focused() != Widget(a) {
			t.Fatalf("first focus = %v, want a", r.Focused())
		}
		r.FocusNext()
		if got := r.Focused(); got != Widget(c) {
			t.Errorf("second focus = %v, want c (b skipped)", got)
		}
		r.FocusNext()
		if r.Focused() != Widget(a) {
			t.Errorf("wrap landed on %v, want a", r.Focused())
		}
	})

	t.Run("disabling the focused widget drops focus to the next focusable", func(t *testing.T) {
		a, b, _, r := newTargets()
		r.FocusNext()
		if r.Focused() != Widget(a) {
			t.Fatalf("focus = %v, want a", r.Focused())
		}
		a.SetEnabled(false)
		// The next key delivery moves focus instead of feeding keys
		// into the widget that just went dead.
		r.KeyAction(KeyEnter, 0)
		if got := r.Focused(); got != Widget(b) {
			t.Errorf("focus = %v, want b (the next focusable)", got)
		}
	})

	t.Run("disabling every focusable drops focus to none", func(t *testing.T) {
		a, b, c, r := newTargets()
		r.FocusNext()
		r.FocusNext()
		r.FocusNext()
		if r.Focused() != Widget(c) {
			t.Fatalf("focus = %v, want c", r.Focused())
		}
		a.SetEnabled(false)
		b.SetEnabled(false)
		c.SetEnabled(false)
		r.Type('x')
		if r.Focused() != nil {
			t.Errorf("focus = %v, want nil with nothing left focusable", r.Focused())
		}
	})
}

func TestBoxDisablePropagatesToSubtree(t *testing.T) {
	inner := NewBox(Column, 0, 0)
	btn := NewButton(NewSpacer(8, 8), 2, 2)
	inner.Append(btn, false)
	guard := NewSwitch(false) // disabled by the app itself, inside the subtree
	inner.Append(guard, false)
	box := NewBox(Column, 0, 0).Append(inner, false)
	leaf := NewSwitch(false) // outside the disabled box
	box.Append(leaf, false)
	r := &Router{Root: enabledRoot(t, box)}

	box.SetEnabled(false)

	t.Run("descendants inherit by query, their own flags untouched", func(t *testing.T) {
		if box.Enabled() {
			t.Error("the box's own flag did not read disabled")
		}
		if !inner.Enabled() || !btn.Enabled() {
			t.Error("the walk rewrote the descendants' own flags; propagation must stay per-query")
		}
		if IsEnabled(inner) || IsEnabled(btn) || IsEnabled(guard) {
			t.Error("descendants did not inherit the disabled state")
		}
		if IsEnabled(leaf) {
			t.Error("a widget outside the box inherited the state")
		}
	})

	t.Run("input and traversal honor the inherited state", func(t *testing.T) {
		p := Point{X: btn.Bounds().X + 4, Y: btn.Bounds().Y + 4}
		r.Move(p)
		r.Press(BTNLeft, p, 0)
		r.Release(BTNLeft, p)
		if r.Hovered() != nil {
			t.Errorf("hover = %v, want nil over a disabled subtree", r.Hovered())
		}
		for range 10 {
			if w := r.Focused(); w == Widget(btn) || w == Widget(guard) {
				t.Fatalf("traversal landed on %v inside a disabled subtree", w)
			}
			r.FocusNext()
		}
	})

	t.Run("re-enabling restores the subtree but not the app's own disable", func(t *testing.T) {
		guard.SetEnabled(false)
		box.SetEnabled(false)
		box.SetEnabled(true)
		if !IsEnabled(btn) {
			t.Error("the button stayed disabled after the box came back")
		}
		if IsEnabled(guard) {
			t.Error("re-enabling the box resurrected a child the app disabled")
		}
	})
}

func TestDisabledTogglesSliderAndScroll(t *testing.T) {
	t.Run("switch ignores clicks and keys", func(t *testing.T) {
		sw := NewSwitch(false)
		enabledRoot(t, sw)
		sw.SetEnabled(false)
		sw.ClickAt(Point{})
		sw.KeyAction(KeyEnter, 0)
		sw.InsertRune(' ')
		if sw.On() {
			t.Error("a disabled switch toggled")
		}
	})

	t.Run("checkbox ignores clicks and keys", func(t *testing.T) {
		cb := NewCheckButton(false)
		enabledRoot(t, cb)
		cb.SetEnabled(false)
		cb.ClickAt(Point{})
		cb.KeyAction(KeyEnter, 0)
		cb.InsertRune(' ')
		if cb.Checked() {
			t.Error("a disabled checkbox toggled")
		}
	})

	t.Run("slider ignores drags and keys", func(t *testing.T) {
		s := NewSlider(0, 100, 0, 50)
		enabledRoot(t, s)
		s.SetEnabled(false)
		s.DragMove(Point{X: s.Bounds().X + 100, Y: 9})
		s.KeyAction(KeyRight, 0)
		if s.Value() != 50 {
			t.Errorf("value = %v, want 50", s.Value())
		}
	})

	t.Run("scroll ignores wheel, drags, and clicks", func(t *testing.T) {
		tall := NewBox(Column, 0, 0)
		for range 10 {
			tall.Append(newStub(50, 20), false)
		}
		sc := NewScroll(tall)
		root := NewBox(Column, 0, 0).Append(sc, false)
		SizeWithin(root, Size{W: 100, H: 100})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
		sc.SetEnabled(false)
		sc.ScrollBy(0, 2)
		if _, offY := sc.Offset(); offY != 0 {
			t.Fatalf("offset = %d, want 0", offY)
		}
		r := &Router{Root: root}
		b := sc.Bounds()
		r.Move(Point{X: b.X + 5, Y: b.Y + 5})
		r.Axis(0, 2, 0)
		if _, offY := sc.Offset(); offY != 0 {
			t.Errorf("axis scrolled a disabled scroll: offset = %d", offY)
		}
	})
}

func TestDisabledMenuIsInert(t *testing.T) {
	fired := 0
	m := NewMenu(entryFace(t), 12,
		MenuItem{Label: "one", OnClick: func() { fired++ }},
		MenuItem{Label: "two", OnClick: func() { fired++ }})
	enabledRoot(t, m)
	m.SetEnabled(false)

	p := Point{X: m.Bounds().X + 10, Y: m.Bounds().Y + 8}
	m.HoverMove(p)
	if m.hovered != -1 {
		t.Errorf("hovered = %d, want -1 on a disabled menu", m.hovered)
	}
	m.KeyAction(KeyDown, 0)
	m.KeyAction(KeyEnter, 0)
	m.ClickAt(p)
	if fired != 0 {
		t.Errorf("fired = %d, want 0", fired)
	}
}

func TestEntryReadOnly(t *testing.T) {
	face := entryFace(t)
	newEntry := func(t *testing.T) *Entry {
		t.Helper()
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.MaxWidth = 60
		e.SetText("secret-key-123")
		e.SetReadOnly(true)
		enabledRoot(t, e)
		return e
	}

	t.Run("mutations are blocked", func(t *testing.T) {
		e := newEntry(t)
		want := e.Text()
		e.Insert("X") // paste, drop
		e.InsertRune('x')
		e.KeyAction(KeyBackspace, 0)
		e.KeyAction(KeyDelete, 0)
		e.Backspace()
		e.Delete()
		e.IMECommit("x")
		if e.Text() != want {
			t.Errorf("text = %q, want %q", e.Text(), want)
		}
		if e.composing() {
			t.Error("a read-only field entered composing")
		}
	})

	t.Run("undo is blocked but the pre-existing stack waits", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.SetText("one")
		e.Insert(" two") // an edit to back out later
		if e.Text() != "one two" {
			t.Fatalf("text = %q", e.Text())
		}
		e.SetReadOnly(true)
		if e.Undo() {
			t.Error("undo ran while read-only")
		}
		if e.Text() != "one two" {
			t.Errorf("text = %q, want untouched one two", e.Text())
		}
		e.SetReadOnly(false)
		if !e.Undo() {
			t.Fatal("undo did not resume after read-only lifted")
		}
		if e.Text() != "one" {
			t.Errorf("text = %q, want the pre-readonly one", e.Text())
		}
	})

	t.Run("selection, copy, caret, and pan still work", func(t *testing.T) {
		e := newEntry(t)
		// A narrow row keeps the field at its capped 60px width, so the
		// text overflows and the pan matters.
		row := NewBox(Row, 0, 0).Append(e, false)
		SizeWithin(row, Size{W: 400, H: 40})
		row.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 40})
		e.KeyAction(KeyHome, 0)
		if e.Cursor() != 0 {
			t.Errorf("caret = %d, want 0 after Home", e.Cursor())
		}
		e.KeyAction(KeyRight, ModShift)
		e.KeyAction(KeyRight, ModShift)
		sel, ok := e.SelectedText()
		if !ok || sel != "se" {
			t.Errorf("copy = %q, %v, want \"se\", true", sel, ok)
		}
		e.KeyAction(KeyEnd, 0)
		if e.ScrollX() == 0 {
			t.Error("the pan never followed the caret in a read-only field")
		}
		e.SelectAll()
		if sel, _ = e.SelectedText(); sel != e.Text() {
			t.Errorf("select-all copied %q, want the whole text", sel)
		}
	})

	t.Run("setText still applies: it is programmatic, not a user edit", func(t *testing.T) {
		e := newEntry(t)
		e.SetText("resolved-value")
		if e.Text() != "resolved-value" {
			t.Errorf("text = %q, want resolved-value", e.Text())
		}
	})

	t.Run("ime never starts composing", func(t *testing.T) {
		e := newEntry(t)
		e.IMEPreedit("x", 1, 1)
		if e.composing() {
			t.Error("preedit showed on a read-only field")
		}
		e.IMEPreedit("x", 1, 1)
		e.SetReadOnly(true) // flips off mid-composing
		e.IMEPreedit("x", 1, 1)
		if e.composing() {
			t.Error("preedit survived the read-only flip")
		}
	})
}

func TestTextAreaReadOnly(t *testing.T) {
	face := entryFace(t)
	a := NewTextArea(face, 13, render.RGB(255, 255, 255))
	a.MaxWidth = 400
	a.SetText("alpha\nbeta")
	a.SetReadOnly(true)
	enabledRoot(t, a)

	t.Run("inserts are blocked", func(t *testing.T) {
		a.KeyAction(KeyEnter, 0)
		a.Insert("X")
		a.InsertRune('x')
		a.KeyAction(KeyBackspace, 0)
		a.KeyAction(KeyDelete, 0)
		if got := a.Text(); got != "alpha\nbeta" {
			t.Errorf("text = %q, want untouched alpha/beta", got)
		}
	})

	t.Run("tab declines the trap and moves focus instead", func(t *testing.T) {
		if a.TrapTab(false) {
			t.Error("a read-only area trapped tab")
		}
		if got := a.Text(); strings.Contains(got, "\t") {
			t.Errorf("text = %q, want no indent inserted", got)
		}
	})

	t.Run("selection, copy, and caret still work", func(t *testing.T) {
		a.SetCursor(0, 0)
		a.KeyAction(KeyDown, ModShift)
		sel, ok := a.SelectedText()
		if !ok || sel != "alpha\n" {
			t.Errorf("copy = %q, %v, want \"alpha\\n\", true", sel, ok)
		}
		a.KeyAction(KeyRight, 0) // collapse to the selection's end (1,0)
		a.KeyAction(KeyEnd, 0)
		if l, c := a.CursorPos(); l != 1 || c != len("beta") {
			t.Errorf("caret = %d:%d, want 1:%d", l, c, len("beta"))
		}
	})
}

func TestDisabledTextWidgetIgnoresKeys(t *testing.T) {
	face := entryFace(t)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	e.SetText("hold")
	e.SetEnabled(false)
	enabledRoot(t, e)
	e.Insert("X")
	e.InsertRune('x')
	e.KeyAction(KeyBackspace, 0)
	e.KeyAction(KeyLeft, 0) // even motion: a disabled widget ignores keys
	if e.Text() != "hold" || e.Cursor() != len("hold") {
		t.Errorf("text = %q cursor = %d, want hold at end", e.Text(), e.Cursor())
	}

	a := NewTextArea(face, 13, render.RGB(255, 255, 255))
	a.SetText("hold")
	a.SetEnabled(false)
	enabledRoot(t, a)
	a.Insert("X")
	a.KeyAction(KeyEnter, 0)
	if a.Text() != "hold" {
		t.Errorf("text = %q, want hold", a.Text())
	}
}

func TestDescribeEnabledAndReadOnly(t *testing.T) {
	face := entryFace(t)

	t.Run("an enabled editable entry", func(t *testing.T) {
		st := Describe(NewEntry(face, 14, render.RGB(255, 255, 255)))
		if !st.Enabled || !st.Editable || !st.Focusable {
			t.Errorf("enabled = %v editable = %v focusable = %v, want all true",
				st.Enabled, st.Editable, st.Focusable)
		}
	})

	t.Run("read-only reports editable false, enabled true", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.SetReadOnly(true)
		st := Describe(e)
		if !st.Enabled || st.Editable {
			t.Errorf("enabled = %v editable = %v, want true/false", st.Enabled, st.Editable)
		}
	})

	t.Run("disabled reports enabled, editable, focusable false", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		e.SetEnabled(false)
		st := Describe(e)
		if st.Enabled || st.Editable || st.Focusable {
			t.Errorf("enabled = %v editable = %v focusable = %v, want all false",
				st.Enabled, st.Editable, st.Focusable)
		}
	})

	t.Run("disabling a container disables the subtree for a11y", func(t *testing.T) {
		e := NewEntry(face, 14, render.RGB(255, 255, 255))
		enabledRoot(t, e)
		if st := Describe(e); !st.Enabled {
			t.Fatal("setup: an entry in an enabled box reports enabled")
		}
		e.Parent().(*Box).SetEnabled(false)
		if st := Describe(e); st.Enabled {
			t.Error("a widget inside a disabled box reported enabled")
		}
	})

	t.Run("a disabled button is not focusable", func(t *testing.T) {
		b := NewButton(NewSpacer(4, 4), 2, 2)
		if !Describe(b).Focusable {
			t.Fatal("setup: an enabled button must be focusable")
		}
		b.SetEnabled(false)
		if Describe(b).Focusable {
			t.Error("a disabled button reported focusable")
		}
	})
}

func TestDisabledPaintSmoke(t *testing.T) {
	defer SetTheme(DarkTheme())
	SetTheme(DarkTheme())

	// paintCanvas renders root and samples the pixel 2px inside the
	// top-left corner of bounds: radius-0 buttons fill that spot with
	// pure background, so the enabled and disabled fills must differ.
	paintBG := func(t *testing.T, root Widget, bounds render.Rect) render.Color {
		t.Helper()
		rects, any := CollectDamage(root)
		if !any {
			t.Fatal("setup: the initial paint owed no damage")
		}
		const width, height = 400, 200
		data := make([]byte, render.Stride(width)*height)
		cv := render.New(data, render.Stride(width), width, height)
		for _, r := range rects {
			prev := cv.PushClip(r)
			root.Paint(cv)
			cv.PopClip(prev)
		}
		o := (bounds.Y+2)*render.Stride(width) + (bounds.X+2)*4
		return render.Color(binary.LittleEndian.Uint32(data[o : o+4]))
	}

	b := NewButton(NewSpacer(8, 8), 8, 0)
	root := enabledRoot(t, b)
	enabledBG := paintBG(t, root, b.Bounds())
	b.SetEnabled(false)
	disabledBG := paintBG(t, root, b.Bounds())
	if enabledBG == disabledBG {
		t.Fatal("disabling a button did not change its paint")
	}
	if want := Current().DisabledSurface(); disabledBG != want {
		t.Errorf("disabled fill = %v, want the derived disabled surface %v", disabledBG, want)
	}

	// The remaining disabled and read-only visuals only need to paint
	// without imbalance: every PushAlpha/PushClip finds its pop.
	face := entryFace(t)
	entries := []*Entry{
		func() *Entry {
			e := NewEntry(face, 14, render.RGB(255, 255, 255))
			e.SetText("k")
			e.SetEnabled(false)
			return e
		}(),
		func() *Entry {
			e := NewEntry(face, 14, render.RGB(255, 255, 255))
			e.SetText("k")
			e.SetReadOnly(true)
			return e
		}(),
	}
	areas := []*TextArea{
		func() *TextArea {
			a := NewTextArea(face, 13, render.RGB(255, 255, 255))
			a.SetText("k")
			a.SetEnabled(false)
			return a
		}(),
		func() *TextArea {
			a := NewTextArea(face, 13, render.RGB(255, 255, 255))
			a.SetText("k")
			a.SetReadOnly(true)
			return a
		}(),
	}
	dd := NewDropdown(face, 12, []string{"a"}, 0)
	dd.SetEnabled(false)
	for _, w := range []Widget{
		func() *Switch { s := NewSwitch(true); s.SetEnabled(false); return s }(),
		func() *CheckButton { c := NewCheckButton(true); c.SetEnabled(false); return c }(),
		func() *Slider { s := NewSlider(0, 1, 0, 0.5); s.SetEnabled(false); return s }(),
		entries[0], entries[1], areas[0], areas[1], dd,
	} {
		r := enabledRoot(t, w)
		paintBG(t, r, r.Bounds())
	}
}
