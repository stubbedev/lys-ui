package buffer

import (
	"errors"
	"slices"
	"testing"

	"github.com/stubbedev/lys-lib/render"
)

func newTestBuffer(w, h int) *Buffer {
	stride := render.Stride(w)
	return &Buffer{
		Data:   make([]byte, stride*h),
		Width:  w,
		Height: h,
		Stride: stride,
		Scale:  1,
		Stale:  render.Rect{W: w, H: h},
	}
}

// arenaBuffer builds a pool create closure over one arena.
func arenaBuffer(a *Arena, w, h int) func() (*Buffer, error) {
	return func() (*Buffer, error) { return a.Acquire(w, h, 1) }
}

func countingCreate(counter *int, w, h int) func() (*Buffer, error) {
	return func() (*Buffer, error) {
		*counter++
		return newTestBuffer(w, h), nil
	}
}

func TestAcquireAllocatesUpToCapacity(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 20), 3)

	for i := range 3 {
		b, err := p.Acquire()
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		if !b.Busy() {
			t.Error("acquired buffer must be busy")
		}
	}

	t.Run("capacity exhausted yields ErrBusy, not a busy buffer", func(t *testing.T) {
		b, err := p.Acquire()
		if !errors.Is(err, ErrBusy) {
			t.Errorf("want ErrBusy, got %v", err)
		}
		if b != nil {
			t.Error("ErrBusy must not return a buffer")
		}
	})
}

func TestReleasedBufferIsReusedBeforeAllocating(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 20), 3)

	first, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	first.Release()

	second, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Errorf("released buffer must be reused, created %d buffers", created)
	}
	if first != second {
		t.Error("release then acquire must return the same buffer")
	}
}

// A buffer handed out stays the compositor's until its release: a
// reused buffer is as busy as a fresh one, so the next Acquire never
// returns the buffer on screen (#220).
func TestAcquireNeverHandsOutAHeldBuffer(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 20), 3)
	first, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	first.Release()
	reused, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if reused != first || !reused.Busy() {
		t.Fatalf("reused buffer busy = %v, want the released buffer marked busy again", reused.Busy())
	}
	next, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if next == reused {
		t.Error("Acquire handed out the buffer the compositor still holds")
	}
}

func TestCreateErrorIsReturnedVerbatim(t *testing.T) {
	want := errors.New("shm pool exploded")
	p := New(func() (*Buffer, error) { return nil, want }, 2)

	b, err := p.Acquire()
	if !errors.Is(err, want) {
		t.Errorf("want the create error, got %v", err)
	}
	if b != nil {
		t.Error("failed create must not return a buffer")
	}
}

func TestResizeRebuildsAtNewSize(t *testing.T) {
	created := 0
	create := countingCreate(&created, 100, 20)
	p := New(create, 3)

	a, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	a.Release()

	old := map[*Buffer]bool{a: true, b: true}

	p.Resize(countingCreate(&created, 200, 40))

	got, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if old[got] {
		t.Error("resize must not hand out a pre-resize buffer")
	}
	if created != 3 {
		t.Errorf("resize must allocate through the new create, created %d", created)
	}
	if got.Width != 200 || got.Height != 40 {
		t.Errorf("resized buffer has size %dx%d, want 200x40", got.Width, got.Height)
	}
	// a was released and is disposed; b was still held by the
	// compositor: dropped from rotation, but kept alive for its release
	// event in the retired list.
	if !a.freed {
		t.Error("released buffer must be disposed by resize")
	}
	if b.Data == nil {
		t.Error("held buffer must keep its storage until the compositor releases it")
	}
	if len(p.retired) != 1 || p.retired[0] != b {
		t.Errorf("held buffer must wait in the retired list, got %d entries", len(p.retired))
	}
}

