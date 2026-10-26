package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/anim"
)

// adaptiveFace builds the test font.
func adaptiveFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestClamp pins the pure measure: a wide allocation centers the
// child at the cap, a narrow one passes through.
func TestClamp(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	child := NewLabel(face, 14, "x", th.Text)
	c := NewClamp(300, child)
	c.Measure(Constraints{Max: Size{W: 500, H: 40}})
	c.Arrange(render.Rect{X: 0, Y: 0, W: 500, H: 40})
	if got := child.Bounds(); got.X != 100 || got.W != 300 {
		t.Errorf("wide clamp = %+v, want centered 300", got)
	}
	c.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 40})
	if got := child.Bounds(); got.X != 0 || got.W != 200 {
		t.Errorf("narrow clamp = %+v, want full 200", got)
	}
}

// TestBreakpointBin pins the adw model: conditions evaluate on the
// allocated width, Apply and Unapply run on the transitions only,
// and repeated equal widths never re-run.
func TestBreakpointBin(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	child := NewLabel(face, 14, "x", th.Text)
	b := NewBreakpointBin(child)
	narrow, wide := 0, 0
	b.Add(Breakpoint{
		Max:     400,
		Apply:   func() { narrow++ },
		Unapply: func() { wide++ },
	})
	layout := func(w int) {
		b.Measure(Constraints{Max: Size{W: w, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: w, H: 100})
	}
	layout(600)
	if narrow != 0 || wide != 0 {
		t.Fatalf("wide start: narrow=%d wide=%d", narrow, wide)
	}
	layout(380)
	if narrow != 1 || wide != 0 {
		t.Fatalf("crossing down: narrow=%d wide=%d", narrow, wide)
	}
	layout(380)
	if narrow != 1 || wide != 0 {
		t.Fatalf("same width re-ran: narrow=%d wide=%d", narrow, wide)
	}
	layout(700)
	if narrow != 1 || wide != 1 {
		t.Fatalf("crossing up: narrow=%d wide=%d", narrow, wide)
	}
	if b.Width() != 700 {
		t.Errorf("reported width = %d", b.Width())
	}
}

// TestViewSwitcher pins the strip: one button per stack page, a
// click switches the stack, ReflectVisible repaints the checked
// states after a programmatic show, and the policies change the
// button content shape.
func TestViewSwitcher(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	page := func(s string) Widget { return NewLabel(face, 14, s, th.Text) }
	stack := NewStack()
	stack.Add("one", page("one"))
	stack.Add("two", page("two"))
	stack.Add("three", page("three"))
	v := NewViewSwitcher(face, 14, stack,
		map[string]string{"one": "First", "two": "Second"}, nil)
	if len(v.buttons) != 3 {
		t.Fatalf("buttons = %d", len(v.buttons))
	}
	v.buttons[1].OnClick()
	if stack.Visible() != "two" {
		t.Errorf("click switched to %q", stack.Visible())
	}
	stack.Show("three")
	v.ReflectVisible()
	v.buttons[0].OnClick()
	if stack.Visible() != "one" {
		t.Errorf("after reflect + click: %q", stack.Visible())
	}
	v.SetPolicy(SwitcherText)
	if len(v.buttons) != 3 {
		t.Error("policy rebuild lost buttons")
	}
}

// TestCarousel pins the paging: pages arrange side by side, a drag
// pans the offset clamped to the page range, release snaps to the
// nearest page (instant clock), and SetPage jumps.
func TestCarousel(t *testing.T) {
	defer anim.SetInstant(true)()
	face := adaptiveFace(t)
	th := DarkTheme()
	c := NewCarousel(face, 14)
	for i := range 3 {
		c.Append(NewLabel(face, 20, string(rune('A'+i)), th.Text))
	}
	c.Measure(Constraints{Max: Size{W: 200, H: 100}})
	c.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 100})
	first, _ := c.pages[0].(Boundser)
	second, _ := c.pages[1].(Boundser)
	if second.Bounds().X != first.Bounds().X+200 {
		t.Fatalf("pages not side by side: %+v %+v", first.Bounds(), second.Bounds())
	}

	// Drag half a page right-to-left; the release snaps to page 1.
	c.PressAt(Point{X: 150, Y: 50})
	c.DragMove(Point{X: 40, Y: 50})
	if c.offset <= 0 || c.offset >= 2 {
		t.Fatalf("drag offset = %v", c.offset)
	}
	c.PressEnd()
	if got := c.Page(); got != 1 {
		t.Errorf("snap landed on page %d, want 1", got)
	}

	// Clamped at the ends.
	c.PressAt(Point{X: 100, Y: 50})
	c.DragMove(Point{X: 500, Y: 50})
	if c.offset != 0 {
		t.Errorf("overscroll left: %v", c.offset)
	}
	c.PressEnd()

	// SetPage jumps and reports.
	paged := -1
	c.OnPage = func(i int) { paged = i }
	c.SetPage(2)
	if c.Page() != 2 || paged != 2 {
		t.Errorf("SetPage: page=%d fired=%d", c.Page(), paged)
	}
}

