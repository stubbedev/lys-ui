package atspi

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	"github.com/stubbedev/lys-lib/render"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/widget"
)

// TestRealAtSpi2Core verifies the bridge against real at-spi2-core
// (`just atspi-verify`): at-spi-bus-launcher on a private session bus
// starts the accessibility bus, the bridge discovers it through
// org.a11y.Bus and embeds with the real registryd, and a client playing
// the assistive technology finds the application under the registry's
// desktop, walks to the entry, reads it, and moves its caret - what
// accerciser does by hand. ATSPI_LIBEXEC names at-spi2-core's libexec
// directory; without it the test skips.
func TestRealAtSpi2Core(t *testing.T) {
	libexec := os.Getenv("ATSPI_LIBEXEC")
	if libexec == "" {
		t.Skip("ATSPI_LIBEXEC unset; `just atspi-verify` runs this against real at-spi2-core")
	}
	bus := dbustest.Session(t)
	session := bus.Address
	runtime := t.TempDir()
	// dbus-daemon, not dbus-broker: under a systemd user session the
	// launcher prefers the broker, which only runs with systemd's
	// socket activation and so dies on this private bus ("No listener
	// socket inherited").
	launcher := exec.Command(filepath.Join(libexec, "at-spi-bus-launcher"), "--launch-immediately")
	launcher.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtime, "ATSPI_DBUS_IMPLEMENTATION=dbus-daemon")
	// The launcher spawns the accessibility bus's dbus-daemon, which
	// outlives a killed launcher: the launcher leads a process group of
	// its own, and the whole group goes at cleanup.
	launcher.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := launcher.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { killGroup(t, launcher, runtime) })

	// The launcher owns org.a11y.Bus once the accessibility bus runs.
	sess := bus.Conn(t)
	var a11yAddr string
	eventually(t, "org.a11y.Bus", func() bool {
		return sess.Object(busName, busPath).Call("org.a11y.Bus.GetAddress", 0).Store(&a11yAddr) == nil
	})
	// Start the registry daemon directly: on a host running a systemd
	// user manager the accessibility bus activates it through systemd
	// (SystemdService=), which serves the desktop session's unit, not
	// this private bus - the activation fails ("unit failed"). The
	// daemon finds the accessibility bus through org.a11y.Bus, as on a
	// desktop.
	registryd := exec.Command(filepath.Join(libexec, "at-spi2-registryd"))
	registryd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtime, "DBUS_SESSION_BUS_ADDRESS="+session)
	if err := registryd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = registryd.Process.Kill()
		_ = registryd.Wait()
	})
	at, err := dbus.Connect(a11yAddr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = at.Close() })
	// The bridge embeds once, at Serve: the registry must own its name
	// by then, as it does long before any application on a desktop.
	eventually(t, "the registry on the accessibility bus", func() bool {
		var has bool
		return at.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.a11y.atspi.Registry").Store(&has) == nil && has
	})

	// The desktop switch: turning IsEnabled on reaches WatchStatus.
	got := make(chan bool, 4)
	stopWatch, err := WatchStatus("", func(on bool) { got <- on })
	if err != nil {
		t.Fatal(err)
	}
	defer stopWatch()
	<-got
	if err := sess.Object(busName, busPath).SetProperty(ifaceStatus+".IsEnabled", dbus.MakeVariant(true)); err != nil {
		t.Fatalf("set IsEnabled: %v", err)
	}
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
	desktop := at.Object("org.a11y.atspi.Registry", RootPath)
	var apps []ref
	eventually(t, "the application under the registry's desktop", func() bool {
		apps = nil
		if err := desktop.Call(ifaceAccessible+".GetChildren", 0).Store(&apps); err != nil {
			return false
		}
		return slices.ContainsFunc(apps, func(r ref) bool { return r.Name == owner || r.Name == br.name })
	})

	app := at.Object(owner, RootPath)
	var toolkit string
	if v, err := app.GetProperty(ifaceApplication + ".ToolkitName"); err != nil {
		t.Fatalf("ToolkitName: %v", err)
	} else if toolkit, _ = v.Value().(string); toolkit != "lys-ui" {
		t.Errorf("toolkit = %q, want lys-ui", toolkit)
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
}

// eventually polls cond for up to five seconds.
// killGroup kills the process group cmd leads, and fails the test if
// any process still names dir (the bus's runtime directory) once the
// group is reaped: a daemon left behind.
func killGroup(t *testing.T, cmd *exec.Cmd, dir string) {
	t.Helper()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for {
		left := processesNaming(t, dir)
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("%s left running: %v", cmd.Path, left)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// processesNaming lists the command lines that mention s.
func processesNaming(t *testing.T, s string) []string {
	t.Helper()
	cmdlines, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, path := range cmdlines {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue // exited since the glob
		}
		if line := strings.ReplaceAll(string(raw), "\x00", " "); strings.Contains(line, s) {
			out = append(out, line)
		}
	}
	return out
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
