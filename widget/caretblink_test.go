package widget

import (
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
)

// A focused entry's caret spans the font's extents and blinks as GTK's
// does: a half cycle of 600ms each way, only its own rect repainting,
// solid again on every input, solid for good after 10s idle, and still
// once the field loses focus (#387).
func TestCaretBlinks(t *testing.T) {
	clock := pinAnimClock(t)
	e := NewEntry(testFace(t), 14, render.RGB(255, 255, 255))
	e.SetText("hello")
	other := newFocusTarget()
	root := NewBox(Column, 0, 0)
	root.Append(e, false).Append(other, false)
	SizeWithin(root, Size{W: 160, H: 80})
	root.Arrange(render.Rect{W: 160, H: 80})
	r := &Router{Root: root}
	if err := r.SetFocus(e); err != nil {
		t.Fatal(err)
	}
	sh := e.shape(e.displayText())
	caret, ok := e.caretRect(sh)
	if !ok {
		t.Fatal("a focused entry has no caret")
	}
	if want := int(sh.Ascent()+0.5) + int(sh.Descent()+0.5); caret.H != want || caret.H >= e.contentRect().H {
		t.Errorf("caret %dpx tall in a %dpx field, want the font's extents %d", caret.H, e.contentRect().H, want)
	}

	CollectDamage(root)
	start := clock.now()
	if !clock.step() || !e.blink.hidden {
		t.Fatal("the caret did not blink off after half a cycle")
	}
	if got := clock.now().Sub(start); got != caretBlinkTime/2 {
		t.Errorf("first blink after %v, want %v", got, caretBlinkTime/2)
	}
	if rects, _ := CollectDamage(root); len(rects) != 1 || rects[0] != caret {
		t.Errorf("the blink damaged %v, want only the caret %v", rects, caret)
	}
	r.Type('!')
	if e.blink.hidden {
		t.Error("typing left the caret hidden")
	}

	toggles := 0
	for clock.step() {
		toggles++
	}
	if e.blink.hidden {
		t.Error("the caret stayed hidden after the blink timeout")
	}
	if want := int(caretBlinkTimeout/(caretBlinkTime/2)) - 1; toggles < want {
		t.Errorf("%d toggles before the timeout, want at least %d", toggles, want)
	}

	if err := r.SetFocus(e); err != nil {
		t.Fatal(err)
	}
	r.Type('?')
	if err := r.SetFocus(other); err != nil {
		t.Fatal(err)
	}
	if e.blink.cancel != nil || e.blink.hidden {
		t.Error("the blink outlived the focus")
	}
}

// Reduced motion keeps the caret solid.
func TestCaretDoesNotBlinkUnderReducedMotion(t *testing.T) {
	t.Cleanup(anim.SetInstant(true))
	e := NewEntry(testFace(t), 14, render.RGB(255, 255, 255))
	root := NewBox(Column, 0, 0)
	root.Append(e, false)
	SizeWithin(root, Size{W: 160, H: 40})
	root.Arrange(render.Rect{W: 160, H: 40})
	if err := (&Router{Root: root}).SetFocus(e); err != nil {
		t.Fatal(err)
	}
	if e.blink.cancel != nil {
		t.Error("a blink started under reduced motion")
	}
}

// Clicking moves the caret and repaints it where it was and where it
// is, nothing more; a drag repaints the rows its selection covers
// (#400).
func TestTextAreaClickRepaintsTheCaret(t *testing.T) {
	ta := NewTextArea(testFace(t), 14, render.RGB(255, 255, 255))
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "a line of text in the area"
	}
	ta.SetText(strings.Join(lines, "\n"))
	root := NewBox(Column, 0, 0)
	root.Append(ta, true)
	arrangeTree(t, root, 300, 600)
	r := &Router{Root: root}
	if err := r.SetFocus(ta); err != nil {
		t.Fatal(err)
	}
	CollectDamage(root)
	old, _ := ta.caretRect()
	ta.ClickAt(Point{X: ta.Bounds().X + 60, Y: ta.Bounds().Y + 200})
	now, _ := ta.caretRect()
	if now == old {
		t.Fatal("the click did not move the caret")
	}
	rects, _ := CollectDamage(root)
	if len(rects) != 2 || !slices.Contains(rects, old) || !slices.Contains(rects, now) {
		t.Errorf("the click damaged %v, want the caret where it was %v and is %v", rects, old, now)
	}
	ta.DragMove(Point{X: ta.Bounds().X + 120, Y: ta.Bounds().Y + 260})
	rects, _ = CollectDamage(root)
	band := render.UnionAll(rects)
	if band.Empty() || band.H >= ta.Bounds().H/2 {
		t.Errorf("the drag damaged %v, want the rows its selection covers", band)
	}
}
