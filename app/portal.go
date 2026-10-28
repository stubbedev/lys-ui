package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

const (
	portalName     = "org.freedesktop.portal.Desktop"
	portalPath     = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalResponse = "org.freedesktop.portal.Request.Response"
	portalClose    = "org.freedesktop.portal.Request.Close"

	portalRequestTimeout = 30 * time.Second
)

var errPortalTimeout = errors.New("app: portal request timed out")

// ErrPortalUnavailable reports that no xdg-desktop-portal answered on
// the session bus, or that the running one has no backend for the
// interface asked for.
var ErrPortalUnavailable = errors.New("app: no xdg-desktop-portal provides this interface")

var portalAbsentErrors = []string{
	"org.freedesktop.DBus.Error.ServiceUnknown",
	"org.freedesktop.DBus.Error.NameHasNoOwner",
	"org.freedesktop.DBus.Error.UnknownInterface",
	"org.freedesktop.DBus.Error.UnknownObject",
	"org.freedesktop.DBus.Error.UnknownMethod",
}

func portalCallError(method string, err error) error {
	var de dbus.Error
	if errors.As(err, &de) && slices.Contains(portalAbsentErrors, de.Name) {
		return fmt.Errorf("app: portal %s: %w: %w", method, ErrPortalUnavailable, err)
	}
	return fmt.Errorf("app: portal %s: %w", method, err)
}

type portalResponseData struct {
	code    uint32
	results map[string]dbus.Variant
}

// portalClient is one session-bus connection to xdg-desktop-portal:
// the Request/Response handshake every interactive portal call uses,
// and the routing of other portal signals to onSignal.
type portalClient struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	sig      chan *dbus.Signal
	nextReq  int
	pending  map[string]chan portalResponseData
	onSignal func(*dbus.Signal)
}

func portalParent(w *Window) string {
	if w == nil || w.win == nil {
		return ""
	}
	if h := w.win.ExportHandle(); h != "" {
		return "wayland:" + h
	}
	return ""
}

func (c *portalClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return nil
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("app: portal: session bus: %w: %w", ErrPortalUnavailable, err)
	}
	c.conn = conn
	c.pending = map[string]chan portalResponseData{}
	c.sig = make(chan *dbus.Signal, 16)
	conn.Signal(c.sig)
	go func() {
		for sig := range c.sig {
			c.dispatch(sig)
		}
	}()
	return nil
}

type tokenKind byte

const (
	requestToken tokenKind = 'r'
	sessionToken tokenKind = 's'
)

func (c *portalClient) token(kind tokenKind) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextReq++
	return fmt.Sprintf("lysui_%c%d", kind, c.nextReq)
}

func (c *portalClient) dispatch(sig *dbus.Signal) {
	if sig.Name != portalResponse {
		if c.onSignal != nil {
			c.onSignal(sig)
		}
		return
	}
	token := pathTail(sig.Path)
	c.mu.Lock()
	ch := c.pending[token]
	delete(c.pending, token)
	c.mu.Unlock()
	if ch == nil {
		return
	}
	resp := portalResponseData{code: 1}
	if len(sig.Body) >= 1 {
		resp.code, _ = sig.Body[0].(uint32)
	}
	if len(sig.Body) >= 2 {
		resp.results, _ = sig.Body[1].(map[string]dbus.Variant)
	}
	ch <- resp
}

func pathTail(path dbus.ObjectPath) string {
	if i := strings.LastIndexByte(string(path), '/'); i >= 0 {
		return string(path)[i+1:]
	}
	return ""
}

// request issues a portal method whose verdict arrives as a Response
// on its request handle and waits for it. opts may carry entries; the
// handle token is added here. It returns the handle too, for calls
// whose effect lasts until the handle is closed (Inhibit).
func (c *portalClient) request(method string, opts map[string]dbus.Variant, args ...any) (portalResponseData, dbus.ObjectPath, error) {
	if err := c.connect(); err != nil {
		return portalResponseData{}, "", err
	}
	token := c.token(requestToken)
	ch := make(chan portalResponseData, 1)
	c.mu.Lock()
	conn := c.conn
	c.pending[token] = ch
	c.mu.Unlock()
	drop := func() {
		c.mu.Lock()
		delete(c.pending, token)
		c.mu.Unlock()
	}
	opts["handle_token"] = dbus.MakeVariant(token)
	ctx, cancel := context.WithTimeout(conn.Context(), portalRequestTimeout)
	defer cancel()
	var handle dbus.ObjectPath
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, method, 0, append(append([]any{}, args...), opts)...).Store(&handle); err != nil {
		drop()
		return portalResponseData{}, "", portalCallError(method, err)
	}
	select {
	case resp := <-ch:
		return resp, handle, nil
	case <-time.After(portalRequestTimeout):
		drop()
		return portalResponseData{}, "", fmt.Errorf("app: portal %s: %w", method, errPortalTimeout)
	}
}

// callPlain issues a portal method with no Request handshake.
func (c *portalClient) callPlain(method string, args ...any) error {
	if err := c.connect(); err != nil {
		return err
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(conn.Context(), portalRequestTimeout)
	defer cancel()
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, method, 0, args...).Err; err != nil {
		return portalCallError(method, err)
	}
	return nil
}

// closeRequest closes a request handle, ending what it holds.
func (c *portalClient) closeRequest(handle dbus.ObjectPath) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(conn.Context(), portalRequestTimeout)
	defer cancel()
	return conn.Object(portalName, handle).CallWithContext(ctx, portalClose, 0).Err
}

func (c *portalClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
}
