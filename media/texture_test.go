package media

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/media/demux"
)

const (
	fakePackets = 50
	fakeGOP     = 5
)

type fakeDecoder struct {
	dev     *gpu.Device
	decoded []time.Duration
	ready   int
	busy    bool
	busyErr error
	lost    error
	closed  bool
}

func (d *fakeDecoder) Busy() (bool, error) { return d.busy, d.busyErr }

func (d *fakeDecoder) Decode(p demux.Packet) error {
	switch {
	case d.closed:
		return errors.New("decode on a closed decoder")
	case d.lost != nil:
		return d.lost
	}
	d.decoded = append(d.decoded, p.PTS)
	d.ready++
	return nil
}

func (d *fakeDecoder) Next() *gpu.VideoFrame {
	if d.closed || d.ready == 0 {
		return nil
	}
	d.ready--
	return &gpu.VideoFrame{}
}

func (d *fakeDecoder) Flush() {}

func (d *fakeDecoder) Reset() error {
	d.ready = 0
	return d.lost
}

func (d *fakeDecoder) Close() { d.closed = true }

type fakeTextures struct {
	refuse error

	mu       sync.Mutex
	next     int
	seeks    []time.Duration
	decoders []*fakeDecoder
}

func (f *fakeTextures) Info() Info {
	return Info{Width: 2, Height: 2, Duration: fakePackets * fakeStep}
}

func (f *fakeTextures) Close() error { return nil }

func (f *fakeTextures) Seek(t time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seeks = append(f.seeks, t)
	f.next = int(t/fakeStep) / fakeGOP * fakeGOP
	return nil
}

func (f *fakeTextures) ReadPacket() (demux.Packet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.next >= fakePackets {
		return demux.Packet{}, io.EOF
	}
	p := demux.Packet{PTS: time.Duration(f.next) * fakeStep, Duration: fakeStep, Key: f.next%fakeGOP == 0}
	f.next++
	return p, nil
}

func (f *fakeTextures) NewVideoDecoder(dev *gpu.Device) (VideoDecoder, error) {
	if f.refuse != nil {
		return nil, f.refuse
	}
	d := &fakeDecoder{dev: dev}
	f.decoders = append(f.decoders, d)
	return d, nil
}

func (f *fakeTextures) sought() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.seeks)
}

type audibleTextures struct {
	*fakeTextures
	opened *[]time.Duration
}

func (audibleTextures) AudioFormat() AudioFormat { return stereo }

func (f audibleTextures) OpenAudio(t time.Duration) (io.ReadCloser, error) {
	*f.opened = append(*f.opened, t)
	return fakePCM{io.LimitReader(nil, 0)}, nil
}

type losses struct{ errs []error }

func (l *losses) lost(err error) { l.errs = append(l.errs, err) }

type fakeDecodes struct {
	pending bool
	lost    error
}

func (d *fakeDecodes) decoded(*gpu.VideoFrame) (bool, error) { return !d.pending, d.lost }

func playTextures(t *testing.T, src Source, open openAudioFunc, body func(h *harness, decodes *fakeDecodes)) {
	play(t, src, open, func(h *harness) {
		decodes := &fakeDecodes{}
		h.s.tex.decoded = decodes.decoded
		body(h, decodes)
	})
}

var errFakeLoss = fmt.Errorf("fake decoder: %w", gpu.ErrDeviceLost)

