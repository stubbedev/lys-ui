package buffer

import (
	"fmt"

	"github.com/stubbedev/lys-lib/drm"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// Timeline is a dmabuf buffer's explicit synchronization: the compositor
// samples it once Acquire signals, and it is the client's again once Release
// does, wp_linux_drm_syncobj_surface_v1's points on one timeline of its own.
// wl_buffer.release means nothing for such a buffer: Release, through the
// renderer's watch, frees it.
type Timeline struct {
	WL               *wlr.WpDrmSyncobjTimelineV1
	Syncobj          drm.Syncobj
	Released         func()
	Acquire, Release uint64
	close            func()
}

// NewTimeline is a timeline over wl, dropped by close when its buffer goes.
func NewTimeline(wl *wlr.WpDrmSyncobjTimelineV1, s drm.Syncobj, close func()) *Timeline {
	return &Timeline{WL: wl, Syncobj: s, close: close}
}

// Next moves the timeline on to a frame's points and returns them.
func (t *Timeline) Next() (acquire, release uint64) {
	t.Acquire, t.Release = t.Release+1, t.Release+2
	return t.Acquire, t.Release
}

// SurfaceSync is one surface's wp_linux_drm_syncobj_surface_v1, made with
// its first explicitly synchronized buffer and destroyed before the surface
// takes one that is not, as the compositor would refuse it then.
type SurfaceSync struct {
	obj *wlr.WpDrmSyncobjSurfaceV1
}

// Points sets b's acquire and release points on surf for the commit that
// attaches b, as the protocol requires of every commit attaching a buffer.
func (s *SurfaceSync) Points(mgr *wlr.WpDrmSyncobjManagerV1, surf *wl.Surface, b *Buffer) error {
	t := b.Timeline
	if t == nil {
		return s.Close()
	}
	if s.obj == nil {
		obj, err := mgr.GetSurface(surf)
		if err != nil {
			return fmt.Errorf("buffer: explicit sync for surface %d: %w", surf.Id(), err)
		}
		s.obj = obj
	}
	hi, lo := split(t.Acquire)
	if err := s.obj.SetAcquirePoint(t.WL, hi, lo); err != nil {
		return fmt.Errorf("buffer: acquire point %d: %w", t.Acquire, err)
	}
	hi, lo = split(t.Release)
	if err := s.obj.SetReleasePoint(t.WL, hi, lo); err != nil {
		return fmt.Errorf("buffer: release point %d: %w", t.Release, err)
	}
	return nil
}

func split(point uint64) (hi, lo uint32) {
	return uint32(point >> 32), uint32(point & 0xffffffff)
}

// Close destroys the surface's object, if it has one.
func (s *SurfaceSync) Close() error {
	if s.obj == nil {
		return nil
	}
	err := s.obj.Destroy()
	s.obj = nil
	return err
}
