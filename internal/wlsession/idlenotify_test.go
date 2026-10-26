package wlsession

import (
	"errors"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/wlr"
)

// Without the protocol idle notification is unavailable; a watch
// tracks idle and resume and runs its hooks.
func TestIdleNotification(t *testing.T) {
	s := newRoutingSession()
	if _, err := s.IdleNotify(time.Second, false); !errors.Is(err, ErrIdleNotifyUnavailable) || s.IdleNotifyAvailable() {
		t.Errorf("no protocol: %v", err)
	}
	var log []string
	n := &IdleNotification{OnIdle: func() { log = append(log, "idle") }, OnResume: func() { log = append(log, "resume") }}
	n.HandleExtIdleNotificationV1Idled(wlr.ExtIdleNotificationV1IdledEvent{})
	if !n.Idle() {
		t.Error("idled did not mark idle")
	}
	n.HandleExtIdleNotificationV1Resumed(wlr.ExtIdleNotificationV1ResumedEvent{})
	if n.Idle() || len(log) != 2 || log[1] != "resume" {
		t.Errorf("hooks: %v idle %v", log, n.Idle())
	}
	n.Destroy() // no proxy: a no-op
}
