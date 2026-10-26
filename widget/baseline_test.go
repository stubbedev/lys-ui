package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestBoxBaseline pins the row's shared baseline: a small label, a
// large label, an entry, and a button line their first baselines up,
// the row as tall as the group, and its own baseline the shared one.
func TestBoxBaseline(t *testing.T) {
	face := chromeFace(t)
	th := DarkTheme()
	small := NewLabel(face, 11, "small", th.Text)
	large := NewLabel(face, 24, "Large", th.Text)
	entry := NewEntry(face, 14, th.Text)
	entry.SetText("entry")
	button := NewButton(NewLabel(face, 14, "Go", th.Text), 6, 4)
	icon := NewSpacer(10, 40)
	row := NewBox(Row, 6, 0)
	for _, w := range []Widget{small, large, entry, button, icon} {
		row.AppendAligned(w, false, AlignBaseline)
	}
	sz := row.Measure(Constraints{Max: Size{W: 600, H: 200}})
	row.Arrange(render.Rect{W: sz.W, H: sz.H})
	line, ok := row.Baseline()
	if !ok {
		t.Fatal("the row has no baseline")
	}
	for name, w := range map[string]Widget{"small": small, "large": large, "entry": entry, "button": button} {
		base, _ := baselineOf(w)
		if got := w.(Boundser).Bounds().Y + base; got != line && name != "entry" {
			t.Errorf("%s baseline at %d, want the row's %d", name, got, line)
		}
	}
	// The entry's bounds are its field (the margin inside): its text
	// still lands on the line.
	if eb, _ := entry.Baseline(); entry.Bounds().Y-marginOf(entry.style(entry)).Top+eb != line {
		t.Errorf("entry baseline off the line")
	}
	if ib := icon.Bounds(); ib.Y != (sz.H-40)/2 {
		t.Errorf("a child without a baseline sits at y=%d, want centered", ib.Y)
	}
}

// TestGoldenBaseline pins mixed sizes on one baseline.
func TestGoldenBaseline(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	row := NewBox(Row, 8, 0)
	row.AppendAligned(NewLabel(face, 11, "caption", th.TextMuted), false, AlignBaseline)
	row.AppendAligned(NewLabel(face, 26, "Title", th.Text), false, AlignBaseline)
	row.AppendAligned(NewButton(NewLabel(face, 14, "Action", th.Text), 6, 4), false, AlignBaseline)
	NewGolden(t, row, "baseline", goldenTheme(th))
}
