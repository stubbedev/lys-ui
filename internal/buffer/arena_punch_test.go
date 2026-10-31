package buffer

import (
	"testing"

	"golang.org/x/sys/unix"
)

func allocatedPages(t *testing.T, a *Arena) int {
	t.Helper()
	var st unix.Stat_t
	if err := unix.Fstat(int(a.file.Fd()), &st); err != nil {
		t.Fatalf("fstat pool: %v", err)
	}
	return int(st.Blocks*512) / unix.Getpagesize()
}

func filled(t *testing.T, a *Arena, w, h int) *Buffer {
	t.Helper()
	b, err := a.Acquire(w, h, 1)
	if err != nil {
		t.Fatalf("acquire %dx%d: %v", w, h, err)
	}
	for i := range b.Data {
		b.Data[i] = 0xff
	}
	return b
}

func TestRetirePunchesFreedSlot(t *testing.T) {
	a := NewArena(nil)
	t.Cleanup(func() { _ = a.Close() })
	w := unix.Getpagesize() / 4
	b := filled(t, a, w, 8)
	if got := allocatedPages(t, a); got != 8 {
		t.Fatalf("allocated pages = %d with the buffer drawn, want 8", got)
	}
	Cancel(b)
	if got := allocatedPages(t, a); got != 0 {
		t.Errorf("allocated pages = %d after retire, want 0: the freed slot stays resident", got)
	}
}

func TestReusedPunchedSlotIsZeroAndUsable(t *testing.T) {
	a := NewArena(nil)
	t.Cleanup(func() { _ = a.Close() })
	w := unix.Getpagesize() / 4
	Cancel(filled(t, a, w, 8))
	b, err := a.Acquire(w, 8, 1)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if b.slot.offset != 0 || a.Size() != 8*unix.Getpagesize() {
		t.Fatalf("reacquire took offset %d with pool size %d, want the punched slot reused", b.slot.offset, a.Size())
	}
	for i, v := range b.Data {
		if v != 0 {
			t.Fatalf("reused slot byte %d = %#x, want 0", i, v)
		}
	}
	for i := range b.Data {
		b.Data[i] = byte(i)
	}
	for i, v := range b.Data {
		if v != byte(i) {
			t.Fatalf("reused slot byte %d = %#x after write, want %#x", i, v, byte(i))
		}
	}
	if got := allocatedPages(t, a); got != 8 {
		t.Errorf("allocated pages = %d after redraw, want 8", got)
	}
}

func TestRetireFreesEveryPageOfASlotBetweenLiveNeighbors(t *testing.T) {
	a := NewArena(nil)
	t.Cleanup(func() { _ = a.Close() })
	w := 3 * unix.Getpagesize() / 8
	first := filled(t, a, w, 1)
	second := filled(t, a, w, 1)
	third := filled(t, a, w, 1)
	drawn := allocatedPages(t, a)
	Cancel(second)
	if got := allocatedPages(t, a); got != drawn-2 {
		t.Errorf("allocated pages = %d after freeing a 1.5-page slot between live ones, want %d: a page it shares with a neighbor stays resident", got, drawn-2)
	}
	for _, b := range []*Buffer{first, third} {
		for i, v := range a.data[b.slot.offset : b.slot.offset+len(b.Data)] {
			if v != 0xff {
				t.Fatalf("live buffer at %d: byte %d = %#x after its neighbor was freed", b.slot.offset, i, v)
			}
		}
	}
	Cancel(first)
	Cancel(third)
	if got := allocatedPages(t, a); got != 0 {
		t.Errorf("allocated pages = %d with every slot free, want 0", got)
	}
}
