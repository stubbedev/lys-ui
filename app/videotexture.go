package app

import (
	"errors"
	"fmt"
	"image"
	"slices"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"

	"github.com/stubbedev/lys-ui/widget"
)

var (
	// ErrNoGPURenderer reports a Video of a stream decoding on the GPU
	// in an application whose windows draw on the CPU: the frames decode
	// on the device the windows draw with.
	ErrNoGPURenderer = errors.New("app: the windows draw on the CPU, and video decoded on the GPU needs the GPU renderer")
	// ErrVideoDeviceGone reports a Video surface whose GPU was closed
	// under it.
	ErrVideoDeviceGone = errors.New("app: the GPU the video drew on is gone")
)

func (a *Application) textureSurfaceFor(v *widget.Video, width, height int, wake, lost func()) (widget.TextureSurface, error) {
	hw, subc, err := a.videoWindow(v)
	if err != nil {
		return nil, err
	}
	r, err := a.renderer()
	switch {
	case err != nil:
		return nil, err
	case r == nil && a.gpuPending, r != nil && r.leaving():
		a.gpuWaiting = append(a.gpuWaiting, wake)
		return nil, widget.ErrVideoDevicePending
	case r == nil:
		return nil, ErrNoGPURenderer
	case r.Failed():
		return nil, fmt.Errorf("%w: %w", ErrNoGPURenderer, r.failed)
	}
	s := &textureSurface{r: r, lost: lost, damage: [1]image.Rectangle{image.Rect(0, 0, width, height)}}
	if err := s.open(a, hw, subc, wake); err != nil {
		return nil, err
	}
	r.videos = append(r.videos, s)
	if err := s.init(width, height); err != nil {
		s.Close()
		return nil, err
	}
	debug.Log("frame", "video surface %d: %dx%d drawn on %s", s.surf.Id(), width, height, r.name)
	return s, nil
}

// wakeGPUWaiting wakes the Videos that waited for the renderer: they
// ask for their surfaces again, on its device or, without one, to be
// told there is none.
func (a *Application) wakeGPUWaiting() {
	waiting := a.gpuWaiting
	a.gpuWaiting = nil
	for _, wake := range waiting {
		wake()
	}
}

func (r *gpuRenderer) failVideos() {
	for _, s := range r.videos {
		if s.err == nil {
			s.err = fmt.Errorf("%w: %s", ErrVideoDeviceGone, r.name)
		}
	}
}

func (a *Application) detachVideos(r *gpuRenderer) {
	for _, s := range slices.Clone(r.videos) {
		a.gpuWaiting = append(a.gpuWaiting, s.wake)
		s.lost()
	}
}

func (a *Application) videoDeviceLost(r *gpuRenderer, err error) {
	if r.closed {
		return
	}
	r.fault(err)
	if r.leaving() {
		return
	}
	a.Invoke(func() {
		a.detachVideos(r)
		a.wakeGPUWaiting()
	})
}

// textureSurface is a Video's subsurface for a stream decoding on the
// GPU: its ring is ARGB8888 targets of the renderer's device, in a
// layout the compositor imports, each frame drawn into one through its
// YCbCr conversion; the picture never leaves the GPU.
type textureSurface struct {
	videoOutput
	r       *gpuRenderer
	lost    func()
	targets [videoRing]*gpu.Target
	// damage is the whole target, kept here so a pass takes it without
	// an allocation.
	damage [1]image.Rectangle
}

func (s *textureSurface) init(width, height int) error {
	if err := setVideoContent(s.app, s.surf); err != nil {
		return err
	}
	for i := range s.targets {
		t, err := s.r.dev.NewTarget(width, height, fourcc.ARGB8888, s.r.modifiers)
		if err != nil {
			return fmt.Errorf("app: video target %d of %d on %s: %w", i+1, videoRing, s.r.name, err)
		}
		s.targets[i] = t
		if err := s.ring[i].importDmabuf(t.Dmabuf()); err != nil {
			return fmt.Errorf("app: video target %d of %d: %w", i+1, videoRing, err)
		}
	}
	return nil
}

func (s *textureSurface) Device() *gpu.Device { return s.r.dev }

func (s *textureSurface) Present(f *gpu.VideoFrame) (bool, error) {
	i, err := s.writable()
	if i < 0 {
		return false, err
	}
	switch err := s.r.video(s.targets[i], s.damage[:], f); {
	case errors.Is(err, gpu.ErrDeviceLost):
		s.DeviceLost(err)
		return false, nil
	case err != nil:
		return false, err
	}
	if err := s.show(i, s.damage[0].Dx(), s.damage[0].Dy()); err != nil {
		return false, err
	}
	return true, nil
}

// drawVideoFrame draws f's picture over all of t, whose whole is
// whole[0], through its YCbCr conversion, and hands t to the
// compositor (gpu.Device.Publish).
func drawVideoFrame(dev *gpu.Device, t *gpu.Target, whole []image.Rectangle, f *gpu.VideoFrame) error {
	p, err := dev.Begin(t, whole)
	if err != nil {
		return fmt.Errorf("app: draw a video frame: %w", err)
	}
	size := f.Size()
	p.DrawFrame(f, gpu.FRect{W: float64(size.X), H: float64(size.Y)}, whole[0], gpu.TransformNormal, 1, gpu.Mask{})
	if err := p.End(); err != nil {
		return fmt.Errorf("app: draw a video frame: %w", err)
	}
	if err := dev.Publish(t); err != nil {
		return fmt.Errorf("app: hand a video frame to the compositor: %w", err)
	}
	return nil
}

func (s *textureSurface) DeviceLost(err error) {
	if !s.closed {
		s.app.videoDeviceLost(s.r, err)
	}
}

func (s *textureSurface) Close() {
	s.close(func() {
		s.r.videos = slices.DeleteFunc(s.r.videos, func(v *textureSurface) bool { return v == s })
		for i, t := range s.targets {
			if t != nil {
				t.Close()
				s.targets[i] = nil
			}
		}
	})
}
