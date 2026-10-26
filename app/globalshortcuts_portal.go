// Global shortcuts over the freedesktop GlobalShortcuts portal: the
// xdg-desktop-portal transport that works on every portal desktop,
// chosen when the compositor offers no hyprland_global_shortcuts_v1
// manager. The portal's bind flow is interactive - the desktop shows
// its shortcut-binding dialog and the user confirms - so a portal
// registration returns only after that confirmation (denial comes
// back as ErrGlobalShortcutDenied). Triggers arrive as Activated and
// Deactivated signals and dispatch onto the loop goroutine like every
// other event.
package app

import (
	"errors"

	"github.com/godbus/dbus/v5"
	"github.com/stubbedev/lys-lib/debug"
)

// ErrGlobalShortcutDenied reports the user refusing the portal's bind
// dialog, or the portal cancelling the request.
var ErrGlobalShortcutDenied = errors.New("app: global shortcut denied by the user")

const (
	portalShortcutsIface = "org.freedesktop.portal.GlobalShortcuts"

	portalCreateSession = portalShortcutsIface + ".CreateSession"
	portalBindShortcuts = portalShortcutsIface + ".BindShortcuts"

	portalActivated   = portalShortcutsIface + ".Activated"
	portalDeactivated = portalShortcutsIface + ".Deactivated"
)

// portalShortcuts is the application's one portal shortcuts session,
// lazily created on the first portal registration.
type portalShortcuts struct {
	portalClient
	app     *Application
	session dbus.ObjectPath
	// live routes Activated and Deactivated by shortcut id.
	live map[string]*GlobalShortcut
}

// portalShortcutsUp reports whether the desktop portal owns its name.
func portalShortcutsUp() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	if err := conn.Object("org.freedesktop.DBus", "/").CallWithContext(
		conn.Context(), "org.freedesktop.DBus.NameHasOwner", 0,
		portalName).Store(&owned); err != nil {
		return false
	}
	return owned
}

func (p *portalShortcuts) connect() error {
	p.mu.Lock()
	if p.live == nil {
		p.live = map[string]*GlobalShortcut{}
	}
	p.onSignal = p.shortcutSignal
	p.mu.Unlock()
	return p.portalClient.connect()
}

// shortcutSignal fires the live shortcut an Activated or Deactivated
// names, on the loop through Invoke.
func (p *portalShortcuts) shortcutSignal(sig *dbus.Signal) {
	if sig.Name != portalActivated && sig.Name != portalDeactivated || len(sig.Body) < 2 {
		return
	}
	id, _ := sig.Body[1].(string)
	p.mu.Lock()
	s := p.live[id]
	p.mu.Unlock()
	if s == nil {
		return
	}
	pressed := sig.Name == portalActivated
	var sec uint64
	if len(sig.Body) >= 3 {
		if us, ok := sig.Body[2].(uint64); ok {
			sec = us / 1_000_000
		}
	}
	p.app.Invoke(func() { s.on(pressed, sec) })
}

func (p *portalShortcuts) call(method string, opts map[string]dbus.Variant, args ...any) (portalResponseData, error) {
	resp, _, err := p.request(method, opts, args...)
	return resp, err
}

// createSession opens the portal session; it has no dialog.
func (p *portalShortcuts) createSession() error {
	if err := p.connect(); err != nil {
		return err
	}
	opts := map[string]dbus.Variant{"session_handle_token": dbus.MakeVariant(p.token(sessionToken))}
	resp, err := p.call(portalCreateSession, opts)
	if err != nil {
		return err
	}
	if resp.code != 0 {
		return ErrGlobalShortcutDenied
	}
	var session dbus.ObjectPath
	if v, ok := resp.results["session_handle"]; ok {
		switch h := v.Value().(type) {
		case dbus.ObjectPath:
			session = h
		case string:
			session = dbus.ObjectPath(h)
		}
	}
	if session == "" {
		return errors.New("app: portal shortcut session handle missing")
	}
	p.mu.Lock()
	p.session = session
	p.mu.Unlock()
	return nil
}

// bindOne runs the interactive bind for one shortcut: the desktop
// shows its binding dialog, and the verdict comes back - denial as
// ErrGlobalShortcutDenied.
func (p *portalShortcuts) bindOne(id, description, trigger string) error {
	p.mu.Lock()
	session := p.session
	p.mu.Unlock()
	if session == "" {
		return ErrGlobalShortcutsUnavailable
	}
	shortcut := map[string]dbus.Variant{"id": dbus.MakeVariant(id)}
	if description != "" {
		shortcut["description"] = dbus.MakeVariant(description)
	}
	if trigger != "" {
		shortcut["preferred_trigger"] = dbus.MakeVariant(trigger)
	}
	resp, err := p.call(portalBindShortcuts, map[string]dbus.Variant{},
		session, []map[string]dbus.Variant{shortcut})
	if err != nil {
		return err
	}
	if resp.code != 0 {
		return ErrGlobalShortcutDenied
	}
	return nil
}

// registerPortal is the RegisterGlobalShortcut portal path: one
// session, then the interactive bind, then live.
func (p *portalShortcuts) registerPortal(id, description, trigger string, s *GlobalShortcut) error {
	p.mu.Lock()
	hasSession := p.session != ""
	p.mu.Unlock()
	if !hasSession {
		if err := p.createSession(); err != nil {
			return err
		}
	}
	if err := p.bindOne(id, description, trigger); err != nil {
		debug.Log("shell", "portal shortcut %s: %v", id, err)
		return err
	}
	p.mu.Lock()
	p.live[id] = s
	p.mu.Unlock()
	return nil
}

// shutdown closes the portal connection with the loop.
func (p *portalShortcuts) shutdown() {
	p.close()
	p.mu.Lock()
	p.live = map[string]*GlobalShortcut{}
	p.mu.Unlock()
}
