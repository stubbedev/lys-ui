package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

func TestMenu(t *testing.T) {
	face := entryFace(t)
	clicked := ""
	menu := NewMenu(face, 14,
		MenuItem{Label: "Cut", OnClick: func() { clicked = "cut" }},
		MenuItem{Label: "Copy", OnClick: func() { clicked = "copy" }},
		MenuItem{Label: "Disabled"},
	)
	sz := menu.Measure(Constraints{Max: Size{W: 300, H: 300}})
	menu.Arrange(render.Rect{X: 10, Y: 20, W: 120, H: sz.H})
	menu.OnDismiss = func() {}

	t.Run("hover tracks the row under the pointer", func(t *testing.T) {
		menu.HoverMove(rowCenter(menu, 1))
		if menu.hovered != 1 {
			t.Errorf("hovered = %d, want 1", menu.hovered)
		}
		menu.HoverMove(Point{X: 5, Y: 5})
		if menu.hovered != -1 {
			t.Errorf("hovered outside rows = %d, want -1", menu.hovered)
		}
	})

	t.Run("clicking a row fires it and dismisses", func(t *testing.T) {
		fired := false
		menu.OnDismiss = func() { fired = true }
		menu.ClickAt(rowCenter(menu, 1))
		if clicked != "copy" {
			t.Errorf("clicked = %q, want copy", clicked)
		}
		if !fired {
			t.Error("click did not dismiss the menu")
		}
	})

	t.Run("clicking a disabled row neither fires nor dismisses", func(t *testing.T) {
		clicked = ""
		fired := false
		menu.OnDismiss = func() { fired = true }
		menu.ClickAt(rowCenter(menu, 2))
		if clicked != "" || fired {
			t.Errorf("disabled row fired (%q, dismiss %v)", clicked, fired)
		}
	})

	t.Run("clicking outside the rows dismisses", func(t *testing.T) {
		fired := false
		menu.OnDismiss = func() { fired = true }
		menu.ClickAt(Point{X: 40, Y: 20 - 50})
		if !fired {
			t.Error("outside click did not dismiss")
		}
	})

	t.Run("painted menu leaves ink on every row", func(t *testing.T) {
		menu.OnDismiss = nil
		menu.Arrange(render.Rect{X: 0, Y: 0, W: 120, H: sz.H})
		data := make([]byte, render.Stride(130)*sz.H)
		cv := render.New(data, render.Stride(130), 130, sz.H)
		menu.Paint(cv)
		for row := range 3 {
			y := rowCenter(menu, row).Y
			ink := 0
			for x := range 120 {
				if render.ColorFromBytes(data[y*render.Stride(130)+x*4:]).A() > 0 {
					ink++
				}
			}
			if ink == 0 {
				t.Errorf("row %d painted nothing", row)
			}
		}
	})
}

// The menu styles as GTK's popup: the `popover.menu` card, a
// `contents` inset, and one `modelbutton` per row — the hovered row
// :hover, inert rows :disabled, check rows :checked while on, and the
// label in the row's color.
func TestMenuPopoverNodes(t *testing.T) {
	loadCSS(t, `popover.menu { background-color: #010203; } popover.menu > contents { padding: 7; } popover.menu > contents modelbutton { color: #0a0b0c; } popover.menu > contents modelbutton:hover { background-color: #0d0e0f; } popover.menu > contents modelbutton:checked { color: #112233; } popover.menu > contents modelbutton:disabled { color: #445566; }`)
	m := NewMenu(testFace(t), 13,
		MenuItem{Label: "one", OnClick: func() {}},
		MenuItem{Label: "two"},
		MenuItem{Label: "check", Kind: ItemCheck, Checked: true, OnClick: func() {}},
	)
	host := NewBox(Column, 0, 0)
	host.Append(m, false)
	frame(t, host, 200, 120)

	if got := m.style(m).Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("card background %v, want the popover.menu rule", got)
	}
	own := boxOf(m.style(m), render.Insets{}).outer()
	if in := m.contentsInset(); in.Top != own.Top+7 || in.Left != own.Left+7 {
		t.Errorf("contents inset %v, want the card's box %v plus the contents padding", in, own)
	}
	first := &m.rows[0]
	if got := pickc(0, first.style(first), style.PropColor, 0); got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("row color %v, want the modelbutton rule", got)
	}
	if !m.rows[1].disabled {
		t.Error("a row with no action is not :disabled")
	}
	check := &m.rows[2]
	if !check.HasState(StateChecked) {
		t.Fatal("checked row missing :checked")
	}
	if got := pickc(0, check.style(check), style.PropColor, 0); got != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("checked row color %v, want the :checked rule", got)
	}
	m.moveHover(0)
	if got := first.style(first).Background; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("hovered row background %v, want the :hover rule", got)
	}
	if got := pickc(0, m.rows[1].style(&m.rows[1]), style.PropColor, 0); got != render.RGB(0x44, 0x55, 0x66) {
		t.Errorf("disabled row color %v, want the :disabled rule", got)
	}
}

// A keyboard highlight survives pointer events over no row - the
// gutter a compositor reports when the popup maps under a resting
// cursor - and yields once the pointer selects a row.
func TestMenuKeyboardHighlightSurvivesStrayPointer(t *testing.T) {
	m := NewMenu(testFace(t), 13, MenuItem{Label: "a", OnClick: func() {}}, MenuItem{Label: "b", OnClick: func() {}})
	m.Measure(Constraints{Max: Size{W: 200, H: 200}})
	m.Arrange(render.Rect{W: 200, H: 200})
	m.KeyAction(KeyDown, 0)
	m.KeyAction(KeyDown, 0)
	if m.hovered != 1 {
		t.Fatalf("hovered %d after two Downs", m.hovered)
	}
	m.HoverMove(Point{X: 100, Y: 199}) // below every row
	m.SetHovered(false)
	if m.hovered != 1 {
		t.Fatalf("a stray pointer took the keyboard highlight: %d", m.hovered)
	}
	r := m.RowBounds(0)
	m.HoverMove(Point{X: r.X + 5, Y: r.Y + r.H/2})
	if m.hovered != 0 {
		t.Fatalf("pointing at row 0 left hovered %d", m.hovered)
	}
	m.HoverMove(Point{X: 100, Y: 199})
	if m.hovered != -1 {
		t.Errorf("a pointer highlight stayed after the pointer left the rows: %d", m.hovered)
	}
}

// rowCenter is the middle of menu row i, where a pointer rests on it.
func rowCenter(m *Menu, i int) Point {
	r := m.RowBounds(i)
	return Point{X: r.X + r.W/2, Y: r.Y + r.H/2}
}
