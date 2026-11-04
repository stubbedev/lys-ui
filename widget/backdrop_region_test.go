package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func glassTree(t *testing.T) (root, glass *Box, spin *Spinner, c *animClock) {
	t.Helper()
	loadCSS(t, `.glass { backdrop-filter: blur(8px); border-radius: 0; }`)
	c = pinAnimClock(t)
	glass = NewBox(Column, 0, 0)
	glass.AddClass("glass")
	glass.SetSizeRequest(100, 50)
	spin = NewSpinner(24)
	spin.SetSpinning(true)
	root = NewBox(Column, 0, 0).Append(glass, false).Append(spin, false)
	t.Cleanup(Host(root))
	return root, glass, spin, c
}

func TestBackdropRegionWalksNothingWithoutAChange(t *testing.T) {
	root, glass, _, c := glassTree(t)
	r := render.Rect{W: 200, H: 200}
	SizeWithin(root, Size{W: 200, H: 200})
	root.Arrange(r)
	first := BackdropRegion(root)
	if !slices.Equal(first, []render.Rect{glass.Bounds()}) {
		t.Fatalf("region %v, want the glass box %v", first, glass.Bounds())
	}
	walks := BackdropWalks()
	for range 50 {
		c.step()
		root.Arrange(r)
		CollectDamage(root)
		BackdropRegion(root)
	}
	if got := BackdropWalks() - walks; got != 0 {
		t.Errorf("%d walks across spinner frames, want 0", got)
	}
	if allocs := testing.AllocsPerRun(50, func() { BackdropRegion(root) }); allocs > 0 {
		t.Errorf("a region nothing moved allocates %v", allocs)
	}
}

func TestBackdropRegionFollowsWhatItReads(t *testing.T) {
	root, glass, _, _ := glassTree(t)
	layoutTo := func(w, h int) {
		SizeWithin(root, Size{W: w, H: h})
		root.Arrange(render.Rect{W: w, H: h})
	}
	layoutTo(200, 200)
	BackdropRegion(root)
	for name, change := range map[string]func(){
		"a resize": func() { layoutTo(300, 200) },
		"a class": func() {
			glass.RemoveClass("glass")
			layoutTo(300, 200)
		},
		"visibility": func() {
			glass.AddClass("glass")
			layoutTo(300, 200)
			BackdropRegion(root)
			glass.SetVisible(false)
			layoutTo(300, 200)
		},
	} {
		before := slices.Clone(BackdropRegion(root))
		change()
		if got := BackdropRegion(root); slices.Equal(got, before) {
			t.Errorf("after %s the region stayed %v", name, got)
		}
		glass.SetVisible(true)
		glass.AddClass("glass")
		layoutTo(200, 200)
	}
}
