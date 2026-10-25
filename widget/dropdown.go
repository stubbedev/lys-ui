package widget

import (
	"math"
	"strings"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/text"
	"github.com/stubbedev/lys-ui/render"
)

const (
	// dropdownMaxListH caps the open item list's height; taller lists clip
	// (the app caps popovers at the same 600px).
	dropdownMaxListH = 600
	// dropdownSlide is how far the list rises into place over the open
	// tween (and falls back on close), masked to the list's own bounds.
	dropdownSlide = 10
	// dropdownTypeTimeout is how long a typed prefix survives after the
	// last key before the type-ahead chain resets.
	dropdownTypeTimeout = time.Second
)

// Dropdown is a combobox: a closed face showing the current selection
// that expands into a themed list of items beneath it.
//
// The list is an inline expansion, not an app.Popover: the widget
// package cannot import app (the dependency points the other way), and
// a self-contained popup keeps the dropdown usable in plain widget
// trees, in tests, and under hosts with no popover plumbing. While
// open the list is a real child (Children) arranged just below the
// face, so hit testing, damage, and focus traversal all see it; hosts
// that want true popup semantics can embed the dropdown in a popover
// of their own. While open the dropdown holds a press-away with the
// router: a click anywhere outside its subtree — another widget, empty
// space, a disabled control — closes the list through the same
// animated exit a selection uses, and it also closes on Esc or a
// second click on the face.
//
// Keyboard: Enter, Space, or Down opens while focused, arrows move the
// highlighted row (Home/End jump), Enter picks, Esc cancels without
// changing the selection.
//
// Type-ahead (GTK combobox parity): while the list is open, printable
// keys jump the highlight to the first row whose label starts with the
// typed prefix (case-insensitive); while closed, they cycle the
// selection through the rows sharing the pressed first letter. Space
// never joins a prefix — it keeps its open/activate role while open
// and opens while closed. Repeated keys cycle among the equal-prefix
// rows; the chain resets after a short idle timeout. Keys that match
// nothing are ignored outright — Enter, Space, arrows, Esc, and Tab
// routing never fight the type-ahead.
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

	menu    *Menu
	open    bool
	hovered bool

	// reveal is the open/close tween's progress: the list fades and
	// rises into place on open, and on close stays painted (closing)
	// until the exit tween lands, then detaches. listRect is the rest
	// rect the tween's slide is masked to.
	reveal       float64
	closing      bool
	listRect     render.Rect
	cancelReveal anim.Cancel

	// type-ahead state: the accumulated case-folded prefix, and the
	// idle timer that clears it.
	typed      string
	typeCancel anim.Cancel
	// faceBtn and faceArrow are the closed face's style nodes
	// (`dropdown > button > arrow`): the button's box paints the face,
	// the arrow's color the chevron.
	faceBtn   stylePart
	faceArrow stylePart

	// OnSelect fires exactly once per selection change, whatever
	// produced it: a click, Enter, or SetSelected. Re-picking the
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
	if d.open {
		if d.cancelReveal != nil {
			d.cancelReveal()
		}
		d.finishClose()
	}
	d.entries = rows
	d.items = make([]string, len(rows))
	for i, e := range rows {
		d.items[i] = e.Label
	}
	d.menu, d.faceIcon, d.faceFor = nil, nil, -1
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
// label and the open list's rows resolve and lay out with; the menu
// inherits it. DirectionAuto (the default) reads it off the label's
// first strong character. Changing the direction invalidates.
func (d *Dropdown) SetDirection(dir Direction) {
	if d.dir == dir {
		return
	}
	d.dir = dir
	if d.menu != nil {
		d.menu.SetDirection(dir)
	}
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
func (d *Dropdown) Opened() bool { return d.open }

// SetEnabled turns the dropdown on or off, following the toolkit-wide
// node.SetEnabled convention (the flag lives on the embedded node;
// Enabled reads it). Disabling closes the list; a disabled dropdown
// ignores clicks and keys and paints muted, the way Menu paints rows
// without an action.
func (d *Dropdown) SetEnabled(enabled bool) {
	was := d.Enabled()
	d.node.SetEnabled(enabled)
	if was && !enabled {
		d.Close()
	}
}

// list lazily builds the item menu from the items. The face arrived
// at construction, so the first open is the only build.
func (d *Dropdown) list() *Menu {
	if d.menu != nil {
		return d.menu
	}
	items := make([]MenuItem, len(d.entries))
	for i, e := range d.entries {
		items[i] = MenuItem{Label: e.Label, OnClick: func() { d.selectIndex(i) }}
		if e.Header {
			items[i] = MenuItem{Label: e.Label, Kind: ItemHeader}
		}
		if e.Icon != nil {
			items[i].Icon = e.Icon()
		}
	}
	d.menu = NewMenu(d.face, d.sizePx, items...)
	// A combobox list has no Alt-letters: type-ahead is its keyboard
	// model, so no row claims (or underlines) a mnemonic.
	d.menu.dropMnemonics()
	d.menu.SetDirection(d.dir)
	// Menu activation dismisses before the row fires, so the list is
	// closed (and its pixels invalidated) by the time OnSelect runs.
	d.menu.OnDismiss = d.Close
	return d.menu
}

// Open shows the item list beneath the face, highlighted on the
// current selection, with the kind's reveal tween: the first frame is
// hidden and offset toward the face — never a flash of the open list —
// and the tween raises it to rest. A disabled dropdown, an empty one,
// and one with no face stay closed.
func (d *Dropdown) Open() {
	if d.open || !d.Enabled() || len(d.items) == 0 {
		return
	}
	m := d.list()
	d.open = true
	m.hovered = d.selected
	d.playReveal(true)
	d.arrangeList()
	m.Invalidate()
	d.Invalidate()
}

// Close starts hiding the item list. The list keeps painting (fading,
// sliding back toward the face) until the exit tween lands, then
// detaches; its pixels are invalidated as an extra rect, both now and
// per tween frame, because the damage walk no longer descends into the
// unexposed child.
func (d *Dropdown) Close() {
	if !d.open || d.closing {
		return
	}
	d.closing = true
	d.InvalidateRect(d.listRect)
	d.Invalidate()
	d.playReveal(false)
}

// finishClose detaches the list after the exit tween landed.
func (d *Dropdown) finishClose() {
	d.closing = false
	d.open = false
	d.reveal = 0
	d.cancelReveal = nil
	if d.menu != nil {
		d.InvalidateRect(d.listRect)
	}
	d.Invalidate()
}

// playReveal runs one reveal tween: rising on open (from the current
// reveal, so re-opening mid-close does not blink), falling on close,
// landing in finishClose. Reduced motion collapses both to their end
// state inside Play — close detaches synchronously, exactly like the
// pre-tween code path.
func (d *Dropdown) playReveal(rising bool) {
	st := surfx.Plan(surfx.KindDropdown, Current().Animations)
	spec := st.Enter
	if !rising {
		spec = st.Exit
	}
	start := d.reveal
	if d.cancelReveal != nil {
		d.cancelReveal()
	}
	d.cancelReveal = anim.Play(anim.Animate(spec.Duration, func(p float64) {
		if rising {
			d.reveal = start + (1-start)*p
		} else {
			d.reveal = start * (1 - p)
		}
		// The slide lives in the arrangement (the menu's rect shifts
		// toward the face); re-place it per frame so the tween lands at
		// rest — the exact rest rect — when the reveal hits 1.
		d.arrangeList()
		d.InvalidateRect(d.listRect)
		d.Invalidate()
		if !rising && p >= 1 {
			d.finishClose()
		}
	}).Easing(spec.Easing))
}

// arrangeList places the item menu directly below the face, at least
// as wide as the face and capped in height, shifted toward the face by
// the unrevealed fraction of the slide (the tween's motion, masked to
// the rest rect at paint time).
func (d *Dropdown) arrangeList() {
	m := d.menu
	nat := m.Measure(Constraints{Max: Size{W: 1 << 24, H: dropdownMaxListH}})
	d.listRect = render.Rect{
		X: d.bounds.X,
		Y: d.bounds.Y + d.bounds.H,
		W: max(d.bounds.W, nat.W),
		H: nat.H,
	}
	dy := int((1 - d.reveal) * dropdownSlide)
	m.Arrange(render.Rect{X: d.listRect.X, Y: d.listRect.Y - dy, W: d.listRect.W, H: d.listRect.H})
	setParents(d, m)
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

// Arrange records the face's rect and, while the list shows (including
// a close tween in flight), keeps it pinned beneath the face.
func (d *Dropdown) Arrange(r render.Rect) {
	d.node.Arrange(r)
	if d.open {
		d.arrangeList()
	}
}

// Paint draws the closed face — surface, selection, chevron — and, on
// top of the frame beneath it, the open item list. A disabled dropdown
// fills with the derived disabled surface and paints muted.
func (d *Dropdown) Paint(cv *render.Canvas) {
	t := Current()
	bg := t.Surface
	switch {
	case d.open:
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

	if d.open {
		// The list hangs below the face, over whatever follows it: it
		// paints on the frame's top layer, after every later sibling.
		// The tween's slide is masked to the list's rest rect: the
		// shifted rows that would paint above it are clipped away, and
		// the fade rides the menu's own colors through PushAlpha.
		reveal := min(max(d.reveal, 0), 1)
		switch reveal {
		case 0:
			// Hidden: paint nothing (the skip proof's zero fast path).
		case 1:
			cv.Overlay(d.menu.Paint)
		default:
			cv.Overlay(func(cv *render.Canvas) {
				prev := cv.PushClip(d.listRect)
				alpha := cv.PushAlpha(reveal)
				d.menu.Paint(cv)
				cv.PopAlpha(alpha)
				cv.PopClip(prev)
			})
		}
	}
}

// Role implements Roleer.
func (d *Dropdown) Role() Role { return RoleComboBox }

// HitTest returns the item list while a point is inside it — the list
// hangs outside the face's bounds — else the dropdown inside its own.
func (d *Dropdown) HitTest(p Point) Widget {
	if d.open {
		if hit := d.menu.HitTest(p); hit != nil {
			return hit
		}
	}
	return d.HitLeaf(d, p)
}

// Children exposes the open item list for focus traversal (and the
// damage walk, which shares the interface); closed, there is nothing
// to traverse into.
func (d *Dropdown) Children() []Widget {
	if d.open {
		return []Widget{d.menu}
	}
	return nil
}

// appendChildren appends the open item list, matching Children.
func (d *Dropdown) appendChildren(buf []Widget) []Widget {
	if d.open {
		return append(buf, d.menu)
	}
	return buf
}

// ClickAt toggles the list; the Router invokes it when a press and
// release land on the face. Presses on the list go to the list; a
// press anywhere outside closes it through PressedOutside. A face
// click mid-close-tween reverses the close: the reveal rises from its
// current fraction, no blink to zero.
func (d *Dropdown) ClickAt(Point) {
	if !d.Enabled() {
		return
	}
	if d.closing {
		d.closing = false
		d.playReveal(true)
		return
	}
	if d.open {
		d.Close()
		return
	}
	d.Open()
}

// WantsPressAway holds while the list shows — open, or mid exit tween
// (a press then still dismisses, restarting nothing).
func (d *Dropdown) WantsPressAway() bool { return d.open }

// PressedOutside is the router's click-away notice: the list closes
// through the same animated exit a selection uses; if the exit is
// already running it keeps falling.
func (d *Dropdown) PressedOutside(Point) { d.Close() }

// SetHovered implements HoverSetter; the face's hover shade repaints.
func (d *Dropdown) SetHovered(on bool) {
	if d.hovered == on {
		return
	}
	d.hovered = on
	d.invalidateState(style.Hover)
}

// KeyAction implements KeyActionHandler: closed, Enter or Down opens;
// open, everything but Esc goes to the item list, so arrows move the
// highlight and Enter picks. Esc closes without changing anything.
func (d *Dropdown) KeyAction(a KeyAction, _ Mods) {
	if !d.Enabled() {
		return
	}
	if d.open {
		if a == KeyDismiss {
			d.Close()
			return
		}
		d.menu.KeyAction(a, 0)
		return
	}
	switch a {
	case KeyEnter, KeyDown:
		d.Open()
	}
}

// InsertRune implements RuneHandler: Space opens, the other half of
// the activation pair (Enter is the first). Open, Space behaves like
// Enter and activates the highlighted row. Every other printable rune
// feeds the type-ahead chain.
func (d *Dropdown) InsertRune(r rune) {
	if !d.Enabled() {
		return
	}
	if r == ' ' {
		d.KeyAction(KeyEnter, 0)
		return
	}
	d.typeAhead(r)
}

// typeAhead applies one printable key to the type-ahead chain:
// extending the prefix jumps to its first match, a repeated key cycles
// among the equal-prefix rows, and a key that matches nothing leaves
// every bit of state alone. Open, the jump moves the highlight; closed,
// it moves the selection (first-letter cycling). Each accepted key
// restarts the idle timer that clears the chain.
func (d *Dropdown) typeAhead(r rune) {
	key := strings.ToLower(string(r))
	prefix := d.typed + key
	rows := d.prefixRows(prefix)
	cycling := false
	if len(rows) == 0 {
		// The extension matched nothing: a fresh single-letter chain
		// (the repeated-key case, or a stale multi-letter buffer) is the
		// retry; a letter with no rows at all is a pass-through.
		prefix = key
		rows = d.prefixRows(key)
		cycling = d.typed != ""
		if len(rows) == 0 {
			return
		}
	}
	current := d.selected
	if d.open && d.menu != nil {
		current = d.menu.hovered
	}
	target := rows[0]
	if cycling {
		for _, i := range rows {
			if i > current {
				target = i
				break
			}
		}
	}
	d.typed = prefix
	d.armTypeTimeout()
	if d.open {
		d.menu.hovered = target
		d.menu.Invalidate()
		return
	}
	d.selectIndex(target)
}

// prefixRows returns the indices of items whose label starts with the
// case-folded prefix, ascending.
func (d *Dropdown) prefixRows(prefix string) []int {
	var rows []int
	for i, label := range d.items {
		if !d.entries[i].Header && strings.HasPrefix(strings.ToLower(label), prefix) {
			rows = append(rows, i)
		}
	}
	return rows
}

// armTypeTimeout (re)starts the idle timer that clears the typed
// chain; the delay keeps its duration under reduced motion, like every
// timing skeleton.
func (d *Dropdown) armTypeTimeout() {
	if d.typeCancel != nil {
		d.typeCancel()
	}
	d.typeCancel = anim.Play(anim.Sequence(
		anim.Delay(dropdownTypeTimeout),
		anim.Animate(0, func(float64) {
			d.typed = ""
			d.typeCancel = nil
		}),
	))
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

// appendStyleKids is the button (its arrow below), the open item list,
// and the face's icon (styleKids).
func (d *Dropdown) appendStyleKids(buf []Widget) []Widget {
	buf = append(buf, &d.faceBtn)
	buf = append(buf, d.Children()...)
	if d.faceIcon != nil {
		buf = append(buf, d.faceIcon)
	}
	return buf
}
