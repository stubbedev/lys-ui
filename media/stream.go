package media

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/stubbedev/lys-lib/debug"
	"github.com/stubbedev/lys-lib/gpu"
	"github.com/stubbedev/lys-lib/render"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/internal/worker"
)

const (
	audioQueueDepth = 64
	audioStopLimit  = 5 * time.Second
)

// ErrNotSeekable reports a Seek on a Stream whose source cannot seek.
var ErrNotSeekable = errors.New("media: the stream cannot seek")

var errStreamClosed = errors.New("media: the stream is closed")

type subscription struct {
	id int
	fn func()
}

// Stream plays a Source, GTK's GtkMediaStream: it decodes a few frames
// ahead (a FrameSource's on a goroutine of its own, a TextureSource's
// packets read on one and decoded on the loop's GPU device), presents
// the frame due at the playback position on the loop's frame clock,
// and, when the source is Audible, plays the sound through PulseAudio
// and follows its clock so picture and sound stay together. Every
// method runs on the loop goroutine and waits on no file and no sound
// server: those are behind goroutines of their own. Subscribers hear
// every change: the frame, the position while playing, and the state.
type Stream struct {
	owner   anim.Owner
	name    string
	src     Source
	info    Info
	seeker  Seeker
	audible Audible
	openOut openAudioFunc
	// dec decodes a FrameSource's frames, tex a TextureSource's: one
	// of them is nil.
	dec *decoder
	tex *textureVideo

	deviceLost func(error)

	playing, ended, loop, closed bool

	pos       time.Duration
	anchorAt  time.Time
	anchorPos time.Duration

	gen, curGen, eofGen int
	cur                 *frameBuf
	frameSeq            uint64
	queue               []decoded
	lastPTS, prevPTS    time.Duration

	audio     *audioLink
	audioWork *worker.Queue
	audioPos  time.Duration
	volume    float64
	volumeSet bool
	muted     bool

	err     error
	subs    []subscription
	nextSub int
	ticker  anim.Cancel
}

// NewStream plays src, named name in the desktop's volume mixer, on
// owner's frame clock: an app.Application, or a widget, whose
// application's windows pace it once one shows the widget. It starts
// paused and decodes the first frame right away; Close releases the
// source and its decoders.
func NewStream(owner anim.Owner, src Source, name string) *Stream {
	return newStream(owner, src, name, openPulse)
}

func newStream(owner anim.Owner, src Source, name string, openOut openAudioFunc) *Stream {
	s := &Stream{owner: owner, name: name, src: src, info: src.Info(), openOut: openOut, curGen: -1, eofGen: -1, volume: 1, audioWork: worker.New(audioQueueDepth)}
	s.seeker, _ = src.(Seeker)
	s.audible, _ = src.(Audible)
	switch v := src.(type) {
	case TextureSource:
		s.tex = newTextureVideo(v)
	case FrameSource:
		s.dec = startDecoder(v, s.seeker, s.info)
	default:
		panic(undecodable(src))
	}
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
	volume := s.volume
	s.onAudio(func(out audioOut) error { return out.SetVolume(volume) })
	s.notify()
}

// Muted reports whether the sound is muted.
func (s *Stream) Muted() bool { return s.muted }

// SetMuted mutes or unmutes the sound.
func (s *Stream) SetMuted(on bool) {
	s.muted = on
	s.onAudio(func(out audioOut) error { return out.SetMuted(on) })
	s.notify()
}

// Closed reports whether Close was called.
func (s *Stream) Closed() bool { return s.closed }

// Err returns the failure that stopped the stream, nil while healthy.
func (s *Stream) Err() error { return s.err }

