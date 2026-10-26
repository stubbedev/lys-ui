package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
)

// solidLeaf fills its rect with one color.
type solidLeaf struct {
	node
	sz  Size
	col render.Color
}

func (s *solidLeaf) Measure(con Constraints) Size { return clampSize(s.sz, con) }
func (s *solidLeaf) Arrange(r render.Rect)        { s.node.Arrange(r) }
func (s *solidLeaf) Paint(cv *render.Canvas)      { cv.FillRect(s.bounds, s.col) }
func (s *solidLeaf) HitTest(p Point) Widget       { return s.HitLeaf(s, p) }

var (
	revealBG   = render.RGB(0, 0, 0)
	revealFill = render.RGB(255, 255, 255)
)

// revealerAt arranges a 40x20 white child revealer at (20, 20) on an
// 80x60 canvas.
func revealerAt(t *testing.T, tr RevealTransition) (*Revealer, *solidLeaf) {
	t.Helper()
	leaf := &solidLeaf{sz: Size{W: 40, H: 20}, col: revealFill}
	r := NewRevealer(leaf)
	r.SetTransition(tr)
	r.SetDuration(100 * time.Millisecond)
	r.Measure(Constraints{Max: Size{W: 80, H: 60}})
	r.Arrange(render.Rect{X: 20, Y: 20, W: 40, H: 20})
	return r, leaf
}

// shot is a painted 80x60 frame.
type shot []byte

// paintReveal paints r over black.
func paintReveal(r *Revealer) shot {
	data := make([]byte, render.Stride(80)*60)
	cv := render.New(data, render.Stride(80), 80, 60)
	cv.Clear(cv.Rect(), revealBG)
	r.Paint(cv)
	return data
}

// lit counts the pixels brighter than the background inside rect.
func lit(data shot, rect render.Rect) int {
	n := 0
	stride := render.Stride(80)
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		for x := rect.X; x < rect.X+rect.W; x++ {
			if data[y*stride+x*4+2] > 8 { // red channel of BGRA
				n++
			}
		}
	}
	return n
}

// redAt is the red channel at (x, y).
func redAt(data shot, x, y int) uint8 {
	return data[y*render.Stride(80)+x*4+2]
}

// advance steps the pinned clock frame by frame while the next frame
// is due within d.
func advance(c *animClock, d time.Duration) {
	until := c.now().Add(d)
	for {
		wake, ok := animclock.Next()
		if !ok || wake.After(until) {
			return
		}
		c.step()
	}
}

func TestRevealerRevealsAndHides(t *testing.T) {
	c := pinAnimClock(t)
	r, _ := revealerAt(t, RevealFade)
	var landed []bool
	r.SetOnTransitionDone(func(v bool) { landed = append(landed, v) })

	if got := lit(paintReveal(r), render.Rect{W: 80, H: 60}); got != 0 {
		t.Fatalf("a new revealer drew %d px, want hidden", got)
	}
	if sz := r.Measure(Constraints{Max: Size{W: 80, H: 60}}); sz != (Size{W: 40, H: 20}) {
		t.Errorf("hidden fade measures %v, want the full child size held", sz)
	}

	r.SetRevealed(true)
	start := c.now()
	advance(c, 50*time.Millisecond)
	if p, want := r.Progress(), float64(c.now().Sub(start))/float64(100*time.Millisecond); p < want-0.01 || p > want+0.01 || p < 0.3 || p > 0.6 {
		t.Errorf("progress after %v = %v, want %v (linear in time)", c.now().Sub(start), p, want)
	}
	if red := redAt(paintReveal(r), 30, 30); red < 100 || red > 160 {
		t.Errorf("half-faded red = %d, want about half", red)
	}
	if len(landed) != 0 {
		t.Errorf("landed mid-flight: %v", landed)
	}
	c.drive()
	if r.Progress() != 1 || len(landed) != 1 || !landed[0] {
		t.Fatalf("after the enter: progress %v, landings %v", r.Progress(), landed)
	}
	if red := redAt(paintReveal(r), 30, 30); red != 255 {
		t.Errorf("revealed red = %d, want full", red)
	}

	// A reversal mid-exit starts from where it is, the full duration.
	r.SetRevealed(false)
	advance(c, 50*time.Millisecond)
	mid := r.Progress()
	r.SetRevealed(true)
	advance(c, 50*time.Millisecond)
	if p := r.Progress(); p <= mid || p >= 1 {
		t.Errorf("reversed from %v to %v after half the duration, want between", mid, p)
	}
	c.drive()
	if !slicesEqualBool(landed, []bool{true, true}) {
		t.Errorf("landings %v: the interrupted exit must not report", landed)
	}
	r.SetRevealed(true) // already the target
	if c.step() {
		t.Error("asking for the current target scheduled a transition")
	}
}

