package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"
)

type yuv struct{ y, u, v byte }

func y4mBytes(header string, w, h int, xs, ys uint, mono bool, frames ...yuv) []byte {
	var b bytes.Buffer
	b.WriteString(header + "\n")
	cw, ch := (w+1<<xs-1)>>xs, (h+1<<ys-1)>>ys
	for _, f := range frames {
		b.WriteString("FRAME\n")
		b.Write(bytes.Repeat([]byte{f.y}, w*h))
		if !mono {
			b.Write(bytes.Repeat([]byte{f.u}, cw*ch))
			b.Write(bytes.Repeat([]byte{f.v}, cw*ch))
		}
	}
	return b.Bytes()
}

type onlyReader struct{ io.Reader }

func readOne(t *testing.T, src FrameSource) (*image.RGBA, time.Duration) {
	t.Helper()
	i := src.Info()
	dst := image.NewRGBA(image.Rect(0, 0, i.Width, i.Height))
	pts, err := src.ReadFrame(dst)
	if err != nil {
		t.Fatal(err)
	}
	return dst, pts
}

func near(a, b uint8) bool { return a-b <= 2 || b-a <= 2 }

func TestY4MColorsFollowBT601InBothRanges(t *testing.T) {
	for _, c := range []struct {
		name   string
		header string
		in     yuv
		want   [3]uint8
	}{
		{"full white", "YUV4MPEG2 W4 H2 F25:1 C420jpeg XCOLORRANGE=FULL", yuv{255, 128, 128}, [3]uint8{255, 255, 255}},
		{"full black", "YUV4MPEG2 W4 H2 F25:1 C420jpeg XCOLORRANGE=FULL", yuv{0, 128, 128}, [3]uint8{0, 0, 0}},
		{"full red", "YUV4MPEG2 W4 H2 F25:1 C420jpeg XCOLORRANGE=FULL", yuv{76, 85, 255}, [3]uint8{255, 0, 0}},
		{"full blue", "YUV4MPEG2 W4 H2 F25:1 C420jpeg XCOLORRANGE=FULL", yuv{29, 255, 107}, [3]uint8{0, 0, 255}},
		{"limited white", "YUV4MPEG2 W4 H2 F25:1", yuv{235, 128, 128}, [3]uint8{255, 255, 255}},
		{"limited black", "YUV4MPEG2 W4 H2 F25:1 XCOLORRANGE=LIMITED", yuv{16, 128, 128}, [3]uint8{0, 0, 0}},
		{"limited green", "YUV4MPEG2 W4 H2 F25:1", yuv{145, 54, 34}, [3]uint8{0, 255, 0}},
	} {
		t.Run(c.name, func(t *testing.T) {
			src, err := NewY4M(bytes.NewReader(y4mBytes(c.header, 4, 2, 1, 1, false, c.in)))
			if err != nil {
				t.Fatal(err)
			}
			img, _ := readOne(t, src)
			p := img.Pix[4*3+img.Stride:]
			if !near(p[0], c.want[0]) || !near(p[1], c.want[1]) || !near(p[2], c.want[2]) || p[3] != 255 {
				t.Errorf("pixel %v, want %v", p[:4], c.want)
			}
		})
	}
}

func TestY4MChromaLayouts(t *testing.T) {
	for _, c := range []struct {
		cs     string
		xs, ys uint
		mono   bool
	}{{"420", 1, 1, false}, {"420mpeg2", 1, 1, false}, {"422", 1, 0, false}, {"444", 0, 0, false}, {"mono", 0, 0, true}} {
		t.Run(c.cs, func(t *testing.T) {
			raw := y4mBytes("YUV4MPEG2 W3 H3 F25:1 XCOLORRANGE=FULL C"+c.cs, 3, 3, c.xs, c.ys, c.mono, yuv{255, 128, 128}, yuv{0, 128, 128})
			src, err := NewY4M(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			first, _ := readOne(t, src)
			second, pts := readOne(t, src)
			if first.Pix[len(first.Pix)-4] != 255 || second.Pix[len(second.Pix)-4] != 0 {
				t.Errorf("corner pixels %v then %v, want white then black", first.Pix[len(first.Pix)-4:], second.Pix[len(second.Pix)-4:])
			}
			if pts != 40*time.Millisecond {
				t.Errorf("second frame at %v, want 40ms", pts)
			}
			if _, err := src.ReadFrame(first); !errors.Is(err, io.EOF) {
				t.Errorf("past the end: %v, want io.EOF", err)
			}
		})
	}
}

func TestY4MTimestampsAreExactRationals(t *testing.T) {
	h := y4mHeader{rateNum: 30000, rateDen: 1001}
	for _, n := range []int64{0, 1, 3, 1_000_000_000} {
		exact := new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(n), big.NewInt(1001*int64(time.Second))), big.NewInt(30000))
		want := new(big.Int).Div(new(big.Int).Add(exact.Num(), new(big.Int).Sub(exact.Denom(), big.NewInt(1))), exact.Denom())
		if got := h.pts(n); int64(got) != want.Int64() {
			t.Errorf("pts(%d) = %d, want %s", n, got, want)
		}
		if n < 1_000_000_000 && h.frameAt(h.pts(n)) != n {
			t.Errorf("frameAt(pts(%d)) = %d", n, h.frameAt(h.pts(n)))
		}
	}
}

