package widget

import (
	"math"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

func newNotebook(t *testing.T) *Notebook {
	t.Helper()
	n := NewNotebook(entryFace(t))
	for _, name := range []string{"one", "two", "three"} {
		n.AppendTab(name, newTextArea(t, "page "+name))
	}
	n.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 300})
	return n
}

// TestNotebookSelect pins page switching: select shows the named page,
// unknown names are no-ops, and OnSelect fires only on real changes.
func TestNotebookSelect(t *testing.T) {
	n := newNotebook(t)

	t.Run("select switches the visible child", func(t *testing.T) {
		kids := n.Children()
		if len(kids) != 1 {
			t.Fatalf("children = %d, want exactly the visible page", len(kids))
		}
		n.SelectTab("two")
		kids = n.Children()
		if len(kids) != 1 || kids[0] != n.tabs[1].w {
			t.Errorf("children did not switch to page two")
		}
	})

	t.Run("selecting an unknown tab is a no-op", func(t *testing.T) {
		n.SelectTab("two")
		n.SelectTab("nope")
		if got := n.SelectedTab(); got != "two" {
			t.Errorf("selected = %q, want two after unknown no-op", got)
		}
	})

	t.Run("OnSelect fires on change and not on re-select", func(t *testing.T) {
		fired := 0
		n.OnSelect = func(string) { fired++ }
		n.SelectTab("three")
		n.SelectTab("three")
		if fired != 1 {
			t.Errorf("OnSelect fired %d times, want 1", fired)
		}
	})
}

func TestNotebookPaintAndHit(t *testing.T) {
	n := newNotebook(t)
	cv := render.New(make([]uint8, 400*300*4), 400*4, 400, 300)

	t.Run("paint runs with a selected page", func(t *testing.T) {
		n.SelectTab("one")
		n.Paint(cv)
	})

	t.Run("tab strip hit selects on click", func(t *testing.T) {
		// Third tab strip: x = 2*tabWidth, inside the bar height.
		p := Point{X: 2*tabWidth + 10, Y: 10}
		if got := n.HitTest(p); got != Widget(n) {
			t.Errorf("tab strip hit = %v, want the notebook", got)
		}
		n.ClickAt(p)
		if got := n.SelectedTab(); got != "three" {
			t.Errorf("selected = %q, want three", got)
		}
	})

	t.Run("close box fires the close hook instead of selecting", func(t *testing.T) {
		n.Closable = true
		closed := ""
		n.OnTabClose = func(name string) { closed = name }
		rect := n.tabRect(1)
		n.ClickAt(Point{X: rect.X + rect.W - 10, Y: rect.Y + 12})
		if closed != "two" {
			t.Errorf("OnTabClose = %q, want two", closed)
		}
		if got := n.SelectedTab(); got == "two" {
			t.Error("close box selected the tab instead of closing")
		}
	})
}

// TestNotebookTraversal pins that keyboard traversal only ever sees
// the visible page, and ctrl+PageUp/PageDown cycle tabs.
func TestNotebookTraversal(t *testing.T) {
	n := newNotebook(t)
	n.SelectTab("two")

	var visited []string
	walker := func(w Widget) {
		if e, ok := w.(*TextArea); ok {
			visited = append(visited, e.Text())
		}
	}
	focusWalker(n, walker)
	if len(visited) != 1 || visited[0] != "page two" {
		t.Errorf("traversal visited %v, want only the visible page", visited)
	}

	n.KeyAction(KeyPriorPage, ModCtrl)
	if got := n.SelectedTab(); got != "one" {
		t.Errorf("after ctrl+PageUp selected = %q, want one", got)
	}
	n.KeyAction(KeyNextPage, ModCtrl)
	n.KeyAction(KeyNextPage, ModCtrl)
	if got := n.SelectedTab(); got != "three" {
		t.Errorf("after ctrl+PageDown x2 selected = %q, want three", got)
	}
}

