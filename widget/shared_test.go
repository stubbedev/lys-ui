package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/render"
)

// A tagged widget that takes over from another with its tag morphs out
// of it: the first frame shows the other's rendering where it was, the
// motion carries the shape across with the colors blending, and it
// lands as itself (#202).
func TestSharedElementMorphs(t *testing.T) {
	loadCSS(t, `.small { min-width: 20px; min-height: 20px; margin: 10px; background-color: #ff0000; border-radius: 10px; }
		.big { min-width: 60px; min-height: 60px; margin-left: 40px; margin-top: 40px; background-color: #0000ff; border-radius: 4px; }`)
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	t.Cleanup(func() { clear(sharedShots) })

	const w, h = 120, 120
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	root := NewBox(Column, 0, 0)
	frame := func() {
		root.Measure(Constraints{Max: Size{W: w, H: h}})
		root.Arrange(render.Rect{W: w, H: h})
		CollectDamage(root)
		cv.Clear(cv.Rect(), 0)
		root.Paint(cv)
	}
	at := func(x, y int) render.Color { return render.ColorFromBytes(data[y*render.Stride(w)+x*4:]) }

	small := NewBox(Column, 0, 0)
	small.AddClass("small")
	small.SetSharedElement("hero")
	root.Append(small, false)
	frame()
	if c := at(20, 20); c.R() < 200 {
		t.Fatalf("the small hero did not paint: %v", c)
	}

	root.Remove(small)
	big := NewBox(Column, 0, 0)
	big.AddClass("big")
	big.SetSharedElement("hero")
	root.Append(big, false)
	frame()
	if c := at(20, 20); c.R() < 200 || c.B() > 60 {
		t.Errorf("first frame of the morph at the small rect = %v, want the small hero's red still there", c)
	}
	if c := at(90, 90); c.A() != 0 {
		t.Errorf("first frame already filled the big rect's far corner: %v", c)
	}

	sawBlend := false
	for range 120 {
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
		frame()
		if c := at(45, 45); c.R() > 40 && c.B() > 40 {
			sawBlend = true
		}
	}
	if !sawBlend {
		t.Error("never saw red and blue blend mid-morph")
	}
	if c := at(70, 70); c.B() < 200 || c.R() > 10 {
		t.Errorf("settled center = %v, want the big hero's blue", c)
	}
	if c := at(20, 20); c.A() != 0 {
		t.Errorf("settled: the small rect still shows %v", c)
	}
}

// Under reduced motion the takeover cross-fades in place: nothing
// travels from the old rect.
func TestSharedElementReducedMotionCrossFades(t *testing.T) {
	loadCSS(t, `.small { min-width: 20px; min-height: 20px; margin: 10px; background-color: #ff0000; }
		.big { min-width: 60px; min-height: 60px; margin-left: 40px; margin-top: 40px; background-color: #0000ff; }`)
	SetTheme(DarkTheme().WithAnimations(false))
	t.Cleanup(func() { SetTheme(nil) })
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	t.Cleanup(func() { clear(sharedShots) })
	const w, h = 120, 120
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	root := NewBox(Column, 0, 0)
	frame := func() {
		root.Measure(Constraints{Max: Size{W: w, H: h}})
		root.Arrange(render.Rect{W: w, H: h})
		CollectDamage(root)
		cv.Clear(cv.Rect(), 0)
		root.Paint(cv)
	}
	small := NewBox(Column, 0, 0)
	small.AddClass("small")
	small.SetSharedElement("hero")
	root.Append(small, false)
	frame()
	root.Remove(small)
	big := NewBox(Column, 0, 0)
	big.AddClass("big")
	big.SetSharedElement("hero")
	root.Append(big, false)
	now = now.Add(animclock.FrameInterval)
	animclock.Tick(now)
	frame()
	if c := render.ColorFromBytes(data[20*render.Stride(w)+20*4:]); c.A() != 0 {
		t.Errorf("reduced motion drew at the old rect: %v", c)
	}
	if c := render.ColorFromBytes(data[70*render.Stride(w)+70*4:]); c.A() == 0 {
		t.Error("reduced motion showed nothing at the new rect mid-fade")
	}
}

// A tagged widget opening on another surface morphs out of the source
// rect carried into its surface's coordinates through both surfaces'
// origins; with the destination unplaced there is no morph, and the
// surface's own reveal stands in (#226).
func TestSharedElementAcrossSurfaces(t *testing.T) {
	loadCSS(t, `.hero { min-width: 20px; min-height: 20px; margin: 10px; background-color: #ff0000; }`)
	now := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return now }))
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	t.Cleanup(func() { clear(sharedShots) })
	window, popover := NewBox(Column, 0, 0), NewBox(Column, 0, 0)
	placed := true
	SetSurfaceOriginResolver(func(root Widget) (Point, bool) {
		switch root {
		case Widget(window):
			return Point{}, true
		case Widget(popover):
			return Point{X: 100, Y: 50}, placed
		}
		return Point{}, false
	})
	t.Cleanup(func() { SetSurfaceOriginResolver(nil) })
	paint := func(root *Box) {
		root.Measure(Constraints{Max: Size{W: 120, H: 120}})
		root.Arrange(render.Rect{W: 120, H: 120})
		CollectDamage(root)
		root.Paint(render.New(make([]byte, render.Stride(120)*120), render.Stride(120), 120, 120))
	}
	button := NewBox(Column, 0, 0)
	button.AddClass("hero")
	button.SetSharedElement("open")
	window.Append(button, false)
	paint(window)

	card := NewBox(Column, 0, 0)
	card.AddClass("hero")
	card.SetSharedElement("open")
	popover.Append(card, false)
	if !SharedMorphPending(popover) {
		t.Error("the popover's content does not report a morph pending")
	}
	paint(popover)
	if card.shared == nil {
		t.Fatal("no morph started across the surfaces")
	}
	if got, want := card.shared.from.rect, (render.Rect{X: 10 - 100, Y: 10 - 50, W: 100, H: 20}); got != want {
		t.Errorf("morph starts at %v, want the button's rect in the popover's coordinates %v", got, want)
	}

	placed = false
	clear(sharedShots)
	paint(window)
	other := NewBox(Column, 0, 0)
	other.AddClass("hero")
	other.SetSharedElement("open")
	popover.Clear()
	popover.Append(other, false)
	paint(popover)
	if other.shared != nil {
		t.Error("an unplaced popover started a morph; want its own reveal")
	}
}
