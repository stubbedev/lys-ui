package media

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/stubbedev/lys-lib/media/aac"
	"github.com/stubbedev/lys-lib/media/demux"
	"github.com/stubbedev/lys-lib/media/opus"
	"github.com/stubbedev/lys-lib/media/vorbis"
	"github.com/stubbedev/lys-lib/pulse/native"
)

const (
	opusSeekPreRoll     = 80 * time.Millisecond
	aacSeekPreRoll      = aac.FrameSize
	vorbisSeekPreRoll   = 8192
	opusFamilyOffset    = 18
	opusFamilyAmbisonic = 255
)

// audioCodec decodes one track's packets to interleaved 16-bit PCM:
// aac, opus and vorbis decoders all are one.
type audioCodec interface {
	Decode(dst []int16, packet []byte) ([]int16, error)
	Reset()
}

// soundCodec is how a track's codec decodes and the PCM it decodes to.
type soundCodec struct {
	codec  audioCodec
	format AudioFormat
	// priming is decoder output at the stream's start the container did
	// not take out of the timestamps.
	priming time.Duration
	// preRoll is how far before a seek target decoding starts to
	// converge, beyond what the container says.
	preRoll time.Duration
}

var errNoSoundDecoder = errors.New("no decoder for the codec")

// openSoundCodec opens the decoder of track, an AAC, Opus or Vorbis
// track.
func openSoundCodec(track demux.Track) (soundCodec, error) {
	switch track.Codec {
	case demux.AAC:
		d, err := aac.NewDecoder(track.Config)
		if err != nil {
			return soundCodec{}, err
		}
		cfg := d.Config()
		layout, err := aacLayout(cfg.Channels)
		return soundCodec{
			codec:   d,
			format:  AudioFormat{Rate: cfg.SampleRate, Layout: layout},
			preRoll: samplesAt(aacSeekPreRoll, cfg.SampleRate) - track.SeekPreRoll,
		}, err
	case demux.Opus:
		d, err := opus.NewDecoder(track.Config)
		if err != nil {
			return soundCodec{}, err
		}
		layout := vorbisLayout(d.Channels())
		if track.Config[opusFamilyOffset] == opusFamilyAmbisonic {
			layout = auxLayout(d.Channels())
		}
		return soundCodec{
			codec:   d,
			format:  AudioFormat{Rate: opus.SampleRate, Layout: layout},
			priming: samplesAt(d.PreSkip(), opus.SampleRate) - track.CodecDelay,
			preRoll: opusSeekPreRoll - track.SeekPreRoll,
		}, nil
	case demux.Vorbis:
		id, comment, setup, err := demux.XiphHeaders(track.Config)
		if err != nil {
			return soundCodec{}, err
		}
		d, err := vorbis.NewDecoder(id, comment, setup)
		if err != nil {
			return soundCodec{}, err
		}
		return soundCodec{
			codec:   d,
			format:  AudioFormat{Rate: d.SampleRate(), Layout: vorbisLayout(d.Channels())},
			preRoll: samplesAt(vorbisSeekPreRoll, d.SampleRate()) - track.SeekPreRoll,
		}, nil
	}
	return soundCodec{}, errNoSoundDecoder
}

func samplesAt(n, rate int) time.Duration {
	return time.Duration(mulDiv(int64(n), int64(time.Second), int64(rate)))
}

var (
	aacLayouts = map[int]native.ChannelMap{
		1: {native.ChannelMono},
		2: {native.ChannelFrontLeft, native.ChannelFrontRight},
		3: {native.ChannelFrontCenter, native.ChannelFrontLeft, native.ChannelFrontRight},
		4: {native.ChannelFrontCenter, native.ChannelFrontLeft, native.ChannelFrontRight, native.ChannelRearCenter},
		5: {native.ChannelFrontCenter, native.ChannelFrontLeft, native.ChannelFrontRight, native.ChannelRearLeft, native.ChannelRearRight},
		6: {native.ChannelFrontCenter, native.ChannelFrontLeft, native.ChannelFrontRight, native.ChannelRearLeft, native.ChannelRearRight, native.ChannelLFE},
		8: {
			native.ChannelFrontCenter, native.ChannelFrontLeftOfCenter, native.ChannelFrontRightOfCenter, native.ChannelFrontLeft,
			native.ChannelFrontRight, native.ChannelRearLeft, native.ChannelRearRight, native.ChannelLFE,
		},
	}
	vorbisLayouts = [...]native.ChannelMap{
		1: {native.ChannelMono},
		2: {native.ChannelFrontLeft, native.ChannelFrontRight},
		3: {native.ChannelFrontLeft, native.ChannelFrontCenter, native.ChannelFrontRight},
		4: {native.ChannelFrontLeft, native.ChannelFrontRight, native.ChannelRearLeft, native.ChannelRearRight},
		5: {native.ChannelFrontLeft, native.ChannelFrontCenter, native.ChannelFrontRight, native.ChannelRearLeft, native.ChannelRearRight},
		6: {native.ChannelFrontLeft, native.ChannelFrontCenter, native.ChannelFrontRight, native.ChannelRearLeft, native.ChannelRearRight, native.ChannelLFE},
		7: {
			native.ChannelFrontLeft, native.ChannelFrontCenter, native.ChannelFrontRight, native.ChannelSideLeft,
			native.ChannelSideRight, native.ChannelRearCenter, native.ChannelLFE,
		},
		8: {
			native.ChannelFrontLeft, native.ChannelFrontCenter, native.ChannelFrontRight, native.ChannelSideLeft,
			native.ChannelSideRight, native.ChannelRearLeft, native.ChannelRearRight, native.ChannelLFE,
		},
	}
)

