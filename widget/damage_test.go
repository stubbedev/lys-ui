// Damage-tracking and measure-cache tests: every pin lives here that
// guards the partial-repaint pipeline - invalidations produce the right
// rects, a static tree costs zero Measure recursion, and a frame with
// only the progress bar animating paints a tiny fraction of the window.
package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// warmDamage drains a fresh tree's theme stamps, mirroring the first
// full frame the app always paints into a fresh buffer.
func warmDamage(root Widget) {
	CollectDamage(root)
}

// paintFrame drives one frame the way app's hostWindow.draw does:
// cached measure, arrange, damage drain, region-clipped paint. It
// returns the pixels actually written and the damage rects.
func paintFrame(cv *render.Canvas, root Widget, w, h int, bg render.Color, stale render.Rect) (int, []render.Rect) {
	root.Measure(Constraints{Max: Size{W: w, H: h}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	rects, _ := CollectDamage(root)
	if !stale.Empty() {
		rects = append(rects, stale)
	}
	region := render.UnionAll(rects)
	prev := cv.PushClip(region)
	cv.Clear(region, bg)
	root.Paint(cv)
	cv.PopClip(prev)
	painted := cv.Touched()
	cv.ResetTouched()
	return painted, rects
}

// measureEntries sums the Measure invocations over the given widgets,
// excluding the tree root itself: the app calls root.Measure once per
// frame unconditionally, and the pin is that nothing below it recurses.
func measureEntries(ws []Widget) int {
	total := 0
	for i, w := range ws {
		if i == 0 {
			continue
		}
		if n, ok := w.(interface{ measureCalls() int }); ok {
			total += n.measureCalls()
		}
	}
	return total
}

func TestCollectDamage(t *testing.T) {
	face := entryFace(t)

	t.Run("SetText damages exactly the label bounds once", func(t *testing.T) {
		lbl := NewLabel(face, 14, "before", Current().Text)
		root := NewBox(Column, 4, 0)
		root.Append(lbl, false)
		root.Measure(Constraints{Max: Size{W: 400, H: 300}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 300})
		warmDamage(root)

		lbl.SetText("after")
		rects, any := CollectDamage(root)
		if !any {
			t.Fatal("SetText produced no damage")
		}
		got := render.UnionAll(rects)
		if got != lbl.Bounds() {
			t.Errorf("damage union %+v, want the label bounds %+v", got, lbl.Bounds())
		}
		if _, any := CollectDamage(root); any {
			t.Error("damage flags were not drained by the collect")
		}
	})

	t.Run("no mutation means no damage", func(t *testing.T) {
		lbl := NewLabel(face, 14, "stable", Current().Text)
		root := NewBox(Row, 0, 0)
		root.Append(lbl, false)
		root.Measure(Constraints{Max: Size{W: 100, H: 100}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
		warmDamage(root)
		if _, any := CollectDamage(root); any {
			t.Error("untouched tree reported damage")
		}
	})

	t.Run("InvalidateRect damages just the explicit rect", func(t *testing.T) {
		sp := NewSpacer(10, 10)
		sp.Arrange(render.Rect{X: 5, Y: 5, W: 10, H: 10})
		warmDamage(sp)
		sp.InvalidateRect(render.Rect{X: 100, Y: 200, W: 3, H: 4})
		rects, any := CollectDamage(sp)
		if !any {
			t.Fatal("InvalidateRect produced no damage")
		}
		got := render.UnionAll(rects)
		want := render.Rect{X: 100, Y: 200, W: 3, H: 4}
		if got != want {
			t.Errorf("union %+v, want the extra rect %+v alone (bounds untouched)", got, want)
		}
	})

	t.Run("a moved widget damages its old and new rect", func(t *testing.T) {
		sp := NewSpacer(10, 10)
		sp.Arrange(render.Rect{X: 0, Y: 0, W: 10, H: 10})
		warmDamage(sp)
		sp.Arrange(render.Rect{X: 50, Y: 0, W: 10, H: 10})
		rects, any := CollectDamage(sp)
		if !any {
			t.Fatal("move produced no damage")
		}
		got := render.UnionAll(rects)
		want := render.Rect{X: 0, Y: 0, W: 60, H: 10}
		if got != want {
			t.Errorf("move damage union %+v, want old ∪ new %+v", got, want)
		}
	})

	t.Run("an unchanged arrange is free", func(t *testing.T) {
		sp := NewSpacer(10, 10)
		sp.Arrange(render.Rect{X: 1, Y: 2, W: 10, H: 10})
		warmDamage(sp)
		sp.Arrange(render.Rect{X: 1, Y: 2, W: 10, H: 10})
		if _, any := CollectDamage(sp); any {
			t.Error("identical arrange reported damage")
		}
	})

	t.Run("a mutation on a hidden notebook page waits for the page flip", func(t *testing.T) {
		p0 := NewBox(Column, 0, 0)
		hidden := NewLabel(face, 12, "page one", Current().Text)
		p0.Append(hidden, false)
		nb := NewNotebook(face)
		nb.AppendTab("a", p0)
		nb.AppendTab("b", NewSpacer(10, 10))
		nb.Measure(Constraints{Max: Size{W: 300, H: 200}})
		nb.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})
		warmDamage(nb)

		// While page b shows, editing page a changes nothing on screen.
		nb.SelectTab("b")
		warmDamage(nb)
		hidden.SetText("page one edited")
		if _, any := CollectDamage(nb); any {
			t.Error("hidden-page mutation produced damage")
		}

		// Selecting the page back repaints it with the new text.
		nb.SelectTab("a")
		rects, any := CollectDamage(nb)
		if !any {
			t.Fatal("page flip produced no damage")
		}
		if got := render.UnionAll(rects); got != nb.Bounds() {
			t.Errorf("damage union %+v, want the notebook bounds %+v", got, nb.Bounds())
		}
	})
}

func TestMutationDamageRects(t *testing.T) {
	face := entryFace(t)

	arranged := func(w Widget, x, y, wd, ht int) {
		t.Helper()
		w.Measure(Constraints{Max: Size{W: 640, H: 470}})
		w.Arrange(render.Rect{X: x, Y: y, W: wd, H: ht})
		warmDamage(w)
	}

	t.Run("slider drag damages the slider bounds", func(t *testing.T) {
		s := NewSlider(0, 100, 0, 0)
		arranged(s, 10, 20, 200, 18)
		s.SetValue(50)
		rects, any := CollectDamage(s)
		if !any {
			t.Fatal("SetValue produced no damage")
		}
		if got := render.UnionAll(rects); got != s.Bounds() {
			t.Errorf("damage %+v, want slider bounds %+v", got, s.Bounds())
		}
	})

	t.Run("progress tween damages the bar bounds", func(t *testing.T) {
		p := NewProgressBar(0)
		arranged(p, 0, 0, 160, 10)
		p.SetValue(0.5)
		rects, any := CollectDamage(p)
		if !any {
			t.Fatal("progress SetValue produced no damage")
		}
		if got := render.UnionAll(rects); got != p.Bounds() {
			t.Errorf("damage %+v, want bar bounds %+v", got, p.Bounds())
		}
	})

	t.Run("switch and checkbox state damage their bounds", func(t *testing.T) {
		sw := NewSwitch(false)
		arranged(sw, 0, 0, 40, 22)
		sw.SetOn(true)
		if rects, any := CollectDamage(sw); !any {
			t.Fatal("SetOn produced no damage")
		} else if got := render.UnionAll(rects); got != sw.Bounds() {
			t.Errorf("damage %+v, want switch bounds", got)
		}

		cb := NewCheckButton(false)
		arranged(cb, 0, 0, 20, 20)
		cb.SetChecked(true)
		if rects, any := CollectDamage(cb); !any {
			t.Fatal("SetChecked produced no damage")
		} else if got := render.UnionAll(rects); got != cb.Bounds() {
			t.Errorf("damage %+v, want checkbox bounds", got)
		}
	})

	t.Run("caret move and selection damage the entry bounds", func(t *testing.T) {
		e := NewEntry(face, 14, Current().Text)
		e.SetText("caret selection")
		arranged(e, 8, 8, 200, 28)

		e.MoveCursor(3)
		rects, any := CollectDamage(e)
		if !any {
			t.Fatal("caret move produced no damage")
		}
		if got := render.UnionAll(rects); got != e.Bounds() {
			t.Errorf("caret damage %+v, want entry bounds %+v", got, e.Bounds())
		}

		e.KeyAction(KeyRight, ModShift)
		rects, any = CollectDamage(e)
		if !any {
			t.Fatal("selection growth produced no damage")
		}
		if got := render.UnionAll(rects); got != e.Bounds() {
			t.Errorf("selection damage %+v, want entry bounds %+v", got, e.Bounds())
		}
	})

	t.Run("text area edits damage the area bounds", func(t *testing.T) {
		a := NewTextArea(face, 13, Current().Text)
		a.SetText("line one\nline two")
		arranged(a, 0, 0, 300, 80)

		a.InsertRune('x')
		rects, any := CollectDamage(a)
		if !any {
			t.Fatal("text area edit produced no damage")
		}
		if got := render.UnionAll(rects); got != a.Bounds() {
			t.Errorf("damage %+v, want area bounds %+v", got, a.Bounds())
		}
	})
}

func TestThemeChangeDamagesTree(t *testing.T) {
	face := entryFace(t)
	lbl := NewLabel(face, 14, "themed", Current().Text)
	root := NewBox(Column, 0, 0)
	root.Append(lbl, false)
	root.Measure(Constraints{Max: Size{W: 200, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 100})
	warmDamage(root)

	SetTheme(Current())
	rects, any := CollectDamage(root)
	if !any {
		t.Fatal("SetTheme produced no damage")
	}
	if got := render.UnionAll(rects); got != root.Bounds() {
		t.Errorf("theme damage %+v, want the root bounds %+v", got, root.Bounds())
	}
	if _, any := CollectDamage(root); any {
		t.Error("theme damage did not drain")
	}
}

// showcase builds a tree shaped like the lys-ui-hello window: header,
// controls, text fields, and a 48-row scrollable list.
type showcase struct {
	root     Widget
	progress *ProgressBar
	widgets  []Widget
}

func buildShowcase(tb testing.TB) showcase {
	tb.Helper()
	face := entryFace(tb)
	th := Current()
	var all []Widget
	keep := func(ws ...Widget) {
		all = append(all, ws...)
	}

	status := NewLabel(face, 12, "events land here", th.TextMuted)
	countLabel := NewLabel(face, 15, "clicked 0 times", th.Text)
	progress := NewProgressBar(0)
	button := NewButton(
		NewBox(Row, 8, 0).Append(NewLabel(face, 15, "click me", th.Text), false), 10, 8)
	slider := NewSlider(0, 1, 0.05, 0)
	sw := NewSwitch(true)
	swLabel := NewLabel(face, 14, "notifications", th.Text)
	check := NewCheckButton(false)
	checkLabel := NewLabel(face, 14, "remember me", th.Text)
	entry := NewEntry(face, 14, th.Text)
	entry.SetPlaceholder("type here")
	area := NewTextArea(face, 13, th.Text)
	area.SetText("multi-line text area:\nselection spans lines.")

	list := NewBox(Column, 4, 0)
	for i := 1; i <= 48; i++ {
		list.Append(NewLabel(face, 12, "server-00.example   up   41ms", th.Text), false)
	}
	scrolled := NewScroll(list)
	scrolled.ShowBars = true

	keep(status, countLabel, progress, button, slider, sw, swLabel, check, checkLabel, entry, area, list, scrolled)
	for _, l := range list.Children() {
		keep(l)
	}

	header := NewBox(Column, 2, 0)
	header.Append(NewLabel(face, 18, "lys-ui showcase", th.Accent), false)
	header.Append(NewLabel(face, 11, "every widget in one window", th.TextMuted), false)

	left := NewBox(Column, 10, 0)
	left.Append(button, false)
	left.Append(countLabel, false)
	left.Append(progress, false)
	left.Append(slider, false)
	left.Append(NewBox(Row, 8, 0).Append(swLabel, false).Append(sw, false), false)
	left.Append(NewBox(Row, 8, 0).Append(check, false).Append(checkLabel, false), false)

	right := NewBox(Column, 6, 0)
	right.Append(NewLabel(face, 11, "entry", th.TextMuted), false)
	right.Append(entry, false)
	right.Append(NewLabel(face, 11, "text area", th.TextMuted), false)
	right.Append(area, false)
	right.Append(NewLabel(face, 11, "list", th.TextMuted), false)
	right.Append(scrolled, true)

	columns := NewBox(Row, 24, 0)
	columns.Append(left, true)
	columns.Append(right, true)

	root := NewBox(Column, 12, 16)
	root.Append(header, false)
	root.Append(columns, true)
	root.Append(status, false)
	// Root first: measureEntries skips index 0, the one unconditional
	// root.Measure call the frame loop makes.
	all = append([]Widget{root}, all...)

	return showcase{root: root, progress: progress, widgets: all}
}

// showWindow is the showcase window size.
const showW, showH = 640, 470

func TestProgressOnlyFramePaintsBelowFivePercent(t *testing.T) {
	show := buildShowcase(t)
	cv := render.New(make([]byte, render.Stride(showW)*showH), render.Stride(showW), showW, showH)

	// Frame one: fresh buffer, fully stale, full repaint.
	painted, rects := paintFrame(cv, show.root, showW, showH, Current().Bg, render.Rect{W: showW, H: showH})
	if area := widgetArea(rects, showW, showH); area != showW*showH {
		t.Fatalf("first frame damaged %d px, want the full %d", area, showW*showH)
	}
	if painted == 0 {
		t.Fatal("first frame painted nothing")
	}

	// Animate: only the progress bar's value changes per frame.
	for i, v := range []float64{0.25, 0.5, 0.75} {
		show.progress.SetValue(v)
		painted, rects = paintFrame(cv, show.root, showW, showH, Current().Bg, render.Rect{})
		area := render.UnionAll(rects)
		if area != show.progress.Bounds() {
			t.Errorf("frame %d damaged %+v, want only the progress bounds %+v", i+2, area, show.progress.Bounds())
		}
		if ratio := float64(painted) / float64(showW*showH); ratio >= 0.05 {
			t.Errorf("frame %d painted %d px (%.1f%%), want < 5%%", i+2, painted, ratio*100)
		}
	}
}

func TestStaticTreeZeroMeasureRecursion(t *testing.T) {
	show := buildShowcase(t)
	con := Constraints{Max: Size{W: showW, H: showH}}

	show.root.Measure(con)
	// Layout pass on a static tree: bounds settle, nothing moves. The
	// first arrange parents the tree, which owes one more pass, as a
	// window's draw runs before painting.
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	for LayoutPending(show.root) {
		show.root.Measure(con)
		show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	}
	CollectDamage(show.root)
	baseline := measureEntries(show.widgets)

	// Frame two on the same tree: the root cache hit must short-circuit
	// the whole recursion.
	show.root.Measure(con)
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	if got := measureEntries(show.widgets) - baseline; got != 0 {
		t.Fatalf("static tree needed %d Measure calls on the second frame, want 0", got)
	}

	// A label change drops the cache along its ancestor chain; the
	// remeasure frame may recurse into that branch, and the frame after
	// converges back to zero.
	for _, w := range show.widgets {
		if l, ok := w.(*Label); ok && l.Text() == "clicked 0 times" {
			l.SetText("clicked 1 time")
			break
		}
	}
	show.root.Measure(con)
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: showW, H: showH})
	baseline = measureEntries(show.widgets)
	show.root.Measure(con)
	if got := measureEntries(show.widgets) - baseline; got != 0 {
		t.Fatalf("after the remeasure frame, %d Measure calls remained, want 0", got)
	}
}

func widgetArea(rects []render.Rect, w, h int) int {
	return render.UnionAll(rects).Intersect(render.Rect{W: w, H: h}).W *
		render.UnionAll(rects).Intersect(render.Rect{W: w, H: h}).H
}

func BenchmarkProgressOnlyFrame(b *testing.B) {
	show := buildShowcase(b)
	cv := render.New(make([]byte, render.Stride(showW)*showH), render.Stride(showW), showW, showH)
	paintFrame(cv, show.root, showW, showH, Current().Bg, render.Rect{W: showW, H: showH})
	painted := 0
	b.ResetTimer()
	for i := range b.N {
		show.progress.SetValue(float64(i%100) / 100)
		painted, _ = paintFrame(cv, show.root, showW, showH, Current().Bg, render.Rect{})
	}
	b.ReportMetric(float64(painted), "px/frame")
	b.ReportMetric(100*float64(painted)/float64(showW*showH), "%-painted")
}

func BenchmarkStaticTreeMeasure(b *testing.B) {
	show := buildShowcase(b)
	con := Constraints{Max: Size{W: showW, H: showH}}
	show.root.Measure(con)
	baseline := measureEntries(show.widgets)
	b.ResetTimer()
	for range b.N {
		show.root.Measure(con)
	}
	b.StopTimer()
	b.ReportMetric(float64(measureEntries(show.widgets)-baseline)/float64(b.N), "Measure-calls/op")
}