func TestY4MFilesSeekAndKnowTheirDuration(t *testing.T) {
	frames := make([]yuv, 10)
	for i := range frames {
		frames[i] = yuv{byte(i * 20), 128, 128}
	}
	prefix := []byte("junk before the stream")
	r := bytes.NewReader(append(prefix, y4mBytes("YUV4MPEG2 W2 H2 F10:1 XCOLORRANGE=FULL", 2, 2, 1, 1, false, frames...)...))
	if _, err := r.Seek(int64(len(prefix)), io.SeekStart); err != nil {
		t.Fatal(err)
	}
	src, err := NewY4M(r)
	if err != nil {
		t.Fatal(err)
	}
	if d := src.Info().Duration; d != time.Second {
		t.Errorf("duration %v, want 1s", d)
	}
	seeker, ok := src.(Seeker)
	if !ok {
		t.Fatal("a ReadSeeker source does not seek")
	}
	for _, c := range []struct {
		to   time.Duration
		want time.Duration
	}{{450 * time.Millisecond, 400 * time.Millisecond}, {0, 0}, {5 * time.Second, 900 * time.Millisecond}, {-time.Second, 0}} {
		if err := seeker.Seek(c.to); err != nil {
			t.Fatal(err)
		}
		img, pts := readOne(t, src)
		if pts != c.want || img.Pix[0] != byte(c.want/(100*time.Millisecond))*20 {
			t.Errorf("seek %v: frame at %v luma %d, want %v", c.to, pts, img.Pix[0], c.want)
		}
	}
}

func TestY4MPipesDoNotSeek(t *testing.T) {
	raw := []byte("YUV4MPEG2 W2 H2 F25:1\nFRAME Ixyz\n" + strings.Repeat("\x80", 6))
	src, err := NewY4M(onlyReader{bytes.NewReader(raw)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := src.(Seeker); ok || src.Info().Duration != 0 {
		t.Error("a plain reader claims to seek or to know its length")
	}
	if _, pts := readOne(t, src); pts != 0 {
		t.Errorf("frame with parameters at %v", pts)
	}
}

func TestY4MErrorsSayWhatIsWrong(t *testing.T) {
	for _, c := range []struct {
		raw, want string
	}{
		{"RIFF....\n", "not a YUV4MPEG2 stream"},
		{"YUV4MPEG2 W2 H2 F25:1 It\n", "interlacing"},
		{"YUV4MPEG2 W2 H2 F25:1 C420p10\n", `colorspace "420p10"`},
		{"YUV4MPEG2 W2 H2\n", "frame rate"},
		{"YUV4MPEG2 W0 H2 F25:1\n", "not positive"},
		{"YUV4MPEG2 W2 H2 F25\n", "num:den"},
		{"YUV4MPEG2 W2 H2 F25:1 " + strings.Repeat("A", maxY4MLine) + "\n", "runs past"},
		{"YUV4MPEG2 W2 H2 F25:1", "header"},
	} {
		if _, err := NewY4M(onlyReader{strings.NewReader(c.raw)}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%.30q: %v, want %q", c.raw, err, c.want)
		}
	}
	read := func(raw string, seekable bool) error {
		var r io.Reader = strings.NewReader(raw)
		if !seekable {
			r = onlyReader{r}
		}
		src, err := NewY4M(r)
		if err != nil {
			return err
		}
		_, err = src.ReadFrame(image.NewRGBA(image.Rect(0, 0, 2, 2)))
		return err
	}
	head := "YUV4MPEG2 W2 H2 F25:1\n"
	for _, c := range []struct {
		raw      string
		seekable bool
		want     string
	}{
		{head + "FRAME\n\x10\x10", false, "truncated"},
		{head + "FRAME Ixyz\n" + strings.Repeat("\x80", 6), true, "bare FRAME"},
		{head + "FRAMX\n" + strings.Repeat("\x80", 6), false, "bare FRAME"},
	} {
		if err := read(c.raw, c.seekable); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%.40q: %v, want %q", c.raw, err, c.want)
		}
	}
	src, _ := NewY4M(strings.NewReader(head))
	if _, err := src.ReadFrame(image.NewRGBA(image.Rect(0, 0, 3, 2))); err == nil || !strings.Contains(err.Error(), "buffer") {
		t.Errorf("a wrong-size buffer: %v", err)
	}
}

func ExampleNewY4M() {
	raw := "YUV4MPEG2 W2 H2 F25:1 XCOLORRANGE=FULL\nFRAME\n" + strings.Repeat("\xff", 4) + "\x80\x80"
	src, _ := NewY4M(strings.NewReader(raw))
	dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pts, _ := src.ReadFrame(dst)
	fmt.Printf("%v %v %v\n", src.Info().Duration, pts, dst.Pix[:4])
	// Output: 40ms 0s [255 255 255 255]
}
