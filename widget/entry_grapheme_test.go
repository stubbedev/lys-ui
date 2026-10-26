package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// Grapheme cluster editing: the editing unit is the grapheme cluster
// (UAX #29), not the rune. A ZWJ family emoji is five runes and one
// character; a flag is two; an e plus combining acute is two runes and
// one accented character. Backspace, Delete, and caret motion move one
// cluster; selection edges land on cluster boundaries; word motion and
// double-click compose with it (#56) because words are cluster runs.
const (
	familyEmoji = "👨‍👩‍👧"   // 5 runes, 1 cluster
	flagEmoji   = "🇩🇪"      // 2 runes, 1 cluster
	tonedEmoji  = "👍🏽"      // 2 runes, 1 cluster (skin-tone modifier)
	accentedE   = "e\u0301" // 2 runes, 1 cluster
)

func TestEntryGraphemeEditing(t *testing.T) {
	newGraphemeEntry := func(t *testing.T, s string) *Entry {
		t.Helper()
		e := NewEntry(entryFace(t), 14, render.RGB(255, 255, 255))
		e.SetText(s)
		return e
	}

	t.Run("backspace removes a ZWJ family emoji whole", func(t *testing.T) {
		e := newGraphemeEntry(t, "a"+familyEmoji+"b")
		e.MoveEnd()
		e.Backspace() // the b
		if got := e.Text(); got != "a"+familyEmoji {
			t.Fatalf("text = %q, want a+%q", got, familyEmoji)
		}
		e.Backspace() // the whole family, not its last rune
		if got := e.Text(); got != "a" {
			t.Fatalf("text = %q, want a (family removed whole)", got)
		}
		if got := e.Cursor(); got != 1 {
			t.Fatalf("cursor = %d, want 1", got)
		}
	})

	t.Run("backspace removes a flag emoji whole", func(t *testing.T) {
		e := newGraphemeEntry(t, "x"+flagEmoji)
		e.MoveEnd()
		e.Backspace()
		if got := e.Text(); got != "x" {
			t.Fatalf("text = %q, want x (flag removed whole)", got)
		}
	})

	t.Run("backspace removes a skin-tone modifier with its base whole", func(t *testing.T) {
		e := newGraphemeEntry(t, tonedEmoji+"!")
		e.MoveEnd()
		e.Backspace() // !
		e.Backspace() // the toned emoji as one unit
		if got := e.Text(); got != "" {
			t.Fatalf("text = %q, want empty (toned emoji removed whole)", got)
		}
	})

	t.Run("delete removes a family emoji whole", func(t *testing.T) {
		e := newGraphemeEntry(t, "a"+familyEmoji+"b")
		e.MoveHome()
		e.Delete() // the a
		e.Delete() // the whole family
		if got := e.Text(); got != "b" {
			t.Fatalf("text = %q, want b (family removed whole)", got)
		}
	})

	t.Run("e plus combining acute edits as one unit", func(t *testing.T) {
		e := newGraphemeEntry(t, "caf"+accentedE+"x")
		e.MoveEnd()
		e.Backspace() // x
		e.Backspace()
		if got := e.Text(); got != "caf" {
			t.Fatalf("text = %q, want caf (accented e removed whole)", got)
		}
	})

	t.Run("caret steps once per cluster", func(t *testing.T) {
		e := newGraphemeEntry(t, "a"+familyEmoji+"b")
		e.MoveHome()
		for _, want := range []int{1, 6, 7, 7} {
			e.MoveCursor(1)
			if got := e.Cursor(); got != want {
				t.Fatalf("right cursor = %d, want %d", got, want)
			}
		}
		for _, want := range []int{6, 1, 0, 0} {
			e.MoveCursor(-1)
			if got := e.Cursor(); got != want {
				t.Fatalf("left cursor = %d, want %d", got, want)
			}
		}
	})

	t.Run("shift+arrow selection spans cluster boundaries exactly", func(t *testing.T) {
		e := newGraphemeEntry(t, "a"+familyEmoji+"b")
		e.MoveHome()
		e.MoveCursor(1)
		e.MoveCursorExtending(1) // over the family
		start, end, active := e.Selection()
		if !active || start != 1 || end != 6 {
			t.Fatalf("selection = %d..%d active=%v, want 1..6 true", start, end, active)
		}
		if got, _ := e.SelectedText(); got != familyEmoji {
			t.Fatalf("selected = %q, want the family whole", got)
		}
		e.MoveCursorExtending(1) // over the b
		if got, _ := e.SelectedText(); got != familyEmoji+"b" {
			t.Fatalf("selected = %q, want family+b", got)
		}
		e.MoveCursorExtending(-1)
		if got, _ := e.SelectedText(); got != familyEmoji {
			t.Fatalf("selected = %q, want family again", got)
		}
	})

	t.Run("a cluster delete is one undo entry", func(t *testing.T) {
		e := newGraphemeEntry(t, "a"+familyEmoji+"b")
		e.MoveEnd()
		e.MoveCursor(-1) // park before the family
		e.Backspace()    // the family, whole
		if got := e.Text(); got != "ab" {
			t.Fatalf("text = %q, want ab", got)
		}
		if !e.Undo() {
			t.Fatal("undo must restore the family")
		}
		if got := e.Text(); got != "a"+familyEmoji+"b" {
			t.Fatalf("after undo text = %q, want a+%q+b", got, familyEmoji)
		}
		if e.Undo() {
			t.Fatal("a cluster delete is exactly one undo entry")
		}
	})

	t.Run("insert counts the cluster: caret math and backspace agree", func(t *testing.T) {
		e := newGraphemeEntry(t, "")
		e.Insert(familyEmoji)
		if got := e.Cursor(); got != 5 {
			t.Fatalf("cursor = %d, want 5 (rune end of one inserted cluster)", got)
		}
		e.Backspace()
		if got := e.Text(); got != "" {
			t.Fatalf("text = %q, want empty (one cluster deleted)", got)
		}
	})

	t.Run("inserting before a combining mark snaps the caret to the cluster end", func(t *testing.T) {
		e := newGraphemeEntry(t, "\u0301x")
		e.MoveHome()
		e.Insert("e") // composes with the following mark
		if got := e.Text(); got != "e\u0301x" {
			t.Fatalf("text = %q, want e+mark+x", got)
		}
		if got := e.Cursor(); got != 2 {
			t.Fatalf("cursor = %d, want 2 (after the composed cluster)", got)
		}
		e.Backspace()
		if got := e.Text(); got != "x" {
			t.Fatalf("text = %q, want x (composed cluster removed whole)", got)
		}
	})

	t.Run("click and drag land on cluster starts", func(t *testing.T) {
		e := newGraphemeEntry(t, "a"+familyEmoji+"b")
		sh := e.face.Shape("a"+familyEmoji+"b", 14)
		e.ClickAt(Point{X: entryPad.Left + int(sh.CaretX(6)), Y: 15})
		if got := e.Cursor(); got != 6 {
			t.Fatalf("click cursor = %d, want 6", got)
		}
		// A click inside the family snaps back to its start.
		e.ClickAt(Point{X: entryPad.Left + int(sh.CaretX(3)), Y: 15})
		if got := e.Cursor(); got != 1 {
			t.Fatalf("mid-cluster click cursor = %d, want 1", got)
		}
	})

	t.Run("word motion composes: the accented cluster is one word unit", func(t *testing.T) {
		e := newGraphemeEntry(t, "caf"+accentedE+" blues")
		e.MoveHome()
		e.KeyAction(KeyRight, ModCtrl)
		if got := e.Cursor(); got != 5 {
			t.Fatalf("ctrl+right cursor = %d, want 5 (past the accented cluster)", got)
		}
		e.KeyAction(KeyBackspace, ModCtrl)
		if got := e.Text(); got != " blues" {
			t.Fatalf("text = %q, want ' blues'", got)
		}
	})

	t.Run("double-click selects the accented cluster's word", func(t *testing.T) {
		e := newGraphemeEntry(t, "caf"+accentedE+" blues")
		sh := e.face.Shape("caf"+accentedE+" blues", 14)
		e.DoubleClickAt(Point{X: entryPad.Left + int(sh.CaretX(2)), Y: 15})
		if got, _ := e.SelectedText(); got != "caf"+accentedE {
			t.Fatalf("selected = %q, want caf+accented cluster", got)
		}
	})

	t.Run("echo masking keeps cluster math logical", func(t *testing.T) {
		e := newGraphemeEntry(t, "x"+familyEmoji)
		e.SetEcho(EchoPassword)
		if got, want := e.displayText(), strings.Repeat(passwordDot, 6); got != want {
			t.Fatalf("display = %q, want %q", got, want)
		}
		e.MoveEnd()
		e.Backspace() // family whole, under masking
		if got := e.Text(); got != "x" {
			t.Fatalf("text = %q, want x (family removed whole while masked)", got)
		}
		if got, want := e.displayText(), passwordDot; got != want {
			t.Fatalf("display = %q, want %q", got, want)
		}
	})
}
