package app

import (
	"testing"

	"github.com/stubbedev/lys-lib/fourcc"

	"github.com/stubbedev/lys-ui/media"
)

func numberedFrame(w, h int) *media.YUVFrame {
	f := &media.YUVFrame{Width: w, Height: h, Pix: make([]byte, media.YUVFrameBytes(w, h))}
	for i := range f.Pix {
		f.Pix[i] = byte(1 + i%250)
	}
	return f
}

// A buffer holds each plane at its aligned stride, chroma halved and
// rounded up; writing a frame fills exactly the planes' samples and
// leaves every padding byte as it was.
func TestVideoLayoutsHoldTheFramesPlanesAndNothingElse(t *testing.T) {
	for _, c := range []struct {
		format       fourcc.Format
		w, h         int
		planes       []videoPlane
		size, filled int
	}{
		{fourcc.YUV420, 32, 16, []videoPlane{{0, 256}, {256 * 16, 256}, {256*16 + 256*8, 256}}, 256*16 + 2*256*8, 32 * 16 * 3 / 2},
		{fourcc.NV12, 32, 16, []videoPlane{{0, 256}, {256 * 16, 256}}, 256*16 + 256*8, 32 * 16 * 3 / 2},
		{fourcc.YUV420, 301, 3, []videoPlane{{0, 512}, {512 * 3, 256}, {512*3 + 256*2, 256}}, 512*3 + 2*256*2, 301*3 + 2*151*2},
		{fourcc.NV12, 301, 3, []videoPlane{{0, 512}, {512 * 3, 512}}, 512*3 + 512*2, 301*3 + 2*151*2},
	} {
		l := newVideoLayout(c.format, c.w, c.h)
		if l.size != c.size || len(l.planes) != len(c.planes) {
			t.Fatalf("%s %dx%d: %d planes over %d bytes, want %d over %d", c.format, c.w, c.h, len(l.planes), l.size, len(c.planes), c.size)
		}
		for i, p := range c.planes {
			if l.planes[i] != p {
				t.Errorf("%s %dx%d plane %d at %+v, want %+v", c.format, c.w, c.h, i, l.planes[i], p)
			}
		}
		buf := make([]byte, l.size)
		f := numberedFrame(c.w, c.h)
		l.write(buf, f)
		filled := 0
		for _, b := range buf {
			if b != 0 {
				filled++
			}
		}
		if filled != c.filled {
			t.Errorf("%s %dx%d: a frame wrote %d bytes, want its %d samples", c.format, c.w, c.h, filled, c.filled)
		}
		d := l.dmabuf(7)
		if d.Format != c.format || d.Modifier != fourcc.Linear || len(d.Planes) != len(c.planes) || d.Planes[1].FD != 7 || int(d.Planes[1].Offset) != c.planes[1].offset {
			t.Errorf("%s dmabuf %+v", c.format, d)
		}
	}
}

func TestNV12InterleavesTheChroma(t *testing.T) {
	f := numberedFrame(4, 2)
	_, u, v := f.Planes()
	l := newVideoLayout(fourcc.NV12, 4, 2)
	buf := make([]byte, l.size)
	l.write(buf, f)
	uv := buf[l.planes[1].offset:]
	if uv[0] != u[0] || uv[1] != v[0] || uv[2] != u[1] || uv[3] != v[1] {
		t.Errorf("NV12 chroma %v, want U and V alternating from %v and %v", uv[:4], u, v)
	}
}

func TestColorRepresentationFollowsTheRange(t *testing.T) {
	if got := colorRepresentationOf(media.YUVColor{Matrix: media.BT601, Range: media.FullRange}); got.Range != 1 || got.Coefficients != 4 {
		t.Errorf("full range: %+v", got)
	}
	if got := colorRepresentationOf(media.YUVColor{Matrix: media.BT601, Range: media.LimitedRange}); got.Range != 2 || got.Coefficients != 4 {
		t.Errorf("limited range: %+v", got)
	}
}

// The buffers a real session allocates are udmabufs whose writes are
// bracketed by the dmabuf sync ioctl.
func TestUdmabufVideoMemoryTakesBracketedWrites(t *testing.T) {
	m, err := newUdmabufMemory(newVideoLayout(fourcc.YUV420, 32, 16).size)
	if err != nil {
		t.Skipf("no udmabuf: %v", err)
	}
	if err := m.beginWrite(); err != nil {
		t.Fatal(err)
	}
	newVideoLayout(fourcc.YUV420, 32, 16).write(m.bytes(), numberedFrame(32, 16))
	if err := m.endWrite(); err != nil {
		t.Fatal(err)
	}
	if m.fd() < 0 || m.bytes()[0] != 1 {
		t.Errorf("fd %d, first byte %d", m.fd(), m.bytes()[0])
	}
	if err := m.close(); err != nil {
		t.Fatal(err)
	}
}
