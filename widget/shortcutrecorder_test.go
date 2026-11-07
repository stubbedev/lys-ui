package widget

import (
	"errors"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/widget/css"
)

func recorderKeycaps(r *ShortcutRecorder) []string {
	var out []string
	walkTree(r.display, 0, func(w Widget, _ int) {
		if l, ok := w.(*Label); ok && l.hasClass(css.Keycap) {
			out = append(out, l.Text())
		}
	})
	return out
}

func recorderPrompt(r *ShortcutRecorder) string {
	var out string
	walkTree(r.display, 0, func(w Widget, _ int) {
		if l, ok := w.(*Label); ok && l.hasClass(css.DimLabel) {
			out = l.Text()
		}
	})
	return out
}

func TestShortcutRecorderCommitsTheFirstKeyWithItsModifiers(t *testing.T) {
	r := NewShortcutRecorder(chromeFace(t), "")
	var changes []string
	r.OnChanged = func(a string) { changes = append(changes, a) }
	if got := recorderPrompt(r); got != "Unassigned" {
		t.Fatalf("resting empty recorder shows %q, want Unassigned", got)
	}
	r.RecordPress(xkb.Keysym('p'), ModCtrl|ModAlt)
	if len(changes) != 0 {
		t.Fatalf("a press at rest committed %v", changes)
	}

	r.button.ClickAt(Point{})
	if !r.Recording() || recorderPrompt(r) != "Press a shortcut…" {
		t.Fatalf("a click did not start recording: recording=%v prompt=%q", r.Recording(), recorderPrompt(r))
	}
	r.RecordPress(xkb.KeyControlL, 0)
	r.RecordPress(xkb.KeyAltL, ModCtrl)
	r.RecordPress(xkb.Keysym('p'), ModCtrl|ModAlt)

	if !slices.Equal(changes, []string{"Ctrl+Alt+P"}) {
		t.Fatalf("OnChanged saw %v, want [Ctrl+Alt+P]", changes)
	}
	if r.Recording() || r.Accel() != "Ctrl+Alt+P" {
		t.Errorf("after the commit recording=%v accel=%q", r.Recording(), r.Accel())
	}
	if got := recorderKeycaps(r); !slices.Equal(got, []string{"Ctrl", "Alt", "P"}) {
		t.Errorf("keycaps %v, want [Ctrl Alt P]", got)
	}
	if !slices.Contains(r.row.Children(), Widget(r.clear)) {
		t.Error("an assigned recorder hides its clear button")
	}
}

func TestShortcutRecorderEscapeKeepsTheOldValue(t *testing.T) {
	r := NewShortcutRecorder(chromeFace(t), "Ctrl+Q")
	fired := 0
	r.OnChanged = func(string) { fired++ }
	r.button.KeyAction(KeyEnter, 0)
	if !r.Recording() {
		t.Fatal("Enter did not start recording")
	}
	r.RecordPress(xkb.KeyShiftL, 0)
	r.RecordPress(xkb.KeyEscape, 0)
	if r.Recording() || r.Accel() != "Ctrl+Q" || fired != 0 {
		t.Errorf("escape: recording=%v accel=%q OnChanged fired %d times", r.Recording(), r.Accel(), fired)
	}
	if got := recorderKeycaps(r); !slices.Equal(got, []string{"Ctrl", "Q"}) {
		t.Errorf("keycaps after cancel %v, want [Ctrl Q]", got)
	}
}

func TestShortcutRecorderBackspaceAndClearUnassign(t *testing.T) {
	r := NewShortcutRecorder(chromeFace(t), "Ctrl+Q")
	var changes []string
	r.OnChanged = func(a string) { changes = append(changes, a) }
	r.button.InsertRune(' ')
	if !r.Recording() {
		t.Fatal("Space did not start recording")
	}
	r.RecordPress(xkb.KeyBackSpace, 0)
	if r.Recording() || r.Accel() != "" || !slices.Equal(changes, []string{""}) {
		t.Fatalf("backspace: recording=%v accel=%q changes=%v", r.Recording(), r.Accel(), changes)
	}
	if slices.Contains(r.row.Children(), Widget(r.clear)) {
		t.Error("an unassigned recorder still offers its clear button")
	}

	r.SetAccel("Super+F9")
	if !slices.Equal(changes, []string{""}) {
		t.Errorf("SetAccel fired OnChanged: %v", changes)
	}
	r.clear.ClickAt(Point{})
	if r.Accel() != "" || !slices.Equal(changes, []string{"", ""}) {
		t.Errorf("clear button: accel=%q changes=%v", r.Accel(), changes)
	}

	r.StartRecording()
	r.RecordPress(xkb.KeyBackSpace, ModCtrl)
	if r.Accel() != "Ctrl+BackSpace" {
		t.Errorf("Ctrl+Backspace is a shortcut, not a clear: accel=%q", r.Accel())
	}
}

