package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/testbuild"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

const dropCSS = `
	button.segment { min-width: 24px; min-height: 24px; padding: 0px; background-color: transparent; }
	segments > indicator { background-color: #00ff00; border-radius: 9999px; }
	segments > indicator.faded { opacity: 0.5; }
`

type dropRig struct {
	t     *testing.T
	clock *animClock
	bar   *Bar
	segs  *SegmentIndicator
	drop  *SegmentDrop
	w, h  int
}

func newDropRig(t *testing.T, edge BarEdge, n int) *dropRig {
	t.Helper()
	old := Current()
	SetTheme(DarkTheme().WithAnimations(true))
	t.Cleanup(func() { SetTheme(old) })
	loadCSS(t, dropCSS)
	r := &dropRig{t: t, clock: pinAnimClock(t), bar: NewBar(edge), segs: NewSegmentIndicator(n), w: 400, h: 400}
	r.bar.Start().AppendAligned(r.segs, false, AlignCenter)
	r.drop = r.segs.EnableDrop()
	t.Cleanup(Host(r.bar))
	r.frame()
	return r
}

func (r *dropRig) frame() {
	SizeWithin(r.bar, Size{W: r.w, H: r.h})
	r.bar.Arrange(render.Rect{W: r.w, H: r.h})
	CollectDamage(r.bar)
}

func (r *dropRig) activate(i int) {
	r.t.Helper()
	r.segs.SetActive(i)
	r.frame()
}

func (r *dropRig) land() {
	r.t.Helper()
	for range 10000 {
		if !r.clock.step() {
			r.frame()
			r.settled()
			return
		}
		r.frame()
	}
	r.t.Fatal("the drop never settled")
}

func (r *dropRig) settled() {
	r.t.Helper()
	if r.drop.Moving() {
		r.t.Error("the drop still reports motion")
	}
	if animclock.Active() {
		r.t.Error("the settled drop keeps the clock running")
	}
	if _, ok := animclock.Next(animclock.Timer); ok {
		r.t.Error("the settled drop schedules a frame")
	}
}

func (r *dropRig) onSegment(i int) {
	r.t.Helper()
	if got, want := r.drop.Bounds(), r.segs.Segment(i).Bounds(); got != want {
		r.t.Errorf("drop at %v, want segment %d's box %v", got, i, want)
	}
}

func (r *dropRig) pixels() []byte {
	data := make([]byte, render.Stride(r.w)*r.h)
	r.bar.Paint(render.New(data, render.Stride(r.w), r.w, r.h))
	return data
}

func (r *dropRig) pixel(data []byte, x, y int) render.Color {
	return render.ColorFromBytes(data[y*render.Stride(r.w)+x*4:])
}

func TestDropRestsOnTheActiveSegmentFromTheFirstFrame(t *testing.T) {
	r := newDropRig(t, BarTop, 4)
	r.activate(2)
	r.onSegment(2)
	r.settled()
}

func TestDropStretchesTravelsAndSettlesOnTheTarget(t *testing.T) {
	for _, edge := range []BarEdge{BarTop, BarLeft} {
		t.Run(edge.class(), func(t *testing.T) {
			r := newDropRig(t, edge, 5)
			r.activate(0)
			rest := r.segs.Segment(0).Bounds()
			r.segs.SetActive(4)
			r.frame()
			if !r.drop.Moving() {
				t.Fatal("a switch did not start the drop")
			}
			along := func(b render.Rect) (pos, size, crossPos, crossSize int) {
				if edge.Vertical() {
					return b.Y, b.H, b.X, b.W
				}
				return b.X, b.W, b.Y, b.H
			}
			_, restSize, restCross, restCrossSize := along(rest)
			maxSize, last := 0, 0
			for r.clock.step() {
				r.frame()
				pos, size, cross, crossSize := along(r.drop.Bounds())
				if size < restSize {
					t.Fatalf("mid-flight the drop %v is shorter than a segment", r.drop.Bounds())
				}
				if cross != restCross || crossSize != restCrossSize {
					t.Fatalf("the drop %v left the axis of a segment %v", r.drop.Bounds(), rest)
				}
				if pos < last {
					t.Fatalf("the trailing edge went back from %d to %d", last, pos)
				}
				last, maxSize = pos, max(maxSize, size)
			}
			r.frame()
			if maxSize < restSize*2 {
				t.Errorf("the drop stretched to %d along the axis, a segment is %d", maxSize, restSize)
			}
			r.settled()
			r.onSegment(4)
		})
	}
}