// TestCarouselNav pins the previous and next buttons: they step a page
// and disable at the ends, Left and Right on them step too, and the
// pages and buttons are the carousel's children.
func TestCarouselNav(t *testing.T) {
	defer anim.SetInstant(true)()
	face := adaptiveFace(t)
	c := NewCarousel(face, 14)
	for i := range 3 {
		c.Append(NewLabel(face, 20, string(rune('A'+i)), 0))
	}
	arrangeTree(t, c, 300, 120)
	prev, next := c.nav[0], c.nav[1]
	if IsEnabled(prev) || !IsEnabled(next) {
		t.Fatalf("on the first page: previous %v next %v, want only next enabled", IsEnabled(prev), IsEnabled(next))
	}
	center := func(b *Button) Point { r := b.Bounds(); return Point{X: r.X + r.W/2, Y: r.Y + r.H/2} }
	if hit := c.HitTest(center(next)); hit != Widget(next) {
		t.Fatalf("hit on the next button %T, want the button", hit)
	}
	next.OnClick()
	next.OnClick()
	if c.Page() != 2 || !IsEnabled(prev) || IsEnabled(next) {
		t.Errorf("after two nexts: page %d, previous %v next %v", c.Page(), IsEnabled(prev), IsEnabled(next))
	}
	if !c.InterceptKey(prev, KeyLeft, 0) || c.Page() != 1 {
		t.Errorf("Left on the previous button left page %d, want 1", c.Page())
	}
	if c.InterceptKey(c.pages[0], KeyLeft, 0) {
		t.Error("Left on a page's widget was taken from it")
	}
	kids := c.Children()
	if len(kids) != 5 || kids[0] != c.pages[0] || kids[4] != Widget(next) {
		t.Errorf("children %v, want the pages then the buttons", kids)
	}
}

// TestNavigationView pins the shell: push deepens and re-titles, pop
// returns (with the hook), the back keys pop, PopToRoot unwinds, and
// the root never pops.
func TestNavigationView(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	page := func(t2 string) *NavigationPage {
		return &NavigationPage{Title: t2, W: NewLabel(face, 14, t2, th.Text)}
	}
	v := NewNavigationView(face, 14, page("root"))
	if v.Depth() != 1 || v.Page().Title != "root" {
		t.Fatalf("root: depth=%d", v.Depth())
	}
	if v.Pop() {
		t.Error("the root popped")
	}
	pops := 0
	v.OnPop = func(popped, now *NavigationPage) {
		pops++
		if popped.Title != "detail" || now.Title != "root" {
			t.Errorf("pop hook: %q -> %q", popped.Title, now.Title)
		}
	}
	v.Push(page("detail"))
	if v.Depth() != 2 || v.Page().Title != "detail" {
		t.Fatalf("push: depth=%d page=%q", v.Depth(), v.Page().Title)
	}
	v.KeyAction(KeyLeft, ModAlt)
	if v.Depth() != 1 || pops != 1 {
		t.Errorf("alt-left: depth=%d pops=%d", v.Depth(), pops)
	}
	v.OnPop = nil
	v.Push(page("a"))
	v.Push(page("b"))
	v.PopToRoot()
	if v.Depth() != 1 {
		t.Errorf("PopToRoot: depth=%d", v.Depth())
	}
}

// TestNavigationSplitView pins the collapse: wide shows sidebar and
// content side by side, the breakpoint collapses to the navigation
// view, ShowContent pushes the content page, and popping returns to
// the sidebar.
func TestNavigationSplitView(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	page := func(t2 string) *NavigationPage {
		return &NavigationPage{Title: t2, W: NewLabel(face, 14, t2, th.Text)}
	}
	side := page("sidebar")
	content := page("content")
	s := NewNavigationSplitView(face, 14, side, content, 400)

	s.Measure(Constraints{Max: Size{W: 600, H: 300}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 600, H: 300})
	if s.Collapsed() {
		t.Fatal("wide split collapsed")
	}

	s.Measure(Constraints{Max: Size{W: 360, H: 300}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 360, H: 300})
	if !s.Collapsed() {
		t.Fatal("narrow split did not collapse")
	}
	s.ShowContent()
	if s.Navigation().Depth() != 2 {
		t.Fatalf("ShowContent: depth=%d", s.Navigation().Depth())
	}
	s.Navigation().KeyAction(KeyBackspace, 0)
	if s.Navigation().Depth() != 1 {
		t.Error("backspace did not return to the sidebar")
	}
}

// TestOverlaySplitView pins the flap: wide shows both, collapsed
// hides the sidebar until ShowSidebar, and the overlay hit-tests
// first when open.
func TestOverlaySplitView(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	side := NewLabel(face, 14, "side", th.Text)
	content := NewLabel(face, 14, "main", th.Text)
	s := NewOverlaySplitView(side, content, 400)

	s.Measure(Constraints{Max: Size{W: 600, H: 200}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 600, H: 200})
	if s.Collapsed() || !s.SidebarShowing() {
		t.Fatal("wide overlay collapsed or hidden")
	}

	s.Measure(Constraints{Max: Size{W: 360, H: 200}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 360, H: 200})
	if !s.Collapsed() || s.SidebarShowing() {
		t.Fatal("narrow overlay did not collapse or still showing")
	}
	s.ShowSidebar(true)
	if !s.SidebarShowing() {
		t.Fatal("ShowSidebar did not open the flap")
	}
	s.ShowSidebar(false)
	if s.SidebarShowing() {
		t.Fatal("ShowSidebar(false) did not close the flap")
	}
}

