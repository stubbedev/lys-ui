package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

// newDropdown builds a face-equipped dropdown arranged at (10, 20)
// whose tree a fake surface host serves.
func newDropdown(t *testing.T, items ...string) (*Dropdown, *fakeSurfaceHost) {
	t.Helper()
	return hostDropdown(t, NewDropdown(entryFace(t), 14, items, 0))
}

// hostDropdown attaches a fake surface host to dd and arranges it at
// (10, 20).
func hostDropdown(t *testing.T, dd *Dropdown) (*Dropdown, *fakeSurfaceHost) {
	t.Helper()
	host := &fakeSurfaceHost{}
	SetSurfaceHost(dd, host)
	sz := dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
	dd.Arrange(render.Rect{X: 10, Y: 20, W: sz.W, H: sz.H})
	return dd, host
}

// openedList is the item list of the popover dd opened last, laid out
// in it.
func openedList(t *testing.T, dd *Dropdown, host *fakeSurfaceHost) *Command {
	t.Helper()
	if len(host.popovers) == 0 {
		t.Fatal("the dropdown opened no popover")
	}
	content := host.popovers[len(host.popovers)-1]
	SetStyleAnchor(content, dd)
	arrangeTree(t, content, 400, 400)
	return content.(*Box).Children()[0].(*Command)
}

// highlightedLabel is the label of the list's highlighted item.
func highlightedLabel(c *Command) string {
	it, _ := c.Highlighted()
	return it.Label
}

func facePoint(dd *Dropdown) Point {
	b := dd.Bounds()
	return Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
}

// The list opens in a popover through the surface host, never inside
// the dropdown's own window, where the window clipped it (#280): the
// popover holds a Command without a search row, focused, at least as
// wide as the face, and the dropdown keeps no in-window list.
func TestDropdownOpensThroughSurfaceHost(t *testing.T) {
	dd, host := newDropdown(t, "Light", "Dark", "System")
	dd.ClickAt(facePoint(dd))
	if !dd.Opened() || len(host.popovers) != 1 {
		t.Fatalf("a face click: opened %v with %d popovers, want one", dd.Opened(), len(host.popovers))
	}
	list := openedList(t, dd, host)
	if list.input != nil {
		t.Error("the list shows a search row")
	}
	if host.opts[0].Focus != Widget(list) || list.FocusTarget() != Widget(list) {
		t.Errorf("the popover focuses %T, want the list", host.opts[0].Focus)
	}
	if !isFocusable(list) {
		t.Error("the list cannot take the focus it is given")
	}
	if w := list.Bounds().W; w < dd.Bounds().W {
		t.Errorf("the list is %dpx wide, narrower than the %dpx face", w, dd.Bounds().W)
	}
	if got := shownLabels(list); len(got) != 3 || got[0] != "Light" || got[2] != "System" {
		t.Errorf("the list shows %v, want the items in order", got)
	}
	if parentOf(list) == Widget(dd) || dd.HitTest(Point{X: dd.Bounds().X + 2, Y: dd.Bounds().Y + dd.Bounds().H + 10}) != nil {
		t.Error("the dropdown still holds an in-window list")
	}
}

// The selected row is highlighted on open and alone carries the
// trailing check, the way a combobox marks its pick.
func TestDropdownListMarksTheSelection(t *testing.T) {
	dd, host := newDropdown(t, "Light", "Dark", "System")
	dd.SetSelected(1)
	dd.Open()
	list := openedList(t, dd, host)
	if got := highlightedLabel(list); got != "Dark" {
		t.Errorf("the list opened highlighting %q, want the selection", got)
	}
	for i, r := range list.rows {
		check, ok := r.item.Trailing.(*Box)
		if !ok {
			t.Fatalf("row %d trails %T, want its check slot", i, r.item.Trailing)
		}
		if got, want := len(check.Children()), map[bool]int{true: 1, false: 0}[i == 1]; got != want {
			t.Errorf("row %d's check slot holds %d widgets, want %d", i, got, want)
		}
		if row := r.Children(); row[len(row)-1] != Widget(check) {
			t.Errorf("row %d ends with %T, want its check", i, row[len(row)-1])
		}
	}
}