// TestResizePreservesHandedOutBuffers pins the resize handshake: a
// buffer the compositor still holds keeps its identity (and its arena
// slot) across the resize, and its release - not the resize - is what
// returns the storage.
func TestResizePreservesHandedOutBuffers(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	defer arena.Close()

	p := New(arenaBuffer(arena, 100, 20), 3)

	held, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if arena.Live() != 1 {
		t.Fatalf("arena live = %d, want 1", arena.Live())
	}

	p.Resize(arenaBuffer(arena, 200, 40))
	if held.Data == nil {
		t.Fatal("resize must preserve a handed-out buffer's storage")
	}
	if arena.Live() != 1 {
		t.Errorf("arena live = %d, want 1 while the compositor holds the buffer", arena.Live())
	}
	if got, err := p.Acquire(); err != nil || got == held {
		t.Fatalf("acquire after resize = %v, %v; want a fresh buffer", got, err)
	}

	// The compositor's release lands after the resize: the buffer
	// retires out of the retired list and its slot rejoins the arena.
	held.Release()
	if arena.Live() != 1 {
		t.Errorf("arena live = %d, want 1 (only the post-resize buffer)", arena.Live())
	}
	if len(p.retired) != 0 {
		t.Errorf("released buffer must leave the retired list, %d left", len(p.retired))
	}
	if p.Pending() != 1 {
		t.Errorf("pending = %d, want 1", p.Pending())
	}
}

// TestRetiredCapDropsOldest pins the pending cap: resize storms must
// not stack unbounded held buffers. Beyond the capacity the oldest
// frame's buffer is dropped instead of allocating more pending slots.
func TestRetiredCapDropsOldest(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 20), 2)

	var generations [][]*Buffer
	for range 4 {
		var gen []*Buffer
		for range 2 { // fill the pool: every buffer busy
			b, err := p.Acquire()
			if err != nil {
				t.Fatal(err)
			}
			gen = append(gen, b)
		}
		generations = append(generations, gen)
		p.Resize(countingCreate(&created, 100, 20)) // all busy -> all preserved
		if len(p.retired) > p.capacity {
			t.Fatalf("retired = %d, must stay capped at %d", len(p.retired), p.capacity)
		}
	}
	// Four resize rounds stacked three generations beyond the cap of 2:
	// the oldest buffers must be gone, the newest kept.
	if len(p.retired) != p.capacity {
		t.Fatalf("retired = %d, want %d after cap", len(p.retired), p.capacity)
	}
	var dropped []*Buffer
	dropped = append(dropped, generations[0]...)
	dropped = append(dropped, generations[1]...)
	dropped = append(dropped, generations[2]...)
	for _, b := range dropped {
		for _, r := range p.retired {
			if r == b {
				t.Error("cap must drop the oldest pending buffers first")
			}
		}
	}
	for _, r := range p.retired {
		if r != generations[3][0] && r != generations[3][1] {
			t.Error("cap must keep the newest pending buffers")
		}
	}
	// Dropped buffers are dead: releasing them must not resurrect or
	// double-return anything.
	generations[0][0].Release()
	if len(p.retired) != p.capacity {
		t.Errorf("dropped buffer release must not touch the pool, retired = %d", len(p.retired))
	}
}

// TestPoolNeverGrowsPastCapacity pins the unbounded-growth guard: while
// the compositor holds every buffer, Acquire reports ErrBusy instead of
// allocating pending buffers beyond the cap.
func TestPoolNeverGrowsPastCapacity(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 20), 3)

	for range 3 {
		if _, err := p.Acquire(); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 50 {
		b, err := p.Acquire()
		if !errors.Is(err, ErrBusy) {
			t.Fatalf("acquire %d: want ErrBusy with every buffer pending, got %v", i, err)
		}
		if b != nil {
			t.Fatal("ErrBusy must not return a buffer")
		}
	}
	if created != 3 || p.Pending() != 3 {
		t.Errorf("pending buffers stacked: created %d, pending %d, want 3/3", created, p.Pending())
	}
}

// TestPoolCloseReturnsSlots pins teardown: a closed pool hands out
// nothing, and released buffers are all gone from tracking.
func TestPoolCloseReturnsSlots(t *testing.T) {
	p := New(countingCreate(new(int), 100, 20), 3)
	a, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	a.Release() // a free, b held

	p.Close()
	if p.Pending() != 1 {
		t.Errorf("pending = %d, want the one held buffer", p.Pending())
	}
	if _, err := p.Acquire(); !errors.Is(err, ErrClosed) {
		t.Errorf("acquire on closed pool = %v, want ErrClosed", err)
	}
	b.Release()
	if p.Pending() != 0 {
		t.Errorf("pending = %d after release, want 0", p.Pending())
	}
}

