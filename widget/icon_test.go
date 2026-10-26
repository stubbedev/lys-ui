package widget

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/icons"
)

const (
	testGreenSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <rect width="16" height="16" fill="#00ff00"/>
</svg>`

	testBlueSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <rect width="16" height="16" fill="#0000ff"/>
</svg>`

	testSymbolicSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <rect width="16" height="16" fill="currentColor"/>
</svg>`
)

// iconTree writes a two-theme fixture tree and points the shared
// icons.Default cache at it; the cleanup restores hicolor and the
// environment search paths.
func iconTree(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("wtest/index.theme", []byte(`[Icon Theme]
Name=W Test
Directories=16x16/apps,symbolic/apps

[16x16/apps]
Size=16
Type=Fixed

[symbolic/apps]
Size=16
`))
	write("wtest/16x16/apps/flag.svg", []byte(testGreenSVG))
	write("wtest/symbolic/apps/face-symbolic.svg", []byte(testSymbolicSVG))
	write("wblue/index.theme", []byte(`[Icon Theme]
Name=W Blue
Directories=16x16/apps

[16x16/apps]
Size=16
Type=Fixed
`))
	write("wblue/16x16/apps/flag.svg", []byte(testBlueSVG))

	def := icons.Default()
	def.SetSearchPaths([]string{root})
	def.SetTheme("wtest")
	t.Cleanup(func() {
		def.SetSearchPaths(nil)
		def.SetTheme("hicolor")
	})
}

// paintIcon measures, arranges, and paints w at the origin of a box x
// box logical canvas at the given device scale, over black. It returns
// the pixel data so tests can read back what painted.
func paintIcon(w *Icon, box, num, denom int) ([]byte, int) {
	dev := (box*num + denom - 1) / denom
	data := make([]byte, render.Stride(dev)*dev)
	cv := render.NewScaled(data, render.Stride(dev), dev, dev, num, denom)
	cv.Clear(cv.Rect(), render.RGB(0, 0, 0))
	w.Measure(Constraints{Max: Size{W: box, H: box}})
	w.Arrange(render.Rect{X: 0, Y: 0, W: box, H: box})
	w.Paint(cv)
	return data, dev
}

func centerPx(data []byte, dev int) render.Color {
	return render.ColorFromBytes(data[(dev/2)*render.Stride(dev)+(dev/2)*4:])
}

func pngDoc(t *testing.T, r, g, b uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.Set(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNewIconStatic(t *testing.T) {
	ic, err := render.LoadSVG([]byte(testGreenSVG), 16, 16)
	if err != nil {
		t.Fatal(err)
	}
	w := NewIcon(ic)

	t.Run("natural size is the raster size", func(t *testing.T) {
		sz := w.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if sz.W != 16 || sz.H != 16 {
			t.Errorf("Measure = %v, want 16x16", sz)
		}
	})

	t.Run("paints as is", func(t *testing.T) {
		data, dev := paintIcon(w, 16, 1, 1)
		if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
			t.Errorf("center = %v, want green", got)
		}
	})
}

func TestNewThemeIconRenders(t *testing.T) {
	iconTree(t)
	w := NewThemeIcon("flag", 16)
	data, dev := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
		t.Errorf("center = %v, want green (the themed flag)", got)
	}
	if err := w.Err(); err != nil {
		t.Errorf("Err = %v, want nil", err)
	}
}

func TestNewFileIconRenders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "red.png")
	if err := os.WriteFile(path, pngDoc(t, 0xff, 0, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	w := NewFileIcon(path, 16)
	data, dev := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, dev); got != render.RGB(0xff, 0, 0) {
		t.Errorf("center = %v, want red", got)
	}
}

func TestNewSVGIconRenders(t *testing.T) {
	w := NewSVGIcon([]byte(testBlueSVG), 16)
	data, dev := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, dev); got != render.RGB(0, 0, 0xff) {
		t.Errorf("center = %v, want blue", got)
	}
}

func TestThemeIconExists(t *testing.T) {
	iconTree(t)
	if !ThemeIconExists("flag") {
		t.Error("flag is in the fixture theme")
	}
	if !ThemeIconExists("face-symbolic") {
		t.Error("the scalable symbolic icon counts too")
	}
	if ThemeIconExists("no-such-icon") {
		t.Error("an unknown name must not exist")
	}
	if ThemeIconExists("") {
		t.Error("the empty name must not exist")
	}
}

func TestNewThemeIconMissing(t *testing.T) {
	iconTree(t)
	w := NewThemeIcon("no-such-icon", 16)
	data, dev := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, dev); got != render.RGB(0, 0, 0) {
		t.Errorf("center = %v, want untouched black", got)
	}
	if !errors.Is(w.Err(), icons.ErrNotFound) {
		t.Errorf("Err = %v, want ErrNotFound", w.Err())
	}
}

func TestIconSymbolicFollowsAccent(t *testing.T) {
	iconTree(t)
	defer SetTheme(DarkTheme())
	face := NewThemeIcon("face-symbolic", 16)

	SetTheme(DarkTheme())
	darkData, darkDev := paintIcon(face, 16, 1, 1)
	dark := centerPx(darkData, darkDev)
	if dark != DarkTheme().Accent {
		t.Errorf("center = %v, want the dark accent", dark)
	}

	SetTheme(LightTheme())
	lightData, lightDev := paintIcon(face, 16, 1, 1)
	light := centerPx(lightData, lightDev)
	if light != LightTheme().Accent {
		t.Errorf("center = %v, want the light accent", light)
	}
	if dark == light {
		t.Error("symbolic icon kept its color across the theme swap")
	}
}

func TestIconPinnedTint(t *testing.T) {
	iconTree(t)
	red := render.RGB(0xff, 0, 0)

	t.Run("themed source", func(t *testing.T) {
		w := NewThemeIcon("flag", 16)
		w.SetTint(red)
		data, dev := paintIcon(w, 16, 1, 1)
		if got := centerPx(data, dev); got != red {
			t.Errorf("center = %v, want the pinned tint", got)
		}
	})

	t.Run("static source recolors eagerly", func(t *testing.T) {
		ic, err := render.LoadSVG([]byte(testGreenSVG), 16, 16)
		if err != nil {
			t.Fatal(err)
		}
		w := NewIcon(ic)
		w.SetTint(red)
		data, dev := paintIcon(w, 16, 1, 1)
		if got := centerPx(data, dev); got != red {
			t.Errorf("center = %v, want the pinned tint", got)
		}
	})
}

func TestIconRescalesWithCanvas(t *testing.T) {
	iconTree(t)
	w := NewThemeIcon("flag", 16)

	paintIcon(w, 16, 1, 1)
	if w1, h1 := w.ic.Size(); w1 != 16 || h1 != 16 {
		t.Errorf("1x raster = %dx%d, want 16x16", w1, h1)
	}
	data, dev := paintIcon(w, 16, 240, 120)
	if w2, h2 := w.ic.Size(); w2 != 32 || h2 != 32 {
		t.Errorf("2x raster = %dx%d, want 32x32", w2, h2)
	}
	if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
		t.Errorf("center = %v, want green", got)
	}
}

func TestIconThemeSwitchRepaints(t *testing.T) {
	iconTree(t)
	w := NewThemeIcon("flag", 16)
	CollectDamage(w) // drain the initial theme stamp
	data, dev := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) {
		t.Fatalf("center = %v, want green", got)
	}
	if _, any := CollectDamage(w); any {
		t.Fatal("unexpected damage after painting")
	}

	// The cache-wide theme switch must surface as damage on the next
	// collect, even though no widget state changed.
	icons.Default().SetTheme("wblue")
	rects, any := CollectDamage(w)
	if !any {
		t.Fatal("theme switch produced no damage")
	}
	if len(rects) != 1 || rects[0] != w.bounds {
		t.Errorf("damage = %v, want the icon bounds %v", rects, w.bounds)
	}

	data, dev = paintIcon(w, 16, 1, 1)
	if got := centerPx(data, dev); got != render.RGB(0, 0, 0xff) {
		t.Errorf("center = %v, want blue (the wblue flag)", got)
	}
}

func TestIconMutationsInvalidate(t *testing.T) {
	iconTree(t)
	w := NewThemeIcon("flag", 16)
	paintIcon(w, 16, 1, 1)
	CollectDamage(w)

	w.SetTint(render.RGB(0xff, 0, 0))
	if rects, any := CollectDamage(w); !any {
		t.Error("SetTint produced no damage")
	} else if len(rects) != 1 || rects[0] != w.bounds {
		t.Errorf("damage = %v, want the icon bounds %v", rects, w.bounds)
	}
}

func TestIconMeasureWantsLogicalSize(t *testing.T) {
	iconTree(t)
	for _, tc := range []struct {
		w    *Icon
		want Size
	}{
		{NewThemeIcon("flag", 16), Size{W: 16, H: 16}},
		{NewSVGIcon([]byte(testBlueSVG), 24), Size{W: 24, H: 24}},
	} {
		sz := tc.w.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if sz != tc.want {
			t.Errorf("Measure = %v, want %v", sz, tc.want)
		}
	}
}

// TestIconSearchPathsAndRefresh pins the public search-path API: a base
// directory set first wins over the rest, a file installed under it
// stays a miss until RefreshIcons, and a live icon showing it repaints.
func TestIconSearchPathsAndRefresh(t *testing.T) {
	iconTree(t)
	fallback := IconSearchPaths()
	extra := t.TempDir()
	actions := filepath.Join(extra, "hicolor", "16x16", "apps")
	if err := os.MkdirAll(actions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "hicolor", "index.theme"), []byte("[Icon Theme]\nName=Hicolor\nDirectories=16x16/apps\n\n[16x16/apps]\nSize=16\nType=Fixed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	SetIconSearchPaths(append([]string{extra}, fallback...))
	if got := IconSearchPaths(); len(got) != len(fallback)+1 || got[0] != extra {
		t.Fatalf("search paths = %v, want %s first", got, extra)
	}
	if !ThemeIconExists("flag") {
		t.Error("flag no longer resolves from the original paths")
	}

	if ThemeIconExists("installed") {
		t.Fatal("installed resolves before it exists")
	}
	w := NewThemeIcon("installed", 16)
	CollectDamage(w)
	paintIcon(w, 16, 1, 1)
	CollectDamage(w)
	if err := os.WriteFile(filepath.Join(actions, "installed.svg"), []byte(testBlueSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, any := CollectDamage(w); any {
		t.Error("damage before RefreshIcons: the cached miss was dropped without a refresh")
	}
	RefreshIcons()
	if _, any := CollectDamage(w); !any {
		t.Fatal("RefreshIcons produced no damage on the live icon")
	}
	data, dev := paintIcon(w, 16, 1, 1)
	if got := centerPx(data, dev); got != render.RGB(0, 0, 0xff) {
		t.Errorf("center = %v, want the installed blue", got)
	}

	SetIconSearchPaths(nil)
	if got := IconSearchPaths(); len(got) == 0 || got[0] == extra {
		t.Errorf("nil search paths = %v, want the XDG default", got)
	}
}

// An icon given more room than its size paints centered in it, as
// GtkImage centers its paintable; an exact fit paints at its origin
// (#251).
func TestIconPaintsCentered(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range src.Pix {
		src.Pix[i] = 0xff
	}
	ic, err := render.IconFromImage(src, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	ink := func(w, h int) render.Rect {
		icon := NewIcon(ic)
		icon.Measure(Constraints{Max: Size{W: w, H: h}})
		icon.Arrange(render.Rect{W: w, H: h})
		buf := make([]byte, render.Stride(w)*h)
		icon.Paint(render.New(buf, render.Stride(w), w, h))
		box := render.Rect{X: w, Y: h}
		right, bottom := 0, 0
		for y := range h {
			for x := range w {
				if render.ColorFromBytes(buf[y*render.Stride(w)+x*4:]).A() == 0 {
					continue
				}
				box.X, box.Y = min(box.X, x), min(box.Y, y)
				right, bottom = max(right, x+1), max(bottom, y+1)
			}
		}
		box.W, box.H = right-box.X, bottom-box.Y
		return box
	}
	if got := ink(24, 16); got != (render.Rect{X: 8, Y: 4, W: 8, H: 8}) {
		t.Errorf("icon in 24x16 painted %v, want centered at (8,4)", got)
	}
	if got := ink(8, 8); got != (render.Rect{W: 8, H: 8}) {
		t.Errorf("icon in an exact fit painted %v, want its origin", got)
	}
}

// A fallback theme icon resolves through its dash-shortened forms; a
// plain one keeps the exact name (#258).
func TestThemeIconFallback(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "hicolor", "scalable", "apps")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "[Icon Theme]\nName=hicolor\nDirectories=scalable/apps\n\n[scalable/apps]\nSize=16\nType=Scalable\nMinSize=8\nMaxSize=512\n"
	if err := os.WriteFile(filepath.Join(root, "hicolor", "index.theme"), []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16" fill="#ff0000"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "preferences-system.svg"), []byte(svg), 0o600); err != nil {
		t.Fatal(err)
	}
	SetIconSearchPaths([]string{root})
	t.Cleanup(func() { SetIconSearchPaths(nil) })
	inked := func(ic *Icon) bool {
		buf := paintPixels(t, ic, 16, 16)
		return pxAt(buf, 16, 8, 8).A() > 0
	}
	if !inked(NewThemeIconFallback("preferences-system-network", 16)) {
		t.Error("the fallback icon painted nothing, want preferences-system")
	}
	if inked(NewThemeIcon("preferences-system-network", 16)) {
		t.Error("an exact lookup of a missing name painted")
	}
}

// inkedRect is the device rect of the non-black pixels in a dev x dev
// buffer.
func inkedRect(data []byte, dev int) render.Rect {
	var r render.Rect
	for y := range dev {
		for x := range dev {
			if render.ColorFromBytes(data[y*render.Stride(dev)+x*4:])&0xffffff != 0 {
				r = r.Union(render.Rect{X: x, Y: y, W: 1, H: 1})
			}
		}
	}
	return r
}

// A 20px icon covers DeviceBox(20, frac) device pixels at every scale,
// themed or inline, straight or through -gtk-icon-transform, not its
// raster's size again times the scale (#374).
func TestIconCoversItsDeviceBox(t *testing.T) {
	iconTree(t)
	for _, xf := range []string{"", "icon { -gtk-icon-transform: scale(1.0001); }"} {
		if xf != "" {
			loadCSS(t, xf)
		}
		for _, frac := range []int{120, 180, 240} {
			for name, w := range map[string]*Icon{
				"theme": NewThemeIcon("flag", 20),
				"svg":   NewSVGIcon([]byte(testGreenSVG), 20),
			} {
				data, dev := paintIcon(w, 40, frac, 120)
				box := icons.DeviceBox(20, uint32(frac))
				at := 10 * frac / 120
				if got, want := inkedRect(data, dev), (render.Rect{X: at, Y: at, W: box, H: box}); got != want {
					t.Errorf("%s at %d/120, transform %q: inked %+v, want %+v", name, frac, xf, got, want)
				}
			}
		}
	}
}
