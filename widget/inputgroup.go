package widget

import "github.com/stubbedev/lys-lib/render"

// InputGroup is an entry with addons before and after it - icons, text,
// buttons - inside one shared border and one focus ring: the group
// draws the input's border, ring-colored while its entry has the focus,
// and the keyboard focus ring goes around the whole group. It styles as
// the `inputgroup` element; addon text and icons take the muted color.
type InputGroup struct {
	composite
	row   *Box
	entry *Entry
}

// NewInputGroup returns a group around entry with no addons yet.
func NewInputGroup(entry *Entry) *InputGroup {
	g := &InputGroup{row: NewBox(Row, 8, 0), entry: entry}
	g.SetElement("inputgroup")
	g.row.SetPadding(render.Insets{Left: 12, Right: 12})
	g.row.Append(entry, true)
	g.initComposite(g, g.row)
	g.boxed = true
	return g
}

// Entry returns the group's entry.
func (g *InputGroup) Entry() *Entry { return g.entry }

// Prepend adds w as the first addon, before the entry.
func (g *InputGroup) Prepend(w Widget) {
	g.row.InsertAligned(0, w, false, AlignCenter)
}

// Append adds w as the last addon, after the entry.
func (g *InputGroup) Append(w Widget) {
	g.row.AppendAligned(w, false, AlignCenter)
}

// focusRingTarget draws the entry's focus ring around the group.
func (g *InputGroup) focusRingTarget() Widget { return g }
