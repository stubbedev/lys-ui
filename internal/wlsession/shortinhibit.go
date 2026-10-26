// Keyboard shortcuts inhibit: optional
// zwp_keyboard_shortcuts_inhibit_unstable_v1 support — the way a
// pane (a VM display, a game view) keeps the compositor's own
// shortcuts from stealing keystrokes while it holds keyboard focus.
// Inhibitors are explicit handles per surface and seat. The manager
// global is feature-detected: without it creation fails with
// ErrShortcutsInhibitUnavailable.
package wlsession

import (
	"errors"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// maxShortcutsInhibitVersion is the
// zwp_keyboard_shortcuts_inhibit_manager_v1 version we bind: the
// protocol stopped at version 1.
const maxShortcutsInhibitVersion = 1

// ErrShortcutsInhibitUnavailable reports that the compositor
// advertised no shortcuts-inhibit manager.
var ErrShortcutsInhibitUnavailable = errors.New("wlsession: no keyboard-shortcuts-inhibit protocol")

// shortcutsInhibitAPI is the request side of
// zwp_keyboard_shortcuts_inhibit_manager_v1, seen through the narrow
// interface below so tests can record the requests. The wire proxy
// satisfies it through wireShortcutsInhibit.
type shortcutsInhibitAPI interface {
	InhibitShortcuts(surface *wl.Surface, seat *wl.Seat) (shortcutsInhibitorAPI, error)
}

// shortcutsInhibitorAPI is the request and listener side of a
// zwp_keyboard_shortcuts_inhibitor_v1. *wlr.ZwpShortcutsInhibitorV1
// satisfies it; tests substitute a recorder.
type shortcutsInhibitorAPI interface {
	Destroy() error
	AddActiveHandler(h wlr.ZwpShortcutsInhibitorV1ActiveHandler)
	AddInactiveHandler(h wlr.ZwpShortcutsInhibitorV1InactiveHandler)
}

// wireShortcutsInhibit adapts the generated manager proxy to the
// narrow interface above.
type wireShortcutsInhibit struct {
	mgr *wlr.ZwpShortcutsInhibitManagerV1
}

// InhibitShortcuts implements shortcutsInhibitAPI.
func (m wireShortcutsInhibit) InhibitShortcuts(surface *wl.Surface, seat *wl.Seat) (shortcutsInhibitorAPI, error) {
	i, err := m.mgr.InhibitShortcuts(surface, seat)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// ShortcutsInhibitor suppresses the compositor's own shortcuts while
// its surface has keyboard focus. The compositor activates the
// inhibitor when focus arrives and deactivates it when focus leaves;
// Active reports which. Destroy when the pane closes; Destroy is
// nil-safe and idempotent.
type ShortcutsInhibitor struct {
	req    shortcutsInhibitorAPI
	active bool
	done   bool
}

// Active reports whether the inhibitor currently suppresses the
// compositor's shortcuts (the surface holds keyboard focus).
func (i *ShortcutsInhibitor) Active() bool { return i != nil && i.active }

// Destroy releases the inhibitor.
func (i *ShortcutsInhibitor) Destroy() {
	if i == nil || i.done {
		return
	}
	i.done = true
	i.active = false
	if i.req != nil {
		_ = i.req.Destroy()
	}
}

// HandleZwpShortcutsInhibitorV1Active implements
// wlr.ZwpShortcutsInhibitorV1ActiveHandler: the surface gained
// keyboard focus and the shortcuts are suppressed.
func (i *ShortcutsInhibitor) HandleZwpShortcutsInhibitorV1Active(wlr.ZwpShortcutsInhibitorV1ActiveEvent) {
	i.active = true
}

// HandleZwpShortcutsInhibitorV1Inactive implements
// wlr.ZwpShortcutsInhibitorV1InactiveHandler.
func (i *ShortcutsInhibitor) HandleZwpShortcutsInhibitorV1Inactive(wlr.ZwpShortcutsInhibitorV1InactiveEvent) {
	i.active = false
}

// ShortcutsInhibitAvailable reports whether the compositor advertised
// zwp_keyboard_shortcuts_inhibit_manager_v1.
func (s *Session) ShortcutsInhibitAvailable() bool { return s.shortcutsInhibitMgr != nil }

// bindShortcutsInhibitManager binds the optional shortcuts-inhibit
// manager global.
func (s *Session) bindShortcutsInhibitManager(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewZwpShortcutsInhibitManagerV1(ctx)
	if !s.bindOptional(ev, maxShortcutsInhibitVersion, mgr) {
		return
	}
	s.shortcutsInhibitMgr = wireShortcutsInhibit{mgr: mgr}
	debug.Log("shell", "keyboard-shortcuts-inhibit-v1 bound")
}

// InhibitShortcuts creates an inhibitor that suppresses the
// compositor's shortcuts for surface while the seat's keyboard focus
// is on it. Fails with ErrShortcutsInhibitUnavailable when the
// compositor lacks the protocol.
func (s *Session) InhibitShortcuts(surface *wl.Surface, seat *wl.Seat) (*ShortcutsInhibitor, error) {
	if s.shortcutsInhibitMgr == nil {
		return nil, ErrShortcutsInhibitUnavailable
	}
	if surface == nil || seat == nil {
		return nil, errors.New("wlsession: shortcuts inhibit needs a surface and a seat")
	}
	req, err := s.shortcutsInhibitMgr.InhibitShortcuts(surface, seat)
	if err != nil {
		return nil, err
	}
	inh := &ShortcutsInhibitor{req: req}
	req.AddActiveHandler(inh)
	req.AddInactiveHandler(inh)
	return inh, nil
}