func TestDropOvershootStaysInsideTheRow(t *testing.T) {
	r := newDropRig(t, BarTop, 3)
	r.activate(0)
	row := r.segs.Bounds()
	r.activate(2)
	for r.clock.step() {
		r.frame()
		if d := r.drop.Bounds(); d.X < row.X || d.X+d.W > row.X+row.W {
			t.Fatalf("the drop %v left the row %v", d, row)
		}
	}
}

func TestDropReversingMidFlightStaysOneShapeAndLands(t *testing.T) {
	r := newDropRig(t, BarTop, 6)
	r.activate(0)
	r.activate(5)
	for range 6 {
		r.clock.step()
		r.frame()
	}
	r.activate(1)
	for r.clock.step() {
		r.frame()
		if r.drop.Bounds().Empty() {
			t.Fatal("the drop collapsed when it turned round")
		}
	}
	r.frame()
	r.settled()
	r.onSegment(1)
}

func TestDropHiddenWithNothingActive(t *testing.T) {
	r := newDropRig(t, BarTop, 3)
	if !r.drop.Bounds().Empty() {
		t.Errorf("nothing active, the drop is at %v", r.drop.Bounds())
	}
	r.settled()
	r.activate(1)
	r.onSegment(1)
	r.settled()
	r.activate(-1)
	if !r.drop.Bounds().Empty() || r.segs.Active() != -1 {
		t.Errorf("SetActive(-1) left the drop at %v, active %d", r.drop.Bounds(), r.segs.Active())
	}
	r.settled()
	data := r.pixels()
	row := r.segs.Bounds()
	for x := row.X; x < row.X+row.W; x++ {
		if r.pixel(data, x, row.Y+row.H/2) == render.RGB(0, 0xff, 0) {
			t.Fatalf("with nothing active the drop painted at x=%d", x)
		}
	}
	r.activate(2)
	r.onSegment(2)
	r.settled()
}

func TestDropOutOfRangeActiveHidesIt(t *testing.T) {
	r := newDropRig(t, BarTop, 3)
	r.activate(1)
	r.activate(9)
	if !r.drop.Bounds().Empty() || r.segs.Active() != -1 {
		t.Errorf("an out-of-range active left the drop at %v, active %d", r.drop.Bounds(), r.segs.Active())
	}
	r.settled()
}

func TestDropGoesWhileMovingStopsTheClock(t *testing.T) {
	r := newDropRig(t, BarTop, 3)
	r.activate(0)
	r.activate(2)
	r.clock.step()
	r.activate(-1)
	if !r.drop.Bounds().Empty() {
		t.Errorf("the drop at %v after none became active mid-flight", r.drop.Bounds())
	}
	r.settled()
}

func TestDropDoesNotAnimateARebuildWithTheSameActive(t *testing.T) {
	face := goldenFace(t)
	r := newDropRig(t, BarTop, 3)
	r.activate(1)
	r.segs.SetCount(3)
	r.segs.SetSegmentContent(1, NewLabel(face, 12, "1", 0))
	r.segs.SetActive(1)
	r.frame()
	r.settled()
	r.onSegment(1)
}

func TestDropFollowsASegmentThatGrewWithoutAnimating(t *testing.T) {
	face := goldenFace(t)
	r := newDropRig(t, BarTop, 3)
	r.activate(2)
	r.segs.SetSegmentContent(0, NewLabel(face, 14, "a long workspace name", 0))
	r.frame()
	r.frame()
	r.settled()
	r.onSegment(2)
}

func TestDropStaysOnAnItemWhoseIndexShifted(t *testing.T) {
	r := newDropRig(t, BarTop, 4)
	r.activate(3)
	r.segs.SetCount(3)
	r.segs.ShiftActive(2)
	r.frame()
	r.settled()
	r.onSegment(2)
}

