// Package atspitest runs real at-spi2-core for a test and reads the
// accessible tree the way an assistive technology does: the bus
// launcher brings up the accessibility bus on a private session bus,
// the registry daemon serves the desktop, and the test walks the
// applications under it over the accessibility bus.
package atspitest

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/dbustest"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"

	"github.com/stubbedev/lys-ui/internal/atspi/proto"
)

// LibexecEnv names the environment variable that points at
// at-spi2-core's libexec directory, where at-spi-bus-launcher and
// at-spi2-registryd live off PATH.
const LibexecEnv = "ATSPI_LIBEXEC"

// SchemaDirEnv names the environment variable that points at the
// compiled GSettings schemas holding org.gnome.desktop.interface: the
// bus launcher keeps the accessibility switch in its
// toolkit-accessibility key, and without the schema IsEnabled reads
// false whatever was set.
const SchemaDirEnv = "ATSPI_GSETTINGS_SCHEMA_DIR"

const waitBound = 5 * time.Second

// Core is real at-spi2-core serving one test.
type Core struct {
	// Session is the private session bus the test now runs on:
	// DBUS_SESSION_BUS_ADDRESS names it, so an application started
	// after Start discovers the accessibility bus through it.
	Session *dbustest.Bus
	// AT is the assistive technology's connection to the
	// accessibility bus.
	AT *dbus.Conn
}

// Start brings up at-spi-bus-launcher on a private session bus and
// at-spi2-registryd on the accessibility bus it starts, and waits until
// the registry owns its name; everything stops at cleanup, and a daemon
// left behind fails the test. Without LibexecEnv the test skips.
func Start(t *testing.T) *Core {
	t.Helper()
	libexec := os.Getenv(LibexecEnv)
	if libexec == "" {
		t.Skipf("%s unset; the dev shell points it at at-spi2-core's libexec", LibexecEnv)
	}
	schemas := os.Getenv(SchemaDirEnv)
	if schemas == "" {
		t.Fatalf("atspitest: %s is set but %s is not; the dev shell points it at gsettings-desktop-schemas", LibexecEnv, SchemaDirEnv)
	}
	bus := dbustest.Session(t)
	runtime := t.TempDir()
	// dbus-daemon, not dbus-broker: under a systemd user session the
	// launcher prefers the broker, which only runs with systemd's
	// socket activation and so dies on this private bus. The switch
	// lives in memory, never in the desktop's own settings.
	launcher := exec.Command(filepath.Join(libexec, "at-spi-bus-launcher"), "--launch-immediately")
	launcher.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtime, "ATSPI_DBUS_IMPLEMENTATION=dbus-daemon", "GSETTINGS_BACKEND=memory", "GSETTINGS_SCHEMA_DIR="+schemas)
	// The accessibility bus's dbus-daemon outlives a killed launcher:
	// the launcher leads a process group, and the whole group goes.
	launcher.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := launcher.Start(); err != nil {
		t.Fatalf("atspitest: start at-spi-bus-launcher: %v", err)
	}
	t.Cleanup(func() { killGroup(t, launcher, runtime) })

	sess := bus.Conn(t)
	var address string
	eventually(t, proto.BusName, func() error {
		return sess.Object(proto.BusName, proto.BusPath).Call(proto.BusName+".GetAddress", 0).Store(&address)
	})
	// The registry starts directly: on a host with a systemd user
	// manager the accessibility bus would activate the desktop
	// session's unit instead of one on this private bus.
	registryd := exec.Command(filepath.Join(libexec, "at-spi2-registryd"))
	registryd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtime, "DBUS_SESSION_BUS_ADDRESS="+bus.Address)
	if err := registryd.Start(); err != nil {
		t.Fatalf("atspitest: start at-spi2-registryd: %v", err)
	}
	t.Cleanup(func() {
		_ = registryd.Process.Kill()
		_ = registryd.Wait()
	})
	at, err := dbus.Connect(address)
	if err != nil {
		t.Fatalf("atspitest: connect to the accessibility bus %s: %v", address, err)
	}
	t.Cleanup(func() { _ = at.Close() })
	eventually(t, "the registry on the accessibility bus", func() error {
		var has bool
		if err := at.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, proto.RegistryName).Store(&has); err != nil {
			return err
		}
		if !has {
			return errors.New(proto.RegistryName + " has no owner")
		}
		return nil
	})
	return &Core{Session: bus, AT: at}
}

