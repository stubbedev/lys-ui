package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// TestSessionLockIsOptional pins the bind-or-skip contract: the lock
// manager never gates Connect, and a session without it reports nil.
func TestSessionLockIsOptional(t *testing.T) {
	for _, g := range requiredGlobals {
		if g == "ext_session_lock_manager_v1" {
			t.Errorf("%q is required; session lock must stay feature-detected", g)
		}
	}
	if (&Session{}).SessionLockManager() != nil {
		t.Error("SessionLockManager is non-nil without the global")
	}
}

// TestOutputWatchers pins the hotplug fan-out: watchers see adds and
// removals after the application hooks, a stopped watcher sees
// nothing more, stop is idempotent, and a watcher stopping itself
// mid-notify does not skip the next one.
func TestOutputWatchers(t *testing.T) {
	s := &Session{
		globals:    make(map[string]bool),
		ifaceNames: make(map[uint32]string),
	}
	var order []string
	s.OnOutputAdded = func(*Output) { order = append(order, "hook+") }
	s.OnOutputRemoved = func(*Output) { order = append(order, "hook-") }

	var aAdded, aRemoved, bAdded []*Output
	var stopA func()
	stopA = s.WatchOutputs(func(o *Output) {
		order = append(order, "a+")
		aAdded = append(aAdded, o)
	}, func(o *Output) {
		order = append(order, "a-")
		aRemoved = append(aRemoved, o)
		stopA() // unsubscribing mid-notify must not skip b
	})
	stopB := s.WatchOutputs(func(o *Output) {
		order = append(order, "b+")
		bAdded = append(bAdded, o)
	}, nil) // a nil half is allowed

	out := &Output{Scale: 1, name: 7}
	s.trackOutput(out)
	s.ifaceNames[7] = "wl_output"
	if len(aAdded) != 1 || aAdded[0] != out || len(bAdded) != 1 {
		t.Fatalf("add fan-out: a=%v b=%v", aAdded, bAdded)
	}

	s.HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent{Name: 7})
	if len(aRemoved) != 1 || aRemoved[0] != out {
		t.Fatalf("remove fan-out: a=%v", aRemoved)
	}
	want := []string{"hook+", "a+", "b+", "hook-", "a-"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}

	// a stopped itself; b stops now. Nobody hears the next output.
	stopB()
	stopB() // idempotent
	stopA()
	s.trackOutput(&Output{Scale: 1, name: 8})
	if len(aAdded) != 1 || len(bAdded) != 1 {
		t.Errorf("stopped watchers still notified: a=%d b=%d", len(aAdded), len(bAdded))
	}
}
