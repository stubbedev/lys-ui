package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/xkb"
)

// testTable parses the checked-in fixture: three sequences, an accent
// pair, a spacing accent, and a three-key chain.
func testTable(t *testing.T) *Table {
	t.Helper()
	tb, err := NewTableFile(filepath.Join("testdata", "XCompose"))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return tb
}

// feedAll feeds sym in order and returns the last result.
func feedAll(s *State, syms ...xkb.Keysym) (Result, string) {
	var res Result
	var text string
	for _, sym := range syms {
		res, text = s.Feed(sym)
	}
	return res, text
}

func TestAccentSequenceCommitsOnCompletion(t *testing.T) {
	s := testTable(t).Start()
	res, text := s.Feed(xkb.KeyDeadAcute)
	if res != Pending || text != "" {
		t.Fatalf("dead_acute = %v %q, want pending with no text", res, text)
	}
	if !s.Composing() {
		t.Fatal("sequence should be in flight after dead_acute")
	}
	res, text = s.Feed('e')
	if res != Done || text != "é" {
		t.Fatalf("dead_acute+e = %v %q, want done \"é\"", res, text)
	}
	if s.Composing() {
		t.Fatal("sequence should be over after the commit")
	}
}

func TestDeadKeyPlusSpaceCommitsSpacingAccent(t *testing.T) {
	s := testTable(t).Start()
	if res, text := feedAll(s, xkb.KeyDeadAcute, xkb.KeySpace); res != Done || text != "´" {
		t.Fatalf("dead_acute+space = %v %q, want done \"´\"", res, text)
	}
}

func TestMultiKeySequence(t *testing.T) {
	s := testTable(t).Start()
	for _, sym := range []xkb.Keysym{'s', 't'} {
		if res, text := s.Feed(sym); res != Pending || text != "" {
			t.Fatalf("feed %v = %v %q, want pending with no text", sym, res, text)
		}
	}
	if res, text := s.Feed('r'); res != Done || text != "★" {
		t.Fatalf("s+t+r = %v %q, want done \"★\"", res, text)
	}
}

func TestNonSequenceKeysymCancelsAndTypes(t *testing.T) {
	s := testTable(t).Start()
	if res, _ := s.Feed(xkb.KeyDeadAcute); res != Pending {
		t.Fatalf("dead_acute = %v, want pending", res)
	}
	// x conflicts with every dead_acute sequence: the sequence is
	// dropped and the key itself is reported as none — normal text.
	if res, text := s.Feed('x'); res != None || text != "" {
		t.Fatalf("x after dead_acute = %v %q, want none", res, text)
	}
	if s.Composing() {
		t.Fatal("sequence should be cancelled")
	}
	// The machine is fresh: x is plain text, and a new sequence starts.
	if res, _ := s.Feed('x'); res != None {
		t.Fatalf("x while idle = %v, want none", res)
	}
	if res, _ := s.Feed(xkb.KeyDeadAcute); res != Pending {
		t.Fatalf("dead_acute after cancel = %v, want pending", res)
	}
}

func TestBackspaceUnwindsOneLevel(t *testing.T) {
	s := testTable(t).Start()
	for _, sym := range []xkb.Keysym{'s', 't'} {
		if res, _ := s.Feed(sym); res != Pending {
			t.Fatalf("feed %v = %v, want pending", sym, res)
		}
	}
	if !s.Backspace() {
		t.Fatal("backspace while pending should unwind")
	}
	if !s.Composing() {
		t.Fatal("one level left: the s prefix should still be pending")
	}
	if res, _ := s.Feed('t'); res != Pending {
		t.Fatalf("t after unwind = %v, want pending", res)
	}
	if res, text := s.Feed('r'); res != Done || text != "★" {
		t.Fatalf("replayed s+t+r = %v %q, want done \"★\"", res, text)
	}
}

func TestBackspaceWithoutPendingSequenceIsNotOurs(t *testing.T) {
	s := testTable(t).Start()
	if s.Backspace() {
		t.Fatal("idle backspace must not be intercepted")
	}
	feedAll(s, xkb.KeyDeadAcute, 'e')
	if s.Composing() {
		t.Fatal("sequence should have committed")
	}
	if s.Backspace() {
		t.Fatal("backspace right after a commit must reach the widget")
	}
}

