package widget

import (
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

// EditableLabel is a label that edits in place (GTK EditableLabel):
// at rest it paints as plain text - the entry flat, read-only, caret
// hidden; a click or Enter starts editing with the text selected; Enter
// or losing focus commits, Escape reverts. It is the entry throughout,
// so editing inherits every Entry behavior (selection, undo, IME) and
// focus never has to move between two widgets.
type EditableLabel struct {
	*Entry
	editing bool
	before  string

	// OnChanged fires when an edit committed a different text.
	OnChanged func(text string)
}

// NewEditableLabel returns a resting label showing s.
func NewEditableLabel(face render.Font, sizePx float64, color render.Color, s string) *EditableLabel {
	l := &EditableLabel{Entry: NewEntry(face, sizePx, color)}
	l.SetElement("editablelabel")
	l.SetText(s)
	l.OnActivate = func(string) { l.StopEditing(true) }
	l.OnEscape = func() bool {
		if !l.editing {
			return false
		}
		l.StopEditing(false)
		return true
	}
	l.SetOnFocusChanged(func(focused bool) {
		if !focused {
			l.StopEditing(true)
		}
	})
	l.rest()
	return l
}

// Editing reports whether the label is in edit mode.
func (l *EditableLabel) Editing() bool { return l.editing }

// StartEditing enters edit mode with the whole text selected.
func (l *EditableLabel) StartEditing() {
	if l.editing || !l.Enabled() {
		return
	}
	l.editing = true
	l.before = l.Text()
	l.RemoveClass(css.Flat)
	l.caretHidden = false
	l.SetReadOnly(false)
	l.SelectAll()
}

// StopEditing leaves edit mode, keeping the edit when commit (firing
// OnChanged if the text differs) and restoring the old text otherwise.
func (l *EditableLabel) StopEditing(commit bool) {
	if !l.editing {
		return
	}
	l.editing = false
	if !commit {
		l.SetText(l.before)
	}
	l.rest()
	if commit && l.Text() != l.before && l.OnChanged != nil {
		l.OnChanged(l.Text())
	}
}

// rest applies the resting look.
func (l *EditableLabel) rest() {
	l.AddClass(css.Flat)
	l.caretHidden = true
	l.SetReadOnly(true)
	l.anchor = l.cursor
}

// ClickAt starts editing at rest; while editing it places the caret.
func (l *EditableLabel) ClickAt(p Point) {
	if !l.editing {
		l.StartEditing()
		return
	}
	l.Entry.ClickAt(p)
}

// KeyAction starts editing on Enter at rest; while editing every key
// is the entry's.
func (l *EditableLabel) KeyAction(a KeyAction, mods Mods) {
	if !l.editing {
		if a == KeyEnter {
			l.StartEditing()
		}
		return
	}
	l.Entry.KeyAction(a, mods)
}

// CursorName is the text cursor only while editing; at rest the
// label keeps the node-level shape like any other widget.
func (l *EditableLabel) CursorName() string {
	if l.editing {
		return l.Entry.CursorName()
	}
	return CursorNameOf(l)
}
