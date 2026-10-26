package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// The CSS benchmarks (docs/css.md, "Performance envelope"): full-tree
// match+compute over the showcase gallery, the single-widget restyle a
// class toggle costs, and the steady-frame paint with a stylesheet
// loaded — which must not move against BenchmarkShowcasePaint, the
// no-stylesheet counterpart. The budgets are bench/baseline.md's, with
// zero steady-state style allocations.

// galleryCSS is the benchmark stylesheet, the design doc's standard
// shape: ~25 rules over element/class/id/state selectors, descendant
// and child combinators, one id rule, one universal.
const galleryCSS = `
button { background-color: #111111; border-radius: 8; color: #ffffff; padding: 8; }
button:hover { background-color: #222222; }
button:active { background-color: #090909; }
button.destructive { background-color: #aa0000; }
button.primary { background-color: #0055bb; border-color: #0077ee; }
button:focus { border-width: 2; border-color: #55aaff; }
entry, textview { background-color: #0c0c0c; padding: 6; border-radius: 6; min-width: 120; }
entry:focus { border-width: 2; border-color: #55aaff; }
label { color: #cccccc; }
label.status { color: #888888; }
row { padding: 4; }
row:hover { background-color: #1a1a1a; }
box.window { background-color: #161616; padding: 12; }
box.column { padding: 8; }
box.column > button { padding: 10; }
box.row > .tool { background-color: #1e1e1e; }
slider { background-color: #333333; }
progressbar { border-radius: 4; background-color: #004488; }
switch { border-radius: 12; }
checkbutton { border-radius: 4; }
textview { font-size: 13; }
popover.menu { padding: 6; }
popover.menu:hover { background-color: #224466; }
#sidebar { background-color: #101014; }
* { font-weight: 400; }
`

// loadGalleryCSS installs the gallery stylesheet (widget/style_test.go)
// for the benchmark's lifetime.
func loadGalleryCSS(b *testing.B) {
	b.Helper()
	LoadStylesheet(galleryCSS)
	b.Cleanup(func() { LoadStylesheet("") })
}

// galleryWidgets counts the showcase tree for the per-widget metric.
func galleryWidgets(root Widget) int {
	n := 0
	walkTree(root, 0, func(Widget, int) { n++ })
	return n
}

// BenchmarkStyleGalleryFullRestyle measures the whole-tree
// match+compute one stylesheet load costs: the generation bump stamps
// every widget stale, and each recomputes (match, cascade, inherit,
// diff) exactly as the next frame's reads will.
func BenchmarkStyleGalleryFullRestyle(b *testing.B) {
	show := buildShowcase(b)
	loadGalleryCSS(b)
	con := Constraints{Max: Size{W: showW, H: showH}}
	show.root.Measure(con)
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	CollectDamage(show.root)
	widgets := galleryWidgets(show.root)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		styleGen++ // the load's stamp, without re-parsing the sheet
		walkTree(show.root, 0, func(w Widget, _ int) {
			if n := nodeOf(w); n != nil {
				n.style(w)
			}
		})
	}
	b.ReportMetric(float64(widgets), "widgets/op")
}

// BenchmarkStyleGalleryClassToggle measures one widget's restyle on a
// class toggle: the marks, the match+compute, and the diff — a list
// row label's steady-state .active flip, non-inherited, so the cost is
// the single widget (the subtree join is pinned by
// TestClassToggleInvalidatesSmallestSubtree).
func BenchmarkStyleGalleryClassToggle(b *testing.B) {
	show := buildShowcase(b)
	loadGalleryCSS(b)
	con := Constraints{Max: Size{W: showW, H: showH}}
	show.root.Measure(con)
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	CollectDamage(show.root)
	// A leaf label with a live ancestor chain: the gallery's most
	// numerous widget.
	var lbl *Label
	walkTree(show.root, 0, func(w Widget, _ int) {
		if l, ok := w.(*Label); ok && lbl == nil && nodeOf(w).parent != nil {
			lbl = l
		}
	})
	if lbl == nil {
		b.Fatal("no leaf label in the gallery")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lbl.RemoveClass("active")
		lbl.AddClass("active")
		lbl.style(lbl) // the next frame's first read: match+compute+diff
	}
}

// BenchmarkStyleGalleryPaint paints the styled gallery every op — the
// steady frame with all cascades warm. The number must sit at
// BenchmarkShowcasePaint's level, proving the paint path reads the
// computed struct instead of restyling.
func BenchmarkStyleGalleryPaint(b *testing.B) {
	show := buildShowcase(b)
	loadGalleryCSS(b)
	cv := render.New(make([]byte, render.Stride(showW)*showH), render.Stride(showW), showW, showH)
	stale := render.Rect{W: showW, H: showH}
	painted := 0
	for b.Loop() {
		painted, _ = paintFrame(cv, show.root, showW, showH, Current().Bg, stale)
	}
	b.ReportMetric(float64(painted), "px/frame")
}
