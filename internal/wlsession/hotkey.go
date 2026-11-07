// Client-chosen global hotkeys: the optional xx_hotkey_v1, where the
// client describes the trigger and the compositor binds it or says why
// not (lys-wm's client_hotkeys). Feature-detected: without the global
// NewHotkeys fails with ErrHotkeysUnavailable. One manager carries one
// app id, so a client binding for several apps (a portal) binds the
// global once per app.
package wlsession

import (
	"errors"
	"fmt"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
	"github.com/stubbedev/lys-lib/xkb"
)

// ErrHotkeysUnavailable reports that the compositor advertised no
// xx_hotkey_manager_v1.
var ErrHotkeysUnavailable = errors.New("wlsession: no xx_hotkey_v1 protocol")

// ErrHotkeyDestroyed answers a commit still in flight when its hotkey
// was destroyed, which cancels it.
var ErrHotkeyDestroyed = errors.New("wlsession: hotkey destroyed before the compositor answered")

// HotkeyDenyReason is why the compositor refused a trigger.
type HotkeyDenyReason uint32

// The deny reasons, xx_hotkey_v1.deny_reason.
const (
	// HotkeyAlreadyBound is a trigger another binding holds.
	HotkeyAlreadyBound HotkeyDenyReason = wlr.HotkeyV1DenyReasonAlreadyBound
	// HotkeyNotPermitted is a trigger the compositor's policy refuses.
	HotkeyNotPermitted HotkeyDenyReason = wlr.HotkeyV1DenyReasonNotPermitted
)

// String names the reason.
func (r HotkeyDenyReason) String() string {
	switch r {
	case HotkeyAlreadyBound:
		return "already bound"
	case HotkeyNotPermitted:
		return "not permitted"
	}
	return fmt.Sprintf("deny reason %d", uint32(r))
}

// HotkeyDeniedError is a commit the compositor refused: why, and its
// advisory message for display ("" for none).
type HotkeyDeniedError struct {
	Reason  HotkeyDenyReason
	Message string
}

// Error implements error.
func (e *HotkeyDeniedError) Error() string {
	if e.Message == "" {
		return "wlsession: hotkey " + e.Reason.String()
	}
	return "wlsession: hotkey " + e.Reason.String() + ": " + e.Message
}

// hotkeyManagerAPI is the request side of xx_hotkey_manager_v1, seen
// through a narrow interface so tests can record the requests.
type hotkeyManagerAPI interface {
	SetAppId(appID string) error
	CreateHotkey() (hotkeyAPI, error)
	Destroy() error
}

// hotkeyGlobal binds a fresh manager on the advertised global, once per
// app id.
type hotkeyGlobal interface {
	bind() (hotkeyManagerAPI, error)
}

// hotkeyAPI is an xx_hotkey_v1. *wlr.HotkeyV1 satisfies it.
type hotkeyAPI interface {
	SetDescription(description string) error
	SetKeyTrigger(keysym, modifiers uint32) error
	Commit() error
	Destroy() error
	AddBoundHandler(h wlr.HotkeyV1BoundHandler)
	AddDeniedHandler(h wlr.HotkeyV1DeniedHandler)
	AddRevokedHandler(h wlr.HotkeyV1RevokedHandler)
	AddTriggeredHandler(h wlr.HotkeyV1TriggeredHandler)
	AddReleasedHandler(h wlr.HotkeyV1ReleasedHandler)
}

// wireHotkeyManager adapts the generated manager proxy to
// hotkeyManagerAPI.
type wireHotkeyManager struct {
	mgr *wlr.HotkeyManagerV1
}

// CreateHotkey implements hotkeyManagerAPI.
func (m wireHotkeyManager) CreateHotkey() (hotkeyAPI, error) {
	k, err := m.mgr.CreateHotkey()
	if err != nil {
		return nil, err
	}
	return k, nil
}

// Destroy implements hotkeyManagerAPI.
func (m wireHotkeyManager) Destroy() error { return m.mgr.Destroy() }

// wireHotkeyGlobal is the advertised xx_hotkey_manager_v1 global.
type wireHotkeyGlobal struct {
	registry *wl.Registry
	ev       wl.RegistryGlobalEvent
}

// bind implements hotkeyGlobal.
func (g wireHotkeyGlobal) bind() (hotkeyManagerAPI, error) {
	ev := g.ev
	mgr := wlr.NewHotkeyManagerV1(g.registry.Context())
	if err := g.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 1), mgr); err != nil {
		return nil, err
	}
	return wireHotkeyManager{mgr: mgr}, nil
}

// HotkeysAvailable reports whether the compositor offers xx_hotkey_v1.
func (s *Session) HotkeysAvailable() bool { return s.hotkeyGlobal != nil }

// Hotkeys is one app id's hotkeys: an xx_hotkey_manager_v1 carrying it.
type Hotkeys struct {
	mgr hotkeyManagerAPI
}

// NewHotkeys binds a manager identified as appID ("" leaves it
// unidentified).
func (s *Session) NewHotkeys(appID string) (*Hotkeys, error) {
	if s.hotkeyGlobal == nil {
		return nil, ErrHotkeysUnavailable
	}
	mgr, err := s.hotkeyGlobal.bind()
	if err != nil {
		return nil, fmt.Errorf("wlsession: bind xx_hotkey_manager_v1 for %q: %w", appID, err)
	}
	if appID != "" {
		if err := mgr.SetAppId(appID); err != nil {
			return nil, fmt.Errorf("wlsession: hotkeys app id %q: %w", appID, err)
		}
	}
	return &Hotkeys{mgr: mgr}, nil
}

