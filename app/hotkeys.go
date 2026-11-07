package app

import "github.com/stubbedev/lys-ui/internal/wlsession"

// ErrHotkeysUnavailable reports a compositor without xx_hotkey_v1.
var ErrHotkeysUnavailable = wlsession.ErrHotkeysUnavailable

// ErrHotkeyDestroyed answers a Bind still in flight when its hotkey
// was destroyed.
var ErrHotkeyDestroyed = wlsession.ErrHotkeyDestroyed

// HotkeyDeniedError is a trigger the compositor refused, with why and
// its advisory message for display.
type HotkeyDeniedError = wlsession.HotkeyDeniedError

// HotkeyDenyReason is why a trigger was refused.
type HotkeyDenyReason = wlsession.HotkeyDenyReason

// The deny reasons.
const (
	// HotkeyAlreadyBound is a trigger another binding (a compositor
	// bind or another client's hotkey) holds.
	HotkeyAlreadyBound = wlsession.HotkeyAlreadyBound
	// HotkeyNotPermitted is a trigger the compositor's policy refuses.
	HotkeyNotPermitted = wlsession.HotkeyNotPermitted
)

// HotkeyEvents are what a bound hotkey reports, on the loop goroutine:
// each press (true) and release of its trigger, and the compositor
// withdrawing the binding with its advisory message.
type HotkeyEvents = wlsession.HotkeyEvents

// HotkeysAvailable reports whether the compositor offers xx_hotkey_v1.
// Call on the loop goroutine.
func (a *Application) HotkeysAvailable() bool { return a.sess.HotkeysAvailable() }

// Hotkeys is the client-chosen global hotkeys of one app id, over
// xx_hotkey_v1 (lys-wm's client_hotkeys): the client names the
// trigger, the compositor binds exactly it or says why not, and a bound
// hotkey fires whichever window has focus. Unlike
// RegisterGlobalShortcut's hyprland_global_shortcuts_v1, where the
// compositor's config decides the key, this binds the key the user
// recorded. The compositor's policy and audit see every hotkey made
// from it as that app's.
type Hotkeys struct{ h *wlsession.Hotkeys }

// NewHotkeys opens the hotkeys of appID ("" for an unidentified
// client). Call on the loop goroutine.
func (a *Application) NewHotkeys(appID string) (*Hotkeys, error) {
	h, err := a.sess.NewHotkeys(appID)
	if err != nil {
		return nil, err
	}
	return &Hotkeys{h: h}, nil
}

// Close releases the app id's manager; hotkeys made from it stay bound
// until destroyed. Call on the loop goroutine.
func (h *Hotkeys) Close() { h.h.Close() }

// Hotkey is one global hotkey, unbound until a Bind is answered.
type Hotkey struct{ k *wlsession.Hotkey }

// NewHotkey makes a hotkey the compositor shows as description.
// Nothing fires before a Bind is answered bound. Call on the loop
// goroutine.
func (h *Hotkeys) NewHotkey(description string, events HotkeyEvents) (*Hotkey, error) {
	k, err := h.h.NewHotkey(description, events)
	if err != nil {
		return nil, err
	}
	return &Hotkey{k: k}, nil
}

// Bind asks the compositor for trigger; done runs on the loop once it
// answers: nil when bound, a *HotkeyDeniedError when refused (a
// binding already established keeps its old trigger), or
// ErrHotkeyDestroyed. Call on the loop goroutine.
func (k *Hotkey) Bind(trigger Accel, done func(error)) error {
	return k.k.Bind(trigger.Sym, trigger.Mods, done)
}

// Destroy releases the binding. Call on the loop goroutine.
func (k *Hotkey) Destroy() { k.k.Destroy() }
