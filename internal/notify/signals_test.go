package notify

import (
	"runtime"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

type classicDaemon struct{ sent chan uint32 }

func (d *classicDaemon) Notify(_ string, _ uint32, _, _, _ string, _ []string, _ map[string]dbus.Variant, _ int32) (uint32, *dbus.Error) {
	d.sent <- 7
	return 7, nil
}

func (d *classicDaemon) CloseNotification(uint32) *dbus.Error { return nil }

type portalDaemon struct{ sent chan string }

func (d *portalDaemon) AddNotification(id string, _ map[string]dbus.Variant) *dbus.Error {
	d.sent <- id
	return nil
}

func (d *portalDaemon) RemoveNotification(string) *dbus.Error { return nil }

func claim(t *testing.T, conn *dbus.Conn, name string) {
	t.Helper()
	if reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("claim %s: %v %v", name, reply, err)
	}
}

func awaitOne[T any](t *testing.T, got chan T, emit func()) T {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		emit()
		for range 2000 {
			select {
			case v := <-got:
				return v
			default:
				runtime.Gosched()
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the broadcast signal never reached the notifier")
	panic("unreachable")
}

func TestClassicBroadcastSignalsArrive(t *testing.T) {
	bus := dbustest.Session(t)
	conn := bus.Conn(t)
	d := &classicDaemon{sent: make(chan uint32, 4)}
	if err := conn.ExportAll(d, classicPath, classicIface); err != nil {
		t.Fatal(err)
	}
	claim(t, conn, classicName)
	n := New()
	defer n.Shutdown()
	actions, closes := make(chan string, 64), make(chan CloseReason, 64)
	n.OnAction(func(id, action string) { actions <- id + "/" + action })
	n.OnClosed(func(_ string, reason CloseReason) { closes <- reason })
	id := n.Notify(Notification{Title: "t"})
	<-d.sent
	got := awaitOne(t, actions, func() {
		if err := conn.Emit(classicPath, classicActionSig, uint32(7), "open"); err != nil {
			t.Error(err)
		}
	})
	if got != id+"/open" {
		t.Errorf("action = %q, want %q", got, id+"/open")
	}
	reason := awaitOne(t, closes, func() {
		if err := conn.Emit(classicPath, classicClosedSig, uint32(7), uint32(ReasonDismissed)); err != nil {
			t.Error(err)
		}
	})
	if reason != ReasonDismissed {
		t.Errorf("reason = %d", reason)
	}
}

func TestPortalBroadcastSignalsArrive(t *testing.T) {
	bus := dbustest.Session(t)
	conn := bus.Conn(t)
	d := &portalDaemon{sent: make(chan string, 4)}
	if err := conn.ExportAll(d, portalPath, portalIface); err != nil {
		t.Fatal(err)
	}
	claim(t, conn, portalDesktopName)
	n := New()
	defer n.Shutdown()
	actions := make(chan string, 64)
	n.OnAction(func(id, action string) { actions <- id + "/" + action })
	id := n.Notify(Notification{Title: "t"})
	<-d.sent
	got := awaitOne(t, actions, func() {
		if err := conn.Emit(portalPath, portalActionSig, id, "open", []dbus.Variant{}); err != nil {
			t.Error(err)
		}
	})
	if got != id+"/open" {
		t.Errorf("action = %q, want %q", got, id+"/open")
	}
}

func TestClosedClassicNotificationIsForgotten(t *testing.T) {
	f := &fakeBus{owned: classicName, classic: 9}
	n := NewWithDial(func() (Bus, error) { return f, nil })
	defer n.Shutdown()
	id := n.Notify(Notification{Title: "t"})
	n.Close(id)
	awaitMethod(t, f, classicCloseCall)
	f.mu.Lock()
	ch := f.ch
	f.mu.Unlock()
	closed := make(chan struct{})
	n.OnClosed(func(string, CloseReason) { close(closed) })
	ch <- &dbus.Signal{Name: classicClosedSig, Body: []any{uint32(9), uint32(ReasonClosedByCall)}}
	<-closed
	deadline := time.Now().Add(patience)
	for n.classicID(9) != "" {
		if time.Now().After(deadline) {
			t.Fatal("the closed notification stayed in the id maps")
		}
		runtime.Gosched()
	}
}
