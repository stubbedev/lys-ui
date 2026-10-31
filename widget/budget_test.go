// The allocation budgets (#77): hard ceilings derived from the measured
// baseline in bench/baseline.md, gated here so a regression fails `just
// check`. Each budget carries 1.2x headroom over the baseline number,
// not invented room: a fix that lowers the baseline should tighten it
// here in the same commit.
package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"
)

// TestAllocBudgetGalleryFrame pins the steady frame: one full
// measure+arrange+damage+paint pass over the showcase gallery. The
// baseline measured 24 allocs/op (bench/baseline.md); after the #120
// sweep the frame measures 1 alloc/op (the harness's own rect append),
// so the budget is ceil(1*1.2).
func TestAllocBudgetGalleryFrame(t *testing.T) {
	skipAllocBudget(t)
	show := buildShowcase(t)
	cv := render.New(make([]byte, render.Stride(showW)*showH), render.Stride(showW), showW, showH)
	stale := render.Rect{W: showW, H: showH}
	const budget = 2
	paintFrame(cv, show.root, showW, showH, Current().Bg, stale) // warm the shape and damage paths
	allocs := testing.AllocsPerRun(50, func() {
		paintFrame(cv, show.root, showW, showH, Current().Bg, stale)
	})
	if allocs > budget {
		t.Errorf("gallery frame allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetStyleFullRestyle pins the whole-tree cascade rebuild:
// every widget restyled under a fresh generation through the per-depth
// snapshot buffers (walkTree, styleKids), matching the
// StyleGalleryFullRestyle benchmark's 0 allocs/op.
func TestAllocBudgetStyleFullRestyle(t *testing.T) {
	skipAllocBudget(t)
	show := buildShowcase(t)
	LoadStylesheet(galleryCSS)
	t.Cleanup(func() { LoadStylesheet("") })
	SizeWithin(show.root, Size{W: showW, H: showH})
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	CollectDamage(show.root)
	restyle := func() {
		styleGen++
		walkTree(show.root, 0, func(w Widget, _ int) {
			if n := nodeOf(w); n != nil {
				n.style(w)
			}
		})
	}
	restyle()
	const budget = 0
	allocs := testing.AllocsPerRun(50, restyle)
	if allocs > budget {
		t.Errorf("full restyle allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetSiblingRestyle pins the sibling-selector restyle: a
// class flip on one row of a wide sibling set restyles the whole set,
// with the filtered sibling list memoized on the parent per damage
// generation instead of rebuilt per restyled widget.
func TestAllocBudgetSiblingRestyle(t *testing.T) {
	skipAllocBudget(t)
	LoadStylesheet(`spacer + spacer { min-width: 7px; } spacer:first-child { min-height: 5px; }`)
	t.Cleanup(func() { LoadStylesheet("") })
	row := NewBox(Row, 0, 0)
	spacers := make([]*Spacer, 24)
	for i := range spacers {
		spacers[i] = NewSpacer(4, 4)
		spacers[i].SetElement("spacer")
		row.Append(spacers[i], false)
	}
	arrangeTree(t, row, 400, 30)
	CollectDamage(row)
	on := false
	toggle := func() {
		on = !on
		spacers[12].SetVisible(on)
		walkTree(row, 0, func(w Widget, _ int) {
			if n := nodeOf(w); n != nil {
				n.style(w)
			}
		})
	}
	toggle()
	const budget = 0
	allocs := testing.AllocsPerRun(50, toggle)
	if allocs > budget {
		t.Errorf("sibling restyle allocates %v allocs/op, budget %d", allocs, budget)
	}
}

// TestAllocBudgetComposingDisplay pins the input-method composing
// reads: while a preedit is on show, the entry's display runes and
// string are memoized across the several reads a frame makes, instead
// of spliced and converted per read.
func TestAllocBudgetComposingDisplay(t *testing.T) {
	skipAllocBudget(t)
	e := NewEntry(entryFace(t), 14, 0)
	e.Insert("hello")
	arrangeTree(t, e, 200, 32)
	e.IMEPreedit("世界", 0, 6)
	read := func() {
		if len(e.displayRunes()) == 0 || e.displayText() == "" {
			panic("empty composing display")
		}
	}
	read()
	const budget = 0
	allocs := testing.AllocsPerRun(100, read)
	if allocs > budget {
		t.Errorf("composing display allocates %v allocs/read, budget %d", allocs, budget)
	}
}

// skipAllocBudget skips an allocation budget under -race, whose
// instrumentation allocates, and under the lysdebug tag, whose traces
// do.
func skipAllocBudget(t *testing.T) {
	t.Helper()
	if raceEnabled || debug.Enabled {
		t.Skip("allocation budgets do not hold under -race or lysdebug")
	}
}

func TestAllocBudgetWrappedLabelRepaint(t *testing.T) {
	skipAllocBudget(t)
	const w, h = 1605, 1391
	root := settingsTree(t)
	cv := render.New(make([]byte, render.Stride(w)*h), render.Stride(w), w, h)
	settingsFrame(root, cv, w, h)
	const budget = 0
	allocs := testing.AllocsPerRun(10, func() { root.Paint(cv) })
	if allocs > budget {
		t.Errorf("repainting wrapped labels allocates %v allocs/op, budget %d", allocs, budget)
	}
}
