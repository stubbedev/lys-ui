package portalsettings

import (
	"slices"
	"sync"

	dbus "github.com/stubbedev/lys-lib/third_party/godbus"
)

// The per-setting machinery every tracked portal key shares:
// one current value, one listener list, one update path. The monitor's
// single mutex covers all settings; update runs the listeners outside
// it, in registration order, skipping tombstoned slots.

// setting is one tracked portal setting's state.
type setting[T comparable] struct {
	current   T
	listeners []func(T)
}

// get returns the current value; the caller routes through the
// monitor's lock.
func (s *setting[T]) get(m *Monitor) T {
	m.mu.Lock()
	defer m.mu.Unlock()
	return s.current
}

// on registers fn and returns its unregister function; the slot, not
// the closure, is the identity (function values are not comparable).
func (s *setting[T]) on(m *Monitor, fn func(T)) (off func()) {
	if fn == nil {
		return func() {}
	}
	m.mu.Lock()
	s.listeners = append(s.listeners, fn)
	i := len(s.listeners) - 1
	m.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			if i < len(s.listeners) {
				s.listeners[i] = nil
			}
		})
	}
}

// update stores v and, when deliver is set and the value actually
// moved, runs the listeners - on the monitor goroutine, serialized,
// without holding the lock (a listener may read or register back).
// Duplicate values are dropped: the portal can re-announce a setting
// nobody changed.
func (s *setting[T]) update(m *Monitor, v T, deliver bool) {
	m.mu.Lock()
	changed := v != s.current
	s.current = v
	var fns []func(T)
	if deliver && changed && !m.closed {
		fns = slices.Clone(s.listeners)
	}
	m.mu.Unlock()
	for _, fn := range fns {
		if fn != nil {
			fn(v)
		}
	}
}

// readSetting reads one portal key once through the shared probe and
// ReadOne, mapping the value with mapFn; any failure - no portal, no
// key, unreadable value - maps to zero.
func readSetting[T any](conn *dbus.Conn, namespace, key string, mapFn func(any) T, zero T) T {
	if !portalOwned(conn) {
		return zero
	}
	body, err := call(conn.Object(portalName, portalPath), readTimeout,
		readOne, namespace, key)
	if err != nil || len(body) != 1 {
		return zero
	}
	return mapFn(body[0])
}

// settingChanged reports whether sig is the SettingChanged for one
// namespace/key pair, and the mapped new value. Signals for other
// settings and namespaces are not ours.
func settingChanged[T any](sig *dbus.Signal, namespace, key string, mapFn func(any) T, zero T) (T, bool) {
	if sig == nil || sig.Name != changedSig || sig.Path != portalPath {
		return zero, false
	}
	if len(sig.Body) != 3 {
		return zero, false
	}
	ns, _ := sig.Body[0].(string)
	k, _ := sig.Body[1].(string)
	if ns != namespace || k != key {
		return zero, false
	}
	return mapFn(sig.Body[2]), true
}

// portalOwned is the bounded name-ownership probe every read starts
// with: no portal means Unknown-shaped zeros, deliberately without
// on-demand activation (a startup that may sit 25s waiting for dbus
// activation is worse than Unknown).
func portalOwned(conn *dbus.Conn) bool {
	body, err := call(conn.BusObject(), probeTimeout,
		"org.freedesktop.DBus.NameHasOwner", portalName)
	if err != nil || len(body) != 1 {
		return false
	}
	owned, _ := body[0].(bool)
	return owned
}