func TestATextureStreamMovesItsDecodingToAnotherDevice(t *testing.T) {
	var opened []time.Duration
	o := &outs{}
	src := &fakeTextures{}
	first, second := &gpu.Device{}, &gpu.Device{}
	reported := &losses{}
	playTextures(t, audibleTextures{src, &opened}, o.open, func(h *harness, _ *fakeDecodes) {
		s := h.s
		if err := s.UseDevice(first, reported.lost); err != nil {
			t.Fatal(err)
		}
		h.settle()
		if s.Texture() == nil {
			t.Fatal("the first device decoded no frame")
		}
		s.Play()
		synctest.Wait()
		out := o.last()
		out.played = 500 * time.Millisecond
		h.step(fakeStep)
		seq := s.FrameSeq()

		s.ReleaseDevice()
		old := src.decoders[0]
		if !old.closed || s.Texture() != nil {
			t.Fatalf("after letting go of the device: decoder closed %v, frame %v", old.closed, s.Texture())
		}
		out.played = 620 * time.Millisecond
		h.step(fakeStep)
		h.settle()
		if !s.Playing() || s.Ended() || s.Err() != nil || s.Position() != 620*time.Millisecond {
			t.Fatalf("between two devices: playing %v, ended %v, error %v, at %v; want the stream playing on at 620ms", s.Playing(), s.Ended(), s.Err(), s.Position())
		}
		if len(old.decoded) == 0 || old.decoded[0] != 0 {
			t.Fatalf("the first device decoded from %v, want from the start", old.decoded)
		}
		decodedOnFirst := len(old.decoded)

		if err := s.UseDevice(second, reported.lost); err != nil {
			t.Fatal(err)
		}
		h.settle()
		if len(src.decoders) != 2 || src.decoders[1].dev != second {
			t.Fatalf("%d decoders opened, want a second one on the new device", len(src.decoders))
		}
		next := src.decoders[1]
		if got := src.sought(); len(got) != 1 || got[0] != 620*time.Millisecond {
			t.Errorf("the source was sought to %v, want once to the position, 620ms", got)
		}
		if len(next.decoded) == 0 || next.decoded[0] != 600*time.Millisecond {
			t.Errorf("the new device decoded from %v, want from the keyframe at 600ms", next.decoded)
		}
		if len(old.decoded) != decodedOnFirst {
			t.Errorf("the closed decoder took %d more packets", len(old.decoded)-decodedOnFirst)
		}
		if s.Texture() == nil || s.FrameSeq() == seq {
			t.Errorf("no frame shown on the new device: frame %v, sequence %d after %d", s.Texture(), s.FrameSeq(), seq)
		}
		if !s.Playing() || s.Err() != nil || s.Position() != 620*time.Millisecond {
			t.Errorf("on the new device: playing %v, error %v, at %v", s.Playing(), s.Err(), s.Position())
		}
		if out.closed || len(o.all) != 1 || len(opened) != 1 {
			t.Errorf("the sound was reopened: closed %v, %d outputs, opened at %v", out.closed, len(o.all), opened)
		}
		if len(reported.errs) != 0 {
			t.Errorf("a device let go of was reported lost: %v", reported.errs)
		}
	})
}

