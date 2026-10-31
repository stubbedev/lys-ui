package headlesstest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestParseTraces(t *testing.T) {
	log := "  0.512s input: wire button 272 state=1 serial=9\n" +
		" 12.000s demo: button clicked 2 times\n" +
		"lys-ui-hello: mapped at 640x470\n" + // plain log line, no category
		"\n" +
		"panic: dead\n"
	trs := ParseTraces([]byte(log))
	if len(trs) != 4 {
		t.Fatalf("parsed %d traces, want 4: %+v", len(trs), trs)
	}
	if trs[0].Category != "input" || trs[0].Secs != 0.512 ||
		trs[0].Message != "wire button 272 state=1 serial=9" {
		t.Errorf("trace 0 = %+v", trs[0])
	}
	if trs[1].Category != "demo" || trs[1].Message != "button clicked 2 times" {
		t.Errorf("trace 1 = %+v", trs[1])
	}
	if trs[2].Category != "" || trs[2].Message != "lys-ui-hello: mapped at 640x470" {
		t.Errorf("plain line not preserved: %+v", trs[2])
	}
	if trs[3].Message != "panic: dead" {
		t.Errorf("panic line not preserved: %+v", trs[3])
	}
}

func TestLogWatcherWaitsForNewLinesOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Pre-existing content is invisible to a watcher attached now.
	if _, err := f.WriteString("  0.100s demo: button clicked 1 times\n"); err != nil {
		t.Fatal(err)
	}
	w, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}

	write := func(s string) {
		t.Helper()
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}

	// A line the watcher must not see yet (missing category filter).
	write("  0.200s frame: draw 640x470\n")
	// The match, then a decoy for later waits.
	write("  0.300s demo: switch false\n")
	if _, err := f.WriteString("  0.400s demo: switc"); err != nil { // partial line
		t.Fatal(err)
	}

	tr, err := w.Wait("demo", "switch false", 2*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if tr.Secs != 0.300 {
		t.Errorf("matched %+v, want the 0.300s trace", tr)
	}

	// Sequential waits advance: the same trace is not handed out twice.
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("h complete\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", "switch false", 150*time.Millisecond); err == nil {
		t.Error("the consumed trace was delivered a second time")
	}
}

func TestLogWatcherKeepsTracesBeyondTheMatch(t *testing.T) {
	// A burst written in one chunk: the first Wait consumes only its own
	// line, the next Wait must still see the trace right behind it.
	// This actually broke live: mapped and the control centers landed in
	// one poll and every control wait starved.
	path := filepath.Join(t.TempDir(), "client.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}
	burst := "  0.015s demo: mapped 640x470\n  0.016s demo: control button center (116,83)\n  0.016s demo: control slider center (116,169)\n"
	if err := os.WriteFile(path, []byte(burst), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", "mapped 640x470", time.Second); err != nil {
		t.Fatalf("mapped: %v", err)
	}
	if _, err := w.Wait("demo", "control button ", time.Second); err != nil {
		t.Fatalf("control button: %v", err)
	}
	if _, err := w.Wait("demo", "control slider ", time.Second); err != nil {
		t.Fatalf("control slider: %v", err)
	}
}

func TestLogWatcherTimeoutCarriesTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("  1.000s input: wire enter surf=3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = w.Wait("demo", "never happens", 80*time.Millisecond)
	if err == nil {
		t.Fatal("wait on a quiet log returned nil")
	}
	if !strings.Contains(err.Error(), "wire enter") {
		t.Errorf("timeout error lacks the log tail: %v", err)
	}
}

// TestSwayRecipePinsTheShowcase guards the compositor config in the
// justfile against drifting from this package: the recipe must pin the
// showcase (AppID, showcaseW/H live here) to a floating window at the
// output's origin, or every traced control center goes stale and the
// clicks below land on chrome. The states client's floating pin keeps
// the confirmed maximize/fullscreen configure sizes deterministic.
func TestSwayRecipePinsTheShowcase(t *testing.T) {
	root, err := ModuleDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "justfile"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(data)
	for _, want := range []string{
		"output * mode 1280x800",
		`for_window [app_id="` + AppID + `"] floating enable`,
		"move position 0 0",
		fmt.Sprintf("resize set %d %d", showcaseW, showcaseH),
		`for_window [app_id="` + StatesAppID + `"] floating enable`,
		fmt.Sprintf("resize set %d %d", statesW, statesH),
		`for_window [app_id="` + MultilistAppID + `"] floating enable`,
		fmt.Sprintf("resize set %d %d", multilistW, multilistH),
		`for_window [title="modal dialog"] floating enable`,
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("test-env sway config lacks %q; keep it in sync with internal/headlesstest:\n%s", want, cfg)
		}
	}
}

// TestHyprlandVMRecipePinsTheShowcase is the sway recipe guard's twin
// for the Hyprland VM gate: the VM's windowrules must pin the same
// floating sizes and positions (Hyprland 0.56 legacy rule syntax:
// effects and match:class fields, comma-separated), or every traced
// coordinate is compositor-relative to a tiling layout instead. The
// dots in the app_ids are regex-escaped in the conf and the escapes
// are doubled again by the nix ” string, so the guard matches the
// escaped source form.
func TestHyprlandVMRecipePinsTheShowcase(t *testing.T) {
	root, err := ModuleDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "tests", "hyprland-vm.nix"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(data)
	nixClass := func(appID string) string {
		return strings.ReplaceAll(regexp.QuoteMeta(appID), `\`, `\\`)
	}
	for _, want := range []string{
		fmt.Sprintf("match:class ^(%s)$, size %d %d, move 0 0", nixClass(AppID), showcaseW, showcaseH),
		fmt.Sprintf("match:class ^(%s)$, size %d %d, move 40 40", nixClass(StatesAppID), statesW, statesH),
		fmt.Sprintf("match:class ^(%s)$, size %d %d, move 0 0", nixClass(MultilistAppID), multilistW, multilistH),
		"match:title ^modal dialog$",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("Hyprland VM config lacks %q; keep it in sync with internal/headlesstest:\n%s", want, cfg)
		}
	}
}

func TestModuleDirFindsRepoRoot(t *testing.T) {
	root, err := ModuleDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "lys-ui-hello")); err != nil {
		t.Errorf("module root %s has no cmd/lys-ui-hello: %v", root, err)
	}
}

func TestKeyForCoversEverythingTheSuiteTypes(t *testing.T) {
	typed := "abc" + "hi" + " " // TypeText inputs across the suite
	for _, r := range typed {
		code, shift, err := KeyFor(r)
		if err != nil {
			t.Errorf("KeyFor(%q): %v", r, err)
		}
		if shift {
			t.Errorf("KeyFor(%q) asks for shift; the suite types only unshifted runes", r)
		}
		if code == 0 {
			t.Errorf("KeyFor(%q) = 0", r)
		}
	}
	if _, _, err := KeyFor('~'); err == nil {
		t.Error("unknown rune did not error")
	}
}

func TestMaxGapSinceCountsTheSilenceAfterTheLastMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := Watch(path)
	if err != nil {
		t.Fatal(err)
	}
	log := "  0.100s frame: start\n" +
		"  0.200s frame: frame committed, waiting for callback\n" +
		"  0.340s frame: frame committed, waiting for callback\n" +
		"  1.100s input: wire key code=57 state=1\n"
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := w.MaxGapSince(1, "frame", "frame committed"); got < 0.75 || got > 0.77 {
		t.Fatalf("MaxGapSince = %.2fs, want the 0.76s of silence after the last commit", got)
	}
}
