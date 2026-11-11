package notify

import (
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	"github.com/stubbedev/lys-lib/dbusx"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/fdtest"
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
		if err := conn.Emit(classicPath, classicActionInvoked.Name(), uint32(7), "open"); err != nil {
			t.Error(err)
		}
	})
	if got != id+"/open" {
		t.Errorf("action = %q, want %q", got, id+"/open")
	}
	reason := awaitOne(t, closes, func() {
		if err := conn.Emit(classicPath, classicNotificationClosed.Name(), uint32(7), uint32(ReasonDismissed)); err != nil {
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
		if err := conn.Emit(portalPath, portalActionInvoked.Name(), id, "open", []dbus.Variant{}); err != nil {
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
	routes := f.routes
	f.mu.Unlock()
	closed := make(chan struct{})
	n.OnClosed(func(string, CloseReason) { close(closed) })
	const daemon = ":1.9"
	sig := &dbus.Signal{Sender: daemon, Path: classicPath, Name: classicNotificationClosed.Name(), Body: []any{uint32(9), uint32(ReasonClosedByCall)}}
	if err := dbusx.Dispatch(sig, daemon, routes); err != nil {
		t.Fatal(err)
	}
	<-closed
	deadline := time.Now().Add(patience)
	for n.classicID(9) != "" {
		if time.Now().After(deadline) {
			t.Fatal("the closed notification stayed in the id maps")
		}
		runtime.Gosched()
	}
}

// Every signal the notifier is sent closes its descriptors: those the
// daemon appends to the spec's arguments, and those of an ActionInvoked
// another peer sends in the daemon's place, which fires nothing.
func TestSignalsCloseTheirDescriptors(t *testing.T) {
	bus := dbustest.Session(t)
	conn := bus.Conn(t)
	d := &classicDaemon{sent: make(chan uint32, 4)}
	if err := conn.ExportAll(d, classicPath, classicIface); err != nil {
		t.Fatal(err)
	}
	claim(t, conn, classicName)
	impostor := bus.Conn(t)
	n := New()
	defer n.Shutdown()
	actions := make(chan string, 64)
	n.OnAction(func(_, action string) { actions <- action })
	n.Notify(Notification{Title: "t"})
	<-d.sent
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	fd := dbus.UnixFD(w.Fd())
	emit := func(from *dbus.Conn, action string) {
		t.Helper()
		if err := from.Emit(classicPath, classicActionInvoked.Name(), uint32(7), action, fd); err != nil {
			t.Error(err)
		}
	}
	await := func(want string) {
		t.Helper()
		select {
		case got := <-actions:
			if got != want {
				t.Fatalf("action %q, want %q", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("the daemon's ActionInvoked %q never arrived", want)
		}
	}
	emit(conn, "warm")
	await("warm")
	before := fdtest.Own(t)
	const events = 16
	for range events {
		emit(impostor, "forged")
	}
	if err := impostor.BusObject().Call("org.freedesktop.DBus.GetId", 0).Err; err != nil {
		t.Fatal(err)
	}
	for i := range events {
		want := "open" + strconv.Itoa(i)
		emit(conn, want)
		await(want)
	}
	fdtest.RequireNoneBeyond(t, before, "the notifier read what the daemon and another peer sent")
}
