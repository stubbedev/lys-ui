package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/neurlang/wayland/wl"
	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/internal/logutil"
	"github.com/stubbedev/lys-ui/transfer"
)

// External launching (#86): app.OpenURL and app.OpenPath hand a URL or
// a file to the user's handlers - the browser for https links, the
// file's app for files - the GTK UriLauncher/FileLauncher pair.
//
// Transport: the xdg-desktop-portal OpenURI interface when a portal
// owns the desktop (sandbox-friendly, and the only correct way inside
// flatpaks), xdg-open(1) otherwise, and nothing succeeds without
// either - the error comes back for the app to surface. The portal
// call carries an xdg-activation token requested from the session at
// call time, so the launched app may take focus per the compositor's
// focus-stealing rules; a session without the activation protocol
// launches without one.

// launchTimeout bounds the portal call.
const launchTimeout = 3 * time.Second

const portalOpenURI = "org.freedesktop.portal.OpenURI.OpenURI"

// launcher is the Application's launch state: the transport seams
// tests substitute, set once at construction to the portal D-Bus call
// and xdg-open and only read after.
type launcher struct {
	openPortal   func(uri, parentWindow, token string) error
	openExternal func(uri string) error
}

// OpenURL opens url in the user's handler for its scheme. Safe from
// any goroutine; the transport work happens off the caller. The
// compositor decides focus, guided by an activation token when the
// session's xdg-activation support issued one.
func (a *Application) OpenURL(uri string) {
	a.launch(uri)
}

// OpenPath opens a file or directory in the user's handler for it,
// the FileLauncher shape.
func (a *Application) OpenPath(path string) {
	a.launch(transfer.FileURI(path))
}

// launch routes one URI: with activation support the open waits for
// its own token (requested on the loop, anchored to the focused
// surface and the latest input event, so it vouches for a real
// interaction), else it opens now.
func (a *Application) launch(uri string) {
	if uri == "" {
		return
	}
	if a.sess == nil || !a.sess.ActivationAvailable() {
		a.launchNow(uri, "")
		return
	}
	a.RequestActivationToken(func(token string, err error) {
		if err != nil {
			debug.Log("shell", "launch %s without an activation token: %v", uri, err)
		}
		a.launchNow(uri, token)
	})
}

// focusSurface is the focused window's surface, for anchoring the
// activation token.
func (a *Application) focusSurface() *wl.Surface {
	if hw := a.focused(); hw != nil && hw.host != nil {
		return hw.host.HostSurface()
	}
	return nil
}

// launchNow runs the transport off the caller's goroutine: portal
// first, xdg-open when no portal answered, an error logged (Debug)
// when neither worked. The focused-surface read happens on the
// caller's side - loop state never leaves the loop goroutine.
func (a *Application) launchNow(uri, token string) {
	l := a.launchState
	parent := ""
	if a.sess != nil {
		if hw := a.focused(); hw != nil {
			parent = portalParent(hw.win)
		}
	}
	go func() {
		portalErr := l.openPortal(uri, parent, token)
		if portalErr == nil {
			return
		}
		if !errors.Is(portalErr, ErrPortalUnavailable) {
			debug.Log("shell", "open uri: portal: %v", portalErr)
		}
		if err := l.openExternal(uri); err != nil {
			logutil.L().Debug("open uri failed", "uri", uri, "portal", portalErr, "fallback", err)
		}
	}()
}

// portalOpen calls the OpenURI interface on the session bus.
func portalOpen(uri, parentWindow, token string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("app: open %s: session bus: %w: %w", uri, ErrPortalUnavailable, err)
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	if err := conn.Object("org.freedesktop.DBus", "/").CallWithContext(
		conn.Context(), "org.freedesktop.DBus.NameHasOwner", 0,
		portalName).Store(&owned); err != nil {
		return fmt.Errorf("app: open %s: ask for %s: %w: %w", uri, portalName, ErrPortalUnavailable, err)
	}
	if !owned {
		return fmt.Errorf("app: open %s: %w", uri, ErrPortalUnavailable)
	}
	options := map[string]dbus.Variant{}
	if token != "" {
		options["activation_token"] = dbus.MakeVariant(token)
	}
	ctx, cancel := context.WithTimeout(conn.Context(), launchTimeout)
	defer cancel()
	return conn.Object(portalName, portalPath).CallWithContext(
		ctx, portalOpenURI, dbus.FlagNoReplyExpected, parentWindow, uri, options).Err
}

// xdgOpen runs xdg-open(1) detached.
func xdgOpen(uri string) error {
	cmd := exec.Command("xdg-open", uri) //nolint:gosec // the documented external opener; the URI is the user-chosen link
	return cmd.Start()
}

// OpenURLFromLink wires a RichLabel's (or any link-emitting widget's)
// OnLinkClick to OpenURL - the sanctioned one-liner links have been
// missing:
//
//	label.OnLinkClick = application.OpenURLFromLink
func (a *Application) OpenURLFromLink(href string) { a.OpenURL(href) }
