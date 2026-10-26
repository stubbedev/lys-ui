package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/style"
)

// TestCSSAttributeSelectors pins the attribute selector surface on
// real widgets: app-set data attributes, the built-in class and role
// attributes, and the restyle an attribute change owes.
func TestCSSAttributeSelectors(t *testing.T) {
	loadCSS(t, `
		[data-kind="danger"] { background-color: #340009; }
		[class~="urgent"] { color: #003409; }
		[role="button"] { padding-top: 5px; }
	`)
	root := NewBox(Column, 0, 0)
	b1 := NewButton(NewLabel(goldenFace(t), 14, "go", 0), 0, 0)
	b2 := NewButton(NewLabel(goldenFace(t), 14, "stop", 0), 0, 0)
	root.Append(b1, false)
	root.Append(b2, false)
	arrangeTree(t, root, 200, 100)
	if got := StyleBackground(b1); got != 0 {
		t.Errorf("unattributed button matched [data-kind]: %v", got)
	}
	b1.SetAttr("data-kind", "danger")
	if got, want := StyleBackground(b1), render.RGB(0x34, 0, 0x09); got != want {
		t.Errorf("[data-kind=danger]: %v want %v", got, want)
	}
	if v, ok := b1.Attr("data-kind"); !ok || v != "danger" {
		t.Errorf("Attr round-trip: %q %v", v, ok)
	}
	b1.SetAttr("data-kind", "safe")
	if got := StyleBackground(b1); got != 0 {
		t.Errorf("a changed attribute value still matched: %v", got)
	}
	b1.RemoveAttr("data-kind")
	if _, ok := b1.Attr("data-kind"); ok {
		t.Error("RemoveAttr left the attribute behind")
	}
	b2.AddClass("urgent")
	if got, want := CascadeColor(b2), render.RGB(0, 0x34, 0x09); got != want {
		t.Errorf("[class~=urgent]: %v want %v", got, want)
	}
	if got := nodeOf(b1).style(b1).Padding.Top; got != 5 {
		t.Errorf("[role=button] on the button's own role: %d", got)
	}
}

// TestCSSPartPseudoElements pins ::placeholder and ::selection
// styling the entry's parts through the part machinery.
func TestCSSPartPseudoElements(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `
		entry::placeholder { color: #333309; }
		entry::selection { background-color: #093333; }
	`)
	root := NewBox(Column, 0, 0)
	e := NewEntry(face, 14, 0)
	e.SetPlaceholder("name")
	root.Append(e, false)
	arrangeTree(t, root, 200, 40)
	ph := e.text.placeholder.style(&e.text.placeholder)
	if got, want := ph.Color, render.RGB(0x33, 0x33, 0x09); got != want {
		t.Errorf("::placeholder color: %v want %v", got, want)
	}
	sel := e.text.selection.style(&e.text.selection)
	if got, want := sel.Background, render.RGB(0x09, 0x33, 0x33); got != want {
		t.Errorf("::selection background: %v want %v", got, want)
	}
}

// TestCSSPseudoElementContent pins ::before/::after on a widget:
// content strings, attr(), and document-order counters.
func TestCSSPseudoElementContent(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `
		.item { counter-increment: row; }
		.item::before { content: counter(row) ". "; }
		.item::after { content: " [" attr(data-tag) "]"; color: #093309; }
	`)
	root := NewBox(Column, 0, 0)
	items := make([]*Label, 3)
	for i := range items {
		items[i] = NewLabel(face, 14, "row", 0)
		items[i].AddClass("item")
		items[i].SetAttr("data-tag", "t")
		root.Append(items[i], false)
	}
	arrangeTree(t, root, 200, 120)
	for i, it := range items {
		if got, want := PseudoContent(it, "before"), string(rune('1'+i))+". "; got != want {
			t.Errorf("item %d ::before content = %q, want %q", i, got, want)
		}
		if got, want := PseudoContent(it, "after"), " [t]"; got != want {
			t.Errorf("item %d ::after content = %q, want %q", i, got, want)
		}
	}
	pv := PseudoStyle(items[0], "after")
	if got, want := pv.Color, render.RGB(0x09, 0x33, 0x09); got != want {
		t.Errorf("::after color: %v want %v", got, want)
	}
	if PseudoStyle(items[0], "nope") != nil {
		t.Error("an unknown pseudo-element name resolved")
	}
	LoadStylesheet("")
	if PseudoStyle(items[0], "before") != nil {
		t.Error("a pseudo style resolved with no sheet loaded")
	}
}

// TestCSSHasRelational pins :has() on widgets, including the restyle
// a descendant's class change owes the :has ancestor.
func TestCSSHasRelational(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `
		.holder:has(.mark) { background-color: #090934; }
		.holder:has(> .direct) { background-color: #340909; }
	`)
	root := NewBox(Column, 0, 0)
	holder := NewBox(Column, 0, 0)
	holder.AddClass("holder")
	child := NewLabel(face, 14, "x", 0)
	holder.Append(child, false)
	deep := NewBox(Column, 0, 0)
	deep.Append(NewLabel(face, 14, "y", 0), false)
	root.Append(holder, false)
	root.Append(deep, false)
	arrangeTree(t, root, 200, 100)
	if got := StyleBackground(holder); got != 0 {
		t.Errorf(":has matched before the class landed: %v", got)
	}
	child.AddClass("mark")
	if got, want := StyleBackground(holder), render.RGB(0x09, 0x09, 0x34); got != want {
		t.Errorf(":has(.mark) through a descendant's class change: %v want %v", got, want)
	}
	child.RemoveClass("mark")
	child.RemoveClass("direct")
	child.AddClass("direct")
	if got, want := StyleBackground(holder), render.RGB(0x34, 0x09, 0x09); got != want {
		t.Errorf(":has(> .direct): %v want %v", got, want)
	}
	child.RemoveClass("direct")
	deep.AddClass("mark")
	if got := StyleBackground(holder); got != 0 {
		t.Errorf(":has matched another subtree's mark: %v", got)
	}
}

// TestCSSEntryStatePseudos pins :read-only, :read-write and
// :placeholder-shown against the entry states the engine tracks.
func TestCSSEntryStatePseudos(t *testing.T) {
	face := goldenFace(t)
	loadCSS(t, `
		entry:read-only { padding-top: 3px; }
		entry:placeholder-shown { padding-bottom: 4px; }
		entry:read-write { padding-left: 5px; }
	`)
	root := NewBox(Column, 0, 0)
	e := NewEntry(face, 14, 0)
	root.Append(e, false)
	arrangeTree(t, root, 200, 40)
	if got := e.style(e).Padding.Left; got != 5 {
		t.Errorf(":read-write: %d", got)
	}
	e.SetPlaceholder("hint")
	if got := e.style(e).Padding.Bottom; got != 4 {
		t.Errorf(":placeholder-shown on an empty entry: %d", got)
	}
	e.SetText("typed")
	if v := e.style(e); v.Has(style.PropPaddingBottom) {
		t.Error(":placeholder-shown matched a filled entry")
	}
	e.SetReadOnly(true)
	if got := e.style(e).Padding.Top; got != 3 {
		t.Errorf(":read-only: %d", got)
	}
	if v := e.style(e); v.Has(style.PropPaddingLeft) {
		t.Error(":read-write matched a read-only entry")
	}
}
