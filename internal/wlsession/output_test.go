package wlsession

import (
	"testing"

	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// TestOutputTransform tracks the geometry event's transform, which
// rotated-output surfaces need for wl_surface.set_buffer_transform.
func TestOutputTransform(t *testing.T) {
	s := &Session{globals: make(map[string]bool), ifaceNames: make(map[uint32]string)}
	out := &Output{Scale: 1}
	e := &outputEvents{sess: s, out: out}

	e.HandleOutputGeometry(wl.OutputGeometryEvent{Transform: 2})
	if out.Transform != 2 {
		t.Errorf("transform = %d, want 2 from the geometry event", out.Transform)
	}
	e.HandleOutputGeometry(wl.OutputGeometryEvent{Transform: 0})
	if out.Transform != 0 {
		t.Errorf("transform = %d, want 0 after the output unrotated", out.Transform)
	}
}

// TestOutputScaleKeepsIntegerFactor pins the integer wl_output.scale
// bookkeeping that predates the fractional protocol: zero factors are
// dropped, real ones recorded.
func TestOutputScaleKeepsIntegerFactor(t *testing.T) {
	s := &Session{globals: make(map[string]bool), ifaceNames: make(map[uint32]string)}
	out := &Output{Scale: 1}
	e := &outputEvents{sess: s, out: out}

	e.HandleOutputScale(wl.OutputScaleEvent{Factor: 2})
	if out.Scale != 2 {
		t.Errorf("scale = %d, want 2", out.Scale)
	}
	e.HandleOutputScale(wl.OutputScaleEvent{Factor: 0})
	if out.Scale != 2 {
		t.Errorf("scale = %d after a zero factor, want 2 kept", out.Scale)
	}
}

// TestOutputHotplug pins the registry-level output bookkeeping: added
// outputs land in Outputs() in arrival order and a removed global
// drops the matching output and fires the removal hook.
func TestOutputHotplug(t *testing.T) {
	s := &Session{
		globals:         make(map[string]bool),
		ifaceNames:      make(map[uint32]string),
		surfaceHandlers: make(map[*wl.Surface]SurfacePointerHandler),
	}

	var added, removed []*Output
	s.OnOutputAdded = func(o *Output) { added = append(added, o) }
	s.OnOutputRemoved = func(o *Output) { removed = append(removed, o) }

	s.trackOutput(&Output{Scale: 1, name: 40})
	s.trackOutput(&Output{Scale: 1, name: 41})
	// Mirror the registry bookkeeping HandleRegistryGlobal performs.
	s.ifaceNames[40] = "wl_output"
	s.ifaceNames[41] = "wl_output"
	if len(s.Outputs()) != 2 || len(added) != 2 {
		t.Fatalf("outputs = %d, added = %d, want 2/2", len(s.Outputs()), len(added))
	}
	if s.Outputs()[0].name != 40 || s.Outputs()[1].name != 41 {
		t.Errorf("output registry names = %d,%d, want 40,41", s.Outputs()[0].name, s.Outputs()[1].name)
	}

	s.HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent{Name: 40})
	if len(s.Outputs()) != 1 || len(removed) != 1 {
		t.Fatalf("after removal: outputs = %d, removed = %d, want 1/1", len(s.Outputs()), len(removed))
	}
	if removed[0].name != 40 {
		t.Errorf("removed output name = %d, want 40", removed[0].name)
	}
	if _, ok := s.ifaceNames[40]; ok {
		t.Error("removed output's registry name still tracked")
	}

	// Removing an unknown name is a no-op.
	s.HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent{Name: 99})
	if len(s.Outputs()) != 1 {
		t.Errorf("unknown removal changed the output set: %d", len(s.Outputs()))
	}
}
