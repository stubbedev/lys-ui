package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestMenuKeyboard(t *testing.T) {
	items := []MenuItem{
		{Label: "cut", OnClick: func() {}},
		MenuSeparator(),
		{Label: "copy", OnClick: func() {}},
	}
	menu := NewMenu(entryFace(t), 13, items...)
	menu.Measure(Constraints{Max: Size{W: 300, H: 300}})
	menu.Arrange(render.Rect{X: 10, Y: 20, W: 120, H: menu.Measure(Constraints{Max: Size{W: 400, H: 400}}).H})

	t.Run("arrows move and clamp at the ends (pinned)", func(t *testing.T) {
		menu.KeyAction(KeyDown, 0)
		if menu.hovered != 0 {
			t.Errorf("first down hovered %d, want 0 (skipping the separator)", menu.hovered)
		}
		menu.KeyAction(KeyDown, 0)
		if menu.hovered != 2 {
			t.Errorf("second down hovered %d, want 2 (separator skipped)", menu.hovered)
		}
		menu.KeyAction(KeyDown, 0)
		if menu.hovered != 2 {
			t.Errorf("down at bottom hovered %d, want clamped 2", menu.hovered)
		}
		menu.KeyAction(KeyEnd, 0)
		if menu.hovered != 2 {
			t.Errorf("end hovered %d, want 2", menu.hovered)
		}
		menu.KeyAction(KeyUp, 0)
		menu.KeyAction(KeyUp, 0)
		if menu.hovered != 0 {
			t.Errorf("up twice hovered %d, want clamped 0", menu.hovered)
		}
	})

	t.Run("enter fires the hovered row", func(t *testing.T) {
		fired := ""
		menu.items[2].OnClick = func() { fired = "copy" }
		menu.KeyAction(KeyEnd, 0)
		menu.KeyAction(KeyEnter, 0)
		if fired != "copy" {
			t.Errorf("enter fired %q, want copy", fired)
		}
	})

	t.Run("esc dismisses", func(t *testing.T) {
		fired := false
		menu.OnDismiss = func() { fired = true }
		menu.KeyAction(KeyDismiss, 0)
		if !fired {
			t.Error("esc did not dismiss")
		}
	})
}

func TestMenuKinds(t *testing.T) {
	items := []MenuItem{
		{Label: "show grid", Kind: ItemCheck, Checked: true, OnClick: func() {}},
		{Label: "sort a", Kind: ItemRadio, Group: "sort", Checked: true, OnClick: func() {}},
		{Label: "sort b", Kind: ItemRadio, Group: "sort", OnClick: func() {}},
	}
	menu := NewMenu(entryFace(t), 13, items...)
	menu.Measure(Constraints{Max: Size{W: 300, H: 300}})
	menu.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: menu.Measure(Constraints{Max: Size{W: 400, H: 400}}).H})

	t.Run("check rows toggle on activate", func(t *testing.T) {
		menu.ClickAt(rowCenter(menu, 0))
		if menu.items[0].Checked {
			t.Error("check row did not toggle off")
		}
		menu.ClickAt(rowCenter(menu, 0))
		if !menu.items[0].Checked {
			t.Error("check row did not toggle on")
		}
	})

	t.Run("radio rows are exclusive within their group", func(t *testing.T) {
		menu.ClickAt(rowCenter(menu, 2))
		if menu.items[1].Checked {
			t.Error("radio a stayed checked after choosing b")
		}
		if !menu.items[2].Checked {
			t.Error("radio b did not become checked")
		}
	})
}

func TestMenuSubmenu(t *testing.T) {
	sub := []MenuItem{{Label: "child", OnClick: func() {}}}
	items := []MenuItem{
		{Label: "open recent", Items: sub},
		{Label: "quit", OnClick: func() {}},
	}
	menu := NewMenu(entryFace(t), 13, items...)
	menu.Measure(Constraints{Max: Size{W: 300, H: 300}})
	menu.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: menu.Measure(Constraints{Max: Size{W: 400, H: 400}}).H})

	t.Run("activating a submenu row fires OnSubmenu without dismissing", func(t *testing.T) {
		var got []MenuItem
		dismissed := false
		menu.OnSubmenu = func(i int, items []MenuItem) { got = items }
		menu.OnDismiss = func() { dismissed = true }
		menu.ClickAt(rowCenter(menu, 0))
		if len(got) != 1 || got[0].Label != "child" {
			t.Errorf("OnSubmenu items = %v, want the child items", got)
		}
		if dismissed {
			t.Error("opening a submenu dismissed the parent")
		}
	})

	t.Run("Right opens the hovered submenu", func(t *testing.T) {
		var opened bool
		menu.OnSubmenu = func(int, []MenuItem) { opened = true }
		menu.KeyAction(KeyUp, 0) // back to the submenu row
		menu.KeyAction(KeyRight, 0)
		if !opened {
			t.Error("Right did not open the hovered submenu")
		}
	})
}
