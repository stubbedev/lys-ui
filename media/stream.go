package media

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/internal/animclock"
)

// ErrNotSeekable reports a Seek on a Stream whose source cannot seek.
var ErrNotSeekable = errors.New("media: the stream cannot seek")

var errStreamClosed = errors.New("media: the stream is closed")

const tickSpan = time.Hour

type subscription struct {
	id int
	fn func()
}

// Stream plays a Source, GTK's GtkMediaStream: it runs the decoder on
// its own goroutine a few frames ahead, presents the frame due at the
// playback position on the loop's frame clock, and, when the source
// is Audible, plays the sound through PulseAudio and follows its clock
// so picture and sound stay together. Every method runs on the loop
// goroutine. Subscribers hear every change: the frame, the position
// while playing, and the state.
type Stream struct {
	name    string
	src     Source
	info    Info
	seeker  Seeker
	audible Audible
	openOut openAudioFunc
	dec     *decoder

	playing, ended, loop, closed bool

	pos       time.Duration
	anchorAt  time.Time
	anchorPos time.Duration

	gen, curGen, eofGen int
	cur                 *frameBuf
	frameSeq            uint64
	queue               []decoded
	lastPTS, prevPTS    time.Duration

	audio     audioOut
	pcm       io.ReadCloser
	audioPos  time.Duration
	volume    float64
	volumeSet bool
	muted     bool

	err     error
	subs    []subscription
	nextSub int
	ticker  func()
}

// NewStream plays src, named name in the desktop's volume mixer. It
// starts paused and decodes the first frame right away; Close releases
// the source and its decoders.
func NewStream(src Source, name string) *Stream {
	return newStream(src, name, openPulse)
}

func newStream(src Source, name string, openOut openAudioFunc) *Stream {
	s := &Stream{name: name, src: src, info: src.Info(), openOut: openOut, curGen: -1, eofGen: -1, volume: 1}
	s.seeker, _ = src.(Seeker)
	s.audible, _ = src.(Audible)
	s.dec = startDecoder(src, s.seeker, s.info)
	s.sync()
	return s
}

// Size returns the frame size in pixels.
func (s *Stream) Size() (w, h int) { return s.info.Width, s.info.Height }

// Duration returns the length, zero when unknown.
func (s *Stream) Duration() time.Duration { return s.info.Duration }

// Seekable reports whether Seek works.
func (s *Stream) Seekable() bool { return s.seeker != nil }

// HasAudio reports whether the stream plays sound.
func (s *Stream) HasAudio() bool { return s.audible != nil }

// Playing reports whether the stream is playing.
func (s *Stream) Playing() bool { return s.playing }

// Ended reports whether playback reached the end (and is not looping).
func (s *Stream) Ended() bool { return s.ended }

// Position returns the playback position.
func (s *Stream) Position() time.Duration { return s.pos }

// Loop reports whether playback restarts at the end.
func (s *Stream) Loop() bool { return s.loop }

// SetLoop makes playback restart at the end; it needs a seekable
// source and is ignored otherwise.
func (s *Stream) SetLoop(on bool) {
	s.loop = on && s.seeker != nil
	s.notify()
}

// Volume returns the volume set through SetVolume, 1 until then.
func (s *Stream) Volume() float64 { return s.volume }

// SetVolume sets the volume, 0 to 1 on a perceptual scale. Until it is
// called the desktop picks the stream's volume.
func (s *Stream) SetVolume(v float64) {
	s.volume, s.volumeSet = min(max(v, 0), 1), true
	if s.audio != nil {
		s.check(s.audio.SetVolume(s.volume))
	}
	s.notify()
}

// Muted reports whether the sound is muted.
func (s *Stream) Muted() bool { return s.muted }

// SetMuted mutes or unmutes the sound.
func (s *Stream) SetMuted(on bool) {
	s.muted = on
	if s.audio != nil {
		s.check(s.audio.SetMuted(on))
	}
	s.notify()
}

// Closed reports whether Close was called.
func (s *Stream) Closed() bool { return s.closed }

// Err returns the failure that stopped the stream, nil while healthy.
func (s *Stream) Err() error { return s.err }

// Frame returns the frame to show, its RGBA nil before the first is
// decoded: a pooled buffer's pixels under the buffer's identity, at a
// version that changes whenever the frame does. The image belongs to
// the stream and stays valid until the next change.
func (s *Stream) Frame() render.VersionedImage {
	if s.cur == nil {
		return render.VersionedImage{Version: s.frameSeq}
	}
	return render.VersionedImage{RGBA: s.cur.img, ID: s.cur.id, Version: s.frameSeq}
}

// Subscribe calls fn on the loop after every change; cancel stops it.
func (s *Stream) Subscribe(fn func()) (cancel func()) {
	s.nextSub++
	id := s.nextSub
	s.subs = append(s.subs, subscription{id: id, fn: fn})
	return func() {
		s.subs = slices.DeleteFunc(s.subs, func(sub subscription) bool { return sub.id == id })
	}
}

// Play starts or resumes playback; at the end it starts over.
func (s *Stream) Play() {
	if s.closed || s.err != nil || s.playing {
		return
	}
	if s.ended {
		if s.seeker == nil {
			return
		}
		s.seekTo(0)
	}
	s.playing = true
	s.anchorAt = time.Time{}
	s.startAudio()
	s.notify()
	s.sync()
}

// Pause pauses playback at the current position.
func (s *Stream) Pause() {
	if !s.playing {
		return
	}
	s.advanceClock(animclock.Now())
	s.playing = false
	if s.audio != nil {
		s.audio.Pause()
	}
	s.notify()
	s.sync()
}

