package widget

import (
	"math"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/internal/text"
	"github.com/stubbedev/lys-ui/render"
)

// Dropdown is a combobox: a closed face showing the current selection
// that opens its items in a popover beneath it, through the surface
// host of its tree (OpenPopover), as a Command list with no search
// row: header rows head its groups, row icons lead their items, and a
// check trails the selected one. The popover closes on a pick, on Esc
// or a click away, or on a second click on the face.
//
// Keyboard: Enter, Space, or Down opens while focused; in the open list
// arrows move the highlight (Home/End jump), Enter or Space picks, and
// printable keys jump the highlight by type-ahead.
//
// Type-ahead on the closed face (GTK combobox parity): printable keys
// cycle the selection through the rows whose label starts with the
// typed prefix (case-insensitive). Space never joins a prefix: it
// opens. Repeated keys cycle among the equal-prefix rows; the chain
// resets after a short idle timeout. Keys that match nothing are
// ignored outright.
//
// The constructor takes the face like every other text widget: face,
// then sizePx, then the items. Face may be a render.Chain for
// mixed-script fallback; a nil face panics there (see requireFace)
// instead of failing later, in shaping.
type Dropdown struct {
	node
	face     render.Font
	sizePx   float64
	dir      Direction
	items    []string
	selected int
	// entries are the rich rows (their labels mirrored in items);
	// faceIcon is the selected row's icon on the face, built for
	// faceFor.
	entries  []DropdownRow
	faceIcon *Icon
	faceFor  int

	// list is the open popover's item list and closeList what closes
	// it; both are nil while the list is closed.
	list      *Command
	closeList func()
	hovered   bool

	// typeAhead is the closed face's type-ahead chain.
	typeAhead typeAhead
	// faceBtn and faceArrow are the closed face's style nodes
	// (`dropdown > button > arrow`): the button's box paints the face,
	// the arrow's color the chevron.
	faceBtn   stylePart
	faceArrow stylePart

	// OnSelect fires exactly once per selection change, whatever
	// produced it: a pick in the list or SetSelected. Re-picking the
	// selected row is not a change and fires nothing.
	OnSelect func(i int)
}

// NewDropdown returns a dropdown showing items, painted with face at
// sizePx, with the selected index clamped into range (-1 when there
// are no items). Face may be a render.Chain for mixed-script fallback.
func NewDropdown(face render.Font, sizePx float64, items []string, selected int) *Dropdown {
	entries := make([]DropdownRow, len(items))
	for i, label := range items {
		entries[i] = DropdownRow{Label: label}
	}
	return buildDropdown("widget.NewDropdown", face, sizePx, entries, selected)
}

// DropdownRow is one row of a dropdown built with NewDropdownRows,
// GTK's DropDown with a row factory. Icon, when set, builds the row's
// leading icon: it is called for the list row and for the face, so
// each owns its instance. A Header row is a group caption: listed,
// never selected, stepped onto, or matched by type-ahead.
type DropdownRow struct {
	Label  string
	Icon   func() *Icon
	Header bool
}

// NewDropdownRows is NewDropdown over rich rows. A selected index on
// a header moves to the nearest selectable row after it (before it at
// the end).
func NewDropdownRows(face render.Font, sizePx float64, items []DropdownRow, selected int) *Dropdown {
	return buildDropdown("widget.NewDropdownRows", face, sizePx, items, selected)
}

func buildDropdown(ctor string, face render.Font, sizePx float64, entries []DropdownRow, selected int) *Dropdown {
	d := &Dropdown{face: requireFace(ctor, face), sizePx: sizePx, entries: entries, selected: -1, faceFor: -1}
	d.faceBtn.SetElement("button")
	d.faceArrow.SetElement("arrow")
	setParents(d, &d.faceBtn)
	setParents(&d.faceBtn, &d.faceArrow)
	d.items = make([]string, len(entries))
	for i, e := range entries {
		d.items[i] = e.Label
	}
	if len(entries) > 0 {
		d.selected = d.selectableFrom(min(max(selected, 0), len(entries)-1))
	}
	return d
}

// SetItems replaces the items (GTK's DropDown.set_model) and selects
// selected, clamped as the constructor clamps it. It fires no
// OnSelect: the caller is choosing what shows, not the user. An open
// list closes at once.
func (d *Dropdown) SetItems(items []string, selected int) {
	rows := make([]DropdownRow, len(items))
	for i, label := range items {
		rows[i] = DropdownRow{Label: label}
	}
	d.SetRows(rows, selected)
}

