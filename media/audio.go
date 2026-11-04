package media

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/jfreymuth/pulse"
	"github.com/jfreymuth/pulse/proto"
)

const (
	audioLatency       = 50 * time.Millisecond
	audioClientTimeout = 2 * time.Second
)

type audioOut interface {
	Played() time.Duration
	Pause()
	Resume()
	SetVolume(linear float64) error
	SetMuted(on bool) error
	Err() error
	Drained() bool
	Close()
}

type openAudioFunc func(pcm io.Reader, name string) (audioOut, error)

func playedFor(frames int64) time.Duration {
	return max(time.Duration(mulDiv(frames, int64(time.Second), AudioRate))-audioLatency, 0)
}

type pulseOut struct {
	client  *pulse.Client
	stream  *pulse.PlaybackStream
	pcm     io.Reader
	frames  atomic.Int64
	closed  atomic.Bool
	drained atomic.Bool
}

type pcmReader struct{ o *pulseOut }

func (pcmReader) Format() byte { return proto.FormatInt16LE }

func (r pcmReader) Read(b []byte) (int, error) {
	if r.o.closed.Load() {
		return 0, pulse.EndOfData
	}
	b = b[:len(b)/audioFrame*audioFrame]
	n, err := io.ReadFull(r.o.pcm, b)
	n = n / audioFrame * audioFrame
	r.o.frames.Add(int64(n / audioFrame))
	switch {
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		if n > 0 {
			return n, nil
		}
		r.o.drained.Store(true)
		return 0, pulse.EndOfData
	case err != nil && r.o.closed.Load():
		return 0, pulse.EndOfData
	}
	return n, err
}

func openPulse(pcm io.Reader, name string) (audioOut, error) {
	c, err := pulse.NewClient(pulse.ClientApplicationName(name), pulse.ClientTimeout(audioClientTimeout))
	if err != nil {
		return nil, fmt.Errorf("media: connect to the PulseAudio server: %w", err)
	}
	o := &pulseOut{client: c, pcm: pcm}
	st, err := c.NewPlayback(pcmReader{o},
		pulse.PlaybackStereo, pulse.PlaybackSampleRate(AudioRate),
		pulse.PlaybackLatency(audioLatency.Seconds()), pulse.PlaybackMediaName(name))
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("media: open a PulseAudio playback stream: %w", err)
	}
	o.stream = st
	st.Start()
	return o, nil
}

func (o *pulseOut) Played() time.Duration { return playedFor(o.frames.Load()) }

func (o *pulseOut) Drained() bool { return o.drained.Load() }

func (o *pulseOut) Pause() { o.stream.Pause() }

func (o *pulseOut) Resume() { o.stream.Resume() }

func (o *pulseOut) SetVolume(linear float64) error {
	v := proto.LinearVolume(linear)
	if err := o.stream.SetVolume(proto.ChannelVolumes{v, v}); err != nil {
		return fmt.Errorf("media: set the stream volume: %w", err)
	}
	return nil
}

func (o *pulseOut) SetMuted(on bool) error {
	if err := o.client.RawRequest(&proto.SetSinkInputMute{SinkInputIndex: o.stream.StreamInputIndex(), Mute: on}, nil); err != nil {
		return fmt.Errorf("media: mute the stream: %w", err)
	}
	return nil
}

func (o *pulseOut) Err() error {
	if err := o.stream.Error(); err != nil {
		return fmt.Errorf("media: audio playback: %w", err)
	}
	return nil
}

func (o *pulseOut) Close() {
	o.closed.Store(true)
	o.stream.Close()
	o.client.Close()
}

// audioLink is one opening of the sound, driven from the stream's audio
// worker and read from the loop. Everything that talks to the sound
// server or forks the decoder runs on the worker; the loop reads only
// the atomics behind these methods.
type audioLink struct {
	ready   atomic.Pointer[openedAudio]
	failure atomic.Pointer[error]
}

type openedAudio struct {
	out audioOut
	pcm io.ReadCloser
}

func (l *audioLink) fail(err error) {
	if err != nil {
		l.failure.CompareAndSwap(nil, &err)
	}
}

// played is how far the sound got, zero until it opened.
func (l *audioLink) played() time.Duration {
	if o := l.ready.Load(); o != nil {
		return o.out.Played()
	}
	return 0
}

func (l *audioLink) drained() bool {
	o := l.ready.Load()
	return o != nil && o.out.Drained()
}

func (l *audioLink) err() error {
	if err := l.failure.Load(); err != nil {
		return *err
	}
	if o := l.ready.Load(); o != nil {
		return o.out.Err()
	}
	return nil
}
