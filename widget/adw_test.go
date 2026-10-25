package widget

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
)

// The swipeable widgets settle on the router's gesture interfaces; a
// method the router never calls (a ReleaseAt, say) would compile and
// silently never fire, so the contract is pinned at compile time.
var (
	_ PressAter  = (*Carousel)(nil)
	_ DragMover  = (*Carousel)(nil)
	_ PressEnder = (*Carousel)(nil)
	_ PressAter  = (*Sheet)(nil)
	_ DragMover  = (*Sheet)(nil)
	_ PressEnder = (*Sheet)(nil)
	_ Clicker    = (*Sheet)(nil)
)

// TestToggleGroup pins the segmented control: one active toggle, the
// mark following it, clicks and Left/Right moving it (clamped), and
// OnChanged firing only when it moved.
func TestToggleGroup(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	g := NewToggleGroup(NewLabel(face, 14, "a", th.Text), NewLabel(face, 14, "b", th.Text), NewLabel(face, 14, "c", th.Text))
	var changes []int
	g.OnChanged = func(i int) { changes = append(changes, i) }
	if g.Active() != 0 || !g.buttons[0].HasState(StateChecked) {
		t.Fatal("the first toggle is not active")
	}
	g.buttons[2].OnClick()
	g.KeyAction(KeyRight, 0) // clamped at the end: no change
	g.KeyAction(KeyLeft, 0)
	g.SetActive(1) // already active: no change
	if len(changes) != 2 || changes[0] != 2 || changes[1] != 1 {
		t.Errorf("changes = %v, want [2 1]", changes)
	}
	for i, b := range g.buttons {
		if b.HasState(StateChecked) != (i == 1) {
			t.Errorf("toggle %d checked=%v", i, b.HasState(StateChecked))
		}
	}
	g.Clear()
	if g.Active() != -1 || g.Len() != 0 {
		t.Errorf("cleared group: active=%d len=%d", g.Active(), g.Len())
	}
}

// TestButtonContent pins the halves: empty icon or label drops it.
func TestButtonContent(t *testing.T) {
	face := adaptiveFace(t)
	both := NewButtonContent(face, 14, "document-open", "Open")
	if n := len(both.row.Children()); n != 2 {
		t.Errorf("both halves: %d children", n)
	}
	label := NewButtonContent(face, 14, "", "Open")
	if n := len(label.row.Children()); n != 1 {
		t.Errorf("label only: %d children", n)
	}
	label.SetLabel("")
	if n := len(label.row.Children()); n != 0 {
		t.Errorf("emptied: %d children", n)
	}
}

// TestAvatar pins the initials and the fallback fill the avatar rule
// restyles.
func TestAvatar(t *testing.T) {
	face := adaptiveFace(t)
	cases := map[string]string{
		"Ada Lovelace":            "AL",
		"grace":                   "G",
		"Jean-Luc   de la Picard": "JP",
		"":                        "",
		"  ":                      "",
	}
	for name, want := range cases {
		if got := NewAvatar(face, 48, name).Initials(); got != want {
			t.Errorf("Initials(%q) = %q, want %q", name, got, want)
		}
	}
	a := NewAvatar(face, 48, "Ada Lovelace")
	center := func() Color { return pxAt(paintPixels(t, a, 48, 48), 48, 24, 4) }
	if got := center(); got != Current().MutedColor() {
		t.Errorf("fallback fill %v, want the muted fill", got)
	}
	loadCSS(t, `avatar { background-color: #102030; }`)
	if got := center(); got != render.RGB(0x10, 0x20, 0x30) {
		t.Errorf("styled fallback fill %v, want the avatar rule", got)
	}
}

// TestSplitButton pins the halves: the main fires OnClick, the arrow
// fires OnMenu with itself as the anchor.
func TestSplitButton(t *testing.T) {
	face := adaptiveFace(t)
	s := NewSplitButton(NewLabel(face, 14, "Save", DarkTheme().Text), 14)
	clicks := 0
	var anchor Boundser
	s.OnClick = func() { clicks++ }
	s.OnMenu = func(a Boundser) { anchor = a }
	s.main.OnClick()
	s.arrow.OnClick()
	if clicks != 1 || anchor != s.Arrow() {
		t.Errorf("clicks=%d anchor=%v", clicks, anchor)
	}
}