func TestDropRetargetsWhenTheItemShiftsMidFlight(t *testing.T) {
	r := newDropRig(t, BarTop, 5)
	r.activate(0)
	r.activate(4)
	for range 4 {
		r.clock.step()
		r.frame()
	}
	r.segs.SetCount(4)
	r.segs.ShiftActive(3)
	r.frame()
	if !r.drop.Moving() {
		t.Error("the flight stopped dead when its target's index shifted")
	}
	r.land()
	r.onSegment(3)
}

func TestDropTargetRemovedMidFlightLandsOnTheNewActive(t *testing.T) {
	r := newDropRig(t, BarTop, 5)
	r.activate(0)
	r.activate(4)
	r.clock.step()
	r.frame()
	r.segs.SetCount(3)
	r.segs.SetActive(1)
	r.frame()
	r.land()
	r.onSegment(1)
}

func TestDropTargetRemovedWithNothingActiveHidesIt(t *testing.T) {
	r := newDropRig(t, BarTop, 5)
	r.activate(0)
	r.activate(4)
	r.clock.step()
	r.frame()
	r.segs.SetCount(2)
	r.frame()
	if !r.drop.Bounds().Empty() || r.segs.Active() != -1 {
		t.Errorf("the drop at %v after its target went, active %d", r.drop.Bounds(), r.segs.Active())
	}
	r.settled()
}

func TestDropLandsAtOnceWithoutMotion(t *testing.T) {
	r := newDropRig(t, BarTop, 4)
	r.activate(0)
	SetReducedMotion(true)
	t.Cleanup(func() { SetReducedMotion(false) })
	r.activate(3)
	r.settled()
	r.onSegment(3)
	SetReducedMotion(false)

	SetTheme(DarkTheme().WithAnimations(false))
	r.activate(1)
	r.settled()
	r.onSegment(1)
	SetTheme(DarkTheme().WithAnimations(true))

	defer SetAnimationsInstant(true)()
	r.activate(2)
	r.settled()
	r.onSegment(2)
}

func TestSettledDropDoesNoWork(t *testing.T) {
	r := newDropRig(t, BarTop, 5)
	r.activate(1)
	r.activate(3)
	r.land()
	r.frame()
	before := r.segs.arrangeCount()
	r.bar.Arrange(render.Rect{W: r.w, H: r.h})
	if n := r.segs.arrangeCount() - before; n != 0 {
		t.Errorf("the settled row was re-arranged %d times", n)
	}
	if rects, _ := CollectDamage(r.bar); len(rects) != 0 {
		t.Errorf("a settled drop owes damage %v", rects)
	}
	cv := render.New(make([]byte, render.Stride(r.w)*r.h), render.Stride(r.w), r.w, r.h)
	r.segs.Paint(cv)
	t.Run("allocations", func(t *testing.T) {
		testbuild.SkipAllocBudget(t)
		if allocs := testing.AllocsPerRun(50, func() {
			r.drop.follow(r.segs.Segment(3))
			r.segs.Paint(cv)
		}); allocs != 0 {
			t.Errorf("a settled drop allocates %v per frame", allocs)
		}
	})
	r.settled()
}

func TestRowWithoutDropKeepsNoAnimationState(t *testing.T) {
	pinAnimClock(t)
	segs := NewSegmentIndicator(3)
	row := NewBox(Row, 0, 0).Append(segs, false)
	arrangeTree(t, row, 200, 40)
	segs.SetActive(2)
	arrangeTree(t, row, 200, 40)
	if segs.drop != nil || animclock.Active() {
		t.Errorf("drop %v, clock active %v, on a row that never enabled one", segs.drop, animclock.Active())
	}
}

