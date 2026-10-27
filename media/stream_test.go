package media

import (
	"errors"
	"image"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

const fakeStep = 40 * time.Millisecond

type fakeSource struct {
	frames int
	failAt int
	mu     sync.Mutex
	next   int
	closed bool
}

func (f *fakeSource) Info() Info {
	return Info{Width: 2, Height: 2, Duration: time.Duration(f.frames) * fakeStep}
}

func (f *fakeSource) ReadFrame(dst *image.RGBA) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case f.closed:
		return 0, errClosed
	case f.next == f.failAt:
		return 0, errors.New("corrupt frame")
	case f.next >= f.frames:
		return 0, io.EOF
	}
	dst.Pix[0] = byte(f.next)
	pts := time.Duration(f.next) * fakeStep
	f.next++
	return pts, nil
}

func (f *fakeSource) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

type seekableFake struct{ *fakeSource }

func (f seekableFake) Seek(t time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next = min(int(t/fakeStep), f.frames-1)
	return nil
}

type audibleFake struct {
	seekableFake
	opened *[]time.Duration
}

type fakePCM struct{ io.Reader }

func (fakePCM) Close() error { return nil }

func (f audibleFake) OpenAudio(t time.Duration) (io.ReadCloser, error) {
	*f.opened = append(*f.opened, t)
	return fakePCM{strings.NewReader("")}, nil
}

type fakeOut struct {
	played         time.Duration
	drained        bool
	err            error
	paused, closed bool
	resumed        int
	volumes        []float64
	muted          []bool
}

func (o *fakeOut) Played() time.Duration { return o.played }
func (o *fakeOut) Pause()                { o.paused = true }
func (o *fakeOut) Resume()               { o.paused = false; o.resumed++ }
func (o *fakeOut) SetVolume(v float64) error {
	o.volumes = append(o.volumes, v)
	return nil
}

func (o *fakeOut) SetMuted(on bool) error {
	o.muted = append(o.muted, on)
	return nil
}
func (o *fakeOut) Err() error    { return o.err }
func (o *fakeOut) Drained() bool { return o.drained }
func (o *fakeOut) Close()        { o.closed = true }

type outs struct{ all []*fakeOut }

func (o *outs) open(io.Reader, string) (audioOut, error) {
	out := &fakeOut{}
	o.all = append(o.all, out)
	return out, nil
}

func (o *outs) last() *fakeOut { return o.all[len(o.all)-1] }

func noAudio(io.Reader, string) (audioOut, error) { panic("a silent source opened audio") }

type harness struct {
	t       *testing.T
	s       *Stream
	notices int
}

func play(t *testing.T, src Source, open openAudioFunc, body func(h *harness)) {
	synctest.Test(t, func(t *testing.T) {
		defer animclock.Reset()
		h := &harness{t: t, s: newStream(src, "test", open)}
		h.s.Subscribe(func() { h.notices++ })
		h.step(0)
		body(h)
		if err := h.s.Close(); err != nil {
			t.Error(err)
		}
	})
}

func (h *harness) step(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
	h.s.advance(time.Now())
}

func (h *harness) settle() {
	for range 50 {
		h.step(0)
	}
}

func (h *harness) frame() int {
	h.t.Helper()
	img := h.s.Frame()
	if img.RGBA == nil {
		h.t.Fatal("no frame")
	}
	return int(img.Pix[0])
}

func (h *harness) want(frame int, pos time.Duration) {
	h.t.Helper()
	if got := h.frame(); got != frame || h.s.Position() != pos {
		h.t.Errorf("frame %d at %v, want frame %d at %v", got, h.s.Position(), frame, pos)
	}
}

func TestStreamPlaysPausesSeeksAndEnds(t *testing.T) {
	src := seekableFake{&fakeSource{frames: 10, failAt: -1}}
	play(t, src, noAudio, func(h *harness) {
		s := h.s
		h.want(0, 0)
		if s.Playing() || s.ticker != nil {
			t.Error("a new stream plays or keeps ticking once its first frame is up")
		}
		s.Play()
		h.step(0)
		h.step(100 * time.Millisecond)
		h.want(2, 100*time.Millisecond)

		s.Pause()
		h.step(time.Second)
		h.want(2, 100*time.Millisecond)

		if err := s.Seek(200 * time.Millisecond); err != nil {
			t.Fatal(err)
		}
		h.step(0)
		h.want(5, 200*time.Millisecond)
		if s.Playing() {
			t.Error("a seek while paused started playback")
		}

		s.Play()
		h.step(0)
		h.step(time.Second)
		h.settle()
		if !s.Ended() || s.Playing() {
			t.Fatalf("past the end: ended %v playing %v", s.Ended(), s.Playing())
		}
		h.want(9, 400*time.Millisecond)

		s.Play()
		h.settle()
		h.want(0, 0)
		if !s.Playing() || s.Ended() {
			t.Error("Play at the end did not start over")
		}
		if h.notices == 0 {
			t.Error("subscribers heard nothing")
		}
	})
	if !src.closed {
		t.Error("Close left the source open")
	}
}

