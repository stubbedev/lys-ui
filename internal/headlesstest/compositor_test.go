package headlesstest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stubbedev/lys-lib/wltest"
)

func TestDriverForPicksAndDefaults(t *testing.T) {
	for name, want := range map[string]string{
		"":         "sway",
		"sway":     "sway",
		"hyprland": "hyprland",
	} {
		d, err := driverFor(name)
		if err != nil {
			t.Fatalf("driverFor(%q): %v", name, err)
		}
		if d.Name() != want {
			t.Errorf("driverFor(%q) = %s, want %s", name, d.Name(), want)
		}
	}
	if _, err := driverFor("labwc"); err == nil {
		t.Error("driverFor accepted an unknown compositor")
	}
}

func TestDriverArtifactNamesMatchTheRecipes(t *testing.T) {
	// The recipes (justfile test-env, tests/hyprland-vm.nix) write
	// these exact names into the session dir; the drivers point the
	// kill pattern, the pid file and the diagnostics at them.
	sway, _ := driverFor("sway")
	if sway.ConfigName() != "sway.cfg" || sway.PIDName() != "sway.pid" || sway.LogName() != "sway.log" {
		t.Errorf("sway driver artifacts: %s %s %s", sway.ConfigName(), sway.PIDName(), sway.LogName())
	}
	hyprland, _ := driverFor("hyprland")
	if hyprland.ConfigName() != "hypr.conf" || hyprland.PIDName() != "hyprland.pid" || hyprland.LogName() != "hyprland.log" {
		t.Errorf("hyprland driver artifacts: %s %s %s", hyprland.ConfigName(), hyprland.PIDName(), hyprland.LogName())
	}
}

func TestEnvPathsFollowTheDriver(t *testing.T) {
	dir := t.TempDir()
	e := &Env{Dir: dir, Display: "wayland-1", compositor: hyprlandCompositor{}}
	if got, want := e.LogPath(), filepath.Join(dir, "hyprland.log"); got != want {
		t.Errorf("LogPath = %s, want %s", got, want)
	}
	if got, want := e.PIDPath(), filepath.Join(dir, "hyprland.pid"); got != want {
		t.Errorf("PIDPath = %s, want %s", got, want)
	}
}

func TestHyprInstanceDirPrefersTheEnvAndThenTheOnlyLiveOne(t *testing.T) {
	dir := t.TempDir()
	live := func(inst string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, "hypr", inst), 0o700); err != nil {
			t.Fatal(err)
		}
		// A socket file only needs to exist for discovery; stat alone
		// stays far below the unix socket path length limit.
		if err := os.WriteFile(filepath.Join(dir, "hypr", inst, ".socket.sock"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := hyprInstanceDir(dir); err == nil {
		t.Fatal("hyprInstanceDir guessed without a live instance")
	}
	live("inst-a")
	got, err := hyprInstanceDir(dir)
	if err != nil || filepath.Base(got) != "inst-a" {
		t.Fatalf("hyprInstanceDir = %s, %v; want inst-a", got, err)
	}
	// A signature leaked from another session names nothing in this
	// dir; discovery must see through it, not follow it.
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "leaked-from-the-host-session")
	got, err = hyprInstanceDir(dir)
	if err != nil || filepath.Base(got) != "inst-a" {
		t.Fatalf("hyprInstanceDir with a leaked sig = %s, %v; want inst-a", got, err)
	}
	live("inst-b")
	if _, err := hyprInstanceDir(dir); err == nil {
		t.Fatal("hyprInstanceDir picked one of two live instances")
	}
	// Separator-bearing garbage never resolves, env or not.
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "../elsewhere")
	if _, err := hyprInstanceDir(dir); err == nil {
		t.Fatal("hyprInstanceDir followed a path-walking signature")
	}
	// And the env still wins when it does name a live instance here.
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "inst-b")
	got, err = hyprInstanceDir(dir)
	if err != nil || filepath.Base(got) != "inst-b" {
		t.Fatalf("hyprInstanceDir with env = %s, %v; want inst-b", got, err)
	}
}

func TestHyprControlCommandRoundTripsOneRequest(t *testing.T) {
	// The control socket is a real unix socket; LoopbackDisplay keeps
	// its path inside the kernel's 107-byte sun_path limit on CI.
	ln, sock := wltest.LoopbackDisplay(t, "ctl-sock")
	requests := make(chan string, 1)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)
		n, _ := conn.Read(buf)
		requests <- string(buf[:n])
		_, _ = conn.Write([]byte("ok\n"))
	}()
	reply, err := hyprControlCommand(sock, "dispatch closewindow class:dev.stubbe.lys.ui.states")
	if err != nil {
		t.Fatalf("hyprControlCommand: %v", err)
	}
	if reply != "ok" {
		t.Errorf("reply = %q, want %q", reply, "ok")
	}
	if got := <-requests; got != "dispatch closewindow class:dev.stubbe.lys.ui.states" {
		t.Errorf("request = %q", got)
	}
}

func TestHyprCloseWindowTargetsTheLiveInstance(t *testing.T) {
	dir := t.TempDir()
	instance := filepath.Join(dir, "hypr", "inst")
	if err := os.MkdirAll(instance, 0o700); err != nil {
		t.Fatal(err)
	}
	// A dead socket file is enough for discovery; CloseWindow fails
	// dialing it, which is the error path under test here.
	if err := os.WriteFile(filepath.Join(instance, ".socket.sock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e := &Env{Dir: dir, Display: "wayland-1", compositor: hyprlandCompositor{}}
	err := e.CloseWindow("dev.stubbe.lys.ui.states")
	if err == nil {
		t.Fatal("CloseWindow dialed a dead control socket without error")
	}
	if !strings.Contains(err.Error(), "connect") {
		t.Errorf("CloseWindow error names the wrong failure: %v", err)
	}
}

func TestProcPIDsMatchingFindsOnlyThePattern(t *testing.T) {
	// This test process carries the test binary path in its cmdline -
	// the same shape a daemonized compositor's config path takes.
	mine := procPIDsMatching(os.Args[0])
	found := false
	for _, pid := range mine {
		if pid == os.Getpid() {
			found = true
		}
	}
	if !found {
		t.Fatalf("procPIDsMatching(%q) = %v, missing self %d", os.Args[0], mine, os.Getpid())
	}
	if got := procPIDsMatching(filepath.Join(t.TempDir(), "nothing.cfg")); len(got) != 0 {
		t.Fatalf("procPIDsMatching matched %d processes on a pattern no one carries", len(got))
	}
}
