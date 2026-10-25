package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/render"
)

// recTarget is a focusTarget that counts key actions and clicks, so
// tests can observe that a removed widget receives neither.
type recTarget struct {
	focusTarget
	keyActions int
	clicks     int
}

func newRecTarget() *recTarget { return &recTarget{} }

// HitTest overrides the promoted one, which would name the embedded
// focusTarget as the hit — the router state must hold the recTarget
// itself for identity checks.
func (r *recTarget) HitTest(p Point) Widget { return r.HitLeaf(r, p) }

func (r *recTarget) KeyAction(a KeyAction, m Mods) { r.keyActions++ }
func (r *recTarget) ClickAt(p Point)               { r.clicks++ }

// captureRemoved installs a recording removal hook; restore unsets it.
// The returned slice receives every widget the mutation API detaches,
// in order.
func captureRemoved() (removed *[]Widget, restore func()) {
	ws := &[]Widget{}
	SetRemovedHook(func(w Widget) { *ws = append(*ws, w) })
	return ws, func() { SetRemovedHook(nil) }
}

// boxFixture measures and arranges b into a 100x20 rect, the state
// mutation tests start from.
func boxFixture(t *testing.T, b *Box) {
	t.Helper()
	b.Measure(Constraints{Max: Size{W: 100, H: 100}})
	b.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
}

func TestBoxRemove(t *testing.T) {
	t.Run("removes by identity, once", func(t *testing.T) {
		b := NewBox(Row, 2, 0)
		a1, a2, a3 := newStub(10, 5), newStub(8, 4), newStub(6, 3)
		b.Append(a1, false)
		b.Append(a2, false)
		b.Append(a3, false)

		if !b.Remove(a2) {
			t.Fatal("Remove missed an existing child")
		}
		got := b.Children()
		if len(got) != 2 || got[0] != a1 || got[1] != a3 {
			t.Errorf("children after remove = %v, want [a1 a3]", got)
		}
		// Double-remove is a no-op: false, and nothing else moved.
		if b.Remove(a2) {
			t.Error("double-remove reported true")
		}
		if got := b.Children(); len(got) != 2 || got[0] != a1 || got[1] != a3 {
			t.Errorf("children changed by double-remove: %v", got)
		}
		if b.Remove(newStub(1, 1)) {
			t.Error("Remove reported true for a widget that was never a child")
		}
	})

	t.Run("clears the parent link and allows re-attach elsewhere", func(t *testing.T) {
		boxA, boxB := NewBox(Row, 0, 0), NewBox(Column, 0, 0)
		w := newStub(5, 5)
		boxA.Append(w, false)
		boxFixture(t, boxA)
		if parentOf(w) != boxA {
			t.Fatal("Arrange did not record boxA as the parent")
		}

		if !boxA.Remove(w) {
			t.Fatal("Remove missed the child")
		}
		if parentOf(w) != nil {
			t.Errorf("parent = %v after removal, want nil", parentOf(w))
		}
		// The old tree must not reach the detached widget.
		walkTree(boxA, 0, func(v Widget, _ int) {
			if v == w {
				t.Error("removed widget still walked from its old parent")
			}
		})

		boxB.Append(w, false)
		boxFixture(t, boxB)
		if parentOf(w) != boxB {
			t.Errorf("parent after re-attach = %v, want boxB (no double parent)", parentOf(w))
		}
		if boxA.Remove(w) {
			t.Error("Remove succeeded on the old box after the widget moved")
		}
	})

	t.Run("reflows the measure cache", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		b.Append(newStub(10, 10), false)
		b.Append(newStub(10, 10), false)
		con := Constraints{Max: Size{W: 100, H: 100}}
		before := b.Measure(con)
		if !b.Remove(b.Children()[0]) {
			t.Fatal("Remove missed the child")
		}
		if after := b.Measure(con); after.W != before.W-10 {
			t.Errorf("measure after remove = %v, want the cache dropped and shrunk by 10", after)
		}
	})

	t.Run("fires the removal hook with the detached widget", func(t *testing.T) {
		removed, restore := captureRemoved()
		defer restore()
		b := NewBox(Row, 0, 0)
		a1, a2 := newStub(1, 1), newStub(2, 2)
		b.Append(a1, false)
		b.Append(a2, false)
		b.Remove(a1)
		if len(*removed) != 1 || (*removed)[0] != a1 {
			t.Errorf("removal hook saw %v, want [a1]", *removed)
		}
		b.Remove(a1) // no double notification
		if len(*removed) != 1 {
			t.Errorf("double-remove fired the hook again: %v", *removed)
		}
	})
}