func TestADeviceLostWhileDecodingIsReportedAndReplaced(t *testing.T) {
	for _, c := range []struct {
		name string
		lose func(h *harness, dec *fakeDecoder, decodes *fakeDecodes)
	}{
		{"decoding a packet", func(h *harness, dec *fakeDecoder, _ *fakeDecodes) {
			h.step(0)
			h.s.Play()
			h.step(0)
			h.step(2 * fakeStep)
			dec.lost = errFakeLoss
			h.step(fakeStep)
		}},
		{"asking whether the decoder is busy", func(h *harness, dec *fakeDecoder, _ *fakeDecodes) {
			h.step(0)
			h.s.Play()
			h.step(0)
			h.step(2 * fakeStep)
			dec.busyErr = errFakeLoss
			h.step(fakeStep)
		}},
		{"seeking", func(h *harness, dec *fakeDecoder, _ *fakeDecodes) {
			h.settle()
			h.s.Play()
			h.step(0)
			dec.lost = errFakeLoss
			if err := h.s.Seek(10 * fakeStep); err != nil {
				h.t.Fatal(err)
			}
		}},
		{"asking whether a frame has decoded", func(h *harness, _ *fakeDecoder, decodes *fakeDecodes) {
			h.settle()
			h.s.Play()
			h.step(0)
			if err := h.s.Seek(10 * fakeStep); err != nil {
				h.t.Fatal(err)
			}
			decodes.lost = errFakeLoss
			h.settle()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := &fakeTextures{}
			first, second := &gpu.Device{}, &gpu.Device{}
			reported := &losses{}
			playTextures(t, src, noAudio, func(h *harness, decodes *fakeDecodes) {
				h.t = t
				s := h.s
				if err := s.UseDevice(first, reported.lost); err != nil {
					t.Fatal(err)
				}
				old := src.decoders[0]
				c.lose(h, old, decodes)
				if len(reported.errs) != 1 || !errors.Is(reported.errs[0], gpu.ErrDeviceLost) {
					t.Fatalf("the loss was reported as %v, want once", reported.errs)
				}
				if !old.closed || s.Texture() != nil || s.Err() != nil || !s.Playing() {
					t.Fatalf("after the loss: decoder closed %v, frame %v, error %v, playing %v; want the stream playing on with no device", old.closed, s.Texture(), s.Err(), s.Playing())
				}
				h.step(fakeStep)
				if len(reported.errs) != 1 {
					t.Errorf("one loss was reported %d times", len(reported.errs))
				}
				at, sought := s.Position(), len(src.sought())
				decodes.lost = nil

				if err := s.UseDevice(second, reported.lost); err != nil {
					t.Fatal(err)
				}
				h.settle()
				next := src.decoders[1]
				key := at / (fakeGOP * fakeStep) * (fakeGOP * fakeStep)
				if got := src.sought(); len(got) != sought+1 || got[sought] != at {
					t.Errorf("the source was sought to %v, want last to the position, %v", got, at)
				}
				if next.dev != second || len(next.decoded) == 0 || next.decoded[0] != key {
					t.Errorf("the new device decoded from %v, want from the keyframe at %v", next.decoded, key)
				}
				if s.Err() != nil || !s.Playing() {
					t.Errorf("on the new device: error %v, playing %v", s.Err(), s.Playing())
				}
			})
		})
	}
}

func TestAFrameStillDecodingIsShownOnceItHasDecoded(t *testing.T) {
	src := &fakeTextures{}
	playTextures(t, src, noAudio, func(h *harness, decodes *fakeDecodes) {
		s := h.s
		decodes.pending = true
		if err := s.UseDevice(&gpu.Device{}, nil); err != nil {
			t.Fatal(err)
		}
		h.settle()
		if s.Texture() != nil || s.FrameSeq() != 0 {
			t.Fatalf("a frame still decoding is shown: %v, sequence %d", s.Texture(), s.FrameSeq())
		}
		if s.Err() != nil || s.ticker == nil {
			t.Fatalf("waiting for the decode: error %v, ticking %v; want the paused stream ticking until its frame is there", s.Err(), s.ticker != nil)
		}
		decodes.pending = false
		h.step(0)
		if s.Texture() == nil || s.FrameSeq() != 1 {
			t.Fatalf("once decoded: frame %v, sequence %d; want it shown", s.Texture(), s.FrameSeq())
		}
		if s.ticker != nil {
			t.Error("the paused stream keeps ticking with its frame shown")
		}
	})
}

func TestADeviceLostWithNoOneToReplaceItFailsTheStream(t *testing.T) {
	src := &fakeTextures{}
	playTextures(t, src, noAudio, func(h *harness, _ *fakeDecodes) {
		s := h.s
		if err := s.UseDevice(&gpu.Device{}, nil); err != nil {
			t.Fatal(err)
		}
		h.step(0)
		s.Play()
		src.decoders[0].lost = errFakeLoss
		h.step(fakeStep)
		if !errors.Is(s.Err(), gpu.ErrDeviceLost) || s.Playing() {
			t.Errorf("error %v, playing %v; want the stream failed by the loss", s.Err(), s.Playing())
		}
	})
}

