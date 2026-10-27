package widget

import (
	"errors"
	"image"
	"sync/atomic"

	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/render"
)

// ErrGPUAreaOffscreen reports a GPUArea call before the area is on
// screen: frames are imported and presented once OnFrame first runs.
var ErrGPUAreaOffscreen = errors.New("widget: the GPUArea is not on screen yet")

// GPUFrame is a GPUArea's request for a frame.
type GPUFrame struct {
	// Size is the area in device pixels: a frame this size shows one
	// to one, any other size is scaled to the area.
	Size Size
}

// GPUBuffer is a dmabuf the compositor imported for a GPUArea.
type GPUBuffer interface {
	// Ready reports whether the import finished; Present waits for it.
	Ready() bool
	// Busy reports whether the compositor still reads the buffer: do
	// not render into it until it is free.
	Busy() bool
	// Err reports a rejected import, or that the area left the screen
	// and the buffer must be imported again.
	Err() error
	// Destroy releases the compositor-side buffer; the dmabuf's fds
	// stay the caller's.
	Destroy()
}

// GPUSurface is the presentation surface package app gives a GPUArea
// on screen. Applications use GPUArea's methods instead.
type GPUSurface interface {
	Place(bounds render.Rect)
	Import(b dmabuf.Buffer) (GPUBuffer, error)
	Present(b GPUBuffer) error
	PresentImage(img *image.RGBA) error
	RequestFrame()
	Close()
}

var gpuHost atomic.Pointer[func(*GPUArea) (GPUSurface, error)]

// SetGPUHost installs the function that gives an on-screen GPUArea its
// surface; package app installs it. nil disables.
func SetGPUHost(fn func(a *GPUArea) (GPUSurface, error)) {
	if fn == nil {
		gpuHost.Store(nil)
		return
	}
	gpuHost.Store(&fn)
}

// GPUArea is GTK's GLArea without cgo: the application renders with
// whatever GPU stack it links (GL, Vulkan, a compute library) into
// dmabufs, or into CPU images, and the area presents them on its own
// compositor surface at its bounds, dmabufs with no copy. The surface
// sits below the window and shows through the area's bounds, so
// widgets drawn over the area stay on top of it. OnFrame paces the
// application: it runs once the area is on screen, after each
// presented frame reaches the screen, after a resize, and on
// QueueFrame. It styles as `gpuarea`.
type GPUArea struct {
	node
	// OnFrame asks for a frame; it runs on the loop.
	OnFrame func(f GPUFrame)
	surface GPUSurface
	err     error
}

// NewGPUArea returns an empty area; give it a size through the layout
// (expand, a minimum size in CSS).
func NewGPUArea() *GPUArea {
	a := &GPUArea{}
	a.SetElement("gpuarea")
	return a
}

// Err returns why the area has no surface: the compositor lacks
// subsurfaces, or the area is not in a window.
func (a *GPUArea) Err() error { return a.err }

// Import imports a dmabuf for presenting. The import completes
// asynchronously; Present of a buffer still importing shows it once
// it is ready.
func (a *GPUArea) Import(b dmabuf.Buffer) (GPUBuffer, error) {
	if a.surface == nil {
		return nil, ErrGPUAreaOffscreen
	}
	return a.surface.Import(b)
}

// Present shows an imported dmabuf.
func (a *GPUArea) Present(b GPUBuffer) error {
	if a.surface == nil {
		return ErrGPUAreaOffscreen
	}
	return a.surface.Present(b)
}

// PresentImage shows a CPU-rendered frame (premultiplied, as
// image.RGBA is), copied into a shared-memory buffer.
func (a *GPUArea) PresentImage(img *image.RGBA) error {
	if a.surface == nil {
		return ErrGPUAreaOffscreen
	}
	return a.surface.PresentImage(img)
}

// QueueFrame asks for an OnFrame at the next frame, for an application
// that skipped presenting and wants to resume.
func (a *GPUArea) QueueFrame() {
	if a.surface != nil {
		a.surface.RequestFrame()
	}
}

// Measure claims no size of its own: the layout sizes the area.
func (a *GPUArea) Measure(axis Axis, forSize int) Sizes {
	if sz, ok := a.measureHit(axis, forSize); ok {
		return sz
	}
	return a.measureStore(axis, forSize, Sizes{})
}

// Arrange places the area's surface over its bounds, creating it the
// first time the area lands on screen.
func (a *GPUArea) Arrange(r render.Rect) {
	a.node.Arrange(r)
	if r.Empty() {
		return
	}
	if a.surface == nil {
		host := gpuHost.Load()
		if host == nil {
			return
		}
		s, err := (*host)(a)
		if err != nil {
			a.err = err
			return
		}
		a.surface, a.err = s, nil
	}
	a.surface.Place(r)
}

// Paint clears the bounds to transparent, the window through which the
// surface below shows.
func (a *GPUArea) Paint(cv *render.Canvas) {
	cv.Clear(a.bounds, 0)
}

// HitTest returns the area when p is inside its bounds.
func (a *GPUArea) HitTest(p Point) Widget { return a.HitLeaf(a, p) }

func (a *GPUArea) detach() {
	if a.surface == nil {
		return
	}
	a.surface.Close()
	a.surface = nil
}

// Detach releases what w's tree holds for being on screen, for a tree
// leaving it (a removed widget, a closed window or popover): every
// GPUArea's surface, and the rasters every Image resampled from a
// source only it holds. A tree shown again re-creates them on its
// next paint.
func Detach(w Widget) {
	walkWidgets(w, func(w Widget) bool {
		if d, ok := w.(detacher); ok {
			d.detach()
		}
		return true
	})
}

// detacher is a widget holding something for being on screen that a
// tree leaving it releases (Detach).
type detacher interface{ detach() }
