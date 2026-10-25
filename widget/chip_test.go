package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

func chipTexts(b *ChipBox) []string {
	var out []string
	for _, c := range b.Chips() {
		out = append(out, c.Text())
	}
	return out
}

func newChipBox(t *testing.T, texts ...string) *ChipBox {
	t.Helper()
	b := NewChipBox()
	for _, s := range texts {
		b.Append(NewChip(goldenFace(t), s))
	}
	return b
}

// dragChip drags c and drops it on target at p, as the router would.
func dragChip(t *testing.T, c *Chip, target *ChipBox, p Point) {
	t.Helper()
	dc := c.DragContent()
	if dc == nil {
		t.Fatal("a chip in a box offered no drag")
	}
	data, err := dc.Bytes(chipMime)
	if err != nil {
		t.Fatal(err)
	}
	if target.DragEnter(dc.Mimes, p) != chipMime {
		t.Fatal("the box refused its own kind of chip")
	}
	target.Drop(chipMime, data, p)
	dc.OnDone(0)
}

// The dismiss button shows while OnDismiss is set and runs it; presses
// elsewhere on the chip are the chip's own, its drag handle.
func TestChipDismiss(t *testing.T) {
	c := NewChip(goldenFace(t), "clock")
	dismissed := 0
	c.SetOnDismiss(func() { dismissed++ })
	arrangeTree(t, c, 200, 40)
	if !c.HasClass("dismissible") || parentOf(c.dismiss) == nil {
		t.Fatal("a dismissible chip shows no button")
	}
	db, lb := c.dismiss.Bounds(), c.label.Bounds()
	if hit := c.HitTest(Point{X: db.X + db.W/2, Y: db.Y + db.H/2}); hit != Widget(c.dismiss) {
		t.Errorf("the button's press went to %T", hit)
	}
	if hit := c.HitTest(Point{X: lb.X + 2, Y: lb.Y + lb.H/2}); hit != Widget(c) {
		t.Errorf("a press on the text went to %T, want the chip", hit)
	}
	c.dismiss.OnClick()
	if dismissed != 1 {
		t.Errorf("dismissed %d times", dismissed)
	}
	if Describe(c.dismiss).Name != "Remove clock" {
		t.Errorf("dismiss button named %q", Describe(c.dismiss).Name)
	}
	c.SetOnDismiss(nil)
	if c.HasClass("dismissible") || len(c.row.Children()) != 1 {
		t.Error("clearing OnDismiss kept the button")
	}
	if NewChip(goldenFace(t), "loose").DragContent() != nil {
		t.Error("a chip in no box offered a drag")
	}
}

// A chip dropped within its box moves to the indicated place, and one
// dropped on another box moves across; OnMoved hears both.
func TestChipBoxReorders(t *testing.T) {
	a := newChipBox(t, "one", "two", "three")
	other := newChipBox(t, "four")
	col := NewBox(Column, 8, 0)
	col.Append(a, false)
	col.Append(other, false)
	arrangeTree(t, col, 400, 200)
	var moves [][3]int
	a.OnMoved = func(src *ChipBox, from, to int) {
		if src != a {
			t.Error("a reorder reported another source")
		}
		moves = append(moves, [3]int{0, from, to})
	}
	last := a.ChipAt(2).Bounds()
	dragChip(t, a.ChipAt(0), a, Point{X: last.X + last.W - 2, Y: last.Y + last.H/2})
	if got := chipTexts(a); len(got) != 3 || got[0] != "two" || got[2] != "one" {
		t.Fatalf("after the reorder %v, want one last", got)
	}
	if a.dropAt != -1 {
		t.Error("the indicator outlived the drop")
	}
	arrangeTree(t, col, 400, 200)
	var crossed []int
	other.OnMoved = func(src *ChipBox, from, to int) {
		if src != a {
			t.Error("a move across reported the wrong source")
		}
		crossed = []int{from, to}
	}
	first := other.ChipAt(0).Bounds()
	dragChip(t, a.ChipAt(0), other, Point{X: first.X + 1, Y: first.Y + first.H/2})
	if got := chipTexts(other); len(got) != 2 || got[0] != "two" || a.Len() != 2 || a.ChipAt(0).box != a || other.ChipAt(0).box != other {
		t.Errorf("across: %v in the target, %v left", got, chipTexts(a))
	}
	if len(moves) != 1 || moves[0] != [3]int{0, 0, 2} || len(crossed) != 2 || crossed[0] != 0 || crossed[1] != 0 {
		t.Errorf("moves %v, crossed %v", moves, crossed)
	}
}

// A drop whose payload is not the chip in flight moves nothing.
func TestChipBoxIgnoresForeignPayloads(t *testing.T) {
	b := newChipBox(t, "one", "two")
	arrangeTree(t, b, 400, 100)
	dc := b.ChipAt(0).DragContent()
	b.Drop(chipMime, []byte("not-the-token"), Point{X: 390, Y: 10})
	dc.OnDone(0)
	if got := chipTexts(b); got[0] != "one" {
		t.Errorf("a foreign payload reordered the box to %v", got)
	}
	if b.DragEnter([]string{chipMime}, Point{}) != "" {
		t.Error("the box took a chip drag with no chip in flight")
	}
}

// TestGoldenChips pins chips with a leading swatch and a dismiss
// button, and a hovering drop indicator, in both palettes.
func TestGoldenChips(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			b := newChipBox(t, "workspaces", "clock", "battery", "network")
			b.ChipAt(0).SetLeading(NewColorSwatch(render.RGB(0x3b, 0x82, 0xf6)))
			for _, c := range b.Chips()[1:] {
				c.SetOnDismiss(func() {})
			}
			b.dropAt = 2
			frame := NewBox(Column, 0, 12)
			frame.Append(b, false)
			NewGolden(t, frame, "chips-"+tc.name, goldenTheme(tc.th), goldenFrame(320, 60))
		})
	}
}

// A ChipBox is a CSS box: its padding insets the chips and is part of
// the drop area; a chip nested in a chip stands out from it.
func TestChipBoxIsAStyledBox(t *testing.T) {
	loadCSS(t, `chipbox { padding: 10px; border: 1px dashed @borders; }`)
	b := newChipBox(t, "one", "two")
	group := NewChip(goldenFace(t), "group")
	inner := NewChip(goldenFace(t), "inner")
	group.SetContent(inner)
	b.Append(group)
	arrangeTree(t, b, 400, 100)
	if got := b.ChipAt(0).Bounds(); got.X != 11 || got.Y != 11 {
		t.Errorf("first chip at %v, want inset by the border and padding", got)
	}
	if b.HitTest(Point{X: 3, Y: 3}) != Widget(b) {
		t.Error("the padding is not the box's own, so no drop lands there")
	}
	if inner.style(inner).Background == group.style(group).Background {
		t.Error("a nested chip has its group's background")
	}
}
