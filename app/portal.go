package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/dbusx"
	"github.com/stubbedev/lys-lib/debug"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

const (
	portalName          = "org.freedesktop.portal.Desktop"
	portalPath          = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalRequestIface  = "org.freedesktop.portal.Request"
	portalRequestPrefix = "/org/freedesktop/portal/desktop/request/"
	portalClose         = portalRequestIface + ".Close"

	portalRequestTimeout = 30 * time.Second
)

var (
	errPortalTimeout = errors.New("app: portal request timed out")
	errPortalClosed  = errors.New("app: the portal connection closed")
)

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

// requestResponse is what a Request's Response carries.
type requestResponse struct {
	Code    uint32                  `dbus:"response"`
	Results map[string]dbus.Variant `dbus:"results"`
}

var requestResponded = dbusx.NewSignal[requestResponse](portalRequestIface, "Response")

// portalClient is one session-bus connection to xdg-desktop-portal:
// the Request/Response handshake every interactive portal call uses,
// and one watch, from the connection on, for the signals routes names
// on the portal's own object. Only the owner of the portal's name is
// heard.
type portalClient struct {
	connectMu sync.Mutex
	mu        sync.Mutex
	conn      *dbus.Conn
	watch     *dbusx.Watch
	closed    bool
	nextReq   int
	routes    []dbusx.Route
}

func portalParent(w *Window) string {
	if w == nil {
		return ""
	}
	if h := w.ExportHandle(); h != "" {
		return "wayland:" + h
	}
	return ""
}

func (c *portalClient) connect() error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	c.mu.Lock()
	connected, closed := c.conn != nil, c.closed
	c.mu.Unlock()
	if connected {
		return nil
	}
	if closed {
		return fmt.Errorf("app: portal: %w", errPortalClosed)
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("app: portal: session bus: %w: %w", ErrPortalUnavailable, err)
	}
	c.mu.Lock()
	routes := c.routes
	c.mu.Unlock()
	var watch *dbusx.Watch
	if len(routes) > 0 {
		unread := func(err error) { debug.Log("portal", "a portal signal was not read: %v", err) }
		if watch, err = dbusx.WatchSignals(conn.Context(), conn, dbusx.FromOwner(portalName), unread, routes...); err != nil {
			return errors.Join(fmt.Errorf("app: portal: %w", err), conn.Close())
		}
	}
	c.mu.Lock()
	closed = c.closed
	if !closed {
		c.conn, c.watch = conn, watch
	}
	c.mu.Unlock()
	if closed {
		endPortal(conn, watch)
		return fmt.Errorf("app: portal: %w", errPortalClosed)
	}
	return nil
}

// endPortal closes conn and waits for its watch, when it has one, to
// end with it.
func endPortal(conn *dbus.Conn, watch *dbusx.Watch) {
	_ = conn.Close()
	if watch == nil {
		return
	}
	if err := watch.Stop(); err != nil && !errors.Is(err, dbusx.ErrDisconnected) {
		debug.Log("portal", "stop watching the portal: %v", err)
	}
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

// requestPath is where the portal puts the Request for the handle
// token on conn: the spec's path, so the Response is watched for before
// the call that makes it.
func requestPath(conn *dbus.Conn, token string) dbus.ObjectPath {
	return dbus.ObjectPath(portalRequestPrefix + strings.NewReplacer(":", "", ".", "_").Replace(conn.Names()[0]) + "/" + token)
}

// request issues a portal method whose verdict arrives as a Response
// on its request handle and waits for it. opts may carry entries; the
// handle token is added here. It returns the handle too, for calls
// whose effect lasts until the handle is closed (Inhibit). No portal
// call here takes a descriptor, so those the results carry are closed;
// a Response that does not read as one is an error.
func (c *portalClient) request(method string, opts map[string]dbus.Variant, args ...any) (portalResponseData, dbus.ObjectPath, error) {
	if err := c.connect(); err != nil {
		return portalResponseData{}, "", err
	}
	token := c.token(requestToken)
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return portalResponseData{}, "", fmt.Errorf("app: portal %s: %w", method, errPortalClosed)
	}
	opts["handle_token"] = dbus.MakeVariant(token)
	ctx, cancel := context.WithTimeout(conn.Context(), portalRequestTimeout)
	defer cancel()
	responses, unreadable := make(chan portalResponseData, 1), make(chan error, 1)
	var watches []*dbusx.Watch
	defer func() {
		for _, w := range watches {
			_ = w.Stop()
		}
	}()
	watch := func(at dbus.ObjectPath) error {
		w, err := dbusx.WatchSignals(ctx, conn, dbusx.FromOwner(portalName),
			func(err error) {
				select {
				case unreadable <- err:
				default:
				}
			},
			dbusx.On(requestResponded, at, func(r requestResponse, _ dbus.Sequence) error {
				select {
				case responses <- portalResponseData{code: r.Code, results: r.Results}:
				default:
				}
				return dbus.CloseFDs([]any{r.Results})
			}))
		if err != nil {
			if conn.Context().Err() != nil {
				return fmt.Errorf("app: portal %s: %w", method, errPortalClosed)
			}
			return fmt.Errorf("app: portal %s: %w", method, err)
		}
		watches = append(watches, w)
		return nil
	}
	predicted := requestPath(conn, token)
	if err := watch(predicted); err != nil {
		return portalResponseData{}, "", err
	}
	var handle dbus.ObjectPath
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, method, 0, append(append([]any{}, args...), opts)...).Store(&handle); err != nil {
		return portalResponseData{}, "", portalCallError(method, err)
	}
	if handle != predicted {
		if err := watch(handle); err != nil {
			return portalResponseData{}, "", err
		}
	}
	select {
	case resp := <-responses:
		return resp, handle, nil
	case err := <-unreadable:
		return portalResponseData{}, "", fmt.Errorf("app: portal %s: %w", method, err)
	case <-ctx.Done():
		if conn.Context().Err() != nil {
			return portalResponseData{}, "", fmt.Errorf("app: portal %s: %w", method, errPortalClosed)
		}
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
	conn, watch := c.conn, c.watch
	c.conn, c.watch, c.closed = nil, nil, true
	c.mu.Unlock()
	if conn != nil {
		endPortal(conn, watch)
	}
}
