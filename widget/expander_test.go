package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/style"
	"github.com/stubbedev/lys-ui/render"
)

func TestExpanderToggleAnimates(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	child := &fixedWidget{sz: Size{W: 120, H: 60}}
	e := NewExpander(face, "Advanced", child)
	con := Constraints{Max: Size{W: 300, H: 500}}
	rect := render.Rect{X: 0, Y: 0, W: 300, H: 500}
	// relayout is the frame's layout pass: the draw loop re-measures
	// and re-arranges the tree every frame, which is what moves the
	// animated height into the child's rect.
	relayout := func() {
		e.Measure(con)
		e.Arrange(rect)
	}
	relayout()
	headH := e.headerH()

	t.Run("closed, the child is hidden and unmeasured", func(t *testing.T) {
		if got := e.Measure(con); got.H != headH {
			t.Errorf("closed height = %d, want the header %d", got.H, headH)
		}
		if kids := e.Children(); len(kids) != 0 {
			t.Errorf("closed expander exposed %d children, want 0", len(kids))
		}
	})

	t.Run("opening animates the height and settles", func(t *testing.T) {
		toggled := 0
		e.OnToggled = func(bool) { toggled++ }
		e.SetOpen(true)
		if toggled != 1 {
			t.Fatalf("OnToggled fired %d times, want 1", toggled)
		}
		// Mid-flight the height sits strictly between header and full.
		c.step()
		relayout()
		mid := e.Measure(con).H
		if mid <= headH || mid >= headH+60 {
			t.Errorf("mid-flight height = %d, want strictly inside (%d, %d)", mid, headH, headH+60)
		}
		c.drive()
		relayout()
		if got := e.Measure(con); got.H != headH+60 {
			t.Errorf("settled height = %d, want %d", got.H, headH+60)
		}
		if !e.Open() {
			t.Error("state not open after opening")
		}
		if animActiveNow(t) {
			t.Error("settled expander still scheduled wakes")
		}
	})

	t.Run("open, the child is exposed and arranged", func(t *testing.T) {
		kids := e.Children()
		if len(kids) != 1 || kids[0] != Widget(child) {
			t.Fatalf("children = %v, want the child", kids)
		}
		if child.Bounds().Empty() {
			t.Error("open expander never arranged its child")
		}
		if child.Bounds().H != 60 {
			t.Errorf("child height = %d, want the full 60 at progress 1", child.Bounds().H)
		}
	})

	t.Run("collapsing animates back and re-hides", func(t *testing.T) {
		e.Toggle()
		c.drive()
		if got := e.Measure(con); got.H != headH {
			t.Errorf("collapsed height = %d, want %d", got.H, headH)
		}
		if kids := e.Children(); len(kids) != 0 {
			t.Errorf("collapsed expander exposed %d children, want 0", len(kids))
		}
		if animActiveNow(t) {
			t.Error("collapsed expander still scheduled wakes")
		}
	})
}

func TestExpanderChildrenOnlyWhileOpen(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	child := &fixedWidget{sz: Size{W: 100, H: 40}}
	e := NewExpander(face, "Details", child)
	e.Measure(Constraints{Max: Size{W: 300, H: 500}})
	e.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 500})

	// The state flips at once, mid-animation included: focus traversal
	// follows the settled state, not the pixels.
	e.SetOpen(true)
	if kids := e.Children(); len(kids) != 1 {
		t.Fatalf("opening expander exposed %d children, want 1", len(kids))
	}
	c.step()
	if kids := e.Children(); len(kids) != 1 {
		t.Fatalf("animating-open expander exposed %d children, want 1", len(kids))
	}
	c.drive()
	e.SetOpen(false)
	if kids := e.Children(); len(kids) != 0 {
		t.Fatalf("closing expander exposed %d children, want 0", len(kids))
	}
	c.drive()
}

func TestExpanderRemembersStateWhileHidden(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	t0 := c.now()
	child := &fixedWidget{sz: Size{W: 100, H: 40}}
	e := NewExpander(face, "Details", child)
	con := Constraints{Max: Size{W: 300, H: 500}}
	e.Measure(con)

	// Toggled while out of the tree: settles instantly, no timer.
	e.SetOpen(true)
	if e.progress != 1 {
		t.Errorf("hidden toggle left progress = %v, want the settled 1", e.progress)
	}
	if animActiveNow(t) {
		t.Error("hidden toggle scheduled animation work")
	}

	// Shown later: full height at once, still no re-animation.
	e.Measure(con)
	e.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 500})
	if got := e.Measure(con); got.H != e.headerH()+40 {
		t.Errorf("shown height = %d, want fully open %d", got.H, e.headerH()+40)
	}
	if animActiveNow(t) {
		t.Error("shown expander scheduled animation work")
	}

	// Hidden again by a parent: the tween stops cold mid-reveal.
	e.SetOpen(false)
	c.step()
	e.Arrange(render.Rect{})
	if animActiveNow(t) {
		t.Error("hidden expander kept its tween")
	}
	// ...and the state is still "closing", remembered for the return.
	if e.Open() {
		t.Error("hidden expander forgot its closing state")
	}
	c.set(t0.Add(time.Second))
	animclock.Tick(c.now())
}