// Frame returns the frame to show, its RGBA nil before the first is
// decoded, after Close and for a stream decoding on the GPU (Texture),
// under its raster's identity: a pooled buffer's at a version that
// moves whenever its pixels do, a StillSource's frame at version 0,
// since its pixels never change. The image belongs to the stream and
// stays valid until the next change or Close. FrameSeq tells when the
// frame changed.
func (s *Stream) Frame() render.VersionedImage {
	switch {
	case s.cur == nil:
		return render.VersionedImage{}
	case s.cur.still:
		return render.VersionedImage{RGBA: s.cur.img, ID: s.cur.id}
	case !s.cur.rgba:
		if err := s.dec.convert(s.cur); err != nil {
			s.fail(err)
			return render.VersionedImage{}
		}
	}
	return render.VersionedImage{RGBA: s.cur.img, ID: s.cur.id, Version: s.frameSeq}
}

// YUV reports the colour of the frames FrameYUV returns, false when
// the source decodes no YUV and FrameYUV returns nil.
func (s *Stream) YUV() (YUVColor, bool) {
	if s.dec == nil || s.dec.yuv == nil {
		return YUVColor{}, false
	}
	return s.dec.yuv.YUV()
}

// FrameYUV returns the frame to show as its decoded YUV planes, nil
// before the first is decoded, after Close and for a source decoding
// no YUV. It belongs to the stream and stays valid until the next
// change or Close; FrameSeq tells when it changed.
func (s *Stream) FrameYUV() *YUVFrame {
	if s.cur == nil {
		return nil
	}
	return s.cur.yuv
}

// SkipRGBA stops converting the frames of a source decoding YUV to
// RGBA as they are decoded, for a viewer that shows FrameYUV; off
// resumes it. Frame keeps working either way: it converts the frame
// it returns when the decoder did not.
func (s *Stream) SkipRGBA(on bool) {
	if s.dec != nil {
		s.dec.skipRGBA.Store(on)
	}
}

// DecodesOnGPU reports whether the stream plays a TextureSource: its
// frames are GPU textures (Texture), decoded on the device UseDevice
// names, and Frame and FrameYUV return none.
func (s *Stream) DecodesOnGPU() bool { return s.tex != nil }

// UseDevice decodes a TextureSource's frames on dev, the device that
// draws them, from the playback position on; until then the stream
// shows no frame. A stream decodes on one device at a time: naming
// another lets go of the one before, as ReleaseDevice does. A device
// that does not decode the video's codec fails the stream with an error
// naming the codec and the file, which UseDevice returns too.
//
// lost is called on the loop when decoding finds dev lost
// (gpu.ErrDeviceLost), the stream having let go of it as ReleaseDevice
// does: whoever named the device has its owner open the next and names
// that. With a nil lost a lost device fails the stream.
func (s *Stream) UseDevice(dev *gpu.Device, lost func(error)) error {
	switch {
	case s.tex == nil:
		return fmt.Errorf("media: %s decodes its frames on the CPU", s.name)
	case s.closed:
		return errStreamClosed
	case s.err != nil:
		return s.err
	case s.tex.attached() && s.tex.dev == dev:
		s.deviceLost = lost
		return nil
	}
	s.releaseDevice()
	if s.tex.rd != nil {
		s.gen++
		s.lastPTS, s.prevPTS = 0, 0
	}
	s.deviceLost = lost
	if err := s.tex.attach(dev, s.gen, s.pos); err != nil {
		s.textureFailed(err)
		s.notify()
		s.sync()
		return s.err
	}
	s.sync()
	return nil
}

// ReleaseDevice lets go of the device UseDevice named, which is going
// away: the frames decoded on it and its decoder are released, and the
// stream goes on as it was, playing or paused, its sound and its clock
// untouched, showing no frame until UseDevice names another device.
// Decoding resumes there at the playback position. A stream decoding
// on no device stays as it is.
func (s *Stream) ReleaseDevice() {
	if s.tex == nil || !s.tex.attached() {
		return
	}
	s.releaseDevice()
	s.notify()
	s.sync()
}

func (s *Stream) releaseDevice() {
	s.tex.detach()
	s.deviceLost = nil
}

func (s *Stream) textureFailed(err error) {
	lost := s.deviceLost
	if lost == nil || !errors.Is(err, gpu.ErrDeviceLost) {
		s.fail(err)
		return
	}
	s.releaseDevice()
	lost(err)
}