// TestGoldenAdaptive pins the painted look: the view switcher pill
// strip and the carousel with its dots.
func TestGoldenAdaptive(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	page := func(s string) Widget {
		return NewLabel(face, 16, s, th.Text)
	}
	stack := NewStack()
	stack.Add("one", page("One"))
	stack.Add("two", page("Two"))
	v := NewViewSwitcher(face, 14, stack, map[string]string{"one": "First", "two": "Second"}, nil)
	v.ReflectVisible()
	NewGolden(t, v, "viewswitcher", goldenTheme(th), goldenFrame(220, 48))

	for _, shot := range bothPalettes("carousel") {
		c := NewCarousel(face, 14)
		for _, s := range [...]string{"A", "B", "C"} {
			c.Append(NewLabel(face, 16, s, shot.th.Text))
		}
		NewGolden(t, c, shot.name, goldenTheme(shot.th), goldenFrame(220, 90))
	}
}

// TestViewSwitcherCheckedFill pins the selection mark: the visible
// page's button carries :checked (the stylesheet's rule, else the
// theme's selected shade, paints it), and a switch moves it; no
// programmatic fill is left behind to outrank a stylesheet.
func TestViewSwitcherCheckedFill(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	stack := NewStack()
	stack.Add("a", NewLabel(face, 14, "a", th.Text))
	stack.Add("b", NewLabel(face, 14, "b", th.Text))
	v := NewViewSwitcher(face, 14, stack, nil, nil)
	if !v.buttons[0].HasState(StateChecked) || v.buttons[0].Bg != 0 || v.buttons[0].BgExplicit {
		t.Fatal("the visible page's button is not marked by state alone")
	}
	stack.Show("b")
	v.ReflectVisible()
	if v.buttons[0].HasState(StateChecked) || v.buttons[0].Bg != 0 || v.buttons[0].BgExplicit {
		t.Error("the old button kept its mark")
	}
	if !v.buttons[1].HasState(StateChecked) {
		t.Error("the new button is not marked")
	}
}

// TestSplitRehomesSharedPages pins the parent re-homing: the page
// widgets sit in both layouts, and after every mode switch their
// ancestor chain runs through the layout actually showing them.
func TestSplitRehomesSharedPages(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	side := &NavigationPage{Title: "side", W: NewLabel(face, 14, "side", th.Text)}
	content := &NavigationPage{Title: "content", W: NewLabel(face, 14, "content", th.Text)}
	s := NewNavigationSplitView(face, 14, side, content, 400)
	layout := func(w int) {
		s.Measure(Constraints{Max: Size{W: w, H: 200}})
		s.Arrange(render.Rect{W: w, H: 200})
	}
	layout(600)
	if DescendsFrom(side.W, s.nav) || !DescendsFrom(side.W, s) {
		t.Fatal("wide: the sidebar is not homed in the wide row")
	}
	layout(300)
	if !DescendsFrom(side.W, s.nav) {
		t.Fatal("collapsed: the sidebar still claims the wide row")
	}
	layout(600)
	if DescendsFrom(side.W, s.nav) || !DescendsFrom(side.W, s.wide) {
		t.Fatal("re-expanded: the sidebar still claims the navigation view")
	}
}

// TestSplitRestylesRehomedPages pins why the core clears the shared
// pages' parents on a mode switch: a plain re-link keeps the cascade
// resolved under the old ancestors, so a rule scoped to the collapsed
// layout's containers would never reach the page. Clearing makes the
// next link a first link, which re-cascades the subtree.
func TestSplitRestylesRehomedPages(t *testing.T) {
	loadCSS(t, `stack label { min-height: 50px; }`)
	face := adaptiveFace(t)
	th := DarkTheme()
	side := &NavigationPage{Title: "side", W: NewLabel(face, 14, "side", th.Text)}
	content := &NavigationPage{Title: "content", W: NewLabel(face, 14, "content", th.Text)}
	s := NewNavigationSplitView(face, 14, side, content, 400)
	height := func(w int) int {
		s.Measure(Constraints{Max: Size{W: w, H: 400}})
		s.Arrange(render.Rect{W: w, H: 400})
		return side.W.Measure(Constraints{Max: Size{W: w, H: 400}}).H
	}
	if h := height(600); h >= 50 {
		t.Fatalf("wide: the stack rule reached the sidebar (h=%d)", h)
	}
	if h := height(300); h < 50 {
		t.Errorf("collapsed: the sidebar kept its wide cascade (h=%d)", h)
	}
	if h := height(600); h >= 50 {
		t.Errorf("re-expanded: the sidebar kept its collapsed cascade (h=%d)", h)
	}
}