// aacLayout is the speakers of an AAC channel configuration, in the
// decoder's order: front centre first, then the front pairs, the
// surround pair and LFE.
func aacLayout(channels int) (native.ChannelMap, error) {
	l, ok := aacLayouts[channels]
	if !ok {
		return nil, fmt.Errorf("no speaker layout for %d AAC channels", channels)
	}
	return l, nil
}

// vorbisLayout is the speakers of n channels in Vorbis order, which
// Opus's mapping families 0 and 1 share; past eight they are unnamed.
func vorbisLayout(n int) native.ChannelMap {
	if n < len(vorbisLayouts) {
		return vorbisLayouts[n]
	}
	return auxLayout(n)
}

func auxLayout(n int) native.ChannelMap {
	l := make(native.ChannelMap, n)
	for i := range l {
		l[i] = native.ChannelAux0 + native.ChannelPosition(i)
	}
	return l
}

// sound is a file's sound track decoded in process: one demuxer of its
// own, which every PCM reader OpenAudio returns shares, the newest
// one reading and the ones before it closed.
type sound struct {
	path    string
	track   demux.Track
	codec   soundCodec
	mu      sync.Mutex
	file    *demux.File
	samples []int16
	reader  *pcmReader
	closed  bool
}

func openSound(path string, track demux.Track) (*sound, error) {
	codec, err := openSoundCodec(track)
	if err != nil {
		return nil, fmt.Errorf("media: %s: %s track %d: %w", path, track.Codec, track.Index, err)
	}
	file, err := demux.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("media: open the sound of %s: %w", path, err)
	}
	if err := file.Select(track.Index); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("media: %w", err)
	}
	return &sound{path: path, track: track, codec: codec, file: file}, nil
}

func (s *sound) open(t time.Duration) (*pcmReader, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errClosed
	}
	if s.reader != nil {
		s.reader.closed = true
	}
	if _, err := s.file.Seek(t - max(s.codec.preRoll, 0)); err != nil {
		return nil, fmt.Errorf("media: sound of %s from %v: %w", s.path, t, err)
	}
	s.codec.codec.Reset()
	s.reader = &pcmReader{s: s, start: t}
	return s.reader, nil
}

func (s *sound) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.reader != nil {
		s.reader.closed = true
	}
	return s.file.Close()
}

// pcmReader is a sound track from start as PCM in its AudioFormat.
type pcmReader struct {
	s       *sound
	start   time.Duration
	pending []int16
	closed  bool
}

// Read fills b with whole samples, decoding packets as it needs them;
// the decoder output presenting before start is dropped, so the PCM
// starts at start whatever the codec's delay and pre-roll.
func (r *pcmReader) Read(b []byte) (int, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.closed {
		return 0, errClosed
	}
	if len(b) < 2 {
		return 0, io.ErrShortBuffer
	}
	for len(r.pending) == 0 {
		if err := r.decode(); err != nil {
			return 0, err
		}
	}
	n := min(len(r.pending), len(b)/2)
	for i, v := range r.pending[:n] {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(v)) //nolint:gosec // a sample's two's complement bits
	}
	r.pending = r.pending[n:]
	return 2 * n, nil
}

func (r *pcmReader) decode() error {
	s := r.s
	p, err := s.file.ReadPacket()
	if errors.Is(err, io.EOF) {
		return io.EOF
	}
	if err != nil {
		return fmt.Errorf("media: sound of %s: %w", s.path, err)
	}
	s.samples, err = s.codec.codec.Decode(s.samples[:0], p.Data)
	if err != nil {
		return fmt.Errorf("media: sound of %s: %s packet at %v: %w", s.path, s.track.Codec, p.PTS, err)
	}
	channels := len(s.codec.format.Layout)
	frames := len(s.samples) / channels
	late := r.start - (p.PTS - max(s.codec.priming, 0))
	drop := 0
	if late > 0 {
		drop = int(min(mulDivCeil(int64(late), int64(s.codec.format.Rate), int64(time.Second)), int64(frames)))
	}
	r.pending = s.samples[drop*channels : frames*channels]
	return nil
}

func (r *pcmReader) Close() error {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	r.closed = true
	if s.reader == r {
		s.reader = nil
	}
	return nil
}