func TestBoxRemoveAt(t *testing.T) {
	b := NewBox(Row, 2, 0)
	a1, a2, a3 := newStub(10, 5), newStub(8, 4), newStub(6, 3)
	b.Append(a1, false)
	b.Append(a2, false)
	b.Append(a3, false)

	b.RemoveAt(1)
	got := b.Children()
	if len(got) != 2 || got[0] != a1 || got[1] != a3 {
		t.Fatalf("children after RemoveAt(1) = %v, want [a1 a3]", got)
	}
	// Out-of-range indexes are no-ops, not panics.
	before := b.Children()
	b.RemoveAt(-1)
	b.RemoveAt(len(b.child))
	b.RemoveAt(len(b.child) + 5)
	if got := b.Children(); !slices.Equal(got, before) {
		t.Errorf("out-of-range RemoveAt changed children: %v", got)
	}
	if parentOf(a2) != nil {
		t.Error("RemoveAt left the parent link set")
	}
}

func TestBoxClear(t *testing.T) {
	b := NewBox(Row, 2, 1)
	kids := []Widget{newStub(10, 5), newStub(8, 4), newStub(6, 3)}
	for i, k := range kids {
		b.Append(k, i == 1)
	}
	boxFixture(t, b)
	for _, k := range kids {
		if parentOf(k) != b {
			t.Fatalf("Arrange did not record the parent for %v", k)
		}
	}

	removed, restore := captureRemoved()
	defer restore()
	b.Clear()

	if len(b.Children()) != 0 {
		t.Errorf("children after Clear = %v, want none", b.Children())
	}
	if !slices.Equal(*removed, kids) {
		t.Errorf("removal hook saw %v, want every child %v", *removed, kids)
	}
	for _, k := range kids {
		if parentOf(k) != nil {
			t.Errorf("parent of %v survived Clear", k)
		}
	}
	if got := b.Measure(Constraints{Max: Size{W: 100, H: 100}}); got != (Size{W: 2, H: 2}) {
		t.Errorf("measure after Clear = %v, want padding only 2x2", got)
	}
	// Clearing an empty box does nothing, fires nothing.
	*removed = nil
	b.Clear()
	if len(*removed) != 0 {
		t.Errorf("clearing an empty box fired the hook: %v", *removed)
	}
}

func TestBoxInsertAt(t *testing.T) {
	t.Run("inserts in the middle, front, and clamped ends", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		a1, a3, a2, a0, a4 := newStub(1, 1), newStub(3, 3), newStub(2, 2), newStub(0, 0), newStub(4, 4)
		b.Append(a1, false)
		b.Append(a3, false)

		b.InsertAt(1, a2, false) // middle
		b.InsertAt(0, a0, false) // front
		b.InsertAt(99, a4, true) // clamps to the back
		want := []Widget{a0, a1, a2, a3, a4}
		if got := b.Children(); !slices.Equal(got, want) {
			t.Fatalf("children = %v, want %v", got, want)
		}
		b.InsertAt(-7, a1, false) // clamps to the front, duplicate is fine
		if got := b.Children()[0]; got != a1 {
			t.Errorf("negative index inserted %v, want front", got)
		}
	})

	t.Run("expands like Append and links the parent", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		x, y := newStub(10, 10), newStub(10, 10)
		b.Append(x, false)
		b.Append(y, false)
		mid := newStub(10, 10)
		b.InsertAt(1, mid, true)
		boxFixture(t, b)

		if parentOf(mid) != b {
			t.Fatal("Arrange did not record the box as the inserted child's parent")
		}
		if mid.rect.W != 80 {
			t.Errorf("inserted expander width = %d, want the 70px leftover plus its 10 natural", mid.rect.W)
		}
		if y.rect.X != 90 {
			t.Errorf("last child at x = %d, want pushed past the expanded middle", y.rect.X)
		}
	})
}

