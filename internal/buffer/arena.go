// Arena: one wl_shm pool per session. A single anonymous file backs the
// whole session's buffers; each buffer is a sub-allocation (a byte range)
// of the shared mapping. Growth goes through file truncate +
// wl_shm_pool.resize + remap, so a session holds exactly one fd and one
// mapping no matter how many windows resize how often - the memfd churn
// and per-buffer mappings this package used to leak are gone.
package buffer

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sync"

	wlos "github.com/neurlang/wayland/os"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/stubbedev/lys-lib/render"
	"golang.org/x/sys/unix"

	"github.com/stubbedev/lys-ui/internal/intconv"
)

// slot is a byte range in the arena's pool file. Buffer geometry (width,
// height, stride) is independent of the slot: any buffer that fits in a
// slot's bytes can live at its offset.
type slot struct {
	offset int
	bytes  int
}

// Arena is one session's shared wl_shm pool. It is not safe for
// concurrent use: like the rest of the buffer lifecycle it belongs to
// the event loop that draws the frames. A nil shm is allowed for
// wire-free tests; the arena then runs without the wl_shm pool proxy and
// hands out buffers with a nil WL.
type Arena struct {
	shm  *wl.Shm
	pool *wl.ShmPool // created with the file, resized on growth
	file *os.File    // the pool's only fd
	data []byte      // the whole pool mapped
	size int         // bytes the file, pool, and mapping agree on
	gen  int         // bumped every remap: outstanding Data windows re-point

	free []slot // free slots sorted by offset, adjacent ones merged
	live int    // sub-allocations handed out and not yet retired

	// lastFormat is the shm format the most recent wire buffer creation
	// advertised (buffer.Format); an inspection point for the format
	// pins - the create_buffer call must never drift to XRGB.
	lastFormat uint32

	closed bool
}

// arenas maps a session's wl_shm to its shared arena, so NewFile callers
// need no new plumbing: every buffer they request sub-allocates from the
// one pool their session owns.
var arenas = struct {
	sync.Mutex
	m map[*wl.Shm]*Arena
}{m: make(map[*wl.Shm]*Arena)}

// forShm returns the session's arena, creating it on first use.
func forShm(shm *wl.Shm) *Arena {
	arenas.Lock()
	defer arenas.Unlock()
	a, ok := arenas.m[shm]
	if !ok {
		a = NewArena(shm)
		arenas.m[shm] = a
	}
	return a
}

// CloseArenas tears down the arena bound to shm, if any: it destroys the
// pool proxy, unmaps, and closes the session's fd. Wire it to session
// close so a reconnecting client does not hold a dead pool.
func CloseArenas(shm *wl.Shm) {
	arenas.Lock()
	a, ok := arenas.m[shm]
	delete(arenas.m, shm)
	arenas.Unlock()
	if ok {
		_ = a.Close()
	}
}

// NewArena returns an empty arena over shm.
func NewArena(shm *wl.Shm) *Arena {
	return &Arena{shm: shm}
}

// Acquire sub-allocates a bufW x bufH buffer from the pool, growing the
// pool when no free slot fits. A fresh buffer is fully stale: reused
// slots carry whatever the last buffer at that offset drew, and the full
// staleness is what forces the repaint that overwrites it.
func (a *Arena) Acquire(bufW, bufH, scale int) (*Buffer, error) {
	if bufW <= 0 || bufH <= 0 {
		return nil, fmt.Errorf("buffer: invalid size %dx%d", bufW, bufH)
	}
	if scale < 1 {
		return nil, fmt.Errorf("buffer: invalid scale %d", scale)
	}
	if a.closed {
		return nil, errors.New("buffer: arena closed")
	}
	stride := render.Stride(bufW)
	need := stride * bufH
	if need > math.MaxInt32 {
		return nil, fmt.Errorf("buffer: size %d exceeds the wayland int32 range", need)
	}

	s, err := a.take(need)
	if err != nil {
		return nil, err
	}
	a.lastFormat = Format
	var wlBuf *wl.Buffer
	if a.pool != nil {
		wlBuf, err = a.pool.CreateBuffer(intconv.Int32(s.offset), intconv.Int32(bufW), intconv.Int32(bufH), intconv.Int32(stride), a.lastFormat)
		if err != nil {
			a.put(s)
			return nil, fmt.Errorf("buffer: wl_shm_pool.create_buffer: %w", err)
		}
	}
	a.live++
	b := &Buffer{
		WL:    wlBuf,
		Data:  a.data[s.offset : s.offset+need],
		Width: bufW, Height: bufH, Stride: stride, Scale: scale,
		Stale: render.Rect{W: bufW, H: bufH},
		arena: a,
		slot:  s,
		gen:   a.gen,
	}
	// The release event is what returns a dropped buffer's slot, so the
	// arena owns the wiring: once per buffer lifetime, here.
	if b.WL != nil {
		wlclient.BufferAddListener(b.WL, ReleaseHandler{B: b})
	}
	return b, nil
}

