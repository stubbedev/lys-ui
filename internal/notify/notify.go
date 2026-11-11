// Package notify sends desktop notifications: the
// xdg-desktop-portal Notification interface when a portal owns the
// desktop, org.freedesktop.Notifications (the GNOME spec every daemon
// implements) as the fallback, and nothing at all without a session
// bus - the appearance.md failure model, inert and logged at Debug.
// Pure Go over the existing godbus dependency.
//
// Threading: every method returns at once and is safe from any
// goroutine, including the Wayland loop. The dial, the probe and every
// bus call run in order on the notifier's worker; OnAction and OnClosed
// callbacks run serialized on the signal goroutine, and the application
// bridges onto its loop with Invoke.
package notify

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-lib/dbusx"
	"github.com/stubbedev/lys-lib/logutil"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/worker"
)

// Action is one notification action: the key the daemon reports back
// on activation and the label its button shows.
type Action struct {
	Key   string
	Label string
}

// Priority is the notification's importance, mapped to the portal's
// priority strings and the classic spec's urgency byte.
type Priority uint8

// Priorities.
const (
	// Low is background information.
	Low Priority = iota
	// Normal is the default.
	Normal
	// High demands attention; classic daemons may let it through
	// do-not-disturb.
	High
	// Critical is High plus (on the classic spec) a request to not
	// expire.
	Critical
)

// Notification is one desktop notification. Title and Body are plain
// text (markup is the daemon's business, not the spec's); Icon is an
// icon-theme name or a file path, resolved by whoever shows it;
// Timeout of zero means the server default, negative means never
// expire (classic spec behavior; portals may ignore it).
type Notification struct {
	Title    string
	Body     string
	Icon     string
	Priority Priority
	Timeout  time.Duration
	Actions  []Action
	// DefaultAction, when set with a key, makes the notification body
	// itself activatable; the portal wire also carries its label.
	DefaultAction Action
	HasDefault    bool
}

// maxBodyRunes caps the body: notification daemons are not document
// viewers, and the portal vardict is not the place to ship one.
const maxBodyRunes = 4000

// CloseReason names why a notification closed, from the classic spec's
// reason codes (the portal reports no reasons).
type CloseReason uint8

// Close reasons, the classic spec's codes.
const (
	ReasonExpired      CloseReason = 1
	ReasonDismissed    CloseReason = 2
	ReasonClosedByCall CloseReason = 3
	ReasonUnspecified  CloseReason = 4
)

// Bus is the session-bus connection a Notifier sends through. Watch
// follows the routes' signals from from until ctx ends or stop is
// called, as dbusx.WatchSignals does.
type Bus interface {
	Call(ctx context.Context, dest string, path dbus.ObjectPath, method string, flags dbus.Flags, ret any, args ...any) error
	Watch(ctx context.Context, from dbusx.Sender, failed func(error), routes ...dbusx.Route) (stop func() error, err error)
	Close() error
}

type connBus struct{ conn *dbus.Conn }

func (b connBus) Call(ctx context.Context, dest string, path dbus.ObjectPath, method string, flags dbus.Flags, ret any, args ...any) error {
	call := b.conn.Object(dest, path).CallWithContext(ctx, method, flags, args...)
	if ret == nil {
		return call.Err
	}
	return call.Store(ret)
}

func (b connBus) Watch(ctx context.Context, from dbusx.Sender, failed func(error), routes ...dbusx.Route) (func() error, error) {
	w, err := dbusx.WatchSignals(ctx, b.conn, from, failed, routes...)
	if err != nil {
		return nil, err
	}
	return w.Stop, nil
}

func (b connBus) Close() error { return b.conn.Close() }

// Notifier owns one session-bus connection and sends notifications
// through the best transport its worker finds on the first send.
// Without a bus (or with neither name owned) it is inert: Notify is a
// no-op that still returns an id, callbacks never fire, and Close is a
// no-op. A queue full of a stuck daemon's work drops the notification
// with a log.
type Notifier struct {
	dial   func() (Bus, error)
	queue  *worker.Queue
	base   context.Context
	cancel context.CancelFunc

	// probed, read and written on the worker alone, says the dial ran.
	probed bool

	mu      sync.Mutex
	bus     Bus
	unwatch func() error
	mode    transport
	nextID  atomic.Int64
	closed  bool
	// classicIDs maps the classic daemon's numeric ids to lys-ui's
	// string ids (the portal transport uses the string id directly);
	// classicOf is the reverse, for Close.
	classicIDs map[uint32]string
	classicOf  map[string]uint32

	// Actions and closes fan out from the signal goroutine, serialized
	// in arrival order.
	onAction func(id, action string)
	onClosed func(id string, reason CloseReason)
}

// transport is which wire the desktop speaks.
type transport uint8

const (
	// transportNone: no bus, or neither name owned.
	transportNone transport = iota
	// transportPortal: xdg-desktop-portal Notification.
	transportPortal
	// transportClassic: org.freedesktop.Notifications.
	transportClassic
)

