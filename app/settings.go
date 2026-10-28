package app

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sync"

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
// Reads snapshot from any goroutine; Set mutates, persists, and
// notifies from any goroutine (notification crosses onto the loop).
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
}

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
// loop; settings outlive it harmlessly (later Sets stop notifying).
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
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, &s.raw); err != nil {
		// Corrupt file: the documented fallback is defaults per key;
		// the next Set rewrites the file clean, dropping the damage.
		debug.Log("config", "settings %s: corrupt file, using defaults: %v", s.path, err)
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

// Set stores v for the key, persists the file atomically (temp file +
// rename, same directory), and notifies the subscribers on the loop
// goroutine. An equal value is a full no-op - nothing written, nobody
// notified - which is what makes settings-to-widget bridges echo-free.
func (k Key[T]) Set(s *Settings, v T) {
	s.mu.Lock()
	old := k.load(s)
	if reflect.DeepEqual(old, v) {
		s.mu.Unlock()
		return
	}
	s.values[k.Name] = v
	s.persist()
	st := s.streams[k.Name]
	s.mu.Unlock()
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

// persist writes every value (preserving unknown keys from the loaded
// file) atomically; the caller holds s.mu. Map keys marshal sorted,
// so the file is stable across writes.
func (s *Settings) persist() {
	merged := make(map[string]json.RawMessage, len(s.raw)+len(s.values))
	maps.Copy(merged, s.raw)
	for name, v := range s.values {
		encoded, err := json.Marshal(v)
		if err != nil {
			debug.Log("config", "settings %s: key %s does not encode: %v", s.path, name, err)
			continue
		}
		merged[name] = encoded
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		debug.Log("config", "settings %s: encode: %v", s.path, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		debug.Log("config", "settings %s: mkdir: %v", s.path, err)
		return
	}
	if err := atomicfile.WriteFile(s.path, data, 0o600); err != nil {
		debug.Log("config", "settings %s: %v", s.path, err)
	}
}
