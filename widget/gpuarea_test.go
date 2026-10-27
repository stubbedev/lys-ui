package widget

import (
	"errors"
	"image"
	"testing"

	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/render"
)

type fakeGPUSurface struct {
	placed []render.Rect
	images int
	closed bool
}

func (f *fakeGPUSurface) Place(r render.Rect) { f.placed = append(f.placed, r) }

func (f *fakeGPUSurface) Import(dmabuf.Buffer) (GPUBuffer, error) { return nil, errors.ErrUnsupported }

func (f *fakeGPUSurface) Present(GPUBuffer) error { return errors.ErrUnsupported }

func (f *fakeGPUSurface) PresentImage(*image.RGBA) error {
	f.images++
	return nil
}

func (f *fakeGPUSurface) RequestFrame() {}

func (f *fakeGPUSurface) Close() { f.closed = true }

func withGPUHost(t *testing.T, fn func(*GPUArea) (GPUSurface, error)) {
	t.Helper()
	SetGPUHost(fn)
	t.Cleanup(func() { SetGPUHost(nil) })
}

func TestGPUAreaAttachesOnScreenAndDetachesOnRemoval(t *testing.T) {
	var surfaces []*fakeGPUSurface
	withGPUHost(t, func(*GPUArea) (GPUSurface, error) {
		s := &fakeGPUSurface{}
		surfaces = append(surfaces, s)
		return s, nil
	})
	area := NewGPUArea()
	if err := area.PresentImage(image.NewRGBA(image.Rect(0, 0, 1, 1))); !errors.Is(err, ErrGPUAreaOffscreen) {
		t.Errorf("presenting before the area is on screen: %v", err)
	}
	area.Arrange(render.Rect{})
	if len(surfaces) != 0 {
		t.Fatal("an empty arrange created a surface")
	}
	root := NewBox(Column, 0, 0)
	root.Append(area, true)
	SizeWithin(root, Size{W: 200, H: 100})
	root.Arrange(render.Rect{W: 200, H: 100})
	root.Arrange(render.Rect{W: 200, H: 100})
	if len(surfaces) != 1 {
		t.Fatalf("%d surfaces after two arranges, want one", len(surfaces))
	}
	if got := surfaces[0].placed; len(got) != 2 || got[1] != (render.Rect{W: 200, H: 100}) {
		t.Errorf("placements %v", got)
	}
	if err := area.PresentImage(image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil || surfaces[0].images != 1 {
		t.Errorf("present on screen: %v, %d images", err, surfaces[0].images)
	}

	root.Remove(area)
	if !surfaces[0].closed {
		t.Error("removing the area left its surface open")
	}
	root.Append(area, true)
	SizeWithin(root, Size{W: 200, H: 100})
	root.Arrange(render.Rect{W: 200, H: 100})
	if len(surfaces) != 2 {
		t.Error("an area back on screen did not get a new surface")
	}
	Detach(root)
	if !surfaces[1].closed {
		t.Error("Detach missed the area")
	}
}

func TestGPUAreaPaintsAHoleForItsSurface(t *testing.T) {
	area := NewGPUArea()
	root := NewOverlay()
	root.Append(area)
	SizeWithin(root, Size{W: 8, H: 8})
	root.Arrange(render.Rect{W: 8, H: 8})
	data := make([]byte, render.Stride(8)*8)
	cv := render.New(data, render.Stride(8), 8, 8)
	cv.Clear(cv.Rect(), render.RGB(255, 255, 255))
	root.Paint(cv)
	if got := render.ColorFromBytes(data[4*(3+8*3):]); got != 0 {
		t.Errorf("the area painted %08x, want transparent", uint32(got))
	}
}

func TestGPUAreaReportsWhyItHasNoSurface(t *testing.T) {
	cause := errors.New("no subcompositor")
	withGPUHost(t, func(*GPUArea) (GPUSurface, error) { return nil, cause })
	area := NewGPUArea()
	area.Arrange(render.Rect{W: 10, H: 10})
	if !errors.Is(area.Err(), cause) {
		t.Errorf("Err %v", area.Err())
	}
	if _, err := area.Import(dmabuf.Buffer{}); !errors.Is(err, ErrGPUAreaOffscreen) {
		t.Errorf("import without a surface: %v", err)
	}
}