func TestShortcutRecorderPreviewsHeldModifiersWithoutCommitting(t *testing.T) {
	r := NewShortcutRecorder(chromeFace(t), "")
	fired := 0
	r.OnChanged = func(string) { fired++ }
	r.StartRecording()
	r.RecordPress(xkb.KeyControlL, 0)
	r.RecordPress(xkb.KeyShiftL, ModCtrl)
	if got := recorderKeycaps(r); !slices.Equal(got, []string{"Ctrl", "Shift"}) {
		t.Errorf("held Ctrl+Shift previews %v", got)
	}
	r.RecordRelease(xkb.KeyShiftL)
	if got := recorderKeycaps(r); !slices.Equal(got, []string{"Ctrl"}) {
		t.Errorf("after releasing Shift the preview is %v, want [Ctrl]", got)
	}
	r.RecordRelease(xkb.KeyControlL)
	if !r.Recording() || fired != 0 || r.Accel() != "" {
		t.Fatalf("modifier-only presses committed: recording=%v fired=%d accel=%q", r.Recording(), fired, r.Accel())
	}
	if got := recorderPrompt(r); got != "Press a shortcut…" {
		t.Errorf("with nothing held the recorder shows %q, want the prompt again", got)
	}
	if len(recorderKeycaps(r)) != 0 {
		t.Errorf("released modifiers still preview %v", recorderKeycaps(r))
	}
}

func TestShortcutRecorderValidateKeepsRecording(t *testing.T) {
	r := NewShortcutRecorder(chromeFace(t), "")
	var changes []string
	r.OnChanged = func(a string) { changes = append(changes, a) }
	r.Validate = func(a string) error {
		if a == "P" {
			return errors.New("a shortcut needs a modifier")
		}
		return nil
	}
	r.StartRecording()
	r.RecordPress(xkb.Keysym('p'), 0)
	if !r.Recording() || len(changes) != 0 {
		t.Fatalf("a refused press committed: recording=%v changes=%v", r.Recording(), changes)
	}
	if r.Error() != "a shortcut needs a modifier" || !slices.Contains(r.box.Children(), Widget(r.err)) {
		t.Errorf("the refusal is not shown: error=%q", r.Error())
	}
	r.RecordPress(xkb.Keysym('p'), ModSuper)
	if r.Recording() || !slices.Equal(changes, []string{"Super+P"}) {
		t.Fatalf("a valid press after a refusal: recording=%v changes=%v", r.Recording(), changes)
	}
	if r.Error() != "" || slices.Contains(r.box.Children(), Widget(r.err)) {
		t.Errorf("the refusal outlived the commit: %q", r.Error())
	}
}

func TestShortcutRecorderLosingFocusCancels(t *testing.T) {
	r := NewShortcutRecorder(chromeFace(t), "Ctrl+Q")
	other := NewButton(NewLabel(chromeFace(t), 14, "Other", 0), 0, 0)
	root := NewBox(Column, 0, 0).Append(r, false).Append(other, false)
	router := &Router{Root: root}
	if err := router.SetFocus(r.button); err != nil {
		t.Fatal(err)
	}
	r.StartRecording()
	if !r.Recording() || router.Focused() != Widget(r.button) {
		t.Fatalf("starting to record lost focus: recording=%v focus=%T", r.Recording(), router.Focused())
	}
	if err := router.SetFocus(other); err != nil {
		t.Fatal(err)
	}
	if r.Recording() || r.Accel() != "Ctrl+Q" {
		t.Errorf("focus left: recording=%v accel=%q", r.Recording(), r.Accel())
	}
}

func TestShortcutRecorderAccessibility(t *testing.T) {
	r := NewShortcutRecorder(chromeFace(t), "Ctrl+Alt+P")
	r.SetAccessibleName("Screenshot")
	st := Describe(r.button)
	if st.Role != RoleButton || st.Name != "Screenshot" || st.Description != "Ctrl+Alt+P" || !st.Focusable {
		t.Errorf("at rest: %+v", st)
	}
	gen := A11yGeneration()
	r.StartRecording()
	if A11yGeneration() == gen {
		t.Error("starting to record did not mark the accessibility tree changed")
	}
	if st := Describe(r.button); st.Description != "Recording, press a shortcut" {
		t.Errorf("recording description %q", st.Description)
	}
	r.RecordPress(xkb.KeyBackSpace, 0)
	if st := Describe(r.button); st.Description != "Unassigned" {
		t.Errorf("cleared description %q", st.Description)
	}
}

func TestAccelLabel(t *testing.T) {
	cases := []struct {
		sym  xkb.Keysym
		mods Mods
		want string
	}{
		{xkb.Keysym('p'), ModCtrl | ModAlt, "Ctrl+Alt+P"},
		{xkb.KeyF9, ModSuper, "Super+F9"},
		{xkb.KeyReturn, ModCtrl | ModShift, "Ctrl+Shift+Return"},
		{xkb.KeyEscape, 0, "Escape"},
	}
	for _, c := range cases {
		if got := AccelLabel(c.sym, c.mods); got != c.want {
			t.Errorf("AccelLabel(%v, %v) = %q, want %q", c.sym, c.mods, got, c.want)
		}
	}
	if IsModifierKeysym(xkb.Keysym('p')) || !IsModifierKeysym(xkb.KeySuperR) {
		t.Error("IsModifierKeysym misclassifies")
	}
}
