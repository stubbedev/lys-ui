package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/icons"
)

// TestGoldenBuiltinIcons pins the bundled Lucide set as stock chrome
// draws it on a host with no icon theme at all: every freedesktop name
// it serves, symbolic, in the theme's text color.
func TestGoldenBuiltinIcons(t *testing.T) {
	prev := IconSearchPaths()
	SetIconSearchPaths([]string{t.TempDir()})
	t.Cleanup(func() { SetIconSearchPaths(prev) })
	th := DarkTheme()
	grid := NewFlowBox(6, 6)
	grid.SetSelectionMode(SelectionNone)
	grid.SetMaxChildrenPerLine(12)
	for _, name := range icons.BuiltinNames() {
		grid.Append(NewThemeIcon(name+"-symbolic", 20))
	}
	NewGolden(t, grid, "icons-builtin", goldenTheme(th), goldenFrame(320, 160))
}