func slicesEqualBool(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRevealerZeroDurationSnaps(t *testing.T) {
	c := pinAnimClock(t)
	r, _ := revealerAt(t, RevealZoom)
	r.SetDuration(0)
	landed := 0
	r.SetOnTransitionDone(func(bool) { landed++ })
	r.SetRevealed(true)
	if r.Progress() != 1 || landed != 1 {
		t.Errorf("zero duration: progress %v, %d landings; want an instant landing", r.Progress(), landed)
	}
	if c.step() {
		t.Error("a zero-duration reveal scheduled frames")
	}
}

func TestRevealerNoneTakesNoSpaceHidden(t *testing.T) {
	pinAnimClock(t)
	r, _ := revealerAt(t, RevealNone)
	if sz := r.Measure(Constraints{Max: Size{W: 80, H: 60}}); sz != (Size{}) {
		t.Errorf("hidden none measures %v, want nothing", sz)
	}
	if r.HitTest(Point{X: 30, Y: 30}) != nil {
		t.Error("a hidden child took input")
	}
	r.SetRevealed(true)
	if sz := r.Measure(Constraints{Max: Size{W: 80, H: 60}}); sz != (Size{W: 40, H: 20}) {
		t.Errorf("shown none measures %v", sz)
	}
	if got := redAt(paintReveal(r), 30, 30); got != 255 {
		t.Errorf("none shows at once: red %d", got)
	}
	if r.HitTest(Point{X: 30, Y: 30}) == nil {
		t.Error("a shown child took no input")
	}
}

func TestRevealerSlideStaysInItsSlot(t *testing.T) {
	c := pinAnimClock(t)
	r, _ := revealerAt(t, RevealSlideUp)
	r.SetRevealed(true)
	advance(c, 50*time.Millisecond)
	cv := paintReveal(r)
	// Entering from below: the bottom of the slot shows, the top not yet.
	if redAt(cv, 30, 38) != 255 || redAt(cv, 30, 22) != 0 {
		t.Errorf("mid-slide: bottom red %d, top red %d; want the child entering from below",
			redAt(cv, 30, 38), redAt(cv, 30, 22))
	}
	if got := lit(cv, render.Rect{X: 20, Y: 40, W: 40, H: 20}); got != 0 {
		t.Errorf("%d px drawn below the slot; a slide is clipped to it", got)
	}
}

func TestRevealerTransformsAndDamage(t *testing.T) {
	c := pinAnimClock(t)
	r, _ := revealerAt(t, RevealZoom)
	r.SetRevealed(true)
	advance(c, 50*time.Millisecond)
	cv := paintReveal(r)
	if redAt(cv, 40, 30) == 0 {
		t.Error("a half zoom drew nothing at the center")
	}
	if redAt(cv, 21, 21) != 0 {
		t.Error("a half zoom reached the corner")
	}

	// A bounce overshoots its slot near the end; the damage covers it,
	// and the frame after repaints what the overshoot left behind.
	b, _ := revealerAt(t, RevealBounce)
	b.SetRevealed(true)
	advance(c, 85*time.Millisecond) // the last frame before landing, still overshooting
	cv = paintReveal(b)
	if got := lit(cv, render.Rect{X: 0, Y: 0, W: 80, H: 20}); got == 0 {
		t.Error("the bounce's overshoot drew nothing above the slot")
	}
	CollectDamage(b)
	c.step() // landed: the slot alone draws now
	if b.Progress() != 1 {
		t.Fatalf("progress %v after one more frame, want the landing", b.Progress())
	}
	rects, any := CollectDamage(b)
	if !any || !coversRect(rects, render.Rect{X: 20, Y: 19, W: 40, H: 1}) {
		t.Errorf("damage %v misses the overshoot above the slot", rects)
	}
}

func coversRect(rects []render.Rect, want render.Rect) bool {
	for _, r := range rects {
		if r.Intersect(want) == want {
			return true
		}
	}
	return false
}

func TestRevealerGenieCollapsesTowardItsEdge(t *testing.T) {
	c := pinAnimClock(t)
	r, _ := revealerAt(t, RevealGenie)
	r.SetGenieEdge(EdgeTop)
	r.SetRevealed(true)
	advance(c, 50*time.Millisecond)
	cv := paintReveal(r)
	if redAt(cv, 40, 20) == 0 || redAt(cv, 40, 38) != 0 {
		t.Errorf("a genie toward the top: top red %d, bottom red %d", redAt(cv, 40, 20), redAt(cv, 40, 38))
	}
}

func TestRevealerFinishLandsAtOnce(t *testing.T) {
	c := pinAnimClock(t)
	r, _ := revealerAt(t, RevealFade)
	var landed []bool
	r.SetOnTransitionDone(func(v bool) { landed = append(landed, v) })
	r.Finish() // nothing running
	if len(landed) != 0 {
		t.Fatalf("Finish with nothing running reported %v", landed)
	}
	r.SetRevealed(true)
	advance(c, 30*time.Millisecond)
	r.Finish()
	if r.Progress() != 1 || !slicesEqualBool(landed, []bool{true}) {
		t.Errorf("after Finish: progress %v, landings %v", r.Progress(), landed)
	}
	if c.step() {
		t.Error("the finished tween kept running")
	}
}

// collapsingAt is revealerAt with a collapsing slide, laid out like a
// box would: measured, then arranged at (20, 20) in what it measured.
func collapsingAt(t *testing.T, tr RevealTransition) (*Revealer, *solidLeaf) {
	t.Helper()
	r, leaf := revealerAt(t, tr)
	r.SetCollapse(true)
	relayout(r)
	return r, leaf
}

func relayout(r *Revealer) Size {
	sz := r.Measure(Constraints{Max: Size{W: 80, H: 60}})
	r.Arrange(render.Rect{X: 20, Y: 20, W: sz.W, H: sz.H})
	return sz
}

func TestRevealerCollapsingSlideGrowsWithItsProgress(t *testing.T) {
	c := pinAnimClock(t)
	r, leaf := collapsingAt(t, RevealSlideDown)
	if sz := relayout(r); sz != (Size{W: 40}) {
		t.Errorf("hidden collapsing slide measures %v, want no height", sz)
	}
	if !r.Collapse() {
		t.Error("Collapse does not report SetCollapse")
	}
	r.SetRevealed(true)
	advance(c, 50*time.Millisecond)
	sz := relayout(r)
	if sz.W != 40 || sz.H < 6 || sz.H > 14 {
		t.Fatalf("mid-slide measures %v, want about half the child's 20px height", sz)
	}
	// Entering from above: the child's bottom rides the slot's bottom.
	if want := (render.Rect{X: 20, Y: 20 + sz.H - 20, W: 40, H: 20}); leaf.bounds != want {
		t.Errorf("child arranged at %v, want %v", leaf.bounds, want)
	}
	cv := paintReveal(r)
	if redAt(cv, 30, 20+sz.H-1) != 255 {
		t.Error("the slot's last row is not drawn")
	}
	if got := lit(cv, render.Rect{X: 20, Y: 0, W: 40, H: 20}); got != 0 {
		t.Errorf("%d px drawn above the slot; the child is clipped to it", got)
	}
	if r.HitTest(Point{X: 30, Y: 19}) != nil {
		t.Error("the child took input outside the slot")
	}
	if r.HitTest(Point{X: 30, Y: 20}) == nil {
		t.Error("the slot took no input")
	}
	c.drive()
	if sz := relayout(r); sz != (Size{W: 40, H: 20}) {
		t.Errorf("revealed measures %v, want the child", sz)
	}
	if leaf.bounds != (render.Rect{X: 20, Y: 20, W: 40, H: 20}) {
		t.Errorf("revealed child at %v", leaf.bounds)
	}
}

func TestRevealerCollapsingSlidesPlaceTheirLeadingEdge(t *testing.T) {
	c := pinAnimClock(t)
	for _, tc := range []struct {
		tr   RevealTransition
		want func(sz Size) render.Rect
	}{
		{RevealSlideUp, func(sz Size) render.Rect { return render.Rect{X: 20, Y: 20, W: 40, H: 20} }},
		{RevealSlideRight, func(sz Size) render.Rect { return render.Rect{X: 20 + sz.W - 40, Y: 20, W: 40, H: 20} }},
		{RevealSlideLeft, func(sz Size) render.Rect { return render.Rect{X: 20, Y: 20, W: 40, H: 20} }},
	} {
		r, leaf := collapsingAt(t, tc.tr)
		r.SetRevealed(true)
		advance(c, 50*time.Millisecond)
		sz := relayout(r)
		horizontal := tc.tr != RevealSlideUp
		if horizontal && (sz.H != 20 || sz.W < 12 || sz.W > 28) || !horizontal && (sz.W != 40 || sz.H < 6 || sz.H > 14) {
			t.Errorf("transition %d mid-slide measures %v", tc.tr, sz)
		}
		if want := tc.want(sz); leaf.bounds != want {
			t.Errorf("transition %d child at %v, want %v", tc.tr, leaf.bounds, want)
		}
		c.drive()
	}
}

func TestRevealerCollapseLeavesOtherTransitionsTheirSlot(t *testing.T) {
	pinAnimClock(t)
	for _, tr := range []RevealTransition{RevealFade, RevealZoom} {
		r, leaf := collapsingAt(t, tr)
		if sz := relayout(r); sz != (Size{W: 40, H: 20}) {
			t.Errorf("hidden collapsing transition %d measures %v, want the child held", tr, sz)
		}
		if leaf.bounds != (render.Rect{X: 20, Y: 20, W: 40, H: 20}) {
			t.Errorf("transition %d child at %v", tr, leaf.bounds)
		}
	}
	r, _ := collapsingAt(t, RevealSlideDown)
	r.SetCollapse(false)
	if r.Collapse() {
		t.Error("Collapse reports true after SetCollapse(false)")
	}
	if sz := relayout(r); sz != (Size{W: 40, H: 20}) {
		t.Errorf("an uncollapsed hidden slide measures %v, want the child held", sz)
	}
}

// Tab skips a hidden revealer's content and reaches a shown one's.
func TestHiddenRevealerContentTakesNoFocus(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	face := goldenFace(t)
	before := NewButton(NewLabel(face, 14, "before", 0), 4, 4)
	inner := NewButton(NewLabel(face, 14, "inner", 0), 4, 4)
	after := NewButton(NewLabel(face, 14, "after", 0), 4, 4)
	rv := NewRevealer(inner)
	root := NewBox(Column, 0, 0).Append(before, false).Append(rv, false).Append(after, false)
	arrangeTree(t, root, 200, 200)
	r := &Router{Root: root}
	r.SetFocus(before)
	r.FocusNext()
	if r.Focused() != Widget(after) {
		t.Fatal("Tab landed in a hidden revealer")
	}
	rv.SetRevealed(true)
	arrangeTree(t, root, 200, 200)
	r.SetFocus(before)
	r.FocusNext()
	if r.Focused() != Widget(inner) {
		t.Error("Tab skipped a shown revealer's content")
	}
}