// The tab bar styles as `notebook > header > tabs > tab`: the header
// and each tab take their own backgrounds, the selected tab is
// :checked, and its label takes the tab's color.
func TestNotebookTabNodes(t *testing.T) {
	loadCSS(t, `notebook header { background-color: #010203; } notebook header tabs tab { color: #0a0b0c; } notebook header tabs tab:checked { background-color: #0d0e0f; color: #112233; }`)
	n := NewNotebook(testFace(t))
	n.AppendTab("one", NewSpacer(20, 20))
	n.AppendTab("two", NewSpacer(20, 20))
	host := NewBox(Column, 0, 0)
	host.Append(n, true)
	frame(t, host, 200, 60)
	if got := n.header.style(&n.header).Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("header background %v, want the header rule", got)
	}
	first, second := n.header.tabs.tab[0], n.header.tabs.tab[1]
	if got := first.style(first).Background; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("selected tab background %v, want the :checked rule", got)
	}
	if got := pickc(0, first.style(first), style.PropColor, 0); got != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("selected tab color %v, want the :checked rule", got)
	}
	if !first.HasState(StateChecked) || second.HasState(StateChecked) {
		t.Errorf("checked states: first %v second %v", first.HasState(StateChecked), second.HasState(StateChecked))
	}
	if got := second.style(second).Background; got != 0 {
		t.Errorf("unselected tab background %v, want none", got)
	}
	n.SelectTab("two")
	if second.style(second).Background != render.RGB(0x0d, 0x0e, 0x0f) || first.style(first).Background != 0 {
		t.Errorf("after the move: first %v second %v", first.style(first).Background, second.style(second).Background)
	}
}

// A tab label widget styles as the tab's child, fills the tab's content
// box, and sizes the tab; the header, tab row, and tabs are CSS boxes
// the pages sit below. Without a stylesheet min size a tab keeps the
// 96x26 floor, and nil restores the painted name.
func TestNotebookTabLabelsAndBoxes(t *testing.T) {
	loadCSS(t, `notebook header { margin-bottom: 6px; }
		notebook header tabs { padding: 4px; border: 1px solid #ffffff; }
		notebook header tabs tab { min-width: 0; min-height: 0; padding: 2px 10px; }
		notebook header tabs tab:not(:last-child) { margin-right: 8px; }
		tab label.tl { color: #112233; }
		tab:checked label.tl { color: #445566; }`)
	face := testFace(t)
	n := NewNotebook(face)
	page := NewSpacer(20, 20)
	n.AppendTab("one", page)
	n.AppendTab("two", NewSpacer(20, 20))
	label := NewLabel(face, 12, "One", 0)
	label.AddClass("tl")
	n.SetTabLabel("one", label)
	n.SetTabLabel("nope", NewLabel(face, 12, "x", 0))
	if n.TabLabel("one") != Widget(label) || n.TabLabel("two") != nil || n.TabLabel("nope") != nil {
		t.Errorf("TabLabel: one %v, two %v, nope %v", n.TabLabel("one"), n.TabLabel("two"), n.TabLabel("nope"))
	}
	host := NewBox(Column, 0, 0)
	host.Append(n, true)
	frame(t, host, 300, 100)

	nat := naturalSize(label)
	first, second := n.tabRect(0), n.tabRect(1)
	want := render.Rect{X: 5, Y: 5, W: nat.W + 20, H: first.H}
	if first != want {
		t.Errorf("first tab %v, want %v (the label plus padding, inside the tabs box)", first, want)
	}
	text := n.face.Shape("two", tabTextPx)
	if second.X != first.X+first.W+8 || second.W != int(math.Ceil(text.Advance()))+20 {
		t.Errorf("second tab %v after %v: want the margin gap and the painted name's width", second, first)
	}
	if got := label.Bounds(); got != (render.Rect{X: first.X + 10, Y: first.Y + 2, W: first.W - 20, H: first.H - 4}) {
		t.Errorf("label at %v, want the content box of %v", got, first)
	}
	if h := n.strip.height; h != 6+2+8+first.H || page.Bounds().Y != h {
		t.Errorf("bar height %d, page at y=%d, want 16 + the row %d", h, page.Bounds().Y, first.H)
	}
	// The tabs box fits its tabs (no tab expands): the two tabs, the
	// margin between them, the 4px padding and 1px border each side.
	if want := first.W + 8 + second.W + 2*(4+1); n.strip.tabs.W != want {
		t.Errorf("tabs box %v, want it fitting its tabs, %d wide", n.strip.tabs, want)
	}
	if got := pickc(0, label.style(label), style.PropColor, 0); got != render.RGB(0x44, 0x55, 0x66) {
		t.Errorf("selected tab's label color %v, want the tab:checked label rule", got)
	}
	n.SelectTab("two")
	if got := pickc(0, label.style(label), style.PropColor, 0); got != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("unselected tab's label color %v, want the tab label rule", got)
	}

	n.SetTabLabel("one", nil)
	frame(t, host, 300, 100)
	if n.TabLabel("one") != nil || label.Parent() != nil || n.tabRect(0).W != int(math.Ceil(n.face.Shape("one", tabTextPx).Advance()))+20 {
		t.Errorf("after nil: label parent %v, tab %v, want the painted name back", label.Parent(), n.tabRect(0))
	}

	LoadStylesheet("")
	bareTheme(t)
	plain := NewNotebook(face)
	plain.AppendTab("a", NewSpacer(10, 10))
	frame(t, plain, 300, 100)
	if got := plain.tabRect(0); got.W != tabWidth || got.H != tabBarHeight || plain.strip.height != tabBarHeight {
		t.Errorf("unstyled tab %v, bar %d, want the %dx%d floor", got, plain.strip.height, tabWidth, tabBarHeight)
	}
}

