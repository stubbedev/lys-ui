// Package testhome runs a test binary against a scratch home and no
// session but its own, so no test, and no application or daemon it
// starts, reads or writes the real user's files or reaches the session
// of whoever runs it: the session bus, the compositor, the sound
// server. It is lys-desktop's testhome, which lys-ui cannot import,
// with the session bus pinned too: godbus looks for one in
// /run/user/<uid> whatever XDG_RUNTIME_DIR says.
package testhome

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/pulse/native"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wltest"
	"github.com/stubbedev/lys-lib/wmipc"
)

const (
	isolatedEnv   = "LYS_TESTHOME"
	runtimeEnv    = "XDG_RUNTIME_DIR"
	sessionBusEnv = "DBUS_SESSION_BUS_ADDRESS"
	pulseEnv      = "PULSE_SERVER"
	displayEnv    = "WAYLAND_DISPLAY"

	probeTimeout = 2 * time.Second
)

var sessionEnv = [...]string{
	wmipc.SocketEnv,
	displayEnv,
	"WAYLAND_SOCKET",
	"DISPLAY",
	"HYPRLAND_INSTANCE_SIGNATURE",
	"NIRI_SOCKET",
	"MANGO_INSTANCE_SIGNATURE",
	"SWAYSOCK",
	"I3SOCK",
	"XDG_CURRENT_DESKTOP",
	"XDG_ACTIVATION_TOKEN",
}

// Run isolates the test binary (Isolate) in a fresh temp directory,
// runs the tests, removes the directory, and exits with their code.
// Call it from TestMain. A test binary started by an isolated test
// (LYS_TESTHOME is set) runs as it is: its environment is the one its
// parent made for it.
func Run(m *testing.M) { os.Exit(Main(m.Run)) }

// Main is Run for a TestMain with work of its own around the tests:
// it isolates the process, calls run, and returns its code.
func Main(run func() int) int {
	if os.Getenv(isolatedEnv) != "" {
		return run()
	}
	home, err := os.MkdirTemp("", "lys-ui-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
		return 1
	}
	code := 1
	if err := Isolate(home); err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
	} else {
		code = run()
	}
	if err := os.RemoveAll(home); err != nil {
		fmt.Fprintln(os.Stderr, "testhome:", err)
		return 1
	}
	return code
}

// Isolate points HOME and the XDG per-user directories under home and
// marks the process isolated. Outside a headless run it also unsets
// every variable that advertises a running compositor, display or
// desktop, and points the runtime directory, which it creates with
// mode 0700, the session bus and the sound server at sockets under
// home that only a test's own fake serves. In a headless run
// (wltest.HeadlessEnv) the session in the environment is the one the
// recipe booted for the tests, and stays: only the sound server is
// pinned, to its runtime directory.
func Isolate(home string) error {
	env := [][2]string{
		{"HOME", home},
		{"XDG_CONFIG_HOME", filepath.Join(home, "config")},
		{"XDG_DATA_HOME", filepath.Join(home, "data")},
		{"XDG_STATE_HOME", filepath.Join(home, "state")},
		{"XDG_CACHE_HOME", filepath.Join(home, "cache")},
		{isolatedEnv, home},
	}
	runtime := os.Getenv(runtimeEnv)
	if os.Getenv(wltest.HeadlessEnv) == "" {
		for _, key := range sessionEnv {
			if err := os.Unsetenv(key); err != nil {
				return fmt.Errorf("unset %s: %w", key, err)
			}
		}
		runtime = filepath.Join(home, "run")
		if err := os.MkdirAll(runtime, 0o700); err != nil {
			return fmt.Errorf("create the runtime dir: %w", err)
		}
		env = append(env,
			[2]string{runtimeEnv, runtime},
			[2]string{sessionBusEnv, "unix:path=" + dbus.EscapeBusAddressValue(filepath.Join(runtime, "bus"))},
		)
	}
	env = append(env, [2]string{pulseEnv, "unix:" + filepath.Join(runtime, "pulse", "native")})
	for _, kv := range env {
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			return fmt.Errorf("set %s: %w", kv[0], err)
		}
	}
	return nil
}

// RequireNoHostSession fails t when the test process could reach the
// session of the user running it: when the runtime directory, the
// session bus or the sound server it would dial is unset, leaving a
// client to guess, or lies with the Wayland display in the host's
// runtime directory, and, outside a headless run, when a display is
// named or a session bus, a compositor or a sound server answers at
// all. Every package whose tests build applications or dial a bus
// calls it from a test of its own, under a TestMain that calls Run.
func RequireNoHostSession(t testing.TB) {
	t.Helper()
	host := filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	reachable := false
	for _, key := range []string{runtimeEnv, sessionBusEnv, pulseEnv} {
		if os.Getenv(key) == "" {
			reachable = true
			t.Errorf("%s is unset: a client guesses the host's (%s)", key, host)
		}
	}
	for _, key := range []string{runtimeEnv, sessionBusEnv, pulseEnv, displayEnv} {
		if v := os.Getenv(key); v == host || strings.Contains(v, host+"/") {
			reachable = true
			t.Errorf("%s=%q is in the host's runtime directory %s", key, v, host)
		}
	}
	if reachable || os.Getenv(wltest.HeadlessEnv) != "" {
		return
	}
	for _, key := range []string{displayEnv, "WAYLAND_SOCKET", "DISPLAY"} {
		if v, set := os.LookupEnv(key); set {
			t.Errorf("%s=%q names a display the tests did not start", key, v)
		}
	}
	if conn, err := dbus.ConnectSessionBus(); err == nil {
		_ = conn.Close()
		t.Errorf("a session bus answers at %s", os.Getenv(sessionBusEnv))
	}
	if display, err := wl.Connect(""); err == nil {
		_ = display.Context().Close()
		t.Errorf("a compositor answers at %s in %s", cmp.Or(os.Getenv(displayEnv), "wayland-0"), os.Getenv(runtimeEnv))
	}
	ctx, cancel := context.WithTimeout(t.Context(), probeTimeout)
	defer cancel()
	if conn, err := native.Dial(ctx, native.Options{ClientName: "lys-ui testhome", Timeout: probeTimeout}); err == nil {
		_ = conn.Close()
		t.Errorf("a sound server answers at %s", os.Getenv(pulseEnv))
	}
}
