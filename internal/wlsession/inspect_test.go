package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/third_party/wayland/wlcursor"
	"github.com/stubbedev/lys-lib/wlr"
)

// TestGlobalsRecordsVersions drives the registry handler with a fake
// global burst (interface names with no bind side effects): Globals
// must report every interface with the version it was advertised
// with, and drop it again on removal.
func TestGlobalsRecordsVersions(t *testing.T) {
	s := &Session{
		globals:        map[string]bool{},
		ifaceNames:     map[uint32]string{},
		globalVersions: map[string]uint32{},
	}
	for _, ev := range []wl.RegistryGlobalEvent{
		{Name: 1, Interface: "test_a", Version: 3},
		{Name: 2, Interface: "test_b", Version: 6},
		{Name: 4, Interface: "test_c", Version: 4},
	} {
		s.HandleRegistryGlobal(ev)
	}
	got := s.Globals()
	for iface, want := range map[string]uint32{"test_a": 3, "test_b": 6, "test_c": 4} {
		if got[iface] != want {
			t.Errorf("globals[%s] = %d, want %d", iface, got[iface], want)
		}
	}
	// The returned map is a copy: mutating it must not touch the session.
	got["test_a"] = 1
	if s.Globals()["test_a"] != 3 {
		t.Error("Globals returned a live map, not a copy")
	}

	s.HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent{Name: 4})
	if _, ok := s.Globals()["test_c"]; ok {
		t.Error("removed global still reported")
	}
}

// TestInspectSnapshot pins the doctor-facing snapshot on a fake
// session: globals with versions, the live outputs, the scale
// protocols, and the resolved cursor theme.
func TestInspectSnapshot(t *testing.T) {
	s := &Session{
		globals:        map[string]bool{},
		ifaceNames:     map[uint32]string{},
		globalVersions: map[string]uint32{},
		viewporter:     &wlr.WpViewporter{},
	}
	s.HandleRegistryGlobal(wl.RegistryGlobalEvent{Name: 2, Interface: "test_a", Version: 6})
	s.trackOutput(&Output{Name: "DP-1", Scale: 2, ModeW: 2560, ModeH: 1440})

	oldThm, oldErr := cursorThm, cursorErr
	cursorThm, cursorErr = nil, nil
	t.Cleanup(func() { cursorThm, cursorErr = oldThm, oldErr })

	info := s.Inspect()
	if info.Globals["test_a"] != 6 {
		t.Errorf("inspect globals[test_a] = %d, want 6", info.Globals["test_a"])
	}
	if len(info.Outputs) != 1 || info.Outputs[0].Name != "DP-1" || info.Outputs[0].Scale != 2 {
		t.Errorf("inspect outputs = %+v", info.Outputs)
	}
	if info.FractionalScale {
		t.Error("fractional scale reported without both protocols bound")
	}
	s.fracScaleManager = &wlr.WpScaleManagerV1{}
	if !s.Inspect().FractionalScale {
		t.Error("both protocols bound but fractional scale not reported")
	}

	// Without a shm global the theme cannot load: the snapshot must
	// say why rather than report an empty theme as resolved.
	if info.CursorErr == nil || info.CursorTheme != "" {
		t.Errorf("cursor theme = %q, err = %v; want an explanatory error", info.CursorTheme, info.CursorErr)
	}

	name, size, err := s.CursorTheme()
	if err == nil || name != "" {
		t.Errorf("CursorTheme without shm = (%q, %d, %v), want an error", name, size, err)
	}
}

func TestCursorThemeReportsLoadedTheme(t *testing.T) {
	s := &Session{}
	oldThm, oldErr := cursorThm, cursorErr
	cursorThm = &wlcursor.Theme{Name: "Sentinel", Size: 24}
	cursorErr = nil
	t.Cleanup(func() { cursorThm, cursorErr = oldThm, oldErr })

	name, size, err := s.CursorTheme()
	if err != nil || name != "Sentinel" || size != 24 {
		t.Errorf("CursorTheme = (%q, %d, %v), want (Sentinel, 24, nil)", name, size, err)
	}
}
