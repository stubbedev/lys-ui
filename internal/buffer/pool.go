// Package buffer owns the wl_shm buffer lifecycle for a session: one
// shared shm pool (Arena) whose sub-allocations rotate through
// per-surface pools, so the shell never draws into a buffer the
// compositor is still reading from and no memfd is ever created twice.
package buffer

import (
	"errors"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// ErrBusy reports that every buffer in the pool is held by the compositor.
// The caller skips the frame and retries after the next release event or
// frame callback.
var ErrBusy = errors.New("buffer: all pool buffers busy")

// ErrClosed reports the pool was closed for its surface's teardown; it
// hands out nothing anymore.
var ErrClosed = errors.New("buffer: pool closed")

// Format is the wl_shm pixel format every buffer this package creates
// advertises to the compositor: ARGB8888, premultiplied. XRGB is
// deliberately never used - a translucent surface (an app background
// with alpha < 255, the default panel shape) would lose its alpha
// channel and composite as garbage. In memory the format is
// little-endian, so a pixel sits as bytes B, G, R, A - the layout
// render.ColorFromBytes decodes and format_test.go pins.
const Format = wl.ShmFormatArgb8888

// Buffer is one wl_shm-backed pixel buffer: in production a
// sub-allocation of the session arena's shared pool, sharing its fd and
// mapping.
type Buffer struct {
	// WL is the wayland buffer object; nil only in tests of pool
	// bookkeeping.
	WL *wl.Buffer

	// Data is the mmap view of the buffer: Height rows of Stride bytes,
	// ARGB8888 premultiplied, byte order B, G, R, A. Nil for a dmabuf
	// buffer.
	Data []byte

	// Target is a dmabuf buffer's GPU target (NewDmabuf), which a frame
	// renders into; nil for a shm buffer.
	Target *gpu.Target

	// Width and Height are in buffer pixels (output scale already
	// applied).
	Width, Height int
	Stride        int
	Scale         int

	// Stale is the region where this buffer's content lags what is on
	// screen: the union of damage painted into other buffers since this
	// one was last presented. A frame drawn here must repaint Stale plus
	// the frame's own damage; a fresh buffer starts fully stale.
	Stale render.Rect

	busy bool
	// used marks a buffer Acquire handed out since the pool's last
	// Trim.
	used bool
	// pending marks a dmabuf buffer whose import the compositor has not
	// answered yet: it has no wl_buffer, and its pool counts it busy.
	pending bool

	// arena owns this buffer's storage; nil for wire-free test buffers.
	arena *Arena
	// slot is the buffer's byte range in the arena's pool.
	slot slot
	// freed marks a buffer its owner dropped: the slot returns to the
	// arena when the compositor releases the buffer (now, if it already
	// has). A freed buffer is never handed out again.
	freed bool
	// gen is the arena mapping generation the Data window was built
	// against; refresh re-points it when the arena remapped on growth.
	gen int
	// pool is the rotation this buffer belongs to; nil for one-shot
	// buffers. Release events use it to leave the retired list.
	pool *Pool
}

// Release marks the buffer free for the next frame. Only the wayland
// release event handler should call it; calling it earlier allows drawing
// into a buffer the compositor may still read.
//
// A pool buffer goes back to its rotation; a buffer no pool owns - a
// one-shot surface's frame - is done after its release: the slot returns
// to the session arena right here. And a buffer its owner already
// dropped (resize, close, Cancel) has its deferred slot return now.
func (b *Buffer) Release() {
	b.busy = false
	if !b.freed && b.pool != nil {
		return // still in its pool's rotation
	}
	b.closeTarget()
	if b.arena != nil {
		b.arena.retire(b)
	}
	if b.pool != nil {
		b.pool.dropRetired(b)
		b.pool = nil
	}
}

// Busy reports whether the compositor still holds the buffer.
func (b *Buffer) Busy() bool {
	return b.busy
}

// refresh re-points the buffer's view into the arena's current mapping.
// Growth remaps, and a pool's rotation may hold a buffer across that:
// slots are file offsets and survive the move, only the window has to be
// rebuilt.
func (b *Buffer) refresh() {
	if b.arena == nil || b.gen == b.arena.gen {
		return
	}
	b.Data = b.arena.data[b.slot.offset : b.slot.offset+b.Stride*b.Height]
	b.gen = b.arena.gen
}

// NewFile allocates a bufW x bufH buffer of buffer pixels at the given
// integer output scale. The signature predates the session pool and is
// kept for its callers: the buffer is no longer a fresh memfd plus a
// throwaway wl_shm pool but a sub-allocation of shm's session arena, so
// N windows and resizes still mean exactly one fd and one mapping.
//
// shm may be nil in wire-free tests: the arena then runs without the
// wl_shm pool proxy and hands out buffers with a nil WL.
func NewFile(shm *wl.Shm, bufW, bufH, scale int) (*Buffer, error) {
	return forShm(shm).Acquire(bufW, bufH, scale)
}

// Cancel gives up a buffer whose commit never went out: the compositor
// never saw it, no release can arrive, and the slot returns to the
// arena immediately. Committed one-shot buffers need no call at all:
// their release event retires them.
func Cancel(b *Buffer) {
	if b == nil || b.freed || b.arena == nil {
		return
	}
	b.freed = true
	b.arena.retire(b)
}

// Pool is a fixed-capacity set of buffers for one surface, backed by the
// session arena through create.
type Pool struct {
	// create allocates a buffer at the pool's current size; swapped out
	// by Resize.
	create   func() (*Buffer, error)
	capacity int
	buffers  []*Buffer
	// front is the buffer presented last: its content is the screen's.
	front *Buffer

	// retired holds buffers Resize or Close found still held by the
	// compositor: at the previous size they can never be handed out
	// again, but destroying them would orphan their arena slots while
	// the compositor reads - so they survive until the release event
	// retires them. The list is capped; overflow drops the oldest.
	retired []*Buffer
}

// New returns a pool holding at most capacity buffers, allocated on demand
// through create. A capacity below 1 is treated as 1.
func New(create func() (*Buffer, error), capacity int) *Pool {
	if capacity < 1 {
		capacity = 1
	}
	return &Pool{create: create, capacity: capacity}
}

// Acquire returns a buffer to draw the next frame into and marks it busy.
// Free buffers are reused before new ones are allocated; a new buffer
// still pending its import joins the pool busy, and the call reports
// ErrBusy until the compositor's answer frees it. ErrBusy means
// every buffer is held by the compositor: skip the frame and retry after
// a release event - the pool never grows past capacity to dodge it, and
// pending buffers never stack beyond the cap. Allocation failures are
// returned verbatim.
func (p *Pool) Acquire() (*Buffer, error) {
	if p.create == nil {
		return nil, ErrClosed
	}
	for _, b := range p.buffers {
		if !b.busy {
			b.refresh()
			b.busy, b.used = true, true
			return b, nil
		}
	}
	if len(p.buffers) < p.capacity {
		b, err := p.create()
		if err != nil {
			return nil, err
		}
		b.busy, b.used = true, true
		b.pool = p
		p.buffers = append(p.buffers, b)
		if b.pending {
			return nil, ErrBusy
		}
		return b, nil
	}
	return nil, ErrBusy
}

// dropRetired removes a buffer from the retired list once its release
// landed (Buffer.Release calls this).
func (p *Pool) dropRetired(b *Buffer) {
	for i, r := range p.retired {
		if r == b {
			p.retired = append(p.retired[:i], p.retired[i+1:]...)
			return
		}
	}
}

// Presented records a committed frame. changed is the region where the
// screen actually differs after this commit (the frame's own damage, not
// the buffer-initialization paint a fresh buffer also performs): b's
// content now matches the screen, while every other buffer in the pool
// lags by changed and must repaint it before its next reuse. This is
// what makes partial damage safe with a rotating pool: whichever buffer
// comes back next carries the accumulated damage of every frame missed
// since its content was on screen. Retired buffers are skipped: they
// were replaced by a resize and will never be shown again.
func (p *Pool) Presented(b *Buffer, changed render.Rect) {
	// b was just repainted to match the screen, whatever it owed before.
	b.Stale = render.Rect{}
	p.front = b
	if changed.Empty() {
		return
	}
	for _, o := range p.buffers {
		if o != b {
			o.Stale = o.Stale.Union(changed)
		}
	}
}

// Front is the buffer presented last, whose content is what the screen
// shows: a reused buffer can copy what it owes from it (CopyRect)
// rather than paint it again. Nil before the first present and after a
// resize.
func (p *Pool) Front() *Buffer { return p.front }

// CopyRect copies r, in buffer pixels, from src's pixels into dst's:
// two shared-memory buffers of one size. Both views follow the arena's
// current mapping first: a buffer that sat out frames can still point
// at the one a growth unmapped.
func CopyRect(dst, src *Buffer, r render.Rect) {
	r = r.Intersect(render.Rect{W: dst.Width, H: dst.Height})
	if r.Empty() {
		return
	}
	dst.refresh()
	src.refresh()
	for y := r.Y; y < r.Y+r.H; y++ {
		at := y*dst.Stride + r.X*4
		copy(dst.Data[at:at+r.W*4], src.Data[at:at+r.W*4])
	}
}

// Copier copies rects between two GPU targets' buffers: a surface's
// GPU renderer. A copy it cannot make reports false, with the error
// when the GPU failed.
type Copier interface {
	Copy(dst, src *Buffer, r render.Rect) (bool, error)
}

// CopyForward brings b up to the screen where it is stale by copying
// that region from the front buffer, which shows what the screen does,
// instead of painting it: a buffer that sat out frames (the third,
// taken while frames outpace releases) can owe most of the surface.
// Shared-memory buffers copy on the CPU, GPU targets through gpu (nil
// when the surface has no renderer). A copy that cannot be made leaves
// b stale, for the frame to paint; the GPU's error comes back.
func (p *Pool) CopyForward(b *Buffer, gpu Copier) error {
	front := p.front
	stale := b.Stale.Intersect(render.Rect{W: b.Width, H: b.Height})
	if stale.Empty() || front == nil || front == b || front.Width != b.Width || front.Height != b.Height {
		return nil
	}
	switch {
	case b.Target != nil && front.Target != nil && gpu != nil:
		if ok, err := gpu.Copy(b, front, stale); !ok {
			return err
		}
	case b.Data != nil && front.Data != nil:
		CopyRect(b, front, stale)
	default:
		return nil
	}
	b.Stale = render.Rect{}
	return nil
}

// Resize swaps the pool to a new buffer size. Free buffers are disposed
// on the spot; buffers the compositor still holds are preserved - their
// wl_buffer objects and arena slots stay alive until the release event
// lands, so a resize never paints over memory the compositor reads.
// The retired list is capped at the pool capacity: overflow drops the
// oldest frame's buffer instead of stacking unbounded pending buffers
// (destroying a busy wl_buffer is allowed; its release simply never
// arrives, and its slot stays reserved until the arena closes).
func (p *Pool) Resize(create func() (*Buffer, error)) {
	p.create = create
	p.front = nil
	for _, b := range p.buffers {
		dispose(b) // free: fully gone; held: dropped, storage waits for release
		if b.busy {
			p.retired = append(p.retired, b)
		}
	}
	p.buffers = p.buffers[:0]
	p.capRetired()
}

// Close drops the pool for a closing surface: free buffers return to
// the arena now, held ones survive until the compositor releases them.
func (p *Pool) Close() {
	p.create = nil
	p.front = nil
	for _, b := range p.buffers {
		dispose(b)
		if b.busy {
			p.retired = append(p.retired, b)
		}
	}
	p.buffers = nil
	p.capRetired()
}

// Reclaim ends the wait on the buffers a closed pool's surface still
// held. Call it once the compositor has processed the surface's
// destruction: a compositor need not release what a destroyed surface
// held (sway does not release the buffer current at the destroy), so
// a held buffer would otherwise keep its slot resident for the rest of
// the session. The compositor no longer reads them; each is destroyed
// and its slot returns to the arena.
func (p *Pool) Reclaim() {
	held := append([]*Buffer(nil), p.retired...)
	for _, b := range held {
		b.Release()
	}
}

// Trim disposes of the free buffers no Acquire handed out since the
// previous Trim, the front aside: a surface that stopped drawing, or
// draws slower than the compositor releases, keeps only what its
// frames use. Acquire allocates again on demand, and a new buffer
// repaints or copies forward what it lacks.
func (p *Pool) Trim() {
	kept := p.buffers[:0]
	for _, b := range p.buffers {
		used := b.used
		b.used = false
		if used || b.busy || b == p.front {
			kept = append(kept, b)
			continue
		}
		dispose(b)
	}
	clear(p.buffers[len(kept):])
	p.buffers = kept
}

// Spares reports whether the rotation holds a buffer beyond the one a
// frame needs: what Trim may free.
func (p *Pool) Spares() bool { return len(p.buffers) > 1 }

// Footprint is how many buffers the pool holds, rotation and retired,
// and their pixel bytes.
func (p *Pool) Footprint() (n, bytes int) {
	for _, list := range [][]*Buffer{p.buffers, p.retired} {
		for _, b := range list {
			n++
			bytes += b.Width * b.Height * 4
		}
	}
	return n, bytes
}

// capRetired bounds the pending stack: beyond the capacity, the oldest
// frame's buffer is destroyed and its slot abandoned rather than kept
// forever.
func (p *Pool) capRetired() {
	for len(p.retired) > p.capacity {
		oldest := p.retired[0]
		p.retired = p.retired[1:]
		if oldest.arena != nil {
			oldest.arena.abandon(oldest)
		}
		oldest.freed = true
		oldest.closeTarget()
		if oldest.WL != nil {
			_ = oldest.WL.Destroy()
			oldest.WL = nil
		}
	}
}

// Pending reports how many buffers the pool still tracks: the rotation
// plus buffers waiting for the compositor's release. Zero after Close
// means every slot came back.
func (p *Pool) Pending() int {
	return len(p.buffers) + len(p.retired)
}

// dispose drops a buffer the pool no longer rotates through: its slot
// goes back to the arena when it is already free, and otherwise when the
// compositor's release lands (the wl_buffer stays alive until then, so
// the release can arrive and the compositor keeps a valid read).
func dispose(b *Buffer) {
	if b == nil || b.freed {
		return
	}
	b.freed = true
	if !b.busy {
		b.closeTarget()
		if b.WL != nil {
			_ = b.WL.Destroy()
			b.WL = nil
		}
		if b.arena != nil {
			b.arena.retire(b)
		}
	}
}

// ReleaseHandler adapts a Buffer to the wayland release event so the pool
// refills as the compositor returns buffers. The arena wires it once per
// buffer at allocation; test-created buffers without a wl object are
// released by hand.
type ReleaseHandler struct {
	B *Buffer
}

// HandleBufferRelease implements wl.BufferReleaseHandler.
func (h ReleaseHandler) HandleBufferRelease(wl.BufferReleaseEvent) {
	h.B.Release()
}
