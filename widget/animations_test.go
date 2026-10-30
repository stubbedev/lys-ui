package widget

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/style"
)

// An `animation` on a box runs its keyframes against the style cache:
// the opacity interpolates between the stops, loops while infinite,
// and alternates direction on odd cycles.
func TestCSSAnimationRunsAndLoops(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes fade {
		from { opacity: 1; }
		to { opacity: 0.5; }
	}
	box { animation: fade 100ms linear infinite; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 50, 50) // one pass so the animation starts
	c.step()
	if got := b.style(b).Opacity; got <= 0.5 || got >= 1 {
		t.Fatalf("mid-flight opacity %v, want between the stops", got)
	}
	for range 12 {
		c.step()
	}
	if got := b.style(b).Opacity; got <= 0.5 || got >= 1 {
		t.Fatalf("opacity %v after a wrap, want the loop running", got)
	}

	// Pausing holds the phase: the value stops moving.
	loadCSS(t, `@keyframes fade { from { opacity: 1; } to { opacity: 0.5; } }
		box { animation: fade 100ms linear infinite; animation-play-state: paused; }`)
	paused := b.style(b).Opacity
	for c.step() {
	}
	if got := b.style(b).Opacity; got != paused {
		t.Errorf("paused opacity moved %v to %v", paused, got)
	}
}

// A finite animation reverts to the cascade when its iterations end,
// and `animation: none` drops the animation at once.
func TestCSSAnimationEndsAndReverts(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes fade { from { opacity: 1; } to { opacity: 0.4; } }
		box { opacity: 0.9; animation: fade 50ms linear 2; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 50, 50)
	for c.step() {
	}
	if got := b.style(b).Opacity; got != 0.9 {
		t.Errorf("after two iterations the opacity is %v, want the cascade's 0.9", got)
	}

	loadCSS(t, `@keyframes fade { from { opacity: 1; } to { opacity: 0.4; } }
		box { animation: none; }`)
	_ = b.style(b)
	if b.style(b).Has(style.PropOpacity) {
		t.Errorf("with no animation the opacity is still declared: %v", b.style(b).Opacity)
	}
}

// An unknown keyframes name animates nothing, and reduced motion does
// not start the tween.
func TestCSSAnimationInactive(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `box { animation: nosuch 100ms linear infinite; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 50, 50)
	if c.step() {
		t.Error("an unknown name scheduled a tween")
	}

	loadCSS(t, `@keyframes fade { from { opacity: 1; } to { opacity: 0.4; } }
		box { animation: fade 100ms linear infinite; }`)
	restore := anim.SetInstant(true)
	defer restore()
	b2 := NewBox(Column, 0, 0)
	frame(t, b2, 50, 50)
	if b2.style(b2).Has(style.PropOpacity) {
		t.Errorf("reduced motion declared an opacity: %v", b2.style(b2).Opacity)
	}
}

func TestCSSAnimationDriveSurvivesEasedEndBeforeItsSpan(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes fade { from { opacity: 1; } to { opacity: 0.5; } }
		box { animation: fade 1s linear infinite; }`)
	b := NewBox(Column, 0, 0)
	start := c.now()
	frame(t, b, 50, 50)
	c.set(start.Add(time.Second - time.Microsecond))
	animclock.Tick(c.now())
	c.set(start.Add(time.Second + 100*time.Millisecond))
	animclock.Tick(c.now())
	if got := b.style(b).Opacity; math.Abs(got-0.95) > 1e-6 {
		t.Fatalf("opacity %v a tenth into the second cycle, want 0.95", got)
	}

	loadCSS(t, `@keyframes fade { from { opacity: 1; } to { opacity: 0.5; } }
		box { animation: none; }`)
	_ = b.style(b)
	if animclock.Active() {
		t.Fatal("a frame drive outlived animation: none")
	}
	for c.step() {
	}
}

func TestCSSAnimationEndsWhenUnderANanosecondRemains(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes fade { from { opacity: 1; } to { opacity: 0.4; } }
		box { opacity: 0.9; animation: fade 0.1000000005s linear; }`)
	b := NewBox(Column, 0, 0)
	frame(t, b, 50, 50)
	c.drive()
	if got := b.style(b).Opacity; got != 0.9 {
		t.Errorf("after the animation the opacity is %v, want the cascade's 0.9", got)
	}
	if animclock.Active() {
		t.Error("a frame drive outlived the animation's end")
	}
}

// The spin keyframes turn an icon: the cached rotation interpolates,
// and the paint draws turned - the icon's left edge becomes its top at
// 90 degrees.
func TestIconRotationAnimation(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes spin {
		from { -gtk-icon-transform: rotate(0deg); }
		to { -gtk-icon-transform: rotate(360deg); }
	}
	image { animation: spin 1s linear infinite; }`)
	ic := NewThemeIcon("foo", 16)
	host := NewBox(Column, 0, 0)
	host.Append(ic, false)
	frame(t, host, 50, 50)
	c.step()
	// Mid-flight: the icon transform is neither the identity nor the
	// settled full turn.
	x := ic.style(ic).IconXform
	if x.M == render.Identity || x.M == (render.Affine{}) {
		t.Fatalf("mid-flight icon transform %v, want inside the turn", x)
	}
}

// A static -gtk-icon-transform turns the icon: at 90 degrees the
// raster's left edge lands on the top row.
func TestIconRotationStatic(t *testing.T) {
	dot := image.NewNRGBA(image.Rect(0, 0, 6, 6))
	dot.Set(0, 3, color.NRGBA{R: 1, G: 2, B: 3, A: 255}) // the left edge, off-center
	src, err := render.IconFromImage(dot, 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	paint := func(css string) []byte {
		loadCSS(t, css)
		w := NewIcon(src)
		data := make([]byte, render.Stride(20)*20)
		cv := render.NewScaled(data, render.Stride(20), 20, 20, 1, 1)
		cv.Clear(cv.Rect(), 0)
		SizeWithin(w, Size{W: 20, H: 20})
		w.Arrange(render.Rect{X: 4, Y: 4, W: 6, H: 6})
		w.Paint(cv)
		return data
	}
	rest := paint(`image { -gtk-icon-transform: none; }`)
	turned := paint(`image { -gtk-icon-transform: rotate(90deg); }`)
	at := func(data []byte, x, y int) render.Color {
		return render.ColorFromBytes(data[y*render.Stride(20)+x*4:])
	}
	// The dot at the icon's left edge (4, 7) turns about the icon's
	// center onto its top row: (6, 4).
	if at(rest, 4, 7) == 0 {
		t.Fatalf("the unturned icon lost its dot: %v", at(rest, 4, 7))
	}
	if at(turned, 6, 4) == 0 {
		t.Errorf("the turned icon has no dot on the top edge")
	}
	if at(turned, 6, 3) != 0 {
		t.Errorf("the turned icon drew above its own box")
	}
	if at(turned, 4, 7) != 0 {
		t.Errorf("the turned icon kept a dot on the left edge")
	}
}
