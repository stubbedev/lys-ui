package widget

import (
	"strconv"
	"strings"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget/css"
)

type searchable interface {
	searchTerms() (title, subtitle string, ok bool)
}

// PreferencesView is the body of libadwaita's AdwPreferencesDialog:
// PreferencesPages behind a view switcher, a search that lists the
// matching rows of every page (title or subtitle, case-insensitive;
// rows opt out with SetSearchable), and subpages pushed over the
// pages with a back button. Activating a search result shows its page
// and scrolls the row into view.
type PreferencesView struct {
	composite
	face     render.Font
	sizePx   float64
	nav      *NavigationView
	stack    *Stack
	switcher *Box
	search   *SearchEntry
	results  *Box
	pages    []*PreferencesPage
	before   string
}

// NewPreferencesView returns an empty preferences view.
func NewPreferencesView(face render.Font, sizePx float64) *PreferencesView {
	v := &PreferencesView{face: requireFace("widget.NewPreferencesView", face), sizePx: sizePx, stack: NewStack(), switcher: NewBox(Row, 0, 0), results: NewBox(Column, 4, 12)}
	v.search = NewSearchEntry(face, sizePx, Tr("Search preferences"))
	v.search.OnSearchChanged = v.filter
	v.stack.Add("search", NewScroll(NewClamp(600, v.results)))
	header := NewBox(Column, 6, 6)
	header.Append(v.switcher, false)
	header.Append(v.search, false)
	body := NewBox(Column, 0, 0)
	body.Append(header, false)
	body.Append(v.stack, true)
	v.nav = NewNavigationView(face, sizePx, &NavigationPage{Title: Tr("Preferences"), W: body})
	v.initComposite(v, v.nav)
	v.SetElement("preferencesview")
	return v
}

// Add appends a page; the switcher shows once there are two.
func (v *PreferencesView) Add(p *PreferencesPage) {
	v.pages = append(v.pages, p)
	v.stack.Add(v.name(len(v.pages)-1), p)
	if len(v.pages) == 1 {
		v.stack.Show(v.name(0))
	}
	v.syncSwitcher()
}

// Pages returns the pages in order.
func (v *PreferencesView) Pages() []*PreferencesPage {
	return append([]*PreferencesPage(nil), v.pages...)
}

// ShowPage shows p.
func (v *PreferencesView) ShowPage(p *PreferencesPage) {
	for i, q := range v.pages {
		if q == p {
			v.stack.Show(v.name(i))
		}
	}
}

// VisiblePage returns the shown page, nil while searching.
func (v *PreferencesView) VisiblePage() *PreferencesPage {
	if i, err := strconv.Atoi(v.stack.Visible()); err == nil && i < len(v.pages) {
		return v.pages[i]
	}
	return nil
}

// Search runs a search for text, as typing in the search entry does;
// empty returns to the page shown before.
func (v *PreferencesView) Search(text string) {
	v.search.SetText(text)
	v.filter(text)
}

// PushSubpage pushes a page titled title over the preferences with a
// back button, AdwPreferencesDialog's push_subpage.
func (v *PreferencesView) PushSubpage(title string, content Widget) {
	v.nav.Push(&NavigationPage{Title: title, W: content})
}

// PopSubpage returns from the topmost subpage, reporting whether there
// was one.
func (v *PreferencesView) PopSubpage() bool { return v.nav.Pop() }

func (v *PreferencesView) name(i int) string { return strconv.Itoa(i) }

func (v *PreferencesView) syncSwitcher() {
	v.switcher.Clear()
	if len(v.pages) < 2 {
		return
	}
	titles, icons := map[string]string{}, map[string]string{}
	for i, p := range v.pages {
		titles[v.name(i)], icons[v.name(i)] = p.title, p.icon
	}
	v.switcher.AppendAligned(NewViewSwitcher(v.face, v.sizePx, v.stack, titles, icons), true, AlignCenter)
}

func (v *PreferencesView) filter(text string) {
	query := strings.ToLower(strings.TrimSpace(text))
	if query == "" {
		if v.stack.Visible() == "search" {
			v.stack.Show(v.before)
		}
		return
	}
	if v.stack.Visible() != "search" {
		v.before = v.stack.Visible()
	}
	v.results.Clear()
	for _, p := range v.pages {
		walkRows(p, func(row Widget, title, subtitle string) {
			if !strings.Contains(strings.ToLower(title), query) && !strings.Contains(strings.ToLower(subtitle), query) {
				return
			}
			result := NewActionRow(v.face, v.sizePx, title, joinPath(p.title, subtitle))
			result.SetActivatable(true)
			result.OnActivate = func() {
				v.search.SetText("")
				v.ShowPage(p)
				p.Reveal(row)
			}
			v.results.Append(result, false)
		})
	}
	if len(v.results.Children()) == 0 {
		none := NewLabel(v.face, v.sizePx, Tr("No results found"), 0)
		none.AddClass(css.DimLabel)
		v.results.AppendAligned(none, false, AlignCenter)
	}
	v.stack.Show("search")
}

func joinPath(page, subtitle string) string {
	switch {
	case page == "":
		return subtitle
	case subtitle == "":
		return page
	}
	return page + " › " + subtitle
}

func walkRows(w Widget, fn func(row Widget, title, subtitle string)) {
	walkWidgets(w, func(w Widget) bool {
		s, ok := w.(searchable)
		if !ok {
			return true
		}
		if title, subtitle, on := s.searchTerms(); on {
			fn(w, title, subtitle)
		}
		return false
	})
}
