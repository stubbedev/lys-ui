package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// The leading icon is GtkEntry's primary icon: it takes its width from
// the text area's start, paints from the cascade, and a press over it
// runs its handler instead of placing the cursor.
func TestEntryLeadingIcon(t *testing.T) {
	loadCSS(t, `entry { padding: 4; } entry > image { color: #010203; min-width: 12; min-height: 12; }`)
	clicked := 0
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	e.SetLeadingIcon("tb-key-symbolic", 14, func() { clicked++ })
	arrangeTree(t, e, 200, 32)
	if e.LeadingIcon() == nil || e.TrailingIcon() != nil {
		t.Fatalf("icons %v/%v, want a leading one only", e.LeadingIcon(), e.TrailingIcon())
	}
	lr := e.leadingRect()
	if lr.Empty() || lr.X < e.Bounds().X || lr.X > e.Bounds().X+e.Bounds().W/2 {
		t.Errorf("leading icon at %v, want inside the field's start half", lr)
	}
	plain := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	arrangeTree(t, plain, 200, 32)
	if got := plain.contentRect().W - e.contentRect().W; got < 12 {
		t.Errorf("the leading icon only took %d px from the text area, want at least the icon's 12", got)
	}
	if n := len(styleKids(e)); n != 2 {
		t.Errorf("style children = %d, want the text node and the icon", n)
	}
	e.ClickAt(Point{X: lr.X + lr.W/2, Y: lr.Y + lr.H/2})
	if clicked != 1 {
		t.Errorf("a press over the leading icon clicked %d times, want once", clicked)
	}
}

// Clearing the leading icon gives the width back and drops the child.
func TestEntryLeadingIconClears(t *testing.T) {
	e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	e.SetLeadingIcon("tb-key-symbolic", 14, nil)
	e.SetLeadingIcon("", 14, nil)
	if e.LeadingIcon() != nil || len(styleKids(e)) != 1 {
		t.Fatal("a cleared leading icon stayed")
	}
	arrangeTree(t, e, 200, 32)
	plain := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
	arrangeTree(t, plain, 200, 32)
	if e.contentRect().W != plain.contentRect().W {
		t.Errorf("cleared leading icon still takes width: %d vs %d", e.contentRect().W, plain.contentRect().W)
	}
}

// The leading icon sits before the text area, never over it.
func TestEntryLeadingIconClearsText(t *testing.T) {
	e := NewEntry(goldenFace(t), 14, DarkTheme().Text)
	e.SetLeadingIcon("system-search-symbolic", 14, nil)
	e.SetText("query")
	e.Measure(Constraints{Max: Size{W: 200, H: 40}})
	e.Arrange(render.Rect{W: 200, H: 30})
	if ic, text := e.leadingRect(), e.contentRect(); ic.X+ic.W > text.X || ic.X < e.bounds.X {
		t.Errorf("icon %+v overlaps text %+v", ic, text)
	}
}
