package portalsettings

import (
	"errors"
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

// readSetting reads one portal key with ReadOne, mapping the value with
// mapFn: a key the portal refuses or an unreadable value maps to zero.
// ok is false when the portal gave no answer at all, which says nothing
// about the preference.
func readSetting[T any](conn *dbus.Conn, namespace, key string, mapFn func(any) T, zero T) (value T, ok bool) {
	body, err := call(conn.Object(portalName, portalPath), readTimeout,
		readOne, namespace, key)
	switch {
	case err != nil && !portalAnswered(err):
		return zero, false
	case err != nil || len(body) != 1:
		return zero, true
	}
	return mapFn(body[0]), true
}

// portalAnswered reports whether err is the portal's own reply rather
// than the bus or the connection failing to reach a portal ready to read.
func portalAnswered(err error) bool {
	var reply dbus.Error
	if !errors.As(err, &reply) {
		return false
	}
	switch reply.Name {
	case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner",
		"org.freedesktop.DBus.Error.NoReply", "org.freedesktop.DBus.Error.Timeout", "org.freedesktop.DBus.Error.Disconnected",
		"org.freedesktop.DBus.Error.UnknownObject", "org.freedesktop.DBus.Error.UnknownInterface", "org.freedesktop.DBus.Error.UnknownMethod":
		return false
	}
	return true
}

// refreshSetting moves one setting to what the portal says, leaving it
// alone when the portal gave no answer.
func refreshSetting[T comparable](m *Monitor, s *setting[T], conn *dbus.Conn, namespace, key string, mapFn func(any) T, zero T, deliver bool) {
	if v, ok := readSetting(conn, namespace, key, mapFn, zero); ok {
		s.update(m, v, deliver)
	}
}

// portalAppeared reports whether sig says the portal's name has just
// gained an owner.
func portalAppeared(sig *dbus.Signal) bool {
	if sig == nil || sig.Name != nameOwnerChanged || len(sig.Body) != 3 {
		return false
	}
	name, _ := sig.Body[0].(string)
	owner, _ := sig.Body[2].(string)
	return name == portalName && owner != ""
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

// portalOwned is the bounded name-ownership probe every refresh starts
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
