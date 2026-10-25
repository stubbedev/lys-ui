package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// typeClock steps past the type-ahead idle timeout without disturbing
// the rest of the schedule.
func typeClock(c *animClock, t *testing.T) {
	t.Helper()
	c.set(c.now().Add(typeAheadTimeout + time.Millisecond))
	animclock.Tick(c.now())
}

// TestDropdownTypeAheadOpen pins the open-list behavior (#62): a
// printable prefix jumps the highlight to the first matching row
// without selecting it, repeated keys cycle among equal prefixes,
// Enter picks the highlight, and the idle timeout resets the chain.
func TestDropdownTypeAheadOpen(t *testing.T) {
	c := pinAnimClock(t)
	dd, host := newDropdown(t, "apple", "apricot", "banana", "cherry", "Apple juice")
	var picked []int
	dd.OnSelect = func(i int) { picked = append(picked, i) }

	dd.Open()
	list := openedList(t, dd, host)
	if got := list.highlight; got != 0 {
		t.Fatalf("open highlight = %d, want the selected row 0", got)
	}

	// A prefix jumps to its first match: "ap" lands on apple (the
	// first ap- row), never touching the selection.
	list.InsertRune('a')
	list.InsertRune('p')
	if got := list.highlight; got != 0 {
		t.Fatalf("highlight after \"ap\" = %d, want 0 (apple)", got)
	}
	if dd.Selected() != 0 || len(picked) != 0 {
		t.Error("type-ahead changed the selection while open")
	}

	// Repeated 'a' cycles the equal-prefix rows: apple, apricot, Apple
	// juice, wrap to apple — each extra key resets onto the single
	// letter and advances past the current row.
	for i, want := range []int{1, 4, 0, 1} {
		list.InsertRune('a')
		if got := list.highlight; got != want {
			t.Fatalf("a #%d highlighted %d, want %d", i+2, got, want)
		}
	}

	// Enter picks the highlighted row.
	list.KeyAction(KeyEnter, 0)
	if dd.Selected() != 1 || dd.Opened() {
		t.Fatalf("selected = %d (open %v) after Enter, want 1, closed", dd.Selected(), dd.Opened())
	}

	// Case-insensitive: "B" jumps to banana.
	dd.Open()
	list = openedList(t, dd, host)
	list.InsertRune('B')
	if got := list.highlight; got != 2 {
		t.Errorf("highlight after \"B\" = %d, want 2 (banana)", got)
	}

	// The idle timeout clears the chain: a later key starts fresh.
	typeClock(c, t)
	if list.typeAhead.typed != "" {
		t.Errorf("chain %q survived the idle timeout", list.typeAhead.typed)
	}
	list.InsertRune('a')
	if got := list.highlight; got != 0 {
		t.Errorf("post-timeout a highlighted %d, want a fresh 0", got)
	}
}

// TestDropdownTypeAheadClosed pins the closed-face decision: printable
// keys first-letter-cycle the selection (firing OnSelect), while Space
// still only opens.
func TestDropdownTypeAheadClosed(t *testing.T) {
	c := pinAnimClock(t)
	dd, _ := newDropdown(t, "apple", "apricot", "banana")
	var picked []int
	dd.OnSelect = func(i int) { picked = append(picked, i) }

	dd.InsertRune('b')
	if dd.Selected() != 2 {
		t.Fatalf("closed b selected %d, want 2", dd.Selected())
	}
	if len(picked) != 1 {
		t.Errorf("closed cycling fired OnSelect %d times, want 1", len(picked))
	}

	// A different letter starts a fresh chain only after the idle
	// timeout; before it, the letters extend the running prefix.
	dd.InsertRune('a')
	if dd.Selected() != 2 {
		t.Errorf("ba extended the prefix onto banana's row: selected %d, want 2", dd.Selected())
	}
	typeClock(c, t)
	dd.InsertRune('a')
	if dd.Selected() != 0 {
		t.Errorf("fresh a selected %d, want 0", dd.Selected())
	}
	dd.InsertRune('a')
	if dd.Selected() != 1 {
		t.Errorf("repeated closed a selected %d, want cycling to 1", dd.Selected())
	}

	// Space never joins the chain: closed, it opens.
	typeClock(c, t)
	dd.InsertRune(' ')
	if !dd.Opened() {
		t.Fatal("Space stopped opening the closed dropdown")
	}
	if dd.typeAhead.typed != "" {
		t.Errorf("Space joined the type-ahead chain: %q", dd.typeAhead.typed)
	}
}

// TestDropdownTypeAheadUnmatchedKeysPassThrough pins that a key with
// no matching rows interferes with nothing: the chain, the highlight,
// and the selection stay put, and the navigation keys keep working.
func TestDropdownTypeAheadUnmatchedKeysPassThrough(t *testing.T) {
	pinAnimClock(t)
	dd, host := newDropdown(t, "apple", "banana")
	dd.InsertRune('q')
	if dd.typeAhead.typed != "" || dd.Selected() != 0 {
		t.Error("an unmatched key on the face left state behind")
	}

	dd.Open()
	list := openedList(t, dd, host)
	list.InsertRune('q')
	if list.typeAhead.typed != "" || list.highlight != 0 {
		t.Errorf("an unmatched key left %q, highlight %d", list.typeAhead.typed, list.highlight)
	}
	list.KeyAction(KeyDown, 0)
	if list.highlight != 1 {
		t.Fatalf("arrows stopped navigating after an unmatched key: highlight %d", list.highlight)
	}
	host.opts[0].OnClosed()
	if dd.Opened() {
		t.Error("the closed popover left the dropdown open")
	}

	// Space while closed still opens (the router's text path lives).
	dd.InsertRune(' ')
	if !dd.Opened() {
		t.Error("Space-opens broke after unmatched type-ahead")
	}
}
