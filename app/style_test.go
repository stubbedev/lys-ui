package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/widget"
)

// The app-level half of the element-name pin (docs/css.md): dialog,
// popover, and tooltip name their cards through nameSurfaceElement;
// toast is a widget type and pins itself in the widget package.
func TestCSSSurfaceElementNames(t *testing.T) {
	// The dialog card carries the name on both shadow branches, whose
	// card is a different widget.
	for _, tc := range []struct {
		name string
		blur int
	}{
		{"shadowed", 16},
		{"flat", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prev := widget.Current()
			widget.SetTheme(widget.DarkTheme().WithShadowBlur(tc.blur))
			t.Cleanup(func() { widget.SetTheme(prev) })

			content := widget.NewBox(widget.Column, 12, 12)
			root, _ := dialogCard(content)
			var named string
			if tc.blur > 0 {
				margin, ok := root.(*widget.Box)
				if !ok || len(margin.Children()) != 1 {
					t.Fatalf("shadowed dialog root = %T", root)
				}
				if e, ok := margin.Children()[0].(interface{ Element() string }); ok {
					named = e.Element()
				}
			} else {
				named = content.Element()
			}
			if named != elemDialog {
				t.Fatalf("dialog card element = %q, want %q", named, elemDialog)
			}
		})
	}

	// A popover names whatever card tree it is given.
	box := widget.NewBox(widget.Row, 0, 0)
	nameSurfaceElement(box, elemPopover)
	if box.Element() != elemPopover {
		t.Errorf("popover card element = %q", box.Element())
	}

	// A non-node-backed widget is skipped silently.
	nameSurfaceElement(unnamed{}, elemPopover)
}

type unnamed struct{}

func (unnamed) Measure(widget.Axis, int) widget.Sizes { return widget.Sizes{} }
func (unnamed) Arrange(render.Rect)                   {}
func (unnamed) Paint(*render.Canvas)                  {}
func (unnamed) HitTest(widget.Point) widget.Widget    { return nil }

// A stylesheet file reloads on the watch an Application lends the widget
// package: nothing ticks, so an unchanged file costs no timer and no stat,
// and a file an editor renames over still lands.
func TestStylesheetHotReloadWatchesAndNeverPolls(t *testing.T) {
	a := loopApp(t)
	onLoop := func(fn func()) {
		t.Helper()
		ran := make(chan struct{})
		a.Invoke(func() {
			defer close(ran)
			fn()
		})
		<-ran
	}
	widget.SetStylesheetWatcher(func(dirs []string, changed func()) (func(), error) {
		return a.WatchFiles(dirs, false, changed)
	})
	widget.SetInvoker(a.Invoke)
	t.Cleanup(func() {
		widget.SetInvoker(nil)
		widget.SetStylesheetWatcher(nil)
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "app.css")
	if err := os.WriteFile(path, []byte(`button { border-radius: 4; }`), 0o600); err != nil {
		t.Fatal(err)
	}
	var sheet *widget.Stylesheet
	onLoop(func() {
		var err error
		if sheet, err = widget.AddStylesheetFile(path, widget.StylePriorityApplication); err != nil {
			t.Error(err)
		}
	})
	if sheet == nil {
		t.FailNow()
	}
	t.Cleanup(func() { onLoop(sheet.Remove) })
	if n := a.queues.timerCount(); n != 0 {
		t.Errorf("%d timers run with a stylesheet file loaded, want none", n)
	}
	replacement := filepath.Join(dir, "next.tmp")
	if err := os.WriteFile(replacement, []byte(`button { border-radius: 9; }`), 0o600); err != nil {
		t.Fatal(err)
	}
	before := widget.DamageGeneration()
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the reload", func() bool {
		widget.WaitAssetLoads()
		return widget.DamageGeneration() != before
	})
}
