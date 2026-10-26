package widget

import (
	"fmt"
	"strconv"

	"github.com/stubbedev/lys-lib/render"
)

// Pagination steps through numbered pages:
// Previous, the page links around the current one with the first and
// last always shown and ellipses for the gaps, and Next. Pages count
// from 1. It styles as the `pagination` element: each link a
// button.page-link, the current one :checked, the ends button.previous
// and button.next, the gaps .ellipsis.
type Pagination struct {
	composite
	row      *Box
	face     render.Font
	count    int
	page     int
	siblings int
	// items are the shown links (page numbers) and gaps (pageEllipsis);
	// links and labels hold each link's button and number, nil at gaps.
	items      []int
	links      []*Button
	labels     []*Label
	prev, next *Button

	// OnPageChanged fires with the new page after the user moved to
	// it, not after SetPage.
	OnPageChanged func(page int)
}

// paginationSize is the text size.
const paginationSize = 14

// NewPagination returns a pagination over count pages at page, with
// one link on each side of the current page. count is at least 1 and
// page within 1..count, or the call panics.
func NewPagination(face render.Font, count, page int) *Pagination {
	p := &Pagination{row: NewBox(Row, 4, 0), face: requireFace("widget.NewPagination", face), siblings: 1}
	p.SetElement("pagination")
	p.initComposite(p, p.row)
	p.boxed = true
	p.SetCount(count, page)
	return p
}

// SetCount replaces the page count and the current page; count is at
// least 1 and page within 1..count, or the call panics.
func (p *Pagination) SetCount(count, page int) {
	if count < 1 || page < 1 || page > count {
		panic(fmt.Sprintf("widget: Pagination.SetCount(%d, %d): want count >= 1 and page in 1..count", count, page))
	}
	p.count, p.page = count, page
	p.rebuild()
}

// Count returns the page count.
func (p *Pagination) Count() int { return p.count }

// SetPage moves to page, within 1..count or the call panics.
func (p *Pagination) SetPage(page int) { p.SetCount(p.count, page) }

// Page returns the current page.
func (p *Pagination) Page() int { return p.page }

// SetSiblings sets how many links show on each side of the current
// page, at least 0.
func (p *Pagination) SetSiblings(n int) {
	p.siblings = max(0, n)
	p.rebuild()
}

// pageEllipsis marks a gap in paginationItems.
const pageEllipsis = 0

// paginationItems lists the links to show for count pages at page with
// siblings on each side: the first and last pages, the current page's
// neighborhood, and pageEllipsis for each gap of two or more pages (a
// gap of one shows that page instead).
func paginationItems(count, page, siblings int) []int {
	lo, hi := max(1, page-siblings), min(count, page+siblings)
	items := make([]int, 0, hi-lo+5)
	appendGap := func(from, to int) {
		switch {
		case to-from == 1:
			items = append(items, from)
		case to-from > 1:
			items = append(items, pageEllipsis)
		}
	}
	if lo > 1 {
		items = append(items, 1)
		appendGap(2, lo)
	}
	for i := lo; i <= hi; i++ {
		items = append(items, i)
	}
	if hi < count {
		appendGap(hi+1, count)
		items = append(items, count)
	}
	return items
}

// rebuild brings the row up to date with the count and page. When the
// links and gaps keep their places it updates them in place, so a
// focused link or step keeps the focus; otherwise it lays the links
// out afresh.
func (p *Pagination) rebuild() {
	items := paginationItems(p.count, p.page, p.siblings)
	if p.prev == nil {
		p.prev = p.step("previous", SymbolChevronLeft, "Previous", "Go to previous page", -1)
		p.next = p.step("next", SymbolChevronRight, "Next", "Go to next page", 1)
	}
	if !sameGaps(p.items, items) {
		p.layout(items)
	}
	p.items = items
	for i, n := range items {
		if n == pageEllipsis {
			continue
		}
		p.labels[i].SetText(strconv.Itoa(n))
		p.links[i].SetAccessibleName(fmt.Sprintf("Go to page %d", n))
		p.links[i].SetState(StateChecked, n == p.page)
	}
	p.prev.SetEnabled(p.page > 1)
	p.next.SetEnabled(p.page < p.count)
	p.InvalidateLayout()
}

// sameGaps reports whether a and b put links and gaps in the same
// places.
func sameGaps(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] == pageEllipsis) != (b[i] == pageEllipsis) {
			return false
		}
	}
	return true
}

// layout builds a link or a gap for each of items between the steps.
func (p *Pagination) layout(items []int) {
	p.row.Clear()
	p.links = make([]*Button, len(items))
	p.labels = make([]*Label, len(items))
	p.row.AppendAligned(p.prev, false, AlignCenter)
	for i, n := range items {
		if n == pageEllipsis {
			gap := NewBox(Column, 0, 0)
			gap.AddClass("ellipsis")
			gap.SetSizeRequest(36, 36)
			gap.Append(NewSymbol(SymbolEllipsis, 16), true)
			p.row.AppendAligned(gap, false, AlignCenter)
			continue
		}
		p.labels[i] = NewLabel(p.face, paginationSize, "", 0)
		p.labels[i].SetAlignment(render.AlignCenter)
		p.links[i] = NewButton(p.labels[i], 0, 0)
		p.links[i].AddClass("page-link")
		p.links[i].OnClick = func() { p.moveTo(p.items[i]) }
		p.row.AppendAligned(p.links[i], false, AlignCenter)
	}
	p.row.AppendAligned(p.next, false, AlignCenter)
}

// step builds the Previous or Next button, moving by delta.
func (p *Pagination) step(class string, icon SymbolKind, text, name string, delta int) *Button {
	row := NewBox(Row, 4, 0)
	chevron := NewSymbol(icon, 16)
	label := NewLabel(p.face, paginationSize, text, 0)
	if delta < 0 {
		row.AppendAligned(chevron, false, AlignCenter).AppendAligned(label, false, AlignCenter)
	} else {
		row.AppendAligned(label, false, AlignCenter).AppendAligned(chevron, false, AlignCenter)
	}
	b := NewButton(row, 0, 0)
	b.AddClass(class)
	b.SetAccessibleName(name)
	b.OnClick = func() { p.moveTo(p.page + delta) }
	return b
}

// moveTo goes to page on the user's behalf and reports it.
func (p *Pagination) moveTo(page int) {
	if page == p.page || page < 1 || page > p.count {
		return
	}
	p.SetPage(page)
	if p.OnPageChanged != nil {
		p.OnPageChanged(page)
	}
}
