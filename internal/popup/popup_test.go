// The popup's tween paint wrapper: enter's first frame paints nothing
// (the zero-alpha skip proof), tween frames paint ink at partial
// reveal, completion rests exactly on the content rect, and the slide
// shifts toward the anchor without ever leaving the surface bounds.
package popup

import (
	"errors"
	"testing"
	"time"

	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/surfx"
	"github.com/stubbedev/lys-ui/internal/wlsession"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// inkProbe is a leaf that fills its bounds — the paint-count hook.
// (Internal packages cannot embed widget.node, so this is hand-rolled.)
type inkProbe struct {
	bounds render.Rect
	paints int
}

func (p *inkProbe) Measure(con widget.Constraints) widget.Size {
	return widget.Size{W: min(60, con.Max.W), H: min(20, con.Max.H)}
}

func (p *inkProbe) Arrange(r render.Rect) { p.bounds = r }

func (p *inkProbe) Paint(cv *render.Canvas) {
	p.paints++
	cv.FillRect(p.bounds, render.RGB(200, 100, 0))
}

func (p *inkProbe) HitTest(widget.Point) widget.Widget { return p }

// newTweenView builds the anim view over an ink probe at (0,0) sized
// 60x20, laid out the way Run lays the popup root out.
func newTweenView(t *testing.T, reveal float64, g Gravity) (*animView, *inkProbe) {
	t.Helper()
	probe := &inkProbe{}
	v := newAnimView(probe, reveal, true, g, 0, nil)
	v.morph.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 100}})
	v.morph.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 20})
	return v, probe
}

func TestAnimViewFirstFramePaintsNothing(t *testing.T) {
	v, probe := newTweenView(t, 0, GravityBottom)
	cv := render.NewScaled(make([]byte, render.Stride(64)*24), render.Stride(64), 64, 24, 1, 120)
	before := cv.Touched()
	v.morph.Paint(cv)
	if probe.paints != 0 || cv.Touched() != before {
		t.Errorf("enter's first frame painted: paints = %d touched delta = %d, want nothing written",
			probe.paints, cv.Touched()-before)
	}
	// And the same holds for a recycled buffer that already holds the
	// last opaque frame: Run raw-clears before the faded paint.
	v.setProgress(1, true)
	cv.ClearDevice(cv.Rect(), render.Color(0)) // raw clear, uncounted here
	v.morph.Paint(cv)
	if cv.Touched() == before {
		t.Fatal("rest state painted nothing; the probe lost ink")
	}
	opaque := cv.Touched()
	before = cv.Touched()
	v.setProgress(0.25, true)
	v.morph.Paint(cv)
	if cv.Touched() <= before || cv.Touched() < opaque/2 {
		t.Errorf("mid-exit frame painted no ink (touched %d, before %d)", cv.Touched(), before)
	}
}

func TestAnimViewRestMatchesContentRect(t *testing.T) {
	_, probe := newTweenView(t, 1, GravityBottom)
	if probe.bounds != (render.Rect{X: 0, Y: 0, W: 60, H: 20}) {
		t.Errorf("rest bounds = %v, want the exact content rect", probe.bounds)
	}
	// Mid-enter the popup grows from the edge facing its anchor: below
	// its anchor (bottom gravity) the top rows are inked and the bottom
	// ones not yet; above it (top gravity), the mirror.
	ink := func(g Gravity, y int) bool {
		v, _ := newTweenView(t, 0.4, g)
		data := make([]byte, render.Stride(60)*20)
		cv := render.New(data, render.Stride(60), 60, 20)
		v.morph.Paint(cv)
		return render.ColorFromBytes(data[y*render.Stride(60)+30*4:]).A() > 0
	}
	if !ink(GravityBottom, 2) || ink(GravityBottom, 18) {
		t.Errorf("bottom gravity mid-enter: top inked %v, bottom inked %v; want it growing down from the top", ink(GravityBottom, 2), ink(GravityBottom, 18))
	}
	if ink(GravityTop, 2) || !ink(GravityTop, 18) {
		t.Errorf("top gravity mid-enter: top inked %v, bottom inked %v; want it growing up from the bottom", ink(GravityTop, 2), ink(GravityTop, 18))
	}
}

// wireFreePopup is a Popup whose wire objects are nil: the state
// machine runs (sealing, dismissal flags, callbacks) while the wire
// requests no-op. The animation clock is pinned, so tween drives are
// exact and the exit keeps the surface mapped until it lands.
func wireFreePopup(t *testing.T, kind surfx.Kind) (*Popup, func()) {
	t.Helper()
	base := time.Unix(1750000000, 0)
	t.Cleanup(animclock.SetClock(func() time.Time { return base }))
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	now := base
	drive := func() {
		for i := 0; ; i++ {
			wake, ok := animclock.Next()
			if !ok {
				return
			}
			if i > 1000 {
				panic("schedule did not drain")
			}
			if wake.After(now) {
				now = wake
			}
			animclock.Tick(now)
		}
	}
	p := &Popup{}
	p.fx = surfx.NewCoordinator(kind, p, nil)
	p.fx.Enter()
	drive()
	return p, drive
}

