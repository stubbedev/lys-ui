package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/unxed/xkb-go"
)

// solidIcon is a static w x w icon of one opaque color.
func solidIcon(t *testing.T, w int, c render.Color) *Icon {
	t.Helper()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><rect width="4" height="4" fill="` + hexOf(c) + `"/></svg>`
	ic, err := render.LoadSVG([]byte(svg), w, w)
	if err != nil {
		t.Fatal(err)
	}
	return NewIcon(ic)
}

func hexOf(c render.Color) string {
	const digits = "0123456789abcdef"
	out := []byte{'#'}
	for _, v := range []uint8{c.R(), c.G(), c.B()} {
		out = append(out, digits[v>>4], digits[v&0xf])
	}
	return string(out)
}

func TestMenuDisabledRows(t *testing.T) {
	face := entryFace(t)
	fired := ""
	menu := NewMenu(face, 14,
		MenuItem{Label: "Open", OnClick: func() { fired = "open" }},
		MenuItem{Label: "Save", OnClick: func() { fired = "save" }, Disabled: true},
		MenuItem{Label: "Share", Items: []MenuItem{{Label: "Mail", OnClick: func() {}}}, Disabled: true},
		MenuItem{Label: "Quit", OnClick: func() { fired = "quit" }},
	)
	sz := menu.Measure(Constraints{Max: Size{W: 300, H: 300}})
	menu.Arrange(render.Rect{X: 0, Y: 0, W: 160, H: sz.H})
	rowY := func(i int) int { return rowCenter(menu, i).Y }

	t.Run("a click on a disabled row neither fires nor dismisses", func(t *testing.T) {
		dismissed := false
		menu.OnDismiss = func() { dismissed = true }
		menu.ClickAt(Point{X: 40, Y: rowY(1)})
		if fired != "" || dismissed {
			t.Errorf("disabled row fired %q, dismissed %v", fired, dismissed)
		}
	})

	t.Run("a disabled submenu does not open", func(t *testing.T) {
		opened := false
		menu.OnSubmenu = func(int, []MenuItem) { opened = true }
		menu.ClickAt(Point{X: 40, Y: rowY(2)})
		menu.hovered = 2
		menu.KeyAction(KeyRight, 0)
		if opened {
			t.Error("a disabled submenu row opened its submenu")
		}
	})

	t.Run("keyboard motion skips disabled rows", func(t *testing.T) {
		menu.hovered = 0
		menu.KeyAction(KeyDown, 0)
		if menu.hovered != 3 {
			t.Errorf("down from row 0 landed on %d, want 3 (rows 1 and 2 disabled)", menu.hovered)
		}
	})

	t.Run("disabled rows take no mnemonic", func(t *testing.T) {
		if menu.ActivateMnemonic(xkb.Keysym('s')) {
			t.Error("Alt+S reached a disabled row")
		}
		if !menu.ActivateMnemonic(xkb.Keysym('q')) || fired != "quit" {
			t.Errorf("Alt+Q on an enabled row: fired = %q", fired)
		}
	})

	t.Run("an enabled row still fires", func(t *testing.T) {
		fired = ""
		menu.OnDismiss = func() {}
		menu.ClickAt(Point{X: 40, Y: rowY(0)})
		if fired != "open" {
			t.Errorf("fired = %q, want open", fired)
		}
	})

	t.Run("a disabled check row keeps its state", func(t *testing.T) {
		check := NewMenu(face, 14, MenuItem{Label: "Wrap", Kind: ItemCheck, Checked: true, OnClick: func() {}, Disabled: true})
		csz := check.Measure(Constraints{Max: Size{W: 300, H: 300}})
		check.Arrange(render.Rect{W: 160, H: csz.H})
		check.ClickAt(Point{X: 40, Y: rowY(0)})
		if !check.items[0].Checked {
			t.Error("a click toggled a disabled check row")
		}
	})
}

func TestMenuRowIcons(t *testing.T) {
	face := entryFace(t)
	red := render.RGB(0xff, 0, 0)
	plain := NewMenu(face, 14, MenuItem{Label: "Open", OnClick: func() {}}, MenuItem{Label: "Close", OnClick: func() {}})
	iconic := NewMenu(face, 14,
		MenuItem{Label: "Open", OnClick: func() {}, Icon: solidIcon(t, 12, red)},
		MenuItem{Label: "Close", OnClick: func() {}},
	)
	con := Constraints{Max: Size{W: 400, H: 400}}

	t.Run("an icon row widens the menu by the slot", func(t *testing.T) {
		pw := plain.Measure(con).W
		iw := iconic.Measure(con).W
		if iw != pw+12+8 {
			t.Errorf("width with icon = %d, want %d (plain %d + 12px icon + 8px gap)", iw, pw+20, pw)
		}
	})

	t.Run("the icon paints at the row's leading edge", func(t *testing.T) {
		sz := iconic.Measure(con)
		data := make([]byte, render.Stride(sz.W)*sz.H)
		cv := render.New(data, render.Stride(sz.W), sz.W, sz.H)
		iconic.Arrange(render.Rect{W: sz.W, H: sz.H})
		iconic.Paint(cv)
		// The slot starts at x 4+8, past the contents pad and the row's.
		y := rowCenter(iconic, 0).Y
		if got := render.ColorFromBytes(data[y*render.Stride(sz.W)+(4+8+6)*4:]); got != red {
			t.Errorf("icon slot pixel = %v, want the red icon", got)
		}
		// Row 1 has no icon: its slot stays the menu surface.
		y1 := rowCenter(iconic, 1).Y
		if got := render.ColorFromBytes(data[y1*render.Stride(sz.W)+(4+8+6)*4:]); got == red {
			t.Error("the icon-less row painted an icon")
		}
	})
}

// TestMenuHoverHookAndRowBounds pins what a nested presenter reads:
// OnHover fires once per row change under the pointer (not on leaving
// the menu), and RowBounds is the row the pointer is over.
func TestMenuHoverHookAndRowBounds(t *testing.T) {
	face := entryFace(t)
	m := NewMenu(face, 13, MenuItem{Label: "one"}, MenuItem{Label: "two"}, MenuItem{Label: "three"})
	m.Measure(Constraints{Max: Size{W: 200, H: 400}})
	m.Arrange(render.Rect{X: 10, Y: 20, W: 120, H: 200})
	var hovered []int
	m.OnHover = func(i int) { hovered = append(hovered, i) }
	center := func(i int) Point {
		r := m.RowBounds(i)
		return Point{X: r.X + r.W/2, Y: r.Y + r.H/2}
	}
	m.HoverMove(center(1))
	m.HoverMove(center(1))
	m.HoverMove(center(2))
	m.SetHovered(false)
	if len(hovered) != 2 || hovered[0] != 1 || hovered[1] != 2 {
		t.Errorf("hovered = %v, want [1 2]", hovered)
	}
	if got := m.itemAt(center(2)); got != 2 {
		t.Errorf("RowBounds(2) center maps to row %d", got)
	}
	if r := m.RowBounds(0); r.X != 10 || r.W != 120 || r.Y < 20 {
		t.Errorf("RowBounds(0) = %v", r)
	}
	if !m.RowBounds(3).Empty() || !m.RowBounds(-1).Empty() {
		t.Error("an out-of-range row has bounds")
	}
	if len(m.Items()) != 3 {
		t.Error("Items")
	}
}

// A separator is a thin row, its rule 4px from the rows either side;
// it took a full item band, leaving a row-high gap
// in every menu (#281). Hit-testing below it still lands on the row
// drawn there, and the rows sit inside the menu's own border.
func TestMenuSeparatorIsAThinRow(t *testing.T) {
	m := NewMenu(entryFace(t), 13, MenuItem{Label: "one"}, MenuSeparator(), MenuItem{Label: "two"})
	size := m.Measure(Constraints{Max: Size{W: 200, H: 400}})
	m.Arrange(render.Rect{X: 0, Y: 0, W: size.W, H: size.H})
	one, sep, two := m.RowBounds(0), m.RowBounds(1), m.RowBounds(2)
	if sep.H != menuSeparatorBand || sep.Y != one.Y+one.H+menuRowGap || two.Y != sep.Y+sep.H+menuRowGap {
		t.Errorf("rows %v, %v, %v: want the separator a %dpx band between them", one, sep, two, menuSeparatorBand)
	}
	if got := m.itemAt(Point{X: two.X + 4, Y: two.Y + two.H/2}); got != 2 {
		t.Errorf("the second item's center maps to row %d", got)
	}
	if want := two.Y + two.H + menuRowGap + m.contentsInset().Bottom; size.H != want {
		t.Errorf("menu height %d, want the rows inside the menu's box, %d", size.H, want)
	}
}

// A separator's rule spans the menu's inside, edge to edge as the
// reference's (-mx-1 under the p-1 contents) does; it stopped 8px short
// of the row on each side (#281).
func TestMenuSeparatorSpansTheMenu(t *testing.T) {
	m := NewMenu(entryFace(t), 13, MenuItem{Label: "one", OnClick: func() {}}, MenuSeparator(), MenuItem{Label: "two", OnClick: func() {}})
	sz := m.Measure(Constraints{Max: Size{W: 300, H: 300}})
	m.Arrange(render.Rect{W: sz.W, H: sz.H})
	data := make([]byte, render.Stride(sz.W)*sz.H)
	cv := render.New(data, render.Stride(sz.W), sz.W, sz.H)
	m.Paint(cv)
	sep := m.RowBounds(1)
	y := sep.Y + sep.H/2
	_, inside := boxRects(boxOf(m.style(m), render.Insets{}), m.bounds)
	for _, x := range []int{inside.X + 1, inside.X + inside.W - 2} {
		if got := render.ColorFromBytes(data[y*render.Stride(sz.W)+x*4:]); got == m.style(m).Background {
			t.Errorf("no rule at x=%d on the separator row", x)
		}
	}
}