func TestCancelDropsTheSequence(t *testing.T) {
	s := testTable(t).Start()
	feedAll(s, xkb.KeyDeadAcute)
	if !s.Composing() {
		t.Fatal("sequence should be pending")
	}
	s.Cancel()
	if s.Composing() {
		t.Fatal("cancel should drop the sequence")
	}
	if res, _ := s.Feed('e'); res != None {
		t.Fatalf("e after cancel = %v, want none", res)
	}
}

func TestModifierKeysymsNeverFeedOrCancel(t *testing.T) {
	s := testTable(t).Start()
	feedAll(s, xkb.KeyDeadAcute)
	if !s.Composing() {
		t.Fatal("sequence should be pending")
	}
	if res, _ := s.Feed(xkb.KeyShiftL); res != None {
		t.Fatalf("shift = %v, want none", res)
	}
	if !s.Composing() {
		t.Fatal("shift must not cancel the sequence")
	}
	if res, text := s.Feed('e'); res != Done || text != "é" {
		t.Fatalf("dead_acute+shift+e = %v %q, want done \"é\"", res, text)
	}
}

func TestDisabledComposeIsInert(t *testing.T) {
	var disabled *Table
	s := disabled.Start() // nil table, nil state
	if res, text := s.Feed('e'); res != None || text != "" {
		t.Fatalf("disabled feed = %v %q, want none", res, text)
	}
	if s.Composing() || s.Backspace() {
		t.Fatal("disabled machine should be inert")
	}
	s.Cancel()
	if _, err := NewTableFile(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("a missing compose file should be an error, not a table")
	}
}

func TestLoadHonorsXComposeFile(t *testing.T) {
	t.Setenv("XCOMPOSEFILE", filepath.Join("testdata", "XCompose"))
	t.Setenv("HOME", t.TempDir()) // keep any real ~/.XCompose out of the way
	tb := Load()
	if tb == nil {
		t.Fatal("Load() = nil with XCOMPOSEFILE set")
	}
	if res, text := feedAll(tb.Start(), xkb.KeyDeadAcute, 'e'); res != Done || text != "é" {
		t.Fatalf("XCOMPOSEFILE table = %v %q, want done \"é\"", res, text)
	}
}

func TestXComposeFileBeatsTheUserFile(t *testing.T) {
	home := t.TempDir()
	user := filepath.Join(home, ".XCompose")
	if err := os.WriteFile(user, []byte("<dead_acute> <e> : \"alt\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XCOMPOSEFILE", filepath.Join("testdata", "XCompose"))
	tb := Load()
	if tb == nil {
		t.Fatal("Load() = nil with XCOMPOSEFILE set")
	}
	if _, text := feedAll(tb.Start(), xkb.KeyDeadAcute, 'e'); text != "é" {
		t.Fatalf("override lost to ~/.XCompose: text = %q, want é", text)
	}
}

func TestLoadWithoutAnyComposeFileIsDisabled(t *testing.T) {
	t.Setenv("XCOMPOSEFILE", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XLOCALEDIR", t.TempDir())
	if tb := Load(); tb != nil {
		t.Fatal("Load() should be nil with no compose file anywhere")
	}
}

func TestSystemComposeFileLive(t *testing.T) {
	const system = "/usr/share/X11/locale/en_US.UTF-8/Compose"
	if _, err := os.Stat(system); err != nil {
		t.Skipf("no system Compose file at %s on this machine", system)
	}
	t.Setenv("XCOMPOSEFILE", "")
	t.Setenv("HOME", t.TempDir())
	tb := Load()
	if tb == nil {
		t.Fatalf("Load() = nil with %s present", system)
	}
	if res, text := feedAll(tb.Start(), xkb.KeyDeadAcute, 'e'); res != Done || text != "é" {
		t.Fatalf("system table dead_acute+e = %v %q, want done \"é\"", res, text)
	}
}
