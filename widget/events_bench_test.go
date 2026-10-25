package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// Per-event input benchmarks over the showcase tree: the pointer-move
// hit walk, a wheel tick routed to the scroll, and a drag-hover walk.
// The allocation sweep (#77) profiles these per event - a gesture
// delivers tens of events per frame, so each allocation here is
// tens-of-times per frame work.

// BenchmarkPointerMove runs the hover hit walk per op, alternating the
// points the way a sweep does: list rows, the entry, empty padding.
func BenchmarkPointerMove(b *testing.B) {
	show := buildShowcase(b)
	show.root.Measure(Constraints{Max: Size{W: showW, H: showH}})
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	r := &Router{Root: show.root}
	pts := []Point{
		{X: 400, Y: 200},
		{X: 420, Y: 240},
		{X: 100, Y: 120},
		{X: 300, Y: 400},
		{X: 200, Y: 60},
		{X: 450, Y: 300},
	}
	b.ReportAllocs()
	for b.Loop() {
		r.Move(pts[b.N%len(pts)])
	}
}

// BenchmarkWheelOverScroll parks the pointer inside the scrolled list
// and routes one wheel tick per op - the event path behind `just axis`.
func BenchmarkWheelOverScroll(b *testing.B) {
	show := buildShowcase(b)
	show.root.Measure(Constraints{Max: Size{W: showW, H: showH}})
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	r := &Router{Root: show.root}
	r.Move(Point{X: 450, Y: 350})
	b.ReportAllocs()
	for b.Loop() {
		r.Axis(0, 40, 0)
	}
}

// BenchmarkDragHover runs the drag-hover walk per op over the same
// spread of points as the move benchmark.
func BenchmarkDragHover(b *testing.B) {
	show := buildShowcase(b)
	show.root.Measure(Constraints{Max: Size{W: showW, H: showH}})
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	r := &Router{Root: show.root}
	r.DragEnter(nil, Point{X: 400, Y: 200})
	pts := []Point{
		{X: 400, Y: 200},
		{X: 420, Y: 240},
		{X: 100, Y: 120},
		{X: 300, Y: 400},
		{X: 450, Y: 300},
	}
	b.ReportAllocs()
	for b.Loop() {
		r.DragHover(pts[b.N%len(pts)])
	}
}