// Seek moves playback to t, clamped to the stream. Playback continues
// from there if it was playing.
func (s *Stream) Seek(t time.Duration) error {
	if s.seeker == nil {
		return ErrNotSeekable
	}
	if s.closed {
		return errStreamClosed
	}
	if s.err != nil {
		return s.err
	}
	s.seekTo(t)
	s.notify()
	s.sync()
	return nil
}

// Close stops playback and releases the source and decoders.
func (s *Stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed, s.playing = true, false
	s.stopAudio()
	s.sync()
	err := s.dec.close()
	s.notify()
	if err != nil {
		return fmt.Errorf("media: close %s: %w", s.name, err)
	}
	return nil
}

func (s *Stream) seekTo(t time.Duration) {
	t = max(t, 0)
	if s.info.Duration > 0 {
		t = min(t, s.info.Duration)
	}
	s.gen++
	for _, d := range s.queue {
		s.dec.recycle(d.img)
	}
	s.queue = s.queue[:0]
	s.lastPTS, s.prevPTS = 0, 0
	s.dec.requestSeek(seekRequest{gen: s.gen, t: t})
	s.pos, s.anchorAt, s.ended = t, time.Time{}, false
	if s.audio != nil {
		s.stopAudio()
		if s.playing {
			s.startAudio()
		}
	}
}

func (s *Stream) startAudio() {
	if s.audible == nil {
		return
	}
	if s.audio != nil {
		s.audio.Resume()
		return
	}
	pcm, err := s.audible.OpenAudio(s.pos)
	if err != nil {
		s.fail(err)
		return
	}
	out, err := s.openOut(pcm, s.name)
	if err != nil {
		_ = pcm.Close()
		s.fail(err)
		return
	}
	s.pcm, s.audio, s.audioPos = pcm, out, s.pos
	if s.volumeSet {
		s.check(out.SetVolume(s.volume))
	}
	if s.muted {
		s.check(out.SetMuted(true))
	}
}

func (s *Stream) stopAudio() {
	if s.audio == nil {
		return
	}
	s.audio.Close()
	_ = s.pcm.Close()
	s.audio, s.pcm = nil, nil
}

func (s *Stream) check(err error) {
	if err != nil {
		s.fail(err)
	}
}

func (s *Stream) fail(err error) {
	if s.err == nil {
		s.err = err
	}
	s.playing = false
	s.stopAudio()
}

func (s *Stream) end() time.Duration {
	if s.info.Duration > 0 {
		return s.info.Duration
	}
	return s.lastPTS + max(s.lastPTS-s.prevPTS, 0)
}

func (s *Stream) collect() {
	for {
		select {
		case d := <-s.dec.out:
			switch {
			case d.gen != s.gen:
				s.dec.recycle(d.img)
			case errors.Is(d.err, io.EOF):
				s.eofGen = s.gen
			case d.err != nil:
				s.fail(d.err)
			default:
				s.prevPTS, s.lastPTS = s.lastPTS, d.pts
				s.queue = append(s.queue, d)
			}
		default:
			return
		}
	}
}

func (s *Stream) advanceClock(now time.Time) {
	if !s.playing {
		return
	}
	if s.audio != nil && s.audio.Drained() {
		s.pos = s.audioPos + s.audio.Played()
		s.stopAudio()
		s.anchorAt, s.anchorPos = now, s.pos
	}
	if s.audio != nil {
		s.pos = s.audioPos + s.audio.Played()
		return
	}
	if s.anchorAt.IsZero() {
		s.anchorAt, s.anchorPos = now, s.pos
	}
	s.pos = s.anchorPos + now.Sub(s.anchorAt)
}

func (s *Stream) present() bool {
	changed := false
	for len(s.queue) > 0 {
		d := s.queue[0]
		if s.curGen == s.gen && d.pts > s.pos {
			break
		}
		s.dec.recycle(s.cur)
		s.cur, s.curGen = d.img, s.gen
		s.frameSeq++
		s.queue = s.queue[1:]
		changed = true
	}
	return changed
}

func (s *Stream) advance(now time.Time) {
	if s.closed {
		return
	}
	s.collect()
	if s.audio != nil {
		s.check(s.audio.Err())
	}
	wasPlaying := s.playing
	s.advanceClock(now)
	changed := s.present()
	if s.playing && s.eofGen == s.gen && len(s.queue) == 0 && s.pos >= s.end() {
		if s.loop {
			s.seekTo(0)
		} else {
			s.pos, s.playing, s.ended = s.end(), false, true
			s.stopAudio()
		}
	}
	if changed || wasPlaying || s.err != nil {
		s.notify()
	}
	s.sync()
}

func (s *Stream) active() bool {
	return !s.closed && s.err == nil && (s.playing || s.curGen != s.gen && s.eofGen != s.gen)
}

func (s *Stream) sync() {
	switch on := s.active(); {
	case on && s.ticker == nil:
		s.launchTicker()
	case !on && s.ticker != nil:
		s.ticker()
		s.ticker = nil
	}
}

func (s *Stream) launchTicker() {
	s.ticker = animclock.Launch([]animclock.Piece{{
		Dur:    tickSpan,
		Easing: func(t float64) float64 { return t },
		Fn: func(t float64) {
			if t >= 1 {
				s.ticker = nil
			}
			s.advance(animclock.Now())
		},
	}})
}

func (s *Stream) notify() {
	for _, sub := range slices.Clone(s.subs) {
		sub.fn()
	}
}
