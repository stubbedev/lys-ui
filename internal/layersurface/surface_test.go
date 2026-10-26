package layersurface

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/wlr"
)

func TestValidateAutoAxisNeedsBothEdgesAnchored(t *testing.T) {
	t.Run("auto width with a single horizontal anchor is rejected", func(t *testing.T) {
		c := Config{Anchor: AnchorTop}
		if err := c.validate(); err == nil {
			t.Error("width 0 with only top anchored must be rejected")
		}
	})

	t.Run("auto height with a single vertical anchor is rejected", func(t *testing.T) {
		c := Config{Anchor: AnchorLeft | AnchorRight}
		if err := c.validate(); err == nil {
			t.Error("height 0 with only left and right anchored must be rejected")
		}
	})

	t.Run("auto width spanning left and right passes", func(t *testing.T) {
		c := Config{Anchor: AnchorTop | AnchorLeft | AnchorRight, Height: 32}
		if err := c.validate(); err != nil {
			t.Errorf("spanning bar config must pass: %v", err)
		}
	})

	t.Run("auto both axes centered passes", func(t *testing.T) {
		c := Config{Anchor: AnchorTop | AnchorBottom | AnchorLeft | AnchorRight}
		if err := c.validate(); err != nil {
			t.Errorf("fully anchored auto-size config must pass: %v", err)
		}
	})

	t.Run("fixed size with a single anchor passes", func(t *testing.T) {
		c := Config{Anchor: AnchorTop, Width: 100, Height: 32}
		if err := c.validate(); err != nil {
			t.Errorf("fixed-size config must pass: %v", err)
		}
	})
}

func TestConfigureStateMachine(t *testing.T) {
	t.Run("configure records size and marks configured", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		if !s.configured {
			t.Error("configure must set the configured flag")
		}
		w, h := s.Size()
		if w != 800 || h != 32 {
			t.Errorf("size = %dx%d, want 800x32", w, h)
		}
	})

	t.Run("zero axes keep the previous size", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		s.applyConfigure(0, 0)
		w, h := s.Size()
		if w != 800 || h != 32 {
			t.Errorf("zero configure clobbered size: %dx%d", w, h)
		}
	})

	t.Run("a second configure resizes", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		s.applyConfigure(1024, 32)
		w, _ := s.Size()
		if w != 1024 {
			t.Errorf("width = %d, want 1024 after resize configure", w)
		}
	})
}

func TestEnsureUsableGate(t *testing.T) {
	t.Run("fresh surface is not usable", func(t *testing.T) {
		if err := (&Surface{}).EnsureUsable(); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("want ErrNotConfigured, got %v", err)
		}
	})

	t.Run("configured surface is usable", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		if err := s.EnsureUsable(); err != nil {
			t.Errorf("configured surface must be usable, got %v", err)
		}
	})

	t.Run("closed surface is not usable, even after configure", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		s.HandleZwlrLayerSurfaceV1Closed(wlr.ZwlrLayerSurfaceV1ClosedEvent{})
		if err := s.EnsureUsable(); !errors.Is(err, ErrClosed) {
			t.Errorf("want ErrClosed, got %v", err)
		}
	})
}

func TestProtocolEnumWireValues(t *testing.T) {
	t.Run("layers", func(t *testing.T) {
		want := map[Layer]uint32{
			LayerBackground: wlr.ZwlrLayerShellV1LayerBackground,
			LayerBottom:     wlr.ZwlrLayerShellV1LayerBottom,
			LayerTop:        wlr.ZwlrLayerShellV1LayerTop,
			LayerOverlay:    wlr.ZwlrLayerShellV1LayerOverlay,
		}
		for l, wire := range want {
			if uint32(l) != wire {
				t.Errorf("%d != wire value %d", l, wire)
			}
		}
	})

	t.Run("anchors", func(t *testing.T) {
		if uint32(AnchorTop) != wlr.ZwlrLayerSurfaceV1AnchorTop ||
			uint32(AnchorBottom) != wlr.ZwlrLayerSurfaceV1AnchorBottom ||
			uint32(AnchorLeft) != wlr.ZwlrLayerSurfaceV1AnchorLeft ||
			uint32(AnchorRight) != wlr.ZwlrLayerSurfaceV1AnchorRight {
			t.Error("anchor bits diverge from the protocol constants")
		}
	})

	t.Run("keyboard modes", func(t *testing.T) {
		want := map[KeyboardMode]uint32{
			KeyboardNone:      wlr.ZwlrLayerSurfaceV1KeyboardInteractivityNone,
			KeyboardExclusive: wlr.ZwlrLayerSurfaceV1KeyboardInteractivityExclusive,
			KeyboardOnDemand:  wlr.ZwlrLayerSurfaceV1KeyboardInteractivityOnDemand,
		}
		for m, wire := range want {
			if uint32(m) != wire {
				t.Errorf("%d != wire value %d", m, wire)
			}
		}
	})
}