// Texture returns the frame to show of a stream decoding on the GPU,
// nil before the first is decoded, after Close and for a stream
// decoding on the CPU. Its picture has finished decoding: draw it
// (gpu.Pass.DrawFrame) on the device UseDevice named, and nothing
// waits for the decoder. It belongs to the stream and stays valid until
// the next change or Close; FrameSeq tells when it changed.
func (s *Stream) Texture() *gpu.VideoFrame {
	if s.tex == nil {
		return nil
	}
	return s.tex.cur
}

// Fail stops the stream with err, what a viewer that cannot show its
// frames calls; Err returns it. A stream that failed already keeps its
// first error.
func (s *Stream) Fail(err error) {
	s.fail(err)
	s.notify()
	s.sync()
}

// FrameSeq moves whenever Frame shows another frame.
func (s *Stream) FrameSeq() uint64 { return s.frameSeq }

// Subscribe calls fn on the loop after every change; cancel stops it.
func (s *Stream) Subscribe(fn func()) (cancel func()) {
	s.nextSub++
	id := s.nextSub
	s.subs = append(slices.Clip(s.subs), subscription{id: id, fn: fn})
	return func() {
		s.subs = slices.DeleteFunc(slices.Clone(s.subs), func(sub subscription) bool { return sub.id == id })
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
	s.onAudio(func(out audioOut) error {
		out.Pause()
		return nil
	})
	if debug.Enabled {
		debug.Log("media", "paused at frame %d", s.frameSeq)
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

// Close stops playback and releases the source and decoders. It returns
// at once: a decoder or sound server that takes its time to go, or never
// goes, holds up its own goroutine and not the caller's. Wait joins it.
func (s *Stream) Close() {
	if s.closed {
		return
	}
	s.closed, s.playing = true, false
	s.stopAudio()
	s.sync()
	// The frames go with the decoder's buffers: nothing reads one
	// after Close.
	s.cur, s.queue = nil, nil
	if s.tex != nil {
		s.tex.close()
	} else {
		s.dec.close()
	}
	s.notify()
}

// Wait blocks until a closed Stream let go of everything it held and
// returns the failure closing its source met. It is for the end of a
// program, off the loop goroutine: a source that never exits holds it
// up.
func (s *Stream) Wait() error {
	if !s.audioWork.Settle(audioStopLimit) {
		return fmt.Errorf("media: close %s: the sound server did not let go", s.name)
	}
	var err error
	if s.tex != nil {
		<-s.tex.closed
		err = s.tex.closeErr
	} else {
		<-s.dec.closed
		err = s.dec.closeErr
	}
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
	if s.tex != nil {
		if err := s.tex.seek(s.gen, t); err != nil {
			s.textureFailed(err)
		}
	} else {
		s.dec.requestSeek(seekRequest{gen: s.gen, t: t})
	}
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
		s.onAudio(func(out audioOut) error {
			out.Resume()
			return nil
		})
		return
	}
	link := &audioLink{}
	s.audio, s.audioPos = link, s.pos
	pos, volume, volumeSet, muted, format := s.pos, s.volume, s.volumeSet, s.muted, s.audible.AudioFormat()
	s.postAudio(func() {
		pcm, err := s.audible.OpenAudio(pos)
		if err != nil {
			link.fail(err)
			return
		}
		out, err := s.openOut(pcm, format, s.name)
		if err != nil {
			_ = pcm.Close()
			link.fail(err)
			return
		}
		link.ready.Store(&openedAudio{out: out, pcm: pcm})
		if volumeSet {
			link.fail(out.SetVolume(volume))
		}
		if muted {
			link.fail(out.SetMuted(true))
		}
	})
}

func (s *Stream) onAudio(fn func(out audioOut) error) {
	link := s.audio
	if link == nil {
		return
	}
	s.postAudio(func() {
		if o := link.ready.Load(); o != nil {
			link.fail(fn(o.out))
		}
	})
}

func (s *Stream) stopAudio() {
	link := s.audio
	if link == nil {
		return
	}
	s.audio = nil
	s.postAudio(func() {
		if o := link.ready.Load(); o != nil {
			o.out.Close()
			_ = o.pcm.Close()
		}
	})
}

func (s *Stream) postAudio(work func()) {
	if !s.audioWork.Post(work) {
		s.fail(fmt.Errorf("media: %s: the sound server stopped answering", s.name))
	}
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
	if s.tex != nil {
		s.collectTextures()
		return
	}
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

func (s *Stream) collectTextures() {
	if !s.tex.attached() || s.err != nil {
		return
	}
	queued, ended, err := s.tex.pump(s.gen)
	if err != nil {
		s.textureFailed(err)
		return
	}
	for _, f := range queued {
		s.prevPTS, s.lastPTS = s.lastPTS, f.PTS()
	}
	if ended {
		s.eofGen = s.gen
	}
}

func (s *Stream) advanceClock(now time.Time) {
	if !s.playing {
		return
	}
	if s.audio != nil && s.audio.drained() {
		s.pos = s.audioPos + s.audio.played()
		s.stopAudio()
		s.anchorAt, s.anchorPos = now, s.pos
	}
	if s.audio != nil {
		s.pos = s.audioPos + s.audio.played()
		return
	}
	if s.anchorAt.IsZero() {
		s.anchorAt, s.anchorPos = now, s.pos
	}
	s.pos = s.anchorPos + now.Sub(s.anchorAt)
}

func (s *Stream) present() bool {
	if s.tex != nil {
		return s.presentTexture()
	}
	shown := 0
	for _, d := range s.queue {
		if s.curGen == s.gen && d.pts > s.pos {
			break
		}
		s.dec.recycle(s.cur)
		s.cur, s.curGen = d.img, s.gen
		s.frameSeq++
		shown++
	}
	s.queue = s.queue[:copy(s.queue, s.queue[shown:])]
	return shown > 0
}

func (s *Stream) presentTexture() bool {
	shown := 0
	for _, f := range s.tex.queue {
		if !s.frameDue(f) {
			break
		}
		decoded, err := s.tex.decoded(f)
		if err != nil {
			s.textureFailed(fmt.Errorf("media: the video frame at %v: %w", f.PTS(), err))
			return false
		}
		if !decoded {
			break
		}
		s.curGen = s.gen
		shown++
	}
	s.tex.show(shown)
	s.frameSeq += uint64(shown)
	return shown > 0
}

// videoDone reports whether the frames decoded to the end of the
// video have all been shown; a TextureSource's stream never given a
// device to decode on shows none and ends by its clock alone, and one
// between two devices ends once the next has decoded to the end.
func (s *Stream) videoDone() bool {
	if s.tex != nil {
		return s.tex.rd == nil || s.eofGen == s.gen && len(s.tex.queue) == 0
	}
	return s.eofGen == s.gen && len(s.queue) == 0
}

func (s *Stream) frameDue(f *gpu.VideoFrame) bool {
	return s.curGen != s.gen || f.PTS() <= s.pos
}

// awaitingFrame reports whether the frame at the playback position is
// still to come: to be decoded, or, on the GPU, decoding still.
func (s *Stream) awaitingFrame() bool {
	switch {
	case s.tex == nil:
		return s.curGen != s.gen && s.eofGen != s.gen
	case !s.tex.attached():
		return false
	case len(s.tex.queue) > 0:
		return s.frameDue(s.tex.queue[0])
	}
	return s.curGen != s.gen && s.eofGen != s.gen
}

func (s *Stream) advance(now time.Time) {
	if s.closed {
		return
	}
	s.collect()
	if s.audio != nil {
		s.check(s.audio.err())
	}
	wasPlaying := s.playing
	s.advanceClock(now)
	changed := s.present()
	if s.playing && s.videoDone() && s.pos >= s.end() {
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
	return !s.closed && s.err == nil && (s.playing || s.awaitingFrame())
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
	s.ticker = anim.Drive(s.owner, func(time.Time) bool {
		s.advance(animclock.Now())
		return true
	})
}

func (s *Stream) notify() {
	for _, sub := range s.subs {
		sub.fn()
	}
}
