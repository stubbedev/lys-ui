package media

import (
	"errors"
	"image"
	"image/color"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/gputest"
	"github.com/stubbedev/lys-lib/media/demux"
	"github.com/stubbedev/lys-lib/testbuild"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/waittest"
)

const fixtureStep = 40 * time.Millisecond

func gpuDevice(t *testing.T) *gpu.Device {
	t.Helper()
	if !gputest.Enabled() {
		t.Skip("LYS_TEST_GPU is unset")
	}
	if _, err := gputest.SetupTest(t); err != nil {
		t.Fatal(err)
	}
	dev, err := gpu.Open()
	if err != nil {
		t.Fatalf("GPU for LYS_TEST_GPU: %v", err)
	}
	t.Cleanup(dev.Close)
	return dev
}

// gpuStream plays name's video on dev, its sound hidden.
func gpuStream(t *testing.T, dev *gpu.Device, name string) *Stream {
	t.Helper()
	src, err := OpenFile(fixture(t, name))
	if err != nil {
		t.Fatal(err)
	}
	s := newStream(new(anim.Clock), WithoutAudio(src), "test", noAudio)
	t.Cleanup(func() {
		s.Close()
		if err := s.Wait(); err != nil {
			t.Error(err)
		}
	})
	if err := s.UseDevice(dev, nil); err != nil {
		t.Fatal(err)
	}
	return s
}

// until advances s at now until done, the packets read on their own
// goroutine meanwhile.
func until(t *testing.T, s *Stream, now time.Time, what string, done func() bool) {
	t.Helper()
	waittest.Until(t, func() bool {
		s.advance(now)
		if done() {
			return true
		}
		if err := s.Err(); err != nil {
			t.Fatalf("waiting for %s: %v", what, err)
		}
		return false
	}, what)
}

func wantShownDecoded(t *testing.T, s *Stream) {
	t.Helper()
	if decoded, err := s.Texture().Decoded(); err != nil || !decoded {
		t.Fatalf("the frame the stream shows has finished decoding: %v, %v; want true", decoded, err)
	}
}

