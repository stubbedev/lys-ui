// Arena coverage: one shm pool per session, growth by truncate + remap,
// and the fd/mapping accounting that keeps weeks-uptime panels from
// leaking. The fd tests run the real allocation path - a real anonymous
// file, real mmap - with a nil shm, so no compositor is involved.
package buffer

import (
	"os"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

// countFDs reports how many fds this process holds. /proc/self/fd
// includes the listing directory's own fd, but that +1 is constant, so
// before/after comparisons are exact.
func countFDs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Skipf("no /proc/self/fd on this system: %v", err)
	}
	return len(entries)
}

// TestCreateResizeCloseWindowsReturnsFds is the leak headline: N
// window-shaped pools rotate, resize up and down over one shared arena,
// and close - the open fd count must return to the baseline, with every
// slot accounted for. A per-buffer memfd scheme (or a pool whose
// resizes orphan storage) shows up here immediately.
func TestCreateResizeCloseWindowsReturnsFds(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	before := countFDs(t)

	arena := NewArena(nil) // one per session, in production
	sizes := [][2]int{{200, 40}, {320, 60}, {180, 30}, {480, 90}, {320, 60}}

	const windows = 4
	for range windows {
		p := New(arenaBuffer(arena, sizes[0][0], sizes[0][1]), 3)
		for _, sz := range sizes {
			// Resizes rebuild the pool exactly like the draw loop
			// does: free buffers disposed, held ones preserved.
			p.Resize(arenaBuffer(arena, sz[0], sz[1]))
			var held []*Buffer
			for range 3 { // fill the rotation: every buffer busy
				b, err := p.Acquire()
				if err != nil {
					t.Fatalf("acquire %dx%d: %v", sz[0], sz[1], err)
				}
				held = append(held, b)
			}
			// The compositor returns everything; freed slots must
			// serve the next size instead of growing the pool.
			for _, b := range held {
				b.Release()
			}
		}
		p.Close()
		if p.Pending() != 0 {
			t.Fatalf("window closed with %d pending buffers", p.Pending())
		}
	}

	if live := arena.Live(); live != 0 {
		t.Errorf("arena live = %d after all windows closed, want 0", live)
	}
	// The session lives on: exactly one pool fd may remain, no matter
	// how many windows, buffers, and resizes just ran.
	if n := countFDs(t); n != before+1 {
		t.Errorf("open fds = %d, want %d (baseline+1, the one pool fd): the window lifecycle leaks fds", n, before+1)
	}

	// Session teardown: the pool proxy, mapping, and the one fd go.
	if err := arena.Close(); err != nil {
		t.Fatalf("arena close: %v", err)
	}
	if n := countFDs(t); n != before {
		t.Errorf("open fds = %d after arena close, want baseline %d", n, before)
	}
	if arena.Size() != 0 || len(arena.free) != 0 {
		t.Errorf("closed arena kept size %d and %d free slots", arena.Size(), len(arena.free))
	}
}

// TestNewFileRoutesThroughSessionPool pins the kept-API contract: every
// NewFile call sub-allocates from one arena bound to the wl_shm, no
// matter how many callers, windows, or resizes happen - and one-shot
// buffers give their slots back. (With a nil shm the arena runs
// wire-free; on the pre-pool code this same call panicked in
// create_pool, which is exactly the churn the pool removed.)
func TestNewFileRoutesThroughSessionPool(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	before := countFDs(t)

	var oneShots []*Buffer
	for range 8 { // eight windows' worth of buffers, one pool
		b, err := NewFile(nil, 64, 16, 1)
		if err != nil {
			t.Fatal(err)
		}
		oneShots = append(oneShots, b)
	}
	if got := countFDs(t); got != before+1 {
		t.Errorf("open fds = %d, want baseline+1 (one arena fd, got per-buffer memfds)", got)
	}
	arenas.Lock()
	a := arenas.m[nil]
	arenas.Unlock()
	if a == nil {
		t.Fatal("NewFile did not register a session arena")
	}
	if a.Size() == 0 || a.Live() != 8 {
		t.Errorf("arena size %d live %d, want the buffers sub-allocated", a.Size(), a.Live())
	}

	// One-shot retirement: the buffers are pool-less, so the compositor's
	// release - not any caller bookkeeping - is what returns their slots.
	for _, b := range oneShots {
		b.Release()
	}
	if a.Live() != 0 {
		t.Errorf("arena live = %d after all one-shots released, want 0", a.Live())
	}
	if n := countFDs(t); n != before+1 {
		t.Errorf("open fds = %d after frees, want baseline+1: freeing must not churn fds", n)
	}
	CloseArenas(nil)
	if n := countFDs(t); n != before {
		t.Errorf("open fds = %d after CloseArenas, want baseline %d", n, before)
	}
}