func TestDropdownClick(t *testing.T) {
	dd, host := newDropdown(t, "Light", "Dark", "System")
	fired := 0
	dd.OnSelect = func(int) { fired++ }

	dd.ClickAt(facePoint(dd))
	list := openedList(t, dd, host)
	list.rows[2].ClickAt(Point{})
	if dd.Selected() != 2 || fired != 1 {
		t.Errorf("clicking a row: selected %d, OnSelect fired %d times; want 2 once", dd.Selected(), fired)
	}
	if dd.Opened() || host.closed != 1 {
		t.Errorf("picking a row left the list open (%d closes)", host.closed)
	}

	dd.ClickAt(facePoint(dd))
	dd.ClickAt(facePoint(dd))
	if dd.Opened() || host.closed != 2 {
		t.Errorf("a second face click left the list open (%d closes)", host.closed)
	}
}

// A router click on the face opens and focuses it; a click on a row of
// the popover's own tree picks it.
func TestDropdownRouterClick(t *testing.T) {
	dd, host := newDropdown(t, "Light", "Dark", "System")
	root := NewBox(Row, 0, 0)
	root.Append(dd, false)
	arrangeTree(t, root, 400, 400)
	router := &Router{Root: root}
	fired := 0
	dd.OnSelect = func(int) { fired++ }

	router.Press(BTNLeft, facePoint(dd), 0)
	router.Release(BTNLeft, facePoint(dd))
	if !dd.Opened() {
		t.Fatal("router click did not open the list")
	}
	if router.Focused() != Widget(dd) {
		t.Fatal("face click did not focus the dropdown")
	}

	list := openedList(t, dd, host)
	pop := &Router{Root: host.popovers[0]}
	pop.SetFocus(host.opts[0].Focus)
	clickAt(pop, list.rows[1])
	if dd.Selected() != 1 || fired != 1 {
		t.Errorf("router click: selected = %d fired = %d, want 1/1", dd.Selected(), fired)
	}
	if pop.Focused() != Widget(list) {
		t.Errorf("a row click moved the popover's focus to %T, want the list", pop.Focused())
	}
}

func TestDropdownKeyboard(t *testing.T) {
	t.Run("Enter, Space, and Down open while focused", func(t *testing.T) {
		for _, key := range []struct {
			name  string
			press func(*Dropdown)
		}{
			{"Enter", func(dd *Dropdown) { dd.KeyAction(KeyEnter, 0) }},
			{"Space", func(dd *Dropdown) { dd.InsertRune(' ') }},
			{"Down", func(dd *Dropdown) { dd.KeyAction(KeyDown, 0) }},
		} {
			dd, host := newDropdown(t, "Light", "Dark", "System")
			key.press(dd)
			if !dd.Opened() || len(host.popovers) != 1 {
				t.Errorf("%s did not open the list", key.name)
			}
		}
	})

	t.Run("the focused list navigates from the selection, Enter picks once", func(t *testing.T) {
		dd, host := newDropdown(t, "Light", "Dark", "System")
		fired := 0
		dd.OnSelect = func(int) { fired++ }
		dd.KeyAction(KeyEnter, 0)
		openedList(t, dd, host)
		r := &Router{Root: host.popovers[0]}
		r.SetFocus(host.opts[0].Focus)
		list := r.Focused().(*Command)
		r.KeyAction(KeyDown, 0)
		if got := highlightedLabel(list); got != "Dark" {
			t.Fatalf("Down highlighted %q, want Dark", got)
		}
		r.KeyAction(KeyEnd, 0)
		r.KeyAction(KeyHome, 0)
		r.KeyAction(KeyDown, 0)
		r.KeyAction(KeyEnter, 0)
		if dd.Selected() != 1 || fired != 1 {
			t.Errorf("selected %d, fired %d; want 1 once", dd.Selected(), fired)
		}
		if dd.Opened() {
			t.Error("Enter left the list open")
		}
	})

	t.Run("Space picks in the list", func(t *testing.T) {
		dd, host := newDropdown(t, "Light", "Dark", "System")
		dd.Open()
		list := openedList(t, dd, host)
		list.KeyAction(KeyUp, 0)
		list.KeyAction(KeyEnd, 0)
		list.InsertRune(' ')
		if dd.Selected() != 2 || dd.Opened() {
			t.Errorf("Space: selected %d, open %v; want System, closed", dd.Selected(), dd.Opened())
		}
	})

	t.Run("a popover closed by the host changes nothing", func(t *testing.T) {
		dd, host := newDropdown(t, "Light", "Dark", "System")
		fired := 0
		dd.OnSelect = func(int) { fired++ }
		dd.KeyAction(KeyEnter, 0)
		list := openedList(t, dd, host)
		list.KeyAction(KeyDown, 0)
		host.opts[0].OnClosed()
		if dd.Opened() {
			t.Fatal("the closed popover left the dropdown open")
		}
		if dd.Selected() != 0 || fired != 0 {
			t.Errorf("the dismissal selected %d and fired %d times", dd.Selected(), fired)
		}
		dd.Close()
		if host.closed != 0 {
			t.Error("Close closed the popover the host had already closed")
		}
	})

	t.Run("arrows do nothing while closed", func(t *testing.T) {
		dd, _ := newDropdown(t, "Light", "Dark", "System")
		dd.KeyAction(KeyUp, 0)
		if dd.Opened() || dd.Selected() != 0 {
			t.Errorf("closed arrows: open = %v selected = %d", dd.Opened(), dd.Selected())
		}
	})
}