// SetAppId implements hotkeyManagerAPI.
func (m wireHotkeyManager) SetAppId(appID string) error { return m.mgr.SetAppId(appID) }

// Close destroys the manager; its hotkeys stay valid.
func (h *Hotkeys) Close() {
	if err := h.mgr.Destroy(); err != nil {
		debug.Log("input", "hotkey manager destroy: %v", err)
	}
}

// HotkeyEvents are what a bound hotkey reports: each press and release
// of its trigger, and the compositor withdrawing the binding.
type HotkeyEvents struct {
	Pressed func(pressed bool)
	Revoked func(message string)
}

// Hotkey is one xx_hotkey_v1: unbound until a Bind is answered bound.
type Hotkey struct {
	req     hotkeyAPI
	events  HotkeyEvents
	pending []func(error)
}

// NewHotkey creates a hotkey shown as description; nothing fires
// before a Bind is answered bound.
func (h *Hotkeys) NewHotkey(description string, events HotkeyEvents) (*Hotkey, error) {
	req, err := h.mgr.CreateHotkey()
	if err != nil {
		return nil, fmt.Errorf("wlsession: create hotkey %q: %w", description, err)
	}
	if description != "" {
		if err := req.SetDescription(description); err != nil {
			return nil, fmt.Errorf("wlsession: describe hotkey %q: %w", description, err)
		}
	}
	k := &Hotkey{req: req, events: events}
	req.AddBoundHandler(k)
	req.AddDeniedHandler(k)
	req.AddRevokedHandler(k)
	req.AddTriggeredHandler(k)
	req.AddReleasedHandler(k)
	return k, nil
}

// HotkeyModifiers is mods as xx_hotkey_manager_v1.modifiers bits; Caps
// Lock is never part of a trigger.
func HotkeyModifiers(mods Mods) uint32 {
	var bits uint32
	if mods&ModShift != 0 {
		bits |= wlr.HotkeyManagerV1ModifiersShift
	}
	if mods&ModCtrl != 0 {
		bits |= wlr.HotkeyManagerV1ModifiersCtrl
	}
	if mods&ModAlt != 0 {
		bits |= wlr.HotkeyManagerV1ModifiersAlt
	}
	if mods&ModSuper != 0 {
		bits |= wlr.HotkeyManagerV1ModifiersSuper
	}
	return bits
}

// Bind commits sym (unshifted) held with mods as the trigger; done
// runs on the loop once the compositor answers, nil when bound, a
// *HotkeyDeniedError when refused (a binding already established stays
// on its old trigger), ErrHotkeyDestroyed when the hotkey went first.
func (k *Hotkey) Bind(sym xkb.Keysym, mods Mods, done func(error)) error {
	if k.req == nil {
		return ErrHotkeyDestroyed
	}
	if sym == xkb.KeyNoSymbol {
		return errors.New("wlsession: hotkey trigger without a key")
	}
	if err := k.req.SetKeyTrigger(uint32(sym), HotkeyModifiers(mods)); err != nil {
		return fmt.Errorf("wlsession: hotkey trigger: %w", err)
	}
	if err := k.req.Commit(); err != nil {
		return fmt.Errorf("wlsession: hotkey commit: %w", err)
	}
	k.pending = append(k.pending, done)
	return nil
}

// Destroy releases the binding; commits in flight are answered
// ErrHotkeyDestroyed. A second call does nothing.
func (k *Hotkey) Destroy() {
	if k.req == nil {
		return
	}
	if err := k.req.Destroy(); err != nil {
		debug.Log("input", "hotkey destroy: %v", err)
	}
	k.req = nil
	pending := k.pending
	k.pending = nil
	for _, done := range pending {
		done(ErrHotkeyDestroyed)
	}
}

// answer pops the oldest commit in flight and answers it.
func (k *Hotkey) answer(err error) {
	if len(k.pending) == 0 {
		debug.Log("input", "hotkey answer without a commit: %v", err)
		return
	}
	done := k.pending[0]
	k.pending = k.pending[1:]
	done(err)
}

// HandleHotkeyV1Bound implements wlr.HotkeyV1BoundHandler.
func (k *Hotkey) HandleHotkeyV1Bound(wlr.HotkeyV1BoundEvent) { k.answer(nil) }

// HandleHotkeyV1Denied implements wlr.HotkeyV1DeniedHandler.
func (k *Hotkey) HandleHotkeyV1Denied(ev wlr.HotkeyV1DeniedEvent) {
	k.answer(&HotkeyDeniedError{Reason: HotkeyDenyReason(ev.Reason), Message: ev.Message})
}

// HandleHotkeyV1Revoked implements wlr.HotkeyV1RevokedHandler.
func (k *Hotkey) HandleHotkeyV1Revoked(ev wlr.HotkeyV1RevokedEvent) {
	if k.req != nil && k.events.Revoked != nil {
		k.events.Revoked(ev.Message)
	}
}

// HandleHotkeyV1Triggered implements wlr.HotkeyV1TriggeredHandler.
func (k *Hotkey) HandleHotkeyV1Triggered(wlr.HotkeyV1TriggeredEvent) { k.press(true) }

// HandleHotkeyV1Released implements wlr.HotkeyV1ReleasedHandler.
func (k *Hotkey) HandleHotkeyV1Released(wlr.HotkeyV1ReleasedEvent) { k.press(false) }

func (k *Hotkey) press(pressed bool) {
	if k.req != nil && k.events.Pressed != nil {
		k.events.Pressed(pressed)
	}
}
