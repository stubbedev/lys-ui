package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/render"
)

// A lysdebug build names each measure-cache miss by reason and widget
// type, and a release build reports nothing (#388).
func TestMeasureMissReport(t *testing.T) {
	MeasureMissReport()
	inner := NewBox(Column, 0, 0)
	inner.Append(newStub(10, 10), false)
	root := NewBox(Column, 0, 0)
	root.Append(inner, false)
	SizeWithin(root, Size{W: 100, H: 100})
	MeasureMissReport()
	SizeWithin(root, Size{W: 100, H: 100})
	if report := MeasureMissReport(); report != "" {
		t.Errorf("a cached layout reported %q", report)
	}
	inner.Append(newStub(10, 10), false)
	SizeWithin(root, Size{W: 100, H: 100})
	report := MeasureMissReport()
	if !debug.Enabled {
		if report != "" {
			t.Errorf("a release build reported %q", report)
		}
		return
	}
	if !strings.Contains(report, "uncached *widget.Box") {
		t.Errorf("report %q, want the box invalidated by its new child", report)
	}
	if MeasureMissReport() != "" {
		t.Error("the report did not clear")
	}
}

// computeCounter counts the measures it actually computes, past the
// cache.
type computeCounter struct {
	node
	computed int
}

func (c *computeCounter) Measure(axis Axis, forSize int) Sizes {
	if s, ok := c.measureHit(axis, forSize); ok {
		return s
	}
	c.computed++
	return c.measureStore(axis, forSize, FixedSizes(c.computed))
}
func (c *computeCounter) Paint(*render.Canvas) {}
func (c *computeCounter) HitTest(Point) Widget { return nil }

// A page in a vertical scroll beside a sidebar is asked its height at
// four widths each frame (the row's whole width and the width the
// sidebar leaves, each with and without the scrollbar gutter): all four
// stay cached, and a fifth evicts the least recently used (#397).
func TestMeasureCacheHoldsFourQueries(t *testing.T) {
	c := &computeCounter{}
	widths := []int{1812, 1820, 1556, 1564}
	for range 3 {
		for _, w := range widths {
			c.Measure(Column, w)
		}
	}
	if c.computed != 4 {
		t.Fatalf("three passes over four widths computed %d times, want 4", c.computed)
	}
	c.Measure(Column, 900)
	c.Measure(Column, widths[3])
	c.Measure(Column, widths[2])
	c.Measure(Column, widths[1])
	if c.computed != 5 {
		t.Errorf("computed %d times, want 5: the most recent widths kept", c.computed)
	}
	c.Measure(Column, widths[0])
	if c.computed != 6 {
		t.Errorf("computed %d times, want 6: the least recently used set evicted", c.computed)
	}
}

// After a style change the first re-measure must not revalidate the
// sizes the other widths were measured at under the old style (#391).
func TestMeasureCacheDropsEveryEntryOnAStyleChange(t *testing.T) {
	c := &computeCounter{}
	c.Measure(Column, 100)
	c.Measure(Column, 200)
	styleGen++
	c.Measure(Column, 100)
	if got := c.Measure(Column, 200); got.Nat != 4 {
		t.Errorf("width 200 after the style change returned the old request %v, want a fresh measure", got)
	}
}

// A lysdebug build names the widgets that owed their whole bounds and
// why, largest first, and a release build reports nothing (#399).
func TestDamageReport(t *testing.T) {
	inner := NewBox(Column, 0, 0)
	inner.Append(newStub(10, 10), false)
	root := NewBox(Column, 0, 0)
	root.Append(inner, false)
	arrangeTree(t, root, 120, 80)
	CollectDamage(root)
	DamageReport()
	root.Invalidate()
	CollectDamage(root)
	report := DamageReport()
	if !debug.Enabled {
		if report != "" {
			t.Errorf("a release build reported %q", report)
		}
		return
	}
	if !strings.HasPrefix(report, "120x80 *widget.Box invalid") {
		t.Errorf("report %q, want the invalidated root first", report)
	}
	if DamageReport() != "" {
		t.Error("the report did not clear")
	}

	SetRootFontSize(RootFontSize() + 2)
	t.Cleanup(func() { SetRootFontSize(RootFontSize() - 2) })
	CollectDamage(root)
	if report := DamageReport(); !strings.HasSuffix(report, "stylegen bumped by: root font size (x1)") {
		t.Errorf("report %q, want the root font size named as what restyled every widget", report)
	}
}

