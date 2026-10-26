package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func TestFixedMeasure(t *testing.T) {
	t.Run("measures to the union of the children's extents", func(t *testing.T) {
		f := NewFixed()
		f.Put(newStub(10, 5), 3, 4)
		f.Put(newStub(8, 6), 20, 10)
		got := f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got != (Size{W: 28 - 3, H: 16 - 4}) {
			t.Errorf("measure = %v, want 25x12", got)
		}
	})

	t.Run("negative coordinates grow the union", func(t *testing.T) {
		f := NewFixed()
		f.Put(newStub(10, 5), 0, 0)
		f.Put(newStub(4, 3), -6, 1)
		got := f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got != (Size{W: 16, H: 5}) {
			t.Errorf("measure = %v, want 16x5", got)
		}
	})

	t.Run("an empty fixed measures zero", func(t *testing.T) {
		got := NewFixed().Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got != (Size{}) {
			t.Errorf("empty fixed = %v, want 0x0", got)
		}
	})

	t.Run("clamps to constraints", func(t *testing.T) {
		f := NewFixed()
		f.Put(newStub(50, 5), 0, 0)
		got := f.Measure(Constraints{Max: Size{W: 30, H: 3}})
		if got.W != 30 || got.H != 3 {
			t.Errorf("measured %v, want clamped to 30x3", got)
		}
	})
}

func TestFixedArrange(t *testing.T) {
	t.Run("children keep their natural size at their coordinates", func(t *testing.T) {
		f := NewFixed()
		a := newStub(10, 5)
		c := newStub(8, 4)
		f.Put(a, 5, 7)
		f.Put(c, 40, 2)
		f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		f.Arrange(render.Rect{X: 10, Y: 20, W: 48, H: 12})

		if a.rect != (render.Rect{X: 15, Y: 27, W: 10, H: 5}) {
			t.Errorf("first child = %v, want 15,27 10x5", a.rect)
		}
		if c.rect != (render.Rect{X: 50, Y: 22, W: 8, H: 4}) {
			t.Errorf("second child = %v, want 50,22 8x4", c.rect)
		}
	})

	t.Run("a larger rect does not stretch the children", func(t *testing.T) {
		f := NewFixed()
		a := newStub(10, 5)
		f.Put(a, 0, 0)
		f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		f.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 100})
		if a.rect != (render.Rect{X: 0, Y: 0, W: 10, H: 5}) {
			t.Errorf("child = %v, want its natural 10x5", a.rect)
		}
	})

	t.Run("an empty fixed arranges nothing and survives", func(t *testing.T) {
		f := NewFixed()
		f.Arrange(render.Rect{X: 0, Y: 0, W: 50, H: 50})
		if len(f.Children()) != 0 {
			t.Errorf("children = %d, want none", len(f.Children()))
		}
	})
}

func TestFixedPutMoveRemove(t *testing.T) {
	t.Run("Move keeps the child attached, Remove detaches it", func(t *testing.T) {
		f := NewFixed()
		a, b := newStub(4, 4), newStub(4, 4)
		f.Put(a, 0, 0).Put(b, 10, 0)
		removed := 0
		SetRemovedHook(func(Widget) { removed++ })
		defer SetRemovedHook(nil)
		f.Move(a, 20, 8)
		if removed != 0 || !slices.Equal(f.Children(), []Widget{a, b}) {
			t.Fatalf("Move detached a child: removed=%d children=%v", removed, f.Children())
		}
		f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		f.Arrange(render.Rect{X: 0, Y: 0, W: 24, H: 8})
		if a.rect != (render.Rect{X: 20, Y: 8, W: 4, H: 4}) {
			t.Errorf("moved child = %v, want 20,8", a.rect)
		}
		if !f.Remove(a) || f.Remove(a) {
			t.Error("Remove reported the wrong membership")
		}
		if removed != 1 || !slices.Equal(f.Children(), []Widget{b}) {
			t.Errorf("after Remove: removed=%d children=%v, want b alone", removed, f.Children())
		}
	})

	t.Run("Put on an attached child moves it", func(t *testing.T) {
		f := NewFixed()
		a := newStub(4, 4)
		f.Put(a, 0, 0)
		f.Put(a, 12, 3)
		if got := f.Children(); !slices.Equal(got, []Widget{a}) {
			t.Fatalf("children = %v, want a once", got)
		}
		f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		f.Arrange(render.Rect{X: 0, Y: 0, W: 16, H: 7})
		if a.rect != (render.Rect{X: 12, Y: 3, W: 4, H: 4}) {
			t.Errorf("re-put child = %v, want 12,3", a.rect)
		}
	})

	t.Run("Move on an unknown widget is a no-op", func(t *testing.T) {
		f := NewFixed()
		f.Move(newStub(4, 4), 5, 5)
		if len(f.Children()) != 0 {
			t.Errorf("children = %d, want none", len(f.Children()))
		}
	})
}

func TestFixedMoveDamagesOldAndNewRects(t *testing.T) {
	f := NewFixed()
	a := newStub(10, 10)
	f.Put(a, 5, 5)
	f.Measure(Constraints{Max: Size{W: 100, H: 100}})
	f.Arrange(render.Rect{X: 0, Y: 0, W: 15, H: 15})
	warmDamage(f)

	f.Move(a, 40, 5)
	nat := f.Measure(Constraints{Max: Size{W: 100, H: 100}})
	f.Arrange(render.Rect{X: 0, Y: 0, W: nat.W, H: nat.H})
	rects, any := CollectDamage(f)
	if !any {
		t.Fatal("Move produced no damage")
	}
	got := render.UnionAll(rects)
	want := (render.Rect{X: 5, Y: 5, W: 10, H: 10}).Union(render.Rect{X: 40, Y: 5, W: 10, H: 10})
	if got.Union(want) != got {
		t.Errorf("move damage union %+v does not cover old ∪ new %+v", got, want)
	}
}