func TestMutationRecomputesBounds(t *testing.T) {
	b := NewBox(Row, 0, 0)
	x, y := newStub(10, 10), newStub(10, 10)
	b.Append(x, false)
	b.Append(y, false)
	boxFixture(t, b)
	if x.rect != (render.Rect{X: 0, Y: 0, W: 10, H: 20}) || y.rect != (render.Rect{X: 10, Y: 0, W: 10, H: 20}) {
		t.Fatalf("fixture misarranged: x=%v y=%v", x.rect, y.rect)
	}

	b.Remove(x)
	// The box owes a repaint of its whole bounds right away — the
	// removed child's pixels inside it are zombie paint until then.
	bounds, _, dirty := b.takeDamage()
	if !dirty || bounds != (render.Rect{X: 0, Y: 0, W: 100, H: 20}) {
		t.Errorf("damage after remove = (%v, %v), want the box bounds dirty", bounds, dirty)
	}

	// Reflow moves the survivor and invalidates its old pixels.
	b.Measure(Constraints{Max: Size{W: 100, H: 100}})
	b.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
	if y.rect != (render.Rect{X: 0, Y: 0, W: 10, H: 20}) {
		t.Errorf("survivor did not reflow: %v", y.rect)
	}
	if y.Bounds() != y.rect {
		t.Errorf("stale-bounds zombie: Bounds() = %v, arranged %v", y.Bounds(), y.rect)
	}
	old := render.Rect{X: 10, Y: 0, W: 10, H: 20}
	if !slices.Contains(y.extras, old) {
		t.Errorf("moved child owed no repaint of its old rect %v (extras %v)", old, y.extras)
	}
}

func TestStackRemove(t *testing.T) {
	t.Run("removing the visible child shows nothing", func(t *testing.T) {
		s := NewStack()
		w1, w2 := newStub(10, 10), newStub(20, 20)
		s.Add("a", w1)
		s.Add("b", w2)
		s.Show("b")

		if !s.Remove("b") {
			t.Fatal("Remove missed the visible child")
		}
		// Pinned: the stack shows nothing — no silent sibling promotion.
		if s.Visible() != "" {
			t.Errorf("visible = %q, want empty", s.Visible())
		}
		if s.Children() != nil {
			t.Errorf("children after removing the visible child = %v, want none", s.Children())
		}
		if parentOf(w2) != nil {
			t.Error("removed child kept its parent link")
		}
		if s.Remove("b") {
			t.Error("double-remove reported true")
		}
		// The next Add names a child again.
		s.Add("c", newStub(1, 1))
		if s.Visible() != "c" {
			t.Errorf("visible after Add = %q, want c", s.Visible())
		}
	})

	t.Run("removing a hidden child keeps the visible one", func(t *testing.T) {
		s := NewStack()
		s.Add("a", newStub(1, 1))
		s.Add("b", newStub(2, 2))
		s.Remove("b")
		if s.Visible() != "a" {
			t.Errorf("visible = %q, want a untouched", s.Visible())
		}
		if s.Remove("zz") {
			t.Error("Remove reported true for an unknown name")
		}
	})

	t.Run("drops the measure contribution of the removed child", func(t *testing.T) {
		s := NewStack()
		big, small := newStub(40, 40), newStub(10, 10)
		s.Add("big", big)
		s.Add("small", small)
		con := Constraints{Max: Size{W: 100, H: 100}}
		if got := s.Measure(con); got.W != 40 {
			t.Fatalf("stack measure = %v, want the 40px child", got)
		}
		s.Remove("big")
		if got := s.Measure(con); got.W != 10 {
			t.Errorf("stack measure after remove = %v, want the cache dropped to 10", got)
		}
	})

	t.Run("replacing via Add detaches the old child", func(t *testing.T) {
		s := NewStack()
		old := newStub(1, 1)
		s.Add("a", old)
		repl, restore := captureRemoved()
		defer restore()
		fresh := newStub(2, 2)
		s.Add("a", fresh)

		if parentOf(old) != nil {
			t.Error("replaced child kept its parent link")
		}
		if len(*repl) != 1 || (*repl)[0] != old {
			t.Errorf("removal hook saw %v, want the replaced child", *repl)
		}
		s.Arrange(render.Rect{X: 0, Y: 0, W: 10, H: 10})
		walkTree(s, 0, func(w Widget, _ int) {
			if w == old {
				t.Error("replaced child still walked from the stack")
			}
		})
	})
}

