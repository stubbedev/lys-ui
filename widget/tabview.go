package widget

import (
	"slices"
	"sync/atomic"

	"github.com/stubbedev/lys-lib/render"
)

// TabPage is one page of a TabView: its child and what its tab shows.
type TabPage struct {
	view      *TabView
	id        uint64
	child     Widget
	title     string
	tooltip   string
	icon      string
	indicator string
	loading   bool
	attention bool
	pinned    bool
	// OnIndicator runs when the tab's indicator icon is clicked (a mute
	// button on a playing tab).
	OnIndicator func()
}

// Child returns the page's content.
func (p *TabPage) Child() Widget { return p.child }

// Title returns the tab title.
func (p *TabPage) Title() string { return p.title }

// SetTitle sets the tab title.
func (p *TabPage) SetTitle(s string) { p.set(&p.title, s) }

// SetTooltip sets the tab's tooltip.
func (p *TabPage) SetTooltip(s string) { p.set(&p.tooltip, s) }

// SetIcon sets the tab's icon by theme name; "" shows none.
func (p *TabPage) SetIcon(name string) { p.set(&p.icon, name) }

// SetIndicator sets the indicator icon shown beside the title by theme
// name; "" shows none. OnIndicator hears clicks on it.
func (p *TabPage) SetIndicator(name string) { p.set(&p.indicator, name) }

// SetLoading shows a spinner in place of the icon while on.
func (p *TabPage) SetLoading(on bool) { p.flag(&p.loading, on) }

// SetNeedsAttention marks the tab until it is selected.
func (p *TabPage) SetNeedsAttention(on bool) { p.flag(&p.attention, on) }

// Pinned reports whether the page is pinned.
func (p *TabPage) Pinned() bool { return p.pinned }

// Loading reports the loading state.
func (p *TabPage) Loading() bool { return p.loading }

// NeedsAttention reports the attention mark.
func (p *TabPage) NeedsAttention() bool { return p.attention }

// View returns the TabView holding the page, nil once closed.
func (p *TabPage) View() *TabView { return p.view }

func (p *TabPage) set(field *string, v string) {
	if *field != v {
		*field = v
		p.changed()
	}
}

func (p *TabPage) flag(field *bool, v bool) {
	if *field != v {
		*field = v
		p.changed()
	}
}

func (p *TabPage) changed() {
	if p.view != nil {
		p.view.notify()
	}
}

var tabPageIDs atomic.Uint64

var tabPages = map[uint64]*TabPage{}

// TabView is libadwaita's AdwTabView: a stack of pages, one visible,
// whose tabs a TabBar (and a TabOverview) present. Pinned pages sit
// first. Closing asks OnClosePage, which may veto.
type TabView struct {
	node
	pages     []*TabPage
	selected  *TabPage
	listeners []func()

	// OnSelect runs when the selected page changes.
	OnSelect func(p *TabPage)
	// OnClosePage, when set, decides whether p closes: return false to
	// keep it (an unsaved-changes prompt).
	OnClosePage func(p *TabPage) bool
	// OnPageAttached and OnPageDetached run when a page joins or leaves
	// this view, by insertion, close or a transfer between views.
	OnPageAttached func(p *TabPage)
	OnPageDetached func(p *TabPage)
}

// NewTabView returns an empty tab view.
func NewTabView() *TabView {
	v := &TabView{}
	v.SetElement("tabview")
	return v
}

// Pages returns the pages in tab order.
func (v *TabView) Pages() []*TabPage { return slices.Clone(v.pages) }

// NPages returns the page count.
func (v *TabView) NPages() int { return len(v.pages) }

// NPinned returns how many pages are pinned.
func (v *TabView) NPinned() int {
	n := 0
	for _, p := range v.pages {
		if p.pinned {
			n++
		}
	}
	return n
}

// Selected returns the visible page, nil when empty.
func (v *TabView) Selected() *TabPage { return v.selected }

// Index returns p's position, -1 when p is not in this view.
func (v *TabView) Index(p *TabPage) int { return slices.Index(v.pages, p) }

// Append adds child as a new last unpinned page titled title.
func (v *TabView) Append(child Widget, title string) *TabPage {
	return v.Insert(len(v.pages), child, title)
}

// Prepend adds child as the first unpinned page.
func (v *TabView) Prepend(child Widget, title string) *TabPage {
	return v.Insert(v.NPinned(), child, title)
}

// Insert adds child at position i, kept among the unpinned pages.
func (v *TabView) Insert(i int, child Widget, title string) *TabPage {
	p := &TabPage{id: tabPageIDs.Add(1), child: child, title: title}
	tabPages[p.id] = p
	v.attach(p, i)
	return p
}

func (v *TabView) attach(p *TabPage, i int) {
	lo, hi := v.NPinned(), len(v.pages)
	if p.pinned {
		lo, hi = 0, v.NPinned()
	}
	i = min(max(i, lo), hi)
	p.view = v
	v.pages = slices.Insert(v.pages, i, p)
	setParents(v, p.child)
	if v.selected == nil {
		v.selected = p
	}
	v.InvalidateLayout()
	if v.OnPageAttached != nil {
		v.OnPageAttached(p)
	}
	v.notify()
}

