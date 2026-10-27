package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// dumpFixture builds the golden tree: a column with a header, a named
// button wrapping a label, and an entry - arranged at a fixed size.
// The router state is a plausible mid-gesture frame: the pointer sits
// on the label inside the button with the button held down, and
// keyboard focus is parked on the slider.
type dumpFixture struct {
	root   Widget
	router *Router
}

func newDumpFixture(t *testing.T) *dumpFixture {
	t.Helper()
	face := entryFace(t)
	header := NewLabel(face, 13, "demo", render.RGB(255, 255, 255))
	header.SetTooltip("the page title")
	inner := NewLabel(face, 13, "save", render.RGB(255, 255, 255))
	button := NewButton(inner, 10, 4)
	button.SetDebugName("toolbar:save")
	button.SetTooltip("saves the thing")
	entry := NewEntry(face, 13, render.RGB(255, 255, 255))
	entry.SetText("type here")
	slider := NewSlider(0, 1, 0.1, 0.5)
	slider.SetDebugName("zoom")
	column := NewBox(Column, 8, 4).
		Append(header, false).
		Append(button, false).
		Append(entry, false).
		Append(slider, false)
	SizeWithin(column, Size{W: 400, H: 300})
	column.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 300})
	r := &Router{Root: column}
	// Hover the button and hold the press - a Button is a hit-test
	// leaf, so the router's hover and press targets are the button
	// itself, not the label inside it.
	bs := button.Bounds()
	p := Point{X: bs.X + bs.W/2, Y: bs.Y + bs.H/2}
	r.Move(p)
	if r.Hovered() == nil {
		t.Fatal("fixture: hover missed the button")
	}
	r.Press(BTNLeft, p, 0)
	// Keyboard focus sits on the slider while the pointer gesture is
	// still held - focus and press targets are independent.
	r.focus = slider
	return &dumpFixture{root: column, router: r}
}

func TestDumpTreeGolden(t *testing.T) {
	fx := newDumpFixture(t)
	got := DumpTree(fx.root, fx.router)
	wantPath := filepath.Join("testdata", "dump.golden")
	want, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("DumpTree does not match golden %s.\n--- got ---\n%s--- want ---\n%s", wantPath, got, want)
	}
}

func TestDumpTreeGoldenWithoutRouter(t *testing.T) {
	fx := newDumpFixture(t)
	got := DumpTree(fx.root, nil)
	if strings.Contains(got, " focused") || strings.Contains(got, " hover") || strings.Contains(got, " pressed") {
		t.Errorf("router-free dump carries input flags:\n%s", got)
	}
	if !strings.Contains(got, `name="toolbar:save"`) {
		t.Errorf("dump lost the debug name:\n%s", got)
	}
}

func TestInspectTreeMatchesDescribeTreeOrder(t *testing.T) {
	fx := newDumpFixture(t)
	live := InspectTree(fx.root, fx.router)
	snap := DescribeTree(fx.root)
	if len(live) != len(snap) {
		t.Fatalf("InspectTree has %d nodes, DescribeTree %d", len(live), len(snap))
	}
	for i := range live {
		if live[i].Bounds != snap[i].Bounds || live[i].Role != snap[i].Role || live[i].Name != snap[i].Name {
			t.Errorf("node %d: live (%+v) disagrees with a11y snapshot (%+v)", i, live[i], snap[i])
		}
	}
}

func TestSetDebugNameSurvivesTextUpdates(t *testing.T) {
	face := entryFace(t)

	t.Run("label SetText keeps the name", func(t *testing.T) {
		l := NewLabel(face, 13, "before", render.RGB(255, 255, 255))
		l.SetDebugName("status:line")
		l.SetText("after")
		if l.Text() != "after" {
			t.Fatalf("SetText did not apply: %q", l.Text())
		}
		if got := l.DebugName(); got != "status:line" {
			t.Errorf("debug name after SetText = %q", got)
		}
	})

	t.Run("entry edits keep the name", func(t *testing.T) {
		e := NewEntry(face, 13, render.RGB(255, 255, 255))
		e.SetDebugName("search:field")
		e.SetText("abc")
		e.Insert("d")
		if got := e.DebugName(); got != "search:field" {
			t.Errorf("debug name after edits = %q", got)
		}
	})

	t.Run("empty name clears", func(t *testing.T) {
		l := NewLabel(face, 13, "x", render.RGB(255, 255, 255))
		l.SetDebugName("temp")
		l.SetDebugName("")
		if got := l.DebugName(); got != "" {
			t.Errorf("cleared name = %q", got)
		}
	})
}

func TestInspectTreeFlags(t *testing.T) {
	fx := newDumpFixture(t)
	nodes := InspectTree(fx.root, fx.router)
	byName := map[string]NodeInfo{}
	for _, ni := range nodes {
		if ni.DebugName != "" {
			byName[ni.DebugName] = ni
		}
	}
	save, ok := byName["toolbar:save"]
	if !ok {
		t.Fatalf("named button missing from snapshot:\n%s", DumpTree(fx.root, fx.router))
	}
	if !save.Focusable {
		t.Error("button not marked focusable")
	}
	if !save.Hovered || !save.Pressed {
		t.Errorf("button flags hover:%v pressed:%v, want true/true", save.Hovered, save.Pressed)
	}
	if save.Focused {
		t.Error("button reported as the focus target; focus is parked on the slider")
	}
	zoom, ok := byName["zoom"]
	if !ok {
		t.Fatalf("named slider missing from snapshot:\n%s", DumpTree(fx.root, fx.router))
	}
	if !zoom.Focused {
		t.Error("slider not reported as the focus target")
	}
	if nodes[0].Depth != 0 {
		t.Errorf("root depth = %d, want 0", nodes[0].Depth)
	}
	var hovered int
	for _, ni := range nodes {
		if ni.Hovered {
			hovered++
		}
	}
	if hovered != 1 {
		t.Errorf("%d nodes flagged hover, want exactly 1", hovered)
	}
}
