package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// The entry's trailing icon is the entry > image node: the cascade
// styles it (color and -gtk-icon-size), the text area gives up its
// width, and a press over it fires the handler instead of placing the
// caret.
func TestEntryTrailingIcon(t *testing.T) {
	loadCSS(t, `entry.network-password-input > image { color: #00ff00; -gtk-icon-size: 12px; }`)
	face := goldenFace(t)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	e.AddClass("network-password-input")
	e.SetText("secret")
	clicks := 0
	e.SetTrailingIcon("ld-eye-symbolic", 14, func() { clicks++ })
	ic := e.TrailingIcon()
	if ic == nil {
		t.Fatal("the trailing icon is unset")
	}
	root := NewBox(Column, 0, 0)
	root.Append(e, false)
	arrangeTree(t, root, 200, 40)

	// The node is entry > image, and the cascade styles it.
	if got := CascadeColor(ic); got != render.RGB(0, 255, 0) {
		t.Errorf("trailing icon color = %v, want the rule's #00ff00", got)
	}
	if sz := SizeWithin(ic, Size{W: 200, H: 40}); sz.W != 12 || sz.H != 12 {
		t.Errorf("trailing icon size %v, want the -gtk-icon-size 12", sz)
	}
	// The text area ends before the icon: typing past it pans instead
	// of running under the glyph.
	in := e.textInsets()
	if in.Right <= entryTrailingGap+12 {
		t.Fatalf("text insets right %d, want the icon width reserved", in.Right)
	}
	// A press over the icon toggles; a press over the text does not.
	r := e.trailingRect()
	if r.Empty() {
		t.Fatal("the trailing icon has no rect")
	}
	e.ClickAt(Point{X: r.X + r.W/2, Y: r.Y + r.H/2})
	if clicks != 1 {
		t.Errorf("the icon press fired %d times, want 1", clicks)
	}
	e.ClickAt(Point{X: e.Bounds().X + 5, Y: e.Bounds().Y + 10})
	if clicks != 1 {
		t.Error("a press over the text reached the icon handler")
	}

	// Clearing removes the reserve.
	e.SetTrailingIcon("", 0, nil)
	if e.TrailingIcon() != nil {
		t.Fatal("clearing left the icon")
	}
	arrangeTree(t, root, 200, 40)
	if in2 := e.textInsets(); in2.Right >= in.Right {
		t.Errorf("cleared insets right %d, want the reserve gone (was %d)", in2.Right, in.Right)
	}
}
