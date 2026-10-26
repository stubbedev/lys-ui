package widget

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// TestEntryOnActivate: Enter fires OnActivate with the contents; a
// disabled entry does not activate, and other keys never do.
func TestEntryOnActivate(t *testing.T) {
	e := NewEntry(testFace(t), 14, render.RGB(255, 255, 255))
	var got []string
	e.OnActivate = func(s string) { got = append(got, s) }
	e.SetText("hunter2")
	e.KeyAction(KeyEnter, 0)
	e.KeyAction(KeyLeft, 0)
	if len(got) != 1 || got[0] != "hunter2" {
		t.Fatalf("activations = %q, want one with the contents", got)
	}
	e.SetEnabled(false)
	e.KeyAction(KeyEnter, 0)
	if len(got) != 1 {
		t.Error("a disabled entry activated")
	}
	plain := NewEntry(testFace(t), 14, render.RGB(255, 255, 255))
	plain.KeyAction(KeyEnter, 0) // no hook: a no-op, no panic
}