// TestRetiredSweepFreesSlotsForReuse checks that released pre-resize
// buffers retire at the next acquire, so their slots serve the new size
// instead of the arena growing.
func TestRetiredSweepFreesSlotsForReuse(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	arena := NewArena(nil)
	defer arena.Close()

	created := 0
	size := 100
	p := New(func() (*Buffer, error) {
		created++
		return arena.Acquire(size, 20, 1)
	}, 3)

	for range 3 { // fill the pool at 100x20
		if _, err := p.Acquire(); err != nil {
			t.Fatal(err)
		}
	}
	first := arena.Size()
	size = 60 // shrink: old slots are big enough to be reused
	p.Resize(func() (*Buffer, error) {
		created++
		return arena.Acquire(size, 20, 1)
	})
	for _, b := range slices.Clone(p.retired) {
		b.Release() // compositor returns everything: slots retire eagerly
	}
	for range 3 {
		if _, err := p.Acquire(); err != nil {
			t.Fatal(err)
		}
	}
	if got := arena.Size(); got != first {
		t.Errorf("arena grew to %d bytes on shrink-and-reuse, want %d (freed slots must be reused)", got, first)
	}
	if arena.Live() != 3 {
		t.Errorf("arena live = %d, want 3", arena.Live())
	}
	if created != 6 {
		t.Errorf("created %d buffers, want 6 (two sizes x 3 pool slots)", created)
	}
}

func TestStride(t *testing.T) {
	if got := render.Stride(800); got != 3200 {
		t.Errorf("Stride(800) = %d, want 3200", got)
	}
	if got := render.Stride(0); got != 0 {
		t.Errorf("Stride(0) = %d, want 0", got)
	}
}

func TestFreshBufferIsFullyStale(t *testing.T) {
	b := newTestBuffer(100, 40)
	if b.Stale != (render.Rect{W: 100, H: 40}) {
		t.Errorf("fresh buffer stale = %+v, want the full buffer", b.Stale)
	}
}

func TestPresentedMovesDamageToOtherBuffers(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 40), 3)

	a, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if a.Stale != (render.Rect{W: 100, H: 40}) {
		t.Fatalf("fresh buffer stale = %+v, want full", a.Stale)
	}
	// Committing a frame through a fully stale buffer changes only the
	// changed region; the initialization paint must not leak into other
	// buffers' staleness.
	p.Presented(a, render.Rect{X: 10, Y: 0, W: 20, H: 40})
	if !a.Stale.Empty() {
		t.Errorf("presented buffer stale = %+v, want empty", a.Stale)
	}

	b, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	p.Presented(b, render.Rect{X: 60, Y: 0, W: 10, H: 40})

	// Re-acquiring a now carries the change missed while b was on
	// screen; the first region does not apply, a was the screen then.
	a.Release()
	a2, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if a2 != a {
		t.Fatal("released buffer was not reused")
	}
	want := render.Rect{X: 60, Y: 0, W: 10, H: 40}
	if a2.Stale != want {
		t.Errorf("rotated buffer stale = %+v, want the missed region %+v", a2.Stale, want)
	}

	// Re-presenting clears only the presented buffer.
	p.Presented(a2, want)
	if !a2.Stale.Empty() {
		t.Errorf("presented buffer stale = %+v, want empty", a2.Stale)
	}
	if b.Stale != want {
		t.Errorf("other buffer stale = %+v, want %+v", b.Stale, want)
	}
}

func TestPresentedEmptyRegionIsNoop(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 50, 20), 2)
	a, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	a.Stale = render.Rect{}
	p.Presented(a, render.Rect{})
	if !a.Stale.Empty() {
		t.Error("empty present must not touch staleness")
	}
}

func TestNewFileRejectsBadGeometry(t *testing.T) {
	t.Run("zero and negative sizes", func(t *testing.T) {
		if _, err := NewFile(nil, 0, 10, 1); err == nil {
			t.Error("zero width must error before touching shm")
		}
		if _, err := NewFile(nil, 10, -5, 1); err == nil {
			t.Error("negative height must error before touching shm")
		}
	})

	t.Run("scale below one", func(t *testing.T) {
		if _, err := NewFile(nil, 10, 10, 0); err == nil {
			t.Error("scale 0 must error before touching shm")
		}
	})
}
