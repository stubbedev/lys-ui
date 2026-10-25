package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

func TestPaginationItems(t *testing.T) {
	for _, tc := range []struct {
		count, page, siblings int
		want                  []int
	}{
		{1, 1, 1, []int{1}},
		{5, 3, 1, []int{1, 2, 3, 4, 5}},
		{10, 1, 1, []int{1, 2, pageEllipsis, 10}},
		{10, 5, 1, []int{1, pageEllipsis, 4, 5, 6, pageEllipsis, 10}},
		{10, 3, 1, []int{1, 2, 3, 4, pageEllipsis, 10}},
		{10, 4, 1, []int{1, 2, 3, 4, 5, pageEllipsis, 10}},
		{10, 10, 1, []int{1, pageEllipsis, 9, 10}},
		{10, 5, 0, []int{1, pageEllipsis, 5, pageEllipsis, 10}},
	} {
		if got := paginationItems(tc.count, tc.page, tc.siblings); !slices.Equal(got, tc.want) {
			t.Errorf("paginationItems(%d, %d, %d) = %v, want %v", tc.count, tc.page, tc.siblings, got, tc.want)
		}
	}
}

// The steps move a page at a time, report it, and disable at the ends;
// a focused step keeps the focus across the move.
func TestPaginationSteps(t *testing.T) {
	p := NewPagination(goldenFace(t), 10, 1)
	var got []int
	p.OnPageChanged = func(page int) { got = append(got, page) }
	arrangeTree(t, p, 600, 60)
	if IsEnabled(p.prev) || !IsEnabled(p.next) {
		t.Fatal("on page 1 Previous is enabled or Next is not")
	}
	r := &Router{Root: p}
	r.SetFocus(p.next)
	for range 5 {
		r.KeyAction(KeyEnter, 0)
		arrangeTree(t, p, 600, 60)
	}
	if p.Page() != 6 || !slices.Equal(got, []int{2, 3, 4, 5, 6}) {
		t.Errorf("page %d, reported %v; want 6 after 2..6", p.Page(), got)
	}
	if r.Focused() != Widget(p.next) {
		t.Error("Next lost the focus while stepping")
	}
	p.SetPage(10)
	if IsEnabled(p.next) {
		t.Error("Next is enabled on the last page")
	}
	if len(got) != 5 {
		t.Error("SetPage reported a change")
	}
	for i, n := range p.items {
		if n == p.Page() && !p.links[i].HasState(StateChecked) {
			t.Error("the current page's link is not checked")
		}
	}
}

func TestPaginationRejectsBadPages(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("page 0 did not panic")
		}
	}()
	NewPagination(goldenFace(t), 3, 0)
}

// fakeSurfaceHost records the menus and popovers a tree opens and how
// many popovers it closed.
type fakeSurfaceHost struct {
	menus    [][]MenuItem
	popovers []Widget
	opts     []PopoverOptions
	closed   int
	// menuClosers are the menus' onClosed callbacks, which a test runs
	// to close them.
	menuClosers []func()
}

func (h *fakeSurfaceHost) Focus(Widget) {}

func (h *fakeSurfaceHost) OpenPopover(_ Boundser, content Widget, opts PopoverOptions) func() {
	h.popovers = append(h.popovers, content)
	h.opts = append(h.opts, opts)
	return func() { h.closed++ }
}

func (h *fakeSurfaceHost) OpenMenu(_ Boundser, _ render.Font, _ float64, items []MenuItem, onClosed func()) {
	h.menus = append(h.menus, items)
	h.menuClosers = append(h.menuClosers, onClosed)
}

// A long path collapses its middle into an ellipsis whose menu lists
// the hidden ancestors; links and menu rows report their index.
func TestBreadcrumbCollapse(t *testing.T) {
	b := NewBreadcrumb(goldenFace(t), "Home", "Docs", "Components", "Data", "Breadcrumb")
	var nav []int
	b.OnNavigate = func(i int) { nav = append(nav, i) }
	b.SetMaxItems(3)
	host := &fakeSurfaceHost{}
	SetSurfaceHost(b, host)
	arrangeTree(t, b, 600, 60)
	var crumbs, ellipses []*Button
	for _, w := range b.row.Children() {
		if btn, ok := w.(*Button); ok {
			if btn.HasClass("ellipsis") {
				ellipses = append(ellipses, btn)
			} else {
				crumbs = append(crumbs, btn)
			}
		}
	}
	if len(crumbs) != 1 || len(ellipses) != 1 {
		t.Fatalf("%d links and %d ellipses, want the first link and one ellipsis", len(crumbs), len(ellipses))
	}
	crumbs[0].OnClick()
	ellipses[0].OnClick()
	if len(host.menus) != 1 || len(host.menus[0]) != 3 || host.menus[0][0].Label != "Docs" {
		t.Fatalf("menus = %v, want Docs, Components and Data", host.menus)
	}
	host.menus[0][2].OnClick()
	if !slices.Equal(nav, []int{0, 3}) {
		t.Errorf("navigated %v, want 0 then 3", nav)
	}
	b.SetMaxItems(0)
	if len(b.row.Children()) != 9 {
		t.Errorf("uncollapsed row has %d children, want five items and four separators", len(b.row.Children()))
	}
}

// A popup opened in a tree no surface hosts is a caller bug.
func TestOpenMenuWithoutHostPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("OpenMenu without a host did not panic")
		}
	}()
	b := NewButton(NewSpacer(0, 0), 0, 0)
	OpenMenu(b, b, goldenFace(t), 14, nil, nil)
}

// SetAccessibleName outranks the tooltip and the content's name.
func TestAccessibleNameWins(t *testing.T) {
	b := NewButton(NewLabel(goldenFace(t), 14, "3", 0), 0, 0)
	b.SetTooltip("tip")
	b.SetAccessibleName("Go to page 3")
	if got := Describe(b).Name; got != "Go to page 3" {
		t.Errorf("name %q, want the accessible name", got)
	}
	b.SetAccessibleName("")
	if got := Describe(b).Name; got != "tip" {
		t.Errorf("name %q, want the tooltip once cleared", got)
	}
}

// TestGoldenShadcnNavigation pins the breadcrumb, full and collapsed,
// and the pagination, in both palettes.
func TestGoldenShadcnNavigation(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			col := NewBox(Column, 20, 0)
			col.Append(NewBreadcrumb(face, "Home", "Components", "Breadcrumb"), false)
			long := NewBreadcrumb(face, "Home", "Docs", "Components", "Data", "Breadcrumb")
			long.SetMaxItems(3)
			col.Append(long, false)
			col.Append(NewPagination(face, 10, 5), false)
			NewGolden(t, col, "shadcn-navigation-"+tc.name, goldenTheme(tc.th), goldenFrame(520, 150))
		})
	}
}
