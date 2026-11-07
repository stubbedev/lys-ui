package widget

import (
	"strings"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/xkb"

	"github.com/stubbedev/lys-ui/widget/css"
)

// shortcutRecorderSize is the recorder's text size.
const shortcutRecorderSize = 14

// ShortcutRecorder is a control that records a key combination (GNOME
// Settings' "Set Shortcut", KDE's KKeySequenceWidget). At rest it
// shows the accelerator as keycaps, or "Unassigned", beside a clear
// button. Activated (a click, Enter or Space) it records: it takes the
// window's raw key stream as a KeyRecorder, previews held modifiers
// live, and commits the first non-modifier key with its modifiers.
// Escape cancels and keeps the old value, Backspace alone clears it,
// and losing focus cancels. A Validate error shows under the control
// while it keeps recording. It styles as the `shortcutrecorder`
// element, the button .recording while it records, the message an
// .error label.
type ShortcutRecorder struct {
	composite
	face      render.Font
	box       *Box
	row       *Box
	button    *recorderButton
	display   *Box
	clear     *Button
	err       *Label
	value     string
	recording bool
	held      Mods

	// OnChanged fires when a press, Backspace or the clear button
	// changed the accelerator, with the new one in the label form
	// ("Ctrl+Alt+P"), empty once cleared. SetAccel does not fire it.
	OnChanged func(accel string)
	// Validate, when set, judges a recorded accelerator before it
	// commits: an error is shown under the control and recording goes
	// on.
	Validate func(accel string) error
}

// recorderButton is the recorder's focusable face: the widget that
// holds focus, so the one the window hands its key stream to.
type recorderButton struct {
	*Button
	r *ShortcutRecorder
}

// NewShortcutRecorder returns a recorder at rest showing accel, in
// the label form ("Ctrl+Alt+P"); empty shows it unassigned.
func NewShortcutRecorder(face render.Font, accel string) *ShortcutRecorder {
	face = requireFace("widget.NewShortcutRecorder", face)
	r := &ShortcutRecorder{face: face, box: NewBox(Column, 4, 0), row: NewBox(Row, 6, 0), display: NewBox(Row, 0, 0), value: accel}
	r.SetElement("shortcutrecorder")
	r.button = &recorderButton{Button: NewButton(r.display, 6, 4), r: r}
	r.button.OnClick = r.StartRecording
	r.button.SetAccessibleName(Tr("Shortcut"))
	r.button.SetOnFocusChanged(func(focused bool) {
		if !focused && r.recording {
			r.stop()
		}
	})
	r.clear = NewButton(NewThemeIcon("edit-clear-symbolic", 16), 6, 4)
	r.clear.SetAccessibleName(Tr("Clear shortcut"))
	r.clear.OnClick = func() {
		r.stop()
		r.commit("")
	}
	r.err = NewLabel(face, shortcutRecorderSize, "", 0)
	r.err.AddClass(css.Error)
	r.err.SetWrap(true)
	r.row.Append(r.button, true)
	r.box.Append(r.row, false)
	r.initComposite(r, r.box)
	r.refresh()
	return r
}

// Accel returns the accelerator in the label form, empty while
// unassigned.
func (r *ShortcutRecorder) Accel() string { return r.value }

// SetAccel shows accel, ending any recording, without firing
// OnChanged: the caller's own value, not the user's edit.
func (r *ShortcutRecorder) SetAccel(accel string) {
	r.value = accel
	r.stop()
}

// SetAccessibleName names the control for assistive technology; the
// name lands on the focusable button.
func (r *ShortcutRecorder) SetAccessibleName(name string) { r.button.SetAccessibleName(name) }

// SetError shows msg under the control and marks it invalid; empty
// clears both. A caller reports a refusal that arrives after the
// commit (a compositor denying the key) through it.
func (r *ShortcutRecorder) SetError(msg string) {
	r.err.SetText(msg)
	r.button.SetClass(css.Error, msg != "")
	r.box.Remove(r.err)
	if msg != "" {
		r.box.Append(r.err, false)
	}
	MarkA11yChanged()
	r.InvalidateLayout()
}

// Error returns the message shown under the control, empty while
// none is.
func (r *ShortcutRecorder) Error() string { return r.err.Text() }

// Recording reports whether the recorder is taking key presses.
func (r *ShortcutRecorder) Recording() bool { return r.recording }