func TestDropPaintsBehindTheSegmentsFromItsStylesheet(t *testing.T) {
	r := newDropRig(t, BarTop, 2)
	loadCSS(t, dropCSS+` button.segment.active { background-image: linear-gradient(#ff0000, #ff0000); background-size: 4px 4px; background-repeat: no-repeat; background-position: center; }`)
	r.activate(0)
	r.frame()
	data := r.pixels()
	seg := r.segs.Segment(0).Bounds()
	if got := r.pixel(data, seg.X+seg.W/2, seg.Y+seg.H/2); got != render.RGB(0xff, 0, 0) {
		t.Errorf("the segment's own paint is %v, want it over the drop", got)
	}
	if got := r.pixel(data, seg.X+3, seg.Y+seg.H/2); got != render.RGB(0, 0xff, 0) {
		t.Errorf("the drop paints %v beside the segment's content", got)
	}
	if got := r.pixel(data, seg.X, seg.Y); got == render.RGB(0, 0xff, 0) {
		t.Error("the drop's corner is square, its radius did not apply")
	}
	other := r.segs.Segment(1).Bounds()
	if got := r.pixel(data, other.X+other.W/2, other.Y+other.H/2); got == render.RGB(0, 0xff, 0) {
		t.Error("the inactive segment shows the drop")
	}
}

func TestDropTakesInlineStyleAndEffects(t *testing.T) {
	r := newDropRig(t, BarTop, 2)
	r.drop.SetInlineStyle("background-color: #0000ff;")
	r.activate(0)
	seg := r.segs.Segment(0).Bounds()
	data := r.pixels()
	if got := r.pixel(data, seg.X+seg.W/2, seg.Y+seg.H/2); got != render.RGB(0, 0, 0xff) {
		t.Errorf("the tinted drop paints %v", got)
	}
	r.drop.SetInlineStyle("")
	r.drop.AddClass("faded")
	data = r.pixels()
	if got := r.pixel(data, seg.X+seg.W/2, seg.Y+seg.H/2); got == render.RGB(0, 0xff, 0) {
		t.Error("the faded drop paints at full strength")
	}
}

func TestDropFollowsTheRowWhenTheBarMovesIt(t *testing.T) {
	face := goldenFace(t)
	r := newDropRig(t, BarTop, 3)
	label := NewLabel(face, 14, "x", 0)
	r.bar.Start().InsertAt(0, label, false)
	r.frame()
	r.activate(1)
	r.frame()
	r.onSegment(1)
	label.SetText("a much wider label than before")
	r.frame()
	r.frame()
	r.settled()
	r.onSegment(1)
}

func TestDropTravelSurvivesARebuildBeforeTheNextFrame(t *testing.T) {
	r := newDropRig(t, BarTop, 4)
	r.activate(0)
	r.segs.SetActive(3)
	r.segs.ShiftActive(3)
	r.frame()
	if !r.drop.Moving() {
		t.Fatal("a second rebuild of the same workspace cancelled the travel")
	}
	r.land()
	r.onSegment(3)
}

func TestDropTravelSurvivesAnIndexShiftBeforeTheNextFrame(t *testing.T) {
	r := newDropRig(t, BarTop, 4)
	r.activate(0)
	r.segs.SetActive(3)
	r.segs.SetCount(3)
	r.segs.SetActive(2)
	r.segs.ShiftActive(2)
	r.frame()
	if !r.drop.Moving() {
		t.Fatal("the shift cancelled the travel")
	}
	r.land()
	r.onSegment(2)
}

func TestDropStaysVisibleWhenTheRowShrinksPastIt(t *testing.T) {
	r := newDropRig(t, BarTop, 4)
	r.activate(3)
	r.segs.SetCount(2)
	r.segs.SetActive(0)
	r.frame()
	row := r.segs.Bounds()
	check := func() {
		t.Helper()
		if d := r.drop.Bounds(); d.Empty() || d.X < row.X || d.X+d.W > row.X+row.W {
			t.Fatalf("the drop at %v in the row %v", d, row)
		}
	}
	check()
	for r.clock.step() {
		r.frame()
		check()
	}
	r.frame()
	r.settled()
	r.onSegment(0)
}

func TestDropWithOneSegmentLeftLandsOnIt(t *testing.T) {
	r := newDropRig(t, BarTop, 2)
	r.activate(1)
	r.segs.SetCount(1)
	r.segs.SetActive(0)
	r.frame()
	r.land()
	r.onSegment(0)
	if d := r.drop.Bounds(); d.Empty() {
		t.Error("the drop vanished")
	}
}
