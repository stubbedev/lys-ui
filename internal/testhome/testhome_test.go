package testhome

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/dbustest"
	"github.com/stubbedev/lys-lib/pulse/pulsetest"
	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
	"github.com/stubbedev/lys-lib/wltest"
)

func TestMain(m *testing.M) { Run(m) }

var isolatedKeys = []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", runtimeEnv, sessionBusEnv, pulseEnv, isolatedEnv}

func restoreAfter(t *testing.T) {
	t.Helper()
	for _, k := range isolatedKeys {
		t.Setenv(k, os.Getenv(k))
	}
}

type verdict struct {
	testing.TB
	errs []string
}

func (v *verdict) Helper() {}

func (v *verdict) Errorf(format string, args ...any) {
	v.errs = append(v.errs, fmt.Sprintf(format, args...))
}

func guard(t *testing.T) []string {
	t.Helper()
	t.Setenv(wltest.HeadlessEnv, "")
	return verdictOf(t)
}

func verdictOf(t *testing.T) []string {
	t.Helper()
	v := &verdict{TB: t}
	RequireNoHostSession(v)
	return v.errs
}

func wantOnly(t *testing.T, errs []string, about string) {
	t.Helper()
	if len(errs) != 1 || !strings.Contains(errs[0], about) {
		t.Errorf("the guard said %q, want one failure about %q", errs, about)
	}
}

func TestTheIsolatedTestBinaryReachesNoHostSession(t *testing.T) {
	RequireNoHostSession(t)
	if errs := guard(t); len(errs) != 0 {
		t.Errorf("outside a headless run the guard says %q", errs)
	}
}

func TestIsolatePointsEveryUserDirUnderHome(t *testing.T) {
	restoreAfter(t)
	t.Setenv(wltest.HeadlessEnv, "")
	for _, k := range isolatedKeys {
		t.Setenv(k, "/real/"+k)
	}
	home := t.TempDir()
	if err := Isolate(home); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"HOME": home, "XDG_CONFIG_HOME": home + "/config", "XDG_DATA_HOME": home + "/data",
		"XDG_STATE_HOME": home + "/state", "XDG_CACHE_HOME": home + "/cache", runtimeEnv: home + "/run",
		sessionBusEnv: "unix:path=" + dbus.EscapeBusAddressValue(home+"/run/bus"),
		pulseEnv:      "unix:" + home + "/run/pulse/native", isolatedEnv: home,
	}
	for _, k := range isolatedKeys {
		if got := os.Getenv(k); got != want[k] {
			t.Errorf("%s = %q, want %q", k, got, want[k])
		}
	}
	if info, err := os.Stat(filepath.Join(home, "run")); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("runtime dir = %v, %v; want a 0700 directory", info, err)
	}
	if errs := guard(t); len(errs) != 0 {
		t.Errorf("the guard fails an isolated process: %q", errs)
	}
}

func TestIsolateFailsWhereTheRuntimeDirCannotBeMade(t *testing.T) {
	restoreAfter(t)
	t.Setenv(wltest.HeadlessEnv, "")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Isolate(file); err == nil {
		t.Error("a home that is a file isolated without error")
	}
}

func TestIsolateUnsetsEverySessionVariable(t *testing.T) {
	restoreAfter(t)
	t.Setenv(wltest.HeadlessEnv, "")
	for _, k := range sessionEnv {
		t.Setenv(k, "host-"+k)
	}
	if err := Isolate(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, k := range sessionEnv {
		if v, set := os.LookupEnv(k); set {
			t.Errorf("%s = %q survived Isolate", k, v)
		}
	}
}

func TestIsolateKeepsTheSessionAHeadlessRecipeBooted(t *testing.T) {
	restoreAfter(t)
	recipe := t.TempDir()
	t.Setenv(wltest.HeadlessEnv, "1")
	t.Setenv(runtimeEnv, recipe)
	t.Setenv(sessionBusEnv, "unix:path="+recipe+"/bus")
	for _, k := range sessionEnv {
		t.Setenv(k, "recipe-"+k)
	}
	home := t.TempDir()
	if err := Isolate(home); err != nil {
		t.Fatal(err)
	}
	for _, k := range sessionEnv {
		if got := os.Getenv(k); got != "recipe-"+k {
			t.Errorf("%s = %q, want the recipe's %q", k, got, "recipe-"+k)
		}
	}
	for k, want := range map[string]string{
		runtimeEnv: recipe, sessionBusEnv: "unix:path=" + recipe + "/bus", pulseEnv: "unix:" + recipe + "/pulse/native",
		"HOME": home, "XDG_CONFIG_HOME": home + "/config",
	} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if errs := verdictOf(t); len(errs) != 0 {
		t.Errorf("the guard fails a headless run on the recipe's session: %q", errs)
	}
}

func TestTheGuardFailsASessionBusThatAnswers(t *testing.T) {
	dbustest.Session(t)
	wantOnly(t, guard(t), "a session bus answers")
}

func TestTheGuardFailsACompositorThatAnswers(t *testing.T) {
	ln, _ := wltest.LoopbackDisplay(t, "wayland-host")
	defer func() { _ = ln.Close() }()
	errs := guard(t)
	if len(errs) != 2 || !strings.Contains(errs[0], "names a display") || !strings.Contains(errs[1], "a compositor answers at wayland-host") {
		t.Errorf("the guard said %q, want the display named and answering", errs)
	}
}

func TestTheGuardFailsASoundServerThatAnswers(t *testing.T) {
	t.Setenv(pulseEnv, pulsetest.New(t).Addr())
	wantOnly(t, guard(t), "a sound server answers")
}

func TestTheGuardFailsAnEndpointLeftToGuess(t *testing.T) {
	for _, k := range []string{runtimeEnv, sessionBusEnv, pulseEnv} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, "")
			wantOnly(t, guard(t), k+" is unset")
		})
	}
}

func TestTheGuardFailsAnEndpointInTheHostsRuntimeDir(t *testing.T) {
	host := filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	for _, c := range [][2]string{
		{runtimeEnv, host},
		{sessionBusEnv, "unix:path=" + host + "/bus"},
		{pulseEnv, "unix:" + host + "/pulse/native"},
		{displayEnv, host + "/wayland-1"},
	} {
		t.Run(c[0], func(t *testing.T) {
			t.Setenv(c[0], c[1])
			wantOnly(t, guard(t), c[0]+"="+strconv.Quote(c[1])+" is in the host's runtime directory")
		})
	}
}

func TestTheGuardFailsAHeadlessRunOnTheHostsSession(t *testing.T) {
	host := filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	t.Setenv(wltest.HeadlessEnv, "1")
	t.Setenv(runtimeEnv, host)
	wantOnly(t, verdictOf(t), "is in the host's runtime directory")
}
