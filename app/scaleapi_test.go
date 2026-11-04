package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/widget"
)

const scaleTestSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="#00ff00"/></svg>`

func TestAWindowsIconsDecodeAtItsScaleBeforeItsFirstFrame(t *testing.T) {
	a := idleBudgetApp(t)
	t.Cleanup(func() { widget.SetInvoker(nil) })
	w := a.windows[0]
	host := &Window{app: a}
	w.win = host
	path := filepath.Join(t.TempDir(), "flag.svg")
	if err := os.WriteFile(path, []byte(scaleTestSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	w.router.Root.(*widget.Box).Append(widget.NewThemeIcon(path, 16), false)
	var heard []int
	cancel := host.OnScaleChanged(func(num, denom int) { heard = append(heard, num*120/denom) })
	for _, frac := range []int{150, 180} {
		w.rescale(uint32(frac))
		if num, denom := host.Scale(); num*120/denom != frac {
			t.Errorf("Scale = %d/%d, want %d/120", num, denom, frac)
		}
		widget.WaitAssetLoads()
		if !widget.ThemeIconPreloaded(path, 16, frac, 120) {
			t.Errorf("at %d/120 the icon was not decoded before the frame", frac)
		}
		a.pump(time.Now())
	}
	if len(heard) != 2 || heard[0] != 150 || heard[1] != 180 {
		t.Errorf("OnScaleChanged heard %v, want [150 180]", heard)
	}
	cancel()
	w.rescale(240)
	if len(heard) != 2 {
		t.Errorf("a cancelled listener heard %v", heard)
	}
}

func TestAScaleTheCompositorConfirmsUnchangedStillReachesTheTree(t *testing.T) {
	a := idleBudgetApp(t)
	t.Cleanup(func() { widget.SetInvoker(nil) })
	w := a.windows[0]
	root := w.router.Root
	if num, _ := widget.DeviceScaleOf(root); num != 0 {
		t.Fatalf("a guessed scale reached the tree: %d", num)
	}
	w.rescale(w.frac120)
	if num, _ := widget.DeviceScaleOf(root); num != int(w.frac120) {
		t.Errorf("tree scale %d, want the confirmed %d", num, w.frac120)
	}
}