func nextFrameDecoded(t *testing.T, s *Stream) bool {
	t.Helper()
	if len(s.tex.queue) == 0 {
		return s.eofGen == s.gen
	}
	decoded, err := s.tex.queue[0].Decoded()
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// centre is the colour of the shown frame's centre, drawn on dev.
func centre(t *testing.T, dev *gpu.Device, f *gpu.VideoFrame) color.RGBA {
	t.Helper()
	size := f.Size()
	tg, err := dev.NewOffscreen(size.X, size.Y)
	if err != nil {
		t.Fatal(err)
	}
	defer tg.Close()
	bounds := image.Rectangle{Max: size}
	p, err := dev.Begin(tg, []image.Rectangle{bounds})
	if err != nil {
		t.Fatal(err)
	}
	p.DrawFrame(f, gpu.FRect{W: float64(size.X), H: float64(size.Y)}, bounds, gpu.TransformNormal, 1, gpu.Mask{})
	if err := p.End(); err != nil {
		t.Fatal(err)
	}
	img, err := tg.ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	return img.RGBAAt(size.X/2, size.Y/2)
}

func reddish(c color.RGBA) bool  { return c.R > 200 && c.G < 60 && c.B < 60 }
func greenish(c color.RGBA) bool { return c.G > 90 && c.R < 60 && c.B < 60 }

var gpuFixtures = []struct {
	name  string
	codec demux.Codec
}{
	{"h264-aac.mp4", demux.H264},
	{"h265-opus.mkv", demux.H265},
}

// A file's frames decode on the GPU in presentation order, B-frames
// and all: seeking lands on the frame showing at the target, decoded
// from the keyframe before it, and playback shows every frame on its
// timestamp, allocating nothing per frame.
func TestFilesDecodeOnTheGPU(t *testing.T) {
	dev := gpuDevice(t)
	for _, c := range gpuFixtures {
		t.Run(c.name, func(t *testing.T) {
			if !slices.Contains(dev.DecodeCodecs(), c.codec) {
				t.Skipf("%s decodes no %s", dev.Name(), c.codec)
			}
			s := gpuStream(t, dev, c.name)
			now := time.Now()
			until(t, s, now, "the first frame", func() bool { return s.Texture() != nil })
			wantShownDecoded(t, s)
			if f := s.Texture(); f.PTS() != 0 || f.Size() != image.Pt(192, 144) || !reddish(centre(t, dev, f)) {
				t.Errorf("first frame at %v, %v, colour %v, want red at 0", f.PTS(), f.Size(), centre(t, dev, f))
			}
			for _, seek := range []struct {
				to, shows time.Duration
				green     bool
			}{
				{620 * time.Millisecond, 600 * time.Millisecond, true},
				{200 * time.Millisecond, 200 * time.Millisecond, false},
				{960 * time.Millisecond, 960 * time.Millisecond, true},
			} {
				if err := s.Seek(seek.to); err != nil {
					t.Fatal(err)
				}
				until(t, s, now, "the frame at the seek target", func() bool { return s.curGen == s.gen })
				f := s.Texture()
				wantShownDecoded(t, s)
				if got := centre(t, dev, f); f.PTS() != seek.shows || greenish(got) != seek.green || !greenish(got) && !reddish(got) {
					t.Errorf("seek to %v shows the frame at %v coloured %v, want the one at %v, green %v", seek.to, f.PTS(), got, seek.shows, seek.green)
				}
			}

			if err := s.Seek(0); err != nil {
				t.Fatal(err)
			}
			until(t, s, now, "the frame at 0", func() bool { return s.curGen == s.gen })
			s.Play()
			waits := dev.BlockingWaits()
			var (
				shown  = append(make([]time.Duration, 0, 64), s.Texture().PTS())
				allocs uint64
				m0, m1 runtime.MemStats
			)
			for step := 0; !s.Ended(); step++ {
				if step == 10 {
					runtime.ReadMemStats(&m0)
				}
				until(t, s, now, "the next frame decoded", func() bool { return nextFrameDecoded(t, s) })
				now = now.Add(fixtureStep)
				s.advance(now)
				if f := s.Texture(); len(shown) == 0 || shown[len(shown)-1] != f.PTS() {
					shown = append(shown, f.PTS())
				}
				if step > 40 {
					t.Fatalf("still playing after %d steps of %v", step, fixtureStep)
				}
			}
			runtime.ReadMemStats(&m1)
			allocs = m1.Mallocs - m0.Mallocs
			if waited := dev.BlockingWaits() - waits; waited != 0 {
				t.Errorf("playback waited for the GPU %d times, want never", waited)
			}
			if len(shown) != 25 || shown[0] != 0 || shown[24] != 24*fixtureStep {
				t.Errorf("playback showed the frames at %v, want every 40 ms of the second", shown)
			}
			for i := 1; i < len(shown); i++ {
				if shown[i]-shown[i-1] != fixtureStep {
					t.Errorf("frame %d at %v follows %v", i, shown[i], shown[i-1])
				}
			}
			t.Logf("%d heap allocations over the last %d frames", allocs, len(shown)-10)
			if testbuild.AllocBudgetsHold && allocs != 0 {
				t.Errorf("%d heap allocations over %d frames of playback, want none", allocs, len(shown)-10)
			}
		})
	}
}

// A codec the device does not decode fails the stream with an error
// naming the codec and the file.
func TestAVideoCodecTheGPUDoesNotDecodeSaysSo(t *testing.T) {
	dev := gpuDevice(t)
	if slices.Contains(dev.DecodeCodecs(), demux.VP9) {
		t.Skipf("%s decodes VP9", dev.Name())
	}
	src, err := OpenFile(fixture(t, "vp9.webm"))
	if err != nil {
		t.Fatal(err)
	}
	s := newStream(new(anim.Clock), src, "test", noAudio)
	defer func() {
		s.Close()
		if err := s.Wait(); err != nil {
			t.Error(err)
		}
	}()
	err = s.UseDevice(dev, nil)
	var unsupported *demux.UnsupportedCodecError
	if !errors.Is(err, gpu.ErrNoVideoDecoder) || !errors.As(err, &unsupported) || !strings.Contains(unsupported.Codec, "VP9") || unsupported.File != fixture(t, "vp9.webm") {
		t.Errorf("UseDevice = %v, want the device's refusal naming VP9 and the file", err)
	}
	if !errors.Is(s.Err(), gpu.ErrNoVideoDecoder) {
		t.Errorf("the stream's error %v, want the refusal", s.Err())
	}
}

func TestAStreamResumesAtItsPositionOnANewGPUDevice(t *testing.T) {
	first := gpuDevice(t)
	second, err := gpu.Open()
	if err != nil {
		t.Fatalf("a second GPU device: %v", err)
	}
	t.Cleanup(second.Close)
	for _, c := range gpuFixtures {
		t.Run(c.name, func(t *testing.T) {
			if !slices.Contains(first.DecodeCodecs(), c.codec) {
				t.Skipf("%s decodes no %s", first.Name(), c.codec)
			}
			s := gpuStream(t, first, c.name)
			now := time.Now()
			const at, shows = 620 * time.Millisecond, 600 * time.Millisecond
			if err := s.Seek(at); err != nil {
				t.Fatal(err)
			}
			until(t, s, now, "the frame at the seek target", func() bool { return s.curGen == s.gen })
			if f := s.Texture(); f.PTS() != shows {
				t.Fatalf("the first device shows the frame at %v, want %v", f.PTS(), shows)
			}

			s.ReleaseDevice()
			if s.Texture() != nil || s.Err() != nil || s.Position() != at {
				t.Fatalf("between the devices: frame %v, error %v, at %v", s.Texture(), s.Err(), s.Position())
			}
			if err := s.UseDevice(second, nil); err != nil {
				t.Fatal(err)
			}
			until(t, s, now, "the frame at the position on the new device", func() bool { return s.curGen == s.gen })
			f := s.Texture()
			if got := centre(t, second, f); f.PTS() != shows || !greenish(got) {
				t.Fatalf("the new device shows the frame at %v coloured %v, want the green one at %v", f.PTS(), got, shows)
			}

			s.Play()
			shown := []time.Duration{f.PTS()}
			for step := 0; !s.Ended(); step++ {
				until(t, s, now, "the next frame decoded", func() bool { return nextFrameDecoded(t, s) })
				now = now.Add(fixtureStep)
				s.advance(now)
				if f := s.Texture(); shown[len(shown)-1] != f.PTS() {
					shown = append(shown, f.PTS())
				}
				if step > 40 {
					t.Fatalf("still playing after %d steps of %v", step, fixtureStep)
				}
			}
			if len(shown) != 10 || shown[9] != 24*fixtureStep {
				t.Errorf("playback on the new device showed the frames at %v, want every 40 ms from %v to the end", shown, shows)
			}
			for i := 1; i < len(shown); i++ {
				if shown[i]-shown[i-1] != fixtureStep {
					t.Errorf("frame %d at %v follows %v", i, shown[i], shown[i-1])
				}
			}
		})
	}
}
