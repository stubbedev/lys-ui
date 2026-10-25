package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// newDropdown builds a face-equipped dropdown arranged at (10, 20), so
// the open list hangs below at a known offset.
func newDropdown(t *testing.T, items ...string) *Dropdown {
	t.Helper()
	dd := NewDropdown(entryFace(t), 14, items, 0)
	sz := dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
	dd.Arrange(render.Rect{X: 10, Y: 20, W: sz.W, H: sz.H})
	return dd
}

// rowPoint maps item i of the open list to a clickable point.
func rowPoint(dd *Dropdown, i int) Point {
	return Point{X: dd.menu.Bounds().X + 10, Y: rowCenter(dd.menu, i).Y}
}

func facePoint(dd *Dropdown) Point {
	b := dd.Bounds()
	return Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
}

func TestDropdownClick(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "Light", "Dark", "System")

	t.Run("clicking the face opens the list below it", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))
		c.drive() // land the reveal: the list rests exactly below the face
		if !dd.Opened() {
			t.Fatal("face click did not open the list")
		}
		if b := dd.menu.Bounds(); b.Y != dd.Bounds().Y+dd.Bounds().H {
			t.Errorf("list top = %d, want directly below the face at %d", b.Y, dd.Bounds().Y+dd.Bounds().H)
		}
	})

	t.Run("clicking a row selects it exactly once and closes", func(t *testing.T) {
		fired := 0
		dd.OnSelect = func(int) { fired++ }
		// Row clicks reach the list through its own ClickAt, exactly
		// what the Router does when the hit lands on the open list.
		dd.menu.ClickAt(rowPoint(dd, 2))
		c.drive() // the close tween lands before the assertions
		if dd.Selected() != 2 {
			t.Errorf("selected = %d, want 2", dd.Selected())
		}
		if fired != 1 {
			t.Errorf("OnSelect fired %d times, want exactly 1", fired)
		}
		if dd.Opened() {
			t.Error("picking a row left the list open")
		}
	})

	t.Run("hit test resolves the list outside the face while open", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))
		if got := dd.HitTest(rowPoint(dd, 1)); got != Widget(dd.menu) {
			t.Errorf("hit = %v, want the item list", got)
		}
		dd.ClickAt(facePoint(dd)) // second face click toggles closed
		c.drive()
		if dd.Opened() {
			t.Error("second face click did not close the list")
		}
	})
}

