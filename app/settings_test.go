package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/waittest"
	"github.com/stubbedev/lys-ui/widget"
)

// settingsEnv points the settings file at a scratch config dir and
// returns the app directory's parent.
func settingsEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func testSettings(t *testing.T, a *Application, appID string) *Settings {
	t.Helper()
	s := NewSettings(a, appID)
	t.Cleanup(s.stop)
	return s
}

func settingsFlush(t *testing.T, s *Settings) {
	t.Helper()
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
}

var (
	setGreeting = Key[string]{Name: "greeting", Default: "hello"}
	setVolume   = Key[float64]{Name: "volume", Default: 0.5}
	setDark     = Key[bool]{Name: "dark-mode", Default: false}
)

// TestSettingsDefaultsBeforeAnyFile pins the first-run behavior: every
// key reads its default and nothing is written until a Set happens.
func TestSettingsDefaultsBeforeAnyFile(t *testing.T) {
	cfg := settingsEnv(t)
	s := testSettings(t, testApp(nil), "dev.stubbe.test.settings")
	if got := setGreeting.Get(s); got != "hello" {
		t.Errorf("greeting = %q, want the default", got)
	}
	if got := setVolume.Get(s); got != 0.5 {
		t.Errorf("volume = %v, want the default", got)
	}
	if _, err := os.Stat(filepath.Join(cfg, "dev.stubbe.test.settings")); !os.IsNotExist(err) {
		t.Error("merely reading settings created the file")
	}
}