// take returns a free slot of at least need bytes: the tightest fit, so
// big freed slots stay available for big buffers. The found slot is
// split when it is strictly bigger, keeping the remainder free. Growing
// is the fallback: the pool file truncates to twice its size (or the
// first fit for need), the wl_shm_pool follows, and the mapping is
// redone - no fd is ever opened twice.
func (a *Arena) take(need int) (slot, error) {
	best := -1
	for i, s := range a.free {
		if s.bytes < need {
			continue
		}
		if best < 0 || s.bytes < a.free[best].bytes {
			best = i
		}
	}
	if best >= 0 {
		s := a.free[best]
		a.free = append(a.free[:best], a.free[best+1:]...)
		if s.bytes > need {
			a.put(slot{offset: s.offset + need, bytes: s.bytes - need})
			s.bytes = need
		}
		return s, nil
	}

	grow := 2 * a.size
	grow = max(grow, a.size+need)
	oldSize := a.size
	if err := a.ensure(grow); err != nil {
		return slot{}, err
	}
	s := slot{offset: oldSize, bytes: need}
	// The new space beyond the allocation stays in the free list; the
	// geometric headroom is only useful if it is reachable.
	a.put(slot{offset: s.offset + need, bytes: a.size - s.offset - need})
	return s, nil
}

// ensure grows the pool to size bytes: truncate the file, follow with
// wl_shm_pool.resize, then move the mapping. The compositor keeps its
// own mapping of the file, so the client-side remap never disturbs a
// buffer it is still reading.
func (a *Arena) ensure(size int) error {
	if size <= a.size {
		return nil
	}
	wireSize, err := intconv.CheckedInt32(size)
	if err != nil {
		return fmt.Errorf("buffer: pool size %d exceeds the wayland int32 range: %w", size, err)
	}
	if a.file == nil {
		fd, err := unix.MemfdCreate("lys-ui-shm", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
		if err != nil {
			return fmt.Errorf("buffer: memfd: %w", err)
		}
		a.file = os.NewFile(uintptr(fd), "lys-ui-shm")
		if err := a.file.Truncate(int64(size)); err != nil {
			return fmt.Errorf("buffer: size the memfd to %d bytes: %w", size, err)
		}
	} else if err := a.file.Truncate(int64(size)); err != nil {
		return fmt.Errorf("buffer: truncate: %w", err)
	}
	data, err := wlos.Mmap(int(a.file.Fd()), 0, size, wlos.ProtRead|wlos.ProtWrite, wlos.MapShared)
	if err != nil {
		return fmt.Errorf("buffer: mmap: %w", err)
	}
	if a.shm != nil {
		if a.pool == nil {
			pool, err := a.shm.CreatePool(a.file.Fd(), wireSize)
			if err != nil {
				_ = wlos.Munmap(data)
				return fmt.Errorf("buffer: wl_shm.create_pool: %w", err)
			}
			a.pool = pool
		} else if err := a.pool.Resize(wireSize); err != nil {
			_ = wlos.Munmap(data)
			return fmt.Errorf("buffer: wl_shm_pool.resize: %w", err)
		}
	}
	if a.data != nil {
		_ = wlos.Munmap(a.data)
		a.gen++ // outstanding Data windows now hang off the old mapping
	}
	a.data = data
	a.size = size
	return nil
}

// put returns a slot to the free list, keeping it sorted by offset and
// merging adjacent slots so fragmentation cannot accumulate.
func (a *Arena) put(s slot) {
	if s.bytes <= 0 {
		return
	}
	i := 0
	for i < len(a.free) && a.free[i].offset < s.offset {
		i++
	}
	a.free = append(a.free, slot{})
	copy(a.free[i+1:], a.free[i:])
	a.free[i] = s
	// Merge left, then right, while the neighbors touch.
	if i > 0 && a.free[i-1].offset+a.free[i-1].bytes == s.offset {
		a.free[i-1].bytes += a.free[i].bytes
		a.free = append(a.free[:i], a.free[i+1:]...)
		i--
	}
	if i+1 < len(a.free) && a.free[i].offset+a.free[i].bytes == a.free[i+1].offset {
		a.free[i].bytes += a.free[i+1].bytes
		a.free = append(a.free[:i+1], a.free[i+2:]...)
	}
}

// retire returns a buffer's slot to the free list. Called when the
// compositor released a freed buffer (Buffer.Release) or when its owner
// dropped it while already free.
func (a *Arena) retire(b *Buffer) {
	if a == nil || a.closed {
		b.arena = nil
		return
	}
	if b.WL != nil {
		_ = b.WL.Destroy()
		b.WL = nil
	}
	a.put(b.slot)
	a.live--
	b.arena = nil
}

// abandon gives up a slot whose wl_buffer was destroyed while the
// compositor still held it: no release will ever land, so the slot must
// not be reused; the space stays reserved until the arena closes. Only
// the retired-overflow cap takes this path, so the loss is bounded by
// the pool capacity.
func (a *Arena) abandon(b *Buffer) {
	if a == nil || a.closed {
		b.arena = nil
		return
	}
	a.live--
	b.arena = nil
}

// Live reports how many sub-allocations are handed out and not yet
// retired. Zero means every slot is back in the free list.
func (a *Arena) Live() int { return a.live }

// Size reports the pool's current byte size: the high-water mark of the
// session's buffer demand.
func (a *Arena) Size() int { return a.size }

// Close destroys the pool proxy, unmaps, and closes the fd. Buffers
// still referencing the arena are detached; their slots are gone with
// the mapping.
func (a *Arena) Close() error {
	a.closed = true
	var err error
	if a.pool != nil {
		err = a.pool.Destroy()
		a.pool = nil
	}
	if a.data != nil {
		_ = wlos.Munmap(a.data)
		a.data = nil
	}
	if a.file != nil {
		if cerr := a.file.Close(); err == nil {
			err = cerr
		}
		a.file = nil
	}
	a.free = nil
	a.size = 0
	a.live = 0
	return err
}
