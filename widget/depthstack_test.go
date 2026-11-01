package widget

import (
	"slices"
	"testing"
)

func TestKidStackClearsEachLevelWhenItsLoopEnds(t *testing.T) {
	var s kidStack
	parent, child := NewBox(Row, 0, 0), NewBox(Row, 0, 0)
	var visited []Widget
	s.each(func(kids []Widget) []Widget { return append(kids, parent, child) }, func(k Widget) {
		visited = append(visited, k)
		if k != parent {
			return
		}
		s.each(func(kids []Widget) []Widget { return append(kids, child) }, func(k Widget) {
			visited = append(visited, k)
		})
		if s.levels[0][1] != child {
			t.Error("a nested walk overwrote the level still being iterated")
		}
	})
	if want := []Widget{parent, child, child}; !slices.Equal(visited, want) {
		t.Errorf("visited %v, want %v", visited, want)
	}
	for depth, level := range s.levels {
		for i, k := range level {
			if k != nil {
				t.Errorf("level %d slot %d still holds %v after the walk", depth, i, k)
			}
		}
	}
	if s.depth != 0 {
		t.Errorf("depth = %d after the walk, want 0", s.depth)
	}
}

func TestKidStackKeepsItsBuffersAcrossWalks(t *testing.T) {
	var s kidStack
	box := NewBox(Row, 0, 0)
	fill := func(kids []Widget) []Widget { return append(kids, box) }
	s.each(fill, func(Widget) {})
	if allocs := testing.AllocsPerRun(10, func() { s.each(fill, func(Widget) {}) }); allocs != 0 {
		t.Errorf("a walk over a grown level allocated %v times, want none", allocs)
	}
}

func TestKidStackUnwindsAPanickingVisit(t *testing.T) {
	var s kidStack
	box := NewBox(Row, 0, 0)
	func() {
		defer func() { _ = recover() }()
		s.each(func(kids []Widget) []Widget { return append(kids, box) }, func(Widget) { panic("visit") })
	}()
	if s.depth != 0 || s.levels[0][0] != nil {
		t.Errorf("depth %d, slot %v after a panicking visit, want 0 and nil", s.depth, s.levels[0][0])
	}
}

func TestDepthStackPopClearsTheLevel(t *testing.T) {
	var s depthStack[*int]
	v := 1
	s.push(1)[0] = &v
	s.pop()
	if s.bufs[0][0] != nil {
		t.Error("a popped level still holds what its pass stored")
	}
}
