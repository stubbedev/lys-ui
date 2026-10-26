package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/wlr"
)

func TestPrimarySelectionIsOptional(t *testing.T) {
	// The primary selection manager must stay out of the required
	// globals so compositors without the protocol keep working:
	// today's behavior is the no-protocol path.
	for _, g := range requiredGlobals {
		if g == "zwp_primary_selection_device_manager_v1" {
			t.Errorf("%q is required; primary selection must stay feature-detected", g)
		}
	}
	have := make(map[string]bool)
	for _, g := range requiredGlobals {
		have[g] = true
	}
	if got := missingGlobals(have); len(got) != 0 {
		t.Errorf("missing globals with every required one bound = %v, want none", got)
	}
}

func TestPrimarySelectionWithoutProtocol(t *testing.T) {
	// A session that never saw the manager global has no device, and
	// the accessors stay nil so every primary-selection call upstream
	// is a harmless no-op — exactly the pre-#23 behavior.
	s := &Session{}
	s.ensurePrimarySelectionDevice() // nothing bound: must stay inert
	if s.PrimarySelectionAvailable() {
		t.Error("PrimarySelectionAvailable = true without the manager global")
	}
	if s.PrimarySelectionDevice() != nil {
		t.Error("PrimarySelectionDevice != nil without the manager global")
	}
	if s.PrimarySelectionManager() != nil {
		t.Error("PrimarySelectionManager != nil without the global")
	}
}

func TestPrimarySelectionVersionCap(t *testing.T) {
	// zwp_primary_selection_unstable_v1 stopped at version 1: every
	// request and event is v1, so a higher advertised version binds
	// at 1, and the constant really is 1.
	if maxPrimarySelectionVersion != 1 {
		t.Errorf("maxPrimarySelectionVersion = %d, want 1 (the protocol's only version)", maxPrimarySelectionVersion)
	}
	if got := bindVersion(5, maxPrimarySelectionVersion); got != 1 {
		t.Errorf("bindVersion(5) = %d, want the protocol cap 1", got)
	}
	if got := bindVersion(1, maxPrimarySelectionVersion); got != 1 {
		t.Errorf("bindVersion(1) = %d, want the advertised 1", got)
	}
}

// Compile-time: the device type the session exposes comes from the
// generated binding, keeping the accessor honest about its type.
var _ = (*wlr.ZwpPrimarySelectionDeviceV1)(nil)
