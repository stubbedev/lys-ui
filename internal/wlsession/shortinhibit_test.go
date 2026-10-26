package wlsession

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// fakeShortcutsInhibitor records one inhibitor's requests and keeps
// the active/inactive listeners the session registered.
type fakeShortcutsInhibitor struct {
	destroyed int
	activeH   wlr.ZwpShortcutsInhibitorV1ActiveHandler
	inactiveH wlr.ZwpShortcutsInhibitorV1InactiveHandler
}

func (f *fakeShortcutsInhibitor) Destroy() error { f.destroyed++; return nil }

func (f *fakeShortcutsInhibitor) AddActiveHandler(h wlr.ZwpShortcutsInhibitorV1ActiveHandler) {
	f.activeH = h
}

func (f *fakeShortcutsInhibitor) AddInactiveHandler(h wlr.ZwpShortcutsInhibitorV1InactiveHandler) {
	f.inactiveH = h
}

// fakeShortcutsInhibit records the manager's create requests.
type fakeShortcutsInhibit struct {
	surfaces []*wl.Surface
	seats    []*wl.Seat
	created  []*fakeShortcutsInhibitor
	err      error
}

func (f *fakeShortcutsInhibit) InhibitShortcuts(surface *wl.Surface, seat *wl.Seat) (shortcutsInhibitorAPI, error) {
	if f.err != nil {
		return nil, f.err
	}
	i := &fakeShortcutsInhibitor{}
	f.surfaces = append(f.surfaces, surface)
	f.seats = append(f.seats, seat)
	f.created = append(f.created, i)
	return i, nil
}

// TestShortcutsInhibitIsOptional pins the bind-or-skip contract: the
// manager global must never gate Connect, and a session without it
// reports the gap explicitly instead of panicking.
func TestShortcutsInhibitIsOptional(t *testing.T) {
	for _, g := range requiredGlobals {
		if g == "zwp_keyboard_shortcuts_inhibit_manager_v1" {
			t.Errorf("%q is required; shortcuts inhibit must stay feature-detected", g)
		}
	}
	s := &Session{}
	if s.ShortcutsInhibitAvailable() {
		t.Error("ShortcutsInhibitAvailable = true without the manager global")
	}
	inh, err := s.InhibitShortcuts(&wl.Surface{}, &wl.Seat{})
	if !errors.Is(err, ErrShortcutsInhibitUnavailable) {
		t.Errorf("InhibitShortcuts without the protocol = %v, want ErrShortcutsInhibitUnavailable", err)
	}
	if inh != nil {
		t.Error("InhibitShortcuts without the protocol must return a nil inhibitor")
	}
	inh.Destroy() // nil-safe no-op
	if inh.Active() {
		t.Error("nil inhibitor must not report active")
	}
}

// TestShortcutsInhibitRoundTrip drives create, the active/inactive
// focus events, and destroy against fakes.
func TestShortcutsInhibitRoundTrip(t *testing.T) {
	s := &Session{}
	fake := &fakeShortcutsInhibit{}
	s.shortcutsInhibitMgr = fake

	surf, seat := &wl.Surface{}, &wl.Seat{}
	inh, err := s.InhibitShortcuts(surf, seat)
	if err != nil {
		t.Fatalf("InhibitShortcuts: %v", err)
	}
	if len(fake.surfaces) != 1 || fake.surfaces[0] != surf || fake.seats[0] != seat {
		t.Errorf("create calls = %v/%v, want the surface and seat", fake.surfaces, fake.seats)
	}
	if fake.created[0].activeH == nil || fake.created[0].inactiveH == nil {
		t.Fatal("inhibitor focus events left unwired")
	}
	if inh.Active() {
		t.Error("a fresh inhibitor must start inactive")
	}

	// Focus enters the pane: the compositor activates the inhibitor.
	fake.created[0].activeH.HandleZwpShortcutsInhibitorV1Active(wlr.ZwpShortcutsInhibitorV1ActiveEvent{})
	if !inh.Active() {
		t.Error("active event must flip the inhibitor on")
	}
	fake.created[0].inactiveH.HandleZwpShortcutsInhibitorV1Inactive(wlr.ZwpShortcutsInhibitorV1InactiveEvent{})
	if inh.Active() {
		t.Error("inactive event must flip the inhibitor off")
	}

	inh.Destroy()
	if fake.created[0].destroyed != 1 {
		t.Errorf("destroy requests = %d, want 1", fake.created[0].destroyed)
	}
	inh.Destroy() // idempotent
	if fake.created[0].destroyed != 1 {
		t.Errorf("double destroy = %d wire requests, want 1", fake.created[0].destroyed)
	}

	// Missing seat or surface is rejected without touching the wire.
	if _, err := s.InhibitShortcuts(nil, seat); err == nil {
		t.Error("nil surface must be rejected")
	}
	if _, err := s.InhibitShortcuts(surf, nil); err == nil {
		t.Error("nil seat must be rejected")
	}
	if len(fake.surfaces) != 1 {
		t.Error("rejected requests reached the wire")
	}

	// A wire-level failure surfaces as an error.
	fake.err = errors.New("create failed")
	if _, err := s.InhibitShortcuts(surf, seat); err == nil {
		t.Error("wire failure must surface")
	}
}
