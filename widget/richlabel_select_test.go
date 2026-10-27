package widget

import (
	"strings"
	"testing"
)

// A selectable rich label takes a drag selection: the press anchors,
// the drag extends, SelectedText reads the range, and a plain click
// drops it. Unselectable labels never select.
func TestRichLabelSelectable(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "selectable public key readout", 0)
	l.SetSelectable(true)
	if !l.Selectable() {
		t.Fatal("selectable did not stick")
	}
	sz := SizeWithin(l, Size{W: 400, H: 40})
	arrangeTree(t, l, sz.W, sz.H)

	l.PressAt(Point{X: l.Bounds().X + 2, Y: l.Bounds().Y + 2})
	l.DragMove(Point{X: l.Bounds().X + sz.W - 2, Y: l.Bounds().Y + 2})
	text, on := l.SelectedText()
	if !on || text == "" {
		t.Fatalf("a full-width drag selected nothing")
	}
	if strings.HasPrefix(text, " ") || strings.HasSuffix(text, " ") {
		t.Errorf("selection %q carries boundary spaces", text)
	}
	l.ClickAt(Point{X: l.Bounds().X + 2, Y: l.Bounds().Y + 2})
	if _, on := l.SelectedText(); on {
		t.Error("a plain click kept the selection")
	}

	plain := NewRichLabel(entryFace(t), 14, "not selectable", 0)
	sz2 := SizeWithin(plain, Size{W: 400, H: 40})
	arrangeTree(t, plain, sz2.W, sz2.H)
	plain.PressAt(Point{X: plain.Bounds().X + 2, Y: plain.Bounds().Y + 2})
	plain.DragMove(Point{X: plain.Bounds().X + sz2.W - 2, Y: plain.Bounds().Y + 2})
	if _, on := plain.SelectedText(); on {
		t.Error("an unselectable label selected")
	}
}

// The selection maps runes on wrapped rows too.
func TestRichLabelSelectableWrapped(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "alpha bravo charlie delta echo foxtrot golf hotel", 0)
	l.SetSelectable(true)
	l.SetWrap(true)
	sz := SizeWithin(l, Size{W: 110, H: 200})
	arrangeTree(t, l, 110, sz.H)
	if len(l.lines) < 2 {
		t.Fatalf("rows = %d, the test needs a wrap", len(l.lines))
	}
	l.PressAt(Point{X: l.Bounds().X + 1, Y: l.Bounds().Y + 2})
	l.DragMove(Point{X: l.Bounds().X + 1, Y: l.Bounds().Y + sz.H - 2})
	text, on := l.SelectedText()
	if !on || len(text) < 10 {
		t.Errorf("a across-rows drag selected %q", text)
	}
}