// A popover still closing after the list reopened does not close the
// new one.
func TestDropdownStaleCloseKeepsTheNewList(t *testing.T) {
	dd, host := newDropdown(t, "Light", "Dark")
	dd.Open()
	dd.Close()
	dd.Open()
	host.opts[0].OnClosed()
	if !dd.Opened() {
		t.Error("the first popover's late close closed the reopened list")
	}
	host.opts[1].OnClosed()
	if dd.Opened() {
		t.Error("the open popover's close left the dropdown open")
	}
}

func TestDropdownOnSelectExactlyOnce(t *testing.T) {
	dd, host := newDropdown(t, "Light", "Dark", "System")
	fired := 0
	dd.OnSelect = func(int) { fired++ }

	t.Run("re-opening and re-picking the selected row fires nothing", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))
		openedList(t, dd, host).rows[0].ClickAt(Point{})
		if fired != 0 {
			t.Errorf("no-change pick fired %d times, want 0", fired)
		}
	})

	t.Run("open plus Enter on the seeded highlight fires nothing", func(t *testing.T) {
		dd.KeyAction(KeyEnter, 0)
		openedList(t, dd, host).KeyAction(KeyEnter, 0)
		if fired != 0 || dd.Opened() {
			t.Errorf("re-pick fired %d times, open %v", fired, dd.Opened())
		}
	})

	t.Run("each real change fires exactly once, pointer or not", func(t *testing.T) {
		dd.SetSelected(2)
		dd.SetSelected(2)
		if fired != 1 {
			t.Errorf("SetSelected fired %d times, want 1", fired)
		}
		dd.KeyAction(KeyEnter, 0)
		list := openedList(t, dd, host)
		list.KeyAction(KeyUp, 0)
		list.KeyAction(KeyEnter, 0)
		if fired != 2 || dd.Selected() != 1 {
			t.Errorf("keyboard change: fired %d times total, selected %d; want 2 and 1", fired, dd.Selected())
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
	typed := NewDropdownOf(entryFace(t), 14, items, 0)
	dd, host := hostDropdown(t, typed.Dropdown)

	if got := typed.Value(); got != "light" {
		t.Fatalf("initial value = %q, want light", got)
	}
	calls := 0
	var got mode
	typed.OnSelect = func(_ int, v mode) {
		calls++
		got = v
	}

	dd.KeyAction(KeyEnter, 0)
	list := openedList(t, dd, host)
	list.KeyAction(KeyDown, 0)
	list.KeyAction(KeyEnter, 0)
	if calls != 1 || got != "dark" {
		t.Fatalf("keyboard pick: calls = %d value = %q, want 1/dark", calls, got)
	}
	if v := typed.Value(); v != "dark" {
		t.Errorf("Value = %q, want dark", v)
	}

	typed.SetSelected(2)
	if v := typed.Value(); v != "system" {
		t.Errorf("Value after SetSelected = %q, want system", v)
	}
	if got := typed.Selected(); got != 2 {
		t.Errorf("Selected = %d, want 2", got)
	}
}

func TestDropdownDisabled(t *testing.T) {
	t.Run("disabled ignores clicks and keys", func(t *testing.T) {
		dd, host := newDropdown(t, "Light", "Dark", "System")
		dd.SetEnabled(false)
		if dd.Enabled() {
			t.Fatal("SetEnabled(false) left the dropdown enabled")
		}
		dd.ClickAt(facePoint(dd))
		dd.KeyAction(KeyEnter, 0)
		dd.KeyAction(KeyDown, 0)
		dd.InsertRune(' ')
		dd.Open()
		if dd.Opened() || len(host.popovers) != 0 {
			t.Error("a disabled dropdown opened")
		}
		dd.SetSelected(1)
		if dd.Selected() != 1 {
			t.Errorf("SetSelected on disabled = %d, want 1", dd.Selected())
		}
	})

	t.Run("disabling closes an open list", func(t *testing.T) {
		dd, host := newDropdown(t, "Light", "Dark", "System")
		dd.KeyAction(KeyEnter, 0)
		dd.SetEnabled(false)
		if dd.Opened() || host.closed != 1 {
			t.Errorf("disabling left the list open (%d closes)", host.closed)
		}
	})
}

// Opening and closing repaint the face, which shows the pressed shade
// while the list is open.
func TestDropdownDamage(t *testing.T) {
	dd, _ := newDropdown(t, "Light", "Dark", "System")
	CollectDamage(dd)
	dd.ClickAt(facePoint(dd))
	if _, any := CollectDamage(dd); !any {
		t.Fatal("opening owed no damage")
	}
	dd.ClickAt(facePoint(dd))
	if _, any := CollectDamage(dd); !any {
		t.Fatal("closing owed no damage")
	}
	if _, any := CollectDamage(dd); any {
		t.Error("idle dropdown still owes damage")
	}
}

func TestDropdownPaintAndRole(t *testing.T) {
	t.Run("dropdown with an empty item list paints and stays closed", func(t *testing.T) {
		dd, host := hostDropdown(t, NewDropdown(entryFace(t), 14, nil, 0))
		sz := dd.Bounds()
		if sz.W <= 0 || sz.H <= 0 {
			t.Fatalf("empty measure = %v", sz)
		}
		dd.ClickAt(facePoint(dd))
		if dd.Opened() || len(host.popovers) != 0 {
			t.Error("a dropdown without items opened a list")
		}
		dd.Arrange(render.Rect{W: sz.W, H: sz.H})
		dd.Paint(render.New(make([]byte, render.Stride(sz.W)*sz.H), render.Stride(sz.W), sz.W, sz.H))
	})

	t.Run("role is combo-box and the name is the selection", func(t *testing.T) {
		dd, _ := newDropdown(t, "Light", "Dark")
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

// Rich rows: headers head the list's groups (never selected, typed to,
// or stepped onto) and leading widgets, icons and swatches, ride the
// list rows and the face.
func TestDropdownRows(t *testing.T) {
	dot := func() Widget {
		return NewSVGIcon([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"><circle cx="4" cy="4" r="4"/></svg>`), 8)
	}
	blue := func() Widget { return NewColorSwatch(render.RGB(0, 0, 0xff)) }
	rows := []DropdownRow{
		{Label: "Colors", Header: true},
		{Label: "Red", Leading: dot},
		{Label: "Blue", Leading: blue},
		{Label: "Greys", Header: true},
		{Label: "Black"},
	}
	d, host := hostDropdown(t, NewDropdownRows(testFace(t), 13, rows, 0))
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
	list := openedList(t, d, host)
	if got := shownLabels(list); len(got) != 3 || got[0] != "Red" || got[2] != "Black" {
		t.Errorf("the list rows are %v, want the three choices", got)
	}
	if len(list.headings) != 2 || list.headings[0].Text() != "Colors" || list.headings[1].Text() != "Greys" {
		t.Error("the headers do not head the list's groups")
	}
	if _, swatch := list.rows[1].item.Icon.(*ColorSwatch); list.rows[0].item.Icon == nil || !swatch || list.rows[2].item.Icon != nil {
		t.Error("the list rows did not take their leading widgets")
	}
	if got := highlightedLabel(list); got != "Blue" {
		t.Errorf("the list opened on %q, want the selection", got)
	}
	list.InsertRune('g')
	if got := highlightedLabel(list); got != "Blue" {
		t.Errorf("open type-ahead g moved to %q, want no match", got)
	}
	list.rows[2].ClickAt(Point{})
	if d.Selected() != 4 || len(fired) != 2 || fired[1] != 4 {
		t.Errorf("picking Black selected %d (fired %v), want its row 4", d.Selected(), fired)
	}

	d.SetSelected(1)
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
	only, onlyHost := hostDropdown(t, NewDropdownRows(testFace(t), 13, []DropdownRow{{Label: "h", Header: true}}, 0))
	if only.Selected() != -1 {
		t.Errorf("all-header dropdown selected %d", only.Selected())
	}
	if only.Open(); only.Opened() || len(onlyHost.popovers) != 0 {
		t.Error("an all-header dropdown opened a list with nothing to pick")
	}
}

func TestDropdownSetItemsReplacesTheList(t *testing.T) {
	dd, host := newDropdown(t, "Default")
	fired := 0
	dd.OnSelect = func(int) { fired++ }
	dd.ClickAt(facePoint(dd))
	dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
	dd.SetItems([]string{"Default", "Headset", "Built-in microphone, wide"}, 2)
	if dd.Opened() || host.closed != 1 {
		t.Error("replacing the items left the old list open")
	}
	if dd.Selected() != 2 || dd.Selection() != "Built-in microphone, wide" || fired != 0 {
		t.Errorf("selected %d %q, OnSelect fired %d; want the new row, silently", dd.Selected(), dd.Selection(), fired)
	}
	narrow, _ := newDropdown(t, "Default")
	if !LayoutPending(dd) || dd.Measure(Constraints{Max: Size{W: 400, H: 100}}).W <= narrow.Bounds().W {
		t.Error("a wider item did not relayout and widen the face")
	}
	dd.Arrange(render.Rect{X: 10, Y: 20, W: 300, H: dd.Bounds().H})
	dd.ClickAt(facePoint(dd))
	list := openedList(t, dd, host)
	if len(list.rows) != 3 {
		t.Errorf("the reopened list shows %d rows, want the 3 new ones", len(list.rows))
	}
	list.rows[1].ClickAt(Point{})
	if dd.Selected() != 1 || fired != 1 {
		t.Errorf("picking a new row: selected %d, fired %d", dd.Selected(), fired)
	}
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

// TestGoldenDropdown pins the face and its open list, the selection's
// row checked and highlighted, in both palettes: the list is the
// combobox's without the search row.
func TestGoldenDropdown(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			dd := NewDropdown(goldenFace(t), 14, []string{"Apple", "Banana", "Blueberry", "Grapes"}, 1)
			_, host := hostDropdown(t, dd)
			dd.Open()
			col := NewBox(Column, 12, 0)
			col.Append(dd, false).Append(host.popovers[0], false)
			NewGolden(t, col, "dropdown-"+tc.name, goldenTheme(tc.th), goldenFrame(220, 240))
		})
	}
}

// A dropdown in a row short of room gives up width down to its floor:
// an Item's actions narrow it to the space left, its selection
// ellipsizes before the chevron, which shows whole, and a short label
// keeps its natural width (#321).
func TestDropdownShrinksAndEllipsizes(t *testing.T) {
	face := testFace(t)
	long := NewDropdown(face, 14, []string{"Creative Live! Cam Sync 4K Analog Stereo"}, 0)
	it := NewItem(face, "Source", "")
	it.AddAction(long)
	h := it.Measure(Constraints{Max: Size{W: 220, H: 200}}).H
	it.Arrange(render.Rect{W: 220, H: h})
	content := it.row.Bounds()
	db := long.Bounds()
	solo := NewDropdown(face, 14, []string{"Creative Live! Cam Sync 4K Analog Stereo"}, 0)
	natural := solo.Measure(Constraints{Max: Size{W: 400, H: 100}}).W
	if db.X+db.W > content.X+content.W || db.W >= natural {
		t.Fatalf("dropdown %v ends past the item's content %v (natural %d)", db, content, natural)
	}
	solo.Arrange(render.Rect{W: 140, H: 36})
	data := paintTree(solo, 140, 36)
	bg := render.ColorFromBytes(data[18*render.Stride(140)+(140-dropdownChevronW+2)*4:])
	for y := 8; y < 28; y++ {
		for x := 140 - dropdownChevronW; x < 140-22; x++ {
			if c := render.ColorFromBytes(data[y*render.Stride(140)+x*4:]); c != bg {
				t.Fatalf("(%d, %d) = %#08x before the chevron; the label ran into its reserve", x, y, uint32(c))
			}
		}
	}
	short := NewDropdown(face, 14, []string{"Default"}, 0)
	row := NewBox(Row, 0, 0)
	row.Append(short, false)
	nat := short.Measure(Constraints{Max: Size{W: 400, H: 100}}).W
	row.Measure(Constraints{Max: Size{W: 326, H: 100}})
	row.Arrange(render.Rect{W: 326, H: 36})
	if got := short.Bounds().W; got != nat {
		t.Errorf("short dropdown %dpx wide, want its natural %d", got, nat)
	}
}
