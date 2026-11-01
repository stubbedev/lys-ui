package wlsession

import "testing"

func TestMissingGlobals(t *testing.T) {
	t.Run("nothing present lists every required global", func(t *testing.T) {
		got := missingGlobals(map[string]bool{})
		if len(got) != len(requiredGlobals) {
			t.Errorf("want %d missing globals, got %v", len(requiredGlobals), got)
		}
	})

	t.Run("everything present lists nothing", func(t *testing.T) {
		have := make(map[string]bool)
		for _, g := range requiredGlobals {
			have[g] = true
		}
		if got := missingGlobals(have); len(got) != 0 {
			t.Errorf("want no missing globals, got %v", got)
		}
	})

	t.Run("only the absent ones are listed", func(t *testing.T) {
		have := map[string]bool{"wl_compositor": true}
		got := missingGlobals(have)
		if len(got) != 1 || got[0] != "wl_shm" {
			t.Errorf("want [wl_shm], got %v", got)
		}
	})

	t.Run("a compositor with neither layer shell nor outputs lists nothing", func(t *testing.T) {
		have := map[string]bool{"wl_compositor": true, "wl_shm": true, "xdg_wm_base": true}
		if got := missingGlobals(have); len(got) != 0 {
			t.Errorf("want no missing globals, got %v", got)
		}
	})

	t.Run("surfaces and buffers stay required", func(t *testing.T) {
		have := map[string]bool{"wl_output": true, "xdg_wm_base": true, "zwlr_layer_shell_v1": true}
		got := missingGlobals(have)
		if len(got) != 2 || got[0] != "wl_compositor" || got[1] != "wl_shm" {
			t.Errorf("want [wl_compositor wl_shm], got %v", got)
		}
	})
}

func TestBindVersion(t *testing.T) {
	if got := bindVersion(6, 4); got != 4 {
		t.Errorf("bindVersion(6, 4) = %d, want 4", got)
	}
	if got := bindVersion(2, 4); got != 2 {
		t.Errorf("bindVersion(2, 4) = %d, want 2 (must not bind above advertised)", got)
	}
}

func TestFractionalScalingIsOptional(t *testing.T) {
	// The fractional-scale protocols must never gate Connect: a
	// compositor providing every required global but neither
	// wp_viewporter nor wp_fractional_scale_manager_v1 is fully usable
	// and keeps the integer behavior.
	have := make(map[string]bool)
	for _, g := range requiredGlobals {
		have[g] = true
	}
	for _, optional := range []string{"wp_viewporter", "wp_fractional_scale_manager_v1"} {
		if missing := missingGlobals(have); len(missing) != 0 {
			t.Fatalf("without %s: missing = %v, want nothing", optional, missing)
		}
		have[optional] = true
	}
	if missing := missingGlobals(have); len(missing) != 0 {
		t.Errorf("with both protocols: missing = %v, want nothing", missing)
	}
}

func TestFractionalAccessorsNilWithoutBinding(t *testing.T) {
	// A session that never saw the globals exposes no protocol objects,
	// which is what drives the integer fallback in internal/scale.
	s := &Session{globals: make(map[string]bool), ifaceNames: make(map[uint32]string)}
	if s.Viewporter() != nil {
		t.Error("Viewporter() must be nil before wp_viewporter is bound")
	}
	if s.FractionalScaleManager() != nil {
		t.Error("FractionalScaleManager() must be nil before the manager is bound")
	}
}