func TestDropdownRouterClick(t *testing.T) {
	dd := newDropdown(t, "Light", "Dark", "System")
	root := NewBox(Row, 0, 0)
	root.Append(dd, false)
	sz := root.Measure(Constraints{Max: Size{W: 400, H: 400}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
	router := &Router{Root: root}

	fired := 0
	dd.OnSelect = func(int) { fired++ }

	// A pointer click on the face opens; the press also focuses, so
	// the later keyboard test runs against a realistically focused
	// dropdown.
	router.Press(BTNLeft, facePoint(dd))
	router.Release(BTNLeft, facePoint(dd))
	if !dd.Opened() {
		t.Fatal("router click did not open the list")
	}
	if router.Focused() != Widget(dd) {
		t.Fatal("face click did not focus the dropdown")
	}

	router.Press(BTNLeft, rowPoint(dd, 1))
	router.Release(BTNLeft, rowPoint(dd, 1))
	if dd.Selected() != 1 || fired != 1 {
		t.Errorf("router click: selected = %d fired = %d, want 1/1", dd.Selected(), fired)
	}
}

func TestDropdownKeyboard(t *testing.T) {
	c := pinAnimClock(t)
	t.Run("Enter, Space, and Down open while focused", func(t *testing.T) {
		for _, key := range []struct {
			name   string
			points func(*Dropdown)
		}{
			{"Enter", func(dd *Dropdown) { dd.KeyAction(KeyEnter, 0) }},
			{"Space", func(dd *Dropdown) { dd.InsertRune(' ') }},
			{"Down", func(dd *Dropdown) { dd.KeyAction(KeyDown, 0) }},
		} {
			dd := newDropdown(t, "Light", "Dark", "System")
			key.points(dd)
			if !dd.Opened() {
				t.Errorf("%s did not open the list", key.name)
			}
		}
	})

	t.Run("arrows navigate from the selection, Enter picks once", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		fired := 0
		dd.OnSelect = func(int) { fired++ }

		dd.KeyAction(KeyEnter, 0) // open
		if got := dd.menu.hovered; got != 0 {
			t.Fatalf("list opened with highlight %d, want the selection 0", got)
		}
		dd.KeyAction(KeyDown, 0)
		if got := dd.menu.hovered; got != 1 {
			t.Fatalf("Down moved highlight to %d, want 1", got)
		}
		dd.KeyAction(KeyUp, 0)
		dd.KeyAction(KeyDown, 0) // back to 1
		dd.KeyAction(KeyEnter, 0)
		c.drive()

		if dd.Selected() != 1 {
			t.Errorf("selected = %d, want 1", dd.Selected())
		}
		if fired != 1 {
			t.Errorf("OnSelect fired %d times, want exactly 1", fired)
		}
		if dd.Opened() {
			t.Error("Enter left the list open")
		}
	})

	t.Run("Esc cancels without changing the selection", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		fired := 0
		dd.OnSelect = func(int) { fired++ }

		dd.KeyAction(KeyEnter, 0)
		dd.KeyAction(KeyDown, 0)
		dd.KeyAction(KeyDown, 0)
		dd.KeyAction(KeyDismiss, 0)
		c.drive()

		if dd.Opened() {
			t.Fatal("Esc did not close the list")
		}
		if dd.Selected() != 0 {
			t.Errorf("cancel changed the selection to %d, want 0", dd.Selected())
		}
		if fired != 0 {
			t.Errorf("cancel fired OnSelect %d times, want 0", fired)
		}
	})

	t.Run("arrows do nothing while closed", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.KeyAction(KeyUp, 0)
		if dd.Opened() || dd.Selected() != 0 {
			t.Errorf("closed arrows: open = %v selected = %d", dd.Opened(), dd.Selected())
		}
	})
}

func TestDropdownOnSelectExactlyOnce(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "Light", "Dark", "System")
	fired := 0
	dd.OnSelect = func(int) { fired++ }

	t.Run("re-opening and re-picking the selected row fires nothing", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))        // open
		dd.menu.ClickAt(rowPoint(dd, 0)) // pick the already-selected row
		c.drive()                        // land the close tween
		if fired != 0 {
			t.Errorf("no-change pick fired %d times, want 0", fired)
		}
	})

	t.Run("open plus Enter on the seeded highlight fires nothing", func(t *testing.T) {
		dd.KeyAction(KeyEnter, 0) // open, highlight on the selection
		dd.KeyAction(KeyEnter, 0) // re-pick it
		c.drive()
		if fired != 0 {
			t.Errorf("re-pick fired %d times, want 0", fired)
		}
	})

	t.Run("each real change fires exactly once, pointer or not", func(t *testing.T) {
		dd.SetSelected(2)
		dd.SetSelected(2)
		if fired != 1 {
			t.Errorf("SetSelected fired %d times, want 1", fired)
		}
		dd.KeyAction(KeyEnter, 0)
		dd.KeyAction(KeyUp, 0) // 2 -> 1
		dd.KeyAction(KeyEnter, 0)
		c.drive()
		if fired != 2 {
			t.Errorf("keyboard change fired %d times total, want 2", fired)
		}
		if dd.Selected() != 1 {
			t.Errorf("selected = %d, want 1", dd.Selected())
		}
	})
}

