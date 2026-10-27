package widget

import (
	"time"

	"github.com/stubbedev/lys-lib/render"
)

// ActionRow is the preferences vocabulary's row (#93): a title with
// an optional subtitle (both ellipsized in the middle when tight),
// start and end pack slots, activatable whole-row clicks, and a
// selectable state - the libadwaita AdwActionRow shape over the same
// Box plumbing every form was hand-built from. Styled as the `row`
// element inside a PreferencesGroup's `rows` list; alone it paints
// its surface too, so a bare row still reads.
type ActionRow struct {
	composite
	packs
	face   render.Font
	sizePx float64

	// column stacks the row line and, when a disclosure is wired, its
	// revealed content; line is the row proper.
	column   *Box
	line     *Box
	middle   *Box
	title    *RichLabel
	subtitle *RichLabel
	reveal   *Revealer
	chevron  *Symbol

	// OnActivate fires for an activatable row's whole-row click or
	// Enter press - the list-activation contract.
	OnActivate func()

	activatable bool
	selected    bool
	pressed     bool
	unsearched  bool
}

// SetSearchable includes or excludes the row from a PreferencesView
// search; rows are searchable by default.
func (r *ActionRow) SetSearchable(on bool) { r.unsearched = !on }

// Subtitle returns the subtitle's text.
func (r *ActionRow) Subtitle() string { return r.subtitle.Text() }

func (r *ActionRow) searchTerms() (title, subtitle string, ok bool) {
	return r.Title(), r.Subtitle(), !r.unsearched
}

// NewActionRow returns a row with a title and an optional subtitle
// (empty hides the line).
func NewActionRow(face render.Font, sizePx float64, title, subtitle string) *ActionRow {
	face = requireFace("widget.NewActionRow", face)
	r := &ActionRow{face: face, sizePx: sizePx}
	r.SetElement("row")
	th := Current()
	r.title = NewRichLabel(face, sizePx, title, th.Text)
	r.title.SetEllipsize(EllipsizeMiddle)
	r.subtitle = NewRichLabel(face, sizePx-3, subtitle, th.TextMuted)
	r.subtitle.SetEllipsize(EllipsizeMiddle)
	r.subtitle.SetVisible(subtitle != "")
	r.middle = NewBox(Column, 1, 0)
	r.middle.Append(r.title, false)
	r.middle.Append(r.subtitle, false)
	r.initPacks(r, 6)
	r.line = NewBox(Row, 10, 6)
	r.line.Append(r.start, false)
	r.line.AppendAligned(r.middle, true, AlignCenter)
	r.line.Append(r.end, false)
	r.column = NewBox(Column, 0, 0)
	r.column.Append(r.line, false)
	r.initComposite(r, r.column)
	r.SetHExpand(true)
	r.surfaceRadius = 4
	return r
}

// SetTitle sets the title; SetSubtitle the line under it (empty
// hides it).
func (r *ActionRow) SetTitle(t string) { r.title.SetMarkup(t) }

// SetSubtitle sets the subtitle; empty hides it.
func (r *ActionRow) SetSubtitle(s string) {
	r.subtitle.SetMarkup(s)
	r.subtitle.SetVisible(s != "")
}

// Title returns the current title.
func (r *ActionRow) Title() string { return r.title.Text() }

// SetActivatable makes the whole row click- and Enter-activatable,
// with the pointer cursor and a disclosure chevron (ShowChevron
// controls the chevron alone).
func (r *ActionRow) SetActivatable(on bool) {
	if r.activatable == on {
		return
	}
	r.activatable = on
	r.cursorName = ""
	if on {
		r.cursorName = "pointer"
	}
	r.Invalidate()
}

// Activatable reports the whole-row activation mode.
func (r *ActionRow) Activatable() bool { return r.activatable }

// ShowChevron shows or hides the trailing disclosure arrow
// independently of activation.
func (r *ActionRow) ShowChevron(on bool) {
	if on == (r.chevron != nil) {
		return
	}
	if on {
		r.chevron = NewSymbol(SymbolChevronRight, int(r.sizePx))
		r.PackEnd(r.chevron)
	} else if i := r.endChildIndex(r.chevron); i >= 0 {
		r.end.RemoveAt(i)
		r.chevron = nil
	}
	r.InvalidateLayout()
}

// childIndex finds a packed child's slot.
func (r *ActionRow) endChildIndex(w Widget) int {
	for i, child := range r.end.Children() {
		if child == w {
			return i
		}
	}
	return -1
}

// SetSelected paints the row's :selected state - the list selection
// contract, driven by whatever owns the selection.
func (r *ActionRow) SetSelected(on bool) {
	r.selected = on
	r.SetState(StateSelected, on)
	r.Invalidate()
}

// Selected reports the selected state.
func (r *ActionRow) Selected() bool { return r.selected }

// SetPressed paints the row's :checked press feedback (the router's
// press/release cycle calls it on activatable rows).
func (r *ActionRow) SetPressed(on bool) {
	r.pressed = on
	r.SetState(StateChecked, on)
	r.Invalidate()
}

// ClickAt activates an activatable row wherever it was clicked; a
// non-activatable row ignores the click (its packed controls handle
// their own).
func (r *ActionRow) ClickAt(p Point) {
	if r.activatable && r.OnActivate != nil {
		r.OnActivate()
	}
}

// KeyAction activates an activatable row on Enter or Space.
func (r *ActionRow) KeyAction(a KeyAction, mods Mods) {
	if !r.activatable || r.OnActivate == nil {
		return
	}
	if a == KeyEnter || a == KeySpace {
		r.OnActivate()
	}
}

// SetDirection mirrors the row's packs (Box's RTL mirroring).
func (r *ActionRow) SetDirection(d Direction) { r.line.SetDirection(d) }

// Direction reports the row's flow direction.
func (r *ActionRow) Direction() Direction { return r.line.Direction() }

// SetDisclosure wires sub-content revealed under the row - the
// ExpanderRow mechanism, a capability of the row itself: w sits in a
// slide-down revealer under the line, initially collapsed.
func (r *ActionRow) SetDisclosure(w Widget) {
	r.reveal = NewRevealer(w)
	r.reveal.SetTransition(RevealSlideDown)
	r.reveal.SetDuration(180 * time.Millisecond)
	r.reveal.SetRevealed(false)
	r.column.Append(r.reveal, false)
	r.InvalidateLayout()
}

// SetRevealed opens or closes the disclosure.
func (r *ActionRow) SetRevealed(on bool) {
	if r.reveal != nil {
		r.reveal.SetRevealed(on)
	}
}

// Revealed reports the disclosure state (false without one).
func (r *ActionRow) Revealed() bool { return r.reveal != nil && r.reveal.Revealed() }

// Paint draws the row surface (selected and press washes) then the
// composed row.
func (r *ActionRow) Paint(cv *render.Canvas) {
	th := Current()
	var fill Color
	if r.selected {
		fill = th.SurfaceHover
	} else if r.pressed {
		fill = th.SurfacePressed
	}
	r.paintSurface(cv, r.line.Bounds(), fill)
	PaintChild(cv, r.root)
}
