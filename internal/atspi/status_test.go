package atspi

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	"github.com/stubbedev/lys-lib/dbusx"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/atspi/proto"
	"github.com/stubbedev/lys-ui/internal/fdtest"
)

// fakeStatus plays at-spi-bus-launcher's org.a11y.Status properties.
type fakeStatus struct {
	mu           sync.Mutex
	enabled      bool
	screenReader bool
}

func (f *fakeStatus) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch prop {
	case "IsEnabled":
		return dbus.MakeVariant(f.enabled), nil
	case "ScreenReaderEnabled":
		return dbus.MakeVariant(f.screenReader), nil
	}
	return dbus.Variant{}, dbus.MakeFailedError(nil)
}

func (f *fakeStatus) GetAll(string) (map[string]dbus.Variant, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return map[string]dbus.Variant{"IsEnabled": dbus.MakeVariant(f.enabled), "ScreenReaderEnabled": dbus.MakeVariant(f.screenReader)}, nil
}

// The watch reports the switch at once and follows a screen reader
// starting and stopping; a bus without org.a11y.Bus is an error.
func TestWatchStatus(t *testing.T) {
	bus := dbustest.Start(t)
	watcher := bus.Conn(t)
	if _, err := WatchStatus(watcher, func(bool) {}); err == nil {
		t.Fatal("a bus without org.a11y.Bus watched")
	}
	conn := bus.Conn(t)
	if _, err := conn.RequestName(proto.BusName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	status := &fakeStatus{}
	if err := conn.ExportAll(status, proto.BusPath, "org.freedesktop.DBus.Properties"); err != nil {
		t.Fatal(err)
	}
	got := make(chan bool, 4)
	stop, err := WatchStatus(watcher, func(on bool) { got <- on })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	next := func() bool {
		t.Helper()
		select {
		case on := <-got:
			return on
		case <-time.After(3 * time.Second):
			t.Fatal("no status report")
			return false
		}
	}
	if next() {
		t.Fatal("initial status enabled, want disabled")
	}
	set := func(screenReader bool) {
		status.mu.Lock()
		status.screenReader = screenReader
		status.mu.Unlock()
		if err := conn.Emit(proto.BusPath, "org.freedesktop.DBus.Properties.PropertiesChanged", proto.IfaceStatus,
			map[string]dbus.Variant{"ScreenReaderEnabled": dbus.MakeVariant(screenReader)}, []string{}); err != nil {
			t.Fatal(err)
		}
	}
	set(true)
	if !next() {
		t.Error("a screen reader starting did not enable")
	}
	set(false)
	if next() {
		t.Error("a screen reader stopping did not disable")
	}
}

func TestAStoppedWatchHandsItsConnectionBack(t *testing.T) {
	bus := dbustest.Start(t)
	conn := bus.Conn(t)
	if _, err := conn.RequestName(proto.BusName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	status := &fakeStatus{}
	if err := conn.ExportAll(status, proto.BusPath, "org.freedesktop.DBus.Properties"); err != nil {
		t.Fatal(err)
	}
	shared := bus.Conn(t)
	reports := 0
	stop, err := WatchStatus(shared, func(bool) { reports++ })
	if err != nil {
		t.Fatal(err)
	}
	stop()
	stop()
	if err := shared.BusObject().Call("org.freedesktop.DBus.GetId", 0).Err; err != nil {
		t.Fatalf("stop closed the shared connection: %v", err)
	}
	if reports != 1 {
		t.Errorf("%d reports, want the one before WatchStatus returned", reports)
	}
	again, err := WatchStatus(shared, func(bool) {})
	if err != nil {
		t.Fatalf("a second watch on the same connection: %v", err)
	}
	again()
}

// Every signal the watch is sent closes its descriptors: those in the
// owner's PropertiesChanged, in its changed values and in arguments
// after the declared ones, and those of a PropertiesChanged another
// peer sends in the launcher's place, which changes nothing.
func TestAStatusWatchClosesTheDescriptorsOfEverySignal(t *testing.T) {
	bus := dbustest.Start(t)
	launcher := bus.Conn(t)
	if _, err := launcher.RequestName(proto.BusName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	if err := launcher.ExportAll(&fakeStatus{}, proto.BusPath, "org.freedesktop.DBus.Properties"); err != nil {
		t.Fatal(err)
	}
	impostor := bus.Conn(t)
	got := make(chan bool, 1)
	stop, err := WatchStatus(bus.Conn(t), func(on bool) { got <- on })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	await := func(want bool) {
		t.Helper()
		select {
		case on := <-got:
			if on != want {
				t.Fatalf("status %v, want %v", on, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("no status report")
		}
	}
	await(false)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	fd := dbus.UnixFD(w.Fd())
	emit := func(conn *dbus.Conn, screenReader bool) {
		t.Helper()
		changed := map[string]dbus.Variant{"ScreenReaderEnabled": dbus.MakeVariant(screenReader), "Handed": dbus.MakeVariant(fd)}
		if err := conn.Emit(proto.BusPath, dbusx.PropertiesChanged.Name(), proto.IfaceStatus, changed, []string{}, fd); err != nil {
			t.Fatal(err)
		}
	}
	emit(launcher, true)
	await(true)
	before := fdtest.Own(t)
	const events = 16
	for range events {
		emit(impostor, false)
	}
	if err := impostor.BusObject().Call("org.freedesktop.DBus.GetId", 0).Err; err != nil {
		t.Fatal(err)
	}
	for i := range events {
		on := i%2 == 1
		emit(launcher, on)
		await(on)
	}
	fdtest.RequireNoneBeyond(t, before, "the watch read what the launcher and another peer sent")
}
