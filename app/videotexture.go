package app

import (
	"errors"
	"fmt"
	"image"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/widget"
)

var (
	// ErrNoGPURenderer reports a Video of a stream decoding on the GPU
	// in an application whose windows draw on the CPU: the frames decode
	// on the device the windows draw with.
	ErrNoGPURenderer = errors.New("app: the windows draw on the CPU, and video decoded on the GPU needs the GPU renderer")
	// ErrVideoDeviceGone reports a Video surface whose GPU was lost or
	// closed under it.
	ErrVideoDeviceGone = errors.New("app: the GPU the video drew on is gone")
)

func (a *Application) textureSurfaceFor(v *widget.Video, width, height int, wake func()) (widget.TextureSurface, error) {
	hw, subc, err := a.videoWindow(v)
	if err != nil {
		return nil, err
	}
	r, err := a.renderer()
	switch {
	case err != nil:
		return nil, err
	case r == nil && a.gpuPending:
		a.gpuWaiting = append(a.gpuWaiting, wake)
		return nil, widget.ErrVideoDevicePending
	case r == nil || r.Failed():
		return nil, ErrNoGPURenderer
	}
	s := &textureSurface{r: r, damage: [1]image.Rectangle{image.Rect(0, 0, width, height)}}
	if err := s.open(a, hw, subc, wake); err != nil {
		return nil, err
	}
	s.device = r.dev
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

// loseVideoTextures fails the Video surfaces drawn on r's device, which
// is closing: their frames and buffers go with it.
func (a *Application) loseVideoTextures(r *gpuRenderer) {
	for _, w := range a.windows {
		for _, o := range w.video {
			if o.device == r.dev && o.err == nil {
				o.err = fmt.Errorf("%w: %s", ErrVideoDeviceGone, r.name)
			}
		}
	}
}

// textureSurface is a Video's subsurface for a stream decoding on the
// GPU: its ring is ARGB8888 targets of the renderer's device, in a
// layout the compositor imports, each frame drawn into one through its
// YCbCr conversion; the picture never leaves the GPU.
type textureSurface struct {
	videoOutput
	r       *gpuRenderer
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
	if err := drawVideoFrame(s.r.dev, s.targets[i], s.damage[:], f); err != nil {
		return false, err
	}
	if err := s.show(i, s.damage[0].Dx(), s.damage[0].Dy()); err != nil {
		return false, err
	}
	return true, nil
}

// drawVideoFrame draws f's picture over all of t, whose whole is
// whole[0], through its YCbCr conversion, and hands t to the
// compositor: the pass's fence on t's dmabuf, which linux-dmabuf's
// implicit synchronization has every reader wait on, or else the pass
// finished here.
func drawVideoFrame(dev *gpu.Device, t *gpu.Target, whole []image.Rectangle, f *gpu.VideoFrame) error {
	p, err := dev.Begin(t, whole)
	if err != nil {
		return fmt.Errorf("app: draw a video frame: %w", err)
	}
	pic := f.Rect()
	src := gpu.FRect{X: float64(pic.Min.X), Y: float64(pic.Min.Y), W: float64(pic.Dx()), H: float64(pic.Dy())}
	p.DrawTexture(f.Texture(), src, whole[0], gpu.TransformNormal, 1, gpu.Mask{})
	if err := p.End(); err != nil {
		return fmt.Errorf("app: draw a video frame: %w", err)
	}
	fd, err := dev.SyncFile()
	switch {
	case errors.Is(err, gpu.ErrNoSyncFile):
		return finishVideoFrame(dev)
	case err != nil:
		return fmt.Errorf("app: a video frame's fence: %w", err)
	case fd < 0:
		return nil
	}
	err = dmabuf.ImportSyncFile(t.Dmabuf().Planes[0].FD, fd)
	if cerr := unix.Close(fd); err == nil && cerr != nil {
		return fmt.Errorf("app: close a video frame's fence: %w", cerr)
	}
	switch {
	case errors.Is(err, dmabuf.ErrNoSyncFileImport):
		return finishVideoFrame(dev)
	case err != nil:
		return fmt.Errorf("app: attach a video frame's fence: %w", err)
	}
	return nil
}

func finishVideoFrame(dev *gpu.Device) error {
	if err := dev.Finish(); err != nil {
		return fmt.Errorf("app: finish a video frame: %w", err)
	}
	return nil
}

func (s *textureSurface) Close() {
	s.close(func() {
		for i, t := range s.targets {
			if t != nil {
				t.Close()
				s.targets[i] = nil
			}
		}
	})
}
