package widget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

type scaledIcons struct {
	root  *Box
	icons map[string]*Icon
}

func newScaledIcons(t *testing.T) scaledIcons {
	t.Helper()
	iconTree(t)
	path := filepath.Join(t.TempDir(), "flag.svg")
	if err := os.WriteFile(path, []byte(testGreenSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	sc := scaledIcons{root: NewBox(Row, 0, 0), icons: map[string]*Icon{
		"file":           NewFileIcon(path, 16),
		"svg":            NewSVGIcon([]byte(testGreenSVG), 16),
		"theme":          NewThemeIcon("flag", 16),
		"theme fallback": NewThemeIconFallback("flag-extra-name", 16),
	}}
	for _, name := range []string{"file", "svg", "theme", "theme fallback"} {
		sc.root.Append(sc.icons[name], false)
	}
	return sc
}

func (sc scaledIcons) paint(num, denom int) []byte {
	const w, h = 64, 16
	dw, dh := (w*num+denom-1)/denom, (h*num+denom-1)/denom
	data := make([]byte, render.Stride(dw)*dh)
	SizeWithin(sc.root, Size{W: w, H: h})
	sc.root.Arrange(render.Rect{W: w, H: h})
	sc.root.Paint(render.NewScaled(data, render.Stride(dw), dw, dh, num, denom))
	return data
}

func (sc scaledIcons) assertExact(t *testing.T, num, denom int) {
	t.Helper()
	for name, ic := range sc.icons {
		if ic.resolvedFrac != fracOf(num, denom) || (ic.ic == nil && !ic.retint.active) {
			t.Errorf("%s: painted from a raster made at %d, want the exact %d", name, ic.resolvedFrac, fracOf(num, denom))
		}
	}
}

func TestATreesFirstPaintAtItsScaleIsExactAndDecodesNothingOnTheLoop(t *testing.T) {
	for _, sc := range fractionalScales {
		t.Run(sc.name, func(t *testing.T) {
			f := newFakeInvoker(t)
			onLoop, _ := countDecodesOnLoop(t)
			tree := newScaledIcons(t)
			SetDeviceScale(tree.root, sc.num, sc.denom)
			settleAssets(t, f)
			tree.paint(sc.num, sc.denom)
			tree.assertExact(t, sc.num, sc.denom)
			if num, denom := DeviceScaleOf(tree.icons["svg"]); fracOf(num, denom) != fracOf(sc.num, sc.denom) {
				t.Errorf("DeviceScaleOf = %d/%d", num, denom)
			}
			if n := onLoop.Load(); n != 0 {
				t.Errorf("%d decodes on the loop", n)
			}
		})
	}
}

func TestAnIconArrangedIntoAScaledTreePreparesBeforeItsPaint(t *testing.T) {
	f := newFakeInvoker(t)
	onLoop, _ := countDecodesOnLoop(t)
	root := NewBox(Row, 0, 0)
	SetDeviceScale(root, 5, 4)
	late := NewSVGIcon([]byte(testGreenSVG), 16)
	root.Append(late, false)
	SizeWithin(root, Size{W: 16, H: 16})
	root.Arrange(render.Rect{W: 16, H: 16})
	settleAssets(t, f)
	data, dev := paintIcon(late, 16, 5, 4)
	if got := centerPx(data, dev); got != render.RGB(0, 0xff, 0) || late.resolvedFrac != fracOf(5, 4) {
		t.Errorf("center %v at %d, want green at the exact scale", got, late.resolvedFrac)
	}
	if n := onLoop.Load(); n != 0 {
		t.Errorf("%d decodes on the loop", n)
	}
}

func TestAScaleChangeDecodesBeforeTheNextPaintAndKeepsTheOldRasterMeanwhile(t *testing.T) {
	f := newFakeInvoker(t)
	onLoop, _ := countDecodesOnLoop(t)
	tree := newScaledIcons(t)
	SetDeviceScale(tree.root, 5, 4)
	settleAssets(t, f)
	tree.paint(5, 4)
	tree.assertExact(t, 5, 4)

	SetDeviceScale(tree.root, 3, 2)
	tree.paint(3, 2)
	for name, ic := range tree.icons {
		if ic.ic == nil && !ic.retint.active {
			t.Errorf("%s: blank while its new raster decodes", name)
		}
	}
	settleAssets(t, f)
	tree.paint(3, 2)
	tree.assertExact(t, 3, 2)
	if n := onLoop.Load(); n != 0 {
		t.Errorf("%d decodes on the loop", n)
	}
}

func TestATreeWithoutAScaleAsksForNothing(t *testing.T) {
	newFakeInvoker(t)
	tree := newScaledIcons(t)
	SizeWithin(tree.root, Size{W: 64, H: 16})
	tree.root.Arrange(render.Rect{W: 64, H: 16})
	assets.mu.Lock()
	queued := len(assets.jobs)
	assets.mu.Unlock()
	if queued != 0 {
		t.Errorf("an unscaled arrange queued %d decodes", queued)
	}
}
