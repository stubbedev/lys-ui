package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// stackFixture is a two-level tree: a leaf, a submenu with a long
// label inside it, and a check row.
func stackFixture(t *testing.T, fired *string) *MenuStack {
	t.Helper()
	return NewMenuStack(entryFace(t), 14,
		MenuItem{Label: "Open", OnClick: func() { *fired = "open" }},
		MenuItem{Label: "Share", OnOpen: func() { *fired = "share opened" }, Items: []MenuItem{
			{Label: "Mail", OnClick: func() { *fired = "mail" }},
			{Label: "A much longer submenu label", OnClick: func() { *fired = "long" }},
		}},
		MenuItem{Label: "Wrap", Kind: ItemCheck, OnClick: func() {}},
	)
}

func layStack(s *MenuStack) {
	sz := s.Measure(Constraints{Max: Size{W: 600, H: 600}})
	s.Arrange(render.Rect{W: sz.W, H: sz.H})
}

func stackRow(s *MenuStack, i int) Point {
	return Point{X: 30, Y: rowCenter(s.Top(), i).Y}
}

func TestMenuStackSlidesIntoSubmenus(t *testing.T) {
	fired := ""
	s := stackFixture(t, &fired)
	dismissed := 0
	s.OnDismiss = func() { dismissed++ }
	layStack(s)

	s.ClickAt(stackRow(s, 1))
	if s.Depth() != 2 || fired != "share opened" {
		t.Fatalf("depth after the submenu row = %d, fired %q; want 2 and its OnOpen", s.Depth(), fired)
	}
	if dismissed != 0 {
		t.Fatal("opening a submenu dismissed the stack")
	}
	top := s.Top().items
	if !strings.HasPrefix(top[0].Label, BackLabelPrefix) || !strings.HasSuffix(top[0].Label, "Share") {
		t.Errorf("back row = %q, want the prefix and the parent label", top[0].Label)
	}

	// The back row returns to the root without dismissing.
	layStack(s)
	s.ClickAt(stackRow(s, 0))
	if s.Depth() != 1 || dismissed != 0 {
		t.Fatalf("back row: depth %d, dismissed %d; want 1, 0", s.Depth(), dismissed)
	}

	// A leaf in the submenu fires and dismisses once.
	layStack(s)
	s.ClickAt(stackRow(s, 1))
	layStack(s)
	s.ClickAt(stackRow(s, 1))
	if fired != "mail" || dismissed != 1 {
		t.Fatalf("submenu leaf: fired %q, dismissed %d; want mail, 1", fired, dismissed)
	}
}

func TestMenuStackKeyboard(t *testing.T) {
	fired := ""
	s := stackFixture(t, &fired)
	dismissed := false
	s.OnDismiss = func() { dismissed = true }
	layStack(s)

	s.Top().hovered = 1
	s.KeyAction(KeyRight, 0)
	if s.Depth() != 2 {
		t.Fatalf("Right on the submenu row: depth = %d", s.Depth())
	}
	// Left pops a level rather than closing.
	s.KeyAction(KeyLeft, 0)
	if s.Depth() != 1 || dismissed {
		t.Fatalf("Left in a submenu: depth %d, dismissed %v", s.Depth(), dismissed)
	}
	// At the root Left closes, as it does on a bare Menu.
	s.KeyAction(KeyLeft, 0)
	if !dismissed {
		t.Error("Left at the root did not dismiss")
	}
}

func TestMenuStackMeasuresTheLargestLevel(t *testing.T) {
	fired := ""
	s := stackFixture(t, &fired)
	con := Constraints{Max: Size{W: 600, H: 600}}
	stack := s.Measure(con)
	root := s.Top().Measure(con)
	if stack.W <= root.W {
		t.Errorf("stack width %d, want wider than the root level's %d (the submenu's long label)", stack.W, root.W)
	}
	// Sliding in never needs more room than the stack measured.
	layStack(s)
	s.ClickAt(stackRow(s, 1))
	if sub := s.Top().Measure(con); sub.W > stack.W || sub.H > stack.H {
		t.Errorf("submenu %v exceeds the stack's %v", sub, stack)
	}
}

func TestMenuStackSetItemsReturnsToRoot(t *testing.T) {
	fired := ""
	s := stackFixture(t, &fired)
	layStack(s)
	s.ClickAt(stackRow(s, 1))
	s.SetItems(MenuItem{Label: "Only", OnClick: func() { fired = "only" }})
	if s.Depth() != 1 || len(s.Top().items) != 1 {
		t.Fatalf("after SetItems: depth %d, rows %d", s.Depth(), len(s.Top().items))
	}
	layStack(s)
	s.ClickAt(stackRow(s, 0))
	if fired != "only" {
		t.Errorf("fired = %q, want the new tree's row", fired)
	}
}

func TestMenuStackCheckStateSticksWhileOpen(t *testing.T) {
	fired := ""
	s := stackFixture(t, &fired)
	s.OnDismiss = func() {}
	layStack(s)
	s.ClickAt(stackRow(s, 2))
	if !s.Top().items[2].Checked {
		t.Error("the check row did not toggle")
	}
}
