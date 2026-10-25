package widget

import (
	"reflect"
	"testing"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/render"
)

// setupMultiList builds a 20-row multiple-mode list in a 100x100
// viewport (5 rows of 20px), painted so the row proxies are cached,
// with the selection callback recorded.
func setupMultiList(t *testing.T) (*List, *countingModel, *[]([]int)) {
	t.Helper()
	model := newCountingModel(20)
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionMultiple)
	var got []([]int)
	l.OnSelectionChanged = func(rows []int) {
		got = append(got, rows)
	}
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
	cv := render.New(make([]uint8, 100*100*4), 100*4, 100, 100)
	l.Paint(cv)
	return l, model, &got
}

// selEquals fails unless the list's selection is exactly want.
func selEquals(t *testing.T, l *List, want ...int) {
	t.Helper()
	got := l.Selection()
	if len(got) != len(want) {
		t.Fatalf("selection = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("selection = %v, want %v", got, want)
		}
	}
}

// TestListMultipleKeyboard pins the multiple-mode keyboard model:
// plain motion collapses to the cursor row, shift extends from the
// anchor, ctrl moves the cursor without selecting, ctrl+space toggles
// the cursor row, and ctrl+a (SelectAll) takes everything — each
// gesture reporting the full set exactly once.
func TestListMultipleKeyboard(t *testing.T) {
	l, _, notifications := setupMultiList(t)

	t.Run("plain arrows collapse to the cursor row", func(t *testing.T) {
		l.KeyAction(KeyDown, 0)
		l.KeyAction(KeyDown, 0)
		selEquals(t, l, 1)
		if n := len(*notifications); n != 2 {
			t.Errorf("notifications = %d after two plain arrows, want 2", n)
		}
	})

	t.Run("shift extends from the anchor", func(t *testing.T) {
		l.Select(2)
		*notifications = nil
		l.KeyAction(KeyDown, ModShift)
		l.KeyAction(KeyDown, ModShift)
		selEquals(t, l, 2, 3, 4)
		if n := len(*notifications); n != 2 {
			t.Errorf("notifications = %d for two extensions, want 2", n)
		}
		last := (*notifications)[len(*notifications)-1]
		if !reflect.DeepEqual(last, []int{2, 3, 4}) {
			t.Errorf("last notification = %v, want the full set [2 3 4]", last)
		}
	})

	t.Run("ctrl moves the cursor without selecting", func(t *testing.T) {
		l.Select(2)
		*notifications = nil
		l.KeyAction(KeyDown, ModCtrl)
		l.KeyAction(KeyDown, ModCtrl)
		selEquals(t, l, 2)
		if len(*notifications) != 0 {
			t.Errorf("cursor motion notified %d times, want 0", len(*notifications))
		}
		// The cursor moved to row 4: ctrl+space toggles it.
		l.KeyAction(KeySpace, ModCtrl)
		selEquals(t, l, 2, 4)
		if len(*notifications) != 1 {
			t.Errorf("ctrl+space notified %d times, want 1", len(*notifications))
		}
	})

	t.Run("home and end join the model", func(t *testing.T) {
		l.Select(2)
		l.KeyAction(KeyEnd, ModShift)
		selEquals(t, l, rangeOf(2, 20)...)
		l.KeyAction(KeyHome, 0)
		selEquals(t, l, 0)
	})

	t.Run("select all through the ctrl+a target", func(t *testing.T) {
		*notifications = nil
		l.SelectAll()
		if got := len(l.Selection()); got != 20 {
			t.Errorf("SelectAll left %d rows, want 20", got)
		}
		if len(*notifications) != 1 {
			t.Errorf("SelectAll notified %d times, want 1", len(*notifications))
		}
	})

	t.Run("enter activates every selected row once", func(t *testing.T) {
		var activated []int
		l.OnActivate = func(i int) { activated = append(activated, i) }
		l.Select(3)
		l.KeyAction(KeyDown, ModShift)
		l.KeyAction(KeyEnter, 0)
		if !reflect.DeepEqual(activated, []int{3, 4}) {
			t.Errorf("activated = %v, want [3 4]", activated)
		}
	})
}