// TestBannerAndStatusPage pin the two announcement widgets' state.
func TestBannerAndStatusPage(t *testing.T) {
	face := adaptiveFace(t)
	b := NewBanner(face, 14, "Offline", "Retry")
	pressed := 0
	b.OnButton = func() { pressed++ }
	if b.Revealed() {
		t.Error("the banner starts revealed")
	}
	b.SetRevealed(true)
	b.button.OnClick()
	if !b.Revealed() || pressed != 1 {
		t.Errorf("revealed=%v pressed=%d", b.Revealed(), pressed)
	}
	if NewBanner(face, 14, "No action", "").button != nil {
		t.Error("an empty button label built a button")
	}

	p := NewStatusPage(face, "", "Nothing here", "Add something to get started.")
	before := len(p.column.Children())
	btn := NewButton(NewLabel(face, 14, "Add", DarkTheme().Text), 8, 4)
	p.SetChild(btn)
	kids := p.column.Children()
	if len(kids) != before+1 || kids[len(kids)-2] != Widget(p.content) || p.content.Children()[0] != Widget(btn) {
		t.Error("SetChild did not land in the content area above the trailing spacer")
	}
	p.SetChild(NewLabel(face, 14, "Learn more", 0))
	if len(p.column.Children()) != before+1 || len(p.content.Children()) != 2 {
		t.Error("a second child did not stack in the content area")
	}
	p.SetIcon(NewSymbol(SymbolClose, 24))
	if p.column.Children()[1] != Widget(p.media) {
		t.Error("SetIcon did not put the tile above the title")
	}
	p.SetIcon(nil)
	if slices.Contains(p.column.Children(), Widget(p.media)) {
		t.Error("SetIcon(nil) kept the tile")
	}
}

// TestWrapBoxJustify pins the justifications on one line of three
// 20px children in 100px with 5px spacing (leftover 30).
func TestWrapBoxJustify(t *testing.T) {
	xs := func(j Justify) []render.Rect {
		f := NewWrapBox(5, 5, j)
		for range 3 {
			f.Append(NewSpacer(20, 10))
		}
		f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		f.Arrange(render.Rect{W: 100, H: 100})
		var out []render.Rect
		for _, c := range f.Children() {
			out = append(out, c.(Boundser).Bounds())
		}
		return out
	}
	if r := xs(JustifyStart); r[0].X != 0 || r[2].X != 50 {
		t.Errorf("start: %v", r)
	}
	if r := xs(JustifyCenter); r[0].X != 15 {
		t.Errorf("center: %v", r)
	}
	if r := xs(JustifyEnd); r[2].X+r[2].W != 100 {
		t.Errorf("end: %v", r)
	}
	if r := xs(JustifyFill); r[0].W != 30 || r[2].X+r[2].W != 100 {
		t.Errorf("fill: %v", r)
	}
	if r := xs(JustifySpread); r[0].X != 0 || r[2].X+r[2].W != 100 || r[1].X != 40 {
		t.Errorf("spread: %v", r)
	}
}

