package wlsession

import (
	"errors"
	"testing"

	"github.com/stubbedev/lys-lib/wlr"
)

// Without the protocols every surface hint reports unavailable rather
// than failing on the wire.
func TestSurfaceHintsUnavailable(t *testing.T) {
	s := newRoutingSession()
	if err := s.SetContentType(nil, ContentPhoto); !errors.Is(err, ErrProtocolUnavailable) {
		t.Errorf("content type: %v", err)
	}
	if err := s.SetSurfaceAlpha(nil, 0.5); !errors.Is(err, ErrProtocolUnavailable) {
		t.Errorf("alpha: %v", err)
	}
	if err := s.Bell(nil); !errors.Is(err, ErrProtocolUnavailable) {
		t.Errorf("bell: %v", err)
	}
	if s.BlurAvailable() {
		t.Error("blur available without ext_background_effect_v1")
	}
	if err := s.SetBlurRegion(nil, nil); !errors.Is(err, ErrProtocolUnavailable) {
		t.Errorf("blur region: %v", err)
	}
	// Bound but without the blur capability offered, blur stays off;
	// the capabilities event turns it on and off.
	caps := blurCapabilities{s}
	s.blurMgr = new(wlr.ExtBackgroundEffectManagerV1)
	if s.BlurAvailable() {
		t.Error("blur available before the capabilities event")
	}
	caps.HandleExtBackgroundEffectManagerV1Capabilities(wlr.ExtBackgroundEffectManagerV1CapabilitiesEvent{Flags: wlr.ExtBackgroundEffectManagerV1CapabilityBlur})
	if !s.BlurAvailable() {
		t.Error("blur unavailable after the compositor offered it")
	}
	caps.HandleExtBackgroundEffectManagerV1Capabilities(wlr.ExtBackgroundEffectManagerV1CapabilitiesEvent{})
	if s.BlurAvailable() {
		t.Error("blur still available after the compositor withdrew it")
	}
	s.blurMgr = nil
	s.releaseSurfaceHints(nil) // nothing bound: a no-op
}
