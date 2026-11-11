package app

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/dbusx"
	"github.com/stubbedev/lys-lib/debug"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/worker"
)

const (
	portalInhibitIface    = "org.freedesktop.portal.Inhibit"
	portalInhibit         = portalInhibitIface + ".Inhibit"
	portalCreateMonitor   = portalInhibitIface + ".CreateMonitor"
	portalQueryEnd        = portalInhibitIface + ".QueryEndResponse"
	portalBackgroundIface = "org.freedesktop.portal.Background"
	portalRequestBg       = portalBackgroundIface + ".RequestBackground"
	portalSetStatus       = portalBackgroundIface + ".SetStatus"

	portalQueueDepth  = 64
	portalStopTimeout = 3 * time.Second
)

// ErrPortalDenied reports a portal request the user or the desktop
// refused.
var ErrPortalDenied = errors.New("app: the portal request was denied")

// ErrPortalBusy reports a portal request refused without being sent: the
// portal has stopped answering and too much is queued behind it, or the
// application has stopped.
var ErrPortalBusy = errors.New("app: the portal queue is full or stopped")

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
	queue         *worker.Queue
	monitor       dbus.ObjectPath
	onState       func(state SessionState, screensaver bool)
	printSettings map[string]dbus.Variant
	printSetup    *PageSetup
}

func (a *Application) desktop() *desktopPortals {
	a.portalsOnce.Do(func() {
		a.portals = &desktopPortals{app: a, queue: worker.New(portalQueueDepth)}
		a.portals.routes = []dbusx.Route{dbusx.On(sessionStateChanged, portalPath, a.portals.stateChanged)}
	})
	return a.portals
}

func (a *Application) reportDone(what string, done func(error), err error) {
	switch {
	case done != nil:
		a.Invoke(func() { done(err) })
	case err != nil:
		debug.Log("app", "%s: %v", what, err)
	}
}

func (d *desktopPortals) post(what string, work func() error, done func(error)) {
	if !d.queue.Post(func() { d.app.reportDone(what, done, work()) }) {
		d.app.reportDone(what, done, fmt.Errorf("app: %s: %w", what, ErrPortalBusy))
	}
}

// Inhibit holds off the session actions in flags, with reason shown to
// the user, through the xdg-desktop-portal Inhibit portal (GTK's
// gtk_application_inhibit). It returns at once, and done runs on the
// loop with the portal's answer: a release that ends the inhibition, or
// ErrPortalDenied, ErrPortalUnavailable or the failure. Calling release
// again does nothing, and a failure to release is logged, since the
// inhibition ends with the connection anyway.
func (a *Application) Inhibit(flags InhibitFlags, reason string, done func(release func(), err error)) {
	d := a.desktop()
	var handle dbus.ObjectPath
	d.post("inhibit", func() error {
		resp, h, err := d.request(portalInhibit, map[string]dbus.Variant{"reason": dbus.MakeVariant(reason)}, "", uint32(flags))
		if err != nil {
			return err
		}
		if resp.code != 0 {
			return fmt.Errorf("app: inhibit %q: %w", reason, ErrPortalDenied)
		}
		handle = h
		return nil
	}, func(err error) {
		if err != nil {
			done(nil, err)
			return
		}
		done(sync.OnceFunc(func() {
			d.post("release the inhibition", func() error { return d.closeRequest(handle) }, nil)
		}), nil)
	})
}

// OnSessionStateChange watches the login session's state through the
// Inhibit portal's monitor. It returns at once, and done (nil logs a
// failure instead) runs on the loop once the monitor stands or failed.
// fn runs on the loop goroutine with the state and whether the
// screensaver is active. When the state is SessionQueryEnd, lys-ui
// answers the portal's query after fn returns and after every Inhibit
// made inside fn reached the portal, so an Inhibit(InhibitLogout, ...)
// made inside fn objects to the logout.
func (a *Application) OnSessionStateChange(fn func(state SessionState, screensaver bool), done func(error)) {
	d := a.desktop()
	d.mu.Lock()
	d.onState = fn
	d.mu.Unlock()
	d.post("session monitor", d.ensureMonitor, done)
}

func (d *desktopPortals) ensureMonitor() error {
	d.mu.Lock()
	has := d.monitor != ""
	d.mu.Unlock()
	if has {
		return nil
	}
	if err := d.connect(); err != nil {
		return err
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

// sessionStateChange is what the Inhibit portal's StateChanged
// carries: the monitor's session, and the state as a vardict.
type sessionStateChange struct {
	Session dbus.ObjectPath         `dbus:"session_handle"`
	State   map[string]dbus.Variant `dbus:"state"`
}

var sessionStateChanged = dbusx.NewSignal[sessionStateChange](portalInhibitIface, "StateChanged")

// stateChanged hands the session state of this application's monitor
// to its listener on the loop, and answers a query-end once the
// listener has had its say. The state carries no descriptor this
// reads, so those it holds are closed.
func (d *desktopPortals) stateChanged(c sessionStateChange, _ dbus.Sequence) error {
	closed := dbus.CloseFDs([]any{c.State})
	d.mu.Lock()
	fn, mine := d.onState, c.Session == d.monitor
	d.mu.Unlock()
	if !mine || fn == nil {
		return closed
	}
	state := SessionRunning
	if v, ok := c.State["session-state"].Value().(uint32); ok {
		state = SessionState(v)
	}
	screensaver, _ := c.State["screensaver-active"].Value().(bool)
	d.app.Invoke(func() {
		fn(state, screensaver)
		if state == SessionQueryEnd {
			d.post("answer the query-end", func() error { return d.callPlain(portalQueryEnd, c.Session) }, nil)
		}
	})
	return closed
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
// the application while it runs in the background. It returns at once,
// and done (nil logs a failure instead) runs on the loop with the
// result.
func (a *Application) SetBackgroundStatus(message string, done func(error)) {
	d := a.desktop()
	d.post("background status", func() error {
		return d.callPlain(portalSetStatus, map[string]dbus.Variant{"message": dbus.MakeVariant(message)})
	}, done)
}

// close ends every portal call in flight by closing the connection, then
// joins the worker. Queued work finds the connection closed and fails.
func (d *desktopPortals) close() {
	d.queue.Stop(0)
	d.portalClient.close()
	if !d.queue.Stop(portalStopTimeout) {
		debug.Log("portal", "the portal worker outlived the stop")
	}
}