// TestPopupDoneRoutesThroughDismiss pins the no-fast-path rule: the
// outside-click dismissal (popup_done) enters the same state machine
// as Esc and programmatic Dismiss — the logical state flips, callbacks
// fire once, and repeated popup_done mid-exit is a no-op.
func TestPopupDoneRoutesThroughDismiss(t *testing.T) {
	p, drive := wireFreePopup(t, surfx.KindMenu)
	p.HandlePopupPopupDone(xdg.PopupPopupDoneEvent{})
	if !p.Dismissed() {
		t.Fatal("popup_done did not dismiss the popup")
	}
	// The surface stays mapped for the exit tween: destroyed only after
	// the last frame lands — never on the spot (that was the old fast
	// path this machine replaces).
	if p.Destroyed() {
		t.Fatal("popup_done destroyed the surface immediately; no exit window")
	}
	drive()
	if !p.Destroyed() {
		t.Error("the exit tween landed but the wire teardown never ran")
	}
	// A popup_done again mid-exit (or after) must be a no-op.
	closed := 0
	p2, drive2 := wireFreePopup(t, surfx.KindMenu)
	p2.SetOnClosed(func() { closed++ })
	p2.HandlePopupPopupDone(xdg.PopupPopupDoneEvent{})
	p2.HandlePopupPopupDone(xdg.PopupPopupDoneEvent{})
	drive2()
	if closed != 1 {
		t.Errorf("OnClosed fired %d times for repeated popup_done, want exactly 1", closed)
	}
}

// TestInputDeadDuringExit pins the click-through contract on the client
// side: after Dismiss the surface's input handlers go inert, so a
// dying popup's tree takes no presses, no hover, and schedules no
// repaints — beside the empty input region that stops the compositor
// from routing events here in the first place.
func TestInputDeadDuringExit(t *testing.T) {
	p, _ := wireFreePopup(t, surfx.KindMenu)
	p.fx.Enter()

	probe := &inkProbe{}
	router := &widget.Router{Root: probe}
	dirties := 0
	input := &popupInput{
		dismissed: p.Dismissed,
		router:    router,
		pointer:   &struct{ x, y float64 }{},
		mods:      func() widget.Mods { return 0 },
		markDirty: func() { dirties++ },
	}
	input.HandlePointerEnter(5, 5)
	input.HandlePointerButton(0x110, 1, 7)
	if dirties != 2 || router.Hovered() == nil {
		t.Fatalf("sanity: live popup dropped input (dirties = %d hovered = %v)", dirties, router.Hovered())
	}

	p.Dismiss()
	input.HandlePointerEnter(6, 6)
	input.HandlePointerButton(0x110, 1, 8)
	input.HandlePointerLeave()
	if dirties != 2 {
		t.Errorf("dismissed popup took input: dirties = %d, want none extra", dirties)
	}
}

// TestPaintPlateShadowGutter pins the popup plate: with a gutter, the
// theme's elevation paints around the content rect, the plate rounds
// inside it, and the gutter's far corner stays transparent — the
// falloff must blend over whatever the popup floats above. Without a
// gutter the plate is the legacy whole-surface fill.
func TestPaintPlateShadowGutter(t *testing.T) {
	defer widget.SetTheme(widget.DarkTheme())
	widget.SetTheme(widget.DarkTheme().WithShadowBlur(6))
	bg := render.RGB(30, 30, 40)

	at := func(data []byte, stride, x, y int) render.Color {
		return render.ColorFromBytes(data[y*stride+x*4:])
	}

	t.Run("a content-painted popup gets no plate", func(t *testing.T) {
		for _, gutter := range []int{0, 6} {
			pc := &Painter{bg: bg, gutter: gutter, noPlate: true}
			data := make([]byte, render.Stride(72)*52)
			pc.paintPlate(render.New(data, render.Stride(72), 72, 52), render.Rect{X: 6, Y: 6, W: 60, H: 40})
			for _, pt := range [][2]int{{36, 26}, {2, 26}} {
				if got := at(data, render.Stride(72), pt[0], pt[1]); got != 0 {
					t.Errorf("gutter %d: pixel %v = %#08x, want transparent", gutter, pt, uint32(got))
				}
			}
		}
	})

	t.Run("gutter carries the shadow, corner stays transparent", func(t *testing.T) {
		pc := &Painter{bg: bg, gutter: 6}
		data := make([]byte, render.Stride(72)*52)
		pc.paintPlate(render.New(data, render.Stride(72), 72, 52), render.Rect{X: 6, Y: 6, W: 60, H: 40})

		if got := at(data, render.Stride(72), 0, 0); got != 0 {
			t.Errorf("gutter corner = %#08x, want untouched transparent", uint32(got))
		}
		if got := at(data, render.Stride(72), 2, 26); got.A() == 0 {
			t.Error("no falloff 3px into the gutter; the plate painted no elevation")
		}
		if got := at(data, render.Stride(72), 36, 26); got != bg {
			t.Errorf("plate center = %#08x, want the popup background %#08x", uint32(got), uint32(bg))
		}
		if got := at(data, render.Stride(72), 7, 7); got == bg {
			t.Error("cut plate corner painted plate color; the plate must round with the shadow")
		}
	})

	t.Run("no gutter fills the surface as before", func(t *testing.T) {
		pc := &Painter{bg: bg}
		w, h := 40, 30
		data := make([]byte, render.Stride(w)*h)
		pc.paintPlate(render.New(data, render.Stride(w), w, h), render.Rect{X: 0, Y: 0, W: w, H: h})

		for _, p := range [][2]int{{0, 0}, {39, 29}, {20, 15}} {
			if got := at(data, render.Stride(w), p[0], p[1]); got != bg {
				t.Errorf("plate at (%d,%d) = %#08x, want the background", p[0], p[1], uint32(got))
			}
		}
	})
}

