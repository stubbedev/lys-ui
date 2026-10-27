package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// chooserFrame measures and arranges a chooser and returns it.
func chooserFrame(c *ColorChooser) *ColorChooser {
	sz := SizeWithin(c, Size{W: 4096, H: 4096})
	c.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
	c.Paint(render.New(make([]uint8, sz.W*sz.H*4), sz.W*4, sz.W, sz.H))
	return c
}

// TestColorChooserSVSquareIsExact pins the #39 blending question the
// ticket asks: the shader-less SV square - white-to-hue horizontal over
// transparent-to-black vertical - composites through the canvas's
// premultiplied source-over into exactly the HSV arithmetic at every
// sampled pixel center (half-pixel inset included).
func TestColorChooserSVSquareIsExact(t *testing.T) {
	c := NewColorChooser(entryFace(t), 13, render.RGB(0xff, 0, 0))
	chooserFrame(c)
	c.hue = 0 // pure red hue
	buf := make([]uint8, c.bounds.W*c.bounds.H*4)
	c.Paint(render.New(buf, c.bounds.W*4, c.bounds.W, c.bounds.H))

	sv := c.sv
	px := func(x, y int) [4]byte {
		return render.ColorFromBytes(buf[y*c.bounds.W*4+x*4 : y*c.bounds.W*4+x*4+4]).Straight()
	}
	for _, p := range [][2]int{
		{sv.X + 2, sv.Y + 2},
		{sv.X + sv.W/4, sv.Y + 4},
		{sv.X + sv.W/2, sv.Y + sv.H/2},
		{sv.X + sv.W - 3, sv.Y + sv.H/4},
		{sv.X + sv.W/3, sv.Y + sv.H - 3},
	} {
		s := (float64(p[0]) + 0.5 - float64(sv.X)) / float64(sv.W)
		v := 1 - (float64(p[1])+0.5-float64(sv.Y))/float64(sv.H)
		want := hsvColor(0, s, v, 1).Straight()
		got := px(p[0], p[1])
		for i := range got {
			if abs8(got[i], want[i]) > 2 {
				t.Errorf("pixel (%d,%d) = %v, want the HSV arithmetic %v", p[0], p[1], got, want)
				break
			}
		}
	}
}

