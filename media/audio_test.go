package media

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stubbedev/lys-lib/pulse/native"
	"github.com/stubbedev/lys-lib/pulse/pulsetest"
)

const firstRequest = AudioRate / 20 * audioFrame

type pulseHarness struct {
	srv *pulsetest.Server
	out *pulseOut
	p   *pulsetest.Playback
}

func openTestPulse(t *testing.T, pcm io.Reader) pulseHarness {
	t.Helper()
	srv := pulsetest.New(t)
	srv.PutSink(native.DeviceInfo{
		Index: 1, Name: "speakers", Description: "Speakers",
		SampleSpec: native.SampleSpec{Format: native.SampleS16LE, Channels: 2, Rate: AudioRate},
		ChannelMap: native.ChannelMap{native.ChannelFrontLeft, native.ChannelFrontRight},
		Volume:     native.CVolume{native.VolumeNorm, native.VolumeNorm}, Card: native.InvalidIndex,
	})
	srv.SetDefaults("speakers", "")
	t.Setenv("PULSE_SERVER", srv.Addr())
	out, err := openPulse(pcm, "clip")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(out.Close)
	return pulseHarness{srv: srv, out: out.(*pulseOut), p: srv.WaitPlaybacks(t, 1)[0]}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func pcmPattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 5)
	}
	return b
}

type endless struct{}

func (endless) Read(b []byte) (int, error) { return len(b), nil }

func TestPulseOutPlaysWholeFramesAsRequestedAndDrains(t *testing.T) {
	pcm := pcmPattern(firstRequest + 250*audioFrame + 3)
	h := openTestPulse(t, bytes.NewReader(pcm))
	if h.p.SampleSpec != (native.SampleSpec{Format: native.SampleS16LE, Channels: AudioChannels, Rate: AudioRate}) ||
		h.p.TargetLength != firstRequest || !h.p.AdjustLatency || h.p.Props["media.name"] != "clip" {
		t.Errorf("server saw %+v", h.p)
	}
	h.p.WaitData(t, firstRequest)
	eventually(t, "the first request counted", func() bool { return h.out.frames.Load() == firstRequest/audioFrame })
	if h.out.Drained() {
		t.Error("drained with audio still unrequested")
	}
	h.p.Request(250 * audioFrame)
	eventually(t, "drained", h.out.Drained)
	if got := h.p.Data(); !bytes.Equal(got, pcm[:len(pcm)-3]) {
		t.Errorf("server received %d bytes, want the %d of whole frames", len(got), len(pcm)-3)
	}
	if got := h.out.frames.Load(); got != firstRequest/audioFrame+250 {
		t.Errorf("counted %d frames", got)
	}
	if err := h.out.Err(); err != nil {
		t.Error(err)
	}
}

func TestPulseOutControlsItsSinkInput(t *testing.T) {
	h := openTestPulse(t, endless{})
	h.out.Pause()
	if !h.p.Corked() {
		t.Error("Pause left the stream running")
	}
	h.out.Resume()
	if h.p.Corked() {
		t.Error("Resume left the stream corked")
	}
	if err := h.out.SetVolume(0.125); err != nil {
		t.Fatal(err)
	}
	if err := h.out.SetMuted(true); err != nil {
		t.Fatal(err)
	}
	st, _ := h.srv.SinkInput(h.p.SinkInput)
	if half := native.VolumeNorm / 2; len(st.Volume) != 2 || st.Volume[0] != half || st.Volume[1] != half || !st.Mute {
		t.Errorf("sink input %+v, want both channels at half the raw volume and muted", st)
	}
	if err := h.out.Err(); err != nil {
		t.Error(err)
	}
}

func TestPulseOutReportsAKilledStream(t *testing.T) {
	h := openTestPulse(t, endless{})
	h.p.WaitData(t, firstRequest)
	h.p.Kill()
	eventually(t, "the kill reported", func() bool { return h.out.Err() != nil })
	if err := h.out.Err(); !errors.Is(err, native.ErrStreamKilled) {
		t.Errorf("Err = %v, want ErrStreamKilled", err)
	}
}

func TestPulseOutClosesQuietly(t *testing.T) {
	h := openTestPulse(t, endless{})
	h.p.WaitData(t, firstRequest)
	h.out.Close()
	h.srv.WaitPlaybacks(t, 0)
	if err := h.out.Err(); err != nil {
		t.Errorf("Close reported %v", err)
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