// SetRows is SetItems over rich rows.
func (d *Dropdown) SetRows(rows []DropdownRow, selected int) {
	d.Close()
	d.entries = rows
	d.items = make([]string, len(rows))
	for i, e := range rows {
		d.items[i] = e.Label
	}
	d.faceIcon, d.faceFor = nil, -1
	d.selected = -1
	if len(rows) > 0 {
		d.selected = d.selectableFrom(min(max(selected, 0), len(rows)-1))
	}
	d.InvalidateLayout()
}

// selectableFrom is the first non-header row at or after i, else the
// last one before it; -1 when every row is a header.
func (d *Dropdown) selectableFrom(i int) int {
	for j := max(i, 0); j < len(d.entries); j++ {
		if !d.entries[j].Header {
			return j
		}
	}
	for j := min(i, len(d.entries)) - 1; j >= 0; j-- {
		if !d.entries[j].Header {
			return j
		}
	}
	return -1
}

// Selected returns the selected item index, -1 when empty.
func (d *Dropdown) Selected() int { return d.selected }

// Items returns the item labels, row order.
func (d *Dropdown) Items() []string {
	out := make([]string, len(d.items))
	copy(out, d.items)
	return out
}

// SetDirection selects the base paragraph direction the selection
// label resolves and lays out with. DirectionAuto (the default) reads
// it off the label's first strong character. Changing the direction
// invalidates.
func (d *Dropdown) SetDirection(dir Direction) {
	if d.dir == dir {
		return
	}
	d.dir = dir
	d.Invalidate()
}

// Direction returns the base paragraph direction the labels resolve
// with.
func (d *Dropdown) Direction() Direction { return d.dir }

// Selection returns the selected item's label, empty when none.
func (d *Dropdown) Selection() string {
	if d.selected < 0 || d.selected >= len(d.items) {
		return ""
	}
	return d.items[d.selected]
}

// SetSelected moves the selection to i (clamped) and fires OnSelect
// when the index changed.
func (d *Dropdown) SetSelected(i int) { d.selectIndex(i) }

// selectIndex clamps i, repaints, and fires OnSelect once on change.
func (d *Dropdown) selectIndex(i int) {
	if len(d.items) == 0 {
		return
	}
	i = min(max(i, 0), len(d.items)-1)
	if d.entries[i].Header {
		return // a caption is not a choice
	}
	if i == d.selected {
		return
	}
	d.selected = i
	d.Invalidate()
	if d.OnSelect != nil {
		d.OnSelect(i)
	}
}

// Opened reports whether the item list is showing.
func (d *Dropdown) Opened() bool { return d.list != nil }

// SetEnabled turns the dropdown on or off, following the toolkit-wide
// node.SetEnabled convention (the flag lives on the embedded node;
// Enabled reads it). Disabling closes the list; a disabled dropdown
// ignores clicks and keys and paints muted.
func (d *Dropdown) SetEnabled(enabled bool) {
	was := d.Enabled()
	d.node.SetEnabled(enabled)
	if was && !enabled {
		d.Close()
	}
}

// newList builds the item list: a Command without a search row whose
// groups the header rows head, its items in row order with their icons
// and the selected one's trailing check, highlighted on the selection.
func (d *Dropdown) newList() *Command {
	var groups []CommandGroup
	highlight, row := -1, 0
	for i, e := range d.entries {
		if e.Header {
			groups = append(groups, CommandGroup{Heading: e.Label})
			continue
		}
		if len(groups) == 0 {
			groups = append(groups, CommandGroup{})
		}
		it := CommandItem{Label: e.Label, Trailing: newCommandCheck(i == d.selected), OnSelect: func() { d.pick(i) }}
		if e.Icon != nil {
			it.Icon = e.Icon()
		}
		g := &groups[len(groups)-1]
		g.Items = append(g.Items, it)
		if i == d.selected {
			highlight = row
		}
		row++
	}
	list := NewCommandList(d.face, groups...)
	list.highlightRow(highlight)
	return list
}

// Open shows the item list in a popover beneath the face, at least as
// wide as the face and highlighted on the selection, with the keyboard
// focus in the list. A disabled dropdown and one with nothing to
// select stay closed.
func (d *Dropdown) Open() {
	if d.list != nil || !d.Enabled() || d.selected < 0 {
		return
	}
	list := d.newList()
	list.SetSizeRequest(d.bounds.W, 0)
	d.list = list
	d.closeList = OpenPopover(d, d, list, PopoverOptions{Focus: list, Gap: PopoverGap, OnClosed: func() { d.listClosed(list) }})
	d.Invalidate()
}

