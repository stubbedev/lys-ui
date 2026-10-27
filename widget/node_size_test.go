package widget

import (
	"testing"
	"unsafe"
)

// Every widget embeds a node, so its size multiplies by every widget
// in every window: pseudo-element styles, 13 KB a set, are allocated
// only where a rule addresses them, and the node stays under budget
// (#410).
func TestNodeSizeBudget(t *testing.T) {
	const budget = 6 << 10
	if got := unsafe.Sizeof(node{}); got > budget {
		t.Errorf("node is %d bytes, budget %d", got, budget)
	}
}

// A widget keeps a pseudo-element set only while a rule addresses one
// of its pseudo-elements; others carry none even with such rules
// loaded, and the set goes when the rule does.
func TestPseudoStylesOnlyWhereAddressed(t *testing.T) {
	face := testFace(t)
	marked := NewLabel(face, 14, "marked", 0)
	marked.AddClass("mark")
	plain := NewLabel(face, 14, "plain", 0)
	root := NewBox(Column, 0, 0)
	root.Append(marked, false).Append(plain, false)
	loadCSS(t, `.mark::before { content: "*"; }`)
	arrangeTree(t, root, 200, 60)
	marked.style(marked)
	plain.style(plain)
	if marked.pseudo == nil || PseudoContent(marked, "before") != "*" {
		t.Errorf("the addressed label lost its ::before (%q)", PseudoContent(marked, "before"))
	}
	if plain.pseudo != nil || PseudoStyle(plain, "before") != nil {
		t.Error("a label no pseudo-element rule addresses keeps a pseudo-element set")
	}
	marked.RemoveClass("mark")
	marked.style(marked)
	if marked.pseudo != nil {
		t.Error("the set outlived the rule that addressed it")
	}
}