// scrollProbe is a hit-anywhere ScrollHandler recording its steps.
type scrollProbe struct {
	inkProbe
	dx, dy int
}

func (s *scrollProbe) HitTest(widget.Point) widget.Widget { return s }
func (s *scrollProbe) ScrollBy(dx, dy int)                { s.dx += dx; s.dy += dy }

// TestPopupWheelScrolls pins the popup's wheel: a scroll over the
// content reaches what scrolls under the pointer (a dropdown's list),
// in the windows' steps, and goes inert once the popup is dismissed.
// Regression: HandlePointerAxis was empty, so nothing in a popover
// scrolled.
func TestPopupWheelScrolls(t *testing.T) {
	p, _ := wireFreePopup(t, surfx.KindMenu)
	p.fx.Enter()
	probe := &scrollProbe{}
	dirties := 0
	input := &popupInput{
		dismissed: p.Dismissed,
		router:    &widget.Router{Root: probe},
		pointer:   &struct{ x, y float64 }{},
		mods:      func() widget.Mods { return 0 },
		markDirty: func() { dirties++ },
	}
	input.HandlePointerEnter(5, 5)
	input.HandlePointerAxis(0, 25)
	input.HandlePointerAxis(0, -3)
	if probe.dy != wlsession.AxisSteps(25)+wlsession.AxisSteps(-3) || probe.dy != 1 || dirties != 3 {
		t.Fatalf("scrolled %d (dirties %d), want 2 then -1 steps", probe.dy, dirties)
	}
	p.Dismiss()
	input.HandlePointerAxis(0, 40)
	if probe.dy != 1 || dirties != 3 {
		t.Errorf("a dismissed popup scrolled: %d", probe.dy)
	}
}

func TestGrabWithoutAnInputEventIsRefused(t *testing.T) {
	sess := &wlsession.Session{}
	if _, err := New(sess, Config{Width: 10, Height: 10}); !errors.Is(err, wlsession.ErrNoInputEvent) {
		t.Errorf("grabbing popup before any input: err = %v, want ErrNoInputEvent", err)
	}
	if _, err := New(sess, Config{Width: 10, Height: 10, NoGrab: true}); errors.Is(err, wlsession.ErrNoInputEvent) {
		t.Errorf("a popup without a grab needs no input event: %v", err)
	}
}

// A popup holding the seat grab keeps it until the popup is destroyed,
// so its exit tween held the keyboard: a dialog the dismissing pick or
// click opened mapped without focus, and keys typed meanwhile went to
// the dying popup. A grabbing popup the client dismisses tears down at
// once; one the compositor dismissed (popup_done, the grab already
// gone) still runs its exit.
func TestDismissingAGrabbingPopupReleasesTheGrabAtOnce(t *testing.T) {
	p, _ := wireFreePopup(t, surfx.KindMenu)
	p.grabbing = true
	closed := 0
	p.SetOnClosed(func() { closed++ })
	p.Dismiss()
	if !p.Destroyed() || closed != 1 {
		t.Errorf("a dismissed grabbing popup: destroyed %v, OnClosed fired %d times; want torn down, once", p.Destroyed(), closed)
	}

	done, drive := wireFreePopup(t, surfx.KindMenu)
	done.grabbing = true
	done.HandlePopupPopupDone(xdg.PopupPopupDoneEvent{})
	if done.Destroyed() {
		t.Fatal("popup_done tore down at once; the grab was already gone, so the exit should run")
	}
	drive()
	if !done.Destroyed() {
		t.Error("the exit landed but the teardown never ran")
	}
}