func TestDropdownOf(t *testing.T) {
	type mode string
	items := []DropdownItem[mode]{
		{Label: "Light", Value: "light"},
		{Label: "Dark", Value: "dark"},
		{Label: "System", Value: "system"},
	}
	dd := NewDropdownOf(entryFace(t), 14, items, 0)
	sz := dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
	dd.Arrange(render.Rect{X: 10, Y: 20, W: sz.W, H: sz.H})

	if got := dd.Value(); got != "light" {
		t.Fatalf("initial value = %q, want light", got)
	}

	calls := 0
	var got mode
	dd.OnSelect = func(i int, v mode) {
		calls++
		got = v
	}

	dd.KeyAction(KeyEnter, 0)
	dd.KeyAction(KeyDown, 0)
	dd.KeyAction(KeyEnter, 0)
	if calls != 1 || got != "dark" {
		t.Fatalf("keyboard pick: calls = %d value = %q, want 1/dark", calls, got)
	}
	if v := dd.Value(); v != "dark" {
		t.Errorf("Value = %q, want dark", v)
	}

	dd.SetSelected(2)
	if v := dd.Value(); v != "system" {
		t.Errorf("Value after SetSelected = %q, want system", v)
	}
	if got := dd.Selected(); got != 2 {
		t.Errorf("Selected = %d, want 2", got)
	}
}

func TestDropdownChildren(t *testing.T) {
	dd := newDropdown(t, "Light", "Dark", "System")
	root := NewBox(Row, 0, 0)
	root.Append(dd, false)
	sz := root.Measure(Constraints{Max: Size{W: 400, H: 400}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
	router := &Router{Root: root}

	t.Run("closed hides the list from traversal", func(t *testing.T) {
		if kids := dd.Children(); kids != nil {
			t.Errorf("closed children = %v, want none", kids)
		}
		// Focus the face without clicking it open: a press focuses,
		// and CancelPress drops the gesture before it becomes one.
		router.Press(BTNLeft, facePoint(dd))
		router.CancelPress()
		if router.Focused() != Widget(dd) {
			t.Fatal("press did not focus the dropdown")
		}
		router.FocusNext()
		if got := router.Focused(); got != Widget(dd) {
			t.Errorf("closed traversal focused %v, want the dropdown to stay put", got)
		}
	})

	t.Run("open exposes exactly the list for traversal", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))
		kids := dd.Children()
		if len(kids) != 1 || kids[0] != Widget(dd.menu) {
			t.Fatalf("open children = %v, want exactly the item list", kids)
		}
		router.FocusNext()
		if got := router.Focused(); got != Widget(dd.menu) {
			t.Errorf("open traversal focused %v, want the item list", got)
		}
	})
}

func TestDropdownDisabled(t *testing.T) {
	c := pinAnimClock(t)
	t.Run("disabled ignores clicks and keys", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.SetEnabled(false)
		if dd.Enabled() {
			t.Fatal("SetEnabled(false) left the dropdown enabled")
		}

		dd.ClickAt(facePoint(dd))
		dd.KeyAction(KeyEnter, 0)
		dd.KeyAction(KeyDown, 0)
		dd.InsertRune(' ')
		if dd.Opened() {
			t.Error("a disabled dropdown opened")
		}
		dd.SetSelected(1) // programmatic changes still apply
		if dd.Selected() != 1 {
			t.Errorf("SetSelected on disabled = %d, want 1", dd.Selected())
		}
	})

	t.Run("disabling closes an open list", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.KeyAction(KeyEnter, 0)
		dd.SetEnabled(false)
		c.drive()
		if dd.Opened() || dd.Children() != nil {
			t.Error("disabling left the list open or traversable")
		}
	})
}

func TestDropdownDamage(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "Light", "Dark", "System")

	// Opening must owe the list's pixels; closing must owe them back,
	// even though the closed list is no longer exposed as a child.
	dd.ClickAt(facePoint(dd))
	c.drive() // land the reveal
	rects, any := CollectDamage(dd)
	if !any {
		t.Fatal("opening owed no damage")
	}
	list := dd.menu.Bounds()
	if !slices.Contains(rects, list) {
		t.Errorf("open damage %v missed the list rect %v", rects, list)
	}
	CollectDamage(dd) // drain

	dd.ClickAt(facePoint(dd)) // close
	rects, any = CollectDamage(dd)
	if !any {
		t.Fatal("closing owed no damage")
	}
	if !slices.Contains(rects, list) {
		t.Errorf("close damage %v missed the old list rect %v", rects, list)
	}
	if _, any := CollectDamage(dd); any {
		t.Error("idle dropdown still owes damage")
	}
}

