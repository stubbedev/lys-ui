package widget

import (
	"slices"

	"github.com/stubbedev/lys-lib/render"
)

// Breadcrumb is the path to the current page:
// a link per ancestor, chevrons between, and the current page last,
// not a link. A path longer than SetMaxItems collapses its middle into
// an ellipsis that opens the hidden ancestors as a menu. It styles as
// the `breadcrumb` element: each link a button.crumb, the current page
// a .page label, the separators .separator and the ellipsis
// button.ellipsis.
type Breadcrumb struct {
	composite
	row      *Box
	face     render.Font
	path     []string
	maxItems int

	// OnNavigate fires with the index into the path of the ancestor the
	// user activated.
	OnNavigate func(i int)
}

// breadcrumbSize is the text size.
const breadcrumbSize = 14

// minCollapsed is the fewest items a collapsed path shows: the first
// ancestor, the ellipsis and the current page.
const minCollapsed = 3

// NewBreadcrumb returns a breadcrumb over path, the root first and the
// current page last.
func NewBreadcrumb(face render.Font, path ...string) *Breadcrumb {
	b := &Breadcrumb{row: NewBox(Row, 6, 0), face: requireFace("widget.NewBreadcrumb", face)}
	b.SetElement("breadcrumb")
	b.initComposite(b, b.row)
	b.boxed = true
	b.SetPath(path...)
	return b
}

// SetPath replaces the path.
func (b *Breadcrumb) SetPath(path ...string) {
	b.path = slices.Clone(path)
	b.rebuild()
}

// Path returns the path.
func (b *Breadcrumb) Path() []string { return slices.Clone(b.path) }

// SetMaxItems collapses a path of more than n items: the first, an
// ellipsis for the middle, and the last n-2. Below 3, or 0, the whole
// path shows.
func (b *Breadcrumb) SetMaxItems(n int) {
	b.maxItems = n
	b.rebuild()
}

// hidden is the range of path indexes the ellipsis stands for; empty
// when the whole path shows.
func (b *Breadcrumb) hidden() (from, to int) {
	if b.maxItems < minCollapsed || len(b.path) <= b.maxItems {
		return 0, 0
	}
	return 1, len(b.path) - (b.maxItems - 2)
}

// rebuild lays the row out from the path.
func (b *Breadcrumb) rebuild() {
	b.row.Clear()
	from, to := b.hidden()
	for i, name := range b.path {
		switch {
		case i == from && to > from:
			b.appendSeparated(b.ellipsis(from, to))
		case i > from && i < to:
			continue
		case i == len(b.path)-1:
			page := NewLabel(b.face, breadcrumbSize, name, 0)
			page.AddClass("page")
			b.appendSeparated(page)
		default:
			b.appendSeparated(b.crumb(i, name))
		}
	}
	b.InvalidateLayout()
}

// appendSeparated appends w, after a separator unless it is first.
func (b *Breadcrumb) appendSeparated(w Widget) {
	if len(b.row.Children()) > 0 {
		sep := NewSymbol(SymbolChevronRight, 14)
		sep.AddClass("separator")
		b.row.AppendAligned(sep, false, AlignCenter)
	}
	b.row.AppendAligned(w, false, AlignCenter)
}

// crumb is the link to path[i].
func (b *Breadcrumb) crumb(i int, name string) *Button {
	link := NewButton(NewLabel(b.face, breadcrumbSize, name, 0), 0, 0)
	link.AddClass("crumb")
	link.OnClick = func() { b.navigate(i) }
	return link
}

// ellipsis is the button that opens path[from:to] as a menu.
func (b *Breadcrumb) ellipsis(from, to int) *Button {
	more := NewButton(NewSymbol(SymbolEllipsis, 16), 0, 0)
	more.AddClass("ellipsis")
	more.SetAccessibleName("Show more")
	more.OnClick = func() {
		items := make([]MenuItem, 0, to-from)
		for i := from; i < to; i++ {
			items = append(items, MenuItem{Label: b.path[i], OnClick: func() { b.navigate(i) }})
		}
		OpenMenu(more, more, b.face, breadcrumbSize, items, MenuOptions{Gap: PopoverGap})
	}
	return more
}

// navigate reports a jump to path[i].
func (b *Breadcrumb) navigate(i int) {
	if b.OnNavigate != nil {
		b.OnNavigate(i)
	}
}