// TestListMultipleKeyboardScroll pins that the keyboard cursor drives
// the virtualized window: ctrl+motion keys scroll without selecting,
// and the proxy rows re-resolve under the new offset.
func TestListMultipleKeyboardScroll(t *testing.T) {
	model := newCountingModel(50)
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionMultiple)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60}) // 3 rows visible
	cv := render.New(make([]uint8, 100*60*4), 100*4, 100, 60)
	l.Paint(cv)

	l.KeyAction(KeyEnd, ModCtrl)
	selEquals(t, l)
	if l.offY != 50*20-60 {
		t.Errorf("offY = %d after ctrl+End, want %d", l.offY, 50*20-60)
	}
	l.Paint(cv)
	if _, ok := l.rows[49]; !ok {
		t.Error("last row not cached after the cursor scrolled to it")
	}
}

// TestListMultiplePointer drives the Router end to end: a rubber-band
// drag selects the swept range and reports one notification with the
// full set; clicks toggle; a click after the band left the anchor row
// ends the gesture without toggling again.
func TestListMultiplePointer(t *testing.T) {
	l, _, notifications := setupMultiList(t)
	r := &Router{Root: l}
	pt := func(row int) Point { return Point{X: 50, Y: row*20 + 10} }

	t.Run("rubber band selects the swept range once", func(t *testing.T) {
		r.Press(BTNLeft, pt(2), 0)
		r.Move(pt(3))
		r.Move(pt(4))
		selEquals(t, l, 2, 3, 4)
		if len(*notifications) != 0 {
			t.Errorf("mid-drag notifications = %d, want 0 until the gesture ends", len(*notifications))
		}
		r.Release(BTNLeft, pt(4))
		selEquals(t, l, 2, 3, 4)
		if len(*notifications) != 1 {
			t.Errorf("notifications = %d for one drag, want 1", len(*notifications))
		}
		if !reflect.DeepEqual((*notifications)[0], []int{2, 3, 4}) {
			t.Errorf("notification = %v, want the full set [2 3 4]", (*notifications)[0])
		}
	})

	t.Run("clicks toggle", func(t *testing.T) {
		*notifications = nil
		r.Press(BTNLeft, pt(0), 0)
		r.Release(BTNLeft, pt(0))
		selEquals(t, l, 0, 2, 3, 4)
		r.Press(BTNLeft, pt(1), 0)
		r.Release(BTNLeft, pt(1))
		selEquals(t, l, 0, 1, 2, 3, 4)
		r.Press(BTNLeft, pt(0), 0)
		r.Release(BTNLeft, pt(0))
		selEquals(t, l, 1, 2, 3, 4)
		if len(*notifications) != 3 {
			t.Errorf("notifications = %d after three toggling clicks, want 3", len(*notifications))
		}
	})

	t.Run("a rapid second click activates, it does not toggle back", func(t *testing.T) {
		var activated []int
		l.OnActivate = func(i int) { activated = append(activated, i) }
		*notifications = nil
		r.Press(BTNLeft, pt(2), 0)
		r.Release(BTNLeft, pt(2))
		r.Press(BTNLeft, pt(2), 0)
		r.Release(BTNLeft, pt(2))
		selEquals(t, l, 1, 3, 4)
		if !reflect.DeepEqual(activated, []int{2}) {
			t.Errorf("activated = %v, want the double-clicked row [2]", activated)
		}
		if len(*notifications) != 1 {
			t.Errorf("notifications = %d across the pair, want 1 (the first click)", len(*notifications))
		}
	})

	t.Run("click after a drag ends the gesture, not a toggle", func(t *testing.T) {
		*notifications = nil
		r.Press(BTNLeft, pt(1), 0)
		r.Move(pt(3))
		r.Move(pt(1))
		r.Release(BTNLeft, pt(1))
		selEquals(t, l, 1)
		if len(*notifications) != 1 {
			t.Errorf("notifications = %d for drag-then-release-on-anchor, want 1", len(*notifications))
		}
	})

	t.Run("a cancelled press still concludes the gesture", func(t *testing.T) {
		*notifications = nil
		r.Press(BTNLeft, pt(1), 0)
		r.Move(pt(2))
		r.CancelPress()
		if len(*notifications) != 1 {
			t.Errorf("notifications = %d after a cancelled press, want 1", len(*notifications))
		}
	})
}

