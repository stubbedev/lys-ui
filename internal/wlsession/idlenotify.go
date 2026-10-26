package wlsession

import (
	"errors"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// ext-idle-notify-v1: the compositor tells the client when the user
// went idle for a requested time and when they came back - the input a
// panel's dim indicator or a lock-screen policy reads. It complements
// idle-inhibit, which asks the compositor not to idle. Without the
// protocol, idle notification is unavailable and nothing fires.

// ErrIdleNotifyUnavailable reports a compositor without
// ext-idle-notify-v1 (or no seat to watch).
var ErrIdleNotifyUnavailable = errors.New("wlsession: idle notification unavailable")

// IdleNotification is one idle watch: OnIdle fires once the seat has
// been idle for its timeout, OnResume when activity returns. Both run
// on the loop goroutine. Destroy ends the watch.
type IdleNotification struct {
	n      *wlr.ExtIdleNotificationV1
	idle   bool
	OnIdle func()
	// OnResume fires when the user is active again after an idle.
	OnResume func()
}

// Idle reports whether the seat is idle by this watch's timeout.
func (n *IdleNotification) Idle() bool { return n.idle }

// Destroy ends the watch.
func (n *IdleNotification) Destroy() {
	if n.n != nil {
		_ = n.n.Destroy()
		n.n = nil
	}
}

// HandleExtIdleNotificationV1Idled implements the idled handler.
func (n *IdleNotification) HandleExtIdleNotificationV1Idled(wlr.ExtIdleNotificationV1IdledEvent) {
	n.idle = true
	debug.Log("seat", "idle notified")
	if n.OnIdle != nil {
		n.OnIdle()
	}
}

// HandleExtIdleNotificationV1Resumed implements the resumed handler.
func (n *IdleNotification) HandleExtIdleNotificationV1Resumed(wlr.ExtIdleNotificationV1ResumedEvent) {
	n.idle = false
	debug.Log("seat", "idle resumed")
	if n.OnResume != nil {
		n.OnResume()
	}
}

// bindIdleNotifier binds the notifier global.
func (s *Session) bindIdleNotifier(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	mgr := wlr.NewExtIdleNotifierV1(ctx)
	if s.bindOptional(ev, 2, mgr) {
		s.idleNotifier, s.idleNotifierVersion = mgr, min(ev.Version, 2)
		debug.Log("shell", "ext-idle-notify-v1 bound")
	}
}

// IdleNotifyAvailable reports whether the compositor offers idle
// notification.
func (s *Session) IdleNotifyAvailable() bool { return s.idleNotifier != nil && s.seat != nil }

// IdleNotify watches the seat for timeout of idleness. inputOnly (the
// protocol's v2) counts only real input, ignoring idle inhibitors - a
// presence indicator wants that; a dim-the-screen policy wants the
// default, which honors them. Fails when the protocol is unavailable.
func (s *Session) IdleNotify(timeout time.Duration, inputOnly bool) (*IdleNotification, error) {
	if !s.IdleNotifyAvailable() {
		return nil, ErrIdleNotifyUnavailable
	}
	ms := intconv.Uint32(max(timeout.Milliseconds(), 1))
	var (
		obj *wlr.ExtIdleNotificationV1
		err error
	)
	if inputOnly && s.idleNotifierVersion >= 2 {
		obj, err = s.idleNotifier.GetInputIdleNotification(ms, s.seat)
	} else {
		obj, err = s.idleNotifier.GetIdleNotification(ms, s.seat)
	}
	if err != nil {
		return nil, err
	}
	n := &IdleNotification{n: obj}
	obj.AddIdledHandler(n)
	obj.AddResumedHandler(n)
	return n, nil
}