// Close closes the item list's popover; a closed dropdown ignores it.
func (d *Dropdown) Close() {
	if d.closeList == nil {
		return
	}
	closeList := d.closeList
	d.list, d.closeList = nil, nil
	d.Invalidate()
	closeList()
}

// listClosed is list's popover going away, however it closed: the face
// stops showing it open unless a newer list replaced it.
func (d *Dropdown) listClosed(list *Command) {
	if d.list != list {
		return
	}
	d.list, d.closeList = nil, nil
	d.Invalidate()
}

// pick is the user's choice of row i in the list: the list closes,
// then the selection moves, so OnSelect runs with the list gone.
func (d *Dropdown) pick(i int) {
	d.Close()
	d.selectIndex(i)
}

// Measure wants the widest item plus arrow and padding, and one row
// height; the closed face keeps its width whatever the selection, so a
// change never relayouts the surrounding tree. 80px is the width
// floor.
func (d *Dropdown) Measure(con Constraints) Size {
	if sz, ok := d.measureHit(con); ok {
		return sz
	}
	w := 80
	// The face button's min-height floors the height, as it does a
	// button's: 36px unstyled, an entry's height.
	h := max(d.face.Shape("lg", d.sizePx).LineHeight()+12, picki(d.faceBtn.style(&d.faceBtn), style.PropMinHeight, entryMinHeight))
	slot := d.iconSlot()
	for _, label := range d.items {
		if adv := int(d.face.Shape(label, d.sizePx).Advance()+0.5) + 14 + 24 + slot; adv > w {
			w = adv
		}
	}
	return d.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Paint draws the face — surface, selection, chevron — in its pressed
// shade while the list is open. A disabled dropdown fills with the
// derived disabled surface and paints muted.
func (d *Dropdown) Paint(cv *render.Canvas) {
	t := Current()
	bg := t.Surface
	switch {
	case d.list != nil:
		bg = t.SurfacePressed
	case d.hovered && d.Enabled():
		bg = t.SurfaceHover
	case !d.Enabled():
		bg = t.DisabledSurface()
	}
	bv := d.faceBtn.style(&d.faceBtn)
	radii := radiusOr(bv, t.Radius)
	fx := pushEffects(cv, &d.faceBtn.node, bv, d.bounds, radii)
	cv = fx.cv
	d.faceBtn.Arrange(d.bounds)
	paintBoxBehind(cv, bv, d.bounds, radii, borderOf(bv), pickc(0, bv, style.PropBackgroundColor, bg))
	paintOutline(cv, bv, d.bounds, radii)
	fx.pop()

	col := t.Text
	if !d.Enabled() {
		col = t.DisabledText()
	}
	lineH := d.face.Shape("lg", d.sizePx).LineHeight()
	baseline := d.bounds.Y + (d.bounds.H-lineH)/2 + int(d.face.Shape("lg", d.sizePx).Ascent()+0.5)
	sh := d.face.ShapeDir(d.Selection(), d.sizePx, d.dir)
	lx := d.bounds.X + 8
	if ic := d.selectedIcon(); ic != nil {
		setParents(d, ic)
		sz := ic.Measure(Constraints{Max: Size{W: d.bounds.W, H: d.bounds.H}})
		ic.Arrange(render.Rect{X: lx, Y: d.bounds.Y + (d.bounds.H-sz.H)/2, W: sz.W, H: sz.H})
		PaintChild(cv, ic)
		lx += sz.W + dropdownIconGap
	}
	if text.RTL(d.Selection(), d.dir) {
		// A right-to-left selection reads from the right edge; keep
		// clear of the chevron.
		lx = d.bounds.X + d.bounds.W - 28 - int(sh.Advance()+0.5)
	}
	d.face.Draw(cv, sh, lx, baseline, col)
	// The chevron: two strokes forming a v at the face's right edge,
	// in the arrow node's color.
	av := d.faceArrow.style(&d.faceArrow)
	chev := pickc(0, av, style.PropColor, col)
	cx, cy := d.bounds.X+d.bounds.W-16, d.bounds.Y+d.bounds.H/2
	strokeChevron(cv, cx, cy, 4, math.Pi/2, chev)
}

// Role implements Roleer.
func (d *Dropdown) Role() Role { return RoleComboBox }

// HitTest returns the dropdown inside its face.
func (d *Dropdown) HitTest(p Point) Widget { return d.HitLeaf(d, p) }

// ClickAt toggles the list; the Router invokes it when a press and
// release land on the face.
func (d *Dropdown) ClickAt(Point) {
	if !d.Enabled() {
		return
	}
	if d.list != nil {
		d.Close()
		return
	}
	d.Open()
}

// SetHovered implements HoverSetter; the face's hover shade repaints.
func (d *Dropdown) SetHovered(on bool) {
	if d.hovered == on {
		return
	}
	d.hovered = on
	d.invalidateState(style.Hover)
}

// KeyAction implements KeyActionHandler: Enter or Down opens. The open
// list holds the focus, so its keys reach it, not the face.
func (d *Dropdown) KeyAction(a KeyAction, _ Mods) {
	if !d.Enabled() {
		return
	}
	switch a {
	case KeyEnter, KeyDown:
		d.Open()
	}
}

// InsertRune implements RuneHandler: Space opens, the other half of
// the activation pair (Enter is the first). Every other printable rune
// feeds the type-ahead chain, which moves the selection.
func (d *Dropdown) InsertRune(r rune) {
	if !d.Enabled() {
		return
	}
	if r == ' ' {
		d.Open()
		return
	}
	next, ok := d.typeAhead.next(r, d.selected, len(d.entries), func(i int) (string, bool) {
		return d.entries[i].Label, !d.entries[i].Header
	})
	if ok {
		d.selectIndex(next)
	}
}

// DropdownItem pairs a display label with the typed value picking it
// reports.
type DropdownItem[T any] struct {
	Label string
	Value T
}

// DropdownOf is a Dropdown whose rows carry typed values: the labels
// feed the visible list, and a change comes back with the value behind
// the picked row. It is a Dropdown in every other respect — embed it in
// a tree as-is.
type DropdownOf[T any] struct {
	*Dropdown
	values []T

	// OnSelect fires exactly once per selection change with the new
	// row's index and value. It shadows the wrapped Dropdown's OnSelect
	// (a func(int)); set this one.
	OnSelect func(i int, value T)
}

// NewDropdownOf builds a typed dropdown from labeled values, painted
// with face at sizePx, showing the selected index clamped into range.
// Face may be a render.Chain for mixed-script fallback; a nil face
// panics here, naming the argument (see requireFace).
func NewDropdownOf[T any](face render.Font, sizePx float64, items []DropdownItem[T], selected int) *DropdownOf[T] {
	face = requireFace("widget.NewDropdownOf", face)
	labels := make([]string, len(items))
	values := make([]T, len(items))
	for i, it := range items {
		labels[i], values[i] = it.Label, it.Value
	}
	d := &DropdownOf[T]{Dropdown: NewDropdown(face, sizePx, labels, selected), values: values}
	d.Dropdown.OnSelect = func(i int) {
		if d.OnSelect != nil && i >= 0 && i < len(d.values) {
			d.OnSelect(i, d.values[i])
		}
	}
	return d
}

// Value returns the picked row's value, the zero T when nothing is
// selected.
func (d *DropdownOf[T]) Value() T {
	var zero T
	i := d.Selected()
	if i < 0 || i >= len(d.values) {
		return zero
	}
	return d.values[i]
}

// dropdownIconGap separates a row's icon from its label on the face.
const dropdownIconGap = 6

// selectedIcon is the face's icon for the selected row, rebuilt when
// the selection moves; nil for a row without one.
func (d *Dropdown) selectedIcon() *Icon {
	if d.selected < 0 || d.selected >= len(d.entries) || d.entries[d.selected].Icon == nil {
		d.faceIcon, d.faceFor = nil, -1
		return nil
	}
	if d.faceFor != d.selected || d.faceIcon == nil {
		d.faceIcon, d.faceFor = d.entries[d.selected].Icon(), d.selected
	}
	return d.faceIcon
}

// iconSlot is the widest row icon plus the gap, 0 without icons: the
// face reserves it so a selection change never relayouts.
func (d *Dropdown) iconSlot() int {
	w := 0
	for _, e := range d.entries {
		if e.Icon != nil {
			w = max(w, e.Icon().Measure(Constraints{Max: Size{W: 1 << 10, H: 1 << 10}}).W+dropdownIconGap)
		}
	}
	return w
}

// appendStyleKids is the button (its arrow below) and the face's icon
// (styleKids).
func (d *Dropdown) appendStyleKids(buf []Widget) []Widget {
	buf = append(buf, &d.faceBtn)
	if d.faceIcon != nil {
		buf = append(buf, d.faceIcon)
	}
	return buf
}