// StartRecording enters recording. A disabled recorder stays at rest.
func (r *ShortcutRecorder) StartRecording() {
	if r.recording || !IsEnabled(r.button) {
		return
	}
	r.recording, r.held = true, 0
	r.refresh()
}

// RecordPress implements KeyRecorder: a modifier key joins the live
// preview, Escape alone cancels, Backspace alone clears, and any other
// key with its modifiers is validated and committed.
func (r *ShortcutRecorder) RecordPress(sym xkb.Keysym, mods Mods) {
	if !r.recording {
		return
	}
	if bit, ok := modifierOf(sym); ok {
		r.held = mods | bit
		r.refresh()
		return
	}
	switch {
	case sym == xkb.KeyEscape && mods == 0:
		r.stop()
		return
	case sym == xkb.KeyBackSpace && mods == 0:
		r.stop()
		r.commit("")
		return
	}
	accel := AccelLabel(sym, mods)
	if r.Validate != nil {
		if err := r.Validate(accel); err != nil {
			r.SetError(err.Error())
			return
		}
	}
	r.stop()
	r.commit(accel)
}

// RecordRelease implements KeyRecorder: a released modifier leaves the
// live preview, and with none held the recorder prompts again.
func (r *ShortcutRecorder) RecordRelease(sym xkb.Keysym) {
	bit, ok := modifierOf(sym)
	if !r.recording || !ok {
		return
	}
	r.held &^= bit
	r.refresh()
}

// stop leaves recording and clears the inline message.
func (r *ShortcutRecorder) stop() {
	r.recording, r.held = false, 0
	r.SetError("")
	r.refresh()
}

// commit makes accel the value, firing OnChanged when it differs.
func (r *ShortcutRecorder) commit(accel string) {
	if accel == r.value {
		return
	}
	r.value = accel
	r.refresh()
	if r.OnChanged != nil {
		r.OnChanged(accel)
	}
}

// refresh rebuilds the button's content and the clear button for the
// state: keycaps of the value or the held modifiers, or a dim prompt.
func (r *ShortcutRecorder) refresh() {
	r.display.Clear()
	switch {
	case r.recording && r.held != 0:
		r.display.AppendAligned(NewShortcutLabel(r.face, strings.Join(modifierNames(r.held), "+")), false, AlignCenter)
	case r.recording:
		r.display.AppendAligned(r.dimLabel(Tr("Press a shortcut…")), false, AlignCenter)
	case r.value != "":
		r.display.AppendAligned(NewShortcutLabel(r.face, r.value), false, AlignCenter)
	default:
		r.display.AppendAligned(r.dimLabel(Tr("Unassigned")), false, AlignCenter)
	}
	r.button.SetClass("recording", r.recording)
	r.row.Remove(r.clear)
	if r.value != "" && !r.recording {
		r.row.AppendAligned(r.clear, false, AlignCenter)
	}
	MarkA11yChanged()
	r.InvalidateLayout()
}

// dimLabel is a secondary-emphasis prompt.
func (r *ShortcutRecorder) dimLabel(text string) *Label {
	l := NewLabel(r.face, shortcutRecorderSize, text, 0)
	l.AddClass(css.DimLabel)
	return l
}

// description is what assistive technology reads after the name: the
// recording state, else the current value.
func (r *ShortcutRecorder) description() string {
	switch {
	case r.recording:
		return Tr("Recording, press a shortcut")
	case r.value == "":
		return Tr("Unassigned")
	}
	return r.value
}

// Recording implements KeyRecorder for the focused button.
func (b *recorderButton) Recording() bool { return b.r.Recording() }

// RecordPress implements KeyRecorder for the focused button.
func (b *recorderButton) RecordPress(sym xkb.Keysym, mods Mods) { b.r.RecordPress(sym, mods) }

// RecordRelease implements KeyRecorder for the focused button.
func (b *recorderButton) RecordRelease(sym xkb.Keysym) { b.r.RecordRelease(sym) }

// HitTest returns the button itself, so a click focuses the
// recorder's face rather than the embedded Button.
func (b *recorderButton) HitTest(p Point) Widget { return b.HitLeaf(b, p) }

// a11yDescription is the recorder's description on its button.
func (b *recorderButton) a11yDescription() string { return b.r.description() }