func (v *TabView) detach(p *TabPage) {
	i := v.Index(p)
	if i < 0 {
		return
	}
	notifyRemoved(p.child)
	v.pages = slices.Delete(v.pages, i, i+1)
	clearParents(p.child)
	p.view = nil
	if v.selected == p {
		v.selected = nil
		if len(v.pages) > 0 {
			v.selectPage(v.pages[min(i, len(v.pages)-1)])
		}
	}
	v.InvalidateLayout()
	if v.OnPageDetached != nil {
		v.OnPageDetached(p)
	}
	v.notify()
}

// ClosePage closes p unless OnClosePage vetoes, and reports whether it
// closed. The page after it, else before it, becomes visible.
func (v *TabView) ClosePage(p *TabPage) bool {
	if v.Index(p) < 0 || (v.OnClosePage != nil && !v.OnClosePage(p)) {
		return false
	}
	v.detach(p)
	delete(tabPages, p.id)
	return true
}

// Select shows p.
func (v *TabView) Select(p *TabPage) {
	if v.Index(p) >= 0 {
		v.selectPage(p)
	}
}

func (v *TabView) selectPage(p *TabPage) {
	if v.selected == p {
		return
	}
	v.selected = p
	p.attention = false
	v.InvalidateLayout()
	if v.OnSelect != nil {
		v.OnSelect(p)
	}
	v.notify()
}

// SelectIndex shows the page at i; out of range does nothing.
func (v *TabView) SelectIndex(i int) {
	if i >= 0 && i < len(v.pages) {
		v.selectPage(v.pages[i])
	}
}

// SelectNext shows the next page, wrapping to the first.
func (v *TabView) SelectNext() { v.step(1) }

// SelectPrevious shows the previous page, wrapping to the last.
func (v *TabView) SelectPrevious() { v.step(-1) }

func (v *TabView) step(d int) {
	if len(v.pages) == 0 {
		return
	}
	i := v.Index(v.selected)
	v.selectPage(v.pages[(i+d+len(v.pages))%len(v.pages)])
}

// Reorder moves p to position i, kept within its pinned or unpinned
// run.
func (v *TabView) Reorder(p *TabPage, i int) {
	from := v.Index(p)
	if from < 0 {
		return
	}
	v.pages = slices.Delete(v.pages, from, from+1)
	lo, hi := v.NPinned(), len(v.pages)
	if p.pinned {
		lo, hi = 0, v.NPinned()
	}
	v.pages = slices.Insert(v.pages, min(max(i, lo), hi), p)
	v.notify()
}

// SetPinned pins or unpins p: pinned pages move to the end of the
// pinned run, unpinned ones to the start of the rest.
func (v *TabView) SetPinned(p *TabPage, on bool) {
	if v.Index(p) < 0 || p.pinned == on {
		return
	}
	pin := v.NPinned()
	p.pinned = on
	if on {
		v.Reorder(p, pin)
	} else {
		v.Reorder(p, pin-1)
	}
}

// TransferPage moves p from its view into this one at position i,
// keeping its child, as a tab dragged between windows. The page is
// selected in its new view.
func (v *TabView) TransferPage(p *TabPage, i int) {
	if p.view == nil {
		return
	}
	if p.view == v {
		v.Reorder(p, i)
		return
	}
	p.view.detach(p)
	v.attach(p, i)
	v.selectPage(p)
}

// Subscribe registers fn for every change of pages, order, selection
// or page state: what a TabBar redraws from.
func (v *TabView) Subscribe(fn func()) (cancel func()) {
	v.listeners = append(v.listeners, fn)
	i := len(v.listeners) - 1
	return func() { v.listeners[i] = nil }
}

func (v *TabView) notify() {
	for _, fn := range slices.Clone(v.listeners) {
		if fn != nil {
			fn()
		}
	}
}

// Children exposes the visible page for traversal.
func (v *TabView) Children() []Widget {
	if v.selected == nil {
		return nil
	}
	return []Widget{v.selected.child}
}

func (v *TabView) appendStyleKids(buf []Widget) []Widget {
	for _, p := range v.pages {
		buf = append(buf, p.child)
	}
	return buf
}

// Measure is the widest page's request, so switching tabs does not
// resize the window.
func (v *TabView) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := v.measureHit(axis, forSize); ok {
		return sz
	}
	var req Sizes
	for _, p := range v.pages {
		req = req.widest(measureChild(v, p.child, axis, forSize))
	}
	return v.measureStore(axis, forSize, req)
}

// Arrange gives the visible page the whole rect.
func (v *TabView) Arrange(r render.Rect) {
	v.node.Arrange(r)
	for _, p := range v.pages {
		if p == v.selected {
			arrangeChild(p.child, r)
		} else {
			p.child.Arrange(render.Rect{})
		}
	}
}

// Paint paints the visible page.
func (v *TabView) Paint(cv *render.Canvas) {
	if v.selected != nil {
		PaintChild(cv, v.selected.child)
	}
}

// HitTest routes into the visible page.
func (v *TabView) HitTest(p Point) Widget {
	if v.selected != nil {
		if hit := HitChild(v.selected.child, p); hit != nil {
			return hit
		}
	}
	return v.HitLeaf(v, p)
}

func tabPageByID(id uint64) *TabPage { return tabPages[id] }
