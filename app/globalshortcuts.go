// Global shortcuts over hyprland-global-shortcuts-v1 (#87: or the
// freedesktop GlobalShortcuts portal, whichever the desktop offers):
// actions the compositor binds to keys and reports back, for apps
// with no focused window (a push-to-talk daemon, a capture watcher).
package app

import (
	"errors"
	"fmt"

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

// GlobalShortcutsAvailable reports through done, on the loop, whether a
// transport exists: the compositor's protocol manager, or the desktop's
// GlobalShortcuts portal. It returns at once; asking the bus runs on the
// portal worker.
func (a *Application) GlobalShortcutsAvailable(done func(bool)) {
	if a.sess.GlobalShortcutsManager() != nil {
		a.Invoke(func() { done(true) })
		return
	}
	if !a.shortcuts.post(a, func() {
		up := portalShortcutsUp()
		a.Invoke(func() { done(up) })
	}) {
		a.Invoke(func() { done(false) })
	}
}

// RegisterGlobalShortcut registers id for appID; the desktop shows
// description and binds trigger (a user-readable hint; the
// compositor's config or the portal's dialog decides the keys). on
// runs on the loop goroutine for every press (true) and release, with
// the event's Unix seconds. The app_id + id pair must be unique for
// the connection. Call on the loop goroutine. It returns at once, and
// done runs on the loop with the shortcut or the failure.
//
// Transport: the compositor's hyprland_global_shortcuts_v1 when
// offered, else the freedesktop GlobalShortcuts portal. The portal's
// bind flow shows the desktop's binding dialog, so done may come long
// after, once the user answered; denial is ErrGlobalShortcutDenied, and
// ErrPortalBusy says too much is queued behind a portal that stopped
// answering.
func (a *Application) RegisterGlobalShortcut(id, appID, description, trigger string, on func(pressed bool, unixSec uint64), done func(*GlobalShortcut, error)) {
	if mgr := a.sess.GlobalShortcutsManager(); mgr != nil {
		p, err := mgr.RegisterShortcut(id, appID, description, trigger)
		if err != nil {
			a.Invoke(func() { done(nil, err) })
			return
		}
		s := &GlobalShortcut{p: p, on: on}
		p.AddPressedHandler(s)
		p.AddReleasedHandler(s)
		a.Invoke(func() { done(s, nil) })
		return
	}
	s := &GlobalShortcut{portal: &a.shortcuts, id: id, on: on}
	if !a.shortcuts.post(a, func() {
		if err := a.shortcuts.registerPortal(id, description, trigger, s); err != nil {
			a.Invoke(func() { done(nil, err) })
			return
		}
		a.Invoke(func() { done(s, nil) })
	}) {
		a.Invoke(func() { done(nil, fmt.Errorf("app: global shortcut %s: %w", id, ErrPortalBusy)) })
	}
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
