package wlsession

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// fakeIdleInhibitor records one inhibitor's requests.
type fakeIdleInhibitor struct{ destroyed int }

func (f *fakeIdleInhibitor) Destroy() error { f.destroyed++; return nil }

// fakeIdleInhibit records the manager's create requests.
type fakeIdleInhibit struct {
	surfaces []*wl.Surface
	created  []*fakeIdleInhibitor
	err      error
}

func (f *fakeIdleInhibit) CreateInhibitor(surface *wl.Surface) (idleInhibitorAPI, error) {
	if f.err != nil {
		return nil, f.err
	}
	i := &fakeIdleInhibitor{}
	f.surfaces = append(f.surfaces, surface)
	f.created = append(f.created, i)
	return i, nil
}

// TestIdleInhibitIsOptional pins the bind-or-skip contract: the
// manager global must never gate Connect, and a session without it
// reports the gap explicitly instead of panicking.
func TestIdleInhibitIsOptional(t *testing.T) {
	for _, g := range requiredGlobals {
		if g == "zwp_idle_inhibit_manager_v1" {
			t.Errorf("%q is required; idle inhibit must stay feature-detected", g)
		}
	}
	s := &Session{}
	if s.IdleInhibitAvailable() {
		t.Error("IdleInhibitAvailable = true without the manager global")
	}
	inh, err := s.InhibitIdle(&wl.Surface{})
	if !errors.Is(err, ErrIdleInhibitUnavailable) {
		t.Errorf("InhibitIdle without the protocol = %v, want ErrIdleInhibitUnavailable", err)
	}
	if inh != nil {
		t.Error("InhibitIdle without the protocol must return a nil inhibitor")
	}
	inh.Destroy() // nil-safe no-op
}

// TestIdleInhibitCreateDestroy drives a create/destroy round trip
// against a fake manager: the surface reaches the wire, and Destroy is
// idempotent.
func TestIdleInhibitCreateDestroy(t *testing.T) {
	s := &Session{}
	fake := &fakeIdleInhibit{}
	s.idleInhibitMgr = fake

	surf := &wl.Surface{}
	inh, err := s.InhibitIdle(surf)
	if err != nil {
		t.Fatalf("InhibitIdle: %v", err)
	}
	if len(fake.surfaces) != 1 || fake.surfaces[0] != surf {
		t.Errorf("create calls = %v, want the inhibiting surface", fake.surfaces)
	}
	if fake.created[0].destroyed != 0 {
		t.Error("a fresh inhibitor must not be destroyed")
	}

	inh.Destroy()
	if fake.created[0].destroyed != 1 {
		t.Errorf("destroy requests = %d, want 1", fake.created[0].destroyed)
	}
	inh.Destroy() // idempotent
	if fake.created[0].destroyed != 1 {
		t.Errorf("double destroy = %d wire requests, want 1", fake.created[0].destroyed)
	}

	// A nil surface is rejected without touching the wire.
	if _, err := s.InhibitIdle(nil); err == nil {
		t.Error("nil surface must be rejected")
	}
	if len(fake.surfaces) != 1 {
		t.Error("rejected request reached the wire")
	}

	// A wire-level failure surfaces as an error, not a half inhibitor.
	fake.err = errors.New("create failed")
	if _, err := s.InhibitIdle(surf); err == nil {
		t.Error("wire failure must surface")
	}
}
