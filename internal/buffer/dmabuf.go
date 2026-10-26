package buffer

import (
	"fmt"

	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/wlr"
)

// NewDmabuf allocates a bufW x bufH buffer the GPU renders into: a
// target of dev laid out in one of modifiers (ones the compositor
// imports), imported through mgr as a wl_buffer. Its Data is nil; a
// frame renders into Target.
func NewDmabuf(dev *gpu.Device, mgr *wlr.ZwpDmabufV1, bufW, bufH, scale int, modifiers []uint64) (*Buffer, error) {
	t, err := dev.NewTarget(bufW, bufH, dmabuf.FormatARGB8888, modifiers)
	if err != nil {
		return nil, err
	}
	params, err := dmabufwl.Params(mgr, t.Dmabuf())
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("buffer: dmabuf %dx%d: %w", bufW, bufH, err)
	}
	wlb, err := params.CreateImmed(int32(bufW), int32(bufH), dmabuf.FormatARGB8888, 0)
	_ = params.Destroy()
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("buffer: dmabuf create_immed %dx%d: %w", bufW, bufH, err)
	}
	b := &Buffer{
		WL: wlb, Width: bufW, Height: bufH, Scale: scale,
		Stale: render.Rect{W: bufW, H: bufH}, Target: t,
	}
	wlb.AddReleaseHandler(ReleaseHandler{B: b})
	return b, nil
}

// closeTarget releases a dmabuf buffer's wl_buffer and GPU target.
func (b *Buffer) closeTarget() {
	if b.Target == nil {
		return
	}
	if b.WL != nil {
		_ = b.WL.Destroy()
		b.WL = nil
	}
	b.Target.Close()
	b.Target = nil
}
