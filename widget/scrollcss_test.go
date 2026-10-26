package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// newSnapScroll builds a scroll of stubH-tall stacked stubs in a
// viewH-tall viewport: a snap point at every stub's start when they
// carry snap-align.
func newSnapScroll(t *testing.T, css, class string, stubH, viewH, n int) (*Scroll, []*stub) {
	t.Helper()
	if css != "" {
		loadCSS(t, css)
	}
	col := NewBox(Column, 0, 0)
	stubs := make([]*stub, n)
	for i := range stubs {
		stubs[i] = newStub(60, stubH)
		if class != "" {
			stubs[i].AddClass(class)
		}
		col.Append(stubs[i], false)
	}
	s := NewScroll(col)
	arrangeTree(t, s, 60, viewH)
	return s, stubs
}

// scroll-behavior: smooth animates a programmatic reveal; auto lands
// at once. The wheel never animates either way.
func TestScrollBehaviorSmoothReveal(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `scrolledwindow { scroll-behavior: smooth; }`)
	child := newStub(60, 200)
	s := NewScroll(child)
	arrangeTree(t, s, 60, 60)
	RevealRect(child, render.Rect{X: 0, Y: 150, W: 60, H: 10})
	if _, offY := s.Offset(); offY != 0 {
		t.Fatalf("smooth reveal jumped at once to %d", offY)
	}
	c.step()
	if _, mid := s.Offset(); mid <= 0 || mid >= 100 {
		t.Fatalf("smooth reveal mid-flight at %d, want it between 0 and 100", mid)
	}
	c.drive()
	if _, offY := s.Offset(); offY != 100 {
		t.Fatalf("smooth reveal settled at %d, want 100", offY)
	}

	// Without the declaration the same reveal lands within the call.
	loadCSS(t, ``)
	child2 := newStub(60, 200)
	s2 := NewScroll(child2)
	arrangeTree(t, s2, 60, 60)
	RevealRect(child2, render.Rect{X: 0, Y: 150, W: 60, H: 10})
	if _, offY := s2.Offset(); offY != 100 {
		t.Fatalf("auto reveal at %d, want 100 at once", offY)
	}

	// The wheel stays instant under smooth.
	loadCSS(t, `scrolledwindow { scroll-behavior: smooth; }`)
	s.ScrollBy(0, 3)
	if _, offY := s.Offset(); offY <= 100 {
		t.Fatalf("wheel scrolled to %d under smooth, want the instant step past 100", offY)
	}
}

// scroll-margin grows the revealed rect and scroll-padding insets the
// viewport: a rect that would clear both edges now scrolls by exactly
// the margin and padding between them.
func TestScrollRevealMargins(t *testing.T) {
	loadCSS(t, `scrolledwindow { scroll-padding: 5px 0 6px; } .m { scroll-margin: 4px 0 8px; }`)
	child := newStub(60, 200)
	child.AddClass("m")
	s := NewScroll(child)
	arrangeTree(t, s, 60, 60)
	// The rect occupies viewport rows 45..50; with a 4px top and 8px
	// bottom margin it spans 41..58, and the padding leaves the
	// usable viewport 5..54 — it now ends past 54 by 4.
	RevealRect(child, render.Rect{X: 0, Y: 45, W: 60, H: 5})
	if _, offY := s.Offset(); offY != 4 {
		t.Fatalf("margin-padded reveal scrolled %d, want 4", offY)
	}
}

// Mandatory snapping settles the wheel step on the snap position;
// proximity lets a resting offset stay when nothing is near.
func TestScrollSnapMandatoryAndProximity(t *testing.T) {
	s, _ := newSnapScroll(t, `scrolledwindow { scroll-snap-type: y mandatory; } .snap { scroll-snap-align: start; }`, "snap", 60, 60, 3)
	s.ScrollBy(0, 5)
	if _, offY := s.Offset(); offY != 60 {
		t.Fatalf("mandatory wheel offset %d, want the snap position 60", offY)
	}

	// Proximity: points 80 apart, viewport 40 — a target farther than
	// half a viewport from both stays where the step left it.
	s2, _ := newSnapScroll(t, `scrolledwindow { scroll-snap-type: y proximity; } .snap { scroll-snap-align: start; }`, "snap", 80, 40, 3)
	s2.SetOffset(0, 25)
	s2.ScrollBy(0, 1)
	_, offY := s2.Offset()
	if offY == 0 || offY == 80 || offY == 160 {
		t.Fatalf("proximity snapped a small step to %d, want it stranded between the points", offY)
	}

	// Proximity still snaps a step that lands near a point.
	s3, _ := newSnapScroll(t, `scrolledwindow { scroll-snap-type: y proximity; } .snap { scroll-snap-align: end; }`, "snap", 60, 60, 3)
	s3.ScrollBy(0, 4)
	if _, offY := s3.Offset(); offY != 60 {
		t.Fatalf("proximity long step settled at %d, want the end-aligned 60", offY)
	}
}

