package app

import (
	"errors"
	"fmt"
	"sync"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

const (
	portalInhibitIface    = "org.freedesktop.portal.Inhibit"
	portalInhibit         = portalInhibitIface + ".Inhibit"
	portalCreateMonitor   = portalInhibitIface + ".CreateMonitor"
	portalStateChanged    = portalInhibitIface + ".StateChanged"
	portalQueryEnd        = portalInhibitIface + ".QueryEndResponse"
	portalBackgroundIface = "org.freedesktop.portal.Background"
	portalRequestBg       = portalBackgroundIface + ".RequestBackground"
	portalSetStatus       = portalBackgroundIface + ".SetStatus"
)

// ErrPortalDenied reports a portal request the user or the desktop
// refused.
var ErrPortalDenied = errors.New("app: the portal request was denied")

// InhibitFlags are the session actions Inhibit holds off.
type InhibitFlags uint32

const (
	// InhibitLogout holds off logging out.
	InhibitLogout InhibitFlags = 1 << iota
	// InhibitUserSwitch holds off switching users.
	InhibitUserSwitch
	// InhibitSuspend holds off suspending.
	InhibitSuspend
	// InhibitIdle holds off the session going idle.
	InhibitIdle
)

// SessionState is the login session's state the Inhibit portal
// reports.
type SessionState uint32

const (
	// SessionRunning is the normal state.
	SessionRunning SessionState = 1
	// SessionQueryEnd asks applications whether the session may end;
	// an application inhibits logout to object.
	SessionQueryEnd SessionState = 2
	// SessionEnding means the session is ending.
	SessionEnding SessionState = 3
)

type desktopPortals struct {
	portalClient
	app           *Application
	monitor       dbus.ObjectPath
	onState       func(state SessionState, screensaver bool)
	printSettings map[string]dbus.Variant
	printSetup    *PageSetup
}

func (a *Application) desktop() *desktopPortals {
	a.portalsOnce.Do(func() {
		a.portals = &desktopPortals{app: a}
		a.portals.onSignal = a.portals.signal
	})
	return a.portals
}

// Inhibit holds off the session actions in flags, with reason shown to
// the user, through the xdg-desktop-portal Inhibit portal (GTK's
// gtk_application_inhibit). The returned release ends the inhibition.
// It blocks for the portal's answer; without a portal it errors.
func (a *Application) Inhibit(flags InhibitFlags, reason string) (release func() error, err error) {
	d := a.desktop()
	resp, handle, err := d.request(portalInhibit, map[string]dbus.Variant{"reason": dbus.MakeVariant(reason)}, "", uint32(flags))
	if err != nil {
		return nil, err
	}
	if resp.code != 0 {
		return nil, fmt.Errorf("app: inhibit %q: %w", reason, ErrPortalDenied)
	}
	var once sync.Once
	return func() error {
		var err error
		once.Do(func() { err = d.closeRequest(handle) })
		return err
	}, nil
}

// OnSessionStateChange watches the login session's state through the
// Inhibit portal's monitor. fn runs on the loop goroutine with the
// state and whether the screensaver is active. When the state is
// SessionQueryEnd, lys-ui answers the portal's query after fn returns, so
// an Inhibit(InhibitLogout, ...) made inside fn objects to the logout.
func (a *Application) OnSessionStateChange(fn func(state SessionState, screensaver bool)) error {
	d := a.desktop()
	d.mu.Lock()
	d.onState = fn
	has := d.monitor != ""
	d.mu.Unlock()
	if has {
		return nil
	}
	resp, _, err := d.request(portalCreateMonitor, map[string]dbus.Variant{"session_handle_token": dbus.MakeVariant(d.token(sessionToken))}, "")
	if err != nil {
		return err
	}
	if resp.code != 0 {
		return fmt.Errorf("app: session monitor: %w", ErrPortalDenied)
	}
	handle, ok := objectPath(resp.results["session_handle"])
	if !ok {
		return errors.New("app: session monitor: the portal sent no session handle")
	}
	d.mu.Lock()
	d.monitor = handle
	d.mu.Unlock()
	return nil
}

func objectPath(v dbus.Variant) (dbus.ObjectPath, bool) {
	switch h := v.Value().(type) {
	case dbus.ObjectPath:
		return h, h != ""
	case string:
		return dbus.ObjectPath(h), h != ""
	}
	return "", false
}

func (d *desktopPortals) signal(sig *dbus.Signal) {
	if sig.Name != portalStateChanged || len(sig.Body) < 2 {
		return
	}
	handle, _ := sig.Body[0].(dbus.ObjectPath)
	info, _ := sig.Body[1].(map[string]dbus.Variant)
	d.mu.Lock()
	fn, mine := d.onState, handle == d.monitor
	d.mu.Unlock()
	if !mine || fn == nil {
		return
	}
	state := SessionRunning
	if v, ok := info["session-state"].Value().(uint32); ok {
		state = SessionState(v)
	}
	screensaver, _ := info["screensaver-active"].Value().(bool)
	d.app.Invoke(func() {
		fn(state, screensaver)
		if state == SessionQueryEnd {
			go func() { _ = d.callPlain(portalQueryEnd, handle) }()
		}
	})
}

// BackgroundRequest asks to keep running without windows and,
// optionally, to start at login.
type BackgroundRequest struct {
	// Reason is shown to the user.
	Reason string
	// Autostart asks to start the application at login.
	Autostart bool
	// Commandline is what autostart runs; empty uses the desktop
	// file's Exec.
	Commandline []string
	// DBusActivatable autostarts through D-Bus activation.
	DBusActivatable bool
}

// BackgroundResult is the desktop's answer.
type BackgroundResult struct {
	// Background reports whether the app may run in the background.
	Background bool
	// Autostart reports whether the app starts at login.
	Autostart bool
}

// RequestBackground asks the Background portal for permission to run
// without windows (and to autostart). The desktop may ask the user, so
// the answer arrives later: done runs on the loop goroutine with the
// result, or with ErrPortalDenied or the failure.
func (a *Application) RequestBackground(req BackgroundRequest, done func(BackgroundResult, error)) {
	d := a.desktop()
	opts := map[string]dbus.Variant{
		"reason":           dbus.MakeVariant(req.Reason),
		"autostart":        dbus.MakeVariant(req.Autostart),
		"dbus-activatable": dbus.MakeVariant(req.DBusActivatable),
	}
	if len(req.Commandline) > 0 {
		opts["commandline"] = dbus.MakeVariant(req.Commandline)
	}
	go func() {
		resp, _, err := d.request(portalRequestBg, opts, "")
		var res BackgroundResult
		switch {
		case err != nil:
		case resp.code != 0:
			err = fmt.Errorf("app: background request: %w", ErrPortalDenied)
		default:
			res.Background, _ = resp.results["background"].Value().(bool)
			res.Autostart, _ = resp.results["autostart"].Value().(bool)
		}
		a.Invoke(func() { done(res, err) })
	}()
}

// SetBackgroundStatus sets the one-line status the desktop shows for
// the application while it runs in the background.
func (a *Application) SetBackgroundStatus(message string) error {
	return a.desktop().callPlain(portalSetStatus, map[string]dbus.Variant{"message": dbus.MakeVariant(message)})
}
