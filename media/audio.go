package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"
	"time"

	"github.com/stubbedev/lys-lib/intconv"
	"github.com/stubbedev/lys-lib/pulse/native"
	"github.com/stubbedev/lys-lib/third_party/gomath"
)

const (
	audioLatency       = 50 * time.Millisecond
	audioClientTimeout = 2 * time.Second
	audioChunksPerSec  = 100
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

type openAudioFunc func(pcm io.Reader, format AudioFormat, name string) (audioOut, error)

func playedFor(frames int64, rate int) time.Duration {
	return max(time.Duration(mulDiv(frames, int64(time.Second), int64(rate)))-audioLatency, 0)
}

type pulseOut struct {
	conn    *native.Conn
	stream  *native.PlaybackStream
	pcm     io.Reader
	format  AudioFormat
	frames  atomic.Int64
	closed  atomic.Bool
	drained atomic.Bool
	failure atomic.Pointer[error]
}

func openPulse(pcm io.Reader, format AudioFormat, name string) (audioOut, error) {
	ctx := context.Background()
	c, err := native.Dial(ctx, native.Options{ClientName: name, Timeout: audioClientTimeout})
	if err != nil {
		return nil, fmt.Errorf("media: connect to the PulseAudio server: %w", err)
	}
	channels, err := intconv.Exact[uint8](len(format.Layout))
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("media: %d audio channels: %w", len(format.Layout), err)
	}
	rate, err := intconv.Exact[uint32](format.Rate)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("media: audio at %d Hz: %w", format.Rate, err)
	}
	st, err := c.CreatePlaybackStream(ctx, native.PlaybackParams{
		SampleSpec: native.SampleSpec{Format: native.SampleS16LE, Channels: channels, Rate: rate},
		ChannelMap: format.Layout,
		Latency:    audioLatency,
		Props:      native.PropList{"media.name": name},
	})
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("media: open a PulseAudio playback stream: %w", err)
	}
	o := &pulseOut{conn: c, stream: st, pcm: pcm, format: format}
	go o.feed()
	return o, nil
}

func (o *pulseOut) feed() {
	frame := o.format.frameBytes()
	buf := make([]byte, max(o.format.Rate/audioChunksPerSec, 1)*frame)
	for {
		n, err := io.ReadFull(o.pcm, buf)
		n -= n % frame
		if n > 0 {
			if werr := o.stream.Write(context.Background(), buf[:n]); werr != nil {
				o.fail(werr)
				return
			}
			o.frames.Add(int64(n / frame))
		}
		switch {
		case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
			o.drained.Store(true)
			return
		case err != nil:
			o.fail(fmt.Errorf("read the PCM: %w", err))
			return
		}
	}
}

func (o *pulseOut) fail(err error) {
	if o.closed.Load() {
		return
	}
	o.failure.CompareAndSwap(nil, &err)
}

func (o *pulseOut) Played() time.Duration { return playedFor(o.frames.Load(), o.format.Rate) }

func (o *pulseOut) Drained() bool { return o.drained.Load() }

func (o *pulseOut) Pause() { o.cork(true) }

func (o *pulseOut) Resume() { o.cork(false) }

func (o *pulseOut) cork(on bool) {
	if err := o.stream.Cork(context.Background(), on); err != nil {
		o.fail(err)
	}
}

func (o *pulseOut) SetVolume(linear float64) error {
	v := native.Volume(math.Round(gomath.Cbrt(linear) * float64(native.VolumeNorm)))
	if err := o.conn.SetSinkInputVolume(context.Background(), o.stream.Info().SinkInput, native.CVolume{v, v}); err != nil {
		return fmt.Errorf("media: set the stream volume: %w", err)
	}
	return nil
}

func (o *pulseOut) SetMuted(on bool) error {
	if err := o.conn.SetSinkInputMute(context.Background(), o.stream.Info().SinkInput, on); err != nil {
		return fmt.Errorf("media: mute the stream: %w", err)
	}
	return nil
}

func (o *pulseOut) Err() error {
	if err := o.failure.Load(); err != nil {
		return fmt.Errorf("media: audio playback: %w", *err)
	}
	return nil
}

func (o *pulseOut) Close() {
	o.closed.Store(true)
	_ = o.conn.Close()
}

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
