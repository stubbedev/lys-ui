package atspi

import (
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
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

// The watch reports the switch at once and follows a screen reader
// starting and stopping; a bus without org.a11y.Bus is an error.
func TestWatchStatus(t *testing.T) {
	bus := dbustest.Start(t)
	watcher := bus.Conn(t)
	if _, err := WatchStatus(watcher, func(bool) {}); err == nil {
		t.Fatal("a bus without org.a11y.Bus watched")
	}
	conn := bus.Conn(t)
	if _, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	status := &fakeStatus{}
	if err := conn.ExportAll(status, busPath, "org.freedesktop.DBus.Properties"); err != nil {
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
		if err := conn.Emit(busPath, "org.freedesktop.DBus.Properties.PropertiesChanged", ifaceStatus,
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
	if _, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatal(err)
	}
	status := &fakeStatus{}
	if err := conn.ExportAll(status, busPath, "org.freedesktop.DBus.Properties"); err != nil {
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
