package app

import (
	"fmt"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// subsurface is a surface of its own under a window's, for content the
// window does not paint (a GPUArea's frames, a Video's): below the
// window, which leaves a hole where it shows, desynchronized so it
// commits at its own pace, transparent to input, and with a viewport
// where the compositor has wp_viewporter.
type subsurface struct {
	surf *wl.Surface
	sub  *wl.Subsurface
	vp   *wlr.WpViewport
}

// newSubsurface makes a subsurface below parent; on an error what it
// made is destroyed.
func (a *Application) newSubsurface(subc *wl.Subcompositor, parent *wl.Surface) (subsurface, error) {
	surf, err := a.sess.Compositor().CreateSurface()
	if err != nil {
		return subsurface{}, fmt.Errorf("app: subsurface surface: %w", err)
	}
	s := subsurface{surf: surf}
	if err := s.init(a, subc, parent); err != nil {
		s.destroy()
		return subsurface{}, err
	}
	return s, nil
}

func (s *subsurface) init(a *Application, subc *wl.Subcompositor, parent *wl.Surface) error {
	var err error
	if s.sub, err = subc.GetSubsurface(s.surf, parent); err != nil {
		return fmt.Errorf("app: get_subsurface: %w", err)
	}
	if err := s.sub.PlaceBelow(parent); err != nil {
		return fmt.Errorf("app: subsurface place_below: %w", err)
	}
	if err := s.sub.SetDesync(); err != nil {
		return fmt.Errorf("app: subsurface set_desync: %w", err)
	}
	region, err := a.sess.Compositor().CreateRegion()
	if err != nil {
		return fmt.Errorf("app: subsurface input region: %w", err)
	}
	err = s.surf.SetInputRegion(region)
	_ = region.Destroy()
	if err != nil {
		return fmt.Errorf("app: subsurface set_input_region: %w", err)
	}
	if vp := a.sess.Viewporter(); vp != nil {
		if s.vp, err = vp.GetViewport(s.surf); err != nil {
			return fmt.Errorf("app: subsurface viewport: %w", err)
		}
	}
	return nil
}

func (s *subsurface) destroy() {
	if s.vp != nil {
		_ = s.vp.Destroy()
	}
	if s.sub != nil {
		_ = s.sub.Destroy()
	}
	_ = s.surf.Destroy()
}

// subsurfaceHoles are the bounds of the GPUArea and Video surfaces
// under the window, where it is not opaque.
func (w *hostWindow) subsurfaceHoles() []render.Rect {
	var holes []render.Rect
	for _, s := range w.gpu {
		if !s.bounds.Empty() {
			holes = append(holes, s.bounds)
		}
	}
	for _, s := range w.video {
		if !s.bounds.Empty() {
			holes = append(holes, s.bounds)
		}
	}
	return holes
}
