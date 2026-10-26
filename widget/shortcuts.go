package widget

import (
	"strings"

	"github.com/stubbedev/lys-lib/render"
)

// ShortcutSection is one titled group of a shortcuts overview.
type ShortcutSection struct {
	Title string
	Items []ShortcutItem
}

// ShortcutItem is one shortcut: what it does, and its keys in the
// accelerator label form ("Ctrl+Shift+P", a chord "Ctrl+X Ctrl+S").
type ShortcutItem struct {
	Title string
	Keys  string
}

// NewShortcutLabel draws keys as keycaps (adw ShortcutLabel): each key
// of an accelerator its own .keycap label, the steps of a chord set
// apart.
func NewShortcutLabel(face render.Font, keys string) *Box {
	face = requireFace("widget.NewShortcutLabel", face)
	row := NewBox(Row, 10, 0)
	for step := range strings.FieldsSeq(keys) {
		row.AppendAligned(NewKbdGroup(face, strings.Split(step, "+")...), false, AlignCenter)
	}
	return row
}

// ShortcutsView is the shortcuts overview (GTK ShortcutsWindow, adw
// ShortcutsDialog's content): a scrolling page of boxed groups, one per
// section, each shortcut a row with its title and keycaps. Fill it
// from the application's accelerator registry
// (app.Application.ShortcutsDialog does) or from any data.
type ShortcutsView struct {
	*PreferencesPage
}

// NewShortcutsView builds the overview of sections.
func NewShortcutsView(face render.Font, sections []ShortcutSection) *ShortcutsView {
	face = requireFace("widget.NewShortcutsView", face)
	v := &ShortcutsView{PreferencesPage: NewPreferencesPage()}
	v.SetElement("shortcuts")
	for _, sec := range sections {
		g := NewPreferencesGroup(face, 14, sec.Title)
		for _, it := range sec.Items {
			row := NewActionRow(face, 14, it.Title, "")
			row.PackEnd(NewShortcutLabel(face, it.Keys))
			g.Add(row)
		}
		v.Add(g)
	}
	return v
}
