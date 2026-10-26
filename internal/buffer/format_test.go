// The buffer-format pin: every shm buffer is ARGB8888 premultiplied,
// advertised as wl.ShmFormatArgb8888 on wl_shm_pool.create_buffer, and
// laid out little-endian (bytes B, G, R, A in the mapping). XRGB would
// discard the alpha channel translucent surfaces exist for, and the
// byte order is the aliasing class of bug the compositor sees directly.
// All wire-free: a nil-shm arena never builds the pool proxy but still
// records the format the wire call would carry.
package buffer

import (
	"testing"

	"github.com/stubbedev/lys-lib/render"
	"github.com/stubbedev/lys-lib/third_party/wayland/wl"
)

// TestFormatIsArgb8888NotXrgb pins the format constant: flipping it to
// XRGB (or anything else) fails here, because translucent backgrounds
// need the alpha channel to reach the compositor's blend.
func TestFormatIsArgb8888NotXrgb(t *testing.T) {
	if Format != wl.ShmFormatArgb8888 {
		t.Fatalf("buffer.Format = %d, want wl.ShmFormatArgb8888 (%d): XRGB discards alpha and breaks every translucent surface",
			Format, wl.ShmFormatArgb8888)
	}
	if Format == wl.ShmFormatXrgb8888 {
		t.Fatal("buffer.Format is XRGB")
	}
}

// TestAcquireAdvertisesArgbToThePool pins the value that flows into
// wl_shm_pool.create_buffer: it must be the ARGB8888 constant, not a
// hardcoded XRGB that crept back into the call site.
func TestAcquireAdvertisesArgbToThePool(t *testing.T) {
	a := NewArena(nil) // wire-free: no pool proxy, the format is still recorded
	defer func() { _ = a.Close() }()

	b, err := a.Acquire(8, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()

	if a.lastFormat != wl.ShmFormatArgb8888 {
		t.Fatalf("create_buffer format = %d, want wl.ShmFormatArgb8888 (%d)", a.lastFormat, wl.ShmFormatArgb8888)
	}
	if a.lastFormat == wl.ShmFormatXrgb8888 {
		t.Fatal("create_buffer advertises XRGB")
	}
}

// TestByteLayoutIsLittleEndianArgb pins the in-memory byte order the
// compositor reads through its own mapping: wl_shm ARGB8888 is
// little-endian, so the pixel 0xAARRGGBB sits as bytes B, G, R, A.
func TestByteLayoutIsLittleEndianArgb(t *testing.T) {
	a := NewArena(nil)
	defer func() { _ = a.Close() }()

	const w, h = 4, 3
	b, err := a.Acquire(w, h, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()

	cv := render.New(b.Data, b.Stride, w, h)
	cv.ClearDevice(cv.Rect(), render.RGB(0x11, 0x22, 0x33))
	// render.RGB(0x11, 0x22, 0x33) is 0xFF112233 (red is the low
	// channel pair's first byte); ClearDevice stores it little-endian,
	// so the mapping must read back as 0x33, 0x22, 0x11, 0xFF -
	// B, G, R, A.
	want := [4]byte{0x33, 0x22, 0x11, 0xFF}
	for y := range h {
		for x := range w {
			o := y*b.Stride + x*4
			var got [4]byte
			copy(got[:], b.Data[o:o+4])
			if got != want {
				t.Fatalf("pixel (%d,%d) bytes = %v, want %v (little-endian B,G,R,A)", x, y, got, want)
			}
		}
	}
}
