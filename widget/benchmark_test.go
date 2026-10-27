// Layout and paint benchmarks over the showcase tree - the lys-ui-hello
// window shape from damage_test.go. One benchmark per frame pass, all
// with fixed sizes and no time-dependent content, so numbers stay
// comparable across runs: CI archives them as an artifact for trend
// watching (go test -bench . -run '^$' -benchmem), none of it gates.
package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// BenchmarkShowcaseMeasure measures the cold layout pass: alternating
// the constrained width every op defeats the measure cache (cache hits
// key on the constraints), so each iteration re-measures the whole tree
// from scratch. The warm path is pinned by BenchmarkStaticTreeMeasure.
func BenchmarkShowcaseMeasure(b *testing.B) {
	show := buildShowcase(b)
	conA := Size{W: showW, H: showH}
	conB := Size{W: showW - 1, H: showH}
	b.ResetTimer()
	for i := range b.N {
		if i%2 == 0 {
			SizeWithin(show.root, conA)
		} else {
			SizeWithin(show.root, conB)
		}
	}
}

// BenchmarkShowcaseArrange measures the per-frame layout walk over the
// arranged tree: the same rect every op, so no damage is owed and the
// number is pure Arrange work over the cached Measure results.
func BenchmarkShowcaseArrange(b *testing.B) {
	show := buildShowcase(b)
	rect := render.Rect{X: 0, Y: 0, W: showW, H: showH}
	SizeWithin(show.root, Size{W: showW, H: showH})
	show.root.Arrange(rect)
	for b.Loop() {
		show.root.Arrange(rect)
	}
}

// BenchmarkShowcasePaint measures a full-window repaint: every frame is
// stale, so this is the upper bound the damage tracker works to stay
// under - BenchmarkProgressOnlyFrame is the incremental counterpart.
func BenchmarkShowcasePaint(b *testing.B) {
	show := buildShowcase(b)
	cv := render.New(make([]byte, render.Stride(showW)*showH), render.Stride(showW), showW, showH)
	stale := render.Rect{W: showW, H: showH}
	painted := 0
	for b.Loop() {
		painted, _ = paintFrame(cv, show.root, showW, showH, Current().Bg, stale)
	}
	b.ReportMetric(float64(painted), "px/frame")
	b.ReportMetric(100*float64(painted)/float64(showW*showH), "%-painted")
}

// BenchmarkEntryPaint repaints one entry with unchanged contents every
// op - the caret-blink steady state. Its shape, selection band, text,
// and caret all read the same (font, size, string), so after the first
// op the shaping cache serves every one of them; the glyph atlas serves
// the rasterization.
func BenchmarkEntryPaint(b *testing.B) {
	face := entryFace(b)
	e := NewEntry(face, 14, Current().Text)
	e.SetText("the quick brown fox jumps over the lazy dog")
	e.MoveHome()
	w, h := 320, 32
	e.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	cv := render.New(make([]byte, render.Stride(w)*h), render.Stride(w), w, h)
	b.ReportAllocs()
	for b.Loop() {
		cv.Clear(cv.Rect(), Current().Bg)
		e.Paint(cv)
	}
}
