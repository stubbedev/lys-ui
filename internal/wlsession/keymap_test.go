package wlsession

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/logutil"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/xkb"
	"golang.org/x/sys/unix"
)

// findXKBData locates the system xkeyboard-config data directory; tests
// that need a real keymap skip without it.
func findXKBData(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("XKB_DATA_DIR"); p != "" {
		return p
	}
	matches, _ := filepath.Glob("/nix/store/*xkeyboard-config*/share/X11/xkb")
	if len(matches) == 0 {
		matches, _ = filepath.Glob("/usr/share/X11/xkb")
	}
	if len(matches) == 0 {
		t.Skip("no xkeyboard-config data found")
	}
	return matches[0]
}

func usKeymap(t *testing.T) *Session {
	t.Helper()
	t.Setenv("XKB_CONFIG_ROOT", findXKBData(t))
	km, err := xkb.NewContext(os.Getenv).NewKeymapFromNames(xkb.Names{Layout: "us"})
	if err != nil {
		t.Skip("us keymap unavailable:", err)
	}
	return &Session{xkbKeymap: km, xkbState: km.NewState()}
}

func TestKeymapTranslation(t *testing.T) {
	s := usKeymap(t)

	t.Run("unshifted a is lowercase", func(t *testing.T) {
		// 30 is the evdev code of the physical A key (xkb keycode 38).
		if got := s.KeyUTF8(30); got != "a" {
			t.Errorf("KeyUTF8 = %q, want a", got)
		}
	})

	t.Run("the shift modifier yields uppercase", func(t *testing.T) {
		if got := s.KeyUTF8(30); got != "a" {
			t.Fatalf("precondition: %q", got)
		}
		s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 1})
		if got := s.KeyUTF8(30); got != "A" {
			t.Errorf("KeyUTF8 with shift = %q, want A", got)
		}
		if s.Mods()&ModShift == 0 {
			t.Error("Mods must report shift after the modifiers event")
		}
	})

	t.Run("arrows produce no text but map to keysyms", func(t *testing.T) {
		if got := s.KeyUTF8(105); got != "" {
			t.Errorf("left arrow produced text %q", got)
		}
		if s.KeySym(105) != xkb.KeyLeft {
			t.Errorf("keysym = %v, want KeyLeft", s.KeySym(105))
		}
	})

	t.Run("without a keymap translation is empty", func(t *testing.T) {
		empty := &Session{}
		if got := empty.KeyUTF8(30); got != "" {
			t.Errorf("no-keymap text = %q", got)
		}
		if empty.KeySym(30) != xkb.KeyNoSymbol {
			t.Error("no-keymap keysym must be KeyNoSymbol")
		}
	})
}

const wtypeKeymap = `xkb_keymap {
xkb_keycodes "(unnamed)" {
	minimum = 8;
	maximum = 10;
	<K1> = 9;
	<K2> = 10;
};
xkb_types "(unnamed)" { include "complete" };
xkb_compatibility "(unnamed)" { include "complete" };
xkb_symbols "(unnamed)" {
	key <K1> {[Return]};
	key <K2> {[a]};
};
};
`

func settleBackground(t *testing.T, s *Session) {
	t.Helper()
	if !s.background().Settle(10 * time.Second) {
		t.Fatal("the background goroutine never settled")
	}
	s.runPosted()
}

func deliverKeymap(t *testing.T, s *Session, text string) {
	t.Helper()
	fd, err := unix.MemfdCreate("keymap", unix.MFD_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Write(fd, append([]byte(text), 0)); err != nil {
		t.Fatal(err)
	}
	s.HandleKeyboardKeymap(wl.KeyboardKeymapEvent{Format: 1, Fd: uintptr(fd), Size: uint32(len(text) + 1)})
	settleBackground(t, s)
}

func TestVirtualKeyboardKeymapWithIncludesTranslates(t *testing.T) {
	t.Setenv("XKB_CONFIG_ROOT", findXKBData(t))
	s := usKeymap(t)
	deliverKeymap(t, s, wtypeKeymap)
	if got := s.KeySym(1); got != xkb.KeyReturn {
		t.Errorf("keycode 9 (evdev 1, Escape on a real layout) = %v, want Return", got)
	}
	if got := s.KeyUTF8(2); got != "a" {
		t.Errorf("keycode 10 = %q, want a", got)
	}
}

func TestKeymapThatDoesNotCompileLeavesNoKeyTranslated(t *testing.T) {
	s := usKeymap(t)
	if got := s.KeySym(1); got != xkb.KeyEscape {
		t.Fatalf("precondition: evdev 1 = %v, want Escape", got)
	}
	var logged bytes.Buffer
	previous := logutil.L()
	logutil.Set(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { logutil.Set(previous) })
	deliverKeymap(t, s, "xkb_keymap { this does not compile")
	if got := s.KeySym(1); got != xkb.KeyNoSymbol {
		t.Errorf("after a bad keymap evdev 1 = %v, want no keysym rather than the old keymap's", got)
	}
	if got := s.KeyUTF8(30); got != "" {
		t.Errorf("after a bad keymap the A key typed %q", got)
	}
	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("no error logged: %q", logged.String())
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 1})
	if s.Mods()&ModShift == 0 {
		t.Error("modifiers still track without a keymap")
	}
}

