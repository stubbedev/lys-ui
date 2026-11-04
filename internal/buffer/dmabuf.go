package buffer

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/fourcc"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"
)

// Allocate makes a bufW x bufH GPU target off the loop and calls done with it
// on the loop.
type Allocate func(bufW, bufH int, done func(*gpu.Target, error))

// NewDmabuf returns a bufW x bufH buffer the GPU renders into, pending: its
// target is allocated through allocate, so the loop waits on no driver, and
// then imported through mgr as a wl_buffer. Its Data is nil; a frame renders
// into Target.
//
// The import is linux-dmabuf's asynchronous create, whose refusal is an
// event: create_immed's is the fatal invalid_wl_buffer error, which
// would end the whole connection. The buffer is pending until the
// compositor answers, and a pool counts it busy meanwhile. A failed
// allocation or a refusal drops the buffer from its pool and reports the
// error to rejected. A pool avoids the frame's wait for both through
// Pool.Prepare.
func NewDmabuf(allocate Allocate, mgr *wlr.ZwpDmabufV1, bufW, bufH, scale int, rejected func(error)) (*Buffer, error) {
	w, err := intconv.Exact[int32](bufW)
	if err != nil {
		return nil, fmt.Errorf("buffer: dmabuf width: %w", err)
	}
	h, err := intconv.Exact[int32](bufH)
	if err != nil {
		return nil, fmt.Errorf("buffer: dmabuf height: %w", err)
	}
	b := &Buffer{Width: bufW, Height: bufH, Scale: scale, Stale: RegionOf(render.Rect{W: bufW, H: bufH}), pending: true}
	imp := &dmabufImport{b: b, rejected: rejected, done: func() {}}
	start := time.Now()
	allocate(bufW, bufH, func(t *gpu.Target, err error) {
		imp.allocate = time.Since(start)
		imp.allocated(mgr, t, w, h, err)
	})
	return b, nil
}

// allocated imports the target allocate made, unless the buffer was dropped
// meanwhile, which closes it.
func (imp *dmabufImport) allocated(mgr *wlr.ZwpDmabufV1, t *gpu.Target, w, h int32, err error) {
	b := imp.b
	if err != nil && b.freed && errors.Is(err, gpu.ErrDeviceClosed) {
		b.Release()
		return
	}
	if err != nil {
		imp.failed(fmt.Errorf("buffer: allocate a %dx%d dmabuf: %w", b.Width, b.Height, err))
		return
	}
	b.Target = t
	if b.freed {
		b.Release()
		return
	}
	params, err := dmabufwl.Params(mgr, t.Dmabuf())
	if err != nil {
		imp.failed(fmt.Errorf("buffer: dmabuf %dx%d: %w", b.Width, b.Height, err))
		return
	}
	imp.done = func() { _ = params.Destroy() }
	imp.sent = time.Now()
	params.AddCreatedHandler(imp)
	params.AddFailedHandler(imp)
	if err := params.Create(w, h, uint32(fourcc.ARGB8888), 0); err != nil {
		_ = params.Destroy()
		imp.failed(fmt.Errorf("buffer: dmabuf create %dx%d: %w", b.Width, b.Height, err))
	}
}

// dmabufImport is one pending import's verdict handler.
type dmabufImport struct {
	b *Buffer
	// done releases the import's params once the compositor answered.
	done     func()
	rejected func(error)
	// allocate is how long the target took to come back; sent is when the import
	// went out. The LYS_DEBUG=frame log reports both.
	allocate time.Duration
	sent     time.Time
}

// HandleZwpBufferParamsV1Created implements the created handler: the
// buffer takes its wl_buffer and goes back to its pool as a released
// one would.
func (imp *dmabufImport) HandleZwpBufferParamsV1Created(ev wlr.ZwpBufferParamsV1CreatedEvent) {
	debug.Log("frame", "dmabuf %dx%d: target allocated in %v, import answered in %v", imp.b.Width, imp.b.Height, imp.allocate, time.Since(imp.sent))
	imp.done()
	imp.created(ev.Buffer)
}

// HandleZwpBufferParamsV1Failed implements the failed handler.
func (imp *dmabufImport) HandleZwpBufferParamsV1Failed(wlr.ZwpBufferParamsV1FailedEvent) {
	imp.done()
	imp.failed(fmt.Errorf("buffer: the compositor refused a %dx%d dmabuf import", imp.b.Width, imp.b.Height))
}

// created gives the buffer its wl_buffer.
func (imp *dmabufImport) created(wlb *wl.Buffer) {
	b := imp.b
	b.pending = false
	b.WL = wlb
	if wlb != nil {
		wlb.AddReleaseHandler(ReleaseHandler{B: b})
	}
	b.Release()
}

// failed drops the buffer and reports why.
func (imp *dmabufImport) failed(err error) {
	b := imp.b
	b.pending, b.busy, b.freed = false, false, true
	b.closeTarget()
	if p := b.pool; p != nil {
		if p.front == b {
			p.front = nil
		}
		p.buffers = slices.DeleteFunc(p.buffers, func(x *Buffer) bool { return x == b })
		p.dropRetired(b)
		b.pool = nil
	}
	imp.rejected(err)
}

// closeTarget releases a dmabuf buffer's wl_buffer and GPU target.
func (b *Buffer) closeTarget() {
	if t := b.Timeline; t != nil {
		b.Timeline = nil
		t.close()
	}
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