// Enable turns the desktop's accessibility switch on (org.a11y.Status
// IsEnabled), as the desktop's accessibility setting does: toolkits
// that follow the switch start serving.
func (c *Core) Enable(t *testing.T) {
	t.Helper()
	status := c.Session.Conn(t).Object(proto.BusName, proto.BusPath)
	if err := status.SetProperty(proto.IfaceStatus+".IsEnabled", dbus.MakeVariant(true)); err != nil {
		t.Fatalf("atspitest: turn %s.IsEnabled on: %v", proto.IfaceStatus, err)
	}
	v, err := status.GetProperty(proto.IfaceStatus + ".IsEnabled")
	if err != nil {
		t.Fatalf("atspitest: read %s.IsEnabled back: %v", proto.IfaceStatus, err)
	}
	if on, _ := v.Value().(bool); !on {
		t.Fatalf("atspitest: %s.IsEnabled reads %v after turning it on", proto.IfaceStatus, v)
	}
}

// Desktop is the registry's desktop, the root every application embeds
// under.
func (c *Core) Desktop() Accessible {
	return Accessible{conn: c.AT, Bus: proto.RegistryName, Path: proto.RootPath}
}

// Application waits until exactly one application whose toolkit is
// toolkit sits under the desktop and returns its root.
func (c *Core) Application(t *testing.T, toolkit string) Accessible {
	t.Helper()
	var found []Accessible
	eventually(t, "a "+toolkit+" application under the registry's desktop", func() error {
		var err error
		if found, err = c.applications(toolkit); err != nil {
			return err
		}
		if len(found) == 0 {
			return errors.New("none listed yet")
		}
		return nil
	})
	if len(found) != 1 {
		t.Fatalf("atspitest: %d %s applications under the desktop, want one", len(found), toolkit)
	}
	return found[0]
}

func (c *Core) applications(toolkit string) ([]Accessible, error) {
	apps, err := c.Desktop().Children()
	if err != nil {
		return nil, err
	}
	var out []Accessible
	for _, a := range apps {
		v, err := a.Object().GetProperty(proto.IfaceApplication + ".ToolkitName")
		if err != nil {
			return nil, fmt.Errorf("atspitest: the toolkit of %s %s: %w", a.Bus, a.Path, err)
		}
		if name, _ := v.Value().(string); name == toolkit {
			out = append(out, a)
		}
	}
	return out, nil
}

// Accessible is one object on the accessibility bus.
type Accessible struct {
	conn *dbus.Conn
	Bus  string
	Path dbus.ObjectPath
}

// Object is the accessible's D-Bus object, for the interfaces this
// package does not read.
func (a Accessible) Object() dbus.BusObject { return a.conn.Object(a.Bus, a.Path) }

// Name is the accessible's name, as a screen reader speaks it.
func (a Accessible) Name() (string, error) {
	v, err := a.Object().GetProperty(proto.IfaceAccessible + ".Name")
	if err != nil {
		return "", fmt.Errorf("atspitest: the name of %s %s: %w", a.Bus, a.Path, err)
	}
	name, ok := v.Value().(string)
	if !ok {
		return "", fmt.Errorf("atspitest: the name of %s %s is a %s", a.Bus, a.Path, v.Signature())
	}
	return name, nil
}

// Children are the accessible's children in order.
func (a Accessible) Children() ([]Accessible, error) {
	var refs []struct {
		Name string
		Path dbus.ObjectPath
	}
	if err := a.Object().Call(proto.IfaceAccessible+".GetChildren", 0).Store(&refs); err != nil {
		return nil, fmt.Errorf("atspitest: the children of %s %s: %w", a.Bus, a.Path, err)
	}
	out := make([]Accessible, len(refs))
	for i, r := range refs {
		out[i] = Accessible{conn: a.conn, Bus: r.Name, Path: r.Path}
	}
	return out, nil
}

// Names walks the tree under a, a first, depth first in child order,
// and lists every name read.
func (a Accessible) Names() ([]string, error) {
	var out []string
	var walk func(Accessible) error
	walk = func(n Accessible) error {
		name, err := n.Name()
		if err != nil {
			return err
		}
		out = append(out, name)
		kids, err := n.Children()
		if err != nil {
			return err
		}
		for _, k := range kids {
			if err := walk(k); err != nil {
				return err
			}
		}
		return nil
	}
	return out, walk(a)
}

func eventually(t *testing.T, what string, cond func() error) {
	t.Helper()
	deadline := time.Now().Add(waitBound)
	for {
		last := cond()
		if last == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("atspitest: timed out waiting for %s: %v", what, last)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// killGroup kills the process group cmd leads and fails the test if a
// process still names dir (the bus's runtime directory) once the group
// is reaped.
func killGroup(t *testing.T, cmd *exec.Cmd, dir string) {
	t.Helper()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	deadline := time.Now().Add(waitBound)
	for {
		left := processesNaming(t, dir)
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("atspitest: %s left running: %v", cmd.Path, left)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func processesNaming(t *testing.T, s string) []string {
	t.Helper()
	cmdlines, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil {
		t.Fatalf("atspitest: list processes: %v", err)
	}
	var out []string
	for _, path := range cmdlines {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if line := strings.ReplaceAll(string(raw), "\x00", " "); strings.Contains(line, s) {
			out = append(out, line)
		}
	}
	return out
}