func TestDropdownPaintAndRole(t *testing.T) {
	c := pinAnimClock(t)
	t.Run("open list paints ink on every row", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.ClickAt(facePoint(dd))
		c.drive() // land the reveal: the list paints at rest
		b := dd.Bounds().Union(dd.menu.Bounds())
		data := make([]byte, render.Stride(b.W)*(b.H+2))
		cv := render.New(data, render.Stride(b.W), b.W, b.H+2)
		dd.Arrange(render.Rect{X: 0, Y: 0, W: dd.Bounds().W, H: dd.Bounds().H})
		dd.Paint(cv)
		for row := range 3 {
			y := rowCenter(dd.menu, row).Y
			ink := 0
			for x := range dd.menu.Bounds().W {
				if render.ColorFromBytes(data[y*render.Stride(b.W)+x*4:]).A() > 0 {
					ink++
				}
			}
			if ink == 0 {
				t.Errorf("row %d painted nothing", row)
			}
		}
	})

	t.Run("dropdown with an empty item list paints and stays closed", func(t *testing.T) {
		dd := NewDropdown(entryFace(t), 14, nil, 0)
		sz := dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
		if sz.W <= 0 || sz.H <= 0 {
			t.Fatalf("empty measure = %v", sz)
		}
		dd.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
		dd.ClickAt(facePoint(dd))
		if dd.Opened() {
			t.Error("a dropdown without items opened a list")
		}
		cv := render.New(make([]byte, render.Stride(sz.W)*sz.H), render.Stride(sz.W), sz.W, sz.H)
		dd.Paint(cv)
	})

	t.Run("role is combo-box and the name is the selection", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark")
		dd.SetSelected(1)
		st := Describe(dd)
		if st.Role != RoleComboBox || st.Role.String() != "combo-box" {
			t.Errorf("role = %v, want combo-box", st.Role)
		}
		if st.Name != "Dark" {
			t.Errorf("a11y name = %q, want the selection", st.Name)
		}
	})
}

// fillSpy paints its whole rect in one color: a later sibling that
// would cover anything painted beneath it.
type fillSpy struct {
	stub
	color render.Color
}

func (f *fillSpy) Paint(cv *render.Canvas) { cv.FillRect(f.rect, f.color) }

// TestDropdownListPaintsAboveLaterSiblings pins the open list on the
// frame's top layer: in a column, the rows after the dropdown paint
// first and the list over them, the way a popover covers them. Painted
// inline, the next row covered the list.
func TestDropdownListPaintsAboveLaterSiblings(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "Light", "Dark", "System")
	red := render.RGB(255, 0, 0)
	next := &fillSpy{nat: Size{W: 200, H: 200}, color: red}
	col := NewBox(Column, 0, 0)
	col.Append(dd, false)
	col.Append(next, false)
	col.Measure(Constraints{Max: Size{W: 200, H: 400}})
	col.Arrange(render.Rect{W: 200, H: 400})
	dd.ClickAt(facePoint(dd))
	c.drive()
	col.Arrange(render.Rect{W: 200, H: 400})
	list := dd.menu.Bounds()
	if list.Intersect(next.rect).Empty() {
		t.Fatalf("the list %v does not hang over the next row %v", list, next.rect)
	}
	paint := func(armed bool) render.Color {
		data := make([]byte, render.Stride(200)*400)
		cv := render.New(data, render.Stride(200), 200, 400)
		if armed {
			cv.BeginOverlays()
		}
		col.Paint(cv)
		if armed {
			cv.FlushOverlays()
		}
		x, y := list.X+list.W/2, max(list.Y, next.rect.Y)+list.H/3
		return render.ColorFromBytes(data[y*render.Stride(200)+x*4:])
	}
	if got := paint(true); got == red {
		t.Error("in a frame the next row painted over the open list")
	}
	// A subtree painted on its own keeps the in-place paint order.
	if got := paint(false); got != red {
		t.Errorf("unarmed, the list escaped the paint order (%v)", got)
	}
}

