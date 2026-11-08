package widget

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stubbedev/lys-lib/icons"
	"github.com/stubbedev/lys-lib/render"
)

var fractionalScales = []struct {
	name       string
	num, denom int
}{{"1.25", 5, 4}, {"1.5", 3, 2}}

func countDecodesOnLoop(t *testing.T) (onLoop, offLoop *atomic.Int32) {
	t.Helper()
	loop := goroutineID()
	onLoop, offLoop = new(atomic.Int32), new(atomic.Int32)
	decode := decodeIcon
	decodeIcon = func(data []byte, name string, size int, frac uint32) (*render.Icon, error) {
		if goroutineID() == loop {
			onLoop.Add(1)
		} else {
			offLoop.Add(1)
		}
		return decode(data, name, size, frac)
	}
	t.Cleanup(func() { decodeIcon = decode })
	return onLoop, offLoop
}

func TestOwnSourceIconsNeverDecodeOnTheLoopAtFractionalScales(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flag.svg")
	if err := os.WriteFile(path, []byte(testGreenSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sc := range fractionalScales {
		for kind, make := range map[string]func() *Icon{
			"file": func() *Icon { return NewFileIcon(path, 16) },
			"svg":  func() *Icon { return NewSVGIcon([]byte(testGreenSVG), 16) },
		} {
			t.Run(sc.name+"/"+kind, func(t *testing.T) {
				f := newFakeInvoker(t)
				onLoop, offLoop := countDecodesOnLoop(t)
				w := make()
				paintIcon(w, 16, sc.num, sc.denom)
				settleAssets(t, f)
				data, dev := paintIcon(w, 16, sc.num, sc.denom)
				if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
					t.Errorf("settled center = %v, want green", got)
				}
				if n := onLoop.Load(); n != 0 {
					t.Errorf("%d decodes on the loop", n)
				}
				if n := offLoop.Load(); n != 1 {
					t.Errorf("%d decodes off the loop, want 1", n)
				}
			})
		}
	}
}

func TestThemeIconNeverDecodesOnTheLoopAtFractionalScales(t *testing.T) {
	for _, sc := range fractionalScales {
		t.Run(sc.name, func(t *testing.T) {
			f := newFakeInvoker(t)
			root := t.TempDir()
			theme := filepath.Join(root, "wfrac")
			if err := os.MkdirAll(filepath.Join(theme, "16x16/apps"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(theme, "16x16/apps/flag.svg"), []byte(testGreenSVG), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(theme, "index.theme"), []byte("[Icon Theme]\nName=W\nDirectories=16x16/apps\n\n[16x16/apps]\nSize=16\nType=Fixed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			def := icons.Default()
			def.SetSearchPaths([]string{root})
			def.SetTheme("wfrac")
			t.Cleanup(func() {
				def.SetSearchPaths(nil)
				def.SetTheme("hicolor")
			})
			w := NewThemeIcon("flag", 16)
			if data, dev := paintIcon(w, 16, sc.num, sc.denom); centerPx(data, dev) != render.RGB(0, 0, 0) {
				t.Error("the first paint decoded inline")
			}
			settleAssets(t, f)
			data, dev := paintIcon(w, 16, sc.num, sc.denom)
			if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
				t.Errorf("settled center = %v, want green", got)
			}
		})
	}
}

func TestThemeIconShowsTheNearestScaleWhileItsOwnDecodes(t *testing.T) {
	f := newFakeInvoker(t)
	iconTree(t)
	PreloadThemeIcon("flag", 16, 1, 1)
	w := NewThemeIcon("flag", 16)
	data, dev := paintIcon(w, 16, 3, 2)
	if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
		t.Errorf("first paint center = %v, want the 1x raster's green, not a blank frame", got)
	}
	if ThemeIconPreloaded("flag", 16, 3, 2) {
		t.Fatal("the 1.5x raster decoded on the paint")
	}
	settleAssets(t, f)
	if !ThemeIconPreloaded("flag", 16, 3, 2) {
		t.Fatal("the 1.5x raster never decoded")
	}
	paintIcon(w, 16, 3, 2)
	if w.resolvedFrac != fracOf(3, 2) {
		t.Errorf("resolved at %d, want the exact %d", w.resolvedFrac, fracOf(3, 2))
	}
}

func TestPreloadPatternPaintsFirstTimeAtFractionalScales(t *testing.T) {
	newFakeInvoker(t)
	iconTree(t)
	for _, sc := range fractionalScales {
		for _, name := range []string{"flag", "flag-extra-name"} {
			PreloadThemeIcon(name, 16, sc.num, sc.denom)
			if !ThemeIconPreloaded(name, 16, sc.num, sc.denom) {
				t.Fatalf("%s at %s: preload did not take", name, sc.name)
			}
			w := NewThemeIconFallback(name, 16)
			data, dev := paintIcon(w, 16, sc.num, sc.denom)
			if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
				t.Errorf("%s at %s: first paint center = %v, want green", name, sc.name, got)
			}
			if w.resolvedFrac != fracOf(sc.num, sc.denom) {
				t.Errorf("%s at %s: resolved at %d, want the exact scale", name, sc.name, w.resolvedFrac)
			}
			assets.mu.Lock()
			queued := len(assets.jobs)
			assets.mu.Unlock()
			if queued != 0 {
				t.Errorf("%s at %s: the paint queued %d decodes", name, sc.name, queued)
			}
		}
	}
}
