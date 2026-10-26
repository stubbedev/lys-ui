package widget

import (
	"slices"

	"github.com/stubbedev/lys-ui/render"
)

// CommandItem is one entry of a Command.
type CommandItem struct {
	// Label is the text shown and matched.
	Label string
	// Keywords are extra terms the search matches the item by.
	Keywords []string
	// Shortcut is the keys shown at the row's end, display only.
	Shortcut string
	// Icon leads the row; nil shows none.
	Icon Widget
	// Trailing ends the row, after the shortcut; nil shows none.
	Trailing Widget
	// OnSelect runs when the item is picked.
	OnSelect func()
	// Disabled shows the item without letting it be picked.
	Disabled bool
}

// CommandGroup is a headed run of items; an empty heading shows none.
type CommandGroup struct {
	Heading string
	Items   []CommandItem
}

// commandRow is an item's row: a box that hovers, highlights and
// picks as a whole and leaves the focus in the search entry.
type commandRow struct {
	*Box
	c     *Command
	item  CommandItem
	group int
	score int
}

// HitTest takes every press inside the row.
func (r *commandRow) HitTest(p Point) Widget { return r.HitLeaf(r, p) }

// SetHovered implements HoverSetter: a hovered row takes the highlight.
func (r *commandRow) SetHovered(on bool) {
	if on && !r.item.Disabled {
		r.c.moveHighlight(slices.Index(r.c.shown, r))
	}
}

// ClickAt implements Clicker: a click picks the row.
func (r *commandRow) ClickAt(Point) {
	if !r.item.Disabled {
		r.c.moveHighlight(slices.Index(r.c.shown, r))
		r.c.pick()
	}
}

// commandListHeight caps the list, cmdk's max-h-[300px].
const commandListHeight = 300

// commandTextSize is the item text size.
const commandTextSize = 14

// Command is a searchable list of actions: a search entry over grouped
// items that filters them fuzzily as the query is typed, ranking the
// best matches first within each group and the groups by their best,
// with an empty state when nothing matches. Up, Down, Home and End move
// the highlight while the entry keeps the focus; Enter or a click picks
// the highlighted item. A list built with NewCommandList has no search
// row: it takes the focus itself, moves the highlight on the same keys,
// picks on Enter or Space, and jumps by type-ahead on printable keys.
// It works inline or as a popover's or dialog's content. It styles as
// the `command` element: the .input row, .heading labels, .item rows
// (:selected while highlighted), .shortcut labels, .separator lines and
// the .empty label.
type Command struct {
	composite
	face render.Font
	// input is the search entry, nil for a list without a search row.
	input     *Entry
	column    *Box
	list      *Box
	scroll    *Scroll
	empty     *Label
	headings  []*Label
	rows      []*commandRow
	shown     []*commandRow
	highlight int
	// revealPending is a highlight set before the rows were laid out,
	// revealed by the next Arrange.
	revealPending bool
	typeAhead     typeAhead
}

// NewCommand returns a command over groups whose search entry shows
// placeholder.
func NewCommand(face render.Font, placeholder string, groups ...CommandGroup) *Command {
	c := &Command{face: requireFace("widget.NewCommand", face)}
	c.input = NewEntry(c.face, commandTextSize, 0)
	c.input.SetPlaceholder(placeholder)
	c.input.OnChanged = func(string) { c.filter() }
	search := NewBox(Row, 8, 0)
	search.AddClass("input")
	search.SetPadding(render.Insets{Left: 12, Right: 12})
	search.AppendAligned(NewSymbol(SymbolSearch, 16), false, AlignCenter)
	search.Append(c.input, true)
	c.build(search, groups)
	return c
}

// NewCommandList returns a command over groups with no search row: a
// pick list, such as a dropdown's, that is its own focus target.
func NewCommandList(face render.Font, groups ...CommandGroup) *Command {
	c := &Command{face: requireFace("widget.NewCommandList", face)}
	c.build(nil, groups)
	return c
}

// build assembles the command under search, nil for none, over groups.
func (c *Command) build(search *Box, groups []CommandGroup) {
	c.column, c.list = NewBox(Column, 0, 0), NewBox(Column, 0, 4)
	c.SetElement("command")
	c.empty = NewLabel(c.face, commandTextSize, "No results found.", 0)
	c.empty.AddClass("empty")
	c.empty.SetAlignment(render.AlignCenter)
	c.scroll = NewScroll(c.list)
	c.scroll.VerticalOnly = true
	// A search palette holds cmdk's list height; a pick list is as tall
	// as its rows, scrolling only where its popover is held shorter.
	c.scroll.SetMaxContentHeight(0)
	if search != nil {
		c.scroll.SetMaxContentHeight(commandListHeight)
		c.column.Append(search, false)
	}
	c.column.Append(c.scroll, true)
	c.initComposite(c, c.column)
	c.boxed = true
	c.SetGroups(groups...)
}