// TestListMultipleChecks pins checkbox rows: in multiple mode the row's
// CheckButton reflects membership, a click toggles membership through
// the check, and the check's own click handling stays out of it.
func TestListMultipleChecks(t *testing.T) {
	checks := make([]*CheckButton, 6)
	model := &sliceModel{}
	for i := range checks {
		checks[i] = NewCheckButton(false)
		model.rows = append(model.rows, checks[i])
	}
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionMultiple)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 120})
	cv := render.New(make([]uint8, 100*120*4), 100*4, 100, 120)
	l.Paint(cv)

	l.Select(1)
	l.toggle(3)
	if !checks[1].Checked() || !checks[3].Checked() {
		t.Error("checks do not reflect membership after selection")
	}
	if checks[0].Checked() || checks[2].Checked() {
		t.Error("unselected rows' checks are on")
	}

	r := &Router{Root: l}
	r.Press(BTNLeft, Point{X: 50, Y: 10}, 0)
	r.Release(BTNLeft, Point{X: 50, Y: 10})
	if !checks[0].Checked() {
		t.Error("clicking row 0 did not reflect its new membership in the check")
	}
	selEquals(t, l, 0, 1, 3)

	// Keyboard toggles reflect too: ctrl+arrows walk the cursor (off row
	// 0 after the click), and ctrl+space flips the cursor row's
	// membership into its check.
	l.KeyAction(KeyDown, ModCtrl)
	l.KeyAction(KeySpace, ModCtrl)
	if checks[1].Checked() {
		t.Error("keyboard toggle-off did not reflect in the row's check")
	}
	if !checks[0].Checked() || !checks[3].Checked() {
		t.Error("untouched member rows lost their checks")
	}
	selEquals(t, l, 0, 3)
}

// sliceModel is a fixed list of row widgets.
type sliceModel struct {
	rows []Widget
}

func (m *sliceModel) Len() int { return len(m.rows) }

func (m *sliceModel) Row(i int) Widget { return m.rows[i] }

// TestListBrowse pins browse mode: pointer motion selects the hovered
// row, and a clearing gesture pins back to a row instead of emptying
// the selection.
func TestListBrowse(t *testing.T) {
	model := newCountingModel(6)
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionBrowse)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 120})
	cv := render.New(make([]uint8, 100*120*4), 100*4, 100, 120)
	l.Paint(cv)

	r := &Router{Root: l}
	r.Move(Point{X: 50, Y: 30})
	if got := l.Selected(); got != 1 {
		t.Errorf("browse hover selected %d, want 1", got)
	}
	r.Move(Point{X: 50, Y: 90})
	if got := l.Selected(); got != 4 {
		t.Errorf("browse hover selected %d, want 4", got)
	}
	l.Select(-1)
	if got := l.Selected(); got != 0 {
		t.Errorf("clearing browse selection left %d, want pinned 0", got)
	}
}

// TestListSelectionInSingleMode pins that the default mode reports
// through the set API without touching the single-row contract.
func TestListSelectionInSingleMode(t *testing.T) {
	l, _, notifications := setupMultiList(t)
	l.SetSelectionMode(SelectionSingle)

	var selects []int
	l.OnSelect = func(i int) { selects = append(selects, i) }
	l.Select(3)
	selEquals(t, l, 3)
	if !reflect.DeepEqual(selects, []int{3}) {
		t.Errorf("OnSelect = %v, want [3]", selects)
	}
	if len(*notifications) != 1 || !reflect.DeepEqual((*notifications)[0], []int{3}) {
		t.Errorf("single-mode notifications = %v, want one [3]", *notifications)
	}
	l.Select(5)
	selEquals(t, l, 5)
	l.Select(-1)
	selEquals(t, l)
}

// TestListSelectionModeMigration pins the mode-switch contract:
// entering multiple mode seeds the set with the selection, leaving it
// keeps the lowest selected row, and an emptying switch notifies.
func TestListSelectionModeMigration(t *testing.T) {
	l, _, notifications := setupMultiList(t)
	l.Select(4)
	*notifications = nil
	l.SetSelectionMode(SelectionMultiple)
	selEquals(t, l, 4)
	l.SetSelectionMode(SelectionSingle)
	selEquals(t, l, 4)
	if len(*notifications) != 0 {
		t.Errorf("set-preserving switches notified %d times, want 0", len(*notifications))
	}

	l.Select(-1)
	l.SetSelectionMode(SelectionMultiple)
	l.toggle(2)
	l.toggle(2)
	*notifications = nil
	l.SetSelectionMode(SelectionSingle)
	selEquals(t, l)
	if len(*notifications) != 0 {
		t.Errorf("emptying switch notified %d times, want 0", len(*notifications))
	}
}

