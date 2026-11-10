package media

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/media/demux"
	"github.com/stubbedev/lys-lib/pulse/native"
	"github.com/stubbedev/lys-lib/testbuild"
)

// The fixtures are one second of a 192x144 picture, red until 500 ms and
// green from there, with a 440 Hz tone over silence from 500 ms on:
//
//	ffmpeg -f lavfi -i 'color=c=red:s=192x144:r=25:d=1,drawbox=enable=gte(t\,0.5):c=green:t=fill' \
//	       -f lavfi -i 'aevalsrc=if(gte(t\,0.5)\,0.5*sin(2*PI*440*t)\,0):s=44100:c=stereo:d=1' \
//	       -c:v libx264 -g 10 -bf 2 -c:a aac h264-aac.mp4
//
// h265-opus.mkv is the same in H.265 (keyint 10, two B-frames) and Opus
// at 48 kHz, h264-flac.mkv has FLAC sound, vp9.webm VP9 video and
// tone.mka the Opus tone alone.
const (
	fixtureDuration = time.Second
	fixtureOnset    = 500 * time.Millisecond
	onsetLevel      = 8000
)

var fixtures = []struct {
	name   string
	format AudioFormat
	// onsetSlack is how far the tone's decoded onset may stray: the
	// codec smears it by a fraction of its frame, far less than the
	// encoder delay a wrong start would shift it by.
	onsetSlack time.Duration
}{
	{"h264-aac.mp4", AudioFormat{Rate: 44100, Layout: native.ChannelMap{native.ChannelFrontLeft, native.ChannelFrontRight}}, 6 * time.Millisecond},
	{"h265-opus.mkv", AudioFormat{Rate: 48000, Layout: native.ChannelMap{native.ChannelFrontLeft, native.ChannelFrontRight}}, 3 * time.Millisecond},
}