// SetGroups replaces the groups and clears the query.
func (c *Command) SetGroups(groups ...CommandGroup) {
	c.headings, c.rows = c.headings[:0], c.rows[:0]
	for g, grp := range groups {
		h := NewLabel(c.face, 12, grp.Heading, 0)
		h.AddClass("heading")
		c.headings = append(c.headings, h)
		for _, it := range grp.Items {
			c.rows = append(c.rows, c.newRow(g, it))
		}
	}
	if c.input != nil {
		c.input.SetText("")
	}
	c.filter()
}

// newRow builds the row for item it of group g.
func (c *Command) newRow(g int, it CommandItem) *commandRow {
	r := &commandRow{Box: NewBox(Row, 8, 0), c: c, item: it, group: g}
	r.AddClass("item")
	r.SetPadding(render.Insets{Left: 8, Right: 8, Top: 6, Bottom: 6})
	r.SetFocusOnClick(false)
	if it.Icon != nil {
		r.AppendAligned(it.Icon, false, AlignCenter)
	}
	r.AppendAligned(NewLabel(c.face, commandTextSize, it.Label, 0), true, AlignCenter)
	if it.Shortcut != "" {
		sc := NewLabel(c.face, 12, it.Shortcut, 0)
		sc.AddClass("shortcut")
		r.AppendAligned(sc, false, AlignCenter)
	}
	if it.Trailing != nil {
		r.AppendAligned(it.Trailing, false, AlignCenter)
	}
	r.SetEnabled(!it.Disabled)
	return r
}

// SetEmptyText replaces the text shown when nothing matches.
func (c *Command) SetEmptyText(s string) { c.empty.SetText(s) }

// FocusTarget returns the widget the keyboard focus belongs on: the
// search entry, or the command itself when it has no search row.
func (c *Command) FocusTarget() Widget {
	if c.input == nil {
		return c
	}
	return c.input
}

// Query returns the search text, empty without a search row.
func (c *Command) Query() string {
	if c.input == nil {
		return ""
	}
	return c.input.Text()
}

// SetQuery replaces the search text and filters by it. A command
// without a search row has no query to set, and panics.
func (c *Command) SetQuery(q string) {
	if c.input == nil {
		panic("widget: Command.SetQuery on a NewCommandList, which has no search row")
	}
	c.input.SetText(q)
}

// score is how well the query matches it: its label's or best
// keyword's fuzzy score.
func score(query string, it CommandItem) (int, bool) {
	best, ok := fuzzyScore(query, it.Label)
	for _, k := range it.Keywords {
		if s, kok := fuzzyScore(query, k); kok && (!ok || s > best) {
			best, ok = s, true
		}
	}
	return best, ok
}

// filter shows the rows matching the query, best first within each
// group and the groups by their best match, and highlights the first.
func (c *Command) filter() {
	q := c.Query()
	c.shown = c.shown[:0]
	for _, r := range c.rows {
		if s, ok := score(q, r.item); ok {
			r.score = s
			c.shown = append(c.shown, r)
		}
	}
	best := make(map[int]int, len(c.headings))
	for _, r := range c.shown {
		if b, ok := best[r.group]; !ok || r.score > b {
			best[r.group] = r.score
		}
	}
	slices.SortStableFunc(c.shown, func(a, b *commandRow) int {
		if a.group != b.group {
			if best[a.group] != best[b.group] {
				return best[b.group] - best[a.group]
			}
			return a.group - b.group
		}
		return b.score - a.score
	})
	c.list.Clear()
	for i, r := range c.shown {
		if i == 0 || r.group != c.shown[i-1].group {
			if i > 0 {
				sep := NewSeparator(Horizontal)
				sep.AddClass("separator")
				c.list.Append(sep, false)
			}
			if h := c.headings[r.group]; h.Text() != "" {
				c.list.Append(h, false)
			}
		}
		c.list.Append(r, false)
	}
	if len(c.shown) == 0 {
		c.list.Append(c.empty, false)
	}
	c.moveHighlight(c.firstEnabled(0, 1))
}

// firstEnabled is the first shown row from i on in step's direction
// that can be picked, or -1.
func (c *Command) firstEnabled(i, step int) int {
	for ; i >= 0 && i < len(c.shown); i += step {
		if !c.shown[i].item.Disabled {
			return i
		}
	}
	return -1
}

