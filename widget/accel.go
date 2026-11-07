package widget

import (
	"strconv"
	"strings"

	"github.com/stubbedev/lys-lib/xkb"
)

// AccelLabel renders a keysym and its held modifiers in the canonical
// accelerator label form ("Ctrl+Shift+P", "Ctrl+Return", "F5"): the
// form app.ParseAccel reads back and NewShortcutLabel draws.
func AccelLabel(sym xkb.Keysym, mods Mods) string {
	name := xkb.KeysymGetName(sym)
	if name == "" {
		name = "0x" + strconv.FormatUint(uint64(sym), 16)
	}
	if len(name) == 1 {
		name = strings.ToUpper(name)
	}
	return strings.Join(append(modifierNames(mods), name), "+")
}

// modifierNames lists the held modifiers in label order.
func modifierNames(mods Mods) []string {
	parts := make([]string, 0, 5)
	if mods&ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ModSuper != 0 {
		parts = append(parts, "Super")
	}
	return parts
}

// IsModifierKeysym reports whether sym is a bare modifier key, one
// that is held for a combination rather than being its key.
func IsModifierKeysym(sym xkb.Keysym) bool {
	_, ok := modifierOf(sym)
	return ok
}

// modifierOf is the modifier bit a modifier key holds down, and
// whether sym is a modifier key at all: Caps Lock and the level-3
// shift are modifier keys that hold no accelerator bit.
func modifierOf(sym xkb.Keysym) (Mods, bool) {
	switch sym {
	case xkb.KeyShiftL, xkb.KeyShiftR:
		return ModShift, true
	case xkb.KeyControlL, xkb.KeyControlR:
		return ModCtrl, true
	case xkb.KeyAltL, xkb.KeyAltR, xkb.KeyMetaL, xkb.KeyMetaR:
		return ModAlt, true
	case xkb.KeySuperL, xkb.KeySuperR:
		return ModSuper, true
	case xkb.KeyCapsLock, xkb.KeyISOLevel3Shift:
		return 0, true
	}
	return 0, false
}

// KeyRecorder is a widget that, while it records, takes the raw key
// stream from the window holding it focused: every press and release
// reaches it before the window's key capture, its accelerators, text
// routing and focus traversal, and none of them sees the press. The
// keysym arrives normalized as accelerators are (letters lowercase)
// and the modifiers without Caps Lock.
type KeyRecorder interface {
	Recording() bool
	RecordPress(sym xkb.Keysym, mods Mods)
	RecordRelease(sym xkb.Keysym)
}
