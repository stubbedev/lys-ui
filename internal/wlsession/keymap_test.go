package wlsession

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/unxed/xkb-go"
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
	ctx := xkb.NewContext(context.Background(), xkb.ContextNoFlags)
	ctx.AppendIncludePath(findXKBData(t))
	km, err := ctx.NewKeymapFromNames(&xkb.RuleNames{Layout: "us"})
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