func TestSetSizeKeepsTheAutoAxisRule(t *testing.T) {
	t.Run("an auto axis without both edges is refused before the wire", func(t *testing.T) {
		// Layer is nil: reaching the wire would panic, so a refusal here
		// proves the rule is enforced locally.
		s := &Surface{cfg: Config{Anchor: AnchorTop, Width: 600, Height: 400}}
		if err := s.SetSize(600, 0); err == nil {
			t.Fatal("height 0 with only top anchored must be refused")
		}
		if s.cfg.Height != 400 {
			t.Errorf("a refused size changed the recorded config: %+v", s.cfg)
		}
	})

	t.Run("a closed surface refuses a new size", func(t *testing.T) {
		s := &Surface{cfg: Config{Width: 600, Height: 400}, closed: true}
		if err := s.SetSize(600, 300); !errors.Is(err, ErrClosed) {
			t.Errorf("SetSize on a closed surface = %v, want ErrClosed", err)
		}
	})
}

// A live surface records each configure and acks its serial; once the
// client destroyed it, a configure still in flight is neither applied
// nor acked (the ack would name a dead object).
func TestConfigureAfterDestroyIsNotAcked(t *testing.T) {
	var acked []uint32
	ack := func(serial uint32) error { acked = append(acked, serial); return nil }

	s := &Surface{}
	s.configure(7, 800, 32, ack)
	if len(acked) != 1 || acked[0] != 7 || !s.configured {
		t.Fatalf("live configure: acked %v configured %v, want serial 7 acked", acked, s.configured)
	}

	s.destroyed = true
	s.configure(8, 1024, 40, ack)
	if len(acked) != 1 {
		t.Fatalf("destroyed surface acked %v; serial 8 must be dropped", acked)
	}
	if w, h := s.Size(); w != 800 || h != 32 {
		t.Errorf("destroyed surface resized to %dx%d", w, h)
	}
}

// The runtime setters keep the creation rules: an anchor that leaves
// an auto axis half-anchored, a layer change on a v1 shell, and
// on-demand keyboard below v4 are all refused before the wire (Layer
// is nil: reaching it would panic), leaving the config as it was.
func TestRuntimeSettersKeepTheRules(t *testing.T) {
	s := &Surface{cfg: Config{Anchor: AnchorLeft | AnchorRight | AnchorTop, Height: 30}}
	if err := s.SetAnchor(AnchorTop); err == nil {
		t.Error("dropping a horizontal anchor under an auto width was accepted")
	}
	if err := s.SetLayer(LayerOverlay); !errors.Is(err, ErrShellTooOld) {
		t.Errorf("set_layer on v1 = %v, want ErrShellTooOld", err)
	}
	if err := s.SetKeyboardMode(KeyboardOnDemand); !errors.Is(err, ErrShellTooOld) {
		t.Errorf("on-demand on v1 = %v, want ErrShellTooOld", err)
	}
	if s.cfg.Anchor != AnchorLeft|AnchorRight|AnchorTop || s.cfg.Layer != 0 || s.cfg.Keyboard != KeyboardNone {
		t.Errorf("a refused change touched the config: %+v", s.cfg)
	}
	if err := (Config{Width: 1, Height: 1, Keyboard: KeyboardOnDemand, shellVersion: 4}).validate(); err != nil {
		t.Errorf("on-demand on v4: %v", err)
	}
	closed := &Surface{cfg: Config{Width: 10, Height: 10}, closed: true}
	for name, err := range map[string]error{
		"margin": closed.SetMargin(Margins{Top: 4}),
		"zone":   closed.SetExclusiveZone(20),
	} {
		if !errors.Is(err, ErrClosed) {
			t.Errorf("%s on a closed surface = %v, want ErrClosed", name, err)
		}
	}
}

// Reshape validates the anchor and size as one final state: dropping an
// anchor under an automatic axis is refused before the wire unless the
// same change makes that axis concrete (Layer is nil: reaching the wire
// would panic, so only refusals run here).
func TestReshapeValidatesTheFinalState(t *testing.T) {
	s := &Surface{cfg: Config{Anchor: AnchorLeft | AnchorRight | AnchorTop, Height: 30}}
	if err := s.Reshape(AnchorTop, 0, 30); err == nil {
		t.Error("an automatic width on a top-only anchor was accepted")
	}
	if s.cfg.Anchor != AnchorLeft|AnchorRight|AnchorTop || s.cfg.Width != 0 {
		t.Errorf("a refused reshape touched the config: %+v", s.cfg)
	}
	closed := &Surface{cfg: Config{Width: 10, Height: 10}, closed: true}
	if err := closed.Reshape(AnchorTop, 10, 10); !errors.Is(err, ErrClosed) {
		t.Errorf("reshape on a closed surface = %v, want ErrClosed", err)
	}
}