// Mandatory snapping re-settles through a resize: an end-aligned
// child's snap position tracks the viewport height, and the stranded
// offset lands back on it in the relayout.
func TestScrollSnapResnapOnResize(t *testing.T) {
	s, _ := newSnapScroll(t, `scrolledwindow { scroll-snap-type: y mandatory; } .snap { scroll-snap-align: end; }`, "snap", 60, 60, 2)
	if _, offY := s.Offset(); offY != 0 {
		t.Fatalf("initial end-aligned offset %d, want 0", offY)
	}
	s.Measure(Constraints{Max: Size{W: 60, H: 40}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 40})
	if _, offY := s.Offset(); offY != 20 {
		t.Fatalf("re-snapped offset %d, want the resized end alignment 20", offY)
	}
}

// scroll-snap-align: center aligns the child's middle with the
// viewport's; scroll-snap-stop: always clamps a target that would skip
// an always panel.
func TestScrollSnapAlignCenterAndStop(t *testing.T) {
	s, _ := newSnapScroll(t, `scrolledwindow { scroll-snap-type: y mandatory; } .snap { scroll-snap-align: center; }`, "snap", 60, 60, 2)
	if pts := s.snapPoints(Column); len(pts) != 2 || pts[0].pos != 0 || pts[1].pos != 60 {
		t.Fatalf("center snap points %v, want 0 and 60", pts)
	}
	if got := s.snapAxisTo(Column, 100, true); got != 60 {
		t.Fatalf("center snap target %d, want 60", got)
	}

	// The middle panel is scroll-snap-stop: always; a target past it
	// ends there instead of skipping to the last.
	s2, stubs := newSnapScroll(t, `scrolledwindow { scroll-snap-type: y mandatory; }
		.snap { scroll-snap-align: start; }
		.stop { scroll-snap-stop: always; }`, "snap", 60, 60, 3)
	stubs[1].AddClass("stop")
	arrangeTree(t, s2, 60, 60)
	if got := s2.snapAxisTo(Column, 160, true); got != 60 {
		t.Fatalf("always-stop target %d, want clamped at the middle 60", got)
	}
}

// Overscroll chaining: auto hands a clamped delta to the parent scroll
// (the root, with none above, glows), contain keeps it as its own
// glow, none shows nothing at all.
func TestOverscrollChaining(t *testing.T) {
	build := func(css string) (*Scroll, *Scroll) {
		t.Helper()
		loadCSS(t, css)
		inner := propagating(NewScroll(newStub(60, 200)))
		outer := propagating(NewScroll(inner))
		arrangeTree(t, outer, 60, 60)
		return outer, inner
	}
	outer, inner := build(``)
	inner.ScrollPixels(0, 40)
	if _, offY := outer.Offset(); offY != 40 {
		t.Fatalf("auto chained outer to %d, want 40", offY)
	}
	if inner.overshoot[1] != 0 {
		t.Fatal("a chained scroll also glowed")
	}

	outer, inner = build(`scrolledwindow { overscroll-behavior: contain; }`)
	inner.ScrollPixels(0, 40)
	if _, offY := outer.Offset(); offY != 0 {
		t.Fatalf("contain chained outer to %d, want 0", offY)
	}
	if inner.overshoot[1] <= 0 {
		t.Fatal("contain showed no edge glow")
	}

	outer, inner = build(`scrolledwindow { overscroll-behavior: none; }`)
	inner.ScrollPixels(0, 40)
	if _, offY := outer.Offset(); offY != 0 {
		t.Fatalf("none chained outer to %d, want 0", offY)
	}
	if inner.overshoot[1] != 0 {
		t.Fatal("none glowed")
	}
}

