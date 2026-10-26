package scale

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/wlr"
)

// fakeSurface records the wl_surface scale requests.
type fakeSurface struct {
	scale     int32
	transform int32
	err       error
}

func (f *fakeSurface) SetBufferScale(s int32) error {
	f.scale = s
	return f.err
}

func (f *fakeSurface) SetBufferTransform(t int32) error {
	f.transform = t
	return f.err
}

// fakeViewport records the wp_viewport destination requests.
type fakeViewport struct {
	destW, destH int32
	err          error
}

func (f *fakeViewport) SetDestination(w, h int32) error {
	f.destW, f.destH = w, h
	return f.err
}

func TestDeviceSizeRoundsUp(t *testing.T) {
	tests := []struct {
		name    string
		v       int
		frac120 uint32
		want    int
	}{
		{"1x keeps the size", 100, 120, 100},
		{"2x doubles", 100, 240, 200},
		{"1.25 rounds a partial up", 33, 150, 42}, // 41.25 -> 42
		{"1.25 exact", 40, 150, 50},
		{"1.5 rounds up", 101, 180, 152}, // 151.5 -> 152
		{"zero scale is 1x", 100, 0, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeviceSize(tc.v, tc.frac120); got != tc.want {
				t.Errorf("DeviceSize(%d, %d) = %d, want %d", tc.v, tc.frac120, got, tc.want)
			}
		})
	}
}

func TestIntegerScaleRoundsUp(t *testing.T) {
	if got := IntegerScale(120); got != 1 {
		t.Errorf("IntegerScale(120) = %d, want 1", got)
	}
	if got := IntegerScale(150); got != 2 {
		t.Errorf("IntegerScale(150) = %d, want 2 (1.25 needs a 2x integer fallback)", got)
	}
	if got := IntegerScale(0); got != 1 {
		t.Errorf("IntegerScale(0) = %d, want 1", got)
	}
}

func TestApplyIntegerFallback(t *testing.T) {
	// No viewport: the controller publishes the rounded-up integer
	// buffer scale on wl_surface, exactly the pre-fractional behavior.
	surf := &fakeSurface{}
	c := &Controller{surf: surf}

	if c.Fractional() {
		t.Error("a controller without protocol objects must report integer mode")
	}
	if err := c.Apply(150, 100, 50); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if surf.scale != 2 {
		t.Errorf("set_buffer_scale = %d, want 2", surf.scale)
	}
	// A surface without a size yet must not touch the wire: the first
	// configure has not arrived.
	surf.scale = 0
	if err := c.Apply(150, 0, 0); err != nil {
		t.Fatalf("apply before configure: %v", err)
	}
	if surf.scale != 0 {
		t.Errorf("set_buffer_scale = %d before configure, want untouched", surf.scale)
	}
}

func TestApplyFractionalSetsDestination(t *testing.T) {
	// With a viewport the destination is the logical size and the
	// buffer scale stays 1, per the fractional-scale protocol.
	surf := &fakeSurface{}
	vp := &fakeViewport{}
	c := &Controller{surf: surf, viewport: vp}

	if !c.Fractional() {
		t.Error("a controller with a viewport must report fractional mode")
	}
	if err := c.Apply(150, 100, 50); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if vp.destW != 100 || vp.destH != 50 {
		t.Errorf("destination = %dx%d, want 100x50 (logical)", vp.destW, vp.destH)
	}
	if surf.scale != 0 {
		t.Errorf("set_buffer_scale = %d, want untouched (must stay 1 in fractional mode)", surf.scale)
	}
}

func TestApplyPropagatesWireErrors(t *testing.T) {
	boom := errors.New("boom")
	surf := &fakeSurface{err: boom}
	c := &Controller{surf: surf}
	if err := c.Apply(240, 10, 10); !errors.Is(err, boom) {
		t.Errorf("apply error = %v, want the wire error", err)
	}
	vp := &fakeViewport{err: boom}
	c2 := &Controller{surf: &fakeSurface{}, viewport: vp}
	if err := c2.Apply(240, 10, 10); !errors.Is(err, boom) {
		t.Errorf("apply error = %v, want the wire error", err)
	}
}

func TestSetTransform(t *testing.T) {
	surf := &fakeSurface{}
	c := &Controller{surf: surf}
	if err := c.SetTransform(3); err != nil {
		t.Fatalf("set transform: %v", err)
	}
	if surf.transform != 3 {
		t.Errorf("set_buffer_transform = %d, want 3", surf.transform)
	}
}

func TestPreferredScaleHandler(t *testing.T) {
	var got []uint32
	c := &Controller{onPreferred: func(f uint32) { got = append(got, f) }}

	c.HandleWpScaleV1PreferredScale(wlr.WpScaleV1PreferredScaleEvent{Scale: 150})
	c.HandleWpScaleV1PreferredScale(wlr.WpScaleV1PreferredScaleEvent{Scale: 0})
	if len(got) != 1 || got[0] != 150 {
		t.Errorf("preferred scales = %v, want [150] (zero events dropped)", got)
	}

	// A nil handler must not panic (popups bind without one).
	var none Controller
	none.HandleWpScaleV1PreferredScale(wlr.WpScaleV1PreferredScaleEvent{Scale: 240})
}
