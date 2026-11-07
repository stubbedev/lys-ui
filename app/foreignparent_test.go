package app

import (
	"errors"
	"testing"
)

// A window has at most one parent, and a foreign one only where the
// compositor imports handles.
func TestWindowParentsAreChecked(t *testing.T) {
	owner := &Window{}
	for _, tc := range []struct {
		name      string
		cfg       WindowConfig
		canImport bool
		ok        bool
		is        error
	}{
		{"no parent", WindowConfig{}, false, true, nil},
		{"a local parent", WindowConfig{Parent: owner}, false, true, nil},
		{"a foreign parent", WindowConfig{ForeignParent: "h"}, true, true, nil},
		{"a modal foreign dialog", WindowConfig{ForeignParent: "h", Modal: true}, true, true, nil},
		{"both parents", WindowConfig{Parent: owner, ForeignParent: "h"}, true, false, nil},
		{"a foreign parent without an importer", WindowConfig{ForeignParent: "h"}, false, false, ErrForeignParentUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.parentErr(tc.canImport)
			if (err == nil) != tc.ok {
				t.Fatalf("parentErr = %v, want ok=%v", err, tc.ok)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("parentErr = %v, want %v", err, tc.is)
			}
		})
	}
}