// scrollbar-gutter: stable reserves the bar's column in measure even
// without overflow; both-edges reserves a matching one on the near
// side.
func TestScrollbarGutterStable(t *testing.T) {
	loadCSS(t, `scrolledwindow { scrollbar-gutter: stable; }`)
	s := NewScroll(newStub(50, 40))
	arrangeTree(t, s, 60, 60)
	if got := s.Measure(Constraints{Max: Size{W: 200, H: 200}}); got.W != 58 {
		t.Fatalf("stable gutter measured %d wide, want 58", got.W)
	}
	if s.viewW != 52 {
		t.Fatalf("stable gutter viewport %d wide, want 52", s.viewW)
	}

	loadCSS(t, `scrolledwindow { scrollbar-gutter: stable both-edges; }`)
	s2 := NewScroll(newStub(50, 40))
	arrangeTree(t, s2, 60, 60)
	if got := s2.Measure(Constraints{Max: Size{W: 200, H: 200}}); got.W != 66 {
		t.Fatalf("both-edges gutter measured %d wide, want 66", got.W)
	}

	loadCSS(t, ``)
	s3 := NewScroll(newStub(50, 40))
	arrangeTree(t, s3, 60, 60)
	if got := s3.Measure(Constraints{Max: Size{W: 200, H: 200}}); got.W != 50 {
		t.Fatalf("auto gutter measured %d wide, want the bare 50", got.W)
	}
}

// The scrollbar-color pair and scrollbar-width reach the bar spec:
// none collapses the standalone bar's measure, thin keeps the slot.
func TestScrollbarColorWidthSpec(t *testing.T) {
	loadCSS(t, `* { scrollbar-color: #ff0000 #0000ff; scrollbar-width: thin; }`)
	s := NewScroll(newStub(60, 200))
	b := NewScrollbar(s, Column)
	arrangeTree(t, b, 8, 60)
	spec := barSpecOf(b.style(b))
	if spec.thumb != render.RGB(255, 0, 0) || spec.track != render.RGB(0, 0, 255) {
		t.Fatalf("spec colors %x %x, want red thumb blue track", spec.thumb, spec.track)
	}
	if spec.thickness() != 4 {
		t.Fatalf("thin thickness %d, want 4", spec.thickness())
	}

	loadCSS(t, `* { scrollbar-width: none; }`)
	b2 := NewScrollbar(s, Column)
	arrangeTree(t, b2, 8, 60)
	if got := b2.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{}) {
		t.Fatalf("none bar measured %v, want zero", got)
	}
}

// transition-behavior: allow-discrete flips a discrete property at the
// tween's halfway point: the from value holds through the first half,
// the to value from halfway on.
func TestTransitionAllowDiscreteFlip(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `button { isolation: isolate; transition: all 200ms allow-discrete; }
		button:hover { isolation: auto; }`)
	b := NewButton(NewSpacer(0, 0), 0, 0)
	arrangeTree(t, b, 40, 40)
	if !b.style(b).Isolate {
		t.Fatal("resting isolation is not isolate")
	}
	b.SetHovered(true)
	_ = b.style(b)
	c.step()
	if !b.style(b).Isolate {
		t.Fatal("the discrete flip happened before halfway")
	}
	c.drive()
	if b.style(b).Isolate {
		t.Fatal("the discrete flip never landed")
	}
}

// animation-composition: add sums the animation's frame onto the
// underlying cascade value instead of replacing it.
func TestAnimationCompositionAdd(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `@keyframes half { from { opacity: 0.5; } to { opacity: 0.5; } }
		button { opacity: 0.5; animation: half 200ms; animation-composition: add; }`)
	b := NewButton(NewSpacer(0, 0), 0, 0)
	arrangeTree(t, b, 40, 40)
	c.step()
	if got := b.style(b).Opacity; got < 0.99 {
		t.Fatalf("add composed opacity %v, want 0.5 + 0.5 = 1", got)
	}
	c.drive()
	if got := b.style(b).Opacity; got != 0.5 {
		t.Fatalf("after the fill-less animation the opacity is %v, want the cascade 0.5", got)
	}
}

// @starting-style gives a newly added widget its entry transition: the
// first style's tween starts from the block's values.
func TestStartingStyleEntryTransition(t *testing.T) {
	c := pinAnimClock(t)
	loadCSS(t, `button { opacity: 1; transition: opacity 200ms; @starting-style { opacity: 0; } }`)
	b := NewButton(NewSpacer(0, 0), 0, 0)
	arrangeTree(t, b, 40, 40)
	c.step()
	if got := b.style(b).Opacity; got <= 0 || got >= 1 {
		t.Fatalf("entry opacity %v, want it mid-flight from the starting 0", got)
	}
	c.drive()
	if got := b.style(b).Opacity; got != 1 {
		t.Fatalf("entry settled at %v, want 1", got)
	}
}
