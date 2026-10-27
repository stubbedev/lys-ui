package buffer

import (
	"fmt"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/dmabuf"
	"github.com/stubbedev/lys-lib/dmabufwl"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
	"github.com/stubbedev/lys-lib/wlr"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// NewDmabuf allocates a bufW x bufH buffer the GPU renders into: a
// target of dev laid out in one of modifiers (ones the compositor
// imports), imported through mgr as a wl_buffer. Its Data is nil; a
// frame renders into Target.
//
// The import is linux-dmabuf's asynchronous create, whose refusal is an
// event: create_immed's is the fatal invalid_wl_buffer error, which
// would end the whole connection. The buffer is pending until the
// compositor answers, and a pool counts it busy meanwhile. A refusal
// drops the buffer from its pool and reports the error to rejected.
func NewDmabuf(dev *gpu.Device, mgr *wlr.ZwpDmabufV1, bufW, bufH, scale int, modifiers []uint64, rejected func(error)) (*Buffer, error) {
	start := time.Now()
	t, err := dev.NewTarget(bufW, bufH, dmabuf.FormatARGB8888, modifiers)
	if err != nil {
		return nil, err
	}
	allocated := time.Now()
	w, err := intconv.CheckedInt32(bufW)
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("buffer: dmabuf width: %w", err)
	}
	h, err := intconv.CheckedInt32(bufH)
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("buffer: dmabuf height: %w", err)
	}
	params, err := dmabufwl.Params(mgr, t.Dmabuf())
	if err != nil {
		t.Close()
		return nil, fmt.Errorf("buffer: dmabuf %dx%d: %w", bufW, bufH, err)
	}
	b := &Buffer{Width: bufW, Height: bufH, Scale: scale, Stale: render.Rect{W: bufW, H: bufH}, Target: t, pending: true}
	imp := &dmabufImport{b: b, done: func() { _ = params.Destroy() }, rejected: rejected, allocate: allocated.Sub(start), sent: time.Now()}
	params.AddCreatedHandler(imp)
	params.AddFailedHandler(imp)
	if err := params.Create(w, h, dmabuf.FormatARGB8888, 0); err != nil {
		_ = params.Destroy()
		t.Close()
		return nil, fmt.Errorf("buffer: dmabuf create %dx%d: %w", bufW, bufH, err)
	}
	return b, nil
}

// dmabufImport is one pending import's verdict handler.
type dmabufImport struct {
	b *Buffer
	// done releases the import's params once the compositor answered.
	done     func()
	rejected func(error)
	// allocate is how long the target took; sent is when the import
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
	imp.failed()
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

// failed drops the refused buffer and reports the refusal.
func (imp *dmabufImport) failed() {
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
	imp.rejected(fmt.Errorf("buffer: the compositor refused a %dx%d dmabuf import", b.Width, b.Height))
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