const (
	portalDesktopName = "org.freedesktop.portal.Desktop"
	portalPath        = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalIface       = "org.freedesktop.portal.Notification"
	portalAdd         = portalIface + ".AddNotification"
	portalRemove      = portalIface + ".RemoveNotification"

	classicName      = "org.freedesktop.Notifications"
	classicPath      = dbus.ObjectPath("/org/freedesktop/Notifications")
	classicIface     = "org.freedesktop.Notifications"
	classicNotify    = classicIface + ".Notify"
	classicCloseCall = classicIface + ".CloseNotification"

	// callTimeout bounds every bus call; notifications are fire and
	// forget, never worth hanging a caller for the godbus default 25s.
	callTimeout = 3 * time.Second

	queueDepth = 64
)

// New returns a notifier over the session bus. It touches no bus: the
// worker dials and picks the transport on the first send, portal when
// the desktop portal owns its name, the classic spec when its daemon
// does, and inert otherwise.
func New() *Notifier {
	return NewWithDial(dialSession)
}

// NewWithDial is New over another bus.
func NewWithDial(dial func() (Bus, error)) *Notifier {
	base, cancel := context.WithCancel(context.Background())
	return &Notifier{dial: dial, queue: worker.New(queueDepth), base: base, cancel: cancel}
}

func dialSession() (Bus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	return connBus{conn: conn}, nil
}

