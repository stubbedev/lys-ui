package widget

import (
	"slices"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/render"
)

// chromeFace builds the test font.
func chromeFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestHeaderBarPacksAndControls pins the bar's layout pieces: title
// and subtitle set, packs append on their sides, controls cluster
// lazily created and toggleable, and the bar reports the move grab.
func TestHeaderBarPacksAndControls(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	bar := NewHeaderBar(face, 14)
	bar.SetTitle("lys-ui")
	bar.SetSubtitle("fourth pass")
	if bar.title.Text() != "lys-ui" || bar.sub.Text() != "fourth pass" {
		t.Fatalf("title/subtitle = %q/%q", bar.title.Text(), bar.sub.Text())
	}

	start := NewLabel(face, 14, "s", th.Text)
	end := NewLabel(face, 14, "e", th.Text)
	bar.PackStart(start)
	bar.PackEnd(end)
	if got := len(bar.start.Children()) + len(bar.end.Children()); got != 2 {
		t.Errorf("packs hold %d children, want 2", got)
	}

	controls := bar.Controls()
	if !bar.WindowMoveGrab() {
		t.Error("the bar does not report the move grab")
	}
	bar.WindowDoubleClick() // no hook: must not panic
	fired := 0
	bar.OnDoubleClick = func() { fired++ }
	bar.WindowDoubleClick()
	if fired != 1 {
		t.Errorf("double press fired %d times", fired)
	}

	// Buttons appear and disappear, idempotently.
	controls.ShowClose(true)
	controls.ShowClose(true)
	if got := len(controls.row.Children()); got != 1 {
		t.Fatalf("close toggle left %d buttons", got)
	}
	controls.ShowMinimize(true)
	controls.ShowMaximize(true)
	if got := len(controls.row.Children()); got != 3 {
		t.Fatalf("controls hold %d buttons, want 3", got)
	}
	controls.ShowMaximize(false)
	controls.ShowMaximize(false)
	if got := len(controls.row.Children()); got != 2 {
		t.Errorf("maximize toggle left %d buttons", got)
	}

	// The close button fires its hook.
	closed := 0
	controls.OnClose = func() { closed++ }
	controls.closeBtn.OnClick()
	if closed != 1 {
		t.Error("the close button did not fire its hook")
	}
}

// TestHeaderBarHitTestIsMoveSurfaceExceptButtons pins the CSD hit
// contract: a press on the bar's background or title resolves to the
// bar (the move surface); a press on a packed button resolves to the
// button.
func TestHeaderBarHitTestIsMoveSurfaceExceptButtons(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	bar := NewHeaderBar(face, 14)
	bar.SetTitle("win")
	btn := NewButton(NewLabel(face, 14, "menu", th.Text), 8, 4)
	bar.PackStart(btn)
	bar.Measure(Constraints{Max: Size{W: 400, H: 40}})
	bar.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 40})
	btnRect := btn.Bounds()

	if hit := bar.HitTest(Point{X: btnRect.X + 2, Y: btnRect.Y + 2}); hit != Widget(btn) {
		t.Errorf("press on the packed button resolved to %T, want the button", hit)
	}
	if hit := bar.HitTest(Point{X: 200, Y: 20}); hit != Widget(bar) {
		t.Errorf("press on the bar background resolved to %T, want the bar", hit)
	}
	if hit := bar.HitTest(Point{X: 390, Y: 20}); hit != Widget(bar) {
		t.Errorf("press near the end resolved to %T, want the bar", hit)
	}
}

// TestActionBarPacks pins the bottom bar's packs.
func TestActionBarPacks(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	bar := NewActionBar()
	bar.PackStart(NewLabel(face, 13, "a", th.Text))
	bar.PackEnd(NewLabel(face, 13, "b", th.Text))
	bar.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 36})
	if got := len(bar.start.Children()) + len(bar.end.Children()); got != 2 {
		t.Errorf("packs hold %d children, want 2", got)
	}
}

// TestMenuBarRootsAndKeyboard pins the bar's activation: roots report
// their anchors, clicks and the F10-style key handoff fire OnRoot.
func TestMenuBarRootsAndKeyboard(t *testing.T) {
	face := chromeFace(t)
	bar := NewMenuBar(face, 14, "File", "Edit")
	if bar.Roots() != 2 || bar.Root(0) == nil || bar.Root(2) != nil {
		t.Fatalf("roots = %d", bar.Roots())
	}
	var opened []int
	bar.OnRoot = func(i int, anchor Boundser) {
		if anchor == nil {
			t.Error("root activation carried no anchor")
		}
		opened = append(opened, i)
	}
	bar.buttons[1].OnClick()
	bar.KeyAction(KeyEnter, 0)
	if len(opened) != 2 || opened[0] != 1 || opened[1] != 0 {
		t.Errorf("opened = %v, want [1 0]", opened)
	}
}

// TestDescendsFrom pins the ancestor walk the input pipeline uses.
func TestDescendsFrom(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	outer := NewBox(Column, 0, 0)
	inner := NewBox(Row, 0, 0)
	label := NewLabel(face, 14, "x", th.Text)
	outer.Append(inner, false)
	inner.Append(label, false)
	outer.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 50})
	if !DescendsFrom(label, outer) || !DescendsFrom(outer, outer) {
		t.Error("the walk missed an ancestor")
	}
	if DescendsFrom(outer, label) {
		t.Error("the walk went the wrong way")
	}
}

// TestGoldenChrome pins the painted look of the three chrome widgets.
func TestGoldenChrome(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	header := NewHeaderBar(face, 14)
	header.SetTitle("lys-ui")
	header.SetSubtitle("window chrome")
	controls := header.Controls()
	controls.ShowClose(true)
	controls.ShowMinimize(true)
	controls.ShowMaximize(true)
	NewGolden(t, header, "headerbar", goldenTheme(th), goldenFrame(360, 44))

	for _, shot := range bothPalettes("menubar") {
		NewGolden(t, NewMenuBar(face, 13, "File", "Edit", "View"), shot.name, goldenTheme(shot.th))
	}
}

// Buttons hidden and shown again (capabilities changing at runtime)
// keep close, minimize, maximize order.
func TestWindowControlsKeepOrder(t *testing.T) {
	c := NewWindowControls(testFace(t), 14)
	c.ShowMaximize(true)
	c.ShowClose(true)
	c.ShowMinimize(true)
	want := []Widget{c.closeBtn, c.minBtn, c.maxBtn}
	if got := c.row.Children(); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want close, minimize, maximize", got)
	}
	c.ShowMinimize(false)
	if got := c.row.Children(); len(got) != 2 || c.minBtn != nil {
		t.Fatalf("hidden minimize left %d buttons", len(got))
	}
	c.ShowMinimize(true)
	if got := c.row.Children(); got[1] != c.minBtn {
		t.Error("minimize did not return between close and maximize")
	}
}
