package atspi

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/render"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/atspitest"
	"github.com/stubbedev/lys-ui/internal/atspi/proto"
	"github.com/stubbedev/lys-ui/widget"
)

// TestRealAtSpi2Core verifies the bridge against real at-spi2-core
// (`just atspi-verify`): atspitest brings up the accessibility bus and
// the registry on a private session bus, the bridge discovers it
// through org.a11y.Bus and embeds with the real registryd, and a client
// playing the assistive technology finds the application under the
// registry's desktop, walks to the entry, reads it, and moves its caret
// - what accerciser does by hand. Without ATSPI_LIBEXEC the test skips.
func TestRealAtSpi2Core(t *testing.T) {
	core := atspitest.Start(t)
	at := core.AT

	// The desktop switch: turning IsEnabled on reaches WatchStatus.
	got := make(chan bool, 4)
	stopWatch, err := WatchStatus(core.Session.Conn(t), func(on bool) { got <- on })
	if err != nil {
		t.Fatal(err)
	}
	defer stopWatch()
	<-got
	core.Enable(t)
	select {
	case on := <-got:
		if !on {
			t.Error("IsEnabled on reported disabled")
		}
	case <-time.After(3 * time.Second):
		t.Error("IsEnabled change never reached the watch")
	}

	// Serve a window holding an entry; discovery goes through the
	// launcher like on a desktop.
	face := faceOf(t)
	entry := widget.NewEntry(face, 13, render.RGB(0, 0, 0))
	entry.SetText("hello")
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(entry, false)
	arrange(t, root, 300, 100)
	scene := &testScene{roots: []widget.Widget{root}}
	br, err := Serve(scene, Options{Poll: -1})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer br.Stop()
	br.Refresh()

	var owner string
	if err := at.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, br.name).Store(&owner); err != nil {
		t.Fatal(err)
	}
	// The real registry lists the application under its desktop.
	if listed := core.Application(t, "lys-ui"); listed.Bus != owner && listed.Bus != br.name {
		t.Fatalf("the desktop lists the application as %s, want %s or %s", listed.Bus, owner, br.name)
	}

	app := at.Object(owner, proto.RootPath)
	var registry string
	if err := at.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, proto.RegistryName).Store(&registry); err != nil {
		t.Fatal(err)
	}
	if v, err := app.GetProperty(proto.IfaceAccessible + ".Parent"); err != nil {
		t.Fatalf("the root's Parent: %v", err)
	} else if parent, ok := v.Value().([]any); !ok || len(parent) != 2 || parent[0] != registry || parent[1] != proto.RootPath {
		t.Errorf("the root's parent is %v, want the registry's desktop (%s, %s), which libatspi recognizes", v.Value(), registry, proto.RootPath)
	}
	var items []cacheItem
	if err := at.Object(owner, CachePath).Call(ifaceCache+".GetItems", 0).Store(&items); err != nil {
		t.Fatalf("Cache.GetItems: %v", err)
	}
	var entryPath dbus.ObjectPath
	for _, it := range items {
		if it.Role == roleEntry {
			entryPath = it.Path.Path
		}
	}
	if entryPath == "" {
		t.Fatalf("no entry among %d cache items", len(items))
	}
	obj := at.Object(owner, entryPath)
	var text string
	if err := obj.Call(ifaceText+".GetText", 0, int32(0), int32(-1)).Store(&text); err != nil || text != "hello" {
		t.Errorf("GetText = %q, %v", text, err)
	}
	var ok bool
	err = obj.Call(ifaceText+".SetCaretOffset", 0, int32(3)).Store(&ok)
	var caret int
	scene.Invoke(func() { caret = entry.Cursor() })
	if err != nil || !ok || caret != 3 {
		t.Errorf("SetCaretOffset: ok %v err %v caret %d", ok, err, caret)
	}

	// An announcement reaches the assistive technology as one
	// object:announcement on the entry, its politeness and text with it.
	if err := at.AddMatchSignal(dbus.WithMatchInterface("org.a11y.atspi.Event.Object"), dbus.WithMatchMember("Announcement")); err != nil {
		t.Fatal(err)
	}
	signals := make(chan *dbus.Signal, 16)
	at.Signal(signals)
	scene.Invoke(func() { widget.Announce(entry, "hello world", widget.Polite) })
	br.Refresh()
	var heard []*dbus.Signal
	timeout := time.After(3 * time.Second)
	for len(heard) == 0 {
		select {
		case s := <-signals:
			if s.Name == "org.a11y.atspi.Event.Object.Announcement" {
				heard = append(heard, s)
			}
		case <-timeout:
			t.Fatal("no Announcement reached the assistive technology")
		}
	}
	s := heard[0]
	if s.Path != entryPath || len(s.Body) != 5 || s.Body[1] != int32(widget.Polite) || s.Body[3].(dbus.Variant).Value() != "hello world" {
		t.Errorf("announcement %s %v, want the text, polite, on the entry %s", s.Path, s.Body, entryPath)
	}
	select {
	case s := <-signals:
		if s.Name == "org.a11y.atspi.Event.Object.Announcement" {
			t.Errorf("a second announcement %v", s.Body)
		}
	case <-time.After(200 * time.Millisecond):
	}
}