// TestFocusRingIsCoveredByLaterSiblings pins the ring in paint order: a
// card overlaid after the focused widget covers the ring where they
// overlap, instead of the ring crossing the card.
func TestFocusRingIsCoveredByLaterSiblings(t *testing.T) {
	red, blue := render.RGB(255, 0, 0), render.RGB(0, 0, 255)
	focused := &fillSpy{nat: Size{W: 100, H: 100}, color: render.RGB(0, 255, 0)}
	card := &fillSpy{nat: Size{W: 100, H: 100}, color: blue}
	ov := NewOverlay().Append(focused).Append(card)
	ov.Measure(Constraints{Max: Size{W: 100, H: 100}})
	ov.Arrange(render.Rect{W: 100, H: 100})
	ring := func(cv *render.Canvas) { cv.BorderRect(render.Rect{X: 10, Y: 10, W: 50, H: 50}, 2, red) }
	paint := func(card bool) render.Color {
		data := make([]byte, render.Stride(100)*100)
		cv := render.New(data, render.Stride(100), 100, 100)
		cv.MarkFocus(focused, ring)
		if card {
			ov.Paint(cv)
		} else {
			focused.Paint(cv)
			cv.Painted(focused)
		}
		cv.FinishFocus()
		return render.ColorFromBytes(data[10*render.Stride(100)+30*4:])
	}
	if got := paint(true); got != blue {
		t.Errorf("under the card the ring shows (%v)", got)
	}
	if got := paint(false); got != red {
		t.Errorf("uncovered, the ring is missing (%v)", got)
	}
}

// The open list claims no mnemonics: no underlines, and Alt-letters
// activate nothing (type-ahead is the dropdown's keyboard model).
func TestDropdownListHasNoMnemonics(t *testing.T) {
	d := NewDropdown(testFace(t), 13, []string{"None", "Fade", "Zoom"}, 0)
	m := d.list()
	for i, r := range m.mnemRunes {
		if r >= 0 {
			t.Errorf("row %d claims mnemonic rune %d", i, r)
		}
	}
	if m.ActivateMnemonic('f') {
		t.Error("an Alt-letter activated a dropdown row")
	}
	if NewMenu(testFace(t), 13, MenuItem{Label: "Fade"}).mnemRunes[0] < 0 {
		t.Error("test premise: a plain menu auto-assigns its mnemonic")
	}
}