func TestExpanderClickAndKeyToggle(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	child := &fixedWidget{sz: Size{W: 100, H: 40}}
	e := NewExpander(face, "Details", child)
	con := Constraints{Max: Size{W: 300, H: 500}}
	relayout := func() {
		e.Measure(con)
		e.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 500})
	}
	relayout()

	head := e.headerRect()
	t.Run("a header click toggles", func(t *testing.T) {
		e.ClickAt(Point{X: head.X + head.W/2, Y: head.Y + head.H/2})
		if !e.Open() {
			t.Error("header click did not open")
		}
		e.ClickAt(Point{X: head.X + head.W/2, Y: head.Y + head.H/2})
		if e.Open() {
			t.Error("second header click did not close")
		}
	})

	t.Run("a body click does not toggle", func(t *testing.T) {
		e.SetOpen(true)
		p := Point{X: e.bounds.X + 10, Y: e.bounds.Y + e.headerH() + 5}
		e.ClickAt(p)
		if !e.Open() {
			t.Error("body click toggled the expander")
		}
		e.SetOpen(false)
	})

	t.Run("keyboard toggles", func(t *testing.T) {
		e.KeyAction(KeyEnter, 0)
		if !e.Open() {
			t.Error("Enter did not open")
		}
		e.InsertRune(' ')
		if e.Open() {
			t.Error("Space did not close")
		}
	})

	t.Run("hit tests resolve the header to the expander", func(t *testing.T) {
		e.SetOpen(true)
		for c.step() {
		}
		relayout()
		if hit := e.HitTest(Point{X: head.X + 3, Y: head.Y + 3}); hit != Widget(e) {
			t.Errorf("header hit = %v, want the expander", hit)
		}
		if hit := e.HitTest(Point{X: e.bounds.X + 10, Y: e.bounds.Y + e.headerH() + 5}); hit != Widget(child) {
			t.Errorf("body hit = %v, want the child", hit)
		}
	})
}

func TestExpanderPaint(t *testing.T) {
	face := entryFace(t)
	c := pinAnimClock(t)
	child := &fixedWidget{sz: Size{W: 100, H: 40}}
	e := NewExpander(face, "Details", child)
	con := Constraints{Max: Size{W: 300, H: 500}}
	relayout := func() {
		e.Measure(con)
		e.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 500})
	}
	w, h := 300, 200
	cv := render.New(make([]byte, render.Stride(w)*h), render.Stride(w), w, h)

	t.Run("closed", func(t *testing.T) {
		relayout()
		e.Paint(cv)
		if child.paint != 0 {
			t.Error("closed expander painted its child")
		}
	})

	t.Run("open reveals the child", func(t *testing.T) {
		e.SetOpen(true)
		for c.step() {
		}
		relayout()
		e.Paint(cv)
		if child.paint == 0 {
			t.Error("open expander did not paint its child")
		}
	})
}

// The header styles through `expander > title > arrow`: the title's
// color paints the caption, the arrow's the chevron.
func TestExpanderTitleAndArrowNodes(t *testing.T) {
	loadCSS(t, `expander > title { color: #010203; } expander > title > arrow { color: #0a0b0c; }`)
	e := NewExpander(testFace(t), "section", NewSpacer(1, 1))
	host := NewBox(Column, 0, 0)
	host.Append(e, false)
	frame(t, host, 100, 60)
	if got := pickc(0, e.titleNode.style(&e.titleNode), style.PropColor, 0); got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("title color %v, want the `expander > title` rule", got)
	}
	if got := pickc(0, e.arrow.style(&e.arrow), style.PropColor, 0); got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("arrow color %v, want the arrow rule", got)
	}
}

// TestGoldenExpander pins shadcn's Collapsible trigger: the title at
// the start and the chevron at the end, down when closed, up when open.
func TestGoldenExpander(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	face := goldenFace(t)
	for _, shot := range bothPalettes("expander") {
		col := NewBox(Column, 8, 0)
		closed := NewExpander(face, "@peduarte starred 3 repositories", NewLabel(face, 14, "@radix-ui/colors", 0))
		open := NewExpander(face, "@peduarte starred 3 repositories", NewLabel(face, 14, "@radix-ui/colors", 0))
		open.Toggle()
		col.Append(closed, false).Append(open, false)
		NewGolden(t, col, shot.name, goldenTheme(shot.th), goldenFrame(320, 90))
	}
}