func (n *Notifier) connect() (Bus, transport) {
	if !n.probed {
		n.probed = true
		n.dialAndProbe()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.bus, n.mode
}

func (n *Notifier) dialAndProbe() {
	bus, err := n.dial()
	if err != nil {
		logutil.L().Debug("notify: no session bus; desktop notifications disabled", "err", err)
		return
	}
	mode := n.probe(bus)
	if mode == transportNone {
		_ = bus.Close()
		return
	}
	unwatch := n.subscribe(bus, mode)
	n.mu.Lock()
	closed := n.closed
	if !closed {
		n.bus, n.unwatch, n.mode = bus, unwatch, mode
	}
	n.mu.Unlock()
	if closed {
		n.release(bus, unwatch)
	}
}

// probe picks the transport by owned names.
func (n *Notifier) probe(bus Bus) transport {
	for _, cand := range []struct {
		name string
		mode transport
	}{{portalDesktopName, transportPortal}, {classicName, transportClassic}} {
		ctx, cancel := context.WithTimeout(n.base, callTimeout)
		var owned bool
		err := bus.Call(ctx, "org.freedesktop.DBus", "/", "org.freedesktop.DBus.NameHasOwner", 0, &owned, cand.name)
		cancel()
		if err != nil {
			logutil.L().Debug("notify: NameHasOwner failed", "name", cand.name, "err", err)
			continue
		}
		if owned {
			return cand.mode
		}
	}
	return transportNone
}

type portalAction struct {
	ID     string `dbus:"id"`
	Action string `dbus:"action"`
}

type classicAction struct {
	ID     uint32 `dbus:"id"`
	Action string `dbus:"action_key"`
}

type classicClosed struct {
	ID     uint32 `dbus:"id"`
	Reason uint32 `dbus:"reason"`
}

var (
	portalActionInvoked       = dbusx.NewSignal[portalAction](portalIface, "ActionInvoked")
	classicActionInvoked      = dbusx.NewSignal[classicAction](classicIface, "ActionInvoked")
	classicNotificationClosed = dbusx.NewSignal[classicClosed](classicIface, "NotificationClosed")
)

// subscribe routes the transport's ActionInvoked and NotificationClosed,
// from the owner of its name alone, to the callbacks on one goroutine,
// which ends when the bus closes or the notifier shuts down. A bus that
// refuses the watch still sends; nothing it does is reported back, and
// the returned stop is nil.
func (n *Notifier) subscribe(bus Bus, mode transport) (stop func() error) {
	from, routes := dbusx.FromOwner(portalDesktopName), []dbusx.Route{
		dbusx.On(portalActionInvoked, portalPath, func(a portalAction, _ dbus.Sequence) error {
			n.fireAction(a.ID, a.Action)
			return nil
		}),
	}
	if mode == transportClassic {
		from, routes = dbusx.FromOwner(classicName), []dbusx.Route{
			dbusx.On(classicActionInvoked, classicPath, func(a classicAction, _ dbus.Sequence) error {
				if known := n.classicID(a.ID); known != "" {
					n.fireAction(known, a.Action)
				}
				return nil
			}),
			dbusx.On(classicNotificationClosed, classicPath, func(c classicClosed, _ dbus.Sequence) error {
				if known := n.classicID(c.ID); known != "" {
					n.fireClosed(known, CloseReason(min(c.Reason, uint32(ReasonUnspecified))))
					n.forgetClassic(c.ID)
				}
				return nil
			}),
		}
	}
	unread := func(err error) { logutil.L().Debug("notify: a signal was not read", "err", err) }
	stop, err := bus.Watch(n.base, from, unread, routes...)
	if err != nil {
		logutil.L().Debug("notify: no signal watch; actions and closes will not arrive", "err", err)
		return nil
	}
	return stop
}

// release ends the watch on bus, when there is one, and closes it.
func (n *Notifier) release(bus Bus, unwatch func() error) {
	if unwatch != nil {
		if err := unwatch(); err != nil && !errors.Is(err, dbusx.ErrDisconnected) {
			logutil.L().Debug("notify: stop watching the signals", "err", err)
		}
	}
	_ = bus.Close()
}

// Notify queues nf for the worker and returns its id at once: generated
// here, stable across transports, safe to hand to Close. The body is
// capped at maxBodyRunes runes.
func (n *Notifier) Notify(nf Notification) string {
	id := "lys-ui-" + time.Now().Format("20060102T150405.000000000") + "-" + itoa(n.nextID.Add(1))
	nf.Body = capBody(nf.Body)
	if !n.queue.Post(func() { n.send(id, nf) }) {
		logutil.L().Debug("notify: dropped a notification, the worker is stuck or stopped", "id", id)
	}
	return id
}

func (n *Notifier) send(id string, nf Notification) {
	bus, mode := n.connect()
	if bus == nil {
		return
	}
	ctx, cancel := context.WithTimeout(n.base, callTimeout)
	defer cancel()
	switch mode {
	case transportPortal:
		if err := bus.Call(ctx, portalDesktopName, portalPath, portalAdd, dbus.FlagNoReplyExpected, nil, id, portalDict(nf)); err != nil {
			logutil.L().Debug("notify: portal AddNotification failed", "err", err)
		}
	case transportClassic:
		var classicID uint32
		if err := bus.Call(ctx, classicName, classicPath, classicNotify, 0, &classicID, "lys-ui", uint32(0), nf.Icon,
			nf.Title, nf.Body, classicActions(nf), classicHints(nf), classicTimeout(nf.Timeout)); err != nil {
			logutil.L().Debug("notify: Notifications.Notify failed", "err", err)
			return
		}
		n.rememberClassic(classicID, id)
	}
}

// Close queues the withdrawal of a still-shown notification; unknown ids
// and inert notifiers are no-ops.
func (n *Notifier) Close(id string) {
	if !n.queue.Post(func() { n.withdraw(id) }) {
		logutil.L().Debug("notify: dropped a close, the worker is stuck or stopped", "id", id)
	}
}

func (n *Notifier) withdraw(id string) {
	bus, mode := n.connect()
	if bus == nil {
		return
	}
	n.mu.Lock()
	classic := n.classicOf[id]
	n.mu.Unlock()
	ctx, cancel := context.WithTimeout(n.base, callTimeout)
	defer cancel()
	var err error
	switch {
	case mode == transportPortal:
		err = bus.Call(ctx, portalDesktopName, portalPath, portalRemove, dbus.FlagNoReplyExpected, nil, id)
	case mode == transportClassic && classic != 0:
		err = bus.Call(ctx, classicName, classicPath, classicCloseCall, dbus.FlagNoReplyExpected, nil, classic)
	}
	if err != nil {
		logutil.L().Debug("notify: withdraw failed", "id", id, "err", err)
	}
}

// Shutdown lets queued notifications go out, within callTimeout in all,
// then closes the connection; the notifier goes inert. A worker stuck
// past that keeps its goroutine until the stuck call returns, and the
// connection closes without waiting for it.
func (n *Notifier) Shutdown() {
	settled := n.queue.Stop(callTimeout)
	n.cancel()
	n.mu.Lock()
	bus, unwatch := n.bus, n.unwatch
	n.bus, n.unwatch, n.mode, n.closed = nil, nil, transportNone, true
	n.mu.Unlock()
	if bus == nil {
		return
	}
	if settled {
		n.release(bus, unwatch)
		return
	}
	go n.release(bus, unwatch)
}

// OnAction registers the activation callback (id, action key); nil
// disables. It fires on the signal goroutine.
func (n *Notifier) OnAction(fn func(id, action string)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onAction = fn
}

// OnClosed registers the close callback; nil disables.
func (n *Notifier) OnClosed(fn func(id string, reason CloseReason)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.onClosed = fn
}

func (n *Notifier) fireAction(id, action string) {
	n.mu.Lock()
	fn := n.onAction
	n.mu.Unlock()
	if fn != nil {
		fn(id, action)
	}
}

func (n *Notifier) fireClosed(id string, reason CloseReason) {
	n.mu.Lock()
	fn := n.onClosed
	n.mu.Unlock()
	if fn != nil {
		fn(id, reason)
	}
}

// rememberClassic maps the daemon's numeric id to lys-ui's string id,
// so classic signals route like portal ones.
func (n *Notifier) rememberClassic(classic uint32, id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.classicIDs == nil {
		n.classicIDs = map[uint32]string{}
		n.classicOf = map[string]uint32{}
	}
	n.classicIDs[classic] = id
	n.classicOf[id] = classic
}

// classicID resolves the daemon's numeric id; unknown ids (replaced or
// reaped notifications) resolve to "" and fire nothing.
func (n *Notifier) classicID(classic uint32) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.classicIDs[classic]
}

func (n *Notifier) forgetClassic(classic uint32) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.classicOf, n.classicIDs[classic])
	delete(n.classicIDs, classic)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
