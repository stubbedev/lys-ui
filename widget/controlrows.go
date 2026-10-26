package widget

import (
	"github.com/stubbedev/lys-lib/render"
)

// The control rows of #93 - libadwaita's settings vocabulary over
// ActionRow and the existing controls. Each embeds its ActionRow
// whole (geometry flows through the row's packs), adds its control to
// the end slot, and exposes the control's own API surface.

// SwitchRow is an ActionRow with a Switch in its end slot: the
// subtitle typically explains the consequence of flipping.
type SwitchRow struct {
	*ActionRow
	sw *Switch
}

// NewSwitchRow returns the row; on, when set, fires after flips,
// whatever the source (GTK toggled).
func NewSwitchRow(face render.Font, sizePx float64, title, subtitle string, on func(on bool)) *SwitchRow {
	r := &SwitchRow{ActionRow: NewActionRow(face, sizePx, title, subtitle)}
	r.sw = NewSwitch(false)
	r.sw.OnChanged = on
	r.PackEnd(r.sw)
	return r
}

// Set flips the switch programmatically (fires the same hook).
func (r *SwitchRow) Set(on bool) { r.sw.SetOn(on) }

// On reports the state.
func (r *SwitchRow) On() bool { return r.sw.On() }

// Switch returns the embedded control, for BindOn.
func (r *SwitchRow) Switch() *Switch { return r.sw }

// SpinRow is an ActionRow with a SpinButton in its end slot.
type SpinRow struct {
	*ActionRow
	spin *SpinButton
}

// NewSpinRow returns the row over [min, max] stepping by step with
// digits decimals; on, when set, receives user-committed changes.
func NewSpinRow(face render.Font, sizePx float64, title, subtitle string, min, max, step float64, digits int, on func(v float64)) *SpinRow {
	r := &SpinRow{ActionRow: NewActionRow(face, sizePx, title, subtitle)}
	r.spin = NewSpinButton(face, sizePx, Current().Text, min, max, step, digits)
	r.spin.OnValueChanged = on
	r.PackEnd(r.spin)
	return r
}

// Value reports the committed value.
func (r *SpinRow) Value() float64 { return r.spin.Value() }

// SetValue sets it silently - mirroring an external value never
// echoes back as a user change.
func (r *SpinRow) SetValue(v float64) { r.spin.SetValue(v) }

// SpinButton returns the embedded control.
func (r *SpinRow) SpinButton() *SpinButton { return r.spin }

// ComboRow is an ActionRow with a Dropdown in its end slot; the
// selection shows on the row's face.
type ComboRow struct {
	*ActionRow
	drop *Dropdown
}

// NewComboRow returns the row over items with selected picked; on,
// when set, receives selection changes.
func NewComboRow(face render.Font, sizePx float64, title, subtitle string, items []string, selected int, on func(i int)) *ComboRow {
	r := &ComboRow{ActionRow: NewActionRow(face, sizePx, title, subtitle)}
	r.drop = NewDropdown(face, sizePx, items, selected)
	r.drop.OnSelect = on
	r.PackEnd(r.drop)
	return r
}

// Selected reports the selection index.
func (r *ComboRow) Selected() int { return r.drop.Selected() }

// SetSelected selects i, firing on like every selection change
// (the Dropdown fires OnSelect whatever the source).
func (r *ComboRow) SetSelected(i int) { r.drop.SetSelected(i) }

// Dropdown returns the embedded control.
func (r *ComboRow) Dropdown() *Dropdown { return r.drop }

// EntryRow is the adw signature: the entry IS the row - title label
// to the leading side, the free-text field filling the rest.
type EntryRow struct {
	*ActionRow
	entry *Entry
}

// NewEntryRow returns the row with the field's text starting at
// initial.
func NewEntryRow(face render.Font, sizePx float64, title, initial string) *EntryRow {
	r := &EntryRow{ActionRow: NewActionRow(face, sizePx, title, "")}
	r.entry = NewEntry(face, sizePx, Current().Text)
	r.entry.SetText(initial)
	r.entry.SetPlaceholder(title)
	r.PackEnd(r.entry)
	return r
}

// Text reports the field's contents.
func (r *EntryRow) Text() string { return r.entry.Text() }

// SetText replaces the field's contents.
func (r *EntryRow) SetText(s string) { r.entry.SetText(s) }

// Entry returns the embedded field, for hooks and BindText.
func (r *EntryRow) Entry() *Entry { return r.entry }

// ButtonRow is a full-width activatable row wearing a button look:
// the title centered, activation whole-row, for the destructive and
// affirmative actions a preferences page ends with.
type ButtonRow struct {
	*ActionRow
}

// NewButtonRow returns the activatable centered row; onClick is its
// activation.
func NewButtonRow(face render.Font, sizePx float64, title string, onClick func()) *ButtonRow {
	r := &ButtonRow{ActionRow: NewActionRow(face, sizePx, title, "")}
	r.SetActivatable(true)
	r.OnActivate = onClick
	return r
}

// ExpanderRow is an ActionRow whose activation discloses sub-rows:
// any widget (more rows, a group) inside the row's own disclosure,
// with the chevron's direction tracking it.
type ExpanderRow struct {
	*ActionRow
	content *Box
	arrow   *Symbol
}

// NewExpanderRow returns a collapsed expander; Add appends into its
// content.
func NewExpanderRow(face render.Font, sizePx float64, title, subtitle string) *ExpanderRow {
	r := &ExpanderRow{ActionRow: NewActionRow(face, sizePx, title, subtitle)}
	r.content = NewBox(Column, 0, 0)
	r.SetDisclosure(r.content)
	r.arrow = NewSymbol(SymbolChevronRight, int(sizePx))
	r.PackEnd(r.arrow)
	r.SetActivatable(true)
	r.OnActivate = r.Expand
	return r
}

// Add appends a sub-row (or any widget) to the disclosed content.
func (r *ExpanderRow) Add(w Widget) {
	r.content.Append(w, false)
	r.InvalidateLayout()
}

// Expand toggles the disclosure.
func (r *ExpanderRow) Expand() {
	on := !r.Revealed()
	r.SetRevealed(on)
	if on {
		r.arrow.SetKind(SymbolChevronDown)
	} else {
		r.arrow.SetKind(SymbolChevronRight)
	}
}

// Expanded reports the disclosure state.
func (r *ExpanderRow) Expanded() bool { return r.Revealed() }
