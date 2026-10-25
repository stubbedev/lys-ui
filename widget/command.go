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

// commandTextSize is the item text size, shadcn's text-sm.
const commandTextSize = 14

// Command is a searchable list of actions (shadcn/ui's Command, after
// cmdk): a search entry over grouped items that filters them fuzzily
// as the query is typed, ranking the best matches first within each
// group and the groups by their best, with an empty state when nothing
// matches. Up, Down, Home and End move the highlight while the entry
// keeps the focus; Enter or a click picks the highlighted item. It
// works inline or as a popover's or dialog's content. It styles as the
// `command` element: the .input row, .heading labels, .item rows
// (:selected while highlighted), .shortcut labels, .separator lines and
// the .empty label.
type Command struct {
	composite
	face      render.Font
	column    *Box
	input     *Entry
	list      *Box
	scroll    *Scroll
	empty     *Label
	headings  []*Label
	rows      []*commandRow
	shown     []*commandRow
	highlight int
}

// NewCommand returns a command over groups whose search entry shows
// placeholder.
func NewCommand(face render.Font, placeholder string, groups ...CommandGroup) *Command {
	face = requireFace("widget.NewCommand", face)
	c := &Command{face: face, column: NewBox(Column, 0, 0), list: NewBox(Column, 0, 4)}
	c.SetElement("command")
	c.input = NewEntry(face, commandTextSize, 0)
	c.input.SetPlaceholder(placeholder)
	c.input.OnChanged = func(string) { c.filter() }
	search := NewBox(Row, 8, 0)
	search.AddClass("input")
	search.SetPadding(render.Insets{Left: 12, Right: 12})
	search.AppendAligned(NewSymbol(SymbolSearch, 16), false, AlignCenter)
	search.Append(c.input, true)
	c.empty = NewLabel(face, commandTextSize, "No results found.", 0)
	c.empty.AddClass("empty")
	c.empty.SetAlignment(render.AlignCenter)
	c.scroll = NewScroll(c.list)
	c.scroll.VerticalOnly = true
	c.scroll.SetMaxContentHeight(commandListHeight)
	c.column.Append(search, false).Append(c.scroll, true)
	c.initComposite(c, c.column)
	c.boxed = true
	c.SetGroups(groups...)
	return c
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
	c.input.SetText("")
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

// Entry returns the search entry, the widget to focus.
func (c *Command) Entry() *Entry { return c.input }

// Query returns the search text.
func (c *Command) Query() string { return c.input.Text() }

// SetQuery replaces the search text and filters by it.
func (c *Command) SetQuery(q string) { c.input.SetText(q) }

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
	q := c.input.Text()
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
// into view.
func (c *Command) moveHighlight(i int) {
	for j, r := range c.shown {
		r.SetState(StateSelected, j == i)
	}
	c.highlight = i
	if i >= 0 {
		RevealRect(c.shown[i], c.shown[i].Bounds())
	}
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

// InterceptKey implements KeyInterceptor: while the entry has the
// focus, the navigation keys move the highlight and Enter picks.
func (c *Command) InterceptKey(target Widget, a KeyAction, _ Mods) bool {
	if target != Widget(c.input) {
		return false
	}
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