// Rich rows: headers are captions (never selected, typed to, or
// stepped onto) and icons ride the list rows and the face.
func TestDropdownRows(t *testing.T) {
	built := 0
	dot := func() *Icon {
		built++
		return NewSVGIcon([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><circle cx="4" cy="4" r="4"/></svg>`), 8)
	}
	rows := []DropdownRow{
		{Label: "Colors", Header: true},
		{Label: "Red", Icon: dot},
		{Label: "Blue", Icon: dot},
		{Label: "Greys", Header: true},
		{Label: "Black"},
	}
	d := NewDropdownRows(testFace(t), 13, rows, 0)
	if d.Selected() != 1 {
		t.Fatalf("selected %d, want the first row after the header", d.Selected())
	}
	var fired []int
	d.OnSelect = func(i int) { fired = append(fired, i) }
	d.SetSelected(3)
	if d.Selected() != 1 || len(fired) != 0 {
		t.Errorf("selecting a header moved to %d (fired %v)", d.Selected(), fired)
	}
	d.InsertRune('g')
	if d.Selected() != 1 {
		t.Errorf("type-ahead landed on the Greys header (%d)", d.Selected())
	}
	d.InsertRune('b')
	if d.Selected() != 2 {
		t.Errorf("type-ahead b = %d, want Blue", d.Selected())
	}
	d.Open()
	d.InsertRune('g')
	if d.menu.hovered == 3 {
		t.Error("open type-ahead highlighted the Greys header")
	}
	d.Close()
	m := d.list()
	if m.selectable(0) || m.selectable(3) || !m.selectable(4) || m.items[0].Kind != ItemHeader {
		t.Error("the list's headers are selectable")
	}
	if m.items[1].Icon == nil || m.items[4].Icon != nil {
		t.Error("the list rows did not take their icons")
	}
	m.activate(0)
	if d.Selected() != 2 {
		t.Error("activating a header selected it")
	}
	if ic := d.selectedIcon(); ic == nil || d.selectedIcon() != ic {
		t.Error("the face has no stable icon for an icon row")
	}
	d.SetSelected(4)
	if d.selectedIcon() != nil {
		t.Error("an iconless row kept the face icon")
	}
	if d.iconSlot() <= dropdownIconGap {
		t.Errorf("icon slot %d, want the icon width plus the gap", d.iconSlot())
	}
	if only := NewDropdownRows(testFace(t), 13, []DropdownRow{{Label: "h", Header: true}}, 0); only.Selected() != -1 {
		t.Errorf("all-header dropdown selected %d", only.Selected())
	}
}

func TestDropdownSetItemsReplacesTheList(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "Default")
	fired := 0
	dd.OnSelect = func(int) { fired++ }
	dd.ClickAt(facePoint(dd))
	c.drive()
	dd.Measure(Constraints{Max: Size{W: 400, H: 100}}) // settle the layout cache
	dd.SetItems([]string{"Default", "Headset", "Built-in microphone, wide"}, 2)
	if dd.Opened() {
		t.Error("replacing the items left the old list open")
	}
	if dd.Selected() != 2 || dd.Selection() != "Built-in microphone, wide" || fired != 0 {
		t.Errorf("selected %d %q, OnSelect fired %d; want the new row, silently", dd.Selected(), dd.Selection(), fired)
	}
	if !LayoutPending(dd) || dd.Measure(Constraints{Max: Size{W: 400, H: 100}}).W <= newDropdown(t, "Default").Bounds().W {
		t.Error("a wider item did not relayout and widen the face")
	}
	dd.Arrange(render.Rect{X: 10, Y: 20, W: 300, H: dd.Bounds().H})
	dd.ClickAt(facePoint(dd))
	c.drive()
	if len(dd.menu.items) != 3 {
		t.Errorf("the reopened list shows %d rows, want the 3 new ones", len(dd.menu.items))
	}
	dd.menu.ClickAt(rowPoint(dd, 1))
	c.drive()
	if dd.Selected() != 1 || fired != 1 {
		t.Errorf("picking a new row: selected %d, fired %d", dd.Selected(), fired)
	}
	// Clamped like the constructor; empty is -1.
	dd.SetItems([]string{"only"}, 5)
	if dd.Selected() != 0 {
		t.Errorf("out of range selected %d, want clamped to 0", dd.Selected())
	}
	dd.SetItems(nil, 0)
	if dd.Selected() != -1 || dd.Selection() != "" {
		t.Errorf("empty: selected %d %q", dd.Selected(), dd.Selection())
	}
	dd.SetRows([]DropdownRow{{Label: "Group", Header: true}, {Label: "a"}}, 0)
	if dd.Selected() != 1 {
		t.Errorf("a header selection moved to %d, want the row after it", dd.Selected())
	}
}

// The closed face styles as `dropdown > button > arrow`: the button's
// box paints the face and the arrow's color the chevron strokes.
func TestDropdownButtonAndArrowNodes(t *testing.T) {
	loadCSS(t, `dropdown > button { background-color: #010203; border-radius: 5; } dropdown > button > arrow { color: #0a0b0c; }`)
	d := NewDropdown(testFace(t), 13, []string{"a", "b"}, 0)
	host := NewBox(Column, 0, 0)
	host.Append(d, false)
	frame(t, host, 100, 60)
	bv := d.faceBtn.style(&d.faceBtn)
	if got := bv.Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("face background %v, want the button rule", got)
	}
	if bv.Radius.TopLeft != 5 {
		t.Errorf("face radius %d, want 5", bv.Radius.TopLeft)
	}
	if got := pickc(0, d.faceArrow.style(&d.faceArrow), style.PropColor, 0); got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("arrow color %v, want the arrow rule", got)
	}
}