func TestStreamLoops(t *testing.T) {
	play(t, seekableFake{&fakeSource{frames: 5, failAt: -1}}, noAudio, func(h *harness) {
		h.s.SetLoop(true)
		h.s.Play()
		h.step(0)
		for range 10 {
			h.step(50 * time.Millisecond)
		}
		if !h.s.Playing() || h.s.Ended() || h.s.Position() >= 200*time.Millisecond {
			t.Errorf("a looping stream: playing %v ended %v at %v", h.s.Playing(), h.s.Ended(), h.s.Position())
		}
	})
}

func TestStreamWithoutSeekingEndsForGood(t *testing.T) {
	play(t, &fakeSource{frames: 3, failAt: -1}, noAudio, func(h *harness) {
		s := h.s
		if s.Seekable() || !errors.Is(s.Seek(0), ErrNotSeekable) {
			t.Error("an unseekable source seeks")
		}
		s.SetLoop(true)
		if s.Loop() {
			t.Error("an unseekable source loops")
		}
		s.Play()
		h.step(0)
		h.step(time.Second)
		h.settle()
		s.Play()
		if !s.Ended() || s.Playing() {
			t.Error("Play restarted a stream that cannot seek back")
		}
	})
}

func TestStreamStopsOnADecodeError(t *testing.T) {
	play(t, seekableFake{&fakeSource{frames: 10, failAt: 3}}, noAudio, func(h *harness) {
		h.s.Play()
		h.step(0)
		h.settle()
		if err := h.s.Err(); err == nil || !strings.Contains(err.Error(), "corrupt frame") {
			t.Fatalf("Err %v", err)
		}
		h.s.Play()
		if h.s.Playing() || h.s.active() {
			t.Error("a failed stream plays or ticks")
		}
	})
}

func TestStreamFollowsTheAudioClock(t *testing.T) {
	var opened []time.Duration
	o := &outs{}
	src := audibleFake{seekableFake{&fakeSource{frames: 25, failAt: -1}}, &opened}
	play(t, src, o.open, func(h *harness) {
		s := h.s
		if !s.HasAudio() {
			t.Fatal("an audible source is silent")
		}
		s.Play()
		out := o.last()
		out.played = 300 * time.Millisecond
		h.step(5 * time.Second)
		h.settle()
		h.want(7, 300*time.Millisecond)

		s.Pause()
		s.Play()
		if out.paused || out.resumed != 1 || len(opened) != 1 {
			t.Errorf("pause and play: paused %v resumed %d opened %v", out.paused, out.resumed, opened)
		}

		s.SetVolume(0.5)
		s.SetMuted(true)
		if err := s.Seek(100 * time.Millisecond); err != nil {
			t.Fatal(err)
		}
		next := o.last()
		if !out.closed || next == out || len(opened) != 2 || opened[1] != 100*time.Millisecond {
			t.Fatalf("seek while playing did not restart the audio at 100ms: %v", opened)
		}
		if len(next.volumes) != 1 || next.volumes[0] != 0.5 || len(next.muted) != 1 || !next.muted[0] {
			t.Errorf("the new output did not get the volume and mute: %v %v", next.volumes, next.muted)
		}
		if len(out.volumes) != 1 {
			t.Error("the first output never heard the volume")
		}

		next.played, next.drained = 200*time.Millisecond, true
		h.step(0)
		if !next.closed || s.Position() != 300*time.Millisecond {
			t.Errorf("drained audio: closed %v at %v, want 300ms", next.closed, s.Position())
		}
		h.step(80 * time.Millisecond)
		if s.Position() != 380*time.Millisecond {
			t.Errorf("after the audio drained the clock reads %v, want 380ms on the frame clock", s.Position())
		}

		s.Pause()
		if err := s.Seek(0); err != nil {
			t.Fatal(err)
		}
		s.Play()
		o.last().err = errors.New("server gone")
		h.step(0)
		if err := s.Err(); err == nil || s.Playing() || !o.last().closed {
			t.Errorf("an audio failure: err %v playing %v", err, s.Playing())
		}
	})
}

func TestWithoutAudioHidesTheTrack(t *testing.T) {
	var opened []time.Duration
	src := WithoutAudio(audibleFake{seekableFake{&fakeSource{frames: 3, failAt: -1}}, &opened})
	if _, ok := src.(Audible); ok {
		t.Error("WithoutAudio kept the track")
	}
	if _, ok := src.(Seeker); !ok {
		t.Error("WithoutAudio lost seeking")
	}
	if _, ok := WithoutAudio(&fakeSource{}).(Seeker); ok {
		t.Error("WithoutAudio made an unseekable source seek")
	}
}

func TestClosedStreamRefusesWork(t *testing.T) {
	play(t, seekableFake{&fakeSource{frames: 3, failAt: -1}}, noAudio, func(h *harness) {
		cancel := h.s.Subscribe(func() { t.Error("a cancelled subscriber heard a change") })
		cancel()
		if err := h.s.Close(); err != nil {
			t.Fatal(err)
		}
		if err := h.s.Seek(0); !errors.Is(err, errStreamClosed) {
			t.Errorf("Seek after Close: %v", err)
		}
		h.s.Play()
		if h.s.Playing() {
			t.Error("a closed stream plays")
		}
	})
}
