package widget

import (
	"strings"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// The search & completion widgets of #92: SearchEntry, SearchBar, and
// ComboEntry, all thin shapes over the Entry whose completion
// machinery lives in completion.go.

// searchDelay is the GTK search-changed default: the time the typing
// must settle before the search runs.
const searchDelay = 150 * time.Millisecond

// SearchEntry is the search preset over Entry: a leading search icon,
// a clear button while text exists, Esc clearing (an empty field
// passes Esc on), and OnSearchChanged - the delayed search-changed
// signal, fired once typing settles for the delay (SetDelay tunes it,
// zero fires synchronously). The entry's whole public surface rides
// along through the embedded field; wire the app's search to
// OnSearchChanged, not OnChanged.
type SearchEntry struct {
	*Entry

	// OnSearchChanged fires with the settled text - the delayed signal
	// a search wants, so a fast typist triggers one search, not one
	// per keystroke.
	OnSearchChanged func(text string)

	delay  time.Duration
	cancel anim.Cancel
}

// NewSearchEntry returns a search field with an optional placeholder.
func NewSearchEntry(face render.Font, sizePx float64, placeholder string) *SearchEntry {
	face = requireFace("widget.NewSearchEntry", face)
	s := &SearchEntry{Entry: NewEntry(face, sizePx, Current().Text), delay: searchDelay}
	s.SetPlaceholder(placeholder)
	s.SetLeadingIcon("system-search", sizePx, nil)
	s.OnEscape = func() bool {
		if s.Text() == "" {
			return false
		}
		s.SetText("")
		return true
	}
	s.OnChanged = s.textChanged
	return s
}

// SetDelay tunes the search-changed settle time; zero fires
// synchronously with every change.
func (s *SearchEntry) SetDelay(d time.Duration) {
	s.delay = max(0, d)
}

// textChanged refreshes the clear button and schedules the settled
// signal on the animation clock - a loop-driven timer, no goroutine,
// cancelled by the next keystroke.
func (s *SearchEntry) textChanged(text string) {
	if text == "" {
		s.SetTrailingIcon("", s.fontPx(), nil)
	} else if s.trailing == nil {
		s.SetTrailingIcon("edit-clear", s.fontPx(), func() { s.SetText("") })
	}
	s.fireChanged(text)
}

// fireChanged delivers the settled signal: immediately for a zero
// delay, otherwise on the animation clock's completion tick.
func (s *SearchEntry) fireChanged(text string) {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.delay == 0 {
		if s.OnSearchChanged != nil {
			s.OnSearchChanged(text)
		}
		return
	}
	s.cancel = anim.Start(s.delay, func(t float64) {
		if t >= 1 && s.OnSearchChanged != nil {
			s.OnSearchChanged(s.Text())
		}
	})
}

// SearchBar is the slide-down search container: a SearchEntry (the
// accessor returns it wired for focus) inside a Revealer that slides
// in from the top, keyed by SetSearchMode - the state an accel or the
// app's shortcut handling flips. The key capture itself stays the
// app's (accel groups, OnKey); the bar owns only the reveal.
type SearchBar struct {
	composite
	reveal *Revealer
	entry  *SearchEntry
}

// NewSearchBar returns a hidden bar with an empty search field; call
// Entry().SetPlaceholder on it for hint text.
func NewSearchBar(face render.Font, sizePx float64) *SearchBar {
	face = requireFace("widget.NewSearchBar", face)
	b := &SearchBar{}
	b.entry = NewSearchEntry(face, sizePx, "")
	inner := NewBox(Row, 0, 8)
	inner.Append(NewSpacer(0, 0), true)
	inner.AppendAligned(b.entry, false, AlignCenter)
	inner.Append(NewSpacer(0, 0), true)
	b.reveal = NewRevealer(inner)
	b.reveal.SetTransition(RevealSlideDown)
	b.reveal.SetDuration(200 * time.Millisecond)
	b.reveal.SetRevealed(false)
	b.initComposite(b, b.reveal)
	return b
}

// Entry returns the bar's search field.
func (b *SearchBar) Entry() *SearchEntry { return b.entry }

// SetSearchMode reveals or hides the bar; revealing keeps whatever
// text the field held.
func (b *SearchBar) SetSearchMode(on bool) { b.reveal.SetRevealed(on) }

// SearchMode reports whether the bar is showing.
func (b *SearchBar) SearchMode() bool { return b.reveal.Revealed() }

// ComboEntry is the editable dropdown, GTK's ComboBox-with-entry: an
// Entry whose completion lists the items (prefix match,
// case-insensitive), a trailing chevron toggling the full list, and
// OnSelected for the pick. Free text stays free - typing an unmatched
// value is valid input, exactly like the GTK original.
type ComboEntry struct {
	*Entry

	// OnSelected fires with the picked item (arrow keys, Enter, or a
	// click in the list) - a choice, unlike free typing.
	OnSelected func(item string)

	items []string
	all   bool
}

// NewComboEntry returns an editable dropdown over items, its text
// starting at selected ("" for empty).
func NewComboEntry(face render.Font, sizePx float64, items []string, selected string) *ComboEntry {
	face = requireFace("widget.NewComboEntry", face)
	c := &ComboEntry{Entry: NewEntry(face, sizePx, Current().Text), items: items}
	c.SetText(selected)
	c.Completion = c.match
	c.OnComplete = func(chosen string) {
		if c.OnSelected != nil {
			c.OnSelected(chosen)
		}
	}
	c.OnEscape = func() bool {
		c.all = false
		c.recomputeCompletion()
		return false
	}
	c.SetTrailingIcon("pan-down-symbolic", sizePx, func() { c.toggleAll() })
	return c
}

// SetItems replaces the item list; the field's text is untouched.
func (c *ComboEntry) SetItems(items []string) {
	c.items = items
	c.recomputeCompletion()
}

// match lists the items for a prefix: while the chevron has the full
// list open, everything; otherwise the case-insensitive prefix
// matches in item order.
func (c *ComboEntry) match(prefix string) []string {
	if c.all {
		return c.items
	}
	p := strings.ToLower(prefix)
	var out []string
	for _, item := range c.items {
		if strings.HasPrefix(strings.ToLower(item), p) {
			out = append(out, item)
		}
	}
	return out
}

// toggleAll flips the chevron's full-list mode and refreshes.
func (c *ComboEntry) toggleAll() {
	c.all = !c.all
	c.recomputeCompletion()
}
