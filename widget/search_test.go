package widget

import (
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/lys-ui/anim"
)

// searchFace builds the test font.
func searchFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestEntryCompletionLifecycle pins the completion contract: typing
// opens the list under a matching prefix, arrows move the highlight
// clamped, Enter picks (text becomes the match, OnChanged then
// OnComplete fire, the list closes), Esc closes without changing the
// text, an exact single match and an empty prefix never open.
func TestEntryCompletionLifecycle(t *testing.T) {
	defer anim.SetInstant(true)()
	anim.SetInstant(true)
	face := searchFace(t)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	all := []string{"apple", "apricot", "avocado"}
	e.Completion = func(prefix string) []string {
		var out []string
		for _, s := range all {
			if strings.HasPrefix(s, prefix) {
				out = append(out, s)
			}
		}
		return out
	}

	e.SetText("a")
	if !e.completionOpen() {
		t.Fatal("a matching prefix did not open the list")
	}
	if len(e.complete) != 3 || e.complete[0] != "apple" {
		t.Fatalf("matches = %v", e.complete)
	}

	// Arrows move, clamped at both ends.
	e.KeyAction(KeyUp, 0)
	if e.completeSel != 0 {
		t.Errorf("up above the first row moved to %d", e.completeSel)
	}
	e.KeyAction(KeyDown, 0)
	e.KeyAction(KeyDown, 0)
	e.KeyAction(KeyDown, 0)
	if e.completeSel != 2 {
		t.Errorf("down past the last row moved to %d", e.completeSel)
	}

	// Enter picks the highlighted row.
	changes, picked := 0, ""
	e.OnChanged = func(s string) { changes++; last = s }
	e.OnComplete = func(s string) { picked = s }
	e.KeyAction(KeyEnter, 0)
	if e.Text() != "avocado" || picked != "avocado" {
		t.Errorf("pick: text=%q picked=%q", e.Text(), picked)
	}
	if e.completionOpen() {
		t.Error("the list stayed open after the pick")
	}

	// Esc closes without touching the text.
	e.SetText("ap")
	if !e.completionOpen() {
		t.Fatal("reopening failed")
	}
	e.OnEscape = func() bool { return false }
	e.KeyAction(KeyDismiss, 0)
	if e.completionOpen() {
		t.Error("Esc did not close the list")
	}
	if e.Text() != "ap" {
		t.Errorf("Esc changed the text: %q", e.Text())
	}

	// An exact match and an empty prefix never open.
	e.SetText("apple")
	if e.completionOpen() {
		t.Error("an exact single match opened the list")
	}
	e.SetText("")
	if e.completionOpen() {
		t.Error("an empty prefix opened the list")
	}
}

var last string

// TestEntryCompletionEscapeHook pins the OnEscape contract: it fires
// only while the list is closed, and consuming stops the key.
func TestEntryCompletionEscapeHook(t *testing.T) {
	face := searchFace(t)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	pressed := 0
	e.SetText("start")
	e.OnEscape = func() bool {
		pressed++
		return pressed == 1
	}
	e.KeyAction(KeyDismiss, 0)
	if pressed != 1 {
		t.Fatalf("OnEscape fired %d times", pressed)
	}
	// The second Esc is not consumed; the entry just ignores it.
	e.KeyAction(KeyDismiss, 0)
	if pressed != 2 {
		t.Errorf("OnEscape stopped firing: %d", pressed)
	}
}

// TestEntryCompletionClickPick pins the pointer half: a click inside
// the list picks its row and never moves the caret.
func TestEntryCompletionClickPick(t *testing.T) {
	face := searchFace(t)
	e := NewEntry(face, 14, render.RGB(255, 255, 255))
	e.Completion = func(string) []string { return []string{"one", "two", "three"} }
	e.SetText("t")
	SizeWithin(e, Size{W: 200, H: 300})
	e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: e.Bounds().H + e.completionHeight()})
	list := e.completionListRect()
	if list.Empty() {
		t.Fatal("the list has no rect")
	}
	if hit := e.HitTest(Point{X: list.X + 4, Y: list.Y + list.H - 2}); hit == nil {
		t.Error("a click in the list did not hit the entry")
	}
	e.ClickAt(Point{X: list.X + 4, Y: list.Y + 2*e.completionRowH() + 2})
	if e.Text() != "three" {
		t.Errorf("click pick: text = %q", e.Text())
	}
}