func TestANewDeviceThatDecodesNoneOfTheVideoFailsTheStream(t *testing.T) {
	src := &fakeTextures{}
	reported := &losses{}
	playTextures(t, src, noAudio, func(h *harness, _ *fakeDecodes) {
		s := h.s
		if err := s.UseDevice(&gpu.Device{}, reported.lost); err != nil {
			t.Fatal(err)
		}
		h.settle()
		s.ReleaseDevice()
		src.refuse = fmt.Errorf("fake source: %w", gpu.ErrNoVideoDecoder)
		err := s.UseDevice(&gpu.Device{}, reported.lost)
		if !errors.Is(err, gpu.ErrNoVideoDecoder) || !errors.Is(s.Err(), gpu.ErrNoVideoDecoder) {
			t.Errorf("UseDevice = %v, stream error %v; want the new device's refusal", err, s.Err())
		}
		if len(reported.errs) != 0 {
			t.Errorf("a refusal was reported as a loss: %v", reported.errs)
		}
	})
}

func TestNamingAnotherDeviceLetsGoOfTheOneBefore(t *testing.T) {
	src := &fakeTextures{}
	first, second := &gpu.Device{}, &gpu.Device{}
	playTextures(t, src, noAudio, func(h *harness, _ *fakeDecodes) {
		s := h.s
		if err := s.UseDevice(first, nil); err != nil {
			t.Fatal(err)
		}
		h.settle()
		if err := s.UseDevice(first, nil); err != nil || len(src.decoders) != 1 || src.decoders[0].closed {
			t.Fatalf("naming the same device again: %v, %d decoders, the first closed %v; want it kept", err, len(src.decoders), src.decoders[0].closed)
		}
		if err := s.UseDevice(second, nil); err != nil {
			t.Fatal(err)
		}
		h.settle()
		if len(src.decoders) != 2 || !src.decoders[0].closed || src.decoders[1].dev != second || src.decoders[1].closed {
			t.Fatalf("%d decoders, the first closed %v; want the first closed and one open on the second device", len(src.decoders), src.decoders[0].closed)
		}
		if s.Err() != nil || s.Texture() == nil {
			t.Errorf("on the second device: error %v, frame %v", s.Err(), s.Texture())
		}
	})
}

func TestABusyDecoderIsGivenNoPacketUntilItHasCaughtUp(t *testing.T) {
	src := &fakeTextures{}
	playTextures(t, src, noAudio, func(h *harness, _ *fakeDecodes) {
		s := h.s
		if err := s.UseDevice(&gpu.Device{}, nil); err != nil {
			t.Fatal(err)
		}
		dec := src.decoders[0]
		dec.busy = true
		h.settle()
		if len(dec.decoded) != 0 {
			t.Fatalf("a busy decoder was given the packets at %v", dec.decoded)
		}
		if s.Err() != nil || s.ticker == nil {
			t.Fatalf("with its decoder busy: error %v, ticking %v; want the stream ticking until its frame is there", s.Err(), s.ticker != nil)
		}
		dec.busy = false
		h.step(0)
		if len(dec.decoded) == 0 || dec.decoded[0] != 0 {
			t.Errorf("once it caught up the decoder took %v, want the packets from the start", dec.decoded)
		}
		h.settle()
		if s.Texture() == nil {
			t.Error("no frame shown after the decoder caught up")
		}
	})
}

func TestADecoderThatCannotSayWhetherItIsBusyFailsTheStream(t *testing.T) {
	src := &fakeTextures{}
	broken := errors.New("fake decoder: closed")
	playTextures(t, src, noAudio, func(h *harness, _ *fakeDecodes) {
		s := h.s
		if err := s.UseDevice(&gpu.Device{}, nil); err != nil {
			t.Fatal(err)
		}
		src.decoders[0].busyErr = broken
		h.step(0)
		if !errors.Is(s.Err(), broken) || len(src.decoders[0].decoded) != 0 {
			t.Errorf("stream error %v after %d packets, want the decoder's failure before any packet", s.Err(), len(src.decoders[0].decoded))
		}
	})
}
