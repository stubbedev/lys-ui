package widget

import "testing"

// rightButton records the buttons it took itself.
type rightButton struct {
	*Button
	got []uint32
}

func (b *rightButton) PointerButton(button uint32) { b.got = append(b.got, button) }

func (b *rightButton) HitTest(p Point) Widget {
	if b.Button.HitTest(p) != nil {
		return b
	}
	return nil
}

func newTestContextMenu(t *testing.T) (*ContextMenu, *Button, *fakeSurfaceHost) {
	t.Helper()
	face := goldenFace(t)
	inner := NewButton(NewLabel(face, 14, "Right click here", 0), 8, 8)
	c := NewContextMenu(face, NewBox(Column, 0, 20).Append(inner, false),
		MenuItem{Label: "Back", Accel: "<Alt>Left", OnClick: func() {}},
		MenuItem{Label: "Reload", OnClick: func() {}})
	host := &fakeSurfaceHost{}
	SetSurfaceHost(c, host)
	arrangeTree(t, c, 300, 100)
	return c, inner, host
}

// A right click anywhere inside, a long press and the Menu key open
// the menu; a left click does not.
func TestContextMenuOpens(t *testing.T) {
	c, inner, host := newTestContextMenu(t)
	r := &Router{Root: c}
	b := inner.Bounds()
	at := Point{X: b.X + 4, Y: b.Y + 4}
	r.Press(BTNLeft, at)
	r.Release(BTNLeft, at)
	if len(host.menus) != 0 {
		t.Fatal("a left click opened the menu")
	}
	r.Press(BTNRight, at)
	if len(host.menus) != 1 || host.menus[0][0].Label != "Back" {
		t.Fatalf("a right click opened %v", host.menus)
	}
	r.Gesture(Gesture{Kind: GestureLongPress, Phase: GestureEnd, At: at})
	r.SetFocus(inner)
	r.KeyAction(KeyContextMenu, 0)
	if len(host.menus) != 3 {
		t.Errorf("long press and Menu key opened %d menus in all, want 3", len(host.menus))
	}
}

// A widget that takes the right button itself keeps it.
func TestContextMenuYieldsToButtonHandlers(t *testing.T) {
	face := goldenFace(t)
	own := &rightButton{Button: NewButton(NewLabel(face, 14, "mine", 0), 8, 8)}
	c := NewContextMenu(face, own, MenuItem{Label: "x", OnClick: func() {}})
	host := &fakeSurfaceHost{}
	SetSurfaceHost(c, host)
	arrangeTree(t, c, 200, 60)
	r := &Router{Root: c}
	b := own.Bounds()
	r.Press(BTNRight, Point{X: b.X + 2, Y: b.Y + 2})
	if len(own.got) != 1 || len(host.menus) != 0 {
		t.Errorf("button handler saw %v, menus %d", own.got, len(host.menus))
	}
}

// TestGoldenMenu pins the context menu's look: shortcuts, a
// disabled row, a submenu, separators, checks and radios, a hovered
// row, in both palettes.
func TestGoldenMenu(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			m := NewMenu(face, 14,
				MenuItem{Label: "Back", Accel: "Alt+Left", OnClick: func() {}},
				MenuItem{Label: "Forward", Accel: "Alt+Right", Disabled: true},
				MenuItem{Label: "Reload", Accel: "Ctrl+R", OnClick: func() {}},
				MenuItem{Label: "More Tools", Items: []MenuItem{{Label: "Save Page", OnClick: func() {}}}},
				MenuSeparator(),
				MenuItem{Label: "Show Bookmarks", Kind: ItemCheck, Checked: true, OnClick: func() {}},
				MenuItem{Label: "Show Full URLs", Kind: ItemCheck, OnClick: func() {}},
				MenuSeparator(),
				MenuItem{Label: "Pedro Duarte", Kind: ItemRadio, Checked: true, Group: "people", OnClick: func() {}},
				MenuItem{Label: "Colm Tuite", Kind: ItemRadio, Group: "people", OnClick: func() {}},
			)
			NewGolden(t, m, "menu-"+tc.name, goldenTheme(tc.th),
				goldenAfterArrange(func() { hoverMenuRow(m, 2) }))
		})
	}
}