// TestSettingsSetPersistsAndReloads is the persistence round trip: Set
// writes the JSON file, a fresh Settings over the same file serves the
// stored values, and unknown keys in the file survive rewrites.
func TestSettingsSetPersistsAndReloads(t *testing.T) {
	cfg := settingsEnv(t)
	s := testSettings(t, testApp(nil), "dev.stubbe.test.settings")
	setVolume.Set(s, 0.75)
	setDark.Set(s, true)
	settingsFlush(t, s)

	path := filepath.Join(cfg, "dev.stubbe.test.settings", "settings.json")
	reloaded := testSettings(t, testApp(nil), "dev.stubbe.test.settings")
	if got := setVolume.Get(reloaded); got != 0.75 {
		t.Errorf("reloaded volume = %v, want 0.75", got)
	}
	if got := setDark.Get(reloaded); !got {
		t.Errorf("reloaded dark-mode = %v, want true", got)
	}
	if got := setGreeting.Get(reloaded); got != "hello" {
		t.Errorf("reloaded greeting = %q, want the default", got)
	}

	// A key the schema does not know rides along untouched.
	if err := os.WriteFile(path,
		[]byte(`{"future-key": [1, 2], "volume": 0.75, "dark-mode": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s = testSettings(t, testApp(nil), "dev.stubbe.test.settings")
	setGreeting.Set(s, "hi")
	settingsFlush(t, s)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]json.RawMessage
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("rewritten settings do not parse: %v", err)
	}
	var future []int
	if err := json.Unmarshal(written["future-key"], &future); err != nil || len(future) != 2 || future[0] != 1 || future[1] != 2 {
		t.Errorf("rewrite dropped or damaged the unknown key: %s", written["future-key"])
	}
}

// TestSettingsCorruptionFallsBackToDefaults pins the documented
// fallback: a corrupt file serves defaults, and the next Set replaces
// the file with a clean one.
func TestSettingsCorruptionFallsBackToDefaults(t *testing.T) {
	dir := settingsEnv(t)
	appID := "dev.stubbe.test.corrupt"
	if err := os.MkdirAll(filepath.Join(dir, appID), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, appID, "settings.json")
	if err := os.WriteFile(path, []byte(`{"greeting": "tru`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := testSettings(t, testApp(nil), appID)
	if got := setGreeting.Get(s); got != "hello" {
		t.Errorf("corrupt file yielded %q, want the default", got)
	}
	setVolume.Set(s, 0.25)
	settingsFlush(t, s)
	reloaded := testSettings(t, testApp(nil), appID)
	if got := setVolume.Get(reloaded); got != 0.25 {
		t.Errorf("post-repair volume = %v, want 0.25", got)
	}
}

// TestSettingsTypeDriftFallsBackToDefault covers a file whose value
// for a key no longer matches the schema's type: the default wins and
// the next write repairs the entry.
func TestSettingsTypeDriftFallsBackToDefault(t *testing.T) {
	dir := settingsEnv(t)
	appID := "dev.stubbe.test.drift"
	if err := os.MkdirAll(filepath.Join(dir, appID), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, appID, "settings.json")
	if err := os.WriteFile(path, []byte(`{"volume": "loud"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := testSettings(t, testApp(nil), appID)
	if got := setVolume.Get(s); got != 0.5 {
		t.Errorf("type-drifted volume = %v, want the default", got)
	}
	setVolume.Set(s, 1)
	settingsFlush(t, s)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "loud") {
		t.Errorf("write kept the drifted value:\n%s", data)
	}
}

// TestSettingsNotifiesOnLoopAndSuppressesEqual pins the notification
// contract: changes reach subscribers on the loop goroutine, an equal
// Set notifies nobody and rewrites nothing.
func TestSettingsNotifiesOnLoopAndSuppressesEqual(t *testing.T) {
	settingsEnv(t)
	a := testApp(nil)
	s := testSettings(t, a, "dev.stubbe.test.notify")
	seen := 0
	cancel := setVolume.Subscribe(s, func(v float64) { seen++ })
	defer cancel()

	setVolume.Set(s, 0.9)
	setVolume.Set(s, 0.9)
	a.pump(time.Now())
	if seen != 1 {
		t.Errorf("two Sets (one equal) notified %d times, want 1", seen)
	}
}

// TestSettingsBindingBridgesToWidgets is the #80 Bind acceptance: a
// key's Binding drives a widget both ways, the write persists, and
// setting the key moves the widget with no echo loop.
func TestSettingsBindingBridgesToWidgets(t *testing.T) {
	settingsEnv(t)
	a := testApp(nil)
	s := testSettings(t, a, "dev.stubbe.test.bind")
	b := setDark.Binding(s)

	sw := widget.NewSwitch(false)
	unbind := sw.BindOn(b)
	defer unbind()

	setDark.Set(s, true)
	a.pump(time.Now())
	if !sw.On() {
		t.Error("the switch never followed the persisted key")
	}
	sw.SetOn(false)
	a.pump(time.Now())
	if setDark.Get(s) {
		t.Error("a switch flip never persisted")
	}
}

func callerGoroutine() string {
	buf := make([]byte, 64)
	return strings.Fields(string(buf[:runtime.Stack(buf, false)]))[1]
}

type settingsSeams struct {
	mu      sync.Mutex
	armed   []func()
	writes  []string
	onCall  int
	loop    string
	blocked chan struct{}
}

func newSettingsSeams(s *Settings) *settingsSeams {
	seams := &settingsSeams{loop: callerGoroutine()}
	s.arm = func(fn func()) func() bool {
		seams.mu.Lock()
		defer seams.mu.Unlock()
		seams.armed = append(seams.armed, fn)
		return func() bool { return true }
	}
	s.write = func(_ string, data []byte) error {
		seams.mu.Lock()
		seams.writes = append(seams.writes, string(data))
		if callerGoroutine() == seams.loop {
			seams.onCall++
		}
		blocked := seams.blocked
		seams.mu.Unlock()
		if blocked != nil {
			<-blocked
		}
		return nil
	}
	return seams
}

func TestSettingsBurstWritesOnceOffTheCaller(t *testing.T) {
	settingsEnv(t)
	s := testSettings(t, testApp(nil), "dev.stubbe.test.burst")
	seams := newSettingsSeams(s)
	for i := range 100 {
		setVolume.Set(s, float64(i)/100)
	}
	if len(seams.writes) != 0 {
		t.Fatalf("%d writes during the burst, want none", len(seams.writes))
	}
	if len(seams.armed) != 1 {
		t.Fatalf("%d writes scheduled, want one", len(seams.armed))
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		seams.armed[0]()
	}()
	<-done
	if len(seams.writes) != 1 || seams.onCall != 0 {
		t.Fatalf("%d writes, %d on the caller; want one, none", len(seams.writes), seams.onCall)
	}
	if !strings.Contains(seams.writes[0], "0.99") {
		t.Errorf("the write missed the last value:\n%s", seams.writes[0])
	}
}

func TestSettingsStuckWriteBlocksNoSet(t *testing.T) {
	settingsEnv(t)
	s := testSettings(t, testApp(nil), "dev.stubbe.test.stuck")
	seams := newSettingsSeams(s)
	seams.blocked = make(chan struct{})
	setVolume.Set(s, 0.1)
	flushed := make(chan error, 1)
	go func() { flushed <- s.Flush() }()
	waittest.Until(t, func() bool {
		seams.mu.Lock()
		defer seams.mu.Unlock()
		return len(seams.writes) == 1
	}, "the first write to start")
	setVolume.Set(s, 0.2)
	if got := setVolume.Get(s); got != 0.2 {
		t.Errorf("volume = %v during the stuck write, want 0.2", got)
	}
	close(seams.blocked)
	if err := <-flushed; err != nil {
		t.Fatal(err)
	}
	settingsFlush(t, s)
	if len(seams.writes) != 2 || !strings.Contains(seams.writes[1], "0.2") {
		t.Errorf("writes = %q, want the later value written after the stuck one", seams.writes)
	}
}

func TestSettingsFlushAtRunExit(t *testing.T) {
	cfg := settingsEnv(t)
	a := testApp(nil)
	s := testSettings(t, a, "dev.stubbe.test.exit")
	s.arm = func(func()) func() bool { return func() bool { return true } }
	setVolume.Set(s, 0.33)
	path := filepath.Join(cfg, "dev.stubbe.test.exit", "settings.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the file exists before the exit: %v", err)
	}
	a.watchers.shutdown()
	if got := setVolume.Get(testSettings(t, testApp(nil), "dev.stubbe.test.exit")); got != 0.33 {
		t.Errorf("after the exit volume = %v, want 0.33", got)
	}
	setVolume.Set(s, 0.44)
	if got := setVolume.Get(testSettings(t, testApp(nil), "dev.stubbe.test.exit")); got != 0.44 {
		t.Errorf("a Set after the exit left volume = %v, want 0.44", got)
	}
}

func TestSettingsFailedWriteStaysOwed(t *testing.T) {
	settingsEnv(t)
	s := testSettings(t, testApp(nil), "dev.stubbe.test.fail")
	seams := newSettingsSeams(s)
	failing := true
	write := s.write
	s.write = func(path string, data []byte) error {
		if failing {
			return errors.New("disk full")
		}
		return write(path, data)
	}
	setVolume.Set(s, 0.7)
	if err := s.Flush(); err == nil || !strings.Contains(err.Error(), "disk full") || !strings.Contains(err.Error(), "settings.json") {
		t.Fatalf("Flush = %v, want the failure with the path", err)
	}
	failing = false
	settingsFlush(t, s)
	if len(seams.writes) != 1 {
		t.Errorf("%d writes after the retry, want one", len(seams.writes))
	}
}
