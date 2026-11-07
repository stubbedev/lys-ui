package media

import (
	"bytes"
	"errors"
	"image"
	"runtime"
	"runtime/debug"
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/internal/testbuild"
)

func yuvPlanes(w, h, frame int) []byte {
	pix := make([]byte, YUVFrameBytes(w, h))
	for i := range pix {
		pix[i] = byte(16 + (i*7+frame*13)%220)
	}
	return pix
}

func y4m420(header string, w, h, frames int) []byte {
	var b bytes.Buffer
	b.WriteString(header + "\n")
	for f := range frames {
		b.WriteString("FRAME\n")
		b.Write(yuvPlanes(w, h, f))
	}
	return b.Bytes()
}

func newYUVFrame(w, h int) *YUVFrame {
	return &YUVFrame{Width: w, Height: h, Pix: make([]byte, YUVFrameBytes(w, h))}
}

func TestYUVFramesAreHalfAgainTheirLuma(t *testing.T) {
	for _, c := range []struct{ w, h, want int }{{4, 2, 12}, {1920, 1080, 1920 * 1080 * 3 / 2}, {3, 3, 9 + 2*4}} {
		if got := YUVFrameBytes(c.w, c.h); got != c.want {
			t.Errorf("YUVFrameBytes(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
	f := newYUVFrame(4, 2)
	y, u, v := f.Planes()
	if len(y) != 8 || len(u) != 2 || len(v) != 2 || &v[1] != &f.Pix[11] {
		t.Errorf("planes %d %d %d", len(y), len(u), len(v))
	}
}

func TestY4MDecodesYUVPlanesAsTheyAre(t *testing.T) {
	for _, c := range []struct {
		header string
		want   YUVColor
	}{
		{"YUV4MPEG2 W6 H4 F25:1 C420jpeg", YUVColor{Matrix: BT601, Range: LimitedRange}},
		{"YUV4MPEG2 W6 H4 F25:1 C420mpeg2 XCOLORRANGE=FULL", YUVColor{Matrix: BT601, Range: FullRange}},
	} {
		src, err := NewY4M(bytes.NewReader(y4m420(c.header, 6, 4, 2)))
		if err != nil {
			t.Fatal(err)
		}
		ys := src.(YUVSource)
		if got, ok := ys.YUV(); !ok || got != c.want {
			t.Errorf("%s: YUV() = %+v, %v, want %+v", c.header, got, ok, c.want)
		}
		dst := newYUVFrame(6, 4)
		for n := range 2 {
			pts, err := ys.ReadFrameYUV(dst)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(dst.Pix, yuvPlanes(6, 4, n)) || dst.Color != c.want || pts != time.Duration(n)*40*time.Millisecond {
				t.Errorf("%s frame %d: planes or colour %+v at %v differ from the stream's", c.header, n, dst.Color, pts)
			}
		}
	}
}

func TestY4MDecodesTheSamePixelsThroughEitherPath(t *testing.T) {
	raw := y4m420("YUV4MPEG2 W6 H4 F25:1", 6, 4, 1)
	rgbaSrc, _ := NewY4M(bytes.NewReader(raw))
	want, _ := readOne(t, rgbaSrc)
	yuvSrc, _ := NewY4M(bytes.NewReader(raw))
	f := newYUVFrame(6, 4)
	if _, err := yuvSrc.(YUVSource).ReadFrameYUV(f); err != nil {
		t.Fatal(err)
	}
	got := image.NewRGBA(want.Rect)
	f.toRGBA(got)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Error("a YUV frame converted later differs from ReadFrame's RGBA")
	}
}

func TestY4MWithoutA420LayoutDecodesNoYUV(t *testing.T) {
	for _, cs := range []string{"422", "444", "mono"} {
		src, err := NewY4M(bytes.NewReader(y4mBytes("YUV4MPEG2 W4 H2 F25:1 C"+cs, 4, 2, 1, 0, cs == "mono", yuv{1, 2, 3})))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := src.(YUVSource).YUV(); ok {
			t.Errorf("C%s: a 4:2:0 YUV source", cs)
		}
		if _, err := src.(YUVSource).ReadFrameYUV(newYUVFrame(4, 2)); !errors.Is(err, errNoYUV) {
			t.Errorf("C%s: ReadFrameYUV = %v, want errNoYUV", cs, err)
		}
	}
	src, _ := NewY4M(bytes.NewReader(y4m420("YUV4MPEG2 W6 H4 F25:1", 6, 4, 1)))
	if _, err := src.(YUVSource).ReadFrameYUV(newYUVFrame(4, 4)); err == nil {
		t.Error("a 6x4 frame was read into a 4x4 buffer")
	}
}

func TestWithoutAudioKeepsTheYUVFrames(t *testing.T) {
	src, _ := NewY4M(bytes.NewReader(y4m420("YUV4MPEG2 W6 H4 F25:1 XCOLORRANGE=FULL", 6, 4, 1)))
	if yuvOf(WithoutAudio(src)) == nil {
		t.Error("WithoutAudio hid the YUV frames")
	}
	if yuvOf(WithoutAudio(&fakeSource{})) != nil {
		t.Error("WithoutAudio made an RGBA source decode YUV")
	}
	if _, err := WithoutAudio(&fakeSource{}).(YUVSource).ReadFrameYUV(newYUVFrame(2, 2)); !errors.Is(err, errNoYUV) {
		t.Errorf("ReadFrameYUV through an RGBA source: %v", err)
	}
}

func (h *harness) pool() []*frameBuf {
	out := []*frameBuf{h.s.cur}
	for _, d := range h.s.queue {
		out = append(out, d.img)
	}
	return out
}

// A viewer showing FrameYUV gets the planes as decoded and costs no
// RGBA conversion; Frame still works, converting on demand, and
// turning SkipRGBA off brings the decoder's conversion back.
func TestStreamSkippingRGBAConvertsNothing(t *testing.T) {
	const w, h = 6, 4
	raw := y4m420("YUV4MPEG2 W6 H4 F25:1", w, h, 32)
	src, err := NewY4M(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	play(t, src, noAudio, func(hs *harness) {
		s := hs.s
		if c, ok := s.YUV(); !ok || c != (YUVColor{Matrix: BT601, Range: LimitedRange}) {
			t.Fatalf("YUV() = %+v, %v", c, ok)
		}
		s.SkipRGBA(true)
		s.Play()
		for range framePool + 2 {
			hs.step(40 * time.Millisecond)
		}
		if s.FrameSeq() < 4 {
			t.Fatalf("%d frames shown", s.FrameSeq())
		}
		f := s.FrameYUV()
		if f == nil || !bytes.Equal(f.Pix, yuvPlanes(w, h, int(s.FrameSeq())-1)) {
			t.Fatal("FrameYUV is not the decoded frame")
		}
		for _, fb := range hs.pool()[:2] {
			if fb.rgba {
				t.Error("a frame was converted to RGBA while the viewer shows YUV")
			}
		}
		rgba := s.Frame()
		if rgba.RGBA == nil || !s.cur.rgba {
			t.Fatal("Frame did not convert the YUV frame it shows")
		}
		ref, _ := NewY4M(bytes.NewReader(raw))
		var want *image.RGBA
		for range s.FrameSeq() {
			want, _ = readOne(t, ref)
		}
		if !bytes.Equal(rgba.Pix, want.Pix) {
			t.Error("Frame's conversion differs from ReadFrame's pixels")
		}
		s.SkipRGBA(false)
		for range framePool + 2 {
			hs.step(40 * time.Millisecond)
		}
		if !s.cur.rgba {
			t.Error("the decoder kept skipping RGBA after SkipRGBA(false)")
		}
	})
	if _, ok := newStream(&fakeSource{frames: 1}, "rgba", noAudio).YUV(); ok {
		t.Error("an RGBA source's stream reports YUV frames")
	}
}

// A playing stream of YUV frames shown as YUV allocates nothing per
// frame once warm: decoding, presenting and notifying included, averaged
// over the frames as testing.AllocsPerRun averages, so the test clock's
// own timer allocations do not count.
func TestStreamSteadyYUVFramesAllocateNothing(t *testing.T) {
	testbuild.SkipAllocBudget(t)
	src, err := NewY4M(bytes.NewReader(y4m420("YUV4MPEG2 W64 H48 F25:1", 64, 48, 16)))
	if err != nil {
		t.Fatal(err)
	}
	play(t, src, noAudio, func(hs *harness) {
		s := hs.s
		s.SkipRGBA(true)
		s.SetLoop(true)
		s.Play()
		for range 40 {
			hs.step(40 * time.Millisecond)
		}
		const frames = 100
		seq := s.FrameSeq()
		runtime.GC()
		defer debug.SetGCPercent(debug.SetGCPercent(-1))
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for range frames {
			hs.step(40 * time.Millisecond)
		}
		runtime.ReadMemStats(&after)
		if shown := s.FrameSeq() - seq; shown < frames*9/10 {
			t.Fatalf("%d frames shown in %d steps", shown, frames)
		}
		if n := after.Mallocs - before.Mallocs; n/frames > 0 {
			t.Errorf("%d heap allocations over %d steady frames, want none per frame", n, frames)
		}
	})
}