// Attaching a stylesheet restyles only the subtree it styles: another
// window's tree owes nothing, and the style generation stands, so no
// window repaints whole (#401).
func TestAttachStylesheetRestylesOnlyItsSubtree(t *testing.T) {
	face := testFace(t)
	other := NewBox(Column, 0, 0)
	other.Append(NewLabel(face, 14, "elsewhere", 0), false)
	arrangeTree(t, other, 200, 40)
	label := NewLabel(face, 14, "popover", 0)
	inner := NewBox(Column, 0, 0)
	inner.Append(label, false)
	root := NewBox(Column, 0, 0)
	root.Append(inner, false)
	arrangeTree(t, root, 200, 40)
	CollectDamage(other)
	CollectDamage(root)
	gen := styleGen

	inner.AttachStylesheet(NewStylesheet(`label { color: #ff0000; }`, StylePriorityUser))
	if styleGen != gen {
		t.Error("attaching a sheet to one subtree bumped the style generation")
	}
	if rects, _ := CollectDamage(other); len(rects) != 0 {
		t.Errorf("another tree owes %v after a sheet came to an unrelated subtree", rects)
	}
	if got := label.style(label).Color; got != render.RGB(0xff, 0, 0) {
		t.Errorf("the label under the sheet is %v, want its red", got)
	}

	fresh := NewBox(Column, 0, 0)
	fresh.Append(NewLabel(face, 14, "unstyled", 0), false)
	fresh.AttachStylesheet(NewStylesheet(`label { color: #00ff00; }`, StylePriorityUser))
	if styleGen != gen {
		t.Error("attaching a sheet to a root never styled bumped the style generation")
	}
}

// A state flip on a tree's root restyles that tree alone: the root
// learns its widget when it first styles, so the flip walks its
// subtree and leaves the style generation standing. A focused popover
// list is its popup's root, and every focus flip on it repainted every
// window whole.
func TestRootStateFlipRestylesOnlyItsTree(t *testing.T) {
	face := testFace(t)
	sheet := AddStylesheet(`.picked label { color: #ff0000; }`, StylePriorityUser)
	t.Cleanup(sheet.Remove)
	other := NewBox(Column, 0, 0)
	other.Append(NewLabel(face, 14, "elsewhere", 0), false)
	arrangeTree(t, other, 200, 40)
	label := NewLabel(face, 14, "popover", 0)
	root := NewBox(Column, 0, 0)
	root.Append(label, false)
	arrangeTree(t, root, 200, 40)
	CollectDamage(other)
	gen := styleGen

	root.AddClass("picked")
	if styleGen != gen {
		t.Error("a class flip on a root bumped the style generation")
	}
	if rects, _ := CollectDamage(other); len(rects) != 0 {
		t.Errorf("another tree owes %v after a root in a different tree flipped", rects)
	}
	if got := label.style(label).Color; got != render.RGB(0xff, 0, 0) {
		t.Errorf("the label under the flipped root is %v, want its red", got)
	}
}

// A stylesheet attached to an already styled root reaches the pages
// its stacks are not showing: they keep their cascades, and a page
// shown later paints with the sheet.
func TestAttachStylesheetReachesHiddenPages(t *testing.T) {
	face := testFace(t)
	hidden := NewLabel(face, 14, "hidden", 0)
	stack := NewStack()
	stack.Add("shown", NewLabel(face, 14, "shown", 0))
	stack.Add("hidden", hidden)
	stack.Show("shown")
	root := NewBox(Column, 0, 0)
	root.Append(stack, false)
	arrangeTree(t, root, 200, 40)
	hidden.style(hidden)

	root.AttachStylesheet(NewStylesheet(`label { color: #ff0000; }`, StylePriorityUser))
	if got := hidden.style(hidden).Color; got != render.RGB(0xff, 0, 0) {
		t.Errorf("the hidden page's label is %v, want the attached sheet's red", got)
	}
}
