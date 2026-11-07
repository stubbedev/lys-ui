package wlsession

import (
	"fmt"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// ColorRepresentation is how a surface's YUV samples become RGB, as
// wp_color_representation_v1 names it: matrix coefficients and range.
type ColorRepresentation struct {
	Coefficients uint32
	Range        uint32
}

type colorRepresentations struct{ s *Session }

func (h colorRepresentations) HandleWpColorRepresentationManagerV1SupportedCoefficientsAndRanges(ev wlr.WpColorRepresentationManagerV1SupportedCoefficientsAndRangesEvent) {
	h.s.colorReps[ColorRepresentation{Coefficients: ev.Coefficients, Range: ev.Range}] = true
}

func (s *Session) bindColorRepresentation(ev wl.RegistryGlobalEvent) {
	mgr := wlr.NewWpColorRepresentationManagerV1(s.registry.Context())
	s.colorReps = map[ColorRepresentation]bool{}
	mgr.AddSupportedCoefficientsAndRangesHandler(colorRepresentations{s})
	if !s.bindOptional(ev, 1, mgr) {
		return
	}
	s.colorRepMgr = mgr
	debug.Log("shell", "wp_color_representation_manager_v1 bound")
}

// NewColorRepresentation tags surf's buffers with c from its next
// commit on, through an object the caller destroys with the surface.
// It is ErrProtocolUnavailable without wp_color_representation_v1 and
// an error when the compositor does not support c, where setting it
// would be a protocol error.
func (s *Session) NewColorRepresentation(surf *wl.Surface, c ColorRepresentation) (*wlr.WpColorRepresentationSurfaceV1, error) {
	if s.colorRepMgr == nil {
		return nil, ErrProtocolUnavailable
	}
	if !s.colorReps[c] {
		return nil, fmt.Errorf("wlsession: the compositor represents no coefficients %d in range %d", c.Coefficients, c.Range)
	}
	obj, err := s.colorRepMgr.GetSurface(surf)
	if err != nil {
		return nil, fmt.Errorf("wlsession: color representation get_surface: %w", err)
	}
	if err := obj.SetCoefficientsAndRange(c.Coefficients, c.Range); err != nil {
		_ = obj.Destroy()
		return nil, fmt.Errorf("wlsession: set_coefficients_and_range: %w", err)
	}
	return obj, nil
}