func fixture(t testing.TB, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func openFixture(t testing.TB, name string) Source {
	t.Helper()
	src, err := OpenFile(fixture(t, name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := src.Close(); err != nil {
			t.Error(err)
		}
	})
	return src
}

func readPCM(t *testing.T, a Audible, from time.Duration) []int16 {
	t.Helper()
	pcm, err := a.OpenAudio(from)
	if err != nil {
		t.Fatal(err)
	}
	defer pcm.Close()
	b, err := io.ReadAll(pcm)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}

// onset is when the first sample past onsetLevel plays, frames counted
// from the PCM's start.
func onset(pcm []int16, f AudioFormat) time.Duration {
	for i, v := range pcm {
		if v > onsetLevel || v < -onsetLevel {
			return samplesAt(i/len(f.Layout), f.Rate)
		}
	}
	return -1
}

func within(got, want, slack time.Duration) bool { return got >= want-slack && got <= want+slack }

func TestOpenFileFindsTheTracks(t *testing.T) {
	for _, c := range fixtures {
		t.Run(c.name, func(t *testing.T) {
			src := openFixture(t, c.name)
			if i := src.Info(); i.Width != 192 || i.Height != 144 || !within(i.Duration, fixtureDuration, 50*time.Millisecond) {
				t.Errorf("info %+v", i)
			}
			if _, ok := src.(TextureSource); !ok {
				t.Error("the file's video does not decode on the GPU")
			}
			a, ok := src.(Audible)
			if !ok {
				t.Fatal("a file with sound is not Audible")
			}
			if f := a.AudioFormat(); f.Rate != c.format.Rate || len(f.Layout) != 2 || f.Layout[0] != c.format.Layout[0] || f.Layout[1] != c.format.Layout[1] {
				t.Errorf("audio format %+v, want %+v", f, c.format)
			}
		})
	}
}

// The sound starts at its timestamps: the AAC encoder's priming the MP4
// edit list names, and the Opus pre-skip, are dropped, so the tone
// sounds 500 ms in as the picture turns green, and seeking lands the
// PCM's first sample on the target.
func TestOpenFileSoundKeepsTime(t *testing.T) {
	for _, c := range fixtures {
		t.Run(c.name, func(t *testing.T) {
			a := openFixture(t, c.name).(Audible)
			f := a.AudioFormat()
			for _, from := range []time.Duration{0, 250 * time.Millisecond, 0, 400 * time.Millisecond} {
				pcm := readPCM(t, a, from)
				if got := onset(pcm, f); !within(got, fixtureOnset-from, c.onsetSlack) {
					t.Errorf("from %v: the tone sounds %v in, want %v", from, got, fixtureOnset-from)
				}
				length := samplesAt(len(pcm)/len(f.Layout), f.Rate)
				if length < fixtureDuration-from || length > fixtureDuration-from+50*time.Millisecond {
					t.Errorf("from %v: %v of sound, want the %v to the end", from, length, fixtureDuration-from)
				}
			}
			pcm := readPCM(t, a, 750*time.Millisecond)
			if got := onset(pcm, f); got < 0 || got > 5*time.Millisecond {
				t.Errorf("from 750 ms the tone sounds %v in, want at once", got)
			}
		})
	}
}

func TestOpeningTheSoundAgainClosesTheReaderBefore(t *testing.T) {
	a := openFixture(t, "h265-opus.mkv").(Audible)
	first, err := a.OpenAudio(0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.OpenAudio(0)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := first.Read(make([]byte, 64)); !errors.Is(err, errClosed) {
		t.Errorf("the reader before reads on: %v", err)
	}
	if n, err := second.Read(make([]byte, 64)); n != 64 || err != nil {
		t.Errorf("the new reader read %d bytes: %v", n, err)
	}
}

func TestSteadySoundAllocatesNothing(t *testing.T) {
	if !testbuild.AllocBudgetsHold {
		t.Skip("allocation budgets do not hold in this build")
	}
	for _, c := range fixtures {
		t.Run(c.name, func(t *testing.T) {
			a := openFixture(t, c.name).(Audible)
			pcm, err := a.OpenAudio(0)
			if err != nil {
				t.Fatal(err)
			}
			defer pcm.Close()
			buf := make([]byte, c.format.Rate/audioChunksPerSec*c.format.frameBytes())
			if _, err := io.ReadFull(pcm, buf); err != nil {
				t.Fatal(err)
			}
			if allocs := testing.AllocsPerRun(20, func() {
				if _, err := io.ReadFull(pcm, buf); err != nil {
					t.Fatal(err)
				}
			}); allocs != 0 {
				t.Errorf("%v allocations per 10 ms of sound, want none", allocs)
			}
		})
	}
}

func TestOpenFileSaysWhatItCannotPlay(t *testing.T) {
	_, err := OpenFile(fixture(t, "h264-flac.mkv"))
	var unsupported *demux.UnsupportedCodecError
	if !errors.As(err, &unsupported) || !strings.Contains(unsupported.Codec, "FLAC") || unsupported.File != fixture(t, "h264-flac.mkv") {
		t.Errorf("FLAC sound: %v", err)
	}
	if _, err := OpenFile(fixture(t, "tone.mka")); err == nil || !strings.Contains(err.Error(), "no video track") || !strings.Contains(err.Error(), "tone.mka") {
		t.Errorf("a sound file: %v", err)
	}
	absent := filepath.Join(t.TempDir(), "absent.mkv")
	if _, err := OpenFile(absent); !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), absent) {
		t.Errorf("a missing file: %v", err)
	}
	if _, err := OpenFile(fixture(t, "../media.go")); !errors.Is(err, demux.ErrUnknownFormat) {
		t.Errorf("a Go file: %v", err)
	}
}

func TestWithoutAudioKeepsTheFileOnTheGPU(t *testing.T) {
	src := WithoutAudio(openFixture(t, "h265-opus.mkv"))
	if _, ok := src.(Audible); ok {
		t.Error("WithoutAudio kept the sound")
	}
	if _, ok := src.(TextureSource); !ok {
		t.Error("WithoutAudio lost the GPU decoding")
	}
}