// TestListChangedDropsMultiIndices pins model shrinkage against the
// multiple-mode set: out-of-range rows leave the set, the cursor and
// anchor clamp, and surviving membership stays.
func TestListChangedDropsMultiIndices(t *testing.T) {
	model := newCountingModel(10)
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionMultiple)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	l.Select(2)
	l.toggle(8)

	model.rows = 5
	l.Changed()
	selEquals(t, l, 2)
	if l.cursor > 4 {
		t.Errorf("cursor = %d after shrink, want clamped", l.cursor)
	}
}

// TestListRubberBandAutoScroll pins the edge auto-scroll: a drag
// resting in the bottom edge band scrolls a row per step and extends
// the band under the stationary pointer, pins at the end of the list,
// and schedules nothing once the gesture ends.
func TestListRubberBandAutoScroll(t *testing.T) {
	c := pinAnimClock(t)
	model := newCountingModel(12)
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionMultiple)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60}) // 3 rows visible
	cv := render.New(make([]uint8, 100*60*4), 100*4, 100, 60)
	l.Paint(cv)

	r := &Router{Root: l}
	r.Press(BTNLeft, Point{X: 50, Y: 10}, 0) // row 0
	r.Move(Point{X: 50, Y: 50})              // bottom edge band (60-24=36)
	selEquals(t, l, 0, 1, 2)

	c.drive()
	if l.offY != 12*20-60 {
		t.Errorf("offY = %d after the pinned auto-scroll, want %d", l.offY, 12*20-60)
	}
	selEquals(t, l, rangeOf(0, 12)...)
	r.Release(BTNLeft, Point{X: 50, Y: 50})

	if animActiveNow(t) {
		t.Error("auto-scroll kept scheduling after the gesture ended")
	}
}

// rangeOf builds the half-open row range lo..hi-1.
func rangeOf(lo, hi int) []int {
	rows := make([]int, 0, max(0, hi-lo))
	for i := lo; i < hi; i++ {
		rows = append(rows, i)
	}
	return rows
}

// TestListAutoScrollPinsAtEnds pins that the auto-scroll stops at the
// list end instead of scheduling forever: the schedule drains even
// with the pointer parked in the band.
func TestListAutoScrollPinsAtEnds(t *testing.T) {
	c := pinAnimClock(t)
	model := newCountingModel(4)
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionMultiple)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	cv := render.New(make([]uint8, 100*60*4), 100*4, 100, 60)
	l.Paint(cv)

	r := &Router{Root: l}
	r.Press(BTNLeft, Point{X: 50, Y: 10}, 0)
	r.Move(Point{X: 50, Y: 50})
	c.drive()
	if l.offY != 4*20-60 {
		t.Errorf("offY = %d, want pinned at %d", l.offY, 4*20-60)
	}
	r.Release(BTNLeft, Point{X: 50, Y: 50})
}

// TestListAutoScrollReducedMotion pins the instant-mode contract: the
// edge band steps one row per motion event, no tween scheduled.
func TestListAutoScrollReducedMotion(t *testing.T) {
	restore := anim.SetInstant(true)
	defer restore()
	model := newCountingModel(12)
	l := NewList(model, 20)
	l.SetSelectionMode(SelectionMultiple)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	cv := render.New(make([]uint8, 100*60*4), 100*4, 100, 60)
	l.Paint(cv)

	r := &Router{Root: l}
	r.Press(BTNLeft, Point{X: 50, Y: 10}, 0)
	r.Move(Point{X: 50, Y: 50})
	if l.offY != 20 {
		t.Errorf("offY = %d after one instant step, want 20", l.offY)
	}
	r.Move(Point{X: 50, Y: 51})
	if l.offY != 40 {
		t.Errorf("offY = %d after a second motion, want 40", l.offY)
	}
	if animActiveNow(t) {
		t.Error("reduced motion scheduled an auto-scroll tween")
	}
	r.Release(BTNLeft, Point{X: 50, Y: 50})
}
