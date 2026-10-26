// Global shortcuts over hyprland-global-shortcuts-v1 (#87: or the
// freedesktop GlobalShortcuts portal, whichever the desktop offers):
// actions the compositor binds to keys and reports back, for apps
// with no focused window (a push-to-talk daemon, a capture watcher).
package app

import (
	"errors"

	"github.com/stubbedev/lys-lib/wlr"
)

// ErrGlobalShortcutsUnavailable reports a desktop with neither
// hyprland_global_shortcuts_v1 nor the GlobalShortcuts portal.
var ErrGlobalShortcutsUnavailable = errors.New("app: no hyprland_global_shortcuts_v1 and no GlobalShortcuts portal")

// GlobalShortcut is one registered shortcut, over either transport.
type GlobalShortcut struct {
	p      *wlr.GlobalShortcutV1
	portal *portalShortcuts
	id     string
	on     func(pressed bool, unixSec uint64)
}

// GlobalShortcutsAvailable reports whether a transport exists: the
// compositor's protocol manager, or the desktop's GlobalShortcuts
// portal.
func (a *Application) GlobalShortcutsAvailable() bool {
	return a.sess.GlobalShortcutsManager() != nil || portalShortcutsUp()
}

// RegisterGlobalShortcut registers id for appID; the desktop shows
// description and binds trigger (a user-readable hint; the
// compositor's config or the portal's dialog decides the keys). on
// runs on the loop goroutine for every press (true) and release, with
// the event's Unix seconds. The app_id + id pair must be unique for
// the connection. Call on the loop goroutine.
//
// Transport: the compositor's hyprland_global_shortcuts_v1 when
// offered, else the freedesktop GlobalShortcuts portal. The portal's
// bind flow shows the desktop's binding dialog and registration
// returns only after the user answered - denial is
// ErrGlobalShortcutDenied - so a portal registration can block on a
// human; call it from a goroutine when that matters.
func (a *Application) RegisterGlobalShortcut(id, appID, description, trigger string, on func(pressed bool, unixSec uint64)) (*GlobalShortcut, error) {
	if mgr := a.sess.GlobalShortcutsManager(); mgr != nil {
		p, err := mgr.RegisterShortcut(id, appID, description, trigger)
		if err != nil {
			return nil, err
		}
		s := &GlobalShortcut{p: p, on: on}
		p.AddPressedHandler(s)
		p.AddReleasedHandler(s)
		return s, nil
	}
	s := &GlobalShortcut{portal: &a.shortcuts, id: id, on: on}
	if err := a.shortcuts.registerPortal(id, description, trigger, s); err != nil {
		return nil, err
	}
	return s, nil
}

// Destroy unregisters the shortcut. Call on the loop goroutine.
func (s *GlobalShortcut) Destroy() {
	if s.portal != nil {
		s.portal.mu.Lock()
		delete(s.portal.live, s.id)
		s.portal.mu.Unlock()
		s.portal = nil
		return
	}
	if s.p == nil {
		return
	}
	_ = s.p.Destroy()
	s.p = nil
}

// HandleGlobalShortcutV1Pressed implements the protocol listener.
func (s *GlobalShortcut) HandleGlobalShortcutV1Pressed(ev wlr.GlobalShortcutV1PressedEvent) {
	s.on(true, shortcutSeconds(ev.TvSecHi, ev.TvSecLo))
}

// HandleGlobalShortcutV1Released implements the protocol listener.
func (s *GlobalShortcut) HandleGlobalShortcutV1Released(ev wlr.GlobalShortcutV1ReleasedEvent) {
	s.on(false, shortcutSeconds(ev.TvSecHi, ev.TvSecLo))
}

func shortcutSeconds(hi, lo uint32) uint64 { return uint64(hi)<<32 | uint64(lo) }
