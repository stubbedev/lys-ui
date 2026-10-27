package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jfreymuth/pulse"
	"github.com/stubbedev/lys-lib/render"
)

func solid(c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	for x := range 2 {
		img.SetRGBA(x, 0, c)
	}
	return img
}

func TestAnimationPlaysItsFramesOnTheirDelays(t *testing.T) {
	a := &render.Animation{
		Frames: []render.StillImage{render.NewStillImage(solid(color.RGBA{1, 0, 0, 255})), render.NewStillImage(solid(color.RGBA{2, 0, 0, 255})), render.NewStillImage(solid(color.RGBA{3, 0, 0, 255}))},
		Delays: []time.Duration{100 * time.Millisecond, 50 * time.Millisecond, 200 * time.Millisecond},
	}
	src, err := NewAnimation(a)
	if err != nil {
		t.Fatal(err)
	}
	if i := src.Info(); i.Width != 2 || i.Height != 1 || i.Duration != 350*time.Millisecond {
		t.Errorf("info %+v", i)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 2, 1))
	for i, want := range []time.Duration{0, 100 * time.Millisecond, 150 * time.Millisecond} {
		pts, err := src.ReadFrame(dst)
		if err != nil || pts != want || dst.Pix[4] != byte(i+1) {
			t.Errorf("frame %d: %v at %v red %d", i, err, pts, dst.Pix[4])
		}
	}
	if _, err := src.ReadFrame(dst); !errors.Is(err, io.EOF) {
		t.Errorf("past the end: %v", err)
	}
	if err := src.(Seeker).Seek(120 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if pts, _ := src.ReadFrame(dst); pts != 100*time.Millisecond {
		t.Errorf("seek into the second frame resumed at %v", pts)
	}
	if _, err := src.ReadFrame(image.NewRGBA(image.Rect(0, 0, 3, 3))); err == nil {
		t.Error("a wrong-size buffer was filled")
	}
	for _, bad := range []*render.Animation{
		{},
		{Frames: a.Frames, Delays: a.Delays[:1]},
		{Frames: a.Frames[:1], Delays: []time.Duration{0}},
	} {
		if _, err := NewAnimation(bad); err == nil {
			t.Errorf("animation %d frames %v delays accepted", len(bad.Frames), bad.Delays)
		}
	}
}

func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
}

func makeClip(t *testing.T, name string, args ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("ffmpeg", append(append([]string{"-v", "error", "-y"}, args...), path)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make %s: %v: %s", name, err, out)
	}
	return path
}

func TestFFmpegDecodesSeeksAndPlaysSound(t *testing.T) {
	requireFFmpeg(t)
	clip := makeClip(t, "red.mkv",
		"-f", "lavfi", "-i", "color=c=red:s=64x48:r=25:d=1",
		"-f", "lavfi", "-i", "sine=f=440:d=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "flac", "-shortest")
	src, err := OpenFile(clip)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	info := src.Info()
	if info.Width != 64 || info.Height != 48 || info.Duration < 900*time.Millisecond || info.Duration > 1100*time.Millisecond {
		t.Errorf("info %+v", info)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 64, 48))
	var last time.Duration
	n := 0
	for {
		pts, err := src.ReadFrame(dst)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if n > 0 && pts-last != 40*time.Millisecond {
			t.Errorf("frame %d at %v after %v", n, pts, last)
		}
		last = pts
		n++
	}
	if n != 25 {
		t.Errorf("%d frames, want 25", n)
	}
	if p := dst.Pix[4*(32+24*64):]; p[0] < 240 || p[1] > 15 || p[2] > 15 {
		t.Errorf("red decoded as %v", p[:4])
	}
	if err := src.(Seeker).Seek(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if pts, err := src.ReadFrame(dst); err != nil || pts != 500*time.Millisecond {
		t.Errorf("after seeking to 500ms: %v at %v", err, pts)
	}
	audible, ok := src.(Audible)
	if !ok {
		t.Fatal("a clip with sound is not Audible")
	}
	pcm, err := audible.OpenAudio(500 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer pcm.Close()
	sound, err := io.ReadAll(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if want := AudioRate * audioFrame / 2; len(sound) < want*9/10 || len(sound) > want*11/10 {
		t.Errorf("%d bytes of sound from 500ms, want about %d", len(sound), want)
	}
	var peak int16
	for i := 0; i+1 < len(sound); i += 2 {
		peak = max(peak, int16(binary.LittleEndian.Uint16(sound[i:])))
	}
	if peak < 1000 {
		t.Errorf("the sine decoded silent, peak %d", peak)
	}
}

func TestFFmpegReportsWhatItCannotPlay(t *testing.T) {
	requireFFmpeg(t)
	silent := makeClip(t, "silent.mkv", "-f", "lavfi", "-i", "color=c=blue:s=16x16:r=10:d=0.3", "-c:v", "libx264")
	src, err := OpenFile(silent)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := src.(Audible); ok {
		t.Error("a clip without sound is Audible")
	}
	if err := src.Close(); err != nil {
		t.Error(err)
	}
	if err := src.(Seeker).Seek(0); !errors.Is(err, errClosed) {
		t.Errorf("Seek after Close: %v", err)
	}
	soundOnly := makeClip(t, "tone.wav", "-f", "lavfi", "-i", "sine=d=0.2")
	if _, err := OpenFile(soundOnly); err == nil || !strings.Contains(err.Error(), "no video stream") {
		t.Errorf("a sound file: %v", err)
	}
	if _, err := OpenFile(filepath.Join(t.TempDir(), "absent.mkv")); err == nil || !strings.Contains(err.Error(), "ffprobe") {
		t.Errorf("a missing file: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := OpenFile(silent); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("without ffmpeg on PATH: %v", err)
	}
}

func TestPCMReaderCountsWholeFramesAndEnds(t *testing.T) {
	o := &pulseOut{pcm: bytes.NewReader(make([]byte, 10*audioFrame+3))}
	r := pcmReader{o}
	buf := make([]byte, 6*audioFrame+1)
	if n, err := r.Read(buf); n != 6*audioFrame || err != nil {
		t.Fatalf("first read %d %v", n, err)
	}
	if n, err := r.Read(buf); n != 4*audioFrame || err != nil {
		t.Fatalf("short read %d %v", n, err)
	}
	if o.Drained() {
		t.Error("drained before the end was read")
	}
	if _, err := r.Read(buf); !errors.Is(err, pulse.EndOfData) || !o.Drained() {
		t.Errorf("at the end: %v drained %v", err, o.Drained())
	}
	if got := o.frames.Load(); got != 10 {
		t.Errorf("counted %d frames, want 10", got)
	}
	o.closed.Store(true)
	if _, err := r.Read(buf); !errors.Is(err, pulse.EndOfData) {
		t.Errorf("after close: %v", err)
	}
}

func TestPlayedSubtractsTheBufferedLatency(t *testing.T) {
	if got := playedFor(AudioRate); got != time.Second-audioLatency {
		t.Errorf("one second handed over plays as %v", got)
	}
	if got := playedFor(10); got != 0 {
		t.Errorf("a few frames in: %v, want 0 while buffering", got)
	}
}

func TestNoAudioServerIsAnError(t *testing.T) {
	t.Setenv("PULSE_SERVER", "unix:"+filepath.Join(t.TempDir(), "absent"))
	if _, err := openPulse(strings.NewReader(""), "test"); err == nil || !strings.Contains(err.Error(), "PulseAudio") {
		t.Errorf("without a server: %v", err)
	}
}