func TestScrollSetChild(t *testing.T) {
	t.Run("swaps content and resets the scroll position", func(t *testing.T) {
		s := NewScroll(newStub(60, 200))
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		s.SetOffset(0, 50)
		old := s.child

		fresh := Widget(newStub(60, 100))
		removed, restore := captureRemoved()
		defer restore()
		s.SetChild(fresh)

		if offX, offY := s.Offset(); offX != 0 || offY != 0 {
			t.Errorf("offset after swap = (%d,%d), want reset to (0,0)", offX, offY)
		}
		if len(*removed) != 1 || (*removed)[0] != old {
			t.Errorf("removal hook saw %v, want the old child", *removed)
		}
		if parentOf(old) != nil {
			t.Error("old child kept its parent link")
		}
		var seen []Widget
		walkTree(s, 0, func(w Widget, _ int) { seen = append(seen, w) })
		if slices.Contains(seen, old) || !slices.Contains(seen, fresh) {
			t.Errorf("traversal after swap saw %v, want the new child only", seen)
		}
		// The measure cache dropped: the new child's range, not the old.
		// The 100px child still overflows vertically, so the vertical
		// gutter stays reserved and the 60px-wide child overflows
		// horizontally by exactly that gutter.
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		if maxX, maxY := s.scrollMax(); maxX != gutter || maxY != 40 {
			t.Errorf("scrollMax after swap = (%d,%d), want (%d,40)", maxX, maxY, gutter)
		}
	})

	t.Run("re-setting the same child is a no-op", func(t *testing.T) {
		cur := newStub(60, 100)
		s := NewScroll(cur)
		s.Measure(Constraints{Max: Size{W: 60, H: 60}})
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
		removed, restore := captureRemoved()
		defer restore()
		s.SetOffset(0, 10)
		s.SetChild(cur)
		if len(*removed) != 0 {
			t.Errorf("no-op swap fired the removal hook: %v", *removed)
		}
		if _, offY := s.Offset(); offY != 10 {
			t.Errorf("no-op swap reset the offset to %d", offY)
		}
	})

	t.Run("nil clears the content", func(t *testing.T) {
		s := NewScroll(newStub(60, 200))
		s.SetChild(nil)
		if s.Children() != nil {
			t.Errorf("children after nil = %v, want none", s.Children())
		}
		con := Constraints{Max: Size{W: 60, H: 60}}
		if got := s.Measure(con); got != (Size{}) {
			t.Errorf("empty scroll measure = %v, want zero", got)
		}
		s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60}) // must not panic
		if hit := s.HitTest(Point{X: 10, Y: 10}); hit != s {
			t.Errorf("empty scroll hit = %v, want the scroll itself", hit)
		}
		var seen []Widget
		walkTree(s, 0, func(w Widget, _ int) { seen = append(seen, w) })
		if len(seen) != 1 || seen[0] != s {
			t.Errorf("empty scroll walk saw %v, want just the scroll", seen)
		}
	})

	t.Run("ends an in-flight bar drag", func(t *testing.T) {
		s := newScrollFixture(t)
		s.SetPressed(true)
		s.dragV = true
		s.SetChild(newStub(60, 200))
		if s.dragV || s.dragH {
			t.Error("content swap left a scrollbar drag attached")
		}
	})
}

// removeRouterFixture builds a root row of three 8x8 recTargets with a
// router over it, all arranged.
func removeRouterFixture(t *testing.T) (root *Box, ts []*recTarget, r *Router) {
	t.Helper()
	root = NewBox(Row, 0, 0)
	ts = []*recTarget{newRecTarget(), newRecTarget(), newRecTarget()}
	for _, w := range ts {
		root.Append(w, false)
	}
	root.Measure(Constraints{Max: Size{W: 100, H: 100}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: 24, H: 8})
	r = &Router{Root: root}
	return root, ts, r
}

// forget removes w through the mutation API with the router wired as
// the Application would.
func forget(t *testing.T, r *Router, b *Box, w Widget) {
	t.Helper()
	SetRemovedHook(r.Forget)
	defer SetRemovedHook(nil)
	if !b.Remove(w) {
		t.Fatal("Remove missed the child")
	}
}

