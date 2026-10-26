// Package notify sends desktop notifications: the
// xdg-desktop-portal Notification interface when a portal owns the
// desktop, org.freedesktop.Notifications (the GNOME spec every daemon
// implements) as the fallback, and nothing at all without a session
// bus - the appearance.md failure model, inert and logged at Debug.
// Pure Go over the existing godbus dependency.
//
// Threading matches internal/portalsettings: Notify blocks on the bus call
// and belongs off the Wayland loop goroutine; OnAction and OnClosed
// callbacks run serialized on the notifier's own goroutine, and the
// application bridges onto its loop with Invoke. Every method is safe
// from any goroutine once New returned.
package notify

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/stubbedev/lys-ui/internal/logutil"
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

// Notifier owns one session-bus connection and sends notifications
// through the best transport it found at startup. Without a bus (or
// with neither name owned) it is inert: Notify is a no-op that still
// returns an id, callbacks never fire, and Close is a no-op.
type Notifier struct {
	// dial is the connection source; nil on an inert notifier.
	dial func() (*dbus.Conn, error)

	mu     sync.Mutex
	conn   *dbus.Conn
	mode   transport
	nextID atomic.Int64
	closed bool
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
	portalActionSig   = portalIface + ".ActionInvoked"

	classicName      = "org.freedesktop.Notifications"
	classicPath      = dbus.ObjectPath("/org/freedesktop/Notifications")
	classicIface     = "org.freedesktop.Notifications"
	classicNotify    = classicIface + ".Notify"
	classicCloseCall = classicIface + ".CloseNotification"
	classicActionSig = classicIface + ".ActionInvoked"
	classicClosedSig = classicIface + ".NotificationClosed"

	// callTimeout bounds every bus call; notifications are fire and
	// forget, never worth hanging a caller for the godbus default 25s.
	callTimeout = 3 * time.Second

	// signalBuffer rides a slow app handler without godbus spawning a
	// goroutine per signal.
	signalBuffer = 16
)

// New probes the session bus once and returns the notifier: portal
// transport when the desktop portal owns its name, the classic spec
// when its daemon does, and an inert notifier without a bus. The probe
// itself is bounded by callTimeout; New blocks that long at most.
func New() *Notifier {
	n := &Notifier{dial: dialSession}
	conn, err := dialSession()
	if err != nil {
		logutil.L().Debug("notify: no session bus; desktop notifications disabled")
		n.dial = nil
		return n
	}
	n.conn = conn
	n.mode = n.probe(conn)
	if n.mode == transportNone {
		_ = conn.Close()
		n.conn = nil
		n.dial = nil
		return n
	}
	n.subscribe()
	return n
}

// dialSession is New's connection source, the appearance package's
// hygiene rule: a private connection the notifier owns and closes.
func dialSession() (*dbus.Conn, error) {
	return dbus.ConnectSessionBus()
}

// probe picks the transport by owned names.
func (n *Notifier) probe(conn *dbus.Conn) transport {
	var owned bool
	if err := conn.Object("org.freedesktop.DBus", "/").CallWithContext(
		conn.Context(), "org.freedesktop.DBus.NameHasOwner", 0,
		portalDesktopName).Store(&owned); err == nil && owned {
		return transportPortal
	}
	if err := conn.Object("org.freedesktop.DBus", "/").CallWithContext(
		conn.Context(), "org.freedesktop.DBus.NameHasOwner", 0,
		classicName).Store(&owned); err == nil && owned {
		return transportClassic
	}
	return transportNone
}

// subscribe routes ActionInvoked and NotificationClosed signals to the
// callbacks on one goroutine.
func (n *Notifier) subscribe() {
	if n.conn == nil {
		return
	}
	ch := make(chan *dbus.Signal, signalBuffer)
	n.conn.Signal(ch)
	go func() {
		for sig := range ch {
			switch sig.Name {
			case portalActionSig:
				if len(sig.Body) >= 2 {
					id, _ := sig.Body[0].(string)
					action, _ := sig.Body[1].(string)
					n.fireAction(id, action)
				}
			case classicActionSig:
				if len(sig.Body) >= 2 {
					id, _ := sig.Body[0].(uint32)
					action, _ := sig.Body[1].(string)
					n.fireAction(n.classicID(id), action)
				}
			case classicClosedSig:
				if len(sig.Body) >= 2 {
					id, _ := sig.Body[0].(uint32)
					reason, _ := sig.Body[1].(uint32)
					n.fireClosed(n.classicID(id), CloseReason(min(reason, uint32(ReasonUnspecified))))
				}
			}
		}
	}()
}

// Notify sends nf and returns its id: stable across transports, safe
// to hand to Close. It blocks on the bus call (bounded); without a
// transport it returns the id without sending. The body is capped at
// maxBodyRunes runes.
func (n *Notifier) Notify(nf Notification) string {
	id := "lys-ui-" + time.Now().Format("20060102T150405.000000000") + "-" + itoa(n.nextID.Add(1))
	nf.Body = capBody(nf.Body)
	if n.dial == nil {
		return id
	}
	n.mu.Lock()
	conn, mode := n.conn, n.mode
	n.mu.Unlock()
	if conn == nil || mode == transportNone {
		return id
	}
	ctx, cancel := context.WithTimeout(conn.Context(), callTimeout)
	defer cancel()
	switch mode {
	case transportPortal:
		if err := conn.Object(portalDesktopName, portalPath).CallWithContext(
			ctx, portalAdd, dbus.FlagNoReplyExpected, id, portalDict(nf)).Err; err != nil {
			logutil.L().Debug("notify: portal AddNotification failed", "err", err)
		}
	case transportClassic:
		var classicID uint32
		if err := conn.Object(classicName, classicPath).CallWithContext(
			ctx, classicNotify, 0, "lys-ui", uint32(0), nf.Icon,
			nf.Title, nf.Body, classicActions(nf), classicHints(nf),
			classicTimeout(nf.Timeout)).Store(&classicID); err != nil {
			logutil.L().Debug("notify: Notifications.Notify failed", "err", err)
		} else {
			n.rememberClassic(classicID, id)
		}
	}
	return id
}

// Close withdraws a still-shown notification; unknown ids and inert
// notifiers are no-ops.
func (n *Notifier) Close(id string) {
	n.mu.Lock()
	conn, mode := n.conn, n.mode
	classic := n.classicOf[id]
	n.mu.Unlock()
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(conn.Context(), callTimeout)
	defer cancel()
	if mode == transportPortal {
		_ = conn.Object(portalDesktopName, portalPath).CallWithContext(
			ctx, portalRemove, dbus.FlagNoReplyExpected, id).Err
	} else if mode == transportClassic && classic != 0 {
		_ = conn.Object(classicName, classicPath).CallWithContext(
			ctx, classicCloseCall, dbus.FlagNoReplyExpected, classic).Err
	}
}

// Shutdown closes the connection; the notifier goes inert.
func (n *Notifier) Shutdown() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed || n.conn == nil {
		n.closed = true
		return
	}
	n.closed = true
	_ = n.conn.Close()
	n.conn = nil
	n.dial = nil
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
