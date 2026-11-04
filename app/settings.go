package app

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/atomicfile"
	"github.com/stubbedev/lys-lib/debug"

	"github.com/stubbedev/lys-ui/widget"
)

// Settings is the persisted settings bag, GSettings/relm4's Settings
// chapter (#80): schema-declared typed keys with defaults, a JSON file
// under the user config directory, atomic writes, and change
// notifications delivered on the loop goroutine through the same
// Stream shape every other messenger uses. The file is
// <UserConfigDir>/<AppID>/settings.json; a missing file, a corrupt
// file, or a value whose type drifted all fall back to the key's
// default, and the next write replaces the file with a clean one.
//
// Reads snapshot from any goroutine; Set mutates memory and notifies
// from any goroutine (notification crosses onto the loop). The file
// follows off the caller's goroutine, a burst of Sets coalesced into
// one write, and Run's exit flushes whatever is still unwritten; Flush
// does it on demand.
//
// A key has one type for the life of the application - two Key values
// with the same Name and different types are a programming error and
// fail loudly at the type assert.
type Settings struct {
	app     *Application
	mu      sync.Mutex
	path    string
	raw     map[string]json.RawMessage
	values  map[string]any
	streams map[string]any
	dirty   bool
	stopped bool
	cancel  func() bool
	arm     func(fn func()) (cancel func() bool)
	write   func(path string, data []byte) error
	ioMu    sync.Mutex
}

const settingsWriteDelay = 250 * time.Millisecond

// Key declares one typed settings entry: a name in the persisted file
// and the default used before anything was ever written. Keys are
// values, declared once next to the code that owns them:
//
//	var darkMode = app.Key[bool]{Name: "dark-mode", Default: false}
type Key[T any] struct {
	Name    string
	Default T
}

// NewSettings loads the settings file for appID, falling back to
// defaults per key. The application wires the notifications onto its
// loop; settings outlive it harmlessly (later Sets stop notifying and
// write at once, there being no loop left to stall or exit to flush).
func NewSettings(a *Application, appID string) *Settings {
	dir, err := os.UserConfigDir()
	if err != nil {
		debug.Log("config", "settings: no user config dir: %v", err)
		dir = os.TempDir()
	}
	s := &Settings{
		app:     a,
		path:    filepath.Join(dir, appID, "settings.json"),
		raw:     map[string]json.RawMessage{},
		values:  map[string]any{},
		streams: map[string]any{},
		arm: func(fn func()) func() bool {
			return time.AfterFunc(settingsWriteDelay, fn).Stop
		},
		write: writeSettingsFile,
	}
	a.OnStop(s.stop)
	data, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s.raw); err != nil {
		// Corrupt file: the documented fallback is defaults per key;
		// the next Set rewrites the file clean, dropping the damage.
		debug.Log("config", "settings %s: corrupt file, using defaults: %v", s.path, err)
		s.raw = nil
	}
	if s.raw == nil {
		s.raw = map[string]json.RawMessage{}
	}
	return s
}

// Get returns the key's current value: the cached Set value, else the
// file's, else the default. Safe from any goroutine.
func (k Key[T]) Get(s *Settings) T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return k.load(s)
}

// Set stores v for the key, schedules the file's atomic rewrite, and
// notifies the subscribers on the loop goroutine. An equal value is a
// full no-op - nothing written, nobody
// notified - which is what makes settings-to-widget bridges echo-free.
func (k Key[T]) Set(s *Settings, v T) {
	s.mu.Lock()
	old := k.load(s)
	if reflect.DeepEqual(old, v) {
		s.mu.Unlock()
		return
	}
	s.values[k.Name] = v
	s.encode(k.Name, v)
	s.dirty = true
	writeNow := s.stopped
	if !writeNow && s.cancel == nil {
		s.cancel = s.arm(s.flushLogged)
	}
	st := s.streams[k.Name]
	s.mu.Unlock()
	if writeNow {
		s.flushLogged()
	}
	if typed, ok := st.(*Stream[T]); ok {
		typed.Send(v)
	}
}

// Subscribe registers fn for every change of the key, delivered on
// the loop goroutine with event-callback guarantees. The returned
// cancel removes the subscription.
func (k Key[T]) Subscribe(s *Settings, fn func(T)) (cancel func()) {
	s.mu.Lock()
	st := k.stream(s)
	s.mu.Unlock()
	return st.Subscribe(fn)
}

// Binding returns the key as a loop-owned Binding wired two-way:
// widget edits Set and persist the key, key changes update every
// widget bound to it, and the equal-value no-ops on both sides keep
// the wiring echo-free. Call it once per key per application and bind
// widgets to the returned Binding:
//
//	entry.BindText(nameKey.Binding(settings))
func (k Key[T]) Binding(s *Settings) *widget.Binding[T] {
	b := widget.NewBinding(k.Get(s))
	k.Subscribe(s, b.Set)
	b.Subscribe(func(v T) { k.Set(s, v) })
	return b
}

// load resolves the key's value, caching it; the caller holds s.mu.
func (k Key[T]) load(s *Settings) T {
	if v, ok := s.values[k.Name]; ok {
		typed, ok := v.(T)
		if !ok {
			panic("app: settings key " + k.Name + " was already read with a different type")
		}
		return typed
	}
	v := k.Default
	if raw, ok := s.raw[k.Name]; ok {
		var decoded T
		if err := json.Unmarshal(raw, &decoded); err != nil {
			debug.Log("config", "settings %s: key %s fell back to its default: %v", s.path, k.Name, err)
		} else {
			v = decoded
		}
	}
	s.values[k.Name] = v
	s.encode(k.Name, v)
	return v
}

// stream lazily creates the key's typed change stream; the caller
// holds s.mu.
func (k Key[T]) stream(s *Settings) *Stream[T] {
	if st, ok := s.streams[k.Name]; ok {
		if typed, ok := st.(*Stream[T]); ok {
			return typed
		}
		panic("app: settings key " + k.Name + " was already subscribed with a different type")
	}
	st := NewStream[T](s.app)
	s.streams[k.Name] = st
	return st
}

// encode records v as the key's file form; the caller holds s.mu. A
// value that does not encode keeps the entry it had.
func (s *Settings) encode(name string, v any) {
	encoded, err := json.Marshal(v)
	if err != nil {
		debug.Log("config", "settings %s: key %s does not encode: %v", s.path, name, err)
		return
	}
	s.raw[name] = encoded
}

// Flush writes unwritten changes now, on the caller's goroutine, and
// returns once they are on disk. A failed write stays owed: the next
// Set or Flush tries again.
func (s *Settings) Flush() error {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	s.dirty = false
	snapshot := maps.Clone(s.raw)
	s.mu.Unlock()
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err == nil {
		err = s.write(s.path, data)
	}
	if err != nil {
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return fmt.Errorf("settings %s: %w", s.path, err)
	}
	return nil
}

func (s *Settings) flushLogged() {
	if err := s.Flush(); err != nil {
		debug.Log("config", "%v", err)
	}
}

func (s *Settings) stop() {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.flushLogged()
}

func writeSettingsFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return atomicfile.WriteFile(path, data, 0o600)
}