// TestArenaGrowthReusesFreedSlots: release-then-acquire at the same size
// must recycle slots, so steady-state frames never grow the pool.
func TestArenaGrowthReusesFreedSlots(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	defer arena.Close()

	var live []*Buffer
	for range 3 {
		b, err := arena.Acquire(100, 50, 1)
		if err != nil {
			t.Fatal(err)
		}
		live = append(live, b)
	}
	size := arena.Size()
	for _, b := range live {
		b.Release()
	}
	if arena.Live() != 0 {
		t.Fatalf("live = %d after releasing everything", arena.Live())
	}
	for range 20 { // steady-state frames at the same size
		b, err := arena.Acquire(100, 50, 1)
		if err != nil {
			t.Fatal(err)
		}
		b.Release()
	}
	if got := arena.Size(); got != size {
		t.Errorf("steady-state frames grew the arena to %d, want %d", got, size)
	}
}

// TestArenaGrowKeepsOneFdAndWritableMemory: growth truncates the file
// and remaps; the fd count stays at one and every generation's mapping
// is usable.
func TestArenaGrowKeepsOneFdAndWritableMemory(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	defer arena.Close()

	baseline := countFDs(t)
	var keep []*Buffer
	for i, sz := range []int{1, 4, 16, 64, 256} {
		b, err := arena.Acquire(sz*1024, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		for j := range b.Data {
			b.Data[j] = byte(i) // both mappings must be writable
		}
		if b.Data[len(b.Data)-1] != byte(i) {
			t.Fatalf("generation %d mapping not readable", i)
		}
		keep = append(keep, b)
	}
	if n := countFDs(t); n != baseline+1 {
		t.Errorf("open fds = %d after 5 growths, want baseline+1", n)
	}
	for _, b := range keep {
		b.Release()
	}
	if arena.Live() != 0 {
		t.Errorf("live = %d, want 0", arena.Live())
	}
}

// TestPoolReuseAcrossGrowthKeepsDataValid is the resize-under-load
// regression: a pool rotating through frames while the arena grows must
// hand out buffers whose views point at the CURRENT mapping - a resize
// storm that reuses a pre-remap view writes into unmapped memory.
func TestPoolReuseAcrossGrowthKeepsDataValid(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	defer arena.Close()

	var held []*Buffer
	w, h := 240, 80
	p := New(func() (*Buffer, error) { return arena.Acquire(w, h, 1) }, 3)
	releaseExceptLast2 := func() {
		for len(held) > 2 {
			held[0].Release()
			held = held[1:]
		}
	}
	for _, size := range [][2]int{{240, 80}, {300, 90}, {180, 60}, {300, 90}} {
		w, h = size[0], size[1]
		p.Resize(func() (*Buffer, error) { return arena.Acquire(w, h, 1) })
		for i := range 4 {
			b, err := p.Acquire()
			if err != nil {
				t.Fatalf("size %dx%d acquire %d: %v", size[0], size[1], i, err)
			}
			if b.slot.offset+len(b.Data) > arena.Size() {
				t.Fatalf("slot %+v runs past the pool size %d", b.slot, arena.Size())
			}
			for j := range b.Data {
				b.Data[j] = 0x80 // faults if the view is stale
			}
			held = append(held, b)
			releaseExceptLast2()
		}
	}
}

// TestRetireMergesAdjacentSlots: freeing neighbors must recombine into
// one slot, so shrink-then-grow cycles hit the free list instead of
// growing the pool again.
func TestRetireMergesAdjacentSlots(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	defer arena.Close()

	var bufs []*Buffer
	for range 4 {
		b, err := arena.Acquire(32, 8, 1) // 128 bytes each, packed tight
		if err != nil {
			t.Fatal(err)
		}
		bufs = append(bufs, b)
	}
	// Free the middle two, then the outer two: every merge case fires.
	bufs[1].Release()
	bufs[2].Release()
	if len(arena.free) != 1 {
		t.Fatalf("free slots = %d after freeing the middle pair, want 1 merged slot", len(arena.free))
	}
	bufs[0].Release()
	if len(arena.free) != 1 {
		t.Fatalf("free slots = %d after freeing the left neighbor, want 1 (triple merge)", len(arena.free))
	}
	bufs[3].Release()
	if len(arena.free) != 1 || arena.free[0].bytes != arena.size {
		t.Fatalf("free = %+v, want one slot covering all %d bytes", arena.free, arena.size)
	}

	// The merged slot serves a buffer larger than any single old slot.
	big, err := arena.Acquire(arena.size/4/4, 4, 1) // (size/16)*4*4 = size/4 bytes
	if err != nil {
		t.Fatalf("merged slot could not serve a big buffer: %v", err)
	}
	big.Release()
}

// TestArenaAcquireAfterCloseErrors guards use-after-teardown.
func TestArenaAcquireAfterCloseErrors(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	if err := arena.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := arena.Acquire(10, 10, 1); err == nil {
		t.Error("acquire on a closed arena must error")
	}
	// Retiring a buffer of a closed arena must be a safe no-op.
	detached := NewArena(nil)
	defer detached.Close()
	b, err := detached.Acquire(10, 10, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Release()
	b.Release() // double release: a safe no-op
}

// TestCancelPinsTheOneShotContract: a buffer whose commit never went
// out is reclaimed immediately, since no release can arrive; pool
// buffers rotate instead, and arena-less buffers are safe no-ops.
func TestCancelPinsTheOneShotContract(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	defer arena.Close()

	uncommitted, err := arena.Acquire(16, 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	Cancel(uncommitted) // never committed: no release can be expected
	if arena.Live() != 0 {
		t.Errorf("live = %d after Cancel, want 0", arena.Live())
	}

	// Cancel on a wire-free (arena-less) buffer is a no-op, not a panic.
	Cancel(&Buffer{})

	// Double cancels are no-ops.
	again, err := arena.Acquire(16, 8, 1)
	if err != nil {
		t.Fatal(err)
	}
	Cancel(again)
	Cancel(again)
	if arena.Live() != 0 {
		t.Errorf("live = %d after double Cancel, want 0", arena.Live())
	}

	// A pool-adopted buffer released after a free rotates through its
	// pool first and only retires when the pool drops it.
	p := New(arenaBuffer(arena, 16, 8), 1)
	b, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	b.Release()
	if arena.Live() != 1 {
		t.Errorf("live = %d after a pool buffer's release, want 1 (it stays in rotation)", arena.Live())
	}
	p.Close()
	b.Release()
	if arena.Live() != 0 {
		t.Errorf("live = %d after close and release, want 0", arena.Live())
	}
}

// TestCloseArenasDetachesSessionPool: session close removes the
// registry entry, so a later NewFile starts a fresh arena.
func TestCloseArenasDetachesSessionPool(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if _, err := NewFile(nil, 8, 8, 1); err != nil {
		t.Fatal(err)
	}
	CloseArenas(nil)
	arenas.Lock()
	_, ok := arenas.m[nil]
	arenas.Unlock()
	if ok {
		t.Error("CloseArenas must remove the registry entry")
	}
	CloseArenas(nil) // idempotent
}

// TestTakeSplitsAndErrors pins the slot math directly: oversized slots
// split, and growth beyond the wayland int32 range is refused.
func TestTakeSplitsAndErrors(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	a := &Arena{} // size 0: the first take goes through a real file
	s, err := a.take(100)
	if err != nil {
		t.Fatal(err)
	}
	if s.offset != 0 || s.bytes != 100 {
		t.Errorf("first take = %+v, want {0 100}", s)
	}
	if err := a.ensure(1 << 31); err == nil {
		t.Error("growth past the wayland int32 range must error")
	}
}

// CopyRect reads a buffer that sat out an arena growth through the
// current mapping, not the one the growth unmapped (#395).
func TestCopyRectAfterArenaGrowth(t *testing.T) {
	a := NewArena(nil)
	t.Cleanup(func() { _ = a.Close() })
	front, err := a.Acquire(4, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range front.Data {
		front.Data[i] = 9
	}
	big, err := a.Acquire(2048, 2048, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = big
	dst, err := a.Acquire(4, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	CopyRect(dst, front, render.Rect{W: 4, H: 3})
	if dst.Data[0] != 9 || dst.Data[len(dst.Data)-1] != 9 {
		t.Errorf("copied %d..%d, want the front's 9s through the grown mapping", dst.Data[0], dst.Data[len(dst.Data)-1])
	}
}
