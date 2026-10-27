package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// A click opens the swatch picker under the button with the current
// color ringed; a swatch picks, fires OnColorSet and closes; the
// expanded chooser's edits fire it as they happen.
func TestColorButtonOpensAPickerAndChooses(t *testing.T) {
	b := NewColorButton(goldenFace(t), 14, render.RGB(1, 2, 3))
	b.Presets = []render.Color{render.RGB(1, 2, 3), render.RGB(4, 5, 6)}
	host := &fakeSurfaceHost{}
	SetSurfaceHost(b, host)
	arrangeTree(t, b, 100, 100)
	var set []render.Color
	b.OnColorSet = func(c render.Color) { set = append(set, c) }
	b.OnClick()
	if len(host.popovers) != 1 || host.opts[0].Gap != PopoverGap {
		t.Fatalf("%d popovers with %+v, want one picker at the popover gap", len(host.popovers), host.opts)
	}
	p := host.popovers[0].(*Box).Children()[0].(*SwatchPicker)
	arrangeTree(t, p, 400, 400)
	swatches := swatchesOf(p)
	if len(swatches) != 2 || !swatches[0].Selected() || swatches[1].Selected() {
		t.Fatalf("swatches %v, want two with the current one ringed", swatches)
	}
	swatchButton(swatches[1]).OnClick()
	if b.Color() != render.RGB(4, 5, 6) || len(set) != 1 || host.closed != 1 {
		t.Errorf("color %v, %d sets, %d closes; want the pick shown, reported and closed", b.Color(), len(set), host.closed)
	}
	b.SetColor(render.RGB(9, 9, 9))
	if len(set) != 1 {
		t.Error("SetColor fired OnColorSet")
	}
	binding := NewBinding(render.RGB(7, 7, 7))
	b.BindColor(binding)
	b.Choose(render.RGB(8, 8, 8))
	if binding.Get() != render.RGB(8, 8, 8) || b.Color() != render.RGB(8, 8, 8) {
		t.Error("the binding did not follow the pick")
	}
}

// The Custom entry opens the full chooser in place, whose edits report
// as they happen and move the ring; the custom row lists its colors.
func TestSwatchPickerCustom(t *testing.T) {
	p := NewSwatchPicker(goldenFace(t), 14, DefaultSwatches(), render.RGB(0xef, 0x44, 0x44))
	p.SetCustom([]render.Color{render.RGB(1, 1, 1)})
	var edits []render.Color
	p.OnEdited = func(c render.Color) { edits = append(edits, c) }
	if n := len(swatchesOf(p)); n != len(DefaultSwatches())+1 {
		t.Fatalf("%d swatches, want the presets and one custom", n)
	}
	p.more.OnClick()
	if !p.Expanded() {
		t.Fatal("Custom did not open the chooser")
	}
	p.chooser.SetColor(render.RGB(1, 1, 1))
	if len(edits) != 1 || p.Color() != render.RGB(1, 1, 1) {
		t.Fatalf("edits %v, color %v", edits, p.Color())
	}
	for _, sw := range swatchesOf(p) {
		if sw.Selected() != (sw.Color() == render.RGB(1, 1, 1)) {
			t.Errorf("swatch %v ringed %v after the edit", sw.Color(), sw.Selected())
		}
	}
	p.SetCustom(nil)
	if n := len(swatchesOf(p)); n != len(DefaultSwatches()) {
		t.Errorf("%d swatches after clearing the custom row", n)
	}
}

// A swatch takes its size from CSS.
func TestColorSwatchSizesFromCSS(t *testing.T) {
	sw := NewColorSwatch(render.RGB(1, 2, 3))
	loadCSS(t, `colorswatch { min-width: 40px; min-height: 12px; }`)
	if got := SizeWithin(sw, Size{W: 100, H: 100}); got != (Size{W: 40, H: 12}) {
		t.Errorf("swatch measures %v, want 40x12", got)
	}
}

// TestGoldenColorSwatches pins the picker: presets with one ringed, a
// translucent custom color over the checkerboard, in both palettes.
func TestGoldenColorSwatches(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			p := NewSwatchPicker(goldenFace(t), 14, DefaultSwatches(), render.RGB(0x3b, 0x82, 0xf6))
			p.SetCustom([]render.Color{render.RGBA(0x80, 0x10, 0x10, 0x80)})
			frame := NewBox(Column, 0, 12)
			frame.AppendAligned(p, false, AlignStart)
			NewGolden(t, frame, "swatchpicker-"+tc.name, goldenTheme(tc.th), goldenFrame(300, 200))
		})
	}
}

// swatchesOf is every swatch under w, in order.
func swatchesOf(w Widget) []*ColorSwatch {
	var out []*ColorSwatch
	walkWidgets(w, func(x Widget) bool {
		if sw, ok := x.(*ColorSwatch); ok {
			out = append(out, sw)
		}
		return true
	})
	return out
}

// swatchButton is the button a picker wraps sw in.
func swatchButton(sw *ColorSwatch) *Button { return parentOf(sw).(*Button) }

func TestFontButtonShowsAndChooses(t *testing.T) {
	b := NewFontButton(testFace(t), 13, FontChoice{Family: "Inter", Size: 13})
	if b.label.Text() != "Inter 13" {
		t.Errorf("label %q", b.label.Text())
	}
	var opened FontChoice
	b.OnOpen = func(f FontChoice) { opened = f }
	b.OnClick()
	b.Choose(FontChoice{Family: "Mono", Size: 11.5})
	if opened.Family != "Inter" || b.Font().Family != "Mono" || b.label.Text() != "Mono 11.5" {
		t.Errorf("opened %+v, now %+v %q", opened, b.Font(), b.label.Text())
	}
}

func TestInscriptionSizesByCountsNotContent(t *testing.T) {
	in := NewInscription(testFace(t), 12, "a very long line of text that would be wide\nsecond\nthird", 0)
	in.SetNatChars(5)
	in.SetNatLines(2)
	short := NewInscription(testFace(t), 12, "x", 0)
	short.SetNatChars(5)
	short.SetNatLines(2)
	a := SizeWithin(in, Size{W: 1000, H: 1000})
	b := SizeWithin(short, Size{W: 1000, H: 1000})
	if a != b || a.W == 0 || a.H != 2*in.lineH {
		t.Errorf("long text measured %v, short %v: the size must come from the counts", a, b)
	}
	in.SetMinChars(8)
	if got := SizeWithin(in, Size{W: 1000, H: 1000}).W; got < minWidth(in) {
		t.Errorf("natural width %d below the minimum %d", got, minWidth(in))
	}
	in.Arrange(render.Rect{W: a.W, H: a.H})
	in.Paint(render.New(make([]uint8, a.W*a.H*4+4096), a.W*4, a.W, a.H))
}

// A ColorButton is a button to the stylesheet: an outline icon button
// as tall as its 36 px peers, not a bare swatch.
func TestColorButtonStylesAsAButton(t *testing.T) {
	b := NewColorButton(goldenFace(t), 14, render.RGB(1, 2, 3))
	row := NewBox(Row, 8, 0)
	row.AppendAligned(b, false, AlignStart)
	arrangeTree(t, row, 200, 80)
	if b.Element() != "button" || b.Bounds().W != 36 || b.Bounds().H != 36 {
		t.Errorf("element %q at %v, want a 36 px button", b.Element(), b.Bounds())
	}
}