// TestBottomSheet pins the sheet's lifecycle on the instant clock:
// open shows it over a hit-tested backdrop, a short handle drag
// springs back, a long one dismisses (OnClosed fires), and a backdrop
// click and Escape close it too.
func TestBottomSheet(t *testing.T) {
	defer anim.SetInstant(true)()
	face := adaptiveFace(t)
	th := DarkTheme()
	content := NewLabel(face, 14, "content", th.Text)
	body := NewBox(Column, 0, 0)
	body.Append(NewSpacer(10, 120), false)
	b := NewBottomSheet(content, body)
	closed := 0
	b.OnClosed = func() { closed++ }
	layout := func() {
		b.Measure(Constraints{Max: Size{W: 300, H: 400}})
		b.Arrange(render.Rect{W: 300, H: 400})
	}
	layout()
	if hit := b.HitTest(Point{X: 150, Y: 380}); hit == Widget(b) {
		t.Fatal("a closed sheet captured the content")
	}

	b.SetOpen(true)
	rect := b.sheetRect()
	if rect.H != 176 || rect.Y != 224 {
		t.Fatalf("open sheet rect = %+v, want 176 tall at the bottom: the handle, the body and 16px padding", rect)
	}
	if hit := b.HitTest(Point{X: 150, Y: 50}); hit != Widget(b) {
		t.Error("the backdrop is not the sheet's")
	}

	b.PressAt(Point{X: 150, Y: rect.Y + 5})
	b.DragMove(Point{X: 150, Y: rect.Y + 20})
	b.PressEnd()
	if !b.Open() || b.dragDY != 0 {
		t.Errorf("a short drag closed it or kept the offset (open=%v dy=%d)", b.Open(), b.dragDY)
	}
	b.PressAt(Point{X: 150, Y: rect.Y + 5})
	b.DragMove(Point{X: 150, Y: rect.Y + 100})
	b.PressEnd()
	if b.Open() || closed != 1 {
		t.Errorf("a long drag: open=%v closed=%d", b.Open(), closed)
	}

	b.SetOpen(true)
	b.ClickAt(Point{X: 150, Y: 20})
	if b.Open() {
		t.Error("a backdrop click left it open")
	}
	b.SetOpen(true)
	b.KeyAction(KeyDismiss, 0)
	if b.Open() || closed != 3 {
		t.Errorf("escape: open=%v closed=%d", b.Open(), closed)
	}
}

// TestGoldenAdw pins the painted look of the new Adwaita widgets.
func TestGoldenAdw(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	g := NewToggleGroup(NewLabel(face, 14, "Day", th.Text), NewLabel(face, 14, "Week", th.Text), NewLabel(face, 14, "Month", th.Text))
	g.SetActive(1)
	NewGolden(t, g, "togglegroup", goldenTheme(th))

	row := NewBox(Row, 8, 0)
	row.Append(NewAvatar(face, 48, "Ada Lovelace"), false)
	pic := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range pic.Pix {
		pic.Pix[i] = 0xff
	}
	pic.Set(0, 0, color.RGBA{0xff, 0, 0, 0xff})
	img := NewAvatar(face, 48, "x")
	img.SetImage(pic)
	row.Append(img, false)
	NewGolden(t, row, "avatars", goldenTheme(th))
	NewGolden(t, NewAvatar(face, 48, "Ada Lovelace"), "avatar-light", goldenTheme(LightTheme()))

	NewGolden(t, NewStatusPage(face, "", "No results", "Try a different search term or clear the filters."), "statuspage", goldenTheme(th), goldenFrame(320, 220))

	b := NewBanner(face, 14, "You are offline", "Retry")
	b.SetRevealed(true)
	NewGolden(t, b, "banner", goldenTheme(th), goldenFrame(320, 48))

	NewGolden(t, NewSplitButton(NewLabel(face, 14, "Save", th.Text), 14), "splitbutton", goldenTheme(th))

	sheet := NewBottomSheet(NewLabel(face, 14, "content", th.Text), NewLabel(face, 14, "Sheet body", th.Text))
	sheet.SetOpen(true)
	NewGolden(t, sheet, "bottomsheet", goldenTheme(th), goldenFrame(240, 200))
}

// A bottom sheet pads its panel 16px and centers it in a side sheet's
// width at most: its content ran edge to edge, the title flush with
// the window's side (#281).
func TestBottomSheetPadsAndCentersItsPanel(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	body := NewBox(Column, 0, 0)
	body.Append(NewSpacer(10, 40), false)
	b := NewBottomSheet(NewSpacer(0, 0), body)
	b.Measure(Constraints{Max: Size{W: 1000, H: 600}})
	b.Arrange(render.Rect{W: 1000, H: 600})
	b.SetOpen(true)
	b.Arrange(render.Rect{W: 1000, H: 600})
	got := body.Bounds()
	want := sheetWidth
	if got.W != want || got.X != (1000-want)/2 {
		t.Errorf("body %v, want %dpx centered in the 1000px edge", got, want)
	}
}