// moveHighlight highlights shown row i (-1 for none) and scrolls it
// into view, now or, before the rows are laid out, at the next
// Arrange.
func (c *Command) moveHighlight(i int) {
	for j, r := range c.shown {
		r.SetState(StateSelected, j == i)
	}
	c.highlight = i
	c.revealPending = i >= 0 && c.shown[i].Bounds().Empty()
	if i >= 0 && !c.revealPending {
		row, start := c.highlightSpan()
		if start.Y+c.scroll.viewH >= row.Y+row.H {
			row = row.Union(start)
		}
		RevealRect(c.shown[i], row)
	}
}

// Arrange lays the command out and reveals a highlight set before its
// rows had a place.
func (c *Command) Arrange(r render.Rect) {
	c.composite.Arrange(r)
	if c.revealPending && c.highlight >= 0 && !c.shown[c.highlight].Bounds().Empty() {
		c.revealPending = false
		// Opening, the viewport starts at the highlight's group (its
		// separator and heading) when the row shows below it, else at
		// the row: no heading opens cut at the top.
		row, start := c.highlightSpan()
		top := row.Y
		if start.Y+c.scroll.viewH >= row.Y+row.H {
			top = start.Y
		}
		RevealRect(c.shown[c.highlight], render.Rect{X: row.X, Y: top, W: row.W, H: c.scroll.viewH})
	}
}

// highlightSpan is the highlighted row's bounds and those of its
// group's start: the separator and heading above the group's first
// row, or that row.
func (c *Command) highlightSpan() (row, start render.Rect) {
	r := c.shown[c.highlight]
	kids := c.list.Children()
	at := slices.Index(kids, Widget(r))
	for at > 0 {
		if prev, ok := kids[at-1].(*commandRow); ok && prev.group != r.group {
			break
		}
		at--
	}
	return r.Bounds(), boundsOf(kids[at])
}

// Highlighted returns the highlighted item and whether there is one.
func (c *Command) Highlighted() (CommandItem, bool) {
	if c.highlight < 0 {
		return CommandItem{}, false
	}
	return c.shown[c.highlight].item, true
}

// pick runs the highlighted item.
func (c *Command) pick() {
	if it, ok := c.Highlighted(); ok && it.OnSelect != nil {
		it.OnSelect()
	}
}

// highlightRow highlights row n in the order the groups listed it,
// -1 for none.
func (c *Command) highlightRow(n int) {
	if n < 0 {
		c.moveHighlight(-1)
		return
	}
	c.moveHighlight(slices.Index(c.shown, c.rows[n]))
}

// navigate moves the highlight on Up, Down, Home and End and picks on
// Enter, reporting whether a was one of them.
func (c *Command) navigate(a KeyAction) bool {
	var next int
	switch a {
	case KeyDown:
		next = c.firstEnabled(c.highlight+1, 1)
	case KeyUp:
		next = c.firstEnabled(c.highlight-1, -1)
	case KeyHome:
		next = c.firstEnabled(0, 1)
	case KeyEnd:
		next = c.firstEnabled(len(c.shown)-1, -1)
	case KeyEnter:
		c.pick()
		return true
	default:
		return false
	}
	if next >= 0 {
		c.moveHighlight(next)
	}
	return true
}

// InterceptKey implements KeyInterceptor: while the search entry has
// the focus, the navigation keys move the highlight and Enter picks.
func (c *Command) InterceptKey(target Widget, a KeyAction, _ Mods) bool {
	return c.input != nil && target == Widget(c.input) && c.navigate(a)
}

// KeyAction implements KeyActionHandler: a command without a search
// row holds the focus itself and navigates on its own keys.
func (c *Command) KeyAction(a KeyAction, _ Mods) { c.navigate(a) }

// InsertRune implements RuneHandler for a command without a search
// row: Space picks the highlighted item, the other half of the
// activation pair, and every other printable rune jumps the highlight
// by type-ahead over the pickable items.
func (c *Command) InsertRune(r rune) {
	if r == ' ' {
		c.pick()
		return
	}
	next, ok := c.typeAhead.next(r, c.highlight, len(c.shown), func(i int) (string, bool) {
		return c.shown[i].item.Label, !c.shown[i].item.Disabled
	})
	if ok {
		c.moveHighlight(next)
	}
}

// focusable implements focusOptOut: only a command without a search
// row takes the focus; with one, the entry holds it.
func (c *Command) focusable() bool { return c.input == nil }

// newCommandCheck returns an item's trailing check slot, 16px square,
// holding a check when on.
func newCommandCheck(on bool) *Box {
	slot := NewBox(Column, 0, 0)
	slot.SetSizeRequest(16, 16)
	setCommandCheck(slot, on)
	return slot
}

// setCommandCheck shows or clears the check in slot.
func setCommandCheck(slot *Box, on bool) {
	slot.Clear()
	if on {
		slot.Append(NewSymbol(SymbolCheck, 16), true)
	}
}