// The tab under the pointer is :hover and alone; its label's cursor is
// the notebook's while it is hovered, and leaving clears both.
func TestNotebookTabHoverAndCursor(t *testing.T) {
	loadCSS(t, `notebook header tabs tab:hover { background-color: #102030; }`)
	face := testFace(t)
	n := NewNotebook(face)
	n.AppendTab("one", NewSpacer(20, 20))
	n.AppendTab("two", NewSpacer(20, 20))
	label := NewLabel(face, 12, "One", 0)
	label.SetCursorName("pointer")
	n.SetTabLabel("one", label)
	frame(t, n, 300, 100)
	center := func(r render.Rect) Point { return Point{X: r.X + r.W/2, Y: r.Y + r.H/2} }
	hovered := func(i int) bool {
		p := n.tabs[i].part
		return p.style(p).Background == render.RGB(0x10, 0x20, 0x30)
	}

	n.HoverMove(center(n.tabRect(0)))
	if !hovered(0) || hovered(1) || n.CursorName() != "pointer" {
		t.Errorf("over the first tab: hover %v/%v, cursor %q", hovered(0), hovered(1), n.CursorName())
	}
	n.HoverMove(center(n.tabRect(1)))
	if hovered(0) || !hovered(1) || n.CursorName() != "" {
		t.Errorf("over the second tab: hover %v/%v, cursor %q", hovered(0), hovered(1), n.CursorName())
	}
	n.SetHovered(false)
	if hovered(0) || hovered(1) {
		t.Error("leaving the notebook left a tab hovered")
	}
}

// The accent underline is the theme's fallback: a selected tab the
// stylesheet gives a background paints that background to its edge.
func TestNotebookUnderlineIsAFallback(t *testing.T) {
	bareTheme(t)
	bottom := func() render.Color {
		n := NewNotebook(testFace(t))
		n.AppendTab("one", NewSpacer(20, 20))
		data := paintPixels(t, n, 300, 80)
		r := n.tabRect(0)
		return pxAt(data, 300, r.X+r.W/2, r.Y+r.H-1)
	}
	if got := bottom(); got != Current().Accent {
		t.Errorf("unstyled selected tab's bottom edge %v, want the accent underline", got)
	}
	loadCSS(t, `notebook header tabs tab:checked { background-color: #0d0e0f; }`)
	if got := bottom(); got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("styled selected tab's bottom edge %v, want its background", got)
	}
}

// Tabs are siblings under the tab row: structural selectors pick the
// first, the last, and the nth, not every tab as an only child.
func TestNotebookTabsAreSiblings(t *testing.T) {
	loadCSS(t, `notebook header tabs tab:first-child { color: #010101; }
		notebook header tabs tab:last-child { color: #030303; }
		notebook header tabs tab:nth-child(2) { color: #020202; }`)
	n := NewNotebook(testFace(t))
	for _, name := range []string{"a", "b", "c"} {
		n.AppendTab(name, NewSpacer(10, 10))
	}
	frame(t, n, 400, 60)
	for i, want := range []render.Color{render.RGB(1, 1, 1), render.RGB(2, 2, 2), render.RGB(3, 3, 3)} {
		p := n.tabs[i].part
		if got := pickc(0, p.style(p), style.PropColor, 0); got != want {
			t.Errorf("tab %d color %v, want %v", i, got, want)
		}
	}
	n.MoveTab(2, 0)
	frame(t, n, 400, 60)
	if p := n.tabs[0].part; pickc(0, p.style(p), style.PropColor, 0) != render.RGB(1, 1, 1) {
		t.Error("a tab moved to the front did not restyle as the first child")
	}
}