func TestRemoveClearsRouterState(t *testing.T) {
	t.Run("removing the focused widget moves focus per focusAfter", func(t *testing.T) {
		b, ts, r := removeRouterFixture(t)
		r.Press(BTNLeft, Point{X: 12, Y: 4}, 0) // focus (and press) t2
		if r.Focused() != ts[1] {
			t.Fatal("fixture: press did not focus t2")
		}
		forget(t, r, b, ts[1])

		if r.Focused() != ts[2] {
			t.Errorf("focus = %v, want the traversal neighbor t3", r.Focused())
		}
		if r.Pressed() != nil {
			t.Errorf("pressed = %v, want cancelled", r.Pressed())
		}
		// Key actions reach the new focus, never the dead widget.
		r.KeyAction(KeyLeft, 0)
		if ts[1].keyActions != 0 || ts[2].keyActions != 1 {
			t.Errorf("key delivery: removed=%d new=%d, want 0 and 1", ts[1].keyActions, ts[2].keyActions)
		}
	})

	t.Run("removing the hovered widget clears hover", func(t *testing.T) {
		b, ts, r := removeRouterFixture(t)
		r.Move(Point{X: 12, Y: 4})
		if r.Hovered() != ts[1] {
			t.Fatal("fixture: move did not hover t2")
		}
		forget(t, r, b, ts[1])
		if r.Hovered() != nil {
			t.Errorf("hovered = %v, want nil", r.Hovered())
		}
	})

	t.Run("a cancelled press cannot click", func(t *testing.T) {
		b, ts, r := removeRouterFixture(t)
		r.Move(Point{X: 12, Y: 4})
		r.Press(BTNLeft, Point{X: 12, Y: 4}, 0)
		forget(t, r, b, ts[1])
		r.Release(BTNLeft, Point{X: 12, Y: 4})
		if ts[1].clicks != 0 {
			t.Errorf("removed widget was clicked %d times", ts[1].clicks)
		}
		if r.Pressed() != nil {
			t.Errorf("pressed = %v after release, want nil", r.Pressed())
		}
	})

	t.Run("focus inside a removed subtree moves too", func(t *testing.T) {
		root := NewBox(Row, 0, 0)
		inner := NewBox(Row, 0, 0)
		t2 := newRecTarget()
		inner.Append(t2, false)
		t3 := newRecTarget()
		root.Append(inner, false)
		root.Append(t3, false)
		root.Measure(Constraints{Max: Size{W: 100, H: 100}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: 16, H: 8})
		r := &Router{Root: root}

		r.Press(BTNLeft, Point{X: 4, Y: 4}, 0) // focus t2 inside inner
		if r.Focused() != t2 {
			t.Fatal("fixture: press did not focus t2")
		}
		forget(t, r, root, inner)
		if r.Focused() != t3 {
			t.Errorf("focus = %v, want the first focusable after the removed subtree", r.Focused())
		}
	})

	t.Run("focus with nowhere to go clears", func(t *testing.T) {
		b, ts, r := removeRouterFixture(t)
		r.Press(BTNLeft, Point{X: 12, Y: 4}, 0)
		for _, w := range b.Children() {
			if w != ts[1] {
				b.Remove(w)
			}
		}
		forget(t, r, b, ts[1])
		if r.Focused() != nil {
			t.Errorf("focus = %v, want nil with nothing focusable left", r.Focused())
		}
	})

	t.Run("unrelated state survives", func(t *testing.T) {
		b, ts, r := removeRouterFixture(t)
		r.Move(Point{X: 4, Y: 4})              // hover t1
		r.Press(BTNLeft, Point{X: 4, Y: 4}, 0) // focus t1
		forget(t, r, b, ts[1])                 // removing t2 touches neither
		if r.Hovered() != ts[0] || r.Focused() != ts[0] {
			t.Errorf("hover=%v focus=%v, want both still t1", r.Hovered(), r.Focused())
		}
	})
}

func TestRemoveMidTraversal(t *testing.T) {
	t.Run("a Children snapshot survives mutation", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		var kids []Widget
		for range 3 {
			w := newRecTarget()
			b.Append(w, false)
			kids = append(kids, w)
		}
		snap := b.Children()
		for i, w := range snap {
			if !b.Remove(w) {
				t.Errorf("remove %d missed", i)
			}
		}
		if len(b.Children()) != 0 {
			t.Errorf("children after removing the snapshot = %v, want none", b.Children())
		}
		if !slices.Equal(snap, kids) {
			t.Errorf("snapshot changed under the iteration: %v", snap)
		}
	})

	t.Run("walkTree visits a consistent pre-mutation set", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		t1, t2, t3 := newRecTarget(), newRecTarget(), newRecTarget()
		b.Append(t1, false)
		b.Append(t2, false)
		b.Append(t3, false)

		var visited []Widget
		walkTree(b, 0, func(w Widget, _ int) {
			visited = append(visited, w)
			if w == t1 {
				b.RemoveAt(1) // detach t2 mid-traversal
			}
		})
		want := []Widget{b, t1, t2, t3}
		if !slices.Equal(visited, want) {
			t.Errorf("walk visited %v, want the pre-mutation set %v", visited, want)
		}
		if b.Remove(t2) {
			t.Error("the mid-walk removal did not stick")
		}
	})

	t.Run("Overlay.Children is a snapshot too", func(t *testing.T) {
		o := NewOverlay()
		t1, t2 := newRecTarget(), newRecTarget()
		o.Append(t1)
		o.Append(t2)
		snap := o.Children()
		snap[0] = nil // mutating the snapshot must not touch the overlay
		if got := o.Children(); got[0] != t1 || got[1] != t2 {
			t.Errorf("overlay children = %v, want the snapshot isolated", got)
		}
	})
}
