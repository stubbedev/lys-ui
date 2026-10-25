package widget

import "testing"

// Widgets built under one palette paint their labels in the next one:
// a label's color comes from the cascade, not from the palette the
// constructor saw (#247).
func TestLabelsFollowAPaletteSwitch(t *testing.T) {
	prev := Current()
	t.Cleanup(func() { SetTheme(prev) })
	SetTheme(DarkTheme())
	face := testFace(t)
	bar := NewMenuBar(face, 14, MenuBarMenu{Label: "File"})
	title := NewWindowTitle(face, 14, "Title", "Subtitle")
	SetTheme(LightTheme())
	th := Current()
	root := bar.buttons[0].child.(*Label)
	if got := root.textColor(root.style(root)); got != th.Text {
		t.Errorf("menubar root %#x after the switch, want the light text %#x", uint32(got), uint32(th.Text))
	}
	if got := title.subtitle.textColor(title.subtitle.style(title.subtitle)); got != th.DisabledText() {
		t.Errorf("window subtitle %#x after the switch, want the light muted text %#x", uint32(got), uint32(th.DisabledText()))
	}
}