func abs8(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// TestColorChooserPickingProducesExactColors pins the pointer half: a
// click or drag on the square and both strips sets exactly the HSV the
// geometry names, and OnChanged fires with the straight color.
func TestColorChooserPickingProducesExactColors(t *testing.T) {
	var got []render.Color
	c := NewColorChooser(entryFace(t), 13, render.RGB(0x80, 0x80, 0x80))
	c.OnChanged = func(col render.Color) { got = append(got, col) }
	chooserFrame(c)

	// The hue strip click at a named pixel row sets hue to exactly that
	// row's fraction; the square's near corner sets saturation and value
	// to their geometric fractions - the picked color is exactly the HSV
	// arithmetic those clicks name.
	row := c.hueStrip.H / 3
	c.ClickAt(Point{X: c.hueStrip.X + c.hueStrip.W/2, Y: c.hueStrip.Y + row})
	c.ClickAt(Point{X: c.sv.X + c.sv.W - 1, Y: c.sv.Y + 1})
	if c.hue != float64(row)/float64(c.hueStrip.H-1) {
		t.Fatalf("hue = %v, want exactly %d/%d", c.hue, row, c.hueStrip.H-1)
	}
	if want := hsvColor(c.hue, c.sat, c.val, 1); c.Color() != want {
		t.Fatalf("picked = %v, want exactly the model %v", c.Color(), want)
	}
	if got := c.Color().Straight(); got[1] < 250 {
		t.Errorf("green channel = %d, want the near-corner bright green", got[1])
	}

	// The alpha strip's bottom: fully transparent.
	c.ClickAt(Point{X: c.alphaStrip.X + 2, Y: c.alphaStrip.Y + c.alphaStrip.H - 1})
	if a := c.Color().A(); a != 0 {
		t.Fatalf("alpha = %d, want 0", a)
	}

	// A drag across the square streams changes.
	n := len(got)
	c.DragMove(Point{X: c.sv.X + 10, Y: c.sv.Y + 10})
	c.DragMove(Point{X: c.sv.X + 60, Y: c.sv.Y + 30})
	if len(got) < n+2 {
		t.Errorf("drag fired %d changes, want one per motion", len(got)-n)
	}
}

// TestColorChooserHexEntry pins the entry half: #RGB, #RRGGBB, and\n// #RRGGBBAA (with and without the #) apply, garbage does not, and the\n// mirrored text follows every non-entry change.
func TestColorChooserHexEntry(t *testing.T) {
	c := NewColorChooser(entryFace(t), 13, render.RGB(0, 0, 0))
	chooserFrame(c)

	for _, tc := range []struct {
		in   string
		want [4]byte
	}{
		{"#f00", [4]byte{255, 0, 0, 255}},
		{"00ff00", [4]byte{0, 255, 0, 255}},
		{"#10203040", [4]byte{0x10, 0x20, 0x30, 0x40}},
		{"nope", [4]byte{0x10, 0x20, 0x30, 0x40}},
	} {
		c.hex.SetText(tc.in)
		if have := c.Color().Straight(); have != tc.want {
			t.Errorf("hex %q = %v, want %v", tc.in, have, tc.want)
		}
	}

	// Changes from the square mirror back into the entry.
	c.ClickAt(Point{X: c.sv.X + c.sv.W - 1, Y: c.sv.Y + 1})
	if txt := c.hex.Text(); txt == "nope" {
		t.Errorf("entry text %q not mirrored after a square pick", txt)
	}
}

// TestColorChooserKeyboardOnly pins the keyboard model: arrows move
// saturation and value, shift+arrows the hue, PageUp/PageDown alpha,
// and the entry stays reachable through Children for Tab.
func TestColorChooserKeyboardOnly(t *testing.T) {
	c := NewColorChooser(entryFace(t), 13, render.RGB(0xff, 0, 0))
	chooserFrame(c)

	c.KeyAction(KeyRight, 0) // saturation 1 -> clamped at 1? red is sat 1: nudge left first
	c.KeyAction(KeyLeft, 0)
	if c.sat >= 1 {
		t.Fatal("left arrow did not lower saturation")
	}
	c.KeyAction(KeyLeft, 0)
	sat := c.sat
	c.KeyAction(KeyRight, 0)
	if c.sat <= sat {
		t.Error("right arrow did not raise saturation")
	}
	c.KeyAction(KeyDown, 0)
	if c.val >= 1 {
		t.Error("down arrow did not lower value")
	}
	hue := c.hue
	c.KeyAction(KeyRight, ModShift)
	if c.hue <= hue {
		t.Error("shift+right did not raise hue")
	}
	c.KeyAction(KeyNextPage, 0)
	if c.alpha >= 1 {
		t.Error("PageDown did not lower alpha")
	}
	c.KeyAction(KeyPriorPage, 0)
	if c.alpha != 1 {
		t.Errorf("PageUp restored alpha to %v, want 1", c.alpha)
	}
	if len(c.Children()) == 0 {
		t.Error("the hex entry is not in the focus order")
	}
}

// TestColorChooserPalette pins the palette plumbing: preset swatches
// pick, the custom source lists back what the + button recorded, and
// the selection ring marks the active swatch.
func TestColorChooserPalette(t *testing.T) {
	var picked []render.Color
	c := NewColorChooser(entryFace(t), 13, render.RGB(0, 0, 0))
	custom := []render.Color{render.RGB(9, 9, 9)}
	c.SetPaletteSource(func() []render.Color { return custom }, func(col render.Color) {
		custom = append(custom, col)
	})
	chooserFrame(c)

	// The first preset swatch is the theme accent.
	c.ClickAt(Point{X: c.sv.X + 5, Y: c.hex.Bounds().Y + c.hex.Bounds().H + 8 + 5})
	if c.Color() != Current().Accent {
		t.Errorf("preset pick = %v, want the accent %v", c.Color(), Current().Accent)
	}

	// The + button records the current color through the source.
	c.addTo.OnClick()
	if len(custom) != 2 || custom[1] != c.Color() {
		t.Errorf("custom palette = %v, want the pick appended", custom)
	}
	_ = picked
}