// TestSearchEntry pins the search preset: leading icon and clear
// button presence, Esc clearing once then passing on, and the delayed
// signal firing once typing settles (instant clock in tests).
func TestSearchEntry(t *testing.T) {
	defer anim.SetInstant(true)()
	anim.SetInstant(true)
	face := searchFace(t)
	s := NewSearchEntry(face, 14, "find")
	if s.leading == nil {
		t.Error("the search icon is missing")
	}
	fired := 0
	s.OnSearchChanged = func(string) { fired++ }

	s.SetText("query")
	if fired != 1 {
		t.Errorf("settled signal fired %d times, want 1", fired)
	}
	if s.trailing == nil {
		t.Error("the clear button did not appear")
	}

	// Esc clears once, then passes on.
	s.KeyAction(KeyDismiss, 0)
	if s.Text() != "" || fired != 2 {
		t.Errorf("after Esc: text=%q fired=%d", s.Text(), fired)
	}
	s.KeyAction(KeyDismiss, 0)
	if fired != 2 {
		t.Error("Esc on an empty field still consumed and fired")
	}

	// Typing again re-shows the clear button; clearing hides it.
	s.SetText("x")
	if s.trailing == nil {
		t.Error("the clear button did not return")
	}
	s.trailingClick(Point{})
	if s.Text() != "" || s.trailing != nil {
		t.Errorf("clear click: text=%q trailing=%v", s.Text(), s.trailing)
	}
}

// TestSearchBar pins the slide-down container: hidden at start,
// search mode flips the reveal, the entry is reachable.
func TestSearchBar(t *testing.T) {
	face := searchFace(t)
	b := NewSearchBar(face, 14)
	if b.SearchMode() {
		t.Error("the bar starts revealed")
	}
	b.SetSearchMode(true)
	if !b.SearchMode() {
		t.Error("SetSearchMode did not reveal")
	}
	if b.Entry() == nil {
		t.Error("no entry")
	}
	b.Entry().SetText("q")
	b.SetSearchMode(false)
	if b.Entry().Text() != "q" {
		t.Error("hiding the bar cleared the field")
	}
}

// TestComboEntry pins the editable dropdown: prefix matches list
// case-insensitively, the chevron opens everything, a pick selects,
// and unmatched free text stays.
func TestComboEntry(t *testing.T) {
	face := searchFace(t)
	c := NewComboEntry(face, 14, []string{"Apple", "Apricot", "Banana"}, "Apple")
	selected := ""
	c.OnSelected = func(s string) { selected = s }

	c.SetText("ba")
	if !c.completionOpen() || len(c.complete) != 1 || c.complete[0] != "Banana" {
		t.Fatalf("prefix match: open=%v matches=%v", c.completionOpen(), c.complete)
	}
	c.KeyAction(KeyEnter, 0)
	if c.Text() != "Banana" || selected != "Banana" {
		t.Errorf("pick: text=%q selected=%q", c.Text(), selected)
	}

	// The chevron's full list ignores the prefix.
	c.toggleAll()
	if !c.completionOpen() || len(c.complete) != 3 {
		t.Fatalf("full list: %v", c.complete)
	}
	c.toggleAll()
	if c.completionOpen() {
		t.Error("toggle did not close")
	}

	// Free text survives unmatched.
	c.SetText("Cherry")
	if c.completionOpen() {
		t.Error("unmatched free text opened the list")
	}
	if c.Text() != "Cherry" {
		t.Errorf("free text changed: %q", c.Text())
	}
}

// TestGoldenSearch pins the search entry and an open completion
// list's painted look.
func TestGoldenSearch(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	s := NewSearchEntry(face, 14, "search")
	s.SetText("query")
	NewGolden(t, s.Entry, "searchentry", goldenTheme(th), goldenFrame(200, 40))

	e := NewEntry(face, 14, th.Text)
	e.Completion = func(string) []string { return []string{"apple", "apricot", "avocado"} }
	e.SetText("ap")
	if !e.completionOpen() {
		t.Fatal("golden setup: list did not open")
	}
	NewGolden(t, e, "entry-completion", goldenTheme(th), goldenFrame(200, 120))
}
