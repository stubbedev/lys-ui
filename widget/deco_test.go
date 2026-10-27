package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// caret-color recolors the entry's caret over the text color; auto
// keeps the text color.
func TestCaretColor(t *testing.T) {
	loadCSS(t, `entry { caret-color: #112233; }`)
	e := NewEntry(testFace(t), 13, 0)
	arrangeTree(t, e, 100, 30)
	v := e.style(e)
	if !v.Has(style.PropCaretColor) || v.CaretColor != render.RGB(0x11, 0x22, 0x33) {
		t.Fatalf("caret color %v declared %v, want #112233", v.CaretColor, v.Has(style.PropCaretColor))
	}
	loadCSS(t, `entry { caret-color: auto; }`)
	_ = e.style(e)
	if e.style(e).Has(style.PropCaretColor) && e.style(e).CaretColor != 0 {
		t.Error("auto left a caret color")
	}
}

// text-decoration: underline strokes the run's underline; none draws
// clean text.
func TestLabelTextDecoration(t *testing.T) {
	face := testFace(t)
	paint := func(css string) []byte {
		loadCSS(t, css)
		l := NewLabel(face, 13, "link", 0)
		host := NewBox(Column, 0, 0)
		host.Append(l, false)
		data := make([]byte, render.Stride(80)*30)
		cv := render.NewScaled(data, render.Stride(80), 80, 30, 1, 1)
		cv.Clear(cv.Rect(), 0)
		SizeWithin(host, Size{W: 80, H: 30})
		host.Arrange(render.Rect{W: 80, H: 30})
		host.Paint(cv)
		return data
	}
	plain := paint(`label { color: #ffffff; }`)
	under := paint(`label { color: #ffffff; text-decoration: underline; }`)
	diffs := 0
	for i := range plain {
		if plain[i] != under[i] {
			diffs++
		}
	}
	if diffs == 0 {
		t.Fatal("text-decoration: underline drew the identical raster")
	}
	none := paint(`label { color: #ffffff; text-decoration: underline; text-decoration: none; }`)
	for i := range plain {
		if plain[i] != none[i] {
			t.Fatal("text-decoration: none did not restore the plain text")
		}
	}
}

// The check node's -gtk-icon-source replaces the painted tick with the
// themed icon, recolored by -gtk-icon-palette.
func TestCheckButtonThemedMark(t *testing.T) {
	iconTree(t)
	loadCSS(t, `checkbutton check {
		-gtk-icon-source: -gtk-icontheme("face-symbolic");
		-gtk-icon-palette: success #112233;
	}`)
	c := NewCheckButton(false)
	arrangeTree(t, c, 40, 40)
	kv := c.check.style(&c.check)
	if kv.IconSource != "face-symbolic" {
		t.Fatalf("icon source %q, want face-symbolic", kv.IconSource)
	}
	if kv.PaletteTint != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("palette tint %v, want #112233", kv.PaletteTint)
	}
	if c.mark != nil {
		t.Error("an unchecked check built its mark early")
	}
	c.SetChecked(true)
	data := make([]byte, render.Stride(40)*40)
	cv := render.NewScaled(data, render.Stride(40), 40, 40, 1, 1)
	c.Paint(cv)
	if c.mark == nil || c.markName != "face-symbolic" {
		t.Fatal("a checked check did not build its themed mark")
	}
	if c.markTint != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("the mark took tint %v, want the palette's", c.markTint)
	}
	if b := c.mark.Bounds(); b.Empty() || !c.indicator.Contains(b.X+b.W/2, b.Y+b.H/2) {
		t.Errorf("the mark sits at %v, want a size centered in the indicator %v", b, c.indicator)
	}
}