func TestControlCharactersAreNotText(t *testing.T) {
	s := usKeymap(t)
	for name, code := range map[string]uint32{"Escape": 1, "Backspace": 14, "Tab": 15, "Return": 28, "Delete": 111} {
		if got := s.KeyUTF8(code); got != "" {
			t.Errorf("%s typed %q", name, got)
		}
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 4})
	if got := s.KeyUTF8(30); got != "" {
		t.Errorf("Ctrl+A typed %q", got)
	}
}

func TestLocksFollowTheCompositor(t *testing.T) {
	s := usKeymap(t)
	const capsLocked, numLocked = 2, 16
	var seen []Locks
	s.OnLocks = func(l Locks) { seen = append(seen, l) }

	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{})
	if len(seen) != 0 {
		t.Fatalf("an event with no lock on reported %v", seen)
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsLocked: capsLocked})
	if got := s.Locks(); !got.Caps || got.Num || got.Scroll {
		t.Errorf("Locks with Caps locked = %+v", got)
	}
	if len(seen) != 1 || !seen[0].Caps {
		t.Errorf("Caps already on at the first modifiers event reported %v", seen)
	}
	if s.Mods()&ModCapsLock != 0 {
		t.Error("Mods reports a lock as a held key")
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsLocked: capsLocked, ModsDepressed: 1})
	if len(seen) != 1 {
		t.Errorf("holding Shift reported a lock change: %v", seen)
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsLocked: capsLocked | numLocked})
	if got := s.Locks(); !got.Caps || !got.Num {
		t.Errorf("Locks with Caps and Num locked = %+v", got)
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{})
	if got := s.Locks(); got != (Locks{}) || len(seen) != 3 {
		t.Errorf("after unlocking: Locks = %+v, reported %v", got, seen)
	}
}

func TestKeyboardLayoutFollowsTheKeymapAndTheGroup(t *testing.T) {
	t.Setenv("XKB_CONFIG_ROOT", findXKBData(t))
	km, err := xkb.NewContext(os.Getenv).NewKeymapFromNames(xkb.Names{Layout: "us,de"})
	if err != nil {
		t.Fatalf("compile us,de: %v", err)
	}
	s := &Session{}
	var seen []KeyboardLayout
	s.OnKeyboardLayout = func(l KeyboardLayout) { seen = append(seen, l) }
	s.installKeymap(compiledKeymap{keymap: km, state: km.NewState()})
	if got := s.KeyboardLayout(); len(got.Names) != 2 || got.Names[0] != "English (US)" || got.Names[1] != "German" || got.Active != 0 {
		t.Fatalf("after the keymap: %+v", got)
	}
	if len(seen) != 1 {
		t.Fatalf("the keymap reported %d changes, want 1", len(seen))
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{ModsDepressed: 1})
	if len(seen) != 1 {
		t.Errorf("Shift alone reported a layout change: %v", seen)
	}
	s.HandleKeyboardModifiers(wl.KeyboardModifiersEvent{Group: 1})
	if got := s.KeyboardLayout(); got.Active != 1 || len(seen) != 2 {
		t.Errorf("group 1: %+v, reported %v", got, seen)
	}
	if got := s.KeyUTF8(21); got != "z" {
		t.Errorf("the Y key in group 1 typed %q, want z", got)
	}
	s.installKeymap(compiledKeymap{keymap: km, state: km.NewState()})
	if got := s.KeyboardLayout(); got.Active != 0 || len(seen) != 3 {
		t.Errorf("a fresh keymap: %+v, reported %v", got, seen)
	}
	s.dropKeymap(os.ErrInvalid)
	if got := s.KeyboardLayout(); len(got.Names) != 0 || len(seen) != 4 {
		t.Errorf("a dropped keymap: %+v, reported %v", got, seen)
	}
}
