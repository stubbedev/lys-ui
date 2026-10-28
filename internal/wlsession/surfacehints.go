package wlsession

import (
	"errors"
	"fmt"
	"math"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// Small per-surface protocols: the content-type hint (the compositor
// may tune for photos, video, games), the alpha modifier (opacity the
// compositor applies, no repaint), the background effect (the
// compositor blurs what is behind a region), and the system bell. Each is
// optional: without it the setter reports ErrProtocolUnavailable and
// the surface renders as before.

// ErrProtocolUnavailable reports a compositor without the protocol a
// surface request needs.
var ErrProtocolUnavailable = errors.New("wlsession: protocol unavailable")

// ContentType is a surface's content kind (wp_content_type_v1).
type ContentType uint32

// Content types.
const (
	ContentNone  ContentType = wlr.WpContentTypeV1TypeNone
	ContentPhoto ContentType = wlr.WpContentTypeV1TypePhoto
	ContentVideo ContentType = wlr.WpContentTypeV1TypeVideo
	ContentGame  ContentType = wlr.WpContentTypeV1TypeGame
)

// bindSurfaceHint binds one of the per-surface hint globals.
func (s *Session) bindSurfaceHint(ev wl.RegistryGlobalEvent) {
	ctx := s.registry.Context()
	switch ev.Interface {
	case "wp_content_type_manager_v1":
		mgr := wlr.NewWpContentTypeManagerV1(ctx)
		if s.bindOptional(ev, 1, mgr) {
			s.contentTypeMgr = mgr
		}
	case "wp_alpha_modifier_v1":
		mgr := wlr.NewWpAlphaModifierV1(ctx)
		if s.bindOptional(ev, 1, mgr) {
			s.alphaMgr = mgr
		}
	case "ext_background_effect_manager_v1":
		mgr := wlr.NewExtBackgroundEffectManagerV1(ctx)
		if s.bindOptional(ev, 1, mgr) {
			s.blurMgr = mgr
			mgr.AddCapabilitiesHandler(blurCapabilities{s})
		}
	case "xdg_system_bell_v1":
		bell := wlr.NewXdgSystemBellV1(ctx)
		if s.bindOptional(ev, 1, bell) {
			s.bell = bell
		}
	default:
		return
	}
	debug.Log("shell", "%s bound", ev.Interface)
}

// SetContentType hints surf's content kind; it applies on the
// surface's next commit.
func (s *Session) SetContentType(surf *wl.Surface, ct ContentType) error {
	if s.contentTypeMgr == nil || surf == nil {
		return ErrProtocolUnavailable
	}
	obj := s.contentTypes[surf]
	if obj == nil {
		var err error
		if obj, err = s.contentTypeMgr.GetSurfaceType(surf); err != nil {
			return err
		}
		if s.contentTypes == nil {
			s.contentTypes = map[*wl.Surface]*wlr.WpContentTypeV1{}
		}
		s.contentTypes[surf] = obj
	}
	return obj.SetType(uint32(ct))
}

// SetSurfaceAlpha sets the opacity the compositor applies to surf
// (0..1) without the client repainting; it applies on the surface's
// next commit.
func (s *Session) SetSurfaceAlpha(surf *wl.Surface, alpha float64) error {
	if s.alphaMgr == nil || surf == nil {
		return ErrProtocolUnavailable
	}
	obj := s.alphaSurfaces[surf]
	if obj == nil {
		var err error
		if obj, err = s.alphaMgr.GetSurface(surf); err != nil {
			return err
		}
		if s.alphaSurfaces == nil {
			s.alphaSurfaces = map[*wl.Surface]*wlr.WpAlphaModifierSurfaceV1{}
		}
		s.alphaSurfaces[surf] = obj
	}
	alpha = math.Min(1, math.Max(0, alpha))
	return obj.SetMultiplier(uint32(math.Round(alpha * math.MaxUint32)))
}

// blurCapabilities tracks the background effect manager's offer: the
// compositor may withdraw blur (and restore it) at any time.
type blurCapabilities struct{ s *Session }

// HandleExtBackgroundEffectManagerV1Capabilities records the blur bit.
func (h blurCapabilities) HandleExtBackgroundEffectManagerV1Capabilities(ev wlr.ExtBackgroundEffectManagerV1CapabilitiesEvent) {
	h.s.blurCapable = ev.Flags&wlr.ExtBackgroundEffectManagerV1CapabilityBlur != 0
	debug.Log("shell", "background blur capability %v", h.s.blurCapable)
}

// BlurAvailable reports whether the compositor blurs behind surfaces:
// ext_background_effect_v1 bound and its blur capability offered.
func (s *Session) BlurAvailable() bool { return s.blurMgr != nil && s.blurCapable }

// SetBlurRegion asks the compositor to blur what is behind rects
// (surface-local, logical pixels) of surf; no rects removes the effect.
// It applies on the surface's next commit.
func (s *Session) SetBlurRegion(surf *wl.Surface, rects []render.Rect) error {
	if !s.BlurAvailable() || surf == nil {
		return ErrProtocolUnavailable
	}
	obj := s.blurSurfaces[surf]
	if obj == nil {
		var err error
		if obj, err = s.blurMgr.GetExtBackgroundEffect(surf); err != nil {
			return err
		}
		if s.blurSurfaces == nil {
			s.blurSurfaces = map[*wl.Surface]*wlr.ExtBackgroundEffectSurfaceV1{}
		}
		s.blurSurfaces[surf] = obj
	}
	if len(rects) == 0 {
		return obj.SetBlurRegion(nil)
	}
	region, err := s.compositor.CreateRegion()
	if err != nil {
		return fmt.Errorf("wlsession: blur region: %w", err)
	}
	for _, r := range rects {
		if err := region.Add(intconv.Clamp[int32](r.X), intconv.Clamp[int32](r.Y), intconv.Clamp[int32](r.W), intconv.Clamp[int32](r.H)); err != nil {
			_ = region.Destroy()
			return fmt.Errorf("wlsession: blur region add: %w", err)
		}
	}
	if err := obj.SetBlurRegion(region); err != nil {
		_ = region.Destroy()
		return fmt.Errorf("wlsession: set_blur_region: %w", err)
	}
	return region.Destroy()
}

// Bell rings the system bell for surf (nil: the session's), the error
// ding of a rejected input; a compositor may flash the window instead.
func (s *Session) Bell(surf *wl.Surface) error {
	if s.bell == nil {
		return ErrProtocolUnavailable
	}
	return s.bell.Ring(surf)
}

// releaseSurfaceHints destroys surf's hint objects with its input.
func (s *Session) releaseSurfaceHints(surf *wl.Surface) {
	if obj := s.contentTypes[surf]; obj != nil {
		_ = obj.Destroy()
		delete(s.contentTypes, surf)
	}
	if obj := s.alphaSurfaces[surf]; obj != nil {
		_ = obj.Destroy()
		delete(s.alphaSurfaces, surf)
	}
	if obj := s.blurSurfaces[surf]; obj != nil {
		_ = obj.Destroy()
		delete(s.blurSurfaces, surf)
	}
}
